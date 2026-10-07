package collector

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"
)

type aggOpt func(*ProcessNetworkAggregatorConfig)

func withKeys(global, perWorkload int) aggOpt {
	return func(c *ProcessNetworkAggregatorConfig) {
		c.MaxGlobalKeys = global
		c.MaxKeysPerWorkload = perWorkload
	}
}

func withWindow(d time.Duration) aggOpt {
	return func(c *ProcessNetworkAggregatorConfig) {
		c.Window = d
		c.TestAllowShortWindow = true
	}
}

func mustAgg(t *testing.T, sink StructuredEvidenceSink, opts ...aggOpt) *ProcessNetworkAggregator {
	t.Helper()
	cfg := ProcessNetworkAggregatorConfig{
		SourceProvenance:     ProcessNetworkSyntheticSourceProvenance,
		Window:               time.Minute,
		TestAllowShortWindow: true,
	}
	for _, o := range opts {
		o(&cfg)
	}
	agg, err := NewProcessNetworkAggregator(cfg, sink)
	if err != nil {
		t.Fatal(err)
	}
	return agg
}

func pnNow() time.Time {
	return time.Date(2026, 10, 6, 12, 0, 1, 0, time.UTC)
}

func baseEvent(name, ip string, port uint16, proto ProcessNetworkProtocol) ProcessNetworkSyntheticEvent {
	return ProcessNetworkSyntheticEvent{
		ObservedAt:  pnNow(),
		Class:       ProcessNetworkEventBytes,
		ProcessName: name,
		Protocol:    proto,
		RemoteIP:    ip,
		RemotePort:  port,
	}
}

func TestProcessNetworkPIDChurnOmitsPID(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	p1, p2 := uint32(11), uint32(22)
	e1 := baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
	e1.ServiceID = "nginx.service"
	e1.PID = &p1
	e2 := e1
	e2.PID = &p2
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{e1, e2}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 1 {
		t.Fatalf("want merge, got %d", len(out.Evidence))
	}
	if out.Evidence[0].PID != nil {
		t.Fatalf("mixed PID must be omitted, got %d", *out.Evidence[0].PID)
	}
}

