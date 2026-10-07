package collector

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"platform/agent/common/telemetry"
)

func TestProcessNetworkValidSyntheticEventRoundTrip(t *testing.T) {
	pid := uint32(42)
	tx := uint64(100)
	ev := ProcessNetworkSyntheticEvent{
		ObservedAt:  time.Date(2026, 10, 6, 12, 0, 1, 0, time.UTC),
		Class:       ProcessNetworkEventBytes,
		ServiceID:   "nginx.service",
		ProcessName: "nginx",
		PID:         &pid,
		Protocol:    ProcessNetworkProtocolTCP,
		RemoteIP:    "192.0.2.10",
		RemotePort:  443,
		BytesTx:     &tx,
	}
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg, err := NewProcessNetworkAggregator(ProcessNetworkAggregatorConfig{SourceProvenance: ProcessNetworkSyntheticSourceProvenance, TestAllowShortWindow: true, Window: time.Minute}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{ev}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 1 {
		t.Fatalf("want 1 evidence, got %d", len(out.Evidence))
	}
	rec := out.Evidence[0]
	if err := rec.Validate(); err != nil {
		t.Fatal(err)
	}
	if rec.RemoteIP.String() != "192.0.2.10" || rec.RemotePort != 443 || rec.Protocol != ProcessNetworkProtocolTCP {
		t.Fatalf("remote mismatch: %+v", rec)
	}
	if rec.IPVersion != "IPv4" {
		t.Fatalf("ip version %s", rec.IPVersion)
	}
	if rec.BytesTx == nil || *rec.BytesTx != 100 {
		t.Fatalf("bytes_tx measured: %+v", rec.BytesTx)
	}
	if rec.PID == nil || *rec.PID != 42 {
		t.Fatal("single PID should be emitted")
	}
	if rec.Quality != ProcessNetworkQualityValid {
		t.Fatalf("quality %s", rec.Quality)
	}
	if rec.SourceProvenance != ProcessNetworkSyntheticSourceProvenance {
		t.Fatalf("provenance %q", rec.SourceProvenance)
	}
	if ProcessNetworkQualityAssertsHostProcessOrNetworkDown(rec.Quality) {
		t.Fatal("quality must not assert DOWN")
	}
}

func TestProcessNetworkInvalidAndMalformedEvents(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	now := time.Date(2026, 10, 6, 12, 0, 1, 0, time.UTC)
	events := []ProcessNetworkSyntheticEvent{
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ServiceID: "nginx.service", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "not-an-ip", RemotePort: 80},
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ServiceID: "nginx.service", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "__overflow_other__", RemotePort: 80},
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ServiceID: "nginx.service", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "other_endpoints", RemotePort: 80},
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ServiceID: "nginx.service", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 0},
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ServiceID: "nginx.service", Protocol: "SCTP", RemoteIP: "192.0.2.1", RemotePort: 80},
		{ObservedAt: now, Class: "BOGUS", ServiceID: "nginx.service", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 80},
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ServiceID: "nginx.service", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 80, LocalIP: "not-an-ip"},
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ServiceID: "nginx.service", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 80, LocalPort: uint16Ptr(0)},
	}
	if err := agg.Ingest(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 0 {
		t.Fatalf("malformed events must not create destination records: %d", len(out.Evidence))
	}
	if !accountingHasDropped(out.Accounting) {
		t.Fatal("malformed events must not be silent; dropped accounting required")
	}
}

func TestProcessNetworkPIDOnlyRejected(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	pid := uint32(9)
	ev := ProcessNetworkSyntheticEvent{
		ObservedAt: time.Date(2026, 10, 6, 12, 0, 1, 0, time.UTC),
		Class:      ProcessNetworkEventOpen,
		PID:        &pid,
		Protocol:   ProcessNetworkProtocolTCP,
		RemoteIP:   "192.0.2.1",
		RemotePort: 80,
	}
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{ev}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 0 {
		t.Fatal("PID-only must not aggregate")
	}
}

func TestProcessNetworkIdentityPrecedence(t *testing.T) {
	e := ProcessNetworkSyntheticEvent{
		ServiceID:          "nginx.service",
		ExecutableIdentity: "sha256:abc",
		ExecutablePath:     "/usr/sbin/nginx",
		ProcessName:        "nginx",
	}
	id, err := durableWorkloadFromEvent(e)
	if err != nil || id.Kind != SelectorKindServiceID {
		t.Fatalf("SERVICE_ID first: %#v %v", id, err)
	}
	e.ServiceID = ""
	id, err = durableWorkloadFromEvent(e)
	if err != nil || id.Kind != SelectorKindExecutableIdentity {
		t.Fatalf("EXECUTABLE_IDENTITY second: %#v %v", id, err)
	}
	e.ExecutableIdentity = ""
	id, err = durableWorkloadFromEvent(e)
	if err != nil || id.Kind != SelectorKindExecutablePath {
		t.Fatalf("EXECUTABLE_PATH third: %#v %v", id, err)
	}
	e.ExecutablePath = ""
	id, err = durableWorkloadFromEvent(e)
	if err != nil || id.Kind != SelectorKindProcessName {
		t.Fatalf("PROCESS_NAME last: %#v %v", id, err)
	}
}

