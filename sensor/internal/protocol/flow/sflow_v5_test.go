package flow

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
	"reflect"
	"testing"
)

func sfWords(values ...uint32) []byte {
	var b []byte
	for _, v := range values {
		b = binary.BigEndian.AppendUint32(b, v)
	}
	return b
}

func sfTLV(tag uint32, body []byte) []byte {
	b := append(sfWords(tag, uint32(len(body))), body...)
	return append(b, make([]byte, -len(body)&3)...)
}

func sfDatagram(addressType uint32, address []byte, samples ...[]byte) []byte {
	b := append(sfWords(5, addressType), address...)
	b = append(b, sfWords(9, 0xfffffffe, 8000, uint32(len(samples)))...)
	for _, sample := range samples {
		b = append(b, sample...)
	}
	return b
}

func sfCompactPacket() []byte {
	header := append(sfWords(1, 9000, 4, 3), 0x45, 0x23, 0x01, 0)
	header = append(header, 0xaa, 0xbb)
	flowSample := sfWords(321, 0x02012345, 1000, 50_000, 2, 75, 0x8000000b, 2)
	flowSample = append(flowSample, sfTLV(1, header)...)
	flowSample = append(flowSample, sfTLV(55<<12|1, []byte{1, 2, 3})...)
	flowSample = append(flowSample, 0xcc)
	counterSample := sfWords(456, 0x0100007f, 1)
	counterSample = append(counterSample, sfTLV(1, []byte{0xde, 0xad})...)
	return sfDatagram(1, []byte{192, 0, 2, 7}, sfTLV(1, flowSample),
		sfTLV(2, counterSample), sfTLV(0xfffff001, []byte{0xee}))
}

func TestSFlowV5CompactSamples(t *testing.T) {
	data := sfCompactPacket()
	d, err := ParseSFlowV5(data)
	if err != nil {
		t.Fatal(err)
	}
	clear(data) // UDP buffer is immediately available for a subsequent read.
	if d.AgentAddressType != 1 || d.AgentAddress != netip.MustParseAddr("192.0.2.7") ||
		d.SubAgentID != 9 || d.SequenceNumber != 0xfffffffe || d.Uptime != 8000 || len(d.Samples) != 3 {
		t.Fatalf("datagram %+v", d)
	}
	wantFlow := SFlowV5Sample{
		Format: 1, SequenceNumber: 321, SourceType: 2, SourceIndex: 0x12345,
		SamplingRate: 1000, SamplePool: 50_000, Drops: 2,
		InputValue: 75, OutputFormat: 2, OutputValue: 11, Extension: []byte{0xcc},
		Records: []SFlowV5Record{
			{Format: 1, Header: &SFlowV5Header{
				Protocol: 1, FrameLength: 9000, Stripped: 4,
				Captured: []byte{0x45, 0x23, 0x01}, Extension: []byte{0xaa, 0xbb},
			}},
			{Enterprise: 55, Format: 1, Opaque: []byte{1, 2, 3}},
		},
	}
	if !reflect.DeepEqual(d.Samples[0], wantFlow) {
		t.Fatalf("flow sample %+v, records %+v", d.Samples[0], d.Samples[0].Records)
	}
	c := d.Samples[1]
	if c.Format != 2 || c.SourceType != 1 || c.SourceIndex != 127 || c.SequenceNumber != 456 ||
		len(c.Records) != 1 || c.Records[0].Format != 1 || c.Records[0].Header != nil ||
		!bytes.Equal(c.Records[0].Opaque, []byte{0xde, 0xad}) {
		t.Fatalf("counter sample %+v", c)
	}
	u := d.Samples[2]
	if u.Enterprise != 0xfffff || u.Format != 1 || len(u.Records) != 0 || !bytes.Equal(u.Opaque, []byte{0xee}) {
		t.Fatalf("enterprise sample %+v", u)
	}
}

func TestSFlowV5ExpandedSamples(t *testing.T) {
	addr := netip.MustParseAddr("2001:db8::babe")
	data := sfDatagram(2, addr.AsSlice(),
		sfTLV(3, sfWords(100, 2, 0xf1234567, 200, 300, 99, 0, 0xdeadbeef, 2, 0xffffffff, 0)),
		sfTLV(4, sfWords(212, 1, 0xf2234567, 0)))
	d, err := ParseSFlowV5(data)
	if err != nil {
		t.Fatal(err)
	}
	f, c := d.Samples[0], d.Samples[1]
	if d.AgentAddress != addr || d.AgentAddressType != 2 || f.SourceType != 2 || f.SourceIndex != 0xf1234567 ||
		f.SequenceNumber != 100 || f.SamplingRate != 200 || f.SamplePool != 300 || f.Drops != 99 ||
		f.InputFormat != 0 || f.InputValue != 0xdeadbeef || f.OutputFormat != 2 || f.OutputValue != 0xffffffff ||
		c.Format != 4 || c.SourceType != 1 || c.SourceIndex != 0xf2234567 || c.SequenceNumber != 212 ||
		len(f.Records) != 0 || len(c.Records) != 0 {
		t.Fatalf("expanded datagram %+v", d)
	}
}

