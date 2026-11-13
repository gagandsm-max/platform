package coreclient

// SensorAcceptanceEvidenceV1 is stamped after durable Sensor spool append and
// persisted in a sequence and payload bound sidecar before Core delivery.
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
