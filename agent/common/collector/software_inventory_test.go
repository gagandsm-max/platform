package collector

import (
	"strings"
	"testing"
	"time"
)

func validInventoryBase(t *testing.T) SoftwareInventorySnapshot {
	t.Helper()
	start := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	return SoftwareInventorySnapshot{
		SchemaVersion:          SoftwareInventorySchemaVersion,
		SnapshotID:             "snap-linux-dpkg-001",
		SourceMode:             InventorySourceModeAgent,
		SourceType:             InventorySourceTypeDPKG,
		InventoryScope:         "linux.dpkg.v1",
		SnapshotMode:           SnapshotModeFull,
		StartedAt:              start,
		CompletedAt:            start.Add(2 * time.Second),
		CaptureStatus:          CaptureStatusComplete,
		ItemCount:              1,
		Truncated:              false,
		IncompleteOrLossReason: "",
		SourceProvenance:       "agent.linux.fixture",
		Quality:                "AUTHORITATIVE_SOURCE",
		Items: []SoftwareInventoryItem{
			{
				NativePackageID:   "dpkg:amd64:openssl=3.0.13-0ubuntu3",
				ProductName:       "openssl",
				Version:           "3.0.13-0ubuntu3",
				VendorPublisher:   "Ubuntu Developers",
				Architecture:      "amd64",
				InstallationScope: InstallationScopeMachine,
			},
		},
	}
}

func TestSoftwareInventoryValidCompleteFull(t *testing.T) {
	snap := validInventoryBase(t)
	if err := snap.Validate(); err != nil {
		t.Fatalf("valid COMPLETE FULL rejected: %v", err)
	}
	if snap.RemovalInferencePermitted() {
		t.Fatal("Slice 1 must not permit removal inference")
	}
}

func TestSoftwareInventoryValidTargeted(t *testing.T) {
	snap := validInventoryBase(t)
	snap.SnapshotMode = SnapshotModeTargeted
	snap.SnapshotID = "snap-linux-dpkg-targeted-001"
	if err := snap.Validate(); err != nil {
		t.Fatalf("valid TARGETED rejected: %v", err)
	}
	if snap.RemovalInferencePermitted() {
		t.Fatal("TARGETED must not permit removal inference in Slice 1")
	}
}

func TestSoftwareInventoryPartialRequiresReason(t *testing.T) {
	snap := validInventoryBase(t)
	snap.CaptureStatus = CaptureStatusPartial
	snap.IncompleteOrLossReason = "source read interrupted"
	if err := snap.Validate(); err != nil {
		t.Fatalf("valid PARTIAL rejected: %v", err)
	}
	snap.IncompleteOrLossReason = ""
	if err := snap.Validate(); err == nil {
		t.Fatal("PARTIAL without reason must fail")
	}
}

func TestSoftwareInventoryFailedRequiresReason(t *testing.T) {
	snap := validInventoryBase(t)
	snap.CaptureStatus = CaptureStatusFailed
	snap.Items = nil
	snap.ItemCount = 0
	snap.IncompleteOrLossReason = "permission denied"
	if err := snap.Validate(); err != nil {
		t.Fatalf("valid FAILED rejected: %v", err)
	}
	snap.IncompleteOrLossReason = ""
	if err := snap.Validate(); err == nil {
		t.Fatal("FAILED without reason must fail")
	}
}

func TestSoftwareInventoryTruncatedState(t *testing.T) {
	snap := validInventoryBase(t)
	snap.CaptureStatus = CaptureStatusPartial
	snap.Truncated = true
	snap.IncompleteOrLossReason = "item safety bound reached"
	if err := snap.Validate(); err != nil {
		t.Fatalf("valid truncated PARTIAL rejected: %v", err)
	}
	snap.CaptureStatus = CaptureStatusComplete
	if err := snap.Validate(); err == nil {
		t.Fatal("COMPLETE+truncated must fail")
	}
}

func TestSoftwareInventoryInvalidCaptureStatusAndMode(t *testing.T) {
	snap := validInventoryBase(t)
	snap.CaptureStatus = "SUPPRESSED"
	if err := snap.Validate(); err == nil {
		t.Fatal("invalid capture_status must fail")
	}
	snap = validInventoryBase(t)
	snap.SnapshotMode = "DELTA"
	if err := snap.Validate(); err == nil {
		t.Fatal("invalid snapshot_mode must fail")
	}
}

func TestSoftwareInventoryInvalidSnapshotIdentity(t *testing.T) {
	cases := []string{"", "   ", "snap\x00id", strings.Repeat("a", maxSoftwareInventorySnapshotIDLen+1)}
	for _, id := range cases {
		snap := validInventoryBase(t)
		snap.SnapshotID = id
		if err := snap.Validate(); err == nil {
			t.Fatalf("snapshot_id %q must fail validation", id)
		}
	}
}

