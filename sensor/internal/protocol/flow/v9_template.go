package flow

import (
	"encoding/binary"
	"errors"
)

var ErrMalformedV9Template = errors.New("malformed NetFlow v9 template")

// V9TemplateKind is the defining FlowSet ID on the wire.
type V9TemplateKind uint16

const (
	V9DataTemplate    V9TemplateKind = 0
	V9OptionsTemplate V9TemplateKind = 1
)

// V9TemplateField preserves the raw field type and its advertised width in bytes.
type V9TemplateField struct {
	Type   uint16
	Length uint16
}

// V9Template describes one definition. ScopeFields precede Fields on the wire
// and are present only in Options Templates. Fields holds the ordinary fields
// of a Data Template or the non-scope fields of an Options Template.
type V9Template struct {
	ID          uint16
	Kind        V9TemplateKind
	ScopeFields []V9TemplateField
	Fields      []V9TemplateField
}

// V9TemplatePacket groups definitions with the exporter-reported observation
// domain of the same packet. SourceID carries no exporter or tenant authority.
type V9TemplatePacket struct {
	SourceID  uint32
	Templates []V9Template
}

// ParseV9Templates reads RFC 3954 sections 5.2 and 6.1 definitions in wire
// order, including repeated IDs. The supported profile requires at least one
// field descriptor per definition. Individual field widths are raw metadata.
// Other FlowSets remain opaque and the reported packet count is not validated.
// An error discards all definitions. The result owns its data.
func ParseV9Templates(datagram []byte) (*V9TemplatePacket, error) {
	envelope, err := ParseV9Envelope(datagram)
	if err != nil {
		return nil, err
	}
	// Validate and count actual descriptors before allocating result storage.
	records, descriptors, err := scanV9Templates(datagram, envelope.FlowSets, nil, nil)
	if err != nil {
		return nil, err
	}
	packet := &V9TemplatePacket{SourceID: envelope.SourceID}
	if records == 0 {
		return packet, nil
	}
	packet.Templates = make([]V9Template, records)
	fields := make([]V9TemplateField, descriptors)
	if _, _, err := scanV9Templates(datagram, envelope.FlowSets, packet.Templates, fields); err != nil {
		return nil, err
	}
	return packet, nil
}

func scanV9Templates(datagram []byte, sets []V9FlowSet, templates []V9Template, fields []V9TemplateField) (int, int, error) {
	var records, descriptors int
	for _, set := range sets {
		kind := V9TemplateKind(set.ID)
		if kind != V9DataTemplate && kind != V9OptionsTemplate {
			continue
		}
		offset := int(set.Offset)
		end := offset + int(set.Length)
		body := datagram[offset+v9SetHeaderLength : end]
		for {
			// RFC 3954 section 2 requires at least one record in either set.
			header, scopeCount, fieldCount, err := v9TemplateLayout(kind, body)
			if err != nil {
				return 0, 0, err
			}
			count := scopeCount + fieldCount
			size := header + 4*count
			if count == 0 || size > len(body) {
				return 0, 0, ErrMalformedV9Template
			}
			if templates != nil {
				record := &templates[records]
				record.ID = binary.BigEndian.Uint16(body[:2])
				record.Kind = kind
				// Bound capacity so appending to a slice cannot overwrite a
				// sibling field group or another template's descriptors.
				dst := fields[descriptors : descriptors+count : descriptors+count]
				for i := range dst {
					spec := body[header+4*i:]
					dst[i] = V9TemplateField{Type: binary.BigEndian.Uint16(spec), Length: binary.BigEndian.Uint16(spec[2:])}
				}
				if scopeCount > 0 {
					record.ScopeFields = dst[:scopeCount:scopeCount]
				}
				if fieldCount > 0 {
					record.Fields = dst[scopeCount:]
				}
			}
			records++
			descriptors += count
			body = body[size:]
			// Section 6.1 permits optional trailing padding to align the
			// next FlowSet to a 4-byte packet boundary. Zeros are advised.
			if len(body) == 0 || (kind == V9OptionsTemplate && len(body) < 4 && end%4 == 0) {
				break
			}
		}
	}
	return records, descriptors, nil
}

// v9TemplateLayout returns the header size and descriptor counts. Widen all
// wire values to int before computing sizes, even on 32-bit platforms.
func v9TemplateLayout(kind V9TemplateKind, body []byte) (header, scopes, fields int, err error) {
	if len(body) < 4 || binary.BigEndian.Uint16(body) < 256 {
		return 0, 0, 0, ErrMalformedV9Template
	}
	if kind == V9DataTemplate {
		return 4, 0, int(binary.BigEndian.Uint16(body[2:4])), nil
	}
	if len(body) < 6 {
		return 0, 0, 0, ErrMalformedV9Template
	}
	scopeBytes := int(binary.BigEndian.Uint16(body[2:4]))
	optionBytes := int(binary.BigEndian.Uint16(body[4:6]))
	if scopeBytes%4 != 0 || optionBytes%4 != 0 {
		return 0, 0, 0, ErrMalformedV9Template
	}
	return 6, scopeBytes / 4, optionBytes / 4, nil
}
