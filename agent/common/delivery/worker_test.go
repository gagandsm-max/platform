package delivery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/queue"
)

type mockQueue struct {
	mu        sync.Mutex
	records   []queue.Record
	deleted   []uint64
	readErr   error
	readCalls int
}

func (m *mockQueue) ReadBatch(maxRecords int, maxBytes int64) ([]queue.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readCalls++
	if m.readErr != nil {
		return nil, m.readErr
	}
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
	echoID    bool
}

func (m *mockTransport) Available() bool { return m.available }
func (m *mockTransport) Deliver(ctx context.Context, b client.Batch) (client.Ack, error) {
	m.calls++
	m.batches = append(m.batches, b)
	if m.err != nil {
		return client.Ack{}, m.err
	}
	ack := m.ack
	if m.echoID {
		ack.AgentID = b.AgentID
		ack.BatchID = b.BatchID
	}
	return ack, nil
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
		echoID:    true,
		ack:       client.Ack{AcceptedThrough: 2},
	}

	w, err := NewWorker(q, tr, "agent-1", 10, 1024*1024, time.Millisecond, 10*time.Millisecond, time.Second, nil)
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
	if tr.batches[0].AgentID != "agent-1" || tr.batches[0].BatchID == "" {
		t.Fatalf("worker must submit AgentID+BatchID before Deliver: %+v", tr.batches[0])
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

	w, err := NewWorker(q, tr, "agent-1", 10, 1024*1024, time.Millisecond, 10*time.Millisecond, time.Second, nil)
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

	w, err := NewWorker(q, tr, "agent-1", 10, 1024*1024, time.Millisecond, 10*time.Millisecond, time.Second, nil)
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
	if stats.ReceiverAvailable {
		t.Fatal("temporary Sensor failure must not report receiver available")
	}
}

func TestDeliveryWorkerAuthAndPermanentRetain(t *testing.T) {
	cases := []client.FailureClass{client.FailureAuthentication, client.FailurePermanent}
	for _, fc := range cases {
		q := &mockQueue{records: []queue.Record{{Sequence: 1, ID: "rec-1", Payload: []byte("a")}}}
		tr := &mockTransport{
			available: true,
			err:       &client.DeliveryError{Code: string(fc), Class: fc},
		}
		w, err := NewWorker(q, tr, "agent-1", 10, 1024, time.Millisecond, time.Millisecond, time.Second, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.DeliverOnce(context.Background()); err == nil {
			t.Fatalf("%s: expected error", fc)
		}
		if q.Len() != 1 {
			t.Fatalf("%s: must retain queue", fc)
		}
		if w.Stats().Successes != 0 {
			t.Fatalf("%s: must not count success", fc)
		}
	}
}

func TestDeliveryWorkerBatchBoundAckMatrix(t *testing.T) {
	records := []queue.Record{
		{Sequence: 5, ID: "rec-5", Checksum: "c5", Payload: []byte("a")},
		{Sequence: 6, ID: "rec-6", Checksum: "c6", Payload: []byte("b")},
	}
	expectedBatchID := client.BuildBatchID("agent-1", records)
	gapRecords := []queue.Record{
		{Sequence: 5, ID: "rec-5", Checksum: "c5", Payload: []byte("a")},
		{Sequence: 7, ID: "rec-7", Checksum: "c7", Payload: []byte("c")},
	}
	gapBatchID := client.BuildBatchID("agent-1", gapRecords)

	cases := []struct {
		name    string
		records []queue.Record
		ack     client.Ack
		want    bool
	}{
		{
			name:    "exact_identity_allows_delete",
			records: records,
			ack:     client.Ack{AcceptedThrough: 6, AgentID: "agent-1", BatchID: expectedBatchID},
			want:    true,
		},
		{
			name:    "wrong_nonempty_batch_id",
			records: records,
			ack:     client.Ack{AcceptedThrough: 6, AgentID: "agent-1", BatchID: "batch-wrong"},
			want:    false,
		},
		{
			name:    "stale_prior_batch_id",
			records: records,
			ack:     client.Ack{AcceptedThrough: 6, AgentID: "agent-1", BatchID: "batch-agent-1-00000000000000000001-00000000000000000001-deadbeefdeadbeef"},
			want:    false,
		},
		{
			name:    "missing_batch_id",
			records: records,
			ack:     client.Ack{AcceptedThrough: 6, AgentID: "agent-1", BatchID: ""},
			want:    false,
		},
		{
			name:    "wrong_nonempty_agent_id",
			records: records,
			ack:     client.Ack{AcceptedThrough: 6, AgentID: "agent-other", BatchID: expectedBatchID},
			want:    false,
		},
		{
			name:    "missing_agent_id",
			records: records,
			ack:     client.Ack{AcceptedThrough: 6, AgentID: "", BatchID: expectedBatchID},
			want:    false,
		},
		{
			name:    "watermark_before_batch",
			records: records,
			ack:     client.Ack{AcceptedThrough: 4, AgentID: "agent-1", BatchID: expectedBatchID},
			want:    false,
		},
		{
			name:    "watermark_after_batch",
			records: records,
			ack:     client.Ack{AcceptedThrough: 99, AgentID: "agent-1", BatchID: expectedBatchID},
			want:    false,
		},
		{
			name:    "watermark_not_in_submitted_records",
			records: gapRecords,
			ack:     client.Ack{AcceptedThrough: 6, AgentID: "agent-1", BatchID: gapBatchID},
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &mockQueue{records: append([]queue.Record(nil), tc.records...)}
			tr := &mockTransport{available: true, ack: tc.ack}
			w, err := NewWorker(q, tr, "agent-1", 10, 1024, time.Millisecond, time.Millisecond, time.Second, nil)
			if err != nil {
				t.Fatal(err)
			}
			delivered, err := w.DeliverOnce(context.Background())
			if tc.want {
				if err != nil || !delivered {
					t.Fatalf("expected success, delivered=%v err=%v", delivered, err)
				}
				if q.Len() != 0 || len(q.deleted) != 1 {
					t.Fatalf("expected deletion, len=%d deleted=%v", q.Len(), q.deleted)
				}
				return
			}
			if err == nil || delivered {
				t.Fatalf("expected rejection, delivered=%v err=%v", delivered, err)
			}
			if q.Len() != len(tc.records) || len(q.deleted) != 0 {
				t.Fatalf("must not delete: len=%d deleted=%v", q.Len(), q.deleted)
			}
		})
	}
}

