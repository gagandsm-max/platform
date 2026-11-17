package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/agent/common"
	"platform/agent/common/config"
	"platform/agent/common/identity"
)

type fakeLifecycle struct {
	startDelay   time.Duration
	startErr     error
	stopDelay    time.Duration
	stopErr      error
	startGate    chan struct{}
	stopGate     chan struct{}
	startCalls   atomic.Int32
	stopCalls    atomic.Int32
	blockStopFor time.Duration
}

func (f *fakeLifecycle) Start(ctx context.Context) error {
	f.startCalls.Add(1)
	if f.startGate != nil {
		select {
		case <-f.startGate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.startDelay > 0 {
		select {
		case <-time.After(f.startDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.startErr
}

func (f *fakeLifecycle) Stop() error {
	f.stopCalls.Add(1)
	if f.stopGate != nil {
		<-f.stopGate
	}
	if f.blockStopFor > 0 {
		time.Sleep(f.blockStopFor)
	}
	if f.stopDelay > 0 {
		time.Sleep(f.stopDelay)
	}
	return f.stopErr
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestSCMBridgeReservesTerminalStatusForSvcWrapper(t *testing.T) {
	for _, tc := range []struct {
		state uint32
		want  bool
	}{
		{scmStateStartPending, true},
		{scmStateRunning, true},
		{scmStateStopPending, true},
		{scmStateStopped, false},
	} {
		if got := shouldForwardSCMStatus(scmStatus{State: tc.state}); got != tc.want {
			t.Errorf("state=%d forward=%v want %v", tc.state, got, tc.want)
		}
	}
}

func TestSCMPendingDeadlinePrecedesWaitHint(t *testing.T) {
	for _, hint := range []time.Duration{time.Millisecond, 100 * time.Millisecond, defaultStartWaitHint, defaultStopWaitHint} {
		deadline := pendingDeadline(hint)
		if deadline <= 0 || deadline >= hint {
			t.Fatalf("wait hint=%s deadline=%s must be positive and earlier", hint, deadline)
		}
	}
}

func TestSCMAdapterHungStartupExitsWithServiceFailure(t *testing.T) {
	startGate := make(chan struct{})
	defer close(startGate)
	life := &fakeLifecycle{startGate: startGate}
	adapter := newSCMAdapter(life, testLogger())
	adapter.startWaitHint = 120 * time.Millisecond

	statuses := make(chan scmStatus, 8)
	started := time.Now()
	specific, errno := adapter.runSCMSession(make(chan scmRequest), statuses)
	close(statuses)
	if time.Since(started) >= time.Second {
		t.Fatal("startup must fail within the SCM wait hint budget")
	}
	if !specific || errno != exitCodeStartFailed {
		t.Fatalf("startup timeout classification: specific=%v errno=%d", specific, errno)
	}
	var saw []scmStatus
	for status := range statuses {
		saw = append(saw, status)
		if status.State == scmStateRunning {
			t.Fatal("hung startup must never report RUNNING")
		}
	}
	assertTransition(t, saw, scmStateStartPending, scmStateStopped)
	if last := saw[len(saw)-1]; last.Win32ExitCode != win32ErrorServiceSpecificError || last.ServiceSpecificExitCode != exitCodeStartFailed {
		t.Fatalf("startup timeout status=%+v", last)
	}
}

func TestSCMAdapterHungFactoryExitsWithServiceFailure(t *testing.T) {
	factoryGate := make(chan struct{})
	defer close(factoryGate)
	adapter := newSCMAdapterWithFactory(func() (agentLifecycle, uint32, error) {
		<-factoryGate
		return &fakeLifecycle{}, 0, nil
	}, testLogger())
	adapter.startWaitHint = 120 * time.Millisecond

	statuses := make(chan scmStatus, 8)
	specific, errno := adapter.runSCMSession(make(chan scmRequest), statuses)
	close(statuses)
	if !specific || errno != exitCodeStartFailed {
		t.Fatalf("factory timeout classification: specific=%v errno=%d", specific, errno)
	}
	var saw []scmStatus
	for status := range statuses {
		saw = append(saw, status)
	}
	assertTransition(t, saw, scmStateStartPending, scmStateStopped)
	if adapter.stopCallCount() != 0 {
		t.Fatal("unconstructed service cannot be stopped")
	}
}

func TestSCMAdapterHungStopExitsWithServiceFailure(t *testing.T) {
	stopGate := make(chan struct{})
	defer close(stopGate)
	life := &fakeLifecycle{stopGate: stopGate}
	adapter := newSCMAdapter(life, testLogger())
	adapter.stopWaitHint = 120 * time.Millisecond

	requests := make(chan scmRequest, 2)
	statuses := make(chan scmStatus, 8)
	done := make(chan struct{})
	var specific bool
	var errno uint32
	go func() {
		defer close(done)
		specific, errno = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()
	saw := drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		if st.State == scmStateRunning {
			send <- scmRequest{Cmd: scmCmdStop}
		}
	}, requests)
	if !specific || errno != exitCodeStopFailed {
		t.Fatalf("stop timeout classification: specific=%v errno=%d", specific, errno)
	}
	assertTransition(t, saw, scmStateStartPending, scmStateRunning, scmStateStopPending, scmStateStopped)
	if last := saw[len(saw)-1]; last.Win32ExitCode != win32ErrorServiceSpecificError || last.ServiceSpecificExitCode != exitCodeStopFailed {
		t.Fatalf("stop timeout status=%+v", last)
	}
	if adapter.stopCallCount() != 1 || life.stopCalls.Load() != 1 {
		t.Fatalf("stop must be invoked exactly once, adapter=%d lifecycle=%d", adapter.stopCallCount(), life.stopCalls.Load())
	}
}

func TestSCMAdapterStartStopLifecycle(t *testing.T) {
	life := &fakeLifecycle{}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest, 4)
	statuses := make(chan scmStatus, 32)

	var specific bool
	var errno uint32
	done := make(chan struct{})
	go func() {
		defer close(done)
		specific, errno = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	saw := drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		if st.State == scmStateRunning {
			if st.Accepts&scmAcceptStop == 0 || st.Accepts&scmAcceptShutdown == 0 {
				t.Fatalf("running accepts=%d want STOP|SHUTDOWN", st.Accepts)
			}
			send <- scmRequest{Cmd: scmCmdStop}
		}
	}, requests)

	if specific || errno != 0 {
		t.Fatalf("exit specific=%v errno=%d", specific, errno)
	}
	if life.startCalls.Load() != 1 || life.stopCalls.Load() != 1 {
		t.Fatalf("start/stop calls=%d/%d", life.startCalls.Load(), life.stopCalls.Load())
	}
	assertTransition(t, saw, scmStateStartPending, scmStateRunning, scmStateStopPending, scmStateStopped)
}

func TestSCMAdapterShutdownControl(t *testing.T) {
	life := &fakeLifecycle{}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest, 2)
	statuses := make(chan scmStatus, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	saw := drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		if st.State == scmStateRunning {
			send <- scmRequest{Cmd: scmCmdShutdown}
		}
	}, requests)

	assertTransition(t, saw, scmStateStartPending, scmStateRunning, scmStateStopPending, scmStateStopped)
	if life.stopCalls.Load() != 1 {
		t.Fatalf("stop calls=%d", life.stopCalls.Load())
	}
}

func TestSCMAdapterStartFailure(t *testing.T) {
	life := &fakeLifecycle{startErr: errors.New("AGENT_SERVICE_START_FAILED: boom")}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest)
	statuses := make(chan scmStatus, 8)
	specific, errno := adapter.runSCMSession(requests, statuses)
	close(statuses)

	saw := make([]scmStatus, 0, 4)
	for st := range statuses {
		saw = append(saw, st)
	}

	if !specific || errno != exitCodeStartFailed {
		t.Fatalf("want service-specific start failure, got specific=%v errno=%d", specific, errno)
	}
	if life.stopCalls.Load() != 0 {
		t.Fatalf("Stop must not run after start failure; calls=%d", life.stopCalls.Load())
	}
	if len(saw) < 2 || saw[0].State != scmStateStartPending || saw[len(saw)-1].State != scmStateStopped {
		t.Fatalf("unexpected statuses: %+v", saw)
	}
	last := saw[len(saw)-1]
	if last.ServiceSpecificExitCode != exitCodeStartFailed || last.Win32ExitCode != win32ErrorServiceSpecificError {
		t.Fatalf("exit codes=%+v", last)
	}
}