func TestSoftwareInventoryMalformedNativePackageIdentity(t *testing.T) {
	snap := validInventoryBase(t)
	snap.Items[0].NativePackageID = "bad\nid"
	if err := snap.Validate(); err == nil {
		t.Fatal("control character in native_package_id must fail")
	}
	snap = validInventoryBase(t)
	snap.Items[0].NativePackageID = ""
	if err := snap.Validate(); err == nil {
		t.Fatal("empty native_package_id must fail")
	}
}

func TestSoftwareInventoryOversizedBounds(t *testing.T) {
	snap := validInventoryBase(t)
	snap.Items[0].NativePackageID = strings.Repeat("p", maxSoftwareInventoryNativePackageIDLen+1)
	if err := snap.Validate(); err == nil {
		t.Fatal("oversized native_package_id must fail without truncation/aliasing")
	}
	snap = validInventoryBase(t)
	items := make([]SoftwareInventoryItem, maxSoftwareInventoryItems+1)
	for i := range items {
		items[i] = SoftwareInventoryItem{
			NativePackageID:   "pkg-" + intString(i),
			ProductName:       "pkg",
			Version:           "1",
			InstallationScope: InstallationScopeMachine,
		}
	}
	snap.Items = items
	snap.ItemCount = len(items)
	if err := snap.Validate(); err == nil {
		t.Fatal("oversized item list must fail")
	}
}

func TestSoftwareInventoryDuplicateNativeIdentitiesNotCollapsed(t *testing.T) {
	snap := validInventoryBase(t)
	snap.Items = []SoftwareInventoryItem{
		{
			NativePackageID:   "dpkg:amd64:openssl=3.0.13",
			ProductName:       "openssl",
			Version:           "3.0.13",
			InstallationScope: InstallationScopeMachine,
		},
		{
			NativePackageID:   "dpkg:amd64:openssl=3.0.13",
			ProductName:       "openssl",
			Version:           "3.0.13",
			InstallationScope: InstallationScopeMachine,
		},
	}
	snap.ItemCount = 2
	if err := snap.Validate(); err != nil {
		t.Fatalf("duplicate native identities must remain representable: %v", err)
	}
	dups := snap.DuplicateNativePackageIDs()
	if len(dups) != 1 || dups[0] != "dpkg:amd64:openssl=3.0.13" {
		t.Fatalf("expected explicit duplicate detection, got %#v", dups)
	}
}

func TestSoftwareInventoryNormalizedIdentityNotAuthoritative(t *testing.T) {
	snap := validInventoryBase(t)
	conf := 0.4
	snap.Items = []SoftwareInventoryItem{
		{
			NativePackageID:         "dpkg:amd64:libssl3=3.0.13",
			ProductName:             "libssl3",
			Version:                 "3.0.13",
			InstallationScope:       InstallationScopeMachine,
			NormalizedProductID:     "openssl",
			NormalizationVersion:    "norm-v1",
			NormalizationConfidence: &conf,
		},
		{
			NativePackageID:         "dpkg:amd64:openssl=3.0.13",
			ProductName:             "openssl",
			Version:                 "3.0.13",
			InstallationScope:       InstallationScopeMachine,
			NormalizedProductID:     "openssl",
			NormalizationVersion:    "norm-v1",
			NormalizationConfidence: &conf,
		},
	}
	snap.ItemCount = 2
	if err := snap.Validate(); err != nil {
		t.Fatalf("ambiguous normalized identity must remain valid: %v", err)
	}
	if snap.Items[0].NativePackageID == snap.Items[1].NativePackageID {
		t.Fatal("native identities must remain distinct")
	}
	if snap.Items[0].NormalizedProductID != snap.Items[1].NormalizedProductID {
		t.Fatal("fixture expects shared normalized id to prove non-authority")
	}
	snap.Items[0].NormalizationVersion = ""
	if err := snap.Validate(); err == nil {
		t.Fatal("normalized_product_id without normalization_version must fail")
	}
}

func TestSoftwareInventorySameSnapshotIDSemantics(t *testing.T) {
	a := validInventoryBase(t)
	b := validInventoryBase(t)
	if a.SnapshotID != b.SnapshotID {
		t.Fatal("same capture fixture must preserve identical snapshot_id")
	}
	b.SnapshotID = "snap-linux-dpkg-002"
	if a.SnapshotID == b.SnapshotID {
		t.Fatal("distinct captures must be able to carry distinct snapshot_id")
	}
	if err := a.Validate(); err != nil || b.Validate() != nil {
		t.Fatal("both snapshots must validate")
	}
}

func intString(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
