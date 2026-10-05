//go:build linux

package collector

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"platform/agent/common/identity"
	"platform/agent/common/telemetry"
)

func TestParsePSI_SomeAndFull(t *testing.T) {
	in := "some avg10=1.25 avg60=2.50 avg300=3.75 total=1000\nfull avg10=0.10 avg60=0.20 avg300=0.30 total=50\n"
	data, ok := parsePSI(strings.NewReader(in))
	if !ok {
		t.Fatal("expected parse success")
	}
	if !data.some.present || data.some.avg10 != 1.25 || data.some.total != 1000 {
		t.Fatalf("some mismatch: %+v", data.some)
	}
	if !data.full.present || data.full.avg300 != 0.30 || data.full.total != 50 {
		t.Fatalf("full mismatch: %+v", data.full)
	}
	samples := pressureSamples("cpu", data)
	if len(samples) != 8 {
		t.Fatalf("expected 8 pressure samples, got %d", len(samples))
	}
}

func TestParsePSI_MissingFull(t *testing.T) {
	in := "some avg10=0.00 avg60=0.00 avg300=0.00 total=42\n"
	data, ok := parsePSI(strings.NewReader(in))
	if !ok || !data.some.present || data.full.present {
		t.Fatalf("expected some-only parse, got %+v ok=%v", data, ok)
	}
	samples := pressureSamples("memory", data)
	if len(samples) != 4 {
		t.Fatalf("expected 4 some-only samples, got %d", len(samples))
	}
	for _, s := range samples {
		if s.Labels["scope"] != "some" {
			t.Fatalf("unexpected scope label: %+v", s)
		}
	}
}

func TestParsePSI_Malformed(t *testing.T) {
	cases := []string{
		"",
		"garbage\n",
		"some avg10=x avg60=0 avg300=0 total=1\n",
		"some avg10=1 avg60=2\n", // missing fields
	}
	for _, in := range cases {
		if _, ok := parsePSI(strings.NewReader(in)); ok {
			t.Fatalf("expected parse failure for %q", in)
		}
	}
}

func TestPressureUnavailableResourcePreservesOthers(t *testing.T) {
	// Simulate Collect path logic: one unavailable resource emits available=0.
	var samples []telemetry.Sample
	samples = append(samples, pressureSamples("cpu", psiData{
		some: psiScope{avg10: 1, avg60: 1, avg300: 1, total: 10, present: true},
	})...)
	samples = append(samples, pressureAvailableSample("cpu", true))
	samples = append(samples, pressureAvailableSample("memory", false))
	samples = append(samples, pressureSamples("io", psiData{
		some: psiScope{avg10: 2, avg60: 2, avg300: 2, total: 20, present: true},
		full: psiScope{avg10: 1, avg60: 1, avg300: 1, total: 5, present: true},
	})...)
	samples = append(samples, pressureAvailableSample("io", true))

	foundCPU := false
	foundMemAvail0 := false
	foundIO := false
	for _, s := range samples {
		if s.MetricKey == "system.pressure.avg10" && s.Labels["resource"] == "cpu" {
			foundCPU = true
		}
		if s.MetricKey == "system.pressure.available" && s.Labels["resource"] == "memory" && s.Value == 0 {
			foundMemAvail0 = true
		}
		if s.MetricKey == "system.pressure.avg10" && s.Labels["resource"] == "io" {
			foundIO = true
		}
		for _, v := range s.Labels {
			if strings.Contains(strings.ToLower(v), "error") {
				t.Fatalf("free-form error label prohibited: %+v", s.Labels)
			}
		}
	}
	if !foundCPU || !foundMemAvail0 || !foundIO {
		t.Fatalf("partial availability not preserved: cpu=%v mem0=%v io=%v", foundCPU, foundMemAvail0, foundIO)
	}
}