func TestDeliveryWorkerCaseOnlyAckAgentIDMismatchRetainsRecords(t *testing.T) {
	records := []queue.Record{
		{Sequence: 5, ID: "rec-5", Checksum: "c5", Payload: []byte("a")},
		{Sequence: 6, ID: "rec-6", Checksum: "c6", Payload: []byte("b")},
	}
	expectedBatchID := client.BuildBatchID("agent-A", records)
	q := &mockQueue{records: append([]queue.Record(nil), records...)}
	tr := &mockTransport{
		available: true,
		ack: client.Ack{
			AcceptedThrough: 6,
			AgentID:         "AGENT-a",
			BatchID:         expectedBatchID,
		},
	}
	w, err := NewWorker(q, tr, "agent-A", 10, 1024, time.Millisecond, time.Millisecond, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}

	delivered, err := w.DeliverOnce(context.Background())
	if err == nil || delivered {
		t.Fatalf("case-only ACK agent mismatch must reject, delivered=%v err=%v", delivered, err)
	}
	if tr.calls != 1 {
		t.Fatalf("expected one generic transport call, got %d", tr.calls)
	}
	if q.Len() != len(records) {
		t.Fatalf("queue records must remain, len=%d", q.Len())
	}
	if len(q.deleted) != 0 {
		t.Fatalf("DeleteThrough must not be called, deleted=%v", q.deleted)
	}
}

