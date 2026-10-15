package agentdrain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/spool"
	"platform/sensor/internal/supportability"
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

func TestIsRecordAttributableCoreError(t *testing.T) {
	for _, code := range []string{
		"ERR_MALFORMED_RECORD",
		"ERR_MALFORMED_RECORD_PAYLOAD",
		"ERR_AGENT_HOST_ID_REQUIRED",
		"ERR_RECORD_TOO_LARGE",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_REQUIRED",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_INVALID",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTED_BEFORE_ACTIVATION",
		"ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION",
	} {
		if !isRecordAttributableCoreError(&coreclient.CoreAPIError{Code: code, HTTPStatus: 400}) {
			t.Fatalf("expected poison allowlist terminal for %s", code)
		}
	}
	for _, code := range []string{
		"ERR_MALFORMED_JSON",
		"ERR_UNSUPPORTED_SCHEMA_VERSION",
		"ERR_CHECKSUM_MISMATCH",
		"ERR_BATCH_TOO_LARGE",
		"ERR_REQUEST_TOO_LARGE",
		"ERR_NON_NUMERIC_RECORD",
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
		if isRecordAttributableCoreError(&coreclient.CoreAPIError{Code: code, HTTPStatus: 403}) {
			t.Fatalf("%s must not be poison-terminal", code)
		}
	}
	for _, status := range []int{400, 401, 403, 409, 413, 422} {
		if isRecordAttributableCoreError(&coreclient.CoreAPIError{Code: "ERR_UNKNOWN_APP", HTTPStatus: status}) {
			t.Fatalf("unknown HTTP %d must not be poison-terminal by status alone", status)
		}
	}
	if isRecordAttributableCoreError(errors.New("other")) {
		t.Fatal("generic error must not be terminal")
	}
}

