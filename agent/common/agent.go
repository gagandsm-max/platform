package common

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/collector"
	"platform/agent/common/config"
	"platform/agent/common/delivery"
	"platform/agent/common/health"
	"platform/agent/common/identity"
	"platform/agent/common/queue"
	"platform/agent/common/scheduler"
)

// LifecycleState is the authoritative AgentService lifecycle state.
// Slice 3 maps these values into health readiness via RuntimeSnapshot.
type LifecycleState string

const (
	LifecycleNew      LifecycleState = "NEW"
	LifecycleStarting LifecycleState = "STARTING"
	LifecycleRunning  LifecycleState = "RUNNING"
	LifecycleStopping LifecycleState = "STOPPING"
	LifecycleStopped  LifecycleState = "STOPPED"
	LifecycleFailed   LifecycleState = "FAILED"
)

// defaultAgentShutdownBudget is the whole-service upper bound for Stop().
// Slice 4 selects systemd TimeoutStopSec above this runtime budget.
const defaultAgentShutdownBudget = 10 * time.Second

// optionalEnrichmentBudgetDenominator derives the shared subordinate optional-
// enrichment budget (activation settle + backend stop) from the single
// whole-service shutdown budget (numerator 1). Optional enrichment must not
// monopolize protected base-Agent scheduler/worker/health teardown.
const optionalEnrichmentBudgetDenominator = 2

var (
	// ErrAlreadyRunning is returned when Start is invoked while STARTING/RUNNING.
	ErrAlreadyRunning = errors.New("AGENT_ALREADY_RUNNING: Start rejected; service already started")
	// ErrRestartUnsupported is returned for same-instance Start after STOPPED/FAILED.
	ErrRestartUnsupported = errors.New("AGENT_RESTART_UNSUPPORTED: same-instance restart is not supported")
	// ErrInvalidStartContext is returned when Start receives a nil or already-done context.
	ErrInvalidStartContext = errors.New("AGENT_START_CONTEXT_INVALID: Start context must be usable")
	// ErrShutdownTimeout is returned when Stop exceeds the service shutdown budget.
	ErrShutdownTimeout = errors.New("AGENT_SHUTDOWN_TIMEOUT: service stop exceeded shutdown budget")
)

type AgentService struct {
	cfg       *config.Config
	identity  *identity.AgentIdentity
	queue     *queue.DurableQueue
	transport client.Transport
	scheduler *scheduler.Scheduler
	worker    *delivery.Worker
	healthSrv *health.Server
	logger    *slog.Logger

	version                  string
	serviceInstanceID        string
	processNetworkCapability collector.ProcessNetworkCapabilitySnapshot
	processNetworkRuntime    ProcessNetworkRuntime

	// registryOverride is applied only during NewAgentService construction.
	registryOverride *collector.Registry

	opMu           sync.Mutex // serializes Start/Stop bodies
	lifeMu         sync.Mutex
	state          LifecycleState
	runCancel      context.CancelFunc
	shutdownBudget time.Duration
	stopErr        error

	// Optional process-network activation: exactly one owned attempt after RUNNING.
	activationDone chan struct{}
}

// ServiceOption configures Agent service construction for BUILD-005 supportability identity.
type ServiceOption func(*AgentService)

// WithSupportabilityIdentity binds the health SUPPORT_EVIDENCE envelope to the
// same process-local identity used by the canonical structured logger.
func WithSupportabilityIdentity(version, serviceInstanceID string) ServiceOption {
	return func(s *AgentService) {
		if strings.TrimSpace(version) != "" {
			s.version = version
		}
		if strings.TrimSpace(serviceInstanceID) != "" {
			s.serviceInstanceID = serviceInstanceID
		}
	}
}

// WithProcessNetworkCapability attaches immutable optional preflight evidence
// when no ProcessNetworkRuntime is injected (for example eBPF disabled).
// It does not participate in Agent lifecycle, readiness, scheduler, queue, or
// delivery decisions.
func WithProcessNetworkCapability(snapshot collector.ProcessNetworkCapabilitySnapshot) ServiceOption {
	return func(s *AgentService) {
		s.processNetworkCapability = snapshot.Copy()
	}
}

