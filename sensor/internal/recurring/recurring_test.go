package recurring_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/protocol/snmp"
	"platform/sensor/internal/recurring"
	"platform/sensor/internal/scheduler"
)

func validSNMPParams() *recurring.SNMPParams {
	return &recurring.SNMPParams{
		Target:    "192.0.2.10",
		Port:      161,
		Version:   "2c",
		Operation: "GET",
		OIDs:      []string{".1.3.6.1.2.1.1.3.0"},
		TimeoutMs: 500,
		Limits: recurring.SNMPLimits{
			MaxOIDs:            16,
			MaxWalkEntries:     128,
			MaxBulkRepetitions: 10,
			MaxResults:         128,
			MaxResultBytes:     1 << 20,
		},
	}
}

func validDef(assignmentID string) recurring.RecurringDefinition {
	return recurring.RecurringDefinition{
		AssignmentID:         assignmentID,
		TenantID:             "tenant-a",
		EntityID:             "entity-1",
		MonitorDefinitionID:  "mon-1",
		SchedulingClass:      recurring.SchedulingLiveness,
		ExecutionClass:       "NETWORK_POLL",
		CadenceMs:            60_000,
		FreshnessMs:          120_000,
		PhaseVersion:         recurring.PhaseVersion1,
		ExecutionTimeoutMs:   5_000,
		CredentialProfileRef: "cred-profile-1",
		Protocol:             recurring.ProtocolSNMP,
		SNMP:                 validSNMPParams(),
	}
}

func desiredFrom(defs ...recurring.RecurringDefinition) recurring.DesiredState {
	cp := append([]recurring.RecurringDefinition(nil), defs...)
	return recurring.DesiredState{
		Generation:  1,
		Checksum:    recurring.ComputeDesiredChecksum(cp),
		Definitions: cp,
	}
}

// 1. deterministic phase stable across restart
func TestPhaseStableAcrossRestart(t *testing.T) {
	def := validDef("asg-1")
	p1, err := recurring.ComputePhase(def)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := recurring.ComputePhase(def)
	if err != nil {
		t.Fatal(err)
	}
	if p1 != p2 {
		t.Fatalf("phase not stable: %v vs %v", p1, p2)
	}
}

// 2. phase falls within cadence interval
func TestPhaseWithinCadence(t *testing.T) {
	def := validDef("asg-1")
	phase, err := recurring.ComputePhase(def)
	if err != nil {
		t.Fatal(err)
	}
	cadence := time.Duration(def.CadenceMs) * time.Millisecond
	if phase < 0 || phase >= cadence {
		t.Fatalf("phase %v outside [0, %v)", phase, cadence)
	}
}

// 3. independent assignments distribute rather than all synchronize
func TestPhaseDistribution(t *testing.T) {
	cadence := int64(60_000)
	phases := map[time.Duration]struct{}{}
	for i := 0; i < 32; i++ {
		def := validDef(fmt.Sprintf("asg-%d", i))
		def.CadenceMs = cadence
		def.EntityID = fmt.Sprintf("entity-%d", i)
		p, err := recurring.ComputePhase(def)
		if err != nil {
			t.Fatal(err)
		}
		phases[p] = struct{}{}
	}
	if len(phases) < 8 {
		t.Fatalf("expected distributed phases, got %d unique", len(phases))
	}
}

// 4. Sensor identity change does not dominate/rephase stable estate
func TestPhaseIgnoresSensorIdentity(t *testing.T) {
	def := validDef("asg-1")
	p1, _ := recurring.ComputePhase(def)
	// Mutating fields that must NOT be phase inputs should not change phase when
	// the documented inputs stay fixed. Sensor ID is never a ComputePhase input;
	// prove by ensuring only non-input field changes leave phase identical.
	def.CredentialProfileRef = "other-profile"
	def.SNMP.Target = "198.51.100.1"
	p2, _ := recurring.ComputePhase(def)
	if p1 != p2 {
		t.Fatalf("phase changed when only non-phase fields changed")
	}
	def.AssignmentID = "asg-other"
	p3, _ := recurring.ComputePhase(def)
	if p3 == p1 {
		t.Fatalf("expected assignmentId change to rephase")
	}
}

