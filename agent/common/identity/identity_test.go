package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentityGeneration(t *testing.T) {
	// Predefined ID
	id1, err := GenerateOrLoadIdentity("agent-custom-99")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1.AgentID != "agent-custom-99" {
		t.Fatalf("expected agent-custom-99, got %s", id1.AgentID)
	}
	if id1.TenantID == "" {
		t.Fatal("expected tenant ID to be non-empty")
	}
	if id1.HostID == "" {
		t.Fatal("expected host ID to be non-empty")
	}

	// Generated ID
	id2, err := GenerateOrLoadIdentity("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id2.AgentID) == 0 {
		t.Fatalf("expected generated agent id, got empty")
	}
	if id2.Hostname == "" {
		t.Fatalf("expected hostname, got empty")
	}
	if id2.TenantID == "" {
		t.Fatal("expected tenant ID to be non-empty")
	}
	if id2.HostID == "" {
		t.Fatal("expected host ID to be non-empty")
	}
}

func TestIdentityPersistenceAndReload(t *testing.T) {
	tempDir := t.TempDir()
	tenant := "tenant-test-alpha"

	// 1. First generation: writes file
	id1, err := LoadOrGenerate(tempDir, "agent-stable-1", tenant)
	if err != nil {
		t.Fatalf("failed initial generation: %v", err)
	}

	idPath := filepath.Join(tempDir, identityFileName)
	fi, err := os.Stat(idPath)
	if err != nil {
		t.Fatalf("expected identity file to exist: %v", err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("expected 0600 permissions, got %o", fi.Mode().Perm())
	}

	// 2. Reload: matches persisted IDs
	id2, err := LoadOrGenerate(tempDir, "agent-stable-1", tenant)
	if err != nil {
		t.Fatalf("failed reload: %v", err)
	}
	if id2.AgentID != id1.AgentID || id2.TenantID != id1.TenantID || id2.HostID != id1.HostID {
		t.Fatalf("reloaded identity does not match original: %+v vs %+v", id2, id1)
	}

	// 3. Tenant mismatch failure
	_, err = LoadOrGenerate(tempDir, "agent-stable-1", "different-tenant")
	if err == nil || !strings.Contains(err.Error(), "IDENTITY_TENANT_MISMATCH") {
		t.Fatalf("expected IDENTITY_TENANT_MISMATCH error, got %v", err)
	}

	// 4. Corrupted file failure
	if err := os.WriteFile(idPath, []byte("NOT_JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = LoadOrGenerate(tempDir, "agent-stable-1", tenant)
	if err == nil || !strings.Contains(err.Error(), "IDENTITY_CORRUPT") {
		t.Fatalf("expected IDENTITY_CORRUPT error, got %v", err)
	}
}

func TestIdentitySymlinkRejection(t *testing.T) {
	tempDir := t.TempDir()
	realFile := filepath.Join(tempDir, "real_identity.json")
	if err := os.WriteFile(realFile, []byte(`{"agentId":"a","tenantId":"t","hostId":"h"}`), 0600); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(tempDir, identityFileName)
	if err := os.Symlink(realFile, symlinkPath); err != nil {
		t.Skip("symlink not supported on this platform")
	}

	_, err := LoadOrGenerate(tempDir, "a", "t")
	if err == nil || !strings.Contains(err.Error(), "IDENTITY_SYMLINK_REJECTED") {
		t.Fatalf("expected IDENTITY_SYMLINK_REJECTED error, got %v", err)
	}
}