func TestSFlowV5EmptyAndMaximumDatagrams(t *testing.T) {
	d, err := ParseSFlowV5(sfDatagram(0, nil))
	if err != nil || d.AgentAddress.IsValid() || d.AgentAddressType != 0 || len(d.Samples) != 0 {
		t.Fatalf("empty datagram %+v, err %v", d, err)
	}
	// Largest whole-XDR datagram fitting an ordinary UDP length.
	packet := sfDatagram(0, nil, sfTLV(4097, make([]byte, 65524-32)))
	if len(packet) != 65524 {
		t.Fatal(len(packet))
	}
	d, err = ParseSFlowV5(packet)
	if err != nil || len(d.Samples[0].Opaque) != 65524-32 {
		t.Fatalf("largest datagram: %v", err)
	}
	// The receiver bounds captures by their opaque length even if the agent
	// could not report the true frame length.
	header := append(sfWords(11, 0, 0, 1), 7, 0, 0, 0)
	sample := append(sfWords(0, 0, 0, 0, 0, 0, 0, 1), sfTLV(1, header)...)
	d, err = ParseSFlowV5(sfDatagram(0, nil, sfTLV(1, sample)))
	if err != nil || !bytes.Equal(d.Samples[0].Records[0].Header.Captured, []byte{7}) {
		t.Fatalf("capture with unknown frame length: %v", err)
	}
}

func TestSFlowV5StructuralErrors(t *testing.T) {
	packet := sfCompactPacket()
	for size := 0; size < len(packet); size++ {
		d, err := ParseSFlowV5(packet[:size])
		if !errors.Is(err, ErrMalformedSFlowV5) || d != nil {
			t.Fatalf("prefix length %d: %+v, %v", size, d, err)
		}
	}
	// Fields below refer to the generated IPv4 packet: sample header at 28,
	// compact flow at 36, raw-header record at 68, captured bytes at 92.
	for _, tc := range []struct {
		name   string
		offset int
		value  uint32
	}{
		{"address_discriminant", 4, 42},
		{"sample_count", 24, 0xffffffff},
		{"outer_unconsumed_sample", 24, 2},
		{"sample_length", 32, 0xfffffff9},
		{"sample_fixed_fields_short", 32, 28},
		{"record_count", 64, 0xffffffff},
		{"record_length", 72, 0xffffffff},
		{"header_fixed_fields_short", 72, 12},
		{"capture_length", 88, 0xffffffff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := bytes.Clone(packet)
			binary.BigEndian.PutUint32(input[tc.offset:], tc.value)
			d, err := ParseSFlowV5(input)
			if d != nil || !errors.Is(err, ErrMalformedSFlowV5) {
				t.Fatalf("received %+v, %v", d, err)
			}
		})
	}
	for _, offset := range []int{95, 98, 111, len(packet) - 1} {
		// Inner header capture, raw-header record, vendor record and vendor sample padding.
		input := bytes.Clone(packet)
		input[offset] = 1
		if d, err := ParseSFlowV5(input); d != nil || !errors.Is(err, ErrMalformedSFlowV5) {
			t.Fatalf("padding at %d: %+v, %v", offset, d, err)
		}
	}
	for _, input := range [][]byte{
		append(bytes.Clone(packet), 0, 0, 0, 0),
		sfDatagram(0, nil, sfTLV(2, sfWords(1, 1, 0)), sfTLV(4, sfWords(2, 1, 1, 0))),
		sfDatagram(0, nil, sfTLV(1, nil)),
		sfDatagram(0, nil, sfTLV(3, sfWords(0, 0, 0))),
		make([]byte, 65528),
	} {
		if d, err := ParseSFlowV5(input); d != nil || !errors.Is(err, ErrMalformedSFlowV5) {
			t.Fatalf("unexpected result, length %d: %+v, %v", len(input), d, err)
		}
	}
	if d, err := ParseSFlowV5(sfWords(4)); d != nil || !errors.Is(err, ErrUnsupportedSFlowVersion) {
		t.Fatalf("version: %+v, %v", d, err)
	}
}

func TestSFlowV5MalformedSuffixAllocations(t *testing.T) {
	invalid := append(sfCompactPacket(), 0, 0, 0, 0)
	if n := testing.AllocsPerRun(20, func() {
		if d, err := ParseSFlowV5(invalid); d != nil || err == nil {
			t.Fatal("accepted trailing bytes")
		}
	}); n > 0 {
		t.Fatalf("%.1f output allocations on an invalid datagram", n)
	}
}

func FuzzSFlowV5Datagram(f *testing.F) {
	f.Add(sfCompactPacket())
	f.Add(sfDatagram(0, nil))
	f.Add(sfDatagram(2, netip.MustParseAddr("::1").AsSlice(), sfTLV(4, sfWords(12, 1, 8, 0))))
	f.Fuzz(func(t *testing.T, data []byte) {
		d, err := ParseSFlowV5(data)
		if err != nil {
			if d != nil {
				t.Fatal("partial datagram after error")
			}
			return
		}
		if d == nil || len(d.Samples) > len(data)/8 {
			t.Fatalf("invalid sample count, size %d", len(data))
		}
		for _, s := range d.Samples {
			if len(s.Records) > len(data)/8 {
				t.Fatalf("invalid record count, size %d", len(data))
			}
		}
	})
}