// Hypothetical typed payloads only: these fixtures do not enable a typed producer.
func appendCompatibilityPayload(t *testing.T, s *spool.DiskSpool, collection string) spool.Record {
	t.Helper()
	payload := durableBatchPayload{
		TenantID: "018f0e0a-0000-7000-8000-000000000001", AgentID: "agent-1",
		SensorID: "sensor-infra-01", Generation: 1, FirstSequence: 10, LastSequence: 10,
		RecordIDs: []string{"r-10"}, BatchChecksum: "chk", BatchID: "ab-1", CorrelationID: "corr-1",
		Timestamp: time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC),
		Envelope:  json.RawMessage(`{"records":[{"sequence":10,"id":"r-10","checksum":"c1","payload":` + collection + `}]}`),
	}
	enc, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := s.Append(spool.AppendRequest{
		Class: spool.ClassOperational, Type: agentBatchRecordType, Payload: enc, CreatedAt: payload.Timestamp,
	})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestDrainTypedCompatibilityRejectionDoesNotInventCoreAck(t *testing.T) {
	const typed = `{"hostId":"h1","evidenceType":"ProcessNetworkEvidence","remoteIP":"192.0.2.10","remotePort":443,"protocol":"TCP","bytesTx":1024}`
	const scalar = `{"hostId":"h1","observations":[{"metricKey":"cpu.util","value":42.5,"observedAt":"2026-09-11T10:00:00Z"}]}`
	cases := []struct {
		name     string
		code     string
		payloads []string
		poison   bool
	}{
		{"typed_without_host", "ERR_AGENT_HOST_ID_REQUIRED", []string{`{"evidenceType":"ProcessNetworkEvidence","bytesTx":1024}`}, true},
		{"typed_with_host", "ERR_NON_NUMERIC_RECORD", []string{typed}, false},
		{"scalar_then_typed", "ERR_NON_NUMERIC_RECORD", []string{scalar, typed}, false},
		{"typed_then_scalar", "ERR_NON_NUMERIC_RECORD", []string{typed, scalar}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := openSpool(t)
			var source []spool.Record
			for _, payload := range tc.payloads {
				source = append(source, appendCompatibilityPayload(t, s, payload))
			}
			// Core semantics are exercised by AgentIngestionValidatorTest; this seam
			// exercises real HTTP-error classification and durable Sensor drain behavior.
			tr := &fakeTransport{err: coreclient.ClassifyAgentTelemetryFailure(400,
				[]byte(`{"status":"REJECTED","code":"`+tc.code+`","message":"unsupported typed evidence"}`))}
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
			var drainErr *Error
			if ok || !errors.As(err, &drainErr) || drainErr.Transient() || !coreclient.IsCoreAPICode(err, tc.code) {
				t.Fatalf("expected classified non-transient rejection, ok=%v err=%v", ok, err)
			}
			if len(tr.last.Records) != len(source) {
				t.Fatalf("forwarded records=%d want=%d", len(tr.last.Records), len(source))
			}
			for i, payload := range tc.payloads {
				var deliveries []struct {
					Payload json.RawMessage `json:"payload"`
				}
				if err := json.Unmarshal(tr.last.Records[i].Records, &deliveries); err != nil {
					t.Fatal(err)
				}
				if len(deliveries) != 1 || string(deliveries[0].Payload) != payload {
					t.Fatalf("Sensor must forward original payload without scalar conversion: %s", tr.last.Records[i].Records)
				}
			}
			st, err := s.Stats()
			if err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(filepath.Join(dir, poisonDirName))
			if err != nil {
				t.Fatal(err)
			}
			if tc.poison {
				if drainErr.Code != CodePoison || st.RecordCount != 0 || len(entries) != 1 || d.Metrics().PoisonCount.Load() != 1 {
					t.Fatalf("host-id failure must park the head, stats=%+v entries=%v err=%v", st, entries, drainErr)
				}
				artifact, err := os.ReadFile(filepath.Join(dir, poisonDirName, entries[0].Name()))
				if err != nil {
					t.Fatal(err)
				}
				parts := strings.SplitN(string(artifact), "\n", 2)
				if len(parts) != 2 || !strings.Contains(parts[0], tc.code) || parts[1] != string(source[0].Payload) {
					t.Fatalf("poison artifact must preserve reason and exact source payload: %s", artifact)
				}
			} else {
				if drainErr.Code != CodeTransport || st.RecordCount != len(source) || len(entries) != 0 || d.Metrics().PoisonCount.Load() != 0 {
					t.Fatalf("non-numeric rejection must retain hot spool without poison, stats=%+v entries=%v", st, entries)
				}
				frozen := tr.last
				// A later scalar record cannot bypass the rejected frozen outbound window.
				appendCompatibilityPayload(t, s, scalar)
				ok, err = d.RunOnce(context.Background())
				if ok || !coreclient.IsCoreAPICode(err, tc.code) || tr.last.BatchID != frozen.BatchID ||
					tr.last.Checksum != frozen.Checksum || tr.last.LastIngressSequence != frozen.LastIngressSequence {
					t.Fatalf("retry must retain identical rejected batch, ok=%v err=%v", ok, err)
				}
				retained, err := s.ReadOrReserveOutboundBatch(10, 1024*1024)
				if err != nil || len(retained.Records) != len(source) {
					t.Fatalf("frozen records not retained: %+v err=%v", retained, err)
				}
				for i, rec := range retained.Records {
					if rec.Sequence != source[i].Sequence || string(rec.Payload) != string(source[i].Payload) {
						t.Fatal("retained payload/sequence changed")
					}
				}
				st, err = s.Stats()
				if err != nil || st.RecordCount != len(source)+1 || d.Metrics().FailuresPermanent.Load() != 2 {
					t.Fatalf("scalar backlog must remain visible after repeated rejection: %+v err=%v", st, err)
				}
			}
			if d.Metrics().Successes.Load() != 0 || d.Metrics().DurableAcks.Load() != 0 || d.Metrics().LastSuccessUnix.Load() != 0 {
				t.Fatal("unsupported typed rejection must never report successful Core ingestion")
			}
		})
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
	// Poison attribution requires a singleton outbound window, even with backlog.
	s, dir := openSpool(t)
	appendNDurable(t, s, 3)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{
		Code: "ERR_RECORD_TOO_LARGE", HTTPStatus: 400, Message: "too large",
	}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		MaxBatchRecords: 1, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
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
				MaxBatchRecords: 1, MaxBatchPayloadBytes: 1024 * 1024, MaxRequestBytes: 2 * 1024 * 1024,
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
	logger := supportability.NewJSONLogger(&logBuf, slog.LevelDebug, "0.1.0-test", "sensor-instance-test").
		With("sensor_id", "sensor-infra-01")
	d, _ := newAuthorityPoisonDrain(t, s, dir, logger)

	_, err := d.RunOnce(context.Background())
	if err == nil {
		t.Fatal("expected poison completion error")
	}
	if d.Metrics().AuthorityPoisonCount.Load() != 1 {
		t.Fatalf("AuthorityPoisonCount=%d", d.Metrics().AuthorityPoisonCount.Load())
	}
	logOutput := logBuf.String()
	var entry map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(logOutput)), &entry); err != nil {
		t.Fatalf("decode parking log: %v\n%s", err, logOutput)
	}
	if entry["message"] != "Parked terminal agent-ingress record" {
		t.Fatalf("unexpected parking log: %v", entry)
	}
	if entry["event_code"] != "ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION" {
		t.Fatalf("event_code=%v log=%v", entry["event_code"], entry)
	}
	safe, ok := entry["safe_context"].(map[string]any)
	if !ok || safe["sequence"] != float64(1) {
		t.Fatalf("safe_context=%v", entry["safe_context"])
	}
	for _, forbidden := range []string{"error", "error_code", "poison_path"} {
		if _, exists := entry[forbidden]; exists {
			t.Fatalf("non-canonical field %s leaked: %v", forbidden, entry)
		}
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

// Issue #216: a request-level rejection does not identify the poison spool head.
func TestIssue216AmbiguousPoisonRetainsFrozenBatch(t *testing.T) {
	const scalar = `{"hostId":"h1","observations":[{"metricKey":"cpu.util","value":42.5,"observedAt":"2026-09-11T10:00:00Z"}]}`
	const typed = `{"evidenceType":"ProcessNetworkEvidence","bytesTx":1024}`
	for _, name := range []string{"scalar_then_typed", "typed_then_scalar", "nested_scalar_then_typed", "nested_typed_then_scalar", "invalid_non_head_durable", "invalid_head_durable", "singleton_request_bound"} {
		t.Run(name, func(t *testing.T) {
			s, dir := openSpool(t)
			switch name {
			case "scalar_then_typed":
				appendCompatibilityPayload(t, s, scalar)
				appendCompatibilityPayload(t, s, typed)
			case "typed_then_scalar":
				appendCompatibilityPayload(t, s, typed)
				appendCompatibilityPayload(t, s, scalar)
			case "nested_scalar_then_typed", "nested_typed_then_scalar":
				first, second := scalar, typed
				if name == "nested_typed_then_scalar" {
					first, second = typed, scalar
				}
				rec := appendCompatibilityPayload(t, s, first)
				var payload durableBatchPayload
				if err := json.Unmarshal(rec.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				// Use a separate spool so the nested envelope occupies exactly one record.
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				s, dir = openSpool(t)
				payload.LastSequence = 11
				payload.Envelope = json.RawMessage(`{"records":[{"sequence":10,"id":"r-10","checksum":"c1","payload":` + first + `},{"sequence":11,"id":"r-11","checksum":"c2","payload":` + second + `}]}`)
				raw, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.Append(spool.AppendRequest{Class: spool.ClassOperational, Type: agentBatchRecordType, Payload: raw}); err != nil {
					t.Fatal(err)
				}
			case "invalid_non_head_durable", "invalid_head_durable":
				if name == "invalid_non_head_durable" {
					appendCompatibilityPayload(t, s, scalar)
				}
				if _, err := s.Append(spool.AppendRequest{Class: spool.ClassOperational, Type: agentBatchRecordType, Payload: []byte(`{"tenantId":"incomplete"}`)}); err != nil {
					t.Fatal(err)
				}
				if name == "invalid_head_durable" {
					appendCompatibilityPayload(t, s, scalar)
				}
			case "singleton_request_bound":
				appendCompatibilityPayload(t, s, scalar)
			}
			tr := &fakeTransport{err: coreclient.ClassifyAgentTelemetryFailure(400, []byte(`{"status":"REJECTED","code":"ERR_AGENT_HOST_ID_REQUIRED","message":"Agent hostId is required for entity resolution"}`))}
			var logs strings.Builder
			cfg := Config{SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir, MaxBatchRecords: 10, InitialBackoff: time.Millisecond, MaxBackoff: 7 * time.Millisecond, Logger: slog.New(slog.NewJSONHandler(&logs, nil))}
			if name == "singleton_request_bound" {
				cfg.MaxRequestBytes = 1
			}
			d, err := New(cfg, s, tr)
			if err != nil {
				t.Fatal(err)
			}
			before, err := s.ReadOrReserveOutboundBatch(10, 1024*1024)
			if err != nil {
				t.Fatal(err)
			}
			waits := 0
			d.wait = func(_ context.Context, delay time.Duration) error {
				waits++
				if delay != cfg.MaxBackoff {
					t.Fatalf("unbounded retry delay=%v", delay)
				}
				return nil
			}
			for attempt := 0; attempt < 2; attempt++ {
				ok, err := d.RunOnce(context.Background())
				if ok || err == nil {
					t.Fatalf("rejected batch claimed success: ok=%v err=%v", ok, err)
				}
				if !strings.Contains(name, "durable") && name != "singleton_request_bound" && !coreclient.IsCoreAPICode(err, "ERR_AGENT_HOST_ID_REQUIRED") {
					t.Fatalf("lost Core rejection: %v", err)
				}
				st, err := s.Stats()
				if err != nil {
					t.Fatal(err)
				}
				if st.RecordCount != len(before.Records) {
					t.Fatalf("NO CORE ACK: valid telemetry removed from replay: count=%d want=%d poison=%d", st.RecordCount, len(before.Records), d.metrics.PoisonCount.Load())
				}
				after, err := s.ReadOrReserveOutboundBatch(10, 1024*1024)
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("sequence/checksum/payload or frozen window changed: before=%+v after=%+v err=%v", before, after, err)
				}
			}
			entries, err := os.ReadDir(d.poisonDir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 || d.metrics.PoisonCount.Load() != 0 || d.metrics.AuthorityPoisonCount.Load() != 0 || d.metrics.Successes.Load() != 0 || d.metrics.DurableAcks.Load() != 0 || d.metrics.LastSuccessUnix.Load() != 0 {
				t.Fatal("ambiguous rejection must not archive/ACK/claim ingestion")
			}
			if waits != 2 || d.metrics.FailuresPermanent.Load() != 2 || d.metrics.Backpressure.Load() != 2 || d.metrics.PendingRecords.Load() != int64(len(before.Records)) {
				t.Fatalf("blocked status/backoff absent: waits=%d permanent=%d pressure=%d", waits, d.metrics.FailuresPermanent.Load(), d.metrics.Backpressure.Load())
			}
			if !strings.Contains(logs.String(), "spool retained") || strings.Contains(logs.String(), "bytesTx") || strings.Contains(logs.String(), "tenantId") {
				t.Fatalf("missing safe blocked log: %s", logs.String())
			}
			// Restart and backlog growth must not thaw or reorder the rejected window.
			appendCompatibilityPayload(t, s, scalar)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			restarted, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.Close()
			after, err := restarted.ReadOrReserveOutboundBatch(10, 1024*1024)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("restart changed frozen replay: %v", err)
			}
			d2, err := New(cfg, restarted, tr)
			if err != nil {
				t.Fatal(err)
			}
			d2.wait = d.wait
			if ok, err := d2.RunOnce(context.Background()); ok || err == nil {
				t.Fatalf("restart bypassed rejection: %v %v", ok, err)
			}
			if !strings.Contains(name, "durable") && name != "singleton_request_bound" {
				original, err := d.makeRequest(before)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(original, tr.last) {
					t.Fatal("restart/growth changed outbound identity/checksum")
				}
			}
		})
	}
}

func TestIssue216SingletonMalformedDurableArchive(t *testing.T) {
	s, dir := openSpool(t)
	payload := []byte(`{"tenantId":"incomplete"}`)
	rec, err := s.Append(spool.AppendRequest{Class: spool.ClassOperational, Type: agentBatchRecordType, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	appendDurable(t, s, 2)
	tr := &fakeTransport{}
	d, err := New(Config{SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir, MaxBatchRecords: 1}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := d.RunOnce(context.Background())
	var de *Error
	if ok || !errors.As(err, &de) || de.Code != CodePoison {
		t.Fatalf("singleton malformed payload not parked: %v %v", ok, err)
	}
	if d.metrics.Attempts.Load() != 0 || d.metrics.Successes.Load() != 0 || d.metrics.DurableAcks.Load() != 0 || d.metrics.PoisonCount.Load() != 1 {
		t.Fatal("quarantine must not claim ingestion")
	}
	files, err := os.ReadDir(d.poisonDir)
	if err != nil || len(files) != 1 {
		t.Fatalf("archive missing: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(d.poisonDir, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.SplitN(string(raw), "\n", 2)
	var meta struct {
		Reason   string `json:"reason"`
		Sequence uint64 `json:"sequence"`
	}
	if len(parts) != 2 {
		t.Fatal("archive incomplete")
	}
	if err := json.Unmarshal([]byte(parts[0]), &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Reason != "ERR_MALFORMED_DURABLE_PAYLOAD" || meta.Sequence != rec.Sequence || parts[1] != string(payload) {
		t.Fatal("archive lost reason/sequence/exact durable payload")
	}
	next, err := s.ReadOrReserveOutboundBatch(1, 1024*1024)
	if err != nil || next.FirstSequence != 2 || next.LastSequence != 2 {
		t.Fatalf("singleton quarantine broke continuity: %+v %v", next, err)
	}
	d.transport = &echoTransport{inner: tr}
	if ok, err := d.RunOnce(context.Background()); !ok || err != nil {
		t.Fatalf("next valid ACK failed: %v %v", ok, err)
	}
	st, err := s.Stats()
	if err != nil || st.RecordCount != 0 || d.metrics.DurableAcks.Load() != 1 {
		t.Fatalf("ACK deletion failed: %+v %v", st, err)
	}
}

func TestIssue216PermanentErrorsDoNotAttributeBatchHead(t *testing.T) {
	for _, code := range []string{"ERR_MALFORMED_JSON", "ERR_MALFORMED_RECORD", "ERR_MALFORMED_RECORD_PAYLOAD", "ERR_UNSUPPORTED_SCHEMA_VERSION", "ERR_CHECKSUM_MISMATCH", "ERR_AGENT_HOST_ID_REQUIRED", "ERR_RECORD_TOO_LARGE", "ERR_BATCH_TOO_LARGE", "ERR_REQUEST_TOO_LARGE", "ERR_NON_NUMERIC_RECORD", "ERR_FUTURE_APPLICATION_ERROR", "ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_REQUIRED", "ERR_REVOKED_ASSIGNMENT_ACCEPTANCE_TIME_INVALID", "ERR_REVOKED_ASSIGNMENT_ACCEPTED_BEFORE_ACTIVATION", "ERR_REVOKED_ASSIGNMENT_ACCEPTED_AFTER_REVOCATION"} {
		t.Run(code, func(t *testing.T) {
			s, dir := openSpool(t)
			appendNDurable(t, s, 2)
			tr := &fakeTransport{err: &coreclient.CoreAPIError{Code: code, Message: "untrusted message SECRET_SENTINEL", HTTPStatus: 400}}
			var logs strings.Builder
			d, err := New(Config{SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir, Logger: slog.New(slog.NewJSONHandler(&logs, nil))}, s, tr)
			if err != nil {
				t.Fatal(err)
			}
			d.wait = func(context.Context, time.Duration) error { return nil }
			ok, err := d.RunOnce(context.Background())
			if ok || !coreclient.IsCoreAPICode(err, code) {
				t.Fatalf("original error lost: %v %v", ok, err)
			}
			st, err := s.Stats()
			if err != nil || st.RecordCount != 2 || d.metrics.PoisonCount.Load() != 0 || d.metrics.DurableAcks.Load() != 0 {
				t.Fatalf("batch head attributed: %+v %v", st, err)
			}
			if !strings.Contains(logs.String(), code) || strings.Contains(logs.String(), "SECRET_SENTINEL") {
				t.Fatalf("unsafe/missing error log: %s", logs.String())
			}
		})
	}
}

func testIssue216BlockedLoopCancellation(t *testing.T, code string, count int) {
	s, dir := openSpool(t)
	appendNDurable(t, s, count)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tr := &fakeTransport{err: &coreclient.CoreAPIError{Code: code, HTTPStatus: 400}}
	d, err := New(Config{SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir, MaxBackoff: time.Hour}, s, &captureTransport{inner: tr, onCall: func(coreclient.AgentTelemetryBatch) { cancel() }})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Start(ctx); err != nil {
		t.Fatal(err)
	}
	shutdown, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	// Await cancellation inside the upload, then shut down the paced loop.
	select {
	case <-ctx.Done():
	case <-shutdown.Done():
		t.Fatal("drain did not reach upload")
	}
	if err := d.Shutdown(shutdown); err != nil {
		t.Fatal(err)
	}
	st, err := s.Stats()
	if err != nil || st.RecordCount != count || d.metrics.Attempts.Load() != 1 || d.metrics.FailuresPermanent.Load() != 1 || d.metrics.DurableAcks.Load() != 0 {
		t.Fatalf("cancellation lost data or spun retries: %+v %v", st, err)
	}
}

func TestIssue216SingletonRequestFailuresRetainFrozenReplay(t *testing.T) {
	const scalar = `{"hostId":"h1","observations":[{"metricKey":"cpu.util","value":42.5,"observedAt":"2026-09-11T10:00:00Z"}]}`
	for _, code := range []string{"ERR_CHECKSUM_MISMATCH", "ERR_UNSUPPORTED_SCHEMA_VERSION", "ERR_MALFORMED_JSON", "ERR_REQUEST_TOO_LARGE"} {
		t.Run(code, func(t *testing.T) {
			s, dir := openSpool(t)
			appendCompatibilityPayload(t, s, scalar)
			status := 400
			if code == "ERR_BATCH_TOO_LARGE" || code == "ERR_REQUEST_TOO_LARGE" {
				status = 413
			}
			tr := &fakeTransport{err: coreclient.ClassifyAgentTelemetryFailure(status, []byte(`{"status":"REJECTED","code":"`+code+`","message":"request rejected"}`))}
			var logs strings.Builder
			cfg := Config{SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir, MaxBatchRecords: 10, MaxBackoff: 7 * time.Millisecond, InitialBackoff: time.Millisecond, Logger: slog.New(slog.NewJSONHandler(&logs, nil))}
			d, err := New(cfg, s, tr)
			if err != nil {
				t.Fatal(err)
			}
			before, err := s.ReadOrReserveOutboundBatch(10, 1024*1024)
			if err != nil {
				t.Fatal(err)
			}
			original, err := d.makeRequest(before)
			if err != nil {
				t.Fatal(err)
			}
			waits := 0
			wait := func(_ context.Context, delay time.Duration) error {
				waits++
				if delay != cfg.MaxBackoff {
					t.Fatalf("retry delay=%v", delay)
				}
				return nil
			}
			d.wait = wait
			assertRetained := func(d *Drain, s *spool.DiskSpool, count int) {
				t.Helper()
				ok, err := d.RunOnce(context.Background())
				var de *Error
				if ok || !errors.As(err, &de) || de.Code != CodeTransport || !coreclient.IsCoreAPICode(err, code) {
					t.Fatalf("rejection lost: ok=%v err=%v", ok, err)
				}
				st, err := s.Stats()
				if err != nil || st.RecordCount != count {
					t.Fatalf("NO CORE ACK: spool count=%d want=%d err=%v", st.RecordCount, count, err)
				}
				after, err := s.ReadOrReserveOutboundBatch(10, 1024*1024)
				if err != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(original, tr.last) {
					t.Fatalf("frozen sequence/checksum/payload/replay identity changed: %v", err)
				}
				entries, err := os.ReadDir(d.poisonDir)
				if err != nil || len(entries) != 0 || d.metrics.PoisonCount.Load() != 0 || d.metrics.AuthorityPoisonCount.Load() != 0 || d.metrics.Successes.Load() != 0 || d.metrics.DurableAcks.Load() != 0 || d.metrics.LastSuccessUnix.Load() != 0 {
					t.Fatal("request rejection archived or falsely ACKed")
				}
				if d.metrics.Backpressure.Load() == 0 || d.metrics.FailuresPermanent.Load() == 0 {
					t.Fatal("blocked status absent")
				}
			}
			assertRetained(d, s, 1)
			assertRetained(d, s, 1)
			appendCompatibilityPayload(t, s, scalar)
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			restarted, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
			if err != nil {
				t.Fatal(err)
			}
			defer restarted.Close()
			d2, err := New(cfg, restarted, tr)
			if err != nil {
				t.Fatal(err)
			}
			d2.wait = wait
			assertRetained(d2, restarted, 2)
			if waits != 3 || d.metrics.Attempts.Load() != 2 || d2.metrics.Attempts.Load() != 1 {
				t.Fatal("retry accounting incorrect")
			}
			if !strings.Contains(logs.String(), code) || !strings.Contains(logs.String(), "spool retained") {
				t.Fatal("visible blocked log absent")
			}
			tr.err = nil
			d2.transport = &echoTransport{inner: tr}
			if ok, err := d2.RunOnce(context.Background()); !ok || err != nil {
				t.Fatalf("valid singleton ACK failed: %v %v", ok, err)
			}
			remaining, err := restarted.ReadOrReserveOutboundBatch(10, 1024*1024)
			if err != nil || len(remaining.Records) != 1 || remaining.FirstSequence != 2 {
				t.Fatalf("ACK crossed frozen window: %+v %v", remaining, err)
			}
			if d2.metrics.Successes.Load() != 1 || d2.metrics.DurableAcks.Load() != 1 || d2.metrics.LastSuccessUnix.Load() == 0 {
				t.Fatal("valid ACK accounting incorrect")
			}
			if ok, err := d2.RunOnce(context.Background()); !ok || err != nil {
				t.Fatalf("backlog ACK failed: %v %v", ok, err)
			}
			st, err := restarted.Stats()
			if err != nil || st.RecordCount != 0 {
				t.Fatalf("ACK failed to drain: %+v %v", st, err)
			}
		})
	}
}

func TestIssue216BlockedLoopCancellation(t *testing.T) {
	testIssue216BlockedLoopCancellation(t, "ERR_AGENT_HOST_ID_REQUIRED", 2)
}
func TestIssue216SingletonRequestFailureCancellation(t *testing.T) {
	for _, code := range []string{"ERR_CHECKSUM_MISMATCH", "ERR_UNSUPPORTED_SCHEMA_VERSION", "ERR_MALFORMED_JSON", "ERR_BATCH_TOO_LARGE", "ERR_REQUEST_TOO_LARGE"} {
		t.Run(code, func(t *testing.T) { testIssue216BlockedLoopCancellation(t, code, 1) })
	}
}

// Simulates the current Core size guard; successful ACKs echo the exact range.
// A size rejection conveys no information about earlier attempts.
type sizeRejectingTransport struct {
	requests  []coreclient.AgentTelemetryBatch
	limit     int
	byteLimit int
}

func (s *sizeRejectingTransport) UploadAgentTelemetry(_ context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
	s.requests = append(s.requests, b)
	encoded, err := json.Marshal(b)
	if err != nil {
		return coreclient.AgentTelemetryAck{}, err
	}
	if b.RecordCount > s.limit || (s.byteLimit > 0 && len(encoded) > s.byteLimit) {
		return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_BATCH_TOO_LARGE", HTTPStatus: 413}
	}
	return coreclient.AgentTelemetryAck{Status: "ACCEPTED", SensorID: b.SensorID, BatchID: b.BatchID, CorrelationID: b.CorrelationID, AcceptedThroughIngressSequence: b.LastIngressSequence}, nil
}

// Models Core's exact-range watermark: first acceptance loses its ACK, size
// guards run before replay lookup, and only the original request recovers ACK.
func TestP1LostAckThenLowerLimitPreservesExactReplay(t *testing.T) {
	s, dir := openSpool(t)
	for i := 0; i < 4; i++ {
		appendCompatibilityPayload(t, s, `{"hostId":"h1","observations":[{"metricKey":"cpu.util","value":42.5}]}`)
	}
	original, err := s.ReadOrReserveOutboundBatch(4, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	var accepted coreclient.AgentTelemetryBatch
	calls := 0
	compatible := true
	tr := p2TransportFunc(func(_ context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		calls++
		if calls == 1 {
			accepted = b
			return coreclient.AgentTelemetryAck{}, errors.New("ACK lost after Core acceptance")
		}
		if !reflect.DeepEqual(b, accepted) {
			t.Fatal("exact replay identity changed")
		}
		if !compatible {
			return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_BATCH_TOO_LARGE", HTTPStatus: 413}
		}
		return coreclient.AgentTelemetryAck{Status: "ACCEPTED", AcceptedThroughIngressSequence: b.LastIngressSequence}, nil
	})
	cfg := Config{SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir, MaxBatchRecords: 4, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond}
	newDrain := func() *Drain {
		d, err := New(cfg, s, tr)
		if err != nil {
			t.Fatal(err)
		}
		d.wait = func(context.Context, time.Duration) error { return nil }
		return d
	}
	d := newDrain()
	if ok, err := d.RunOnce(context.Background()); ok || err == nil {
		t.Fatalf("lost ACK: %v %v", ok, err)
	}
	compatible = false
	for i := 0; i < 2; i++ {
		appendCompatibilityPayload(t, s, `{"hostId":"h1","observations":[{"metricKey":"cpu.util","value":42.5}]}`)
	}
	for i := 0; i < 3; i++ {
		if ok, err := d.RunOnce(context.Background()); ok || !coreclient.IsCoreAPICode(err, "ERR_BATCH_TOO_LARGE") {
			t.Fatalf("size rejection: %v %v", ok, err)
		}
		frozen, err := s.ReadOrReserveOutboundBatch(1, 1)
		if err != nil || !reflect.DeepEqual(frozen, original) {
			t.Fatalf("frozen source drift: %v", err)
		}
		st, err := s.Stats()
		if err != nil || st.RecordCount != 6 || d.metrics.DurableAcks.Load() != 0 || d.metrics.PoisonCount.Load() != 0 {
			t.Fatalf("false ACK/loss: %+v %v", st, err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		s, err = spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
		if err != nil {
			t.Fatal(err)
		}
		d = newDrain()
	}
	defer s.Close()
	compatible = true
	if ok, err := d.RunOnce(context.Background()); !ok || err != nil {
		t.Fatalf("original ACK recovery: %v %v", ok, err)
	}
	remaining, err := s.ReadOrReserveOutboundBatch(4, 1024*1024)
	if err != nil || remaining.FirstSequence != 5 || remaining.LastSequence != 6 || len(remaining.Records) != 2 || d.metrics.DurableAcks.Load() != 1 {
		t.Fatalf("backlog reordered/lost: %+v %v", remaining, err)
	}
}

func TestP1SingletonSizeRejectionRetriesAcrossRestart(t *testing.T) {
	s, dir := openSpool(t)
	appendCompatibilityPayload(t, s, `{"hostId":"h1","observations":[{"metricKey":"cpu.util","value":42.5}]}`)
	original, err := s.ReadOrReserveOutboundBatch(1, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	// Compatibility with sidecars written by the previous singleton dead-end.
	sidecar := filepath.Join(dir, "outbound-window.json")
	raw, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	var win map[string]any
	if err := json.Unmarshal(raw, &win); err != nil {
		t.Fatal(err)
	}
	win["sizeRejected"] = true
	raw, err = json.Marshal(win)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecar, raw, 0640); err != nil {
		t.Fatal(err)
	}
	tr := &sizeRejectingTransport{limit: 0}
	cfg := Config{SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond}
	for i := 0; i < 3; i++ {
		d, err := New(cfg, s, tr)
		if err != nil {
			t.Fatal(err)
		}
		d.wait = func(context.Context, time.Duration) error { return nil }
		if ok, err := d.RunOnce(context.Background()); ok || !coreclient.IsCoreAPICode(err, "ERR_BATCH_TOO_LARGE") {
			t.Fatalf("retry: %v %v", ok, err)
		}
		frozen, err := s.ReadOrReserveOutboundBatch(1, 1)
		if err != nil || !reflect.DeepEqual(frozen, original) {
			t.Fatal("singleton source changed")
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		s, err = spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
		if err != nil {
			t.Fatal(err)
		}
	}
	defer s.Close()
	if len(tr.requests) != 3 {
		t.Fatalf("hidden suppression: %d", len(tr.requests))
	}
	for _, b := range tr.requests {
		if !reflect.DeepEqual(b, tr.requests[0]) {
			t.Fatal("singleton identity drift")
		}
	}
	tr.limit = 1
	d, err := New(cfg, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := d.RunOnce(context.Background()); !ok || err != nil {
		t.Fatalf("corrected limit: %v %v", ok, err)
	}
	st, err := s.Stats()
	if err != nil || st.RecordCount != 0 || d.metrics.DurableAcks.Load() != 1 || d.metrics.PoisonCount.Load() != 0 {
		t.Fatalf("ACK not applied: %+v %v", st, err)
	}
}

func TestP2RebatchAllowlistFailsClosed(t *testing.T) {
	for _, failure := range []struct {
		code   string
		status int
	}{
		{"ERR_BATCH_TOO_LARGE", 400}, {"ERR_REQUEST_TOO_LARGE", 413}, {"", 413}, {"ERR_CHECKSUM_MISMATCH", 413}, {"ERR_AUTH_IDENTITY_MISMATCH", 413}, {"ERR_PREVIOUS_BATCH_PENDING", 409},
	} {
		t.Run(failure.code+fmt.Sprint(failure.status), func(t *testing.T) {
			s, dir := openSpool(t)
			appendNDurable(t, s, 2)
			before, err := s.ReadOrReserveOutboundBatch(2, 1024*1024)
			if err != nil {
				t.Fatal(err)
			}
			tr := &fakeTransport{err: &coreclient.CoreAPIError{Code: failure.code, HTTPStatus: failure.status}}
			d, err := New(Config{SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir}, s, tr)
			if err != nil {
				t.Fatal(err)
			}
			d.wait = func(context.Context, time.Duration) error { return nil }
			if ok, err := d.RunOnce(context.Background()); ok || err == nil {
				t.Fatalf("%v %v", ok, err)
			}
			after, err := s.ReadOrReserveOutboundBatch(1, 1)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("ambiguous range changed")
			}
			blocked, err := s.OutboundBatchSizeRejected()
			if err != nil || blocked {
				t.Fatal("unproven size block")
			}
		})
	}
}

func TestP1ConcurrentRunOncePreservesRejectedRange(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 4)
	tr := &sizeRejectingTransport{limit: 2}
	entered := make(chan struct{})
	release := make(chan struct{})
	calls := 0
	transport := p2TransportFunc(func(ctx context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		calls++
		if calls == 1 {
			close(entered)
			<-release
		}
		return tr.UploadAgentTelemetry(ctx, b)
	})
	d, err := New(Config{SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir}, s, transport)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }
	results := make(chan error, 2)
	go func() { _, err := d.RunOnce(context.Background()); results <- err }()
	<-entered
	secondStarted := make(chan struct{})
	go func() { close(secondStarted); _, err := d.RunOnce(context.Background()); results <- err }()
	<-secondStarted
	close(release)
	firstErr, secondErr := <-results, <-results
	if firstErr == nil || secondErr == nil {
		t.Fatalf("want both original requests size rejected: %v %v", firstErr, secondErr)
	}
	if len(tr.requests) != 2 || tr.requests[0].RecordCount != 4 || tr.requests[1].RecordCount != 4 {
		t.Fatalf("frozen range changed: %+v", tr.requests)
	}
	st, err := s.Stats()
	if err != nil || st.RecordCount != 4 || d.metrics.DurableAcks.Load() != 0 {
		t.Fatalf("size rejection changed records: %+v %v", st, err)
	}
}

type p2TransportFunc func(context.Context, coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error)

func (f p2TransportFunc) UploadAgentTelemetry(ctx context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
	return f(ctx, b)
}

func TestP1SizeRetryPacingAndShutdown(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	calls := make(chan time.Time, 8)
	tr := p2TransportFunc(func(_ context.Context, _ coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		calls <- time.Now()
		return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_BATCH_TOO_LARGE", HTTPStatus: 413}
	})
	d, err := New(Config{SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir, InitialBackoff: time.Millisecond, MaxBackoff: 80 * time.Millisecond, IdleInterval: time.Millisecond}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise direct callers: each rejected attempt waits before returning.
	for i := 0; i < 3; i++ {
		if ok, err := d.RunOnce(context.Background()); ok || err == nil {
			t.Fatalf("%v %v", ok, err)
		}
	}
	previous := <-calls
	for i := 0; i < 2; i++ {
		next := <-calls
		if next.Sub(previous) < d.cfg.MaxBackoff {
			t.Fatalf("retry storm: %v", next.Sub(previous))
		}
		previous = next
	}
	if d.metrics.PendingRecords.Load() != 1 || d.metrics.Backpressure.Load() != 3 || d.metrics.DurableAcks.Load() != 0 {
		t.Fatal("blocked diagnostics incorrect")
	}
	d.cfg.MaxBackoff = time.Hour
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("production loop suppressed retry")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := d.Shutdown(ctx); err != nil {
		t.Fatalf("backoff delayed shutdown: %v", err)
	}
	st, err := s.Stats()
	if err != nil || st.RecordCount != 1 {
		t.Fatal("cancellation lost record")
	}
}

func TestDeliveryHealthBlocked413SmallBacklog(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{Code: "ERR_BATCH_TOO_LARGE", HTTPStatus: 413}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }

	if snap := d.Snapshot(); snap.State != DeliveryStateUp || snap.Blocked {
		t.Fatalf("startup must not claim blocked: %+v", snap)
	}
	if ok, err := d.RunOnce(context.Background()); ok || !coreclient.IsCoreAPICode(err, "ERR_BATCH_TOO_LARGE") {
		t.Fatalf("want size block: ok=%v err=%v", ok, err)
	}
	snap := d.Snapshot()
	if !snap.Blocked || snap.State != DeliveryStateDegraded || snap.ReasonCode != "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("delivery health=%+v", snap)
	}
	if snap.PendingRecords < 1 {
		t.Fatalf("backlog missing: %+v", snap)
	}
	if snap.BlockedSince.IsZero() {
		t.Fatal("blocked_since transition evidence missing")
	}
	raw := fmt.Sprintf("%+v", snap)
	for _, forbidden := range []string{"DEVICE_DOWN", "password", "Bearer", "payload"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("leaked %q in %+v", forbidden, snap)
		}
	}
}

func TestDeliveryHealthRecoversOnVerifiedAck(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	calls := 0
	tr := p2TransportFunc(func(_ context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		calls++
		if calls == 1 {
			return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_BATCH_TOO_LARGE", HTTPStatus: 413}
		}
		return coreclient.AgentTelemetryAck{
			Status: "ACCEPTED", AcceptedThroughIngressSequence: b.LastIngressSequence,
		}, nil
	})
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }
	if ok, err := d.RunOnce(context.Background()); ok || err == nil {
		t.Fatalf("first block: %v %v", ok, err)
	}
	if snap := d.Snapshot(); !snap.Blocked {
		t.Fatal("expected degraded before recovery")
	}
	if ok, err := d.RunOnce(context.Background()); !ok || err != nil {
		t.Fatalf("ack recovery: %v %v", ok, err)
	}
	snap := d.Snapshot()
	if snap.Blocked || snap.State != DeliveryStateUp || snap.ReasonCode != "" {
		t.Fatalf("delivery must clear after ACK: %+v", snap)
	}
	if snap.DurableAcks != 1 {
		t.Fatalf("durable acks=%d", snap.DurableAcks)
	}
	if snap.LastSuccessTime.IsZero() {
		t.Fatal("last success transition evidence missing")
	}
}

func TestDeliveryHealthPersistentRejectionStaysDegraded(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 2)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{Code: "ERR_REQUEST_TOO_LARGE", HTTPStatus: 413}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		MaxBatchRecords: 2, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }
	var since time.Time
	for i := 0; i < 3; i++ {
		if ok, err := d.RunOnce(context.Background()); ok || !coreclient.IsCoreAPICode(err, "ERR_REQUEST_TOO_LARGE") {
			t.Fatalf("attempt %d: %v %v", i, ok, err)
		}
		snap := d.Snapshot()
		if !snap.Blocked || snap.State != DeliveryStateDegraded || snap.ReasonCode != "ERR_REQUEST_TOO_LARGE" {
			t.Fatalf("attempt %d health=%+v", i, snap)
		}
		if i == 0 {
			since = snap.BlockedSince
		} else if !snap.BlockedSince.Equal(since) {
			t.Fatalf("blocked_since reset across persistent rejection: %v -> %v", since, snap.BlockedSince)
		}
	}
}

func TestDeliveryHealthRestartWithFrozenBatch(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	tr := &sizeRejectingTransport{limit: 0}
	cfg := Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}
	d, err := New(cfg, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }
	if ok, err := d.RunOnce(context.Background()); ok || err == nil {
		t.Fatalf("seed block: %v %v", ok, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	d2, err := New(cfg, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d2.wait = func(context.Context, time.Duration) error { return nil }
	// In-memory health resets on restart; first observed retention re-degrades.
	if snap := d2.Snapshot(); snap.Blocked {
		t.Fatalf("stale blocked claim before observation: %+v", snap)
	}
	if ok, err := d2.RunOnce(context.Background()); ok || err == nil {
		t.Fatalf("replay block: %v %v", ok, err)
	}
	snap := d2.Snapshot()
	if !snap.Blocked || snap.State != DeliveryStateDegraded || snap.PendingRecords < 1 {
		t.Fatalf("restart frozen batch health=%+v", snap)
	}
}

func TestDeliveryHealthLegacySizeRejectedSidecar(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	if _, err := s.ReadOrReserveOutboundBatch(1, 1024*1024); err != nil {
		t.Fatal(err)
	}
	sidecar := filepath.Join(dir, "outbound-window.json")
	raw, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	var win map[string]any
	if err := json.Unmarshal(raw, &win); err != nil {
		t.Fatal(err)
	}
	win["sizeRejected"] = true
	raw, err = json.Marshal(win)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecar, raw, 0640); err != nil {
		t.Fatal(err)
	}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, &sizeRejectingTransport{limit: 0})
	if err != nil {
		t.Fatal(err)
	}
	snap := d.Snapshot()
	if !snap.Blocked || snap.State != DeliveryStateDegraded || snap.ReasonCode != "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("legacy sizeRejected must surface delivery degradation: %+v", snap)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }
	if ok, err := d.RunOnce(context.Background()); ok || !coreclient.IsCoreAPICode(err, "ERR_BATCH_TOO_LARGE") {
		t.Fatalf("legacy sidecar must still retry exact batch: %v %v", ok, err)
	}
}

func TestDeliveryHealthConcurrentSnapshotReads(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{Code: "ERR_BATCH_TOO_LARGE", HTTPStatus: 413}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }
	if ok, err := d.RunOnce(context.Background()); ok || err == nil {
		t.Fatalf("block: %v %v", ok, err)
	}
	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				snap := d.Snapshot()
				if snap.State != DeliveryStateDegraded || !snap.Blocked {
					t.Errorf("concurrent snap=%+v", snap)
					return
				}
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

func TestDeliveryHealthCancellationPreservesBlockedClaim(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	tr := &fakeTransport{err: &coreclient.CoreAPIError{Code: "ERR_BATCH_TOO_LARGE", HTTPStatus: 413}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Hour,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	d.wait = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}
	if ok, err := d.RunOnce(ctx); ok || err == nil {
		t.Fatalf("cancelled block: %v %v", ok, err)
	}
	snap := d.Snapshot()
	if !snap.Blocked || snap.State != DeliveryStateDegraded {
		t.Fatalf("cancellation must not clear blocked delivery: %+v", snap)
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer shutdownCancel()
	if err := d.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := d.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if snap := d.Snapshot(); !snap.Blocked {
		t.Fatalf("shutdown must not invent recovery: %+v", snap)
	}
}

// seedLegacySizeRejectedSidecar reserves the outbound window and stamps the
// pre-65fe619 sizeRejected diagnostic flag without suppressing replay.
func seedLegacySizeRejectedSidecar(t *testing.T, s *spool.DiskSpool, dir string) {
	t.Helper()
	if _, err := s.ReadOrReserveOutboundBatch(1, 1024*1024); err != nil {
		t.Fatal(err)
	}
	sidecar := filepath.Join(dir, "outbound-window.json")
	raw, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	var win map[string]any
	if err := json.Unmarshal(raw, &win); err != nil {
		t.Fatal(err)
	}
	win["sizeRejected"] = true
	raw, err = json.Marshal(win)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecar, raw, 0640); err != nil {
		t.Fatal(err)
	}
}

// TestDeliveryHealthStaleLegacyObservationCannotReassertAfterAck proves the
// Codex P2 race: Snapshot observes legacy sizeRejected+pending, RunOnce
// completes a durable ACK and clears deliveryBlocked, then the stale observer
// must not reassert DEGRADED on an empty backlog.
func TestDeliveryHealthStaleLegacyObservationCannotReassertAfterAck(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	seedLegacySizeRejectedSidecar(t, s, dir)

	tr := p2TransportFunc(func(_ context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		return coreclient.AgentTelemetryAck{
			Status: "ACCEPTED", AcceptedThroughIngressSequence: b.LastIngressSequence,
		}, nil
	})
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }

	observed := make(chan struct{})
	release := make(chan struct{})
	d.afterLegacySizeObserve = func() {
		close(observed)
		<-release
	}

	var snap DeliveryStatus
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		snap = d.Snapshot()
	}()

	select {
	case <-observed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for legacy size observation barrier")
	}

	if ok, err := d.RunOnce(context.Background()); !ok || err != nil {
		close(release)
		wg.Wait()
		t.Fatalf("durable ACK during stale observation: ok=%v err=%v", ok, err)
	}
	close(release)
	wg.Wait()

	if snap.Blocked || snap.State != DeliveryStateUp || snap.ReasonCode != "" {
		t.Fatalf("stale legacy observation reasserted blocked health: %+v", snap)
	}
	if snap.PendingRecords != 0 {
		t.Fatalf("expected zero backlog after ACK, got pending=%d", snap.PendingRecords)
	}
	if snap.DurableAcks != 1 {
		t.Fatalf("durable acks=%d want 1", snap.DurableAcks)
	}

	final := d.Snapshot()
	if final.Blocked || final.State != DeliveryStateUp || final.PendingRecords != 0 {
		t.Fatalf("post-ACK healthy state lost: %+v", final)
	}
}

func TestDeliveryHealthLegacySidecarRemainsDegradedWithoutAck(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	seedLegacySizeRejectedSidecar(t, s, dir)
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, &sizeRejectingTransport{limit: 0})
	if err != nil {
		t.Fatal(err)
	}
	snap := d.Snapshot()
	if !snap.Blocked || snap.State != DeliveryStateDegraded || snap.ReasonCode != "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("legitimate legacy size rejection must stay DEGRADED: %+v", snap)
	}
	if snap.PendingRecords < 1 {
		t.Fatalf("backlog missing under legitimate block: %+v", snap)
	}
}

