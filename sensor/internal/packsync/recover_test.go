package packsync

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"platform/sensor/internal/coreclient"
)

func TestRecoverRejectsMissingArtifactReference(t *testing.T) {
	dir := t.TempDir()
	sha := strings.Repeat("a", 64)
	packs := []AppliedPackRef{{PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha}}
	writeAppliedFixture(t, dir, AppliedState{
		Generation: 1,
		Checksum:   ComputeDesiredChecksum(packs),
		Packs:      packs,
	}, nil)

	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if snap.TrustedApplied || snap.AppliedGeneration != 0 || snap.LastErrorCode != ErrCodeRestartFailClosed {
		t.Fatalf("missing artifact must fail closed without publishing trust: %+v", snap)
	}
}

func TestRecoverRejectsWrongStateChecksum(t *testing.T) {
	dir := t.TempDir()
	manifest := minimalManifest("fixture.pack", "1.0.0")
	sha := shaOf(manifest)
	packs := []AppliedPackRef{{PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha}}
	writeAppliedFixture(t, dir, AppliedState{
		Generation: 1,
		Checksum:   strings.Repeat("c", 64),
		Packs:      packs,
	}, map[string][]byte{sha: manifest})

	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if snap.TrustedApplied || snap.AppliedGeneration != 0 {
		t.Fatalf("wrong state checksum must not publish trust: %+v", snap)
	}
}

func TestRecoverRejectsDuplicatePackIdentity(t *testing.T) {
	dir := t.TempDir()
	m1 := minimalManifest("fixture.pack", "1.0.0")
	m2 := minimalManifest("fixture.pack", "2.0.0")
	sha1, sha2 := shaOf(m1), shaOf(m2)
	packs := []AppliedPackRef{
		{PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha1},
		{PackID: "fixture.pack", PackVersion: "2.0.0", ManifestSHA256: sha2},
	}
	// Use a fabricated checksum so invariant failure is pack-id dup, not checksum.
	writeAppliedFixture(t, dir, AppliedState{
		Generation: 1,
		Checksum:   strings.Repeat("d", 64),
		Packs:      packs,
	}, map[string][]byte{sha1: m1, sha2: m2})

	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().TrustedApplied {
		t.Fatal("duplicate pack id must fail closed")
	}
}

func TestRecoverRejectsPackListMutationUnchangedChecksum(t *testing.T) {
	dir := t.TempDir()
	mA := minimalManifest("fixture.a", "1.0.0")
	mB := minimalManifest("fixture.b", "1.0.0")
	shaA, shaB := shaOf(mA), shaOf(mB)
	honest := []AppliedPackRef{
		{PackID: "fixture.a", PackVersion: "1.0.0", ManifestSHA256: shaA},
		{PackID: "fixture.b", PackVersion: "1.0.0", ManifestSHA256: shaB},
	}
	checksum := ComputeDesiredChecksum(honest)
	// Mutate pack list while keeping the checksum for the original set.
	mutated := []AppliedPackRef{
		{PackID: "fixture.a", PackVersion: "1.0.0", ManifestSHA256: shaA},
	}
	writeAppliedFixture(t, dir, AppliedState{
		Generation: 1,
		Checksum:   checksum,
		Packs:      mutated,
	}, map[string][]byte{shaA: mA, shaB: mB})

	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().TrustedApplied || s.Snapshot().AppliedGeneration != 0 {
		t.Fatalf("pack-list mutation must fail closed: %+v", s.Snapshot())
	}
}

