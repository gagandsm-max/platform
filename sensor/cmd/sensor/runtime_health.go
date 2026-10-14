package main

import (
	"context"
	"time"

	"platform/sensor/internal/health"
	sensorruntime "platform/sensor/internal/runtime"
)

// runtimeHealthRefreshTimeout bounds readiness refresh so /readyz cannot hang
// indefinitely if a component Ready path ever blocks. ICMP Ready remains
// non-I/O; the timeout is a defensive contract for the adapter.
const runtimeHealthRefreshTimeout = 500 * time.Millisecond

// runtimeHealthAdapter refreshes component readiness before exposing runtime
// state to /readyz. It is attached only when NETWORK runtime is activated.
type runtimeHealthAdapter struct {
	rt *sensorruntime.Runtime
}

func (a runtimeHealthAdapter) Snapshot() health.RuntimeHealth {
	if a.rt == nil {
		return health.RuntimeHealth{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), runtimeHealthRefreshTimeout)
	defer cancel()
	// Ignore RefreshHealth error: DEGRADED is a valid observed state and the
	// snapshot carries the authoritative readiness classification.
	_ = a.rt.RefreshHealth(ctx)
	return health.RuntimeHealth{State: string(a.rt.Snapshot().State)}
}