func TestDeliveryWorkerPartialAck(t *testing.T) {
	q := &mockQueue{
		records: []queue.Record{
			{Sequence: 1, ID: "rec-1", Payload: []byte("a")},
			{Sequence: 2, ID: "rec-2", Payload: []byte("b")},
		},
	}
	tr := &mockTransport{available: true, echoID: true, ack: client.Ack{AcceptedThrough: 1}}
	w, err := NewWorker(q, tr, "agent-1", 10, 1024, time.Millisecond, time.Millisecond, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.DeliverOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if q.Len() != 1 || q.records[0].Sequence != 2 {
		t.Fatalf("partial ACK should leave seq 2: %+v", q.records)
	}
}

type failingDeleteQueue struct {
	mockQueue
}

func (f *failingDeleteQueue) DeleteThrough(seq uint64) error {
	return errors.New("QUEUE_DELETE_FAILED: injected")
}

func TestDeliveryWorkerDeleteFailureNotSuccess(t *testing.T) {
	q := &failingDeleteQueue{mockQueue: mockQueue{
		records: []queue.Record{{Sequence: 1, ID: "rec-1", Payload: []byte("a")}},
	}}
	tr := &mockTransport{available: true, echoID: true, ack: client.Ack{AcceptedThrough: 1}}
	w, err := NewWorker(q, tr, "agent-1", 10, 1024, time.Millisecond, time.Millisecond, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.DeliverOnce(context.Background()); err == nil {
		t.Fatal("expected delete failure")
	}
	if w.Stats().Successes != 0 {
		t.Fatal("delete failure must not count as success")
	}
}

func TestDeliveryWorkerRequiresAgentID(t *testing.T) {
	_, err := NewWorker(&mockQueue{}, &mockTransport{}, "", 10, 1024, time.Millisecond, time.Millisecond, time.Second, nil)
	if err == nil {
		t.Fatal("empty agent ID must fail NewWorker")
	}
}

func TestDeliveryWorkerStopIdempotent(t *testing.T) {
	q := &mockQueue{}
	tr := &mockTransport{available: false}
	w, err := NewWorker(q, tr, "agent-1", 10, 1024, time.Millisecond, time.Millisecond, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	w.Stop()
	w.Stop() // must not panic
}

func TestDeliveryWorkerOversizedDoesNotRetryLoopOrDelete(t *testing.T) {
	q := &mockQueue{records: []queue.Record{{Sequence: 1, ID: "rec-1", Payload: []byte("a")}}}
	tr := &mockTransport{
		available: true,
		err:       &client.DeliveryError{Code: "ERR_REQUEST_TOO_LARGE", Class: client.FailureOversized},
	}
	w, err := NewWorker(q, tr, "agent-1", 10, 1024, time.Millisecond, time.Millisecond, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	w.Stop()
	if tr.calls != 1 {
		t.Fatalf("oversized delivery must not hot-retry, calls=%d", tr.calls)
	}
	if q.Len() != 1 {
		t.Fatal("413/oversized must retain the local queue")
	}
	if w.Stats().Successes != 0 {
		t.Fatal("oversized must not count as success")
	}
	if w.Stats().LastFailureClass != client.FailureOversized {
		t.Fatalf("expected FailureOversized, got %s", w.Stats().LastFailureClass)
	}
	if w.Stats().LastErrorCode != "ERR_REQUEST_TOO_LARGE" {
		t.Fatalf("expected ERR_REQUEST_TOO_LARGE, got %s", w.Stats().LastErrorCode)
	}
}

func TestDeliveryWorkerBlockedSequenceDoesNotSubmitLaterRecords(t *testing.T) {
	dir := t.TempDir()
	q, err := queue.NewDurableQueue(dir, 10, 1024*1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("two")); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Enqueue([]byte("three")); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "active"))
	if err != nil {
		t.Fatal(err)
	}
	var seq2 string
	for _, e := range entries {
		if len(e.Name()) >= 23 && e.Name()[4:23] == "0000000000000000002" {
			seq2 = filepath.Join(dir, "active", e.Name())
			break
		}
	}
	if seq2 == "" {
		t.Fatal("missing sequence 2")
	}
	if err := os.WriteFile(seq2, []byte(`{"sequence":2,"id":"rec-x","checksum":"wrong","payload":"dGVzdA=="}`), 0600); err != nil {
		t.Fatal(err)
	}

	tr := &mockTransport{available: true, echoID: true, ack: client.Ack{AcceptedThrough: 1}}
	w, err := NewWorker(q, tr, "agent-1", 10, 1024*1024, time.Millisecond, time.Millisecond, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	delivered, err := w.DeliverOnce(context.Background())
	if err != nil || !delivered {
		t.Fatalf("prefix sequence 1 must deliver, delivered=%v err=%v", delivered, err)
	}
	if tr.calls != 1 || len(tr.batches[0].Records) != 1 || tr.batches[0].Records[0].Sequence != 1 {
		t.Fatalf("must submit only sequence 1, calls=%d batch=%+v", tr.calls, tr.batches)
	}

	delivered, err = w.DeliverOnce(context.Background())
	if delivered || !errors.Is(err, queue.ErrDeliveryBlocked) {
		t.Fatalf("later records must be blocked, delivered=%v err=%v", delivered, err)
	}
	if tr.calls != 1 {
		t.Fatalf("blocked stream must not submit sequence 3, calls=%d", tr.calls)
	}
	if q.Len() != 1 {
		t.Fatalf("sequence 3 must remain intact in active, len=%d", q.Len())
	}
	if w.Stats().LastFailureClass != client.FailureBlocked {
		t.Fatalf("expected FailureBlocked, got %s", w.Stats().LastFailureClass)
	}
	if w.Stats().LastErrorCode != "QUEUE_DELIVERY_BLOCKED" {
		t.Fatalf("expected QUEUE_DELIVERY_BLOCKED, got %s", w.Stats().LastErrorCode)
	}
}

func TestDeliveryWorkerPermanentAndAuthStillRetainWithoutSkip(t *testing.T) {
	retainErrs := []error{
		&client.DeliveryError{Code: "ERR_AUTH_IDENTITY_MISMATCH", Class: client.FailureAuthentication},
		&client.DeliveryError{Code: "ERR_REPLAY_CONFLICT", Class: client.FailurePermanent},
		&client.DeliveryError{Code: "DELIVERY_ACK_INVALID", Class: client.FailurePermanent},
		&client.DeliveryError{Code: "ERR_REQUEST_TOO_LARGE", Class: client.FailureOversized},
		&client.DeliveryError{Code: "ERR_SEQUENCE_GAP", Class: client.FailurePermanent},
	}
	for _, derr := range retainErrs {
		q := &mockQueue{records: []queue.Record{
			{Sequence: 1, ID: "rec-1", Payload: []byte("keep-me")},
			{Sequence: 2, ID: "rec-2", Payload: []byte("later")},
		}}
		tr := &mockTransport{available: true, err: derr}
		w, err := NewWorker(q, tr, "agent-1", 10, 1024, time.Millisecond, time.Millisecond, time.Second, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.DeliverOnce(context.Background()); err == nil {
			t.Fatalf("%v: expected delivery error", derr)
		}
		if q.Len() != 2 || len(q.deleted) != 0 {
			t.Fatalf("%v: must retain all records without skip, len=%d deleted=%v", derr, q.Len(), q.deleted)
		}
	}
}

func TestDeliveryWorkerDestExistsDoesNotRetry(t *testing.T) {
	if client.Classify(queue.ErrQuarantineDestExists) != client.FailureBlocked {
		t.Fatal("destination-exists must classify as BLOCKED, not TEMPORARY")
	}
	q := &mockQueue{
		records: []queue.Record{{Sequence: 1, ID: "rec-1", Payload: []byte("a")}},
		readErr: queue.ErrQuarantineDestExists,
	}
	tr := &mockTransport{available: true}
	w, err := NewWorker(q, tr, "agent-1", 10, 1024, time.Millisecond, time.Millisecond, time.Second, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w.Start(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if w.Stats().LastFailureClass == client.FailureBlocked {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if w.Stats().LastFailureClass != client.FailureBlocked {
		t.Fatalf("expected FailureBlocked, got %s", w.Stats().LastFailureClass)
	}
	time.Sleep(80 * time.Millisecond)
	q.mu.Lock()
	callsAfterBlock := q.readCalls
	q.mu.Unlock()
	w.Stop()
	if tr.calls != 0 {
		t.Fatalf("destination-exists must not submit telemetry, transport calls=%d", tr.calls)
	}
	if callsAfterBlock != 1 {
		t.Fatalf("destination-exists must not hot-retry ReadBatch, calls=%d", callsAfterBlock)
	}
	if w.Stats().LastErrorCode != "QUEUE_QUARANTINE_DEST_EXISTS" {
		t.Fatalf("expected QUEUE_QUARANTINE_DEST_EXISTS, got %s", w.Stats().LastErrorCode)
	}
	if w.Stats().Successes != 0 {
		t.Fatal("destination-exists must not count as success")
	}
}
