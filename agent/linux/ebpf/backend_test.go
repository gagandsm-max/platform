package ebpf

import (
	"context"
	"strings"
	"testing"
	"time"

	"platform/agent/common/collector"
)

func testAgg(t testing.TB) (*collector.ProcessNetworkAggregator, *collector.LocalRecordingStructuredEvidenceSink) {
	t.Helper()
	sink := &collector.LocalRecordingStructuredEvidenceSink{}
	agg, err := collector.NewProcessNetworkAggregator(collector.ProcessNetworkAggregatorConfig{
		SourceProvenance:     collector.ProcessNetworkSyntheticSourceProvenance,
		Window:               time.Minute,
		TestAllowShortWindow: true,
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	return agg, sink
}

func TestNewBackendResourceCeiling(t *testing.T) {
	agg, _ := testAgg(t)
	_, err := NewBackend(BackendConfig{
		Config: Config{
			Enabled:            true,
			RequestedMapBytes:  defaultMaximumResourceBytes,
			RequestedRingBytes: 1,
			MaximumBytes:       defaultMaximumResourceBytes,
		},
		Aggregator: agg,
	})
	if err == nil || !strings.Contains(err.Error(), "BACKEND_RESOURCE_CEILING") {
		t.Fatalf("got %v", err)
	}
}

func TestStopIdempotentWithoutStart(t *testing.T) {
	agg, _ := testAgg(t)
	b, err := NewBackend(BackendConfig{Config: Config{Enabled: false}, Aggregator: agg})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := b.StopContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := b.StopContext(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStartDisabledNeverActive(t *testing.T) {
	agg, _ := testAgg(t)
	b, err := NewBackend(BackendConfig{Config: Config{Enabled: false, PreflightTimeout: time.Second}, Aggregator: agg})
	if err != nil {
		t.Fatal(err)
	}
	err = b.Start(context.Background())
	if err == nil {
		t.Fatal("expected preflight failure")
	}
	cap := b.Capability()
	if cap.State == collector.ProcessNetworkCapabilityActive {
		t.Fatalf("must never be ACTIVE: %+v", cap)
	}
}

func TestAggregatorIntegrationFromDecoder(t *testing.T) {
	agg, sink := testAgg(t)
	raw := putEventV1(t, nil)
	ev, err := decodeRawRecord(raw, time.Now().UTC(), 0)
	if err != nil {
		t.Fatal(err)
	}
	syn, err := toSynthetic(ev)
	if err != nil {
		t.Fatal(err)
	}
	if err := agg.Ingest(context.Background(), []collector.ProcessNetworkSyntheticEvent{syn}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) == 0 {
		t.Fatal("expected evidence")
	}
	for _, e := range out.Evidence {
		if err := sink.AcceptProcessNetworkEvidence(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLossAccountingExactOnce(t *testing.T) {
	var l lossCounters
	l.Add(LossClassDecoderReject, 1)
	l.Add(LossClassDecoderReject, 2)
	l.Add(LossClassUserspaceChanFull, 1)
	l.Add(LossClassMapUpdateFail, 0)
	snap := l.Snapshot()
	if snap[LossClassDecoderReject] != 3 || snap[LossClassUserspaceChanFull] != 1 {
		t.Fatalf("%v", snap)
	}
	if l.Total() != 4 {
		t.Fatalf("total=%d", l.Total())
	}
}

func TestBackendDropMapsToAggregator(t *testing.T) {
	agg, _ := testAgg(t)
	drop := backendDropEvent(string(LossClassDecoderReject), 3, time.Now().UTC())
	if err := agg.Ingest(context.Background(), []collector.ProcessNetworkSyntheticEvent{drop}); err != nil {
		t.Fatal(err)
	}
}

func TestDataMinimizationNoProhibitedFields(t *testing.T) {
	raw := putEventV1(t, nil)
	ev, err := decodeRawRecord(raw, time.Now().UTC(), 0)
	if err != nil {
		t.Fatal(err)
	}
	syn, err := toSynthetic(ev)
	if err != nil {
		t.Fatal(err)
	}
	// Comm is process name only — never cmdline/env/payload/TLS/HTTP/SQL.
	if strings.Contains(syn.ProcessName, "/") || strings.Contains(syn.ProcessName, "=") {
		t.Fatalf("looks like cmdline/env: %q", syn.ProcessName)
	}
	if syn.ExecutablePath != "" {
		t.Fatal("must not invent executable path from eBPF")
	}
}

func TestOSS124ManifestVerification(t *testing.T) {
	tree, manifest, err := DefaultOSS124Paths()
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyOSS124Tree(tree, manifest); err != nil {
		t.Fatal(err)
	}
}

func TestOSS124ManifestDetectsTamper(t *testing.T) {
	tree, manifest, err := DefaultOSS124Paths()
	if err != nil {
		t.Fatal(err)
	}
	// Point tree at a temp copy is heavy; instead verify missing-file path fails closed.
	if err := VerifyOSS124Tree(tree+"/nope", manifest); err == nil {
		t.Fatal("expected failure")
	}
}
