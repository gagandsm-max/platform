package snmpintegration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	protocolsnmp "platform/sensor/internal/protocol/snmp"
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

type Component struct {
	executor      *Executor
	executions    atomic.Uint64
	successful    atomic.Uint64
	failed        atomic.Uint64
	spoolFailures atomic.Uint64
	mu            sync.RWMutex
	registered    bool
	lastSuccess   time.Time
	lastFailure   time.Time
	lastErrorCode string
	degraded      bool
	now           func() time.Time
}

func newComponent(executor *Executor) *Component {
	return &Component{executor: executor, now: time.Now}
}
func (c *Component) Name() string                    { return "snmp" }
func (c *Component) Capability() runtime.Capability { return runtime.CapabilitySNMP }
func (c *Component) Start(context.Context) error {
	c.mu.RLock()
	registered := c.registered
	c.mu.RUnlock()
	if !registered {
		return fmt.Errorf("SNMP_COMPONENT_NOT_REGISTERED: SNMP executor route is not registered")
	}
	return nil
}
func (c *Component) Ready(context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.registered {
		return fmt.Errorf("SNMP_COMPONENT_NOT_REGISTERED: SNMP executor route is not registered")
	}
	if c.degraded {
		return fmt.Errorf("SNMP_COMPONENT_DEGRADED: last error code %s", c.lastErrorCode)
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
	if spoolFailure {
		c.degraded = true
	}
	c.mu.Unlock()
}

func Register(rt *runtime.Runtime, registry *runtime.ExecutorRegistry, executor *Executor) error {
	if rt == nil || registry == nil || executor == nil || executor.component == nil {
		return fmt.Errorf("SNMP_INTEGRATION_INVALID_CONFIG: runtime, registry and SNMP integration executor are required")
	}
	snapshot := rt.Snapshot()
	if snapshot.State != runtime.StateInitializing {
		return fmt.Errorf("SNMP_INTEGRATION_REGISTRATION_CLOSED: runtime must be INITIALIZING")
	}
	if !runtime.RoleAllows(snapshot.Role, runtime.CapabilitySNMP) {
		return fmt.Errorf("SNMP_INTEGRATION_ROLE_NOT_ALLOWED: role %s does not allow SNMP", snapshot.Role)
	}
	enabled := false
	for _, capability := range snapshot.EnabledCapabilities {
		if capability == runtime.CapabilitySNMP {
			enabled = true
			break
		}
	}
	if !enabled {
		return fmt.Errorf("SNMP_INTEGRATION_CAPABILITY_DISABLED: SNMP capability is not enabled")
	}
	for _, component := range snapshot.Components {
		if component.Capability == runtime.CapabilitySNMP {
			return fmt.Errorf("SNMP_INTEGRATION_COMPONENT_CONFLICT: SNMP component is already registered")
		}
	}
	if err := registry.Register(runtime.CapabilitySNMP, protocolsnmp.SchedulerJobKind, scheduler.ClassNetworkPoll, executor); err != nil {
		return err
	}
	if err := rt.RegisterComponent(executor.component); err != nil {
		return err
	}
	executor.component.markRegistered()
	return nil
}