// WithProcessNetworkRuntime injects an optional subordinate enrichment runtime.
// Concrete Linux eBPF Backend construction remains under agent/linux/**.
// Activation is attempted asynchronously only after base Agent reaches RUNNING.
func WithProcessNetworkRuntime(rt ProcessNetworkRuntime) ServiceOption {
	return func(s *AgentService) {
		s.processNetworkRuntime = rt
	}
}

// withCollectorRegistryForTest replaces the default collector registry. Unexported
// so production callers cannot widen collection surface from Slice 2.
func withCollectorRegistryForTest(reg *collector.Registry) ServiceOption {
	return func(s *AgentService) {
		s.registryOverride = reg
	}
}

func NewAgentService(
	cfg *config.Config,
	ident *identity.AgentIdentity,
	tr client.Transport,
	logger *slog.Logger,
	opts ...ServiceOption,
) (*AgentService, error) {
	if cfg == nil {
		return nil, errors.New("AGENT_CONFIG_REQUIRED: config must be provided")
	}
	if ident == nil {
		return nil, errors.New("AGENT_IDENTITY_REQUIRED: identity must be provided")
	}
	if logger == nil {
		logger = slog.Default()
	}

	svc := &AgentService{
		cfg:               cfg,
		identity:          ident,
		logger:            logger,
		version:           "0.1.0-dev",
		serviceInstanceID: "",
		state:             LifecycleNew,
		shutdownBudget:    defaultAgentShutdownBudget,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}

	// If no transport injected, wire production HTTPS+mTLS to the assigned Sensor.
	// Tests inject UnavailableTransport or a fake Transport explicitly.
	if tr == nil {
		httpsTr, err := client.NewHTTPSTransport(client.HTTPSConfig{
			SensorURL:      cfg.SensorURL,
			AgentID:        ident.AgentID,
			TenantID:       ident.TenantID,
			SiteID:         cfg.SiteID,
			CACertFile:     cfg.CAFile,
			ClientCertFile: cfg.CertFile,
			ClientKeyFile:  cfg.KeyFile,
			Timeout:        cfg.DeliveryTimeout,
			Logger:         logger,
		})
		if err != nil {
			return nil, fmt.Errorf("AGENT_TRANSPORT_INIT_FAILED: %w", err)
		}
		tr = httpsTr
	}
	svc.transport = tr

	// 1. Initialize durable queue
	q, err := queue.NewDurableQueue(
		cfg.QueueDir,
		cfg.MaxBufferRecords,
		cfg.MaxBufferBytes,
		cfg.MaxRecordBytes,
	)
	if err != nil {
		return nil, fmt.Errorf("AGENT_QUEUE_INIT_FAILED: %w", err)
	}
	svc.queue = q

	// 2. Initialize startup-immutable collector registry.
	registry := svc.registryOverride
	if registry == nil {
		registry, err = collector.NewDefaultRegistry(
			cfg.MaxFilesystems,
			cfg.CollectionInterval,
			cfg.CollectionTimeout,
		)
		if err != nil {
			return nil, fmt.Errorf("AGENT_COLLECTOR_REGISTRY_INIT_FAILED: %w", err)
		}
	}
	svc.registryOverride = nil // drop construction-only reference

	// 3. Initialize scheduler (per-collector schedule, timeout isolation,
	// deterministic phase spreading, bounded concurrency).
	sched, err := scheduler.NewScheduler(
		ident,
		registry,
		q,
		0, // default bounded concurrency
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("AGENT_SCHEDULER_INIT_FAILED: %w", err)
	}
	svc.scheduler = sched

	// 4. Initialize delivery worker
	worker, err := delivery.NewWorker(
		q,
		tr,
		ident.AgentID,
		cfg.MaxBatchRecords,
		cfg.MaxBatchBytes,
		cfg.RetryBase,
		cfg.RetryMax,
		cfg.DeliveryTimeout,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("AGENT_DELIVERY_INIT_FAILED: %w", err)
	}
	svc.worker = worker

	// 5. Initialize health server
	healthOpts := []health.Option{}
	if svc.version != "" || svc.serviceInstanceID != "" {
		healthOpts = append(healthOpts, health.WithIdentity(svc.version, svc.serviceInstanceID))
	}
	healthSrv, err := health.NewServer(cfg.HealthAddress, svc, logger, healthOpts...)
	if err != nil {
		return nil, fmt.Errorf("AGENT_HEALTH_INIT_FAILED: %w", err)
	}
	svc.healthSrv = healthSrv

	return svc, nil
}

