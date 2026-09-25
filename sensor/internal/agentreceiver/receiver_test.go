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

func newTestReceiver(t *testing.T, role runtime.Role, identity AgentIdentityVerifier, authorizer AssignmentAuthorizer, sink AcceptanceSink) *Receiver {
	t.Helper()
	cfg := Config{
		SensorID:        "sensor-infra-01",
		SensorRole:      role,
		MaxBodyBytes:    1024 * 1024,
		MaxBatchRecords: 50,
		MaxRecordBytes:  64 * 1024,
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	if identity == nil {
		identity = NewStaticTestIdentityVerifier()
		identity.(*StaticTestIdentityVerifier).AllowCommonName("agent-1", "agent-1")
	}
	rcv, err := NewReceiver(cfg, identity, authorizer, sink, logger)
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
		SchemaVersion: SchemaVersionV1,
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

func sampleStreamID(hex32 string) string {
	return "stream-" + hex32
}

func sampleV2Envelope(agentID, tenantID, streamID string, recs ...DeliveryRecord) DeliveryEnvelope {
	env := sampleEnvelope(agentID, tenantID, recs...)
	env.SchemaVersion = SchemaVersionV2
	env.AgentStreamID = streamID
	return env
}

func mockTLSServer(handler http.Handler, certCN string) (*httptest.Server, *http.Client) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if certCN != "" {
			cert := &x509.Certificate{
				Subject:  pkix.Name{CommonName: certCN},
				DNSNames: []string{certCN},
			}
			r.TLS = &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{cert},
				VerifiedChains:   [][]*x509.Certificate{{cert}},
			}
		}
		handler.ServeHTTP(w, r)
	}))
	server.Start()
	return server, server.Client()
}

func postJSON(t *testing.T, client *http.Client, url string, body []byte) *http.Response {
	t.Helper()
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
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

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	_, err := NewReceiver(Config{SensorID: "s1", SensorRole: runtime.RoleNetwork}, nil, nil, nil, logger)
	if err == nil {
		t.Fatalf("expected NewReceiver to fail for NETWORK role")
	}
	if !strings.Contains(err.Error(), ErrCodeRoleIneligible) {
		t.Errorf("expected ErrCodeRoleIneligible in error, got %v", err)
	}

	_, err = NewReceiver(Config{SensorID: "", SensorRole: runtime.RoleInfrastructure}, nil, nil, nil, logger)
	if err == nil {
		t.Fatal("empty SensorID must fail")
	}
}

func TestRejectionWithoutClientCertificate(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Code != ErrCodeAuthRequired {
		t.Errorf("expected %s, got %s", ErrCodeAuthRequired, errResp.Code)
	}
}

func TestRejectionOnIdentityMismatch(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-rogue", "agent-rogue")
	auth := NewInMemoryAssignmentAuthorizer()
	// Rogue peer must be assigned so authorize succeeds; claim agentId is then checked.
	auth.Allow("agent-rogue", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-rogue")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
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

func TestFailClosedIdentityVerifier(t *testing.T) {
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, NewFailClosedIdentityVerifier(), auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401 fail-closed identity, got %d", resp.StatusCode)
	}
}

func TestRejectionWhenNotAssigned(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if errResp.Code != ErrCodeAgentNotAssigned {
		t.Errorf("expected %s, got %s", ErrCodeAgentNotAssigned, errResp.Code)
	}
}

func TestSpoofTenantDenied(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	env := sampleEnvelope("agent-1", "tenant-spoof", sampleRecord(1, "r-1", []byte(`{"m":1}`)))
	body, _ := json.Marshal(env)
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("spoof tenant must be denied, got %d", resp.StatusCode)
	}
}

func TestWrongSensorAssignmentDenied(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "other-sensor", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong sensor assignment must deny, got %d", resp.StatusCode)
	}
}

func TestRevokedAssignmentDenied(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	auth.Revoke("agent-1", "sensor-infra-01")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("revoked assignment must deny, got %d", resp.StatusCode)
	}
}

func TestSameAgentIDDifferentTenantsIsolated(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	// Authoritative assignment is for tenant-A only.
	auth.Allow("agent-1", "sensor-infra-01", "tenant-A", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	// Claiming tenant-B must fail even if textual agent id matches.
	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-B", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-tenant claim must deny, got %d", resp.StatusCode)
	}

	bodyOK, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-A", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	respOK := postJSON(t, client, srv.URL+EndpointPath, bodyOK)
	defer respOK.Body.Close()
	if respOK.StatusCode != http.StatusOK {
		t.Fatalf("valid tenant path must succeed, got %d", respOK.StatusCode)
	}
}

