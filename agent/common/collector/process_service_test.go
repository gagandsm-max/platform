package collector

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"platform/agent/common/telemetry"
)

func TestProcessServiceSelectorValidKinds(t *testing.T) {
	cases := []ProcessServiceSelector{
		{Kind: SelectorKindServiceID, Value: "nginx.service"},
		{Kind: SelectorKindExecutableIdentity, Value: "sha256:abcdef0123456789"},
		{Kind: SelectorKindExecutablePath, Value: "/usr/sbin/nginx"},
		{Kind: SelectorKindProcessName, Value: "nginx"},
		{Kind: SelectorKindProcessName, Value: "c++"},
	}
	for _, sel := range cases {
		if err := sel.Validate(); err != nil {
			t.Fatalf("valid selector %#v rejected: %v", sel, err)
		}
	}
	serviceRank := (ProcessServiceSelector{Kind: SelectorKindServiceID, Value: "a"}).SelectorPrecedenceRank()
	nameRank := (ProcessServiceSelector{Kind: SelectorKindProcessName, Value: "a"}).SelectorPrecedenceRank()
	if serviceRank >= nameRank {
		t.Fatal("SERVICE_ID must outrank PROCESS_NAME")
	}
}

func TestProcessServiceSelectorRejectsPIDAndUnsafe(t *testing.T) {
	cases := []ProcessServiceSelector{
		{Kind: "PID", Value: "1234"},
		{Kind: "PROCESS_ID", Value: "1234"},
		{Kind: SelectorKindProcessName, Value: "1234"},
		{Kind: "", Value: "nginx"},
		{Kind: SelectorKindServiceID, Value: ""},
		{Kind: SelectorKindServiceID, Value: "nginx\x00.service"},
		{Kind: SelectorKindProcessName, Value: "ngin*"},
		{Kind: SelectorKindProcessName, Value: "nginx|evil"},
		{Kind: SelectorKindExecutablePath, Value: "/bin/sh -c evil"},
		{Kind: SelectorKindProcessName, Value: "bash -c id"},
		{Kind: "REGEX", Value: "nginx.*"},
		{Kind: "GLOB", Value: "nginx*"},
		{Kind: "SHELL", Value: "systemctl status nginx"},
		{Kind: "COMMAND", Value: "ps aux"},
		{Kind: SelectorKindServiceID, Value: strings.Repeat("n", maxProcessServiceSelectorValueLen+1)},
	}
	for _, sel := range cases {
		if err := sel.Validate(); err == nil {
			t.Fatalf("selector %#v must be rejected", sel)
		}
	}
}

func TestProcessServiceCatalogueStableIdentitySeparateFromPID(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cat := ProcessServiceDiscoveryCatalogue{
		SchemaVersion:    ProcessServiceSchemaVersion,
		CatalogueID:      "cat-linux-001",
		CollectedAt:      now,
		SourceProvenance: "agent.linux.fixture",
		Quality:          "OBSERVED",
		Entries: []ProcessServiceCatalogueEntry{
			{
				ServiceID:          "nginx.service",
				ExecutableIdentity: "sha256:abc",
				ExecutablePath:     "/usr/sbin/nginx",
				ProcessFamilyName:  "nginx",
				Runtime: &ProcessServiceRuntimeInstance{
					PID:        4242,
					ObservedAt: now,
				},
				SourceProvenance: "systemd+proc",
				Quality:          "OBSERVED",
			},
		},
	}
	if err := cat.Validate(); err != nil {
		t.Fatalf("valid catalogue rejected: %v", err)
	}
	sel, err := cat.Entries[0].PreferredDurableSelector()
	if err != nil {
		t.Fatalf("preferred selector: %v", err)
	}
	if sel.Kind != SelectorKindServiceID || sel.Value != "nginx.service" {
		t.Fatalf("expected SERVICE_ID selector, got %#v", sel)
	}
	if sel.Value == "4242" || sel.Kind == "PID" {
		t.Fatal("PID must not become durable selector")
	}
}

