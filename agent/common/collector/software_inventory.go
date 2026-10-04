package collector

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Software inventory typed source evidence (BUILD-011 Slice 1).
//
// This is an Agent-side internal model aligned to the canonical
// software_inventory_snapshot contract. It is not a physical wire envelope,
// not a persistence contract, and not a HostCollector scalar Sample path.
// Downstream reconciliation/change events remain BUILD-098.

const SoftwareInventorySchemaVersion = 1

// Implementation safety bounds for parser/memory protection. These are NOT
// customer-facing product capacity contracts; live collectors may apply tighter
// caller-supplied limits in later slices.
const (
	maxSoftwareInventoryItems              = 4096
	maxSoftwareInventorySnapshotIDLen      = 128
	maxSoftwareInventoryScopeLen           = 256
	maxSoftwareInventorySourceTypeLen      = 64
	maxSoftwareInventoryReasonLen          = 512
	maxSoftwareInventoryProvenanceLen      = 512
	maxSoftwareInventoryQualityLen         = 128
	maxSoftwareInventoryNativePackageIDLen = 512
	maxSoftwareInventoryProductNameLen     = 256
	maxSoftwareInventoryVersionLen         = 128
	maxSoftwareInventoryVendorLen          = 256
	maxSoftwareInventoryArchitectureLen    = 64
	maxSoftwareInventoryNormalizedIDLen    = 256
	maxSoftwareInventoryNormVersionLen     = 64
)

type SnapshotMode string

const (
	SnapshotModeFull     SnapshotMode = "FULL"
	SnapshotModeTargeted SnapshotMode = "TARGETED"
)

type CaptureStatus string

const (
	CaptureStatusComplete CaptureStatus = "COMPLETE"
	CaptureStatusPartial  CaptureStatus = "PARTIAL"
	CaptureStatusFailed   CaptureStatus = "FAILED"
)

type InventorySourceMode string

const (
	InventorySourceModeAgent      InventorySourceMode = "AGENT"
	InventorySourceModeSSH        InventorySourceMode = "SSH"
	InventorySourceModeWinRMWMICIM InventorySourceMode = "WINRM_WMI_CIM"
)

type InventorySourceType string

const (
	InventorySourceTypeDPKG                 InventorySourceType = "DPKG"
	InventorySourceTypeRPM                  InventorySourceType = "RPM"
	InventorySourceTypeMSI                  InventorySourceType = "MSI"
	InventorySourceTypeWindowsProductSource InventorySourceType = "WINDOWS_PRODUCT_SOURCE"
	InventorySourceTypePackageManager       InventorySourceType = "PACKAGE_MANAGER"
	InventorySourceTypeOtherCertified       InventorySourceType = "OTHER_CERTIFIED"
)

type InstallationScope string

const (
	InstallationScopeMachine InstallationScope = "MACHINE"
	InstallationScopeUser    InstallationScope = "USER"
	InstallationScopeUnknown InstallationScope = "UNKNOWN"
)

// SoftwareInventoryItem retains authoritative native package identity.
// NormalizedProductID is optional, versioned and never authoritative.
type SoftwareInventoryItem struct {
	NativePackageID                 string
	ProductName                     string
	Version                         string
	VendorPublisher                 string // optional
	Architecture                    string // optional
	InstallationScope               InstallationScope
	AuthoritativeInstallOrUpdateAt  *time.Time // nullable / provenance-qualified when set
	NormalizedProductID             string     // optional, non-authoritative
	NormalizationVersion            string     // required when NormalizedProductID is set
	NormalizationConfidence         *float64   // optional; must be in [0,1] when set
}

// SoftwareInventorySnapshot is the typed Agent-side inventory source evidence.
// Endpoint/tenant/entity authority is inherited later from transport/Core and
// must not be self-asserted here as canonical entity truth.
type SoftwareInventorySnapshot struct {
	SchemaVersion           int
	SnapshotID              string
	SourceMode              InventorySourceMode
	SourceType              InventorySourceType
	InventoryScope          string
	SnapshotMode            SnapshotMode
	StartedAt               time.Time
	CompletedAt             time.Time
	CaptureStatus           CaptureStatus
	ItemCount               int
	Truncated               bool
	IncompleteOrLossReason  string
	SourceProvenance        string
	Quality                 string
	Items                   []SoftwareInventoryItem
}

