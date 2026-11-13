//go:build !linux

package agentreceiver

import "time"

type unsupportedClockEvidenceProvider struct{}

func productionClockEvidenceProvider() ClockEvidenceProvider {
	return unsupportedClockEvidenceProvider{}
}

func (unsupportedClockEvidenceProvider) Sample(time.Time) ClockSample {
	return unqualifiedClock("UNSUPPORTED_CLOCK_PLATFORM")
}
