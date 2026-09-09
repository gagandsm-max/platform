package collector

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"platform/sensor/internal/observation"
)

type mockSink struct {
	mu       sync.Mutex
	envelopes []observation.Envelope
	receipts  []observation.Receipt
}

func (m *mockSink) Write(ctx context.Context, env observation.Envelope) (observation.Receipt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seq := uint64(len(m.envelopes) + 1)
	rec := observation.Receipt{Sequence: seq, RecordID: env.BatchID}
	m.envelopes = append(m.envelopes, env)
	m.receipts = append(m.receipts, rec)
	return rec, nil
}

func (m *mockSink) MaxPayloadBytes() int {
	return 1024 * 1024
}

func TestCollectorHarvestAndSpool(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	sink := &mockSink{}

	engine := NewCollectorEngine("sensor-net-01", "network", sink, 10*time.Millisecond, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)

	// Wait for at least 2 ticks
	time.Sleep(50 * time.Millisecond)

	engine.Stop()

	sink.mu.Lock()
	count := len(sink.envelopes)
	sink.mu.Unlock()

	if count < 2 {
		t.Fatalf("expected at least 2 envelopes written to sink, got %d", count)
	}

	sink.mu.Lock()
	first := sink.envelopes[0]
	sink.mu.Unlock()

	if first.SensorID != "sensor-net-01" {
		t.Errorf("expected sensorID sensor-net-01, got %s", first.SensorID)
	}
	if first.Protocol != "SENSOR" {
		t.Errorf("expected protocol SENSOR, got %s", first.Protocol)
	}
}
