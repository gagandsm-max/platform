package flow

import (
	"encoding/binary"
	"errors"
)

const (
	v9HeaderLength    = 20
	v9SetHeaderLength = 4
	v9MaxUDPPayload   = 65535 - 8 // Ordinary UDP length field includes the UDP header.
)

var (
	ErrMalformedV9Envelope  = errors.New("malformed NetFlow v9 envelope")
	ErrUnsupportedV9Version = errors.New("unsupported NetFlow v9 envelope version")
)

// V9Envelope contains raw exporter-reported fields and the ordered FlowSet frames
// in one UDP payload. SourceID is the claimed observation domain.
type V9Envelope struct {
	Version        uint16
	Count          uint16 // Reported total records across FlowSets.
	SysUpTime      uint32 // Milliseconds since exporter boot.
	UnixSecs       uint32 // Export time in Unix seconds.
	SequenceNumber uint32 // Export packet counter for this observation domain.
	SourceID       uint32
	FlowSets       []V9FlowSet
}

// V9FlowSet locates an opaque FlowSet in the original datagram. Offset points to
// its ID, and Length covers the set header, body and any exporter padding.
type V9FlowSet struct {
	ID     uint16
	Offset uint16
	Length uint16
}

// ParseV9Envelope checks RFC 3954 packet and FlowSet framing only. It leaves
// record counts, templates, payload semantics and exporter authority unverified.
// The returned metadata retains no reference to the datagram.
func ParseV9Envelope(datagram []byte) (*V9Envelope, error) {
	if len(datagram) < v9HeaderLength || len(datagram) > v9MaxUDPPayload {
		return nil, ErrMalformedV9Envelope
	}
	version := binary.BigEndian.Uint16(datagram[:2])
	if version != 9 {
		return nil, ErrUnsupportedV9Version
	}
	if len(datagram) == v9HeaderLength {
		return nil, ErrMalformedV9Envelope
	}

	// Validate all boundaries before allocating descriptors for untrusted input.
	setCount := 0
	for offset := v9HeaderLength; offset < len(datagram); {
		remaining := datagram[offset:]
		if len(remaining) < v9SetHeaderLength {
			return nil, ErrMalformedV9Envelope
		}
		length := binary.BigEndian.Uint16(remaining[2:4])
		if length < v9SetHeaderLength || int(length) > len(remaining) {
			return nil, ErrMalformedV9Envelope
		}
		setCount++
		offset += int(length)
	}

	sets := make([]V9FlowSet, setCount)
	offset := v9HeaderLength
	for i := range sets {
		header := datagram[offset : offset+v9SetHeaderLength]
		length := binary.BigEndian.Uint16(header[2:4])
		sets[i] = V9FlowSet{
			ID:     binary.BigEndian.Uint16(header[:2]),
			Offset: uint16(offset),
			Length: length,
		}
		offset += int(length)
	}

	return &V9Envelope{
		Version:        version,
		Count:          binary.BigEndian.Uint16(datagram[2:4]),
		SysUpTime:      binary.BigEndian.Uint32(datagram[4:8]),
		UnixSecs:       binary.BigEndian.Uint32(datagram[8:12]),
		SequenceNumber: binary.BigEndian.Uint32(datagram[12:16]),
		SourceID:       binary.BigEndian.Uint32(datagram[16:20]),
		FlowSets:       sets,
	}, nil
}
