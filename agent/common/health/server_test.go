package health

import (
	"context"
	"encoding/json"
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
	if !strings.Contains(body, `"status":"DEGRADED"`) && !strings.Contains(body, `"status": "DEGRADED"`) {
		// compact JSON encoder may omit spaces
		var hMap map[string]any
		if err := json.Unmarshal(wHealth.Body.Bytes(), &hMap); err != nil {
			t.Fatal(err)
		}
		if hMap["status"] != "DEGRADED" {
			t.Fatalf("expected DEGRADED status, got %v body=%s", hMap["status"], body)
		}
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
	if hMap["status"] != "DEGRADED" {
		t.Fatalf("expected DEGRADED status, got %v", hMap["status"])
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
	if !strings.Contains(wReadyFailed.Body.String(), "DELIVERY_NETWORK_ERROR") {
		t.Fatalf("expected last error code in ready body, got %s", wReadyFailed.Body.String())
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
