package collector

import (
	"context"

	"platform/agent/common/telemetry"
)

// HostCollector is the collection execution contract. Scheduling metadata and
// startup registration live in Registry; collectors remain schedule-agnostic.
type HostCollector interface {
	Name() string
	Collect(ctx context.Context) ([]telemetry.Sample, error)
}
