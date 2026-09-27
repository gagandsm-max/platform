package control_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"platform/sensor/internal/control"
)

func TestStoreAbsentLoad(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "applied.json")
	store, err := control.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := store.Load()
	if err != nil || snap != nil {
		t.Fatalf("expected nil snapshot, got %#v err=%v", snap, err)
	}
}

func TestStoreAtomicWriteLoadRestart(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "applied.json")
	store, err := control.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := control.GoldenConfiguration()
	checksum, err := control.ComputeDesiredChecksum(cfg)
	if err != nil {
		t.Fatal(err)
	}
	snap := control.AppliedSnapshot{
		SchemaVersion: 1,
		Generation:    7,
		Checksum:      checksum,
		Configuration: cfg,
		AppliedAt:     time.Now().UTC().Truncate(time.Second),
	}
	if err := store.Commit(snap); err != nil {
		t.Fatal(err)
	}
	reopened, err := control.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Load()
	if err != nil || loaded == nil {
		t.Fatalf("load failed: %#v %v", loaded, err)
	}
	if loaded.Generation != 7 || loaded.Checksum != checksum {
		t.Fatalf("unexpected snapshot %#v", loaded)
	}
}

func TestStoreCorruptJSONFailClosed(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "applied.json")
	if err := os.WriteFile(path, []byte(`{"not":"valid"`), 0o640); err != nil {
		t.Fatal(err)
	}
	store, err := control.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Load()
	if err == nil || control.CodeOf(err) != control.CodeStateCorrupt {
		t.Fatalf("expected corrupt, got %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("corrupt snapshot must not be deleted: %v", statErr)
	}
}

func TestStoreUnknownFieldAndTamperFailClosed(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "applied.json")
	cfg := control.GoldenConfiguration()
	checksum, err := control.ComputeDesiredChecksum(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	withExtra := []byte(`{"schemaVersion":1,"generation":3,"checksum":"` + checksum + `","configuration":` + string(cfgJSON) + `,"appliedAt":"2026-09-27T00:00:00Z","extra":true}`)
	if err := os.WriteFile(path, withExtra, 0o640); err != nil {
		t.Fatal(err)
	}
	store, err := control.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil || control.CodeOf(err) != control.CodeStateCorrupt {
		t.Fatalf("unknown field must fail closed: %v", err)
	}

	tampered := []byte(`{"schemaVersion":1,"generation":3,"checksum":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","configuration":` + string(cfgJSON) + `,"appliedAt":"2026-09-27T00:00:00Z"}`)
	if err := os.WriteFile(path, tampered, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil || control.CodeOf(err) != control.CodeStateCorrupt {
		t.Fatalf("checksum tamper must fail closed: %v", err)
	}
}

func TestStoreInvalidGenerationFailClosed(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "applied.json")
	cfg := control.GoldenConfiguration()
	checksum, _ := control.ComputeDesiredChecksum(cfg)
	cfgJSON, _ := json.Marshal(cfg)
	body := []byte(`{"schemaVersion":1,"generation":0,"checksum":"` + checksum + `","configuration":` + string(cfgJSON) + `,"appliedAt":"2026-09-27T00:00:00Z"}`)
	if err := os.WriteFile(path, body, 0o640); err != nil {
		t.Fatal(err)
	}
	store, err := control.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil || control.CodeOf(err) != control.CodeStateCorrupt {
		t.Fatalf("generation 0 must fail closed: %v", err)
	}
}

func TestStorePriorSurvivesFailedReplacement(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "applied.json")
	store, err := control.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := control.GoldenConfiguration()
	checksum, _ := control.ComputeDesiredChecksum(cfg)
	good := control.AppliedSnapshot{
		SchemaVersion: 1, Generation: 2, Checksum: checksum, Configuration: cfg, AppliedAt: time.Now().UTC(),
	}
	if err := store.Commit(good); err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.Checksum = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := store.Commit(bad); err == nil {
		t.Fatal("expected semantic checksum validation failure")
	}
	loaded, err := store.Load()
	if err != nil || loaded == nil || loaded.Generation != 2 {
		t.Fatalf("prior snapshot must survive: %#v %v", loaded, err)
	}
}

func TestStoreRejectsSymlinkTarget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	real := filepath.Join(dir, "real.json")
	link := filepath.Join(dir, "link.json")
	if err := os.WriteFile(real, []byte(`{}`), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := control.NewStore(link); err == nil {
		t.Fatal("symlink path must be rejected")
	}
}

func TestStoreNoSecretFieldsPersisted(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "applied.json")
	store, err := control.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := control.GoldenConfiguration()
	checksum, _ := control.ComputeDesiredChecksum(cfg)
	if err := store.Commit(control.AppliedSnapshot{
		SchemaVersion: 1, Generation: 1, Checksum: checksum, Configuration: cfg, AppliedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(raw))
	for _, forbidden := range []string{"bootstrap", "privatekey", "password", "token", "tenantid", "sensorid", "credential"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("forbidden field fragment %q present in snapshot", forbidden)
		}
	}
}
