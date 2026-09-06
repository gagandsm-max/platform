package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/sensor/internal/scheduler"
)

type fakeScheduler struct {
	submitted atomic.Int64
	shutdown  atomic.Int64
}

func (f *fakeScheduler) Submit(scheduler.Job) (scheduler.Submission, error) {
	f.submitted.Add(1)
	return scheduler.Submission{}, nil
}
func (f *fakeScheduler) Shutdown(context.Context) error {
	f.shutdown.Add(1)
	return nil
}
func (f *fakeScheduler) Snapshot() scheduler.Snapshot {
	return scheduler.Snapshot{State: scheduler.StateRunning, Submitted: uint64(f.submitted.Load())}
}

type executorFunc func(context.Context, scheduler.Job) (scheduler.ExecutionResult, error)

func (f executorFunc) Execute(ctx context.Context, j scheduler.Job) (scheduler.ExecutionResult, error) {
	return f(ctx, j)
}

type fakeComponent struct {
	name       string
	cap        Capability
	readyErr   error
	startErr   error
	started    atomic.Int64
	drained    atomic.Int64
	stopped    atomic.Int64
	canceled   chan struct{}
	cancelOnce sync.Once
}

func (f *fakeComponent) Name() string                 { return f.name }
func (f *fakeComponent) Capability() Capability      { return f.cap }
func (f *fakeComponent) Ready(context.Context) error { return f.readyErr }
func (f *fakeComponent) Drain(context.Context) error { f.drained.Add(1); return nil }
func (f *fakeComponent) Shutdown(context.Context) error {
	f.stopped.Add(1)
	return nil
}
func (f *fakeComponent) Start(ctx context.Context) error {
	f.started.Add(1)
	if f.startErr != nil {
		return f.startErr
	}
	if f.canceled != nil {
		go func() {
			<-ctx.Done()
			f.cancelOnce.Do(func() { close(f.canceled) })
		}()
	}
	return nil
}

func TestEveryRoleHasCapabilities(t *testing.T) {
	for _, role := range []Role{RoleNetwork, RoleInfrastructure, RoleStreamingEvent, RoleSynthetic, RoleCloudAPI, RoleCombined} {
		if err := ValidateRole(role); err != nil {
			t.Fatal(err)
		}
		if len(CapabilitiesForRole(role)) == 0 {
			t.Fatalf("role %s has no capabilities", role)
		}
	}
}

func TestInvalidRoleCapabilityCombination(t *testing.T) {
	if err := ValidateCapabilities(RoleSynthetic, []Capability{CapabilitySNMP}); err == nil {
		t.Fatal("expected role/capability validation error")
	}
}

func TestExecutorRegistryRejectsDuplicateAndClassMismatch(t *testing.T) {
	r := NewExecutorRegistry()
	exec := executorFunc(func(context.Context, scheduler.Job) (scheduler.ExecutionResult, error) {
		return scheduler.ExecutionResult{}, nil
	})
	if err := r.Register(CapabilitySNMP, "SNMP", scheduler.ClassNetworkPoll, exec); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(CapabilitySNMP, "SNMP", scheduler.ClassNetworkPoll, exec); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	if _, err := r.Execute(context.Background(), scheduler.Job{Kind: "SNMP", Class: scheduler.ClassInfra}); err == nil {
		t.Fatal("expected class mismatch")
	}
}

func TestRuntimeStartupAndRepeatedShutdown(t *testing.T) {
	sched := &fakeScheduler{}
	reg := NewExecutorRegistry()
	comp := &fakeComponent{name: "syslog", cap: CapabilitySyslog, canceled: make(chan struct{})}
	rt, err := New(Config{Role: RoleStreamingEvent, EnabledCapabilities: []Capability{CapabilitySyslog}}, sched, reg)
	if err != nil {
		t.Fatal(err)
	}
	if err = rt.RegisterComponent(comp); err != nil {
		t.Fatal(err)
	}
	if err = rt.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := rt.Snapshot().State; got != StateReady {
		t.Fatalf("state=%s, want READY", got)
	}
	if err = rt.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = rt.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sched.shutdown.Load() != 1 {
		t.Fatalf("scheduler shutdown=%d, want 1", sched.shutdown.Load())
	}
	select {
	case <-comp.canceled:
	case <-time.After(time.Second):
		t.Fatal("component did not receive runtime cancellation")
	}
}

