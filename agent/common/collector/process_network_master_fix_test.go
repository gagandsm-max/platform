package collector

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestProcessNetworkProvenanceRequiredPreservedAndRejected(t *testing.T) {
	if _, err := NewProcessNetworkAggregator(ProcessNetworkAggregatorConfig{Window: time.Minute, TestAllowShortWindow: true}, nil); err == nil {
		t.Fatal("empty provenance must be rejected")
	}
	for _, bad := range []string{"", "  agent.linux.process_network.synthetic  ", "agent\x00linux", strings.Repeat("p", maxProcessNetworkProvenanceLen+1)} {
		_, err := NewProcessNetworkAggregator(ProcessNetworkAggregatorConfig{
			SourceProvenance:     bad,
			Window:               time.Minute,
			TestAllowShortWindow: true,
		}, nil)
		if err == nil {
			t.Fatalf("malformed provenance accepted: %q", bad)
		}
		if !strings.Contains(err.Error(), "PROCESS_NETWORK_PROVENANCE_INVALID") {
			t.Fatalf("want provenance error, got %v", err)
		}
	}

	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 1 {
		t.Fatalf("got %d evidence", len(out.Evidence))
	}
	if out.Evidence[0].SourceProvenance != ProcessNetworkSyntheticSourceProvenance {
		t.Fatalf("evidence provenance %q", out.Evidence[0].SourceProvenance)
	}
	for _, acc := range out.Accounting {
		if acc.SourceProvenance != ProcessNetworkSyntheticSourceProvenance {
			t.Fatalf("accounting provenance %q", acc.SourceProvenance)
		}
	}
}

func TestProcessNetworkFutureBackendProvenanceReusesAggregator(t *testing.T) {
	const backend = "agent.linux.process_network.backend_fixture"
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg, err := NewProcessNetworkAggregator(ProcessNetworkAggregatorConfig{
		SourceProvenance:     backend,
		Window:               time.Minute,
		TestAllowShortWindow: true,
	}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Evidence[0].SourceProvenance != backend {
		t.Fatalf("backend provenance not preserved: %q", out.Evidence[0].SourceProvenance)
	}
}

func TestProcessNetworkLateEventRemainsExactBucket(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withWindow(time.Minute))
	t0 := time.Date(2026, 10, 6, 12, 0, 10, 0, time.UTC)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseAt("nginx", "192.0.2.1", 80, t0)}); err != nil {
		t.Fatal(err)
	}
	out1, err := agg.FlushDue(context.Background(), t0.Truncate(time.Minute).Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	bucket := out1.Evidence[0].BucketStartAt
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseAt("nginx", "192.0.2.9", 80, t0)}); err != nil {
		t.Fatal(err)
	}
	next := baseAt("nginx", "192.0.2.2", 80, t0.Truncate(time.Minute).Add(time.Minute+time.Second))
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{next}); err != nil {
		t.Fatal(err)
	}
	out2, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sawExact := false
	for _, acc := range out2.Accounting {
		if acc.LateEventsCount > 0 {
			if !acc.BucketStartAt.Equal(bucket) {
				t.Fatalf("late attributed to %s want %s", acc.BucketStartAt, bucket)
			}
			sawExact = true
		}
	}
	if !sawExact {
		t.Fatalf("late exact bucket missing: %+v", out2.Accounting)
	}
}