func TestDeliveryHealthConcurrentSnapshotAndAckRace(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	seedLegacySizeRejectedSidecar(t, s, dir)

	var calls atomic.Uint64
	tr := p2TransportFunc(func(_ context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		calls.Add(1)
		return coreclient.AgentTelemetryAck{
			Status: "ACCEPTED", AcceptedThroughIngressSequence: b.LastIngressSequence,
		}, nil
	})
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 64; j++ {
				_ = d.Snapshot()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		if ok, err := d.RunOnce(context.Background()); !ok || err != nil {
			t.Errorf("ACK under concurrent Snapshot: ok=%v err=%v", ok, err)
		}
	}()
	close(start)
	wg.Wait()

	snap := d.Snapshot()
	if snap.Blocked || snap.State != DeliveryStateUp || snap.PendingRecords != 0 {
		t.Fatalf("concurrent Snapshot/ACK left stale degradation: %+v", snap)
	}
	if snap.DurableAcks != 1 || calls.Load() != 1 {
		t.Fatalf("acks=%d calls=%d", snap.DurableAcks, calls.Load())
	}
}

// Issue #230 repro: after Core 413 retainBlocked → DEGRADED, an exact-replay
// singleton ERR_MALFORMED_RECORD parks successfully but health stays DEGRADED.
func TestIssue230QuarantineClearsStaleDeliveryDegraded(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	var calls atomic.Uint64
	var firstBatchID string
	tr := p2TransportFunc(func(_ context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		n := calls.Add(1)
		if n == 1 {
			firstBatchID = b.BatchID
			return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_BATCH_TOO_LARGE", HTTPStatus: 413}
		}
		if b.BatchID != firstBatchID || len(b.Records) != 1 {
			t.Errorf("exact frozen replay broken: call=%d batchID=%q first=%q records=%d", n, b.BatchID, firstBatchID, len(b.Records))
		}
		return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_MALFORMED_RECORD", Message: "bad record", HTTPStatus: 400}
	})
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }

	if ok, err := d.RunOnce(context.Background()); ok || !coreclient.IsCoreAPICode(err, "ERR_BATCH_TOO_LARGE") {
		t.Fatalf("first 413: ok=%v err=%v", ok, err)
	}
	if snap := d.Snapshot(); !snap.Blocked || snap.State != DeliveryStateDegraded || snap.ReasonCode != "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("expected DEGRADED after 413: %+v", snap)
	}

	ok, parkErr := d.RunOnce(context.Background())
	if ok || parkErr == nil {
		t.Fatalf("singleton quarantine: ok=%v err=%v", ok, parkErr)
	}
	var de *Error
	if !errors.As(parkErr, &de) || de.Code != CodePoison {
		t.Fatalf("want poison park error, got %v", parkErr)
	}

	st, statsErr := s.Stats()
	if statsErr != nil {
		t.Fatal(statsErr)
	}
	if st.RecordCount != 0 {
		t.Fatalf("poisoned record must leave spool, count=%d", st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 1 {
		t.Fatalf("poison count=%d", d.Metrics().PoisonCount.Load())
	}
	if d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("quarantine must not count DurableAcks, got %d", d.Metrics().DurableAcks.Load())
	}

	snap := d.Snapshot()
	if snap.Blocked || snap.State != DeliveryStateUp || snap.ReasonCode != "" || snap.PendingRecords != 0 {
		t.Fatalf("Issue #230: delivery must recover after durable quarantine: %+v", snap)
	}
	if snap.DurableAcks != 0 {
		t.Fatalf("snapshot DurableAcks=%d", snap.DurableAcks)
	}
	for _, forbidden := range []string{"DEVICE_DOWN", "password", "Bearer"} {
		if strings.Contains(fmt.Sprintf("%+v", snap), forbidden) {
			t.Fatalf("leaked %q in %+v", forbidden, snap)
		}
	}
}