func TestSCMAdapterRepeatedStopExactlyOnce(t *testing.T) {
	stopGate := make(chan struct{})
	life := &fakeLifecycle{stopGate: stopGate}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest, 8)
	statuses := make(chan scmStatus, 32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	pendingSeen := false
	saw := drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		switch st.State {
		case scmStateRunning:
			send <- scmRequest{Cmd: scmCmdStop}
		case scmStateStopPending:
			if pendingSeen {
				return
			}
			pendingSeen = true
			send <- scmRequest{Cmd: scmCmdStop}
			send <- scmRequest{Cmd: scmCmdShutdown}
			send <- scmRequest{Cmd: scmCmdStop}
			close(stopGate)
		}
	}, requests)

	assertHasStates(t, saw, scmStateRunning, scmStateStopPending, scmStateStopped)
	if life.stopCalls.Load() != 1 {
		t.Fatalf("exactly-once Stop violated: calls=%d", life.stopCalls.Load())
	}
	if adapter.stopCallCount() != 1 {
		t.Fatalf("adapter stopCallCount=%d", adapter.stopCallCount())
	}
}

func TestSCMAdapterStopPendingWaitHint(t *testing.T) {
	stopGate := make(chan struct{})
	life := &fakeLifecycle{stopGate: stopGate}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest, 2)
	statuses := make(chan scmStatus, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	var pending []scmStatus
	_ = drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		switch st.State {
		case scmStateRunning:
			send <- scmRequest{Cmd: scmCmdStop}
		case scmStateStopPending:
			pending = append(pending, st)
			if len(pending) == 1 {
				close(stopGate)
			}
		}
	}, requests)

	if len(pending) == 0 {
		t.Fatal("expected STOP_PENDING status")
	}
	// Only interrogate echoes may republish the same pending status; no timer
	// advances CheckPoint. The first STOP_PENDING must carry the stop WaitHint.
	first := pending[0]
	if first.WaitHintMs == 0 {
		t.Fatal("STOP_PENDING wait hint must be non-zero")
	}
	wantHint := uint32(adapter.stopWaitHint.Milliseconds())
	if first.WaitHintMs != wantHint {
		t.Fatalf("STOP_PENDING WaitHintMs=%d want %d", first.WaitHintMs, wantHint)
	}
	if first.CheckPoint != 1 {
		t.Fatalf("initial STOP_PENDING CheckPoint=%d want 1", first.CheckPoint)
	}
}

func TestSCMAdapterDefaultStopWaitHintMatchesStopBudget(t *testing.T) {
	adapter := newSCMAdapter(&fakeLifecycle{}, testLogger())
	if adapter.stopWaitHint < agentServiceStopBudget {
		t.Fatalf("stop WaitHint %s must cover AgentService.Stop budget %s", adapter.stopWaitHint, agentServiceStopBudget)
	}
	if adapter.stopWaitHint != defaultStopWaitHint {
		t.Fatalf("stop WaitHint=%s want default %s", adapter.stopWaitHint, defaultStopWaitHint)
	}
	if defaultStopWaitHint != agentServiceStopBudget+stopWaitHintMargin {
		t.Fatalf("defaultStopWaitHint=%s want budget+margin", defaultStopWaitHint)
	}
}