func TestProcessNetworkLateAfterStubEvictionNeverBecomesOtherBucket(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	cfg := ProcessNetworkAggregatorConfig{
		SourceProvenance:     ProcessNetworkSyntheticSourceProvenance,
		Window:               time.Minute,
		TestAllowShortWindow: true,
		MaxClosedWindows:     1,
		MaxRetainedWindows:   maxProcessNetworkRetainedWindows,
		MaxAccountingStubs:   4,
	}
	agg, err := NewProcessNetworkAggregator(cfg, sink)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 6, 12, 0, 10, 0, time.UTC)
	var buckets []time.Time
	for i := 0; i < 2; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseAt("nginx", fmt.Sprintf("192.0.2.%d", i+1), 80, ts)}); err != nil {
			t.Fatal(err)
		}
		out, err := agg.FlushDue(context.Background(), ts.Truncate(time.Minute).Add(time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		buckets = append(buckets, out.Evidence[0].BucketStartAt)
	}
	lateA := baseAt("nginx", "198.51.100.9", 80, base)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{lateA}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range out.Evidence {
		if rec.RemoteIP.String() == "198.51.100.9" {
			t.Fatal("late must not reopen as ordinary evidence")
		}
		if rec.BucketStartAt.Equal(buckets[1]) && rec.DroppedRecordsCount > 0 {
			t.Fatal("late for A must not appear as a drop on bucket B evidence")
		}
	}
	sawLate := false
	for _, acc := range out.Accounting {
		if acc.LateEventsCount > 0 && acc.BucketStartAt.Equal(buckets[1]) {
			t.Fatalf("late for A stamped onto later bucket B: %+v", acc)
		}
		if acc.LateEventsCount > 0 && !acc.BucketStartAt.Equal(buckets[0]) {
			t.Fatalf("late must remain bucket A %s, got %s", buckets[0], acc.BucketStartAt)
		}
		if acc.LateEventsCount > 0 && acc.BucketStartAt.Equal(buckets[0]) {
			sawLate = true
		}
	}
	if !sawLate {
		t.Fatalf("late for bucket A missing: %+v", out.Accounting)
	}
}

func TestProcessNetworkRetentionShedTiedToRequestedBucket(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	// Combined live+closed capacity must leave at least one live slot (MaxRetainedWindows > MaxClosedWindows).
	cfg := ProcessNetworkAggregatorConfig{
		SourceProvenance:     ProcessNetworkSyntheticSourceProvenance,
		Window:               time.Minute,
		TestAllowShortWindow: true,
		MaxRetainedWindows:   2,
		MaxClosedWindows:     1,
		MaxAccountingStubs:   4,
	}
	agg, err := NewProcessNetworkAggregator(cfg, sink)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 10, 6, 12, 0, 10, 0, time.UTC)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseAt("nginx", "192.0.2.1", 80, t0)}); err != nil {
		t.Fatal(err)
	}
	out1, err := agg.FlushDue(context.Background(), t0.Truncate(time.Minute).Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	closedBucket := out1.Evidence[0].BucketStartAt
	// Occupy remaining live headroom so combined live+closed == MaxRetainedWindows.
	tLive := t0.Truncate(time.Minute).Add(time.Minute + time.Second)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseAt("nginx", "192.0.2.7", 80, tLive)}); err != nil {
		t.Fatal(err)
	}
	tShed := t0.Truncate(time.Minute).Add(2*time.Minute + time.Second)
	wantShedBucket := alignUTC(tShed, time.Minute)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseAt("nginx", "192.0.2.8", 80, tShed)}); err != nil {
		t.Fatal(err)
	}
	out2, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range out2.Evidence {
		if rec.RemoteIP.String() == "192.0.2.8" {
			t.Fatalf("shed event must not create dest evidence: %+v", rec)
		}
	}
	saw := false
	for _, acc := range out2.Accounting {
		if acc.ShedRecordsCount > 0 {
			if acc.BucketStartAt.Equal(closedBucket) {
				t.Fatal("shed for new bucket must not attach to previously closed bucket")
			}
			if !acc.BucketStartAt.Equal(wantShedBucket) {
				t.Fatalf("shed bucket %s want %s", acc.BucketStartAt, wantShedBucket)
			}
			saw = true
		}
	}
	if !saw {
		t.Fatalf("requested-bucket shed missing: %+v", out2.Accounting)
	}
}

func TestProcessNetworkZeroTimeNeverAttributesLaterWindow(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	zero := ProcessNetworkSyntheticEvent{
		Class:       ProcessNetworkEventBytes,
		ProcessName: "nginx",
		Protocol:    ProcessNetworkProtocolTCP,
		RemoteIP:    "192.0.2.1",
		RemotePort:  80,
	}
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{zero}); !errors.Is(err, ErrProcessNetworkObservedAtInvalid) {
		t.Fatalf("zero-time: %v", err)
	}
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 1 {
		t.Fatalf("got %d", len(out.Evidence))
	}
	if out.Evidence[0].DroppedRecordsCount != 0 {
		t.Fatalf("zero-time must not drop onto later evidence: %d", out.Evidence[0].DroppedRecordsCount)
	}
	for _, acc := range out.Accounting {
		if acc.DroppedRecordsCount > 0 {
			t.Fatalf("zero-time must not appear on later window accounting: %+v", acc)
		}
	}
}

