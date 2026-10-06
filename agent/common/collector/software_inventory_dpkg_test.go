package collector

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testNow() time.Time {
	return time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
}

func fixtureFile(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", "software_inventory", name)
}

func newTestCollector(t *testing.T) *LinuxSoftwareInventoryCollector {
	t.Helper()
	c := NewLinuxSoftwareInventoryCollector()
	c.now = testNow
	c.sink = &LocalRecordingStructuredEvidenceSink{}
	return c
}

func collectorWithDPKGFile(t *testing.T, name string) *LinuxSoftwareInventoryCollector {
	t.Helper()
	c := newTestCollector(t)
	c.dpkgPath = fixtureFile(t, name)
	c.openDPKG = c.openDefaultDPKG
	return c
}

func collectorWithDPKGData(t *testing.T, data string) *LinuxSoftwareInventoryCollector {
	t.Helper()
	c := newTestCollector(t)
	c.openDPKG = func(ctx context.Context) (io.ReadCloser, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return io.NopCloser(strings.NewReader(data)), nil
	}
	return c
}

func collectorWithDPKGOpenError(t *testing.T, err error) *LinuxSoftwareInventoryCollector {
	t.Helper()
	c := newTestCollector(t)
	c.openDPKG = func(ctx context.Context) (io.ReadCloser, error) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, err
	}
	return c
}

func fullDPKGRequest(id string) SoftwareInventoryRequest {
	return SoftwareInventoryRequest{
		SnapshotID:   id,
		SnapshotMode: SnapshotModeFull,
		SourceType:   InventorySourceTypeDPKG,
	}
}

func targetedDPKGRequest(id string, selectors ...SoftwareInventorySelector) SoftwareInventoryRequest {
	return SoftwareInventoryRequest{
		SnapshotID:   id,
		SnapshotMode: SnapshotModeTargeted,
		SourceType:   InventorySourceTypeDPKG,
		Selectors:    selectors,
	}
}

func installedStanza(name, arch, version string) string {
	return "Package: " + name + "\nStatus: install ok installed\nArchitecture: " + arch + "\nVersion: " + version + "\n"
}

func TestDPKGCompleteFullValidFixture(t *testing.T) {
	c := collectorWithDPKGFile(t, "dpkg_status_complete")
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-full-001"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if err := snap.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusComplete || snap.Truncated {
		t.Fatalf("expected COMPLETE non-truncated, got status=%s truncated=%v reason=%q", snap.CaptureStatus, snap.Truncated, snap.IncompleteOrLossReason)
	}
	if snap.ItemCount != 4 {
		t.Fatalf("expected 4 installed items, got %d: %#v", snap.ItemCount, nativeIDs(snap))
	}
	if snap.RemovalInferencePermitted() {
		t.Fatal("BUILD-011 must not permit removal inference")
	}
	if snap.SourceType != InventorySourceTypeDPKG || snap.SourceMode != InventorySourceModeAgent {
		t.Fatal("source metadata mismatch")
	}
	if snap.InventoryScope != linuxDPKGInventoryScope {
		t.Fatalf("inventory_scope=%q", snap.InventoryScope)
	}
}

func TestDPKGOnlyInstallOkInstalledEmitted(t *testing.T) {
	c := collectorWithDPKGFile(t, "dpkg_status_complete")
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-status-filter"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	forbidden := []string{"leftover-config", "uninstalled-pkg", "purged-pkg", "unpacked-pkg", "hold-pkg"}
	for _, item := range snap.Items {
		for _, name := range forbidden {
			if item.ProductName == name {
				t.Fatalf("non-installed status %q was emitted", name)
			}
		}
		if item.InstallationScope != InstallationScopeMachine {
			t.Fatal("dpkg installation scope must be MACHINE")
		}
		if item.AuthoritativeInstallOrUpdateAt != nil {
			t.Fatal("dpkg must not fabricate install/update time")
		}
		if item.NormalizedProductID != "" {
			t.Fatal("Slice 4 must not populate NormalizedProductID")
		}
	}
}

