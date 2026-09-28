package config

import (
	"testing"
	"time"

	"platform/sensor/internal/scheduler"
)

func validSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		MaxWorkers:              8,
		MaxQueueDepth:           16,
		MaxTrackedAssignments:   64,
		MaxTrackedJobs:          24,
		MaxPayloadBytes:         1048576,
		DefaultExecutionTimeout: 10 * time.Second,
		ShutdownGracePeriod:     5 * time.Second,
		RetryMaxAttempts:        3,
		RetryInitialBackoff:     100 * time.Millisecond,
		RetryMaxBackoff:         2 * time.Second,
		RetryJitterFraction:     0.2,
		Pools: map[string]PoolConfig{
			"NETWORK_POLL": {Workers: 4, QueueDepth: 8},
			"STREAMING":    {Workers: 2, QueueDepth: 4},
		},
	}
}

func TestToSchedulerConfigExactMapping(t *testing.T) {
	in := validSchedulerConfig()
	out, err := ToSchedulerConfig(in)
	if err != nil {
		t.Fatalf("ToSchedulerConfig: %v", err)
	}
	if out.MaxWorkers != in.MaxWorkers ||
		out.MaxQueueDepth != in.MaxQueueDepth ||
		out.MaxTrackedAssignments != in.MaxTrackedAssignments ||
		out.MaxTrackedJobs != in.MaxTrackedJobs ||
		out.MaxPayloadBytes != in.MaxPayloadBytes ||
		out.DefaultExecutionTimeout != in.DefaultExecutionTimeout ||
		out.ShutdownGracePeriod != in.ShutdownGracePeriod {
		t.Fatalf("scalar mismatch: %+v", out)
	}
	if out.Retry.MaxAttempts != in.RetryMaxAttempts ||
		out.Retry.InitialBackoff != in.RetryInitialBackoff ||
		out.Retry.MaxBackoff != in.RetryMaxBackoff ||
		out.Retry.JitterFraction != in.RetryJitterFraction {
		t.Fatalf("retry mismatch: %+v", out.Retry)
	}
	if len(out.Pools) != 2 {
		t.Fatalf("pools=%d", len(out.Pools))
	}
	if out.Pools[scheduler.ClassNetworkPoll] != (scheduler.PoolConfig{Workers: 4, QueueDepth: 8}) {
		t.Fatalf("NETWORK_POLL=%+v", out.Pools[scheduler.ClassNetworkPoll])
	}
	if out.Pools[scheduler.ClassStreaming] != (scheduler.PoolConfig{Workers: 2, QueueDepth: 4}) {
		t.Fatalf("STREAMING=%+v", out.Pools[scheduler.ClassStreaming])
	}
}

func TestToSchedulerConfigAllPoolClasses(t *testing.T) {
	in := validSchedulerConfig()
	in.MaxWorkers = 12
	in.MaxQueueDepth = 24
	in.MaxTrackedJobs = 36
	in.Pools = map[string]PoolConfig{
		"NETWORK_POLL": {Workers: 2, QueueDepth: 4},
		"STREAMING":    {Workers: 2, QueueDepth: 4},
		"INFRA":        {Workers: 2, QueueDepth: 4},
		"DISCOVERY":    {Workers: 2, QueueDepth: 4},
		"NCM":          {Workers: 2, QueueDepth: 4},
		"SYNTHETIC":    {Workers: 2, QueueDepth: 4},
	}
	out, err := ToSchedulerConfig(in)
	if err != nil {
		t.Fatalf("ToSchedulerConfig: %v", err)
	}
	want := []scheduler.ExecutionClass{
		scheduler.ClassNetworkPoll,
		scheduler.ClassStreaming,
		scheduler.ClassInfra,
		scheduler.ClassDiscovery,
		scheduler.ClassNCM,
		scheduler.ClassSynthetic,
	}
	if len(out.Pools) != len(want) {
		t.Fatalf("pools=%d want=%d", len(out.Pools), len(want))
	}
	for _, class := range want {
		if out.Pools[class] != (scheduler.PoolConfig{Workers: 2, QueueDepth: 4}) {
			t.Fatalf("class %s=%+v", class, out.Pools[class])
		}
	}
}

func TestToSchedulerConfigUnknownPoolFailsClosed(t *testing.T) {
	in := validSchedulerConfig()
	in.Pools = map[string]PoolConfig{
		"UNKNOWN_CLASS": {Workers: 1, QueueDepth: 1},
	}
	if _, err := ToSchedulerConfig(in); err == nil {
		t.Fatal("expected unknown pool class to fail")
	}
}

func TestToSchedulerConfigInvalidResultFails(t *testing.T) {
	in := validSchedulerConfig()
	in.MaxWorkers = 1
	in.Pools = map[string]PoolConfig{
		"NETWORK_POLL": {Workers: 2, QueueDepth: 1},
	}
	if _, err := ToSchedulerConfig(in); err == nil {
		t.Fatal("expected Validate failure")
	}
}

func TestToSchedulerConfigReturnedMapIndependent(t *testing.T) {
	in := validSchedulerConfig()
	out, err := ToSchedulerConfig(in)
	if err != nil {
		t.Fatalf("ToSchedulerConfig: %v", err)
	}
	in.Pools["NETWORK_POLL"] = PoolConfig{Workers: 1, QueueDepth: 1}
	if out.Pools[scheduler.ClassNetworkPoll].Workers != 4 {
		t.Fatalf("mutated source leaked into result: %+v", out.Pools[scheduler.ClassNetworkPoll])
	}
	out.Pools[scheduler.ClassNetworkPoll] = scheduler.PoolConfig{Workers: 99, QueueDepth: 99}
	if in.Pools["NETWORK_POLL"].Workers != 1 {
		t.Fatalf("result mutation leaked into source: %+v", in.Pools["NETWORK_POLL"])
	}
}

func TestToSchedulerConfigNoHiddenDefaults(t *testing.T) {
	in := SchedulerConfig{
		MaxWorkers: 2, MaxQueueDepth: 2, MaxTrackedAssignments: 4, MaxTrackedJobs: 4,
		MaxPayloadBytes: 1024, DefaultExecutionTimeout: time.Second, ShutdownGracePeriod: time.Second,
		RetryMaxAttempts: 1, RetryInitialBackoff: time.Millisecond, RetryMaxBackoff: time.Millisecond,
		RetryJitterFraction: 0,
		Pools: map[string]PoolConfig{
			"INFRA": {Workers: 1, QueueDepth: 1},
		},
	}
	out, err := ToSchedulerConfig(in)
	if err != nil {
		t.Fatalf("ToSchedulerConfig: %v", err)
	}
	if out.Retry.JitterFraction != 0 {
		t.Fatalf("unexpected jitter default: %v", out.Retry.JitterFraction)
	}
	if len(out.Pools) != 1 || out.Pools[scheduler.ClassInfra].Workers != 1 {
		t.Fatalf("unexpected pools: %+v", out.Pools)
	}
}
