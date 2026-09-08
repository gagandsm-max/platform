package discovery

import "time"

type Target struct {
	Address string `json:"address"`
	Type    string `json:"type"` // IPV4, IPV6, HOSTNAME
}

type Evidence struct {
	TargetAddress    string         `json:"target"`
	Protocol         string         `json:"protocol"` // ICMP, SNMP
	Method           string         `json:"method"`
	Status           string         `json:"status"`            // SUCCESS, FAILED, INCONCLUSIVE
	ReachabilityKind string         `json:"reachability_kind"` // OBSERVED, DERIVED, SUSPECTED, UNKNOWN
	TechnicalCode    string         `json:"technical_code,omitempty"`
	Attributes       map[string]any `json:"attributes,omitempty"`
	RawEvidence      map[string]any `json:"raw_evidence,omitempty"`
	DurationMs       int64          `json:"duration_ms"`
}

type Task struct {
	TaskID               string        `json:"task_id"`
	RequestID            string        `json:"request_id"`
	TenantID             string        `json:"tenant_id"`
	CorrelationID        string        `json:"correlation_id"`
	AssignmentVersion    uint64        `json:"assignment_version"`
	Targets              []Target      `json:"targets"`
	RequestedMethods     []string      `json:"requested_methods"`
	CredentialProfileRef string        `json:"credential_profile_ref,omitempty"`
	SNMPPort             int           `json:"snmp_port"`
	Timeout              time.Duration `json:"timeout"`
	MaxRetries           int           `json:"max_retries"`
}

type Result struct {
	RequestID     string     `json:"request_id"`
	TenantID      string     `json:"tenant_id"`
	CorrelationID string     `json:"correlation_id"`
	Status        string     `json:"status"` // COMPLETED, FAILED
	FailureReason string     `json:"failure_reason,omitempty"`
	Evidence      []Evidence `json:"evidence"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   time.Time  `json:"completed_at"`
	DurationMs    int64      `json:"duration_ms"`
}