func TestProcessNetworkDifferentWorkloadsDoNotMerge(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	a := baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
	b := baseEvent("sshd", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{a, b}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 2 {
		t.Fatalf("got %d", len(out.Evidence))
	}
}

func TestProcessNetworkContainerAndNetnsSplit(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	ns1, ns2 := uint64(100), uint64(200)
	a := baseEvent("app", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
	a.ContainerID = "ctrA"
	b := a
	b.ContainerID = "ctrB"
	c := a
	c.NetworkNamespace = &ns1
	d := a
	d.NetworkNamespace = &ns2
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{a, b, c, d}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 4 {
		t.Fatalf("container/netns must split, got %d", len(out.Evidence))
	}
}

func TestProcessNetworkSameKeyMergeDifferentIPPortProtocol(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	tx := uint64(5)
	a := baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
	a.BytesTx = &tx
	b := a
	c := baseEvent("nginx", "192.0.2.2", 80, ProcessNetworkProtocolTCP)
	d := baseEvent("nginx", "192.0.2.1", 443, ProcessNetworkProtocolTCP)
	e := baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolUDP)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{a, b, c, d, e}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 4 {
		t.Fatalf("want 4 keys (merged same + ip + port + proto), got %d", len(out.Evidence))
	}
	var merged *ProcessNetworkEvidence
	for i := range out.Evidence {
		r := &out.Evidence[i]
		if r.RemoteIP.String() == "192.0.2.1" && r.RemotePort == 80 && r.Protocol == ProcessNetworkProtocolTCP {
			merged = r
		}
	}
	if merged == nil || merged.BytesTx == nil || *merged.BytesTx != 10 {
		t.Fatalf("same key must merge bytes: %+v", merged)
	}
}

func TestProcessNetworkMixedLocalEndpointOmission(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	p1, p2 := uint16(40000), uint16(40001)
	a := baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
	a.LocalIP = "10.0.0.1"
	a.LocalPort = &p1
	b := a
	b.LocalIP = "10.0.0.2"
	b.LocalPort = &p2
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{a, b}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 1 {
		t.Fatal("same remote key")
	}
	if out.Evidence[0].LocalIP != nil || out.Evidence[0].LocalPort != nil {
		t.Fatal("mixed local endpoint must be omitted")
	}
}

func TestProcessNetworkTCPMeasuredMetricsUDPDoesNotFabricate(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	now := pnNow()
	tx := uint64(20)
	rx := uint64(0)
	dur := uint64(15)
	rtt := uint32(1200)
	rtx := uint32(2)
	tcp := []ProcessNetworkSyntheticEvent{
		{ObservedAt: now, Class: ProcessNetworkEventOpen, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443},
		{ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443, BytesTx: &tx, BytesRx: &rx},
		{ObservedAt: now, Class: ProcessNetworkEventRTT, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443, RTTUs: &rtt},
		{ObservedAt: now, Class: ProcessNetworkEventRetransmit, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443, RetransmitCount: &rtx},
		{ObservedAt: now, Class: ProcessNetworkEventFail, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443},
		{ObservedAt: now, Class: ProcessNetworkEventClose, ProcessName: "app", Protocol: ProcessNetworkProtocolTCP, RemoteIP: "192.0.2.1", RemotePort: 443, DurationMs: &dur},
	}
	udp := ProcessNetworkSyntheticEvent{
		ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "app", Protocol: ProcessNetworkProtocolUDP,
		RemoteIP: "192.0.2.1", RemotePort: 53, BytesTx: &tx, BytesRx: &rx,
		RTTUs: &rtt, RetransmitCount: &rtx, DurationMs: &dur,
	}
	if err := agg.Ingest(context.Background(), append(tcp, udp)); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var tcpRec, udpRec *ProcessNetworkEvidence
	for i := range out.Evidence {
		r := &out.Evidence[i]
		if r.Protocol == ProcessNetworkProtocolTCP {
			tcpRec = r
		} else {
			udpRec = r
		}
	}
	if tcpRec == nil || udpRec == nil {
		t.Fatal("need both")
	}
	if tcpRec.BytesTx == nil || *tcpRec.BytesTx != 20 || tcpRec.BytesRx == nil || *tcpRec.BytesRx != 0 {
		t.Fatal("TCP measured zero rx vs measured tx")
	}
	if tcpRec.NewConnections == nil || tcpRec.ClosedConnections == nil || tcpRec.ConnectionFailures == nil {
		t.Fatal("TCP connection counters")
	}
	if tcpRec.MeanRTTUs == nil || *tcpRec.MeanRTTUs != 1200 || tcpRec.RetransmitCount == nil || *tcpRec.RetransmitCount != 2 {
		t.Fatal("TCP rtt/rtx")
	}
	if udpRec.BytesTx == nil || udpRec.MeanRTTUs != nil || udpRec.RetransmitCount != nil ||
		udpRec.NewConnections != nil || udpRec.ActiveConnections != nil || udpRec.ConnectionFailures != nil {
		t.Fatalf("UDP must not fabricate TCP metrics: %+v", udpRec)
	}
}

func TestProcessNetworkPerWorkloadAndGlobalCaps(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withKeys(3, 2))
	now := pnNow()
	var events []ProcessNetworkSyntheticEvent
	for i := 1; i <= 6; i++ {
		events = append(events, ProcessNetworkSyntheticEvent{
			ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "w1",
			Protocol: ProcessNetworkProtocolTCP, RemoteIP: fmt.Sprintf("192.0.2.%d", i), RemotePort: 80,
		})
	}
	events = append(events, ProcessNetworkSyntheticEvent{
		ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "w2",
		Protocol: ProcessNetworkProtocolTCP, RemoteIP: "198.51.100.1", RemotePort: 80,
	})
	events = append(events, ProcessNetworkSyntheticEvent{
		ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "w3",
		Protocol: ProcessNetworkProtocolTCP, RemoteIP: "203.0.113.1", RemotePort: 80,
	})
	if err := agg.Ingest(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	if agg.ActiveKeyCount() > 3 {
		t.Fatalf("global cap violated: %d", agg.ActiveKeyCount())
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) > 3 {
		t.Fatalf("emitted %d keys", len(out.Evidence))
	}
	for _, rec := range out.Evidence {
		if rec.Quality == ProcessNetworkQualityValid && rec.Workload.ProcessName == "w1" {
			t.Fatal("workload over cap cannot stay silently VALID")
		}
	}
}

func TestProcessNetworkLateHeavyDestinationReplacesColderCandidate(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withKeys(64, 2))
	now := pnNow()
	coldA := bytesN("svc", "192.0.2.1", 80, 1, now)
	coldB := bytesN("svc", "192.0.2.2", 80, 1, now)
	hot := bytesN("svc", "192.0.2.9", 80, 8, now)
	var events []ProcessNetworkSyntheticEvent
	events = append(events, coldA...)
	events = append(events, coldB...)
	events = append(events, hot...)
	if err := agg.Ingest(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 2 {
		t.Fatalf("per-workload cap 2, got %d", len(out.Evidence))
	}
	ips := map[string]uint64{}
	for _, rec := range out.Evidence {
		if rec.BytesTx != nil {
			ips[rec.RemoteIP.String()] = *rec.BytesTx
		} else {
			ips[rec.RemoteIP.String()] = 0
		}
	}
	if _, ok := ips["192.0.2.9"]; !ok {
		t.Fatalf("late heavy dest must be represented: %v", ips)
	}
	if _, ok := ips["192.0.2.1"]; ok && ips["192.0.2.9"] > 0 {
		// 192.0.2.1 may be evicted; if present it must not hold 192.0.2.9 bytes
	}
	if rec, ok := findIP(out.Evidence, "192.0.2.9"); ok {
		if rec.BytesTx == nil || *rec.BytesTx != 8 {
			t.Fatalf("replacement exact metrics are only its own events, got %v", rec.BytesTx)
		}
		if rec.Quality == ProcessNetworkQualityValid {
			t.Fatal("replacement after overflow cannot stay VALID")
		}
	}
}

func TestProcessNetworkDeterministicLexicographicTie(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withKeys(64, 2))
	now := pnNow()
	// Equal heat 1 for .10 and .20; new .30 arrives. Evict lexicographically smaller dest key.
	events := []ProcessNetworkSyntheticEvent{
		baseAt("svc", "192.0.2.10", 80, now),
		baseAt("svc", "192.0.2.20", 80, now),
		baseAt("svc", "192.0.2.30", 80, now),
	}
	if err := agg.Ingest(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 2 {
		t.Fatalf("got %d", len(out.Evidence))
	}
	k10 := destinationKey(netip.MustParseAddr("192.0.2.10"), 80, ProcessNetworkProtocolTCP)
	k20 := destinationKey(netip.MustParseAddr("192.0.2.20"), 80, ProcessNetworkProtocolTCP)
	if k10 >= k20 {
		t.Fatalf("precondition: %s < %s", k10, k20)
	}
	if _, ok := findIP(out.Evidence, "192.0.2.10"); ok {
		t.Fatal("lowest-heat lex-smallest candidate must be evicted")
	}
	if _, ok := findIP(out.Evidence, "192.0.2.20"); !ok {
		t.Fatal("survivor .20 missing")
	}
	if rec, ok := findIP(out.Evidence, "192.0.2.30"); !ok {
		t.Fatal("new dest missing")
	} else if rec.BytesTx != nil && *rec.BytesTx != 0 {
		// bytes not set on these events
	}
	ov := overflowSummary(out.Accounting, ProcessNetworkProtocolTCP)
	if ov == nil || ov.OverflowedRecordsCount == 0 || ov.EvictedCandidateCount == 0 {
		t.Fatal("evicted exact metrics enter overflow")
	}
}

func TestProcessNetworkEvictedMetricsDoNotTransferToReplacement(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withKeys(64, 1))
	now := pnNow()
	txA := uint64(999)
	txB := uint64(3)
	a := baseAt("svc", "192.0.2.1", 80, now)
	a.BytesTx = &txA
	b := baseAt("svc", "192.0.2.2", 80, now)
	b.BytesTx = &txB
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{a, b}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 1 {
		t.Fatalf("got %d", len(out.Evidence))
	}
	rec := out.Evidence[0]
	if rec.RemoteIP.String() != "192.0.2.2" {
		t.Fatalf("replacement dest %s", rec.RemoteIP)
	}
	if rec.BytesTx == nil || *rec.BytesTx != 3 {
		t.Fatalf("must not inherit 999, got %v", rec.BytesTx)
	}
	found := false
	for _, acc := range out.Accounting {
		for _, ov := range acc.Overflows {
			if ov.BytesTx != nil && *ov.BytesTx == 999 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("evicted 999 bytes must be in overflow only")
	}
}

func TestProcessNetworkRandomCardinalityAttackRemainsBounded(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withKeys(32, 8))
	now := pnNow()
	events := make([]ProcessNetworkSyntheticEvent, 0, 512)
	for i := 0; i < 512; i++ {
		events = append(events, ProcessNetworkSyntheticEvent{
			ObservedAt: now, Class: ProcessNetworkEventBytes, ProcessName: "scanner",
			Protocol:   ProcessNetworkProtocolTCP,
			RemoteIP:   fmt.Sprintf("203.0.%d.%d", i/256, i%256),
			RemotePort: uint16(1 + i%65535),
		})
	}
	if err := agg.Ingest(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	if agg.ActiveKeyCount() > 32 {
		t.Fatalf("unbounded keys %d", agg.ActiveKeyCount())
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) > 32 {
		t.Fatalf("emitted %d", len(out.Evidence))
	}
	if agg.RetainedWindowCount() > 8 {
		t.Fatalf("windows %d", agg.RetainedWindowCount())
	}
}

func TestProcessNetworkBackendDropAccounting(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	n := uint32(7)
	good := baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
	drop := ProcessNetworkSyntheticEvent{
		ObservedAt:   pnNow(),
		Class:        ProcessNetworkEventBackendDrop,
		DroppedCount: &n,
	}
	if drop.RemoteIP != "" {
		t.Fatal("fixture must not invent remote")
	}
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{good, drop}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 1 {
		t.Fatal("backend drop must not create a socket record")
	}
	if out.Evidence[0].RemoteIP.String() == "" {
		t.Fatal("ordinary record still has IP")
	}
	found := false
	for _, acc := range out.Accounting {
		if acc.Workload == nil && acc.DroppedRecordsCount == 7 {
			found = true
			if acc.Quality == ProcessNetworkQualityValid {
				t.Fatal("backend drop must make quality non-VALID")
			}
		}
	}
	if !found {
		t.Fatalf("unattributed drop accounting missing: %+v", out.Accounting)
	}
}

func TestProcessNetworkQualitySampledPartialResourceGuarded(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withKeys(64, 1))
	now := pnNow()
	sampled := baseAt("sshd", "192.0.2.8", 22, now)
	sampled.Sampled = true
	guarded := baseAt("cron", "192.0.2.9", 25, now)
	guarded.ResourceGuarded = true
	first := baseAt("nginx", "192.0.2.1", 80, now)
	overflow := baseAt("nginx", "192.0.2.2", 80, now)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{sampled, guarded, first, overflow}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sawSampled, sawPartial, sawGuarded := false, false, false
	for _, rec := range out.Evidence {
		if rec.SampledFlag || rec.Quality == ProcessNetworkQualitySampled {
			sawSampled = true
		}
		if rec.PartialFlag || rec.Quality == ProcessNetworkQualityPartial {
			sawPartial = true
		}
		if rec.ResourceGuardedFlag || rec.Quality == ProcessNetworkQualityResourceGuarded {
			sawGuarded = true
		}
		if ProcessNetworkQualityAssertsHostProcessOrNetworkDown(rec.Quality) {
			t.Fatal("DOWN forbidden")
		}
	}
	for _, acc := range out.Accounting {
		if acc.PartialFlag || acc.Quality == ProcessNetworkQualityPartial {
			sawPartial = true
		}
		if acc.ResourceGuardedFlag || acc.Quality == ProcessNetworkQualityResourceGuarded {
			sawGuarded = true
		}
		if acc.SampledEventsCount > 0 {
			sawSampled = true
		}
	}
	if !sawSampled || !sawPartial || !sawGuarded {
		t.Fatalf("quality flags sampled=%v partial=%v guarded=%v evidence=%+v acc=%+v", sawSampled, sawPartial, sawGuarded, out.Evidence, out.Accounting)
	}
}