func TestProcessNetworkAccountingStubAttackRemainsBounded(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	cfg := ProcessNetworkAggregatorConfig{
		SourceProvenance:     ProcessNetworkSyntheticSourceProvenance,
		Window:               time.Minute,
		TestAllowShortWindow: true,
		MaxRetainedWindows:   2,
		MaxClosedWindows:     1,
		MaxAccountingStubs:   3,
	}
	agg, err := NewProcessNetworkAggregator(cfg, sink)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 6, 12, 0, 10, 0, time.UTC)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseAt("nginx", "192.0.2.1", 80, base)}); err != nil {
		t.Fatal(err)
	}
	if _, err := agg.FlushDue(context.Background(), base.Truncate(time.Minute).Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	rejected := 0
	for i := 1; i <= 32; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseAt("scan", fmt.Sprintf("203.0.113.%d", i), 80, ts)})
		if errors.Is(err, ErrProcessNetworkAccountingBoundExceeded) {
			rejected++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if rejected == 0 {
		t.Fatal("random timestamp attack must eventually fail closed")
	}
	if n := agg.RetainedWindowCount(); n > cfg.MaxRetainedWindows+cfg.MaxClosedWindows+cfg.MaxAccountingStubs {
		t.Fatalf("unbounded maps %d", n)
	}
	if len(agg.accountingStubs) > cfg.MaxAccountingStubs {
		t.Fatalf("accounting stubs %d", len(agg.accountingStubs))
	}
}

func TestProcessNetworkConfigRejectsOverridesAboveHardCeilings(t *testing.T) {
	cases := []ProcessNetworkAggregatorConfig{
		{MaxGlobalKeys: maxProcessNetworkGlobalKeys + 1},
		{MaxKeysPerWorkload: maxProcessNetworkKeysPerWorkload + 1},
		{MaxInputBatch: maxProcessNetworkInputBatch + 1},
		{MaxRetainedWindows: maxProcessNetworkRetainedWindows + 1},
		{MaxClosedWindows: maxProcessNetworkClosedWindows + 1},
		{MaxAccountingStubs: maxProcessNetworkAccountingStubs + 1},
	}
	for i, cfg := range cases {
		cfg.SourceProvenance = ProcessNetworkSyntheticSourceProvenance
		cfg.Window = time.Minute
		cfg.TestAllowShortWindow = true
		if _, err := NewProcessNetworkAggregator(cfg, nil); err == nil || !strings.Contains(err.Error(), "PROCESS_NETWORK_BOUND_INVALID") {
			t.Fatalf("case %d: unsafe override accepted: %v", i, err)
		}
	}
}

// F4: retention configs with no live-window headroom must be rejected; valid configs must recover.
func TestProcessNetworkRetentionRejectsNoLiveHeadroom(t *testing.T) {
	invalid := []struct {
		retained int
		closed   int
	}{
		{1, 1},
		{2, 2},
	}
	for i, tc := range invalid {
		_, err := NewProcessNetworkAggregator(ProcessNetworkAggregatorConfig{
			SourceProvenance:     ProcessNetworkSyntheticSourceProvenance,
			Window:               time.Minute,
			TestAllowShortWindow: true,
			MaxRetainedWindows:   tc.retained,
			MaxClosedWindows:     tc.closed,
		}, nil)
		if err == nil || !strings.Contains(err.Error(), "PROCESS_NETWORK_BOUND_INVALID") {
			t.Fatalf("case %d (%d/%d): no-live-headroom config accepted: %v", i, tc.retained, tc.closed, err)
		}
	}

	valid := []ProcessNetworkAggregatorConfig{
		{MaxRetainedWindows: 2, MaxClosedWindows: 1},
		{MaxRetainedWindows: maxProcessNetworkRetainedWindows, MaxClosedWindows: maxProcessNetworkClosedWindows},
		{}, // defaults 4/2
	}
	for i, cfg := range valid {
		cfg.SourceProvenance = ProcessNetworkSyntheticSourceProvenance
		cfg.Window = time.Minute
		cfg.TestAllowShortWindow = true
		if _, err := NewProcessNetworkAggregator(cfg, nil); err != nil {
			t.Fatalf("valid case %d rejected: %v", i, err)
		}
	}
}