func TestProcessServiceCataloguePIDOnlyRejected(t *testing.T) {
	now := time.Now().UTC()
	cat := ProcessServiceDiscoveryCatalogue{
		SchemaVersion:    ProcessServiceSchemaVersion,
		CatalogueID:      "cat-linux-002",
		CollectedAt:      now,
		SourceProvenance: "agent.linux.fixture",
		Quality:          "OBSERVED",
		Entries: []ProcessServiceCatalogueEntry{
			{
				Runtime: &ProcessServiceRuntimeInstance{PID: 7, ObservedAt: now},
			},
		},
	}
	if err := cat.Validate(); err == nil {
		t.Fatal("PID-only catalogue entry must fail")
	}
}

func TestProcessServiceCatalogueBoundsAndAmbiguousIdentity(t *testing.T) {
	now := time.Now().UTC()
	cat := ProcessServiceDiscoveryCatalogue{
		SchemaVersion:    ProcessServiceSchemaVersion,
		CatalogueID:      "cat-linux-003",
		CollectedAt:      now,
		SourceProvenance: "agent.linux.fixture",
		Quality:          "PARTIAL",
		Truncated:        true,
		IncompleteReason: "entry safety bound reached",
		Entries: []ProcessServiceCatalogueEntry{
			{
				ProcessFamilyName: "worker",
				SourceProvenance:  "proc",
				Quality:           "AMBIGUOUS_NAME_ONLY",
			},
		},
	}
	if err := cat.Validate(); err != nil {
		t.Fatalf("name-only ambiguous entry should validate with explicit quality: %v", err)
	}
	sel, err := cat.Entries[0].PreferredDurableSelector()
	if err != nil {
		t.Fatalf("process-name fallback selector: %v", err)
	}
	if sel.Kind != SelectorKindProcessName {
		t.Fatalf("expected PROCESS_NAME fallback, got %s", sel.Kind)
	}
	// Oversized entries
	entries := make([]ProcessServiceCatalogueEntry, maxProcessServiceCatalogueEntries+1)
	for i := range entries {
		entries[i] = ProcessServiceCatalogueEntry{
			ProcessFamilyName: "worker-" + intString(i),
			SourceProvenance:  "proc",
			Quality:           "OBSERVED",
		}
	}
	cat.Entries = entries
	cat.Truncated = false
	cat.IncompleteReason = ""
	if err := cat.Validate(); err == nil {
		t.Fatal("oversized catalogue must fail")
	}
}

func TestStructuredEvidenceSinkUnwiredAndLocalOnly(t *testing.T) {
	ctx := context.Background()
	snap := validInventoryBase(t)
	cat := ProcessServiceDiscoveryCatalogue{
		SchemaVersion:    ProcessServiceSchemaVersion,
		CatalogueID:      "cat-linux-sink",
		CollectedAt:      time.Now().UTC(),
		SourceProvenance: "agent.linux.fixture",
		Quality:          "OBSERVED",
		Entries: []ProcessServiceCatalogueEntry{
			{ServiceID: "sshd.service", SourceProvenance: "systemd", Quality: "OBSERVED"},
		},
	}

	var unwired UnwiredStructuredEvidenceSink
	if unwired.DeliveryState() != StructuredEvidenceTransportNotWired {
		t.Fatal("unwired sink must report TRANSPORT_NOT_WIRED")
	}
	if ClaimsPhysicalDelivery(unwired.DeliveryState()) {
		t.Fatal("unwired sink must not claim physical delivery")
	}
	if err := unwired.AcceptSoftwareInventory(ctx, snap); !errors.Is(err, ErrStructuredTransportNotWired) {
		t.Fatalf("expected transport-not-wired, got %v", err)
	}
	if err := unwired.AcceptProcessServiceCatalogue(ctx, cat); !errors.Is(err, ErrStructuredTransportNotWired) {
		t.Fatalf("expected transport-not-wired, got %v", err)
	}

	local := &LocalRecordingStructuredEvidenceSink{}
	if err := local.AcceptSoftwareInventory(ctx, snap); err != nil {
		t.Fatalf("local sink inventory: %v", err)
	}
	if err := local.AcceptProcessServiceCatalogue(ctx, cat); err != nil {
		t.Fatalf("local sink catalogue: %v", err)
	}
	if local.DeliveryState() != StructuredEvidenceLocalOnly {
		t.Fatal("local sink must report LOCAL_ONLY")
	}
	if ClaimsPhysicalDelivery(local.DeliveryState()) {
		t.Fatal("local sink must not claim physical delivery success")
	}
	if len(local.Inventories) != 1 || len(local.Catalogues) != 1 {
		t.Fatal("local sink should record validated evidence")
	}
}