// ackFailSpool fails AcknowledgeThrough when armed so poison publish can succeed
// while spool removal fails closed (pre-commit: durable AckedThrough does not advance).
type ackFailSpool struct {
	Spool
	failAck bool
}

func (a *ackFailSpool) AcknowledgeThrough(seq uint64) error {
	if a.failAck {
		return errors.New("injected AcknowledgeThrough failure")
	}
	return a.Spool.AcknowledgeThrough(seq)
}

// partialAckSpool distinguishes pre-commit vs post-commit AcknowledgeThrough failures.
// Post-commit modes call the real spool first (watermark durable) then surface cleanup
// errors matching DiskSpool's post-persist failure classes — without changing spool.go.
type partialAckSpool struct {
	Spool
	dir            string
	mode           string // pre_commit | post_commit_remove | post_commit_fsync | post_commit_stats_uncertain
	ackedAttempted bool
	failStats      bool
}

func (p *partialAckSpool) AcknowledgeThrough(seq uint64) error {
	p.ackedAttempted = true
	switch p.mode {
	case "pre_commit":
		return errors.New("injected AcknowledgeThrough failure before durable state")
	case "post_commit_remove", "post_commit_fsync", "post_commit_stats_uncertain":
		var orphan []byte
		recordPath := filepath.Join(p.dir, fmt.Sprintf("record-%020d.json", seq))
		if p.mode == "post_commit_remove" {
			raw, err := os.ReadFile(recordPath)
			if err != nil {
				return err
			}
			orphan = raw
		}
		if err := p.Spool.AcknowledgeThrough(seq); err != nil {
			return err
		}
		if p.mode == "post_commit_remove" {
			if err := os.WriteFile(recordPath, orphan, 0640); err != nil {
				return err
			}
			return errors.New("acknowledged record metadata is durable but file cleanup failed")
		}
		if p.mode == "post_commit_stats_uncertain" {
			p.failStats = true
			return errors.New("injected post-commit cleanup failure with uncertain stats")
		}
		return errors.New("injected final spool directory fsync failure")
	default:
		return p.Spool.AcknowledgeThrough(seq)
	}
}