func TestDPKGConfigFilesAndUninstalledExcluded(t *testing.T) {
	data := installedStanza("keep", "amd64", "1") + "\n" +
		"Package: cfg\nStatus: deinstall ok config-files\nArchitecture: amd64\nVersion: 1\n\n" +
		"Package: gone\nStatus: not-installed\nArchitecture: amd64\nVersion: 1\n"
	c := collectorWithDPKGData(t, data)
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-exclude"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.ItemCount != 1 || snap.Items[0].ProductName != "keep" {
		t.Fatalf("expected only keep, got %#v", nativeIDs(snap))
	}
}

func TestDPKGNativeIdentityExactnessAndMultiarch(t *testing.T) {
	c := collectorWithDPKGFile(t, "dpkg_status_complete")
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-identity"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	want := map[string]SoftwareInventoryItem{
		"dpkg:amd64:openssl=3.0.13-0ubuntu3": {ProductName: "openssl", Version: "3.0.13-0ubuntu3", Architecture: "amd64"},
		"dpkg:i386:openssl=3.0.13-0ubuntu3":  {ProductName: "openssl", Version: "3.0.13-0ubuntu3", Architecture: "i386"},
		"dpkg:amd64:bash=5.1-6ubuntu1":       {ProductName: "bash", Version: "5.1-6ubuntu1", Architecture: "amd64"},
	}
	seen := map[string]int{}
	for _, item := range snap.Items {
		seen[item.NativePackageID]++
		exp, ok := want[item.NativePackageID]
		if !ok {
			t.Fatalf("unexpected identity %q", item.NativePackageID)
		}
		if item.ProductName != exp.ProductName || item.Version != exp.Version || item.Architecture != exp.Architecture {
			t.Fatalf("identity mapping mismatch for %q: %#v", item.NativePackageID, item)
		}
	}
	if seen["dpkg:amd64:openssl=3.0.13-0ubuntu3"] != 2 {
		t.Fatalf("duplicate native identities must be preserved, got %d", seen["dpkg:amd64:openssl=3.0.13-0ubuntu3"])
	}
	if seen["dpkg:i386:openssl=3.0.13-0ubuntu3"] != 1 {
		t.Fatal("multiarch packages must remain distinct")
	}
}

func TestDPKGOptionalMaintainerAndIgnoredFields(t *testing.T) {
	c := collectorWithDPKGFile(t, "dpkg_status_complete")
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-fields"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	foundVendor := false
	for _, item := range snap.Items {
		if strings.Contains(strings.ToLower(item.ProductName), "depends") ||
			strings.Contains(strings.ToLower(item.Version), "libc6") ||
			strings.Contains(item.VendorPublisher, "Secure Sockets") ||
			strings.Contains(item.NativePackageID, "Conffiles") {
			t.Fatalf("ignored dpkg fields leaked into inventory: %#v", item)
		}
		if item.ProductName == "openssl" && item.Architecture == "amd64" {
			if item.VendorPublisher != "Ubuntu Developers <ubuntu-devel-discuss@lists.ubuntu.com>" {
				t.Fatalf("maintainer mapping: %q", item.VendorPublisher)
			}
			foundVendor = true
		}
	}
	if !foundVendor {
		t.Fatal("expected openssl maintainer mapping")
	}
}

func TestDPKGMalformedAndTruncatedStanzaPartial(t *testing.T) {
	c := collectorWithDPKGFile(t, "dpkg_status_malformed")
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-malformed"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusPartial {
		t.Fatalf("mixed valid+malformed must be PARTIAL, got %s reason=%q", snap.CaptureStatus, snap.IncompleteOrLossReason)
	}
	if snap.ItemCount != 2 {
		t.Fatalf("expected 2 usable items, got %d", snap.ItemCount)
	}
	if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossRecordMalformed) {
		t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
	}

	c = collectorWithDPKGFile(t, "dpkg_status_truncated_stanza")
	snap, err = c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-trunc-stanza"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusFailed && snap.CaptureStatus != CaptureStatusPartial {
		t.Fatalf("truncated last stanza must not be COMPLETE, got %s", snap.CaptureStatus)
	}
	if snap.CaptureStatus == CaptureStatusComplete {
		t.Fatal("truncated last stanza COMPLETE forbidden")
	}
	if strings.TrimSpace(snap.IncompleteOrLossReason) == "" {
		t.Fatal("truncated last stanza requires categorical reason")
	}
}