func TestProcessNetworkWindowRolloverAndLateClosedEvent(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withWindow(time.Minute))
	t0 := time.Date(2026, 10, 6, 12, 0, 10, 0, time.UTC)
	e1 := baseAt("nginx", "192.0.2.1", 80, t0)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{e1}); err != nil {
		t.Fatal(err)
	}
	out1, err := agg.FlushDue(context.Background(), t0.Truncate(time.Minute).Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(out1.Evidence) != 1 {
		t.Fatalf("rollover flush %d", len(out1.Evidence))
	}
	late := baseAt("nginx", "192.0.2.1", 80, t0)
	late.Class = ProcessNetworkEventBytes
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{late}); err != nil {
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
	for _, rec := range out2.Evidence {
		if rec.RemoteIP.String() == "192.0.2.1" && rec.BucketStartAt.Equal(out1.Evidence[0].BucketStartAt) {
			t.Fatal("late event must not rewrite a closed window destination")
		}
	}
	sawLate := false
	for _, acc := range out2.Accounting {
		if acc.LateEventsCount > 0 {
			sawLate = true
		}
	}
	if !sawLate {
		t.Fatalf("late accounting missing: %+v", out2.Accounting)
	}
}

func TestProcessNetworkCancelAndClose(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := agg.Ingest(ctx, []ProcessNetworkSyntheticEvent{baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ingest cancel: %v", err)
	}
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)}); err != nil {
		t.Fatal(err)
	}
	if err := agg.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	err = agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseEvent("nginx", "192.0.2.3", 80, ProcessNetworkProtocolTCP)})
	if err == nil {
		t.Fatal("ingest after close must fail")
	}
}