func TestProcessNetworkRetentionRecoversAfterPressure(t *testing.T) {
	for _, cfg := range []ProcessNetworkAggregatorConfig{
		{MaxRetainedWindows: 2, MaxClosedWindows: 1, MaxAccountingStubs: 4},
		{MaxRetainedWindows: maxProcessNetworkRetainedWindows, MaxClosedWindows: maxProcessNetworkClosedWindows, MaxAccountingStubs: 4},
	} {
		cfg.SourceProvenance = ProcessNetworkSyntheticSourceProvenance
		cfg.Window = time.Minute
		cfg.TestAllowShortWindow = true
		agg, err := NewProcessNetworkAggregator(cfg, &LocalRecordingStructuredEvidenceSink{})
		if err != nil {
			t.Fatal(err)
		}
		base := time.Date(2026, 10, 6, 12, 0, 10, 0, time.UTC)
		// Fill combined live+closed capacity, then force shed.
		if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseAt("nginx", "192.0.2.1", 80, base)}); err != nil {
			t.Fatal(err)
		}
		if _, err := agg.FlushDue(context.Background(), base.Truncate(time.Minute).Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		for i := 1; i < cfg.MaxRetainedWindows; i++ {
			ts := base.Add(time.Duration(i) * time.Minute)
			if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseAt("nginx", fmt.Sprintf("192.0.2.%d", i+1), 80, ts)}); err != nil {
				t.Fatal(err)
			}
		}
		shedTS := base.Add(time.Duration(cfg.MaxRetainedWindows) * time.Minute)
		if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseAt("nginx", "198.51.100.1", 80, shedTS)}); err != nil {
			t.Fatal(err)
		}
		if _, err := agg.FlushAll(context.Background()); err != nil {
			t.Fatal(err)
		}
		// After flush, live capacity must recover for ordinary aggregation.
		recoverTS := shedTS.Add(time.Minute)
		if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseAt("nginx", "203.0.113.9", 80, recoverTS)}); err != nil {
			t.Fatal(err)
		}
		out, err := agg.FlushAll(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, rec := range out.Evidence {
			if rec.RemoteIP.String() == "203.0.113.9" {
				found = true
			}
		}
		if !found {
			t.Fatalf("normal aggregation did not recover after retention pressure (cfg retained=%d closed=%d): %+v",
				cfg.MaxRetainedWindows, cfg.MaxClosedWindows, out)
		}
	}
}

func TestProcessNetworkOverflowNeverPublishesActiveGauge(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withKeys(64, 1))
	now := pnNow()
	events := []ProcessNetworkSyntheticEvent{
		{ObservedAt: now, Class: ProcessNetworkEventOpen, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443},
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.2", RemotePort: 443, BytesTx: uint64Ptr(1)},
		{ObservedAt: now, Class: ProcessNetworkEventClose, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443, DurationMs: uint64Ptr(10)},
	}
	if err := agg.Ingest(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, acc := range out.Accounting {
		for _, ov := range acc.Overflows {
			if ov.ActiveConnections != nil {
				t.Fatalf("overflow active gauge is not lifecycle-safe: %+v", ov)
			}
		}
	}
}

func TestProcessNetworkSampledWorkloadAccountingNotDoubleCountedByOverflow(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withKeys(64, 1))
	now := pnNow()
	sampled := baseAt("app", "192.0.2.1", 443, now)
	sampled.Sampled = true
	replacement := baseAt("app", "192.0.2.2", 443, now)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{sampled, replacement}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, acc := range out.Accounting {
		if acc.Workload == nil {
			continue
		}
		found = true
		if acc.SampledEventsCount != 1 {
			t.Fatalf("sampled workload total double counted overflow subset: %d", acc.SampledEventsCount)
		}
		ov := overflowSummary([]ProcessNetworkWindowAccounting{acc}, ProcessNetworkProtocolTCP)
		if ov == nil || ov.SampledEventsCount != 1 {
			t.Fatalf("overflow sampled subset missing or wrong: %+v", ov)
		}
	}
	if !found {
		t.Fatal("workload accounting missing")
	}
}