func TestDPKGOversizedStanzaAndField(t *testing.T) {
	c := collectorWithDPKGData(t, installedStanza("ok", "amd64", "1")+"\nPackage: big\nStatus: install ok installed\nArchitecture: amd64\nVersion: 1\n"+strings.Repeat("X: "+strings.Repeat("y", 80)+"\n", 20)+"\n"+installedStanza("tail", "amd64", "1"))
	c.bounds.MaxDpkgStanzaBytes = 200
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-oversize-stanza"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusPartial {
		t.Fatalf("oversized stanza must be PARTIAL, got %s", snap.CaptureStatus)
	}

	c = collectorWithDPKGData(t, "Package: "+strings.Repeat("p", maxSoftwareInventoryProductNameLen+8)+"\nStatus: install ok installed\nArchitecture: amd64\nVersion: 1.0\n")
	snap, err = c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-oversize-field"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusFailed && snap.CaptureStatus != CaptureStatusPartial {
		t.Fatalf("oversized identity must not COMPLETE, got %s", snap.CaptureStatus)
	}
	if snap.ItemCount != 0 {
		t.Fatal("oversized authoritative identity must be skipped, not truncated into another identity")
	}
}

func TestDPKGControlCharactersSkipped(t *testing.T) {
	data := "Package: bad\x00pkg\nStatus: install ok installed\nArchitecture: amd64\nVersion: 1\n\n" + installedStanza("good", "amd64", "1")
	c := collectorWithDPKGData(t, data)
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-control"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusPartial {
		t.Fatalf("control characters must yield PARTIAL, got %s", snap.CaptureStatus)
	}
	if snap.ItemCount != 1 || snap.Items[0].ProductName != "good" {
		t.Fatalf("expected only good, got %#v", nativeIDs(snap))
	}
}

func TestDPKGMaxItemCountAndInputByteCeiling(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 5; i++ {
		b.WriteString(installedStanza("pkg"+intString(i), "amd64", "1"))
		b.WriteByte('\n')
	}
	c := collectorWithDPKGData(t, b.String())
	c.bounds.MaxItems = 3
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-item-bound"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusPartial || !snap.Truncated || snap.ItemCount != 3 {
		t.Fatalf("item bound: status=%s truncated=%v count=%d", snap.CaptureStatus, snap.Truncated, snap.ItemCount)
	}
	if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossItemBoundReached) {
		t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
	}

	c = collectorWithDPKGData(t, strings.Repeat(installedStanza("x", "amd64", "1")+"\n", 8))
	c.bounds.MaxDpkgBytes = 40
	snap, err = c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-byte-ceiling"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus == CaptureStatusComplete {
		t.Fatal("input byte ceiling must not COMPLETE")
	}
	if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossSourceTruncated) && !snap.Truncated {
		t.Fatalf("expected truncation evidence, got status=%s reason=%q", snap.CaptureStatus, snap.IncompleteOrLossReason)
	}
}

func TestDPKGTargetedExactMatchAndAbsent(t *testing.T) {
	c := collectorWithDPKGFile(t, "dpkg_status_complete")
	snap, err := c.Capture(context.Background(), targetedDPKGRequest("snap-dpkg-targeted", SoftwareInventorySelector{ProductName: "bash"}))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.SnapshotMode != SnapshotModeTargeted || snap.ItemCount != 1 || snap.Items[0].ProductName != "bash" {
		t.Fatalf("targeted bash mismatch: %#v", nativeIDs(snap))
	}
	if snap.RemovalInferencePermitted() {
		t.Fatal("TARGETED must not permit removal inference")
	}

	snap, err = c.Capture(context.Background(), targetedDPKGRequest("snap-dpkg-absent", SoftwareInventorySelector{ProductName: "definitely-not-installed"}))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusComplete {
		t.Fatalf("absent TARGETED package is not source failure, got %s", snap.CaptureStatus)
	}
	if snap.ItemCount != 0 {
		t.Fatal("absent TARGETED package must not invent an item")
	}
	if strings.Contains(snap.IncompleteOrLossReason, "SOFTWARE_REMOVED") {
		t.Fatal("absent TARGETED package must not imply SOFTWARE_REMOVED")
	}
}