func (p *partialAckSpool) Stats() (spool.Stats, error) {
	if p.failStats {
		return spool.Stats{}, errors.New("injected Stats failure after partial acknowledgement")
	}
	return p.Spool.Stats()
}

func (p *partialAckSpool) ReconcileAckedCleanup() error {
	type reconciler interface {
		ReconcileAckedCleanup() error
	}
	if r, ok := p.Spool.(reconciler); ok {
		return r.ReconcileAckedCleanup()
	}
	return errors.New("ReconcileAckedCleanup unavailable on wrapped spool")
}

func (p *partialAckSpool) UnresolvedAckedCleanup() (bool, uint64, error) {
	type reporter interface {
		UnresolvedAckedCleanup() (bool, uint64, error)
	}
	if r, ok := p.Spool.(reporter); ok {
		return r.UnresolvedAckedCleanup()
	}
	return false, 0, nil
}

func TestIssue230PoisonFailuresNeverFalselyRecover(t *testing.T) {
	cases := []struct {
		name  string
		arm   func(*Drain)
		disarm func(*Drain)
	}{
		{
			name: "poison_dir_fsync",
			arm: func(d *Drain) {
				d.syncDir = func(string) error { return errors.New("injected poison dir fsync failure") }
			},
			disarm: func(d *Drain) { d.syncDir = nil },
		},
		{
			name: "poison_file_publish",
			arm: func(d *Drain) {
				d.poisonRename = func(string, string) error {
					return errors.New("injected poison rename failure")
				}
			},
			disarm: func(d *Drain) { d.poisonRename = nil },
		},
		{
			name: "acknowledge_through",
			arm: func(d *Drain) {
				d.spool = &ackFailSpool{Spool: d.spool, failAck: true}
			},
			disarm: func(d *Drain) {
				if w, ok := d.spool.(*ackFailSpool); ok {
					d.spool = w.Spool
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, dir := openSpool(t)
			appendNDurable(t, s, 1)
			calls := 0
			tr := p2TransportFunc(func(_ context.Context, _ coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
				calls++
				if calls == 1 {
					return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_BATCH_TOO_LARGE", HTTPStatus: 413}
				}
				return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_MALFORMED_RECORD", Message: "bad", HTTPStatus: 400}
			})
			d, err := New(Config{
				SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
				InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
			}, s, tr)
			if err != nil {
				t.Fatal(err)
			}
			d.wait = func(context.Context, time.Duration) error { return nil }
			if ok, err := d.RunOnce(context.Background()); ok || !coreclient.IsCoreAPICode(err, "ERR_BATCH_TOO_LARGE") {
				t.Fatalf("413 setup: ok=%v err=%v", ok, err)
			}
			if snap := d.Snapshot(); !snap.Blocked || snap.ReasonCode != "ERR_BATCH_TOO_LARGE" {
				t.Fatalf("precondition DEGRADED missing: %+v", snap)
			}
			tc.arm(d)
			ok, err := d.RunOnce(context.Background())
			tc.disarm(d)
			if ok || err == nil {
				t.Fatalf("poison failure must surface: ok=%v err=%v", ok, err)
			}
			st, _ := s.Stats()
			if st.RecordCount != 1 {
				t.Fatalf("failed poison must retain spool head, count=%d", st.RecordCount)
			}
			if d.Metrics().PoisonCount.Load() != 0 || d.Metrics().DurableAcks.Load() != 0 {
				t.Fatalf("failed poison must not advance counts: poison=%d acks=%d",
					d.Metrics().PoisonCount.Load(), d.Metrics().DurableAcks.Load())
			}
			snap := d.Snapshot()
			if !snap.Blocked || snap.State != DeliveryStateDegraded || snap.ReasonCode != "ERR_BATCH_TOO_LARGE" {
				t.Fatalf("poison failure must retain prior DEGRADED: %+v", snap)
			}
			if snap.PendingRecords != 1 {
				t.Fatalf("pending after failed poison=%d", snap.PendingRecords)
			}
		})
	}
}

func TestIssue230LocalMalformedQuarantineClearsPreexistingBlock(t *testing.T) {
	// Atypical but reachable: delivery already DEGRADED from a prior retainBlocked,
	// then a locally recognized malformed singleton is durably parked.
	s, dir := openSpool(t)
	payload := []byte(`{"tenantId":"incomplete"}`)
	rec, err := s.Append(spool.AppendRequest{Class: spool.ClassOperational, Type: agentBatchRecordType, Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		MaxBatchRecords: 1, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, &fakeTransport{})
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }
	d.markDeliveryBlocked("ERR_BATCH_TOO_LARGE", spool.Batch{FirstSequence: rec.Sequence, LastSequence: rec.Sequence})
	if snap := d.Snapshot(); !snap.Blocked || snap.ReasonCode != "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("preexisting block missing: %+v", snap)
	}

	ok, err := d.RunOnce(context.Background())
	var de *Error
	if ok || !errors.As(err, &de) || de.Code != CodePoison {
		t.Fatalf("local malformed park: ok=%v err=%v", ok, err)
	}
	if d.Metrics().PoisonCount.Load() != 1 || d.Metrics().DurableAcks.Load() != 0 || d.Metrics().Attempts.Load() != 0 {
		t.Fatalf("local quarantine counters: poison=%d acks=%d attempts=%d",
			d.Metrics().PoisonCount.Load(), d.Metrics().DurableAcks.Load(), d.Metrics().Attempts.Load())
	}
	snap := d.Snapshot()
	if snap.Blocked || snap.State != DeliveryStateUp || snap.ReasonCode != "" || snap.PendingRecords != 0 {
		t.Fatalf("local quarantine must clear obsolete block: %+v", snap)
	}
}

func TestIssue230HeadOnlyQuarantineRetainsReplayableTail(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 2)
	calls := 0
	var firstHeadSeq uint64
	tr := p2TransportFunc(func(_ context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		calls++
		switch calls {
		case 1:
			firstHeadSeq = b.FirstIngressSequence
			return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_BATCH_TOO_LARGE", HTTPStatus: 413}
		case 2:
			if b.FirstIngressSequence != firstHeadSeq || len(b.Records) != 1 {
				t.Errorf("quarantine attempt identity drift: %+v", b)
			}
			return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_MALFORMED_RECORD", Message: "bad", HTTPStatus: 400}
		default:
			return coreclient.AgentTelemetryAck{
				Status: "ACCEPTED", AcceptedThroughIngressSequence: b.LastIngressSequence,
			}, nil
		}
	})
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		MaxBatchRecords: 1, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }

	if ok, err := d.RunOnce(context.Background()); ok || !coreclient.IsCoreAPICode(err, "ERR_BATCH_TOO_LARGE") {
		t.Fatalf("413: ok=%v err=%v", ok, err)
	}
	ok, err := d.RunOnce(context.Background())
	var de *Error
	if ok || !errors.As(err, &de) || de.Code != CodePoison {
		t.Fatalf("head quarantine: ok=%v err=%v", ok, err)
	}
	if d.Metrics().PoisonCount.Load() != 1 || d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("poison=%d acks=%d", d.Metrics().PoisonCount.Load(), d.Metrics().DurableAcks.Load())
	}
	snap := d.Snapshot()
	if snap.Blocked || snap.State != DeliveryStateUp || snap.ReasonCode != "" || snap.PendingRecords != 1 {
		t.Fatalf("after head quarantine health=%+v", snap)
	}
	st, _ := s.Stats()
	if st.RecordCount != 1 {
		t.Fatalf("tail must remain, count=%d", st.RecordCount)
	}
	if ok, err := d.RunOnce(context.Background()); !ok || err != nil {
		t.Fatalf("tail replay ACK: ok=%v err=%v", ok, err)
	}
	final := d.Snapshot()
	if final.Blocked || final.PendingRecords != 0 || final.DurableAcks != 1 {
		t.Fatalf("tail ACK health=%+v", final)
	}
}

func TestIssue230StaleLegacyObservationCannotReassertAfterQuarantine(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 2)
	seedLegacySizeRejectedSidecar(t, s, dir)

	tr := p2TransportFunc(func(_ context.Context, _ coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_MALFORMED_RECORD", Message: "bad", HTTPStatus: 400}
	})
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		MaxBatchRecords: 1, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }

	observed := make(chan struct{})
	release := make(chan struct{})
	d.afterLegacySizeObserve = func() {
		close(observed)
		<-release
	}

	var snap DeliveryStatus
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		snap = d.Snapshot()
	}()
	select {
	case <-observed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for legacy size observation barrier")
	}

	ok, err := d.RunOnce(context.Background())
	var de *Error
	if ok || !errors.As(err, &de) || de.Code != CodePoison {
		close(release)
		wg.Wait()
		t.Fatalf("quarantine during stale observation: ok=%v err=%v", ok, err)
	}
	close(release)
	wg.Wait()

	if snap.Blocked || snap.State != DeliveryStateUp {
		t.Fatalf("stale legacy observation reasserted after quarantine: %+v", snap)
	}
	if d.Metrics().PoisonCount.Load() != 1 || d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("poison=%d acks=%d", d.Metrics().PoisonCount.Load(), d.Metrics().DurableAcks.Load())
	}
	final := d.Snapshot()
	if final.Blocked || final.State != DeliveryStateUp || final.PendingRecords != 1 {
		t.Fatalf("post-quarantine health=%+v", final)
	}
}

