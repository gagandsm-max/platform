package coreclient

import (
	"context"
	"net/http"
)

const (
	recurringScheduleDesiredStatePath = "/api/v1/sensor/recurring-schedules/desired-state"
	recurringScheduleAppliedStatePath = "/api/v1/sensor/recurring-schedules/applied-state"

	CodeRecurringStateStale       ErrorCode = "ERR_SENSOR_RECURRING_STATE_STALE"
	CodeRecurringChecksumMismatch ErrorCode = "ERR_SENSOR_RECURRING_CHECKSUM_MISMATCH"
)

// RecurringScheduleSNMPLimits mirrors Sensor non-secret SNMP safety bounds.
type RecurringScheduleSNMPLimits struct {
	MaxOIDs            int    `json:"maxOids"`
	MaxWalkEntries     int    `json:"maxWalkEntries"`
	MaxBulkRepetitions uint32 `json:"maxBulkRepetitions"`
	MaxResults         int    `json:"maxResults"`
	MaxResultBytes     int    `json:"maxResultBytes"`
}

// RecurringScheduleSNMPParams are non-secret SNMP parameters (no communities/secrets).
type RecurringScheduleSNMPParams struct {
	Target    string                      `json:"target"`
	Port      int                         `json:"port"`
	Version   string                      `json:"version"`
	Operation string                      `json:"operation"`
	OIDs      []string                    `json:"oids"`
	TimeoutMs int64                       `json:"timeoutMs"`
	Limits    RecurringScheduleSNMPLimits `json:"limits"`
}

// RecurringScheduleDefinition is one Core-authored recurring assignment row.
type RecurringScheduleDefinition struct {
	AssignmentID         string                       `json:"assignmentId"`
	TenantID             string                       `json:"tenantId"`
	EntityID             string                       `json:"entityId"`
	MonitorDefinitionID  string                       `json:"monitorDefinitionId"`
	SchedulingClass      string                       `json:"schedulingClass"`
	ExecutionClass       string                       `json:"executionClass"`
	CadenceMs            int64                        `json:"cadenceMs"`
	FreshnessMs          int64                        `json:"freshnessMs"`
	PhaseVersion         uint32                       `json:"phaseVersion"`
	Generation           int64                        `json:"generation,omitempty"`
	ExecutionTimeoutMs   int64                        `json:"executionTimeoutMs"`
	CredentialProfileRef string                       `json:"credentialProfileRef"`
	Protocol             string                       `json:"protocol"`
	SNMP                 *RecurringScheduleSNMPParams `json:"snmp,omitempty"`
}

// RecurringScheduleDesiredState is the Core desired/applied view for recurring schedules.
type RecurringScheduleDesiredState struct {
	Generation        int64                         `json:"generation"`
	Checksum          string                        `json:"checksum"`
	AppliedGeneration int64                         `json:"appliedGeneration"`
	AppliedChecksum   string                        `json:"appliedChecksum"`
	Definitions       []RecurringScheduleDefinition `json:"definitions"`
}

// FetchRecurringScheduleDesiredState GETs the Sensor-scoped desired recurring schedule.
func (c *Client) FetchRecurringScheduleDesiredState(ctx context.Context) (RecurringScheduleDesiredState, error) {
	var out RecurringScheduleDesiredState
	if err := c.doJSON(ctx, http.MethodGet, recurringScheduleDesiredStatePath, nil, "", "", &out); err != nil {
		return RecurringScheduleDesiredState{}, err
	}
	if out.Generation < 0 {
		return RecurringScheduleDesiredState{}, newError(CodeProtocol, "desired-state generation is negative", nil, false)
	}
	if out.Checksum != "" && !isLowerHex64(out.Checksum) {
		return RecurringScheduleDesiredState{}, newError(CodeProtocol, "desired-state checksum is not lowercase 64-hex", nil, false)
	}
	if out.AppliedGeneration < 0 || out.AppliedGeneration > out.Generation {
		return RecurringScheduleDesiredState{}, newError(CodeProtocol, "Core applied generation is outside desired-state bounds", nil, false)
	}
	if out.AppliedGeneration == 0 && out.AppliedChecksum != "" {
		return RecurringScheduleDesiredState{}, newError(CodeProtocol, "Core applied checksum must be empty at generation zero", nil, false)
	}
	if out.AppliedGeneration > 0 && !isLowerHex64(out.AppliedChecksum) {
		return RecurringScheduleDesiredState{}, newError(CodeProtocol, "Core applied checksum is not lowercase 64-hex", nil, false)
	}
	if out.Definitions == nil {
		out.Definitions = []RecurringScheduleDefinition{}
	}
	return out, nil
}

// AckRecurringScheduleAppliedState PUTs Sensor applied generation/checksum.
func (c *Client) AckRecurringScheduleAppliedState(ctx context.Context, generation int64, checksum string) (AppliedStateAck, error) {
	payload := struct {
		Generation int64   `json:"generation"`
		Checksum   *string `json:"checksum"`
	}{Generation: generation}
	if checksum != "" {
		payload.Checksum = &checksum
	}
	var out AppliedStateAck
	if err := c.doJSONWithConflictCodes(ctx, http.MethodPut, recurringScheduleAppliedStatePath, payload, &out); err != nil {
		return AppliedStateAck{}, err
	}
	return out, nil
}
