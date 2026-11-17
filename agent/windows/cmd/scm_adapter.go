package main

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"platform/agent/common"
)

// SCM state / control constants mirror Win32 SERVICE_* values so Linux unit
// tests can exercise the adapter without importing golang.org/x/sys/windows.
const (
	scmStateStopped      = 1
	scmStateStartPending = 2
	scmStateStopPending  = 3
	scmStateRunning      = 4

	scmCmdStop        = 1
	scmCmdInterrogate = 4
	scmCmdShutdown    = 5

	scmAcceptStop     = 1
	scmAcceptShutdown = 4

	scmCmdsAccepted = scmAcceptStop | scmAcceptShutdown

	defaultStartWaitHint = 15 * time.Second
	// Leave one second for scheduling and SCM status delivery before a pending
	// wait hint expires. Do not claim fictitious progress during a hang.
	scmDeadlineMargin = time.Second

	// agentServiceStopBudget mirrors common.defaultAgentShutdownBudget (10s).
	// SCM WaitHint must cover that budget plus scheduling margin because this
	// adapter does not publish fictitious CheckPoint advances while Stop runs.
	agentServiceStopBudget = 10 * time.Second
	stopWaitHintMargin     = 5 * time.Second
	defaultStopWaitHint    = agentServiceStopBudget + stopWaitHintMargin

	exitCodeConfigFailed      = 1
	exitCodeIdentityFailed    = 2
	exitCodeServiceInitFailed = 3
	exitCodeStartFailed       = 4
	exitCodeStopFailed        = 5

	// win32ErrorServiceSpecificError is ERROR_SERVICE_SPECIFIC_ERROR (1066).
	win32ErrorServiceSpecificError = 1066
)

// agentLifecycle is the AgentService Start/Stop seam. SCM never owns scheduling,
// spool, or transport — AgentService remains the sole lifecycle authority.
type agentLifecycle interface {
	Start(ctx context.Context) error
	Stop() error
}

// agentRuntimeFactory constructs AgentService after START_PENDING is published.
// On failure it returns a service-specific exit code (1=config, 2=identity,
// 3=construction) without calling os.Exit.
type agentRuntimeFactory func() (agentLifecycle, uint32, error)

// scmStatus is the SCM-visible status payload (state, accepts, progress).
type scmStatus struct {
	State                   uint32
	Accepts                 uint32
	CheckPoint              uint32
	WaitHintMs              uint32
	Win32ExitCode           uint32
	ServiceSpecificExitCode uint32
}

// scmRequest is a control request from SCM (or a test harness).
type scmRequest struct {
	Cmd           uint32
	CurrentStatus scmStatus
}

// scmAdapter implements the Windows Service Control Manager protocol against an
// injected agentLifecycle. It is build-tag free so lifecycle transitions can be
// unit-tested on Linux CI hosts.
//
// Production Windows service mode supplies agentRuntimeFactory so configuration,
// identity and AgentService construction run only after START_PENDING. Tests may
// inject a pre-built lifecycle to exercise Start/Stop state-machine edges.
type scmAdapter struct {
	life    agentLifecycle
	factory agentRuntimeFactory
	logger  *slog.Logger

	startWaitHint time.Duration
	stopWaitHint  time.Duration

	stopOnce  sync.Once
	stopErr   error
	stopCalls atomic.Int32

	current atomic.Value // scmStatus
}

func newSCMAdapter(life agentLifecycle, logger *slog.Logger) *scmAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	a := &scmAdapter{
		life:          life,
		logger:        logger,
		startWaitHint: defaultStartWaitHint,
		stopWaitHint:  defaultStopWaitHint,
	}
	a.current.Store(scmStatus{State: scmStateStopped})
	return a
}

func newSCMAdapterWithFactory(factory agentRuntimeFactory, logger *slog.Logger) *scmAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	a := &scmAdapter{
		factory:       factory,
		logger:        logger,
		startWaitHint: defaultStartWaitHint,
		stopWaitHint:  defaultStopWaitHint,
	}
	a.current.Store(scmStatus{State: scmStateStopped})
	return a
}

func (a *scmAdapter) report(statuses chan<- scmStatus, st scmStatus) {
	a.current.Store(st)
	statuses <- st
}

