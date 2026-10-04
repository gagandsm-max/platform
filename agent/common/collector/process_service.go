package collector

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Process/service selector and discovery catalogue source contracts
// (BUILD-011 Slice 1). These are Agent-side internal models only.
// They do not create Core entities, do not wire physical transport, and do
// not grant remediation/restart authority.

const ProcessServiceSchemaVersion = 1

// Implementation safety bounds (not customer-facing product capacity contracts).
const (
	maxProcessServiceSelectorValueLen    = 512
	maxProcessServiceCatalogueEntries    = 2048
	maxProcessServiceIdentityLen         = 512
	maxProcessServiceNameLen             = 256
	maxProcessServiceProvenanceLen       = 512
	maxProcessServiceQualityLen          = 128
	maxProcessServiceCatalogueIDLen      = 128
	maxProcessServiceInstanceTokenLen    = 256
	maxProcessServiceIncompleteReasonLen = 512
)

// SelectorKind enumerates durable process/service monitor selector classes.
type SelectorKind string

const (
	SelectorKindServiceID          SelectorKind = "SERVICE_ID"
	SelectorKindExecutableIdentity SelectorKind = "EXECUTABLE_IDENTITY"
	SelectorKindExecutablePath     SelectorKind = "EXECUTABLE_PATH"
	SelectorKindProcessName        SelectorKind = "PROCESS_NAME"
)

// ProcessServiceSelector is a durable monitor target identity.
// PID is prohibited as a durable selector kind.
type ProcessServiceSelector struct {
	Kind  SelectorKind
	Value string
}

// Validate enforces deterministic, bounded, non-executable selector semantics.
func (s ProcessServiceSelector) Validate() error {
	switch s.Kind {
	case SelectorKindServiceID, SelectorKindExecutableIdentity, SelectorKindExecutablePath, SelectorKindProcessName:
	case "PID", "PROCESS_ID", "REGEX", "GLOB", "SHELL", "COMMAND", "SCRIPT":
		return fmt.Errorf("PROCESS_SERVICE_SELECTOR_KIND_REJECTED: %q is not a durable selector kind", s.Kind)
	default:
		if strings.TrimSpace(string(s.Kind)) == "" {
			return errors.New("PROCESS_SERVICE_SELECTOR_KIND_INVALID: selector kind is required")
		}
		return fmt.Errorf("PROCESS_SERVICE_SELECTOR_KIND_INVALID: unsupported kind %q", s.Kind)
	}
	value := s.Value
	if strings.TrimSpace(value) == "" {
		return errors.New("PROCESS_SERVICE_SELECTOR_VALUE_INVALID: selector value is required")
	}
	if value != strings.TrimSpace(value) {
		return errors.New("PROCESS_SERVICE_SELECTOR_VALUE_INVALID: leading/trailing whitespace is not allowed")
	}
	if len(value) > maxProcessServiceSelectorValueLen {
		return fmt.Errorf(
			"PROCESS_SERVICE_SELECTOR_VALUE_BOUND_EXCEEDED: implementation safety bound is %d",
			maxProcessServiceSelectorValueLen,
		)
	}
	if hasControlRunes(value) {
		return errors.New("PROCESS_SERVICE_SELECTOR_VALUE_INVALID: control characters are prohibited")
	}
	// Selector VALUES are literal exact-match data only. They are never executed,
	// shell-quoted, globbed or compiled as regex. Security is kind allowlist +
	// exact match + non-execution — not a metacharacter blacklist on values.
	return nil
}

// SelectorPrecedenceRank returns lower numbers for preferred durable kinds.
// Preference: SERVICE_ID → EXECUTABLE_IDENTITY/PATH → PROCESS_NAME.
func (s ProcessServiceSelector) SelectorPrecedenceRank() int {
	switch s.Kind {
	case SelectorKindServiceID:
		return 1
	case SelectorKindExecutableIdentity:
		return 2
	case SelectorKindExecutablePath:
		return 3
	case SelectorKindProcessName:
		return 4
	default:
		return 100
	}
}

// ProcessServiceRuntimeInstance carries ephemeral runtime observations only.
// PID must never become durable selector/entity authority.
type ProcessServiceRuntimeInstance struct {
	PID           int
	ObservedAt    time.Time
	InstanceToken string // optional opaque runtime token; not durable entity authority
}

// ProcessServiceCatalogueEntry distinguishes stable identity from ephemeral PID.
type ProcessServiceCatalogueEntry struct {
	ServiceID          string // stable service/unit identity when available
	ExecutableIdentity string // stable executable identity when available
	ExecutablePath     string // executable path when available
	ProcessFamilyName  string // process family / exact name when applicable
	Runtime            *ProcessServiceRuntimeInstance
	SourceProvenance   string
	Quality            string
}