func TestMeminfoSwapArithmetic(t *testing.T) {
	in := "MemTotal: 1000 kB\nMemAvailable: 400 kB\nSwapTotal: 2000 kB\nSwapFree: 500 kB\n"
	info, err := parseMeminfo(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	samples, err := meminfoSamples(info)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]telemetry.Sample{}
	for _, s := range samples {
		byKey[s.MetricKey] = s
	}
	if byKey["system.memory.swap.total"].Value != 2000*1024 {
		t.Fatalf("swap total=%v", byKey["system.memory.swap.total"].Value)
	}
	if byKey["system.memory.swap.available"].Value != 500*1024 {
		t.Fatalf("swap avail=%v", byKey["system.memory.swap.available"].Value)
	}
	if byKey["system.memory.swap.used"].Value != 1500*1024 {
		t.Fatalf("swap used=%v", byKey["system.memory.swap.used"].Value)
	}
	wantUtil := (1500.0 / 2000.0) * 100.0
	if math.Abs(byKey["system.memory.swap.utilization"].Value-wantUtil) > 0.001 {
		t.Fatalf("swap util=%v want %v", byKey["system.memory.swap.utilization"].Value, wantUtil)
	}
}

func TestMeminfoZeroSwap(t *testing.T) {
	in := "MemTotal: 1000 kB\nMemAvailable: 500 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n"
	info, err := parseMeminfo(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	samples, err := meminfoSamples(info)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]float64{}
	for _, s := range samples {
		byKey[s.MetricKey] = s.Value
	}
	if byKey["system.memory.swap.total"] != 0 || byKey["system.memory.swap.used"] != 0 || byKey["system.memory.swap.utilization"] != 0 {
		t.Fatalf("zero-swap mismatch: %+v", byKey)
	}
}

func TestVmstatPresentMissingMalformed(t *testing.T) {
	present := parseVmstatSamples(strings.NewReader("" +
		"nr_free_pages 1\n" +
		"pgfault 100\n" +
		"pgmajfault 10\n" +
		"pswpin 2\n" +
		"pswpout 3\n" +
		"oom_kill 0\n"))
	if len(present) != 5 {
		t.Fatalf("expected 5 vmstat samples, got %d", len(present))
	}
	for _, s := range present {
		if s.MetricKey == "system.memory.oom_kills" && s.Value != 0 {
			t.Fatal("explicit zero oom_kill must be emitted")
		}
	}

	partial := parseVmstatSamples(strings.NewReader("pgfault 9\nignored 1\n"))
	if len(partial) != 1 || partial[0].MetricKey != "system.memory.page.faults" {
		t.Fatalf("missing optional fields must not invent zeros: %+v", partial)
	}

	malformed := parseVmstatSamples(strings.NewReader("pgfault notanumber\npswpin\n"))
	if len(malformed) != 0 {
		t.Fatalf("malformed lines must be skipped, got %+v", malformed)
	}
}

func TestInodeArithmeticAndZeroTotal(t *testing.T) {
	labels := map[string]string{"mount": "/", "fstype": "ext4"}
	samples := inodeSamples(1000, 250, labels)
	if len(samples) != 4 {
		t.Fatalf("got %d", len(samples))
	}
	byKey := map[string]float64{}
	for _, s := range samples {
		byKey[s.MetricKey] = s.Value
		if s.Labels["mount"] != "/" || s.Labels["fstype"] != "ext4" {
			t.Fatalf("labels not preserved: %+v", s.Labels)
		}
	}
	if byKey["system.filesystem.inodes.total"] != 1000 || byKey["system.filesystem.inodes.available"] != 250 {
		t.Fatalf("%+v", byKey)
	}
	if byKey["system.filesystem.inodes.used"] != 750 {
		t.Fatalf("used=%v", byKey["system.filesystem.inodes.used"])
	}
	if math.Abs(byKey["system.filesystem.inodes.utilization"]-75.0) > 0.001 {
		t.Fatalf("util=%v", byKey["system.filesystem.inodes.utilization"])
	}

	if inodeSamples(0, 0, labels) != nil {
		t.Fatal("zero-total inodes must not emit samples")
	}
	// Invalid free > total is clamped.
	clamped := inodeSamples(10, 20, labels)
	if clamped[2].Value != 0 || clamped[1].Value != 10 {
		t.Fatalf("expected clamp, got %+v", clamped)
	}
}

