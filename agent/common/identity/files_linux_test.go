//go:build linux

package identity

import (
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

func TestIdentitySyncOnlyStateDirectoryUnderHardenedParents(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "platform", "agent")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	// Mirrors an installer/systemd-provisioned writable StateDirectory beneath
	// a protected hierarchy. Root/ancestor sync is deliberately unavailable.
	if err := os.Chmod(parent, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0700) })
	var calls []string
	syncState := func(path string) error {
		calls = append(calls, path)
		if path != dir {
			return syscall.EROFS
		}
		return syncIdentityDir(path)
	}
	first, err := loadOrGenerate(dir, "a", "t", syncState)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrGenerate(dir, "a", "t", syncState)
	if err != nil || second.AgentID != first.AgentID {
		t.Fatalf("reload: %v %v", second, err)
	}
	if !reflect.DeepEqual(calls, []string{dir, dir}) {
		t.Fatalf("sync paths: %v", calls)
	}
}

func TestIdentityRejectsUnsafeWritableAncestor(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "state")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0777); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(parent, 0700)
	if _, err := LoadOrGenerate(dir, "a", "t"); err == nil {
		t.Fatal("unsafe ancestor accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, identityFileName)); !os.IsNotExist(err) {
		t.Fatal("identity published")
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