func TestSCMAdapterStopDuringStart(t *testing.T) {
	startGate := make(chan struct{})
	life := &fakeLifecycle{startGate: startGate}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest, 2)
	statuses := make(chan scmStatus, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	// Deterministic sync boundary: send STOP, then wait until the adapter
	// publishes STOP_PENDING (STOP acceptance) before releasing Start.
	stopSent := false
	gateReleased := false
	saw := drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		switch {
		case st.State == scmStateStartPending && !stopSent:
			stopSent = true
			send <- scmRequest{Cmd: scmCmdStop}
		case st.State == scmStateStopPending && stopSent && !gateReleased:
			gateReleased = true
			close(startGate)
		}
	}, requests)

	if !stopSent || !gateReleased {
		t.Fatalf("sync boundary missed: stopSent=%v gateReleased=%v saw=%v", stopSent, gateReleased, statesOnly(saw))
	}
	for _, s := range saw {
		if s.State == scmStateRunning {
			t.Fatalf("unexpected RUNNING after stop-during-start: %+v", statesOnly(saw))
		}
	}
	assertHasStates(t, saw, scmStateStartPending, scmStateStopPending, scmStateStopped)
	if life.stopCalls.Load() != 1 {
		t.Fatalf("stop calls=%d", life.stopCalls.Load())
	}
}

func TestSCMAdapterStalledStartupDoesNotClaimProgress(t *testing.T) {
	startGate := make(chan struct{})
	life := &fakeLifecycle{startGate: startGate}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest, 4)
	statuses := make(chan scmStatus, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	// Observe START_PENDING for longer than the former 1s checkpoint interval
	// would have required to invent progress, then interrogate and release.
	stallFor := 150 * time.Millisecond

	var pending []scmStatus
	var saw []scmStatus
	deadline := time.After(3 * time.Second)
	phase := 0
	for {
		select {
		case st, ok := <-statuses:
			if !ok {
				goto finishedStartStall
			}
			saw = append(saw, st)
			switch st.State {
			case scmStateStartPending:
				pending = append(pending, st)
				if phase == 0 {
					phase = 1
					go func() {
						time.Sleep(stallFor)
						requests <- scmRequest{Cmd: scmCmdInterrogate}
					}()
				} else if phase == 1 {
					phase = 2
					go func() {
						time.Sleep(stallFor)
						close(startGate)
					}()
				}
			case scmStateRunning:
				if phase == 2 {
					phase = 3
					requests <- scmRequest{Cmd: scmCmdStop}
				}
			}
		case <-done:
			for {
				select {
				case st, ok := <-statuses:
					if !ok {
						goto finishedStartStall
					}
					saw = append(saw, st)
					if st.State == scmStateStartPending {
						pending = append(pending, st)
					}
				default:
					goto finishedStartStall
				}
			}
		case <-deadline:
			t.Fatalf("timeout; phase=%d saw=%v", phase, statesOnly(saw))
		}
	}
finishedStartStall:
	if len(pending) == 0 {
		t.Fatal("expected START_PENDING")
	}
	if pending[0].CheckPoint != 1 || pending[0].WaitHintMs == 0 {
		t.Fatalf("initial START_PENDING must publish CheckPoint=1 with wait hint: %+v", pending[0])
	}
	for _, st := range pending {
		if st.CheckPoint != 1 {
			t.Fatalf("stalled start claimed progress: CheckPoint=%d statuses=%+v", st.CheckPoint, pending)
		}
	}
	// Initial START_PENDING + one interrogate echo only — no timer-driven
	// START_PENDING publications while Start remains stalled.
	if len(pending) != 2 {
		t.Fatalf("unexpected START_PENDING count=%d want 2 (initial+interrogate); timer-driven progress? %+v", len(pending), pending)
	}
	assertHasStates(t, saw, scmStateStartPending, scmStateRunning, scmStateStopPending, scmStateStopped)
}

func TestSCMAdapterStalledShutdownDoesNotClaimProgress(t *testing.T) {
	stopGate := make(chan struct{})
	life := &fakeLifecycle{stopGate: stopGate}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest, 4)
	statuses := make(chan scmStatus, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	stallFor := 150 * time.Millisecond

	var pending []scmStatus
	var saw []scmStatus
	deadline := time.After(3 * time.Second)
	phase := 0
	for {
		select {
		case st, ok := <-statuses:
			if !ok {
				goto finishedStopStall
			}
			saw = append(saw, st)
			switch st.State {
			case scmStateRunning:
				if phase == 0 {
					phase = 1
					requests <- scmRequest{Cmd: scmCmdStop}
				}
			case scmStateStopPending:
				pending = append(pending, st)
				if phase == 1 {
					phase = 2
					go func() {
						time.Sleep(stallFor)
						requests <- scmRequest{Cmd: scmCmdInterrogate}
					}()
				} else if phase == 2 {
					phase = 3
					go func() {
						time.Sleep(stallFor)
						close(stopGate)
					}()
				}
			}
		case <-done:
			for {
				select {
				case st, ok := <-statuses:
					if !ok {
						goto finishedStopStall
					}
					saw = append(saw, st)
					if st.State == scmStateStopPending {
						pending = append(pending, st)
					}
				default:
					goto finishedStopStall
				}
			}
		case <-deadline:
			t.Fatalf("timeout; phase=%d saw=%v", phase, statesOnly(saw))
		}
	}
finishedStopStall:
	if len(pending) == 0 {
		t.Fatal("expected STOP_PENDING")
	}
	if pending[0].CheckPoint != 1 || pending[0].WaitHintMs == 0 {
		t.Fatalf("initial STOP_PENDING must publish CheckPoint=1 with wait hint: %+v", pending[0])
	}
	for _, st := range pending {
		if st.CheckPoint != 1 {
			t.Fatalf("stalled stop claimed progress: CheckPoint=%d statuses=%+v", st.CheckPoint, pending)
		}
	}
	if len(pending) != 2 {
		t.Fatalf("unexpected STOP_PENDING count=%d want 2 (initial+interrogate); timer-driven progress? %+v", len(pending), pending)
	}
}

