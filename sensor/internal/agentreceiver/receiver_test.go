package agentreceiver

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"platform/sensor/internal/runtime"
)

func newTestReceiver(t *testing.T, role runtime.Role, authorizer AssignmentAuthorizer, sink AcceptanceSink) *Receiver {
	t.Helper()
	cfg := Config{
		SensorID:        "sensor-infra-01",
		SensorRole:      role,
		MaxBodyBytes:    1024 * 1024,
		MaxBatchRecords: 50,
		MaxRecordBytes:  64 * 1024,
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	rcv, err := NewReceiver(cfg, authorizer, sink, logger)
	if err != nil {
		t.Fatalf("failed to create receiver: %v", err)
	}
	return rcv
}

func sampleRecord(seq uint64, id string, payload []byte) DeliveryRecord {
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	return DeliveryRecord{
		Sequence:  seq,
		ID:        id,
		CreatedAt: now,
		Checksum:  ComputeRecordChecksum(seq, id, now, payload),
		Payload:   payload,
	}
}

func sampleEnvelope(agentID, tenantID string, recs ...DeliveryRecord) DeliveryEnvelope {
	first := uint64(0)
	last := uint64(0)
	if len(recs) > 0 {
		first = recs[0].Sequence
		last = recs[len(recs)-1].Sequence
	}
	return DeliveryEnvelope{
		SchemaVersion: CurrentSchemaVersion,
		AgentID:       agentID,
		TenantID:      tenantID,
		SiteID:        "site-primary",
		CorrelationID: "corr-1001",
		BatchID:       "batch-2001",
		FirstSequence: first,
		LastSequence:  last,
		RecordCount:   len(recs),
		Records:       recs,
	}
}

func mockTLSServer(handler http.Handler, certCN string) (*httptest.Server, *http.Client) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock TLS ConnectionState with peer certificate
		if certCN != "" {
			cert := &x509.Certificate{
				Subject:  pkix.Name{CommonName: certCN},
				DNSNames: []string{certCN},
			}
			r.TLS = &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{cert},
			}
		}
		handler.ServeHTTP(w, r)
	}))
	server.Start()
	return server, server.Client()
}

func TestRoleEligibility(t *testing.T) {
	if !IsRoleEligible(runtime.RoleInfrastructure) {
		t.Errorf("INFRASTRUCTURE role must be eligible")
	}
	if !IsRoleEligible(runtime.RoleCombined) {
		t.Errorf("COMBINED role must be eligible")
	}
	if IsRoleEligible(runtime.RoleNetwork) {
		t.Errorf("NETWORK role must NOT be eligible")
	}
	if IsRoleEligible(runtime.RoleStreamingEvent) {
		t.Errorf("STREAMING_EVENT role must NOT be eligible")
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	_, err := NewReceiver(Config{SensorID: "s1", SensorRole: runtime.RoleNetwork}, nil, nil, logger)
	if err == nil {
		t.Fatalf("expected NewReceiver to fail for NETWORK role")
	}
	if !strings.Contains(err.Error(), ErrCodeRoleIneligible) {
		t.Errorf("expected ErrCodeRoleIneligible in error, got %v", err)
	}
}

func TestRejectionWithoutClientCertificate(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, auth, sink)

	// No TLS peer cert
	srv, client := mockTLSServer(rcv, "")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp, err := client.Post(srv.URL+EndpointPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Code != ErrCodeAuthRequired {
		t.Errorf("expected %s, got %s", ErrCodeAuthRequired, errResp.Code)
	}
	if rcv.Metrics().Snapshot().AuthFailures != 1 {
		t.Errorf("expected 1 auth failure metric, got %d", rcv.Metrics().Snapshot().AuthFailures)
	}
}

func TestRejectionOnIdentityMismatch(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "tenant-1", "sensor-infra-01")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, auth, sink)

	// Certificate says agent-rogue, envelope claims agent-1
	srv, client := mockTLSServer(rcv, "agent-rogue.agent.platform.local")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp, err := client.Post(srv.URL+EndpointPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Code != ErrCodeAuthIdentityMismatch {
		t.Errorf("expected %s, got %s", ErrCodeAuthIdentityMismatch, errResp.Code)
	}
}

