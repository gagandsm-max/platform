package delivery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/queue"
)

type mockQueue struct {
	mu      sync.Mutex
	records []queue.Record
	deleted []uint64
}

func (m *mockQueue) ReadBatch(maxRecords int, maxBytes int64) ([]queue.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.records) == 0 {
		return nil, nil
	}
	if maxRecords > len(m.records) {
		maxRecords = len(m.records)
	}
	out := make([]queue.Record, maxRecords)
	copy(out, m.records[:maxRecords])
	return out, nil
}

func (m *mockQueue) DeleteThrough(seq uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, seq)
	var remaining []queue.Record
	for _, r := range m.records {
		if r.Sequence > seq {
			remaining = append(remaining, r)
		}
	}
	m.records = remaining
	return nil
}

func (m *mockQueue) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.records)
}

type mockTransport struct {
	available bool
	ack       client.Ack
	err       error
	calls     int
	batches   []client.Batch
}

func (m *mockTransport) Available() bool { return m.available }
func (m *mockTransport) Deliver(ctx context.Context, b client.Batch) (client.Ack, error) {
	m.calls++
	m.batches = append(m.batches, b)
	if m.err != nil {
		return client.Ack{}, m.err
	}
	return m.ack, nil
}

func TestDeliveryWorkerSuccess(t *testing.T) {
	q := &mockQueue{
		records: []queue.Record{
			{Sequence: 1, ID: "rec-1", Payload: []byte("a")},
			{Sequence: 2, ID: "rec-2", Payload: []byte("b")},
		},
	}
	tr := &mockTransport{
		available: true,
		ack:       client.Ack{AcceptedThrough: 2},
	}

	w, err := NewWorker(q, tr, 10, 1024*1024, time.Millisecond, 10*time.Millisecond, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}

	delivered, err := w.DeliverOnce(context.Background())
	if err != nil {
		t.Fatalf("unexpected delivery error: %v", err)
	}
	if !delivered {
		t.Fatal("expected delivered to be true")
	}

	if tr.calls != 1 {
		t.Fatalf("expected 1 transport call, got %d", tr.calls)
	}
	if q.Len() != 0 {
		t.Fatalf("expected 0 remaining records after ACK, got %d", q.Len())
	}

	stats := w.Stats()
	if stats.Successes != 1 || stats.Attempts != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestDeliveryWorkerUnavailableRetainsRecords(t *testing.T) {
	q := &mockQueue{
		records: []queue.Record{
			{Sequence: 1, ID: "rec-1", Payload: []byte("a")},
		},
	}
	tr := client.NewUnavailableTransport("AGENT_RECEIVER_NOT_IMPLEMENTED")

	w, err := NewWorker(q, tr, 10, 1024*1024, time.Millisecond, 10*time.Millisecond, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}

	delivered, err := w.DeliverOnce(context.Background())
	if err == nil {
		t.Fatal("expected error with unavailable transport")
	}
	if delivered {
		t.Fatal("expected delivered to be false")
	}

	// Record MUST remain in queue!
	if q.Len() != 1 {
		t.Fatalf("record was prematurely deleted! Expected 1, got %d", q.Len())
	}

	stats := w.Stats()
	if stats.Successes != 0 || stats.Attempts != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if stats.LastFailureClass != client.FailureReceiverUnavailable {
		t.Fatalf("expected FailureReceiverUnavailable, got %s", stats.LastFailureClass)
	}
	if stats.ReceiverAvailable {
		t.Fatal("expected receiver to not be available")
	}
}

func TestDeliveryWorkerTemporaryErrorRetainsRecords(t *testing.T) {
	q := &mockQueue{
		records: []queue.Record{
			{Sequence: 1, ID: "rec-1", Payload: []byte("a")},
		},
	}
	tr := &mockTransport{
		available: true,
		err:       &client.DeliveryError{Code: "TEMP_IO", Class: client.FailureTemporary, Err: errors.New("io error")},
	}

	w, err := NewWorker(q, tr, 10, 1024*1024, time.Millisecond, 10*time.Millisecond, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}

	delivered, err := w.DeliverOnce(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if delivered {
		t.Fatal("expected false")
	}
	if q.Len() != 1 {
		t.Fatal("record must remain in queue")
	}

	stats := w.Stats()
	if stats.LastFailureClass != client.FailureTemporary {
		t.Fatalf("expected FailureTemporary, got %s", stats.LastFailureClass)
	}
}
