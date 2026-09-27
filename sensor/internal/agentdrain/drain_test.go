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
	ack  coreclient.AgentTelemetryAck
	err  error
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
		TenantID:      "018f0e0a-0000-7000-8000-000000000001",
		AgentID:       "agent-1",
		SensorID:      "sensor-infra-01",
		Generation:    1,
		FirstSequence: 10,
		LastSequence:  10,
		RecordIDs:     []string{"r-10"},
		BatchChecksum: "chk",
		BatchID:       "ab-1",
		CorrelationID: "corr-1",
		Timestamp:     time.Now().UTC(),
		Envelope:      json.RawMessage(`{"records":[{"sequence":10,"id":"r-10","checksum":"c1","payload":{"hostId":"h1","observations":[]}}]}`),
	}
	enc, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := s.Append(spool.AppendRequest{
		Class:     spool.ClassOperational,
		Type:      agentBatchRecordType,
		Payload:   enc,
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
		SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
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
		Status:                         "ACCEPTED",
		SensorID:                       batch.SensorID,
		BatchID:                        batch.BatchID,
		AcceptedThroughIngressSequence: batch.LastIngressSequence,
		CorrelationID:                  batch.CorrelationID,
	}, nil
}

func TestDrainRetainsSpoolOnTransientFailure(t *testing.T) {
	s, dir := openSpool(t)
	_ = appendDurable(t, s, 1)
	tr := &fakeTransport{err: transientErr{}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
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

func TestDrainParksMalformedPayload(t *testing.T) {
	s, dir := openSpool(t)
	rec := appendDurable(t, s, 1)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{
		Code: "ERR_MALFORMED_RECORD", Message: "bad record", HTTPStatus: 400,
	}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
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
		Code: "ERR_MALFORMED_RECORD", Message: "bad record", HTTPStatus: 400,
	}}
	var logBuf strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
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
		Code: "ERR_MALFORMED_RECORD", Message: "bad record", HTTPStatus: 400,
	}}
	syncCalled := false
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
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
		SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
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

