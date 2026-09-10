//go:build !linux

package collector

import (
	"context"

	"platform/agent/common/telemetry"
)

func NewDefaultCollectors(maxFilesystems int) []HostCollector {
	return []HostCollector{
		&StubCollector{name: "stub"},
	}
}

type StubCollector struct {
	name string
}

func (s *StubCollector) Name() string { return s.name }

func (s *StubCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	return nil, nil
}
