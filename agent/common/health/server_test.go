package health

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/delivery"
	"platform/agent/common/queue"
)

func TestHealthDeliveryBlockedDegraded(t *testing.T) {
	prov := &mockStatsProvider{receiverAvailable: true, deliveryBlocked: true, quarantine: 1}
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	wHealth := httptest.NewRecorder()
	s.handleHealth(wHealth, httptest.NewRequest(http.MethodGet, "/health", nil))
	if wHealth.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /health 503 when delivery blocked, got %d", wHealth.Code)
	}
	body := wHealth.Body.String()
	if !strings.Contains(body, "QUEUE_DELIVERY_BLOCKED") {
		t.Fatalf("expected QUEUE_DELIVERY_BLOCKED reason, got %s", body)
	}
	if !strings.Contains(body, "QUEUE_QUARANTINE_PRESENT") {
		t.Fatalf("expected QUEUE_QUARANTINE_PRESENT reason, got %s", body)
	}
	var hMap map[string]any
	if err := json.Unmarshal(wHealth.Body.Bytes(), &hMap); err != nil {
		t.Fatal(err)
	}
	if hMap["document_type"] != "SUPPORT_EVIDENCE" || hMap["component"] != "platform-agent" {
		t.Fatalf("support evidence identity=%v", hMap)
	}
	if hMap["component_state"] != "DEGRADED" {
		t.Fatalf("expected DEGRADED component_state, got %v", hMap["component_state"])
	}
}

func TestHealthAckRecoveryConflictDegraded(t *testing.T) {
	prov := &mockStatsProvider{receiverAvailable: true, ackRecoveryConflict: true}
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	wHealth := httptest.NewRecorder()
	s.handleHealth(wHealth, httptest.NewRequest(http.MethodGet, "/health", nil))
	if wHealth.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /health 503 for ack recovery conflict, got %d", wHealth.Code)
	}
	body := wHealth.Body.String()
	if !strings.Contains(body, "QUEUE_ACK_RECOVERY_CONFLICT") {
		t.Fatalf("expected QUEUE_ACK_RECOVERY_CONFLICT, got %s", body)
	}
	var hMap map[string]any
	if err := json.Unmarshal(wHealth.Body.Bytes(), &hMap); err != nil {
		t.Fatal(err)
	}
	if hMap["component_state"] != "DEGRADED" {
		t.Fatalf("expected DEGRADED component_state, got %v body=%s", hMap["component_state"], body)
	}
	if strings.Contains(body, "payload") || strings.Contains(body, "replacement") || strings.Contains(body, "secret") {
		t.Fatalf("health must not include payload material: %s", body)
	}
}

func TestHealthUnknownSequenceDegraded(t *testing.T) {
	prov := &mockStatsProvider{receiverAvailable: true, unknownSequence: true}
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	wHealth := httptest.NewRecorder()
	s.handleHealth(wHealth, httptest.NewRequest(http.MethodGet, "/health", nil))
	if wHealth.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /health 503 for unknown-sequence block, got %d", wHealth.Code)
	}
	body := wHealth.Body.String()
	if !strings.Contains(body, "QUEUE_UNKNOWN_SEQUENCE_BLOCK") {
		t.Fatalf("expected QUEUE_UNKNOWN_SEQUENCE_BLOCK, got %s", body)
	}
	if strings.Contains(body, "payload") || strings.Contains(body, "secret") {
		t.Fatalf("health must not include payload material: %s", body)
	}
}

func TestHealthQueueFullDegraded(t *testing.T) {
	prov := &mockStatsProvider{receiverAvailable: true, atCapacity: true}
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	wHealth := httptest.NewRecorder()
	s.handleHealth(wHealth, reqHealth)
	if wHealth.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /health 503 when queue full, got %d", wHealth.Code)
	}
}