func TestProcessNetworkOverflowPreservesTCPMetricsAndNotUDP(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withKeys(64, 1))
	now := pnNow()
	dur := uint64(40)
	rtt := uint32(1500)
	rtx := uint32(4)
	tx := uint64(77)
	tcpVictim := []ProcessNetworkSyntheticEvent{
		{ObservedAt: now, Class: ProcessNetworkEventOpen, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443},
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443, BytesTx: &tx},
		{ObservedAt: now, Class: ProcessNetworkEventRTT, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443, RTTUs: &rtt},
		{ObservedAt: now, Class: ProcessNetworkEventRetransmit, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443, RetransmitCount: &rtx},
		{ObservedAt: now, Class: ProcessNetworkEventClose, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443, DurationMs: &dur},
	}
	tcpReplacement := ProcessNetworkSyntheticEvent{
		ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP,
		RemoteIP: "192.0.2.2", RemotePort: 443, BytesTx: uint64Ptr(3),
	}
	if err := agg.Ingest(context.Background(), append(tcpVictim, tcpReplacement)); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rec, ok := findIP(out.Evidence, "192.0.2.2"); !ok {
		t.Fatal("replacement missing")
	} else if rec.BytesTx == nil || *rec.BytesTx != 3 {
		t.Fatalf("evicted metrics reached replacement: %+v", rec.BytesTx)
	} else if rec.RetransmitCount != nil || (rec.DurationTotalMs != nil && *rec.DurationTotalMs == 40) {
		t.Fatalf("victim TCP metrics must not appear on replacement: %+v", rec)
	}
	ov := overflowSummary(out.Accounting, ProcessNetworkProtocolTCP)
	if ov == nil {
		t.Fatal("tcp overflow missing")
	}
	if ov.BytesTx == nil || *ov.BytesTx != 77 {
		t.Fatalf("overflow bytes %v", ov.BytesTx)
	}
	if ov.ActiveConnections != nil {
		t.Fatalf("overflow must not publish non-additive active-connection gauge: %+v", ov.ActiveConnections)
	}
	if ov.DurationTotalMs == nil || *ov.DurationTotalMs != 40 {
		t.Fatalf("overflow duration %+v", ov)
	}
	if ov.MeanRTTUs == nil || *ov.MeanRTTUs != 1500 {
		t.Fatalf("overflow mean rtt %+v", ov.MeanRTTUs)
	}
	if ov.RetransmitCount == nil || *ov.RetransmitCount != 4 {
		t.Fatalf("overflow rtx %+v", ov.RetransmitCount)
	}
	if ov.EvictedCandidateCount != 1 {
		t.Fatalf("evicted candidates %d", ov.EvictedCandidateCount)
	}

	sink2 := &LocalRecordingStructuredEvidenceSink{}
	agg2 := mustAgg(t, sink2, withKeys(64, 1))
	udpA := ProcessNetworkSyntheticEvent{
		ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "dns", Protocol: ProcessNetworkProtocolUDP,
		RemoteIP: "192.0.2.10", RemotePort: 53, BytesTx: &tx, RTTUs: &rtt, RetransmitCount: &rtx, DurationMs: &dur,
	}
	udpB := udpA
	udpB.RemoteIP = "192.0.2.11"
	if err := agg2.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{udpA, udpB}); err != nil {
		t.Fatal(err)
	}
	out2, err := agg2.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ovUDP := overflowSummary(out2.Accounting, ProcessNetworkProtocolUDP)
	if ovUDP == nil {
		t.Fatal("udp overflow missing")
	}
	if ovUDP.ActiveConnections != nil || ovUDP.DurationTotalMs != nil || ovUDP.MeanRTTUs != nil ||
		ovUDP.RetransmitCount != nil || ovUDP.NewConnections != nil || ovUDP.ClosedConnections != nil || ovUDP.ConnectionFailures != nil {
		t.Fatalf("UDP overflow fabricated TCP metrics: %+v", ovUDP)
	}
}