// TestSyncOnceFastPathDoesNotTrustMutatedAppliedMatchingDesiredChecksum guards P1-1:
// recoverOnStart fails (TrustedApplied=false) when applied.json packs are mutated but
// the checksum field still equals the Core desired checksum for the full honest set.
// syncOnce must not setSuccess/TrustedApplied on generation+checksum match alone.
func TestSyncOnceFastPathDoesNotTrustMutatedAppliedMatchingDesiredChecksum(t *testing.T) {
	dir := t.TempDir()
	mA := minimalManifest("fixture.a", "1.0.0")
	mB := minimalManifest("fixture.b", "1.0.0")
	shaA, shaB := shaOf(mA), shaOf(mB)
	honest := []AppliedPackRef{
		{PackID: "fixture.a", PackVersion: "1.0.0", ManifestSHA256: shaA},
		{PackID: "fixture.b", PackVersion: "1.0.0", ManifestSHA256: shaB},
	}
	checksum := ComputeDesiredChecksum(honest)
	mutated := []AppliedPackRef{
		{PackID: "fixture.a", PackVersion: "1.0.0", ManifestSHA256: shaA},
	}
	writeAppliedFixture(t, dir, AppliedState{
		Generation: 1,
		Checksum:   checksum,
		Packs:      mutated,
	}, map[string][]byte{shaA: mA, shaB: mB})

	entryA := coreclient.DesiredPackEntry{
		PackID: "fixture.a", PackVersion: "1.0.0", ManifestSHA256: shaA, ArtifactBytes: len(mA),
	}
	entryB := coreclient.DesiredPackEntry{
		PackID: "fixture.b", PackVersion: "1.0.0", ManifestSHA256: shaB, ArtifactBytes: len(mB),
	}
	core := &fakeCore{
		desired: coreclient.CapabilityPackDesiredState{
			Generation:        1,
			Checksum:          checksum,
			AppliedGeneration: 0,
			AppliedChecksum:   "",
			Packs:             []coreclient.DesiredPackEntry{entryA, entryB},
		},
		arts: map[string]coreclient.CapabilityPackArtifact{
			"fixture.a@1.0.0": {PackID: "fixture.a", PackVersion: "1.0.0", ManifestSHA256: shaA, Bytes: mA},
			"fixture.b@1.0.0": {PackID: "fixture.b", PackVersion: "1.0.0", ManifestSHA256: shaB, Bytes: mB},
		},
	}

	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().TrustedApplied {
		t.Fatal("recovery must leave TrustedApplied false for mutated applied.json")
	}

	s.syncOnce(context.Background())
	snap := s.Snapshot()
	if snap.TrustedApplied {
		t.Fatalf("fast path must not publish trust without applied revalidation: %+v", snap)
	}
	if snap.LastErrorCode != ErrCodeRestartFailClosed || !snap.Degraded {
		t.Fatalf("expected fail-closed after fast-path reject: %+v", snap)
	}
	if len(core.acks) != 0 {
		t.Fatalf("must not ACK untrusted mutated applied state, got %d acks", len(core.acks))
	}
	if core.artifactFetches != 0 {
		t.Fatalf("must not fall through to apply/download on gen+checksum match, fetches=%d", core.artifactFetches)
	}
}

// TestSyncOnceFastPathDoesNotTrustMissingArtifactsMatchingChecksum covers the
// honest applied.json + missing artifact case: recover fails, Core desired matches
// gen+checksum, fast path must re-verify artifacts and stay fail-closed.
func TestSyncOnceFastPathDoesNotTrustMissingArtifactsMatchingChecksum(t *testing.T) {
	dir := t.TempDir()
	mA := minimalManifest("fixture.a", "1.0.0")
	mB := minimalManifest("fixture.b", "1.0.0")
	shaA, shaB := shaOf(mA), shaOf(mB)
	honest := []AppliedPackRef{
		{PackID: "fixture.a", PackVersion: "1.0.0", ManifestSHA256: shaA},
		{PackID: "fixture.b", PackVersion: "1.0.0", ManifestSHA256: shaB},
	}
	checksum := ComputeDesiredChecksum(honest)
	// Only one artifact on disk — incomplete relative to applied pack list.
	writeAppliedFixture(t, dir, AppliedState{
		Generation: 1,
		Checksum:   checksum,
		Packs:      honest,
	}, map[string][]byte{shaA: mA})

	entryA := coreclient.DesiredPackEntry{
		PackID: "fixture.a", PackVersion: "1.0.0", ManifestSHA256: shaA, ArtifactBytes: len(mA),
	}
	entryB := coreclient.DesiredPackEntry{
		PackID: "fixture.b", PackVersion: "1.0.0", ManifestSHA256: shaB, ArtifactBytes: len(mB),
	}
	core := &fakeCore{
		desired: coreclient.CapabilityPackDesiredState{
			Generation:        1,
			Checksum:          checksum,
			AppliedGeneration: 1,
			AppliedChecksum:   checksum,
			Packs:             []coreclient.DesiredPackEntry{entryA, entryB},
		},
		arts: map[string]coreclient.CapabilityPackArtifact{
			"fixture.a@1.0.0": {PackID: "fixture.a", PackVersion: "1.0.0", ManifestSHA256: shaA, Bytes: mA},
			"fixture.b@1.0.0": {PackID: "fixture.b", PackVersion: "1.0.0", ManifestSHA256: shaB, Bytes: mB},
		},
	}

	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().TrustedApplied {
		t.Fatal("missing artifact must leave TrustedApplied false")
	}

	s.syncOnce(context.Background())
	snap := s.Snapshot()
	if snap.TrustedApplied {
		t.Fatalf("fast path must not trust without artifacts: %+v", snap)
	}
	if snap.LastErrorCode != ErrCodeRestartFailClosed || !snap.Degraded {
		t.Fatalf("expected fail-closed: %+v", snap)
	}
	if len(core.acks) != 0 || core.artifactFetches != 0 {
		t.Fatalf("must not ACK or re-apply on unverified fast path: acks=%d fetches=%d", len(core.acks), core.artifactFetches)
	}
}

func TestRecoverRejectsCorruptArtifact(t *testing.T) {
	dir := t.TempDir()
	manifest := minimalManifest("fixture.pack", "1.0.0")
	sha := shaOf(manifest)
	packs := []AppliedPackRef{{PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha}}
	corrupt := []byte(`{"schemaVersion":"1.0.0","packId":"fixture.pack","packVersion":"9.9.9"}`)
	// Place corrupt bytes under the expected sha name (content hash will not match).
	writeAppliedFixture(t, dir, AppliedState{
		Generation: 1,
		Checksum:   ComputeDesiredChecksum(packs),
		Packs:      packs,
	}, map[string][]byte{sha: corrupt})

	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().TrustedApplied || s.Snapshot().AppliedGeneration != 0 {
		t.Fatalf("corrupt artifact must not publish trusted generation: %+v", s.Snapshot())
	}
}

