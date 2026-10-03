package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
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
	calls   atomic.Int32
}

func (m *mockEnqueuer) Enqueue(payload []byte) (*queue.Record, error) {
	m.calls.Add(1)
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
	name         string
	samples      []telemetry.Sample
	err          error
	delay        time.Duration
	ignoreCancel bool
	panicOnce    atomic.Bool
	doPanic      bool
	started      chan struct{}
	release      chan struct{}
	calls        atomic.Int32
	maxSeen      *atomic.Int32
	inFlight     *atomic.Int32
	enterHook    func()
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
	if m.doPanic && m.panicOnce.CompareAndSwap(false, true) {
		panic("simulated collector panic")
	}
	if m.ignoreCancel {
		if m.release != nil {
			<-m.release
		} else if m.delay > 0 {
			time.Sleep(m.delay)
		}
		if m.err != nil {
			return nil, m.err
		}
		return m.samples, nil
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

	// Aggregate enqueue: one durable record per cycle, not per collector.
	if enq.count() != 1 {
		t.Fatalf("expected 1 aggregated enqueued record, got %d", enq.count())
	}

	enq.mu.Lock()
	var batch telemetry.CollectionBatch
	if err := json.Unmarshal(enq.records[0], &batch); err != nil {
		enq.mu.Unlock()
		t.Fatalf("failed to unmarshal batch: %v", err)
	}
	enq.mu.Unlock()

	if len(batch.Observations) != 1 || batch.Observations[0].MetricKey != "system.cpu.utilization" {
		t.Fatalf("expected cpu observation in aggregate batch, got %+v", batch.Observations)
	}
	if len(batch.Errors) != 1 || batch.Errors[0].Collector != "mem_failing" {
		t.Fatalf("expected mem_failing error in aggregate batch, got %+v", batch.Errors)
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

func TestSchedulerAggregateEnqueuePreservesHorizon(t *testing.T) {
	// Default-like cardinality: N collectors in one cycle must produce 1 queue record.
	const n = 3
	regs := make([]collector.Registration, 0, n)
	for i := 0; i < n; i++ {
		c := &mockCollector{
			name:    "c" + string(rune('a'+i)),
			samples: []telemetry.Sample{{MetricKey: "m", Value: float64(i), Unit: "u"}},
		}
		regs = append(regs, collector.Registration{
			Collector: c, Enabled: true, Interval: 15 * time.Second, Timeout: time.Second,
		})
	}
	enq := &mockEnqueuer{}
	sched, err := NewScheduler(testIdent(), mustRegistry(t, regs...), enq, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	sched.RunOnce(context.Background())
	if enq.count() != 1 {
		t.Fatalf("horizon regression: expected 1 aggregate record for %d collectors, got %d", n, enq.count())
	}
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[0], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Observations) != n {
		t.Fatalf("expected %d observations in aggregate, got %d", n, len(batch.Observations))
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
	if enq.count() != 1 {
		t.Fatalf("expected one aggregate enqueue, got %d", enq.count())
	}
}

func TestSchedulerNonCooperativeTimeoutReleasesAdmission(t *testing.T) {
	// Two non-cooperative collectors fill concurrency=2. If timeout does not release
	// admission, the healthy collector waiting for a slot is starved indefinitely.
	release1 := make(chan struct{})
	release2 := make(chan struct{})
	stuck1Started := make(chan struct{}, 1)
	stuck2Started := make(chan struct{}, 1)
	stuck1 := &mockCollector{
		name:         "stuck1",
		ignoreCancel: true,
		release:      release1,
		started:      stuck1Started,
		samples:      []telemetry.Sample{{MetricKey: "s1", Value: 1, Unit: "u"}},
	}
	stuck2 := &mockCollector{
		name:         "stuck2",
		ignoreCancel: true,
		release:      release2,
		started:      stuck2Started,
		samples:      []telemetry.Sample{{MetricKey: "s2", Value: 1, Unit: "u"}},
	}
	okStarted := make(chan struct{}, 1)
	ok := &mockCollector{
		name:    "ok",
		started: okStarted,
		samples: []telemetry.Sample{{MetricKey: "ok", Value: 1, Unit: "u"}},
	}
	reg := mustRegistry(t,
		collector.Registration{Collector: stuck1, Enabled: true, Interval: time.Hour, Timeout: 40 * time.Millisecond},
		collector.Registration{Collector: stuck2, Enabled: true, Interval: time.Hour, Timeout: 40 * time.Millisecond},
		collector.Registration{Collector: ok, Enabled: true, Interval: time.Hour, Timeout: time.Second},
	)
	enq := &mockEnqueuer{}
	sched, err := NewScheduler(testIdent(), reg, enq, 2, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	var stuckWG sync.WaitGroup
	stuckWG.Add(2)
	go func() {
		defer stuckWG.Done()
		sched.executeCollector(ctx, sched.runtimes[0])
	}()
	go func() {
		defer stuckWG.Done()
		sched.executeCollector(ctx, sched.runtimes[1])
	}()

	for _, ch := range []chan struct{}{stuck1Started, stuck2Started} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			close(release1)
			close(release2)
			t.Fatal("stuck collectors did not start")
		}
	}
	if got := sched.Status().InFlight; got != 2 {
		close(release1)
		close(release2)
		t.Fatalf("expected both admission slots held by stuck collectors, InFlight=%d", got)
	}

	okDone := make(chan struct{})
	go func() {
		sched.executeCollector(ctx, sched.runtimes[2])
		close(okDone)
	}()

	select {
	case <-okStarted:
	case <-time.After(500 * time.Millisecond):
		close(release1)
		close(release2)
		t.Fatal("ok collector starved — non-cooperative collectors held admission after timeout")
	}

	select {
	case <-okDone:
	case <-time.After(time.Second):
		close(release1)
		close(release2)
		t.Fatal("ok executeCollector did not return")
	}

	stuckWG.Wait()
	if st := sched.Status(); st.InFlight != 0 {
		close(release1)
		close(release2)
		t.Fatalf("global admission InFlight=%d want 0 after timeout release", st.InFlight)
	}
	for _, name := range []string{"stuck1", "stuck2"} {
		var st CollectorStatus
		for _, c := range sched.Status().Collectors {
			if c.Name == name {
				st = c
			}
		}
		if st.ErrorCount < 1 {
			close(release1)
			close(release2)
			t.Fatalf("%s should record timeout failure: %+v", name, st)
		}
	}
	if ok.calls.Load() != 1 {
		close(release1)
		close(release2)
		t.Fatalf("ok calls=%d", ok.calls.Load())
	}

	close(release1)
	close(release2)
	deadline := time.Now().Add(time.Second)
	for {
		still := false
		for _, c := range sched.Status().Collectors {
			if (c.Name == "stuck1" || c.Name == "stuck2") && c.InFlight {
				still = true
			}
		}
		if !still {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stuck outstanding did not clear after release")
		}
		time.Sleep(5 * time.Millisecond)
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
	ident := testIdent()
	names := []string{"cpu", "memory", "filesystem", "diskio", "net"}
	phases := map[time.Duration]string{}
	for _, name := range names {
		p := PhaseOffsetForTest(ident.TenantID, ident.AgentID, name, PhaseAlgorithmVersionForTest(), interval)
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
	sched, err := NewScheduler(ident, mustRegistry(t, regs...), &mockEnqueuer{}, 2, nil)
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
	if enq.count() != 1 {
		t.Fatalf("expected one aggregate enqueue, got %d", enq.count())
	}
}

func TestSchedulerEnqueueFailureAccountingContinues(t *testing.T) {
	ok := &mockCollector{
		name:    "ok",
		samples: []telemetry.Sample{{MetricKey: "ok.metric", Value: 1, Unit: "u"}},
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: ok, Enabled: true, Interval: 30 * time.Millisecond, Timeout: time.Second,
	})
	enq := &mockEnqueuer{err: errors.New("disk full")}
	sched, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	sched.RunOnce(context.Background())
	if enq.calls.Load() < 1 {
		t.Fatal("expected enqueue attempt")
	}
	if enq.count() != 0 {
		t.Fatalf("enqueue should have failed closed, got %d records", enq.count())
	}
	_, runErrors := sched.Stats()
	if runErrors < 1 {
		t.Fatalf("expected runErrors accounting for enqueue failure, got %d", runErrors)
	}
	st := sched.Status()
	if len(st.Collectors) != 1 || st.Collectors[0].LastError != "enqueue_failed" {
		t.Fatalf("expected enqueue_failed collector status, got %+v", st.Collectors)
	}

	// Scheduler must continue operating after enqueue failure.
	enq.err = nil
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for enq.count() < 1 {
		if time.Now().After(deadline) {
			sched.Stop()
			t.Fatal("scheduler did not continue after enqueue failure")
		}
		time.Sleep(10 * time.Millisecond)
	}
	sched.Stop()
	if ok.calls.Load() < 2 {
		t.Fatalf("expected continued collection after enqueue failure, calls=%d", ok.calls.Load())
	}
}

func TestSchedulerPanicContained(t *testing.T) {
	panicCollector := &mockCollector{
		name:    "panicky",
		doPanic: true,
		samples: []telemetry.Sample{{MetricKey: "p", Value: 1, Unit: "u"}},
	}
	ok := &mockCollector{
		name:    "ok",
		samples: []telemetry.Sample{{MetricKey: "ok.metric", Value: 1, Unit: "u"}},
	}
	reg := mustRegistry(t,
		collector.Registration{Collector: panicCollector, Enabled: true, Interval: time.Hour, Timeout: time.Second},
		collector.Registration{Collector: ok, Enabled: true, Interval: time.Hour, Timeout: time.Second},
	)
	enq := &mockEnqueuer{}
	sched, err := NewScheduler(testIdent(), reg, enq, 2, nil)
	if err != nil {
		t.Fatal(err)
	}

	sched.RunOnce(context.Background())

	if ok.calls.Load() != 1 {
		t.Fatalf("ok collector should still run after panic containment, calls=%d", ok.calls.Load())
	}
	st := sched.Status()
	var panicStatus, okStatus CollectorStatus
	for _, c := range st.Collectors {
		switch c.Name {
		case "panicky":
			panicStatus = c
		case "ok":
			okStatus = c
		}
	}
	if panicStatus.ErrorCount < 1 || panicStatus.LastError == "" {
		t.Fatalf("panic should become typed collector failure: %+v", panicStatus)
	}
	if okStatus.ErrorCount != 0 || okStatus.RunCount != 1 {
		t.Fatalf("ok status=%+v", okStatus)
	}
	if enq.count() != 1 {
		t.Fatalf("expected aggregate enqueue including panic error, got %d", enq.count())
	}
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[0], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	sawPanic := false
	for _, e := range batch.Errors {
		if e.Collector == "panicky" && e.Code == "COLLECTION_FAILED" {
			sawPanic = true
		}
	}
	if !sawPanic {
		t.Fatalf("expected panicky COLLECTION_FAILED in batch errors: %+v", batch.Errors)
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

func TestPhaseRestartStableAndFleetDistributed(t *testing.T) {
	interval := 15 * time.Second
	ident := testIdent()
	alg := PhaseAlgorithmVersionForTest()

	p1 := PhaseOffsetForTest(ident.TenantID, ident.AgentID, "cpu", alg, interval)
	p2 := PhaseOffsetForTest(ident.TenantID, ident.AgentID, "cpu", alg, interval)
	if p1 != p2 {
		t.Fatalf("phase not restart-stable: %v vs %v", p1, p2)
	}

	// Reconstructed schedulers for the same Agent must expose the same phase.
	reg := mustRegistry(t, collector.Registration{
		Collector: &mockCollector{name: "cpu", samples: []telemetry.Sample{{MetricKey: "m", Value: 1, Unit: "u"}}},
		Enabled:   true, Interval: interval, Timeout: time.Second,
	})
	s1, err := NewScheduler(ident, reg, &mockEnqueuer{}, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := NewScheduler(ident, mustRegistry(t, collector.Registration{
		Collector: &mockCollector{name: "cpu", samples: []telemetry.Sample{{MetricKey: "m", Value: 1, Unit: "u"}}},
		Enabled:   true, Interval: interval, Timeout: time.Second,
	}), &mockEnqueuer{}, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s1.Status().Collectors[0].PhaseOffset != s2.Status().Collectors[0].PhaseOffset {
		t.Fatalf("reconstructed scheduler phase drift: %v vs %v",
			s1.Status().Collectors[0].PhaseOffset, s2.Status().Collectors[0].PhaseOffset)
	}
	if s1.Status().Collectors[0].PhaseOffset != p1 {
		t.Fatalf("scheduler phase mismatch helper=%v sched=%v", p1, s1.Status().Collectors[0].PhaseOffset)
	}

	// Different Agents must not all collapse onto one phase for the same collector.
	seen := map[time.Duration]string{}
	for i := 0; i < 32; i++ {
		id := &identity.AgentIdentity{
			AgentID:  "agent-" + strconv.Itoa(i),
			TenantID: "tenant-1",
			HostID:   "host-1",
			Hostname: "node-1",
			OS:       "linux",
			Arch:     "amd64",
		}
		p := PhaseOffsetForTest(id.TenantID, id.AgentID, "cpu", alg, interval)
		seen[p] = id.AgentID
	}
	if len(seen) < 8 {
		t.Fatalf("expected fleet phase distribution across Agents, distinct=%d", len(seen))
	}

	// Next due remains anchored to the same cadence boundary across restart.
	now := time.Unix(1_700_000_000, 0).UTC()
	due1 := NextCadenceDueForTest(now, interval, p1)
	due2 := NextCadenceDueForTest(now.Add(3*time.Millisecond), interval, p1)
	if !due1.Equal(due2) {
		t.Fatalf("restart shifted cadence boundary: %v vs %v", due1, due2)
	}
	if due1.Before(now) {
		t.Fatalf("due %v before now %v", due1, now)
	}
	// Boundary must sit on the phase lattice.
	if (due1.UnixNano()-int64(p1))%int64(interval) != 0 {
		t.Fatalf("due %v not aligned to phase lattice p=%v", due1, p1)
	}
}

func TestNominalCadenceNoExecutionDrift(t *testing.T) {
	interval := 100 * time.Millisecond
	phase := 20 * time.Millisecond
	fired := time.Unix(0, int64(phase)).UTC()
	// Execution finishes 40ms after due (inside the period).
	finished := fired.Add(40 * time.Millisecond)
	next := CoalesceNextDueForTest(fired, finished, interval, phase)
	want := fired.Add(interval)
	if !next.Equal(want) {
		t.Fatalf("execution duration shifted cadence: next=%v want=%v", next, want)
	}
}

func TestMissedCyclesCoalesceSkip(t *testing.T) {
	interval := 100 * time.Millisecond
	phase := 10 * time.Millisecond
	fired := time.Unix(0, int64(phase)).UTC()
	// Wall clock jumped three periods ahead during a slow/blocked run.
	now := fired.Add(3*interval + 5*time.Millisecond)
	next := CoalesceNextDueForTest(fired, now, interval, phase)
	if !next.After(now) {
		t.Fatalf("next due %v must be after now %v", next, now)
	}
	// Exactly one future boundary — no catch-up stack.
	prev := next.Add(-interval)
	if prev.After(now) {
		t.Fatalf("coalesce failed; leftover catch-up boundary %v after now %v", prev, now)
	}
	if (next.UnixNano()-int64(phase))%int64(interval) != 0 {
		t.Fatalf("coalesced due not on phase lattice: %v", next)
	}
}

func TestNoOverlappingExecutionPerCollector(t *testing.T) {
	var inFlight atomic.Int32
	var maxSeen atomic.Int32
	release := make(chan struct{})
	started := make(chan struct{}, 8)
	c := &mockCollector{
		name:     "slow",
		release:  release,
		started:  started,
		samples:  []telemetry.Sample{{MetricKey: "m", Value: 1, Unit: "u"}},
		inFlight: &inFlight,
		maxSeen:  &maxSeen,
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: c, Enabled: true, Interval: 15 * time.Millisecond, Timeout: time.Second,
	})
	sched, err := NewScheduler(testIdent(), reg, &mockEnqueuer{}, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		sched.Stop()
		t.Fatal("collector did not start")
	}
	// Hold first execution while multiple nominal periods elapse.
	time.Sleep(80 * time.Millisecond)
	if got := int(maxSeen.Load()); got > 1 {
		close(release)
		sched.Stop()
		t.Fatalf("overlapping executions observed: maxInFlight=%d", got)
	}
	close(release)
	sched.Stop()
	if c.calls.Load() < 1 {
		t.Fatal("expected at least one execution")
	}
}

func TestScheduleBoundedWhenIntervalLessThanExecution(t *testing.T) {
	var inFlight atomic.Int32
	var maxSeen atomic.Int32
	c := &mockCollector{
		name:     "long",
		delay:    60 * time.Millisecond,
		samples:  []telemetry.Sample{{MetricKey: "m", Value: 1, Unit: "u"}},
		inFlight: &inFlight,
		maxSeen:  &maxSeen,
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: c, Enabled: true, Interval: 20 * time.Millisecond, Timeout: time.Second,
	})
	sched, err := NewScheduler(testIdent(), reg, &mockEnqueuer{}, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sched.Start(ctx)
	time.Sleep(250 * time.Millisecond)
	sched.Stop()

	if got := int(maxSeen.Load()); got > 1 {
		t.Fatalf("unbounded overlap under interval<execution: maxInFlight=%d", got)
	}
	if c.calls.Load() < 2 {
		t.Fatalf("expected repeated coalesced runs, calls=%d", c.calls.Load())
	}
	// Cadence must not enqueue one record per missed tick; aggregate flush remains bounded.
	// Primary invariant here is no overlapping self-execution.
}

func TestPendingMergePreservesMultipleContributions(t *testing.T) {
	c := &mockCollector{name: "cpu", samples: []telemetry.Sample{{MetricKey: "m", Value: 1, Unit: "u"}}}
	reg := mustRegistry(t, collector.Registration{
		Collector: c, Enabled: true, Interval: time.Hour, Timeout: time.Second,
	})
	enq := &mockEnqueuer{}
	sched, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	t1 := time.Unix(100, 0).UTC()
	t2 := time.Unix(101, 0).UTC()
	obs1, err := telemetry.Normalize(testIdent(), "cpu", t1, telemetry.Sample{MetricKey: "system.cpu.utilization", Value: 1, Unit: "%"})
	if err != nil {
		t.Fatal(err)
	}
	obs2, err := telemetry.Normalize(testIdent(), "cpu", t2, telemetry.Sample{MetricKey: "system.cpu.utilization", Value: 2, Unit: "%"})
	if err != nil {
		t.Fatal(err)
	}
	sched.deposit(pendingContribution{
		name: "cpu", collectedAt: t1, durationMs: 1,
		observations: []telemetry.Observation{obs1},
	})
	sched.deposit(pendingContribution{
		name: "cpu", collectedAt: t2, durationMs: 2,
		observations: []telemetry.Observation{obs2},
	})
	sched.flushPending()

	if enq.count() != 1 {
		t.Fatalf("expected one aggregate batch, got %d", enq.count())
	}
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[0], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Observations) != 2 {
		t.Fatalf("silent overwrite: expected 2 observations, got %d", len(batch.Observations))
	}
	if batch.Observations[0].Value != 1 || batch.Observations[1].Value != 2 {
		t.Fatalf("observations not preserved: %+v", batch.Observations)
	}
}

func TestSlowEnqueueFlushDoesNotOverwritePending(t *testing.T) {
	c := &mockCollector{
		name:    "cpu",
		samples: []telemetry.Sample{{MetricKey: "system.cpu.utilization", Value: 1, Unit: "%"}},
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: c, Enabled: true, Interval: 30 * time.Millisecond, Timeout: time.Second,
	})

	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	enq := &blockingEnqueuer{entered: entered, release: release}

	sched, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	// First contribution + blocking flush.
	t1 := time.Unix(200, 0).UTC()
	obs1, err := telemetry.Normalize(testIdent(), "cpu", t1, telemetry.Sample{MetricKey: "system.cpu.utilization", Value: 11, Unit: "%"})
	if err != nil {
		t.Fatal(err)
	}
	sched.deposit(pendingContribution{
		name: "cpu", collectedAt: t1, durationMs: 1,
		observations: []telemetry.Observation{obs1},
	})

	flushDone := make(chan struct{})
	go func() {
		sched.flushPending()
		close(flushDone)
	}()

	select {
	case <-entered:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("enqueue did not block")
	}

	// Second contribution arrives while flush/enqueue is still slow.
	t2 := time.Unix(201, 0).UTC()
	obs2, err := telemetry.Normalize(testIdent(), "cpu", t2, telemetry.Sample{MetricKey: "system.cpu.utilization", Value: 22, Unit: "%"})
	if err != nil {
		t.Fatal(err)
	}
	sched.deposit(pendingContribution{
		name: "cpu", collectedAt: t2, durationMs: 1,
		observations: []telemetry.Observation{obs2},
	})

	close(release)
	select {
	case <-flushDone:
	case <-time.After(time.Second):
		t.Fatal("flush did not complete")
	}

	// Flush the contribution that arrived during the slow enqueue window.
	sched.flushPending()

	if enq.count() != 2 {
		t.Fatalf("expected both cycle batches to survive slow enqueue, got %d", enq.count())
	}
	values := map[float64]bool{}
	enq.mu.Lock()
	for _, raw := range enq.records {
		var batch telemetry.CollectionBatch
		if err := json.Unmarshal(raw, &batch); err != nil {
			enq.mu.Unlock()
			t.Fatal(err)
		}
		for _, obs := range batch.Observations {
			values[obs.Value] = true
		}
	}
	enq.mu.Unlock()
	if !values[11] || !values[22] {
		t.Fatalf("slow enqueue window lost observations: %v", values)
	}
}

func TestPendingMergeSameCycleBeforeFlush(t *testing.T) {
	// Same-collector contributions before any flush must merge, not overwrite.
	c := &mockCollector{name: "cpu", samples: []telemetry.Sample{{MetricKey: "m", Value: 1, Unit: "u"}}}
	reg := mustRegistry(t, collector.Registration{
		Collector: c, Enabled: true, Interval: time.Hour, Timeout: time.Second,
	})
	enq := &mockEnqueuer{}
	sched, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Unix(300, 0).UTC()
	for i := 0; i < 3; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		obs, err := telemetry.Normalize(testIdent(), "cpu", ts, telemetry.Sample{
			MetricKey: "system.cpu.utilization", Value: float64(10 + i), Unit: "%",
		})
		if err != nil {
			t.Fatal(err)
		}
		sched.deposit(pendingContribution{
			name: "cpu", collectedAt: ts, durationMs: int64(i + 1),
			observations: []telemetry.Observation{obs},
		})
	}
	sched.flushPending()
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[0], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Observations) != 3 {
		t.Fatalf("expected merged 3 observations, got %d", len(batch.Observations))
	}
}

type blockingEnqueuer struct {
	mu       sync.Mutex
	records  [][]byte
	entered  chan struct{}
	release  chan struct{}
	calls    atomic.Int32
}

func (b *blockingEnqueuer) Enqueue(payload []byte) (*queue.Record, error) {
	b.calls.Add(1)
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-b.release
	b.mu.Lock()
	defer b.mu.Unlock()
	b.records = append(b.records, payload)
	return &queue.Record{Sequence: uint64(len(b.records)), Payload: payload}, nil
}

func (b *blockingEnqueuer) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.records)
}

func pendingItemCountForTest(s *Scheduler, name string) int {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	return pendingItemCount(s.pending[name])
}

func mustObservation(t *testing.T, ident *identity.AgentIdentity, at time.Time, value float64) telemetry.Observation {
	t.Helper()
	obs, err := telemetry.Normalize(ident, "cpu", at, telemetry.Sample{
		MetricKey: "system.cpu.utilization", Value: value, Unit: "%",
	})
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	return obs
}

func TestPendingOverflowExplicitAndBounded(t *testing.T) {
	ident := testIdent()
	reg := mustRegistry(t, collector.Registration{
		Collector: &mockCollector{name: "cpu", samples: []telemetry.Sample{{MetricKey: "m", Value: 1, Unit: "u"}}},
		Enabled:   true, Interval: time.Hour, Timeout: time.Second,
	})
	enq := &mockEnqueuer{}
	sched, err := NewScheduler(ident, reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	base := time.Unix(400, 0).UTC()
	for i := 0; i < maxPendingItemsPerCollector; i++ {
		sched.deposit(pendingContribution{
			name: "cpu", collectedAt: base.Add(time.Duration(i) * time.Millisecond), durationMs: 1,
			observations: []telemetry.Observation{mustObservation(t, ident, base, float64(i))},
		})
	}
	if got := pendingItemCountForTest(sched, "cpu"); got != maxPendingItemsPerCollector {
		t.Fatalf("pre-overflow pending count=%d want %d", got, maxPendingItemsPerCollector)
	}
	if st := sched.Status(); st.PendingOverflows != 0 {
		t.Fatalf("unexpected overflow before crossing bound: %+v", st)
	}

	// Crossing the bound must fail loud.
	sched.deposit(pendingContribution{
		name: "cpu", collectedAt: base.Add(time.Second), durationMs: 1,
		observations: []telemetry.Observation{mustObservation(t, ident, base.Add(time.Second), 999)},
	})

	if got := pendingItemCountForTest(sched, "cpu"); got > maxPendingItemsPerCollector {
		t.Fatalf("pending unbounded after overflow: count=%d bound=%d", got, maxPendingItemsPerCollector)
	}
	st := sched.Status()
	if st.PendingOverflows < 1 {
		t.Fatalf("PendingOverflows=%d want >=1", st.PendingOverflows)
	}
	_, runErrors := sched.Stats()
	if runErrors < 1 || st.RunErrors < 1 {
		t.Fatalf("RunErrors not reflected: stats=%d status=%d", runErrors, st.RunErrors)
	}

	// Repeated excess deposits must remain bounded and keep the marker.
	overflowsBefore := st.PendingOverflows
	for i := 0; i < 64; i++ {
		sched.deposit(pendingContribution{
			name: "cpu", collectedAt: base.Add(2*time.Second + time.Duration(i)*time.Millisecond), durationMs: 1,
			observations: []telemetry.Observation{mustObservation(t, ident, base, float64(1000+i))},
		})
		if got := pendingItemCountForTest(sched, "cpu"); got > maxPendingItemsPerCollector {
			t.Fatalf("unbounded growth under repeated overflow: count=%d", got)
		}
	}
	st = sched.Status()
	if st.PendingOverflows <= overflowsBefore {
		t.Fatalf("repeated excess did not increment PendingOverflows: before=%d after=%d", overflowsBefore, st.PendingOverflows)
	}

	sched.flushPending()
	if enq.count() != 1 {
		t.Fatalf("expected one durable batch, got %d", enq.count())
	}
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[0], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	sawOverflow := false
	for _, e := range batch.Errors {
		if e.Code == "PENDING_OVERFLOW" && e.Collector == "cpu" {
			sawOverflow = true
			break
		}
	}
	if !sawOverflow {
		t.Fatalf("durable batch missing PENDING_OVERFLOW: %+v", batch.Errors)
	}
	if len(batch.Observations)+len(batch.Errors) > maxPendingItemsPerCollector {
		t.Fatalf("flushed batch exceeded bound: obs=%d err=%d", len(batch.Observations), len(batch.Errors))
	}
}

func TestPendingOverflowErrorHeavyBoundRemainsReal(t *testing.T) {
	// Error-heavy pending has few/no observations to trim; the hard bound must still hold.
	reg := mustRegistry(t, collector.Registration{
		Collector: &mockCollector{name: "cpu", samples: []telemetry.Sample{{MetricKey: "m", Value: 1, Unit: "u"}}},
		Enabled:   true, Interval: time.Hour, Timeout: time.Second,
	})
	enq := &mockEnqueuer{}
	sched, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	base := time.Unix(500, 0).UTC()
	for i := 0; i < maxPendingItemsPerCollector; i++ {
		sched.deposit(pendingContribution{
			name: "cpu", collectedAt: base.Add(time.Duration(i) * time.Millisecond), durationMs: 1,
			errors: []telemetry.CollectorError{{
				Collector: "cpu",
				Code:      "COLLECTION_FAILED",
				Summary:   "err-" + strconv.Itoa(i),
			}},
		})
	}
	if got := pendingItemCountForTest(sched, "cpu"); got != maxPendingItemsPerCollector {
		t.Fatalf("error-heavy fill count=%d want %d", got, maxPendingItemsPerCollector)
	}

	sched.deposit(pendingContribution{
		name: "cpu", collectedAt: base.Add(time.Second), durationMs: 1,
		errors: []telemetry.CollectorError{{
			Collector: "cpu", Code: "COLLECTION_FAILED", Summary: "overflow-trigger",
		}},
	})

	if got := pendingItemCountForTest(sched, "cpu"); got > maxPendingItemsPerCollector {
		t.Fatalf("error-heavy overflow broke bound: count=%d bound=%d", got, maxPendingItemsPerCollector)
	}
	st := sched.Status()
	if st.PendingOverflows < 1 || st.RunErrors < 1 {
		t.Fatalf("error-heavy overflow not accounted: PendingOverflows=%d RunErrors=%d", st.PendingOverflows, st.RunErrors)
	}

	for i := 0; i < 100; i++ {
		sched.deposit(pendingContribution{
			name: "cpu", collectedAt: base.Add(2*time.Second + time.Duration(i)*time.Millisecond), durationMs: 1,
			errors: []telemetry.CollectorError{{
				Collector: "cpu", Code: "COLLECTION_FAILED", Summary: "more-" + strconv.Itoa(i),
			}},
		})
		if got := pendingItemCountForTest(sched, "cpu"); got > maxPendingItemsPerCollector {
			t.Fatalf("error-heavy repeated overflow unbounded: count=%d", got)
		}
	}

	sched.flushPending()
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[0], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Observations) != 0 {
		t.Fatalf("expected error-heavy batch without observations, got %d", len(batch.Observations))
	}
	sawOverflow := false
	for _, e := range batch.Errors {
		if e.Code == "PENDING_OVERFLOW" {
			sawOverflow = true
			break
		}
	}
	if !sawOverflow {
		t.Fatalf("error-heavy durable batch missing PENDING_OVERFLOW: %+v", batch.Errors)
	}
	if len(batch.Errors) > maxPendingItemsPerCollector {
		t.Fatalf("error-heavy flushed errors unbounded: %d", len(batch.Errors))
	}
}

func TestSchedulerStopContextIdempotent(t *testing.T) {
	enq := &mockEnqueuer{}
	col := &mockCollector{name: "cpu", delay: time.Millisecond}
	reg := mustRegistry(t, collector.Registration{Collector: col, Enabled: true, Interval: time.Hour, Timeout: time.Second})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Start(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.StopContext(ctx); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if err := s.StopContext(ctx); err != nil {
		t.Fatalf("repeated StopContext: %v", err)
	}
}

func TestSchedulerShutdownCancelDoesNotRecordFailure(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &mockCollector{name: "cpu", started: started, release: release}
	reg := mustRegistry(t, collector.Registration{Collector: col, Enabled: true, Interval: 10 * time.Millisecond, Timeout: 2 * time.Second})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.Start(ctx)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}
	_, errsBefore := s.Stats()
	cancel()
	if err := s.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	close(release)
	_, errsAfter := s.Stats()
	if errsAfter != errsBefore {
		t.Fatalf("shutdown cancel recorded failure: before=%d after=%d", errsBefore, errsAfter)
	}
}

func TestSchedulerStopContextWaitsForOutstandingNonCooperative(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &mockCollector{
		name:         "cpu",
		ignoreCancel: true,
		started:      started,
		release:      release,
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: 10 * time.Millisecond, Timeout: 40 * time.Millisecond,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Start(context.Background())
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}

	// Allow per-collector timeout to create an outstanding waiter.
	deadline := time.Now().Add(time.Second)
	for {
		st := s.Status()
		if len(st.Collectors) > 0 && st.Collectors[0].Outstanding {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected outstanding after non-cooperative timeout")
		}
		time.Sleep(5 * time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if err := s.StopContext(ctx); err == nil {
		t.Fatal("StopContext must not report clean completion while outstanding")
	}

	// Repeated StopContext must stay bounded (no waiter multiplication).
	start := time.Now()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel2()
	_ = s.StopContext(ctx2)
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("repeated StopContext too slow: %v", time.Since(start))
	}

	close(release)
	cleanup, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cleanupCancel()
	if err := s.StopContext(cleanup); err != nil {
		t.Fatalf("outstanding waiter did not clean up after release: %v", err)
	}
	st := s.Status()
	if len(st.Collectors) > 0 && st.Collectors[0].Outstanding {
		t.Fatal("outstanding must clear after release")
	}
}

func TestSchedulerDeadlineFirstRemainsFailureDespiteLaterShutdown(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &mockCollector{
		name:         "cpu",
		ignoreCancel: true,
		started:      started,
		release:      release,
		samples:      []telemetry.Sample{{MetricKey: "system.cpu.utilization", Value: 1, Unit: "%"}},
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: 10 * time.Millisecond, Timeout: 40 * time.Millisecond,
	})
	parent, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	barrierHit := make(chan struct{})
	proceed := make(chan struct{})
	var barrierOnce sync.Once
	s.SetClassifyBarrierForTest(func() {
		barrierOnce.Do(func() { close(barrierHit) })
		<-proceed
	})

	s.Start(parent)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}
	select {
	case <-barrierHit:
	case <-time.After(2 * time.Second):
		t.Fatal("classify barrier not reached after deadline")
	}

	// Shutdown begins AFTER the per-collector deadline already won.
	parentCancel()
	close(proceed)
	close(release)

	deadline := time.Now().Add(2 * time.Second)
	for {
		_, errs := s.Stats()
		if errs > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("deadline-first timeout must remain COLLECTION_FAILED")
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = s.StopContext(context.Background())
}

func TestSchedulerShutdownFirstSuppressesFailure(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &mockCollector{
		name:    "cpu",
		started: started,
		release: release,
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: 10 * time.Millisecond, Timeout: 2 * time.Second,
	})
	parent, parentCancel := context.WithCancel(context.Background())
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	barrierHit := make(chan struct{})
	proceed := make(chan struct{})
	var once sync.Once
	s.SetClassifyBarrierForTest(func() {
		once.Do(func() { close(barrierHit) })
		<-proceed
	})

	s.Start(parent)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}

	// Shutdown wins first; then allow classification.
	parentCancel()
	select {
	case <-barrierHit:
	case <-time.After(2 * time.Second):
		// Cooperative collector returns Canceled; barrier may run on non-timeout path too.
	}
	close(proceed)
	close(release)
	if err := s.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	_, errs := s.Stats()
	if errs != 0 {
		t.Fatalf("shutdown-first cancellation must not record failure, errs=%d", errs)
	}
}