func TestProcessNetworkConcurrentIngestFlush(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withKeys(128, 16))
	ctx := context.Background()
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				ev := ProcessNetworkSyntheticEvent{
					ObservedAt:  pnNow(),
					Class:       ProcessNetworkEventBytes,
					ProcessName: fmt.Sprintf("w%d", id%3),
					Protocol:    ProcessNetworkProtocolTCP,
					RemoteIP:    fmt.Sprintf("192.0.2.%d", 1+i%10),
					RemotePort:  80,
				}
				_ = agg.Ingest(ctx, []ProcessNetworkSyntheticEvent{ev})
				if i%7 == 0 {
					_, _ = agg.FlushDue(ctx, pnNow().Add(time.Minute))
				}
			}
		}(g)
	}
	wg.Wait()
	_, _ = agg.FlushAll(ctx)
	if agg.ActiveKeyCount() > 128 {
		t.Fatalf("race grew keys to %d", agg.ActiveKeyCount())
	}
}

func TestProcessNetworkClosedWindowHorizonDoesNotReopen(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	cfg := ProcessNetworkAggregatorConfig{
		SourceProvenance:     ProcessNetworkSyntheticSourceProvenance,
		Window:               time.Minute,
		TestAllowShortWindow: true,
		MaxClosedWindows:     2,
		MaxRetainedWindows:   maxProcessNetworkRetainedWindows,
	}
	agg, err := NewProcessNetworkAggregator(cfg, sink)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 6, 12, 0, 10, 0, time.UTC)
	for i := 0; i < 3; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		ev := baseAt("nginx", fmt.Sprintf("192.0.2.%d", i+1), 80, ts)
		if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{ev}); err != nil {
			t.Fatal(err)
		}
		if _, err := agg.FlushDue(context.Background(), ts.Truncate(time.Minute).Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	late := baseAt("nginx", "198.51.100.9", 80, base)
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{late}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range out.Evidence {
		if rec.RemoteIP.String() == "198.51.100.9" {
			t.Fatal("late event after stub eviction must not reopen a closed window as ordinary evidence")
		}
	}
	sawLate := false
	for _, acc := range out.Accounting {
		if acc.LateEventsCount > 0 {
			sawLate = true
		}
	}
	if !sawLate {
		t.Fatalf("orphan late must be accounted: %+v", out.Accounting)
	}
}