func TestDrainPreventsNewWork(t *testing.T) {
	sched := &fakeScheduler{}
	reg := NewExecutorRegistry()
	exec := executorFunc(func(context.Context, scheduler.Job) (scheduler.ExecutionResult, error) {
		return scheduler.ExecutionResult{}, nil
	})
	_ = reg.Register(CapabilitySNMP, "SNMP", scheduler.ClassNetworkPoll, exec)
	rt, _ := New(Config{Role: RoleNetwork, EnabledCapabilities: []Capability{CapabilitySNMP}}, sched, reg)
	if err := rt.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := rt.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Submit(scheduler.Job{Kind: "SNMP", Class: scheduler.ClassNetworkPoll}); err == nil {
		t.Fatal("draining runtime accepted work")
	}
}

func TestDegradedComponent(t *testing.T) {
	sched := &fakeScheduler{}
	reg := NewExecutorRegistry()
	comp := &fakeComponent{name: "syslog", cap: CapabilitySyslog, readyErr: errors.New("listener unavailable")}
	rt, _ := New(Config{Role: RoleStreamingEvent, EnabledCapabilities: []Capability{CapabilitySyslog}}, sched, reg)
	_ = rt.RegisterComponent(comp)
	if err := rt.Start(context.Background()); err == nil {
		t.Fatal("expected degraded startup")
	}
	if got := rt.Snapshot().State; got != StateDegraded {
		t.Fatalf("state=%s, want DEGRADED", got)
	}
	_ = rt.Shutdown(context.Background())
}

func TestConfigVersionTransition(t *testing.T) {
	sched := &fakeScheduler{}
	reg := NewExecutorRegistry()
	exec := executorFunc(func(context.Context, scheduler.Job) (scheduler.ExecutionResult, error) {
		return scheduler.ExecutionResult{}, nil
	})
	_ = reg.Register(CapabilitySNMP, "SNMP", scheduler.ClassNetworkPoll, exec)
	rt, _ := New(Config{Role: RoleNetwork, EnabledCapabilities: []Capability{CapabilitySNMP}}, sched, reg)
	_ = rt.Start(context.Background())

	called := 0
	if err := rt.ApplyConfiguration(context.Background(), 2, "abc", func(context.Context) error {
		called++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	snap := rt.Snapshot()
	if snap.DesiredConfigVersion != 2 || snap.AppliedConfigVersion != 2 || called != 1 {
		t.Fatalf("snapshot=%+v called=%d", snap, called)
	}
	if err := rt.ApplyConfiguration(context.Background(), 1, "old", func(context.Context) error { return nil }); err == nil {
		t.Fatal("stale config accepted")
	}
}

func TestComponentCancellation(t *testing.T) {
	sched := &fakeScheduler{}
	reg := NewExecutorRegistry()
	comp := &fakeComponent{name: "flow", cap: CapabilityFlow, canceled: make(chan struct{})}
	rt, _ := New(Config{Role: RoleStreamingEvent, EnabledCapabilities: []Capability{CapabilityFlow}}, sched, reg)
	_ = rt.RegisterComponent(comp)
	_ = rt.Start(context.Background())
	_ = rt.Shutdown(context.Background())
	select {
	case <-comp.canceled:
	case <-time.After(time.Second):
		t.Fatal("component context not cancelled")
	}
}

func TestSchedulerRegistrationFailure(t *testing.T) {
	sched := &fakeScheduler{}
	reg := NewExecutorRegistry()
	rt, _ := New(Config{Role: RoleNetwork, EnabledCapabilities: []Capability{CapabilitySNMP}}, sched, reg)
	if err := rt.Start(context.Background()); err == nil {
		t.Fatal("expected missing capability implementation error")
	}
	if got := rt.Snapshot().State; got != StateFailed {
		t.Fatalf("state=%s, want FAILED", got)
	}
}

func TestComponentStartFailureShutsDownAlreadyStartedComponents(t *testing.T) {
	sched := &fakeScheduler{}
	reg := NewExecutorRegistry()
	first := &fakeComponent{name: "syslog", cap: CapabilitySyslog}
	second := &fakeComponent{name: "flow", cap: CapabilityFlow, startErr: errors.New("bind failed")}
	rt, _ := New(Config{Role: RoleStreamingEvent, EnabledCapabilities: []Capability{CapabilitySyslog, CapabilityFlow}}, sched, reg)
	_ = rt.RegisterComponent(first)
	_ = rt.RegisterComponent(second)
	if err := rt.Start(context.Background()); err == nil {
		t.Fatal("expected startup failure")
	}
	if first.stopped.Load() != 1 {
		t.Fatalf("first component shutdown count=%d, want 1", first.stopped.Load())
	}
}