func TestDiskstatsParserValidMalformedOverflowCardinality(t *testing.T) {
	valid := "   8       0 sda 10 0 20 30 40 0 50 60 1 70 80\n" +
		"   8       1 sda1 1 0 2 3 4 0 5 6 0 7 8\n" +
		"   7       0 loop0 1 0 2 3 4 0 5 6 0 7 8\n"
	devs := parseDiskstats(strings.NewReader(valid))
	if len(devs) != 2 {
		t.Fatalf("expected loop excluded, got %+v", deviceNames(devs))
	}
	selected := selectDiskDevices(devs, maxDiskIODevices)
	if selected[0].name != "sda" || selected[1].name != "sda1" {
		t.Fatalf("deterministic order failed: %v", deviceNames(selected))
	}
	samples := diskIOSamples(selected[0])
	byKey := map[string]float64{}
	for _, s := range samples {
		byKey[s.MetricKey] = s.Value
		if s.Labels["device"] != "sda" {
			t.Fatalf("device label=%v", s.Labels)
		}
	}
	if byKey["system.disk.read.operations"] != 10 || byKey["system.disk.write.operations"] != 40 {
		t.Fatalf("%+v", byKey)
	}
	if byKey["system.disk.read.bytes"] != 20*512 || byKey["system.disk.write.bytes"] != 50*512 {
		t.Fatalf("sector bytes mismatch: %+v", byKey)
	}

	malformed := parseDiskstats(strings.NewReader("8 0 sdb bad\n8 0 sdc 1 2\n"))
	if len(malformed) != 0 {
		t.Fatalf("malformed must be skipped: %+v", malformed)
	}

	// Overflow sectors: multiplication guard drops byte metrics only.
	overflowDev := diskstatsDevice{name: "nvme0n1", readSectors: ^uint64(0), writeSectors: 1}
	oSamples := diskIOSamples(overflowDev)
	for _, s := range oSamples {
		if s.MetricKey == "system.disk.read.bytes" {
			t.Fatal("overflow read sectors must not emit read.bytes")
		}
		if s.MetricKey == "system.disk.write.bytes" && s.Value != 512 {
			t.Fatalf("write bytes=%v", s.Value)
		}
	}

	var many []diskstatsDevice
	for i := 0; i < 70; i++ {
		many = append(many, diskstatsDevice{name: "sd" + itoa(i)})
	}
	// Unsorted input; selection must sort then truncate.
	many[0].name = "zzz"
	many[1].name = "aaa"
	trimmed := selectDiskDevices(many, 64)
	if len(trimmed) != 64 {
		t.Fatalf("cardinality bound=%d", len(trimmed))
	}
	if trimmed[0].name != "aaa" {
		t.Fatalf("expected deterministic sort start aaa, got %s", trimmed[0].name)
	}
	for i := 1; i < len(trimmed); i++ {
		if trimmed[i].name < trimmed[i-1].name {
			t.Fatalf("not sorted at %d", i)
		}
	}
}