func TestSCMAdapterRepeatedStopAndShutdownDuringPending(t *testing.T) {
	stopGate := make(chan struct{})
	life := &fakeLifecycle{stopGate: stopGate}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest, 8)
	statuses := make(chan scmStatus, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		specific, errno := adapter.runSCMSession(requests, statuses)
		close(statuses)
		if specific || errno != 0 {
			t.Errorf("unexpected exit specific=%v errno=%d", specific, errno)
		}
	}()

	flooded := false
	_ = drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		switch st.State {
		case scmStateRunning:
			send <- scmRequest{Cmd: scmCmdStop}
		case scmStateStopPending:
			if flooded {
				return
			}
			flooded = true
			for i := 0; i < 5; i++ {
				send <- scmRequest{Cmd: scmCmdStop}
				send <- scmRequest{Cmd: scmCmdShutdown}
			}
			close(stopGate)
		}
	}, requests)

	if life.stopCalls.Load() != 1 {
		t.Fatalf("exactly-once Stop violated under STOP/SHUTDOWN flood: calls=%d", life.stopCalls.Load())
	}
}

func TestSCMAdapterFinalSCMErrorClassification(t *testing.T) {
	cases := []struct {
		name         string
		stopErr      error
		wantSpecific bool
		wantErrno    uint32
	}{
		{
			name:         "shutdown_timeout",
			stopErr:      common.ErrShutdownTimeout,
			wantSpecific: true,
			wantErrno:    exitCodeStopFailed,
		},
		{
			name:         "wrapped_shutdown_timeout",
			stopErr:      fmt.Errorf("wrap: %w", common.ErrShutdownTimeout),
			wantSpecific: true,
			wantErrno:    exitCodeStopFailed,
		},
		{
			name:         "generic_stop_failure",
			stopErr:      errors.New("AGENT_SERVICE_STOP_FAILED: boom"),
			wantSpecific: true,
			wantErrno:    exitCodeStopFailed,
		},
		{
			name:         "clean_stop",
			stopErr:      nil,
			wantSpecific: false,
			wantErrno:    0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			life := &fakeLifecycle{stopErr: tc.stopErr}
			adapter := newSCMAdapter(life, testLogger())

			requests := make(chan scmRequest, 1)
			statuses := make(chan scmStatus, 8)
			var specific bool
			var errno uint32
			done := make(chan struct{})
			go func() {
				defer close(done)
				specific, errno = adapter.runSCMSession(requests, statuses)
				close(statuses)
			}()

			saw := drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
				if st.State == scmStateRunning {
					send <- scmRequest{Cmd: scmCmdStop}
				}
			}, requests)

			if specific != tc.wantSpecific || errno != tc.wantErrno {
				t.Fatalf("specific=%v errno=%d want specific=%v errno=%d", specific, errno, tc.wantSpecific, tc.wantErrno)
			}
			last := saw[len(saw)-1]
			if last.State != scmStateStopped {
				t.Fatalf("final state=%d", last.State)
			}
			if tc.wantSpecific {
				if last.Win32ExitCode != win32ErrorServiceSpecificError || last.ServiceSpecificExitCode != tc.wantErrno {
					t.Fatalf("final status codes=%+v", last)
				}
			} else if last.Win32ExitCode != 0 || last.ServiceSpecificExitCode != 0 {
				t.Fatalf("clean stop must not set exit codes: %+v", last)
			}
		})
	}
}

func TestSCMAdapterStartFailureWhileStopRequested(t *testing.T) {
	startGate := make(chan struct{})
	life := &fakeLifecycle{
		startGate: startGate,
		startErr:  errors.New("AGENT_SERVICE_START_FAILED: init"),
	}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest, 2)
	statuses := make(chan scmStatus, 16)
	done := make(chan struct{})
	var specific bool
	var errno uint32
	go func() {
		defer close(done)
		specific, errno = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	stopAcked := false
	saw := drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		switch {
		case st.State == scmStateStartPending && !stopAcked:
			send <- scmRequest{Cmd: scmCmdStop}
		case st.State == scmStateStopPending && !stopAcked:
			stopAcked = true
			close(startGate)
		}
	}, requests)

	if !specific || errno != exitCodeStartFailed {
		t.Fatalf("want start-failure exit, specific=%v errno=%d", specific, errno)
	}
	if life.stopCalls.Load() != 0 {
		t.Fatalf("Stop must not run after start failure; calls=%d", life.stopCalls.Load())
	}
	assertHasStates(t, saw, scmStateStartPending, scmStateStopPending, scmStateStopped)
}