func TestSchedulerStopRaceWithTimeoutOutstandingTransition(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &mockCollector{
		name:         "cpu",
		ignoreCancel: true,
		started:      started,
		release:      release,
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: 5 * time.Millisecond, Timeout: 20 * time.Millisecond,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Start(context.Background())
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
			defer cancel()
			_ = s.StopContext(ctx)
		}()
	}
	wg.Wait()
	close(release)
	cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.StopContext(cleanup); err != nil {
		t.Fatalf("cleanup after race: %v", err)
	}
}

func TestRunOnceStopContextTimesOutWhileNonCooperativeOutstanding(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &mockCollector{
		name:         "cpu",
		ignoreCancel: true,
		started:      started,
		release:      release,
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: time.Hour, Timeout: 30 * time.Millisecond,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		s.RunOnce(context.Background())
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("RunOnce collector did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if err := s.StopContext(ctx); err == nil {
		t.Fatal("StopContext must time out while RunOnce non-cooperative execution is outstanding")
	}

	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunOnce did not finish after release")
	}
	cleanup, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cleanupCancel()
	if err := s.StopContext(cleanup); err != nil {
		t.Fatalf("StopContext should complete after release: %v", err)
	}
}

func TestRunOnceAfterStopStartsNoCollectorCalls(t *testing.T) {
	enq := &mockEnqueuer{}
	col := &mockCollector{name: "cpu", delay: time.Millisecond}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: time.Hour, Timeout: time.Second,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	s.RunOnce(context.Background())
	if got := col.calls.Load(); got != 0 {
		t.Fatalf("RunOnce after Stop must not invoke collectors, calls=%d", got)
	}
}

