package runtime

import (
	"context"
	"fmt"
	"sync"

	"platform/sensor/internal/scheduler"
)

type Route struct {
	Capability Capability
	Class      scheduler.ExecutionClass
	Executor   scheduler.Executor
}

type ExecutorRegistry struct {
	mu     sync.RWMutex
	routes map[scheduler.JobKind]Route
}

func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{routes: make(map[scheduler.JobKind]Route)}
}

func (r *ExecutorRegistry) Register(capability Capability, kind scheduler.JobKind, class scheduler.ExecutionClass, executor scheduler.Executor) error {
	if r == nil {
		return fmt.Errorf("SENSOR_RUNTIME_REGISTRY_UNAVAILABLE: executor registry is nil")
	}
	if capability == "" || kind == "" || class == "" || executor == nil {
		return fmt.Errorf("SENSOR_RUNTIME_INVALID_EXECUTOR: capability, job kind, class and executor are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.routes[kind]; ok {
		return fmt.Errorf("SENSOR_RUNTIME_EXECUTOR_CONFLICT: job kind %s already registered for capability %s", kind, existing.Capability)
	}
	r.routes[kind] = Route{Capability: capability, Class: class, Executor: executor}
	return nil
}

func (r *ExecutorRegistry) HasCapability(capability Capability) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, route := range r.routes {
		if route.Capability == capability {
			return true
		}
	}
	return false
}

func (r *ExecutorRegistry) Execute(ctx context.Context, job scheduler.Job) (scheduler.ExecutionResult, error) {
	if r == nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(fmt.Errorf("SENSOR_RUNTIME_REGISTRY_UNAVAILABLE: executor registry is nil"))
	}
	r.mu.RLock()
	route, ok := r.routes[job.Kind]
	r.mu.RUnlock()
	if !ok {
		return scheduler.ExecutionResult{}, scheduler.Permanent(fmt.Errorf("SENSOR_RUNTIME_EXECUTOR_NOT_FOUND: no executor registered for job kind %s", job.Kind))
	}
	if job.Class != route.Class {
		return scheduler.ExecutionResult{}, scheduler.Permanent(fmt.Errorf("SENSOR_RUNTIME_EXECUTION_CLASS_MISMATCH: job kind %s requires class %s", job.Kind, route.Class))
	}
	return route.Executor.Execute(ctx, job)
}