func TestSCMAdapterInterrogateEchoesCurrentStatus(t *testing.T) {
	stopGate := make(chan struct{})
	life := &fakeLifecycle{stopGate: stopGate}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest, 4)
	statuses := make(chan scmStatus, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	phase := 0
	var saw []scmStatus
	deadline := time.After(3 * time.Second)
	for {
		select {
		case st, ok := <-statuses:
			if !ok {
				goto finished
			}
			saw = append(saw, st)
			switch phase {
			case 0:
				if st.State == scmStateRunning {
					requests <- scmRequest{Cmd: scmCmdInterrogate}
					phase = 1
				}
			case 1:
				if st.State != scmStateRunning {
					t.Fatalf("running interrogate echo state=%d", st.State)
				}
				requests <- scmRequest{Cmd: scmCmdStop}
				phase = 2
			case 2:
				if st.State == scmStateStopPending {
					requests <- scmRequest{Cmd: scmCmdInterrogate}
					phase = 3
				}
			case 3:
				if st.State != scmStateStopPending {
					t.Fatalf("stop interrogate echo state=%d", st.State)
				}
				close(stopGate)
				phase = 4
			}
		case <-done:
			for {
				select {
				case st, ok := <-statuses:
					if !ok {
						goto finished
					}
					saw = append(saw, st)
				default:
					goto finished
				}
			}
		case <-deadline:
			t.Fatalf("timeout; phase=%d saw=%v", phase, statesOnly(saw))
		}
	}
finished:
	if phase < 4 {
		t.Fatalf("incomplete interrogate sequence phase=%d saw=%v", phase, statesOnly(saw))
	}
}

func TestSCMAdapterStopTimeoutClassification(t *testing.T) {
	life := &fakeLifecycle{stopErr: common.ErrShutdownTimeout}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest, 1)
	statuses := make(chan scmStatus, 8)

	var specific bool
	var errno uint32
	done := make(chan struct{})
	go func() {
		defer close(done)
		specific, errno = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	_ = drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		if st.State == scmStateRunning {
			send <- scmRequest{Cmd: scmCmdStop}
		}
	}, requests)

	if !specific || errno != exitCodeStopFailed {
		t.Fatalf("want stop failure exit, specific=%v errno=%d", specific, errno)
	}
}

func TestSCMAdapterBlockedStopStillCompletes(t *testing.T) {
	life := &fakeLifecycle{blockStopFor: 30 * time.Millisecond}
	adapter := newSCMAdapter(life, testLogger())

	requests := make(chan scmRequest, 1)
	statuses := make(chan scmStatus, 32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	saw := drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		if st.State == scmStateRunning {
			send <- scmRequest{Cmd: scmCmdStop}
		}
	}, requests)

	assertTransition(t, saw, scmStateStartPending, scmStateRunning, scmStateStopPending, scmStateStopped)
}

func TestForceConsoleModeEnv(t *testing.T) {
	t.Setenv("PLATFORM_AGENT_CONSOLE", "true")
	if !forceConsoleMode() {
		t.Fatal("expected console mode from PLATFORM_AGENT_CONSOLE")
	}
	t.Setenv("PLATFORM_AGENT_CONSOLE", "0")
	if forceConsoleMode() {
		t.Fatal("PLATFORM_AGENT_CONSOLE=0 must not force console")
	}
}

func TestClassifyRunExit(t *testing.T) {
	if classifyRunExit(errConsoleStart) != exitStart {
		t.Fatal("console start")
	}
	if classifyRunExit(errConsoleStop) != exitStop {
		t.Fatal("console stop")
	}
	if classifyRunExit(errSCMRun) != exitSCM {
		t.Fatal("scm run")
	}
	if classifyRunExit(&processExitError{code: exitConfig, err: errors.New("cfg")}) != exitConfig {
		t.Fatal("process exit config")
	}
	if classifyRunExit(&processExitError{code: exitIdentity, err: errors.New("id")}) != exitIdentity {
		t.Fatal("process exit identity")
	}
	if classifyRunExit(&processExitError{code: exitService, err: errors.New("svc")}) != exitService {
		t.Fatal("process exit service")
	}
}

func TestSCMExitCodesAligned(t *testing.T) {
	// Console process exits and SCM service-specific codes share 1–5.
	if exitConfig != exitCodeConfigFailed || exitIdentity != exitCodeIdentityFailed || exitService != exitCodeServiceInitFailed {
		t.Fatalf("init codes drifted: config=%d/%d identity=%d/%d service=%d/%d",
			exitConfig, exitCodeConfigFailed, exitIdentity, exitCodeIdentityFailed, exitService, exitCodeServiceInitFailed)
	}
	if exitStart != exitCodeStartFailed || exitStop != exitCodeStopFailed {
		t.Fatalf("SCM-specific codes must stay aligned with process exits: start=%d/%d stop=%d/%d",
			exitStart, exitCodeStartFailed, exitStop, exitCodeStopFailed)
	}
}

func TestSCMAdapterFactoryInitAfterStartPending(t *testing.T) {
	var pendingBeforeInit atomic.Bool
	var adapter *scmAdapter
	life := &fakeLifecycle{}
	factory := func() (agentLifecycle, uint32, error) {
		if adapter.currentStatus().State == scmStateStartPending {
			pendingBeforeInit.Store(true)
		}
		return life, 0, nil
	}
	adapter = newSCMAdapterWithFactory(factory, testLogger())

	requests := make(chan scmRequest, 2)
	statuses := make(chan scmStatus, 16)
	var specific bool
	var errno uint32
	done := make(chan struct{})
	go func() {
		defer close(done)
		specific, errno = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	saw := drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		if st.State == scmStateRunning {
			send <- scmRequest{Cmd: scmCmdStop}
		}
	}, requests)

	if !pendingBeforeInit.Load() {
		t.Fatal("factory must observe START_PENDING before initialization")
	}
	if specific || errno != 0 {
		t.Fatalf("exit specific=%v errno=%d", specific, errno)
	}
	if life.startCalls.Load() != 1 || life.stopCalls.Load() != 1 {
		t.Fatalf("start/stop calls=%d/%d", life.startCalls.Load(), life.stopCalls.Load())
	}
	assertTransition(t, saw, scmStateStartPending, scmStateRunning, scmStateStopPending, scmStateStopped)
}

