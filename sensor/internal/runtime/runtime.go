package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"platform/sensor/internal/scheduler"
)

type State string

const (
	StateInitializing State = "INITIALIZING"
	StateStarting     State = "STARTING"
	StateReady        State = "READY"
	StateDegraded     State = "DEGRADED"
	StateDraining     State = "DRAINING"
	StateStopping     State = "STOPPING"
	StateStopped      State = "STOPPED"
	StateFailed       State = "FAILED"
)

type ComponentState string

const (
	ComponentReady    ComponentState = "READY"
	ComponentDegraded ComponentState = "DEGRADED"
	ComponentStopped  ComponentState = "STOPPED"
)

type Component interface {
	Name() string
	Capability() Capability
	Start(context.Context) error
	Ready(context.Context) error
	Drain(context.Context) error
	Shutdown(context.Context) error
}

type Scheduler interface {
	Submit(scheduler.Job) (scheduler.Submission, error)
	Shutdown(context.Context) error
	Snapshot() scheduler.Snapshot
}

type Config struct {
	Role                  Role
	EnabledCapabilities   []Capability
	DesiredConfigVersion  uint64
	AppliedConfigVersion  uint64
	AppliedConfigChecksum string
}

func (c Config) Validate() error {
	if err := ValidateCapabilities(c.Role, c.EnabledCapabilities); err != nil {
		return err
	}
	if c.AppliedConfigVersion > c.DesiredConfigVersion {
		return fmt.Errorf("SENSOR_RUNTIME_INVALID_CONFIG_VERSION: applied config version exceeds desired version")
	}
	return nil
}

type ComponentSnapshot struct {
	Name       string
	Capability Capability
	State      ComponentState
	Error      string
}

type Snapshot struct {
	State                 State
	Role                  Role
	EnabledCapabilities   []Capability
	DesiredConfigVersion  uint64
	AppliedConfigVersion  uint64
	AppliedConfigChecksum string
	Components            []ComponentSnapshot
	Scheduler             scheduler.Snapshot
}

type Runtime struct {
	mu              sync.RWMutex
	cfg             Config
	scheduler       Scheduler
	registry        *ExecutorRegistry
	components      map[Capability]Component
	order           []Capability
	state           State
	componentStatus map[Capability]ComponentSnapshot
	rootCtx         context.Context
	cancel          context.CancelFunc
	started         bool
	shutdownOnce    sync.Once
	shutdownErr     error
}

func New(cfg Config, sched Scheduler, registry *ExecutorRegistry) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if sched == nil {
		return nil, fmt.Errorf("SENSOR_RUNTIME_INVALID_CONFIG: scheduler is required")
	}
	if registry == nil {
		return nil, fmt.Errorf("SENSOR_RUNTIME_INVALID_CONFIG: executor registry is required")
	}
	rootCtx, cancel := context.WithCancel(context.Background())
	return &Runtime{
		cfg:             cfg,
		scheduler:       sched,
		registry:        registry,
		components:      make(map[Capability]Component),
		state:           StateInitializing,
		componentStatus: make(map[Capability]ComponentSnapshot),
		rootCtx:         rootCtx,
		cancel:          cancel,
	}, nil
}

