package collector

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func collectorWithRPMStdout(t *testing.T, stdout string, runErr error, captured *rpmInvocation) *LinuxSoftwareInventoryCollector {
	t.Helper()
	c := newTestCollector(t)
	c.rpmResolver = func() (string, error) { return "/usr/bin/rpm", nil }
	c.rpmRunner = func(ctx context.Context, inv rpmInvocation, stdoutBuf, stderrBuf *boundedBuffer) error {
		if captured != nil {
			*captured = inv
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if stdout != "" {
			_, _ = stdoutBuf.Write([]byte(stdout))
		}
		return runErr
	}
	return c
}

func fullRPMRequest(id string) SoftwareInventoryRequest {
	return SoftwareInventoryRequest{
		SnapshotID:   id,
		SnapshotMode: SnapshotModeFull,
		SourceType:   InventorySourceTypeRPM,
	}
}

func TestRPMCompleteFixtureMappingAndNEVRA(t *testing.T) {
	raw, err := os.ReadFile(fixtureFile(t, "rpm_qa_complete.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var inv rpmInvocation
	c := collectorWithRPMStdout(t, string(raw), nil, &inv)
	snap, err := c.Capture(context.Background(), fullRPMRequest("snap-rpm-full-001"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if err := snap.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusComplete || snap.Truncated {
		t.Fatalf("expected COMPLETE, got %s truncated=%v reason=%q", snap.CaptureStatus, snap.Truncated, snap.IncompleteOrLossReason)
	}
	if snap.InventoryScope != linuxRPMInventoryScope {
		t.Fatalf("scope=%q", snap.InventoryScope)
	}
	if inv.Executable != "/usr/bin/rpm" {
		t.Fatalf("executable=%q", inv.Executable)
	}
	if err := rpmArgsSafe(inv.Args); err != nil {
		t.Fatal(err)
	}

	wantIDs := map[string]int{
		"rpm:x86_64:openssl=1:3.0.7-18.el9": 1,
		"rpm:x86_64:bash=0:5.1.8-6.el9":     2,
		"rpm:x86_64:glibc=0:2.34-100.el9":   1,
		"rpm:i686:openssl=1:3.0.7-18.el9":   1,
	}
	got := map[string]int{}
	var sawVendor, sawNoVendor bool
	for _, item := range snap.Items {
		got[item.NativePackageID]++
		if item.NormalizedProductID != "" {
			t.Fatal("Slice 4 must not populate NormalizedProductID")
		}
		if item.Architecture == "amd64" {
			t.Fatal("RPM architecture must not be normalized into DPKG architecture")
		}
		eq := strings.Index(item.NativePackageID, "=")
		if eq < 0 || item.NativePackageID[eq+1:] != item.Version {
			t.Fatalf("NativePackageID EVR must match typed Version: id=%q version=%q", item.NativePackageID, item.Version)
		}
		if item.ProductName == "openssl" && item.Architecture == "x86_64" {
			if item.Version != "1:3.0.7-18.el9" {
				t.Fatalf("typed EVR version=%q", item.Version)
			}
			if item.VendorPublisher != "Red Hat, Inc." {
				t.Fatalf("vendor=%q", item.VendorPublisher)
			}
			if item.AuthoritativeInstallOrUpdateAt == nil {
				t.Fatal("expected provenance-qualified RPM INSTALLTIME")
			}
			sawVendor = true
		}
		if item.ProductName == "bash" && item.Version != "0:5.1.8-6.el9" {
			t.Fatalf("bash typed EVR version=%q", item.Version)
		}
		if item.ProductName == "glibc" {
			if item.VendorPublisher != "" {
				t.Fatalf("none vendor must be omitted, got %q", item.VendorPublisher)
			}
			sawNoVendor = true
		}
	}
	if !sawVendor || !sawNoVendor {
		t.Fatal("expected optional vendor handling")
	}
	for id, n := range wantIDs {
		if got[id] != n {
			t.Fatalf("identity %s: got %d want %d (%v)", id, got[id], n, got)
		}
	}
}

func TestRPMNoEpochDeterministic(t *testing.T) {
	stdout := "zsh\t(none)\t5.8\t1.el9\tx86_64\t(none)\t0\nzsh\t\t5.8\t1.el9\tx86_64\tVendor\t1\n"
	c := collectorWithRPMStdout(t, stdout, nil, nil)
	snap, err := c.Capture(context.Background(), fullRPMRequest("snap-rpm-epoch"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.ItemCount != 2 {
		t.Fatalf("count=%d", snap.ItemCount)
	}
	for _, item := range snap.Items {
		if item.NativePackageID != "rpm:x86_64:zsh=0:5.8-1.el9" {
			t.Fatalf("no-epoch identity must be explicit zero, got %q", item.NativePackageID)
		}
		if item.Version != "0:5.8-1.el9" {
			t.Fatalf("no-epoch typed Version must preserve EVR, got %q", item.Version)
		}
	}
}

func TestRPMMalformedOversizedAndDuplicates(t *testing.T) {
	stdout := "good\t0\t1\t1.el9\tx86_64\tV\t1\nnot-enough-fields\nbad\t0\t" + strings.Repeat("v", maxSoftwareInventoryVersionLen+8) + "\t1\tx86_64\n"
	c := collectorWithRPMStdout(t, stdout+"\ngood\t0\t1\t1.el9\tx86_64\tV\t1\n", nil, nil)
	snap, err := c.Capture(context.Background(), fullRPMRequest("snap-rpm-malformed"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusPartial {
		t.Fatalf("expected PARTIAL, got %s reason=%q", snap.CaptureStatus, snap.IncompleteOrLossReason)
	}
	if snap.ItemCount != 2 {
		t.Fatalf("duplicates of usable records must be preserved, count=%d", snap.ItemCount)
	}
}

func TestRPMExecutableUnavailablePermissionAndExecutionFailure(t *testing.T) {
	c := newTestCollector(t)
	c.rpmResolver = func() (string, error) { return "", os.ErrNotExist }
	snap, err := c.Capture(context.Background(), fullRPMRequest("snap-rpm-missing"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusFailed || snap.ItemCount != 0 {
		t.Fatalf("unavailable must be FAILED empty, got %s count=%d", snap.CaptureStatus, snap.ItemCount)
	}

	c = collectorWithRPMStdout(t, "", os.ErrPermission, nil)
	snap, err = c.Capture(context.Background(), fullRPMRequest("snap-rpm-eacces"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusFailed || !strings.Contains(snap.IncompleteOrLossReason, inventoryLossSourcePermissionDenied) {
		t.Fatalf("permission: status=%s reason=%q", snap.CaptureStatus, snap.IncompleteOrLossReason)
	}

	c = collectorWithRPMStdout(t, "", errors.New("rpm exploded"), nil)
	snap, err = c.Capture(context.Background(), fullRPMRequest("snap-rpm-exec-fail"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus == CaptureStatusComplete {
		t.Fatal("execution failure must not COMPLETE empty")
	}
	if snap.ItemCount != 0 || snap.CaptureStatus != CaptureStatusFailed {
		t.Fatalf("status=%s count=%d", snap.CaptureStatus, snap.ItemCount)
	}
}

func TestRPMInternalQueryTimeoutNotCallerDeadline(t *testing.T) {
	c := newTestCollector(t)
	c.bounds.RPMQueryTimeout = 20 * time.Millisecond
	c.rpmResolver = func() (string, error) { return "/usr/bin/rpm", nil }
	c.rpmRunner = func(ctx context.Context, inv rpmInvocation, stdout, stderr *boundedBuffer) error {
		<-ctx.Done()
		return ctx.Err()
	}
	parent, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	snap, err := c.Capture(parent, fullRPMRequest("snap-rpm-timeout"))
	if err != nil {
		t.Fatalf("internal timeout must not surface as caller error, got %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("internal RPM timeout must not become caller DeadlineExceeded")
	}
	if snap.CaptureStatus != CaptureStatusFailed {
		t.Fatalf("internal timeout expected FAILED, got %s", snap.CaptureStatus)
	}
	if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossSourceTimeout) {
		t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
	}
}

func TestRPMCallerCancellationAndDeadline(t *testing.T) {
	c := newTestCollector(t)
	c.rpmResolver = func() (string, error) { return "/usr/bin/rpm", nil }
	c.rpmRunner = func(ctx context.Context, inv rpmInvocation, stdout, stderr *boundedBuffer) error {
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Capture(ctx, fullRPMRequest("snap-rpm-cancel"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected Canceled, got %v", err)
	}

	deadline, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	_, err = c.Capture(deadline, fullRPMRequest("snap-rpm-deadline"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestRPMStdoutStderrBoundsAndNoRawPayload(t *testing.T) {
	c := newTestCollector(t)
	c.bounds.MaxRPMStdoutBytes = 32
	c.bounds.MaxRPMStderrBytes = 8
	c.rpmResolver = func() (string, error) { return "/usr/bin/rpm", nil }
	c.rpmRunner = func(ctx context.Context, inv rpmInvocation, stdout, stderr *boundedBuffer) error {
		_, _ = stdout.Write([]byte(strings.Repeat("openssl\t1\t3\t1.el9\tx86_64\tV\t1\n", 20)))
		_, _ = stderr.Write([]byte("SUPER_SECRET_RPM_STDERR_SHOULD_NOT_LEAK"))
		return nil
	}
	snap, err := c.Capture(context.Background(), fullRPMRequest("snap-rpm-bounds"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus == CaptureStatusComplete {
		t.Fatal("stdout truncation must not COMPLETE")
	}
	blob := snap.IncompleteOrLossReason + snap.Quality + snap.SourceProvenance
	if strings.Contains(blob, "SUPER_SECRET") || strings.Contains(blob, "RPM_STDERR") {
		t.Fatalf("raw stderr leaked into snapshot: %#v", snap)
	}
	for _, item := range snap.Items {
		if strings.Contains(item.ProductName, "SUPER_SECRET") {
			t.Fatal("raw rpm output leaked into items")
		}
	}
}

func TestRPMAllowlistFixedArgvNoShellNoMutation(t *testing.T) {
	if isAllowlistedRPMExecutable("/tmp/rpm") || isAllowlistedRPMExecutable("rpm") || isAllowlistedRPMExecutable("/usr/bin/bash") {
		t.Fatal("allowlist must reject non-allowlisted paths")
	}
	if !isAllowlistedRPMExecutable("/usr/bin/rpm") || !isAllowlistedRPMExecutable("/bin/rpm") {
		t.Fatal("allowlist must include /usr/bin/rpm and /bin/rpm")
	}
	args := rpmInventoryArgv()
	if err := rpmArgsSafe(args); err != nil {
		t.Fatal(err)
	}
	if args[0] != "-qa" || args[1] != "--queryformat" || args[2] != rpmQueryFormat {
		t.Fatalf("fixed argv mismatch: %#v", args)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "sh -c") || strings.Contains(joined, "bash") {
		t.Fatal("argv must not invoke a shell")
	}
	for _, tok := range rpmMutationTokens {
		for _, arg := range args {
			if arg == tok {
				t.Fatalf("mutation token %q in argv", tok)
			}
		}
	}
	if err := rpmArgsSafe([]string{"-e", "openssl"}); err == nil {
		t.Fatal("mutation argv must be rejected")
	}
}

func TestRPMTargetedLocalFilterDoesNotAlterArgv(t *testing.T) {
	raw, err := os.ReadFile(fixtureFile(t, "rpm_qa_complete.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var fullInv, targetedInv rpmInvocation
	full := collectorWithRPMStdout(t, string(raw), nil, &fullInv)
	_, err = full.Capture(context.Background(), fullRPMRequest("snap-rpm-argv-full"))
	if err != nil {
		t.Fatal(err)
	}
	targeted := collectorWithRPMStdout(t, string(raw), nil, &targetedInv)
	snap, err := targeted.Capture(context.Background(), SoftwareInventoryRequest{
		SnapshotID:   "snap-rpm-targeted",
		SnapshotMode: SnapshotModeTargeted,
		SourceType:   InventorySourceTypeRPM,
		Selectors: []SoftwareInventorySelector{
			{ProductName: "bash"},
			{NativePackageID: "rpm:i686:openssl=1:3.0.7-18.el9"},
			{ProductName: "$(reboot); rm -rf /"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(fullInv.Args, "\x00") != strings.Join(targetedInv.Args, "\x00") {
		t.Fatalf("TARGETED values mutated argv: full=%q targeted=%q", fullInv.Args, targetedInv.Args)
	}
	if snap.ItemCount != 3 {
		t.Fatalf("expected bash duplicates + i686 openssl, got %d %#v", snap.ItemCount, nativeIDs(snap))
	}
	for _, arg := range targetedInv.Args {
		if strings.Contains(arg, "bash") || strings.Contains(arg, "reboot") || strings.Contains(arg, "openssl") {
			t.Fatalf("package/customer value entered argv: %q", arg)
		}
	}
}

func TestRPMFailureDoesNotCreateCompleteEmpty(t *testing.T) {
	c := collectorWithRPMStdout(t, "", exec.ErrNotFound, nil)
	snap, err := c.Capture(context.Background(), fullRPMRequest("snap-rpm-empty-fail"))
	if err != nil {
		t.Fatal(err)
	}
	if snap.CaptureStatus == CaptureStatusComplete {
		t.Fatal("failure must not create COMPLETE empty snapshot")
	}
}

func TestRPMNotFoundResolverUsesAllowlistOnly(t *testing.T) {
	c := newTestCollector(t)
	c.rpmResolver = func() (string, error) { return "/tmp/evil-rpm", nil }
	snap, err := c.Capture(context.Background(), fullRPMRequest("snap-rpm-not-allowlisted"))
	if err != nil {
		t.Fatal(err)
	}
	if snap.CaptureStatus != CaptureStatusFailed {
		t.Fatalf("non-allowlisted resolver path must fail closed, got %s", snap.CaptureStatus)
	}
}

func TestRPMTargetedZeroSelectorsNeverInvokeRPM(t *testing.T) {
	resolved, ran := 0, 0
	c := newTestCollector(t)
	c.rpmResolver = func() (string, error) {
		resolved++
		return "/usr/bin/rpm", nil
	}
	c.rpmRunner = func(ctx context.Context, inv rpmInvocation, stdout, stderr *boundedBuffer) error {
		ran++
		return nil
	}
	for _, sel := range [][]SoftwareInventorySelector{nil, {}} {
		resolved, ran = 0, 0
		snap, err := c.Capture(context.Background(), SoftwareInventoryRequest{
			SnapshotID:   "snap-rpm-targeted-zero",
			SnapshotMode: SnapshotModeTargeted,
			SourceType:   InventorySourceTypeRPM,
			Selectors:    sel,
		})
		if err != nil {
			t.Fatalf("capture: %v", err)
		}
		if resolved != 0 || ran != 0 {
			t.Fatalf("zero-target TARGETED must not resolve/run RPM (resolved=%d ran=%d)", resolved, ran)
		}
		if snap.CaptureStatus != CaptureStatusFailed || snap.ItemCount != 0 {
			t.Fatalf("status=%s count=%d", snap.CaptureStatus, snap.ItemCount)
		}
		if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossCallerRequestInvalid) {
			t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
		}
	}
}

func TestRPMTargetedAllInvalidNeverInvokeRPM(t *testing.T) {
	ran := 0
	c := newTestCollector(t)
	c.rpmResolver = func() (string, error) { return "/usr/bin/rpm", nil }
	c.rpmRunner = func(ctx context.Context, inv rpmInvocation, stdout, stderr *boundedBuffer) error {
		ran++
		return nil
	}
	snap, err := c.Capture(context.Background(), SoftwareInventoryRequest{
		SnapshotID:   "snap-rpm-targeted-all-invalid",
		SnapshotMode: SnapshotModeTargeted,
		SourceType:   InventorySourceTypeRPM,
		Selectors:    []SoftwareInventorySelector{{}, {NativePackageID: ""}},
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if ran != 0 {
		t.Fatal("all-invalid TARGETED must not execute RPM")
	}
	if snap.CaptureStatus != CaptureStatusFailed {
		t.Fatalf("status=%s", snap.CaptureStatus)
	}
	if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossSelectorInvalid) {
		t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
	}
}

func TestRPMTargetedValidPlusInvalidIsPartialQuality(t *testing.T) {
	raw, err := os.ReadFile(fixtureFile(t, "rpm_qa_complete.txt"))
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	c := collectorWithRPMStdout(t, string(raw), nil, nil)
	orig := c.rpmRunner
	c.rpmRunner = func(ctx context.Context, inv rpmInvocation, stdout, stderr *boundedBuffer) error {
		ran++
		return orig(ctx, inv, stdout, stderr)
	}
	snap, err := c.Capture(context.Background(), SoftwareInventoryRequest{
		SnapshotID:   "snap-rpm-targeted-mixed",
		SnapshotMode: SnapshotModeTargeted,
		SourceType:   InventorySourceTypeRPM,
		Selectors: []SoftwareInventorySelector{
			{ProductName: "bash"},
			{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ran == 0 {
		t.Fatal("mixed valid+invalid TARGETED may query RPM after recording request quality")
	}
	if snap.CaptureStatus != CaptureStatusPartial || snap.Quality != inventoryQualityPartial {
		t.Fatalf("status=%s quality=%s reason=%q", snap.CaptureStatus, snap.Quality, snap.IncompleteOrLossReason)
	}
	if snap.ItemCount != 2 {
		t.Fatalf("expected bash duplicates, got %d %#v", snap.ItemCount, nativeIDs(snap))
	}
	if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossSelectorInvalid) {
		t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
	}
}

func TestRPMTypedEVRVersionDifferences(t *testing.T) {
	line := func(epoch, version, release string) string {
		return "openssl\t" + epoch + "\t" + version + "\t" + release + "\tx86_64\tRed Hat, Inc.\t1700000000\n"
	}
	base := collectorWithRPMStdout(t, line("1", "3.0.7", "18.el9"), nil, nil)
	baseSnap, err := base.Capture(context.Background(), fullRPMRequest("snap-rpm-evr-base"))
	if err != nil {
		t.Fatal(err)
	}
	if baseSnap.Items[0].Version != "1:3.0.7-18.el9" || baseSnap.Items[0].NativePackageID != "rpm:x86_64:openssl=1:3.0.7-18.el9" {
		t.Fatalf("base mapping %#v", baseSnap.Items[0])
	}

	cases := []struct {
		id      string
		epoch   string
		version string
		release string
		wantVer string
		wantID  string
	}{
		{id: "release-only", epoch: "1", version: "3.0.7", release: "19.el9", wantVer: "1:3.0.7-19.el9", wantID: "rpm:x86_64:openssl=1:3.0.7-19.el9"},
		{id: "epoch-only", epoch: "2", version: "3.0.7", release: "18.el9", wantVer: "2:3.0.7-18.el9", wantID: "rpm:x86_64:openssl=2:3.0.7-18.el9"},
		{id: "version-only", epoch: "1", version: "3.0.8", release: "18.el9", wantVer: "1:3.0.8-18.el9", wantID: "rpm:x86_64:openssl=1:3.0.8-18.el9"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			c := collectorWithRPMStdout(t, line(tc.epoch, tc.version, tc.release), nil, nil)
			snap, err := c.Capture(context.Background(), fullRPMRequest("snap-rpm-evr-"+tc.id))
			if err != nil {
				t.Fatal(err)
			}
			if snap.ItemCount != 1 {
				t.Fatalf("count=%d", snap.ItemCount)
			}
			if snap.Items[0].Version != tc.wantVer {
				t.Fatalf("Version=%q want %q", snap.Items[0].Version, tc.wantVer)
			}
			if snap.Items[0].NativePackageID != tc.wantID {
				t.Fatalf("NativePackageID=%q want %q", snap.Items[0].NativePackageID, tc.wantID)
			}
			if snap.Items[0].Version == baseSnap.Items[0].Version {
				t.Fatalf("%s difference must change typed Version", tc.id)
			}
			if snap.Items[0].NativePackageID == baseSnap.Items[0].NativePackageID {
				t.Fatalf("%s difference must change NativePackageID", tc.id)
			}
			eq := strings.Index(snap.Items[0].NativePackageID, "=")
			if snap.Items[0].NativePackageID[eq+1:] != snap.Items[0].Version {
				t.Fatal("NativePackageID and typed Version EVR must be consistent")
			}
		})
	}
}

func TestRPMOversizedComposedEVRRejectedNotTruncated(t *testing.T) {
	ver := strings.Repeat("v", 80)
	rel := strings.Repeat("r", 80)
	stdout := "openssl\t1\t" + ver + "\t" + rel + "\tx86_64\tV\t1\ngood\t0\t1\t1.el9\tx86_64\tV\t1\n"
	c := collectorWithRPMStdout(t, stdout, nil, nil)
	snap, err := c.Capture(context.Background(), fullRPMRequest("snap-rpm-evr-oversize"))
	if err != nil {
		t.Fatal(err)
	}
	if snap.CaptureStatus != CaptureStatusPartial {
		t.Fatalf("oversized EVR must be PARTIAL, got %s reason=%q", snap.CaptureStatus, snap.IncompleteOrLossReason)
	}
	if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossRecordOversized) {
		t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
	}
	if snap.ItemCount != 1 || snap.Items[0].ProductName != "good" {
		t.Fatalf("expected only good, got %#v", nativeIDs(snap))
	}
	composed := "1:" + ver + "-" + rel
	if len(composed) <= maxSoftwareInventoryVersionLen {
		t.Fatal("test fixture must exceed version bound")
	}
	for _, item := range snap.Items {
		if len(item.Version) > maxSoftwareInventoryVersionLen {
			t.Fatal("oversized EVR must not be truncated into Version")
		}
		if strings.HasPrefix(item.Version, "1:"+ver[:10]) {
			t.Fatal("oversized EVR must not be emitted")
		}
	}
}

func TestRPMMalformedNameArchDelimitersRejected(t *testing.T) {
	good := "keep\t0\t1\t1.el9\tx86_64\tV\t1\n"
	cases := []struct {
		name string
		line string
	}{
		{name: "name-colon", line: "amd64:openssl\t1\t3.0.7\t18.el9\tx86_64\tV\t1\n"},
		{name: "name-equals", line: "openssl=evil\t1\t3.0.7\t18.el9\tx86_64\tV\t1\n"},
		{name: "name-slash", line: "openssl/evil\t1\t3.0.7\t18.el9\tx86_64\tV\t1\n"},
		{name: "arch-colon", line: "openssl\t1\t3.0.7\t18.el9\tx86_64:evil\tV\t1\n"},
		{name: "arch-equals", line: "openssl\t1\t3.0.7\t18.el9\tx86_64=evil\tV\t1\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := collectorWithRPMStdout(t, good+tc.line, nil, nil)
			snap, err := c.Capture(context.Background(), fullRPMRequest("snap-rpm-id-"+tc.name))
			if err != nil {
				t.Fatal(err)
			}
			if snap.CaptureStatus != CaptureStatusPartial {
				t.Fatalf("status=%s reason=%q", snap.CaptureStatus, snap.IncompleteOrLossReason)
			}
			if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossIdentityInvalid) {
				t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
			}
			if snap.ItemCount != 1 || snap.Items[0].ProductName != "keep" {
				t.Fatalf("got %#v", nativeIDs(snap))
			}
			for _, item := range snap.Items {
				if strings.Contains(item.NativePackageID, "evil") || strings.Contains(item.ProductName, "=") || strings.Contains(item.ProductName, ":") {
					t.Fatalf("ambiguous identity accepted: %#v", item)
				}
			}
		})
	}
}

func TestRPMNativeIdentityCollisionNegative(t *testing.T) {
	stdout := "openssl\t1\t3.0.7\t18.el9\tx86_64\tV\t1\n" +
		"x86_64:openssl\t1\t3.0.7\t18.el9\tx86_64\tV\t1\n" +
		"openssl=1\t1\t3.0.7\t18.el9\tx86_64\tV\t1\n"
	c := collectorWithRPMStdout(t, stdout, nil, nil)
	snap, err := c.Capture(context.Background(), fullRPMRequest("snap-rpm-id-collision"))
	if err != nil {
		t.Fatal(err)
	}
	if snap.CaptureStatus != CaptureStatusPartial || snap.ItemCount != 1 {
		t.Fatalf("status=%s count=%d ids=%#v", snap.CaptureStatus, snap.ItemCount, nativeIDs(snap))
	}
	if snap.Items[0].NativePackageID != "rpm:x86_64:openssl=1:3.0.7-18.el9" || snap.Items[0].Version != "1:3.0.7-18.el9" {
		t.Fatalf("unexpected mapping %#v", snap.Items[0])
	}
}

func TestRPMQueryFormatIsPlatformConstant(t *testing.T) {
	if strings.Contains(rpmQueryFormat, "%{SCRIPT") || strings.Contains(rpmQueryFormat, "%{FILENAMES}") || strings.Contains(rpmQueryFormat, "%{DESCRIPTION}") {
		t.Fatal("query format must not collect scripts, file lists, or descriptions")
	}
	if _, err := os.Stat(filepath.Clean("/usr/bin/rpm")); err == nil {
		t.Log("host has /usr/bin/rpm; live smoke is gated separately")
	}
}
