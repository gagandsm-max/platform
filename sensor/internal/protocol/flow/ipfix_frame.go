package flow

import (
	"encoding/binary"
	"errors"
)

const (
	ipfixMessageHeaderLength = 16
	ipfixSetHeaderLength     = 4
	ipfixMaxMessageLength    = 65535 // RFC 7011's 16-bit IPFIX Message Length.
)

var (
	ErrMalformedIPFIXEnvelope  = errors.New("malformed IPFIX envelope")
	ErrUnsupportedIPFIXVersion = errors.New("unsupported IPFIX version")
)

// IPFIXEnvelope contains only the exporter-reported header and the ordered Set
// framing (RFC 7011 §§3.1, 3.3.2). ObservationDomainID is untrusted input,
// not a tenant, device or exporter authorization. SequenceNumber counts Data
// Records, not messages or Sets, and is not validated by this framing layer.
// The returned envelope owns its metadata and does not retain datagram bytes.
type IPFIXEnvelope struct {
	Version             uint16
	Length              uint16
	ExportTime          uint32
	SequenceNumber      uint32
	ObservationDomainID uint32
	Sets                []IPFIXSet
}

// IPFIXSet records the ID and the complete wire extent of an opaque Set.
// Offset points to the 4-byte Set header; Length includes any padding.
// Set IDs 2/3 signal Template/Options Template; >=256 signal Data Sets.
// Reserved IDs are retained only as opaque framing evidence, never granted
// template or data-record semantics by this parser.
type IPFIXSet struct {
	ID     uint16
	Offset uint16
	Length uint16
}

// ParseIPFIXEnvelope validates a single IPFIX v10 message's byte boundaries.
// It accepts a header-only message: RFC 7011 permits zero or more Sets.
// Template withdrawal, template state, row decoding, trusted exporter binding,
// UDP admission and tenant authority are deliberately out of scope.
// On any malformed envelope it returns nil with an error (no partial result).
func ParseIPFIXEnvelope(datagram []byte) (*IPFIXEnvelope, error) {
	if len(datagram) < ipfixMessageHeaderLength || len(datagram) > ipfixMaxMessageLength {
		return nil, ErrMalformedIPFIXEnvelope
	}
	if binary.BigEndian.Uint16(datagram[0:2]) != 10 {
		return nil, ErrUnsupportedIPFIXVersion
	}
	if int(binary.BigEndian.Uint16(datagram[2:4])) != len(datagram) {
		return nil, ErrMalformedIPFIXEnvelope
	}

	// Validate every length before allocating descriptor storage or returning
	// any output. Never use header SequenceNumber to infer Set/row counts.
	setCount := 0
	for off := ipfixMessageHeaderLength; off < len(datagram); {
		remaining := len(datagram) - off
		if remaining < ipfixSetHeaderLength {
			return nil, ErrMalformedIPFIXEnvelope
		}
		length := int(binary.BigEndian.Uint16(datagram[off+2 : off+4]))
		if length < ipfixSetHeaderLength || length > remaining {
			return nil, ErrMalformedIPFIXEnvelope
		}
		setCount++
		off += length
	}

	sets := make([]IPFIXSet, setCount)
	off := ipfixMessageHeaderLength
	for i := range sets {
		sets[i] = IPFIXSet{
			ID:     binary.BigEndian.Uint16(datagram[off : off+2]),
			Offset: uint16(off),
			Length: binary.BigEndian.Uint16(datagram[off+2 : off+4]),
		}
		off += int(sets[i].Length)
	}

	return &IPFIXEnvelope{
		Version:             10,
		Length:              uint16(len(datagram)),
		ExportTime:          binary.BigEndian.Uint32(datagram[4:8]),
		SequenceNumber:      binary.BigEndian.Uint32(datagram[8:12]),
		ObservationDomainID: binary.BigEndian.Uint32(datagram[12:16]),
		Sets:                sets,
	}, nil
}
