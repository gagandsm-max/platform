package icmpintegration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	protocolicmp "platform/sensor/internal/protocol/icmp"
	"platform/sensor/internal/runtime"
	"platform/sensor/internal/scheduler"
)

type HealthSnapshot struct {
	Enabled            bool      `json:"enabled"`
	Registered         bool      `json:"registered"`
	Executions         uint64    `json:"executions"`
	Successful         uint64    `json:"successful"`
	Failed             uint64    `json:"failed"`
	SpoolWriteFailures uint64    `json:"spoolWriteFailures"`
	LastSuccessAt      time.Time `json:"lastSuccessAt,omitempty"`
	LastFailureAt      time.Time `json:"lastFailureAt,omitempty"`
	LastErrorCode      string    `json:"lastErrorCode,omitempty"`
	Degraded           bool      `json:"degraded"`
}

// localCapabilityCheck opens/closes local ping sockets once at Start. It must
// not send probe traffic to a customer target and must not run from Ready.
type localCapabilityCheck func(context.Context) error

const localCapabilityTimeout = 2 * time.Second

type Component struct {
	executor       *Executor
	executions     atomic.Uint64
	successful     atomic.Uint64
	failed         atomic.Uint64
	spoolFailures  atomic.Uint64
	mu             sync.RWMutex
	registered     bool
	lastSuccess    time.Time
	lastFailure    time.Time
	lastErrorCode  string
	degraded       bool
	now            func() time.Time
	capability     localCapabilityCheck
	capabilityOnce sync.Once
}

func newComponent(executor *Executor) *Component {
	return &Component{executor: executor, now: time.Now}
}

// SetLocalCapabilityCheck installs an optional one-time startup verifier.
// Nil clears the check. Safe before Start; ignored after the first Start run.
func (c *Component) SetLocalCapabilityCheck(check localCapabilityCheck) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.capability = check
	c.mu.Unlock()
}

func (c *Component) Name() string                   { return "icmp" }
func (c *Component) Capability() runtime.Capability { return runtime.CapabilityICMP }
func (c *Component) Start(ctx context.Context) error {
	c.mu.RLock()
	registered := c.registered
	check := c.capability
	c.mu.RUnlock()
	if !registered {
		return fmt.Errorf("ICMP_COMPONENT_NOT_REGISTERED: ICMP executor route is not registered")
	}
	c.capabilityOnce.Do(func() {
		if check == nil {
			return
		}
		parent := ctx
		if parent == nil {
			parent = context.Background()
		}
		verifyCtx, cancel := context.WithTimeout(parent, localCapabilityTimeout)
		defer cancel()
		if err := check(verifyCtx); err != nil {
			code := string(protocolicmp.CodeOf(err))
			if !collectorUnusable(code) {
				// Timeouts and unexpected local open failures are Sensor-local
				// capability gaps, not target reachability.
				code = string(protocolicmp.CodeProtocol)
			}
			c.markCollectorUnusable(code)
		}
	})
	return nil
}
func (c *Component) Ready(context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.registered {
		return fmt.Errorf("ICMP_COMPONENT_NOT_REGISTERED: ICMP executor route is not registered")
	}
	if c.degraded {
		return fmt.Errorf("ICMP_COMPONENT_DEGRADED: last error code %s", c.lastErrorCode)
	}
	return nil
}
func (c *Component) Drain(context.Context) error    { return nil }
func (c *Component) Shutdown(context.Context) error { return nil }

func (c *Component) Snapshot() HealthSnapshot {
	if c == nil {
		return HealthSnapshot{}
	}
	c.mu.RLock()
	snap := HealthSnapshot{Enabled: true, Registered: c.registered, LastSuccessAt: c.lastSuccess, LastFailureAt: c.lastFailure, LastErrorCode: c.lastErrorCode, Degraded: c.degraded}
	c.mu.RUnlock()
	snap.Executions = c.executions.Load()
	snap.Successful = c.successful.Load()
	snap.Failed = c.failed.Load()
	snap.SpoolWriteFailures = c.spoolFailures.Load()
	return snap
}
func (c *Component) markRegistered()  { c.mu.Lock(); c.registered = true; c.mu.Unlock() }
func (c *Component) recordExecution() { c.executions.Add(1) }
func (c *Component) recordSuccess() {
	c.successful.Add(1)
	c.mu.Lock()
	c.lastSuccess = c.now().UTC()
	c.lastErrorCode = ""
	c.degraded = false
	c.mu.Unlock()
}
func (c *Component) recordFailure(code string, spoolFailure bool) {
	c.failed.Add(1)
	if spoolFailure {
		c.spoolFailures.Add(1)
	}
	c.mu.Lock()
	c.lastFailure = c.now().UTC()
	c.lastErrorCode = code
	// Spool failure and collector-unusable protocol sources degrade the ICMP
	// component. Transient target timeouts/unreachable/transport stay evidence-only.
	if spoolFailure || collectorUnusable(code) {
		c.degraded = true
	}
	c.mu.Unlock()
}

// markCollectorUnusable latches startup/local socket unusability without
// counting an execution. Ready becomes DEGRADED; DEVICE_DOWN is never invented.
func (c *Component) markCollectorUnusable(code string) {
	c.mu.Lock()
	c.lastFailure = c.now().UTC()
	c.lastErrorCode = code
	c.degraded = true
	c.mu.Unlock()
}

// collectorUnusable identifies Sensor-local ICMP socket capability failures.
// These must degrade component readiness without inventing DEVICE_DOWN.
func collectorUnusable(code string) bool {
	return code == string(protocolicmp.CodePermission) || code == string(protocolicmp.CodeProtocol)
}

func Register(rt *runtime.Runtime, registry *runtime.ExecutorRegistry, executor *Executor) error {
	if rt == nil || registry == nil || executor == nil || executor.component == nil {
		return fmt.Errorf("ICMP_INTEGRATION_INVALID_CONFIG: runtime, registry and ICMP integration executor are required")
	}
	snapshot := rt.Snapshot()
	if snapshot.State != runtime.StateInitializing {
		return fmt.Errorf("ICMP_INTEGRATION_REGISTRATION_CLOSED: runtime must be INITIALIZING")
	}
	if !runtime.RoleAllows(snapshot.Role, runtime.CapabilityICMP) {
		return fmt.Errorf("ICMP_INTEGRATION_ROLE_NOT_ALLOWED: role %s does not allow ICMP", snapshot.Role)
	}
	enabled := false
	for _, capability := range snapshot.EnabledCapabilities {
		if capability == runtime.CapabilityICMP {
			enabled = true
			break
		}
	}
	if !enabled {
		return fmt.Errorf("ICMP_INTEGRATION_CAPABILITY_DISABLED: ICMP capability is not enabled")
	}
	for _, component := range snapshot.Components {
		if component.Capability == runtime.CapabilityICMP {
			return fmt.Errorf("ICMP_INTEGRATION_COMPONENT_CONFLICT: ICMP component is already registered")
		}
	}
	if err := registry.Register(runtime.CapabilityICMP, protocolicmp.JobKind, scheduler.ClassNetworkPoll, executor); err != nil {
		return err
	}
	if err := rt.RegisterComponent(executor.component); err != nil {
		return err
	}
	executor.component.markRegistered()
	return nil
}