func TestStructuredEvidenceNotHostCollectorOrScalarSample(t *testing.T) {
	snapType := reflect.TypeOf(SoftwareInventorySnapshot{})
	catType := reflect.TypeOf(ProcessServiceDiscoveryCatalogue{})
	hostCollector := reflect.TypeOf((*HostCollector)(nil)).Elem()
	if snapType.Implements(hostCollector) || reflect.PointerTo(snapType).Implements(hostCollector) {
		t.Fatal("SoftwareInventorySnapshot must not implement HostCollector")
	}
	if catType.Implements(hostCollector) || reflect.PointerTo(catType).Implements(hostCollector) {
		t.Fatal("ProcessServiceDiscoveryCatalogue must not implement HostCollector")
	}

	// Compile-time/shape proof: structured models are not telemetry.Sample.
	var sample telemetry.Sample
	_ = sample
	if reflect.TypeOf(SoftwareInventorySnapshot{}) == reflect.TypeOf(telemetry.Sample{}) {
		t.Fatal("inventory snapshot must not be telemetry.Sample")
	}
	if reflect.TypeOf(ProcessServiceDiscoveryCatalogue{}) == reflect.TypeOf(telemetry.Sample{}) {
		t.Fatal("catalogue must not be telemetry.Sample")
	}

	// Explicit negative: no conversion helper may treat structured evidence as scalar labels.
	if _, ok := any(SoftwareInventorySnapshot{}).(telemetry.Sample); ok {
		t.Fatal("inventory snapshot must not convert to Sample")
	}
	if _, ok := any(ProcessServiceDiscoveryCatalogue{}).(telemetry.Sample); ok {
		t.Fatal("catalogue must not convert to Sample")
	}
}

func TestPackageMetadataAndSelectorsAreNotShellInput(t *testing.T) {
	snap := validInventoryBase(t)
	// Control-bearing package metadata is rejected (no silent normalization into shell input).
	snap.Items[0].ProductName = "openssl\n"
	if err := snap.Validate(); err == nil {
		t.Fatal("control characters in package metadata must be rejected")
	}
	// Selector values that look like shell/command forms are rejected.
	for _, sel := range []ProcessServiceSelector{
		{Kind: SelectorKindProcessName, Value: "sh -c id"},
		{Kind: SelectorKindExecutablePath, Value: "/bin/sh -c evil"},
		{Kind: SelectorKindProcessName, Value: "nginx;reboot"},
	} {
		if err := sel.Validate(); err == nil {
			t.Fatalf("shell-form selector %#v must be rejected", sel)
		}
	}
	// Inventory/selector models expose no execute/command API.
	snapType := reflect.TypeOf(SoftwareInventorySnapshot{})
	selType := reflect.TypeOf(ProcessServiceSelector{})
	for _, typ := range []reflect.Type{snapType, selType} {
		for i := 0; i < typ.NumMethod(); i++ {
			name := typ.Method(i).Name
			lower := strings.ToLower(name)
			if strings.Contains(lower, "exec") || strings.Contains(lower, "command") || strings.Contains(lower, "shell") {
				t.Fatalf("%s exposes forbidden method %s", typ.Name(), name)
			}
		}
	}
}