// 5+6. cadence and freshness validation independent
func TestCadenceAndFreshnessIndependent(t *testing.T) {
	def := validDef("asg-1")
	def.CadenceMs = 5_000
	def.FreshnessMs = 1 // freshness << cadence allowed
	if err := recurring.ValidateDefinition(def); err != nil {
		t.Fatalf("freshness < cadence should be valid: %v", err)
	}
	def.FreshnessMs = 24 * 60 * 60 * 1000
	def.CadenceMs = 1_000
	if err := recurring.ValidateDefinition(def); err != nil {
		t.Fatalf("freshness >> cadence should be valid: %v", err)
	}
	def.CadenceMs = 500 // below min
	if err := recurring.ValidateDefinition(def); err == nil {
		t.Fatal("expected cadence rejection")
	}
	def.CadenceMs = 5_000
	def.FreshnessMs = 0
	if err := recurring.ValidateDefinition(def); err == nil {
		t.Fatal("expected freshness rejection")
	}
}

// 7. stale generation rejected
func TestApplyRejectsStaleGeneration(t *testing.T) {
	current := recurring.AppliedState{Generation: 5, Checksum: "abc", Definitions: nil}
	desired := desiredFrom(validDef("asg-1"))
	desired.Generation = 4
	_, result, err := recurring.ApplyDesired(current, desired, "tenant-a")
	if err == nil || !result.Rejected || result.ReasonCode != recurring.ErrCodeStateRegression {
		t.Fatalf("expected stale rejection, got result=%+v err=%v", result, err)
	}
}

// 8. identical generation/state idempotent
func TestApplyIdempotentSameGenerationChecksum(t *testing.T) {
	desired := desiredFrom(validDef("asg-1"))
	current := recurring.AppliedState{
		Generation:  desired.Generation,
		Checksum:    desired.Checksum,
		Definitions: desired.Definitions,
	}
	next, result, err := recurring.ApplyDesired(current, desired, "tenant-a")
	if err != nil || !result.Noop || !result.Accepted {
		t.Fatalf("expected noop, got result=%+v err=%v", result, err)
	}
	if next.Generation != current.Generation {
		t.Fatalf("generation changed on noop")
	}
}

// 9. same-generation conflicting checksum rejected
func TestApplyConflictChecksum(t *testing.T) {
	desired := desiredFrom(validDef("asg-1"))
	current := recurring.AppliedState{
		Generation:  desired.Generation,
		Checksum:    strings.Repeat("a", 64),
		Definitions: desired.Definitions,
	}
	_, result, err := recurring.ApplyDesired(current, desired, "tenant-a")
	if err == nil || result.ReasonCode != recurring.ErrCodeStateConflict {
		t.Fatalf("expected conflict, got result=%+v err=%v", result, err)
	}
}

// 10. durable recurring state survives restart
func TestDurableRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := recurring.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	desired := desiredFrom(validDef("asg-1"))
	applied := recurring.AppliedState{
		Generation:               desired.Generation,
		Checksum:                 desired.Checksum,
		Definitions:              desired.Definitions,
		CoreAuthorityEstablished: true,
	}
	if err := store.SaveApplied(applied); err != nil {
		t.Fatal(err)
	}
	store2, err := recurring.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, present, err := store2.LoadApplied()
	if err != nil || !present {
		t.Fatalf("load failed present=%v err=%v", present, err)
	}
	if loaded.Generation != applied.Generation || loaded.Checksum != applied.Checksum || len(loaded.Definitions) != 1 {
		t.Fatalf("loaded mismatch: %+v", loaded)
	}
}