func TestDrainParksRecordTooLarge(t *testing.T) {
	s, dir := openSpool(t)
	_ = appendDurable(t, s, 1)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{
		Code: "ERR_RECORD_TOO_LARGE", Message: "record exceeds maximum size", HTTPStatus: 413,
	}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
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
		t.Fatalf("oversized Core rejection must leave hot spool, got %d", st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 1 {
		t.Fatalf("poison count=%d", d.Metrics().PoisonCount.Load())
	}
	entries, _ := os.ReadDir(filepath.Join(dir, poisonDirName))
	if len(entries) == 0 {
		t.Fatal("expected poison file for ERR_RECORD_TOO_LARGE")
	}
}

func TestIsTerminalCoreError(t *testing.T) {
	for _, code := range []string{
		"ERR_MALFORMED_JSON",
		"ERR_MALFORMED_RECORD",
		"ERR_MALFORMED_RECORD_PAYLOAD",
		"ERR_UNSUPPORTED_SCHEMA_VERSION",
		"ERR_CHECKSUM_MISMATCH",
		"ERR_AGENT_HOST_ID_REQUIRED",
		"ERR_RECORD_TOO_LARGE",
		"ERR_BATCH_TOO_LARGE",
		"ERR_REQUEST_TOO_LARGE",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_REQUIRED",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_INVALID",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTED_BEFORE_ACTIVATION",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION",
	} {
		if !isTerminalCoreError(&coreclient.CoreAPIError{Code: code, HTTPStatus: 400}) {
			t.Fatalf("expected poison allowlist terminal for %s", code)
		}
	}
	for _, code := range []string{
		"ERR_AGENT_ENTITY_UNRESOLVED",
		"ERR_AGENT_ASSIGNMENT_UNKNOWN",
		"ERR_AUTH_IDENTITY_MISMATCH",
		"ERR_AUTH_CLIENT_CERT_REQUIRED",
		"ERR_SENSOR_AUTHORITY_UNAVAILABLE",
		"ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH",
		"ERR_AGENT_ACCEPTING_SENSOR_MISMATCH",
		"ERR_TENANT_SCOPE_MISMATCH",
		"ERR_REVOKED_ASSIGNMENT_REVOKED_AT_MISSING",
		"ERR_PREVIOUS_BATCH_PENDING",
		"ERR_BATCH_PUBLICATION_IN_PROGRESS",
		"ERR_SENSOR_STREAM_CHANGED",
		"ERR_IDEMPOTENCY_CONFLICT",
		"ERR_SEQUENCE_REGRESSION",
		"ERR_SEQUENCE_GAP",
		"ERR_SEQUENCE_OUT_OF_BOUNDS",
		"ERR_AGENT_STREAM_ID_INVALID",
		"ERR_OTHER_CONTRACT",
		"ERR_UNKNOWN_APPLICATION",
	} {
		if isTerminalCoreError(&coreclient.CoreAPIError{Code: code, HTTPStatus: 403}) {
			t.Fatalf("%s must not be poison-terminal", code)
		}
	}
	for _, status := range []int{400, 401, 403, 409, 413, 422} {
		if isTerminalCoreError(&coreclient.CoreAPIError{Code: "ERR_UNKNOWN_APP", HTTPStatus: status}) {
			t.Fatalf("unknown HTTP %d must not be poison-terminal by status alone", status)
		}
	}
	if isTerminalCoreError(errors.New("other")) {
		t.Fatal("generic error must not be terminal")
	}
}

func TestDrainRetainsSpoolOnAuthorityAndContinuityErrors(t *testing.T) {
	cases := []struct {
		name      string
		code      string
		status    int
		transient bool
	}{
		{"previous_batch_pending", "ERR_PREVIOUS_BATCH_PENDING", 409, true},
		{"publication_in_progress", "ERR_BATCH_PUBLICATION_IN_PROGRESS", 429, true},
		{"stream_changed", "ERR_SENSOR_STREAM_CHANGED", 409, false},
		{"idempotency_conflict", "ERR_IDEMPOTENCY_CONFLICT", 409, false},
		{"sequence_regression", "ERR_SEQUENCE_REGRESSION", 409, false},
		{"sequence_gap", "ERR_SEQUENCE_GAP", 400, false},
		{"sequence_out_of_bounds", "ERR_SEQUENCE_OUT_OF_BOUNDS", 400, false},
		{"agent_stream_id_invalid", "ERR_AGENT_STREAM_ID_INVALID", 400, false},
		{"sensor_authority_unavailable", "ERR_SENSOR_AUTHORITY_UNAVAILABLE", 403, false},
		{"auth_client_cert_required", "ERR_AUTH_CLIENT_CERT_REQUIRED", 401, false},
		{"auth_identity_mismatch", "ERR_AUTH_IDENTITY_MISMATCH", 403, false},
		{"agent_assignment_unknown", "ERR_AGENT_ASSIGNMENT_UNKNOWN", 422, false},
		{"agent_assignment_sensor_mismatch", "ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH", 403, false},
		{"agent_accepting_sensor_mismatch", "ERR_AGENT_ACCEPTING_SENSOR_MISMATCH", 403, false},
		{"tenant_scope_mismatch", "ERR_TENANT_SCOPE_MISMATCH", 403, false},
		{"agent_entity_unresolved", "ERR_AGENT_ENTITY_UNRESOLVED", 422, false},
		{"revoked_at_missing", "ERR_REVOKED_ASSIGNMENT_REVOKED_AT_MISSING", 422, false},
		{"unknown_400", "ERR_UNKNOWN_APPLICATION_400", 400, false},
		{"unknown_403", "ERR_UNKNOWN_APPLICATION_403", 403, false},
		{"unknown_409", "ERR_UNKNOWN_APPLICATION_409", 409, false},
		{"unknown_413", "ERR_UNKNOWN_APPLICATION_413", 413, false},
		{"unknown_422", "ERR_UNKNOWN_APPLICATION_422", 422, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := openSpool(t)
			_ = appendDurable(t, s, 1)
			body := []byte(`{"status":"REJECTED","code":"` + tc.code + `","message":"retain"}`)
			tr := &fakeTransport{err: coreclient.ClassifyAgentTelemetryFailure(tc.status, body)}
			d, err := New(Config{
				SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
				MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
				RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
				IdleInterval: time.Millisecond, ShutdownGrace: time.Second, JitterFraction: 0,
			}, s, tr)
			if err != nil {
				t.Fatal(err)
			}
			ok, err := d.RunOnce(context.Background())
			if ok {
				t.Fatal("expected no successful drain")
			}
			if err == nil {
				t.Fatal("expected error")
			}
			st, _ := s.Stats()
			if st.RecordCount != 1 {
				t.Fatalf("spool must be retained for %s, got count=%d", tc.code, st.RecordCount)
			}
			if d.Metrics().PoisonCount.Load() != 0 {
				t.Fatalf("poison must not be created for %s", tc.code)
			}
			entries, _ := os.ReadDir(filepath.Join(dir, poisonDirName))
			if len(entries) != 0 {
				t.Fatalf("poison file created for %s: %v", tc.code, entries)
			}
			if tc.transient {
				var drainErr *Error
				if !errors.As(err, &drainErr) || !drainErr.Transient() {
					t.Fatalf("%s must be retryable/transient, got %v", tc.code, err)
				}
			} else {
				var drainErr *Error
				if errors.As(err, &drainErr) && drainErr.Transient() {
					t.Fatalf("%s must fail closed (non-transient) with spool retained, got %v", tc.code, err)
				}
			}
		})
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
		SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
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
		SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
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

func appendNDurable(t *testing.T, s *spool.DiskSpool, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		_ = appendDurable(t, s, uint64(i+1))
	}
}

func TestDrainFrozenWindowSurvivesGrowthRestartAndExactIdentity(t *testing.T) {
	dir := t.TempDir()
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	appendNDurable(t, s, 5)

	var first coreclient.AgentTelemetryBatch
	calls := 0
	tr := &fakeTransport{}
	tr.err = errors.New("transport down")
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		MaxBatchRecords: 5, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond,
		JitterFraction: 0, IdleInterval: time.Hour, ShutdownGrace: time.Second,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	capture := &captureTransport{inner: tr, onCall: func(b coreclient.AgentTelemetryBatch) {
		calls++
		if calls == 1 {
			first = b
		} else if b.FirstIngressSequence != first.FirstIngressSequence ||
			b.LastIngressSequence != first.LastIngressSequence ||
			b.Checksum != first.Checksum || b.BatchID != first.BatchID {
			t.Fatalf("retry must reuse exact frozen Agent-ingress identity\n first=%+v\n retry=%+v", first, b)
		}
	}}
	d.transport = capture

	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("first drain must fail, ok=%v err=%v", ok, err)
	}
	if first.FirstIngressSequence != 1 || first.LastIngressSequence != 5 {
		t.Fatalf("first range=%d..%d", first.FirstIngressSequence, first.LastIngressSequence)
	}

	appendNDurable(t, s, 5) // sequences 6..10
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s2.Close() })

	tr.err = nil
	tr.ack = coreclient.AgentTelemetryAck{} // filled by capture echo
	d2, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s2.StreamID(), AgentSpoolDir: dir,
		MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond,
		JitterFraction: 0, IdleInterval: time.Hour, ShutdownGrace: time.Second,
	}, s2, tr)
	if err != nil {
		t.Fatal(err)
	}
	d2.transport = &echoCaptureTransport{onCall: func(b coreclient.AgentTelemetryBatch) {
		calls++
		if b.FirstIngressSequence != first.FirstIngressSequence ||
			b.LastIngressSequence != first.LastIngressSequence ||
			b.Checksum != first.Checksum || b.BatchID != first.BatchID {
			t.Fatalf("post-restart retry must reuse exact frozen identity\n first=%+v\n retry=%+v", first, b)
		}
	}}
	ok, err = d2.RunOnce(context.Background())
	if err != nil || !ok {
		t.Fatalf("retry after growth/restart must succeed: ok=%v err=%v", ok, err)
	}
	next, err := s2.ReadOrReserveOutboundBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if next.FirstSequence != 6 || next.LastSequence != 10 {
		t.Fatalf("after ACK next Agent window must be 6..10, got %#v", next)
	}
}