// LifecycleState returns the race-safe authoritative service lifecycle state.
func (s *AgentService) LifecycleState() LifecycleState {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	return s.state
}

func (s *AgentService) setShutdownBudgetForTest(d time.Duration) {
	s.lifeMu.Lock()
	defer s.lifeMu.Unlock()
	s.shutdownBudget = d
}

func (s *AgentService) setState(next LifecycleState) {
	s.lifeMu.Lock()
	s.state = next
	s.lifeMu.Unlock()
}

func (s *AgentService) setStateLocked(next LifecycleState) {
	s.state = next
}

func (s *AgentService) Start(ctx context.Context) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	s.lifeMu.Lock()
	switch s.state {
	case LifecycleNew:
		s.setStateLocked(LifecycleStarting)
		s.lifeMu.Unlock()
	case LifecycleStarting, LifecycleRunning, LifecycleStopping:
		s.lifeMu.Unlock()
		return ErrAlreadyRunning
	case LifecycleStopped, LifecycleFailed:
		s.lifeMu.Unlock()
		return ErrRestartUnsupported
	default:
		s.lifeMu.Unlock()
		return fmt.Errorf("AGENT_LIFECYCLE_INVALID: unexpected state %q", s.state)
	}

	s.logger.Info("AmrudAi Agent service starting",
		"event_code", "AGENT_SERVICE_STARTING",
		"safe_context", map[string]any{
			"lifecycle_state": string(LifecycleStarting),
			"host_id":         s.identity.HostID,
			"os":              s.identity.OS,
			"arch":            s.identity.Arch,
		},
	)

	if ctx == nil {
		return s.failStart(fmt.Errorf("%w: nil context", ErrInvalidStartContext))
	}
	if err := ctx.Err(); err != nil {
		return s.failStart(fmt.Errorf("%w: %v", ErrInvalidStartContext, err))
	}

	// Deliberate detachment: Start(ctx) only validates that ctx is usable at
	// startup. Runtime cancellation is owned exclusively by AgentService.Stop()
	// via runCancel. Cancelling the caller's Start context after Start returns
	// must not terminate workers/scheduler (see TestAgentServiceStartContextDetached).
	runCtx, runCancel := context.WithCancel(context.Background())

	if err := s.healthSrv.Start(); err != nil {
		runCancel()
		s.logger.Error("Failed to start health server",
			"event_code", "AGENT_HEALTH_START_FAILED",
			"safe_context", map[string]any{"reason_class": "startup_failed"})
		return s.failStart(fmt.Errorf("AGENT_HEALTH_START_FAILED: %w", err))
	}

	s.worker.Start(runCtx)
	s.scheduler.Start(runCtx)

	s.lifeMu.Lock()
	s.runCancel = runCancel
	s.setStateLocked(LifecycleRunning)
	s.stopErr = nil
	var activationDone chan struct{}
	if s.processNetworkRuntime != nil {
		activationDone = make(chan struct{})
		s.activationDone = activationDone
	}
	s.lifeMu.Unlock()

	s.logger.Info("AmrudAi Agent service running",
		"event_code", "AGENT_SERVICE_RUNNING",
		"safe_context", map[string]any{
			"lifecycle_state":    string(LifecycleRunning),
			"receiver_available": s.transport.Available(),
		},
	)

	// Optional enrichment activation is subordinate and asynchronous: base Agent
	// must already be RUNNING and Start must return without waiting on eBPF.
	if activationDone != nil {
		go s.activateProcessNetworkOnce(runCtx, activationDone)
	}
	return nil
}

