package common

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/collector"
	"platform/agent/common/config"
)

// fakeProcessNetworkRuntime is a test double for optional enrichment lifecycle.
type fakeProcessNetworkRuntime struct {
	mu sync.Mutex

	startGate     chan struct{} // when non-nil, Start blocks until closed or ctx done
	ignoreCancel  bool          // non-cooperative Start: ignore ctx until startGate closes
	startDelay    time.Duration
	startErr      error
	startCalls    atomic.Uint32
	stopCalls     atomic.Uint32
	stopErr       error
	stopDelay     time.Duration

	capability collector.ProcessNetworkCapabilitySnapshot
	support    collector.ProcessNetworkSupportSnapshot
}

func (f *fakeProcessNetworkRuntime) Start(ctx context.Context) error {
	f.startCalls.Add(1)
	if f.startDelay > 0 {
		timer := time.NewTimer(f.startDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !f.ignoreCancel {
				return ctx.Err()
			}
			<-timer.C
		}
	}
	if f.startGate != nil {
		if f.ignoreCancel {
			<-f.startGate
		} else {
			select {
			case <-f.startGate:
			case <-ctx.Done():
				f.mu.Lock()
				f.capability.State = collector.ProcessNetworkCapabilityUnavailable
				f.capability.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonPreflightCancelled}
				f.capability.ObservedAt = time.Now().UTC()
				f.capability.TransitionAt = f.capability.ObservedAt
				f.mu.Unlock()
				return ctx.Err()
			}
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		if f.capability.State == "" || f.capability.State == collector.ProcessNetworkCapabilityActive {
			f.capability.State = collector.ProcessNetworkCapabilityUnavailable
		}
		return f.startErr
	}
	now := time.Now().UTC()
	f.capability.State = collector.ProcessNetworkCapabilityActive
	f.capability.ReasonCodes = nil
	f.capability.ObservedAt = now
	f.capability.TransitionAt = now
	f.capability.LastSuccessAt = now
	f.capability.ValidUntil = now.Add(time.Hour)
	f.capability.PreflightPassed = true
	f.support.Present = true
	return nil
}

func (f *fakeProcessNetworkRuntime) StopContext(ctx context.Context) error {
	f.stopCalls.Add(1)
	if f.stopDelay > 0 {
		timer := time.NewTimer(f.stopDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.capability.State == collector.ProcessNetworkCapabilityActive {
		now := time.Now().UTC()
		f.capability.State = collector.ProcessNetworkCapabilityUnavailable
		f.capability.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonBackendStopped}
		f.capability.ObservedAt = now
		f.capability.TransitionAt = now
	}
	return f.stopErr
}

func (f *fakeProcessNetworkRuntime) Capability() collector.ProcessNetworkCapabilitySnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.capability.Copy()
}

func (f *fakeProcessNetworkRuntime) SupportSnapshot() collector.ProcessNetworkSupportSnapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.support.Copy()
}

func (f *fakeProcessNetworkRuntime) setCapability(state collector.ProcessNetworkCapabilityState, reasons ...collector.ProcessNetworkCapabilityReason) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().UTC()
	f.capability = collector.ProcessNetworkCapabilitySnapshot{
		State:         state,
		ReasonCodes:   append([]collector.ProcessNetworkCapabilityReason(nil), reasons...),
		ObservedAt:    now,
		TransitionAt:  now,
		ValidUntil:    now.Add(time.Hour),
		NextRecheckAt: now.Add(time.Hour),
		SourceQuality: "FIXTURE",
	}
	f.support.Present = true
}

func testEBPFAgentCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg := testAgentConfig(t, "127.0.0.1:0")
	cfg.CollectionInterval = time.Hour
	cfg.MaxBatchBytes = 384 * 1024
	return cfg
}

func waitLifecycle(t *testing.T, svc *AgentService, want LifecycleState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if svc.LifecycleState() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("lifecycle want %s got %s", want, svc.LifecycleState())
}

func waitCapability(t *testing.T, rt *fakeProcessNetworkRuntime, want collector.ProcessNetworkCapabilityState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if rt.Capability().State == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("capability want %s got %s", want, rt.Capability().State)
}