func TestDrainPoisonAbandonsWindowBeforeHeadAck(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 3)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{
		Code: "ERR_RECORD_TOO_LARGE", HTTPStatus: 400, Message: "too large",
	}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		MaxBatchRecords: 3, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond,
		JitterFraction: 0, IdleInterval: time.Hour, ShutdownGrace: time.Second,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("expected poison, ok=%v err=%v", ok, err)
	}
	st, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.RecordCount != 2 {
		t.Fatalf("poison head ACK must leave remaining records, got %d", st.RecordCount)
	}
	next, err := s.ReadOrReserveOutboundBatch(10, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	if next.FirstSequence != 2 || next.LastSequence != 3 {
		t.Fatalf("after poison abandon, new window over remaining records: %#v", next)
	}
}

type captureTransport struct {
	inner  *fakeTransport
	onCall func(coreclient.AgentTelemetryBatch)
}

func (c *captureTransport) UploadAgentTelemetry(ctx context.Context, batch coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
	if c.onCall != nil {
		c.onCall(batch)
	}
	return c.inner.UploadAgentTelemetry(ctx, batch)
}

type echoCaptureTransport struct {
	onCall func(coreclient.AgentTelemetryBatch)
}

func (e *echoCaptureTransport) UploadAgentTelemetry(_ context.Context, batch coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
	if e.onCall != nil {
		e.onCall(batch)
	}
	return coreclient.AgentTelemetryAck{
		Status: "ACCEPTED", SensorID: batch.SensorID, BatchID: batch.BatchID,
		AcceptedThroughIngressSequence: batch.LastIngressSequence, CorrelationID: batch.CorrelationID,
	}, nil
}

