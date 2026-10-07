package collector

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Local observational harness only. Not BUILD-019/130 capacity certification.
func BenchmarkProcessNetworkAggregatorIngest(b *testing.B) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg, err := NewProcessNetworkAggregator(ProcessNetworkAggregatorConfig{
		SourceProvenance:     ProcessNetworkSyntheticSourceProvenance,
		Window:               time.Minute,
		TestAllowShortWindow: true,
		MaxGlobalKeys:        4096,
		MaxKeysPerWorkload:   64,
	}, sink)
	if err != nil {
		b.Fatal(err)
	}
	now := time.Date(2026, 10, 6, 12, 0, 1, 0, time.UTC)
	events := make([]ProcessNetworkSyntheticEvent, 256)
	for i := range events {
		events[i] = ProcessNetworkSyntheticEvent{
			ObservedAt:  now,
			Class:       ProcessNetworkEventBytes,
			ProcessName: "bench",
			Protocol:    ProcessNetworkProtocolTCP,
			RemoteIP:    fmt.Sprintf("198.51.100.%d", i%200+1),
			RemotePort:  uint16(80 + i%20),
		}
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := agg.Ingest(ctx, events); err != nil {
			b.Fatal(err)
		}
	}
}
