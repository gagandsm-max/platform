package packsync

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"platform/sensor/internal/coreclient"
)

func TestFreshSensorNotReadyBeforeFirstCoreSync(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if !snap.TrustedApplied {
		t.Fatal("local empty state must be structurally trusted")
	}
	if snap.CoreAuthorityEstablished || snap.Ready() {
		t.Fatalf("must not be ready before first Core sync: %+v", snap)
	}
	if _, err := os.Stat(filepath.Join(dir, stateDirName, appliedStateFile)); !os.IsNotExist(err) {
		t.Fatalf("fresh sensor must not invent applied.json before Core confirmation: %v", err)
	}
}

func TestFreshSensorAuthFailureKeepsNotReady(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{
		fetchErr: coreclient.ErrAuthenticationForTest(),
	})
	if err != nil {
		t.Fatal(err)
	}
	s.syncOnce(context.Background())
	snap := s.Snapshot()
	if snap.LastErrorCode != ErrCodeAuthFailed || !snap.Degraded {
		t.Fatalf("expected auth fail-closed degraded: %+v", snap)
	}
	if snap.CoreAuthorityEstablished || snap.Ready() {
		t.Fatalf("auth failure must not establish Core authority: %+v", snap)
	}
}

func TestFreshSensorAuthorizationFailureKeepsNotReady(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{
		fetchErr: coreclient.ErrAuthorizationForTest(),
	})
	if err != nil {
		t.Fatal(err)
	}
	s.syncOnce(context.Background())
	snap := s.Snapshot()
	if snap.LastErrorCode != ErrCodeAuthFailed {
		t.Fatalf("expected authz mapped to auth failed: %+v", snap)
	}
	if snap.CoreAuthorityEstablished || snap.Ready() {
		t.Fatalf("authorization failure must not establish Core authority: %+v", snap)
	}
}

func TestFreshSensorTransientFailureKeepsNotReady(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{
		fetchErr: coreclient.ErrTransientTransportForTest(),
	})
	if err != nil {
		t.Fatal(err)
	}
	s.syncOnce(context.Background())
	snap := s.Snapshot()
	if snap.LastErrorCode != ErrCodeSyncTransient {
		t.Fatalf("expected transient code: %+v", snap)
	}
	if snap.CoreAuthorityEstablished || snap.Ready() {
		t.Fatalf("transient failure must not establish Core authority: %+v", snap)
	}
}

func TestFreshSensorCoreGen0EmptyEstablishesAuthorityAndReady(t *testing.T) {
	dir := t.TempDir()
	core := &fakeCore{desired: coreclient.CapabilityPackDesiredState{Generation: 0}}
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Ready() {
		t.Fatal("must not be ready before first successful sync")
	}
	s.syncOnce(context.Background())
	snap := s.Snapshot()
	if !snap.TrustedApplied || !snap.CoreAuthorityEstablished || !snap.Ready() {
		t.Fatalf("Core gen0 empty must establish authority and readiness: %+v", snap)
	}
	if snap.Degraded || snap.LastErrorCode != "" || snap.AppliedGeneration != 0 {
		t.Fatalf("unexpected post-sync status: %+v", snap)
	}
	applied, present, err := s.loadApplied()
	if err != nil || !present {
		t.Fatalf("Core confirmation must durably write applied.json: present=%v err=%v", present, err)
	}
	if !applied.CoreAuthorityEstablished || applied.Generation != 0 || len(applied.Packs) != 0 {
		t.Fatalf("durable authority marker missing/incorrect: %+v", applied)
	}
	if len(core.acks) != 0 {
		t.Fatalf("generation zero must not emit ACK, got %d", len(core.acks))
	}
}

func TestRestartAfterCoreConfirmedEmptyRemainsReady(t *testing.T) {
	dir := t.TempDir()
	core := &fakeCore{desired: coreclient.CapabilityPackDesiredState{Generation: 0}}
	first, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	first.syncOnce(context.Background())
	if !first.Snapshot().Ready() {
		t.Fatal("setup requires Core-confirmed empty readiness")
	}

	second, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{
		fetchErr: coreclient.ErrTransientTransportForTest(),
	})
	if err != nil {
		t.Fatal(err)
	}
	snap := second.Snapshot()
	if !snap.TrustedApplied || !snap.CoreAuthorityEstablished || !snap.Ready() {
		t.Fatalf("restart must recover durable Core authority before Core reconnect: %+v", snap)
	}
}

func TestRestartAfterConfirmedEmptyWithCoreOutageReadyDegraded(t *testing.T) {
	dir := t.TempDir()
	core := &fakeCore{desired: coreclient.CapabilityPackDesiredState{Generation: 0}}
	first, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	first.syncOnce(context.Background())

	second, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{
		fetchErr: coreclient.ErrTransientTransportForTest(),
	})
	if err != nil {
		t.Fatal(err)
	}
	second.syncOnce(context.Background())
	snap := second.Snapshot()
	if !snap.Ready() {
		t.Fatalf("must remain ready on transient Core outage after confirmed empty: %+v", snap)
	}
	if snap.LastErrorCode != ErrCodeSyncTransient {
		t.Fatalf("expected transient evidence: %+v", snap)
	}
}