func TestRecoverEmptyInstallTrusted(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if !snap.TrustedApplied || snap.Degraded || snap.AppliedGeneration != 0 {
		t.Fatalf("fresh empty install must be trusted: %+v", snap)
	}
}

func TestRecoverArtifactFIFODoesNotHang(t *testing.T) {
	dir := t.TempDir()
	sha := strings.Repeat("e", 64)
	packs := []AppliedPackRef{{PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha}}
	artDir := filepath.Join(dir, artifactsDirName)
	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(artDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fifoPath := filepath.Join(artDir, sha+".json")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(AppliedState{
		Generation: 1,
		Checksum:   ComputeDesiredChecksum(packs),
		Packs:      packs,
	})
	if err := os.WriteFile(filepath.Join(stateDir, appliedStateFile), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	done := make(chan *Syncer, 1)
	go func() {
		s, _ := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
		done <- s
	}()
	select {
	case s := <-done:
		if s.Snapshot().TrustedApplied {
			t.Fatal("FIFO artifact must fail closed")
		}
		if s.Snapshot().LastErrorCode != ErrCodeRestartFailClosed {
			t.Fatalf("expected restart fail-closed, got %+v", s.Snapshot())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("FIFO artifact recovery hung")
	}
}

func TestRecoverRejectsDirectoryArtifact(t *testing.T) {
	dir := t.TempDir()
	sha := strings.Repeat("f", 64)
	packs := []AppliedPackRef{{PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha}}
	artDir := filepath.Join(dir, artifactsDirName)
	if err := os.MkdirAll(filepath.Join(artDir, sha+".json"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeAppliedFixture(t, dir, AppliedState{
		Generation: 1,
		Checksum:   ComputeDesiredChecksum(packs),
		Packs:      packs,
	}, nil)

	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().TrustedApplied {
		t.Fatal("directory artifact must be rejected")
	}
}

func TestRecoverRejectsSymlinkArtifact(t *testing.T) {
	dir := t.TempDir()
	manifest := minimalManifest("fixture.pack", "1.0.0")
	sha := shaOf(manifest)
	packs := []AppliedPackRef{{PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha}}
	artDir := filepath.Join(dir, artifactsDirName)
	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(artDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(artDir, sha+".json")); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(AppliedState{
		Generation: 1,
		Checksum:   ComputeDesiredChecksum(packs),
		Packs:      packs,
	})
	if err := os.WriteFile(filepath.Join(stateDir, appliedStateFile), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().TrustedApplied {
		t.Fatal("symlink artifact must be rejected")
	}
}

func TestRecoverRejectsOversizedArtifact(t *testing.T) {
	dir := t.TempDir()
	sha := strings.Repeat("1", 64)
	packs := []AppliedPackRef{{PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha}}
	oversized := []byte(strings.Repeat("x", 256*1024+1))
	writeAppliedFixture(t, dir, AppliedState{
		Generation: 1,
		Checksum:   ComputeDesiredChecksum(packs),
		Packs:      packs,
	}, map[string][]byte{sha: oversized})

	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().TrustedApplied {
		t.Fatal("oversized artifact must be rejected")
	}
}

func TestRecoverRejectsUnixSocketArtifact(t *testing.T) {
	// Keep path short: Linux sockaddr_un.sun_path is ~108 bytes.
	dir, err := os.MkdirTemp("/tmp", "ps")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	sha := strings.Repeat("2", 64)
	packs := []AppliedPackRef{{PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha}}
	artDir := filepath.Join(dir, artifactsDirName)
	if err := os.MkdirAll(artDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sockPath := filepath.Join(artDir, sha+".json")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Skipf("unix socket listen not available for this path: %v", err)
	}
	defer ln.Close()

	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(AppliedState{
		Generation: 1,
		Checksum:   ComputeDesiredChecksum(packs),
		Packs:      packs,
	})
	if err := os.WriteFile(filepath.Join(stateDir, appliedStateFile), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	done := make(chan *Syncer, 1)
	go func() {
		s, _ := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
		done <- s
	}()
	select {
	case s := <-done:
		if s.Snapshot().TrustedApplied {
			t.Fatal("socket artifact must be rejected")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("socket artifact recovery hung")
	}
}

func TestWriteArtifactDurableRejectsExistingFIFO(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("3", 64)
	fifoPath := filepath.Join(dir, artifactsDirName, sha+".json")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- s.writeArtifactDurable(sha, []byte(`{"schemaVersion":"1.0.0"}`))
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected FIFO existing artifact rejection")
		}
		if !strings.Contains(err.Error(), ErrCodeArtifactVerify) && !strings.Contains(err.Error(), ErrCodeDurableWrite) {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("writeArtifactDurable hung on FIFO")
	}
}
