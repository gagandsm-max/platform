package packsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"platform/sensor/internal/coreclient"
)

type fakeCore struct {
	mu       sync.Mutex
	desired  coreclient.CapabilityPackDesiredState
	arts     map[string]coreclient.CapabilityPackArtifact
	acks            []ackCall
	artifactFetches int
	fetchErr        error
	ackErr          error
}

type ackCall struct {
	Generation int64
	Checksum   string
}

func (f *fakeCore) FetchCapabilityPackDesiredState(ctx context.Context) (coreclient.CapabilityPackDesiredState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fetchErr != nil {
		return coreclient.CapabilityPackDesiredState{}, f.fetchErr
	}
	return f.desired, nil
}

func (f *fakeCore) FetchCapabilityPackArtifact(ctx context.Context, packID, packVersion string, maxBytes int) (coreclient.CapabilityPackArtifact, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.artifactFetches++
	key := packID + "@" + packVersion
	art, ok := f.arts[key]
	if !ok {
		return coreclient.CapabilityPackArtifact{}, errors.New("missing artifact")
	}
	return art, nil
}

func (f *fakeCore) AckCapabilityPackAppliedState(ctx context.Context, generation int64, checksum string) (coreclient.AppliedStateAck, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ackErr != nil {
		return coreclient.AppliedStateAck{}, f.ackErr
	}
	f.acks = append(f.acks, ackCall{Generation: generation, Checksum: checksum})
	return coreclient.AppliedStateAck{Status: "ACCEPTED", Generation: generation, Checksum: checksum}, nil
}

func TestPackSyncApplyAckAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	manifest := minimalManifest("fixture.pack", "1.0.0")
	sha := shaOf(manifest)
	checksum := "ab" + sha[:62]
	core := &fakeCore{
		desired: coreclient.CapabilityPackDesiredState{
			Generation: 1,
			Checksum:   checksum,
			Packs: []coreclient.DesiredPackEntry{{
				PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha, ArtifactBytes: len(manifest),
			}},
		},
		arts: map[string]coreclient.CapabilityPackArtifact{
			"fixture.pack@1.0.0": {PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha, Bytes: manifest},
		},
	}
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	s.syncOnce(context.Background())
	if len(core.acks) != 1 {
		t.Fatalf("expected 1 ack, got %d", len(core.acks))
	}
	appliedPath := filepath.Join(dir, stateDirName, appliedStateFile)
	raw, err := os.ReadFile(appliedPath)
	if err != nil {
		t.Fatal(err)
	}
	var applied AppliedState
	if err := json.Unmarshal(raw, &applied); err != nil {
		t.Fatal(err)
	}
	if applied.Generation != 1 || applied.Checksum != checksum {
		t.Fatalf("unexpected applied %+v", applied)
	}
	artPath := filepath.Join(dir, artifactsDirName, sha+".json")
	got, err := os.ReadFile(artPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(manifest) {
		t.Fatal("artifact bytes mismatch")
	}

	s.syncOnce(context.Background())
	if len(core.acks) != 1 {
		t.Fatalf("idempotent sync must not re-ACK, got %d", len(core.acks))
	}
}

func TestPackSyncLostAckRetriesWithoutRedownload(t *testing.T) {
	dir := t.TempDir()
	manifest := minimalManifest("fixture.pack", "1.0.0")
	sha := shaOf(manifest)
	checksum := "af" + sha[:62]
	core := &fakeCore{
		desired: coreclient.CapabilityPackDesiredState{
			Generation: 1,
			Checksum:   checksum,
			Packs: []coreclient.DesiredPackEntry{{
				PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha, ArtifactBytes: len(manifest),
			}},
		},
		arts: map[string]coreclient.CapabilityPackArtifact{
			"fixture.pack@1.0.0": {PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha, Bytes: manifest},
		},
		ackErr: errors.New("simulated lost ACK response"),
	}
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}

	s.syncOnce(context.Background())
	applied, err := s.loadApplied()
	if err != nil {
		t.Fatal(err)
	}
	if applied.Generation != 1 || applied.Checksum != checksum {
		t.Fatalf("durable applied state missing after lost ACK: %+v", applied)
	}
	if core.artifactFetches != 1 {
		t.Fatalf("expected one artifact fetch, got %d", core.artifactFetches)
	}
	if len(core.acks) != 0 {
		t.Fatalf("failed ACK must not be recorded as successful: %d", len(core.acks))
	}

	core.mu.Lock()
	core.ackErr = nil
	core.mu.Unlock()
	s.syncOnce(context.Background())

	if len(core.acks) != 1 {
		t.Fatalf("expected retry ACK to converge Core state, got %d", len(core.acks))
	}
	if core.artifactFetches != 1 {
		t.Fatalf("ACK retry must not re-download artifact, fetches=%d", core.artifactFetches)
	}
	if s.Snapshot().Degraded {
		t.Fatal("successful ACK retry must clear degraded status")
	}
}

