package config

import (
	"fmt"

	"platform/sensor/internal/scheduler"
)

// ToSchedulerConfig converts validated RuntimeConfig scheduler bounds into a
// scheduler.Config without defaults, clamping, or reinterpretation.
func ToSchedulerConfig(in SchedulerConfig) (scheduler.Config, error) {
	pools := make(map[scheduler.ExecutionClass]scheduler.PoolConfig, len(in.Pools))
	for name, pool := range in.Pools {
		class, err := mapExecutionClass(name)
		if err != nil {
			return scheduler.Config{}, err
		}
		pools[class] = scheduler.PoolConfig{
			Workers:    pool.Workers,
			QueueDepth: pool.QueueDepth,
		}
	}

	out := scheduler.Config{
		MaxWorkers:              in.MaxWorkers,
		MaxQueueDepth:           in.MaxQueueDepth,
		MaxTrackedAssignments:   in.MaxTrackedAssignments,
		MaxTrackedJobs:          in.MaxTrackedJobs,
		MaxPayloadBytes:         in.MaxPayloadBytes,
		DefaultExecutionTimeout: in.DefaultExecutionTimeout,
		ShutdownGracePeriod:     in.ShutdownGracePeriod,
		Retry: scheduler.RetryPolicy{
			MaxAttempts:    in.RetryMaxAttempts,
			InitialBackoff: in.RetryInitialBackoff,
			MaxBackoff:     in.RetryMaxBackoff,
			JitterFraction: in.RetryJitterFraction,
		},
		Pools: pools,
	}
	if err := out.Validate(); err != nil {
		return scheduler.Config{}, err
	}
	return out, nil
}

func mapExecutionClass(name string) (scheduler.ExecutionClass, error) {
	switch name {
	case string(scheduler.ClassNetworkPoll):
		return scheduler.ClassNetworkPoll, nil
	case string(scheduler.ClassStreaming):
		return scheduler.ClassStreaming, nil
	case string(scheduler.ClassInfra):
		return scheduler.ClassInfra, nil
	case string(scheduler.ClassDiscovery):
		return scheduler.ClassDiscovery, nil
	case string(scheduler.ClassNCM):
		return scheduler.ClassNCM, nil
	case string(scheduler.ClassSynthetic):
		return scheduler.ClassSynthetic, nil
	default:
		return "", fmt.Errorf("SENSOR_CONFIG_SCHEDULER_POOL_CLASS_INVALID: %s", name)
	}
}
