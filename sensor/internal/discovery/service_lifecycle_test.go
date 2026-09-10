package discovery

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/sensor/internal/coreclient"
)

type lifecycleCoreClient struct {
	fetches atomic.Int64
	blockCh chan struct{}
	unblock chan struct{}
}

func (c *lifecycleCoreClient) FetchDiscoveryTask(ctx context.Context) (coreclient.DiscoveryTaskResponse, error) {
	c.fetches.Add(1)
	if c.blockCh != nil {
		select {
		case c.blockCh <- struct{}{}:
		default:
		}
	}
	if c.unblock != nil {
		select {
		case <-c.unblock:
		case <-ctx.Done():
			return coreclient.DiscoveryTaskResponse{}, ctx.Err()
		}
	}
	return coreclient.DiscoveryTaskResponse{Status: "OK", HasTask: false}, nil
}

func (c *lifecycleCoreClient) SubmitDiscoveryResults(context.Context, coreclient.DiscoveryResultSubmission, string) (coreclient.DiscoveryResultAck, error) {
	return coreclient.DiscoveryResultAck{}, nil
}

func newLifecycleService(t *testing.T, client CoreDiscoveryClient, interval time.Duration) *Service {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	exec := NewExecutor("sensor-lifecycle", 1, logger)
	return NewService("sensor-lifecycle", client, exec, interval, logger)
}

func TestServiceLifecycle_StopBeforeStart(t *testing.T) {
	client := &lifecycleCoreClient{}
	svc := newLifecycleService(t, client, time.Hour)
	svc.Stop()
	svc.Start(context.Background())
	time.Sleep(20 * time.Millisecond)
	if got := client.fetches.Load(); got != 0 {
		t.Fatalf("expected no worker after Stop-before-Start, fetches=%d", got)
	}
	svc.Stop() // idempotent
}

func TestServiceLifecycle_RepeatedStartSingleWorker(t *testing.T) {
	client := &lifecycleCoreClient{}
	svc := newLifecycleService(t, client, 15*time.Millisecond)
	ctx := context.Background()
	svc.Start(ctx)
	svc.Start(ctx)
	svc.Start(ctx)

	deadline := time.Now().Add(500 * time.Millisecond)
	for client.fetches.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	first := client.fetches.Load()
	if first < 1 {
		t.Fatal("expected at least one poll from single worker")
	}
	time.Sleep(45 * time.Millisecond)
	second := client.fetches.Load()
	if second > first+6 {
		t.Fatalf("repeated Start appeared to spawn extra workers: before=%d after=%d", first, second)
	}
	svc.Stop()
	afterStop := client.fetches.Load()
	time.Sleep(40 * time.Millisecond)
	if client.fetches.Load() != afterStop {
		t.Fatalf("polls continued after Stop: before=%d after=%d", afterStop, client.fetches.Load())
	}
}

func TestServiceLifecycle_StopIdempotentAfterStart(t *testing.T) {
	client := &lifecycleCoreClient{}
	svc := newLifecycleService(t, client, time.Hour)
	svc.Start(context.Background())
	svc.Stop()
	svc.Stop()
	svc.Stop()
}

func TestServiceLifecycle_ConcurrentStartStopNoOrphanWorker(t *testing.T) {
	client := &lifecycleCoreClient{}
	svc := newLifecycleService(t, client, 10*time.Millisecond)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			svc.Start(ctx)
		}()
		go func() {
			defer wg.Done()
			svc.Stop()
		}()
	}
	wg.Wait()
	svc.Stop()

	stable := client.fetches.Load()
	time.Sleep(50 * time.Millisecond)
	if client.fetches.Load() != stable {
		t.Fatalf("worker remained after concurrent Start/Stop settled: before=%d after=%d",
			stable, client.fetches.Load())
	}
}

func TestServiceLifecycle_StopWaitsForInflightPoll(t *testing.T) {
	unblock := make(chan struct{})
	entered := make(chan struct{}, 1)
	client := &lifecycleCoreClient{blockCh: entered, unblock: unblock}
	svc := newLifecycleService(t, client, 5*time.Millisecond)
	svc.Start(context.Background())

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("poll did not start")
	}

	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Stop returned before inflight poll finished")
	case <-time.After(30 * time.Millisecond):
	}

	close(unblock)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return after poll completed")
	}

	stable := client.fetches.Load()
	time.Sleep(40 * time.Millisecond)
	if client.fetches.Load() != stable {
		t.Fatalf("polls continued after Stop returned: before=%d after=%d", stable, client.fetches.Load())
	}
}
