package flow

import (
	"encoding/binary"
	"errors"
	"reflect"
	"slices"
	"testing"
)

func v9TemplateTestPacket(sets ...[]byte) []byte {
	packet := []byte{0, 9, 0xff, 0xff, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xde, 0xad, 0xbe, 0xef}
	for _, set := range sets {
		packet = append(packet, set...)
	}
	return packet
}

func v9TemplateTestSet(id uint16, body []byte) []byte {
	set := make([]byte, 4, 4+len(body))
	binary.BigEndian.PutUint16(set, id)
	binary.BigEndian.PutUint16(set[2:], uint16(cap(set)))
	return append(set, body...)
}

func v9TemplateTestWords(words ...uint16) []byte {
	var wire []byte
	for _, word := range words {
		wire = binary.BigEndian.AppendUint16(wire, word)
	}
	return wire
}

func TestParseV9TemplatesRFCExamples(t *testing.T) {
	// RFC 3954 sections 11.2 and 11.4, including the example's two pad bytes.
	packet := v9TemplateTestPacket(
		[]byte{0, 0, 0, 28, 1, 0, 0, 5, 0, 8, 0, 4, 0, 12, 0, 4, 0, 15, 0, 4, 0, 2, 0, 4, 0, 1, 0, 4},
		[]byte{0, 1, 0, 24, 1, 1, 0, 4, 0, 8, 0, 3, 0, 2, 0, 41, 0, 2, 0, 42, 0, 2, 0, 0},
	)
	original := slices.Clone(packet)
	got, err := ParseV9Templates(packet)
	if err != nil || !slices.Equal(packet, original) {
		t.Fatalf("decode failed or modified input: %v", err)
	}
	clear(packet) // Exporter receive buffer can be reused.
	want := &V9TemplatePacket{SourceID: 0xdeadbeef, Templates: []V9Template{
		{ID: 256, Kind: V9DataTemplate, Fields: []V9TemplateField{{8, 4}, {12, 4}, {15, 4}, {2, 4}, {1, 4}}},
		{ID: 257, Kind: V9OptionsTemplate, ScopeFields: []V9TemplateField{{3, 2}}, Fields: []V9TemplateField{{41, 2}, {42, 2}}},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseV9TemplatesRepeatedAndOpaque(t *testing.T) {
	ordinary := v9TemplateTestSet(0, v9TemplateTestWords(65535, 2, 0x8001, 0, 0x8001, 65535, 256, 1, 0, 9))
	options := v9TemplateTestSet(1, v9TemplateTestWords(65535, 4, 0, 0xffff, 0, 256, 0, 4, 0, 65535))
	opaque := v9TemplateTestSet(255, []byte{1, 0, 0, 0, 0, 0, 0})
	data := v9TemplateTestSet(65535, []byte{0xff})
	packet := v9TemplateTestPacket(data, ordinary, opaque, options, ordinary)
	fields := []V9TemplateField{{0x8001, 0}, {0x8001, 65535}}
	first := V9Template{ID: 65535, Kind: V9DataTemplate, Fields: fields}
	second := V9Template{ID: 256, Kind: V9DataTemplate, Fields: []V9TemplateField{{0, 9}}}
	want := []V9Template{
		first, second,
		{ID: 65535, Kind: V9OptionsTemplate, ScopeFields: []V9TemplateField{{65535, 0}}},
		{ID: 256, Kind: V9OptionsTemplate, Fields: []V9TemplateField{{0, 65535}}},
		first, second,
	}
	got, err := ParseV9Templates(packet)
	if err != nil || !reflect.DeepEqual(got.Templates, want) {
		t.Fatalf("got %+v (%v), want %+v", got, err, want)
	}
	got.Templates[0].Fields[0].Length++
	got.Templates[2].ScopeFields = append(got.Templates[2].ScopeFields, V9TemplateField{})
	if !reflect.DeepEqual(got.Templates[4], first) || !reflect.DeepEqual(got.Templates[3], want[3]) {
		t.Fatal("mutating a template changed another definition")
	}
	for _, count := range []uint16{0, 1, 65535} {
		packet := v9TemplateTestPacket(opaque, data)
		binary.BigEndian.PutUint16(packet[2:], count)
		got, err := ParseV9Templates(packet)
		if err != nil || got == nil || got.SourceID != 0xdeadbeef || len(got.Templates) != 0 {
			t.Fatalf("opaque-only packet with count %d: %+v, %v", count, got, err)
		}
	}
}

func TestParseV9TemplatesOptionsAlignment(t *testing.T) {
	record := v9TemplateTestWords(257, 4, 4, 1, 4, 34, 4)
	// The preceding set can have any wire length. Alignment is relative
	// to the start of the datagram and padding is optional.
	for precedingBytes := 0; precedingBytes < 4; precedingBytes++ {
		preceding := v9TemplateTestSet(511, make([]byte, precedingBytes))
		for n := 0; n <= 3; n++ {
			body := append(slices.Clone(record), []byte{0xbd, 0xba, 0xff}[:n]...)
			options := v9TemplateTestSet(1, body)
			packet := v9TemplateTestPacket(preceding, options, v9TemplateTestSet(2, nil))
			got, err := ParseV9Templates(packet)
			optionsEnd := 20 + len(preceding) + len(options)
			if n == 0 || optionsEnd%4 == 0 {
				if err != nil || len(got.Templates) != 1 {
					t.Fatalf("prefix %d, pad %d: %+v, %v", precedingBytes, n, got, err)
				}
			} else if got != nil || !errors.Is(err, ErrMalformedV9Template) {
				t.Fatalf("unaligned prefix %d, pad %d: %+v, %v", precedingBytes, n, got, err)
			}
		}
	}
}

func TestParseV9TemplatesAtomicErrors(t *testing.T) {
	words := v9TemplateTestWords
	valid := words(256, 1, 4, 1)
	validOption := words(300, 4, 4, 1, 4, 8, 4)
	for _, tc := range []struct {
		name string
		id   uint16
		body []byte
	}{
		{"empty ordinary set", 0, nil},
		{"empty options set", 1, nil},
		{"only options padding", 1, []byte{0, 0}},
		{"partial ordinary header", 0, []byte{1, 0, 0}},
		{"partial option header", 1, words(300, 4)},
		{"reserved ordinary ID", 0, words(255, 1, 8, 4)},
		{"reserved option ID", 1, words(1, 4, 0, 1, 1)},
		{"fieldless ordinary", 0, words(256, 0)},
		{"fieldless options", 1, words(256, 0, 0)},
		{"ordinary field count overrun", 0, words(256, 65535, 8, 4)},
		{"ordinary trailing byte", 0, append(slices.Clone(valid), 0)},
		{"options lengths must each align", 1, words(300, 2, 6, 1, 4, 8, 4)},
		{"option length misaligned", 1, words(300, 4, 2, 1, 4, 8)},
		{"option scope length overrun", 1, words(300, 8, 0, 1, 4)},
		{"option field length overrun", 1, words(300, 4, 8, 1, 4, 8, 4)},
		{"option combined length overflows uint16", 1, words(300, 32768, 32768)},
		{"ordinary second record truncated", 0, append(slices.Clone(valid), 1, 1, 0, 1)},
		{"options second record truncated", 1, append(slices.Clone(validOption), 1, 2, 0, 4)},
		{"padding between options", 1, append(append(slices.Clone(validOption), 0, 0), validOption...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			packet := v9TemplateTestPacket(v9TemplateTestSet(0, valid), v9TemplateTestSet(tc.id, tc.body), v9TemplateTestSet(300, nil))
			got, err := ParseV9Templates(packet)
			if got != nil || !errors.Is(err, ErrMalformedV9Template) {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
	packet := v9TemplateTestPacket(v9TemplateTestSet(0, valid))
	for _, malformed := range [][]byte{nil, packet[:19], append(slices.Clone(packet), 0xff), make([]byte, 65528)} {
		if got, err := ParseV9Templates(malformed); got != nil || !errors.Is(err, ErrMalformedV9Envelope) {
			t.Fatalf("malformed envelope: %+v, %v", got, err)
		}
	}
	packet[1] = 10
	if got, err := ParseV9Templates(packet); got != nil || !errors.Is(err, ErrUnsupportedV9Version) {
		t.Fatalf("wrong version: %+v, %v", got, err)
	}
}

func TestParseV9TemplatesLargestDatagram(t *testing.T) {
	const count = 16373
	body := make([]byte, 4+count*4)
	binary.BigEndian.PutUint16(body, 65535)
	binary.BigEndian.PutUint16(body[2:], count)
	binary.BigEndian.PutUint16(body[len(body)-4:], 40000)
	binary.BigEndian.PutUint16(body[len(body)-2:], 65000)
	packet := v9TemplateTestPacket(v9TemplateTestSet(2, []byte{0, 0, 0}), v9TemplateTestSet(0, body))
	if len(packet) != 65527 {
		t.Fatal("invalid boundary fixture")
	}
	got, err := ParseV9Templates(packet)
	if err != nil || len(got.Templates) != 1 || len(got.Templates[0].Fields) != count ||
		got.Templates[0].Fields[count-1] != (V9TemplateField{40000, 65000}) {
		t.Fatalf("maximum-size field vector: %v", err)
	}
}

func FuzzParseV9Templates(f *testing.F) {
	f.Add(v9MixedPacket())
	f.Add(v9TemplateTestPacket(v9TemplateTestSet(1, v9TemplateTestWords(256, 32768, 32768))))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, datagram []byte) {
		got, err := ParseV9Templates(datagram)
		if err != nil {
			if got != nil || !(errors.Is(err, ErrMalformedV9Template) || errors.Is(err, ErrMalformedV9Envelope) || errors.Is(err, ErrUnsupportedV9Version)) {
				t.Fatalf("unexpected failure: %+v, %v", got, err)
			}
			return
		}
		if got == nil || len(datagram) < 24 || got.SourceID != binary.BigEndian.Uint32(datagram[16:20]) {
			t.Fatal("invalid source context")
		}
		minimum := 20
		for _, def := range got.Templates {
			count := len(def.ScopeFields) + len(def.Fields)
			if def.ID < 256 || count == 0 {
				t.Fatal("invalid template ID or field count")
			}
			minimum += 4*count + 4
			switch def.Kind {
			case V9DataTemplate:
				if len(def.ScopeFields) != 0 {
					t.Fatal("ordinary template has scope")
				}
			case V9OptionsTemplate:
				minimum += 2
			default:
				t.Fatal("unknown template kind")
			}
		}
		if minimum > len(datagram) {
			t.Fatal("definitions exceed datagram")
		}
	})
}