func TestConcurrentRunOnceSingleFlightWhileOutstanding(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &mockCollector{
		name:         "cpu",
		ignoreCancel: true,
		started:      started,
		release:      release,
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: time.Hour, Timeout: 40 * time.Millisecond,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.RunOnce(context.Background()) }()
	go func() { defer wg.Done(); s.RunOnce(context.Background()) }()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		st := s.Status()
		if len(st.Collectors) > 0 && st.Collectors[0].Outstanding {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected outstanding from first RunOnce")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := col.calls.Load(); got != 1 {
		t.Fatalf("collector call count must stay 1 while outstanding, got %d", got)
	}

	close(release)
	wg.Wait()
	if got := col.calls.Load(); got != 1 {
		t.Fatalf("collector execution multiplied after release, calls=%d", got)
	}
}

func TestRunOnceSemaphoreWaiterSkippedAfterOutstandingRecheck(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &mockCollector{
		name:         "cpu",
		ignoreCancel: true,
		started:      started,
		release:      release,
	}
	// maxConcurrency=1 forces the second admit to wait on the semaphore.
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: time.Hour, Timeout: 40 * time.Millisecond,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.RunOnce(context.Background())
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("first collector did not start")
	}

	// Second RunOnce blocks on semaphore until first times out and releases.
	secondDone := make(chan struct{})
	go func() {
		s.RunOnce(context.Background())
		close(secondDone)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		st := s.Status()
		if len(st.Collectors) > 0 && st.Collectors[0].Outstanding {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected first execution to become outstanding")
		}
		time.Sleep(5 * time.Millisecond)
	}

	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("second RunOnce did not finish after outstanding recheck skip")
	}
	if got := col.calls.Load(); got != 1 {
		t.Fatalf("second execution must be skipped after outstanding recheck, calls=%d", got)
	}

	close(release)
	wg.Wait()
}

