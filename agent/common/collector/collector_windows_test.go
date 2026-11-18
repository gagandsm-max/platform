//go:build windows

package collector

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsProviderFunc func(context.Context, string, int) (windowsSnapshot, error)

func (f windowsProviderFunc) read(ctx context.Context, source string, limit int) (windowsSnapshot, error) {
	return f(ctx, source, limit)
}

func TestWindowsDefaultRegistryUsesRealBoundedCollector(t *testing.T) {
	collectors := NewDefaultCollectors(0)
	if len(collectors) != 5 {
		t.Fatalf("default collector count=%d want 5", len(collectors))
	}
	want := []string{"windows_cpu", "windows_memory", "windows_volumes", "windows_network", "windows_os"}
	for i, name := range want {
		if collectors[i].Name() != name {
			t.Fatalf("collector[%d]=%q want %q", i, collectors[i].Name(), name)
		}
	}
	if len(collectors) > MaxRegisteredCollectors {
		t.Fatalf("collector count=%d exceeds registry bound %d", len(collectors), MaxRegisteredCollectors)
	}
	seen := map[string]bool{}
	for _, c := range collectors {
		if seen[c.Name()] {
			t.Fatalf("duplicate collector name %q", c.Name())
		}
		seen[c.Name()] = true
	}
	if got := NewDefaultCollectors(1000); len(got) != len(collectors) {
		t.Fatalf("unbounded maxFilesystems changed collector count: %d", len(got))
	}
}

func TestMIBIfRow2LayoutMatchesWindowsABI(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("ABI assertion currently targets Windows amd64")
	}
	if got := unsafe.Sizeof(mibIfRow2{}); got != 1352 {
		t.Fatalf("MIB_IF_ROW2 size=%d want 1352", got)
	}
	if got := unsafe.Offsetof(mibIfRow2{}.inOctets); got != 1208 {
		t.Fatalf("MIB_IF_ROW2 InOctets offset=%d want 1208", got)
	}
}

func TestWindowsCollectorEmitsSupportedHostScalars(t *testing.T) {
	up := float64(42)
	call := 0
	c := newWindowsHostCollector("windows_test", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		call++
		cpu := cpuTimes{idle: 80, total: 100}
		if call > 1 {
			cpu = cpuTimes{idle: 160, total: 200}
		}
		return windowsSnapshot{
			uptimeSeconds: &up, logicalProcessors: 8, cpu: &cpu,
			memory:     &memoryFacts{total: 1000, available: 250, commitLimit: 2000, commitAvailable: 500},
			volumes:    []volumeFacts{{mount: "C:\\", total: 1000, available: 250}},
			interfaces: []interfaceFacts{{id: "guid-ethernet", name: "Ethernet", rxBytes: 12, txBytes: 34}},
		}, nil
	}), 4)
	first, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 {
		t.Fatal("first collection emitted no evidence")
	}
	second, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, s := range second {
		got[s.MetricKey] = s.Value
	}
	for k, want := range map[string]float64{"system.uptime": 42, "system.cpu.count": 8, "system.cpu.utilization": 20, "system.memory.total": 1000, "system.memory.used": 750, "system.windows.memory.commit_limit": 2000, "system.filesystem.total": 1000, "system.network.rx.bytes": 12} {
		if got[k] != want {
			t.Errorf("%s=%v want %v", k, got[k], want)
		}
	}
	if _, ok := got["system.filesystem.inodes.total"]; ok {
		t.Fatal("Windows inode count must not be represented as Linux inode evidence")
	}
}

func TestWindowsCollectorCounterResetAndWrapDoNotCreateFalseUtilization(t *testing.T) {
	max := ^uint64(0)
	values := []cpuTimes{{idle: 10, total: 20}, {idle: 15, total: 30}, {idle: 2, total: 4}, {idle: 3, total: 5}, {idle: max - 4, total: max - 2}, {idle: 2, total: 3}, {idle: 3, total: 4}}
	i := 0
	c := newWindowsHostCollector("windows_cpu", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		v := values[i]
		i++
		return windowsSnapshot{logicalProcessors: 4, cpu: &v}, nil
	}), 1)
	for n, want := range []bool{false, true, false, true, true, false, true} {
		samples, err := c.Collect(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, s := range samples {
			if s.MetricKey == "system.cpu.utilization" {
				found = true
				if math.IsNaN(s.Value) || math.IsInf(s.Value, 0) {
					t.Fatalf("invalid CPU value %v", s.Value)
				}
			}
		}
		if found != want {
			t.Fatalf("sample %d utilization=%v want %v", n, found, want)
		}
	}
}