func TestProcessNetworkGlobalCapShedCountedOnce(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink, withKeys(2, 2))
	now := pnNow()
	events := []ProcessNetworkSyntheticEvent{
		baseAt("w1", "192.0.2.1", 80, now),
		baseAt("w2", "198.51.100.1", 80, now),
		baseAt("w1", "192.0.2.3", 80, now),
	}
	if err := agg.Ingest(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var shed uint32
	for _, acc := range out.Accounting {
		if acc.Workload != nil && acc.Workload.ProcessName == "w1" {
			if acc.ShedRecordsCount != 1 {
				t.Fatalf("workload shed must be 1, got %d", acc.ShedRecordsCount)
			}
			if acc.OverflowedRecordsCount != 1 {
				t.Fatalf("overflowed records must be 1, got %d", acc.OverflowedRecordsCount)
			}
			if acc.EvictedCandidateCount != 0 {
				t.Fatalf("global-cap shed is not an eviction, got evicted %d", acc.EvictedCandidateCount)
			}
			shed = acc.ShedRecordsCount
		}
	}
	if shed != 1 {
		t.Fatalf("expected w2 shed accounting, got %+v", out.Accounting)
	}
}

func TestProcessNetworkPIDOnlyIsAccounted(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)
	pid := uint32(9)
	ev := ProcessNetworkSyntheticEvent{
		ObservedAt: pnNow(),
		Class:      ProcessNetworkEventOpen,
		PID:        &pid,
		Protocol:   ProcessNetworkProtocolTCP,
		RemoteIP:   "192.0.2.1",
		RemotePort: 80,
	}
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{ev}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 0 {
		t.Fatal("PID-only must not emit destination evidence")
	}
	if !accountingHasDropped(out.Accounting) {
		t.Fatal("PID-only rejection must not be silent")
	}
}

func TestProcessNetworkInputBatchBound(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	cfg := ProcessNetworkAggregatorConfig{SourceProvenance: ProcessNetworkSyntheticSourceProvenance, Window: time.Minute, TestAllowShortWindow: true, MaxInputBatch: 4}
	agg, err := NewProcessNetworkAggregator(cfg, sink)
	if err != nil {
		t.Fatal(err)
	}
	events := make([]ProcessNetworkSyntheticEvent, 5)
	for i := range events {
		events[i] = baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
	}
	if err := agg.Ingest(context.Background(), events); err == nil {
		t.Fatal("batch cap")
	}
}

