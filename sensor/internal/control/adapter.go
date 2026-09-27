package control

import (
	"fmt"
	"math"
	"time"

	"platform/sensor/internal/config"
)

// OverlayDesired overlays Core-controlled v1 fields onto an existing RuntimeConfig baseline.
// Identity, TLS, Core endpoint, spool, bootstrap token and capabilities are preserved from baseline.
func OverlayDesired(baseline config.RuntimeConfig, desired DesiredConfiguration) (config.RuntimeConfig, error) {
	out := baseline
	out.LogLevel = desired.LogLevel

	scheduler, err := mapScheduler(desired.Scheduler)
	if err != nil {
		return config.RuntimeConfig{}, err
	}
	out.Scheduler = scheduler

	transport, err := mapTransport(desired.Transport)
	if err != nil {
		return config.RuntimeConfig{}, err
	}
	out.Transport = transport

	if err := out.Validate(); err != nil {
		return config.RuntimeConfig{}, newCodedError(CodeConfigInvalid, "candidate RuntimeConfig failed validation", err)
	}
	return out, nil
}

func mapScheduler(in DesiredScheduler) (config.SchedulerConfig, error) {
	defaultTimeout, err := msToDuration(in.DefaultExecutionTimeoutMs)
	if err != nil {
		return config.SchedulerConfig{}, newCodedError(CodeConfigInvalid, "defaultExecutionTimeoutMs overflow", err)
	}
	shutdownGrace, err := msToDuration(in.ShutdownGracePeriodMs)
	if err != nil {
		return config.SchedulerConfig{}, newCodedError(CodeConfigInvalid, "shutdownGracePeriodMs overflow", err)
	}
	initialBackoff, err := msToDuration(in.RetryInitialBackoffMs)
	if err != nil {
		return config.SchedulerConfig{}, newCodedError(CodeConfigInvalid, "retryInitialBackoffMs overflow", err)
	}
	maxBackoff, err := msToDuration(in.RetryMaxBackoffMs)
	if err != nil {
		return config.SchedulerConfig{}, newCodedError(CodeConfigInvalid, "retryMaxBackoffMs overflow", err)
	}
	if in.RetryJitterBasisPoints < 0 || in.RetryJitterBasisPoints > MaxJitterBasisPoints {
		return config.SchedulerConfig{}, newCodedError(CodeConfigInvalid, "retryJitterBasisPoints out of range", nil)
	}
	jitter := float64(in.RetryJitterBasisPoints) / float64(MaxJitterBasisPoints)

	pools := make(map[string]config.PoolConfig, len(in.Pools))
	for name, pool := range in.Pools {
		pools[name] = config.PoolConfig{Workers: pool.Workers, QueueDepth: pool.QueueDepth}
	}
	return config.SchedulerConfig{
		MaxWorkers:              in.MaxWorkers,
		MaxQueueDepth:           in.MaxQueueDepth,
		MaxTrackedAssignments:   in.MaxTrackedAssignments,
		MaxTrackedJobs:          in.MaxTrackedJobs,
		MaxPayloadBytes:         in.MaxPayloadBytes,
		DefaultExecutionTimeout: defaultTimeout,
		ShutdownGracePeriod:     shutdownGrace,
		RetryMaxAttempts:        in.RetryMaxAttempts,
		RetryInitialBackoff:     initialBackoff,
		RetryMaxBackoff:         maxBackoff,
		RetryJitterFraction:     jitter,
		Pools:                   pools,
	}, nil
}

func mapTransport(in DesiredTransport) (config.TransportConfig, error) {
	timeout, err := msToDuration(in.RequestTimeoutMs)
	if err != nil {
		return config.TransportConfig{}, newCodedError(CodeConfigInvalid, "requestTimeoutMs overflow", err)
	}
	return config.TransportConfig{
		RequestTimeout:     timeout,
		MaxRequestBytes:    in.MaxRequestBytes,
		MaxResponseBytes:   in.MaxResponseBytes,
		UploadBatchRecords: in.UploadBatchRecords,
		UploadBatchBytes:   in.UploadBatchBytes,
	}, nil
}

func msToDuration(ms int) (time.Duration, error) {
	if ms <= 0 {
		return 0, fmt.Errorf("milliseconds must be positive")
	}
	if int64(ms) > math.MaxInt64/int64(time.Millisecond) {
		return 0, fmt.Errorf("milliseconds overflow duration")
	}
	return time.Duration(ms) * time.Millisecond, nil
}