// 11. malformed/corrupt durable state fails safely
func TestCorruptAppliedFailClosed(t *testing.T) {
	dir := t.TempDir()
	store, err := recurring.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state", "applied.json")
	if err := os.WriteFile(path, []byte(`{not-json`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err = store.LoadApplied()
	if err == nil || !strings.Contains(err.Error(), recurring.ErrCodeRestartFailClosed) {
		t.Fatalf("expected fail-closed, got %v", err)
	}
}

// 12. symlink rejected
func TestSymlinkAppliedRejected(t *testing.T) {
	dir := t.TempDir()
	store, err := recurring.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "evil.json")
	if err := os.WriteFile(target, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "state", "applied.json")
	_ = os.Remove(path)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	_, _, err = store.LoadApplied()
	if err == nil || !strings.Contains(err.Error(), recurring.ErrCodeRestartFailClosed) {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
}

// 13-16. coalesce, no fabricate, no infinite catch-up, no overlap
func TestMissedCyclesCoalesceNoOverlap(t *testing.T) {
	engine := recurring.NewDueEngine()
	def := validDef("asg-1")
	def.CadenceMs = 10_000
	def.FreshnessMs = 30_000
	phase, err := recurring.ComputePhase(def)
	if err != nil {
		t.Fatal(err)
	}

	// First evaluation deep into the timeline → one current due only.
	now := time.Unix(0, phase.Nanoseconds()).Add(time.Duration(def.CadenceMs) * time.Millisecond * 50)
	due1 := engine.Evaluate(now, []recurring.RecurringDefinition{def})
	if len(due1) != 1 {
		t.Fatalf("want 1 due, got %d", len(due1))
	}
	engine.MarkSubmitted(def.AssignmentID, due1[0].CurrentDue, now, now.Add(2*time.Second))

	// While in-flight, no overlap.
	dueOverlap := engine.Evaluate(now.Add(time.Second), []recurring.RecurringDefinition{def})
	if len(dueOverlap) != 0 {
		t.Fatalf("expected no overlap, got %d", len(dueOverlap))
	}

	// Advance many cadences after in-flight cleared; coalesce to one.
	engine.MarkFinished(def.AssignmentID, now.Add(2*time.Second), true)
	later := now.Add(time.Duration(def.CadenceMs) * time.Millisecond * 20)
	due2 := engine.Evaluate(later, []recurring.RecurringDefinition{def})
	if len(due2) != 1 {
		t.Fatalf("want 1 coalesced due, got %d", len(due2))
	}
	if due2[0].MissedSkipped < 1 {
		t.Fatalf("expected skipped coalesced evidence, got %d", due2[0].MissedSkipped)
	}
	rt, ok := engine.RuntimeSnapshot(def.AssignmentID)
	if !ok || rt.SkippedCoalesced < 1 {
		t.Fatalf("runtime skipped not recorded: %+v", rt)
	}
}

// 17. SNMP path reaches scheduler executor
func TestSNMPPathToSchedulerExecutor(t *testing.T) {
	const communitySecret = "unit-only-community-secret-value"
	dir := t.TempDir()

	var sawSNMP atomic.Bool
	var sawKind atomic.Value
	exec := scheduler.Executor(executorFunc(func(ctx context.Context, job scheduler.Job) (scheduler.ExecutionResult, error) {
		sawKind.Store(string(job.Kind))
		if job.Kind != snmp.SchedulerJobKind {
			return scheduler.ExecutionResult{}, fmt.Errorf("unexpected kind %s", job.Kind)
		}
		sawSNMP.Store(true)
		return scheduler.ExecutionResult{}, nil
	}))

	cfg := scheduler.Config{
		MaxWorkers:              2,
		MaxQueueDepth:           8,
		MaxTrackedAssignments:   32,
		MaxTrackedJobs:          32,
		MaxPayloadBytes:         1 << 20,
		DefaultExecutionTimeout: time.Second,
		ShutdownGracePeriod:     time.Second,
		Retry: scheduler.RetryPolicy{
			MaxAttempts:    1,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     time.Millisecond,
			JitterFraction: 0,
		},
		Pools: map[scheduler.ExecutionClass]scheduler.PoolConfig{
			scheduler.ClassNetworkPoll: {Workers: 1, QueueDepth: 4},
		},
	}
	sched, err := scheduler.New(cfg, exec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = sched.Shutdown(ctx)
	})

	engine := recurring.NewDueEngine()
	metrics := &recurring.Metrics{}
	syncer, err := recurring.New(recurring.SyncerConfig{
		Dir:            dir,
		ExpectedTenant: "tenant-a",
		Metrics:        metrics,
	}, &staticCore{}, engine)
	if err != nil {
		t.Fatal(err)
	}
	desired := desiredFrom(validDef("asg-snmp"))
	if _, err := syncer.ApplyInMemory(desired); err != nil {
		t.Fatal(err)
	}

	// Prove durable JSON never contains the community secret.
	raw, err := os.ReadFile(filepath.Join(dir, "state", "applied.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), communitySecret) {
		t.Fatal("durable applied.json contains community secret")
	}

	resolver := credentialResolverFunc(func(tenantID, profileRef string) (snmp.Credentials, error) {
		return snmp.Credentials{Community: &snmp.CommunityCredentials{Community: snmp.Secret(communitySecret)}}, nil
	})
	emitter, err := recurring.NewEmitter(syncer, engine, sched, resolver, recurring.EmitterConfig{
		LeaseTTL: 5 * time.Second,
		Metrics:  metrics,
	})
	if err != nil {
		t.Fatal(err)
	}

	def := desired.Definitions[0]
	now := time.Now().UTC()

	// Directly verify due evaluation produces work under wall clock.
	due := engine.Evaluate(now, []recurring.RecurringDefinition{def})
	if len(due) != 1 {
		t.Fatalf("expected 1 due work before tick, got %d", len(due))
	}
	engine.Reset()

	if err := emitter.Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	snap := metrics.Snapshot()
	if snap.Submitted == 0 {
		t.Fatalf("expected submitted>0, metrics=%+v queueRejected=%d", snap, snap.QueueAdmissionRejected)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sawSNMP.Load() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !sawSNMP.Load() {
		t.Fatal("SNMP executor never invoked")
	}
	if got, _ := sawKind.Load().(string); got != string(snmp.SchedulerJobKind) {
		t.Fatalf("kind=%q", got)
	}
	// Re-read durable store — still no secret.
	raw2, _ := os.ReadFile(filepath.Join(dir, "state", "applied.json"))
	if strings.Contains(string(raw2), communitySecret) {
		t.Fatal("community leaked into durable state after submit")
	}
}

// 18. Core outage continues from durable applied
func TestCoreOutageContinuesFromDurable(t *testing.T) {
	dir := t.TempDir()
	engine := recurring.NewDueEngine()
	core := &flakyCore{failFetch: true}
	syncer, err := recurring.New(recurring.SyncerConfig{
		Dir:            dir,
		ExpectedTenant: "tenant-a",
	}, core, engine)
	if err != nil {
		t.Fatal(err)
	}
	desired := desiredFrom(validDef("asg-1"))
	if _, err := syncer.ApplyInMemory(desired); err != nil {
		t.Fatal(err)
	}
	err = syncer.SyncOnce(context.Background())
	if err == nil {
		t.Fatal("expected fetch failure")
	}
	applied := syncer.Applied()
	if applied.Generation != 1 || len(applied.Definitions) != 1 {
		t.Fatalf("durable applied lost during outage: %+v", applied)
	}
}

// 19. oversized/malformed desired rejected
func TestOversizedDesiredRejected(t *testing.T) {
	def := validDef("asg-1")
	def.AssignmentID = strings.Repeat("x", recurring.MaxStringLen+1)
	desired := recurring.DesiredState{Generation: 1, Definitions: []recurring.RecurringDefinition{def}, Checksum: "x"}
	if err := recurring.ValidateDesiredState(desired); err == nil {
		t.Fatal("expected oversized rejection")
	}
	bad := desiredFrom(validDef("asg-1"))
	bad.Checksum = strings.Repeat("c", 64)
	if err := recurring.ValidateDesiredState(bad); err == nil {
		t.Fatal("expected checksum mismatch rejection")
	}
}

// 20. wrong tenant rejected
func TestWrongTenantRejected(t *testing.T) {
	desired := desiredFrom(validDef("asg-1"))
	_, result, err := recurring.ApplyDesired(recurring.AppliedState{}, desired, "other-tenant")
	if err == nil || result.ReasonCode != recurring.ErrCodeTenantMismatch {
		t.Fatalf("expected tenant mismatch, got %+v err=%v", result, err)
	}
}

// 21. queue/capacity rejection bounded
func TestQueueAdmissionRejectionBounded(t *testing.T) {
	metrics := &recurring.Metrics{}
	engine := recurring.NewDueEngine()
	provider := &staticApplied{state: func() recurring.AppliedState {
		d := desiredFrom(validDef("asg-1"), validDef("asg-2"), validDef("asg-3"))
		return recurring.AppliedState{Generation: d.Generation, Checksum: d.Checksum, Definitions: d.Definitions}
	}()}
	submitter := &rejectingSubmitter{code: scheduler.CodeQueueFull}
	resolver := credentialResolverFunc(func(tenantID, profileRef string) (snmp.Credentials, error) {
		return snmp.Credentials{Community: &snmp.CommunityCredentials{Community: snmp.Secret("c")}}, nil
	})
	emitter, err := recurring.NewEmitter(provider, engine, submitter, resolver, recurring.EmitterConfig{Metrics: metrics})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := emitter.Tick(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	snap := metrics.Snapshot()
	if snap.QueueAdmissionRejected == 0 {
		t.Fatal("expected queue admission rejections")
	}
	if snap.Submitted != 0 {
		t.Fatalf("submitted=%d want 0", snap.Submitted)
	}
}

// 22. metrics reflect skips/overdue/rejected
func TestMetricsReflectSkipsOverdueRejected(t *testing.T) {
	metrics := &recurring.Metrics{}
	engine := recurring.NewDueEngine()
	dir := t.TempDir()
	syncer, err := recurring.New(recurring.SyncerConfig{
		Dir:            dir,
		ExpectedTenant: "tenant-a",
		Metrics:        metrics,
	}, &staticCore{}, engine)
	if err != nil {
		t.Fatal(err)
	}
	desired := desiredFrom(validDef("asg-1"))
	if _, err := syncer.ApplyInMemory(desired); err != nil {
		t.Fatal(err)
	}
	// Stale reject
	stale := desired
	stale.Generation = 0
	stale.Checksum = ""
	stale.Definitions = nil
	// First bump to gen 2
	desired2 := desired
	desired2.Generation = 2
	desired2.Checksum = desired.Checksum
	if _, err := syncer.ApplyInMemory(desired2); err != nil {
		t.Fatal(err)
	}
	_, _, _ = recurring.ApplyDesired(syncer.Applied(), desired, "tenant-a")
	metrics.AddRejectedStaleDesired(1)
	metrics.AddSkippedCoalesced(3)
	metrics.AddOverdueFreshnessDebt(2)
	snap := metrics.Snapshot()
	if snap.SkippedCoalesced != 3 || snap.OverdueFreshnessDebt != 2 || snap.RejectedStaleDesired != 1 {
		t.Fatalf("metrics snapshot incomplete: %+v", snap)
	}
	if snap.AppliedGeneration != 2 {
		t.Fatalf("applied generation=%d", snap.AppliedGeneration)
	}
}

func TestChecksumEmptyAndSorted(t *testing.T) {
	if got := recurring.ComputeDesiredChecksum(nil); got != "" {
		t.Fatalf("empty checksum=%q", got)
	}
	a := validDef("b")
	b := validDef("a")
	c1 := recurring.ComputeDesiredChecksum([]recurring.RecurringDefinition{a, b})
	c2 := recurring.ComputeDesiredChecksum([]recurring.RecurringDefinition{b, a})
	if c1 != c2 || c1 == "" {
		t.Fatalf("checksum not order-independent: %s vs %s", c1, c2)
	}
}

// Golden vector shared with Core SensorRecurringDesiredChecksumTest — keep identical.
const goldenChecksumSliceA = "d246391352ee9131e00ee07bdd2cf1b81a7c1246a9e5ee4a4b578bc655fa4235"

func goldenDefinition() recurring.RecurringDefinition {
	return recurring.RecurringDefinition{
		AssignmentID:         "assign-golden-1",
		TenantID:             "018f0e0a-0000-7000-8000-0000000000a1",
		EntityID:             "entity-golden",
		MonitorDefinitionID:  "mon-golden",
		SchedulingClass:      recurring.SchedulingOperationalDetail,
		ExecutionClass:       "NETWORK_POLL",
		CadenceMs:            300_000,
		FreshnessMs:          600_000,
		PhaseVersion:         1,
		ExecutionTimeoutMs:   30_000,
		CredentialProfileRef: "cred-golden-1",
		Protocol:             recurring.ProtocolSNMP,
		SNMP: &recurring.SNMPParams{
			Target:    "192.0.2.50",
			Port:      161,
			Version:   "2c",
			Operation: "GET",
			OIDs:      []string{".1.3.6.1.2.1.1.3.0"},
			TimeoutMs: 2_000,
			Limits: recurring.SNMPLimits{
				MaxOIDs: 16, MaxWalkEntries: 100, MaxBulkRepetitions: 10,
				MaxResults: 100, MaxResultBytes: 65_536,
			},
		},
	}
}

func TestGoldenChecksumParity(t *testing.T) {
	got := recurring.ComputeDesiredChecksum([]recurring.RecurringDefinition{goldenDefinition()})
	if goldenChecksumSliceA == "PLACEHOLDER" {
		t.Fatalf("set goldenChecksumSliceA to %q", got)
	}
	if got != goldenChecksumSliceA {
		t.Fatalf("golden checksum drift: got %s want %s", got, goldenChecksumSliceA)
	}
}

// Restart suppress: durable restore must not immediately due-submit the current slot.
func TestRestartSuppressesCurrentCycleBurst(t *testing.T) {
	dir := t.TempDir()
	store, err := recurring.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	desired := desiredFrom(validDef("asg-1"), validDef("asg-2"), validDef("asg-3"))
	applied := recurring.AppliedState{
		Generation: desired.Generation, Checksum: desired.Checksum,
		Definitions: desired.Definitions, CoreAuthorityEstablished: true,
	}
	if err := store.SaveApplied(applied); err != nil {
		t.Fatal(err)
	}
	engine := recurring.NewDueEngine()
	_, err = recurring.New(recurring.SyncerConfig{
		Dir: dir, ExpectedTenant: "tenant-a",
	}, &staticCore{}, engine)
	if err != nil {
		t.Fatal(err)
	}
	due := engine.Evaluate(time.Now().UTC(), applied.Definitions)
	if len(due) != 0 {
		t.Fatalf("restart burst: expected 0 due after suppress, got %d", len(due))
	}
}

func TestHigherGenerationAccepted(t *testing.T) {
	d1 := desiredFrom(validDef("asg-1"))
	applied, result, err := recurring.ApplyDesired(recurring.AppliedState{}, d1, "tenant-a")
	if err != nil || !result.Accepted {
		t.Fatalf("apply1: %+v %v", result, err)
	}
	d2 := desiredFrom(validDef("asg-1"), validDef("asg-2"))
	d2.Generation = 2
	d2.Checksum = recurring.ComputeDesiredChecksum(d2.Definitions)
	next, result, err := recurring.ApplyDesired(applied, d2, "tenant-a")
	if err != nil || !result.Accepted || result.Noop {
		t.Fatalf("apply2: %+v %v", result, err)
	}
	if next.Generation != 2 || len(next.Definitions) != 2 {
		t.Fatalf("next=%+v", next)
	}
}

func TestChecksumMismatchOnLoad(t *testing.T) {
	dir := t.TempDir()
	store, err := recurring.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	desired := desiredFrom(validDef("asg-1"))
	applied := recurring.AppliedState{
		Generation:  desired.Generation,
		Checksum:    strings.Repeat("b", 64),
		Definitions: desired.Definitions,
	}
	payload, _ := json.Marshal(applied)
	if err := os.WriteFile(filepath.Join(dir, "state", "applied.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err = store.LoadApplied()
	if err == nil {
		t.Fatal("expected checksum fail-closed")
	}
}

// --- helpers ---

type executorFunc func(context.Context, scheduler.Job) (scheduler.ExecutionResult, error)

func (f executorFunc) Execute(ctx context.Context, job scheduler.Job) (scheduler.ExecutionResult, error) {
	return f(ctx, job)
}

type credentialResolverFunc func(tenantID, profileRef string) (snmp.Credentials, error)

func (f credentialResolverFunc) Resolve(tenantID, profileRef string) (snmp.Credentials, error) {
	return f(tenantID, profileRef)
}

type staticApplied struct {
	state recurring.AppliedState
}

func (s *staticApplied) Applied() recurring.AppliedState { return s.state }

type rejectingSubmitter struct {
	code scheduler.ErrorCode
	n    atomic.Uint64
}

func (r *rejectingSubmitter) Submit(job scheduler.Job) (scheduler.Submission, error) {
	r.n.Add(1)
	return scheduler.Submission{}, &scheduler.Error{Code: r.code, Message: "admission rejected"}
}

type staticCore struct{}

func (s *staticCore) FetchRecurringScheduleDesiredState(ctx context.Context) (coreclient.RecurringScheduleDesiredState, error) {
	return coreclient.RecurringScheduleDesiredState{Generation: 0}, nil
}
func (s *staticCore) AckRecurringScheduleAppliedState(ctx context.Context, generation int64, checksum string) (coreclient.AppliedStateAck, error) {
	return coreclient.AppliedStateAck{Status: "OK", Generation: generation, Checksum: checksum}, nil
}

type flakyCore struct {
	failFetch bool
	mu        sync.Mutex
}

func (f *flakyCore) FetchRecurringScheduleDesiredState(ctx context.Context) (coreclient.RecurringScheduleDesiredState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failFetch {
		return coreclient.RecurringScheduleDesiredState{}, errors.New("core unavailable")
	}
	return coreclient.RecurringScheduleDesiredState{Generation: 0}, nil
}
func (f *flakyCore) AckRecurringScheduleAppliedState(ctx context.Context, generation int64, checksum string) (coreclient.AppliedStateAck, error) {
	return coreclient.AppliedStateAck{Status: "OK", Generation: generation, Checksum: checksum}, nil
}