func TestRejectionWhenNotAssigned(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	// Deliberately NOT authorizing agent-1 to this sensor
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp, err := client.Post(srv.URL+EndpointPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Code != ErrCodeAgentNotAssigned {
		t.Errorf("expected %s, got %s", ErrCodeAgentNotAssigned, errResp.Code)
	}
	if rcv.Metrics().Snapshot().AssignmentDenials != 1 {
		t.Errorf("expected 1 assignment denial metric, got %d", rcv.Metrics().Snapshot().AssignmentDenials)
	}
}

func TestSuccessfulDeliveryAndAck(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "tenant-1", "sensor-infra-01")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	rec1 := sampleRecord(101, "r-101", []byte(`{"cpu":25.5}`))
	rec2 := sampleRecord(102, "r-102", []byte(`{"mem":70.0}`))
	env := sampleEnvelope("agent-1", "tenant-1", rec1, rec2)

	body, _ := json.Marshal(env)
	resp, err := client.Post(srv.URL+EndpointPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var ack DeliveryAck
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		t.Fatalf("failed to decode ACK: %v", err)
	}

	if ack.Status != "ACCEPTED" {
		t.Errorf("expected status ACCEPTED, got %s", ack.Status)
	}
	if ack.AcceptedThroughSequence != 102 {
		t.Errorf("expected accepted through 102, got %d", ack.AcceptedThroughSequence)
	}
	if ack.AgentID != "agent-1" {
		t.Errorf("expected agent-1, got %s", ack.AgentID)
	}
	if ack.CorrelationID != "corr-1001" {
		t.Errorf("expected corr-1001, got %s", ack.CorrelationID)
	}

	snap := rcv.Metrics().Snapshot()
	if snap.AcceptedBatches != 1 || snap.AcceptedRecords != 2 || snap.LastAckedSequence != 102 {
		t.Errorf("unexpected metrics: %+v", snap)
	}
	if sink.RecordCount() != 2 {
		t.Errorf("expected 2 records in sink, got %d", sink.RecordCount())
	}
}

func TestExactReplayIdempotency(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "tenant-1", "sensor-infra-01")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	rec1 := sampleRecord(1, "r-1", []byte(`{"val":1}`))
	env := sampleEnvelope("agent-1", "tenant-1", rec1)
	body, _ := json.Marshal(env)

	// First attempt
	resp1, err := client.Post(srv.URL+EndpointPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first request expected 200, got %d", resp1.StatusCode)
	}

	// Exact replay attempt
	resp2, err := client.Post(srv.URL+EndpointPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("replay request failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("replay request expected 200, got %d", resp2.StatusCode)
	}

	var ack2 DeliveryAck
	_ = json.NewDecoder(resp2.Body).Decode(&ack2)
	if ack2.AcceptedThroughSequence != 1 {
		t.Errorf("expected cached ACK for seq 1, got %d", ack2.AcceptedThroughSequence)
	}

	// Downstream sink was called ONLY ONCE
	if sink.BatchCount() != 1 {
		t.Errorf("expected 1 batch in sink despite replay, got %d", sink.BatchCount())
	}
	if rcv.Metrics().Snapshot().ReplayExactHits != 1 {
		t.Errorf("expected 1 replay exact hit metric, got %d", rcv.Metrics().Snapshot().ReplayExactHits)
	}
}

func TestConflictingReplayRejection(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "tenant-1", "sensor-infra-01")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	rec1 := sampleRecord(1, "r-1", []byte(`{"val":1}`))
	env1 := sampleEnvelope("agent-1", "tenant-1", rec1)
	body1, _ := json.Marshal(env1)

	resp1, _ := client.Post(srv.URL+EndpointPath, "application/json", bytes.NewReader(body1))
	resp1.Body.Close()

	// Same batchId "batch-2001", but different payload
	rec2 := sampleRecord(1, "r-1", []byte(`{"val":999}`))
	env2 := sampleEnvelope("agent-1", "tenant-1", rec2)
	body2, _ := json.Marshal(env2)

	resp2, err := client.Post(srv.URL+EndpointPath, "application/json", bytes.NewReader(body2))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 Conflict, got %d", resp2.StatusCode)
	}
	var errResp ErrorResponse
	_ = json.NewDecoder(resp2.Body).Decode(&errResp)
	if errResp.Code != ErrCodeReplayConflict {
		t.Errorf("expected %s, got %s", ErrCodeReplayConflict, errResp.Code)
	}
	if rcv.Metrics().Snapshot().ReplayConflicts != 1 {
		t.Errorf("expected 1 replay conflict metric, got %d", rcv.Metrics().Snapshot().ReplayConflicts)
	}
}