func TestSuccessfulDeliveryAndAck(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	rec1 := sampleRecord(101, "r-101", []byte(`{"cpu":25.5}`))
	rec2 := sampleRecord(102, "r-102", []byte(`{"mem":70.0}`))
	env := sampleEnvelope("agent-1", "tenant-1", rec1, rec2)

	body, _ := json.Marshal(env)
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var ack DeliveryAck
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		t.Fatalf("failed to decode ACK: %v", err)
	}

	if ack.Status != "ACCEPTED" || ack.AcceptedThroughSequence != 102 {
		t.Errorf("unexpected ACK: %+v", ack)
	}
	if ack.AgentID != "agent-1" || ack.BatchID != "batch-2001" {
		t.Errorf("expected batch-bound ACK, got %+v", ack)
	}
	if sink.RecordCount() != 2 {
		t.Errorf("expected 2 records in sink, got %d", sink.RecordCount())
	}
}

func TestExactReplayIdempotency(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	rec1 := sampleRecord(1, "r-1", []byte(`{"val":1}`))
	env := sampleEnvelope("agent-1", "tenant-1", rec1)
	body, _ := json.Marshal(env)

	resp1 := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("first request expected 200, got %d", resp1.StatusCode)
	}

	resp2 := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("replay request expected 200, got %d", resp2.StatusCode)
	}
	if sink.BatchCount() != 1 {
		t.Errorf("expected 1 batch in sink despite replay, got %d", sink.BatchCount())
	}
}

func TestConflictingReplayRejection(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	rec1 := sampleRecord(1, "r-1", []byte(`{"val":1}`))
	env1 := sampleEnvelope("agent-1", "tenant-1", rec1)
	body1, _ := json.Marshal(env1)
	resp1 := postJSON(t, client, srv.URL+EndpointPath, body1)
	resp1.Body.Close()

	rec2 := sampleRecord(1, "r-1", []byte(`{"val":999}`))
	env2 := sampleEnvelope("agent-1", "tenant-1", rec2)
	body2, _ := json.Marshal(env2)
	resp2 := postJSON(t, client, srv.URL+EndpointPath, body2)
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("expected 409 Conflict, got %d", resp2.StatusCode)
	}
}

func TestRecordChecksumFailureRejection(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	rec := sampleRecord(1, "r-1", []byte(`{"val":1}`))
	rec.Checksum = "0000000000000000000000000000000000000000000000000000000000000000"
	env := sampleEnvelope("agent-1", "tenant-1", rec)
	body, _ := json.Marshal(env)

	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestSequenceGapRejection(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	rec1 := sampleRecord(1, "r-1", []byte(`{"val":1}`))
	rec3 := sampleRecord(3, "r-3", []byte(`{"val":3}`))
	env := sampleEnvelope("agent-1", "tenant-1", rec1, rec3)
	env.LastSequence = 3
	env.RecordCount = 2
	body, _ := json.Marshal(env)

	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestEmptyBatchRejection(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	env := sampleEnvelope("agent-1", "tenant-1")
	body, _ := json.Marshal(env)
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", resp.StatusCode)
	}
}

func TestStrictUnknownFieldsRejection(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	raw := `{"schemaVersion":1,"agentId":"agent-1","tenantId":"tenant-1","correlationId":"c1","batchId":"b1","firstSequence":1,"lastSequence":1,"recordCount":1,"unknownField":"malicious","records":[{"sequence":1,"id":"r1","createdAt":"2026-09-08T12:00:00Z","checksum":"abc","payload":{}}]}`
	resp := postJSON(t, client, srv.URL+EndpointPath, []byte(raw))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request on unknown fields, got %d", resp.StatusCode)
	}
}

func TestContentTypeRequired(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	req, _ := http.NewRequest(http.MethodPost, srv.URL+EndpointPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415, got %d", resp.StatusCode)
	}
}

func TestSanitizedClientErrors(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	sink.SetInjectedError(io.ErrUnexpectedEOF)
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	var errResp ErrorResponse
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	if strings.Contains(errResp.Message, "unexpected EOF") {
		t.Fatalf("must not leak sink error text: %q", errResp.Message)
	}
}

func TestDownstreamSinkFailure(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	sink.SetInjectedError(io.ErrUnexpectedEOF)
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"val":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("expected 503 Service Unavailable, got %d", resp.StatusCode)
	}
}

