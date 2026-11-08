package identity

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentityDirectorySyncFailurePreservesPublishedIdentity(t *testing.T) {
	dir := t.TempDir()
	want := errors.New("injected directory sync failure")
	id, err := loadOrGenerate(dir, "", "t", func(string) error { return want })
	if id != nil || !errors.Is(err, want) {
		t.Fatalf("uncertain publication reported success: %v %v", id, err)
	}
	before, err := os.ReadFile(filepath.Join(dir, identityFileName))
	if err != nil {
		t.Fatal(err)
	}
	id, err = LoadOrGenerate(dir, "", "t")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(dir, identityFileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || id.AgentID == "" {
		t.Fatal("recovery replaced identity")
	}
	id, err = loadOrGenerate(dir, "", "t", func(string) error { return want })
	if id != nil || !errors.Is(err, want) {
		t.Fatal("reload bypassed durable directory reconciliation")
	}
}

func TestPersistedIdentityRejectsOversizedMetadata(t *testing.T) {
	dir := t.TempDir()
	data := `{"agentId":"a","tenantId":"t","hostId":"h","hostname":"` + strings.Repeat("x", 65536) + `"}`
	if err := os.WriteFile(filepath.Join(dir, identityFileName), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrGenerate(dir, "a", "t"); err == nil || !strings.Contains(err.Error(), "IDENTITY_TOO_LARGE") {
		t.Fatalf("oversized valid identity must fail closed, got %v", err)
	}
}
