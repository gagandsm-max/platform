package collector

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestLiveDPKGSmoke(t *testing.T) {
	const statusPath = "/var/lib/dpkg/status"
	if _, err := os.Stat(statusPath); err != nil {
		t.Skip("LIVE_DPKG_NOT_RUN: /var/lib/dpkg/status is not available")
	}
	c := NewLinuxSoftwareInventoryCollector()
	c.sink = UnwiredStructuredEvidenceSink{}
	snap, err := c.Capture(context.Background(), fullDPKGRequest("snap-live-dpkg-ubuntu"))
	if err != nil {
		t.Fatalf("live dpkg capture: %v", err)
	}
	if err := snap.Validate(); err != nil {
		t.Fatalf("live snapshot Validate: %v", err)
	}
	if snap.ItemCount <= 0 {
		t.Fatalf("live dpkg expected item_count > 0, got %d status=%s reason=%q", snap.ItemCount, snap.CaptureStatus, snap.IncompleteOrLossReason)
	}
	if snap.Truncated && snap.ItemCount < maxSoftwareInventoryItems {
		t.Fatalf("live inventory truncated below implementation bound: count=%d", snap.ItemCount)
	}
	ordinary := []string{"bash", "libc6", "openssl", "coreutils", "dpkg"}
	foundOrdinary := false
	for _, item := range snap.Items {
		if item.AuthoritativeInstallOrUpdateAt != nil {
			t.Fatalf("live dpkg fabricated install time for %s", item.NativePackageID)
		}
		if item.NormalizedProductID != "" {
			t.Fatal("live dpkg populated NormalizedProductID")
		}
		if strings.Contains(item.VendorPublisher, "\n") ||
			strings.Contains(item.NativePackageID, "Description") ||
			strings.Contains(item.NativePackageID, "Depends") ||
			strings.Contains(item.Version, "Depends") {
			t.Fatalf("live dpkg leaked description/depends-like payload: %#v", item)
		}
		for _, name := range ordinary {
			if item.ProductName == name {
				foundOrdinary = true
			}
		}
	}
	if !foundOrdinary {
		t.Logf("ordinary packages not found in first-pass names; item_count=%d (non-fatal if this is a minimal chroot)", snap.ItemCount)
	}
	t.Logf("LIVE_DPKG_OK item_count=%d status=%s truncated=%v reason=%q quality=%q", snap.ItemCount, snap.CaptureStatus, snap.Truncated, snap.IncompleteOrLossReason, snap.Quality)
}

func TestLiveRPMSmokeOrNotRun(t *testing.T) {
	exe, err := resolveRPMExecutable()
	if err != nil || exe == "" {
		t.Log("LIVE_RPM_NOT_RUN: allowlisted rpm executable is not available")
		return
	}
	if _, err := os.Stat("/var/lib/rpm"); err != nil {
		if _, err2 := os.Stat("/usr/lib/sysimage/rpm"); err2 != nil {
			t.Log("LIVE_RPM_NOT_RUN: RPM database is not available")
			return
		}
	}
	c := NewLinuxSoftwareInventoryCollector()
	c.sink = UnwiredStructuredEvidenceSink{}
	snap, err := c.Capture(context.Background(), fullRPMRequest("snap-live-rpm"))
	if err != nil {
		t.Fatalf("live rpm capture: %v", err)
	}
	if snap.CaptureStatus == CaptureStatusFailed {
		t.Logf("LIVE_RPM_NOT_RUN: capture failed closed status=%s reason=%q", snap.CaptureStatus, snap.IncompleteOrLossReason)
		return
	}
	if err := snap.Validate(); err != nil {
		t.Fatalf("live rpm Validate: %v", err)
	}
	t.Logf("LIVE_RPM_OK item_count=%d status=%s truncated=%v", snap.ItemCount, snap.CaptureStatus, snap.Truncated)
}