func TestWindowsCollectorBoundsAndDeduplicatesSources(t *testing.T) {
	up := float64(1)
	c := newWindowsHostCollector("windows_test", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		return windowsSnapshot{uptimeSeconds: &up,
			volumes:    []volumeFacts{{mount: "c:\\", total: 100, available: 20}, {mount: "C:\\", total: 200, available: 10}, {mount: "D:\\", total: 50, available: 5}},
			interfaces: []interfaceFacts{{id: "guid-a", name: "NIC", rxBytes: 1}, {id: "guid-b", name: "nic", rxBytes: 2}, {id: "guid-c", name: "Other", rxBytes: 3}},
		}, nil
	}), 1)
	samples, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	vols, nics := map[string]bool{}, map[string]bool{}
	for _, s := range samples {
		if s.MetricKey == "system.filesystem.total" {
			vols[s.Labels["mount"]] = true
		}
		if s.MetricKey == "system.network.rx.bytes" {
			nics[s.Labels["interface"]] = true
		}
	}
	if len(vols) != 1 || len(nics) != 3 {
		t.Fatalf("volume/interface bounds failed: %v %v", vols, nics)
	}
	if len(dedupeVolumes([]volumeFacts{{mount: "D:\\"}, {mount: "d:\\"}}, 8)) != 1 {
		t.Fatal("duplicate Windows volumes survived normalization")
	}
}

// This unit assertion covers the local Collect return only. The generic
// scheduler drops these samples on error; end-to-end partial-source recovery
// remains a scoped blocker requiring an existing contract or scheduler scope.
func TestWindowsCollectorReturnsPartialSnapshotWithProviderError(t *testing.T) {
	resetWindowsBackoff()
	defer resetWindowsBackoff()
	up := float64(5)
	providerErr := errors.New("WINDOWS_VOLUME_UNAVAILABLE: C: access denied")
	c := newWindowsHostCollector("windows_test", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		return windowsSnapshot{uptimeSeconds: &up}, providerErr
	}), 2)
	samples, err := c.Collect(context.Background())
	if !errors.Is(err, providerErr) {
		t.Fatalf("error=%v want provider error", err)
	}
	if len(samples) != 1 || samples[0].MetricKey != "system.uptime" {
		t.Fatalf("partial evidence=%+v", samples)
	}
	if samples[0].Value == 0 {
		t.Fatal("missing source was converted to a false zero")
	}
	for _, sample := range samples {
		if strings.HasPrefix(sample.MetricKey, "system.filesystem.") {
			t.Fatalf("inaccessible volume emitted false data: %+v", sample)
		}
	}
}

func TestWindowsCollectorMissingNICDoesNotEmitFalseZero(t *testing.T) {
	up := float64(5)
	c := newWindowsHostCollector("windows_test", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		return windowsSnapshot{uptimeSeconds: &up}, nil
	}), 2)
	samples, err := c.Collect(context.Background())
	if err != nil && !strings.Contains(err.Error(), "NO_EVIDENCE") {
		t.Fatal(err)
	}
	for _, sample := range samples {
		if strings.HasPrefix(sample.MetricKey, "system.network.") {
			t.Fatalf("missing NIC emitted %s=%v", sample.MetricKey, sample.Value)
		}
	}
}

func TestWindowsCollectorFailuresAreIsolatedBySource(t *testing.T) {
	provider := windowsProviderFunc(func(_ context.Context, source string, _ int) (windowsSnapshot, error) {
		switch source {
		case "windows_volumes":
			return windowsSnapshot{volumes: []volumeFacts{{mount: "C:\\", total: 100, available: 25}}}, errors.New("volume denied")
		case "windows_memory":
			return windowsSnapshot{memory: &memoryFacts{total: 100, available: 40}}, nil
		case "windows_network":
			return windowsSnapshot{interfaces: []interfaceFacts{{id: "guid-a", name: "Ethernet", rxBytes: 9}}}, nil
		default:
			return windowsSnapshot{}, nil
		}
	})
	volumes := newWindowsHostCollector("windows_volumes", provider, 8)
	if samples, err := volumes.Collect(context.Background()); err == nil || len(samples) == 0 {
		t.Fatalf("volume error evidence not surfaced: samples=%v err=%v", samples, err)
	}
	for _, source := range []string{"windows_memory", "windows_network"} {
		samples, err := newWindowsHostCollector(source, provider, 8).Collect(context.Background())
		if err != nil || len(samples) == 0 {
			t.Fatalf("source %s suppressed by volume error: samples=%v err=%v", source, samples, err)
		}
	}
}

