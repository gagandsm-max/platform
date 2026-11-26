package flow

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"net/netip"
	"sync"
	"testing"
)

func sequenceTestKey() SFlowV5StreamKey {
	return SFlowV5StreamKey{
		TenantID: "test-tenant", ExporterBinding: "test-binding", VantageID: "collector-1",
		Source: netip.MustParseAddrPort("192.0.2.3:50100"), Collector: netip.MustParseAddrPort("[2001:db8::1]:6343"),
		AgentAddress: netip.MustParseAddr("192.0.2.4"), SubAgentID: 7,
	}
}

func sequenceWords(words ...uint32) []byte {
	b := make([]byte, 0, 4*len(words))
	for _, word := range words {
		b = binary.BigEndian.AppendUint32(b, word)
	}
	return b
}

// A standard IPv4 sFlow header followed by complete XDR sample elements.
func sequenceDatagram(seq, uptime uint32, samples ...[]byte) []byte {
	b := sequenceWords(5, 1, 0xc0000204, 7, seq, uptime, uint32(len(samples)))
	for _, sample := range samples {
		b = append(b, sample...)
	}
	return b
}

func sequenceStream(t *testing.T) *SFlowV5Stream {
	t.Helper()
	stream, err := NewSFlowV5Stream(sequenceTestKey())
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

func sequenceObserve(t *testing.T, stream *SFlowV5Stream, trust bool, b []byte) *SFlowV5Observation {
	t.Helper()
	obs, err := stream.Observe(sequenceTestKey(), trust, b)
	if err != nil {
		t.Fatal(err)
	}
	if obs.Key != sequenceTestKey() || obs.Current.Digest != sha256.Sum256(b) || obs.Current.UptimeTrusted != trust {
		t.Fatalf("wrong observation provenance: %+v", obs)
	}
	return obs
}

func TestSFlowV5StreamSerialTransitions(t *testing.T) {
	tests := []struct {
		name                         string
		from, to, prevUptime, uptime uint32
		prevTrust, trust             bool
		status                       SFlowV5SequenceStatus
		missing                      uint32
	}{
		{"contiguous", 7, 8, 15, 15, true, true, SFlowV5Contiguous, 0},
		{"sequenceWrap", 0xffffffff, 0, 15, 16, true, true, SFlowV5Contiguous, 0},
		{"wrapWithGap", 0xfffffffe, 1, 15, 16, true, true, SFlowV5SequenceGap, 2},
		{"lastForwardDistance", 2, 0x80000001, 15, 16, true, true, SFlowV5SequenceGap, 0x7ffffffe},
		{"firstOldDistance", 2, 0x80000002, 15, 15, true, true, SFlowV5Old, 0},
		{"halfRangeWithLaterClock", 2, 0x80000002, 15, 16, true, true, SFlowV5Unknown, 0},
		{"backtrackWithLaterClock", 7, 6, 15, 16, true, true, SFlowV5Unknown, 0},
		{"old", 7, 6, 15, 15, true, true, SFlowV5Old, 0},
		{"unknownCurrent", 7, 6, 15, 14, true, false, SFlowV5Unknown, 0},
		{"unknownPrevious", 7, 6, 15, 14, false, true, SFlowV5Unknown, 0},
		{"untrustedContiguous", 7, 8, 15, 14, false, false, SFlowV5Contiguous, 0},
		{"duplicate", 7, 7, 15, 15, true, false, SFlowV5Duplicate, 0},
		{"collision", 7, 7, 15, 16, true, true, SFlowV5Collision, 0},
		{"uptimeDecreaseForward", 7, 9, 15, 14, true, true, SFlowV5Discontinuity, 0},
		{"uptimeDecreaseSameSequence", 7, 7, 15, 0, true, true, SFlowV5Discontinuity, 0},
		{"uptimeDecreaseOld", 7, 6, 15, 14, true, true, SFlowV5Discontinuity, 0},
		{"uptimeWrap", 0xffffffff, 0, 0xffffffff, 0, true, true, SFlowV5Discontinuity, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stream := sequenceStream(t)
			first := sequenceObserve(t, stream, tc.prevTrust, sequenceDatagram(tc.from, tc.prevUptime))
			if first.Status != SFlowV5Baseline || first.Epoch != 1 || first.HasPrevious ||
				first.MissingDatagrams != 0 || first.Datagram == nil {
				t.Fatalf("bad first baseline: %+v", first)
			}
			next := sequenceObserve(t, stream, tc.trust, sequenceDatagram(tc.to, tc.uptime))
			if next.Status != tc.status || next.MissingDatagrams != tc.missing || !next.HasPrevious ||
				next.Previous != first.Current || next.Current.SequenceNumber != tc.to || next.Current.Uptime != tc.uptime {
				t.Fatalf("transition: %+v", next)
			}
			wantEpoch := uint64(1)
			if tc.status == SFlowV5Discontinuity {
				wantEpoch = 2
			}
			wantPrior := first.Current
			admit := tc.status == SFlowV5Discontinuity || tc.status == SFlowV5Contiguous || tc.status == SFlowV5SequenceGap
			if admit {
				wantPrior = next.Current
			}
			if next.Epoch != wantEpoch || (next.Datagram != nil) != admit {
				t.Fatalf("wrong epoch or payload authority: %+v", next)
			}
			last := sequenceObserve(t, stream, true, sequenceDatagram(wantPrior.SequenceNumber+1, wantPrior.Uptime))
			if last.Status != SFlowV5Contiguous || last.Previous != wantPrior || last.Epoch != wantEpoch {
				t.Fatalf("prior state was not retained correctly: %+v", last)
			}
		})
	}
}