func TestCancellationDoesNotProduceSuccessAck(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"val":1}`))))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

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

func TestNoPlatformLocalAuthorityAssumption(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	// Only exact CN mapping — suffix authority is not invented.
	ident.AllowCommonName("agent-1.agent.platform.local", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1.agent.platform.local")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exact CN mapping should succeed without suffix inventing, got %d", resp.StatusCode)
	}
}

func TestRejectUnverifiedPeerCertificatesAlone(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	fake := &x509.Certificate{Subject: pkix.Name{CommonName: "agent-1"}}
	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	req := httptest.NewRequest(http.MethodPost, EndpointPath, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{fake},
		VerifiedChains:   nil,
	}
	rr := httptest.NewRecorder()
	rcv.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without VerifiedChains, got %d", rr.Code)
	}
	var errResp ErrorResponse
	_ = json.NewDecoder(rr.Body).Decode(&errResp)
	if errResp.Code != ErrCodeAuthInvalid && errResp.Code != ErrCodeAuthRequired {
		t.Fatalf("expected auth error code, got %s", errResp.Code)
	}
}

func TestPayloadSiteRejectedWhenAuthoritativeSiteEmpty(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "") // site-less assignment
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	env := sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`)))
	env.SiteID = "attacker-site"
	body, _ := json.Marshal(env)
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("payload site with empty auth site must reject, got %d", resp.StatusCode)
	}
}

func TestMismatchedSiteRejected(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	env := sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`)))
	env.SiteID = "site-other"
	body, _ := json.Marshal(env)
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("mismatched site must reject, got %d", resp.StatusCode)
	}
}

func TestOmittedPayloadSiteUsesAuthoritativeSite(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	env := sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`)))
	env.SiteID = ""
	body, _ := json.Marshal(env)
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("omitted payload site must succeed, got %d", resp.StatusCode)
	}
	batches := sink.Batches()
	if len(batches) != 1 || batches[0].SiteID != "site-primary" {
		t.Fatalf("sink must receive authoritative site, got %+v", batches)
	}
}

func TestPayloadCannotAlterNormalizedSinkEnvelope(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-auth", "site-auth")
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	// Agent ID is exact-case; tenant/site claims remain normalized to authoritative values.
	env := sampleEnvelope("agent-1", "TENANT-AUTH", sampleRecord(1, "r-1", []byte(`{"m":1}`)))
	env.SiteID = "SITE-AUTH"
	body, _ := json.Marshal(env)
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected success, got %d", resp.StatusCode)
	}
	got := sink.Batches()[0]
	if got.AgentID != "agent-1" || got.TenantID != "tenant-auth" || got.SiteID != "site-auth" {
		t.Fatalf("sink envelope must be normalized, got agent=%q tenant=%q site=%q", got.AgentID, got.TenantID, got.SiteID)
	}
}

func TestInconsistentAssignmentSensorRejected(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := &inconsistentAuthorizer{ctx: AssignmentContext{
		TenantID: "tenant-1",
		SiteID:   "site-primary",
		SensorID: "wrong-sensor",
		AgentID:  "agent-1",
		Generation: 1,
		Assigned: true,
	}}
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong assignment sensor must reject, got %d", resp.StatusCode)
	}
}

func TestInconsistentAssignmentAgentRejected(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := &inconsistentAuthorizer{ctx: AssignmentContext{
		TenantID: "tenant-1",
		SiteID:   "site-primary",
		SensorID: "sensor-infra-01",
		AgentID:  "other-agent",
		Generation: 1,
		Assigned: true,
	}}
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong assignment agent must reject, got %d", resp.StatusCode)
	}
}

func TestAssignmentContextEmptyAgentIDRejected(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := &inconsistentAuthorizer{ctx: AssignmentContext{
		TenantID: "tenant-1",
		SiteID:   "site-primary",
		SensorID: "sensor-infra-01",
		AgentID:  "",
		Generation: 1,
		Assigned: true,
	}}
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("empty authoritative agent ID must reject, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Code != ErrCodeAgentNotAssigned {
		t.Fatalf("expected %s, got %s", ErrCodeAgentNotAssigned, errResp.Code)
	}
	if sink.BatchCount() != 0 {
		t.Fatalf("sink must not be called, got %d batches", sink.BatchCount())
	}
	if rcv.ReplayTracker().Size() != 0 {
		t.Fatalf("replay must not record rejected assignment, size=%d", rcv.ReplayTracker().Size())
	}
	if rcv.Metrics().Snapshot().AcceptedBatches != 0 {
		t.Fatal("rejected assignment must not produce a successful acceptance")
	}
}