func TestPackSyncRestartReAcksDurableAppliedState(t *testing.T) {
	dir := t.TempDir()
	manifest := minimalManifest("fixture.pack", "1.0.0")
	sha := shaOf(manifest)
	checksum := "bc" + sha[:62]
	core := &fakeCore{
		desired: coreclient.CapabilityPackDesiredState{
			Generation: 1,
			Checksum:   checksum,
			Packs: []coreclient.DesiredPackEntry{{
				PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha, ArtifactBytes: len(manifest),
			}},
		},
		arts: map[string]coreclient.CapabilityPackArtifact{
			"fixture.pack@1.0.0": {PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha, Bytes: manifest},
		},
		ackErr: errors.New("simulated lost ACK response"),
	}

	first, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	first.syncOnce(context.Background())
	if core.artifactFetches != 1 {
		t.Fatalf("expected initial artifact fetch, got %d", core.artifactFetches)
	}

	core.mu.Lock()
	core.ackErr = nil
	core.mu.Unlock()
	restarted, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	restarted.syncOnce(context.Background())

	if len(core.acks) != 1 {
		t.Fatalf("restart must re-ACK durable non-zero state, got %d", len(core.acks))
	}
	if core.artifactFetches != 1 {
		t.Fatalf("restart ACK recovery must not re-download artifact, fetches=%d", core.artifactFetches)
	}
}

func TestPackSyncGenerationZeroDoesNotAck(t *testing.T) {
	dir := t.TempDir()
	core := &fakeCore{desired: coreclient.CapabilityPackDesiredState{Generation: 0}}
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	s.syncOnce(context.Background())
	if len(core.acks) != 0 {
		t.Fatalf("generation zero must not emit ACK, got %d", len(core.acks))
	}
}

func TestPackSyncRegressionAndConflict(t *testing.T) {
	dir := t.TempDir()
	sha := strings.Repeat("a", 64)
	core := &fakeCore{
		desired: coreclient.CapabilityPackDesiredState{Generation: 2, Checksum: "bb" + sha[:62]},
	}
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.writeAppliedDurable(AppliedState{Generation: 3, Checksum: "cc" + sha[:62], Packs: nil}); err != nil {
		t.Fatal(err)
	}
	s.syncOnce(context.Background())
	if s.Snapshot().LastErrorCode != ErrCodeStateRegression {
		t.Fatalf("expected regression, got %s", s.Snapshot().LastErrorCode)
	}

	if err := s.writeAppliedDurable(AppliedState{Generation: 2, Checksum: "dd" + sha[:62], Packs: nil}); err != nil {
		t.Fatal(err)
	}
	s.syncOnce(context.Background())
	if s.Snapshot().LastErrorCode != ErrCodeStateConflict {
		t.Fatalf("expected conflict, got %s", s.Snapshot().LastErrorCode)
	}
}

func TestPackSyncFailureKeepsPriorNoAck(t *testing.T) {
	dir := t.TempDir()
	manifest := minimalManifest("fixture.pack", "1.0.0")
	sha := shaOf(manifest)
	core := &fakeCore{
		desired: coreclient.CapabilityPackDesiredState{
			Generation: 1,
			Checksum:   "ee" + sha[:62],
			Packs: []coreclient.DesiredPackEntry{{
				PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha, ArtifactBytes: len(manifest),
			}},
		},
		arts: map[string]coreclient.CapabilityPackArtifact{},
	}
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	s.syncOnce(context.Background())
	if len(core.acks) != 0 {
		t.Fatal("must not ACK on failure")
	}
}

func TestPackSyncAuthFailClosedRetainsApplied(t *testing.T) {
	dir := t.TempDir()
	authErr := coreclient.ErrAuthenticationForTest()
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{fetchErr: authErr})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.writeAppliedDurable(AppliedState{Generation: 5, Checksum: strings.Repeat("a", 64), Packs: nil}); err != nil {
		t.Fatal(err)
	}
	s.syncOnce(context.Background())
	snap := s.Snapshot()
	if snap.LastErrorCode != ErrCodeAuthFailed || !snap.Degraded {
		t.Fatalf("expected auth fail-closed degraded, got %+v", snap)
	}
	applied, err := s.loadApplied()
	if err != nil || applied.Generation != 5 {
		t.Fatalf("applied must be retained: %+v %v", applied, err)
	}
}

