package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"platform/agent/common/delivery"
	"platform/agent/common/queue"
)

type mockStatsProvider struct {
	receiverAvailable bool
}

func (m *mockStatsProvider) QueueStats() queue.QueueStats {
	return queue.QueueStats{ActiveCount: 5, ActiveBytes: 1024}
}

func (m *mockStatsProvider) DeliveryStats() delivery.DeliveryStats {
	return delivery.DeliveryStats{
		ReceiverAvailable: m.receiverAvailable,
		Attempts:          10,
		Successes:         5,
	}
}

func (m *mockStatsProvider) CollectorStats() (time.Time, int) {
	return time.Now().UTC(), 0
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

	// 2. When receiver is available
	provAvailable := &mockStatsProvider{receiverAvailable: true}
	s2, _ := NewServer("127.0.0.1:0", provAvailable, nil)

	wReady2 := httptest.NewRecorder()
	s2.handleReady(wReady2, reqReady)
	if wReady2.Code != http.StatusOK {
		t.Fatalf("expected /ready 200 when receiver available, got %d", wReady2.Code)
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