// PreferredDurableSelector returns the highest-precedence durable selector that
// can be derived from the entry. PID is never returned.
func (e ProcessServiceCatalogueEntry) PreferredDurableSelector() (ProcessServiceSelector, error) {
	candidates := []ProcessServiceSelector{
		{Kind: SelectorKindServiceID, Value: e.ServiceID},
		{Kind: SelectorKindExecutableIdentity, Value: e.ExecutableIdentity},
		{Kind: SelectorKindExecutablePath, Value: e.ExecutablePath},
		{Kind: SelectorKindProcessName, Value: e.ProcessFamilyName},
	}
	var best *ProcessServiceSelector
	bestRank := 1000
	for i := range candidates {
		c := candidates[i]
		if strings.TrimSpace(c.Value) == "" {
			continue
		}
		if err := c.Validate(); err != nil {
			continue
		}
		rank := c.SelectorPrecedenceRank()
		if rank < bestRank {
			cp := c
			best = &cp
			bestRank = rank
		}
	}
	if best == nil {
		return ProcessServiceSelector{}, errors.New("PROCESS_SERVICE_CATALOGUE_IDENTITY_INSUFFICIENT: no durable selector identity available")
	}
	return *best, nil
}

// ProcessServiceDiscoveryCatalogue is bounded Agent-side discovery source evidence.
// It does not claim Core entity creation, physical transport or persistence.
type ProcessServiceDiscoveryCatalogue struct {
	SchemaVersion    int
	CatalogueID      string
	CollectedAt      time.Time
	SourceProvenance string
	Quality          string
	Truncated        bool
	IncompleteReason string
	Entries          []ProcessServiceCatalogueEntry
}

// Validate checks catalogue bounds and identity/provenance invariants.
func (c ProcessServiceDiscoveryCatalogue) Validate() error {
	if c.SchemaVersion != ProcessServiceSchemaVersion {
		return fmt.Errorf("PROCESS_SERVICE_CATALOGUE_SCHEMA_INVALID: unsupported schema_version %d", c.SchemaVersion)
	}
	if err := requireProcessToken("catalogue_id", c.CatalogueID, maxProcessServiceCatalogueIDLen); err != nil {
		return err
	}
	if c.CollectedAt.IsZero() {
		return errors.New("PROCESS_SERVICE_CATALOGUE_TIME_INVALID: collected_at is required")
	}
	if err := requireProcessToken("source_provenance", c.SourceProvenance, maxProcessServiceProvenanceLen); err != nil {
		return err
	}
	if err := requireProcessToken("quality", c.Quality, maxProcessServiceQualityLen); err != nil {
		return err
	}
	if len(c.Entries) > maxProcessServiceCatalogueEntries {
		return fmt.Errorf(
			"PROCESS_SERVICE_CATALOGUE_BOUND_EXCEEDED: implementation safety bound is %d entries, got %d",
			maxProcessServiceCatalogueEntries,
			len(c.Entries),
		)
	}
	if c.Truncated && strings.TrimSpace(c.IncompleteReason) == "" {
		return errors.New("PROCESS_SERVICE_CATALOGUE_REASON_REQUIRED: truncated catalogues require incomplete_reason")
	}
	if err := optionalProcessToken("incomplete_reason", c.IncompleteReason, maxProcessServiceIncompleteReasonLen); err != nil {
		return err
	}
	for i, entry := range c.Entries {
		if err := entry.validate(i); err != nil {
			return err
		}
	}
	return nil
}

func (e ProcessServiceCatalogueEntry) validate(index int) error {
	prefix := fmt.Sprintf("PROCESS_SERVICE_CATALOGUE_ENTRY[%d]", index)
	if err := optionalProcessToken(prefix+".service_id", e.ServiceID, maxProcessServiceIdentityLen); err != nil {
		return err
	}
	if err := optionalProcessToken(prefix+".executable_identity", e.ExecutableIdentity, maxProcessServiceIdentityLen); err != nil {
		return err
	}
	if err := optionalProcessToken(prefix+".executable_path", e.ExecutablePath, maxProcessServiceIdentityLen); err != nil {
		return err
	}
	if err := optionalProcessToken(prefix+".process_family_name", e.ProcessFamilyName, maxProcessServiceNameLen); err != nil {
		return err
	}
	if err := optionalProcessToken(prefix+".source_provenance", e.SourceProvenance, maxProcessServiceProvenanceLen); err != nil {
		return err
	}
	if err := optionalProcessToken(prefix+".quality", e.Quality, maxProcessServiceQualityLen); err != nil {
		return err
	}
	hasStable := strings.TrimSpace(e.ServiceID) != "" ||
		strings.TrimSpace(e.ExecutableIdentity) != "" ||
		strings.TrimSpace(e.ExecutablePath) != "" ||
		strings.TrimSpace(e.ProcessFamilyName) != ""
	if !hasStable {
		return fmt.Errorf("%s: insufficient identity; PID-only rows are not durable catalogue evidence", prefix)
	}
	if e.Runtime != nil {
		if e.Runtime.PID <= 0 {
			return fmt.Errorf("%s.runtime.pid invalid: must be > 0", prefix)
		}
		if e.Runtime.ObservedAt.IsZero() {
			return fmt.Errorf("%s.runtime.observed_at is required", prefix)
		}
		if err := optionalProcessToken(prefix+".runtime.instance_token", e.Runtime.InstanceToken, maxProcessServiceInstanceTokenLen); err != nil {
			return err
		}
		// PID may exist as ephemeral observation but must never validate as durable selector.
		pidSelector := ProcessServiceSelector{Kind: "PID", Value: strconv.Itoa(e.Runtime.PID)}
		if err := pidSelector.Validate(); err == nil {
			return fmt.Errorf("%s: PID unexpectedly accepted as durable selector", prefix)
		}
	}
	return nil
}