type mockStatsProvider struct {
	receiverAvailable    bool
	atCapacity           bool
	quarantine           int
	collectorErrors      int
	lastErrorCode        string
	lastFailureClass     client.FailureClass
	deliveryBlocked      bool
	unknownSequence      bool
	locationUncertain    bool
	ackRecoveryConflict  bool
}

func (m *mockStatsProvider) QueueStats() queue.QueueStats {
	return queue.QueueStats{
		ActiveCount:              5,
		ActiveBytes:              1024,
		AtCapacity:               m.atCapacity,
		QuarantineCount:          m.quarantine,
		DeliveryBlocked:          m.deliveryBlocked || m.unknownSequence || m.locationUncertain || m.ackRecoveryConflict,
		UnknownSequenceBlock:     m.unknownSequence,
		LocationUncertain:        m.locationUncertain,
		AckRecoveryConflictBlock: m.ackRecoveryConflict,
	}
}

func (m *mockStatsProvider) DeliveryStats() delivery.DeliveryStats {
	return delivery.DeliveryStats{
		ReceiverAvailable: m.receiverAvailable,
		Attempts:          10,
		Successes:         5,
		LastErrorCode:     m.lastErrorCode,
		LastFailureClass:  m.lastFailureClass,
	}
}

func (m *mockStatsProvider) CollectorStats() (time.Time, int) {
	return time.Now().UTC(), m.collectorErrors
}

func TestHealthEndpoints(t *testing.T) {
	// 1. When receiver is unavailable (production initial state)
	provUnavailable := &mockStatsProvider{receiverAvailable: false}
	s1, err := NewServer("127.0.0.1:0", provUnavailable, nil)
	if err != nil {
		t.Fatal(err)
	}

	// /live is 200
	reqLive := httptest.NewRequest(http.MethodGet, "/live", nil)
	wLive := httptest.NewRecorder()
	s1.handleLive(wLive, reqLive)
	if wLive.Code != http.StatusOK {
		t.Fatalf("expected /live 200, got %d", wLive.Code)
	}

	// /ready is 503 (degraded because receiver is not yet implemented)
	reqReady := httptest.NewRequest(http.MethodGet, "/ready", nil)
	wReady := httptest.NewRecorder()
	s1.handleReady(wReady, reqReady)
	if wReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /ready 503 while receiver unavailable, got %d", wReady.Code)
	}

	// /health returns DEGRADED
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	wHealth := httptest.NewRecorder()
	s1.handleHealth(wHealth, reqHealth)
	if wHealth.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /health 503, got %d", wHealth.Code)
	}

	var hMap map[string]any
	if err := json.Unmarshal(wHealth.Body.Bytes(), &hMap); err != nil {
		t.Fatal(err)
	}
	if hMap["document_type"] != "SUPPORT_EVIDENCE" {
		t.Fatalf("expected SUPPORT_EVIDENCE, got %v", hMap["document_type"])
	}
	if hMap["component_state"] != "DEGRADED" {
		t.Fatalf("expected DEGRADED component_state, got %v", hMap["component_state"])
	}

	// 2. Wired HTTPS transport is not readiness: last failure class must degrade /ready.
	provFailedDelivery := &mockStatsProvider{
		receiverAvailable: true,
		lastFailureClass:  client.FailureTemporary,
		lastErrorCode:     "DELIVERY_NETWORK_ERROR",
	}
	sFailed, err := NewServer("127.0.0.1:0", provFailedDelivery, nil)
	if err != nil {
		t.Fatal(err)
	}
	wReadyFailed := httptest.NewRecorder()
	sFailed.handleReady(wReadyFailed, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if wReadyFailed.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /ready 503 after last delivery failure, got %d", wReadyFailed.Code)
	}
	var readyMap map[string]any
	if err := json.Unmarshal(wReadyFailed.Body.Bytes(), &readyMap); err != nil {
		t.Fatal(err)
	}
	if readyMap["reason_code"] != "DELIVERY_NETWORK_ERROR" || readyMap["ready"] != false || readyMap["http_semantic"] != float64(503) {
		t.Fatalf("unexpected ready body=%v", readyMap)
	}
	if readyMap["status"] != "NOT_READY" || readyMap["component"] != "platform-agent" {
		t.Fatalf("unexpected ready identity=%v", readyMap)
	}

	wHealthFailed := httptest.NewRecorder()
	sFailed.handleHealth(wHealthFailed, httptest.NewRequest(http.MethodGet, "/health", nil))
	if wHealthFailed.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /health 503 after last delivery failure, got %d", wHealthFailed.Code)
	}

	// 3. When receiver is available and last delivery succeeded
	provAvailable := &mockStatsProvider{receiverAvailable: true}
	s2, _ := NewServer("127.0.0.1:0", provAvailable, nil)

	wReady2 := httptest.NewRecorder()
	s2.handleReady(wReady2, reqReady)
	if wReady2.Code != http.StatusOK {
		t.Fatalf("expected /ready 200 when receiver available, got %d", wReady2.Code)
	}
}