func TestIssue230ConcurrentSnapshotAndQuarantineRace(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	seedLegacySizeRejectedSidecar(t, s, dir)

	var calls atomic.Uint64
	tr := p2TransportFunc(func(_ context.Context, _ coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		calls.Add(1)
		return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_MALFORMED_RECORD", Message: "bad", HTTPStatus: 400}
	})
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 64; j++ {
				_ = d.Snapshot()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		ok, err := d.RunOnce(context.Background())
		var de *Error
		if ok || !errors.As(err, &de) || de.Code != CodePoison {
			t.Errorf("quarantine under concurrent Snapshot: ok=%v err=%v", ok, err)
		}
	}()
	close(start)
	wg.Wait()

	snap := d.Snapshot()
	if snap.Blocked || snap.State != DeliveryStateUp || snap.PendingRecords != 0 {
		t.Fatalf("concurrent Snapshot/quarantine left stale degradation: %+v", snap)
	}
	if d.Metrics().PoisonCount.Load() != 1 || snap.DurableAcks != 0 || calls.Load() != 1 {
		t.Fatalf("poison=%d acks=%d calls=%d", d.Metrics().PoisonCount.Load(), snap.DurableAcks, calls.Load())
	}
}

// setupIssue230PartialAckDrain prepares DEGRADED-via-413 then arms a partialAckSpool
// for the subsequent singleton poison park.
func setupIssue230PartialAckDrain(t *testing.T, records int, mode string) (*Drain, *spool.DiskSpool, string, *partialAckSpool) {
	t.Helper()
	s, dir := openSpool(t)
	appendNDurable(t, s, records)
	calls := 0
	var firstBatchID string
	tr := p2TransportFunc(func(_ context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		calls++
		if calls == 1 {
			firstBatchID = b.BatchID
			return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_BATCH_TOO_LARGE", HTTPStatus: 413}
		}
		if b.BatchID != firstBatchID || len(b.Records) != 1 {
			t.Errorf("exact frozen replay identity broken: call=%d batchID=%q first=%q records=%d",
				calls, b.BatchID, firstBatchID, len(b.Records))
		}
		return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_MALFORMED_RECORD", Message: "bad", HTTPStatus: 400}
	})
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		MaxBatchRecords: 1, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }
	if ok, err := d.RunOnce(context.Background()); ok || !coreclient.IsCoreAPICode(err, "ERR_BATCH_TOO_LARGE") {
		t.Fatalf("413 setup: ok=%v err=%v", ok, err)
	}
	if snap := d.Snapshot(); !snap.Blocked || snap.ReasonCode != "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("precondition DEGRADED missing: %+v", snap)
	}
	wrap := &partialAckSpool{Spool: s, dir: dir, mode: mode}
	d.spool = wrap
	return d, s, dir, wrap
}

func TestIssue230PartialAckPreCommitRetainsDegraded(t *testing.T) {
	d, s, _, _ := setupIssue230PartialAckDrain(t, 1, "pre_commit")
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("pre-commit ack failure must surface: ok=%v err=%v", ok, err)
	}
	var de *Error
	if !errors.As(err, &de) || de.Code != CodeSpool {
		t.Fatalf("want CodeSpool pre-commit failure, got %v", err)
	}
	st, _ := s.Stats()
	if st.AckedThrough != 0 || st.RecordCount != 1 {
		t.Fatalf("pre-commit must not advance watermark: acked=%d count=%d", st.AckedThrough, st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 0 || d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("pre-commit must not advance poison/acks: poison=%d acks=%d",
			d.Metrics().PoisonCount.Load(), d.Metrics().DurableAcks.Load())
	}
	snap := d.Snapshot()
	if !snap.Blocked || snap.State != DeliveryStateDegraded || snap.ReasonCode != "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("pre-commit must retain DEGRADED: %+v", snap)
	}
}

func TestIssue230PartialAckPostCommitRemoveClearsObsoleteBlock(t *testing.T) {
	d, s, dir, wrap := setupIssue230PartialAckDrain(t, 1, "post_commit_remove")
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("post-commit remove failure must surface: ok=%v err=%v", ok, err)
	}
	var de *Error
	if !errors.As(err, &de) || de.Code != CodeSpool {
		t.Fatalf("want CodeSpool post-commit failure, got %v", err)
	}
	if !strings.Contains(de.msg, "watermark durable but spool cleanup failed") {
		t.Fatalf("want partial-ack message, got %q", de.msg)
	}
	st, statsErr := s.Stats()
	if statsErr != nil {
		t.Fatal(statsErr)
	}
	if st.AckedThrough < 1 {
		t.Fatalf("post-commit remove must leave durable AckedThrough advanced, got %d", st.AckedThrough)
	}
	if st.RecordCount != 0 {
		t.Fatalf("logical pending must exclude acked orphan, count=%d", st.RecordCount)
	}
	orphan := filepath.Join(dir, fmt.Sprintf("record-%020d.json", uint64(1)))
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("orphan record file must remain after injected remove failure: %v", err)
	}
	poisonEntries, _ := os.ReadDir(filepath.Join(dir, poisonDirName))
	if len(poisonEntries) == 0 {
		t.Fatal("poison quarantine file must be preserved")
	}
	if d.Metrics().PoisonCount.Load() != 1 || d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("poison=%d DurableAcks=%d (no false Core ACK)", d.Metrics().PoisonCount.Load(), d.Metrics().DurableAcks.Load())
	}
	snap := d.Snapshot()
	assertCleanupFaultDegraded(t, snap, 0)
	if snap.ReasonCode == "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("obsolete ERR_BATCH_TOO_LARGE must not remain: %+v", snap)
	}
	if snap.DurableAcks != 0 {
		t.Fatalf("snapshot must not report Core ACK: %+v", snap)
	}
	if !wrap.ackedAttempted {
		t.Fatal("AcknowledgeThrough was not attempted")
	}
	for _, forbidden := range []string{"DEVICE_DOWN", "password", "Bearer"} {
		if strings.Contains(fmt.Sprintf("%+v", snap), forbidden) {
			t.Fatalf("leaked %q in %+v", forbidden, snap)
		}
	}
}

func TestIssue230PartialAckPostCommitFsyncClearsObsoleteBlock(t *testing.T) {
	d, s, _, _ := setupIssue230PartialAckDrain(t, 1, "post_commit_fsync")
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("post-commit fsync failure must surface: ok=%v err=%v", ok, err)
	}
	var de *Error
	if !errors.As(err, &de) || de.Code != CodeSpool || !strings.Contains(de.msg, "watermark durable") {
		t.Fatalf("want partial-ack CodeSpool, got %v", err)
	}
	st, _ := s.Stats()
	if st.AckedThrough < 1 || st.RecordCount != 0 {
		t.Fatalf("fsync post-commit state acked=%d count=%d", st.AckedThrough, st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 1 || d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("poison=%d acks=%d", d.Metrics().PoisonCount.Load(), d.Metrics().DurableAcks.Load())
	}
	snap := d.Snapshot()
	assertCleanupFaultDegraded(t, snap, 0)
	if snap.ReasonCode == "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("obsolete delivery block must clear while cleanup fault remains: %+v", snap)
	}
}

func TestIssue230PartialAckUncertainStatsRetainsDegraded(t *testing.T) {
	d, s, _, _ := setupIssue230PartialAckDrain(t, 1, "post_commit_stats_uncertain")
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("uncertain inspect must still surface spool error: ok=%v err=%v", ok, err)
	}
	var de *Error
	if !errors.As(err, &de) || de.Code != CodeSpool {
		t.Fatalf("want CodeSpool, got %v", err)
	}
	if strings.Contains(de.msg, "watermark durable but spool cleanup failed") {
		t.Fatalf("uncertain Stats must not claim reconciled recovery: %q", de.msg)
	}
	// Underlying watermark may be durable, but health must not clear without proof.
	if acked, err := s.AckedThrough(); err != nil || acked < 1 {
		t.Fatalf("fixture expected durable watermark underneath uncertain inspect: acked=%d err=%v", acked, err)
	}
	if d.Metrics().PoisonCount.Load() != 0 || d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("uncertain path must not count poison recovery: poison=%d acks=%d",
			d.Metrics().PoisonCount.Load(), d.Metrics().DurableAcks.Load())
	}
	snap := d.Snapshot()
	if !snap.Blocked || snap.State != DeliveryStateDegraded || snap.ReasonCode != "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("uncertain persistence must retain DEGRADED: %+v", snap)
	}
}

func TestIssue230PartialAckPostCommitWithReplayableTail(t *testing.T) {
	d, s, _, _ := setupIssue230PartialAckDrain(t, 2, "post_commit_fsync")
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("partial park must surface cleanup error: ok=%v err=%v", ok, err)
	}
	st, _ := s.Stats()
	if st.RecordCount != 1 || st.AckedThrough < 1 {
		t.Fatalf("tail must remain pending under advanced watermark: acked=%d count=%d", st.AckedThrough, st.RecordCount)
	}
	if d.Metrics().PoisonCount.Load() != 1 || d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("poison=%d acks=%d", d.Metrics().PoisonCount.Load(), d.Metrics().DurableAcks.Load())
	}
	snap := d.Snapshot()
	assertCleanupFaultDegraded(t, snap, 1)
	if snap.ReasonCode == "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("obsolete 413 must clear while cleanup fault and tail remain: %+v", snap)
	}
	// Disarm wrapper so tail Core ACK uses real spool acknowledgement.
	d.spool = s
	d.transport = p2TransportFunc(func(_ context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		return coreclient.AgentTelemetryAck{
			Status: "ACCEPTED", AcceptedThroughIngressSequence: b.LastIngressSequence,
		}, nil
	})
	if ok, err := d.RunOnce(context.Background()); !ok || err != nil {
		t.Fatalf("tail replay ACK: ok=%v err=%v", ok, err)
	}
	final := d.Snapshot()
	// Recovery runs at RunOnce start (orphan cleanup + dir sync); tail ACK must not
	// be the sole reason health recovered.
	if final.Blocked || final.State != DeliveryStateUp || final.ReasonCode != "" || final.PendingRecords != 0 || final.DurableAcks != 1 {
		t.Fatalf("tail ACK health=%+v", final)
	}
}

func TestIssue230PartialAckRestartAfterPostCommit(t *testing.T) {
	d, s, dir, _ := setupIssue230PartialAckDrain(t, 1, "post_commit_fsync")
	streamID := s.StreamID()
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("partial park: ok=%v err=%v", ok, err)
	}
	acked, err := s.AckedThrough()
	if err != nil || acked < 1 {
		t.Fatalf("durable watermark before restart: acked=%d err=%v", acked, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	persisted, err := reopened.AckedThrough()
	if err != nil || persisted != acked {
		t.Fatalf("restart must preserve AckedThrough: got %d want %d err=%v", persisted, acked, err)
	}
	st, err := reopened.Stats()
	if err != nil || st.RecordCount != 0 {
		t.Fatalf("restart logical backlog: %+v err=%v", st, err)
	}
	d2, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: streamID, AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, reopened, &fakeTransport{})
	if err != nil {
		t.Fatal(err)
	}
	snap := d2.Snapshot()
	if snap.Blocked || snap.ReasonCode == "ERR_BATCH_TOO_LARGE" || snap.PendingRecords != 0 || snap.DurableAcks != 0 {
		t.Fatalf("restart after fsync partial ack must not resurrect delivery block or invent Core ACK: %+v", snap)
	}
	// No orphaned record files after successful remove+injected fsync: cleanup fault
	// is process-local and correctly absent after restart.
	if snap.State != DeliveryStateUp || snap.ReasonCode != "" {
		t.Fatalf("fsync-class restart without orphans must be UP: %+v", snap)
	}
	poisonEntries, _ := os.ReadDir(filepath.Join(dir, poisonDirName))
	if len(poisonEntries) == 0 {
		t.Fatal("poison files must survive restart")
	}
}