func TestProcessNetworkIPv4MappedCanonicalization(t *testing.T) {
	addr, err := canonicalizeRemoteAddr("::ffff:192.0.2.9")
	if err != nil {
		t.Fatal(err)
	}
	if !addr.Is4() || addr.String() != "192.0.2.9" {
		t.Fatalf("unmap failed: %s is4=%v", addr, addr.Is4())
	}
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	now := time.Date(2026, 10, 6, 12, 0, 1, 0, time.UTC)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "curl", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "::ffff:192.0.2.9", RemotePort: 443},
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "curl", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.9", RemotePort: 443},
	}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 1 {
		t.Fatalf("mapped+v4 must merge, got %d", len(out.Evidence))
	}
	if out.Evidence[0].IPVersion != "IPv4" {
		t.Fatal("canonical record is IPv4")
	}
}

func TestProcessNetworkIPv6OrdinaryEvidence(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	now := time.Date(2026, 10, 6, 12, 0, 1, 0, time.UTC)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "curl", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "2001:db8::1", RemotePort: 443},
	}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 1 || out.Evidence[0].IPVersion != "IPv6" {
		t.Fatalf("ipv6 record: %+v", out.Evidence)
	}
	if err := out.Evidence[0].Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessNetworkOrdinaryEvidenceAlwaysRealIPNoOverflowFakeIP(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withKeys(2, 64))
	now := time.Date(2026, 10, 6, 12, 0, 1, 0, time.UTC)
	var events []ProcessNetworkSyntheticEvent
	for i := 1; i <= 8; i++ {
		events = append(events, ProcessNetworkSyntheticEvent{
			ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "scan",
			Protocol: ProcessNetworkProtocolTCP, RemoteIP: fmt.Sprintf("192.0.2.%d", i), RemotePort: 80,
		})
	}
	if err := agg.Ingest(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) == 0 {
		t.Fatal("expected some real destinations")
	}
	for _, rec := range out.Evidence {
		if !rec.RemoteIP.IsValid() || rec.RemoteIP.IsUnspecified() {
			t.Fatal("ordinary evidence must have real IP")
		}
		if isForbiddenRemoteToken(rec.RemoteIP.String()) {
			t.Fatal("overflow sentinel must not appear as remote IP")
		}
		if err := rec.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if !accountingHasOverflow(out.Accounting) {
		t.Fatal("expected overflow accounting, not fake IP")
	}
}

func TestProcessNetworkDataMinimizationForbidsPayloadLikeFields(t *testing.T) {
	forbidden := []string{
		"payload", "packet", "cmdline", "commandline", "argv", "environ", "environment",
		"tls", "plaintext", "httpheader", "httpbody", "sql", "stacktrace", "stack",
		"bpf", "rawbpf", "profiler", "flame", "secret", "credential", "password",
		"cookie", "authorization", "socketpayload",
	}
	types := []reflect.Type{
		reflect.TypeOf(ProcessNetworkSyntheticEvent{}),
		reflect.TypeOf(ProcessNetworkEvidence{}),
		reflect.TypeOf(ProcessNetworkWindowAccounting{}),
		reflect.TypeOf(ProcessNetworkOverflowSummary{}),
		reflect.TypeOf(ProcessNetworkWorkloadIdentity{}),
	}
	for _, typ := range types {
		walkTypeFields(t, typ, nil, forbidden)
	}
}

func TestProcessNetworkNotHostCollectorOrRegistry(t *testing.T) {
	host := reflect.TypeOf((*HostCollector)(nil)).Elem()
	if reflect.TypeOf(ProcessNetworkEvidence{}).Implements(host) ||
		reflect.PointerTo(reflect.TypeOf(ProcessNetworkEvidence{})).Implements(host) {
		t.Fatal("ProcessNetworkEvidence must not implement HostCollector")
	}
	if reflect.TypeOf(ProcessNetworkAggregator{}).Implements(host) ||
		reflect.PointerTo(reflect.TypeOf(ProcessNetworkAggregator{})).Implements(host) {
		t.Fatal("aggregator must not implement HostCollector")
	}
}

func TestProcessNetworkScalarConversionRejected(t *testing.T) {
	rec := ProcessNetworkEvidence{
		SchemaVersion:    ProcessNetworkSchemaVersion,
		SourceProvenance: ProcessNetworkSyntheticSourceProvenance,
		BucketStartAt:    time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC),
		BucketEndAt:      time.Date(2026, 10, 6, 12, 1, 0, 0, time.UTC),
		BucketSeconds:    60,
		Workload:         ProcessNetworkWorkloadIdentity{Kind: SelectorKindProcessName, Value: "nginx", ProcessName: "nginx"},
		RemoteIP:         netip.MustParseAddr("192.0.2.1"),
		RemotePort:       80,
		Protocol:         ProcessNetworkProtocolTCP,
		IPVersion:        "IPv4",
		Quality:          ProcessNetworkQualityValid,
	}
	samples, err := EncodeProcessNetworkEvidenceAsScalarSamples(rec)
	if samples != nil || err == nil || !strings.Contains(err.Error(), "PROCESS_NETWORK_SCALAR_ENCODING_REJECTED") {
		t.Fatalf("scalar encoding must be rejected: %v %v", samples, err)
	}
	if _, ok := any(rec).(telemetry.Sample); ok {
		t.Fatal("evidence must not convert to Sample")
	}
}