func TestWindowsNICIdentitySurvivesAliasCollisionTruncationAndRename(t *testing.T) {
	long := strings.Repeat("x", 140)
	rows := []interfaceFacts{{id: "luid-1", name: long, rxBytes: 1}, {id: "luid-2", name: long + "a", rxBytes: 2}, {id: "luid-1", name: "Renamed", rxBytes: 3}}
	got := boundedInterfaces(rows)
	if len(got) != 2 {
		t.Fatalf("NIC identity collisions produced %d rows: %+v", len(got), got)
	}
	if got[0].id == got[1].id || got[0].name == got[1].name {
		t.Fatalf("distinct NICs conflated: %+v", got)
	}
	if got[0].name != "Renamed" {
		t.Fatalf("same stable interface identity did not replace renamed alias: %+v", got)
	}
}

func TestWindowsZeroGUIDInterfacesUseDistinctLUIDIdentity(t *testing.T) {
	zero := windows.GUID{}
	rows := []interfaceFacts{
		{id: windowsInterfaceID(zero, 0x101), name: "Ethernet One", rxBytes: 11},
		{id: windowsInterfaceID(zero, 0x202), name: "Ethernet Two", rxBytes: 22},
	}
	got := boundedInterfaces(rows)
	if len(got) != 2 || got[0].id == got[1].id {
		t.Fatalf("zero-GUID interfaces collapsed: %+v", got)
	}
	if got[0].id != "luid-0000000000000101" || got[1].id != "luid-0000000000000202" {
		t.Fatalf("unexpected zero-GUID fallback identity: %+v", got)
	}
}

func TestWindowsVolumeMountPathParsesAllMultiSZItems(t *testing.T) {
	encode := func(items ...string) []uint16 {
		var out []uint16
		for _, item := range items {
			out = append(out, utf16.Encode([]rune(item))...)
			out = append(out, 0)
		}
		return append(out, 0)
	}
	got, err := parseVolumeMountPaths(encode("C:\\", "C:\\mount\\alpha\\", "C:\\mount\\beta\\"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "C:\\" || got[1] != "C:\\mount\\alpha\\" || got[2] != "C:\\mount\\beta\\" {
		t.Fatalf("MULTI_SZ paths=%q", got)
	}
	empty, err := parseVolumeMountPaths([]uint16{0, 0})
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty MULTI_SZ paths=%q err=%v", empty, err)
	}
}

func TestWindowsProviderPanicIsContainedAndReleasesAdmission(t *testing.T) {
	resetWindowsBackoff()
	defer resetWindowsBackoff()
	calls := 0
	c := newWindowsHostCollector("windows_test", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		calls++
		if calls == 1 {
			panic("deterministic provider panic")
		}
		return windowsSnapshot{uptimeSeconds: ptrFloat64(7)}, nil
	}), 1)
	if _, err := c.Collect(context.Background()); err == nil || !strings.Contains(err.Error(), "WINDOWS_SOURCE_PANIC") {
		t.Fatalf("provider panic was not converted to a source error: %v", err)
	}
	if got := len(windowsSourceGates["windows_test"]); got != 0 {
		t.Fatalf("source gate leaked after panic: len=%d", got)
	}
	if got := len(windowsOrphanBudget); got != 0 {
		t.Fatalf("global orphan budget leaked after panic: len=%d", got)
	}
	resetWindowsBackoff()
	samples, err := c.Collect(context.Background())
	if err != nil || len(samples) != 1 || samples[0].Value != 7 {
		t.Fatalf("source did not recover after contained panic: samples=%v err=%v", samples, err)
	}
}