func TestIssue230PartialAckConcurrentSnapshotAndQuarantine(t *testing.T) {
	d, s, _, _ := setupIssue230PartialAckDrain(t, 1, "post_commit_fsync")
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 64; j++ {
				_ = d.Snapshot()
			}
		}()
	}
	var parkErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		var ok bool
		ok, parkErr = d.RunOnce(context.Background())
		if ok || parkErr == nil {
			t.Errorf("partial quarantine must return cleanup error: ok=%v err=%v", ok, parkErr)
		}
	}()
	close(start)
	wg.Wait()

	var de *Error
	if !errors.As(parkErr, &de) || de.Code != CodeSpool {
		t.Fatalf("want CodeSpool partial ack, got %v", parkErr)
	}
	st, _ := s.Stats()
	if st.AckedThrough < 1 || st.RecordCount != 0 {
		t.Fatalf("concurrent partial ack state acked=%d count=%d", st.AckedThrough, st.RecordCount)
	}
	snap := d.Snapshot()
	assertCleanupFaultDegraded(t, snap, 0)
	if snap.DurableAcks != 0 || snap.ReasonCode == "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("concurrent Snapshot/partial-quarantine health=%+v", snap)
	}
	if d.Metrics().PoisonCount.Load() != 1 {
		t.Fatalf("poison=%d", d.Metrics().PoisonCount.Load())
	}
}

func TestIssue230PartialAckLegacySizeRejectedObservation(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	seedLegacySizeRejectedSidecar(t, s, dir)
	tr := p2TransportFunc(func(_ context.Context, _ coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		return coreclient.AgentTelemetryAck{}, &coreclient.CoreAPIError{Code: "ERR_MALFORMED_RECORD", Message: "bad", HTTPStatus: 400}
	})
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }
	if snap := d.Snapshot(); !snap.Blocked || snap.ReasonCode != "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("legacy sizeRejected must observe DEGRADED: %+v", snap)
	}
	wrap := &partialAckSpool{Spool: s, dir: dir, mode: "post_commit_fsync"}
	d.spool = wrap
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("partial park: ok=%v err=%v", ok, err)
	}
	if d.Metrics().PoisonCount.Load() != 1 || d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("poison=%d acks=%d", d.Metrics().PoisonCount.Load(), d.Metrics().DurableAcks.Load())
	}
	snap := d.Snapshot()
	assertCleanupFaultDegraded(t, snap, 0)
	if snap.ReasonCode == "ERR_BATCH_TOO_LARGE" {
		t.Fatalf("legacy delivery block must clear; cleanup fault must remain: %+v", snap)
	}
}

func TestIssue230PartialAckEmptyLogicalBacklogNoFalseCoreAck(t *testing.T) {
	d, s, _, _ := setupIssue230PartialAckDrain(t, 1, "post_commit_remove")
	ok, err := d.RunOnce(context.Background())
	if ok {
		t.Fatal("partial acknowledgement must not report RunOnce success (false Core ACK)")
	}
	if err == nil {
		t.Fatal("storage cleanup failure must remain visible")
	}
	if d.Metrics().DurableAcks.Load() != 0 || d.Metrics().Successes.Load() != 0 {
		t.Fatalf("no false Core ACK counters: DurableAcks=%d Successes=%d",
			d.Metrics().DurableAcks.Load(), d.Metrics().Successes.Load())
	}
	st, _ := s.Stats()
	if st.RecordCount != 0 || st.AckedThrough < 1 {
		t.Fatalf("empty logical backlog expected: %+v", st)
	}
	snap := d.Snapshot()
	assertCleanupFaultDegraded(t, snap, 0)
	if snap.DurableAcks != 0 {
		t.Fatalf("empty backlog must not invent Core ACK: %+v", snap)
	}
}

func assertCleanupFaultDegraded(t *testing.T, snap DeliveryStatus, pending int64) {
	t.Helper()
	if snap.State != DeliveryStateDegraded || snap.ReasonCode != reasonSpoolCleanupFailed {
		t.Fatalf("want cleanup DEGRADED/%s, got %+v", reasonSpoolCleanupFailed, snap)
	}
	if snap.Blocked {
		t.Fatalf("cleanup fault must not claim Core delivery Blocked: %+v", snap)
	}
	if snap.PendingRecords != pending {
		t.Fatalf("pending=%d want %d in %+v", snap.PendingRecords, pending, snap)
	}
	for _, forbidden := range []string{"DEVICE_DOWN", "password", "Bearer"} {
		if strings.Contains(fmt.Sprintf("%+v", snap), forbidden) {
			t.Fatalf("leaked %q in %+v", forbidden, snap)
		}
	}
}

func TestIssue230CleanupFaultSurvivesUnrelatedUpload(t *testing.T) {
	d, s, dir, _ := setupIssue230PartialAckDrain(t, 1, "post_commit_remove")
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("partial park: ok=%v err=%v", ok, err)
	}
	assertCleanupFaultDegraded(t, d.Snapshot(), 0)
	orphan := filepath.Join(dir, fmt.Sprintf("record-%020d.json", uint64(1)))
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("orphan required for this case: %v", err)
	}

	// Block recovery fsync so tryRecoverCleanupFault cannot clear the fault
	// even if orphan removal already succeeded.
	d.syncDir = func(path string) error {
		if path == d.cfg.AgentSpoolDir {
			return errors.New("injected spool-dir recovery fsync failure")
		}
		return defaultSyncDir(path)
	}
	d.spool = s
	appendNDurable(t, s, 1)
	d.transport = p2TransportFunc(func(_ context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		return coreclient.AgentTelemetryAck{
			Status: "ACCEPTED", AcceptedThroughIngressSequence: b.LastIngressSequence,
		}, nil
	})
	if ok, err := d.RunOnce(context.Background()); !ok || err != nil {
		t.Fatalf("unrelated upload ACK: ok=%v err=%v", ok, err)
	}
	if d.Metrics().DurableAcks.Load() != 1 {
		t.Fatalf("DurableAcks=%d", d.Metrics().DurableAcks.Load())
	}
	// Empty backlog + successful unrelated upload must not clear cleanup fault.
	snap := d.Snapshot()
	assertCleanupFaultDegraded(t, snap, 0)

	// Proven recovery: directory sync succeeds (orphans already removable/absent).
	d.syncDir = nil
	if ok, err := d.RunOnce(context.Background()); ok || err != nil {
		t.Fatalf("idle recovery RunOnce: ok=%v err=%v", ok, err)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("recovery must leave no acked orphan, err=%v", err)
	}
	final := d.Snapshot()
	if final.Blocked || final.State != DeliveryStateUp || final.ReasonCode != "" || final.PendingRecords != 0 {
		t.Fatalf("cleanup recovery must clear fault: %+v", final)
	}
}

func TestIssue230CleanupFaultRestartOpenRecoversOrphan(t *testing.T) {
	d, s, dir, _ := setupIssue230PartialAckDrain(t, 1, "post_commit_remove")
	streamID := s.StreamID()
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("partial park: ok=%v err=%v", ok, err)
	}
	assertCleanupFaultDegraded(t, d.Snapshot(), 0)
	acked, err := s.AckedThrough()
	if err != nil || acked < 1 {
		t.Fatalf("watermark: acked=%d err=%v", acked, err)
	}
	orphan := filepath.Join(dir, fmt.Sprintf("record-%020d.json", uint64(1)))
	if _, err := os.Stat(orphan); err != nil {
		t.Fatalf("orphan must exist before restart: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	// DiskSpool.recover removes validated acked orphans on Open when removal succeeds.
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("spool recover should remove acked orphan when possible, err=%v", err)
	}
	d2, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: streamID, AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, reopened, &fakeTransport{})
	if err != nil {
		t.Fatal(err)
	}
	healed := d2.Snapshot()
	if healed.Blocked || healed.State != DeliveryStateUp || healed.ReasonCode != "" || healed.DurableAcks != 0 {
		t.Fatalf("restart after successful spool orphan recover must be UP: %+v", healed)
	}
}

func TestIssue230SnapshotDoesNotScanPlantedAckedOrphans(t *testing.T) {
	s, dir := openSpool(t)
	r := appendDurable(t, s, 1)
	recordPath := filepath.Join(dir, fmt.Sprintf("record-%020d.json", r.Sequence))
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AcknowledgeThrough(r.Sequence); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordPath, raw, 0640); err != nil {
		t.Fatal(err)
	}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, s, &fakeTransport{})
	if err != nil {
		t.Fatal(err)
	}
	if snap := d.Snapshot(); snap.State != DeliveryStateUp || snap.ReasonCode != "" || snap.DurableAcks != 0 {
		t.Fatalf("Snapshot must not directory-scan planted orphans into DEGRADED or invent ACK: %+v", snap)
	}
	if _, err := os.Stat(recordPath); err != nil {
		t.Fatalf("Snapshot must not delete planted orphan: %v", err)
	}
}

func TestIssue230UnresolvedCleanupReporterReassertsCheaply(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	wrap := &unresolvedCleanupSpool{Spool: s, unresolved: true, seq: 1}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, wrap, &fakeTransport{})
	if err != nil {
		t.Fatal(err)
	}
	snap := d.Snapshot()
	assertCleanupFaultDegraded(t, snap, 1)
	if snap.DurableAcks != 0 {
		t.Fatalf("unresolved cleanup must not invent Core ACK: %+v", snap)
	}
	for _, forbidden := range []string{"DEVICE_DOWN", "password", "Bearer"} {
		if strings.Contains(fmt.Sprintf("%+v", snap), forbidden) {
			t.Fatalf("leaked %q in %+v", forbidden, snap)
		}
	}
}

func TestIssue230CleanupReconcilePreservesPendingAndQuarantinesUnsafe(t *testing.T) {
	d, s, dir, _ := setupIssue230PartialAckDrain(t, 1, "post_commit_remove")
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("partial park: ok=%v err=%v", ok, err)
	}
	assertCleanupFaultDegraded(t, d.Snapshot(), 0)
	ackedOrphan := filepath.Join(dir, fmt.Sprintf("record-%020d.json", uint64(1)))
	if _, err := os.Stat(ackedOrphan); err != nil {
		t.Fatalf("acked orphan required: %v", err)
	}
	pending := appendDurable(t, s, 2)
	pendingPath := filepath.Join(dir, fmt.Sprintf("record-%020d.json", pending.Sequence))
	pendingRaw, err := os.ReadFile(pendingPath)
	if err != nil {
		t.Fatalf("pending telemetry required: %v", err)
	}

	zeroPath := filepath.Join(dir, fmt.Sprintf("record-%020d.json", uint64(0)))
	if err := os.WriteFile(zeroPath, []byte(`{"not":"a-record"}`), 0640); err != nil {
		t.Fatal(err)
	}
	badName := filepath.Join(dir, "record-not-a-sequence.json")
	if err := os.WriteFile(badName, []byte("garbage"), 0640); err != nil {
		t.Fatal(err)
	}
	// Symlink must not use a pending-looking record sequence name: Stats() would
	// fail-closed on unsafe pending paths before cleanup recovery can run.
	symlinkPath := filepath.Join(dir, "unsafe-acked-looking-link")
	target := filepath.Join(dir, "outside-target")
	if err := os.WriteFile(target, []byte("secret"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, symlinkPath); err != nil {
		t.Fatal(err)
	}

	d.spool = s
	// Recover cleanup without selecting/uploading pending work.
	d.tryRecoverCleanupFault()
	if _, err := os.Stat(ackedOrphan); !os.IsNotExist(err) {
		t.Fatalf("validated acked orphan must be removed by DiskSpool, err=%v", err)
	}
	if _, err := os.Stat(zeroPath); !os.IsNotExist(err) {
		t.Fatalf("sequence-zero artifact must be quarantined, err=%v", err)
	}
	if _, err := os.Stat(badName); !os.IsNotExist(err) {
		t.Fatalf("invalid-name artifact must be quarantined, err=%v", err)
	}
	if _, err := os.Lstat(symlinkPath); !os.IsNotExist(err) {
		t.Fatalf("symlink artifact must be quarantined, err=%v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink target must not be deleted: %v", err)
	}
	gotPending, err := os.ReadFile(pendingPath)
	if err != nil || !reflect.DeepEqual(gotPending, pendingRaw) {
		t.Fatalf("pending valid telemetry must remain intact")
	}
	qEntries, err := os.ReadDir(filepath.Join(dir, "quarantine"))
	if err != nil || len(qEntries) < 3 {
		t.Fatalf("expected quarantined unsafe artifacts, got %d err=%v", len(qEntries), err)
	}
	final := d.Snapshot()
	if final.Blocked || final.State != DeliveryStateUp || final.ReasonCode != "" || final.PendingRecords != 1 || final.DurableAcks != 0 {
		t.Fatalf("after safe reconcile health=%+v", final)
	}
}

type unresolvedCleanupSpool struct {
	Spool
	unresolved bool
	seq        uint64
}

func (u *unresolvedCleanupSpool) UnresolvedAckedCleanup() (bool, uint64, error) {
	return u.unresolved, u.seq, nil
}

func (u *unresolvedCleanupSpool) ReconcileAckedCleanup() error {
	type reconciler interface {
		ReconcileAckedCleanup() error
	}
	if r, ok := u.Spool.(reconciler); ok {
		return r.ReconcileAckedCleanup()
	}
	return errors.New("ReconcileAckedCleanup unavailable")
}

