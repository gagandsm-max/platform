//go:build linux

package collector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"platform/agent/common/telemetry"
)

func fakeProcStat(pid int, comm, state string, utime, stime, start, rss uint64) string {
	return fmt.Sprintf(
		"%d (%s) %s 1 1 1 0 -1 0 0 0 0 0 %d %d 0 0 20 0 1 0 %d 123456 %d 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0",
		pid, comm, state, utime, stime, start, rss,
	)
}

func writeFakePID(t *testing.T, root string, pid int, comm, state, exeTarget, cgroup, io, statusExtra string, utime, stime, start, rss uint64) {
	t.Helper()
	dir := filepath.Join(root, strconvPID(pid))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(fakeProcStat(pid, comm, state, utime, stime, start, rss)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "comm"), []byte(comm+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status := fmt.Sprintf("Name:\t%s\nState:\t%s (running)\nVmRSS:\t%d kB\n%s", comm, state, rss, statusExtra)
	if err := os.WriteFile(filepath.Join(dir, "status"), []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
	if cgroup != "" {
		if err := os.WriteFile(filepath.Join(dir, "cgroup"), []byte(cgroup), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if io != "" {
		if err := os.WriteFile(filepath.Join(dir, "io"), []byte(io), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if exeTarget != "" {
		if err := os.Symlink(exeTarget, filepath.Join(dir, "exe")); err != nil {
			t.Fatal(err)
		}
	}
}

func strconvPID(pid int) string { return fmt.Sprintf("%d", pid) }

func newTestLinuxSource(t *testing.T, procRoot string) *LinuxProcessServiceSource {
	t.Helper()
	src := NewLinuxProcessServiceSource()
	src.procRoot = procRoot
	src.now = func() time.Time { return time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC) }
	src.systemctlPath = ""
	src.maxEntries = 8
	src.maxDirents = 64
	src.pageSize = 4096
	return src
}

func TestParseProcStatCPUStartRSS(t *testing.T) {
	raw := fakeProcStat(42, "nginx", "S", 10, 20, 9999, 100)
	got, ok := parseProcStat(42, raw)
	if !ok {
		t.Fatal("parse failed")
	}
	if got.comm != "nginx" || got.state != "S" || !got.hasCPU || got.utime != 10 || got.stime != 20 {
		t.Fatalf("%+v", got)
	}
	if !got.hasStart || got.startTime != 9999 || !got.hasRSS || got.rssPages != 100 {
		t.Fatalf("%+v", got)
	}
	if _, ok := parseProcStat(42, "garbage"); ok {
		t.Fatal("malformed stat must fail")
	}
	if _, ok := parseProcStat(7, raw); ok {
		t.Fatal("pid mismatch must fail")
	}
	if _, ok := parseProcStat(42, "42 (nginx) S 1"); ok {
		t.Fatal("short stat must fail")
	}
	if _, ok := parseProcStat(42, "42 (ng"+string(rune(0))+"inx) S 1 1 1 0 -1 0 0 0 0 0 1 1 0 0 20 0 1 0 1 0 1"); ok {
		t.Fatal("NUL in stat must fail")
	}
	bytes, ok := rssPagesToBytes(2, 4096)
	if !ok || bytes != 8192 {
		t.Fatalf("rss pages %d ok=%v", bytes, ok)
	}
	if _, ok := rssPagesToBytes(2, 0); ok {
		t.Fatal("zero page size must fail")
	}
	if _, ok := rssPagesToBytes(^uint64(0), 4096); ok {
		t.Fatal("overflow must fail")
	}
}

func TestParseCgroupServiceAttribution(t *testing.T) {
	if got := parseCgroupService("0::/system.slice/nginx.service\n"); got != "nginx.service" {
		t.Fatalf("v2 got %q", got)
	}
	if got := parseCgroupService("1:name=systemd:/system.slice/sshd.service\n"); got != "sshd.service" {
		t.Fatalf("v1 got %q", got)
	}
	if got := parseCgroupService("0::/user.slice/user-1000.slice/session-1.scope\n"); got != "" {
		t.Fatalf("scope must not become service, got %q", got)
	}
	if got := parseCgroupService("0::/system.slice/../../etc/passwd\n"); got != "" {
		t.Fatalf("hostile cgroup must be rejected, got %q", got)
	}
	if got := parseCgroupService("0::/system.slice/foo@bar.service\n"); got != "foo@bar.service" {
		t.Fatalf("template unit got %q", got)
	}
}

func TestLinuxProcEnumerationMatchingAndBounds(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "nginx-bin")
	if err := os.WriteFile(bin, []byte("elf"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakePID(t, root, 42, "nginx", "S", bin, "0::/system.slice/nginx.service\n",
		"rchar: 1\nwchar: 1\nread_bytes: 4096\nwrite_bytes: 512\n", "", 11, 22, 1000, 8)
	writeFakePID(t, root, 7, "sshd", "R", "", "", "", "", 1, 1, 50, 2)
	if err := os.WriteFile(filepath.Join(root, "cpuinfo"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "self"), 0o755); err != nil {
		t.Fatal(err)
	}

	src := newTestLinuxSource(t, root)
	ctx := context.Background()
	snap, err := src.CollectMatching(ctx, []ProcessServiceSelector{
		{Kind: SelectorKindServiceID, Value: "nginx.service"},
		{Kind: SelectorKindProcessName, Value: "sshd"},
		{Kind: SelectorKindProcessName, Value: "missingd"},
		{Kind: SelectorKindExecutablePath, Value: bin},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := snap.Catalogue.Validate(); err != nil {
		t.Fatal(err)
	}
	if snap.ProcessSourceClass != ProcessServiceSourceObserved {
		t.Fatalf("class %s", snap.ProcessSourceClass)
	}
	if len(snap.Catalogue.Entries) != 2 {
		t.Fatalf("entries %d", len(snap.Catalogue.Entries))
	}
	var nginx ProcessServiceCatalogueEntry
	for _, e := range snap.Catalogue.Entries {
		if e.ProcessFamilyName == "nginx" {
			nginx = e
		}
	}
	if nginx.ServiceID != "nginx.service" || nginx.ExecutablePath != bin || nginx.ExecutableIdentity == "" {
		t.Fatalf("nginx identity %+v", nginx)
	}
	if nginx.Runtime == nil || nginx.Runtime.PID != 42 || nginx.Runtime.State != "S" {
		t.Fatalf("runtime %+v", nginx.Runtime)
	}
	if !nginx.Runtime.HasCPU || nginx.Runtime.CPUUserTicks != 11 || nginx.Runtime.CPUSystemTicks != 22 {
		t.Fatalf("cpu %+v", nginx.Runtime)
	}
	if !nginx.Runtime.HasStartTime || nginx.Runtime.StartTimeTicks != 1000 {
		t.Fatalf("start %+v", nginx.Runtime)
	}
	if !nginx.Runtime.HasRSS || nginx.Runtime.RSSBytes != 8*1024 {
		t.Fatalf("rss %+v", nginx.Runtime)
	}
	if !nginx.Runtime.HasIO || nginx.Runtime.IOReadBytes != 4096 || nginx.Runtime.IOWriteBytes != 512 {
		t.Fatalf("io %+v", nginx.Runtime)
	}
	if nginx.Runtime.InstanceToken != "42:1000" {
		t.Fatalf("token %q", nginx.Runtime.InstanceToken)
	}
	foundUnmatched := false
	for _, m := range snap.Matches {
		switch m.Selector.Value {
		case "nginx.service", bin, "sshd":
			if !m.Matched {
				t.Fatalf("expected match %#v", m)
			}
		case "missingd":
			foundUnmatched = true
			if m.Matched || m.SourceClass != ProcessServiceSourceSelectorUnmatched {
				t.Fatalf("unmatched %#v", m)
			}
		}
	}
	if !foundUnmatched {
		t.Fatal("missing selector not reported")
	}
	if snap.AssertsHostOrServiceDown() {
		t.Fatal("must not assert DOWN")
	}
}

func TestLinuxProcTruncationAndDirentBound(t *testing.T) {
	root := t.TempDir()
	src := newTestLinuxSource(t, root)
	src.maxEntries = 2
	for i := 1; i <= 5; i++ {
		writeFakePID(t, root, i, "w", "S", "", "", "", "", 0, 0, uint64(i), 1)
	}
	snap, err := src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Catalogue.Truncated || snap.Catalogue.IncompleteReason == "" {
		t.Fatalf("expected truncation %#v", snap.Catalogue)
	}
	if len(snap.Catalogue.Entries) != 2 {
		t.Fatalf("entries %d", len(snap.Catalogue.Entries))
	}
	if got := cataloguePIDs(snap); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("truncated PID set must be numeric-lowest, got %v", got)
	}
	if snap.ProcessSourceClass != ProcessServiceSourceTruncated {
		t.Fatalf("class %s", snap.ProcessSourceClass)
	}
	if err := snap.Catalogue.Validate(); err != nil {
		t.Fatal(err)
	}

	root2 := t.TempDir()
	src2 := newTestLinuxSource(t, root2)
	src2.maxDirents = 3
	for i := 1; i <= 8; i++ {
		writeFakePID(t, root2, 100+i, "w", "S", "", "", "", "", 0, 0, uint64(i), 1)
	}
	snap2, err := src2.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snap2.Catalogue.Truncated || snap2.ProcessSourceClass != ProcessServiceSourceTruncated {
		t.Fatalf("dirent truncation %#v class=%s", snap2.Catalogue, snap2.ProcessSourceClass)
	}
}

func TestLinuxDisappearingPIDAndMalformedOversized(t *testing.T) {
	root := t.TempDir()
	writeFakePID(t, root, 1, "init", "S", "", "", "", "", 0, 0, 1, 1)
	emptyPID := filepath.Join(root, "99")
	if err := os.Mkdir(emptyPID, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(root, "70")
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "stat"), []byte("not-a-stat"), 0o644); err != nil {
		t.Fatal(err)
	}
	huge := filepath.Join(root, "80")
	if err := os.Mkdir(huge, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(huge, "stat"), []byte(strings.Repeat("x", maxProcFileBytes+8)), 0o644); err != nil {
		t.Fatal(err)
	}
	longName := filepath.Join(root, "81")
	if err := os.Mkdir(longName, 0o755); err != nil {
		t.Fatal(err)
	}
	longComm := strings.Repeat("n", maxProcessServiceNameLen+1)
	if err := os.WriteFile(filepath.Join(longName, "stat"), []byte(fakeProcStat(81, longComm, "S", 1, 1, 1, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	longPathRoot := root
	longExe := filepath.Join(root, "82")
	if err := os.Mkdir(longExe, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(longExe, "stat"), []byte(fakeProcStat(82, "ok", "S", 1, 1, 1, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(longExe, "comm"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/"+strings.Repeat("p", maxProcessServiceIdentityLen+8), filepath.Join(longExe, "exe")); err != nil {
		t.Fatal(err)
	}

	src := newTestLinuxSource(t, longPathRoot)
	snap, err := src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range snap.Catalogue.Entries {
		if e.Runtime != nil && (e.Runtime.PID == 99 || e.Runtime.PID == 70 || e.Runtime.PID == 80 || e.Runtime.PID == 81) {
			t.Fatalf("race/malformed/oversized pid must not be catalogued: %+v", e)
		}
		if e.Runtime != nil && e.Runtime.PID == 82 && e.ExecutablePath != "" {
			t.Fatalf("oversized exe path must not be stored: %q", e.ExecutablePath)
		}
	}
	foundInit := false
	for _, e := range snap.Catalogue.Entries {
		if e.ProcessFamilyName == "init" {
			foundInit = true
		}
	}
	if !foundInit {
		t.Fatal("valid pid 1 must remain")
	}
}

func TestLinuxHostileProcessNameAndProcPathSafety(t *testing.T) {
	if _, err := procPIDPath("/proc", 1, "cmdline"); err == nil {
		t.Fatal("cmdline leaf must be rejected")
	}
	if _, err := procPIDPath("/proc", 1, "environ"); err == nil {
		t.Fatal("environ leaf must be rejected")
	}
	if _, err := procPIDPath("/proc", 1, "../stat"); err == nil {
		t.Fatal("traversal leaf must be rejected")
	}
	if _, err := procPIDPath("/tmp/proc", 1, "stat"); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeFakePID(t, root, 5, "x", "S", "", "", "", "", 1, 1, 1, 1)
	// Overwrite comm/stat with control-bearing name after the valid skeleton.
	if err := os.WriteFile(filepath.Join(root, "5", "stat"), []byte(fakeProcStat(5, "bad\x01name", "S", 1, 1, 1, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	src := newTestLinuxSource(t, root)
	snap, err := src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range snap.Catalogue.Entries {
		if strings.Contains(e.ProcessFamilyName, "\n") {
			t.Fatal("hostile name must not be catalogued")
		}
	}
}

func TestLinuxNoCmdlineOrEnvironCollection(t *testing.T) {
	src, err := os.ReadFile("process_service_linux.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, forbidden := range []string{`procPIDLeafCmdline`, `"/cmdline"`, `"/environ"`, `procPIDLeafEnviron`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("implementation references forbidden source %s", forbidden)
		}
	}
	if !strings.Contains(body, `procPIDLeafStat`) || !strings.Contains(body, `procPIDLeafComm`) {
		t.Fatal("expected allowlisted proc leaves")
	}
}

func TestLinuxContextCancellation(t *testing.T) {
	root := t.TempDir()
	writeFakePID(t, root, 1, "init", "S", "", "", "", "", 1, 1, 1, 1)
	src := newTestLinuxSource(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := src.Collect(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
}

func TestLinuxProcUnavailable(t *testing.T) {
	src := newTestLinuxSource(t, filepath.Join(t.TempDir(), "missing-proc"))
	snap, err := src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.ProcessSourceClass != ProcessServiceSourceUnavailable {
		t.Fatalf("class %s", snap.ProcessSourceClass)
	}
	if snap.AssertsHostOrServiceDown() {
		t.Fatal("unavailable proc is not host DOWN")
	}
}

func TestLinuxSourceIsNotHostCollectorAndNotScalar(t *testing.T) {
	srcType := reflect.TypeOf((*LinuxProcessServiceSource)(nil))
	host := reflect.TypeOf((*HostCollector)(nil)).Elem()
	if srcType.Implements(host) || srcType.Elem().Implements(host) {
		t.Fatal("LinuxProcessServiceSource must not implement HostCollector")
	}
	src := NewLinuxProcessServiceSource()
	snap, err := src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	samples, encErr := EncodeProcessServiceCatalogueAsScalarSamples(snap.Catalogue)
	if encErr == nil || samples != nil {
		t.Fatal("catalogue must not encode as samples")
	}
	_ = telemetry.Sample{}
}

func TestLinuxSystemdRunnerUnavailablePermissionTimeoutTruncation(t *testing.T) {
	root := t.TempDir()
	writeFakePID(t, root, 1, "init", "S", "", "", "", "", 1, 1, 1, 1)
	src := newTestLinuxSource(t, root)

	snap, err := src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.SystemdSourceClass != ProcessServiceSourceUnavailable {
		t.Fatalf("empty path should be unavailable, got %s", snap.SystemdSourceClass)
	}

	src.systemctlPath = "/usr/bin/systemctl"
	src.runner = func(ctx context.Context, executable string, args []string) systemctlRunResult {
		if executable != "/usr/bin/systemctl" {
			t.Fatalf("executable %s", executable)
		}
		if err := assertSystemctlArgvSafe(args); err != nil {
			t.Fatal(err)
		}
		return systemctlRunResult{err: os.ErrPermission}
	}
	snap, err = src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.SystemdSourceClass != ProcessServiceSourcePermissionDenied {
		t.Fatalf("class %s", snap.SystemdSourceClass)
	}
	if snap.AssertsHostOrServiceDown() {
		t.Fatal("permission denied is not DOWN")
	}

	src.systemdTimeout = 30 * time.Millisecond
	src.runner = func(ctx context.Context, executable string, args []string) systemctlRunResult {
		<-ctx.Done()
		return systemctlRunResult{err: ctx.Err()}
	}
	snap, err = src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.SystemdSourceClass != ProcessServiceSourceUnavailable {
		t.Fatalf("timeout class %s", snap.SystemdSourceClass)
	}

	src.runner = func(ctx context.Context, executable string, args []string) systemctlRunResult {
		if args[0] == systemdListUnitsVerb {
			var b strings.Builder
			for i := 0; i < 4; i++ {
				fmt.Fprintf(&b, "u%d.service loaded active running x\n", i)
			}
			return systemctlRunResult{stdout: []byte(b.String())}
		}
		return systemctlRunResult{stdout: bytesRepeatShow(4), truncated: true}
	}
	src.maxEntries = 2048
	snap, err = src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Catalogue.Truncated && snap.SystemdSourceClass != ProcessServiceSourceTruncated {
		t.Fatalf("expected systemd truncation class=%s truncated=%v", snap.SystemdSourceClass, snap.Catalogue.Truncated)
	}
}

func bytesRepeatShow(n int) []byte {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "Id=u%d.service\nActiveState=active\nSubState=running\nResult=success\nNRestarts=0\nMainPID=%d\nExecMainStatus=0\n\n", i, i+1)
	}
	return []byte(b.String())
}

func TestLinuxSystemdHappyPathMainPIDEphemeral(t *testing.T) {
	root := t.TempDir()
	writeFakePID(t, root, 1, "init", "S", "", "", "", "", 1, 1, 1, 1)
	src := newTestLinuxSource(t, root)
	src.systemctlPath = "/bin/systemctl"
	src.runner = func(ctx context.Context, executable string, args []string) systemctlRunResult {
		if err := assertSystemctlArgvSafe(args); err != nil {
			return systemctlRunResult{err: err}
		}
		switch args[0] {
		case systemdListUnitsVerb:
			return systemctlRunResult{stdout: []byte("nginx.service loaded active running nginx\n")}
		case systemdShowVerb:
			return systemctlRunResult{stdout: []byte("Id=nginx.service\nActiveState=active\nSubState=running\nResult=success\nNRestarts=3\nMainPID=4242\nExecMainStatus=0\n")}
		default:
			return systemctlRunResult{err: fmt.Errorf("unexpected verb %s", args[0])}
		}
	}
	snap, err := src.CollectMatching(context.Background(), []ProcessServiceSelector{
		{Kind: SelectorKindServiceID, Value: "nginx.service"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if snap.SystemdSourceClass != ProcessServiceSourceObserved || len(snap.Services) != 1 {
		t.Fatalf("services %#v class=%s", snap.Services, snap.SystemdSourceClass)
	}
	svc := snap.Services[0]
	if svc.NRestarts != 3 || svc.MainPID != 4242 || svc.ExecMainStatus != 0 {
		t.Fatalf("%+v", svc)
	}
	if svc.MainPIDIsDurableSelector() {
		t.Fatal("MainPID durable")
	}
	if len(snap.Matches) != 1 || !snap.Matches[0].Matched || snap.Matches[0].Service == nil {
		t.Fatalf("matches %#v", snap.Matches)
	}
}

func TestLimitedBufferTruncates(t *testing.T) {
	var buf limitedBuffer
	buf.limit = 8
	n, err := buf.Write([]byte("0123456789abcdef"))
	if err != nil || n != 16 || !buf.truncated || string(buf.buf) != "01234567" {
		t.Fatalf("buf=%q trunc=%v n=%d err=%v", buf.buf, buf.truncated, n, err)
	}
}

func TestLiveLinuxProcessDiscoverySmoke(t *testing.T) {
	src := NewLinuxProcessServiceSource()
	src.systemctlPath = ""
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	snap, err := src.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ProcessSourceClass != ProcessServiceSourceObserved && snap.ProcessSourceClass != ProcessServiceSourceTruncated {
		t.Fatalf("live proc class %s", snap.ProcessSourceClass)
	}
	if len(snap.Catalogue.Entries) == 0 {
		t.Fatal("live /proc discovery returned no processes")
	}
	if err := snap.Catalogue.Validate(); err != nil {
		t.Fatal(err)
	}
	self := os.Getpid()
	foundSelf := false
	for _, e := range snap.Catalogue.Entries {
		if e.Runtime != nil && e.Runtime.PID == self {
			foundSelf = true
		}
		if e.Runtime != nil && e.Runtime.PID <= 0 {
			t.Fatal("invalid pid")
		}
		pidSel := ProcessServiceSelector{Kind: "PID", Value: fmt.Sprintf("%d", e.Runtime.PID)}
		if pidSel.Validate() == nil {
			t.Fatal("live PID accepted as durable selector")
		}
	}
	if !foundSelf {
		t.Log("current PID not in bounded catalogue (possible truncation); other processes still observed")
	}
	if snap.AssertsHostOrServiceDown() {
		t.Fatal("live discovery must not assert DOWN")
	}
}

func TestLiveSystemdReadOnlySmoke(t *testing.T) {
	src := NewLinuxProcessServiceSource()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	snap, err := src.Collect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	switch snap.SystemdSourceClass {
	case ProcessServiceSourceUnavailable, ProcessServiceSourcePermissionDenied, ProcessServiceSourceUnsupported:
		t.Logf("systemd not available in this environment: %s", snap.SystemdSourceClass)
		if snap.AssertsHostOrServiceDown() {
			t.Fatal("unavailable systemd must not assert DOWN")
		}
		return
	case ProcessServiceSourceObserved, ProcessServiceSourceTruncated:
	default:
		t.Fatalf("unexpected systemd class %s", snap.SystemdSourceClass)
	}
	if len(snap.Services) == 0 && snap.SystemdSourceClass == ProcessServiceSourceObserved {
		t.Log("systemd observed with zero service units")
	}
	for _, svc := range snap.Services {
		if err := validateSystemdUnitID(svc.UnitID); err != nil {
			t.Fatalf("unit %q: %v", svc.UnitID, err)
		}
		if svc.MainPIDIsDurableSelector() {
			t.Fatal("live MainPID became durable selector")
		}
	}
}

func TestDefaultCollectorsUnchangedByProcessSource(t *testing.T) {
	cols := NewDefaultCollectors(16)
	names := map[string]bool{}
	for _, c := range cols {
		names[c.Name()] = true
		if c.Name() == "process" || c.Name() == "process_service" {
			t.Fatal("process/service source must not register as HostCollector")
		}
	}
	for _, need := range []string{"cpu", "memory", "filesystem", "pressure", "diskio", "net", "osfacts"} {
		if !names[need] {
			t.Fatalf("missing collector %s", need)
		}
	}
}

func TestExecSystemctlRejectsNonAllowlisted(t *testing.T) {
	res := execSystemctl(context.Background(), "/bin/sh", []string{"show"})
	if !errors.Is(res.err, os.ErrNotExist) && res.err == nil {
		t.Fatalf("expected reject, got %+v", res)
	}
	res = execSystemctl(context.Background(), "/usr/bin/systemctl", []string{"restart", "nginx.service"})
	if res.err == nil {
		t.Fatal("mutation argv must not execute")
	}
}

func TestClassifyPermissionSyscall(t *testing.T) {
	if classifyProcPIDError(syscall.EACCES) != ProcessServiceSourcePermissionDenied {
		t.Fatal("EACCES")
	}
	if classifyProcPIDError(syscall.ESRCH) != ProcessServiceSourcePIDRace {
		t.Fatal("ESRCH")
	}
	if classifyProcOpenError(os.ErrNotExist) != ProcessServiceSourceUnavailable {
		t.Fatal("missing proc")
	}
}

func cataloguePIDs(snap ProcessServiceSourceSnapshot) []int {
	out := make([]int, 0, len(snap.Catalogue.Entries))
	for _, e := range snap.Catalogue.Entries {
		if e.Runtime != nil {
			out = append(out, e.Runtime.PID)
		}
	}
	return out
}

func TestLinuxProcTruncationIsDeterministicAcrossCreationOrder(t *testing.T) {
	orders := [][]int{
		{5, 1, 4, 2, 3},
		{1, 2, 3, 4, 5},
		{5, 4, 3, 2, 1},
		{3, 5, 1, 4, 2},
	}
	var first []int
	for i, order := range orders {
		root := t.TempDir()
		for _, pid := range order {
			writeFakePID(t, root, pid, "w", "S", "", "", "", "", 0, 0, uint64(pid), 1)
		}
		src := newTestLinuxSource(t, root)
		src.maxEntries = 3
		src.maxDirents = 64
		snap, err := src.Collect(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !snap.Catalogue.Truncated || snap.ProcessSourceClass != ProcessServiceSourceTruncated {
			t.Fatalf("order %v expected truncation %#v class=%s", order, snap.Catalogue, snap.ProcessSourceClass)
		}
		got := cataloguePIDs(snap)
		want := []int{1, 2, 3}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("creation order %v selected %v want %v", order, got, want)
		}
		if i == 0 {
			first = append([]int{}, got...)
		} else if !reflect.DeepEqual(got, first) {
			t.Fatalf("selected PID set drifted: first=%v got=%v order=%v", first, got, order)
		}
	}
}

func TestLinuxMixedPIDSourceQualityEvidence(t *testing.T) {
	root := t.TempDir()
	writeFakePID(t, root, 1, "init", "S", "", "", "", "", 0, 0, 1, 1)

	raceDir := filepath.Join(root, "99")
	if err := os.Mkdir(raceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	permPID := 40
	writeFakePID(t, root, permPID, "secret", "S", "", "", "", "", 0, 0, 40, 1)

	bad := filepath.Join(root, "70")
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "stat"), []byte("not-a-stat"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := newTestLinuxSource(t, root)
	src.openProcFile = func(path string, limit int) ([]byte, error) {
		if strings.Contains(path, "/"+strconvPID(permPID)+"/") {
			return nil, os.ErrPermission
		}
		return readBoundedFile(path, limit)
	}
	snap, err := src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foundInit := false
	for _, e := range snap.Catalogue.Entries {
		if e.Runtime != nil && (e.Runtime.PID == 99 || e.Runtime.PID == permPID || e.Runtime.PID == 70) {
			t.Fatalf("failed pid must not be catalogued: %+v", e)
		}
		if e.ProcessFamilyName == "init" {
			foundInit = true
		}
	}
	if !foundInit {
		t.Fatal("valid process must remain usable")
	}
	q := snap.SourceQuality
	if q.PIDRace == 0 || q.PermissionDenied == 0 || q.MalformedProcess == 0 {
		t.Fatalf("expected all categorical counters, got %+v", q)
	}
	if q.PIDRace > 8 || q.PermissionDenied > 8 || q.MalformedProcess > 8 {
		t.Fatalf("categorical failures must stay bounded, got %+v", q)
	}
	if snap.Catalogue.Quality != "PARTIAL" {
		t.Fatalf("mixed quality %q", snap.Catalogue.Quality)
	}
	if snap.AssertsHostOrServiceDown() {
		t.Fatal("source quality loss must not infer DOWN")
	}
}

func TestLinuxValidProcessPlusPIDRace(t *testing.T) {
	root := t.TempDir()
	writeFakePID(t, root, 1, "init", "S", "", "", "", "", 0, 0, 1, 1)
	if err := os.Mkdir(filepath.Join(root, "88"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := newTestLinuxSource(t, root)
	snap, err := src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.SourceQuality.PIDRace == 0 || len(snap.Catalogue.Entries) == 0 {
		t.Fatalf("expected valid+race %+v entries=%d", snap.SourceQuality, len(snap.Catalogue.Entries))
	}
	if snap.Catalogue.Quality != "PARTIAL" || snap.AssertsHostOrServiceDown() {
		t.Fatalf("quality=%s down=%v", snap.Catalogue.Quality, snap.AssertsHostOrServiceDown())
	}
}

func TestLinuxValidProcessPlusPermissionDenied(t *testing.T) {
	root := t.TempDir()
	writeFakePID(t, root, 1, "init", "S", "", "", "", "", 0, 0, 1, 1)
	writeFakePID(t, root, 50, "hidden", "S", "", "", "", "", 0, 0, 50, 1)
	src := newTestLinuxSource(t, root)
	src.openProcFile = func(path string, limit int) ([]byte, error) {
		if strings.Contains(path, "/50/") {
			return nil, syscall.EACCES
		}
		return readBoundedFile(path, limit)
	}
	snap, err := src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.SourceQuality.PermissionDenied == 0 || len(snap.Catalogue.Entries) == 0 {
		t.Fatalf("expected valid+permission %+v", snap.SourceQuality)
	}
	if snap.Catalogue.Quality != "PARTIAL" || snap.AssertsHostOrServiceDown() {
		t.Fatal("permission denied must be PARTIAL, not DOWN")
	}
}

func TestLinuxValidProcessPlusMalformedPIDSource(t *testing.T) {
	root := t.TempDir()
	writeFakePID(t, root, 1, "init", "S", "", "", "", "", 0, 0, 1, 1)
	bad := filepath.Join(root, "71")
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "stat"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := newTestLinuxSource(t, root)
	snap, err := src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.SourceQuality.MalformedProcess == 0 || len(snap.Catalogue.Entries) == 0 {
		t.Fatalf("expected valid+malformed %+v", snap.SourceQuality)
	}
	if snap.Catalogue.Quality != "PARTIAL" || snap.AssertsHostOrServiceDown() {
		t.Fatal("malformed pid source must be PARTIAL, not DOWN")
	}
}

func TestLinuxSystemdMixedMalformedPartialQuality(t *testing.T) {
	root := t.TempDir()
	writeFakePID(t, root, 1, "init", "S", "", "", "", "", 1, 1, 1, 1)
	src := newTestLinuxSource(t, root)
	src.systemctlPath = "/usr/bin/systemctl"
	src.runner = func(ctx context.Context, executable string, args []string) systemctlRunResult {
		if err := assertSystemctlArgvSafe(args); err != nil {
			return systemctlRunResult{err: err}
		}
		switch args[0] {
		case systemdListUnitsVerb:
			return systemctlRunResult{stdout: []byte("nginx.service loaded active running nginx\nsshd.service loaded active running sshd\n")}
		case systemdShowVerb:
			return systemctlRunResult{stdout: []byte("Id=nginx.service\nActiveState=active\nSubState=running\nResult=success\nNRestarts=0\nMainPID=9\nExecMainStatus=0\n\nnot-a-block\n")}
		default:
			return systemctlRunResult{err: fmt.Errorf("unexpected verb %s", args[0])}
		}
	}
	snap, err := src.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snap.SourceQuality.SystemdRejectedBlocks == 0 {
		t.Fatalf("malformed systemd block must be counted: %+v", snap.SourceQuality)
	}
	if len(snap.Services) != 1 || snap.Services[0].UnitID != "nginx.service" {
		t.Fatalf("valid systemd evidence must remain: %#v", snap.Services)
	}
	if snap.Catalogue.Quality != "PARTIAL" {
		t.Fatalf("quality %s", snap.Catalogue.Quality)
	}
	if snap.SystemdSourceClass != ProcessServiceSourceObserved {
		t.Fatalf("mixed systemd class %s", snap.SystemdSourceClass)
	}
	if snap.AssertsHostOrServiceDown() {
		t.Fatal("systemd malformed mix must not infer DOWN")
	}
}

func TestLinuxCallerCancelDuringSystemdExecution(t *testing.T) {
	root := t.TempDir()
	writeFakePID(t, root, 1, "init", "S", "", "", "", "", 1, 1, 1, 1)
	src := newTestLinuxSource(t, root)
	src.systemctlPath = "/usr/bin/systemctl"
	src.systemdTimeout = 3 * time.Second
	started := make(chan struct{})
	src.runner = func(ctx context.Context, executable string, args []string) systemctlRunResult {
		select {
		case <-started:
		default:
			close(started)
		}
		<-ctx.Done()
		return systemctlRunResult{err: ctx.Err()}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := src.Collect(ctx)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("systemd runner did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Collect did not return after parent cancel")
	}
}

func TestLinuxInternalSystemdTimeoutDistinctFromCallerCancel(t *testing.T) {
	root := t.TempDir()
	writeFakePID(t, root, 1, "init", "S", "", "", "", "", 1, 1, 1, 1)
	src := newTestLinuxSource(t, root)
	src.systemctlPath = "/usr/bin/systemctl"
	src.systemdTimeout = 30 * time.Millisecond
	src.runner = func(ctx context.Context, executable string, args []string) systemctlRunResult {
		<-ctx.Done()
		return systemctlRunResult{err: ctx.Err()}
	}
	parent, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	snap, err := src.Collect(parent)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("internal timeout must not surface as caller error, got %v", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatal("internal timeout must not be context.Canceled")
	}
	if snap.SystemdSourceClass != ProcessServiceSourceUnavailable {
		t.Fatalf("timeout class %s", snap.SystemdSourceClass)
	}
	if snap.AssertsHostOrServiceDown() {
		t.Fatal("timeout is not DOWN")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("internal timeout not bounded, elapsed=%s", elapsed)
	}
	if parent.Err() != nil {
		t.Fatalf("parent context should remain live, err=%v", parent.Err())
	}
}