func TestSCMAdapterFactoryInitFailures(t *testing.T) {
	cases := []struct {
		name      string
		code      uint32
		wantClass string
	}{
		{name: "config", code: exitCodeConfigFailed, wantClass: "config_load_failed"},
		{name: "identity", code: exitCodeIdentityFailed, wantClass: "identity_init_failed"},
		{name: "service", code: exitCodeServiceInitFailed, wantClass: "initialization_failed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if initFailureReasonClass(tc.code) != tc.wantClass {
				t.Fatalf("reason class=%s want %s", initFailureReasonClass(tc.code), tc.wantClass)
			}

			var initCalls atomic.Int32
			factory := func() (agentLifecycle, uint32, error) {
				initCalls.Add(1)
				return nil, tc.code, fmt.Errorf("injected %s failure", tc.name)
			}
			adapter := newSCMAdapterWithFactory(factory, testLogger())

			requests := make(chan scmRequest)
			statuses := make(chan scmStatus, 8)
			specific, errno := adapter.runSCMSession(requests, statuses)
			close(statuses)

			saw := make([]scmStatus, 0, 4)
			for st := range statuses {
				saw = append(saw, st)
			}

			if initCalls.Load() != 1 {
				t.Fatalf("factory calls=%d", initCalls.Load())
			}
			if !specific || errno != tc.code {
				t.Fatalf("specific=%v errno=%d want code=%d", specific, errno, tc.code)
			}
			if adapter.life != nil {
				t.Fatal("lifecycle must remain unset after init failure")
			}
			for _, st := range saw {
				if st.State == scmStateRunning {
					t.Fatalf("RUNNING must not appear on init failure: %+v", statesOnly(saw))
				}
			}
			if len(saw) < 2 || saw[0].State != scmStateStartPending || saw[len(saw)-1].State != scmStateStopped {
				t.Fatalf("unexpected statuses: %+v", statesOnly(saw))
			}
			last := saw[len(saw)-1]
			if last.Win32ExitCode != win32ErrorServiceSpecificError || last.ServiceSpecificExitCode != tc.code {
				t.Fatalf("final codes=%+v want service-specific %d", last, tc.code)
			}
		})
	}
}

func TestSCMAdapterFactoryStartFailure(t *testing.T) {
	life := &fakeLifecycle{startErr: errors.New("AGENT_SERVICE_START_FAILED: boom")}
	factory := func() (agentLifecycle, uint32, error) {
		return life, 0, nil
	}
	adapter := newSCMAdapterWithFactory(factory, testLogger())

	requests := make(chan scmRequest)
	statuses := make(chan scmStatus, 8)
	specific, errno := adapter.runSCMSession(requests, statuses)
	close(statuses)

	saw := make([]scmStatus, 0, 4)
	for st := range statuses {
		saw = append(saw, st)
	}

	if !specific || errno != exitCodeStartFailed {
		t.Fatalf("want start failure, specific=%v errno=%d", specific, errno)
	}
	if life.startCalls.Load() != 1 || life.stopCalls.Load() != 0 {
		t.Fatalf("start/stop calls=%d/%d", life.startCalls.Load(), life.stopCalls.Load())
	}
	for _, st := range saw {
		if st.State == scmStateRunning {
			t.Fatalf("RUNNING on start failure: %+v", statesOnly(saw))
		}
	}
	last := saw[len(saw)-1]
	if last.Win32ExitCode != win32ErrorServiceSpecificError || last.ServiceSpecificExitCode != exitCodeStartFailed {
		t.Fatalf("exit codes=%+v want Win32ExitCode=%d ServiceSpecificExitCode=%d",
			last, win32ErrorServiceSpecificError, exitCodeStartFailed)
	}
	if last.Win32ExitCode != 1066 || last.ServiceSpecificExitCode != 4 {
		t.Fatalf("Win32/service-specific mapping drifted: win32=%d specific=%d", last.Win32ExitCode, last.ServiceSpecificExitCode)
	}
}

func TestSCMAdapterFactoryStopWhileBootstrapBlocked(t *testing.T) {
	factoryEntered := make(chan struct{})
	factoryRelease := make(chan struct{})
	var enterOnce sync.Once
	var initCalls atomic.Int32
	life := &fakeLifecycle{}
	factory := func() (agentLifecycle, uint32, error) {
		initCalls.Add(1)
		enterOnce.Do(func() { close(factoryEntered) })
		<-factoryRelease
		return life, 0, nil
	}
	adapter := newSCMAdapterWithFactory(factory, testLogger())

	requests := make(chan scmRequest, 8)
	statuses := make(chan scmStatus, 32)
	var specific bool
	var errno uint32
	done := make(chan struct{})
	go func() {
		defer close(done)
		specific, errno = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	stopSent := false
	dupSent := false
	saw := drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		switch {
		case st.State == scmStateStartPending && !stopSent:
			// Explicit acceptance barrier: factory must have started before STOP.
			select {
			case <-factoryEntered:
			case <-time.After(3 * time.Second):
				t.Fatal("factory did not enter before STOP barrier")
			}
			stopSent = true
			send <- scmRequest{Cmd: scmCmdStop}
		case st.State == scmStateStopPending && stopSent && !dupSent:
			dupSent = true
			send <- scmRequest{Cmd: scmCmdStop}
			send <- scmRequest{Cmd: scmCmdShutdown}
			close(factoryRelease)
		}
	}, requests)

	if !stopSent || !dupSent {
		t.Fatalf("barriers missed: stopSent=%v dupSent=%v saw=%v", stopSent, dupSent, statesOnly(saw))
	}
	if specific || errno != 0 {
		t.Fatalf("exit specific=%v errno=%d", specific, errno)
	}
	if initCalls.Load() != 1 {
		t.Fatalf("factory initialization calls=%d want 1", initCalls.Load())
	}
	if life.startCalls.Load() != 1 {
		t.Fatalf("AgentService.Start calls=%d want 1", life.startCalls.Load())
	}
	if life.stopCalls.Load() != 1 {
		t.Fatalf("AgentService.Stop calls=%d want 1", life.stopCalls.Load())
	}
	if adapter.stopCallCount() != 1 {
		t.Fatalf("adapter stopCallCount=%d want 1", adapter.stopCallCount())
	}
	for _, st := range saw {
		if st.State == scmStateRunning {
			t.Fatalf("RUNNING must not appear after acknowledged STOP: %+v", statesOnly(saw))
		}
	}
	assertHasStates(t, saw, scmStateStartPending, scmStateStopPending, scmStateStopped)
	last := saw[len(saw)-1]
	if last.State != scmStateStopped || last.Win32ExitCode != 0 || last.ServiceSpecificExitCode != 0 {
		t.Fatalf("final STOPPED clean exit required: %+v", last)
	}
}

