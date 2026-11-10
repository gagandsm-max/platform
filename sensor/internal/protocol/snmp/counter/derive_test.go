package counter

import (
	"math"
	"testing"
	"time"
)

func baseIdentity(width Width) Identity {
	return Identity{
		MetricOID:    "1.3.6.1.2.1.31.1.1.1.6.1",
		SourceEntity: "entity-1",
		InterfaceID:  "if-1",
		Width:        width,
	}
}

func sample(id Identity, value uint64, at time.Time) Sample {
	return Sample{Identity: id, Value: value, ObservedAt: at}
}

func uint64Ptr(v uint64) *uint64 { return &v }

func TestCounter32OrdinaryIncrement(t *testing.T) {
	id := baseIdentity(Width32)
	prev := sample(id, 100, time.Unix(1000, 0).UTC())
	curr := sample(id, 150, time.Unix(1010, 0).UTC())

	got := Derive(prev, curr, ContinuityContinuous)
	if got.Quality != QualityValid || got.Delta == nil || *got.Delta != 50 || got.Wrapped {
		t.Fatalf("unexpected result: %+v delta=%v", got, got.Delta)
	}
	if got.PreviousRaw != 100 || got.CurrentRaw != 150 {
		t.Fatalf("raw values mutated: prev=%d curr=%d", got.PreviousRaw, got.CurrentRaw)
	}
	if got.RatePerSecond == nil || math.Abs(*got.RatePerSecond-5) > 1e-9 {
		t.Fatalf("rate=%v want 5", got.RatePerSecond)
	}
}