func TestRunOnceStopContextStressRace(t *testing.T) {
	enq := &mockEnqueuer{}
	release := make(chan struct{})
	close(release) // cooperative enough that some completes; ignoreCancel false by default
	col := &mockCollector{
		name:    "cpu",
		delay:   5 * time.Millisecond,
		release: nil,
	}
	// Mix non-cooperative bursts.
	col2 := &mockCollector{
		name:         "mem",
		ignoreCancel: true,
		delay:        30 * time.Millisecond,
		release:      make(chan struct{}),
	}
	reg := mustRegistry(t,
		collector.Registration{Collector: col, Enabled: true, Interval: time.Hour, Timeout: 15 * time.Millisecond},
		collector.Registration{Collector: col2, Enabled: true, Interval: time.Hour, Timeout: 15 * time.Millisecond},
	)
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.RunOnce(context.Background())
		}()
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			defer cancel()
			_ = s.StopContext(ctx)
		}()
	}
	wg.Wait()
	close(col2.release)
	cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.StopContext(cleanup); err != nil {
		t.Fatalf("final StopContext: %v", err)
	}
	// Single-flight: while each collector can run multiple sequential times across
	// iterations, overlapping multiplication is prevented; mem ignoreCancel with
	// shared release means at most one outstanding at a time, so calls should be
	// modest. Hard invariant checked: never more than one outstanding in status
	// snapshot after stop.
	st := s.Status()
	for _, c := range st.Collectors {
		if c.Outstanding {
			t.Fatalf("collector %s still outstanding after final stop", c.Name)
		}
		if c.InFlight {
			t.Fatalf("collector %s still in-flight after final stop", c.Name)
		}
	}
}