func TestLiveAndReadyCanonicalEnvelopes(t *testing.T) {
	s, err := NewServer("127.0.0.1:0", &mockStatsProvider{receiverAvailable: true}, nil,
		WithIdentity("0.1.0-test", "agent-instance-test"))
	if err != nil {
		t.Fatal(err)
	}

	wLive := httptest.NewRecorder()
	s.handleLive(wLive, httptest.NewRequest(http.MethodGet, "/live", nil))
	if wLive.Code != http.StatusOK {
		t.Fatalf("live=%d", wLive.Code)
	}
	var live map[string]any
	if err := json.Unmarshal(wLive.Body.Bytes(), &live); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"status", "component", "ts", "service_instance_id"} {
		if _, ok := live[key]; !ok {
			t.Fatalf("live missing %s: %v", key, live)
		}
	}
	if live["status"] != "UP" || live["component"] != "platform-agent" {
		t.Fatalf("live body=%v", live)
	}
	if _, exists := live["runtime"]; exists {
		t.Fatalf("live must not emit legacy runtime field: %v", live)
	}

	wReady := httptest.NewRecorder()
	s.handleReady(wReady, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if wReady.Code != http.StatusOK {
		t.Fatalf("ready=%d body=%s", wReady.Code, wReady.Body.String())
	}
	var ready map[string]any
	if err := json.Unmarshal(wReady.Body.Bytes(), &ready); err != nil {
		t.Fatal(err)
	}
	if ready["status"] != "UP" || ready["ready"] != true || ready["http_semantic"] != float64(200) {
		t.Fatalf("ready body=%v", ready)
	}

	wAlias := httptest.NewRecorder()
	s.handleReady(wAlias, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if wAlias.Code != http.StatusOK {
		t.Fatalf("healthz alias=%d", wAlias.Code)
	}
}

func TestHealthUnavailableReasonUsesLastErrorCode(t *testing.T) {
	prov := &mockStatsProvider{receiverAvailable: false, lastErrorCode: "DELIVERY_NETWORK_ERROR"}
	s, err := NewServer("127.0.0.1:0", prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	wReady := httptest.NewRecorder()
	s.handleReady(wReady, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if wReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", wReady.Code)
	}
	if !strings.Contains(wReady.Body.String(), "DELIVERY_NETWORK_ERROR") {
		t.Fatalf("expected last error code in ready body, got %s", wReady.Body.String())
	}
	if strings.Contains(wReady.Body.String(), "AGENT_RECEIVER_NOT_IMPLEMENTED") {
		t.Fatal("health must not hardcode AGENT_RECEIVER_NOT_IMPLEMENTED")
	}
}

func TestHealthServerNonLoopbackRejection(t *testing.T) {
	_, err := NewServer("0.0.0.0:8089", nil, nil)
	if err == nil {
		t.Fatal("expected error on 0.0.0.0 non-loopback address")
	}
}

func TestHealthServerLifecycle(t *testing.T) {
	s, err := NewServer("127.0.0.1:0", &mockStatsProvider{receiverAvailable: false}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Stop(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestHealthzMuxAliasIsReadiness(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	s, err := NewServer(addr, &mockStatsProvider{receiverAvailable: true}, nil,
		WithIdentity("0.1.0-test", "agent-mux-test"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.Stop(ctx)
	}()

	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err = http.Get("http://" + addr + "/healthz")
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("healthz mux probe failed: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status=%d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["ready"] != true || body["http_semantic"] != float64(200) {
		t.Fatalf("healthz readiness body=%v", body)
	}
	if _, ok := body["document_type"]; ok {
		t.Fatalf("healthz must not be SUPPORT_EVIDENCE: %v", body)
	}
}

func TestReadyNotReadyWhileShuttingDown(t *testing.T) {
	s, err := NewServer("127.0.0.1:0", &mockStatsProvider{receiverAvailable: true}, nil,
		WithIdentity("0.1.0-test", "agent-shutdown-ready"))
	if err != nil {
		t.Fatal(err)
	}
	s.shuttingDown.Store(true)

	wReady := httptest.NewRecorder()
	s.handleReady(wReady, httptest.NewRequest(http.MethodGet, "/ready", nil))
	if wReady.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready during shutdown=%d body=%s", wReady.Code, wReady.Body.String())
	}
	var ready map[string]any
	if err := json.Unmarshal(wReady.Body.Bytes(), &ready); err != nil {
		t.Fatal(err)
	}
	if ready["ready"] != false || ready["status"] != "NOT_READY" || ready["reason_code"] != "AGENT_SHUTTING_DOWN" {
		t.Fatalf("ready shutdown body=%v", ready)
	}

	wAlias := httptest.NewRecorder()
	s.handleReady(wAlias, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if wAlias.Code != http.StatusServiceUnavailable {
		t.Fatalf("healthz during shutdown=%d", wAlias.Code)
	}

	wLive := httptest.NewRecorder()
	s.handleLive(wLive, httptest.NewRequest(http.MethodGet, "/live", nil))
	if wLive.Code != http.StatusOK {
		t.Fatalf("live during shutdown must remain 200, got %d", wLive.Code)
	}
	var live map[string]any
	if err := json.Unmarshal(wLive.Body.Bytes(), &live); err != nil {
		t.Fatal(err)
	}
	if live["status"] != "SHUTTING_DOWN" {
		t.Fatalf("live shutdown body=%v", live)
	}
}

func TestSupportEvidenceLocalQueueFailureKeepsTransportUp(t *testing.T) {
	// Mimic delivery.recordQueueFailure: ReceiverAvailable stays true while
	// LastFailureClass/LastErrorCode capture a local spool fault after a valid ACK.
	prov := &mockStatsProvider{
		receiverAvailable: true,
		lastFailureClass:  client.FailureBlocked,
		lastErrorCode:     "QUEUE_ACK_COMMIT_FAILED",
	}
	s, err := NewServer("127.0.0.1:0", prov, nil, WithIdentity("0.1.0-test", "agent-local-queue"))
	if err != nil {
		t.Fatal(err)
	}

	wHealth := httptest.NewRecorder()
	s.handleHealth(wHealth, httptest.NewRequest(http.MethodGet, "/health", nil))
	if wHealth.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected /health 503 for local queue failure, got %d", wHealth.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(wHealth.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	deps, _ := body["dependency_status"].(map[string]any)
	if deps["sensor_transport"] != "UP" {
		t.Fatalf("local queue failure must not mark sensor_transport DOWN: %v", body)
	}
	if body["delivery_status"] != "FAILED" {
		t.Fatalf("expected delivery_status FAILED for local queue failure, got %v", body["delivery_status"])
	}
}
