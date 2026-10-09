package ebpf

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"platform/agent/common/collector"
)

// stallingSource blocks ReadBounded until the test releases or ctx cancels.
type stallingSource struct {
	release chan struct{}
}

func (s stallingSource) ReadBounded(ctx context.Context, _ string, _ int64) ([]byte, bool, error) {
	select {
	case <-s.release:
		return nil, false, errors.New("stall released")
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
}

func (s stallingSource) Lstat(string) (os.FileInfo, error) {
	return nil, os.ErrNotExist
}

func (s stallingSource) Quality() string { return "FIXTURE" }

func TestCanceledBackendStartDoesNotPublishActive(t *testing.T) {
	agg, _ := testAgg(t)
	release := make(chan struct{})
	b, err := NewBackend(BackendConfig{
		Config: Config{
			Enabled:          true,
			PreflightTimeout: 2 * time.Second,
			RecheckMin:       time.Minute,
			RecheckMax:       time.Hour,
		},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	b.preflight = New(Config{
		Enabled:          true,
		PreflightTimeout: 2 * time.Second,
		RecheckMin:       time.Minute,
		RecheckMax:       time.Hour,
	})
	b.preflight.source = stallingSource{release: release}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- b.Start(ctx) }()

	// Allow Start to enter preflight, then cancel Agent runtime.
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("canceled Start must return error")
		}
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("Start did not observe cancellation")
	}
	close(release)

	cap := b.Capability()
	if cap.State == collector.ProcessNetworkCapabilityActive {
		t.Fatalf("canceled Start must not publish ACTIVE: %+v", cap)
	}
}

func TestCanceledBackendStartClosesUnpublishedResources(t *testing.T) {
	agg, _ := testAgg(t)
	b, err := NewBackend(BackendConfig{
		Config: Config{
			Enabled:          false, // preflight fails closed without kernel acquire
			PreflightTimeout: time.Second,
			RecheckMin:       time.Minute,
			RecheckMax:       time.Hour,
		},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled
	err = b.Start(ctx)
	if err == nil {
		t.Fatal("expected cancel/preflight failure")
	}
	if b.Capability().State == collector.ProcessNetworkCapabilityActive {
		t.Fatal("must not be ACTIVE")
	}
	b.mu.Lock()
	handles := b.handles
	reader := b.reader
	started := b.started
	b.mu.Unlock()
	if handles != nil || reader != nil || started {
		t.Fatalf("unpublished resources retained: handles=%v reader=%v started=%v", handles != nil, reader != nil, started)
	}
}

func TestBackendStopContextDoesNotBlockBehindCanceledStart(t *testing.T) {
	agg, _ := testAgg(t)
	release := make(chan struct{})
	b, err := NewBackend(BackendConfig{
		Config: Config{
			Enabled:          true,
			PreflightTimeout: 2 * time.Second,
			RecheckMin:       time.Minute,
			RecheckMax:       time.Hour,
		},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	b.preflight = New(Config{
		Enabled:          true,
		PreflightTimeout: 2 * time.Second,
		RecheckMin:       time.Minute,
		RecheckMax:       time.Hour,
	})
	b.preflight.source = stallingSource{release: release}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = b.Start(ctx)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	// Capability must remain readable (Start no longer holds mu for whole Start).
	_ = b.Capability()
	if err := b.StopContext(stopCtx); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	close(release)
	wg.Wait()
	if b.Capability().State == collector.ProcessNetworkCapabilityActive {
		t.Fatal("ACTIVE after cancel+stop")
	}
}