func (r *Runtime) RegisterComponent(component Component) error {
	if component == nil {
		return fmt.Errorf("SENSOR_RUNTIME_INVALID_COMPONENT: component is nil")
	}
	capability := component.Capability()
	name := component.Name()
	if capability == "" || name == "" {
		return fmt.Errorf("SENSOR_RUNTIME_INVALID_COMPONENT: component name and capability are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return fmt.Errorf("SENSOR_RUNTIME_REGISTRATION_CLOSED: components cannot be registered after startup")
	}
	if !containsCapability(r.cfg.EnabledCapabilities, capability) {
		return fmt.Errorf("SENSOR_RUNTIME_CAPABILITY_DISABLED: component capability %s is not enabled", capability)
	}
	if _, exists := r.components[capability]; exists {
		return fmt.Errorf("SENSOR_RUNTIME_COMPONENT_CONFLICT: capability %s already has a component", capability)
	}
	r.components[capability] = component
	r.order = append(r.order, capability)
	r.componentStatus[capability] = ComponentSnapshot{Name: name, Capability: capability, State: ComponentStopped}
	return nil
}

func (r *Runtime) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	if r.started {
		state := r.state
		r.mu.Unlock()
		if state == StateReady || state == StateDegraded {
			return nil
		}
		return fmt.Errorf("SENSOR_RUNTIME_ALREADY_STARTED: runtime state is %s", state)
	}
	if err := r.validateImplementationsLocked(); err != nil {
		r.state = StateFailed
		r.mu.Unlock()
		return err
	}
	r.started = true
	r.state = StateStarting
	order := append([]Capability(nil), r.order...)
	components := make(map[Capability]Component, len(r.components))
	for k, v := range r.components {
		components[k] = v
	}
	r.mu.Unlock()

	started := make([]Capability, 0, len(order))
	for _, capability := range order {
		if err := components[capability].Start(r.rootCtx); err != nil {
			for i := len(started) - 1; i >= 0; i-- {
				_ = components[started[i]].Shutdown(ctx)
			}
			r.mu.Lock()
			r.state = StateFailed
			r.setComponentErrorLocked(capability, err)
			r.mu.Unlock()
			return fmt.Errorf("SENSOR_RUNTIME_COMPONENT_START_FAILED: %s: %w", components[capability].Name(), err)
		}
		started = append(started, capability)
	}
	return r.RefreshHealth(ctx)
}

func (r *Runtime) validateImplementationsLocked() error {
	for _, capability := range r.cfg.EnabledCapabilities {
		_, component := r.components[capability]
		executor := r.registry.HasCapability(capability)
		if !component && !executor {
			return fmt.Errorf("SENSOR_RUNTIME_CAPABILITY_UNIMPLEMENTED: enabled capability %s has no registered component or executor", capability)
		}
	}
	return nil
}

func (r *Runtime) RefreshHealth(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.RLock()
	if !r.started {
		r.mu.RUnlock()
		return fmt.Errorf("SENSOR_RUNTIME_NOT_STARTED: runtime has not started")
	}
	if r.state == StateDraining || r.state == StateStopping || r.state == StateStopped || r.state == StateFailed {
		r.mu.RUnlock()
		return nil
	}
	components := make(map[Capability]Component, len(r.components))
	for k, v := range r.components {
		components[k] = v
	}
	r.mu.RUnlock()

	degraded := false
	statuses := make(map[Capability]ComponentSnapshot, len(components))
	for capability, component := range components {
		status := ComponentSnapshot{Name: component.Name(), Capability: capability, State: ComponentReady}
		if err := component.Ready(ctx); err != nil {
			degraded = true
			status.State = ComponentDegraded
			status.Error = err.Error()
		}
		statuses[capability] = status
	}
	r.mu.Lock()
	for k, v := range statuses {
		r.componentStatus[k] = v
	}
	if degraded {
		r.state = StateDegraded
	} else {
		r.state = StateReady
	}
	r.mu.Unlock()
	if degraded {
		return fmt.Errorf("SENSOR_RUNTIME_DEGRADED: one or more components are not ready")
	}
	return nil
}

func (r *Runtime) Submit(job scheduler.Job) (scheduler.Submission, error) {
	r.mu.RLock()
	state := r.state
	r.mu.RUnlock()
	if state != StateReady && state != StateDegraded {
		return scheduler.Submission{}, fmt.Errorf("SENSOR_RUNTIME_NOT_ACCEPTING_WORK: runtime state is %s", state)
	}
	return r.scheduler.Submit(job)
}

func (r *Runtime) Drain(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	switch r.state {
	case StateDraining, StateStopping, StateStopped:
		r.mu.Unlock()
		return nil
	case StateReady, StateDegraded:
		r.state = StateDraining
	default:
		state := r.state
		r.mu.Unlock()
		return fmt.Errorf("SENSOR_RUNTIME_DRAIN_INVALID_STATE: %s", state)
	}
	order := append([]Capability(nil), r.order...)
	components := make(map[Capability]Component, len(r.components))
	for k, v := range r.components {
		components[k] = v
	}
	r.mu.Unlock()
	var errs []error
	for i := len(order) - 1; i >= 0; i-- {
		if err := components[order[i]].Drain(ctx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", components[order[i]].Name(), err))
		}
	}
	return errors.Join(errs...)
}