func TestAuthorityPoisonQuarantineAdvancesPastHead(t *testing.T) {
	codes := []string{
		"ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_REQUIRED",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_INVALID",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTED_BEFORE_ACTIVATION",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION",
	}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			s, dir := openSpool(t)
			_ = appendDurable(t, s, 1)
			_ = appendDurable(t, s, 2)
			calls := 0
			tr := &fakeTransport{}
			tr.err = &coreclient.CoreAPIError{Code: code, Message: "revoked temporal", HTTPStatus: 422}
			d, err := New(Config{
				SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
				MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
				RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
				IdleInterval: time.Millisecond, ShutdownGrace: time.Second, JitterFraction: 0,
			}, s, tr)
			if err != nil {
				t.Fatal(err)
			}
			ok, err := d.RunOnce(context.Background())
			if ok || err == nil {
				t.Fatalf("expected poison path, ok=%v err=%v", ok, err)
			}
			st, _ := s.Stats()
			if st.RecordCount != 1 {
				t.Fatalf("poison must remove head only, remaining=%d", st.RecordCount)
			}
			if d.Metrics().PoisonCount.Load() != 1 {
				t.Fatalf("PoisonCount=%d", d.Metrics().PoisonCount.Load())
			}
			if d.Metrics().AuthorityPoisonCount.Load() != 1 {
				t.Fatalf("AuthorityPoisonCount=%d", d.Metrics().AuthorityPoisonCount.Load())
			}
			entries, _ := os.ReadDir(filepath.Join(dir, poisonDirName))
			if len(entries) == 0 {
				t.Fatal("expected poison file")
			}
			// Subsequent valid record can drain.
			tr.err = nil
			tr.ack = coreclient.AgentTelemetryAck{}
			cap := &echoCaptureTransport{onCall: func(coreclient.AgentTelemetryBatch) { calls++ }}
			d2, err := New(Config{
				SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
				MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
				RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
				IdleInterval: time.Millisecond, ShutdownGrace: time.Second, JitterFraction: 0,
			}, s, cap)
			if err != nil {
				t.Fatal(err)
			}
			ok, err = d2.RunOnce(context.Background())
			if !ok || err != nil {
				t.Fatalf("next valid record must drain, ok=%v err=%v", ok, err)
			}
			if calls != 1 {
				t.Fatalf("expected one upload of remaining record, calls=%d", calls)
			}
			st, _ = s.Stats()
			if st.RecordCount != 0 {
				t.Fatalf("spool must be empty after next drain, got %d", st.RecordCount)
			}
		})
	}
}

