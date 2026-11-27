//go:build windows

package queue

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsSyncDir(t *testing.T) {
	dir := t.TempDir()
	if err := syncDir(dir); err != nil {
		t.Fatalf("sync owned directory: %v", err)
	}

	// Exercise a path above the legacy MAX_PATH limit without changing the
	// machine's long-path settings.
	longDir := filepath.Join(dir, strings.Repeat("a", 150), strings.Repeat("b", 150))
	if err := os.MkdirAll(longDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := syncDir(longDir); err != nil {
		t.Fatalf("sync long directory path: %v", err)
	}

	missing := filepath.Join(dir, "missing")
	if err := syncDir(missing); !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		t.Fatalf("expected native file-not-found error, got %v", err)
	}

	file := filepath.Join(dir, "regular-file")
	if err := os.WriteFile(file, []byte("record"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := syncDir(file); !errors.Is(err, ErrInvalidDirectory) {
		t.Fatalf("expected regular-file rejection, got %v", err)
	}

	// A directory handle that denies write sharing makes the real GENERIC_WRITE
	// open fail without modifying ACLs or relying on the runner's privileges.
	path, err := syncDirPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	syncErr := syncDir(dir)
	if err := windows.CloseHandle(handle); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(syncErr, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("expected native sharing violation, got %v", syncErr)
	}
	if err := syncDir(dir); err != nil {
		t.Fatalf("sync after releasing the conflicting handle: %v", err)
	}
}

func TestWindowsDurableQueueSyncDirLifecycle(t *testing.T) {
	dir := t.TempDir()
	reopen := func() *DurableQueue {
		t.Helper()
		q, err := NewDurableQueue(dir, 10, 1<<20, 1024)
		if err != nil {
			t.Fatalf("open durable queue: %v", err)
		}
		return q
	}

	q := reopen()
	stream := q.AgentStreamID()
	first, err := q.Enqueue([]byte("acknowledged"))
	if err != nil {
		t.Fatal(err)
	}
	q = reopen()
	commitAcceptedThrough(t, q, first.Sequence)
	firstPath := filepath.Join(q.activeDir, recordFileName(first.Sequence, first.ID))
	if _, err := os.Stat(firstPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ACKed record to be unlinked, stat returned %v", err)
	}

	q = reopen()
	if q.AgentStreamID() != stream || q.acceptedThrough != first.Sequence || len(q.acceptedRecords) != 0 {
		t.Fatal("reopened queue lost stream or ACK state, or retained cleanup authorization")
	}
	second, err := q.Enqueue([]byte("corrupt me"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Sequence != first.Sequence+1 {
		t.Fatalf("expected next sequence %d, got %d", first.Sequence+1, second.Sequence)
	}
	name := recordFileName(second.Sequence, second.ID)
	if err := os.WriteFile(filepath.Join(q.activeDir, name), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}

	batch, err := q.ReadBatch(10, 1<<20)
	if err != nil || len(batch) != 0 {
		t.Fatalf("read corrupt record: batch=%+v, err=%v", batch, err)
	}
	q = reopen()
	if stats := q.Stats(); stats.ActiveCount != 0 || stats.QuarantineCount != 1 {
		t.Fatalf("unexpected state after quarantine and reopen: %+v", stats)
	}
	if _, err := os.Stat(filepath.Join(q.quarantineDir, name)); err != nil {
		t.Fatalf("expected quarantined record after reopen: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, transitionIntentFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected intent to be cleared, stat returned %v", err)
	}
}