type countingUnresolvedSpool struct {
	Spool
	unresolved      bool
	seq             uint64
	unresolvedCalls atomic.Uint64
	reconcileCalls  atomic.Uint64
}

func (u *countingUnresolvedSpool) UnresolvedAckedCleanup() (bool, uint64, error) {
	u.unresolvedCalls.Add(1)
	return u.unresolved, u.seq, nil
}

func (u *countingUnresolvedSpool) ReconcileAckedCleanup() error {
	u.reconcileCalls.Add(1)
	type reconciler interface {
		ReconcileAckedCleanup() error
	}
	if r, ok := u.Spool.(reconciler); ok {
		return r.ReconcileAckedCleanup()
	}
	return errors.New("ReconcileAckedCleanup unavailable")
}

func TestIssue230SnapshotCleanupObservationIsO1NotDirectoryScan(t *testing.T) {
	s, dir := openSpool(t)
	const backlog = 200
	appendNDurable(t, s, backlog)
	wrap := &countingUnresolvedSpool{Spool: s, unresolved: true, seq: 1}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, wrap, &fakeTransport{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		snap := d.Snapshot()
		assertCleanupFaultDegraded(t, snap, int64(backlog))
		if snap.DurableAcks != 0 || strings.Contains(fmt.Sprintf("%+v", snap), "DEVICE_DOWN") {
			t.Fatalf("health must stay truthful under backlog: %+v", snap)
		}
	}
	if wrap.reconcileCalls.Load() != 0 {
		t.Fatalf("Snapshot must not invoke ReconcileAckedCleanup (got %d)", wrap.reconcileCalls.Load())
	}
	if wrap.unresolvedCalls.Load() < 1 {
		t.Fatal("Snapshot must use O(1) UnresolvedAckedCleanup reporter")
	}
	st, err := s.Stats()
	if err != nil || st.RecordCount != backlog {
		t.Fatalf("pending backlog must remain intact: %+v err=%v", st, err)
	}
}

func TestIssue230CleanupFaultConcurrentSnapshotAndSpoolOps(t *testing.T) {
	d, s, _, _ := setupIssue230PartialAckDrain(t, 1, "post_commit_remove")
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("partial park: ok=%v err=%v", ok, err)
	}
	d.spool = s
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 40; j++ {
				_ = d.Snapshot()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for j := 0; j < 8; j++ {
			_, _ = s.Stats()
			_ = s.ReconcileAckedCleanup()
		}
	}()
	close(start)
	wg.Wait()
	if ok, err := d.RunOnce(context.Background()); ok || err != nil {
		t.Fatalf("recovery after concurrent ops: ok=%v err=%v", ok, err)
	}
	final := d.Snapshot()
	if final.Blocked || final.State != DeliveryStateUp || final.ReasonCode != "" || final.DurableAcks != 0 {
		t.Fatalf("concurrent cleanup recovery health=%+v", final)
	}
}

func TestIssue230CleanupFaultClearsAfterFsyncRecovery(t *testing.T) {
	d, s, _, _ := setupIssue230PartialAckDrain(t, 1, "post_commit_fsync")
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("partial park: ok=%v err=%v", ok, err)
	}
	assertCleanupFaultDegraded(t, d.Snapshot(), 0)
	d.spool = s
	// Empty backlog: recovery is orphan cleanup (none) + spool dir sync only.
	if ok, err := d.RunOnce(context.Background()); ok || err != nil {
		t.Fatalf("recovery RunOnce: ok=%v err=%v", ok, err)
	}
	final := d.Snapshot()
	if final.Blocked || final.State != DeliveryStateUp || final.ReasonCode != "" {
		t.Fatalf("fsync recovery must clear cleanup fault: %+v", final)
	}
	if d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("recovery must not invent Core ACK: %d", d.Metrics().DurableAcks.Load())
	}
}

func TestIssue230CleanupFaultNotClearedBySnapshotAlone(t *testing.T) {
	d, _, _, _ := setupIssue230PartialAckDrain(t, 1, "post_commit_fsync")
	ok, err := d.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("partial park: ok=%v err=%v", ok, err)
	}
	for i := 0; i < 8; i++ {
		assertCleanupFaultDegraded(t, d.Snapshot(), 0)
	}
}

// TestIssue230StaleCleanupObservationCannotReassertAfterClear proves the P2-1
// race: Snapshot observes unresolved cleanup, concurrent recovery clears the
// fault (cleanupGen advances), then the stale observer must not reassert
// DEGRADED.
func TestIssue230StaleCleanupObservationCannotReassertAfterClear(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	wrap := &unresolvedCleanupSpool{Spool: s, unresolved: true, seq: 1}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, wrap, &fakeTransport{})
	if err != nil {
		t.Fatal(err)
	}

	observed := make(chan struct{})
	release := make(chan struct{})
	d.afterCleanupUnresolvedObserve = func() {
		close(observed)
		<-release
	}

	var snap DeliveryStatus
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		snap = d.Snapshot()
	}()

	select {
	case <-observed:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for cleanup unresolved observation barrier")
	}

	// Concurrent recovery: spool flag cleared and drain generation advanced.
	wrap.unresolved = false
	d.clearCleanupFault()
	close(release)
	wg.Wait()

	if snap.State != DeliveryStateUp || snap.ReasonCode != "" || snap.Blocked {
		t.Fatalf("stale cleanup observation reasserted DEGRADED: %+v", snap)
	}
	final := d.Snapshot()
	if final.State != DeliveryStateUp || final.ReasonCode != "" || final.Blocked {
		t.Fatalf("post-clear healthy state lost: %+v", final)
	}
}

// TestIssue230CleanupObservationStillAssertsTrueFault ensures generation fencing
// does not suppress a live unresolved-cleanup signal after a prior clear.
func TestIssue230CleanupObservationStillAssertsTrueFault(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 1)
	wrap := &unresolvedCleanupSpool{Spool: s, unresolved: false, seq: 0}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	}, wrap, &fakeTransport{})
	if err != nil {
		t.Fatal(err)
	}
	d.markCleanupFault(1)
	d.clearCleanupFault()
	wrap.unresolved = true
	wrap.seq = 2
	assertCleanupFaultDegraded(t, d.Snapshot(), 1)
}

// TestIssue230CleanupRecoveryBackoffPacesExpensiveRetries proves P2-2: under
// persistent reconcile failure, RunOnce does not rescan every IdleInterval.
// Skipped attempts leave the fault set and must not invent Core ACKs.
func TestIssue230CleanupRecoveryBackoffPacesExpensiveRetries(t *testing.T) {
	s, dir := openSpool(t)
	wrap := &failingReconcileSpool{Spool: s, failErr: errors.New("injected reconcile failure")}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		InitialBackoff: time.Hour, MaxBackoff: time.Hour, JitterFraction: 0,
		IdleInterval: time.Millisecond,
	}, wrap, &fakeTransport{})
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }
	d.jitter = func(base time.Duration, _ float64) time.Duration { return base }
	// faultSeq 0 is covered by an empty AckedThrough watermark so recovery
	// reaches the expensive reconcile path without a prior partial ACK.
	d.markCleanupFault(0)

	const iterations = 25
	for i := 0; i < iterations; i++ {
		ok, err := d.RunOnce(context.Background())
		if ok || err != nil {
			t.Fatalf("iteration %d: ok=%v err=%v", i, ok, err)
		}
	}
	calls := wrap.reconcileCalls.Load()
	if calls != 1 {
		t.Fatalf("expensive reconcile paced calls=%d want 1 across %d RunOnce", calls, iterations)
	}
	assertCleanupFaultDegraded(t, d.Snapshot(), 0)
	if d.Metrics().DurableAcks.Load() != 0 {
		t.Fatalf("paced recovery must not invent Core ACK: %d", d.Metrics().DurableAcks.Load())
	}

	// Elapse the paced window: next RunOnce must attempt again, still fail, still faulted.
	d.deliveryMu.Lock()
	d.cleanupRetryAfter = time.Time{}
	d.deliveryMu.Unlock()
	if ok, err := d.RunOnce(context.Background()); ok || err != nil {
		t.Fatalf("retry after pace: ok=%v err=%v", ok, err)
	}
	if got := wrap.reconcileCalls.Load(); got != 2 {
		t.Fatalf("after pace window reconcileCalls=%d want 2", got)
	}
	assertCleanupFaultDegraded(t, d.Snapshot(), 0)
}

// TestIssue230CleanupRecoveryBackoffKeepsPendingDeliveryOperational proves
// paced cleanup recovery does not block pending telemetry delivery under fault.
func TestIssue230CleanupRecoveryBackoffKeepsPendingDeliveryOperational(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 3)
	wrap := &failingReconcileSpool{Spool: s, failErr: errors.New("injected reconcile failure")}
	var uploads atomic.Uint64
	tr := p2TransportFunc(func(_ context.Context, b coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error) {
		uploads.Add(1)
		return coreclient.AgentTelemetryAck{
			Status: "ACCEPTED", AcceptedThroughIngressSequence: b.LastIngressSequence,
		}, nil
	})
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		MaxBatchRecords: 10, InitialBackoff: time.Hour, MaxBackoff: time.Hour, JitterFraction: 0,
	}, wrap, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }
	d.jitter = func(base time.Duration, _ float64) time.Duration { return base }
	d.markCleanupFault(0)

	delivered := false
	for i := 0; i < 5; i++ {
		ok, err := d.RunOnce(context.Background())
		if err != nil {
			t.Fatalf("delivery under cleanup fault: ok=%v err=%v", ok, err)
		}
		if ok {
			delivered = true
			break
		}
	}
	if !delivered || uploads.Load() == 0 {
		t.Fatalf("pending delivery must proceed under paced cleanup fault: delivered=%v uploads=%d", delivered, uploads.Load())
	}
	assertCleanupFaultDegraded(t, d.Snapshot(), 0)
	if wrap.reconcileCalls.Load() != 1 {
		t.Fatalf("reconcileCalls=%d want 1 (paced)", wrap.reconcileCalls.Load())
	}
}

// TestIssue230CleanupRecoveryBackoffUnderLargeBacklog verifies paced recovery
// still avoids repeated directory reconcile while a large pending backlog exists.
func TestIssue230CleanupRecoveryBackoffUnderLargeBacklog(t *testing.T) {
	s, dir := openSpool(t)
	appendNDurable(t, s, 200)
	wrap := &failingReconcileSpool{Spool: s, failErr: errors.New("injected reconcile failure")}
	// Retain the backlog: transient upload failure keeps pending without deliveryBlocked
	// masking the independent cleanup-fault health signal.
	tr := &fakeTransport{err: transientErr{}}
	d, err := New(Config{
		SensorID: "sensor-infra-01", StreamID: s.StreamID(), AgentSpoolDir: dir,
		MaxBatchRecords: 1, InitialBackoff: time.Hour, MaxBackoff: time.Hour, JitterFraction: 0,
	}, wrap, tr)
	if err != nil {
		t.Fatal(err)
	}
	d.wait = func(context.Context, time.Duration) error { return nil }
	d.jitter = func(base time.Duration, _ float64) time.Duration { return base }
	d.markCleanupFault(0)

	for i := 0; i < 40; i++ {
		_, _ = d.RunOnce(context.Background())
	}
	if got := wrap.reconcileCalls.Load(); got != 1 {
		t.Fatalf("large-backlog paced reconcileCalls=%d want 1", got)
	}
	snap := d.Snapshot()
	assertCleanupFaultDegraded(t, snap, 200)
	if snap.PendingRecords != 200 {
		t.Fatalf("pending backlog disturbed: %+v", snap)
	}
}

// failingReconcileSpool forces ReconcileAckedCleanup failures while counting calls.
type failingReconcileSpool struct {
	Spool
	failErr        error
	reconcileCalls atomic.Uint64
}

func (f *failingReconcileSpool) ReconcileAckedCleanup() error {
	f.reconcileCalls.Add(1)
	if f.failErr != nil {
		return f.failErr
	}
	type reconciler interface {
		ReconcileAckedCleanup() error
	}
	if r, ok := f.Spool.(reconciler); ok {
		return r.ReconcileAckedCleanup()
	}
	return errors.New("ReconcileAckedCleanup unavailable")
}

func (f *failingReconcileSpool) UnresolvedAckedCleanup() (bool, uint64, error) {
	type reporter interface {
		UnresolvedAckedCleanup() (bool, uint64, error)
	}
	if r, ok := f.Spool.(reporter); ok {
		return r.UnresolvedAckedCleanup()
	}
	return false, 0, nil
}
