package collector

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"platform/agent/common/telemetry"
)

func TestSoftwareInventoryCollectorIsNotHostCollector(t *testing.T) {
	if inventoryCollectorImplementsHostCollector() {
		t.Fatal("LinuxSoftwareInventoryCollector must not implement HostCollector")
	}
	c := NewLinuxSoftwareInventoryCollector()
	host := reflect.TypeOf((*HostCollector)(nil)).Elem()
	typ := reflect.TypeOf(c)
	if typ.Implements(host) || reflect.TypeOf(*c).Implements(host) {
		t.Fatal("inventory collector type implements HostCollector")
	}
	if _, ok := any(c).(HostCollector); ok {
		t.Fatal("inventory collector value implements HostCollector")
	}
}

func TestSoftwareInventoryNotInDefaultRegistry(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("default registry is linux-host collectors")
	}
	reg, err := NewDefaultRegistry(8, 15*time.Second, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range reg.Entries() {
		lower := strings.ToLower(e.Name)
		if strings.Contains(lower, "software") || strings.Contains(lower, "inventory") || strings.Contains(lower, "dpkg") || strings.Contains(lower, "rpm") {
			t.Fatalf("software inventory leaked into default registry as %q", e.Name)
		}
		if fmt.Sprintf("%T", e.Collector) == "*collector.LinuxSoftwareInventoryCollector" {
			t.Fatal("LinuxSoftwareInventoryCollector registered in default Registry")
		}
	}
	for _, hc := range NewDefaultCollectors(8) {
		if strings.Contains(strings.ToLower(hc.Name()), "software") {
			t.Fatalf("default collectors include %q", hc.Name())
		}
	}
}

func TestSoftwareInventoryCannotBecomeScalarSampleLabels(t *testing.T) {
	snap := validInventoryBase(t)
	if _, ok := inventorySnapshotAsSample(snap); ok {
		t.Fatal("inventory snapshot must not convert to telemetry.Sample")
	}
	if _, ok := any(snap).(telemetry.Sample); ok {
		t.Fatal("snapshot converted to sample")
	}
	typ := reflect.TypeOf(&LinuxSoftwareInventoryCollector{})
	for i := 0; i < typ.NumMethod(); i++ {
		name := typ.Method(i).Name
		if name == "Collect" {
			t.Fatal("inventory collector must not expose HostCollector Collect")
		}
	}
	if _, err := telemetry.Normalize(nil, "software-inventory", snap.StartedAt, telemetry.Sample{
		MetricKey: "software.package",
		Value:     1,
		Unit:      "count",
		Labels:    map[string]string{"package": snap.Items[0].ProductName},
	}); err == nil {
		t.Fatal("expected identity failure; software inventory must not ride scalar Normalize")
	}
}

func TestSoftwareInventorySinkRemainsUnwiredLocal(t *testing.T) {
	c := NewLinuxSoftwareInventoryCollector()
	if c.sink.DeliveryState() != StructuredEvidenceTransportNotWired {
		t.Fatalf("default sink delivery=%s", c.sink.DeliveryState())
	}
	if ClaimsPhysicalDelivery(c.sink.DeliveryState()) {
		t.Fatal("default sink must not claim physical delivery")
	}

	local := &LocalRecordingStructuredEvidenceSink{}
	c = collectorWithDPKGFile(t, "dpkg_status_complete")
	c.sink = local
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-local-sink"))
	if err != nil {
		t.Fatal(err)
	}
	if local.DeliveryState() != StructuredEvidenceLocalOnly || ClaimsPhysicalDelivery(local.DeliveryState()) {
		t.Fatal("local sink must remain LOCAL_ONLY")
	}
	if len(local.Inventories) != 1 || local.Inventories[0].SnapshotID != snap.SnapshotID {
		t.Fatal("local sink should record validated inventory only")
	}
}

func TestSoftwareInventoryRemovalInferenceRemainsFalse(t *testing.T) {
	c := collectorWithDPKGFile(t, "dpkg_status_complete")
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-bound-removal"))
	if err != nil {
		t.Fatal(err)
	}
	if snap.RemovalInferencePermitted() {
		t.Fatal("RemovalInferencePermitted must remain false")
	}
}

func TestSoftwareInventorySlice4SourceDoesNotClaimDelivery(t *testing.T) {
	for _, name := range []string{
		"software_inventory_collect.go",
		"software_inventory_dpkg.go",
		"software_inventory_rpm.go",
	} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		text := string(src)
		for _, tok := range []string{"SOFTWARE_INSTALLED", "SOFTWARE_REMOVED", "SOFTWARE_VERSION_CHANGED"} {
			if strings.Contains(text, tok) {
				t.Fatalf("%s contains forbidden change token %q", name, tok)
			}
		}
	}
}

func TestSoftwareInventoryGoModUnchangedShape(t *testing.T) {
	mod, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(mod)
	if !strings.HasPrefix(body, "module platform/agent\n") {
		t.Fatalf("unexpected module: %q", body)
	}
	if strings.Contains(body, "require (") || strings.Contains(body, "\nrequire ") {
		t.Fatal("Slice 4 must not add module requirements")
	}
}

func TestSoftwareInventorySlice4FilesDoNotUseShellOrPathExec(t *testing.T) {
	fset := token.NewFileSet()
	src, err := os.ReadFile("software_inventory_dpkg.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "exec.Command") {
		t.Fatal("dpkg collector must not execute commands")
	}
	rpmSrc, err := os.ReadFile("software_inventory_rpm.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rpmSrc), "exec.Command(") && !strings.Contains(string(rpmSrc), "exec.CommandContext") {
		t.Fatal("rpm collector must use CommandContext only")
	}
	if strings.Count(string(rpmSrc), "exec.CommandContext") != 1 {
		t.Fatalf("expected a single CommandContext call, got source scan mismatch")
	}
	for _, name := range []string{
		"software_inventory_collect.go",
		"software_inventory_dpkg.go",
		"software_inventory_rpm.go",
	} {
		fileSrc, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.ParseFile(fset, name, fileSrc, parser.SkipObjectResolution); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSoftwareInventorySnapshotIDCallerAuthority(t *testing.T) {
	c := collectorWithDPKGFile(t, "dpkg_status_complete")
	a, err := c.Capture(context.Background(), fullDPKGRequest("snap-stable-001"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Capture(context.Background(), fullDPKGRequest("snap-stable-001"))
	if err != nil {
		t.Fatal(err)
	}
	if a.SnapshotID != "snap-stable-001" || b.SnapshotID != "snap-stable-001" {
		t.Fatal("caller-supplied snapshot_id must be retained")
	}
	c.newID = func() (string, error) { return "local-capture-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil }
	generated, err := c.Capture(context.Background(), SoftwareInventoryRequest{SnapshotMode: SnapshotModeFull, SourceType: InventorySourceTypeDPKG})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(generated.SnapshotID, localCaptureIDPrefix) {
		t.Fatalf("generated id must be explicitly local-capture, got %q", generated.SnapshotID)
	}
}

func TestSoftwareInventoryDefaultSinkUnwiredOnCapture(t *testing.T) {
	c := collectorWithDPKGFile(t, "dpkg_status_complete")
	c.sink = UnwiredStructuredEvidenceSink{}
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-unwired"))
	if err != nil {
		t.Fatalf("unwired sink must not fail capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusComplete {
		t.Fatalf("status=%s", snap.CaptureStatus)
	}
}
