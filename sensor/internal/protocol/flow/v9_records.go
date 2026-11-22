package flow

import (
	"errors"
	"slices"
	"time"
)

var ErrMalformedV9Record = errors.New("malformed NetFlow v9 data record")

// V9RecordLayout describes a single fixed-width row. All scope descriptors
// precede the other fields. Field returns a value, so its storage stays private.
type V9RecordLayout struct {
	kind          V9TemplateKind
	scopes, width int
	fields        []V9TemplateField
}

func (l V9RecordLayout) Kind() V9TemplateKind        { return l.kind }
func (l V9RecordLayout) ScopeCount() int             { return l.scopes }
func (l V9RecordLayout) Width() int                  { return l.width }
func (l V9RecordLayout) FieldCount() int             { return len(l.fields) }
func (l V9RecordLayout) Field(i int) V9TemplateField { return l.fields[i] }

type V9SetStatus uint8

const (
	V9Decoded V9SetStatus = iota
	V9TemplateMissing
	V9TemplateExpired
	V9PaddingAmbiguous
)

// V9RecordSet is one Data FlowSet. On successful decode Raw owns Count rows
// of Layout.Width() bytes each, in wire order, with alignment padding removed.
// Unresolved sets contain no rows. RefreshedAt is the last accepted definition's
// trusted receive time when available.
type V9RecordSet struct {
	TemplateID  uint16
	Status      V9SetStatus
	Layout      *V9RecordLayout
	Raw         []byte
	Count       int
	RefreshedAt time.Time
}

// VisitRow visits the scope and field values of row index in wire order. Each
// value borrows only its own bytes in Raw. Returning false from visit stops the
// walk. VisitRow returns false for invalid row geometry, status or arguments.
func (s V9RecordSet) VisitRow(index int, visit func(V9TemplateField, []byte) bool) bool {
	if visit == nil || s.Status != V9Decoded || s.Layout == nil || index < 0 || index >= s.Count {
		return false
	}
	layout := *s.Layout
	if layout.width <= 0 || len(s.Raw)%layout.width != 0 || len(s.Raw)/layout.width != s.Count {
		return false
	}
	start := index * layout.width
	row := s.Raw[start : start+layout.width]
	for _, field := range layout.fields {
		n := int(field.Length)
		if !visit(field, row[:n:n]) {
			break
		}
		row = row[n:]
	}
	return true
}

type V9RecordPacket struct {
	Key                            V9StreamKey
	ReceivedAt                     time.Time
	Timeout                        time.Duration
	SequenceNumber, MissingPackets uint32
	UnixSecs, SysUpTime            uint32 // Exporter-reported clocks from this packet.
	Sets                           []V9RecordSet
}

// RFC 3954 sections 5.3 and 6.2 allow unpadded data or padding to a 4-byte
// packet boundary. Padding can be nonzero. Count every possible positive row
// count before trusting the boundary, irrespective of the packet's Count.
func v9DataSet(id uint16, body []byte, end int, entry v9CacheEntry, now time.Time, timeout time.Duration, emit bool) (V9RecordSet, error) {
	set := V9RecordSet{TemplateID: id, RefreshedAt: entry.at}
	if len(body) == 0 {
		return set, ErrMalformedV9Record
	}
	if entry.layout == nil {
		set.Status = V9TemplateMissing
		return set, nil
	}
	if now.Sub(entry.at) >= timeout {
		set.Status = V9TemplateExpired
		return set, nil
	}
	width := entry.layout.width
	maxPad := 0
	if end%4 == 0 {
		maxPad = 3
	}
	for pad := 0; pad <= maxPad; pad++ {
		n := len(body) - pad
		if n < width || n%width != 0 {
			continue
		}
		if set.Count != 0 {
			set.Count = 0
			set.Status = V9PaddingAmbiguous
			return set, nil
		}
		set.Count = n / width
	}
	if set.Count == 0 {
		return set, ErrMalformedV9Record
	}
	if !emit {
		return set, nil
	}
	// The public header can be overwritten by callers; keep the cache's
	// header separate. Its descriptor array is private and immutable.
	layout := *entry.layout
	set.Layout = &layout
	set.Raw = slices.Clone(body[:set.Count*width])
	return set, nil
}
