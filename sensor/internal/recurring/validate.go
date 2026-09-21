package recurring

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Validation error codes (stable, machine-readable).
const (
	ErrCodeInvalidDesired = "SENSOR_RECURRING_INVALID_DESIRED"
	ErrCodeOversized      = "SENSOR_RECURRING_OVERSIZED"
	ErrCodeSecretField    = "SENSOR_RECURRING_SECRET_FIELD"
)

var allowedScheduling = map[SchedulingClass]struct{}{
	SchedulingLiveness:            {},
	SchedulingOperationalCritical: {},
	SchedulingOperationalDetail:   {},
	SchedulingStateTopology:       {},
	SchedulingInventory:           {},
	SchedulingLifecycle:           {},
	SchedulingEventStream:         {},
	SchedulingInvestigation:       {},
}

var allowedExecution = map[string]struct{}{
	"NETWORK_POLL": {},
	"STREAMING":    {},
	"INFRA":        {},
	"DISCOVERY":    {},
	"NCM":          {},
	"SYNTHETIC":    {},
}

var forbiddenSecretKeys = []string{
	"community", "authsecret", "privacysecret", "privatekey", "password", "bearer",
	"auth_secret", "privacy_secret", "private_key", "communitystring",
}

// ValidateDesiredState validates the full desired payload including size bound.
func ValidateDesiredState(desired DesiredState) error {
	raw, err := json.Marshal(desired)
	if err != nil {
		return fmt.Errorf("%s: failed to measure desired state: %w", ErrCodeInvalidDesired, err)
	}
	if len(raw) > MaxDesiredBytes {
		return fmt.Errorf("%s: desired state exceeds %d bytes", ErrCodeOversized, MaxDesiredBytes)
	}
	if desired.Generation < 0 {
		return fmt.Errorf("%s: generation must not be negative", ErrCodeInvalidDesired)
	}
	if len(desired.Definitions) > MaxDefinitions {
		return fmt.Errorf("%s: definition count exceeds %d", ErrCodeOversized, MaxDefinitions)
	}
	if desired.Generation == 0 {
		if len(desired.Definitions) != 0 || desired.Checksum != "" {
			return fmt.Errorf("%s: generation 0 requires empty definitions and checksum", ErrCodeInvalidDesired)
		}
		return nil
	}
	seen := make(map[string]struct{}, len(desired.Definitions))
	for i := range desired.Definitions {
		def := desired.Definitions[i]
		if err := ValidateDefinition(def); err != nil {
			return err
		}
		if _, dup := seen[def.AssignmentID]; dup {
			return fmt.Errorf("%s: duplicate assignmentId %q", ErrCodeInvalidDesired, def.AssignmentID)
		}
		seen[def.AssignmentID] = struct{}{}
	}
	computed := ComputeDesiredChecksum(desired.Definitions)
	if desired.Checksum != computed {
		return fmt.Errorf("%s: checksum mismatch", ErrCodeInvalidDesired)
	}
	return nil
}

