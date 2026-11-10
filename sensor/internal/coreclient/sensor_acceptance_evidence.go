package coreclient

// SensorAcceptanceEvidenceV1 is stamped by the accepting Sensor and persisted
// with its Agent spool payload. The spool sequence is filled from the durable
// record when the exact request is built for Core.
type SensorAcceptanceEvidenceV1 struct {
	EvidenceVersion         int    `json:"evidenceVersion"`
	AcceptedAtUTC           string `json:"acceptedAtUtc"`
	ClockQuality            string `json:"clockQuality"`
	ClockMaxErrorMs         *int64 `json:"clockMaxErrorMs,omitempty"`
	ClockSyncAgeMs          *int64 `json:"clockSyncAgeMs,omitempty"`
	ClockSource             string `json:"clockSource"`
	SensorServiceInstanceID string `json:"sensorServiceInstanceId"`
	DurableSpoolSequence    uint64 `json:"durableSpoolSequence,omitempty"`
}