func TestNetDevParserValidMalformedCardinalityAndLoopback(t *testing.T) {
	in := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo: 100 1 0 0 0 0 0 0 100 1 0 0 0 0 0 0
  eth0: 200 3 1 2 0 0 0 0 300 4 5 6 0 0 0 0
  eth1: 10 1 0 0 0 0 0 0 20 2 0 0 0 0 0 0
`
	ifaces := parseNetDev(strings.NewReader(in))
	if len(ifaces) != 2 {
		t.Fatalf("loopback must be excluded, got %+v", ifaceNames(ifaces))
	}
	selected := selectNetInterfaces(ifaces, maxNetInterfaces)
	if selected[0].name != "eth0" || selected[1].name != "eth1" {
		t.Fatalf("order=%v", ifaceNames(selected))
	}
	samples := netCounterSamples(selected[0])
	byKey := map[string]float64{}
	for _, s := range samples {
		byKey[s.MetricKey] = s.Value
	}
	if byKey["system.network.rx.bytes"] != 200 || byKey["system.network.tx.dropped"] != 6 {
		t.Fatalf("%+v", byKey)
	}

	malformed := parseNetDev(strings.NewReader("ethX: 1 2\nbadline\n"))
	if len(malformed) != 0 {
		t.Fatalf("malformed must skip: %+v", malformed)
	}

	// Overlong interface name excluded.
	longName := strings.Repeat("x", maxInterfaceNameLen+1)
	if isEmittableInterface(longName) || isEmittableInterface("lo") {
		t.Fatal("bounded name / loopback exclusion failed")
	}

	var many []netDevIface
	for i := 0; i < 70; i++ {
		many = append(many, netDevIface{name: "eth" + itoa(i)})
	}
	many[0].name = "zzz0"
	many[1].name = "aaa0"
	trimmed := selectNetInterfaces(many, 64)
	if len(trimmed) != 64 || trimmed[0].name != "aaa0" {
		t.Fatalf("net cardinality/order failed: len=%d first=%s", len(trimmed), trimmed[0].name)
	}
}

func TestOperstateVocabulary(t *testing.T) {
	cases := map[string]string{
		"up":             "up",
		"DOWN":           "down",
		"unknown":        "unknown",
		"dormant":        "dormant",
		"lowerlayerdown": "lowerlayerdown",
		"testing":        "testing",
		"notpresent":     "notpresent",
		"":               "unknown",
		"weird-state":    "other",
	}
	for in, want := range cases {
		if got := normalizeOperstate(in); got != want {
			t.Fatalf("normalizeOperstate(%q)=%q want %q", in, got, want)
		}
	}
	s := netStateSample("eth0", "")
	if s.Value != 1 || s.Unit != "state" || s.Labels["state"] != "unknown" || s.Labels["interface"] != "eth0" {
		t.Fatalf("unreadable operstate sample: %+v", s)
	}
}

func TestOSFactsParser(t *testing.T) {
	in := "cpu 1 2 3\nbtime 1700000000\nprocs_running 4\nprocs_blocked 1\nintr 9\n"
	facts, ok := parseOSFacts(strings.NewReader(in))
	if !ok || facts.btime != 1700000000 || facts.procsRunning != 4 || facts.procsBlocked != 1 {
		t.Fatalf("parse failed: %+v ok=%v", facts, ok)
	}
	samples := osfactsSamples(facts)
	if len(samples) != 3 {
		t.Fatalf("samples=%d", len(samples))
	}

	if _, ok := parseOSFacts(strings.NewReader("btime 1\nprocs_running 2\n")); ok {
		t.Fatal("missing procs_blocked must fail")
	}
	if _, ok := parseOSFacts(strings.NewReader("btime bad\nprocs_running 1\nprocs_blocked 0\n")); ok {
		t.Fatal("malformed btime must fail completeness")
	}
}

func TestDefaultRegistryContainsNewCollectorsOnce(t *testing.T) {
	collectors := NewDefaultCollectors(8)
	want := []string{"cpu", "memory", "filesystem", "pressure", "diskio", "net", "osfacts"}
	if len(collectors) != len(want) {
		t.Fatalf("len=%d want %d", len(collectors), len(want))
	}
	seen := map[string]int{}
	for i, c := range collectors {
		if c.Name() != want[i] {
			t.Fatalf("order[%d]=%s want %s", i, c.Name(), want[i])
		}
		seen[c.Name()]++
	}
	for _, name := range want {
		if seen[name] != 1 {
			t.Fatalf("collector %s count=%d", name, seen[name])
		}
	}

	reg, err := NewDefaultRegistry(8, 15*time.Second, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if reg.Len() != len(want) {
		t.Fatalf("registry len=%d", reg.Len())
	}
}

func TestScalarCollectorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []HostCollector{
		NewPressureCollector(),
		NewDiskIOCollector(),
		NewNetCollector(),
		NewOSFactsCollector(),
		NewMemoryCollector(),
		NewFilesystemCollector(4),
	} {
		if _, err := c.Collect(ctx); err == nil {
			t.Fatalf("%s: expected cancelled context error", c.Name())
		}
	}
}

func TestNormalizeAcceptsNewMetricShapes(t *testing.T) {
	ident := &identity.AgentIdentity{TenantID: "t1", AgentID: "a1", HostID: "h1"}
	now := time.Now()
	samples := []telemetry.Sample{
		{MetricKey: "system.pressure.avg10", Value: 1.5, Unit: "%", Labels: map[string]string{"resource": "cpu", "scope": "some"}},
		{MetricKey: "system.pressure.total", Value: 10, Unit: "us", Labels: map[string]string{"resource": "io", "scope": "full"}},
		{MetricKey: "system.pressure.available", Value: 1, Unit: "bool", Labels: map[string]string{"resource": "memory"}},
		{MetricKey: "system.memory.swap.total", Value: 1024, Unit: "bytes"},
		{MetricKey: "system.memory.swap.utilization", Value: 12.5, Unit: "%"},
		{MetricKey: "system.memory.page.faults", Value: 9, Unit: "count"},
		{MetricKey: "system.memory.oom_kills", Value: 0, Unit: "count"},
		{MetricKey: "system.filesystem.inodes.total", Value: 100, Unit: "count", Labels: map[string]string{"mount": "/", "fstype": "ext4"}},
		{MetricKey: "system.filesystem.inodes.utilization", Value: 50, Unit: "%", Labels: map[string]string{"mount": "/", "fstype": "ext4"}},
		{MetricKey: "system.disk.read.bytes", Value: 512, Unit: "bytes", Labels: map[string]string{"device": "sda"}},
		{MetricKey: "system.disk.io.weighted_time", Value: 3, Unit: "ms", Labels: map[string]string{"device": "sda"}},
		{MetricKey: "system.network.rx.bytes", Value: 1, Unit: "bytes", Labels: map[string]string{"interface": "eth0"}},
		{MetricKey: "system.network.interface.state", Value: 1, Unit: "state", Labels: map[string]string{"interface": "eth0", "state": "up"}},
		{MetricKey: "system.boot.time", Value: 1700000000, Unit: "s"},
		{MetricKey: "system.processes.running", Value: 2, Unit: "count"},
		{MetricKey: "system.processes.blocked", Value: 0, Unit: "count"},
	}
	for _, s := range samples {
		if _, err := telemetry.Normalize(ident, "pressure", now, s); err != nil {
			t.Fatalf("Normalize rejected %+v: %v", s, err)
		}
	}
}

func TestLiveUbuntuSmokeDefaultCollectors(t *testing.T) {
	collectors := NewDefaultCollectors(16)
	ctx := context.Background()
	for _, c := range collectors {
		samples, err := c.Collect(ctx)
		if err != nil {
			// Explicit collection error is allowed; must not panic.
			t.Logf("collector %s returned explicit error: %v", c.Name(), err)
			continue
		}
		for _, s := range samples {
			if s.MetricKey == "" || s.Unit == "" {
				t.Fatalf("%s: empty metric/unit: %+v", c.Name(), s)
			}
			if math.IsNaN(s.Value) || math.IsInf(s.Value, 0) {
				t.Fatalf("%s: non-finite value: %+v", c.Name(), s)
			}
			if len(s.Labels) > 16 {
				t.Fatalf("%s: label cardinality: %+v", c.Name(), s)
			}
		}
		t.Logf("collector %s emitted %d samples", c.Name(), len(samples))
	}
}

func deviceNames(devs []diskstatsDevice) []string {
	out := make([]string, len(devs))
	for i, d := range devs {
		out[i] = d.name
	}
	return out
}

func ifaceNames(ifaces []netDevIface) []string {
	out := make([]string, len(ifaces))
	for i, iface := range ifaces {
		out[i] = iface.name
	}
	return out
}
