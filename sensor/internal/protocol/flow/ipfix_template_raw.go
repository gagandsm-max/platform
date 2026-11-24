package flow

import (
	"encoding/binary"
	"errors"
)

var ErrMalformedIPFIXRawTemplate = errors.New("malformed IPFIX raw template")

// IPFIXRawField preserves wire descriptors, including enterprise-specific IDs
// and variable-length (65535) fields. It does not resolve Information Elements.
type IPFIXRawField struct {
	ElementID        uint16
	Length           uint16
	EnterpriseNumber uint32
	Enterprise       bool
}

// IPFIXRawTemplate is a raw Template or Options Template definition or a
// withdrawal request in wire order. A Withdrawal is evidence only: UDP
// withdrawals MUST NOT revoke a collector's existing template authority.
// AllTemplates is true for the special Set 2/ID 2 or Set 3/ID 3 form.
type IPFIXRawTemplate struct {
	ID           uint16
	Options      bool
	ScopeCount   uint16
	Withdrawal   bool
	AllTemplates bool
	Fields       []IPFIXRawField
}

// IPFIXRawTemplatePacket contains only untrusted exporter-reported metadata.
// ObservationDomainID is NOT a tenant or authenticated exporter identifier.
// Returned records and descriptors own their storage, independent of wire.
type IPFIXRawTemplatePacket struct {
	ObservationDomainID uint32
	ExportTime          uint32
	Templates           []IPFIXRawTemplate
}

// ParseIPFIXRawTemplates validates the complete IPFIX message and extracts
// Template Set 2 and Options Template Set 3 records without installing them.
// Data Sets and reserved Set IDs remain opaque. Malformed templates yield no
// partial result. Caller MUST NOT mutate wire concurrently with this call.
// Template keys, lifetimes, UDP withdrawal policy and record decoding are
// intentionally owned by later exporter-bound stateful layers.
func ParseIPFIXRawTemplates(wire []byte) (*IPFIXRawTemplatePacket, error) {
	env, err := ParseIPFIXEnvelope(wire)
	if err != nil {
		return nil, err
	}
	records, descriptors, err := scanIPFIXRawTemplates(wire, env.Sets, nil, nil)
	if err != nil {
		return nil, err
	}
	packet := &IPFIXRawTemplatePacket{ObservationDomainID: env.ObservationDomainID, ExportTime: env.ExportTime}
	if records == 0 {
		return packet, nil
	}
	packet.Templates = make([]IPFIXRawTemplate, records)
	fields := make([]IPFIXRawField, descriptors)
	if _, _, err := scanIPFIXRawTemplates(wire, env.Sets, packet.Templates, fields); err != nil {
		return nil, err
	}
	return packet, nil
}

// scanIPFIXRawTemplates first validates and counts on a nil destination, then
// walks the same immutable wire to populate pre-sized private slices. Both
// passes reject incomplete enterprise specifiers before materializing results.
func scanIPFIXRawTemplates(wire []byte, sets []IPFIXSet, templates []IPFIXRawTemplate, fields []IPFIXRawField) (int, int, error) {
	var records, descriptors int
	for _, set := range sets {
		if set.ID != 2 && set.ID != 3 {
			continue
		}
		body := wire[int(set.Offset)+ipfixSetHeaderLength : int(set.Offset)+int(set.Length)]
		if len(body) == 0 { // A Template Set contains at least one record.
			return 0, 0, ErrMalformedIPFIXRawTemplate
		}
		setRecords := 0
		for len(body) != 0 {
			if len(body) < 4 {
				// Padding is shorter than the smallest Template Record, and
				// RFC 7011 requires every padding byte to be zero.
				if setRecords == 0 || !allZeroIPFIXPadding(body) {
					return 0, 0, ErrMalformedIPFIXRawTemplate
				}
				break
			}
			id := binary.BigEndian.Uint16(body[0:2])
			count := int(binary.BigEndian.Uint16(body[2:4]))
			options := set.ID == 3
			record := IPFIXRawTemplate{ID: id, Options: options}
			header := 4
			if count == 0 {
				// RFC 7011 Section 8.1 uses 4-byte withdrawal records for
				// BOTH Template and Options Template Sets; even the special
				// withdraw-all IDs have no Options scope count.
				if id < 256 && id != set.ID {
					return 0, 0, ErrMalformedIPFIXRawTemplate
				}
				record.Withdrawal = true
				record.AllTemplates = id == set.ID
			} else {
				if id < 256 {
					return 0, 0, ErrMalformedIPFIXRawTemplate
				}
				if options {
					if len(body) < 6 {
						return 0, 0, ErrMalformedIPFIXRawTemplate
					}
					record.ScopeCount = binary.BigEndian.Uint16(body[4:6])
					if record.ScopeCount == 0 || int(record.ScopeCount) > count {
						return 0, 0, ErrMalformedIPFIXRawTemplate
					}
					header = 6
				}
			}
			pos := header
			for i := 0; i < count; i++ {
				if len(body)-pos < 4 {
					return 0, 0, ErrMalformedIPFIXRawTemplate
				}
				rawID := binary.BigEndian.Uint16(body[pos : pos+2])
				field := IPFIXRawField{ElementID: rawID & 0x7fff, Length: binary.BigEndian.Uint16(body[pos+2 : pos+4]), Enterprise: rawID&0x8000 != 0}
				pos += 4
				if field.Enterprise {
					if len(body)-pos < 4 {
						return 0, 0, ErrMalformedIPFIXRawTemplate
					}
					field.EnterpriseNumber = binary.BigEndian.Uint32(body[pos : pos+4])
					pos += 4
				}
				if templates != nil {
					fields[descriptors+i] = field
				}
			}
			if templates != nil && count != 0 {
				// Limit capacity to this record to prevent appends aliasing
				// another definition's descriptor storage.
				record.Fields = fields[descriptors : descriptors+count : descriptors+count]
			}
			if templates != nil {
				templates[records] = record
			}
			records++
			setRecords++
			descriptors += count
			body = body[pos:]
		}
	}
	return records, descriptors, nil
}

func allZeroIPFIXPadding(b []byte) bool {
	for _, octet := range b {
		if octet != 0 {
			return false
		}
	}
	return true
}