func TestProcessNetworkSaturationCheckedArithmetic(t *testing.T) {
	m := pnMetrics{hasTx: true, bytesTx: math.MaxUint64 - 1}
	tx := uint64(5)
	if !applyMetrics(&m, ProcessNetworkSyntheticEvent{Class: ProcessNetworkEventBytes, Protocol: ProcessNetworkProtocolUDP, BytesTx: &tx}) {
		t.Fatal("uint64 bytes saturation must be reported")
	}
	if m.bytesTx != math.MaxUint64 || !m.saturated {
		t.Fatalf("wrapped or unmarked: tx=%d sat=%v", m.bytesTx, m.saturated)
	}
	dst := pnMetrics{hasTx: true, bytesTx: math.MaxUint64 - 2}
	if !foldMetrics(&dst, pnMetrics{hasTx: true, bytesTx: 10}) {
		t.Fatal("fold uint64 saturation")
	}
	if dst.bytesTx != math.MaxUint64 {
		t.Fatalf("fold wrap %d", dst.bytesTx)
	}

	u := pnMetrics{hasNew: true, newConn: math.MaxUint32}
	if !applyMetrics(&u, ProcessNetworkSyntheticEvent{Class: ProcessNetworkEventOpen, Protocol: ProcessNetworkProtocolTCP}) {
		t.Fatal("uint32 newConn saturation")
	}
	if u.newConn != math.MaxUint32 || u.active == 0 {
		t.Fatalf("uint32 wrap new=%d active=%d", u.newConn, u.active)
	}
	u.active = math.MaxUint32
	if !applyMetrics(&u, ProcessNetworkSyntheticEvent{Class: ProcessNetworkEventOpen, Protocol: ProcessNetworkProtocolTCP}) {
		t.Fatal("active saturation")
	}
	if u.active != math.MaxUint32 {
		t.Fatalf("active wrapped to %d", u.active)
	}

	c := newCandidate(netip.MustParseAddr("192.0.2.1"), 80, ProcessNetworkProtocolTCP)
	c.heat = math.MaxUint64
	satIncHeat(c)
	if c.heat != math.MaxUint64 || !c.saturated {
		t.Fatalf("heat wrap heat=%d sat=%v", c.heat, c.saturated)
	}
	satIncHeat(c)
	if c.heat != math.MaxUint64 {
		t.Fatal("heat inc wrap")
	}

	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	n := uint32(math.MaxUint32)
	drop := ProcessNetworkSyntheticEvent{
		ObservedAt:   pnNow(),
		Class:        ProcessNetworkEventBackendDrop,
		DroppedCount: &n,
	}
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{drop, drop}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, acc := range out.Accounting {
		if acc.Workload == nil && acc.DroppedRecordsCount == math.MaxUint32 {
			found = true
			if !acc.PartialFlag || !acc.ResourceGuardedFlag {
				t.Fatalf("saturation quality missing: %+v", acc)
			}
			if acc.Quality != ProcessNetworkQualityResourceGuarded && acc.Quality != ProcessNetworkQualityPartial {
				if qualityRank(acc.Quality) < qualityRank(ProcessNetworkQualityResourceGuarded) {
					t.Fatalf("want RESOURCE_GUARDED-class quality, got %s", acc.Quality)
				}
			}
		}
	}
	if !found {
		t.Fatalf("saturated drop accounting missing: %+v", out.Accounting)
	}
}

func TestProcessNetworkWholeSecondWindowInvariant(t *testing.T) {
	if _, err := NewProcessNetworkAggregator(ProcessNetworkAggregatorConfig{
		SourceProvenance:     ProcessNetworkSyntheticSourceProvenance,
		Window:               1500 * time.Millisecond,
		TestAllowShortWindow: true,
	}, nil); err == nil || !strings.Contains(err.Error(), "PROCESS_NETWORK_WINDOW_INVALID") {
		t.Fatalf("fractional window must be rejected, got %v", err)
	}

	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	acc := ProcessNetworkWindowAccounting{
		SchemaVersion:    ProcessNetworkSchemaVersion,
		SourceProvenance: ProcessNetworkSyntheticSourceProvenance,
		BucketStartAt:    start,
		BucketEndAt:      start.Add(1500 * time.Millisecond),
		BucketSeconds:    1,
		Quality:          ProcessNetworkQualityPartial,
		PartialFlag:      true,
	}
	if err := acc.Validate(); err == nil || !strings.Contains(err.Error(), "PROCESS_NETWORK_BUCKET_INVALID") {
		t.Fatalf("fractional bucket interval must be rejected, got %v", err)
	}
}