func TestProcessNetworkDefaultAggregatorIsUnwired(t *testing.T) {
	agg, err := NewProcessNetworkAggregator(ProcessNetworkAggregatorConfig{SourceProvenance: ProcessNetworkSyntheticSourceProvenance, Window: time.Minute, TestAllowShortWindow: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)}); err != nil {
		t.Fatal(err)
	}
	_, err = agg.FlushAll(context.Background())
	if !errors.Is(err, ErrStructuredTransportNotWired) {
		t.Fatalf("unwired flush: %v", err)
	}
}

func bytesN(name, ip string, port uint16, n int, now time.Time) []ProcessNetworkSyntheticEvent {
	out := make([]ProcessNetworkSyntheticEvent, n)
	one := uint64(1)
	for i := 0; i < n; i++ {
		e := baseAt(name, ip, port, now)
		e.BytesTx = &one
		out[i] = e
	}
	return out
}

func baseAt(name, ip string, port uint16, now time.Time) ProcessNetworkSyntheticEvent {
	return ProcessNetworkSyntheticEvent{
		ObservedAt:  now,
		Class:       ProcessNetworkEventBytes,
		ProcessName: name,
		Protocol:    ProcessNetworkProtocolTCP,
		RemoteIP:    ip,
		RemotePort:  port,
	}
}

func findIP(recs []ProcessNetworkEvidence, ip string) (ProcessNetworkEvidence, bool) {
	for _, r := range recs {
		if r.RemoteIP.String() == ip {
			return r, true
		}
	}
	return ProcessNetworkEvidence{}, false
}

func overflowSummary(acc []ProcessNetworkWindowAccounting, proto ProcessNetworkProtocol) *ProcessNetworkOverflowSummary {
	for _, a := range acc {
		for i := range a.Overflows {
			if a.Overflows[i].Protocol == proto {
				return &a.Overflows[i]
			}
		}
	}
	return nil
}

// F1: pipe-delimited keys must not collide distinct ProcessName/ContainerID pairs.
func TestProcessNetworkWorkloadKeyDelimiterCollision(t *testing.T) {
	sink := &LocalRecordingStructuredEvidenceSink{}
	agg := mustAgg(t, sink)

	a := baseEvent("a|b", "192.0.2.10", 80, ProcessNetworkProtocolTCP)
	a.ContainerID = "c"
	txA := uint64(7)
	a.BytesTx = &txA

	b := baseEvent("a", "192.0.2.20", 443, ProcessNetworkProtocolTCP)
	b.ContainerID = "b|c"
	txB := uint64(11)
	b.BytesTx = &txB

	same := baseEvent("a|b", "192.0.2.10", 80, ProcessNetworkProtocolTCP)
	same.ContainerID = "c"
	txSame := uint64(3)
	same.BytesTx = &txSame

	ns1, ns2 := uint64(1), uint64(2)
	nsA := baseEvent("shared", "198.51.100.1", 80, ProcessNetworkProtocolTCP)
	nsA.NetworkNamespace = &ns1
	nsB := baseEvent("shared", "198.51.100.2", 80, ProcessNetworkProtocolTCP)
	nsB.NetworkNamespace = &ns2

	if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{a, b, same, nsA, nsB}); err != nil {
		t.Fatal(err)
	}
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 4 {
		t.Fatalf("collision pair + same-key merge + netns split want 4 records, got %d", len(out.Evidence))
	}

	recA, ok := findIP(out.Evidence, "192.0.2.10")
	if !ok {
		t.Fatal("missing ProcessName=a|b / ContainerID=c record")
	}
	if recA.Workload.Value != "a|b" || recA.Workload.ContainerID != "c" {
		t.Fatalf("identity corrupted for a|b/c: %+v", recA.Workload)
	}
	if recA.BytesTx == nil || *recA.BytesTx != 10 {
		t.Fatalf("same-key merge for a|b/c failed: %+v", recA)
	}

	recB, ok := findIP(out.Evidence, "192.0.2.20")
	if !ok {
		t.Fatal("missing ProcessName=a / ContainerID=b|c record")
	}
	if recB.Workload.Value != "a" || recB.Workload.ContainerID != "b|c" {
		t.Fatalf("identity corrupted for a/b|c: %+v", recB.Workload)
	}
	if recB.BytesTx == nil || *recB.BytesTx != 11 {
		t.Fatalf("distinct workload bytes wrong: %+v", recB)
	}

	nsRecs := 0
	for _, r := range out.Evidence {
		if r.RemoteIP.String() == "198.51.100.1" || r.RemoteIP.String() == "198.51.100.2" {
			nsRecs++
			if r.Workload.NetworkNamespace == nil {
				t.Fatalf("namespace must remain present: %+v", r.Workload)
			}
		}
	}
	if nsRecs != 2 {
		t.Fatalf("namespace separation lost: %d", nsRecs)
	}
}

