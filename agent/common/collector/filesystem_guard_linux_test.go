//go:build linux

package collector

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestFilesystemRemoteSourcesExcludedByDefault(t *testing.T) {
	for _, fs := range []string{"nfs", "nfs4", "cifs", "smb3", "9p", "ceph", "afs", "autofs", "fuse.sshfs", "fuse.glusterfs"} {
		if !ignoredFSTypes[fs] {
			t.Errorf("remote source %s can block default statfs", fs)
		}
	}
}

func TestFilesystemBlockedWorkerOwnershipAndGlobalBound(t *testing.T) {
	gate := make(chan struct{})
	var calls atomic.Int32
	stat := func(string, *syscall.Statfs_t) error { calls.Add(1); <-gate; return nil }
	c, other := NewFilesystemCollector(4), NewFilesystemCollector(4)
	c.statfs, other.statfs = stat, stat
	defer func() {
		close(gate)
		deadline := time.Now().Add(time.Second)
		for len(filesystemWorkers) != 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if len(filesystemWorkers) != 0 {
			t.Error("worker ownership not released after actual completion")
		}
	}()
	if _, reason := c.statMount(context.Background(), "/blocked"); reason != "timeout" {
		t.Fatal(reason)
	}
	for i := 0; i < 100; i++ {
		if _, reason := c.statMount(context.Background(), "/blocked"); reason != "outstanding" {
			t.Fatal(reason)
		}
	}
	if _, reason := other.statMount(context.Background(), "/another"); reason != "timeout" {
		t.Fatal(reason)
	}
	if _, reason := c.statMount(context.Background(), "/third"); reason != "resource_guarded" {
		t.Fatal(reason)
	}
	if calls.Load() != 2 {
		t.Fatalf("unbounded replacement calls: %d", calls.Load())
	}
}

func TestFilesystemMissingEvidenceDoesNotBecomeZeroCapacity(t *testing.T) {
	c := NewFilesystemCollector(3)
	c.openMounts = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("dev /bad ext4 rw 0 0\ndev /good ext4 rw 0 0\nremote /remote nfs rw 0 0\n")), nil
	}
	c.statfs = func(path string, out *syscall.Statfs_t) error {
		if path == "/remote" {
			t.Error("remote filesystem probed")
		}
		if path == "/bad" {
			return errors.New("private path")
		}
		*out = syscall.Statfs_t{Bsize: 4096, Blocks: 100, Bfree: 50, Bavail: 40, Files: 100, Ffree: 80}
		return nil
	}
	samples, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	missing, good := false, false
	for _, s := range samples {
		if s.Labels["mount"] == "/bad" {
			if s.MetricKey != "system.filesystem.available_evidence" || s.Value != 0 || s.Labels["reason"] != "read_failed" {
				t.Fatalf("fabricated failure evidence: %v", s)
			}
			missing = true
		}
		if s.Labels["mount"] == "/good" && s.MetricKey == "system.filesystem.total" && s.Value == 409600 {
			good = true
		}
	}
	if !missing || !good {
		t.Fatalf("missing=%v good=%v", missing, good)
	}
}

func TestFilesystemCancellationStartsNoWorker(t *testing.T) {
	c := NewFilesystemCollector(1)
	c.statfs = func(string, *syscall.Statfs_t) error { t.Error("cancelled operation started"); return nil }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, reason := c.statMount(ctx, "/"); reason != "cancelled" {
		t.Fatal(reason)
	}
}