func TestSCMAdapterFactoryInitFailureWhileStopRequested(t *testing.T) {
	cases := []struct {
		name string
		code uint32
	}{
		{name: "config", code: exitCodeConfigFailed},
		{name: "identity", code: exitCodeIdentityFailed},
		{name: "service", code: exitCodeServiceInitFailed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			factoryEntered := make(chan struct{})
			factoryRelease := make(chan struct{})
			var enterOnce sync.Once
			var initCalls atomic.Int32
			factory := func() (agentLifecycle, uint32, error) {
				initCalls.Add(1)
				enterOnce.Do(func() { close(factoryEntered) })
				<-factoryRelease
				return nil, tc.code, fmt.Errorf("injected %s failure", tc.name)
			}
			adapter := newSCMAdapterWithFactory(factory, testLogger())

			requests := make(chan scmRequest, 4)
			statuses := make(chan scmStatus, 16)
			var specific bool
			var errno uint32
			done := make(chan struct{})
			go func() {
				defer close(done)
				specific, errno = adapter.runSCMSession(requests, statuses)
				close(statuses)
			}()

			stopSent := false
			stopAcked := false
			saw := drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
				switch {
				case st.State == scmStateStartPending && !stopSent:
					select {
					case <-factoryEntered:
					case <-time.After(3 * time.Second):
						t.Fatal("factory did not enter before STOP barrier")
					}
					stopSent = true
					send <- scmRequest{Cmd: scmCmdStop}
				case st.State == scmStateStopPending && stopSent && !stopAcked:
					stopAcked = true
					close(factoryRelease)
				}
			}, requests)

			if !stopAcked {
				t.Fatalf("STOP_PENDING acknowledgment missed; saw=%v", statesOnly(saw))
			}
			if initCalls.Load() != 1 {
				t.Fatalf("factory calls=%d want 1", initCalls.Load())
			}
			if !specific || errno != tc.code {
				t.Fatalf("specific=%v errno=%d want code=%d", specific, errno, tc.code)
			}
			if adapter.life != nil {
				t.Fatal("lifecycle must remain unset after init failure")
			}
			if adapter.stopCallCount() != 0 {
				t.Fatalf("Stop must not run without constructed lifecycle; calls=%d", adapter.stopCallCount())
			}
			for _, st := range saw {
				if st.State == scmStateRunning {
					t.Fatalf("RUNNING must not appear on init failure: %+v", statesOnly(saw))
				}
			}
			assertHasStates(t, saw, scmStateStartPending, scmStateStopPending, scmStateStopped)
			last := saw[len(saw)-1]
			if last.State != scmStateStopped {
				t.Fatalf("final state=%d", last.State)
			}
			if last.Win32ExitCode != win32ErrorServiceSpecificError || last.ServiceSpecificExitCode != tc.code {
				t.Fatalf("final codes=%+v want Win32=%d specific=%d", last, win32ErrorServiceSpecificError, tc.code)
			}
			if last.Win32ExitCode != 1066 {
				t.Fatalf("Win32ExitCode=%d want 1066", last.Win32ExitCode)
			}
		})
	}
}

func TestSCMAdapterFactoryStopAfterSuccessfulInit(t *testing.T) {
	life := &fakeLifecycle{}
	var initCalls atomic.Int32
	factory := func() (agentLifecycle, uint32, error) {
		initCalls.Add(1)
		return life, 0, nil
	}
	adapter := newSCMAdapterWithFactory(factory, testLogger())

	requests := make(chan scmRequest, 2)
	statuses := make(chan scmStatus, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = adapter.runSCMSession(requests, statuses)
		close(statuses)
	}()

	saw := drainUntil(t, statuses, done, func(st scmStatus, send chan<- scmRequest) {
		if st.State == scmStateRunning {
			send <- scmRequest{Cmd: scmCmdShutdown}
		}
	}, requests)

	if initCalls.Load() != 1 {
		t.Fatalf("factory calls=%d", initCalls.Load())
	}
	assertTransition(t, saw, scmStateStartPending, scmStateRunning, scmStateStopPending, scmStateStopped)
	if life.startCalls.Load() != 1 || life.stopCalls.Load() != 1 {
		t.Fatalf("start/stop calls=%d/%d", life.startCalls.Load(), life.stopCalls.Load())
	}
}

