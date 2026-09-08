package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
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
}

func (m *mockEnqueuer) Enqueue(payload []byte) (*queue.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = append(m.records, payload)
	return &queue.Record{Sequence: uint64(len(m.records)), Payload: payload}, nil
}

type mockCollector struct {
	name    string
	samples []telemetry.Sample
	err     error
}

func (m *mockCollector) Name() string { return m.name }
func (m *mockCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.samples, nil
}

func TestSchedulerRunOnce(t *testing.T) {
	ident := &identity.AgentIdentity{
		AgentID:  "agent-sched-1",
		TenantID: "tenant-1",
		HostID:   "host-1",
		Hostname: "node-1",
		OS:       "linux",
		Arch:     "amd64",
	}

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

	enq := &mockEnqueuer{}
	sched, err := NewScheduler(ident, []collector.HostCollector{c1, c2}, enq, time.Second, time.Second, nil)
	if err != nil {
		t.Fatalf("failed to create scheduler: %v", err)
	}

	sched.RunOnce(context.Background())

	enq.mu.Lock()
	defer enq.mu.Unlock()

	if len(enq.records) != 1 {
		t.Fatalf("expected 1 enqueued record, got %d", len(enq.records))
	}

	var batch telemetry.CollectionBatch
	if err := json.Unmarshal(enq.records[0], &batch); err != nil {
		t.Fatalf("failed to unmarshal batch: %v", err)
	}

	if len(batch.Observations) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(batch.Observations))
	}
	if batch.Observations[0].MetricKey != "system.cpu.utilization" {
		t.Fatalf("unexpected metric: %s", batch.Observations[0].MetricKey)
	}

	if len(batch.Errors) != 1 {
		t.Fatalf("expected 1 collector error, got %d", len(batch.Errors))
	}
	if batch.Errors[0].Collector != "mem_failing" {
		t.Fatalf("unexpected error collector: %s", batch.Errors[0].Collector)
	}
}

func TestSchedulerLifecycle(t *testing.T) {
	ident := &identity.AgentIdentity{
		AgentID:  "agent-sched-2",
		TenantID: "tenant-1",
		HostID:   "host-1",
	}

	c := &mockCollector{name: "stub", samples: []telemetry.Sample{{MetricKey: "k", Value: 1, Unit: "u"}}}
	enq := &mockEnqueuer{}

	sched, err := NewScheduler(ident, []collector.HostCollector{c}, enq, 50*time.Millisecond, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	sched.Start(ctx)

	time.Sleep(120 * time.Millisecond)
	cancel()
	sched.Stop()

	enq.mu.Lock()
	defer enq.mu.Unlock()
	if len(enq.records) < 2 {
		t.Fatalf("expected at least 2 collections, got %d", len(enq.records))
	}
}