// activateProcessNetworkOnce performs exactly one owned optional backend Start.
// Failure never calls failStart and never stops scheduler/worker/health.
func (s *AgentService) activateProcessNetworkOnce(ctx context.Context, done chan struct{}) {
	defer close(done)
	rt := s.processNetworkRuntime
	if rt == nil {
		return
	}
	s.logger.Info("AmrudAi Agent optional process-network activation beginning",
		"event_code", "AGENT_EBPF_ACTIVATION_BEGIN",
		"safe_context", map[string]any{
			"lifecycle_state": string(LifecycleRunning),
			"reason_class":    "optional_enrichment",
		},
	)
	if err := rt.Start(ctx); err != nil {
		capSnap := rt.Capability()
		s.logger.Info("AmrudAi Agent optional process-network activation failed",
			"event_code", "AGENT_EBPF_ACTIVATION_FAILED",
			"safe_context", map[string]any{
				"lifecycle_state": string(s.LifecycleState()),
				"reason_class":    "optional_enrichment_failed",
				"capability":      string(capSnap.State),
			},
		)
		return
	}
	capSnap := rt.Capability()
	s.logger.Info("AmrudAi Agent optional process-network activation succeeded",
		"event_code", "AGENT_EBPF_ACTIVATION_SUCCEEDED",
		"safe_context", map[string]any{
			"lifecycle_state": string(s.LifecycleState()),
			"capability":      string(capSnap.State),
		},
	)
}

func (s *AgentService) failStart(err error) error {
	s.lifeMu.Lock()
	s.setStateLocked(LifecycleFailed)
	s.stopErr = err
	s.lifeMu.Unlock()
	s.logger.Error("AmrudAi Agent service startup failed",
		"event_code", "AGENT_SERVICE_START_FAILED",
		"safe_context", map[string]any{
			"lifecycle_state": string(LifecycleFailed),
			"reason_class":    "startup_failed",
		},
	)
	return err
}

