package main

import (
	"context"

	"platform/sensor/internal/health"
	sensorruntime "platform/sensor/internal/runtime"
)

// runtimeHealthAdapter refreshes component readiness before exposing runtime
// state to /readyz. It is attached only when NETWORK runtime is activated.
//
// Ready() is contractually non-blocking (local state/mutex only). A context
// timeout cannot preempt a Ready that ignores cancellation without a detached
// goroutine, which would leak on hang — so this adapter does not advertise a
// false interruptible bound.
type runtimeHealthAdapter struct {
	rt *sensorruntime.Runtime
}

func (a runtimeHealthAdapter) Snapshot() health.RuntimeHealth {
	if a.rt == nil {
		return health.RuntimeHealth{}
	}
	// Ignore RefreshHealth error: DEGRADED is a valid observed state and the
	// snapshot carries the authoritative readiness classification.
	_ = a.rt.RefreshHealth(context.Background())
	return health.RuntimeHealth{State: string(a.rt.Snapshot().State)}
}
