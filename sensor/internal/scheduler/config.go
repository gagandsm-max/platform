package scheduler

import "time"

type PoolConfig struct {
	Workers    int
	QueueDepth int
}

type RetryPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	JitterFraction float64
}

// Config contains explicit resource bounds. The scheduler intentionally has no
// hidden production defaults for worker/queue sizing.
type Config struct {
	MaxWorkers              int
	MaxQueueDepth           int
	MaxTrackedAssignments   int
	MaxTrackedJobs          int
	MaxPayloadBytes         int
	DefaultExecutionTimeout time.Duration
	ShutdownGracePeriod     time.Duration
	Retry                   RetryPolicy
	Pools                   map[ExecutionClass]PoolConfig
}

func (c Config) Validate() error {
	if c.MaxWorkers <= 0 {
		return newError(CodeInvalidConfig, "MaxWorkers must be greater than zero", nil)
	}
	if c.MaxQueueDepth <= 0 {
		return newError(CodeInvalidConfig, "MaxQueueDepth must be greater than zero", nil)
	}
	if c.MaxTrackedAssignments <= 0 {
		return newError(CodeInvalidConfig, "MaxTrackedAssignments must be greater than zero", nil)
	}
	if c.MaxTrackedJobs <= 0 {
		return newError(CodeInvalidConfig, "MaxTrackedJobs must be greater than zero", nil)
	}
	if c.MaxPayloadBytes <= 0 {
		return newError(CodeInvalidConfig, "MaxPayloadBytes must be greater than zero", nil)
	}
	if c.DefaultExecutionTimeout <= 0 {
		return newError(CodeInvalidConfig, "DefaultExecutionTimeout must be greater than zero", nil)
	}
	if c.ShutdownGracePeriod <= 0 {
		return newError(CodeInvalidConfig, "ShutdownGracePeriod must be greater than zero", nil)
	}
	if len(c.Pools) == 0 {
		return newError(CodeInvalidConfig, "at least one execution pool is required", nil)
	}

	totalWorkers := 0
	totalQueueDepth := 0
	for class, pool := range c.Pools {
		if class == "" {
			return newError(CodeInvalidConfig, "execution class must not be empty", nil)
		}
		if pool.Workers <= 0 {
			return newError(CodeInvalidConfig, "pool Workers must be greater than zero", nil)
		}
		if pool.QueueDepth <= 0 {
			return newError(CodeInvalidConfig, "pool QueueDepth must be greater than zero", nil)
		}
		if pool.Workers > c.MaxWorkers-totalWorkers {
			return newError(CodeInvalidConfig, "sum of pool workers exceeds MaxWorkers", nil)
		}
		totalWorkers += pool.Workers
		if pool.QueueDepth > c.MaxQueueDepth-totalQueueDepth {
			return newError(CodeInvalidConfig, "sum of pool queue depths exceeds MaxQueueDepth", nil)
		}
		totalQueueDepth += pool.QueueDepth
	}

	if c.MaxTrackedJobs < c.MaxQueueDepth || c.MaxTrackedJobs-c.MaxQueueDepth < c.MaxWorkers {
		return newError(CodeInvalidConfig, "MaxTrackedJobs must cover configured queued and active work", nil)
	}
	if c.Retry.MaxAttempts <= 0 {
		return newError(CodeInvalidConfig, "Retry.MaxAttempts must be greater than zero", nil)
	}
	if uint64(c.Retry.MaxAttempts) > uint64(^uint32(0)) {
		return newError(CodeInvalidConfig, "Retry.MaxAttempts exceeds scheduler attempt representation", nil)
	}
	if c.Retry.InitialBackoff <= 0 {
		return newError(CodeInvalidConfig, "Retry.InitialBackoff must be greater than zero", nil)
	}
	if c.Retry.MaxBackoff < c.Retry.InitialBackoff {
		return newError(CodeInvalidConfig, "Retry.MaxBackoff must be greater than or equal to InitialBackoff", nil)
	}
	if c.Retry.JitterFraction < 0 || c.Retry.JitterFraction > 1 {
		return newError(CodeInvalidConfig, "Retry.JitterFraction must be between 0 and 1", nil)
	}
	return nil
}

func cloneConfig(c Config) Config {
	cloned := c
	cloned.Pools = make(map[ExecutionClass]PoolConfig, len(c.Pools))
	for class, pool := range c.Pools {
		cloned.Pools[class] = pool
	}
	return cloned
}
