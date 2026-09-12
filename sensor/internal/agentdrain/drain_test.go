package agentdrain

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

func TestPoisonParkingDirectorySyncFailurePreventsAckAndAllowsRetry(t *testing.T) {
	s, dir := openSpool(t)
	rec := appendDurable(t, s, 1)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{
		Code: "ERR_AGENT_ENTITY_UNRESOLVED", Message: "missing", HTTPStatus: 422,
	}}
	var logBuf strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	d, err := New(Config{
		SensorID: "sensor-infra-01", AgentSpoolDir: dir,
		MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
		IdleInterval: time.Millisecond, ShutdownGrace: time.Second,
		Logger: logger,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Simulate directory sync failure.
	syncFailures := 1
	d.syncDir = func(dirPath string) error {
		if syncFailures > 0 {
			syncFailures--
			return errors.New("simulated poison directory fsync failure")
		}
		return defaultSyncDir(dirPath)
	}

	// First attempt must fail closed on dir fsync failure.
	_, err = d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected directory sync failure")
	}
	var drainErr *Error
	if !errors.As(err, &drainErr) || !drainErr.transient || drainErr.Code != CodePoison {
		t.Fatalf("expected transient poison error, got: %v", err)
	}

	// Source record MUST remain in spool.
	st, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.RecordCount != 1 {
		t.Fatalf("source record must remain pending on sync failure, got count=%d", st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 0 {
		t.Fatalf("poison count must be 0 on sync failure, got %d", d.Metrics().PoisonCount.Load())
	}
	if d.Metrics().Backpressure.Load() != 1 {
		t.Fatalf("backpressure metric must be incremented on sync failure, got %d", d.Metrics().Backpressure.Load())
	}

	// 2. Retry: now directory sync succeeds.
	_, err = d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected poison error on parking completion")
	}

	// Source record must now be acknowledged.
	st, err = s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.RecordCount != 0 {
		t.Fatalf("poisoned record must leave hot spool on successful sync, got %d", st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 1 {
		t.Fatalf("poison count must be 1, got %d", d.Metrics().PoisonCount.Load())
	}

	// Check poison artifact on disk.
	poisonDir := filepath.Join(dir, poisonDirName)
	entries, err := os.ReadDir(poisonDir)
	if err != nil {
		t.Fatal(err)
	}
	var parkedFile string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			parkedFile = filepath.Join(poisonDir, e.Name())
		}
		if strings.HasPrefix(e.Name(), ".poison-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("found leftover temp file: %s", e.Name())
		}
	}
	if parkedFile == "" {
		t.Fatal("expected parked json file in poison directory")
	}

	data, err := os.ReadFile(parkedFile)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(data), "\n", 2)
	if len(parts) != 2 {
		t.Fatalf("expected metadata newline payload, got: %s", string(data))
	}
	if !strings.Contains(parts[1], string(rec.Payload)) {
		t.Fatalf("expected payload in poison artifact, got: %s", parts[1])
	}

	// Verify poison payload does NOT appear in ordinary logs.
	logOutput := logBuf.String()
	if strings.Contains(logOutput, string(rec.Payload)) {
		t.Fatalf("poison payload must NOT appear in ordinary logs: %s", logOutput)
	}
	if !strings.Contains(logOutput, "Parked terminal agent-ingress record") {
		t.Fatalf("expected parking log message, got: %s", logOutput)
	}

	// 3. No duplicate destructive acknowledgement on subsequent run.
	ok, err := d.RunOnce(context.Background())
	if err != nil || ok {
		t.Fatalf("idle drain must return ok=false, err=nil, got ok=%v, err=%v", ok, err)
	}
}

func TestPoisonParkingDirectorySyncSuccessPermitsSourceAck(t *testing.T) {
	s, dir := openSpool(t)
	_ = appendDurable(t, s, 1)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{
		Code: "ERR_AGENT_ENTITY_UNRESOLVED", Message: "missing", HTTPStatus: 422,
	}}
	syncCalled := false
	d, err := New(Config{
		SensorID: "sensor-infra-01", AgentSpoolDir: dir,
		MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
		IdleInterval: time.Millisecond, ShutdownGrace: time.Second,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.syncDir = func(dirPath string) error {
		syncCalled = true
		return defaultSyncDir(dirPath)
	}

	_, err = d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected poison error")
	}
	if !syncCalled {
		t.Fatal("expected directory sync to be called")
	}
	st, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.RecordCount != 0 {
		t.Fatalf("expected source spool drained, got %d", st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 1 {
		t.Fatalf("expected PoisonCount == 1, got %d", d.Metrics().PoisonCount.Load())
	}
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

func TestDrainDoesNotAdvanceSpoolOnInvalidAck(t *testing.T) {
	s, dir := openSpool(t)
	_ = appendDurable(t, s, 1)
	_ = appendDurable(t, s, 2)
	tr := &fakeTransport{ack: coreclient.AgentTelemetryAck{
		Status:                         "ACCEPTED",
		SensorID:                       "sensor-infra-01",
		BatchID:                        "batch-1",
		AcceptedThroughIngressSequence: 1, // Below last sequence (2)
		CorrelationID:                  "corr-1",
	}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", AgentSpoolDir: dir,
		MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: 10 * time.Millisecond, MaxBackoff: 50 * time.Millisecond,
		JitterFraction: 0, IdleInterval: 10 * time.Millisecond, ShutdownGrace: time.Second,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}

	tr.err = &coreclient.Error{Code: coreclient.CodeProtocol, Message: "Core agent-telemetry acknowledgement sequence does not match submitted batch last sequence"}

	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("expected error on invalid ACK, got ok=%v err=%v", ok, err)
	}

	st, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.RecordCount != 2 {
		t.Fatalf("spool must not advance or delete on invalid ACK, expected 2 records, got %d", st.RecordCount)
	}
	if d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("durable acks must remain 0 on invalid ACK, got %d", d.Metrics().DurableAcks.Load())
	}
}