func TestRestartFailClosedOnBadChecksum(t *testing.T) {
	dir := t.TempDir()
	artDir := filepath.Join(dir, artifactsDirName)
	stateDir := filepath.Join(dir, stateDirName)
	_ = os.MkdirAll(artDir, 0o700)
	_ = os.MkdirAll(stateDir, 0o700)
	sha := strings.Repeat("a", 64)
	_ = os.WriteFile(filepath.Join(artDir, sha+".json"), []byte(`{"schemaVersion":"1.0.0","packId":"p","packVersion":"1"}`), 0o600)
	state := AppliedState{Generation: 1, Checksum: sha, Packs: []AppliedPackRef{{
		PackID: "p", PackVersion: "1", ManifestSHA256: sha,
	}}}
	raw, _ := json.Marshal(state)
	_ = os.WriteFile(filepath.Join(stateDir, appliedStateFile), raw, 0o600)

	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.recoverOnStart(); err == nil {
		t.Fatal("expected recover fail closed on checksum mismatch")
	}
}

func TestLoadAppliedRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, []byte(`{"generation":0,"packs":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(stateDir, appliedStateFile)); err != nil {
		t.Fatal(err)
	}
	s := &Syncer{cfg: Config{Dir: dir}}
	if _, err := s.loadApplied(); err == nil || !strings.Contains(err.Error(), ErrCodeRestartFailClosed) {
		t.Fatalf("expected fail-closed symlink rejection, got %v", err)
	}
}

func TestLoadAppliedRejectsNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(filepath.Join(stateDir, appliedStateFile), 0o700); err != nil {
		t.Fatal(err)
	}
	s := &Syncer{cfg: Config{Dir: dir}}
	if _, err := s.loadApplied(); err == nil || !strings.Contains(err.Error(), ErrCodeRestartFailClosed) {
		t.Fatalf("expected fail-closed non-regular rejection, got %v", err)
	}
}

func TestLoadAppliedRejectsOversizedState(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	payload := []byte(strings.Repeat("x", int(maxAppliedStateBytes+1)))
	if err := os.WriteFile(filepath.Join(stateDir, appliedStateFile), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	s := &Syncer{cfg: Config{Dir: dir}}
	if _, err := s.loadApplied(); err == nil || !strings.Contains(err.Error(), ErrCodeRestartFailClosed) {
		t.Fatalf("expected fail-closed oversized state rejection, got %v", err)
	}
}

func TestRestartRecoversValidAppliedState(t *testing.T) {
	dir := t.TempDir()
	artDir := filepath.Join(dir, artifactsDirName)
	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(artDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := minimalManifest("fixture.pack", "1.0.0")
	sha := shaOf(manifest)
	if err := os.WriteFile(filepath.Join(artDir, sha+".json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	state := AppliedState{
		Generation: 1,
		Checksum:   strings.Repeat("b", 64),
		Packs: []AppliedPackRef{{
			PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha,
		}},
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, appliedStateFile), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if snap.Degraded || snap.LastErrorCode != "" || snap.AppliedGeneration != 1 || snap.PackCount != 1 {
		t.Fatalf("valid restart state did not recover cleanly: %+v", snap)
	}
}

func TestResolveCapabilityPackDirDistinct(t *testing.T) {
	got, err := ResolveCapabilityPackDir("", "/var/lib/platform/sensor/spool")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/var/lib/platform/sensor/capability-packs" {
		t.Fatalf("got %s", got)
	}
	_, err = ResolveCapabilityPackDir("relative/path", "/var/lib/platform/sensor/spool")
	if err == nil {
		t.Fatal("relative explicit must fail")
	}
}

func TestEnvelopeIdentityMismatch(t *testing.T) {
	err := verifyEnvelope([]byte(`{"schemaVersion":"1.0.0","packId":"other","packVersion":"1.0.0"}`), "expected", "1.0.0")
	if err == nil {
		t.Fatal("expected identity mismatch")
	}
}

func minimalManifest(packID, version string) []byte {
	m := map[string]any{
		"schemaVersion": "1.0.0",
		"packId":        packID,
		"packVersion":   version,
		"name":          "t",
	}
	b, _ := json.Marshal(m)
	return b
}

func shaOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