func (r *Runtime) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.shutdownOnce.Do(func() {
		r.mu.Lock()
		r.state = StateStopping
		order := append([]Capability(nil), r.order...)
		components := make(map[Capability]Component, len(r.components))
		for k, v := range r.components {
			components[k] = v
		}
		r.mu.Unlock()

		r.cancel()
		var errs []error
		if err := r.scheduler.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("scheduler: %w", err))
		}
		for i := len(order) - 1; i >= 0; i-- {
			if err := components[order[i]].Shutdown(ctx); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", components[order[i]].Name(), err))
			}
		}
		r.mu.Lock()
		r.state = StateStopped
		for capability, status := range r.componentStatus {
			status.State = ComponentStopped
			status.Error = ""
			r.componentStatus[capability] = status
		}
		r.shutdownErr = errors.Join(errs...)
		r.mu.Unlock()
	})
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.shutdownErr
}

func (r *Runtime) ApplyConfiguration(ctx context.Context, version uint64, checksum string, apply func(context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if version == 0 || checksum == "" {
		return fmt.Errorf("SENSOR_RUNTIME_INVALID_CONFIG_VERSION: version and checksum are required")
	}
	r.mu.Lock()
	if version < r.cfg.DesiredConfigVersion || version < r.cfg.AppliedConfigVersion {
		r.mu.Unlock()
		return fmt.Errorf("SENSOR_RUNTIME_STALE_CONFIG: version %d is older than current desired/applied version", version)
	}
	if version == r.cfg.AppliedConfigVersion && checksum == r.cfg.AppliedConfigChecksum {
		r.cfg.DesiredConfigVersion = version
		r.mu.Unlock()
		return nil
	}
	r.cfg.DesiredConfigVersion = version
	r.mu.Unlock()

	if apply == nil {
		return fmt.Errorf("SENSOR_RUNTIME_CONFIG_APPLY_FAILED: apply function is required")
	}
	if err := apply(ctx); err != nil {
		r.mu.Lock()
		if r.state == StateReady {
			r.state = StateDegraded
		}
		r.mu.Unlock()
		return fmt.Errorf("SENSOR_RUNTIME_CONFIG_APPLY_FAILED: %w", err)
	}
	r.mu.Lock()
	r.cfg.AppliedConfigVersion = version
	r.cfg.AppliedConfigChecksum = checksum
	r.mu.Unlock()
	return nil
}

func (r *Runtime) Snapshot() Snapshot {
	r.mu.RLock()
	snap := Snapshot{
		State:                 r.state,
		Role:                  r.cfg.Role,
		EnabledCapabilities:   append([]Capability(nil), r.cfg.EnabledCapabilities...),
		DesiredConfigVersion:  r.cfg.DesiredConfigVersion,
		AppliedConfigVersion:  r.cfg.AppliedConfigVersion,
		AppliedConfigChecksum: r.cfg.AppliedConfigChecksum,
	}
	for _, status := range r.componentStatus {
		snap.Components = append(snap.Components, status)
	}
	r.mu.RUnlock()
	sort.Slice(snap.EnabledCapabilities, func(i, j int) bool { return snap.EnabledCapabilities[i] < snap.EnabledCapabilities[j] })
	sort.Slice(snap.Components, func(i, j int) bool { return snap.Components[i].Capability < snap.Components[j].Capability })
	snap.Scheduler = r.scheduler.Snapshot()
	return snap
}

func (r *Runtime) setComponentErrorLocked(capability Capability, err error) {
	status := r.componentStatus[capability]
	status.State = ComponentDegraded
	if err != nil {
		status.Error = err.Error()
	}
	r.componentStatus[capability] = status
}

func containsCapability(values []Capability, target Capability) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
