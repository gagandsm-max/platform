package scheduler

import (
	"context"
	"testing"
	"time"
)

func TestBackoffExponentialAndBounded(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:    8,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     40 * time.Millisecond,
		JitterFraction: 0,
	}
	identityJitter := func(delay time.Duration, _ float64) time.Duration { return delay }

	cases := []struct {
		retry int
		want  time.Duration
	}{
		{retry: 1, want: 10 * time.Millisecond},
		{retry: 2, want: 20 * time.Millisecond},
		{retry: 3, want: 40 * time.Millisecond},
		{retry: 4, want: 40 * time.Millisecond},
		{retry: 8, want: 40 * time.Millisecond},
	}
	for _, tc := range cases {
		if got := backoffDuration(policy, tc.retry, identityJitter); got != tc.want {
			t.Fatalf("backoffDuration(retry=%d) = %s, want %s", tc.retry, got, tc.want)
		}
	}
}

func TestBackoffJitterCannotExceedMaximum(t *testing.T) {
	policy := RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
		JitterFraction: 1,
	}
	overMaxJitter := func(time.Duration, float64) time.Duration { return time.Hour }
	if got := backoffDuration(policy, 2, overMaxJitter); got != policy.MaxBackoff {
		t.Fatalf("backoffDuration() = %s, want max %s", got, policy.MaxBackoff)
	}
}

func TestWaitWithContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if err := waitWithContext(ctx, time.Hour); err == nil {
		t.Fatal("waitWithContext() error = nil, want cancellation")
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("cancelled wait took %s", elapsed)
	}
}
