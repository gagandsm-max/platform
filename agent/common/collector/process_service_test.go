package collector

import (
	"context"
	"errors"
	"fmt"
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

func TestProcessServiceSelectorAcceptsLiteralExactValues(t *testing.T) {
	// Exact selector VALUES are literal data. Metacharacters and numeric names
	// remain representable because KIND + exact match + non-execution is the boundary.
	cases := []ProcessServiceSelector{
		{Kind: SelectorKindProcessName, Value: "1234"},
		{Kind: SelectorKindProcessName, Value: "worker*blue"},
		{Kind: SelectorKindProcessName, Value: "name[1]"},
		{Kind: SelectorKindProcessName, Value: "bash -c id"},
		{Kind: SelectorKindExecutablePath, Value: "/opt/acme/bin/app;blue"},
		{Kind: SelectorKindExecutablePath, Value: "/opt/My App/bin/service"},
		{Kind: SelectorKindServiceID, Value: "literal$name.service"},
		{Kind: SelectorKindProcessName, Value: "nginx|worker"},
		{Kind: SelectorKindExecutablePath, Value: "/bin/sh -c evil"},
	}
	for _, sel := range cases {
		if err := sel.Validate(); err != nil {
			t.Fatalf("literal exact selector %#v must be accepted: %v", sel, err)
		}
	}
}

func TestProcessServiceSelectorRejectsForbiddenKindsAndInvalidValues(t *testing.T) {
	cases := []ProcessServiceSelector{
		{Kind: "PID", Value: "1234"},
		{Kind: "PROCESS_ID", Value: "1234"},
		{Kind: "REGEX", Value: "nginx.*"},
		{Kind: "GLOB", Value: "nginx*"},
		{Kind: "SHELL", Value: "systemctl status nginx"},
		{Kind: "COMMAND", Value: "ps aux"},
		{Kind: "SCRIPT", Value: "restart.sh"},
		{Kind: "", Value: "nginx"},
		{Kind: SelectorKindServiceID, Value: ""},
		{Kind: SelectorKindServiceID, Value: "  nginx.service"},
		{Kind: SelectorKindServiceID, Value: "nginx.service  "},
		{Kind: SelectorKindServiceID, Value: "nginx\x00.service"},
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

func TestProcessServiceRuntimeValidation(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	base := func(rt *ProcessServiceRuntimeInstance) ProcessServiceDiscoveryCatalogue {
		return ProcessServiceDiscoveryCatalogue{
			SchemaVersion:    ProcessServiceSchemaVersion,
			CatalogueID:      "cat-runtime-val",
			CollectedAt:      now,
			SourceProvenance: "agent.linux.fixture",
			Quality:          "OBSERVED",
			Entries: []ProcessServiceCatalogueEntry{
				{
					ServiceID:        "sshd.service",
					Runtime:          rt,
					SourceProvenance: "systemd",
					Quality:          "OBSERVED",
				},
			},
		}
	}

	if err := base(&ProcessServiceRuntimeInstance{PID: 1, ObservedAt: now}).Validate(); err != nil {
		t.Fatalf("valid PID + ObservedAt must be accepted: %v", err)
	}
	if err := base(&ProcessServiceRuntimeInstance{PID: 0, ObservedAt: now}).Validate(); err == nil {
		t.Fatal("runtime PID 0 must be rejected")
	}
	if err := base(&ProcessServiceRuntimeInstance{PID: -1, ObservedAt: now}).Validate(); err == nil {
		t.Fatal("negative runtime PID must be rejected")
	}
	if err := base(&ProcessServiceRuntimeInstance{PID: 42, ObservedAt: time.Time{}}).Validate(); err == nil {
		t.Fatal("runtime with zero ObservedAt must be rejected")
	}
	if err := base(&ProcessServiceRuntimeInstance{
		PID:           42,
		ObservedAt:    now,
		InstanceToken: strings.Repeat("t", maxProcessServiceInstanceTokenLen+1),
	}).Validate(); err == nil {
		t.Fatal("oversized InstanceToken must be rejected without truncation")
	}
	if err := base(&ProcessServiceRuntimeInstance{
		PID:           42,
		ObservedAt:    now,
		InstanceToken: "tok\x00en",
	}).Validate(); err == nil {
		t.Fatal("control character in InstanceToken must be rejected")
	}
}

func TestProcessServiceCatalogueIncompleteReasonValidation(t *testing.T) {
	now := time.Now().UTC()
	cat := ProcessServiceDiscoveryCatalogue{
		SchemaVersion:    ProcessServiceSchemaVersion,
		CatalogueID:      "cat-reason-val",
		CollectedAt:      now,
		SourceProvenance: "agent.linux.fixture",
		Quality:          "PARTIAL",
		Truncated:        true,
		IncompleteReason: "entry safety bound reached",
		Entries: []ProcessServiceCatalogueEntry{
			{ProcessFamilyName: "worker", SourceProvenance: "proc", Quality: "OBSERVED"},
		},
	}
	if err := cat.Validate(); err != nil {
		t.Fatalf("valid truncated catalogue rejected: %v", err)
	}

	cat.IncompleteReason = ""
	if err := cat.Validate(); err == nil {
		t.Fatal("truncated catalogue without reason must be rejected")
	}

	cat.IncompleteReason = strings.Repeat("r", maxProcessServiceIncompleteReasonLen+1)
	if err := cat.Validate(); err == nil {
		t.Fatal("oversized IncompleteReason must be rejected without truncation")
	}

	cat.IncompleteReason = "bound\x00reached"
	if err := cat.Validate(); err == nil {
		t.Fatal("control character in IncompleteReason must be rejected")
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

func TestProcessServiceExactSelectorMatchingAndPrecedence(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	nginx := ProcessServiceCatalogueEntry{
		ServiceID:          "nginx.service",
		ExecutableIdentity: "linux-dev-ino:1:2:3",
		ExecutablePath:     "/usr/sbin/nginx",
		ProcessFamilyName:  "nginx",
		Runtime:            &ProcessServiceRuntimeInstance{PID: 100, ObservedAt: now, InstanceToken: "100:10"},
		SourceProvenance:   "proc",
		Quality:            "OBSERVED",
	}
	sshd := ProcessServiceCatalogueEntry{
		ProcessFamilyName: "sshd",
		ExecutablePath:    "/usr/sbin/sshd",
		Runtime:           &ProcessServiceRuntimeInstance{PID: 100, ObservedAt: now, InstanceToken: "100:99"},
		SourceProvenance:  "proc",
		Quality:           "OBSERVED",
	}
	selService := ProcessServiceSelector{Kind: SelectorKindServiceID, Value: "nginx.service"}
	selPath := ProcessServiceSelector{Kind: SelectorKindExecutablePath, Value: "/usr/sbin/nginx"}
	selName := ProcessServiceSelector{Kind: SelectorKindProcessName, Value: "nginx"}
	selIdent := ProcessServiceSelector{Kind: SelectorKindExecutableIdentity, Value: "linux-dev-ino:1:2:3"}
	if !selService.MatchesEntry(nginx) || selService.MatchesEntry(sshd) {
		t.Fatal("SERVICE_ID matching must be exact")
	}
	if selName.MatchesEntry(sshd) {
		t.Fatal("PROCESS_NAME nginx must not match sshd")
	}
	caseName := ProcessServiceSelector{Kind: SelectorKindProcessName, Value: "Nginx"}
	if caseName.MatchesEntry(nginx) {
		t.Fatal("matching is case-sensitive exact")
	}
	pref, err := nginx.PreferredDurableSelector()
	if err != nil || pref.Kind != SelectorKindServiceID {
		t.Fatalf("precedence must prefer SERVICE_ID, got %#v err=%v", pref, err)
	}
	pathOnly := ProcessServiceCatalogueEntry{ExecutablePath: "/usr/sbin/nginx", ProcessFamilyName: "nginx"}
	pref, err = pathOnly.PreferredDurableSelector()
	if err != nil || pref.Kind != SelectorKindExecutablePath {
		t.Fatalf("path must outrank name, got %#v", pref)
	}
	matches := MatchProcessServiceSelectors([]ProcessServiceCatalogueEntry{nginx, sshd}, []ProcessServiceSelector{selService, selPath, selIdent, selName})
	for _, m := range matches {
		if !m.Matched || m.SourceClass != ProcessServiceSourceObserved || len(m.Entries) != 1 {
			t.Fatalf("expected observed match %#v", m)
		}
	}
}

func TestProcessServicePIDRejectedAndReuseDoesNotRetarget(t *testing.T) {
	now := time.Now().UTC()
	pidSel := ProcessServiceSelector{Kind: "PID", Value: "100"}
	if err := pidSel.Validate(); err == nil {
		t.Fatal("PID must not be a durable selector")
	}
	first := ProcessServiceCatalogueEntry{
		ProcessFamilyName: "nginx",
		Runtime:           &ProcessServiceRuntimeInstance{PID: 100, ObservedAt: now, StartTimeTicks: 10, HasStartTime: true, InstanceToken: "100:10"},
		SourceProvenance:  "proc",
		Quality:           "OBSERVED",
	}
	reused := ProcessServiceCatalogueEntry{
		ProcessFamilyName: "sshd",
		Runtime:           &ProcessServiceRuntimeInstance{PID: 100, ObservedAt: now, StartTimeTicks: 50, HasStartTime: true, InstanceToken: "100:50"},
		SourceProvenance:  "proc",
		Quality:           "OBSERVED",
	}
	sel := ProcessServiceSelector{Kind: SelectorKindProcessName, Value: "nginx"}
	if sel.MatchesEntry(reused) {
		t.Fatal("PID reuse must not retarget PROCESS_NAME selector")
	}
	if !sel.MatchesEntry(first) {
		t.Fatal("original identity must still match")
	}
	if first.Runtime.PID != reused.Runtime.PID {
		t.Fatal("fixture must share ephemeral PID")
	}
	if first.Runtime.InstanceToken == reused.Runtime.InstanceToken {
		t.Fatal("start-time instance token must change on PID reuse")
	}
	unmatched := MatchProcessServiceSelectors([]ProcessServiceCatalogueEntry{reused}, []ProcessServiceSelector{sel})
	if unmatched[0].Matched || unmatched[0].SourceClass != ProcessServiceSourceSelectorUnmatched {
		t.Fatalf("unmatched selector class: %#v", unmatched[0])
	}
	if SourceClassAssertsProcessOrHostDown(unmatched[0].SourceClass) {
		t.Fatal("unmatched selector must not assert DOWN")
	}
}

func TestProcessServiceSourceErrorsDoNotAssertDown(t *testing.T) {
	classes := []ProcessServiceSourceClass{
		ProcessServiceSourceObserved,
		ProcessServiceSourceUnavailable,
		ProcessServiceSourcePermissionDenied,
		ProcessServiceSourceUnsupported,
		ProcessServiceSourcePIDRace,
		ProcessServiceSourceMalformed,
		ProcessServiceSourceTruncated,
		ProcessServiceSourceSelectorUnmatched,
	}
	for _, c := range classes {
		if SourceClassAssertsProcessOrHostDown(c) {
			t.Fatalf("source class %s must not assert DOWN", c)
		}
	}
	snap := ProcessServiceSourceSnapshot{
		ProcessSourceClass: ProcessServiceSourceUnavailable,
		SystemdSourceClass: ProcessServiceSourcePermissionDenied,
		Services: []SystemdServiceObservation{{
			UnitID: "sshd.service", ActiveState: "failed", Result: "exit-code",
			SourceClass: ProcessServiceSourceObserved, Quality: "OBSERVED",
		}},
		Matches: []ProcessServiceSelectorMatch{{
			Selector:    ProcessServiceSelector{Kind: SelectorKindProcessName, Value: "missing"},
			SourceClass: ProcessServiceSourceSelectorUnmatched,
		}},
	}
	if snap.AssertsHostOrServiceDown() {
		t.Fatal("snapshot source failures must not assert host/service DOWN")
	}
}

func TestEncodeProcessServiceCatalogueAsScalarSamplesRejected(t *testing.T) {
	cat := ProcessServiceDiscoveryCatalogue{
		SchemaVersion:    ProcessServiceSchemaVersion,
		CatalogueID:      "cat-scalar-reject",
		CollectedAt:      time.Now().UTC(),
		SourceProvenance: "agent.linux.fixture",
		Quality:          "OBSERVED",
		Entries:          []ProcessServiceCatalogueEntry{{ProcessFamilyName: "nginx", SourceProvenance: "proc", Quality: "OBSERVED"}},
	}
	samples, err := EncodeProcessServiceCatalogueAsScalarSamples(cat)
	if err == nil || samples != nil {
		t.Fatal("structured catalogue must not encode as scalar samples")
	}
	if !strings.Contains(err.Error(), "PROCESS_SERVICE_SCALAR_ENCODING_REJECTED") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestSystemdArgvIsFixedReadOnlyNoShell(t *testing.T) {
	if !systemctlExecutableAllowed("/usr/bin/systemctl") || !systemctlExecutableAllowed("/bin/systemctl") {
		t.Fatal("allowlisted systemctl paths")
	}
	if systemctlExecutableAllowed("/usr/bin/sh") || systemctlExecutableAllowed("systemctl") {
		t.Fatal("non-allowlisted executable must be rejected")
	}
	list := systemdListUnitsArgv()
	if list[0] != "list-units" {
		t.Fatalf("list verb %q", list[0])
	}
	args, err := systemdShowArgv([]string{"nginx.service", "sshd.service"})
	if err != nil {
		t.Fatal(err)
	}
	if args[0] != "show" {
		t.Fatalf("show verb %q", args[0])
	}
	end := -1
	for i, a := range args {
		if a == "--" {
			end = i
			break
		}
	}
	if end < 0 {
		t.Fatalf("show argv missing end-of-options: %v", args)
	}
	if strings.Join(args[end+1:], ",") != "nginx.service,sshd.service" {
		t.Fatalf("units must follow --: %v", args)
	}
	for i, a := range args[:end] {
		if i > 0 && !strings.HasPrefix(a, "-") {
			t.Fatalf("pre-separator argument %q is not an option", a)
		}
	}
	joined := strings.Join(append(append([]string{}, list...), args...), " ")
	for _, bad := range []string{"sh -c", "bash -c", "/bin/sh", "/bin/bash"} {
		if strings.Contains(joined, bad) {
			t.Fatalf("argv contains shell form %q: %s", bad, joined)
		}
	}
	for _, mut := range systemdMutationVerbs {
		if err := assertSystemctlArgvSafe([]string{mut}); err == nil {
			t.Fatalf("mutation verb %q must be rejected", mut)
		}
		for _, a := range append(list, args...) {
			if a == mut {
				t.Fatalf("read-only argv contains mutation %q", mut)
			}
		}
	}
	if _, err := systemdShowArgv([]string{"../../etc/passwd"}); err == nil {
		t.Fatal("path-traversal unit id must be rejected")
	}
	if _, err := systemdShowArgv([]string{"evil/unit.service"}); err == nil {
		t.Fatal("unit id with slash must be rejected")
	}
}

func TestSystemdShowArgvRejectsOptionLookingUnitIDs(t *testing.T) {
	if err := (ProcessServiceSelector{Kind: SelectorKindServiceID, Value: "--version.service"}).Validate(); err != nil {
		t.Fatalf("SERVICE_ID exact validation must still accept literal values: %v", err)
	}
	hostile := []string{
		"--version.service",
		"-H.service",
		"--property=Id",
		"--no-pager.service",
		"-a.service",
		"--help",
	}
	for _, id := range hostile {
		if err := validateSystemdUnitID(id); err == nil {
			t.Fatalf("adapter must reject leading-hyphen unit id %q", id)
		}
		if _, err := systemdShowArgv([]string{id}); err == nil {
			t.Fatalf("systemdShowArgv must reject %q", id)
		}
	}
	if err := assertSystemctlArgvSafe([]string{
		systemdShowVerb, "--no-pager", systemdShowPropertyArg(), "--version.service",
	}); err == nil {
		t.Fatal("show argv without -- and with option-looking unit must be rejected")
	}
	if err := assertSystemctlArgvSafe([]string{
		systemdShowVerb, "--no-pager", "--", "--version.service",
	}); err == nil {
		t.Fatal("unit after -- still must not start with hyphen")
	}
	safe, err := systemdShowArgv([]string{"nginx.service"})
	if err != nil {
		t.Fatal(err)
	}
	if err := assertSystemctlArgvSafe(safe); err != nil {
		t.Fatal(err)
	}
	for i, a := range safe {
		if a == "--" {
			for _, unit := range safe[i+1:] {
				if strings.HasPrefix(unit, "-") {
					t.Fatalf("unit-controlled value %q would alter option parsing", unit)
				}
			}
			break
		}
	}
}

func TestSystemdShowParserFixtures(t *testing.T) {
	raw := strings.Join([]string{
		"Id=nginx.service\nActiveState=active\nSubState=running\nResult=success\nNRestarts=0\nMainPID=4242\nExecMainStatus=0",
		"Id=idle.service\nActiveState=inactive\nSubState=dead\nResult=success\nNRestarts=0\nMainPID=0\nExecMainStatus=0",
		"Id=broken.service\nActiveState=failed\nSubState=failed\nResult=exit-code\nNRestarts=12\nMainPID=0\nExecMainStatus=1",
		"Id=boot.service\nActiveState=activating\nSubState=start\nResult=success\nNRestarts=1\nMainPID=9\nExecMainStatus=0",
		"Id=stop.service\nActiveState=deactivating\nSubState=stop\nResult=success\nNRestarts=0\nMainPID=8\nExecMainStatus=0",
		"Id=reload.service\nActiveState=reloading\nSubState=reload\nResult=success\nNRestarts=0\nMainPID=7\nExecMainStatus=0",
		"Id=odd.service\nActiveState=unknown\nSubState=unknown\nResult=unknown\nMainPID=0",
	}, "\n\n")
	obs, rejected, err := parseSystemdShow(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rejected != 0 {
		t.Fatalf("rejected=%d", rejected)
	}
	if len(obs) != 7 {
		t.Fatalf("got %d observations", len(obs))
	}
	byID := map[string]SystemdServiceObservation{}
	for _, o := range obs {
		byID[o.UnitID] = o
		if o.MainPIDIsDurableSelector() {
			t.Fatalf("MainPID must remain ephemeral for %s", o.UnitID)
		}
		sel, err := o.DurableSelector()
		if err != nil || sel.Kind != SelectorKindServiceID {
			t.Fatalf("durable selector %s: %#v %v", o.UnitID, sel, err)
		}
	}
	if byID["nginx.service"].ActiveState != "active" || byID["nginx.service"].MainPID != 4242 || !byID["nginx.service"].HasMainPID {
		t.Fatalf("active fixture %+v", byID["nginx.service"])
	}
	if byID["idle.service"].ActiveState != "inactive" || byID["idle.service"].HasMainPID {
		t.Fatalf("inactive fixture %+v", byID["idle.service"])
	}
	if byID["broken.service"].ActiveState != "failed" || byID["broken.service"].Result != "exit-code" || byID["broken.service"].NRestarts != 12 || byID["broken.service"].ExecMainStatus != 1 {
		t.Fatalf("failed fixture %+v", byID["broken.service"])
	}
	if byID["boot.service"].ActiveState != "activating" || byID["stop.service"].ActiveState != "deactivating" || byID["reload.service"].ActiveState != "reloading" {
		t.Fatal("transitional states")
	}
	if byID["odd.service"].ActiveState != "unknown" || byID["odd.service"].HasNRestarts {
		t.Fatalf("unknown fixture %+v", byID["odd.service"])
	}
}

func TestSystemdShowParserMixedMalformedCountsRejected(t *testing.T) {
	raw := strings.Join([]string{
		"Id=nginx.service\nActiveState=active\nSubState=running\nResult=success\nNRestarts=0\nMainPID=1\nExecMainStatus=0",
		"this-block-is-not-key-value",
		"Id=oversized.service\nActiveState=" + strings.Repeat("x", maxSystemdFieldLen+8),
		"Id=sshd.service\nActiveState=active\nSubState=running\nResult=success\nNRestarts=0\nMainPID=2\nExecMainStatus=0",
	}, "\n\n")
	obs, rejected, err := parseSystemdShow(raw)
	if err != nil {
		t.Fatal(err)
	}
	if rejected != 2 {
		t.Fatalf("rejected=%d", rejected)
	}
	if len(obs) != 2 || obs[0].UnitID != "nginx.service" || obs[1].UnitID != "sshd.service" {
		t.Fatalf("valid blocks must remain usable: %+v", obs)
	}
	for _, o := range obs {
		if SourceClassAssertsProcessOrHostDown(o.SourceClass) {
			t.Fatal("malformed systemd block must not infer DOWN")
		}
	}
}

func TestSystemdListUnitsParserTruncation(t *testing.T) {
	var b strings.Builder
	for i := 0; i < maxSystemdUnits+3; i++ {
		fmt.Fprintf(&b, "svc-%d.service loaded active running dummy\n", i)
	}
	units, truncated, err := parseSystemdListUnits(b.String())
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(units) != maxSystemdUnits {
		t.Fatalf("truncated=%v len=%d", truncated, len(units))
	}
}

func TestPackageMetadataAndSelectorsAreNotExecutable(t *testing.T) {
	snap := validInventoryBase(t)
	// Control-bearing package metadata is rejected (no silent normalization into shell input).
	snap.Items[0].ProductName = "openssl\n"
	if err := snap.Validate(); err == nil {
		t.Fatal("control characters in package metadata must be rejected")
	}
	// Literal exact selector values that look shell-like remain data and are accepted.
	for _, sel := range []ProcessServiceSelector{
		{Kind: SelectorKindProcessName, Value: "sh -c id"},
		{Kind: SelectorKindExecutablePath, Value: "/bin/sh -c evil"},
		{Kind: SelectorKindProcessName, Value: "nginx;reboot"},
	} {
		if err := sel.Validate(); err != nil {
			t.Fatalf("literal exact selector %#v must be accepted as data: %v", sel, err)
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
