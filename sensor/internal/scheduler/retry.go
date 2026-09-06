package scheduler

import (
	"context"
	"math/rand"
	"time"
)

const maxDuration = time.Duration(1<<63 - 1)

type runtimeHooks struct {
	now    func() time.Time
	wait   func(context.Context, time.Duration) error
	jitter func(time.Duration, float64) time.Duration
}

func defaultRuntimeHooks() runtimeHooks {
	return runtimeHooks{
		now:    time.Now,
		wait:   waitWithContext,
		jitter: jitterDuration,
	}
}

func waitWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func jitterDuration(base time.Duration, fraction float64) time.Duration {
	if base <= 0 || fraction <= 0 {
		return base
	}
	factor := 1 + ((rand.Float64()*2)-1)*fraction
	value := float64(base) * factor
	if value < 0 {
		return 0
	}
	if value > float64(maxDuration) {
		return maxDuration
	}
	return time.Duration(value)
}

func backoffDuration(policy RetryPolicy, retryNumber int, jitter func(time.Duration, float64) time.Duration) time.Duration {
	if retryNumber <= 0 {
		return 0
	}
	delay := policy.InitialBackoff
	for i := 1; i < retryNumber; i++ {
		if delay >= policy.MaxBackoff/2 {
			delay = policy.MaxBackoff
			break
		}
		delay *= 2
	}
	if delay > policy.MaxBackoff {
		delay = policy.MaxBackoff
	}
	delay = jitter(delay, policy.JitterFraction)
	if delay < 0 {
		return 0
	}
	if delay > policy.MaxBackoff {
		return policy.MaxBackoff
	}
	return delay
}