// ValidateDefinition enforces cadence/freshness independence, enums, bounds and
// no-secret contracts. Cadence and freshness are both required and positive within
// bounds; neither is inferred from the other.
func ValidateDefinition(def RecurringDefinition) error {
	if err := requireString("assignmentId", def.AssignmentID); err != nil {
		return err
	}
	if err := requireString("tenantId", def.TenantID); err != nil {
		return err
	}
	if err := requireString("entityId", def.EntityID); err != nil {
		return err
	}
	if err := requireString("monitorDefinitionId", def.MonitorDefinitionID); err != nil {
		return err
	}
	if _, ok := allowedScheduling[def.SchedulingClass]; !ok {
		return fmt.Errorf("%s: invalid schedulingClass %q", ErrCodeInvalidDesired, def.SchedulingClass)
	}
	if _, ok := allowedExecution[def.ExecutionClass]; !ok {
		return fmt.Errorf("%s: invalid executionClass %q", ErrCodeInvalidDesired, def.ExecutionClass)
	}
	if def.CadenceMs < MinCadenceMs || def.CadenceMs > MaxCadenceMs {
		return fmt.Errorf("%s: cadenceMs out of bounds", ErrCodeInvalidDesired)
	}
	if def.FreshnessMs < MinFreshnessMs || def.FreshnessMs > MaxFreshnessMs {
		return fmt.Errorf("%s: freshnessMs out of bounds", ErrCodeInvalidDesired)
	}
	// Explicit independence: do not require freshness >= cadence or any ratio.
	if def.PhaseVersion == 0 {
		return fmt.Errorf("%s: phaseVersion is required", ErrCodeInvalidDesired)
	}
	if def.ExecutionTimeoutMs < MinExecutionTimeoutMs || def.ExecutionTimeoutMs > MaxExecutionTimeoutMs {
		return fmt.Errorf("%s: executionTimeoutMs out of bounds", ErrCodeInvalidDesired)
	}
	if err := requireString("protocol", def.Protocol); err != nil {
		return err
	}
	if def.Protocol != ProtocolSNMP {
		return fmt.Errorf("%s: unsupported protocol %q", ErrCodeInvalidDesired, def.Protocol)
	}
	if err := requireString("credentialProfileRef", def.CredentialProfileRef); err != nil {
		return err
	}
	if def.SNMP == nil {
		return fmt.Errorf("%s: snmp params required for SNMP protocol", ErrCodeInvalidDesired)
	}
	if err := validateSNMPParams(def.SNMP); err != nil {
		return err
	}
	if err := rejectSecretFields(def); err != nil {
		return err
	}
	return nil
}

func validateSNMPParams(p *SNMPParams) error {
	if err := requireString("snmp.target", p.Target); err != nil {
		return err
	}
	if p.Port < MinPort || p.Port > MaxPort {
		return fmt.Errorf("%s: snmp.port out of bounds", ErrCodeInvalidDesired)
	}
	if err := requireString("snmp.version", p.Version); err != nil {
		return err
	}
	switch p.Version {
	case "1", "2c", "3":
	default:
		return fmt.Errorf("%s: unsupported snmp.version %q", ErrCodeInvalidDesired, p.Version)
	}
	if err := requireString("snmp.operation", p.Operation); err != nil {
		return err
	}
	switch strings.ToUpper(p.Operation) {
	case "GET", "GETNEXT", "GETBULK", "WALK", "BULKWALK":
	default:
		return fmt.Errorf("%s: unsupported snmp.operation %q", ErrCodeInvalidDesired, p.Operation)
	}
	if len(p.OIDs) == 0 {
		return fmt.Errorf("%s: snmp.oids must not be empty", ErrCodeInvalidDesired)
	}
	if len(p.OIDs) > MaxOIDs {
		return fmt.Errorf("%s: snmp.oids exceeds %d", ErrCodeOversized, MaxOIDs)
	}
	for _, oid := range p.OIDs {
		if strings.TrimSpace(oid) == "" || utf8.RuneCountInString(oid) > MaxStringLen {
			return fmt.Errorf("%s: snmp.oid invalid", ErrCodeInvalidDesired)
		}
	}
	if p.TimeoutMs < MinSNMPTimeoutMs || p.TimeoutMs > MaxSNMPTimeoutMs {
		return fmt.Errorf("%s: snmp.timeoutMs out of bounds", ErrCodeInvalidDesired)
	}
	if p.Limits.MaxOIDs <= 0 || p.Limits.MaxWalkEntries <= 0 || p.Limits.MaxBulkRepetitions == 0 ||
		p.Limits.MaxResults <= 0 || p.Limits.MaxResultBytes <= 0 {
		return fmt.Errorf("%s: snmp.limits must all be positive", ErrCodeInvalidDesired)
	}
	return nil
}

func requireString(field, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s: %s is required", ErrCodeInvalidDesired, field)
	}
	if utf8.RuneCountInString(value) > MaxStringLen {
		return fmt.Errorf("%s: %s exceeds max length", ErrCodeOversized, field)
	}
	return nil
}

func rejectSecretFields(def RecurringDefinition) error {
	raw, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("%s: failed to inspect definition: %w", ErrCodeInvalidDesired, err)
	}
	lower := strings.ToLower(string(raw))
	for _, key := range forbiddenSecretKeys {
		if strings.Contains(lower, `"`+key+`"`) {
			return fmt.Errorf("%s: forbidden secret field %q", ErrCodeSecretField, key)
		}
	}
	return nil
}
