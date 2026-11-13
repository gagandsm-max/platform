package agentreceiver

import "time"

// ClockEvidenceProvider samples clock quality at Sensor spool acceptance.
// Missing or unprovable bounds stay UNQUALIFIED; callers never infer quality.
type ClockEvidenceProvider interface {
	Sample(acceptedAt time.Time) ClockSample
}

type ClockSample struct {
	Quality    string
	MaxErrorMs *int64
	SyncAgeMs  *int64
	Source     string
}

func unqualifiedClock(source string) ClockSample {
	return ClockSample{Quality: "UNQUALIFIED", Source: source}
}