// Validate checks deterministic typed-source invariants for one snapshot.
// It validates identity and state; it does not own capture lifecycle, Core
// replay ordering, current-state reconciliation or SOFTWARE_* event generation.
func (s SoftwareInventorySnapshot) Validate() error {
	if s.SchemaVersion != SoftwareInventorySchemaVersion {
		return fmt.Errorf("SOFTWARE_INVENTORY_SCHEMA_INVALID: unsupported schema_version %d", s.SchemaVersion)
	}
	if err := requireBoundedToken("snapshot_id", s.SnapshotID, maxSoftwareInventorySnapshotIDLen); err != nil {
		return err
	}
	switch s.SourceMode {
	case InventorySourceModeAgent, InventorySourceModeSSH, InventorySourceModeWinRMWMICIM:
	default:
		return fmt.Errorf("SOFTWARE_INVENTORY_SOURCE_MODE_INVALID: %q", s.SourceMode)
	}
	if err := requireBoundedToken("source_type", string(s.SourceType), maxSoftwareInventorySourceTypeLen); err != nil {
		return err
	}
	switch s.SourceType {
	case InventorySourceTypeDPKG, InventorySourceTypeRPM, InventorySourceTypeMSI,
		InventorySourceTypeWindowsProductSource, InventorySourceTypePackageManager,
		InventorySourceTypeOtherCertified:
	default:
		return fmt.Errorf("SOFTWARE_INVENTORY_SOURCE_TYPE_INVALID: %q", s.SourceType)
	}
	if err := requireBoundedToken("inventory_scope", s.InventoryScope, maxSoftwareInventoryScopeLen); err != nil {
		return err
	}
	switch s.SnapshotMode {
	case SnapshotModeFull, SnapshotModeTargeted:
	default:
		return fmt.Errorf("SOFTWARE_INVENTORY_SNAPSHOT_MODE_INVALID: %q", s.SnapshotMode)
	}
	switch s.CaptureStatus {
	case CaptureStatusComplete, CaptureStatusPartial, CaptureStatusFailed:
	default:
		return fmt.Errorf("SOFTWARE_INVENTORY_CAPTURE_STATUS_INVALID: %q", s.CaptureStatus)
	}
	if s.StartedAt.IsZero() || s.CompletedAt.IsZero() {
		return errors.New("SOFTWARE_INVENTORY_TIME_INVALID: started_at and completed_at are required")
	}
	if s.CompletedAt.Before(s.StartedAt) {
		return errors.New("SOFTWARE_INVENTORY_TIME_INVALID: completed_at must not precede started_at")
	}
	if err := optionalBoundedText("incomplete_or_loss_reason", s.IncompleteOrLossReason, maxSoftwareInventoryReasonLen); err != nil {
		return err
	}
	if err := requireBoundedToken("source_provenance", s.SourceProvenance, maxSoftwareInventoryProvenanceLen); err != nil {
		return err
	}
	if err := requireBoundedToken("quality", s.Quality, maxSoftwareInventoryQualityLen); err != nil {
		return err
	}
	if len(s.Items) > maxSoftwareInventoryItems {
		return fmt.Errorf(
			"SOFTWARE_INVENTORY_ITEM_BOUND_EXCEEDED: implementation safety bound is %d items, got %d",
			maxSoftwareInventoryItems,
			len(s.Items),
		)
	}
	if s.ItemCount != len(s.Items) {
		return fmt.Errorf(
			"SOFTWARE_INVENTORY_ITEM_COUNT_MISMATCH: item_count=%d len(items)=%d",
			s.ItemCount,
			len(s.Items),
		)
	}
	if s.CaptureStatus == CaptureStatusComplete && s.Truncated {
		return errors.New("SOFTWARE_INVENTORY_STATE_INVALID: COMPLETE capture cannot be truncated")
	}
	if s.CaptureStatus == CaptureStatusPartial || s.CaptureStatus == CaptureStatusFailed || s.Truncated {
		if strings.TrimSpace(s.IncompleteOrLossReason) == "" {
			return errors.New("SOFTWARE_INVENTORY_REASON_REQUIRED: PARTIAL/FAILED/truncated captures require incomplete_or_loss_reason")
		}
	}
	for i, item := range s.Items {
		if err := item.validate(i); err != nil {
			return err
		}
	}
	return nil
}

