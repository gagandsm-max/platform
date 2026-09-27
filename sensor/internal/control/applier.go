package control

import (
	"context"

	"platform/sensor/internal/config"
)

// Applier applies a fully validated RuntimeConfig candidate for an exact generation/checksum.
// Synchronous completion means the runtime accepted the configuration. The same
// generation/checksum must be safe to retry. Slice 4 attaches production Sensor runtime.
type Applier interface {
	Apply(ctx context.Context, generation uint64, checksum string, cfg config.RuntimeConfig) error
}

// FuncApplier adapts a function to Applier.
type FuncApplier func(ctx context.Context, generation uint64, checksum string, cfg config.RuntimeConfig) error

func (f FuncApplier) Apply(ctx context.Context, generation uint64, checksum string, cfg config.RuntimeConfig) error {
	return f(ctx, generation, checksum, cfg)
}