func TestCounter64OrdinaryIncrement(t *testing.T) {
	id := baseIdentity(Width64)
	prev := sample(id, 1<<60, time.Unix(1000, 0).UTC())
	curr := sample(id, 1<<60+42, time.Unix(1002, 0).UTC())

	got := Derive(prev, curr, ContinuityContinuous)
	if got.Quality != QualityValid || got.Delta == nil || *got.Delta != 42 || got.Wrapped {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestCounter32ExactWrapTrustedContinuity(t *testing.T) {
	id := baseIdentity(Width32)
	prev := sample(id, math.MaxUint32-1, time.Unix(1000, 0).UTC())
	curr := sample(id, 2, time.Unix(1001, 0).UTC())

	got := Derive(prev, curr, ContinuityContinuous)
	want := uint64(4) // (2^32 - (MaxUint32-1)) + 2 = 4
	if got.Quality != QualityValid || got.Delta == nil || *got.Delta != want || !got.Wrapped {
		t.Fatalf("wrap result=%+v delta=%v want=%d", got, got.Delta, want)
	}
}

func TestCounter64ExactWrapTrustedContinuity(t *testing.T) {
	id := baseIdentity(Width64)
	prev := sample(id, math.MaxUint64-3, time.Unix(1000, 0).UTC())
	curr := sample(id, 5, time.Unix(1001, 0).UTC())

	got := Derive(prev, curr, ContinuityContinuous)
	want := uint64(9) // 2^64 - (MaxUint64-3) + 5 = 9
	if got.Quality != QualityValid || got.Delta == nil || *got.Delta != want || !got.Wrapped {
		t.Fatalf("wrap result=%+v delta=%v want=%d", got, got.Delta, want)
	}
	// Exact arithmetic must not have routed through float.
	if float64(*got.Delta) != 9 {
		t.Fatalf("unexpected float coercion path for tiny wrap delta")
	}
}

func TestCounter64NearMaxUint64Precision(t *testing.T) {
	id := baseIdentity(Width64)
	prev := sample(id, math.MaxUint64-10, time.Unix(1000, 0).UTC())
	curr := sample(id, math.MaxUint64-1, time.Unix(1001, 0).UTC())

	got := Derive(prev, curr, ContinuityContinuous)
	if got.Quality != QualityValid || got.Delta == nil || *got.Delta != 9 {
		t.Fatalf("near-max result=%+v", got)
	}
	if got.PreviousRaw != math.MaxUint64-10 || got.CurrentRaw != math.MaxUint64-1 {
		t.Fatalf("authoritative near-max values changed")
	}
}

func TestExplicitRebootNoDelta(t *testing.T) {
	id := baseIdentity(Width64)
	prev := sample(id, 9000, time.Unix(1000, 0).UTC())
	curr := sample(id, 10, time.Unix(1010, 0).UTC())

	got := Derive(prev, curr, ContinuityResetOrReboot)
	if got.Quality != QualityResetOrReboot || got.Delta != nil || got.RatePerSecond != nil || got.Wrapped {
		t.Fatalf("reboot must suppress delta/rate: %+v", got)
	}
}

func TestExplicitDiscontinuityNoDelta(t *testing.T) {
	id := baseIdentity(Width32)
	prev := sample(id, 500, time.Unix(1000, 0).UTC())
	curr := sample(id, 10, time.Unix(1010, 0).UTC())

	got := Derive(prev, curr, ContinuityDiscontinuity)
	if got.Quality != QualityDiscontinuity || got.Delta != nil || got.RatePerSecond != nil {
		t.Fatalf("discontinuity must suppress delta/rate: %+v", got)
	}
}

func TestUnknownDecreaseNoGuessedWrap(t *testing.T) {
	id := baseIdentity(Width32)
	prev := sample(id, 1000, time.Unix(1000, 0).UTC())
	curr := sample(id, 10, time.Unix(1010, 0).UTC())

	got := Derive(prev, curr, ContinuityUnknown)
	if got.Quality != QualityAmbiguousDecrease || got.Delta != nil || got.Wrapped {
		t.Fatalf("unknown decrease must not guess wrap: %+v", got)
	}
}

func TestIdentityChangeNoDelta(t *testing.T) {
	prevID := baseIdentity(Width64)
	currID := baseIdentity(Width64)
	currID.InterfaceID = "if-2"
	prev := sample(prevID, 100, time.Unix(1000, 0).UTC())
	curr := sample(currID, 200, time.Unix(1010, 0).UTC())

	got := Derive(prev, curr, ContinuityContinuous)
	if got.Quality != QualityIdentityMismatch || got.Delta != nil {
		t.Fatalf("identity change must refuse delta: %+v", got)
	}
}

func TestWidthChangeNoDelta(t *testing.T) {
	prev := sample(baseIdentity(Width32), 100, time.Unix(1000, 0).UTC())
	curr := sample(baseIdentity(Width64), 200, time.Unix(1010, 0).UTC())

	got := Derive(prev, curr, ContinuityContinuous)
	if got.Quality != QualityWidthMismatch || got.Delta != nil {
		t.Fatalf("width change must refuse delta: %+v", got)
	}
}

func TestZeroDeltaIsValid(t *testing.T) {
	id := baseIdentity(Width64)
	prev := sample(id, 77, time.Unix(1000, 0).UTC())
	curr := sample(id, 77, time.Unix(1005, 0).UTC())

	got := Derive(prev, curr, ContinuityContinuous)
	if got.Quality != QualityValid || got.Delta == nil || *got.Delta != 0 {
		t.Fatalf("zero delta should be valid: %+v", got)
	}
	if got.RatePerSecond == nil || *got.RatePerSecond != 0 {
		t.Fatalf("zero rate want 0 got %v", got.RatePerSecond)
	}
}

func TestNonIncreasingTimestampNoRate(t *testing.T) {
	id := baseIdentity(Width32)
	prev := sample(id, 10, time.Unix(1010, 0).UTC())
	curr := sample(id, 20, time.Unix(1010, 0).UTC())

	got := Derive(prev, curr, ContinuityContinuous)
	if got.Delta == nil || *got.Delta != 10 {
		t.Fatalf("exact delta should remain: %+v", got)
	}
	if got.RatePerSecond != nil || got.Quality != QualityInvalidTime {
		t.Fatalf("non-increasing time must suppress rate: %+v", got)
	}
}

func TestRateOnlyAfterExactDelta(t *testing.T) {
	id := baseIdentity(Width64)
	prev := sample(id, 1000, time.Unix(1000, 0).UTC())
	curr := sample(id, 10, time.Unix(1010, 0).UTC())

	got := Derive(prev, curr, ContinuityUnknown)
	if got.Delta != nil || got.RatePerSecond != nil {
		t.Fatalf("rate must not exist without exact delta: %+v", got)
	}
}

func TestLargeCounter64DeltaExactBeforeRate(t *testing.T) {
	id := baseIdentity(Width64)
	// Delta larger than 2^53 so float64 cannot hold it exactly.
	const delta uint64 = (1 << 60) + 12345
	prev := sample(id, 100, time.Unix(0, 0).UTC())
	curr := sample(id, 100+delta, time.Unix(10, 0).UTC())

	got := Derive(prev, curr, ContinuityContinuous)
	if got.Delta == nil || *got.Delta != delta {
		t.Fatalf("exact delta lost: got=%v want=%d", got.Delta, delta)
	}
	if got.PreviousRaw != 100 || got.CurrentRaw != 100+delta {
		t.Fatalf("raw source values changed")
	}
	if got.RatePerSecond == nil {
		t.Fatal("expected derived rate after exact delta")
	}
	// Rate may be float64; authoritative delta must remain exact uint64.
	if float64(delta)/10 != *got.RatePerSecond {
		t.Fatalf("rate=%v want %v", *got.RatePerSecond, float64(delta)/10)
	}
}

func TestTableDrivenBoundaries(t *testing.T) {
	id32 := baseIdentity(Width32)
	id64 := baseIdentity(Width64)
	t0 := time.Unix(1000, 0).UTC()
	t1 := time.Unix(1010, 0).UTC()

	tests := []struct {
		name        string
		prev, curr  Sample
		continuity  Continuity
		wantQuality Quality
		wantDelta   *uint64
		wantWrap    bool
		wantRate    bool
	}{
		{
			name:        "counter32_max_no_wrap",
			prev:        sample(id32, math.MaxUint32-5, t0),
			curr:        sample(id32, math.MaxUint32, t1),
			continuity:  ContinuityContinuous,
			wantQuality: QualityValid,
			wantDelta:   uint64Ptr(5),
		},
		{
			name:        "counter32_wrap_from_max_to_zero",
			prev:        sample(id32, math.MaxUint32, t0),
			curr:        sample(id32, 0, t1),
			continuity:  ContinuityContinuous,
			wantQuality: QualityValid,
			wantDelta:   uint64Ptr(1),
			wantWrap:    true,
			wantRate:    true,
		},
		{
			name:        "counter64_wrap_from_max_to_zero",
			prev:        sample(id64, math.MaxUint64, t0),
			curr:        sample(id64, 0, t1),
			continuity:  ContinuityContinuous,
			wantQuality: QualityValid,
			wantDelta:   uint64Ptr(1),
			wantWrap:    true,
			wantRate:    true,
		},
		{
			name:        "counter32_out_of_domain",
			prev:        sample(id32, math.MaxUint32+1, t0),
			curr:        sample(id32, math.MaxUint32+2, t1),
			continuity:  ContinuityContinuous,
			wantQuality: QualityOutOfDomain,
		},
		{
			name:        "metric_oid_change",
			prev:        sample(id64, 1, t0),
			curr:        sample(Identity{MetricOID: "9.9.9", SourceEntity: "entity-1", InterfaceID: "if-1", Width: Width64}, 2, t1),
			continuity:  ContinuityContinuous,
			wantQuality: QualityIdentityMismatch,
		},
		{
			name:        "observation_generation_change",
			prev:        sample(Identity{MetricOID: id64.MetricOID, SourceEntity: id64.SourceEntity, InterfaceID: id64.InterfaceID, Width: Width64, ObservationGeneration: "g1"}, 1, t0),
			curr:        sample(Identity{MetricOID: id64.MetricOID, SourceEntity: id64.SourceEntity, InterfaceID: id64.InterfaceID, Width: Width64, ObservationGeneration: "g2"}, 2, t1),
			continuity:  ContinuityContinuous,
			wantQuality: QualityIdentityMismatch,
		},
		{
			name:        "unknown_increase_no_delta",
			prev:        sample(id64, 10, t0),
			curr:        sample(id64, 20, t1),
			continuity:  ContinuityUnknown,
			wantQuality: QualityUnknownContinuity,
		},
		{
			name:        "invalid_width",
			prev:        sample(Identity{MetricOID: "1.2.3", SourceEntity: "e", Width: 16}, 1, t0),
			curr:        sample(Identity{MetricOID: "1.2.3", SourceEntity: "e", Width: 16}, 2, t1),
			continuity:  ContinuityContinuous,
			wantQuality: QualityInvalidWidth,
		},
		{
			name:        "backward_time_keeps_delta",
			prev:        sample(id32, 10, t1),
			curr:        sample(id32, 25, t0),
			continuity:  ContinuityContinuous,
			wantQuality: QualityInvalidTime,
			wantDelta:   uint64Ptr(15),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Derive(tc.prev, tc.curr, tc.continuity)
			if got.Quality != tc.wantQuality {
				t.Fatalf("quality=%s want=%s result=%+v", got.Quality, tc.wantQuality, got)
			}
			if tc.wantDelta == nil {
				if got.Delta != nil {
					t.Fatalf("delta=%v want nil", *got.Delta)
				}
			} else if got.Delta == nil || *got.Delta != *tc.wantDelta {
				t.Fatalf("delta=%v want=%d", got.Delta, *tc.wantDelta)
			}
			if got.Wrapped != tc.wantWrap {
				t.Fatalf("wrapped=%v want=%v", got.Wrapped, tc.wantWrap)
			}
			if tc.wantRate && got.RatePerSecond == nil {
				t.Fatal("expected rate")
			}
			if got.PreviousRaw != tc.prev.Value || got.CurrentRaw != tc.curr.Value {
				t.Fatalf("raw values changed")
			}
		})
	}
}

func TestAssessSysUpTimeContinuousAndAmbiguous(t *testing.T) {
	prevWall := time.Unix(1000, 0).UTC()
	currWall := prevWall.Add(10 * time.Second)

	cont, reason := AssessSysUpTime(SysUpTimeInput{
		PreviousTicks: 1000,
		CurrentTicks:  2000, // +10.00s in TimeTicks
		PreviousWall:  prevWall,
		CurrentWall:   currWall,
		Tolerance:     50 * time.Millisecond,
	})
	if cont != ContinuityContinuous {
		t.Fatalf("continuity=%s reason=%s", cont, reason)
	}

	// Decrease that looks like a TimeTicks wrap matching wall elapsed.
	wrapCont, _ := AssessSysUpTime(SysUpTimeInput{
		PreviousTicks: math.MaxUint32 - 50,
		CurrentTicks:  949, // modular advance = 1000 ticks = 10s
		PreviousWall:  prevWall,
		CurrentWall:   currWall,
		Tolerance:     100 * time.Millisecond,
	})
	if wrapCont != ContinuityContinuous {
		t.Fatalf("uptime wrap should remain continuous, got %s", wrapCont)
	}

	// Decrease that does not match wall elapsed must stay UNKNOWN (not reboot).
	unknown, _ := AssessSysUpTime(SysUpTimeInput{
		PreviousTicks: 500000,
		CurrentTicks:  10,
		PreviousWall:  prevWall,
		CurrentWall:   currWall,
		Tolerance:     100 * time.Millisecond,
	})
	if unknown != ContinuityUnknown {
		t.Fatalf("ambiguous uptime decrease must be UNKNOWN, got %s", unknown)
	}
}

func TestCompatibleIdentity(t *testing.T) {
	a := baseIdentity(Width64)
	b := a
	if !CompatibleIdentity(a, b) {
		t.Fatal("identical identities should match")
	}
	b.SourceEntity = "other"
	if CompatibleIdentity(a, b) {
		t.Fatal("source entity change must not match")
	}
}