func TestMissingWindowsVolumeProcedureIsTypedAndDoesNotLeakAdmission(t *testing.T) {
	original := findVolumeProcedure
	t.Cleanup(func() { findVolumeProcedure = original })
	findVolumeProcedure = func(p *windows.LazyProc) error {
		if p == procGetDiskFreeSpaceEx {
			return errors.New("procedure not found")
		}
		return nil
	}
	_, err := readFixedVolumes(context.Background(), 1)
	var sourceErr *volumeSourceError
	if !errors.As(err, &sourceErr) || sourceErr.procedure != procGetDiskFreeSpaceEx.Name || !strings.Contains(err.Error(), "WINDOWS_VOLUME_API_UNAVAILABLE") {
		t.Fatalf("missing procedure did not produce typed volume-source error: %T %v", err, err)
	}
	if got := len(volumeCallGate); got != 0 {
		t.Fatalf("missing procedure leaked volume gate: len=%d", got)
	}
	// Reaching this assertion proves the unavailable API did not terminate the
	// test process. Also prove another source collection remains usable.
	restore := findVolumeProcedure
	findVolumeProcedure = original
	t.Cleanup(func() { findVolumeProcedure = restore })
	c := newWindowsHostCollector("windows_test", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		return windowsSnapshot{uptimeSeconds: ptrFloat64(9)}, nil
	}), 1)
	samples, collectErr := c.Collect(context.Background())
	if collectErr != nil || len(samples) != 1 || samples[0].Value != 9 {
		t.Fatalf("subsequent collection failed after volume API error: samples=%v err=%v", samples, collectErr)
	}
}

func TestWindowsImmediateRepeatedCollectionDoesNotReportStaleBusy(t *testing.T) {
	resetWindowsBackoff()
	defer resetWindowsBackoff()
	c := newWindowsHostCollector("windows_test", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		return windowsSnapshot{uptimeSeconds: ptrFloat64(1)}, nil
	}), 1)
	for i := 0; i < 100; i++ {
		if samples, err := c.Collect(context.Background()); err != nil || len(samples) != 1 {
			t.Fatalf("collection %d failed after immediate prior completion: samples=%v err=%v", i, samples, err)
		}
	}
}

func ptrFloat64(v float64) *float64 { return &v }

func TestWindowsZeroSizeVolumeHasNoUtilization(t *testing.T) {
	c := newWindowsHostCollector("windows_test", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		return windowsSnapshot{volumes: []volumeFacts{{mount: "empty", total: 0, available: 0}}}, nil
	}), 1)
	samples, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range samples {
		if sample.MetricKey == "system.filesystem.utilization" {
			t.Fatalf("unmeasurable utilization emitted: %+v", sample)
		}
	}
}

func TestWindowsSourceTimeoutIsBoundedAndDoesNotBlockOtherSources(t *testing.T) {
	resetWindowsBackoff()
	defer resetWindowsBackoff()
	old := windowsSourceTimeout
	windowsSourceTimeout = 20 * time.Millisecond
	defer func() { windowsSourceTimeout = old }()
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	releaseProvider := func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}
	t.Cleanup(releaseProvider)
	blocked := newWindowsHostCollector("windows_os", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		close(started)
		<-release // deliberately ignores context like a stuck native API
		close(finished)
		return windowsSnapshot{}, nil
	}), 1)
	startedResult := make(chan error, 1)
	go func() { _, err := blocked.Collect(context.Background()); startedResult <- err }()
	waitWindowsTestSignal(t, started, "blocked provider start")
	if err := waitWindowsTestError(t, startedResult, "blocked source timeout"); err == nil || !strings.Contains(err.Error(), "WINDOWS_SOURCE_TIMEOUT") {
		t.Fatalf("blocked source error=%v", err)
	}
	fast := newWindowsHostCollector("windows_memory", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		return windowsSnapshot{memory: &memoryFacts{total: 100, available: 50}}, nil
	}), 1)
	samples, err := fast.Collect(context.Background())
	if err != nil || len(samples) == 0 {
		t.Fatalf("independent memory source stalled: samples=%v err=%v", samples, err)
	}
	releaseProvider()
	waitWindowsTestSignal(t, finished, "blocked provider completion")
	waitWindowsTestAdmissionDrain(t, "windows_os")
}