func TestDPKGTargetedDoesNotImplyUnrelatedRemoval(t *testing.T) {
	c := collectorWithDPKGFile(t, "dpkg_status_complete")
	snap, err := c.Capture(context.Background(), targetedDPKGRequest("snap-dpkg-targeted-scope", SoftwareInventorySelector{ProductName: "bash"}))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	for _, item := range snap.Items {
		if item.ProductName == "openssl" {
			t.Fatal("unrequested packages must not appear in TARGETED results")
		}
	}
	if hasSoftwareChangeToken(snap) {
		t.Fatal("TARGETED capture must not generate SOFTWARE_* tokens")
	}
}

func TestDPKGCallerCancellationAndDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := collectorWithDPKGFile(t, "dpkg_status_complete")
	c.openDPKG = func(ctx context.Context) (io.ReadCloser, error) {
		cancel()
		return io.NopCloser(strings.NewReader(installedStanza("x", "amd64", "1"))), nil
	}
	_, err := c.Capture(ctx, fullDPKGRequest("snap-dpkg-cancel"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	deadline, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	_, err = collectorWithDPKGFile(t, "dpkg_status_complete").Capture(deadline, fullDPKGRequest("snap-dpkg-deadline"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestDPKGPermissionDeniedAndSourceMissing(t *testing.T) {
	snap, err := collectorWithDPKGOpenError(t, os.ErrPermission).Capture(context.Background(), fullDPKGRequest("snap-dpkg-eacces"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusFailed || snap.ItemCount != 0 {
		t.Fatalf("permission denied must be FAILED empty, got %s count=%d", snap.CaptureStatus, snap.ItemCount)
	}
	if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossSourcePermissionDenied) {
		t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
	}

	snap, err = collectorWithDPKGOpenError(t, os.ErrNotExist).Capture(context.Background(), fullDPKGRequest("snap-dpkg-missing"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusFailed || snap.CaptureStatus == CaptureStatusComplete {
		t.Fatalf("missing source must be FAILED, got %s", snap.CaptureStatus)
	}
	if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossSourceMissing) {
		t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
	}
}

func TestDPKGSourceFailureNeverCompleteEmpty(t *testing.T) {
	snap, err := collectorWithDPKGOpenError(t, os.ErrPermission).Capture(context.Background(), fullDPKGRequest("snap-dpkg-no-complete-empty"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus == CaptureStatusComplete {
		t.Fatal("source failure must never emit COMPLETE empty")
	}
	if snap.ItemCount == 0 && snap.CaptureStatus != CaptureStatusFailed {
		t.Fatalf("empty failed source must be FAILED, got %s", snap.CaptureStatus)
	}
}

func TestDPKGTargetedNilAndEmptySelectorsFailWithoutSource(t *testing.T) {
	opened := 0
	c := newTestCollector(t)
	c.openDPKG = func(ctx context.Context) (io.ReadCloser, error) {
		opened++
		return io.NopCloser(strings.NewReader(installedStanza("bash", "amd64", "1"))), nil
	}
	cases := []struct {
		name string
		sel  []SoftwareInventorySelector
	}{
		{name: "nil", sel: nil},
		{name: "empty", sel: []SoftwareInventorySelector{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opened = 0
			snap, err := c.Capture(context.Background(), SoftwareInventoryRequest{
				SnapshotID:   "snap-dpkg-targeted-zero-" + tc.name,
				SnapshotMode: SnapshotModeTargeted,
				SourceType:   InventorySourceTypeDPKG,
				Selectors:    tc.sel,
			})
			if err != nil {
				t.Fatalf("capture: %v", err)
			}
			if opened != 0 {
				t.Fatal("invalid zero-target TARGETED request must not open DPKG")
			}
			if snap.CaptureStatus != CaptureStatusFailed || snap.ItemCount != 0 {
				t.Fatalf("expected FAILED empty, got status=%s count=%d", snap.CaptureStatus, snap.ItemCount)
			}
			if snap.Quality != inventoryQualityFailed {
				t.Fatalf("quality=%q", snap.Quality)
			}
			if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossCallerRequestInvalid) {
				t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
			}
		})
	}
}

func TestDPKGTargetedAllInvalidSelectorsFailWithoutSource(t *testing.T) {
	opened := 0
	c := newTestCollector(t)
	c.openDPKG = func(ctx context.Context) (io.ReadCloser, error) {
		opened++
		return io.NopCloser(strings.NewReader(installedStanza("bash", "amd64", "1"))), nil
	}
	snap, err := c.Capture(context.Background(), targetedDPKGRequest(
		"snap-dpkg-targeted-all-invalid",
		SoftwareInventorySelector{},
		SoftwareInventorySelector{ProductName: " \t "},
	))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if opened != 0 {
		t.Fatal("all-invalid TARGETED request must not open DPKG")
	}
	if snap.CaptureStatus != CaptureStatusFailed || snap.ItemCount != 0 {
		t.Fatalf("expected FAILED empty, got status=%s count=%d", snap.CaptureStatus, snap.ItemCount)
	}
	if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossSelectorInvalid) {
		t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
	}
}

func TestDPKGTargetedValidPlusInvalidIsPartialQuality(t *testing.T) {
	opened := 0
	c := collectorWithDPKGFile(t, "dpkg_status_complete")
	orig := c.openDPKG
	c.openDPKG = func(ctx context.Context) (io.ReadCloser, error) {
		opened++
		return orig(ctx)
	}
	snap, err := c.Capture(context.Background(), targetedDPKGRequest(
		"snap-dpkg-targeted-mixed",
		SoftwareInventorySelector{ProductName: "bash"},
		SoftwareInventorySelector{},
	))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if opened == 0 {
		t.Fatal("mixed valid+invalid TARGETED may enumerate source after request-quality recording")
	}
	if snap.CaptureStatus != CaptureStatusPartial || snap.Quality != inventoryQualityPartial {
		t.Fatalf("mixed selectors must be PARTIAL, got status=%s quality=%s reason=%q", snap.CaptureStatus, snap.Quality, snap.IncompleteOrLossReason)
	}
	if snap.ItemCount != 1 || snap.Items[0].ProductName != "bash" {
		t.Fatalf("expected bash only, got %#v", nativeIDs(snap))
	}
	if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossSelectorInvalid) {
		t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
	}
}

func TestDPKGFullDoesNotRequireSelectors(t *testing.T) {
	c := collectorWithDPKGFile(t, "dpkg_status_complete")
	snap, err := c.Capture(context.Background(), SoftwareInventoryRequest{
		SnapshotID:   "snap-dpkg-full-no-sel",
		SnapshotMode: SnapshotModeFull,
		SourceType:   InventorySourceTypeDPKG,
		Selectors:    nil,
	})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusComplete || snap.ItemCount <= 0 {
		t.Fatalf("FULL without selectors must capture, got status=%s count=%d", snap.CaptureStatus, snap.ItemCount)
	}
}

func TestDPKGDuplicateAuthoritativeFieldsAreMalformed(t *testing.T) {
	good := installedStanza("keep", "amd64", "1.0")
	cases := []struct {
		name string
		body string
	}{
		{name: "duplicate-package", body: "Package: first\nPackage: second\nStatus: install ok installed\nArchitecture: amd64\nVersion: 9.9.9\n"},
		{name: "duplicate-status", body: "Package: overwritten-status\nStatus: not-installed\nStatus: install ok installed\nArchitecture: amd64\nVersion: 1.0\n"},
		{name: "duplicate-version", body: "Package: overwritten-version\nStatus: install ok installed\nArchitecture: amd64\nVersion: 1.0\nVersion: 9.9.9\n"},
		{name: "duplicate-architecture", body: "Package: overwritten-arch\nStatus: install ok installed\nArchitecture: i386\nArchitecture: amd64\nVersion: 1.0\n"},
		{name: "duplicate-maintainer", body: "Package: overwritten-maint\nStatus: install ok installed\nArchitecture: amd64\nVersion: 1.0\nMaintainer: First <a@b>\nMaintainer: Last <c@d>\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := collectorWithDPKGData(t, good+"\n"+tc.body)
			snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-"+tc.name))
			if err != nil {
				t.Fatalf("capture: %v", err)
			}
			if snap.CaptureStatus != CaptureStatusPartial {
				t.Fatalf("duplicate fields must be PARTIAL, got %s reason=%q", snap.CaptureStatus, snap.IncompleteOrLossReason)
			}
			if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossRecordMalformed) {
				t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
			}
			if snap.ItemCount != 1 || snap.Items[0].ProductName != "keep" {
				t.Fatalf("duplicate stanza must not last-wins overwrite identity, got %#v", snap.Items)
			}
			for _, item := range snap.Items {
				if item.ProductName == "second" || item.ProductName == "first" ||
					item.ProductName == "overwritten-status" || item.ProductName == "overwritten-version" ||
					item.ProductName == "overwritten-arch" || item.ProductName == "overwritten-maint" ||
					item.Version == "9.9.9" || item.Architecture == "i386" ||
					strings.Contains(item.VendorPublisher, "Last") {
					t.Fatalf("silently overwritten identity emitted: %#v", item)
				}
			}
		})
		t.Run(tc.name+"-only", func(t *testing.T) {
			c := collectorWithDPKGData(t, tc.body)
			snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-only-"+tc.name))
			if err != nil {
				t.Fatalf("capture: %v", err)
			}
			if snap.CaptureStatus != CaptureStatusFailed || snap.ItemCount != 0 {
				t.Fatalf("duplicate-only source must FAIL empty, got status=%s count=%d items=%#v", snap.CaptureStatus, snap.ItemCount, snap.Items)
			}
			if snap.Quality == inventoryQualityAuthoritative {
				t.Fatal("duplicate authoritative fields must never be COMPLETE authoritative")
			}
		})
	}
}

func TestDPKGMalformedIdentityDelimitersRejected(t *testing.T) {
	good := installedStanza("keep", "amd64", "1.0")
	cases := []struct {
		name string
		body string
	}{
		{name: "package-equals", body: installedStanza("openssl=evil", "amd64", "1.0")},
		{name: "package-colon", body: installedStanza("amd64:openssl", "amd64", "1.0")},
		{name: "package-slash", body: installedStanza("openssl/evil", "amd64", "1.0")},
		{name: "arch-colon", body: installedStanza("openssl", "amd64:all", "1.0")},
		{name: "arch-equals", body: installedStanza("openssl", "amd64=all", "1.0")},
		{name: "arch-uppercase", body: installedStanza("openssl", "AMD64", "1.0")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := collectorWithDPKGData(t, good+"\n"+tc.body)
			snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-id-"+tc.name))
			if err != nil {
				t.Fatalf("capture: %v", err)
			}
			if snap.CaptureStatus != CaptureStatusPartial {
				t.Fatalf("malformed identity must be PARTIAL, got %s reason=%q", snap.CaptureStatus, snap.IncompleteOrLossReason)
			}
			if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossIdentityInvalid) {
				t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
			}
			if snap.ItemCount != 1 || snap.Items[0].ProductName != "keep" {
				t.Fatalf("expected only keep, got %#v", nativeIDs(snap))
			}
			for _, item := range snap.Items {
				if strings.Contains(item.NativePackageID, "evil") ||
					strings.Contains(item.ProductName, ":") ||
					strings.Contains(item.ProductName, "=") ||
					strings.Contains(item.Architecture, ":") ||
					strings.Contains(item.Architecture, "=") {
					t.Fatalf("malformed identity accepted as valid-looking native ID: %#v", item)
				}
			}
		})
	}
}

func TestDPKGPackageNameMinimumLength(t *testing.T) {
	good := installedStanza("openssl", "amd64", "3.0.13")
	cases := []struct {
		name    string
		pkg     string
		accept  bool
	}{
		{name: "single-letter", pkg: "a", accept: false},
		{name: "single-digit", pkg: "1", accept: false},
		{name: "two-letter", pkg: "ab", accept: true},
		{name: "letter-digit", pkg: "a1", accept: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDPKGNativeSafePackageName(tc.pkg); got != tc.accept {
				t.Fatalf("isDPKGNativeSafePackageName(%q)=%v want %v", tc.pkg, got, tc.accept)
			}
			c := collectorWithDPKGData(t, good+"\n"+installedStanza(tc.pkg, "amd64", "1.0"))
			snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-namelen-"+tc.name))
			if err != nil {
				t.Fatalf("capture: %v", err)
			}
			ids := nativeIDs(snap)
			if tc.accept {
				if snap.ItemCount != 2 {
					t.Fatalf("expected openssl + %s, got %#v", tc.pkg, ids)
				}
				return
			}
			if snap.CaptureStatus != CaptureStatusPartial {
				t.Fatalf("short name must be PARTIAL, got %s reason=%q", snap.CaptureStatus, snap.IncompleteOrLossReason)
			}
			if !strings.Contains(snap.IncompleteOrLossReason, inventoryLossIdentityInvalid) {
				t.Fatalf("reason=%q", snap.IncompleteOrLossReason)
			}
			if snap.ItemCount != 1 || snap.Items[0].ProductName != "openssl" {
				t.Fatalf("expected only openssl, got %#v", ids)
			}
		})
	}
	for _, pkg := range []string{"bash", "libc6", "coreutils", "openssl"} {
		if !isDPKGNativeSafePackageName(pkg) {
			t.Fatalf("ordinary package name %q must remain accepted", pkg)
		}
	}
}