func TestAssignmentContextCaseOnlyAgentIDMismatchRejected(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := &inconsistentAuthorizer{ctx: AssignmentContext{
		TenantID: "tenant-1",
		SiteID:   "site-primary",
		SensorID: "sensor-infra-01",
		AgentID:  "AGENT-1",
		Generation: 1,
		Assigned: true,
	}}
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("case-only authoritative agent mismatch must reject, got %d", resp.StatusCode)
	}
	var errResp ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Code != ErrCodeAgentNotAssigned {
		t.Fatalf("expected %s, got %s", ErrCodeAgentNotAssigned, errResp.Code)
	}
	if sink.BatchCount() != 0 {
		t.Fatalf("sink must not be called, got %d batches", sink.BatchCount())
	}
	if rcv.ReplayTracker().Size() != 0 {
		t.Fatalf("replay must not record rejected assignment, size=%d", rcv.ReplayTracker().Size())
	}
	if rcv.Metrics().Snapshot().AcceptedBatches != 0 {
		t.Fatal("rejected assignment must not produce a successful acceptance")
	}
}

func TestAssignmentContextExactAgentIDMatchAccepted(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := &inconsistentAuthorizer{ctx: AssignmentContext{
		TenantID: "tenant-1",
		SiteID:   "site-primary",
		SensorID: "sensor-infra-01",
		AgentID:  "agent-1",
		Generation: 1,
		Assigned: true,
	}}
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)

	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body, _ := json.Marshal(sampleEnvelope("agent-1", "tenant-1", sampleRecord(1, "r-1", []byte(`{"m":1}`))))
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exact authoritative agent ID must pass, got %d", resp.StatusCode)
	}
	var ack DeliveryAck
	if err := json.NewDecoder(resp.Body).Decode(&ack); err != nil {
		t.Fatalf("failed to decode ACK: %v", err)
	}
	if ack.Status != "ACCEPTED" || ack.AgentID != "agent-1" {
		t.Fatalf("unexpected successful ACK: %+v", ack)
	}
	if sink.BatchCount() != 1 {
		t.Fatalf("exact authoritative match must reach sink once, got %d", sink.BatchCount())
	}
	if rcv.ReplayTracker().Size() != 1 {
		t.Fatalf("successful delivery must record replay state, size=%d", rcv.ReplayTracker().Size())
	}
}

func TestReceiverHTTP413RequestTooLarge(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.Allow("agent-1", "sensor-infra-01", "tenant-1", "site-primary")
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	rcv, err := NewReceiver(Config{
		SensorID:        "sensor-infra-01",
		SensorRole:      runtime.RoleInfrastructure,
		MaxBodyBytes:    64,
		MaxBatchRecords: 50,
		MaxRecordBytes:  64 * 1024,
	}, ident, auth, NewInMemoryAcceptanceSink(), logger)
	if err != nil {
		t.Fatal(err)
	}
	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	body := []byte(`{"schemaVersion":1,"agentId":"agent-1","tenantId":"tenant-1","correlationId":"c","batchId":"b","firstSequence":1,"lastSequence":1,"recordCount":1,"records":[{"sequence":1,"id":"r","createdAt":"2026-09-08T12:00:00Z","checksum":"00","payload":{"pad":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}}]}`)
	if int64(len(body)) <= 64 {
		t.Fatalf("fixture must exceed MaxBodyBytes, got %d", len(body))
	}
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected HTTP 413, got %d", resp.StatusCode)
	}
	var er ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		t.Fatal(err)
	}
	if er.Code != ErrCodeRequestTooLarge {
		t.Fatalf("expected %s, got %q", ErrCodeRequestTooLarge, er.Code)
	}
}

func TestReceiverLogsAndAcceptsV2StreamIdentity(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-1", "agent-1")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.AllowGeneration("agent-1", "sensor-infra-01", "tenant-1", "site-primary", 1)
	sink := NewInMemoryAcceptanceSink()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)
	srv, client := mockTLSServer(rcv, "agent-1")
	defer srv.Close()

	env := sampleV2Envelope("agent-1", "tenant-1", testStreamA(), sampleRecord(1, "r1", []byte(`{"ok":true}`)))
	body, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("HTTP %d", resp.StatusCode)
	}
	batches := sink.Batches()
	if len(batches) != 1 || batches[0].AgentStreamID != testStreamA() {
		t.Fatalf("sink must receive agentStreamId, got %+v", batches)
	}
}