// F2: missing-first optional PID/local endpoint must not become order-dependent common identity.
func TestProcessNetworkOptionalIdentityOrderIndependent(t *testing.T) {
	type step struct {
		pid   *uint32
		lip   string
		lport *uint16
	}
	pid := uint32(42)
	port := uint16(40000)
	cases := []struct {
		name   string
		steps  []step
		wantPID bool
		wantIP  bool
		wantPort bool
	}{
		{name: "missing_then_present", steps: []step{{}, {pid: &pid, lip: "10.0.0.1", lport: &port}}},
		{name: "present_then_missing", steps: []step{{pid: &pid, lip: "10.0.0.1", lport: &port}, {}}},
		{name: "missing_present_same", steps: []step{{}, {pid: &pid, lip: "10.0.0.1", lport: &port}, {pid: &pid, lip: "10.0.0.1", lport: &port}}},
		{name: "all_missing", steps: []step{{}, {}}},
		{name: "all_equal_present", steps: []step{
			{pid: &pid, lip: "10.0.0.1", lport: &port},
			{pid: &pid, lip: "10.0.0.1", lport: &port},
		}, wantPID: true, wantIP: true, wantPort: true},
		{name: "differing_present", steps: []step{
			{pid: &pid, lip: "10.0.0.1", lport: &port},
			{pid: uint32Ptr(99), lip: "10.0.0.2", lport: uint16Ptr(40001)},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name+"_single_ingest", func(t *testing.T) {
			agg := mustAgg(t, &LocalRecordingStructuredEvidenceSink{})
			events := make([]ProcessNetworkSyntheticEvent, len(tc.steps))
			for i, s := range tc.steps {
				e := baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
				e.PID = s.pid
				e.LocalIP = s.lip
				e.LocalPort = s.lport
				events[i] = e
			}
			if err := agg.Ingest(context.Background(), events); err != nil {
				t.Fatal(err)
			}
			assertOptionalIdentity(t, agg, tc.wantPID, tc.wantIP, tc.wantPort, pid, "10.0.0.1", port)
		})
		t.Run(tc.name+"_separate_ingest", func(t *testing.T) {
			agg := mustAgg(t, &LocalRecordingStructuredEvidenceSink{})
			for _, s := range tc.steps {
				e := baseEvent("nginx", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
				e.PID = s.pid
				e.LocalIP = s.lip
				e.LocalPort = s.lport
				if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{e}); err != nil {
					t.Fatal(err)
				}
			}
			assertOptionalIdentity(t, agg, tc.wantPID, tc.wantIP, tc.wantPort, pid, "10.0.0.1", port)
		})
	}
}

func assertOptionalIdentity(t *testing.T, agg *ProcessNetworkAggregator, wantPID, wantIP, wantPort bool, pid uint32, lip string, port uint16) {
	t.Helper()
	out, err := agg.FlushAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Evidence) != 1 {
		t.Fatalf("want 1 record, got %d", len(out.Evidence))
	}
	rec := out.Evidence[0]
	if wantPID {
		if rec.PID == nil || *rec.PID != pid {
			t.Fatalf("want common PID %d, got %+v", pid, rec.PID)
		}
	} else if rec.PID != nil {
		t.Fatalf("PID must be omitted, got %d", *rec.PID)
	}
	if wantIP {
		if rec.LocalIP == nil || rec.LocalIP.String() != lip {
			t.Fatalf("want common local IP %s, got %+v", lip, rec.LocalIP)
		}
	} else if rec.LocalIP != nil {
		t.Fatalf("local IP must be omitted, got %s", rec.LocalIP)
	}
	if wantPort {
		if rec.LocalPort == nil || *rec.LocalPort != port {
			t.Fatalf("want common local port %d, got %+v", port, rec.LocalPort)
		}
	} else if rec.LocalPort != nil {
		t.Fatalf("local port must be omitted, got %d", *rec.LocalPort)
	}
}