func (s *AgentService) Stop() error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	s.lifeMu.Lock()
	switch s.state {
	case LifecycleStopped:
		err := s.stopErr
		s.lifeMu.Unlock()
		return err
	case LifecycleFailed:
		// Startup failure or prior shutdown failure: terminal, idempotent.
		err := s.stopErr
		s.lifeMu.Unlock()
		return err
	case LifecycleNew:
		s.setStateLocked(LifecycleStopped)
		s.stopErr = nil
		s.lifeMu.Unlock()
		s.logger.Info("AmrudAi Agent service stopped",
			"event_code", "AGENT_SERVICE_STOPPED",
			"safe_context", map[string]any{
				"lifecycle_state": string(LifecycleStopped),
				"reason_class":    "stop_before_start",
			},
		)
		return nil
	case LifecycleStarting, LifecycleRunning, LifecycleStopping:
		s.setStateLocked(LifecycleStopping)
		budget := s.shutdownBudget
		cancel := s.runCancel
		activationDone := s.activationDone
		rt := s.processNetworkRuntime
		s.lifeMu.Unlock()

		if budget <= 0 {
			budget = defaultAgentShutdownBudget
		}

		s.logger.Info("AmrudAi Agent service stopping",
			"event_code", "AGENT_SERVICE_STOPPING",
			"safe_context", map[string]any{
				"lifecycle_state": string(LifecycleStopping),
				"reason_class":    "shutdown_requested",
				"budget_ms":       budget.Milliseconds(),
			},
		)

		// One whole-service absolute deadline. Activation settle and backend stop
		// share one subordinate optional-enrichment budget derived from it —
		// never an additive budget that can starve protected teardown.
		deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), budget)
		defer deadlineCancel()

		optionalBudget := optionalEnrichmentBudget(budget)
		optionalDeadline := time.Now().Add(optionalBudget)

		var activationErr error
		// Boundedly settle the one optional activation attempt before StopContext
		// so cooperative Starts can finish under the shared optional-enrichment
		// sub-budget. Lifecycle ownership is Backend.stopRequested via
		// StopContext — never a capability snapshot. Do not cancel the Agent
		// runtime here: that recreates cancel-before-flush when ACTIVE workers
		// already exist, and is unnecessary because StopContext latches
		// stopRequested so an in-progress Start cannot commit workers after Stop.
		if activationDone != nil {
			settleBudget := time.Until(optionalDeadline)
			if settleBudget < 0 {
				settleBudget = 0
			}
			settleCtx, settleCancel := context.WithTimeout(deadlineCtx, settleBudget)
			select {
			case <-activationDone:
				settleCancel()
			case <-settleCtx.Done():
				settleCancel()
				activationErr = fmt.Errorf("%w: optional process-network activation did not settle before optional-enrichment sub-budget", ErrShutdownTimeout)
				s.logger.Error("AmrudAi Agent optional process-network activation unsettled during shutdown",
					"event_code", "AGENT_EBPF_ACTIVATION_SHUTDOWN_UNSETTLED",
					"safe_context", map[string]any{
						"lifecycle_state":  string(LifecycleStopping),
						"reason_class":     "shutdown_timeout",
						"detail":           "process_network_activation_unsettled",
						"settle_budget_ms": settleBudget.Milliseconds(),
					},
				)
			}
		}

		var backendErr error
		// Always request Backend.StopContext when a runtime is present — even if
		// activation settle timed out. StopContext is linearizable with Start via
		// the Backend mutex and stopRequested: pre-commit Starts abort; committed
		// workers are shut down within the remaining optional-enrichment budget.
		// Skipping StopContext orphans detached workers after a late ACTIVE commit.
		if rt != nil {
			backendBudget := time.Until(optionalDeadline)
			if backendBudget < 0 {
				backendBudget = 0
			}
			backendCtx, backendCancel := context.WithTimeout(deadlineCtx, backendBudget)
			backendErr = rt.StopContext(backendCtx)
			backendCancel()
		}

		// Cancel owned runtime context for protected components after optional
		// enrichment has had its chance to flush under a live stop context.
		if cancel != nil {
			cancel()
		}

		var (
			schedErr  error
			workerErr error
			compWG    sync.WaitGroup
		)
		compWG.Add(2)
		go func() {
			defer compWG.Done()
			schedErr = s.scheduler.StopContext(deadlineCtx)
		}()
		go func() {
			defer compWG.Done()
			workerErr = s.worker.StopContext(deadlineCtx)
		}()
		compWG.Wait()

		healthErr := s.healthSrv.Stop(deadlineCtx)

		if err := joinShutdownErrors(deadlineCtx, activationErr, backendErr, schedErr, workerErr, healthErr); err != nil {
			stopErr := classifyAgentStopError(deadlineCtx, err)
			s.lifeMu.Lock()
			s.setStateLocked(LifecycleFailed)
			s.stopErr = stopErr
			s.lifeMu.Unlock()
			reasonClass := "shutdown_failed"
			if errors.Is(stopErr, ErrShutdownTimeout) {
				reasonClass = "shutdown_timeout"
			}
			s.logger.Error("AmrudAi Agent service shutdown failed",
				"event_code", "AGENT_SERVICE_STOP_FAILED",
				"safe_context", map[string]any{
					"lifecycle_state": string(LifecycleFailed),
					"reason_class":    reasonClass,
				},
			)
			return stopErr
		}

		s.lifeMu.Lock()
		s.setStateLocked(LifecycleStopped)
		s.stopErr = nil
		s.lifeMu.Unlock()
		s.logger.Info("AmrudAi Agent service stopped",
			"event_code", "AGENT_SERVICE_STOPPED",
			"safe_context", map[string]any{
				"lifecycle_state": string(LifecycleStopped),
				"reason_class":    "shutdown_complete",
			},
		)
		return nil
	default:
		s.lifeMu.Unlock()
		return fmt.Errorf("AGENT_LIFECYCLE_INVALID: unexpected state %q", s.state)
	}
}

// optionalEnrichmentBudget returns the shared subordinate budget for optional
// process-network activation settle and backend stop. It is derived from the
// whole-service budget and never extends Stop beyond that single absolute deadline.
func optionalEnrichmentBudget(whole time.Duration) time.Duration {
	if whole <= 0 {
		return 0
	}
	sub := whole / time.Duration(optionalEnrichmentBudgetDenominator)
	if sub < time.Millisecond {
		if whole > time.Millisecond {
			return time.Millisecond
		}
		return whole
	}
	return sub
}

