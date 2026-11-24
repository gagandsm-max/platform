package flow

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
)

var (
	ErrMalformedSFlowV5        = errors.New("malformed sFlow v5 datagram")
	ErrUnsupportedSFlowVersion = errors.New("unsupported sFlow datagram version")
)

// SFlowV5Datagram holds one decoded datagram. Uptime is in milliseconds.
// An address type of zero means the agent did not report an address.
type SFlowV5Datagram struct {
	AgentAddress     netip.Addr
	AgentAddressType uint32
	SubAgentID       uint32
	SequenceNumber   uint32
	Uptime           uint32
	Samples          []SFlowV5Sample
}

// SFlowV5Sample holds either a standard flow/counter sample or an opaque
// sample. Rate, pool, drops and interface fields are present in flow samples.
type SFlowV5Sample struct {
	Enterprise, Format              uint32
	SequenceNumber                  uint32
	SourceType, SourceIndex         uint32
	SamplingRate, SamplePool, Drops uint32
	InputFormat, InputValue         uint32
	OutputFormat, OutputValue       uint32
	Records                         []SFlowV5Record
	Opaque, Extension               []byte
}

// SFlowV5Record retains unknown formats in their enclosing sample's namespace.
type SFlowV5Record struct {
	Enterprise, Format uint32
	Opaque             []byte
	Header             *SFlowV5Header
}

// SFlowV5Header separates the exporter's reported frame and stripped lengths
// from the octets actually captured. The capture length is len(Captured).
type SFlowV5Header struct {
	Protocol, FrameLength, Stripped uint32
	Captured, Extension             []byte
}

// ParseSFlowV5 decodes one non-jumbogram UDP payload. The result owns its
// byte slices and remains valid after the receive buffer is reused.
func ParseSFlowV5(data []byte) (*SFlowV5Datagram, error) {
	// The largest ordinary IPv6 UDP payload is 65535 minus its 8-byte header.
	if len(data) > 65535-8 {
		return nil, ErrMalformedSFlowV5
	}
	// Check the entire envelope and all nested counts before allocating output
	// from them. A malformed suffix therefore cannot allocate a valid prefix.
	if _, err := readSFlowV5(data, false); err != nil {
		return nil, err
	}
	packet, err := readSFlowV5(bytes.Clone(data), true)
	if err != nil {
		return nil, err
	}
	return &packet, nil
}

type sflowXDR struct {
	data []byte
	bad  bool
}

func (x *sflowXDR) take(n int) []byte {
	if x.bad || len(x.data) < n {
		x.bad = true
		return nil
	}
	b := x.data[:n:n]
	x.data = x.data[n:]
	return b
}

func (x *sflowXDR) word() uint32 {
	b := x.take(4)
	if b == nil {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

func (x *sflowXDR) opaque() []byte {
	n := x.word()
	if x.bad || n > uint32(len(x.data)) {
		x.bad = true
		return nil
	}
	b := x.take(int(n))
	for _, pad := range x.take(int(-n & 3)) {
		if pad != 0 {
			x.bad = true
		}
	}
	return b
}

// Each element of an sFlow array starts with a tag and an opaque length.
func (x *sflowXDR) arrayCount() int {
	n := x.word()
	if x.bad || n > uint32(len(x.data)/8) {
		x.bad = true
		return 0
	}
	return int(n)
}

func readSFlowV5(data []byte, materialize bool) (SFlowV5Datagram, error) {
	var packet SFlowV5Datagram
	x := sflowXDR{data: data}
	version := x.word()
	if x.bad {
		return packet, ErrMalformedSFlowV5
	}
	if version != 5 {
		return packet, ErrUnsupportedSFlowVersion
	}
	packet.AgentAddressType = x.word()
	switch packet.AgentAddressType {
	case 0: // UNKNOWN has no address payload.
	case 1:
		if b := x.take(4); b != nil {
			packet.AgentAddress = netip.AddrFrom4([4]byte(b))
		}
	case 2:
		if b := x.take(16); b != nil {
			packet.AgentAddress = netip.AddrFrom16([16]byte(b))
		}
	default:
		return packet, ErrMalformedSFlowV5
	}
	packet.SubAgentID = x.word()
	packet.SequenceNumber = x.word()
	packet.Uptime = x.word()
	n := x.arrayCount()
	if materialize && n != 0 {
		packet.Samples = make([]SFlowV5Sample, n)
	}
	var compact, expanded bool
	for i := 0; i < n; i++ {
		tag := x.word()
		body := x.opaque()
		if x.bad {
			return packet, ErrMalformedSFlowV5
		}
		// The agent must use one encoding for its standard sample families.
		compact = compact || tag == 1 || tag == 2
		expanded = expanded || tag == 3 || tag == 4
		if compact && expanded {
			return packet, ErrMalformedSFlowV5
		}
		sample, err := readSFlowSample(tag, body, materialize)
		if err != nil {
			return packet, err
		}
		if materialize {
			packet.Samples[i] = sample
		}
	}
	if x.bad || len(x.data) != 0 {
		return packet, ErrMalformedSFlowV5
	}
	return packet, nil
}

func readSFlowSample(tag uint32, body []byte, materialize bool) (SFlowV5Sample, error) {
	sample := SFlowV5Sample{Enterprise: tag >> 12, Format: tag & 0xfff}
	if tag < 1 || tag > 4 {
		sample.Opaque = body
		return sample, nil
	}

	x := sflowXDR{data: body}
	sample.SequenceNumber = x.word()
	if tag <= 2 {
		source := x.word()
		sample.SourceType, sample.SourceIndex = source>>24, source&0xffffff
	} else {
		sample.SourceType, sample.SourceIndex = x.word(), x.word()
	}
	flowSample := tag == 1 || tag == 3
	if flowSample {
		sample.SamplingRate, sample.SamplePool, sample.Drops = x.word(), x.word(), x.word()
		if tag == 1 {
			input, output := x.word(), x.word()
			sample.InputFormat, sample.InputValue = input>>30, input&0x3fffffff
			sample.OutputFormat, sample.OutputValue = output>>30, output&0x3fffffff
		} else {
			sample.InputFormat, sample.InputValue = x.word(), x.word()
			sample.OutputFormat, sample.OutputValue = x.word(), x.word()
		}
	}
	n := x.arrayCount()
	if materialize && n != 0 {
		sample.Records = make([]SFlowV5Record, n)
	}
	for i := 0; i < n; i++ {
		recordTag := x.word()
		data := x.opaque()
		if x.bad {
			return sample, ErrMalformedSFlowV5
		}
		record := SFlowV5Record{Enterprise: recordTag >> 12, Format: recordTag & 0xfff}
		if flowSample && recordTag == 1 {
			h := sflowXDR{data: data}
			protocol, frameLength, stripped := h.word(), h.word(), h.word()
			captured := h.opaque()
			if h.bad {
				return sample, ErrMalformedSFlowV5
			}
			if materialize {
				record.Header = &SFlowV5Header{
					Protocol: protocol, FrameLength: frameLength, Stripped: stripped,
					Captured: captured, Extension: h.data,
				}
			}
		} else {
			record.Opaque = data
		}
		if materialize {
			sample.Records[i] = record
		}
	}
	if x.bad {
		return sample, ErrMalformedSFlowV5
	}
	// SFLOW-DATAGRAM5 allows opaque standard and enterprise structures to
	// append fields without changing their tag. The parent supplied the bound.
	sample.Extension = x.data
	return sample, nil
}