func TestWindowsGlobalOrphanBudgetBoundsHungProviders(t *testing.T) {
	resetWindowsBackoff()
	defer resetWindowsBackoff()
	old := windowsSourceTimeout
	windowsSourceTimeout = 15 * time.Millisecond
	defer func() { windowsSourceTimeout = old }()
	var releases []chan struct{}
	var started []chan struct{}
	var finished []chan struct{}
	var results []chan error
	released := make([]bool, 0, windowsMaxOrphanedSources)
	releaseAll := func() {
		for i, release := range releases {
			if !released[i] {
				close(release)
				released[i] = true
			}
		}
	}
	t.Cleanup(releaseAll)
	for _, source := range []string{"windows_os", "windows_cpu", "windows_network", "windows_volumes", "windows_memory"} {
		release, ready, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
		releases = append(releases, release)
		released = append(released, false)
		started = append(started, ready)
		finished = append(finished, done)
		result := make(chan error, 1)
		results = append(results, result)
		c := newWindowsHostCollector(source, windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
			close(ready)
			<-release
			close(done)
			return windowsSnapshot{}, nil
		}), 1)
		go func() { _, err := c.Collect(context.Background()); result <- err }()
	}
	for _, ready := range started {
		waitWindowsTestSignal(t, ready, "hung provider start")
	}
	for _, result := range results {
		if err := waitWindowsTestError(t, result, "hung source timeout"); err == nil || !strings.Contains(err.Error(), "WINDOWS_SOURCE_TIMEOUT") {
			t.Fatalf("hung source did not time out: %v", err)
		}
	}
	blocked := newWindowsHostCollector("windows_test", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		return windowsSnapshot{memory: &memoryFacts{total: 100, available: 50}}, nil
	}), 1)
	if _, err := blocked.Collect(context.Background()); err == nil || !strings.Contains(err.Error(), "WINDOWS_GLOBAL_HUNG_CALL_BUDGET_EXHAUSTED") {
		t.Fatalf("global orphan budget did not fail closed: %v", err)
	}
	releaseAll()
	for _, done := range finished {
		waitWindowsTestSignal(t, done, "hung provider completion")
	}
	waitWindowsTestAdmissionDrain(t, "windows_os", "windows_cpu", "windows_network", "windows_volumes", "windows_memory")
}

func waitWindowsTestAdmissionDrain(t *testing.T, sources ...string) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		allReleased := len(windowsOrphanBudget) == 0
		for _, source := range sources {
			if len(windowsSourceGates[source]) != 0 {
				allReleased = false
			}
		}
		if allReleased {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("Windows admission did not drain: sources=%v budget=%d", sources, len(windowsOrphanBudget))
		}
	}
}

func waitWindowsTestSignal(t *testing.T, signal <-chan struct{}, what string) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", what)
	}
}

func waitWindowsTestError(t *testing.T, result <-chan error, what string) error {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", what)
		return nil
	}
}

func resetWindowsBackoff() {
	windowsBackoffMu.Lock()
	windowsBackoff = map[string]struct {
		until    time.Time
		failures uint8
	}{}
	windowsBackoffMu.Unlock()
}

func TestWindowsVolumeLimitReportsIncompleteCoverage(t *testing.T) {
	c := newWindowsHostCollector("windows_test", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		return windowsSnapshot{volumeTruncated: true, volumes: []volumeFacts{{mount: "C:\\", total: 100, available: 40}}}, nil
	}), 1)
	samples, err := c.Collect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "WINDOWS_VOLUME_COVERAGE_TRUNCATED") {
		t.Fatalf("coverage cap hidden: samples=%v err=%v", samples, err)
	}
}

func TestWindowsCollectorHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	c := newWindowsHostCollector("windows_os", windowsProviderFunc(func(ctx context.Context, _ string, _ int) (windowsSnapshot, error) {
		close(started)
		<-ctx.Done()
		return windowsSnapshot{}, ctx.Err()
	}), 1)
	finished := make(chan error, 1)
	go func() { _, err := c.Collect(ctx); finished <- err }()
	<-started
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v want context canceled", err)
	}
	ctx2, cancel2 := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel2()
	if _, err := c.Collect(ctx2); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v want deadline", err)
	}
}

func TestWindowsCollectorDoesNotClaimWholeHostCPUAbove64Processors(t *testing.T) {
	up := float64(10)
	cpu := cpuTimes{idle: 20, total: 50}
	c := newWindowsHostCollector("windows_cpu", windowsProviderFunc(func(context.Context, string, int) (windowsSnapshot, error) {
		return windowsSnapshot{uptimeSeconds: &up, logicalProcessors: 128, cpu: &cpu}, nil
	}), 2)
	samples, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range samples {
		if s.MetricKey == "system.cpu.utilization" {
			t.Fatal("group-local API was presented as whole-host CPU")
		}
	}
}
