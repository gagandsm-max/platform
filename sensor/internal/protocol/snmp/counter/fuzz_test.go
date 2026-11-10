package counter

import (
	"math"
	"testing"
	"time"
)

func fuzzIdentity(width Width) Identity {
	return Identity{
		MetricOID:    "1.3.6.1.2.1.2.2.1.10.1",
		SourceEntity: "fuzz-entity",
		ContextID:    DefaultContextID,
		InterfaceID:  "fuzz-if",
		Width:        width,
	}
}

func expectedContinuousDelta(prevVal, currVal, max uint64) (delta uint64, wrapped bool) {
	if currVal >= prevVal {
		return currVal - prevVal, false
	}
	return (max - prevVal) + currVal + 1, true
}

// FuzzDeriveValidContinuousRequiresExactDelta asserts that valid-input class
// samples always produce the exact mathematical delta (never a vacuous nil pass).
func FuzzDeriveValidContinuousRequiresExactDelta(f *testing.F) {
	f.Add(uint64(0), uint64(1), uint8(32), int64(1000), int64(1010))
	f.Add(uint64(math.MaxUint32-1), uint64(3), uint8(32), int64(1000), int64(1001))
	f.Add(uint64(math.MaxUint32), uint64(0), uint8(32), int64(1000), int64(1002))
	f.Add(uint64(math.MaxUint64-7), uint64(4), uint8(64), int64(1000), int64(1002))
	f.Add(uint64(math.MaxUint64), uint64(0), uint8(64), int64(5), int64(15))
	f.Add(uint64(1<<53), uint64(1<<53)+99, uint8(64), int64(5), int64(15))
	f.Add(uint64((1<<53)-1), uint64((1<<53)+1), uint8(64), int64(1), int64(2))
	f.Add(uint64(1<<60), uint64(1<<60)+99, uint8(64), int64(5), int64(15))

	f.Fuzz(func(t *testing.T, prevVal, currVal uint64, widthBits uint8, prevUnix, currUnix int64) {
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
		if width == Width32 {
			prevVal &= math.MaxUint32
			currVal &= math.MaxUint32
		}
		// Valid-input class: strictly increasing timestamps.
		if currUnix <= prevUnix {
			return
		}

		id := fuzzIdentity(width)
		prev := Sample{Identity: id, Value: prevVal, ObservedAt: time.Unix(prevUnix, 0).UTC()}
		curr := Sample{Identity: id, Value: currVal, ObservedAt: time.Unix(currUnix, 0).UTC()}
		srcPrev, srcCurr := prev.Value, curr.Value

		got := Derive(prev, curr, ContinuityContinuous)

		if prev.Value != srcPrev || curr.Value != srcCurr {
			t.Fatalf("authoritative source values mutated")
		}
		if got.PreviousRaw != srcPrev || got.CurrentRaw != srcCurr {
			t.Fatalf("result raw values diverged from sources")
		}
		if got.Delta == nil {
			t.Fatalf("valid continuous input must produce delta; quality=%s reason=%s", got.Quality, got.Reason)
		}
		want, wantWrap := expectedContinuousDelta(prevVal, currVal, max)
		if *got.Delta != want {
			t.Fatalf("delta=%d want=%d", *got.Delta, want)
		}
		if got.Wrapped != wantWrap {
			t.Fatalf("wrapped=%v want=%v", got.Wrapped, wantWrap)
		}
		if *got.Delta > max {
			t.Fatalf("delta %d exceeds counter domain max %d", *got.Delta, max)
		}
		switch got.Quality {
		case QualityValid, QualityInvalidTime, QualityTimeIntervalOutOfRange:
			// rate may be suppressed for non-representable intervals; delta remains.
		default:
			t.Fatalf("unexpected quality=%s for trusted continuous derivation", got.Quality)
		}
		if got.RatePerSecond != nil && got.Quality != QualityValid {
			t.Fatalf("rate present with quality=%s", got.Quality)
		}
	})
}

// FuzzDeriveNonContinuousNeverEmitsDelta asserts invalid/unknown continuity
// never produces a traffic delta.
func FuzzDeriveNonContinuousNeverEmitsDelta(f *testing.F) {
	f.Add(uint64(0), uint64(1), uint8(32), uint8(0), int64(1000), int64(1010))
	f.Add(uint64(math.MaxUint32-1), uint64(3), uint8(32), uint8(1), int64(1000), int64(1001))
	f.Add(uint64(math.MaxUint64-7), uint64(4), uint8(64), uint8(2), int64(1000), int64(1002))
	f.Add(uint64(1000), uint64(10), uint8(64), uint8(0), int64(1000), int64(1010))
	f.Add(uint64(1<<53), uint64(10), uint8(64), uint8(1), int64(5), int64(15))
	f.Add(uint64(math.MaxUint64), uint64(0), uint8(64), uint8(2), int64(1), int64(2))

	f.Fuzz(func(t *testing.T, prevVal, currVal uint64, widthBits uint8, contKind uint8, prevUnix, currUnix int64) {
		var width Width
		switch widthBits {
		case 32:
			width = Width32
		case 64:
			width = Width64
		default:
			return
		}
		if width == Width32 {
			prevVal &= math.MaxUint32
			currVal &= math.MaxUint32
		}

		var continuity Continuity
		switch contKind % 3 {
		case 0:
			continuity = ContinuityUnknown
		case 1:
			continuity = ContinuityResetOrReboot
		default:
			continuity = ContinuityDiscontinuity
		}

		id := fuzzIdentity(width)
		prev := Sample{Identity: id, Value: prevVal, ObservedAt: time.Unix(prevUnix, 0).UTC()}
		curr := Sample{Identity: id, Value: currVal, ObservedAt: time.Unix(currUnix, 0).UTC()}
		srcPrev, srcCurr := prev.Value, curr.Value

		got := Derive(prev, curr, continuity)

		if prev.Value != srcPrev || curr.Value != srcCurr {
			t.Fatalf("authoritative source values mutated")
		}
		if got.PreviousRaw != srcPrev || got.CurrentRaw != srcCurr {
			t.Fatalf("result raw values diverged from sources")
		}
		if got.Delta != nil {
			t.Fatalf("non-continuous continuity=%s must not emit delta=%d", continuity, *got.Delta)
		}
		if got.RatePerSecond != nil {
			t.Fatalf("non-continuous continuity=%s must not emit rate", continuity)
		}
		if got.Wrapped {
			t.Fatalf("non-continuous continuity=%s must not set wrapped", continuity)
		}
	})
}