func TestRecordChecksumFailureRejection(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "tenant-1", "sensor-infra-01")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	rec := sampleRecord(1, "r-1", []byte(`{"val":1}`))
	rec.Checksum = "0000000000000000000000000000000000000000000000000000000000000000" // tampered checksum
	env := sampleEnvelope("agent-1", "tenant-1", rec)
	body, _ := json.Marshal(env)

	resp, err := client.Post(srv.URL+EndpointPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Code != ErrCodeRecordChecksumMismatch {
		t.Errorf("expected %s, got %s", ErrCodeRecordChecksumMismatch, errResp.Code)
	}
	if rcv.Metrics().Snapshot().ChecksumFailures != 1 {
		t.Errorf("expected 1 checksum failure metric, got %d", rcv.Metrics().Snapshot().ChecksumFailures)
	}
}

func TestSequenceGapRejection(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "tenant-1", "sensor-infra-01")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	rec1 := sampleRecord(1, "r-1", []byte(`{"val":1}`))
	rec3 := sampleRecord(3, "r-3", []byte(`{"val":3}`)) // Missing seq 2
	env := sampleEnvelope("agent-1", "tenant-1", rec1, rec3)
	env.LastSequence = 3
	env.RecordCount = 2
	body, _ := json.Marshal(env)

	resp, err := client.Post(srv.URL+EndpointPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Code != ErrCodeSequenceGap {
		t.Errorf("expected %s, got %s", ErrCodeSequenceGap, errResp.Code)
	}
}

func TestEmptyBatchRejection(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "tenant-1", "sensor-infra-01")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	env := sampleEnvelope("agent-1", "tenant-1") // 0 records
	body, _ := json.Marshal(env)

	resp, err := client.Post(srv.URL+EndpointPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Code != ErrCodeEmptyBatch {
		t.Errorf("expected %s, got %s", ErrCodeEmptyBatch, errResp.Code)
	}
}

func TestStrictUnknownFieldsRejection(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "tenant-1", "sensor-infra-01")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	raw := `{"schemaVersion":1,"agentId":"agent-1","tenantId":"tenant-1","correlationId":"c1","batchId":"b1","firstSequence":1,"lastSequence":1,"recordCount":1,"unknownField":"malicious","records":[{"sequence":1,"id":"r1","createdAt":"2026-09-08T12:00:00Z","checksum":"abc","payload":{}}]}`

	resp, err := client.Post(srv.URL+EndpointPath, "application/json", strings.NewReader(raw))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request on unknown fields, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Code != ErrCodeMalformedJSON {
		t.Errorf("expected %s, got %s", ErrCodeMalformedJSON, errResp.Code)
	}
}

func TestDownstreamSinkFailure(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "tenant-1", "sensor-infra-01")
	sink := NewInMemoryAcceptanceSink()
	sink.SetInjectedError(io.ErrUnexpectedEOF)
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	rec := sampleRecord(1, "r-1", []byte(`{"val":1}`))
	env := sampleEnvelope("agent-1", "tenant-1", rec)
	body, _ := json.Marshal(env)

	resp, err := client.Post(srv.URL+EndpointPath, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 Service Unavailable, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Code != ErrCodeSinkFailed {
		t.Errorf("expected %s, got %s", ErrCodeSinkFailed, errResp.Code)
	}
	if rcv.Metrics().Snapshot().SinkFailures != 1 {
		t.Errorf("expected 1 sink failure metric, got %d", rcv.Metrics().Snapshot().SinkFailures)
	}
}

func TestCancellationDoesNotProduceSuccessAck(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "tenant-1", "sensor-infra-01")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	rec := sampleRecord(1, "r-1", []byte(`{"val":1}`))
	env := sampleEnvelope("agent-1", "tenant-1", rec)
	body, _ := json.Marshal(env)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL+EndpointPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	_, err := client.Do(req)
	if err == nil {
		t.Fatalf("expected error on cancelled request, got nil")
	}

	if sink.BatchCount() != 0 {
		t.Errorf("sink should not receive cancelled batch, got %d", sink.BatchCount())
	}
}
