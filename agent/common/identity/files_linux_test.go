//go:build linux

package identity

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

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