func TestRunningBeforeOptionalBackendActivationCompletes(t *testing.T) {
	gate := make(chan struct{})
	rt := &fakeProcessNetworkRuntime{startGate: gate}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)

	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-run-before"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	startReturned := make(chan struct{})
	go func() {
		if err := svc.Start(context.Background()); err != nil {
			t.Errorf("Start: %v", err)
		}
		close(startReturned)
	}()
	select {
	case <-startReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("AgentService.Start blocked on optional backend activation")
	}
	waitLifecycle(t, svc, LifecycleRunning, time.Second)
	if !svc.SchedulerStatus().Running {
		t.Fatal("scheduler must run before backend activation completes")
	}
	if rt.startCalls.Load() == 0 {
		// activation may not have entered Start yet; give a brief window
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) && rt.startCalls.Load() == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if rt.Capability().State == collector.ProcessNetworkCapabilityActive {
		t.Fatal("backend must not be ACTIVE before gate opens")
	}
	close(gate)
	waitCapability(t, rt, collector.ProcessNetworkCapabilityActive, 2*time.Second)
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestBlockedFakeBackendStartKeepsBaseReady(t *testing.T) {
	gate := make(chan struct{})
	rt := &fakeProcessNetworkRuntime{startGate: gate}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)

	addr := freeLoopbackAddr(t)
	cfg := testEBPFAgentCfg(t)
	cfg.HealthAddress = addr

	svc, err := NewAgentService(cfg, testAgentIdent("agent-blocked-start"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	startAt := time.Now()
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(startAt) > 500*time.Millisecond {
		t.Fatalf("Start took too long with blocked backend: %s", time.Since(startAt))
	}
	waitLifecycle(t, svc, LifecycleRunning, time.Second)
	if !svc.SchedulerStatus().Running {
		t.Fatal("scheduler must remain running")
	}

	resp, err := http.Get("http://" + addr + "/ready")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ready status=%d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["ready"] != true || body["status"] == "NOT_READY" {
		t.Fatalf("blocked eBPF must not make base NOT_READY: %v", body)
	}

	close(gate)
	_ = svc.Stop()
}

func TestBackendStartSuccessDynamicCapabilityActive(t *testing.T) {
	rt := &fakeProcessNetworkRuntime{}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-active"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCapability(t, rt, collector.ProcessNetworkCapabilityActive, 2*time.Second)
	snap := svc.RuntimeSnapshot()
	if snap.ProcessNetworkCapability.State != collector.ProcessNetworkCapabilityActive {
		t.Fatalf("dynamic capability=%s", snap.ProcessNetworkCapability.State)
	}
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestBackendStartFailureKeepsLifecycleRunning(t *testing.T) {
	cases := []struct {
		name   string
		state  collector.ProcessNetworkCapabilityState
		reason collector.ProcessNetworkCapabilityReason
	}{
		{"privilege", collector.ProcessNetworkCapabilityPrivilegeDenied, collector.ReasonPrivilegeDenied},
		{"kernel", collector.ProcessNetworkCapabilityKernelIncompatible, collector.ReasonBTFMissing},
		{"unavailable", collector.ProcessNetworkCapabilityUnavailable, collector.ReasonLoadFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &fakeProcessNetworkRuntime{startErr: errors.New("BACKEND_FIXTURE_FAIL")}
			rt.setCapability(tc.state, tc.reason)
			svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-fail-"+tc.name),
				client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) && rt.startCalls.Load() == 0 {
				time.Sleep(5 * time.Millisecond)
			}
			time.Sleep(50 * time.Millisecond)
			if svc.LifecycleState() != LifecycleRunning {
				t.Fatalf("lifecycle=%s", svc.LifecycleState())
			}
			if !svc.SchedulerStatus().Running {
				t.Fatal("scheduler stopped after backend failure")
			}
			snap := svc.RuntimeSnapshot()
			if snap.ProcessNetworkCapability.State != tc.state {
				t.Fatalf("capability=%s", snap.ProcessNetworkCapability.State)
			}
			if err := svc.Stop(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOptionalProbeUnavailableKeepsPrimaryActive(t *testing.T) {
	rt := &fakeProcessNetworkRuntime{}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-opt-probe"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCapability(t, rt, collector.ProcessNetworkCapabilityActive, 2*time.Second)
	rt.mu.Lock()
	rt.capability.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonOptionalProbeUnavailable}
	rt.support.OptionalProbeAttached = false
	rt.mu.Unlock()
	snap := svc.RuntimeSnapshot()
	if snap.ProcessNetworkCapability.State != collector.ProcessNetworkCapabilityActive {
		t.Fatal("optional probe unavailable must keep ACTIVE")
	}
	if len(snap.ProcessNetworkCapability.ReasonCodes) != 1 ||
		snap.ProcessNetworkCapability.ReasonCodes[0] != collector.ReasonOptionalProbeUnavailable {
		t.Fatalf("reasons=%v", snap.ProcessNetworkCapability.ReasonCodes)
	}
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeFailureAfterActiveUpdatesCapabilityNotLifecycle(t *testing.T) {
	rt := &fakeProcessNetworkRuntime{}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-runtime-fail"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCapability(t, rt, collector.ProcessNetworkCapabilityActive, 2*time.Second)
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonReaderFailed)
	snap := svc.RuntimeSnapshot()
	if snap.ProcessNetworkCapability.State != collector.ProcessNetworkCapabilityUnavailable {
		t.Fatalf("state=%s", snap.ProcessNetworkCapability.State)
	}
	if svc.LifecycleState() != LifecycleRunning {
		t.Fatalf("lifecycle=%s", svc.LifecycleState())
	}
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestEBPFDisabledNoRuntimeActivation(t *testing.T) {
	now := time.Now().UTC()
	capSnap := collector.ProcessNetworkCapabilitySnapshot{
		State:         collector.ProcessNetworkCapabilityUnavailable,
		ReasonCodes:   []collector.ProcessNetworkCapabilityReason{collector.ReasonFeatureDisabled},
		ObservedAt:    now,
		TransitionAt:  now,
		ValidUntil:    now.Add(time.Hour),
		SourceQuality: "DISABLED",
	}
	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-disabled"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkCapability(capSnap))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap := svc.RuntimeSnapshot()
	if snap.ProcessNetworkCapability.State != collector.ProcessNetworkCapabilityUnavailable {
		t.Fatalf("state=%s", snap.ProcessNetworkCapability.State)
	}
	if len(snap.ProcessNetworkCapability.ReasonCodes) != 1 ||
		snap.ProcessNetworkCapability.ReasonCodes[0] != collector.ReasonFeatureDisabled {
		t.Fatalf("reasons=%v", snap.ProcessNetworkCapability.ReasonCodes)
	}
	if snap.ProcessNetworkSupport.Present {
		t.Fatal("disabled path must not invent support counters")
	}
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestDynamicCapabilityNotStaleAfterTransition(t *testing.T) {
	rt := &fakeProcessNetworkRuntime{}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-dyn-cap"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	before := svc.RuntimeSnapshot().ProcessNetworkCapability.Copy()
	if before.State == collector.ProcessNetworkCapabilityActive {
		t.Fatal("pre-start must not be ACTIVE")
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCapability(t, rt, collector.ProcessNetworkCapabilityActive, 2*time.Second)
	after := svc.RuntimeSnapshot().ProcessNetworkCapability
	if after.State != collector.ProcessNetworkCapabilityActive {
		t.Fatalf("after=%s", after.State)
	}
	if before.State == after.State && before.TransitionAt.Equal(after.TransitionAt) {
		t.Fatal("capability endpoint must not return stale pre-activation copy")
	}
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestCapabilityCallerMutationCannotMutateBackend(t *testing.T) {
	rt := &fakeProcessNetworkRuntime{}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-copy"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCapability(t, rt, collector.ProcessNetworkCapabilityActive, 2*time.Second)
	snap := svc.RuntimeSnapshot()
	snap.ProcessNetworkCapability.State = collector.ProcessNetworkCapabilityPrivilegeDenied
	snap.ProcessNetworkCapability.ReasonCodes = []collector.ProcessNetworkCapabilityReason{collector.ReasonPrivilegeDenied}
	snap.ProcessNetworkSupport.MapUpdateFail = 999
	again := svc.RuntimeSnapshot()
	if again.ProcessNetworkCapability.State != collector.ProcessNetworkCapabilityActive {
		t.Fatal("caller mutation mutated backend-owned capability")
	}
	if again.ProcessNetworkSupport.MapUpdateFail == 999 {
		t.Fatal("caller mutation mutated backend-owned support counters")
	}
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
}

// cancelOrderRuntime exposes the P1-A cancel-before-StopContext defect: if the
// Agent cancels the shared runtime before StopContext, pending enrichment flush
// cannot install a live stop context.
type cancelOrderRuntime struct {
	fakeProcessNetworkRuntime
	agentCtx               context.Context
	canceledBeforeStop     atomic.Bool
	stopObservedLiveRunCtx atomic.Bool
}

func (f *cancelOrderRuntime) Start(ctx context.Context) error {
	f.agentCtx = ctx
	go func() {
		<-ctx.Done()
		if f.stopCalls.Load() == 0 {
			f.canceledBeforeStop.Store(true)
		}
	}()
	return f.fakeProcessNetworkRuntime.Start(ctx)
}

func (f *cancelOrderRuntime) StopContext(ctx context.Context) error {
	if f.agentCtx != nil && f.agentCtx.Err() == nil {
		f.stopObservedLiveRunCtx.Store(true)
	}
	return f.fakeProcessNetworkRuntime.StopContext(ctx)
}

func TestAgentStopInstallsBackendFlushBeforeRuntimeCancel(t *testing.T) {
	rt := &cancelOrderRuntime{}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-cancel-order"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCapability(t, &rt.fakeProcessNetworkRuntime, collector.ProcessNetworkCapabilityActive, 2*time.Second)
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if rt.stopCalls.Load() != 1 {
		t.Fatalf("backend StopContext calls=%d", rt.stopCalls.Load())
	}
	if rt.canceledBeforeStop.Load() {
		t.Fatal("P1-A regression: Agent canceled shared runtime before Backend.StopContext")
	}
	if !rt.stopObservedLiveRunCtx.Load() {
		t.Fatal("Backend.StopContext must observe a live Agent runtime to install shutdown flush")
	}
	if svc.LifecycleState() != LifecycleStopped {
		t.Fatalf("lifecycle=%s", svc.LifecycleState())
	}
}

// activeBeforeDoneRuntime reproduces ACTIVE publication before activationDone
// closes: Start publishes ACTIVE, signals a barrier, then blocks until released.
// Agent.Stop must not cancel accepting workers before StopContext.
type activeBeforeDoneRuntime struct {
	fakeProcessNetworkRuntime
	activePublished        chan struct{}
	releaseStart           chan struct{}
	agentCtx               context.Context
	canceledBeforeStop     atomic.Bool
	stopObservedLiveRunCtx atomic.Bool
}

func (f *activeBeforeDoneRuntime) Start(ctx context.Context) error {
	f.startCalls.Add(1)
	f.agentCtx = ctx
	go func() {
		<-ctx.Done()
		if f.stopCalls.Load() == 0 {
			f.canceledBeforeStop.Store(true)
		}
	}()
	now := time.Now().UTC()
	f.mu.Lock()
	f.capability.State = collector.ProcessNetworkCapabilityActive
	f.capability.ReasonCodes = nil
	f.capability.ObservedAt = now
	f.capability.TransitionAt = now
	f.capability.LastSuccessAt = now
	f.capability.ValidUntil = now.Add(time.Hour)
	f.capability.PreflightPassed = true
	f.support.Present = true
	f.mu.Unlock()
	if f.activePublished != nil {
		close(f.activePublished)
	}
	if f.releaseStart != nil {
		select {
		case <-f.releaseStart:
		case <-ctx.Done():
			// Accepting workers must not be canceled solely because activationDone
			// is still open; if this fires before StopContext, the race regressed.
			return ctx.Err()
		}
	}
	return nil
}

func (f *activeBeforeDoneRuntime) StopContext(ctx context.Context) error {
	if f.agentCtx != nil && f.agentCtx.Err() == nil {
		f.stopObservedLiveRunCtx.Store(true)
	}
	return f.fakeProcessNetworkRuntime.StopContext(ctx)
}

func TestActiveBeforeActivationDoneStopDoesNotCancelWorkers(t *testing.T) {
	activePublished := make(chan struct{})
	releaseStart := make(chan struct{})
	rt := &activeBeforeDoneRuntime{
		activePublished: activePublished,
		releaseStart:    releaseStart,
	}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)

	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-active-before-done"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-activePublished:
	case <-time.After(2 * time.Second):
		t.Fatal("ACTIVE was not published before activationDone barrier")
	}
	// activationDone is still open (Start blocked on releaseStart).
	stopDone := make(chan error, 1)
	go func() { stopDone <- svc.Stop() }()

	// Allow Stop to observe open activationDone + ACTIVE and choose cancel policy.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && rt.stopCalls.Load() == 0 && !rt.canceledBeforeStop.Load() {
		time.Sleep(5 * time.Millisecond)
	}
	if rt.canceledBeforeStop.Load() && rt.stopCalls.Load() == 0 {
		close(releaseStart)
		<-stopDone
		t.Fatal("P1 activation race: Stop canceled accepting workers before StopContext")
	}
	close(releaseStart)
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop deadlocked across ACTIVE-before-activationDone barrier")
	}
	if rt.canceledBeforeStop.Load() {
		t.Fatal("Stop must not cancel runtime before StopContext when backend already ACTIVE")
	}
	if !rt.stopObservedLiveRunCtx.Load() {
		t.Fatal("StopContext must observe live Agent runtime for shutdown flush")
	}
	if rt.stopCalls.Load() != 1 {
		t.Fatalf("StopContext calls=%d", rt.stopCalls.Load())
	}
}

func TestTypedBackendShutdownFlushSurvivesAgentStop(t *testing.T) {
	flushErr := errors.New("BACKEND_SHUTDOWN_FLUSH_INCOMPLETE: fixture sink blocked")
	rt := &fakeProcessNetworkRuntime{stopErr: flushErr}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-typed-flush"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	svc.setShutdownBudgetForTest(2 * time.Second)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCapability(t, rt, collector.ProcessNetworkCapabilityActive, 2*time.Second)
	err = svc.Stop()
	if err == nil {
		t.Fatal("expected shutdown flush failure")
	}
	if !errors.Is(err, flushErr) {
		t.Fatalf("typed flush error must survive AgentService.Stop, got %v", err)
	}
	if errors.Is(err, ErrShutdownTimeout) {
		t.Fatal("non-timeout sink/flush failure must not be mislabeled ErrShutdownTimeout")
	}
	if svc.LifecycleState() != LifecycleFailed {
		t.Fatalf("lifecycle=%s", svc.LifecycleState())
	}
}

func TestProtectedTeardownUnderOptionalBudgetExhaustion(t *testing.T) {
	gate := make(chan struct{})
	rt := &fakeProcessNetworkRuntime{startGate: gate, ignoreCancel: true}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	addr := freeLoopbackAddr(t)
	cfg := testEBPFAgentCfg(t)
	cfg.HealthAddress = addr
	svc, err := NewAgentService(cfg, testAgentIdent("agent-optional-budget"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	svc.setShutdownBudgetForTest(200 * time.Millisecond)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && rt.startCalls.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	err = svc.Stop()
	if err == nil || !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("expected activation settle timeout, got %v", err)
	}
	// Even when activation settle times out, StopContext must still be requested
	// so a late Start commit cannot orphan detached workers.
	if rt.stopCalls.Load() != 1 {
		t.Fatalf("StopContext must run despite unsettled activation, calls=%d", rt.stopCalls.Load())
	}
	if !svc.scheduler.AdmissionClosedForTest() {
		t.Fatal("protected scheduler teardown must run after optional budget exhaustion")
	}
	httpClient := &http.Client{Timeout: 100 * time.Millisecond}
	if resp, liveErr := httpClient.Get("http://" + addr + "/live"); liveErr == nil {
		_ = resp.Body.Close()
		t.Fatal("protected health teardown must run after optional budget exhaustion")
	}
	close(gate)
}

func TestClassifyAgentStopErrorDistinguishesCancelFromTimeout(t *testing.T) {
	deadlineCtx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()
	canceled := context.Canceled
	got := classifyAgentStopError(deadlineCtx, canceled)
	if errors.Is(got, ErrShutdownTimeout) {
		t.Fatalf("bare Canceled must not be labeled timeout: %v", got)
	}
	if !errors.Is(got, context.Canceled) {
		t.Fatalf("Canceled identity must be preserved: %v", got)
	}

	expired, expireCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Millisecond))
	expireCancel()
	got = classifyAgentStopError(expired, errors.New("backend flush incomplete"))
	if !errors.Is(got, ErrShutdownTimeout) {
		t.Fatalf("deadline exhaustion must wrap ErrShutdownTimeout: %v", got)
	}
}

func TestStopDuringOptionalActivationKeepsProtectedTeardown(t *testing.T) {
	gate := make(chan struct{})
	rt := &fakeProcessNetworkRuntime{startGate: gate}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	addr := freeLoopbackAddr(t)
	cfg := testEBPFAgentCfg(t)
	cfg.HealthAddress = addr
	svc, err := NewAgentService(cfg, testAgentIdent("agent-stop-activating"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	svc.setShutdownBudgetForTest(2 * time.Second)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && rt.startCalls.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	stopDone := make(chan error, 1)
	go func() { stopDone <- svc.Stop() }()
	time.Sleep(50 * time.Millisecond)
	close(gate)
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop deadlocked while optional activation in progress")
	}
	if svc.LifecycleState() != LifecycleStopped {
		t.Fatalf("lifecycle=%s", svc.LifecycleState())
	}
	_, err = http.Get("http://" + addr + "/live")
	if err == nil {
		t.Fatal("health must stop after protected teardown")
	}
}

func TestResourceGuardedCapabilityDoesNotForceNotReady(t *testing.T) {
	rt := &fakeProcessNetworkRuntime{}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	addr := freeLoopbackAddr(t)
	cfg := testEBPFAgentCfg(t)
	cfg.HealthAddress = addr
	svc, err := NewAgentService(cfg, testAgentIdent("agent-guarded-ready"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCapability(t, rt, collector.ProcessNetworkCapabilityActive, 2*time.Second)
	rt.setCapability(collector.ProcessNetworkCapabilityResourceGuarded, collector.ReasonResourceLimit)
	resp, err := http.Get("http://" + addr + "/ready")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		t.Fatal("RESOURCE_GUARDED must not force base Agent NOT_READY")
	}
	snap := svc.RuntimeSnapshot()
	if snap.ProcessNetworkCapability.State != collector.ProcessNetworkCapabilityResourceGuarded {
		t.Fatalf("capability=%s", snap.ProcessNetworkCapability.State)
	}
	if svc.LifecycleState() != LifecycleRunning {
		t.Fatalf("lifecycle=%s", svc.LifecycleState())
	}
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestStopWhileBackendActivationRunning(t *testing.T) {
	gate := make(chan struct{})
	rt := &fakeProcessNetworkRuntime{startGate: gate}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-stop-during"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	svc.setShutdownBudgetForTest(2 * time.Second)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && rt.startCalls.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	stopDone := make(chan error, 1)
	go func() { stopDone <- svc.Stop() }()
	// Allow Stop to cancel runtime, then release activation so Start observes cancel.
	time.Sleep(50 * time.Millisecond)
	close(gate)
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Stop deadlocked while activation in progress")
	}
	if rt.stopCalls.Load() != 1 {
		t.Fatalf("backend StopContext calls=%d", rt.stopCalls.Load())
	}
}

func TestNonCooperativeActivationStopTimeout(t *testing.T) {
	gate := make(chan struct{})
	rt := &fakeProcessNetworkRuntime{startGate: gate, ignoreCancel: true}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	addr := freeLoopbackAddr(t)
	cfg := testEBPFAgentCfg(t)
	cfg.HealthAddress = addr
	svc, err := NewAgentService(cfg, testAgentIdent("agent-noncoop"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	// Whole-service budget; activation settle is a subordinate half-window.
	svc.setShutdownBudgetForTest(300 * time.Millisecond)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && rt.startCalls.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	httpClient := &http.Client{Timeout: 100 * time.Millisecond}
	liveBefore, liveErr := httpClient.Get("http://" + addr + "/live")
	if liveErr != nil {
		t.Fatalf("health must be up before Stop: %v", liveErr)
	}
	_ = liveBefore.Body.Close()

	start := time.Now()
	err = svc.Stop()
	elapsed := time.Since(start)
	if err == nil || !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("expected bounded shutdown timeout, got %v", err)
	}
	if elapsed > time.Second {
		t.Fatalf("Stop hung too long: %s", elapsed)
	}
	if svc.LifecycleState() != LifecycleFailed {
		t.Fatalf("lifecycle=%s", svc.LifecycleState())
	}
	// Unsettled activation must still request StopContext so a late Start commit
	// cannot orphan detached workers (Backend.stopRequested ownership).
	if rt.stopCalls.Load() != 1 {
		t.Fatalf("StopContext must run despite unsettled activation, calls=%d", rt.stopCalls.Load())
	}
	// Protected base teardown must continue after activation sub-budget expiry.
	// Stop only returns after the scheduler+worker WaitGroup and health.Stop.
	if !svc.scheduler.AdmissionClosedForTest() {
		t.Fatal("scheduler stop must be attempted after activation timeout")
	}
	if svc.SchedulerStatus().Running {
		t.Fatal("scheduler must not remain running after stop attempt")
	}
	workerCtx, workerCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	workerErr := svc.worker.StopContext(workerCtx)
	workerCancel()
	if workerErr != nil {
		t.Fatalf("worker stop must have been attempted during AgentService.Stop: %v", workerErr)
	}
	liveAfter, liveAfterErr := httpClient.Get("http://" + addr + "/live")
	if liveAfterErr == nil {
		_ = liveAfter.Body.Close()
		t.Fatal("health stop must be attempted last after activation timeout")
	}
	close(gate) // unblock leaked activation for test process exit
	time.Sleep(20 * time.Millisecond)
}

func TestSuccessfulStopInvokesBackendCleanupOnce(t *testing.T) {
	rt := &fakeProcessNetworkRuntime{}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-stop-once"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCapability(t, rt, collector.ProcessNetworkCapabilityActive, 2*time.Second)
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
	if rt.stopCalls.Load() != 1 {
		t.Fatalf("stop calls=%d", rt.stopCalls.Load())
	}
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
	if rt.stopCalls.Load() != 1 {
		t.Fatalf("idempotent Stop must not re-stop backend: %d", rt.stopCalls.Load())
	}
}

func TestBackendStopTimeoutSurfacedInAgentShutdown(t *testing.T) {
	rt := &fakeProcessNetworkRuntime{stopDelay: time.Second, stopErr: errors.New("BACKEND_STOP_DEADLINE")}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-stop-to"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	svc.setShutdownBudgetForTest(100 * time.Millisecond)
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCapability(t, rt, collector.ProcessNetworkCapabilityActive, 2*time.Second)
	err = svc.Stop()
	if err == nil || !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("expected shutdown timeout surfacing backend stop failure, got %v", err)
	}
	if svc.LifecycleState() != LifecycleFailed {
		t.Fatalf("lifecycle=%s", svc.LifecycleState())
	}
}

func TestHealthServerStoppedAfterRuntimeComponents(t *testing.T) {
	rt := &fakeProcessNetworkRuntime{}
	rt.setCapability(collector.ProcessNetworkCapabilityUnavailable, collector.ReasonBackendNotWired)
	addr := freeLoopbackAddr(t)
	cfg := testEBPFAgentCfg(t)
	cfg.HealthAddress = addr

	svc, err := NewAgentService(cfg, testAgentIdent("agent-health-last"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitCapability(t, rt, collector.ProcessNetworkCapabilityActive, 2*time.Second)
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
	if rt.stopCalls.Load() != 1 {
		t.Fatal("backend must stop before health teardown completes")
	}
	_, err = http.Get("http://" + addr + "/live")
	if err == nil {
		t.Fatal("health server must be stopped after runtime shutdown")
	}
}

func TestSchedulerWorkerUnchangedByBackendActivationFailure(t *testing.T) {
	rt := &fakeProcessNetworkRuntime{startErr: errors.New("BACKEND_FIXTURE_FAIL")}
	rt.setCapability(collector.ProcessNetworkCapabilityPrivilegeDenied, collector.ReasonPrivilegeDenied)
	svc, err := NewAgentService(testEBPFAgentCfg(t), testAgentIdent("agent-sched-ok"),
		client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && rt.startCalls.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	st := svc.SchedulerStatus()
	if !st.Running || st.EnabledCollectors <= 0 {
		t.Fatalf("scheduler degraded unexpectedly: %+v", st)
	}
	d := svc.DeliveryStats()
	if d.ReceiverAvailable {
		t.Fatal("delivery worker wiring changed unexpectedly")
	}
	if err := svc.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessNetworkStillUnwiredFromScalarPath(t *testing.T) {
	rec := collector.ProcessNetworkEvidence{
		SchemaVersion:    collector.ProcessNetworkSchemaVersion,
		SourceProvenance: collector.ProcessNetworkEBPFSourceProvenance,
		BucketStartAt:    time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC),
		BucketEndAt:      time.Date(2026, 10, 9, 12, 1, 0, 0, time.UTC),
		BucketSeconds:    60,
		Workload: collector.ProcessNetworkWorkloadIdentity{
			Kind:        collector.SelectorKindProcessName,
			Value:       "nginx",
			ProcessName: "nginx",
		},
		RemoteIP:   netip.MustParseAddr("192.0.2.1"),
		RemotePort: 80,
		Protocol:   collector.ProcessNetworkProtocolTCP,
		IPVersion:  "IPv4",
		Quality:    collector.ProcessNetworkQualityValid,
	}
	samples, encErr := collector.EncodeProcessNetworkEvidenceAsScalarSamples(rec)
	if samples != nil || encErr == nil || !strings.Contains(encErr.Error(), "PROCESS_NETWORK_SCALAR_ENCODING_REJECTED") {
		t.Fatalf("scalar path must remain rejected: %v %v", samples, encErr)
	}
	var sink collector.UnwiredStructuredEvidenceSink
	err := sink.AcceptProcessNetworkEvidence(context.Background(), rec)
	if !errors.Is(err, collector.ErrStructuredTransportNotWired) {
		t.Fatalf("expected TRANSPORT_NOT_WIRED, got %v", err)
	}
}

func TestEBPFReadinessNonInterferenceMatrix(t *testing.T) {
	states := []struct {
		state  collector.ProcessNetworkCapabilityState
		reason collector.ProcessNetworkCapabilityReason
	}{
		{collector.ProcessNetworkCapabilityUnavailable, collector.ReasonFeatureDisabled},
		{collector.ProcessNetworkCapabilityPrivilegeDenied, collector.ReasonPrivilegeDenied},
		{collector.ProcessNetworkCapabilityKernelIncompatible, collector.ReasonBTFMissing},
		{collector.ProcessNetworkCapabilityUnavailable, collector.ReasonLoadFailed},
	}
	for _, tc := range states {
		t.Run(string(tc.state)+"_"+string(tc.reason), func(t *testing.T) {
			rt := &fakeProcessNetworkRuntime{startErr: errors.New("fail")}
			rt.setCapability(tc.state, tc.reason)
			addr := freeLoopbackAddr(t)
			cfg := testEBPFAgentCfg(t)
			cfg.HealthAddress = addr
			svc, err := NewAgentService(cfg, testAgentIdent("agent-ready-"+string(tc.reason)),
				client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, WithProcessNetworkRuntime(rt))
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			time.Sleep(80 * time.Millisecond)
			resp, err := http.Get("http://" + addr + "/ready")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusServiceUnavailable {
				t.Fatalf("eBPF %s must not force NOT_READY", tc.state)
			}
			_ = svc.Stop()
		})
	}
}
