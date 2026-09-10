package collector

import (
	"context"

	"platform/agent/common/telemetry"
)

type HostCollector interface {
	Name() string
	Collect(ctx context.Context) ([]telemetry.Sample, error)
}