func (a *scmAdapter) currentStatus() scmStatus {
	if v := a.current.Load(); v != nil {
		return v.(scmStatus)
	}
	return scmStatus{State: scmStateStopped}
}

// x/sys/windows/svc reports SERVICE_STOPPED itself after Handler.Execute
// returns. The adapter retains its final state for local lifecycle tests, but
// the Windows bridge must never forward that state to the SCM status channel:
// reporting it there would make SetServiceStatus(SERVICE_STOPPED) run twice.
func shouldForwardSCMStatus(st scmStatus) bool {
	return st.State != scmStateStopped
}

// pendingDeadline is earlier than the pending WaitHint wherever possible.
// Tiny wait hints are used by deterministic timeout tests.
func pendingDeadline(waitHint time.Duration) time.Duration {
	if waitHint <= 0 {
		return 0
	}
	if waitHint <= 2*scmDeadlineMargin {
		return waitHint - waitHint/4
	}
	return waitHint - scmDeadlineMargin
}

// invokeStop runs AgentService.Stop exactly once and records the result.
func (a *scmAdapter) invokeStop() error {
	a.stopOnce.Do(func() {
		a.stopCalls.Add(1)
		a.stopErr = a.life.Stop()
	})
	return a.stopErr
}

func (a *scmAdapter) stopCallCount() int32 {
	return a.stopCalls.Load()
}

func initFailureReasonClass(code uint32) string {
	switch code {
	case exitCodeConfigFailed:
		return "config_load_failed"
	case exitCodeIdentityFailed:
		return "identity_init_failed"
	case exitCodeServiceInitFailed:
		return "initialization_failed"
	default:
		return "startup_failed"
	}
}