func TestCollectorReturnedDeadlineExceededIsGenuineFailure(t *testing.T) {
	enq := &mockEnqueuer{}
	col := &mockCollector{
		name: "cpu",
		err:  context.DeadlineExceeded,
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: time.Hour, Timeout: time.Second,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.RunOnce(context.Background())

	_, errs := s.Stats()
	if errs < 1 {
		t.Fatalf("expected scheduler error counter increment, got %d", errs)
	}
	st := s.Status()
	if len(st.Collectors) == 0 || st.Collectors[0].ErrorCount < 1 {
		t.Fatalf("expected collector error counter increment, status=%+v", st.Collectors)
	}
	if enq.count() != 1 {
		t.Fatalf("expected deposited failure batch, got %d records", enq.count())
	}
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[0], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Errors) != 1 || batch.Errors[0].Code != "COLLECTION_FAILED" {
		t.Fatalf("expected COLLECTION_FAILED deposit, got %+v", batch.Errors)
	}
}

func TestCollectorReturnedCanceledIsGenuineFailure(t *testing.T) {
	enq := &mockEnqueuer{}
	col := &mockCollector{
		name: "cpu",
		err:  context.Canceled,
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: time.Hour, Timeout: time.Second,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.RunOnce(context.Background())

	_, errs := s.Stats()
	if errs < 1 {
		t.Fatalf("collector-returned Canceled must remain a genuine failure, errs=%d", errs)
	}
	if enq.count() != 1 {
		t.Fatalf("expected COLLECTION_FAILED deposit, got %d records", enq.count())
	}
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[0], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Errors) != 1 || batch.Errors[0].Code != "COLLECTION_FAILED" {
		t.Fatalf("expected COLLECTION_FAILED, got %+v", batch.Errors)
	}
}