func TestAuthorityPoisonDirSyncFailureDoesNotIncrementAuthorityPoisonCount(t *testing.T) {
	s, dir := openSpool(t)
	_ = appendDurable(t, s, 1)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{
		Code: "ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION", Message: "revoked", HTTPStatus: 422,
	}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
		MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
		IdleInterval: time.Millisecond, ShutdownGrace: time.Second, JitterFraction: 0,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.syncDir = func(string) error { return errors.New("injected poison dir fsync failure") }
	_, err = d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected poison durability failure")
	}
	st, _ := s.Stats()
	if st.RecordCount != 1 {
		t.Fatalf("head must be retained, count=%d", st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 0 || d.Metrics().AuthorityPoisonCount.Load() != 0 {
		t.Fatalf("failed poison must not increment counts: poison=%d authority=%d",
			d.Metrics().PoisonCount.Load(), d.Metrics().AuthorityPoisonCount.Load())
	}
}

func TestMalformedPoisonDoesNotIncrementAuthorityPoisonCount(t *testing.T) {
	s, dir := openSpool(t)
	_ = appendDurable(t, s, 1)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{
		Code: "ERR_MALFORMED_RECORD", Message: "bad", HTTPStatus: 400,
	}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
		MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
		IdleInterval: time.Millisecond, ShutdownGrace: time.Second, JitterFraction: 0,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected poison")
	}
	if d.Metrics().PoisonCount.Load() != 1 {
		t.Fatalf("PoisonCount=%d", d.Metrics().PoisonCount.Load())
	}
	if d.Metrics().AuthorityPoisonCount.Load() != 0 {
		t.Fatalf("AuthorityPoisonCount must stay 0 for malformed poison, got %d", d.Metrics().AuthorityPoisonCount.Load())
	}
}

func newAuthorityPoisonDrain(t *testing.T, s *spool.DiskSpool, dir string, logger *slog.Logger) (*Drain, *fakeTransport) {
	t.Helper()
	tr := &fakeTransport{err: &coreclient.CoreAPIError{
		Code: "ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION", Message: "revoked", HTTPStatus: 422,
	}}
	cfg := Config{
		SensorID: "sensor-infra-01", StreamID: "stream-00112233445566778899aabbccddeeff", AgentSpoolDir: dir,
		MaxBatchRecords: 10, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
		RequestTimeout: time.Second, InitialBackoff: time.Millisecond, MaxBackoff: 2 * time.Millisecond,
		IdleInterval: time.Millisecond, ShutdownGrace: time.Second, JitterFraction: 0,
		Logger: logger,
	}
	d, err := New(cfg, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	return d, tr
}

func assertPoisonFailureRetainsHead(t *testing.T, d *Drain, s *spool.DiskSpool, dir string, logBuf *strings.Builder, payload []byte) {
	t.Helper()
	_, err := d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected poison durability failure")
	}
	var drainErr *Error
	if !errors.As(err, &drainErr) || drainErr.Code != CodePoison {
		t.Fatalf("expected poison error, got %v", err)
	}
	st, _ := s.Stats()
	if st.RecordCount != 1 {
		t.Fatalf("head must remain in spool, count=%d", st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 0 || d.Metrics().AuthorityPoisonCount.Load() != 0 {
		t.Fatalf("failed poison must not increment counts: poison=%d authority=%d",
			d.Metrics().PoisonCount.Load(), d.Metrics().AuthorityPoisonCount.Load())
	}
	poisonDir := filepath.Join(dir, poisonDirName)
	entries, err := os.ReadDir(poisonDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".poison-") && strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp poison file must be cleaned up after failure: %s", e.Name())
		}
		if strings.HasSuffix(e.Name(), ".json") {
			t.Fatalf("final poison artifact must not exist after pre-publish failure: %s", e.Name())
		}
	}
	if logBuf != nil && len(payload) > 0 && strings.Contains(logBuf.String(), string(payload)) {
		t.Fatalf("payload must not appear in logs: %s", logBuf.String())
	}
}