func TestSFlowV5StreamExactBytesAndSampleSequences(t *testing.T) {
	stream := sequenceStream(t)
	// Compact flow and counter samples have independent raw sequence values.
	flowSample := sequenceWords(1, 32, 900, 2, 1000, 10000, 0, 1, 2, 0)
	counterSample := sequenceWords(2, 12, 3, 2, 0)
	wire := sequenceDatagram(40, 0, flowSample, counterSample)
	replay := bytes.Clone(wire)
	first := sequenceObserve(t, stream, true, wire)
	if first.Datagram == nil || len(first.Datagram.Samples) != 2 ||
		first.Datagram.Samples[0].SequenceNumber != 900 || first.Datagram.Samples[1].SequenceNumber != 3 {
		t.Fatalf("raw sample sequence numbers were lost: %+v", first)
	}
	// Reuse the receive buffer and mutate caller-owned output; admission must
	// still compare against the exact original bytes independently.
	clear(wire)
	first.Datagram.SequenceNumber = 800
	first.Current.SequenceNumber = 800
	first.Current.Digest = [32]byte{}
	first.Datagram.Samples[1].SequenceNumber = 800
	duplicate := sequenceObserve(t, stream, true, replay)
	if duplicate.Status != SFlowV5Duplicate || duplicate.Datagram != nil || duplicate.Previous.SequenceNumber != 40 {
		t.Fatalf("buffer or output affected the tracker: %+v", duplicate)
	}
	binary.BigEndian.PutUint32(counterSample[8:], 0x80000000)
	collision := sequenceObserve(t, stream, true, sequenceDatagram(40, 0, flowSample, counterSample))
	if collision.Status != SFlowV5Collision || collision.Datagram != nil || collision.Current.Digest == duplicate.Current.Digest {
		t.Fatalf("same-header sample-only change: %+v", collision)
	}
	obs := sequenceObserve(t, stream, true, sequenceDatagram(41, 1, flowSample, counterSample))
	if obs.Status != SFlowV5Contiguous || obs.MissingDatagrams != 0 || obs.Datagram == nil ||
		obs.Datagram.Samples[1].SequenceNumber != 0x80000000 || obs.Previous.Digest != duplicate.Current.Digest {
		t.Fatalf("sample sequence changed datagram authority: %+v", obs)
	}
}