// F3: workload accounting must emit non-key metadata only when common across all represented events.
func TestProcessNetworkWorkloadAccountingCommonOnlyMetadata(t *testing.T) {
	t.Run("mixed_metadata_ordinary", func(t *testing.T) {
		for _, reverse := range []bool{false, true} {
			name := "order_ab"
			if reverse {
				name = "order_ba"
			}
			t.Run(name, func(t *testing.T) {
				agg := mustAgg(t, &LocalRecordingStructuredEvidenceSink{})
				a := baseEvent("worker", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
				a.ServiceID = "app.service"
				a.ExecutableIdentity = "exe-a"
				a.ExecutablePath = "/usr/bin/a"
				a.CgroupPath = "/sys/fs/cgroup/a"
				b := baseEvent("worker", "192.0.2.2", 80, ProcessNetworkProtocolTCP)
				b.ServiceID = "app.service"
				b.ExecutableIdentity = "exe-b"
				b.ExecutablePath = "/usr/bin/b"
				b.CgroupPath = "/sys/fs/cgroup/b"
				events := []ProcessNetworkSyntheticEvent{a, b}
				if reverse {
					events[0], events[1] = events[1], events[0]
				}
				if err := agg.Ingest(context.Background(), events); err != nil {
					t.Fatal(err)
				}
				out, err := agg.FlushAll(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				wlAcc := workloadAccounting(out.Accounting)
				if wlAcc == nil || wlAcc.Workload == nil {
					t.Fatalf("missing workload accounting: %+v", out.Accounting)
				}
				id := wlAcc.Workload
				if id.ServiceID != "app.service" {
					t.Fatalf("common ServiceID lost: %+v", id)
				}
				if id.ExecutableIdentity != "" || id.ExecutablePath != "" || id.CgroupPath != "" {
					t.Fatalf("mixed non-key metadata must be omitted: %+v", id)
				}
				if id.Kind != SelectorKindServiceID || id.Value != "app.service" {
					t.Fatalf("key identity must remain: %+v", id)
				}
			})
		}
	})

	t.Run("mixed_metadata_with_overflow", func(t *testing.T) {
		agg := mustAgg(t, &LocalRecordingStructuredEvidenceSink{}, withKeys(64, 1))
		a := baseEvent("worker", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
		a.ServiceID = "app.service"
		a.ExecutablePath = "/usr/bin/a"
		b := baseEvent("worker", "192.0.2.2", 80, ProcessNetworkProtocolTCP)
		b.ServiceID = "app.service"
		b.ExecutablePath = "/usr/bin/b"
		if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{a, b}); err != nil {
			t.Fatal(err)
		}
		out, err := agg.FlushAll(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		wlAcc := workloadAccounting(out.Accounting)
		if wlAcc == nil || wlAcc.Workload == nil {
			t.Fatalf("missing workload accounting: %+v", out.Accounting)
		}
		if wlAcc.EvictedCandidateCount == 0 && wlAcc.OverflowedRecordsCount == 0 && len(wlAcc.Overflows) == 0 {
			t.Fatalf("expected overflow/eviction evidence: %+v", wlAcc)
		}
		if wlAcc.Workload.ExecutablePath != "" {
			t.Fatalf("overflow-mixed path must not publish first worker path: %+v", wlAcc.Workload)
		}
		if wlAcc.Workload.ServiceID != "app.service" {
			t.Fatalf("common ServiceID must remain: %+v", wlAcc.Workload)
		}
	})

	t.Run("genuinely_common_retained", func(t *testing.T) {
		agg := mustAgg(t, &LocalRecordingStructuredEvidenceSink{})
		a := baseEvent("worker", "192.0.2.1", 80, ProcessNetworkProtocolTCP)
		a.ServiceID = "app.service"
		a.ExecutablePath = "/usr/bin/shared"
		a.CgroupPath = "/sys/fs/cgroup/shared"
		b := a
		b.RemoteIP = "192.0.2.2"
		if err := agg.Ingest(context.Background(), []ProcessNetworkSyntheticEvent{a, b}); err != nil {
			t.Fatal(err)
		}
		out, err := agg.FlushAll(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		wlAcc := workloadAccounting(out.Accounting)
		if wlAcc == nil || wlAcc.Workload == nil {
			t.Fatal("missing workload accounting")
		}
		id := wlAcc.Workload
		if id.ExecutablePath != "/usr/bin/shared" || id.CgroupPath != "/sys/fs/cgroup/shared" || id.ServiceID != "app.service" {
			t.Fatalf("common metadata must be retained: %+v", id)
		}
	})
}

func workloadAccounting(acc []ProcessNetworkWindowAccounting) *ProcessNetworkWindowAccounting {
	for i := range acc {
		if acc[i].Workload != nil {
			return &acc[i]
		}
	}
	return nil
}