func TestProcessNetworkUnwiredSinkAndNoPhysicalDelivery(t *testing.T) {
	var unwired UnwiredStructuredEvidenceSink
	if unwired.DeliveryState() != StructuredEvidenceTransportNotWired {
		t.Fatal("default unwired")
	}
	if ClaimsPhysicalDelivery(unwired.DeliveryState()) {
		t.Fatal("must not claim delivery")
	}
	rec := ProcessNetworkEvidence{
		SchemaVersion:    ProcessNetworkSchemaVersion,
		SourceProvenance: ProcessNetworkSyntheticSourceProvenance,
		BucketStartAt:    time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC),
		BucketEndAt:      time.Date(2026, 10, 6, 12, 1, 0, 0, time.UTC),
		BucketSeconds:    60,
		Workload:         ProcessNetworkWorkloadIdentity{Kind: SelectorKindProcessName, Value: "nginx", ProcessName: "nginx"},
		RemoteIP:         netip.MustParseAddr("192.0.2.1"),
		RemotePort:       80,
		Protocol:         ProcessNetworkProtocolTCP,
		IPVersion:        "IPv4",
		Quality:          ProcessNetworkQualityValid,
	}
	err := unwired.AcceptProcessNetworkEvidence(context.Background(), rec)
	if !errors.Is(err, ErrStructuredTransportNotWired) {
		t.Fatalf("got %v", err)
	}
	acc := ProcessNetworkWindowAccounting{
		SchemaVersion:    ProcessNetworkSchemaVersion,
		SourceProvenance: ProcessNetworkSyntheticSourceProvenance,
		BucketStartAt:    rec.BucketStartAt,
		BucketEndAt:      rec.BucketEndAt,
		BucketSeconds:    60,
		Quality:          ProcessNetworkQualityPartial,
		PartialFlag:      true,
	}
	err = unwired.AcceptProcessNetworkWindowAccounting(context.Background(), acc)
	if !errors.Is(err, ErrStructuredTransportNotWired) {
		t.Fatalf("got %v", err)
	}
	local := &LocalRecordingStructuredEvidenceSink{}
	if local.DeliveryState() != StructuredEvidenceLocalOnly {
		t.Fatal("local only")
	}
	if ClaimsPhysicalDelivery(local.DeliveryState()) {
		t.Fatal("local must not claim physical delivery")
	}
	if err := local.AcceptProcessNetworkEvidence(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func TestProcessNetworkGoModUnchanged(t *testing.T) {
	modPath := filepath.Join("..", "..", "go.mod")
	b, err := os.ReadFile(modPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "module platform/agent") {
		t.Fatal("module path")
	}
	if strings.Contains(text, "cilium") || strings.Contains(text, "ebpf") || strings.Contains(strings.ToLower(text), "require ") {
		t.Fatalf("Slice 5 must not add Go dependencies:\n%s", text)
	}
}

func walkTypeFields(t *testing.T, typ reflect.Type, path []string, forbidden []string) {
	t.Helper()
	if typ == nil {
		return
	}
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return
	}
	if typ.PkgPath() != "" && !strings.Contains(typ.PkgPath(), "platform/agent") {
		return
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		name := strings.ToLower(f.Name)
		joined := strings.Join(append(path, f.Name), ".")
		for _, bad := range forbidden {
			if name == bad || strings.Contains(name, bad) {
				t.Fatalf("forbidden payload-like field %s", joined)
			}
		}
		tag := strings.ToLower(f.Tag.Get("json"))
		for _, bad := range forbidden {
			if strings.Contains(tag, bad) {
				t.Fatalf("forbidden json tag on %s: %s", joined, f.Tag)
			}
		}
		if f.Type.Kind() == reflect.Struct || f.Type.Kind() == reflect.Pointer || f.Type.Kind() == reflect.Slice {
			walkTypeFields(t, f.Type, append(path, f.Name), forbidden)
		}
	}
}

func accountingHasDropped(acc []ProcessNetworkWindowAccounting) bool {
	for _, a := range acc {
		if a.DroppedRecordsCount > 0 {
			return true
		}
	}
	return false
}

func accountingHasOverflow(acc []ProcessNetworkWindowAccounting) bool {
	for _, a := range acc {
		if a.EvictedCandidateCount > 0 || a.OverflowedRecordsCount > 0 || len(a.Overflows) > 0 {
			return true
		}
	}
	return false
}