func TestBootstrapInitializeInjectedFailures(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*agentBootstrapDeps)
		wantCode uint32
	}{
		{
			name: "config",
			mutate: func(d *agentBootstrapDeps) {
				d.loadConfig = func() (*config.Config, error) {
					return nil, errors.New("config boom")
				}
			},
			wantCode: exitConfig,
		},
		{
			name: "identity",
			mutate: func(d *agentBootstrapDeps) {
				d.loadConfig = func() (*config.Config, error) {
					return &config.Config{LogLevel: "INFO", StateDir: "configured"}, nil
				}
				d.loadIdentity = func(stateDir, agentID, tenantID string) (*identity.AgentIdentity, error) {
					return nil, errors.New("identity boom")
				}
			},
			wantCode: exitIdentity,
		},
		{
			name: "service",
			mutate: func(d *agentBootstrapDeps) {
				d.loadConfig = func() (*config.Config, error) {
					return &config.Config{LogLevel: "INFO", StateDir: "configured"}, nil
				}
				d.loadIdentity = func(stateDir, agentID, tenantID string) (*identity.AgentIdentity, error) {
					return &identity.AgentIdentity{AgentID: "agent-test", HostID: "host-test", OS: "test", Arch: "test"}, nil
				}
				d.newService = func(cfg *config.Config, ident *identity.AgentIdentity, logger *slog.Logger) (*common.AgentService, error) {
					return nil, errors.New("service boom")
				}
			},
			wantCode: exitService,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			levelVar := new(slog.LevelVar)
			deps := agentBootstrapDeps{
				levelVar:        levelVar,
				bootstrapLogger: testLogger(),
			}
			tc.mutate(&deps)
			rt, code, err := deps.initialize()
			if err == nil || rt != nil {
				t.Fatalf("expected failure, rt=%v err=%v", rt, err)
			}
			if code != tc.wantCode {
				t.Fatalf("code=%d want %d", code, tc.wantCode)
			}
			pe := &processExitError{code: int(code), err: err}
			if classifyRunExit(pe) != int(tc.wantCode) {
				t.Fatalf("classify=%d", classifyRunExit(pe))
			}
		})
	}
}

func TestBootstrapInitializeSuccess(t *testing.T) {
	levelVar := new(slog.LevelVar)
	wantCfg := &config.Config{LogLevel: "DEBUG", StateDir: "configured", AgentID: "a1"}
	wantIdent := &identity.AgentIdentity{AgentID: "a1", HostID: "h1", OS: "windows", Arch: "amd64"}
	wantSvc := &common.AgentService{}
	deps := agentBootstrapDeps{
		levelVar:        levelVar,
		bootstrapLogger: testLogger(),
		loadConfig: func() (*config.Config, error) {
			return wantCfg, nil
		},
		loadIdentity: func(stateDir, agentID, tenantID string) (*identity.AgentIdentity, error) {
			if stateDir != "configured" || agentID != "a1" {
				t.Fatalf("identity args stateDir=%q agentID=%q", stateDir, agentID)
			}
			return wantIdent, nil
		},
		newService: func(cfg *config.Config, ident *identity.AgentIdentity, logger *slog.Logger) (*common.AgentService, error) {
			if cfg != wantCfg || ident != wantIdent || logger == nil {
				t.Fatal("newService args mismatch")
			}
			return wantSvc, nil
		},
	}
	rt, code, err := deps.initialize()
	if err != nil || code != 0 || rt == nil {
		t.Fatalf("initialize failed: code=%d err=%v", code, err)
	}
	if rt.svc != wantSvc || rt.cfg != wantCfg || rt.ident != wantIdent {
		t.Fatal("runtime ownership mismatch")
	}
	if levelVar.Level() != slog.LevelDebug {
		t.Fatalf("log level=%v", levelVar.Level())
	}
}

func TestConsoleEntrypointReportsInitFailure(t *testing.T) {
	deps := agentBootstrapDeps{
		bootstrapLogger: testLogger(),
		loadConfig: func() (*config.Config, error) {
			return nil, errors.New("console config boom")
		},
	}
	err := runConsoleEntrypoint(deps)
	var pe *processExitError
	if !errors.As(err, &pe) || pe.code != exitConfig {
		t.Fatalf("want processExitError config, got %v", err)
	}
}

func drainUntil(
	t *testing.T,
	statuses <-chan scmStatus,
	done <-chan struct{},
	onStatus func(scmStatus, chan<- scmRequest),
	requests chan<- scmRequest,
) []scmStatus {
	t.Helper()
	var saw []scmStatus
	deadline := time.After(3 * time.Second)
	for {
		select {
		case st, ok := <-statuses:
			if !ok {
				return saw
			}
			saw = append(saw, st)
			if onStatus != nil {
				onStatus(st, requests)
			}
		case <-done:
			for {
				select {
				case st, ok := <-statuses:
					if !ok {
						return saw
					}
					saw = append(saw, st)
				default:
					return saw
				}
			}
		case <-deadline:
			t.Fatalf("timeout waiting for SCM session; saw=%v", statesOnly(saw))
			return saw
		}
	}
}

func assertTransition(t *testing.T, saw []scmStatus, want ...uint32) {
	t.Helper()
	var filtered []uint32
	var last uint32
	for i, st := range saw {
		if i == 0 || st.State != last {
			filtered = append(filtered, st.State)
			last = st.State
		}
	}
	if len(filtered) < len(want) {
		t.Fatalf("transitions=%v want prefix %v (raw=%v)", filtered, want, statesOnly(saw))
	}
	for i := range want {
		if filtered[i] != want[i] {
			t.Fatalf("transitions=%v want %v", filtered, want)
		}
	}
}

func assertHasStates(t *testing.T, saw []scmStatus, want ...uint32) {
	t.Helper()
	have := map[uint32]bool{}
	for _, st := range saw {
		have[st.State] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Fatalf("missing state %d in %v", w, statesOnly(saw))
		}
	}
}

func statesOnly(saw []scmStatus) []uint32 {
	out := make([]uint32, len(saw))
	for i, st := range saw {
		out[i] = st.State
	}
	return out
}