func TestProcessNetworkTypedValidationStrengthened(t *testing.T) {
	start := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	baseAcc := ProcessNetworkWindowAccounting{
		SchemaVersion:    ProcessNetworkSchemaVersion,
		SourceProvenance: ProcessNetworkSyntheticSourceProvenance,
		BucketStartAt:    start,
		BucketEndAt:      end,
		BucketSeconds:    60,
		Quality:          ProcessNetworkQualityPartial,
		PartialFlag:      true,
	}
	bad := baseAcc
	bad.BucketStartAt = time.Time{}
	if err := bad.Validate(); err == nil {
		t.Fatal("zero start")
	}
	bad = baseAcc
	bad.BucketEndAt = start.Add(-time.Second)
	if err := bad.Validate(); err == nil {
		t.Fatal("end before start")
	}
	bad = baseAcc
	bad.BucketSeconds = 0
	if err := bad.Validate(); err == nil {
		t.Fatal("zero duration")
	}
	bad = baseAcc
	bad.BucketSeconds = 30
	if err := bad.Validate(); err == nil || !strings.Contains(err.Error(), "PROCESS_NETWORK_BUCKET_INVALID") {
		t.Fatalf("inconsistent duration: %v", err)
	}
	bad = baseAcc
	bad.SourceProvenance = "  x"
	if err := bad.Validate(); err == nil {
		t.Fatal("whitespace provenance")
	}
	bad = baseAcc
	bad.Overflows = []ProcessNetworkOverflowSummary{{Protocol: "SCTP"}}
	if err := bad.Validate(); err == nil {
		t.Fatal("invalid overflow protocol")
	}
	bad = baseAcc
	bad.Overflows = []ProcessNetworkOverflowSummary{{
		Protocol:          ProcessNetworkProtocolUDP,
		ActiveConnections: uint32Ptr(1),
	}}
	if err := bad.Validate(); err == nil {
		t.Fatal("udp tcp metrics")
	}

	rec := ProcessNetworkEvidence{
		SchemaVersion:    ProcessNetworkSchemaVersion,
		SourceProvenance: ProcessNetworkSyntheticSourceProvenance,
		BucketStartAt:    start,
		BucketEndAt:      end,
		BucketSeconds:    60,
		Workload:         ProcessNetworkWorkloadIdentity{Kind: SelectorKindProcessName, Value: "nginx", ProcessName: "nginx"},
		RemoteIP:         netip.MustParseAddr("192.0.2.1"),
		RemotePort:       80,
		Protocol:         ProcessNetworkProtocolTCP,
		IPVersion:        "IPv4",
		Quality:          ProcessNetworkQualityValid,
	}
	mapped := netip.MustParseAddr("::ffff:10.0.0.1")
	rec.LocalIP = &mapped
	if err := rec.Validate(); err == nil {
		t.Fatal("mapped local IP")
	}
	v4 := netip.MustParseAddr("10.0.0.1")
	rec.LocalIP = &v4
	zero := uint16(0)
	rec.LocalPort = &zero
	if err := rec.Validate(); err == nil {
		t.Fatal("local port 0")
	}
	rec.LocalPort = nil
	rec.LocalIP = nil
	rec.SourceProvenance = "agent.linux.process_network.synthetic\n"
	if err := rec.Validate(); err == nil {
		t.Fatal("control provenance")
	}
	rec.SourceProvenance = ProcessNetworkSyntheticSourceProvenance
	rec.BucketSeconds = 15
	if err := rec.Validate(); err == nil {
		t.Fatal("evidence bucket duration mismatch")
	}
}
