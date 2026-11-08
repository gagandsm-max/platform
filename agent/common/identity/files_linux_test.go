//go:build linux

package identity

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
)

type identityDirectoryOwner struct {
	os.FileInfo
	uid uint32
}

func (d identityDirectoryOwner) Sys() any {
	stat := *d.FileInfo.Sys().(*syscall.Stat_t)
	stat.Uid = d.uid
	return &stat
}

func TestIdentityDirectoryRejectsForeignOwner(t *testing.T) {
	info, err := os.Lstat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, uid := range []uint32{0, uint32(os.Geteuid()), ^uint32(0)} {
		want := uid == 0 || uid == uint32(os.Geteuid())
		if got := trustedIdentityDirectory(identityDirectoryOwner{info, uid}); got != want {
			t.Fatalf("directory owner %d trusted=%v, want %v", uid, got, want)
		}
	}
}

func TestIdentitySyncIncludesDirectoryAncestors(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "state")
	var got, want []string
	_, err := loadOrGenerate(dir, "a", "t", func(path string) error {
		got = append(got, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for path := dir; ; path = filepath.Dir(path) {
		want = append(want, path)
		if path == filepath.Dir(path) {
			break
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("directory publication syncs = %v, want %v", got, want)
	}
}

func TestIdentityParentSyncFailureRetainsPublishedIdentity(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "state")
	want := errors.New("parent sync uncertainty")
	var calls []string
	id, err := loadOrGenerate(dir, "", "t", func(path string) error {
		calls = append(calls, path)
		if path == filepath.Dir(dir) {
			return want
		}
		return nil
	})
	if id != nil || !errors.Is(err, want) || len(calls) != 2 {
		t.Fatalf("parent uncertainty did not stop publication success: id=%v err=%v calls=%v", id, err, calls)
	}
	before, err := os.ReadFile(filepath.Join(dir, identityFileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrGenerate(dir, "", "t"); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(dir, identityFileName))
	if err != nil || string(before) != string(after) {
		t.Fatalf("parent uncertainty recovery replaced identity: %v", err)
	}
}

func TestIdentityRejectsWritableStateDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrGenerate(dir, "a", "t"); err == nil {
		t.Fatal("writable state directory accepted")
	}
	if _, err := os.Lstat(filepath.Join(dir, identityFileName)); !os.IsNotExist(err) {
		t.Fatal("unsafe state directory was written")
	}
}

func TestConcurrentIdentityInitializationNeverReplacesWinner(t *testing.T) {
	dir := t.TempDir()
	start := make(chan struct{})
	ids := make(chan string, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			id, err := LoadOrGenerate(dir, "", "t")
			if err == nil {
				ids <- id.AgentID
			}
		}()
	}
	close(start)
	wg.Wait()
	close(ids)
	winner, err := LoadOrGenerate(dir, "", "t")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for id := range ids {
		count++
		if id != winner.AgentID {
			t.Fatalf("successful identity changed: %s != %s", id, winner.AgentID)
		}
	}
	if count == 0 {
		t.Fatal("no initializer succeeded")
	}
}

func TestIdentityRejectsHardlink(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadOrGenerate(dir, "a", "t"); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(filepath.Join(dir, identityFileName), filepath.Join(t.TempDir(), "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrGenerate(dir, "a", "t"); err == nil {
		t.Fatal("hardlinked identity accepted")
	}
}

func TestIdentityOpenDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("/etc/passwd", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if f, err := openIdentityFile(filepath.Join(dir, "link")); err == nil {
		f.Close()
		t.Fatal("followed symlink")
	}
}

func TestIdentityRejectsSymlinkAncestorWithoutWritingTarget(t *testing.T) {
	dir, target := t.TempDir(), t.TempDir()
	if err := os.Symlink(target, filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrGenerate(filepath.Join(dir, "alias", "state"), "a", "t"); err == nil {
		t.Fatal("symlink ancestor accepted")
	}
	if _, err := os.Stat(filepath.Join(target, "state")); !os.IsNotExist(err) {
		t.Fatal("unsafe target mutated")
	}
}

func TestPersistedIdentityRejectsPublicPermissions(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadOrGenerate(dir, "a", "t"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, identityFileName), 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrGenerate(dir, "a", "t"); err == nil || !strings.Contains(err.Error(), "IDENTITY_PERMISSIONS") {
		t.Fatalf("unsafe identity permissions must fail closed, got %v", err)
	}
}