func TestSuccessfulCollectorResultNoFailureFromCleanupCancel(t *testing.T) {
	enq := &mockEnqueuer{}
	col := &mockCollector{
		name:    "cpu",
		samples: []telemetry.Sample{{MetricKey: "system.cpu.utilization", Value: 1, Unit: "%"}},
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: time.Hour, Timeout: time.Second,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.RunOnce(context.Background())

	_, errs := s.Stats()
	if errs != 0 {
		t.Fatalf("successful collect must not record failure from cleanup cancel, errs=%d", errs)
	}
	st := s.Status()
	if len(st.Collectors) > 0 && st.Collectors[0].ErrorCount != 0 {
		t.Fatalf("collector ErrorCount must stay 0, got %d", st.Collectors[0].ErrorCount)
	}
	if enq.count() != 1 {
		t.Fatalf("expected one success batch, got %d", enq.count())
	}
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[0], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Errors) != 0 {
		t.Fatalf("cleanup cancel must not deposit errors, got %+v", batch.Errors)
	}
	if len(batch.Observations) != 1 {
		t.Fatalf("expected one observation, got %d", len(batch.Observations))
	}
}

func TestStopFinalFlushEnqueuesAcceptedObservation(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &mockCollector{
		name:         "cpu",
		ignoreCancel: true,
		started:      started,
		release:      release,
		samples:      []telemetry.Sample{{MetricKey: "system.cpu.utilization", Value: 7, Unit: "%"}},
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: 10 * time.Millisecond, Timeout: 2 * time.Second,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Start(context.Background())
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}

	stopErr := make(chan error, 1)
	go func() { stopErr <- s.StopContext(context.Background()) }()

	// Allow flush loop to observe stop and exit before collector release.
	time.Sleep(50 * time.Millisecond)
	before := enq.count()
	close(release)

	select {
	case err := <-stopErr:
		if err != nil {
			t.Fatalf("clean Stop failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StopContext did not complete")
	}
	if enq.count() <= before {
		t.Fatal("authoritative final flush must enqueue accepted observation after flush-loop exit")
	}
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[len(enq.records)-1], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, obs := range batch.Observations {
		if obs.MetricKey == "system.cpu.utilization" && obs.Value == 7 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("final durable batch missing accepted observation: %+v", batch.Observations)
	}
	if s.PendingItemCountForTest() != 0 {
		t.Fatalf("pending must be empty after clean Stop, got %d", s.PendingItemCountForTest())
	}
}

func TestStopFinalFlushEnqueuesAcceptedCollectorError(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &mockCollector{
		name:         "cpu",
		ignoreCancel: true,
		started:      started,
		release:      release,
		err:          errors.New("post-stop collector failure"),
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: 10 * time.Millisecond, Timeout: 2 * time.Second,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Start(context.Background())
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}

	stopErr := make(chan error, 1)
	go func() { stopErr <- s.StopContext(context.Background()) }()
	time.Sleep(50 * time.Millisecond)
	close(release)

	select {
	case err := <-stopErr:
		if err != nil {
			t.Fatalf("clean Stop failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StopContext did not complete")
	}

	found := false
	enq.mu.Lock()
	for _, raw := range enq.records {
		var batch telemetry.CollectionBatch
		if err := json.Unmarshal(raw, &batch); err != nil {
			enq.mu.Unlock()
			t.Fatal(err)
		}
		for _, e := range batch.Errors {
			if e.Code == "COLLECTION_FAILED" {
				found = true
			}
		}
	}
	enq.mu.Unlock()
	if !found {
		t.Fatal("final durable batch must contain COLLECTION_FAILED before clean Stop")
	}
	if s.PendingItemCountForTest() != 0 {
		t.Fatalf("pending must be empty after clean Stop, got %d", s.PendingItemCountForTest())
	}
}

func TestRunOnceBlockingEnqueueKeepsStopContextTimedOut(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	enq := &blockingEnqueuer{entered: entered, release: release}
	col := &mockCollector{
		name:    "cpu",
		samples: []telemetry.Sample{{MetricKey: "system.cpu.utilization", Value: 1, Unit: "%"}},
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: time.Hour, Timeout: time.Second,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	runDone := make(chan struct{})
	go func() {
		s.RunOnce(context.Background())
		close(runDone)
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("RunOnce final enqueue did not start")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	if err := s.StopContext(ctx); err == nil {
		t.Fatal("StopContext must not return clean while accepted RunOnce enqueue is blocked")
	}

	close(release)
	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("RunOnce did not finish after enqueue release")
	}
	cleanup, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cleanupCancel()
	if err := s.StopContext(cleanup); err != nil {
		t.Fatalf("StopContext should complete after enqueue release: %v", err)
	}
}

func TestRunOnceAfterStopNoCollectorNoEnqueue(t *testing.T) {
	enq := &mockEnqueuer{}
	col := &mockCollector{name: "cpu", delay: time.Millisecond, samples: []telemetry.Sample{{MetricKey: "system.cpu.utilization", Value: 1, Unit: "%"}}}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: time.Hour, Timeout: time.Second,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	beforeCalls := enq.calls.Load()
	s.RunOnce(context.Background())
	if got := col.calls.Load(); got != 0 {
		t.Fatalf("RunOnce after Stop must not invoke collectors, calls=%d", got)
	}
	if enq.calls.Load() != beforeCalls || enq.count() != 0 {
		t.Fatalf("RunOnce after Stop must not enqueue, calls=%d count=%d", enq.calls.Load(), enq.count())
	}
}

func TestCollectImmutableCanceledWhileLiveIsFailure(t *testing.T) {
	// Result-first context.Canceled with immutable live completion state must stay failure
	// even if parent cancellation arrives before classification.
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &mockCollector{
		name:    "cpu",
		started: started,
		release: release,
		err:     context.Canceled,
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: time.Hour, Timeout: 2 * time.Second,
	})
	parent, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	barrierHit := make(chan struct{})
	proceed := make(chan struct{})
	var once sync.Once
	s.SetClassifyBarrierForTest(func() {
		once.Do(func() { close(barrierHit) })
		<-proceed
	})

	done := make(chan struct{})
	go func() {
		s.RunOnce(parent)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}
	close(release) // Collect returns Canceled while runCtx is still live
	select {
	case <-barrierHit:
	case <-time.After(2 * time.Second):
		t.Fatal("classify barrier not reached")
	}
	// Parent cancel AFTER immutable evidence was published — must not rewrite outcome.
	parentCancel()
	close(proceed)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunOnce did not finish")
	}

	_, errs := s.Stats()
	if errs < 1 {
		t.Fatalf("immutable result-first Canceled must remain failure, errs=%d", errs)
	}
	if enq.count() != 1 {
		t.Fatalf("expected COLLECTION_FAILED deposit, got %d", enq.count())
	}
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[0], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Errors) != 1 || batch.Errors[0].Code != "COLLECTION_FAILED" {
		t.Fatalf("expected COLLECTION_FAILED, got %+v", batch.Errors)
	}
}

