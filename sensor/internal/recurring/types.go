package recurring

// SchedulingClass is the monitoring urgency/class dimension. It is distinct from
// ExecutionClass (scheduler worker-pool class) and Monitor Group.
type SchedulingClass string

const (
	SchedulingLiveness            SchedulingClass = "LIVENESS"
	SchedulingOperationalCritical SchedulingClass = "OPERATIONAL_CRITICAL"
	SchedulingOperationalDetail   SchedulingClass = "OPERATIONAL_DETAIL"
	SchedulingStateTopology       SchedulingClass = "STATE_TOPOLOGY"
	SchedulingInventory           SchedulingClass = "INVENTORY"
	SchedulingLifecycle           SchedulingClass = "LIFECYCLE"
	SchedulingEventStream         SchedulingClass = "EVENT_STREAM"
	SchedulingInvestigation       SchedulingClass = "INVESTIGATION"
)

// ProtocolSNMP is the only Slice A protocol value on recurring definitions.
const ProtocolSNMP = "SNMP"

// Bounds for desired-state validation (fail closed).
const (
	MaxDefinitions               = 512
	MaxStringLen                 = 256
	MaxOIDs                      = 128
	MaxDesiredBytes              = 1 << 20
	MinCadenceMs          int64  = 1000
	MaxCadenceMs          int64  = 24 * 60 * 60 * 1000 // 24h
	MinFreshnessMs        int64  = 1
	MaxFreshnessMs        int64  = 24 * 60 * 60 * 1000
	MinExecutionTimeoutMs int64  = 1
	MaxExecutionTimeoutMs int64  = 60 * 60 * 1000 // 1h
	MinSNMPTimeoutMs      int64  = 1
	MaxSNMPTimeoutMs      int64  = 60 * 1000
	MinPort                      = 1
	MaxPort                      = 65535
	MaxAppliedStateBytes  int64  = MaxDesiredBytes
	PhaseVersion1         uint32 = 1
)

// SNMPLimits are non-secret collection safety bounds carried on the definition.
type SNMPLimits struct {
	MaxOIDs            int    `json:"maxOids"`
	MaxWalkEntries     int    `json:"maxWalkEntries"`
	MaxBulkRepetitions uint32 `json:"maxBulkRepetitions"`
	MaxResults         int    `json:"maxResults"`
	MaxResultBytes     int    `json:"maxResultBytes"`
}

// SNMPParams are non-secret SNMP collection parameters. Communities, auth/privacy
// secrets and other credentials MUST NOT appear here; use CredentialProfileRef.
type SNMPParams struct {
	Target    string     `json:"target"`
	Port      int        `json:"port"`
	Version   string     `json:"version"`
	Operation string     `json:"operation"`
	OIDs      []string   `json:"oids"`
	TimeoutMs int64      `json:"timeoutMs"`
	Limits    SNMPLimits `json:"limits"`
}

// RecurringDefinition is one authorized recurring monitoring assignment.
type RecurringDefinition struct {
	AssignmentID         string          `json:"assignmentId"`
	TenantID             string          `json:"tenantId"`
	EntityID             string          `json:"entityId"`
	MonitorDefinitionID  string          `json:"monitorDefinitionId"`
	SchedulingClass      SchedulingClass `json:"schedulingClass"`
	ExecutionClass       string          `json:"executionClass"`
	CadenceMs            int64           `json:"cadenceMs"`
	FreshnessMs          int64           `json:"freshnessMs"`
	PhaseVersion         uint32          `json:"phaseVersion"`
	Generation           int64           `json:"generation,omitempty"` // optional per-definition fence
	ExecutionTimeoutMs   int64           `json:"executionTimeoutMs"`
	CredentialProfileRef string          `json:"credentialProfileRef"`
	Protocol             string          `json:"protocol"`
	SNMP                 *SNMPParams     `json:"snmp,omitempty"`
}

// DesiredState is Core-authored desired recurring schedule for this Sensor.
type DesiredState struct {
	Generation  int64                 `json:"generation"`
	Checksum    string                `json:"checksum"`
	Definitions []RecurringDefinition `json:"definitions"`
}

// AppliedState is durable Sensor-applied recurring schedule.
type AppliedState struct {
	Generation               int64                 `json:"generation"`
	Checksum                 string                `json:"checksum,omitempty"`
	Definitions              []RecurringDefinition `json:"definitions"`
	CoreAuthorityEstablished bool                  `json:"coreAuthorityEstablished,omitempty"`
}