// DuplicateNativePackageIDs reports native identities that appear more than once.
// Duplicates remain explicitly detectable; they are never silently collapsed.
func (s SoftwareInventorySnapshot) DuplicateNativePackageIDs() []string {
	counts := make(map[string]int, len(s.Items))
	for _, item := range s.Items {
		id := strings.TrimSpace(item.NativePackageID)
		if id == "" {
			continue
		}
		counts[id]++
	}
	var dups []string
	for id, n := range counts {
		if n > 1 {
			dups = append(dups, id)
		}
	}
	return dups
}

// RemovalInferencePermitted is always false in BUILD-011 Slice 1.
// Absence/removal inference is a Core/BUILD-098 responsibility and only from
// COMPLETE + non-truncated + authoritative FULL snapshots under the same
// endpoint/source/inventory_scope comparison key.
func (s SoftwareInventorySnapshot) RemovalInferencePermitted() bool {
	return false
}

func (item SoftwareInventoryItem) validate(index int) error {
	prefix := fmt.Sprintf("SOFTWARE_INVENTORY_ITEM[%d]", index)
	if err := requireBoundedToken(prefix+".native_package_id", item.NativePackageID, maxSoftwareInventoryNativePackageIDLen); err != nil {
		return err
	}
	if err := requireBoundedToken(prefix+".product_name", item.ProductName, maxSoftwareInventoryProductNameLen); err != nil {
		return err
	}
	if err := requireBoundedToken(prefix+".version", item.Version, maxSoftwareInventoryVersionLen); err != nil {
		return err
	}
	if err := optionalBoundedText(prefix+".vendor_publisher", item.VendorPublisher, maxSoftwareInventoryVendorLen); err != nil {
		return err
	}
	if err := optionalBoundedText(prefix+".architecture", item.Architecture, maxSoftwareInventoryArchitectureLen); err != nil {
		return err
	}
	switch item.InstallationScope {
	case InstallationScopeMachine, InstallationScopeUser, InstallationScopeUnknown:
	default:
		return fmt.Errorf("%s.installation_scope invalid: %q", prefix, item.InstallationScope)
	}
	normID := strings.TrimSpace(item.NormalizedProductID)
	if normID != "" {
		if err := requireBoundedToken(prefix+".normalized_product_id", item.NormalizedProductID, maxSoftwareInventoryNormalizedIDLen); err != nil {
			return err
		}
		if err := requireBoundedToken(prefix+".normalization_version", item.NormalizationVersion, maxSoftwareInventoryNormVersionLen); err != nil {
			return err
		}
	} else if strings.TrimSpace(item.NormalizationVersion) != "" {
		return fmt.Errorf("%s.normalization_version set without normalized_product_id", prefix)
	}
	if item.NormalizationConfidence != nil {
		c := *item.NormalizationConfidence
		if c < 0 || c > 1 {
			return fmt.Errorf("%s.normalization_confidence out of range: %v", prefix, c)
		}
	}
	return nil
}

func requireBoundedToken(field, value string, maxLen int) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return fmt.Errorf("SOFTWARE_INVENTORY_FIELD_INVALID: %s is required", field)
	}
	if len(v) > maxLen {
		// Explicit failure — never silently truncate authoritative identifiers.
		return fmt.Errorf("SOFTWARE_INVENTORY_FIELD_BOUND_EXCEEDED: %s exceeds implementation safety bound %d", field, maxLen)
	}
	if hasControlRunes(v) {
		return fmt.Errorf("SOFTWARE_INVENTORY_FIELD_INVALID: %s contains control characters", field)
	}
	if v != value {
		return fmt.Errorf("SOFTWARE_INVENTORY_FIELD_INVALID: %s has leading/trailing whitespace", field)
	}
	return nil
}

func optionalBoundedText(field, value string, maxLen int) error {
	if value == "" {
		return nil
	}
	if len(value) > maxLen {
		return fmt.Errorf("SOFTWARE_INVENTORY_FIELD_BOUND_EXCEEDED: %s exceeds implementation safety bound %d", field, maxLen)
	}
	if hasControlRunes(value) {
		return fmt.Errorf("SOFTWARE_INVENTORY_FIELD_INVALID: %s contains control characters", field)
	}
	return nil
}

func hasControlRunes(v string) bool {
	return strings.IndexFunc(v, unicode.IsControl) >= 0
}
