package flow

import (
	"math"
	"testing"
	"time"
)

func TestV9ChargeExpandedFields(t *testing.T) {
	for _, tc := range []struct {
		remaining, rows, fields int
		wantRemaining           int
		wantFits                bool
	}{
		{math.MaxInt, math.MaxInt, 2, math.MaxInt, false},
		{math.MaxInt, 1, math.MaxInt, 0, true},
		{0, 1, 1, 0, false},
		{7, 3, 2, 1, true},
	} {
		remaining, fits := v9ChargeExpandedFields(tc.remaining, tc.rows, tc.fields)
		if remaining != tc.wantRemaining || fits != tc.wantFits {
			t.Errorf("%+v: got remaining=%d fits=%t", tc, remaining, fits)
		}
	}
}

func TestV9ExpansionBudgetConfig(t *testing.T) {
	if got := ErrV9ExpansionBudget.Error(); got != "NetFlow v9 expanded-field budget exceeded" {
		t.Fatalf("unexpected refusal message: %q", got)
	}
	fixture, key, _ := testV9Stream(t, 1, 1)
	for _, budget := range []int{-1, 0, 1, math.MaxInt} {
		limits := fixture.limits
		limits.MaxExpandedFields = budget
		stream, err := NewV9Stream(key, limits)
		if budget <= 0 {
			if stream != nil || err != ErrV9StreamConfig {
				t.Errorf("budget %d: got %v, %v", budget, stream, err)
			}
		} else if err != nil || stream == nil {
			t.Errorf("budget %d: got %v, %v", budget, stream, err)
		}
	}
}

func TestV9ExpansionBudget(t *testing.T) {
	// Each set has one row and two descriptors, including one zero-width
	// descriptor. The positive row width keeps its padding interpretation unique.
	def := func(id uint16) []byte {
		return v9TemplateTestSet(0, v9TemplateTestWords(id, 2, 7, 0, 8, 4))
	}
	data := func(id uint16) []byte {
		return v9TemplateTestSet(id, []byte{1, 2, 3, 4})
	}
	for _, name := range []string{"aggregate", "malformed tail", "gap", "expiry", "capacity", "continuity"} {
		t.Run(name, func(t *testing.T) {
			fixture, key, at := testV9Stream(t, 3, 6)
			limits := fixture.limits
			limits.MaxExpandedFields = 4
			s, err := NewV9Stream(key, limits)
			if err != nil {
				t.Fatal(err)
			}
			decode := func(when time.Time, wire []byte, want error) *V9RecordPacket {
				t.Helper()
				p, err := s.Decode(key, when, wire)
				if err != want || (p == nil) != (want != nil) {
					t.Fatalf("got packet=%+v err=%v, want err=%v", p, err, want)
				}
				return p
			}
			check := func(p *V9RecordPacket, want ...V9SetStatus) {
				t.Helper()
				if len(p.Sets) != len(want) {
					t.Fatalf("got %d sets, want %d", len(p.Sets), len(want))
				}
				for i, status := range want {
					set := p.Sets[i]
					if set.Status != status {
						t.Fatalf("set %d: got %v, want %v", i, set.Status, status)
					}
					if status == V9Decoded && (set.Count != 1 || set.Layout == nil || set.Layout.FieldCount() != 2 || set.Layout.Field(0).Length != 0) {
						t.Fatalf("changed row layout: %+v", set)
					}
				}
			}
			initial := v9WirePacket(10, 100, 10, def(256), def(257), data(256), data(257))
			check(decode(at, initial, nil), V9Decoded, V9Decoded)
			twoRows := v9TemplateTestSet(256, []byte{1, 2, 3, 4, 5, 6, 7, 8})
			over := [][]byte{data(256), def(256), def(258), data(258), data(257)}
			switch name {
			case "aggregate":
				wire := v9WirePacket(11, 101, 10, twoRows, def(256), def(258), data(258))
				decode(at, wire, ErrV9ExpansionBudget)
				decode(at, wire, ErrV9ExpansionBudget)
				decode(at, initial, ErrV9LatePacket)
				p := decode(at, v9WirePacket(12, 102, 10, data(256), data(257), data(258)), nil)
				check(p, V9TemplateMissing, V9Decoded, V9TemplateMissing)
				if p.MissingPackets != 0 {
					t.Fatalf("refused sequence counted as a gap: %d", p.MissingPackets)
				}
			case "malformed tail":
				malformed := v9TemplateTestSet(258, []byte{1})
				decode(at, v9WirePacket(11, 101, 10, append(over, malformed)...), ErrMalformedV9Record)
				check(decode(at, v9WirePacket(11, 101, 10, data(256), data(257), data(258)), nil),
					V9Decoded, V9Decoded, V9TemplateMissing)
			case "gap":
				wire := v9WirePacket(13, 103, 10, def(258), data(258), data(258), data(258))
				decode(at, wire, ErrV9ExpansionBudget)
				check(decode(at, v9WirePacket(14, 104, 10, data(256), data(257), data(258)), nil),
					V9TemplateMissing, V9TemplateMissing, V9TemplateMissing)
			case "expiry":
				check(decode(at.Add(limits.Timeout), v9WirePacket(11, 101, 10, data(256), data(256), data(257), data(258)), nil),
					V9TemplateExpired, V9TemplateExpired, V9TemplateExpired, V9TemplateMissing)
			case "capacity":
				wire := v9WirePacket(11, 101, 10, append(over, def(259))...)
				decode(at, wire, ErrV9StreamCapacity)
				decode(at, wire, ErrV9StreamCapacity)
			case "continuity":
				wire := v9WirePacket(11, 101, 10, over...)
				decode(at, wire, ErrV9ExpansionBudget)
				disputed := [][]byte{def(258), data(258), data(258), data(258)}
				decode(at, v9WirePacket(11, 101, 10, disputed...), ErrV9SequenceCollision)
				decode(at, v9WirePacket(12, 1, 11, disputed...), ErrV9EpochRequired)
				decode(at, v9WirePacket(13, 102, 11, def(257)), ErrV9EpochRequired)
			}
		})
	}
}

func TestV9ExpansionBudgetSkipsAmbiguousSets(t *testing.T) {
	fixture, key, at := testV9Stream(t, 2, 2)
	limits := fixture.limits
	limits.MaxExpandedFields = 1
	s, err := NewV9Stream(key, limits)
	if err != nil {
		t.Fatal(err)
	}
	wire := v9WirePacket(1, 0, 0,
		v9TemplateTestSet(0, v9TemplateTestWords(256, 1, 8, 1, 257, 1, 8, 4)),
		v9TemplateTestSet(256, []byte{1, 2, 3, 4}),
		v9TemplateTestSet(257, []byte{1, 2, 3, 4}),
	)
	p, err := s.Decode(key, at, wire)
	if err != nil || p == nil || len(p.Sets) != 2 ||
		p.Sets[0].Status != V9PaddingAmbiguous || p.Sets[1].Status != V9Decoded || p.Sets[1].Count != 1 {
		t.Fatalf("got packet=%+v err=%v", p, err)
	}
}