func requireProcessToken(field, value string, maxLen int) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return fmt.Errorf("PROCESS_SERVICE_FIELD_INVALID: %s is required", field)
	}
	if len(v) > maxLen {
		return fmt.Errorf("PROCESS_SERVICE_FIELD_BOUND_EXCEEDED: %s exceeds implementation safety bound %d", field, maxLen)
	}
	if hasControlRunes(v) {
		return fmt.Errorf("PROCESS_SERVICE_FIELD_INVALID: %s contains control characters", field)
	}
	if v != value {
		return fmt.Errorf("PROCESS_SERVICE_FIELD_INVALID: %s has leading/trailing whitespace", field)
	}
	return nil
}

func optionalProcessToken(field, value string, maxLen int) error {
	if value == "" {
		return nil
	}
	return requireProcessToken(field, value, maxLen)
}

// ---------------------------------------------------------------------------
// Typed structured-evidence sink boundary (local / unwired)
// ---------------------------------------------------------------------------

// StructuredEvidenceDeliveryState makes physical delivery status explicit.
type StructuredEvidenceDeliveryState string

const (
	// StructuredEvidenceTransportNotWired means BUILD-016/062 physical delivery
	// is not connected. Local validation/recording may still occur.
	StructuredEvidenceTransportNotWired StructuredEvidenceDeliveryState = "TRANSPORT_NOT_WIRED"
	// StructuredEvidenceLocalOnly means evidence was accepted for local/test use
	// only and must not be interpreted as durable Agent→Sensor delivery success.
	StructuredEvidenceLocalOnly StructuredEvidenceDeliveryState = "LOCAL_ONLY"
)

// ErrStructuredTransportNotWired is returned when a sink refuses to claim delivery.
var ErrStructuredTransportNotWired = errors.New("STRUCTURED_EVIDENCE_TRANSPORT_NOT_WIRED: physical Agent structured delivery is owned by BUILD-016/062 and is not connected in BUILD-011 Slice 1")

// StructuredEvidenceSink is the narrow producer/test seam for typed source evidence.
// Implementations must not enqueue into HostCollector/Sample/Observation/CollectionBatch.
type StructuredEvidenceSink interface {
	AcceptSoftwareInventory(ctx context.Context, snap SoftwareInventorySnapshot) error
	AcceptProcessServiceCatalogue(ctx context.Context, cat ProcessServiceDiscoveryCatalogue) error
	DeliveryState() StructuredEvidenceDeliveryState
}

// UnwiredStructuredEvidenceSink is the default production-safe sink: validation
// may run, but delivery success is never claimed.
type UnwiredStructuredEvidenceSink struct{}

func (UnwiredStructuredEvidenceSink) AcceptSoftwareInventory(_ context.Context, snap SoftwareInventorySnapshot) error {
	if err := snap.Validate(); err != nil {
		return err
	}
	return ErrStructuredTransportNotWired
}

func (UnwiredStructuredEvidenceSink) AcceptProcessServiceCatalogue(_ context.Context, cat ProcessServiceDiscoveryCatalogue) error {
	if err := cat.Validate(); err != nil {
		return err
	}
	return ErrStructuredTransportNotWired
}

func (UnwiredStructuredEvidenceSink) DeliveryState() StructuredEvidenceDeliveryState {
	return StructuredEvidenceTransportNotWired
}

// LocalRecordingStructuredEvidenceSink records validated evidence for tests.
// DeliveryState remains LOCAL_ONLY and never asserts physical transport success.
type LocalRecordingStructuredEvidenceSink struct {
	Inventories []SoftwareInventorySnapshot
	Catalogues  []ProcessServiceDiscoveryCatalogue
}

func (s *LocalRecordingStructuredEvidenceSink) AcceptSoftwareInventory(_ context.Context, snap SoftwareInventorySnapshot) error {
	if err := snap.Validate(); err != nil {
		return err
	}
	s.Inventories = append(s.Inventories, snap)
	return nil
}

func (s *LocalRecordingStructuredEvidenceSink) AcceptProcessServiceCatalogue(_ context.Context, cat ProcessServiceDiscoveryCatalogue) error {
	if err := cat.Validate(); err != nil {
		return err
	}
	s.Catalogues = append(s.Catalogues, cat)
	return nil
}

func (s *LocalRecordingStructuredEvidenceSink) DeliveryState() StructuredEvidenceDeliveryState {
	return StructuredEvidenceLocalOnly
}

// ClaimsPhysicalDelivery reports whether a sink state may be treated as successful
// Agent→Sensor structured delivery. Slice 1 always returns false.
func ClaimsPhysicalDelivery(state StructuredEvidenceDeliveryState) bool {
	switch state {
	case StructuredEvidenceTransportNotWired, StructuredEvidenceLocalOnly:
		return false
	default:
		return false
	}
}