func TestCollectImmutableDeadlineExceededWhileLiveIsFailure(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	col := &mockCollector{
		name:    "cpu",
		started: started,
		release: release,
		err:     context.DeadlineExceeded,
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: time.Hour, Timeout: 2 * time.Second,
	})
	parent, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}

	barrierHit := make(chan struct{})
	proceed := make(chan struct{})
	var once sync.Once
	s.SetClassifyBarrierForTest(func() {
		once.Do(func() { close(barrierHit) })
		<-proceed
	})

	done := make(chan struct{})
	go func() {
		s.RunOnce(parent)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}
	close(release)
	select {
	case <-barrierHit:
	case <-time.After(2 * time.Second):
		t.Fatal("classify barrier not reached")
	}
	parentCancel()
	close(proceed)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunOnce did not finish")
	}

	_, errs := s.Stats()
	if errs < 1 {
		t.Fatalf("immutable result-first DeadlineExceeded must remain failure, errs=%d", errs)
	}
	var batch telemetry.CollectionBatch
	enq.mu.Lock()
	err = json.Unmarshal(enq.records[0], &batch)
	enq.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Errors) != 1 || batch.Errors[0].Code != "COLLECTION_FAILED" {
		t.Fatalf("expected COLLECTION_FAILED, got %+v", batch.Errors)
	}
}