func TestAuthorityPoisonWriteFailureRetainsHead(t *testing.T) {
	s, dir := openSpool(t)
	rec := appendDurable(t, s, 1)
	var logBuf strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	d, tr := newAuthorityPoisonDrain(t, s, dir, logger)

	d.poisonWrite = func(*os.File, []byte) (int, error) {
		return 0, errors.New("injected poison file write failure")
	}
	assertPoisonFailureRetainsHead(t, d, s, dir, &logBuf, rec.Payload)

	// Retry succeeds after seam removed.
	d.poisonWrite = nil
	tr.err = &coreclient.CoreAPIError{
		Code: "ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION", Message: "revoked", HTTPStatus: 422,
	}
	_, err := d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected poison completion error after successful park")
	}
	st, _ := s.Stats()
	if st.RecordCount != 0 {
		t.Fatalf("retry must advance past head, count=%d", st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 1 || d.Metrics().AuthorityPoisonCount.Load() != 1 {
		t.Fatalf("retry counts poison=%d authority=%d", d.Metrics().PoisonCount.Load(), d.Metrics().AuthorityPoisonCount.Load())
	}
}

func TestAuthorityPoisonFileSyncFailureRetainsHead(t *testing.T) {
	s, dir := openSpool(t)
	rec := appendDurable(t, s, 1)
	var logBuf strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	d, tr := newAuthorityPoisonDrain(t, s, dir, logger)

	d.poisonSync = func(*os.File) error {
		return errors.New("injected poison file fsync failure")
	}
	assertPoisonFailureRetainsHead(t, d, s, dir, &logBuf, rec.Payload)

	d.poisonSync = nil
	tr.err = &coreclient.CoreAPIError{
		Code: "ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION", Message: "revoked", HTTPStatus: 422,
	}
	_, err := d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected poison completion error after successful park")
	}
	st, _ := s.Stats()
	if st.RecordCount != 0 {
		t.Fatalf("retry must advance past head, count=%d", st.RecordCount)
	}
	if d.Metrics().AuthorityPoisonCount.Load() != 1 {
		t.Fatalf("AuthorityPoisonCount=%d", d.Metrics().AuthorityPoisonCount.Load())
	}
}

func TestAuthorityPoisonRenameFailureRetainsHead(t *testing.T) {
	s, dir := openSpool(t)
	rec := appendDurable(t, s, 1)
	var logBuf strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	d, tr := newAuthorityPoisonDrain(t, s, dir, logger)

	d.poisonRename = func(string, string) error {
		return errors.New("injected poison rename failure")
	}
	assertPoisonFailureRetainsHead(t, d, s, dir, &logBuf, rec.Payload)

	d.poisonRename = nil
	tr.err = &coreclient.CoreAPIError{
		Code: "ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION", Message: "revoked", HTTPStatus: 422,
	}
	_, err := d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected poison completion error after successful park")
	}
	st, _ := s.Stats()
	if st.RecordCount != 0 {
		t.Fatalf("retry must advance past head, count=%d", st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 1 || d.Metrics().AuthorityPoisonCount.Load() != 1 {
		t.Fatalf("retry counts poison=%d authority=%d", d.Metrics().PoisonCount.Load(), d.Metrics().AuthorityPoisonCount.Load())
	}
}

func TestAuthorityPoisonSuccessfulLogOmitsPathAndPayload(t *testing.T) {
	s, dir := openSpool(t)
	rec := appendDurable(t, s, 1)
	var logBuf strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	d, _ := newAuthorityPoisonDrain(t, s, dir, logger)

	_, err := d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected poison completion error")
	}
	if d.Metrics().AuthorityPoisonCount.Load() != 1 {
		t.Fatalf("AuthorityPoisonCount=%d", d.Metrics().AuthorityPoisonCount.Load())
	}
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "Parked terminal agent-ingress record") {
		t.Fatalf("expected parking log, got %s", logOutput)
	}
	if !strings.Contains(logOutput, "reason=ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION") {
		t.Fatalf("log must contain reason: %s", logOutput)
	}
	if !strings.Contains(logOutput, "sequence=") {
		t.Fatalf("log must contain sequence: %s", logOutput)
	}
	if strings.Contains(logOutput, "poison_path") {
		t.Fatalf("log must not contain poison_path: %s", logOutput)
	}
	if strings.Contains(logOutput, string(rec.Payload)) {
		t.Fatalf("log must not contain payload: %s", logOutput)
	}
	poisonDir := filepath.Join(dir, poisonDirName)
	entries, _ := os.ReadDir(poisonDir)
	for _, e := range entries {
		if strings.Contains(logOutput, e.Name()) {
			t.Fatalf("log must not contain poison file name %s: %s", e.Name(), logOutput)
		}
	}
}