func TestSFlowV5StreamTrustedBindingRequired(t *testing.T) {
	key := sequenceTestKey()
	missing := []func(*SFlowV5StreamKey){
		func(k *SFlowV5StreamKey) { k.TenantID = "" }, func(k *SFlowV5StreamKey) { k.ExporterBinding = " " },
		func(k *SFlowV5StreamKey) { k.VantageID = "" }, func(k *SFlowV5StreamKey) { k.AgentAddress = netip.Addr{} },
		func(k *SFlowV5StreamKey) { k.Collector = netip.MustParseAddrPort("[::]:6343") },
		func(k *SFlowV5StreamKey) { k.Source = netip.MustParseAddrPort("[::ffff:0.0.0.0]:99") },
		func(k *SFlowV5StreamKey) { k.Source = netip.MustParseAddrPort("192.0.2.3:0") },
	}
	for i, change := range missing {
		invalid := key
		change(&invalid)
		if stream, err := NewSFlowV5Stream(invalid); stream != nil || !errors.Is(err, ErrSFlowV5StreamScope) {
			t.Fatalf("missing binding case %d accepted: %v", i, err)
		}
	}
	packet := sequenceDatagram(95, 20)
	var empty SFlowV5Stream
	if obs, err := empty.Observe(SFlowV5StreamKey{}, true, packet); obs != nil || !errors.Is(err, ErrSFlowV5StreamScope) {
		t.Fatalf("uninitialized stream accepted: %v", err)
	}
	stream := sequenceStream(t)
	wrong := []func(*SFlowV5StreamKey){
		func(k *SFlowV5StreamKey) { k.TenantID = "other" }, func(k *SFlowV5StreamKey) { k.ExporterBinding = "other" },
		func(k *SFlowV5StreamKey) { k.VantageID = "other" }, func(k *SFlowV5StreamKey) { k.SubAgentID++ },
		func(k *SFlowV5StreamKey) { k.AgentAddress = netip.MustParseAddr("192.0.2.22") },
		func(k *SFlowV5StreamKey) { k.Source = netip.MustParseAddrPort("192.0.2.3:50101") },
		func(k *SFlowV5StreamKey) { k.Collector = netip.MustParseAddrPort("[2001:db8::2]:6343") },
	}
	for i, change := range wrong {
		other := key
		change(&other)
		if obs, err := stream.Observe(other, true, packet); obs != nil || !errors.Is(err, ErrSFlowV5StreamScope) {
			t.Fatalf("cross-stream case %d accepted: %v", i, err)
		}
	}
	for _, invalidWire := range [][]byte{
		sequenceWords(5, 1, 0xc0000205, 7, 95, 20, 0), // different wire agent
		sequenceWords(5, 1, 0xc0000204, 8, 95, 20, 0), // different wire sub-agent
		sequenceWords(5, 0, 7, 95, 20, 0),             // protocol UNKNOWN
	} {
		if obs, err := stream.Observe(key, true, invalidWire); obs != nil || !errors.Is(err, ErrSFlowV5StreamScope) {
			t.Fatalf("mismatched wire identity accepted: %v", err)
		}
	}
	first := sequenceObserve(t, stream, true, packet)
	if first.Status != SFlowV5Baseline {
		t.Fatalf("rejected stream claimed authority: %+v", first)
	}
	truncated := sequenceDatagram(96, 1, sequenceWords(2, 12, 5)) // valid header, malformed inner sample
	if obs, err := stream.Observe(key, true, truncated); obs != nil || !errors.Is(err, ErrMalformedSFlowV5) {
		t.Fatalf("bad packet accepted: %v", err)
	}
	after := sequenceObserve(t, stream, true, sequenceDatagram(96, 22))
	if after.Status != SFlowV5Contiguous || after.Epoch != 1 || after.Previous != first.Current {
		t.Fatalf("failed parse changed sequence epoch: %+v", after)
	}
	fresh := sequenceObserve(t, sequenceStream(t), true, sequenceDatagram(400000, 22))
	if fresh.Status != SFlowV5Baseline || fresh.HasPrevious || fresh.MissingDatagrams != 0 {
		t.Fatalf("fresh collector inferred continuity: %+v", fresh)
	}
}

func TestSFlowV5StreamConcurrentReplay(t *testing.T) {
	const n = 32
	stream := sequenceStream(t)
	key := sequenceTestKey()
	wire := sequenceDatagram(0, 0)
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]*SFlowV5Observation, n)
	errs := make([]error, n)
	for i := range n {
		wg.Go(func() {
			<-start
			results[i], errs[i] = stream.Observe(key, true, wire)
		})
	}
	close(start)
	wg.Wait()
	admitted := 0
	for i, obs := range results {
		if errs[i] != nil || obs == nil {
			t.Fatalf("observation %d: %v", i, errs[i])
		}
		if obs.Status == SFlowV5Baseline && obs.Datagram != nil {
			admitted++
		} else if obs.Status != SFlowV5Duplicate || obs.Datagram != nil || obs.Epoch != 1 || obs.MissingDatagrams != 0 {
			t.Fatalf("unexpected concurrent classification: %+v", obs)
		}
	}
	if admitted != 1 {
		t.Fatalf("admitted %d copies, want one", admitted)
	}
}