// joinShutdownErrors preserves typed error identity via errors.Join so
// errors.Is / errors.As work through the AgentService.Stop boundary.
// When the whole-service deadline (or cancel) has already fired, that cause is
// retained alongside any component errors — never omitted merely because a
// component failure is also present.
func joinShutdownErrors(deadlineCtx context.Context, errs ...error) error {
	var nonempty []error
	for _, err := range errs {
		if err != nil {
			nonempty = append(nonempty, err)
		}
	}
	if derr := deadlineCtx.Err(); derr != nil {
		present := false
		for _, err := range nonempty {
			if errors.Is(err, derr) {
				present = true
				break
			}
		}
		if !present {
			nonempty = append(nonempty, derr)
		}
	}
	if len(nonempty) == 0 {
		return nil
	}
	if len(nonempty) == 1 {
		return nonempty[0]
	}
	return errors.Join(nonempty...)
}

// classifyAgentStopError wraps with ErrShutdownTimeout only for authentic
// deadline exhaustion. Explicit cancellation, sink failures, incomplete flush,
// backend runtime faults and activation settlement failures retain their typed
// identities. Ordinary context.Canceled is never mislabeled as a timeout.
// When the whole-service deadline is exhausted, that cause is preserved even if
// another typed error is also present.
func classifyAgentStopError(deadlineCtx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrShutdownTimeout) {
		return err
	}
	deadlineExhausted := errors.Is(deadlineCtx.Err(), context.DeadlineExceeded) ||
		errors.Is(err, context.DeadlineExceeded)
	if deadlineExhausted {
		return fmt.Errorf("%w: %w", ErrShutdownTimeout, err)
	}
	if derr := deadlineCtx.Err(); derr != nil && !errors.Is(err, derr) {
		return errors.Join(err, derr)
	}
	return err
}

func (s *AgentService) QueueStats() queue.QueueStats {
	return s.queue.Stats()
}

func (s *AgentService) DeliveryStats() delivery.DeliveryStats {
	return s.worker.Stats()
}

func (s *AgentService) CollectorStats() (time.Time, int) {
	return s.scheduler.Stats()
}

// SchedulerStatus exposes detailed scheduler/collector evidence for operability
// inspection. Readiness consumes the same evidence via RuntimeSnapshot.
func (s *AgentService) SchedulerStatus() scheduler.Status {
	return s.scheduler.Status()
}

// RuntimeSnapshot adapts authoritative AgentService evidence for health readiness.
// Lifecycle is read first and released before queue/delivery/scheduler locks.
// Process-network capability is dynamic when a runtime is injected.
func (s *AgentService) RuntimeSnapshot() health.RuntimeSnapshot {
	life := string(s.LifecycleState())

	qStats := s.queue.Stats()
	dStats := s.worker.Stats()
	st := s.scheduler.Status()

	collectors := make([]health.CollectorSnapshot, 0, len(st.Collectors))
	for _, c := range st.Collectors {
		collectors = append(collectors, health.CollectorSnapshot{
			Name:        c.Name,
			Enabled:     c.Enabled,
			Outstanding: c.Outstanding,
			LastError:   c.LastError,
		})
	}

	capSnap := s.processNetworkCapability.Copy()
	var support collector.ProcessNetworkSupportSnapshot
	if s.processNetworkRuntime != nil {
		capSnap = s.processNetworkRuntime.Capability()
		support = s.processNetworkRuntime.SupportSnapshot()
	}

	return health.RuntimeSnapshot{
		Lifecycle:                life,
		Queue:                    qStats,
		Delivery:                 dStats,
		Admission:                st.Admission,
		SchedulerRunning:         st.Running,
		EnabledCollectors:        st.EnabledCollectors,
		Collectors:               collectors,
		CollectorLastRun:         st.LastRun,
		CollectorRunErrors:       st.RunErrors,
		ProcessNetworkCapability: capSnap,
		ProcessNetworkSupport:    support,
	}
}