func TestDPKGNativeIdentityCollisionNegative(t *testing.T) {
	data := installedStanza("openssl", "amd64", "3.0.13") + "\n" +
		installedStanza("amd64:openssl", "amd64", "3.0.13") + "\n" +
		installedStanza("openssl=3.0.13", "amd64", "forged")
	c := collectorWithDPKGData(t, data)
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-id-collision"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.CaptureStatus != CaptureStatusPartial {
		t.Fatalf("hostile identities must not COMPLETE, got %s", snap.CaptureStatus)
	}
	if snap.ItemCount != 1 || snap.Items[0].NativePackageID != "dpkg:amd64:openssl=3.0.13" {
		t.Fatalf("expected only unambiguous openssl identity, got %#v", nativeIDs(snap))
	}
	if snap.Items[0].Version != "3.0.13" {
		t.Fatalf("dpkg version must remain native, got %q", snap.Items[0].Version)
	}
}

func TestDPKGVersionRemainsNativeSourceValue(t *testing.T) {
	data := installedStanza("openssl", "amd64", "1:3.0.13-0ubuntu3")
	c := collectorWithDPKGData(t, data)
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-dpkg-native-version"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if snap.ItemCount != 1 {
		t.Fatalf("count=%d", snap.ItemCount)
	}
	if snap.Items[0].Version != "1:3.0.13-0ubuntu3" {
		t.Fatalf("dpkg version rewritten: %q", snap.Items[0].Version)
	}
	if snap.Items[0].NativePackageID != "dpkg:amd64:openssl=1:3.0.13-0ubuntu3" {
		t.Fatalf("native id=%q", snap.Items[0].NativePackageID)
	}
}

func TestDPKGNoSoftwareChangeGeneration(t *testing.T) {
	snap, err := collectorWithDPKGFile(t, "dpkg_status_complete").Capture(context.Background(), fullDPKGRequest("snap-dpkg-no-change"))
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if hasSoftwareChangeToken(snap) {
		t.Fatal("BUILD-011 must not generate SOFTWARE_* change evidence")
	}
}

func nativeIDs(snap SoftwareInventorySnapshot) []string {
	out := make([]string, 0, len(snap.Items))
	for _, item := range snap.Items {
		out = append(out, item.NativePackageID)
	}
	return out
}

func hasSoftwareChangeToken(snap SoftwareInventorySnapshot) bool {
	blob := string(snap.CaptureStatus) + snap.Quality + snap.IncompleteOrLossReason + snap.SourceProvenance
	for _, item := range snap.Items {
		blob += item.NativePackageID + item.ProductName + item.Version + item.VendorPublisher
	}
	for _, tok := range []string{"SOFTWARE_INSTALLED", "SOFTWARE_REMOVED", "SOFTWARE_VERSION_CHANGED"} {
		if strings.Contains(blob, tok) {
			return true
		}
	}
	return false
}
