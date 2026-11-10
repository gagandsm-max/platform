package counter

import (
	"math"
	"testing"
	"time"
)

func FuzzDeriveExactDeltaDomain(f *testing.F) {
	f.Add(uint64(0), uint64(1), uint8(32), true, int64(1000), int64(1010))
	f.Add(uint64(math.MaxUint32-1), uint64(3), uint8(32), true, int64(1000), int64(1001))
	f.Add(uint64(math.MaxUint64-7), uint64(4), uint8(64), true, int64(1000), int64(1002))
	f.Add(uint64(1000), uint64(10), uint8(64), false, int64(1000), int64(1010))
	f.Add(uint64(1<<60), uint64(1<<60)+99, uint8(64), true, int64(5), int64(15))

	f.Fuzz(func(t *testing.T, prevVal, currVal uint64, widthBits uint8, continuous bool, prevUnix, currUnix int64) {
		var width Width
		switch widthBits {
		case 32:
			width = Width32
		case 64:
			width = Width64
		default:
			return
		}
		max, ok := MaxValue(width)
		if !ok {
			return
		}
		// Keep inputs inside the declared domain so property checks focus on
		// derivation invariants rather than out-of-domain rejection.
		if width == Width32 {
			prevVal &= math.MaxUint32
			currVal &= math.MaxUint32
		}

		id := Identity{
			MetricOID:    "1.3.6.1.2.1.2.2.1.10.1",
			SourceEntity: "fuzz-entity",
			InterfaceID:  "fuzz-if",
			Width:        width,
		}
		prev := Sample{Identity: id, Value: prevVal, ObservedAt: time.Unix(prevUnix, 0).UTC()}
		curr := Sample{Identity: id, Value: currVal, ObservedAt: time.Unix(currUnix, 0).UTC()}
		continuity := ContinuityUnknown
		if continuous {
			continuity = ContinuityContinuous
		}

		srcPrev, srcCurr := prev.Value, curr.Value
		got := Derive(prev, curr, continuity)

		if prev.Value != srcPrev || curr.Value != srcCurr {
			t.Fatalf("authoritative source values mutated")
		}
		if got.PreviousRaw != srcPrev || got.CurrentRaw != srcCurr {
			t.Fatalf("result raw values diverged from sources")
		}
		if got.Delta == nil {
			return
		}
		if *got.Delta > max {
			t.Fatalf("delta %d exceeds counter domain max %d", *got.Delta, max)
		}
		if continuity != ContinuityContinuous {
			t.Fatalf("delta must not be emitted without trusted continuity")
		}
		if currVal >= prevVal {
			if *got.Delta != currVal-prevVal {
				t.Fatalf("monotonic delta=%d want=%d", *got.Delta, currVal-prevVal)
			}
			if got.Wrapped {
				t.Fatal("wrapped set on non-decreasing pair")
			}
		} else {
			want := (max - prevVal) + currVal + 1
			if *got.Delta != want {
				t.Fatalf("wrap delta=%d want=%d", *got.Delta, want)
			}
			if !got.Wrapped {
				t.Fatal("expected wrapped=true")
			}
		}
		if got.RatePerSecond != nil && currUnix <= prevUnix {
			t.Fatal("rate emitted with non-increasing timestamps")
		}
		if got.RatePerSecond != nil && got.Quality != QualityValid {
			t.Fatalf("rate present with quality=%s", got.Quality)
		}
	})
}
