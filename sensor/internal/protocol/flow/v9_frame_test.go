package flow

import (
	"encoding/binary"
	"errors"
	"reflect"
	"slices"
	"testing"
)

func v9MixedPacket() []byte {
	return []byte{
		0, 9, 0, 9, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
		0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10,
		0, 0, 0, 20, // Two templates: one byte for protocol, two for source port.
		1, 0, 0, 1, 0, 4, 0, 1,
		1, 1, 0, 1, 0, 7, 0, 2,
		0, 1, 0, 20, // Options template 258 with two padding bytes.
		1, 2, 0, 4, 0, 4, 0, 1, 0, 4, 0, 34, 0, 4, 0, 0,
		1, 0, 0, 7, 1, 6, 17, // Three protocol records, unaligned next set.
		1, 1, 0, 6, 0x01, 0xbb,
		1, 2, 0, 12, 0, 0, 0, 1, 0, 0, 3, 0xe8,
		1, 0, 0, 5, 6, // Reuse template 256.
	}
}

func TestParseV9EnvelopeMixedRecords(t *testing.T) {
	packet := v9MixedPacket()
	original := slices.Clone(packet)
	got, err := ParseV9Envelope(packet)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(packet, original) {
		t.Fatal("parser changed input")
	}
	clear(packet) // Receive buffers may be reused immediately.
	want := &V9Envelope{
		Version: 9, Count: 9, SysUpTime: 0x01234567, UnixSecs: 0x89abcdef,
		SequenceNumber: 0xfedcba98, SourceID: 0x76543210,
		FlowSets: []V9FlowSet{
			{ID: 0, Offset: 20, Length: 20}, {ID: 1, Offset: 40, Length: 20},
			{ID: 256, Offset: 60, Length: 7}, {ID: 257, Offset: 67, Length: 6},
			{ID: 258, Offset: 73, Length: 12}, {ID: 256, Offset: 85, Length: 5},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseV9EnvelopeOpaqueIDsAndCount(t *testing.T) {
	packet := append(v9MixedPacket()[:20:20], 0, 2, 0, 4, 0, 255, 0, 4, 255, 255, 0, 4)
	want := []V9FlowSet{{ID: 2, Offset: 20, Length: 4}, {ID: 255, Offset: 24, Length: 4}, {ID: 65535, Offset: 28, Length: 4}}
	for _, count := range []uint16{0, 0x0123, 65535} {
		binary.BigEndian.PutUint16(packet[2:4], count)
		got, err := ParseV9Envelope(packet)
		if err != nil {
			t.Fatal(err)
		}
		if got.Count != count || !slices.Equal(got.FlowSets, want) {
			t.Fatalf("count %d: got %+v", count, got)
		}
	}
}

func TestParseV9EnvelopeTruncation(t *testing.T) {
	packet := v9MixedPacket()
	// A prefix ending on a set boundary is independently well-framed even
	// when the full packet's reported record count is still in the header.
	validEnds := []int{40, 60, 67, 73, 85, 90}
	for end := 0; end <= len(packet); end++ {
		got, err := ParseV9Envelope(packet[:end])
		if slices.Contains(validEnds, end) {
			if err != nil || got == nil {
				t.Fatalf("valid prefix %d: got %+v, %v", end, got, err)
			}
		} else if got != nil || !errors.Is(err, ErrMalformedV9Envelope) {
			t.Fatalf("invalid prefix %d: got %+v, %v", end, got, err)
		}
	}
}

func TestParseV9EnvelopeInvalidFields(t *testing.T) {
	packet := v9MixedPacket()
	for _, length := range []uint16{0, 1, 2, 3, 6, 65535} {
		binary.BigEndian.PutUint16(packet[87:89], length) // Last set has five bytes.
		got, err := ParseV9Envelope(packet)
		if got != nil || !errors.Is(err, ErrMalformedV9Envelope) {
			t.Fatalf("invalid set length %d: got %+v, %v", length, got, err)
		}
	}
	for _, version := range []uint16{5, 10, 0x0900} {
		packet = v9MixedPacket()
		binary.BigEndian.PutUint16(packet[:2], version)
		got, err := ParseV9Envelope(packet)
		if got != nil || !errors.Is(err, ErrUnsupportedV9Version) {
			t.Fatalf("version %d: got %+v, %v", version, got, err)
		}
	}
}

func TestParseV9EnvelopeUDPBound(t *testing.T) {
	const maxPayload = 65535 - 8
	packet := make([]byte, maxPayload+1)
	packet[1] = 9
	binary.BigEndian.PutUint16(packet[22:24], maxPayload-20)
	got, err := ParseV9Envelope(packet[:maxPayload])
	if err != nil || got == nil || !slices.Equal(got.FlowSets, []V9FlowSet{{ID: 0, Offset: 20, Length: maxPayload - 20}}) {
		t.Fatalf("maximum single frame: got %+v, %v", got, err)
	}
	// This longer payload still has a structurally fitting FlowSet length.
	binary.BigEndian.PutUint16(packet[22:24], maxPayload+1-20)
	got, err = ParseV9Envelope(packet)
	if got != nil || !errors.Is(err, ErrMalformedV9Envelope) {
		t.Fatalf("oversized UDP payload: got %+v, %v", got, err)
	}

	const maxSets = (maxPayload - 20) / 4
	for i := 0; i < maxSets; i++ {
		binary.BigEndian.PutUint16(packet[22+4*i:], 4)
	}
	const last = 20 + 4*(maxSets-1)
	binary.BigEndian.PutUint16(packet[last+2:], maxPayload-last)
	got, err = ParseV9Envelope(packet[:maxPayload])
	if err != nil {
		t.Fatal(err)
	}
	if len(got.FlowSets) != maxSets || got.FlowSets[maxSets-1] != (V9FlowSet{ID: 0, Offset: last, Length: maxPayload - last}) {
		t.Fatalf("maximum density produced %d sets or an incorrect final frame", len(got.FlowSets))
	}
}

func TestParseV9EnvelopeLateFailureAllocations(t *testing.T) {
	packet := make([]byte, 65527)
	packet[1] = 9
	for offset := 20; offset+4 <= len(packet); offset += 4 {
		packet[offset+3] = 4
	}
	// The last three bytes cannot form a FlowSet header.
	allocs := testing.AllocsPerRun(10, func() {
		got, err := ParseV9Envelope(packet)
		if got != nil || !errors.Is(err, ErrMalformedV9Envelope) {
			t.Fatalf("late truncation: got %+v, %v", got, err)
		}
	})
	if allocs != 0 {
		t.Fatalf("late malformed envelope incurred %g allocations", allocs)
	}
}

func FuzzParseV9Envelope(f *testing.F) {
	packet := v9MixedPacket()
	f.Add(packet)
	f.Add(packet[:22])
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, datagram []byte) {
		got, err := ParseV9Envelope(datagram)
		if err != nil {
			if got != nil || !(errors.Is(err, ErrMalformedV9Envelope) || errors.Is(err, ErrUnsupportedV9Version)) {
				t.Fatalf("unexpected failure: %+v, %v", got, err)
			}
			return
		}
		if got == nil || got.Version != 9 || len(datagram) > 65527 || len(got.FlowSets) == 0 {
			t.Fatal("invalid envelope accepted")
		}
		next := 20
		for _, set := range got.FlowSets {
			if int(set.Offset) != next || set.Length < 4 || next+int(set.Length) > len(datagram) {
				t.Fatalf("invalid frame %+v at byte %d", set, next)
			}
			if set.ID != binary.BigEndian.Uint16(datagram[next:]) || set.Length != binary.BigEndian.Uint16(datagram[next+2:]) {
				t.Fatalf("frame differs from wire at byte %d", next)
			}
			next += int(set.Length)
		}
		if next != len(datagram) {
			t.Fatalf("framed %d of %d bytes", next, len(datagram))
		}
	})
}
