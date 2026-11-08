package common

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/collector"
	"platform/agent/common/config"
	"platform/agent/common/identity"
	"platform/agent/common/telemetry"
)

type lifecycleCollector struct {
	name         string
	ignoreCancel bool
	started      chan struct{}
	release      chan struct{}
	delay        time.Duration
	err          error
	calls        atomic.Int32
}

func (c *lifecycleCollector) Name() string { return c.name }
func (c *lifecycleCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	c.calls.Add(1)
	if c.started != nil {
		select {
		case c.started <- struct{}{}:
		default:
		}
	}
	if c.ignoreCancel {
		if c.release != nil {
			<-c.release
		} else if c.delay > 0 {
			time.Sleep(c.delay)
		}
		if c.err != nil {
			return nil, c.err
		}
		return []telemetry.Sample{{MetricKey: "system.cpu.utilization", Value: 1, Unit: "%"}}, nil
	}
	if c.release != nil {
		select {
		case <-c.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if c.delay > 0 {
		select {
		case <-time.After(c.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if c.err != nil {
		return nil, c.err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return []telemetry.Sample{{MetricKey: "system.cpu.utilization", Value: 1, Unit: "%"}}, nil
}

type blockingTransport struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (t *blockingTransport) Available() bool { return true }
func (t *blockingTransport) Deliver(ctx context.Context, _ client.Batch) (client.Ack, error) {
	t.calls.Add(1)
	if t.entered != nil {
		select {
		case t.entered <- struct{}{}:
		default:
		}
	}
	if t.release != nil {
		<-t.release // ignore ctx: non-cooperative
		return client.Ack{}, errors.New("blocking transport released")
	}
	select {
	case <-ctx.Done():
		return client.Ack{}, ctx.Err()
	case <-time.After(50 * time.Millisecond):
		return client.Ack{}, &client.DeliveryError{Code: "SENSOR_UNAVAILABLE_TEST", Class: client.FailureReceiverUnavailable}
	}
}

type failTransport struct {
	err error
}

func (t *failTransport) Available() bool { return true }
func (t *failTransport) Deliver(_ context.Context, _ client.Batch) (client.Ack, error) {
	return client.Ack{}, t.err
}

func testAgentConfig(t *testing.T, healthAddr string) *config.Config {
	t.Helper()
	tempDir := t.TempDir()
	if healthAddr == "" {
		healthAddr = "127.0.0.1:0"
	}
	return &config.Config{
		TenantID:           "tenant-test",
		SensorURL:          "https://sensor.platform.local:8443",
		CoreURL:            "https://core.platform.local:8443",
		StateDir:           tempDir,
		QueueDir:           filepath.Join(tempDir, "queue"),
		HealthAddress:      healthAddr,
		CollectionInterval: 50 * time.Millisecond,
		CollectionTimeout:  500 * time.Millisecond,
		DeliveryTimeout:    500 * time.Millisecond,
		RetryBase:          10 * time.Millisecond,
		RetryMax:           50 * time.Millisecond,
		MaxBufferRecords:   100,
		MaxBufferBytes:     1024 * 1024,
		MaxRecordBytes:     64 * 1024,
		MaxBatchRecords:    10,
		MaxBatchBytes:      1024 * 1024,
		MaxFilesystems:     8,
	}
}

func testAgentIdent(id string) *identity.AgentIdentity {
	return &identity.AgentIdentity{
		AgentID:  id,
		TenantID: "tenant-test",
		HostID:   "host-" + id,
		Hostname: "node-test",
		OS:       "linux",
		Arch:     "amd64",
	}
}

func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func singleCollectorRegistry(t *testing.T, c collector.HostCollector, interval, timeout time.Duration) *collector.Registry {
	t.Helper()
	reg, err := collector.NewRegistry([]collector.Registration{{
		Collector: c,
		Enabled:   true,
		Interval:  interval,
		Timeout:   timeout,
	}})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

func TestAgentServiceLifecycleStatesCleanPath(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	cfg.CollectionInterval = time.Hour
	col := &lifecycleCollector{name: "cpu", delay: time.Millisecond}
	reg := singleCollectorRegistry(t, col, time.Hour, time.Second)
	stub := client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST")
	svc, err := NewAgentService(cfg, testAgentIdent("agent-life-clean"), stub, nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if got := svc.LifecycleState(); got != LifecycleNew {
		t.Fatalf("expected NEW, got %s", got)
	}

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got := svc.LifecycleState(); got != LifecycleRunning {
		t.Fatalf("expected RUNNING, got %s", got)
	}

	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := svc.LifecycleState(); got != LifecycleStopped {
		t.Fatalf("expected STOPPED, got %s", got)
	}
}

func TestAgentServiceDoubleStartRejected(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	cfg.CollectionInterval = time.Hour
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu"}, time.Hour, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-double-start"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := svc.Start(context.Background()); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("expected ErrAlreadyRunning, got %v", err)
	}
	_ = svc.Stop()
}

func TestAgentServiceStopAfterStoppedIdempotent(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	cfg.CollectionInterval = time.Hour
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu"}, time.Hour, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-stop-idem"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if got := svc.LifecycleState(); got != LifecycleStopped {
		t.Fatalf("expected STOPPED, got %s", got)
	}
}

func TestAgentServiceStartAfterStoppedRejected(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	cfg.CollectionInterval = time.Hour
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu"}, time.Hour, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-no-restart"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := svc.Start(context.Background()); !errors.Is(err, ErrRestartUnsupported) {
		t.Fatalf("expected ErrRestartUnsupported, got %v", err)
	}
}

func TestAgentServiceStopBeforeStart(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu"}, time.Hour, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-stop-first"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}
	if got := svc.LifecycleState(); got != LifecycleStopped {
		t.Fatalf("expected STOPPED, got %s", got)
	}
	if err := svc.Start(context.Background()); !errors.Is(err, ErrRestartUnsupported) {
		t.Fatalf("expected ErrRestartUnsupported after stop-before-start, got %v", err)
	}
}

func TestAgentServiceConcurrentStopOnce(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	cfg.CollectionInterval = time.Hour
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu"}, time.Hour, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-conc-stop"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- svc.Stop()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent Stop error: %v", err)
		}
	}
	if got := svc.LifecycleState(); got != LifecycleStopped {
		t.Fatalf("expected STOPPED, got %s", got)
	}
}

func TestAgentServiceCanceledStartContextNoPartialRuntime(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu"}, time.Hour, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-bad-ctx"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.Start(ctx); !errors.Is(err, ErrInvalidStartContext) {
		t.Fatalf("expected ErrInvalidStartContext, got %v", err)
	}
	if got := svc.LifecycleState(); got != LifecycleFailed {
		t.Fatalf("expected FAILED, got %s", got)
	}
	// Health must not be accepting.
	clientHTTP := &http.Client{Timeout: 100 * time.Millisecond}
	resp, err := clientHTTP.Get("http://" + cfg.HealthAddress + "/live")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("health listener should not accept after canceled Start")
	}
}

func TestAgentServiceHealthBindFailureFailedNoLeak(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	cfg := testAgentConfig(t, ln.Addr().String())
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu"}, time.Hour, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-health-fail"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := svc.Start(context.Background()); err == nil {
		t.Fatal("expected health bind failure")
	}
	if got := svc.LifecycleState(); got != LifecycleFailed {
		t.Fatalf("expected FAILED, got %s", got)
	}
	if st := svc.SchedulerStatus(); st.Running {
		t.Fatal("scheduler must not be running after health bind failure")
	}
}

func TestAgentServiceRetainsQueueOnShutdown(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	cfg.CollectionInterval = 20 * time.Millisecond
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu", delay: time.Millisecond}, 20*time.Millisecond, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-retain"), client.NewUnavailableTransport("SENSOR_UNAVAILABLE_TEST"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for svc.QueueStats().ActiveCount < 1 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if svc.QueueStats().ActiveCount < 1 {
		t.Fatal("expected buffered queue records before stop")
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if svc.QueueStats().ActiveCount < 1 {
		t.Fatal("shutdown must retain unacknowledged queue records")
	}
}

func TestAgentServiceBoundedShutdownNonCooperativeDelivery(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	cfg.DeliveryTimeout = time.Minute // transport ignores cancel; service budget must still bind Stop
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	tr := &blockingTransport{entered: entered, release: release}
	col := &lifecycleCollector{name: "cpu", delay: time.Millisecond}
	reg := singleCollectorRegistry(t, col, 20*time.Millisecond, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-bound-stop"), tr, nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	svc.setShutdownBudgetForTest(150 * time.Millisecond)

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	queueDeadline := time.Now().Add(3 * time.Second)
	for svc.QueueStats().ActiveCount < 1 && time.Now().Before(queueDeadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if svc.QueueStats().ActiveCount < 1 {
		t.Fatal("expected queue records before blocking delivery")
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("delivery did not enter blocking transport")
	}

	start := time.Now()
	err = svc.Stop()
	elapsed := time.Since(start)
	if err == nil || !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("expected ErrShutdownTimeout, got %v", err)
	}
	if elapsed > 800*time.Millisecond {
		t.Fatalf("Stop exceeded service budget significantly: %v", elapsed)
	}
	if got := svc.LifecycleState(); got != LifecycleFailed {
		t.Fatalf("expected FAILED after timeout, got %s", got)
	}

	// Repeated Stop must not spawn additional waiters / hang.
	start = time.Now()
	err2 := svc.Stop()
	if !errors.Is(err2, ErrShutdownTimeout) {
		t.Fatalf("repeated Stop expected prior ErrShutdownTimeout, got %v", err2)
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("repeated Stop appeared to wait again: %v", time.Since(start))
	}

	close(release)
	// Prove the non-cooperative deliverer/waiter cleans up after release.
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cleanupCancel()
	if err := svc.worker.StopContext(cleanupCtx); err != nil {
		t.Fatalf("worker did not finish after release: %v", err)
	}
}

func TestAgentServiceHealthNotAcceptingAfterStop(t *testing.T) {
	addr := freeLoopbackAddr(t)
	cfg := testAgentConfig(t, addr)
	cfg.CollectionInterval = time.Hour
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu"}, time.Hour, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-health-down"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	httpClient := &http.Client{Timeout: 200 * time.Millisecond}
	resp, err := httpClient.Get("http://" + addr + "/live")
	if err != nil {
		t.Fatalf("live before stop: %v", err)
	}
	_ = resp.Body.Close()

	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	resp, err = httpClient.Get("http://" + addr + "/live")
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("health must not accept after Stop")
	}
}

func TestAgentServiceShutdownDoesNotRecordCollectorFailure(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &lifecycleCollector{name: "cpu", started: started, release: release}
	reg := singleCollectorRegistry(t, col, 10*time.Millisecond, 2*time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-cancel-collect"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}
	_, errsBefore := svc.CollectorStats()
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	close(release)
	_, errsAfter := svc.CollectorStats()
	if errsAfter != errsBefore {
		t.Fatalf("shutdown cancellation recorded collector failure: before=%d after=%d", errsBefore, errsAfter)
	}
}

func TestAgentServiceRealCollectionDeadlineStillFails(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	cfg.CollectionInterval = 20 * time.Millisecond
	cfg.CollectionTimeout = 30 * time.Millisecond
	col := &lifecycleCollector{name: "cpu", ignoreCancel: true, delay: 500 * time.Millisecond}
	reg := singleCollectorRegistry(t, col, 20*time.Millisecond, 30*time.Millisecond)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-real-timeout"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var errs int
	for time.Now().Before(deadline) {
		_, errs = svc.CollectorStats()
		if errs > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if errs == 0 {
		t.Fatal("expected genuine collection deadline to record failure")
	}
	_ = svc.Stop()
}

func TestAgentServiceShutdownDoesNotRecordDeliveryFailure(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	cfg.CollectionInterval = 15 * time.Millisecond
	entered := make(chan struct{}, 1)
	// Cooperative transport: waits on ctx so shutdown cancel returns DELIVERY_CONTEXT_ABORTED-like cancel.
	tr := &coopCancelTransport{entered: entered}
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu", delay: time.Millisecond}, 15*time.Millisecond, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-cancel-deliv"), tr, nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("delivery did not start")
	}
	before := svc.DeliveryStats()
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	after := svc.DeliveryStats()
	if after.RetryAttempt != before.RetryAttempt {
		t.Fatalf("shutdown cancellation incremented retry state: before=%d after=%d", before.RetryAttempt, after.RetryAttempt)
	}
	if after.LastErrorCode != before.LastErrorCode && after.LastErrorCode == "DELIVERY_CONTEXT_ABORTED" {
		t.Fatalf("shutdown cancellation recorded delivery failure code %q", after.LastErrorCode)
	}
}

func TestAgentServiceRealDeliveryFailureStillRecorded(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	cfg.CollectionInterval = 15 * time.Millisecond
	tr := &failTransport{err: &client.DeliveryError{Code: "SENSOR_UNAVAILABLE_TEST", Class: client.FailureReceiverUnavailable}}
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu", delay: time.Millisecond}, 15*time.Millisecond, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-real-deliv-fail"), tr, nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var st client.FailureClass
	for time.Now().Before(deadline) {
		d := svc.DeliveryStats()
		if d.RetryAttempt > 0 || d.LastErrorCode != "" {
			st = d.LastFailureClass
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st == "" && svc.DeliveryStats().LastErrorCode == "" {
		t.Fatal("expected real delivery failure to be recorded")
	}
	_ = svc.Stop()
}

func TestAgentServiceStartStopConcurrency(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	cfg.CollectionInterval = time.Hour
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu"}, time.Hour, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-start-stop-race"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = svc.Start(context.Background())
		}()
		go func() {
			defer wg.Done()
			_ = svc.Stop()
		}()
	}
	wg.Wait()
	_ = svc.Stop()
	state := svc.LifecycleState()
	if state != LifecycleStopped && state != LifecycleFailed {
		t.Fatalf("expected terminal state, got %s", state)
	}
}

type coopCancelTransport struct {
	entered chan struct{}
}

func (t *coopCancelTransport) Available() bool { return true }
func (t *coopCancelTransport) Deliver(ctx context.Context, _ client.Batch) (client.Ack, error) {
	if t.entered != nil {
		select {
		case t.entered <- struct{}{}:
		default:
		}
	}
	<-ctx.Done()
	return client.Ack{}, ctx.Err()
}

func TestAgentServiceNonCooperativeCollectorStopTimesOut(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &lifecycleCollector{
		name:         "cpu",
		ignoreCancel: true,
		started:      started,
		release:      release,
	}
	reg := singleCollectorRegistry(t, col, 10*time.Millisecond, 40*time.Millisecond)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-noncoop-collect"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	svc.setShutdownBudgetForTest(120 * time.Millisecond)

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}

	// Wait until scheduler has an outstanding non-cooperative waiter.
	deadline := time.Now().Add(2 * time.Second)
	for {
		st := svc.SchedulerStatus()
		if len(st.Collectors) > 0 && st.Collectors[0].Outstanding {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected outstanding collector before Stop")
		}
		time.Sleep(5 * time.Millisecond)
	}

	start := time.Now()
	err = svc.Stop()
	elapsed := time.Since(start)
	if err == nil || !errors.Is(err, ErrShutdownTimeout) {
		t.Fatalf("expected ErrShutdownTimeout, got %v", err)
	}
	if elapsed > 800*time.Millisecond {
		t.Fatalf("Stop exceeded bounded budget significantly: %v", elapsed)
	}
	if got := svc.LifecycleState(); got != LifecycleFailed {
		t.Fatalf("expected FAILED, got %s", got)
	}

	start = time.Now()
	err2 := svc.Stop()
	if !errors.Is(err2, ErrShutdownTimeout) {
		t.Fatalf("repeated Stop expected prior timeout err, got %v", err2)
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("repeated Stop waited again: %v", time.Since(start))
	}

	close(release)
	cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := svc.scheduler.StopContext(cleanup); err != nil {
		t.Fatalf("scheduler outstanding did not clean up after release: %v", err)
	}
	st := svc.SchedulerStatus()
	if len(st.Collectors) > 0 && st.Collectors[0].Outstanding {
		t.Fatal("outstanding must clear after release")
	}
}

func TestAgentServiceStartContextDetachedFromRuntime(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	cfg.CollectionInterval = time.Hour
	reg := singleCollectorRegistry(t, &lifecycleCollector{name: "cpu"}, time.Hour, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-start-detach"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}

	startCtx, startCancel := context.WithCancel(context.Background())
	if err := svc.Start(startCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Cancelling the caller's Start context must not terminate the service runtime.
	startCancel()
	time.Sleep(50 * time.Millisecond)
	if got := svc.LifecycleState(); got != LifecycleRunning {
		t.Fatalf("expected RUNNING after Start-context cancel, got %s", got)
	}
	if !svc.SchedulerStatus().Running {
		t.Fatal("scheduler must keep running when only Start context is cancelled")
	}
	if err := svc.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := svc.LifecycleState(); got != LifecycleStopped {
		t.Fatalf("expected STOPPED after Stop, got %s", got)
	}
}

func TestAgentServiceCollectorDeadlineFirstRemainsFailure(t *testing.T) {
	cfg := testAgentConfig(t, freeLoopbackAddr(t))
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &lifecycleCollector{
		name:         "cpu",
		ignoreCancel: true,
		started:      started,
		release:      release,
	}
	reg := singleCollectorRegistry(t, col, 10*time.Millisecond, 40*time.Millisecond)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-deadline-first"), client.NewUnavailableTransport("x"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatalf("construct: %v", err)
	}

	barrierHit := make(chan struct{})
	proceed := make(chan struct{})
	var once sync.Once
	svc.scheduler.SetClassifyBarrierForTest(func() {
		once.Do(func() { close(barrierHit) })
		<-proceed
	})

	if err := svc.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}
	select {
	case <-barrierHit:
	case <-time.After(2 * time.Second):
		t.Fatal("classify barrier not reached")
	}

	// Begin service shutdown only after collector deadline classification is ready.
	go func() { _ = svc.Stop() }()
	// Give Stop a moment to cancel runtime, then allow classification.
	time.Sleep(20 * time.Millisecond)
	close(proceed)
	close(release)

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, errs := svc.CollectorStats()
		if errs > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("deadline-first collector timeout must remain recorded")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Ensure Stop finishes (may timeout on outstanding until release already closed).
	_ = svc.Stop()
}

// Fixed cardinality/label bytes reproduce the oversized lifecycle collection
// without depending on the runner's live /proc population.
type integrityLifecycleCollector struct{}

func (*integrityLifecycleCollector) Name() string { return "fixture" }
func (*integrityLifecycleCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	samples := make([]telemetry.Sample, 192)
	for i := range samples {
		samples[i] = telemetry.Sample{MetricKey: "fixture.metric", Value: float64(i), Unit: "count", Labels: map[string]string{"fixture": strings.Repeat("x", 128)}}
	}
	return samples, nil
}

func TestIssue302RuntimeSnapshotCarriesAdmissionLoss(t *testing.T) {
	cfg := testAgentConfig(t, "127.0.0.1:0")
	cfg.MaxRecordBytes = 128 // Smaller than the immutable collection envelope.
	reg := singleCollectorRegistry(t, &integrityLifecycleCollector{}, time.Hour, time.Second)
	svc, err := NewAgentService(cfg, testAgentIdent("agent-302"), client.NewUnavailableTransport("TEST_UNAVAILABLE"), nil, withCollectorRegistryForTest(reg))
	if err != nil {
		t.Fatal(err)
	}
	svc.scheduler.RunOnce(context.Background())
	st := svc.RuntimeSnapshot()
	if st.Admission.DroppedObservations != 192 || st.Admission.DurableObservations != 0 || svc.QueueStats().ActiveCount != 0 {
		t.Fatalf("health adapter hid loss: %+v", st.Admission)
	}
}
