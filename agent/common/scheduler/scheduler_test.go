package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/agent/common/collector"
	"platform/agent/common/identity"
	"platform/agent/common/queue"
	"platform/agent/common/telemetry"
)

type mockEnqueuer struct {
	mu      sync.Mutex
	records [][]byte
	err     error
}

func (m *mockEnqueuer) Enqueue(payload []byte) (*queue.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	m.records = append(m.records, payload)
	return &queue.Record{Sequence: uint64(len(m.records)), Payload: payload}, nil
}

func (m *mockEnqueuer) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.records)
}

type mockCollector struct {
	name      string
	samples   []telemetry.Sample
	err       error
	delay     time.Duration
	started   chan struct{}
	release   chan struct{}
	calls     atomic.Int32
	maxSeen   *atomic.Int32
	inFlight  *atomic.Int32
	enterHook func()
}

func (m *mockCollector) Name() string { return m.name }
func (m *mockCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	m.calls.Add(1)
	if m.inFlight != nil && m.maxSeen != nil {
		cur := m.inFlight.Add(1)
		for {
			prev := m.maxSeen.Load()
			if cur <= prev || m.maxSeen.CompareAndSwap(prev, cur) {
				break
			}
		}
		defer m.inFlight.Add(-1)
	}
	if m.enterHook != nil {
		m.enterHook()
	}
	if m.started != nil {
		select {
		case m.started <- struct{}{}:
		default:
		}
	}
	if m.release != nil {
		select {
		case <-m.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if m.err != nil {
		return nil, m.err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return m.samples, nil
}

func testIdent() *identity.AgentIdentity {
	return &identity.AgentIdentity{
		AgentID:  "agent-sched-1",
		TenantID: "tenant-1",
		HostID:   "host-1",
		Hostname: "node-1",
		OS:       "linux",
		Arch:     "amd64",
	}
}

func mustRegistry(t *testing.T, regs ...collector.Registration) *collector.Registry {
	t.Helper()
	reg, err := collector.NewRegistry(regs)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

func TestSchedulerRunOnce(t *testing.T) {
	c1 := &mockCollector{
		name: "cpu",
		samples: []telemetry.Sample{
			{MetricKey: "system.cpu.utilization", Value: 25.0, Unit: "%"},
		},
	}
	c2 := &mockCollector{
		name: "mem_failing",
		err:  errors.New("simulated error"),
	}

	reg := mustRegistry(t,
		collector.Registration{Collector: c1, Enabled: true, Interval: time.Second, Timeout: time.Second},
		collector.Registration{Collector: c2, Enabled: true, Interval: time.Second, Timeout: time.Second},
	)

	enq := &mockEnqueuer{}
	sched, err := NewScheduler(testIdent(), reg, enq, 2, nil)
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}

	sched.RunOnce(context.Background())

	// Per-collector enqueue: success batch + error batch.
	if enq.count() != 2 {
		t.Fatalf("expected 2 enqueued records, got %d", enq.count())
	}

	var sawObs, sawErr bool
	enq.mu.Lock()
	defer enq.mu.Unlock()
	for _, raw := range enq.records {
		var batch telemetry.CollectionBatch
		if err := json.Unmarshal(raw, &batch); err != nil {
			t.Fatalf("failed to unmarshal batch: %v", err)
		}
		if len(batch.Observations) == 1 && batch.Observations[0].MetricKey == "system.cpu.utilization" {
			sawObs = true
		}
		if len(batch.Errors) == 1 && batch.Errors[0].Collector == "mem_failing" {
			sawErr = true
		}
	}
	if !sawObs || !sawErr {
		t.Fatalf("expected observation and error batches, sawObs=%v sawErr=%v", sawObs, sawErr)
	}

	_, runErrors := sched.Stats()
	if runErrors < 1 {
		t.Fatalf("expected runErrors>=1, got %d", runErrors)
	}
	st := sched.Status()
	if st.EnabledCollectors != 2 || st.TotalCollectors != 2 {
		t.Fatalf("status collectors enabled=%d total=%d", st.EnabledCollectors, st.TotalCollectors)
	}
}

func TestSchedulerLifecycle(t *testing.T) {
	c := &mockCollector{name: "stub", samples: []telemetry.Sample{{MetricKey: "k", Value: 1, Unit: "u"}}}
	reg := mustRegistry(t, collector.Registration{
		Collector: c, Enabled: true, Interval: 40 * time.Millisecond, Timeout: time.Second,
	})
	enq := &mockEnqueuer{}

	sched, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sched.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for {
		if enq.count() >= 2 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			sched.Stop()
			t.Fatalf("expected at least 2 collections, got %d", enq.count())
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	sched.Stop()

	if enq.count() < 2 {
		t.Fatalf("expected at least 2 collections, got %d", enq.count())
	}
	if sched.Status().Running {
		t.Fatal("scheduler should not report running after Stop")
	}
}

func TestSchedulerTimeoutIsolation(t *testing.T) {
	// A blocks until timeout; B completes quickly and must not be cancelled by A's timeout.
	aStarted := make(chan struct{}, 1)
	a := &mockCollector{
		name:    "slow_a",
		delay:   200 * time.Millisecond,
		started: aStarted,
		samples: []telemetry.Sample{{MetricKey: "a", Value: 1, Unit: "u"}},
	}
	bDone := make(chan struct{})
	b := &mockCollector{
		name: "fast_b",
		samples: []telemetry.Sample{
			{MetricKey: "b", Value: 2, Unit: "u"},
		},
		enterHook: func() { close(bDone) },
	}

	reg := mustRegistry(t,
		collector.Registration{Collector: a, Enabled: true, Interval: time.Hour, Timeout: 50 * time.Millisecond},
		collector.Registration{Collector: b, Enabled: true, Interval: time.Hour, Timeout: time.Second},
	)
	enq := &mockEnqueuer{}
	sched, err := NewScheduler(testIdent(), reg, enq, 2, nil)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	sched.RunOnce(context.Background())
	elapsed := time.Since(start)

	select {
	case <-bDone:
	default:
		t.Fatal("fast collector B never started — timeout isolation broken")
	}
	if b.calls.Load() != 1 {
		t.Fatalf("B calls=%d want 1", b.calls.Load())
	}
	if a.calls.Load() != 1 {
		t.Fatalf("A calls=%d want 1", a.calls.Load())
	}
	// Wall clock should be near A's timeout, not serialized A+B delays.
	if elapsed > 350*time.Millisecond {
		t.Fatalf("execution looks serialized: elapsed=%v", elapsed)
	}

	st := sched.Status()
	var aStatus, bStatus *CollectorStatus
	for i := range st.Collectors {
		switch st.Collectors[i].Name {
		case "slow_a":
			aStatus = &st.Collectors[i]
		case "fast_b":
			bStatus = &st.Collectors[i]
		}
	}
	if aStatus == nil || bStatus == nil {
		t.Fatal("missing collector status")
	}
	if aStatus.ErrorCount < 1 {
		t.Fatalf("A should record timeout error, status=%+v", *aStatus)
	}
	if bStatus.ErrorCount != 0 || bStatus.RunCount != 1 {
		t.Fatalf("B should succeed independently, status=%+v", *bStatus)
	}
}

func TestSchedulerBoundedConcurrency(t *testing.T) {
	var inFlight atomic.Int32
	var maxSeen atomic.Int32
	const n = 4
	const maxConc = 2

	regs := make([]collector.Registration, 0, n)
	collectors := make([]*mockCollector, 0, n)
	for i := 0; i < n; i++ {
		c := &mockCollector{
			name:     "c" + string(rune('a'+i)),
			delay:    80 * time.Millisecond,
			samples:  []telemetry.Sample{{MetricKey: "m", Value: 1, Unit: "u"}},
			inFlight: &inFlight,
			maxSeen:  &maxSeen,
		}
		collectors = append(collectors, c)
		regs = append(regs, collector.Registration{
			Collector: c, Enabled: true, Interval: time.Hour, Timeout: time.Second,
		})
	}
	reg := mustRegistry(t, regs...)
	sched, err := NewScheduler(testIdent(), reg, &mockEnqueuer{}, maxConc, nil)
	if err != nil {
		t.Fatal(err)
	}

	sched.RunOnce(context.Background())

	if got := int(maxSeen.Load()); got > maxConc {
		t.Fatalf("max concurrency exceeded: saw %d want <= %d", got, maxConc)
	}
	if got := int(maxSeen.Load()); got < 1 {
		t.Fatal("expected concurrent execution evidence")
	}
	for _, c := range collectors {
		if c.calls.Load() != 1 {
			t.Fatalf("collector %s calls=%d", c.name, c.calls.Load())
		}
	}
	if sched.Status().MaxConcurrency != maxConc {
		t.Fatalf("status MaxConcurrency=%d", sched.Status().MaxConcurrency)
	}
}

func TestSchedulerIndependentIntervals(t *testing.T) {
	fast := &mockCollector{
		name:    "fast",
		samples: []telemetry.Sample{{MetricKey: "f", Value: 1, Unit: "u"}},
	}
	slow := &mockCollector{
		name:    "slow",
		samples: []telemetry.Sample{{MetricKey: "s", Value: 1, Unit: "u"}},
	}
	reg := mustRegistry(t,
		collector.Registration{Collector: fast, Enabled: true, Interval: 25 * time.Millisecond, Timeout: time.Second},
		collector.Registration{Collector: slow, Enabled: true, Interval: 150 * time.Millisecond, Timeout: time.Second},
	)
	sched, err := NewScheduler(testIdent(), reg, &mockEnqueuer{}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx)
	// Cover at least one full slow interval plus phase offset headroom.
	time.Sleep(400 * time.Millisecond)
	sched.Stop()

	fastCalls := fast.calls.Load()
	slowCalls := slow.calls.Load()
	if fastCalls <= slowCalls {
		t.Fatalf("expected fast collector more runs than slow: fast=%d slow=%d", fastCalls, slowCalls)
	}
	if slowCalls < 1 {
		t.Fatalf("slow collector never ran: %d", slowCalls)
	}
}

func TestSchedulerPhaseSpreading(t *testing.T) {
	interval := 15 * time.Second
	names := []string{"cpu", "memory", "filesystem", "diskio", "net"}
	phases := map[time.Duration]string{}
	for _, name := range names {
		p := PhaseOffsetForTest(name, interval)
		if p < 0 || p >= interval {
			t.Fatalf("phase %v out of range for %s", p, name)
		}
		if other, ok := phases[p]; ok {
			t.Fatalf("identical phase %v for %q and %q", p, other, name)
		}
		phases[p] = name
	}
	if len(phases) != len(names) {
		t.Fatalf("expected distinct phases, got %d", len(phases))
	}

	// Registry-backed scheduler must expose non-identical phases for defaults.
	regs := make([]collector.Registration, 0, len(names))
	for _, name := range names {
		regs = append(regs, collector.Registration{
			Collector: &mockCollector{name: name, samples: []telemetry.Sample{{MetricKey: "m", Value: 1, Unit: "u"}}},
			Enabled:   true,
			Interval:  interval,
			Timeout:   time.Second,
		})
	}
	sched, err := NewScheduler(testIdent(), mustRegistry(t, regs...), &mockEnqueuer{}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[time.Duration]bool{}
	for _, c := range sched.Status().Collectors {
		if seen[c.PhaseOffset] {
			t.Fatalf("scheduler assigned identical phase %v", c.PhaseOffset)
		}
		seen[c.PhaseOffset] = true
	}
}

func TestSchedulerStopClean(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	c := &mockCollector{
		name:    "blocking",
		started: started,
		release: release,
		samples: []telemetry.Sample{{MetricKey: "m", Value: 1, Unit: "u"}},
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: c, Enabled: true, Interval: time.Millisecond, Timeout: 500 * time.Millisecond,
	})
	sched, err := NewScheduler(testIdent(), reg, &mockEnqueuer{}, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sched.Start(ctx)

	select {
	case <-started:
	case <-time.After(time.Second):
		cancel()
		close(release)
		sched.Stop()
		t.Fatal("collector did not start")
	}

	done := make(chan struct{})
	go func() {
		// Unblock collector shortly after Stop begins waiting.
		time.Sleep(20 * time.Millisecond)
		close(release)
		sched.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("Stop did not complete")
	}
	cancel()
	if sched.Status().Running {
		t.Fatal("expected not running after Stop")
	}
}

func TestSchedulerCollectorErrorDoesNotKillOthers(t *testing.T) {
	fail := &mockCollector{name: "fail", err: errors.New("boom")}
	ok := &mockCollector{
		name:    "ok",
		samples: []telemetry.Sample{{MetricKey: "ok.metric", Value: 1, Unit: "u"}},
	}
	reg := mustRegistry(t,
		collector.Registration{Collector: fail, Enabled: true, Interval: time.Hour, Timeout: time.Second},
		collector.Registration{Collector: ok, Enabled: true, Interval: time.Hour, Timeout: time.Second},
	)
	enq := &mockEnqueuer{}
	sched, err := NewScheduler(testIdent(), reg, enq, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	sched.RunOnce(context.Background())

	if ok.calls.Load() != 1 {
		t.Fatalf("ok collector calls=%d", ok.calls.Load())
	}
	st := sched.Status()
	var failStatus, okStatus CollectorStatus
	for _, c := range st.Collectors {
		switch c.Name {
		case "fail":
			failStatus = c
		case "ok":
			okStatus = c
		}
	}
	if failStatus.ErrorCount < 1 {
		t.Fatalf("fail status=%+v", failStatus)
	}
	if okStatus.ErrorCount != 0 || okStatus.RunCount != 1 {
		t.Fatalf("ok status=%+v", okStatus)
	}
	if enq.count() < 2 {
		t.Fatalf("expected both collectors to enqueue, got %d", enq.count())
	}
}

func TestSchedulerNormalizationEnqueuePreserved(t *testing.T) {
	c := &mockCollector{
		name: "cpu",
		samples: []telemetry.Sample{
			{MetricKey: "system.cpu.utilization", Value: 12.5, Unit: "%"},
			{MetricKey: "", Value: 1, Unit: "u"}, // rejected by normalize
		},
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: c, Enabled: true, Interval: time.Hour, Timeout: time.Second,
	})
	enq := &mockEnqueuer{}
	sched, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	sched.RunOnce(context.Background())
	if enq.count() != 1 {
		t.Fatalf("enqueue count=%d", enq.count())
	}
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[0], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if batch.SchemaVersion != telemetry.SchemaVersion {
		t.Fatalf("schema=%d", batch.SchemaVersion)
	}
	if batch.SourceType != "AGENT" || batch.AgentID != "agent-sched-1" {
		t.Fatalf("batch identity drift: %+v", batch)
	}
	if len(batch.Observations) != 1 || batch.Observations[0].MetricKey != "system.cpu.utilization" {
		t.Fatalf("observations=%+v", batch.Observations)
	}
}

func TestSlowCollectorDoesNotSerializeBeyondConcurrency(t *testing.T) {
	// With concurrency=2, two slow collectors plus one fast: fast should complete
	// while slows are still in flight — not wait for both slows sequentially.
	var inFlight atomic.Int32
	var maxSeen atomic.Int32
	slow1 := &mockCollector{
		name: "slow1", delay: 120 * time.Millisecond,
		samples:  []telemetry.Sample{{MetricKey: "s1", Value: 1, Unit: "u"}},
		inFlight: &inFlight, maxSeen: &maxSeen,
	}
	slow2 := &mockCollector{
		name: "slow2", delay: 120 * time.Millisecond,
		samples:  []telemetry.Sample{{MetricKey: "s2", Value: 1, Unit: "u"}},
		inFlight: &inFlight, maxSeen: &maxSeen,
	}
	fastStarted := make(chan struct{})
	fast := &mockCollector{
		name:    "fast",
		samples: []telemetry.Sample{{MetricKey: "f", Value: 1, Unit: "u"}},
		enterHook: func() {
			select {
			case <-fastStarted:
			default:
				close(fastStarted)
			}
		},
		inFlight: &inFlight, maxSeen: &maxSeen,
	}

	reg := mustRegistry(t,
		collector.Registration{Collector: slow1, Enabled: true, Interval: time.Hour, Timeout: time.Second},
		collector.Registration{Collector: slow2, Enabled: true, Interval: time.Hour, Timeout: time.Second},
		collector.Registration{Collector: fast, Enabled: true, Interval: time.Hour, Timeout: time.Second},
	)
	sched, err := NewScheduler(testIdent(), reg, &mockEnqueuer{}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	done := make(chan struct{})
	go func() {
		sched.RunOnce(context.Background())
		close(done)
	}()

	select {
	case <-fastStarted:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("fast collector blocked behind serialized slows")
	}
	<-done
	elapsed := time.Since(start)
	if int(maxSeen.Load()) > 2 {
		t.Fatalf("concurrency exceeded: %d", maxSeen.Load())
	}
	// Two waves of ~120ms under concurrency 2, not three sequential 120ms.
	if elapsed > 400*time.Millisecond {
		t.Fatalf("looks overly serialized: elapsed=%v", elapsed)
	}
}