func TestRestartAfterConfirmedNonEmptyWithCoreOutageRemainsReady(t *testing.T) {
	dir := t.TempDir()
	manifest := minimalManifest("fixture.pack", "1.0.0")
	sha := shaOf(manifest)
	entry := coreclient.DesiredPackEntry{
		PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha, ArtifactBytes: len(manifest),
	}
	checksum := checksumForDesired(entry)
	core := &fakeCore{
		desired: coreclient.CapabilityPackDesiredState{
			Generation: 1,
			Checksum:   checksum,
			Packs:      []coreclient.DesiredPackEntry{entry},
		},
		arts: map[string]coreclient.CapabilityPackArtifact{
			"fixture.pack@1.0.0": {PackID: "fixture.pack", PackVersion: "1.0.0", ManifestSHA256: sha, Bytes: manifest},
		},
	}
	first, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	first.syncOnce(context.Background())
	if !first.Snapshot().Ready() || first.Snapshot().AppliedGeneration != 1 {
		t.Fatalf("setup requires non-empty confirmed state: %+v", first.Snapshot())
	}

	second, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{
		fetchErr: coreclient.ErrTransientTransportForTest(),
	})
	if err != nil {
		t.Fatal(err)
	}
	second.syncOnce(context.Background())
	snap := second.Snapshot()
	if !snap.Ready() || snap.AppliedGeneration != 1 || snap.PackCount != 1 {
		t.Fatalf("must remain ready with prior non-empty state: %+v", snap)
	}
	if snap.LastErrorCode != ErrCodeSyncTransient {
		t.Fatalf("expected transient evidence: %+v", snap)
	}
}

func TestCorruptAuthorityMarkerFailClosed(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, stateDirName)
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Partially written / corrupt metadata must fail closed.
	if err := os.WriteFile(filepath.Join(stateDir, appliedStateFile), []byte(`{"generation":0,"packs":[],"coreAuthorityEstablished":`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if snap.TrustedApplied || snap.CoreAuthorityEstablished || snap.Ready() {
		t.Fatalf("corrupt authority metadata must fail closed: %+v", snap)
	}
	if snap.LastErrorCode != ErrCodeRestartFailClosed {
		t.Fatalf("expected restart fail-closed: %+v", snap)
	}
}

func TestGen0AppliedWithoutAuthorityFlagNotReady(t *testing.T) {
	dir := t.TempDir()
	writeAppliedFixture(t, dir, AppliedState{
		Generation: 0,
		Packs:      []AppliedPackRef{},
	}, nil)
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if !snap.TrustedApplied {
		t.Fatal("structurally valid gen0 file remains locally trusted")
	}
	if snap.CoreAuthorityEstablished || snap.Ready() {
		t.Fatalf("gen0 without coreAuthorityEstablished must not claim Core authority: %+v", snap)
	}
}

func TestAuthorityFlagWithInvalidUnderlyingStateFailClosed(t *testing.T) {
	dir := t.TempDir()
	writeAppliedFixture(t, dir, AppliedState{
		Generation:               0,
		Checksum:                 "not-empty",
		Packs:                    []AppliedPackRef{},
		CoreAuthorityEstablished: true,
	}, nil)
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	snap := s.Snapshot()
	if snap.TrustedApplied || snap.CoreAuthorityEstablished || snap.Ready() {
		t.Fatalf("authority flag must not override invalid underlying state: %+v", snap)
	}
}

func TestDurableAuthorityWriteFailureDoesNotPublishReady(t *testing.T) {
	dir := t.TempDir()
	core := &fakeCore{desired: coreclient.CapabilityPackDesiredState{Generation: 0}}
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	s.TestingForceAppliedWriteFailure(true)

	s.syncOnce(context.Background())
	snap := s.Snapshot()
	if snap.CoreAuthorityEstablished || snap.Ready() {
		t.Fatalf("durable write failure must not publish ready/authority: %+v", snap)
	}
	if snap.LastErrorCode != ErrCodeDurableWrite || !snap.Degraded {
		t.Fatalf("expected durable-write fail-closed: %+v", snap)
	}
	if _, err := os.Stat(filepath.Join(dir, stateDirName, appliedStateFile)); !os.IsNotExist(err) {
		t.Fatalf("failed authority write must leave applied.json absent: %v", err)
	}
}

func TestCrashBeforeAuthorityCommitLeavesNotReadyOnRestart(t *testing.T) {
	// Simulates Core response accepted in-memory path interrupted before durable
	// authority commit: no applied.json on disk, restart remains not ready.
	dir := t.TempDir()
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{
		desired: coreclient.CapabilityPackDesiredState{Generation: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Intentionally do not call syncOnce — crash before durable commit.
	restart, err := New(Config{Dir: dir, PollInterval: time.Hour}, &fakeCore{})
	if err != nil {
		t.Fatal(err)
	}
	if restart.Snapshot().Ready() || restart.Snapshot().CoreAuthorityEstablished {
		t.Fatalf("restart without durable authority commit must stay not ready: %+v", restart.Snapshot())
	}
	_ = s
}

func TestCoreGen0DesiredStateReplayIdempotent(t *testing.T) {
	dir := t.TempDir()
	core := &fakeCore{desired: coreclient.CapabilityPackDesiredState{Generation: 0}}
	s, err := New(Config{Dir: dir, PollInterval: time.Hour}, core)
	if err != nil {
		t.Fatal(err)
	}
	s.syncOnce(context.Background())
	s.syncOnce(context.Background())
	applied, present, err := s.loadApplied()
	if err != nil || !present || !applied.CoreAuthorityEstablished {
		t.Fatalf("replay must keep durable authority: present=%v applied=%+v err=%v", present, applied, err)
	}
	if !s.Snapshot().Ready() {
		t.Fatal("replay of Core gen0 must remain ready")
	}
	raw, err := os.ReadFile(filepath.Join(dir, stateDirName, appliedStateFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"coreAuthorityEstablished":true`) {
		t.Fatalf("durable JSON must retain authority flag: %s", raw)
	}
	var decoded AppliedState
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Generation != 0 || len(decoded.Packs) != 0 {
		t.Fatalf("replay must not invent generation or packs: %+v", decoded)
	}
}
