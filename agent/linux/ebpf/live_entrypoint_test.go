//go:build live_ebpf

package ebpf

import (
	"context"
	"os"
	"testing"
	"time"

	"platform/agent/common/collector"
)

// Live kernel certification is owned by Antigravity on a privileged HUT.
// This entrypoint is intentionally excluded from default CI via the live_ebpf
// build tag. Example:
//
//	go test -tags live_ebpf ./linux/ebpf -run TestLiveBackendStartStop -count=1 -v
func TestLiveBackendStartStop(t *testing.T) {
	if os.Getuid() != 0 && os.Getenv("PLATFORM_AGENT_LIVE_EBPF") != "1" {
		t.Skip("set PLATFORM_AGENT_LIVE_EBPF=1 and provide CAP_BPF/CAP_PERFMON (Antigravity HUT)")
	}
	sink := &collector.LocalRecordingStructuredEvidenceSink{}
	agg, err := collector.NewProcessNetworkAggregator(collector.ProcessNetworkAggregatorConfig{
		SourceProvenance: collector.ProcessNetworkSyntheticSourceProvenance,
		Window:           AggregationWindow,
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewBackend(BackendConfig{
		Config: Config{
			Enabled:            true,
			PreflightTimeout:   5 * time.Second,
			RecheckMin:         time.Minute,
			RecheckMax:         time.Hour,
			RequestedMapBytes:  RequestedMapBytesEstimate,
			RequestedRingBytes: RequestedRingBytesExact,
			LowWaterSatisfied:  true,
		},
		Aggregator: agg,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := b.Start(ctx); err != nil {
		t.Fatalf("Start: %v capability=%+v", err, b.Capability())
	}
	if b.Capability().State != collector.ProcessNetworkCapabilityActive {
		t.Fatalf("want ACTIVE got %+v", b.Capability())
	}
	time.Sleep(2 * time.Second)
	if err := b.StopContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.StopContext(ctx); err != nil {
		t.Fatal(err)
	}
}
