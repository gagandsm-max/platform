package flow

import (
	"errors"
	"slices"
	"testing"
)

func TestV9OrderedRowsAndFieldOwnership(t *testing.T) {
	s, k, at := testV9Stream(t, 4, 10)
	before := v9TemplateTestSet(256, []byte{0, 0, 0, 0})
	ordinary := v9TemplateTestSet(0, v9TemplateTestWords(256, 2, 60000, 0, 60000, 4))
	// Scope type 60000 and option type 60000 remain independent descriptors.
	options := v9TemplateTestSet(1, v9TemplateTestWords(256, 4, 8, 60000, 1, 60000, 0, 8, 3))
	raw := []byte{9, 8, 7, 6, 5, 4, 3, 2}
	data := v9TemplateTestSet(256, raw)
	wire := v9WirePacket(3, 100, 900, before, ordinary, data, options, data)
	pkt, err := s.Decode(k, at, wire)
	if err != nil {
		t.Fatal(err)
	}
	clear(wire)
	if pkt.UnixSecs != 900 || pkt.SysUpTime != 100 {
		t.Fatalf("wrong exporter clocks: %+v", pkt)
	}
	if pkt.Key != k || pkt.ReceivedAt != at || pkt.Timeout != s.limits.Timeout || pkt.SequenceNumber != 3 || len(pkt.Sets) != 3 || pkt.Sets[0].Status != V9TemplateMissing || pkt.Sets[0].VisitRow(0, nil) {
		t.Fatalf("unexpected packet %+v", pkt)
	}
	for i, set := range pkt.Sets[1:] {
		if set.Count != 2 || !slices.Equal(raw, set.Raw) || set.RefreshedAt != at {
			t.Fatalf("set %d: %+v", i, set)
		}
		var lengths []int
		var types []uint16
		var values []byte
		if !set.VisitRow(1, func(field V9TemplateField, value []byte) bool {
			if len(value) != cap(value) {
				t.Fatalf("field %d leaks neighboring capacity", field.Type)
			}
			lengths = append(lengths, len(value))
			types = append(types, field.Type)
			values = append(values, value...)
			return true
		}) {
			t.Fatal("row rejected")
		}
		if !slices.Equal(values, raw[4:]) {
			t.Fatalf("wrong row %x", values)
		}
		if i == 0 {
			if set.Layout.Kind() != V9DataTemplate || set.Layout.ScopeCount() != 0 || !slices.Equal(lengths, []int{0, 4}) || !slices.Equal(types, []uint16{60000, 60000}) {
				t.Fatal("ordinary field order")
			}
		} else if set.Layout.Kind() != V9OptionsTemplate || set.Layout.ScopeCount() != 1 || !slices.Equal(lengths, []int{1, 0, 3}) || !slices.Equal(types, []uint16{60000, 60000, 8}) {
			t.Fatal("option field order")
		}
		visits := 0
		if !set.VisitRow(0, func(V9TemplateField, []byte) bool { visits++; return false }) || visits != 1 {
			t.Fatal("early termination")
		}
		check := func(set V9RecordSet, index int) {
			t.Helper()
			if set.VisitRow(index, func(V9TemplateField, []byte) bool { t.Fatal("unexpected callback"); return false }) {
				t.Fatal("invalid geometry accepted")
			}
		}
		check(set, -1)
		check(set, 2)
		cut := set
		cut.Raw = cut.Raw[:7]
		check(cut, 0)
		cut = set
		cut.Count = int(^uint(0) >> 1)
		check(cut, 0)
		cut = set
		cut.Status = V9TemplateMissing
		check(cut, 0)
	}
	*pkt.Sets[2].Layout = V9RecordLayout{}
	clear(pkt.Sets[2].Raw)
	again, err := s.Decode(k, at, v9WirePacket(4, 100, 900, data))
	if err != nil || again.Sets[0].Layout.ScopeCount() != 1 || !slices.Equal(again.Sets[0].Raw, raw) {
		t.Fatal("result mutated cached layout or input")
	}
}

func TestV9DataPaddingCandidates(t *testing.T) {
	for _, tc := range []struct {
		name        string
		width       int
		prefix, raw []byte
		count       int
		status      V9SetStatus
		wantErr     error
	}{
		{"unaligned unpadded", 2, []byte{0}, []byte{1, 2, 3, 4}, 2, V9Decoded, nil},
		{"one byte followed by alignment padding", 3, nil, []byte{1, 2, 3, 0xfe}, 1, V9Decoded, nil},
		{"short ambiguous", 2, nil, []byte{1, 2, 3, 4}, 0, V9PaddingAmbiguous, nil},
		{"positive rows only", 2, []byte{0, 0}, []byte{1, 2}, 1, V9Decoded, nil},
		{"nonaligning trailing octet", 4, nil, []byte{1, 2, 3, 4, 0}, 0, 0, ErrMalformedV9Record},
		{"no row fits", 8, nil, []byte{0, 0, 0, 0}, 0, 0, ErrMalformedV9Record},
		{"empty set", 1, nil, nil, 0, 0, ErrMalformedV9Record},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, k, at := testV9Stream(t, 2, 2)
			tpl := v9TemplateTestSet(0, v9TemplateTestWords(256, 1, 8, uint16(tc.width)))
			wire := v9WirePacket(1, 0, 0, tpl, v9TemplateTestSet(8, tc.prefix), v9TemplateTestSet(256, tc.raw))
			pkt, err := s.Decode(k, at, wire)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error %v want %v", err, tc.wantErr)
			}
			if err != nil {
				if pkt != nil {
					t.Fatal("partial packet")
				}
				return
			}
			set := pkt.Sets[0]
			if set.Status != tc.status || set.Count != tc.count || !slices.Equal(set.Raw, tc.raw[:tc.count*tc.width]) {
				t.Fatalf("unexpected raw set %+v", set)
			}
			if tc.count == 0 && (set.Layout != nil || len(set.Raw) != 0) {
				t.Fatal("ambiguous rows leaked")
			}
		})
	}
}