// runSCMSession drives START_PENDING → RUNNING → STOP_PENDING → STOPPED.
// Pending phases publish a single honest status with WaitHint; CheckPoint does
// not advance on a timer. RUNNING / STOPPED are published only when the
// underlying AgentService Start / Stop operation completes.
//
// Start is invoked with a detached background context: AgentService deliberately
// ignores post-return cancellation of the Start argument, and Stop() is the sole
// runtime cancellation authority. Cancelling Start(ctx) is therefore never used
// as a stop mechanism. A hung start fails the SCM session at the pending
// deadline, so the owning Windows service process can exit rather than remain
// indefinitely START_PENDING; this is not a retry or background restart.
func (a *scmAdapter) runSCMSession(requests <-chan scmRequest, statuses chan<- scmStatus) (svcSpecific bool, errno uint32) {
	startHintMs := uint32(a.startWaitHint.Milliseconds())
	startTimer := time.NewTimer(pendingDeadline(a.startWaitHint))
	defer startTimer.Stop()

	a.report(statuses, scmStatus{
		State:      scmStateStartPending,
		CheckPoint: 1,
		WaitHintMs: startHintMs,
	})

	a.logger.Info("AmrudAi Windows Agent SCM start pending",
		"event_code", "AGENT_SCM_START_PENDING",
		"safe_context", map[string]any{
			"scm_state":    "START_PENDING",
			"wait_hint_ms": a.startWaitHint.Milliseconds(),
			"reason_class": "scm_lifecycle",
		},
	)

	type startResult struct {
		err      error
		initCode uint32 // non-zero when factory failed before AgentService.Start
	}
	started := make(chan startResult, 1)
	go func() {
		life := a.life
		if a.factory != nil {
			constructed, code, err := a.factory()
			if err != nil {
				if code == 0 {
					code = exitCodeServiceInitFailed
				}
				started <- startResult{err: err, initCode: code}
				return
			}
			life = constructed
			a.life = constructed
		}
		if life == nil {
			started <- startResult{
				err:      errors.New("AGENT_SCM_LIFECYCLE_MISSING"),
				initCode: exitCodeServiceInitFailed,
			}
			return
		}
		// Detached start context: lifecycle ownership stays with AgentService.Stop.
		started <- startResult{err: life.Start(context.Background())}
	}()

	stopRequested := false
	stopReason := "stop"

	for {
		select {
		case <-startTimer.C:
			a.logger.Error("AmrudAi Windows Agent SCM startup timed out",
				"event_code", "AGENT_SCM_START_TIMEOUT",
				"safe_context", map[string]any{
					"scm_state":    "STOPPED",
					"reason_class": "startup_timeout",
				})
			a.report(statuses, scmStatus{
				State:                   scmStateStopped,
				Win32ExitCode:           win32ErrorServiceSpecificError,
				ServiceSpecificExitCode: exitCodeStartFailed,
			})
			return true, exitCodeStartFailed
		case res := <-started:
			if res.err != nil {
				code := uint32(exitCodeStartFailed)
				reasonClass := "startup_failed"
				eventCode := "AGENT_SCM_START_FAILED"
				if res.initCode != 0 {
					code = res.initCode
					reasonClass = initFailureReasonClass(res.initCode)
					eventCode = "AGENT_SCM_INIT_FAILED"
				}
				a.logger.Error("AmrudAi Windows Agent SCM start failed",
					"event_code", eventCode,
					"safe_context", map[string]any{
						"scm_state":    "STOPPED",
						"reason_class": reasonClass,
						"exit_code":    code,
					},
				)
				a.report(statuses, scmStatus{
					State:                   scmStateStopped,
					Win32ExitCode:           win32ErrorServiceSpecificError,
					ServiceSpecificExitCode: code,
				})
				return true, code
			}
			if stopRequested {
				// Start completed after STOP was accepted. AgentService.opMu is
				// released, so Stop() can proceed. Do not cancel Start(ctx).
				return a.performStop(requests, statuses, stopReason)
			}
			a.report(statuses, scmStatus{
				State:   scmStateRunning,
				Accepts: scmCmdsAccepted,
			})
			a.logger.Info("AmrudAi Windows Agent SCM running",
				"event_code", "AGENT_SCM_RUNNING",
				"safe_context", map[string]any{
					"scm_state":    "RUNNING",
					"reason_class": "scm_lifecycle",
				},
			)
			return a.serveRunning(requests, statuses)

		case req := <-requests:
			switch req.Cmd {
			case scmCmdInterrogate:
				a.report(statuses, a.currentStatus())
			case scmCmdStop, scmCmdShutdown:
				if stopRequested {
					a.logger.Info("AmrudAi Windows Agent SCM duplicate stop ignored",
						"event_code", "AGENT_SCM_STOP_DUPLICATE",
						"safe_context", map[string]any{
							"scm_state":    "STOP_PENDING",
							"reason_class": "exactly_once_stop",
							"stop_calls":   a.stopCallCount(),
						},
					)
					continue
				}
				stopRequested = true
				if req.Cmd == scmCmdShutdown {
					stopReason = "shutdown"
				} else {
					stopReason = "stop"
				}
				// Visible STOP acceptance boundary: report STOP_PENDING before
				// Start returns. Stop() itself waits until Start releases opMu.
				a.report(statuses, scmStatus{
					State:      scmStateStopPending,
					CheckPoint: 1,
					WaitHintMs: uint32(a.stopWaitHint.Milliseconds()),
				})
				a.logger.Info("AmrudAi Windows Agent SCM stop accepted during start",
					"event_code", "AGENT_SCM_STOP_ACCEPTED",
					"safe_context", map[string]any{
						"scm_state":    "STOP_PENDING",
						"reason_class": stopReason,
					},
				)
			default:
				// Pause/Continue and other controls are not accepted.
				a.logger.Info("AmrudAi Windows Agent SCM ignored control",
					"event_code", "AGENT_SCM_CONTROL_IGNORED",
					"safe_context", map[string]any{
						"control":      req.Cmd,
						"reason_class": "unsupported_control",
					},
				)
			}
		}
	}
}

func (a *scmAdapter) serveRunning(requests <-chan scmRequest, statuses chan<- scmStatus) (bool, uint32) {
	for {
		select {
		case req := <-requests:
			switch req.Cmd {
			case scmCmdInterrogate:
				a.report(statuses, a.currentStatus())
			case scmCmdStop, scmCmdShutdown:
				reason := "stop"
				if req.Cmd == scmCmdShutdown {
					reason = "shutdown"
				}
				a.logger.Info("AmrudAi Windows Agent SCM stop accepted",
					"event_code", "AGENT_SCM_STOP_ACCEPTED",
					"safe_context", map[string]any{
						"scm_state":    "RUNNING",
						"reason_class": reason,
					},
				)
				return a.performStop(requests, statuses, reason)
			default:
				a.logger.Info("AmrudAi Windows Agent SCM ignored control",
					"event_code", "AGENT_SCM_CONTROL_IGNORED",
					"safe_context", map[string]any{
						"control":      req.Cmd,
						"reason_class": "unsupported_control",
					},
				)
			}
		}
	}
}

