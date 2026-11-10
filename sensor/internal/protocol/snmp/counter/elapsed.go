package counter

import (
	"math"
	"time"
)

// checkedElapsed returns the exact wall-clock duration between previous and
// current when the interval is strictly positive and representable as
// time.Duration (int64 nanoseconds). It does not use time.Time.Sub, which can
// silently saturate for extreme intervals.
//
// ok is false when timestamps are not strictly increasing or the interval
// exceeds the package rate-calculation representability contract.
func checkedElapsed(previous, current time.Time) (time.Duration, bool) {
	if !current.After(previous) {
		return 0, false
	}

	sec := current.Unix() - previous.Unix()
	nsec := int64(current.Nanosecond()) - int64(previous.Nanosecond())
	if nsec < 0 {
		sec--
		nsec += int64(time.Second)
	}
	if sec < 0 {
		return 0, false
	}

	const maxSeconds = math.MaxInt64 / int64(time.Second)
	if sec > maxSeconds {
		return 0, false
	}
	if sec == maxSeconds {
		remainder := math.MaxInt64 % int64(time.Second)
		if nsec > remainder {
			return 0, false
		}
	}

	elapsed := time.Duration(sec)*time.Second + time.Duration(nsec)
	if elapsed <= 0 {
		return 0, false
	}
	return elapsed, true
}

// elapsedQuality classifies why rate derivation cannot use an observation pair's
// wall-clock interval after an exact delta has already been produced.
func elapsedQuality(previous, current time.Time) (elapsed time.Duration, quality Quality, reason string, ok bool) {
	if !current.After(previous) {
		return 0, QualityInvalidTime, "observation timestamps are not strictly increasing; exact delta retained without rate", false
	}
	elapsed, ok = checkedElapsed(previous, current)
	if !ok {
		return 0, QualityTimeIntervalOutOfRange, "elapsed observation interval is not representable for rate derivation; exact delta retained without rate", false
	}
	return elapsed, QualityValid, "", true
}
