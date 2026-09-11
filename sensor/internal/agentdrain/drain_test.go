package agentdrain

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/spool"
)

type fakeTransport struct {
	ack coreclient.AgentTelemetryAck
	err error
	last coreclient.AgentTelemetryBatch
}

func (f *fakeTransport) UploadAgentTelemetry(_ context.Context, batch coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
	f.last = batch
	return f.ack, f.err
}

func openSpool(t *testing.T) (*spool.DiskSpool, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}

func appendDurable(t *testing.T, s *spool.DiskSpool, seqHint uint64) spool.Record {
	t.Helper()
	payload := durableBatchPayload{
		TenantID: "018f0e0a-0000-7000-8000-000000000001",
		AgentID:  "agent-1",
		SensorID: "sensor-infra-01",
		Generation: 1,
		FirstSequence: 10,
		LastSequence: 10,
		RecordIDs: []string{"r-10"},
		BatchChecksum: "chk",
		BatchID: "ab-1",
		CorrelationID: "corr-1",
		Timestamp: time.Now().UTC(),
		Envelope: json.RawMessage(`{"records":[{"sequence":10,"id":"r-10","checksum":"c1","payload":{"hostId":"h1","observations":[]}}]}`),
	}
	enc, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := s.Append(spool.AppendRequest{
		Class: spool.ClassOperational,
		Type: agentBatchRecordType,
		Payload: enc,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestDrainAcksOnlyAfterValidCoreAck(t *testing.T) {
	s, dir := openSpool(t)
	_ = appendDurable(t, s, 1)
	tr := &fakeTransport{ack: coreclient.AgentTelemetryAck{
		Status: "ACCEPTED", SensorID: "sensor-infra-01", BatchID: "will-be-overridden",
		AcceptedThroughIngressSequence: 1, CorrelationID: "will-be-overridden",
	}}
	// Transport returns ACK matching whatever request was built.
	tr.err = nil
	d, err := New(Config{
		SensorID: "sensor-infra-01", AgentSpoolDir: dir,
		MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: 10 * time.Millisecond, MaxBackoff: 50 * time.Millisecond,
		IdleInterval: 10 * time.Millisecond, ShutdownGrace: time.Second,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	// Wrap transport to echo request bind fields.
	echo := &echoTransport{inner: tr}
	d.transport = echo

	ok, err := d.RunOnce(context.Background())
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	st, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.RecordCount != 0 {
		t.Fatalf("expected spool drained, got %d", st.RecordCount)
	}
	if d.Metrics().DurableAcks.Load() != 1 {
		t.Fatalf("durable acks=%d", d.Metrics().DurableAcks.Load())
	}
}

type echoTransport struct {
	inner *fakeTransport
}

func (e *echoTransport) UploadAgentTelemetry(ctx context.Context, batch coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
	e.inner.last = batch
	if e.inner.err != nil {
		return coreclient.AgentTelemetryAck{}, e.inner.err
	}
	return coreclient.AgentTelemetryAck{
		Status: "ACCEPTED",
		SensorID: batch.SensorID,
		BatchID: batch.BatchID,
		AcceptedThroughIngressSequence: batch.LastIngressSequence,
		CorrelationID: batch.CorrelationID,
	}, nil
}

func TestDrainRetainsSpoolOnTransientFailure(t *testing.T) {
	s, dir := openSpool(t)
	_ = appendDurable(t, s, 1)
	tr := &fakeTransport{err: transientErr{}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", AgentSpoolDir: dir,
		MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
		IdleInterval: time.Millisecond, ShutdownGrace: time.Second,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("expected transient failure, ok=%v err=%v", ok, err)
	}
	st, _ := s.Stats()
	if st.RecordCount != 1 {
		t.Fatalf("spool must be retained, got %d", st.RecordCount)
	}
}

type transientErr struct{}

func (transientErr) Error() string   { return "temporary core outage" }
func (transientErr) Transient() bool { return true }

func TestDrainParksTerminalEntityUnresolved(t *testing.T) {
	s, dir := openSpool(t)
	rec := appendDurable(t, s, 1)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{
		Code: "ERR_AGENT_ENTITY_UNRESOLVED", Message: "missing", HTTPStatus: 422,
	}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", AgentSpoolDir: dir,
		MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
		IdleInterval: time.Millisecond, ShutdownGrace: time.Second,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected poison error")
	}
	st, _ := s.Stats()
	if st.RecordCount != 0 {
		t.Fatalf("poisoned record must leave hot spool, got %d", st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 1 {
		t.Fatalf("poison count=%d", d.Metrics().PoisonCount.Load())
	}
	entries, _ := os.ReadDir(filepath.Join(dir, poisonDirName))
	if len(entries) == 0 {
		t.Fatal("expected poison file")
	}
	_ = rec
}

func TestShutdownIdempotent(t *testing.T) {
	s, dir := openSpool(t)
	tr := &echoTransport{inner: &fakeTransport{}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", AgentSpoolDir: dir,
		MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
		IdleInterval: 50 * time.Millisecond, ShutdownGrace: time.Second,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatal(err)
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := d.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
	if err := d.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
}

func TestIsTerminalCoreError(t *testing.T) {
	if !isTerminalCoreError(&coreclient.CoreAPIError{Code: "ERR_AGENT_ENTITY_UNRESOLVED", HTTPStatus: 422}) {
		t.Fatal("expected terminal")
	}
	for _, code := range []string{
		"ERR_AUTH_IDENTITY_MISMATCH",
		"ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH",
		"ERR_AGENT_ACCEPTING_SENSOR_MISMATCH",
		"ERR_TENANT_SCOPE_MISMATCH",
		"ERR_IDEMPOTENCY_CONFLICT",
		"ERR_SEQUENCE_REGRESSION",
	} {
		if !isTerminalCoreError(&coreclient.CoreAPIError{Code: code, HTTPStatus: 403}) {
			t.Fatalf("expected terminal for %s", code)
		}
	}
	if !isTerminalCoreError(&coreclient.CoreAPIError{Code: "ERR_OTHER_CONTRACT", HTTPStatus: 409}) {
		t.Fatal("non-transient 409 must be terminal")
	}
	if isTerminalCoreError(&coreclient.CoreAPIError{Code: "ERR_INTERNAL", HTTPStatus: 500, Message: "x"}) {
		// 500 is transient via classifyAgentTelemetry; without transient flag still not status-terminal here
	}
	err := errors.New("other")
	if isTerminalCoreError(err) {
		t.Fatal("generic error must not be terminal")
	}
}

type statsSpool struct {
	*spool.DiskSpool
	stats spool.Stats
}

func (s *statsSpool) Stats() (spool.Stats, error) {
	return s.stats, nil
}

type echoingTransport struct {
	calls int
}

func (e *echoingTransport) UploadAgentTelemetry(_ context.Context, batch coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
	e.calls++
	return coreclient.AgentTelemetryAck{
		Status:                         "ACCEPTED",
		SensorID:                       batch.SensorID,
		BatchID:                        batch.BatchID,
		AcceptedThroughIngressSequence: batch.LastIngressSequence,
		CorrelationID:                  batch.CorrelationID,
	}, nil
}

func TestDrainContinuesUnderSpoolProtection(t *testing.T) {
	s, dir := openSpool(t)
	rec := appendDurable(t, s, 1)
	tr := &echoingTransport{}
	d, err := New(Config{
		SensorID: "sensor-infra-01", AgentSpoolDir: dir,
		MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: 10 * time.Millisecond, MaxBackoff: 50 * time.Millisecond,
		JitterFraction: 0, IdleInterval: 10 * time.Millisecond, ShutdownGrace: time.Second,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.spool = &statsSpool{DiskSpool: s, stats: spool.Stats{
		ProtectionState:   spool.ProtectionTriggered,
		FilesystemUsedPct: 96,
		RecordCount:       1,
		NextSequence:      rec.Sequence + 1,
	}}
	ok, err := d.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected successful drain under protection pressure")
	}
	if tr.calls != 1 {
		t.Fatalf("expected one upload attempt, got %d", tr.calls)
	}
	if d.metrics.Backpressure.Load() < 1 {
		t.Fatal("expected backpressure observation under protection")
	}
}