func (a *scmAdapter) performStop(requests <-chan scmRequest, statuses chan<- scmStatus, reason string) (bool, uint32) {
	stopHintMs := uint32(a.stopWaitHint.Milliseconds())

	// If STOP was already acknowledged during START_PENDING, keep the existing
	// STOP_PENDING status. Otherwise publish a single honest STOP_PENDING with
	// WaitHint covering the AgentService.Stop budget (+ scheduling margin).
	if cur := a.currentStatus(); cur.State != scmStateStopPending {
		a.report(statuses, scmStatus{
			State:      scmStateStopPending,
			CheckPoint: 1,
			WaitHintMs: stopHintMs,
		})
	}

	a.logger.Info("AmrudAi Windows Agent SCM stop pending",
		"event_code", "AGENT_SCM_STOP_PENDING",
		"safe_context", map[string]any{
			"scm_state":    "STOP_PENDING",
			"reason_class": reason,
			"wait_hint_ms": a.stopWaitHint.Milliseconds(),
		},
	)

	done := make(chan struct{})
	go func() {
		_ = a.invokeStop()
		close(done)
	}()
	stopTimer := time.NewTimer(pendingDeadline(a.stopWaitHint))
	defer stopTimer.Stop()

	for {
		select {
		case <-stopTimer.C:
			// No synthetic checkpoint or clean STOPPED: exit the service
			// process with a failure when a blocked Stop exhausts its budget.
			// The Go svc wrapper publishes terminal STOPPED on handler return.
			a.logger.Error("AmrudAi Windows Agent SCM stop timed out",
				"event_code", "AGENT_SCM_STOP_TIMEOUT",
				"safe_context", map[string]any{
					"scm_state":    "STOPPED",
					"reason_class": "shutdown_timeout",
				})
			a.report(statuses, scmStatus{
				State:                   scmStateStopped,
				Win32ExitCode:           win32ErrorServiceSpecificError,
				ServiceSpecificExitCode: exitCodeStopFailed,
			})
			return true, exitCodeStopFailed
		case <-done:
			err := a.stopErr
			st := scmStatus{State: scmStateStopped}
			svcSpecific := false
			var errno uint32
			if err != nil {
				svcSpecific = true
				errno = exitCodeStopFailed
				st.Win32ExitCode = win32ErrorServiceSpecificError
				st.ServiceSpecificExitCode = exitCodeStopFailed
				reasonClass := "shutdown_failed"
				if errors.Is(err, common.ErrShutdownTimeout) {
					reasonClass = "shutdown_timeout"
				}
				a.logger.Error("AmrudAi Windows Agent SCM stop completed with error",
					"event_code", "AGENT_SCM_STOP_FAILED",
					"safe_context", map[string]any{
						"scm_state":    "STOPPED",
						"reason_class": reasonClass,
					},
				)
			} else {
				a.logger.Info("AmrudAi Windows Agent SCM stopped",
					"event_code", "AGENT_SCM_STOPPED",
					"safe_context", map[string]any{
						"scm_state":    "STOPPED",
						"reason_class": reason,
					},
				)
			}
			a.report(statuses, st)
			return svcSpecific, errno

		case req := <-requests:
			switch req.Cmd {
			case scmCmdInterrogate:
				a.report(statuses, a.currentStatus())
			case scmCmdStop, scmCmdShutdown:
				// Exactly-once Stop: duplicate STOP/SHUTDOWN is acknowledged by
				// remaining in STOP_PENDING without a second Stop() call.
				a.logger.Info("AmrudAi Windows Agent SCM duplicate stop ignored",
					"event_code", "AGENT_SCM_STOP_DUPLICATE",
					"safe_context", map[string]any{
						"scm_state":    "STOP_PENDING",
						"reason_class": "exactly_once_stop",
						"stop_calls":   a.stopCallCount(),
					},
				)
			}
		}
	}
}