func TestCollectReturnWithParentCancelAlreadyPresentIsSuppressed(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	col := &mockCollector{
		name:    "cpu",
		started: started,
		release: make(chan struct{}), // wait on ctx.Done via cooperative path without release signal
	}
	// Cooperative: wait on release OR ctx.Done. We never close release; parent cancel wins.
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: 10 * time.Millisecond, Timeout: 2 * time.Second,
	})
	parent, parentCancel := context.WithCancel(context.Background())
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Start(parent)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}
	parentCancel()
	if err := s.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	_, errs := s.Stats()
	if errs != 0 {
		t.Fatalf("parent-cancel-at-return must be suppressed, errs=%d", errs)
	}
}

func TestCollectReturnWithDeadlineAlreadyPresentIsFailure(t *testing.T) {
	enq := &mockEnqueuer{}
	started := make(chan struct{}, 1)
	col := &mockCollector{
		name:    "cpu",
		started: started,
		// Cooperative wait: returns ctx.Err() when deadline fires.
		release: make(chan struct{}),
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: 10 * time.Millisecond, Timeout: 30 * time.Millisecond,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.Start(context.Background())
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not start")
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, errs := s.Stats()
		if errs > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("deadline-at-return must remain COLLECTION_FAILED")
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = s.StopContext(context.Background())
}

func TestCleanStopLeavesPendingEmptyAndNoLaterEnqueue(t *testing.T) {
	enq := &mockEnqueuer{}
	col := &mockCollector{
		name:    "cpu",
		samples: []telemetry.Sample{{MetricKey: "system.cpu.utilization", Value: 3, Unit: "%"}},
	}
	reg := mustRegistry(t, collector.Registration{
		Collector: col, Enabled: true, Interval: time.Hour, Timeout: time.Second,
	})
	s, err := NewScheduler(testIdent(), reg, enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.RunOnce(context.Background())
	if err := s.StopContext(context.Background()); err != nil {
		t.Fatalf("StopContext: %v", err)
	}
	if s.PendingItemCountForTest() != 0 {
		t.Fatalf("pending must be empty after clean Stop, got %d", s.PendingItemCountForTest())
	}
	after := enq.calls.Load()
	s.RunOnce(context.Background())
	if enq.calls.Load() != after {
		t.Fatalf("no enqueue may occur after clean Stop, before=%d after=%d", after, enq.calls.Load())
	}
	if got := col.calls.Load(); got != 1 {
		t.Fatalf("post-stop RunOnce must not invoke collector again, calls=%d", got)
	}
}
