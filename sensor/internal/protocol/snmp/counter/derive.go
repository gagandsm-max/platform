package counter

import "time"

// CompatibleIdentity reports whether two counter identities describe the same
// derivation series. ObservationGeneration is compared only when at least one
// side supplies it. Width is always compared.
func CompatibleIdentity(a, b Identity) bool {
	if a.MetricOID != b.MetricOID {
		return false
	}
	if a.SourceEntity != b.SourceEntity {
		return false
	}
	if a.InterfaceID != b.InterfaceID {
		return false
	}
	if a.Width != b.Width {
		return false
	}
	if a.ObservationGeneration != "" || b.ObservationGeneration != "" {
		if a.ObservationGeneration != b.ObservationGeneration {
			return false
		}
	}
	return true
}

// Derive computes an exact integer delta and optional derived rate from two
// counter samples under an explicit continuity assessment.
//
// Invariants:
//   - previous/current raw values are copied into Result unchanged;
//   - Counter64 arithmetic stays in exact uint64 until after a valid delta;
//   - a decrease is never treated as wrap unless ContinuityContinuous;
//   - rate requires a valid exact delta and strictly increasing timestamps.
func Derive(previous, current Sample, continuity Continuity) Result {
	out := Result{
		PreviousRaw: previous.Value,
		CurrentRaw:  current.Value,
		Continuity:  continuity,
	}

	if previous.Identity.Width != current.Identity.Width {
		out.Quality = QualityWidthMismatch
		out.Reason = "counter width changed between observations"
		return out
	}
	if !CompatibleIdentity(previous.Identity, current.Identity) {
		out.Quality = QualityIdentityMismatch
		out.Reason = "counter identity changed between observations"
		return out
	}

	width := current.Identity.Width
	max, ok := MaxValue(width)
	if !ok {
		out.Quality = QualityInvalidWidth
		out.Reason = "unsupported counter width"
		return out
	}
	if previous.Value > max || current.Value > max {
		out.Quality = QualityOutOfDomain
		out.Reason = "counter value exceeds declared width domain"
		return out
	}

	switch continuity {
	case ContinuityResetOrReboot:
		out.Quality = QualityResetOrReboot
		out.Reason = "reset or reboot established; no valid traffic delta"
		return out
	case ContinuityDiscontinuity:
		out.Quality = QualityDiscontinuity
		out.Reason = "source discontinuity established; no valid traffic delta"
		return out
	case ContinuityUnknown:
		if current.Value < previous.Value {
			out.Quality = QualityAmbiguousDecrease
			out.Reason = "counter decreased without continuity evidence to distinguish wrap from reset"
			return out
		}
		out.Quality = QualityUnknownContinuity
		out.Reason = "continuity evidence insufficient to prove a valid delta"
		return out
	case ContinuityContinuous:
		// handled below
	default:
		out.Quality = QualityInvalidContinuity
		out.Reason = "unsupported continuity assessment"
		return out
	}

	var delta uint64
	switch {
	case current.Value >= previous.Value:
		delta = current.Value - previous.Value
	default:
		// Trusted continuity permits a single-width wrap only.
		delta = (max - previous.Value) + current.Value + 1
		out.Wrapped = true
	}
	out.Delta = &delta
	out.Quality = QualityValid
	out.Reason = "exact delta derived under trusted continuity"

	elapsed := current.ObservedAt.Sub(previous.ObservedAt)
	if elapsed <= 0 {
		out.Quality = QualityInvalidTime
		out.Reason = "observation timestamps are not strictly increasing; exact delta retained without rate"
		return out
	}
	rate := float64(delta) / elapsed.Seconds()
	out.RatePerSecond = &rate
	return out
}

// Elapsed returns the wall-clock duration between samples. Non-positive
// durations are invalid for rate derivation.
func Elapsed(previous, current Sample) (time.Duration, bool) {
	elapsed := current.ObservedAt.Sub(previous.ObservedAt)
	if elapsed <= 0 {
		return elapsed, false
	}
	return elapsed, true
}
