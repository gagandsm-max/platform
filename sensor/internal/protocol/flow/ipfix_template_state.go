package flow

import (
	"errors"
	"math"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"
)

var (
	ErrIPFIXTemplateConfig              = errors.New("invalid IPFIX template cache configuration")
	ErrIPFIXTemplateScope               = errors.New("IPFIX template stream context mismatch")
	ErrIPFIXTemplateOrder               = errors.New("old or ambiguous IPFIX template stream order")
	ErrIPFIXTemplateRedefinitionInvalid = errors.New("invalid IPFIX template definition batch")
	ErrIPFIXTemplateCapacity            = errors.New("IPFIX template cache capacity exceeded")
	ErrIPFIXTemplateMissing             = errors.New("IPFIX template missing")
	ErrIPFIXTemplateExpired             = errors.New("IPFIX template expired")
	ErrIPFIXTemplateUncertain           = errors.New("IPFIX template definition uncertain")
)

// IPFIXTemplateStreamKey is supplied by trusted ingress. A loss of continuity
// requires a new epoch and an empty cache.
type IPFIXTemplateStreamKey struct {
	ExporterBinding     string
	Source, Collector   netip.AddrPort
	ObservationDomainID uint32
	Epoch               uint64
}

type IPFIXTemplateCacheLimits struct {
	Timeout                 time.Duration
	MaxTemplates, MaxFields int
}

// IPFIXTemplateSnapshot owns its field descriptors when returned by LookupOrdered.
type IPFIXTemplateSnapshot struct {
	Template    IPFIXRawTemplate
	RefreshedAt time.Time
	ExportTime  uint32
	Generation  uint64
}

type ipfixTemplateEntry struct {
	IPFIXTemplateSnapshot
	uncertain bool
	refusedAt uint32
}

// IPFIXTemplateCache holds volatile definitions for one exact stream key.
// It must not be copied after first use.
type IPFIXTemplateCache struct {
	mu          sync.Mutex
	key         IPFIXTemplateStreamKey
	limits      IPFIXTemplateCacheLimits
	entries     map[uint16]ipfixTemplateEntry
	lastReceive time.Time
	lastExport  uint32
	generation  uint64
}

func NewIPFIXTemplateCache(key IPFIXTemplateStreamKey, limits IPFIXTemplateCacheLimits) (*IPFIXTemplateCache, error) {
	if strings.TrimSpace(key.ExporterBinding) == "" || key.Epoch == 0 ||
		!validV9Endpoint(key.Source) || !validV9Endpoint(key.Collector) ||
		limits.Timeout <= 0 || limits.MaxTemplates <= 0 || limits.MaxFields <= 0 {
		return nil, ErrIPFIXTemplateConfig
	}
	return &IPFIXTemplateCache{key: key, limits: limits, entries: make(map[uint16]ipfixTemplateEntry)}, nil
}

// ApplyOrdered accepts a complete raw template batch only after the caller
// independently establishes current exporter/epoch, packet order and wire
// position. This cache does not prove those properties or grant decode authority.
// The caller must keep packet and its descriptors immutable during the call.
func (c *IPFIXTemplateCache) ApplyOrdered(key IPFIXTemplateStreamKey, receivedAt time.Time, packet *IPFIXRawTemplatePacket) error {
	if c == nil || c.entries == nil || receivedAt.IsZero() || packet == nil {
		return ErrIPFIXTemplateConfig
	}
	if key != c.key || packet.ObservationDomainID != c.key.ObservationDomainID {
		return ErrIPFIXTemplateScope
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	exportTime := packet.ExportTime
	if c.beforeBoundary(receivedAt, exportTime) {
		return ErrIPFIXTemplateOrder
	}

	equalExport := !c.lastReceive.IsZero() && exportTime == c.lastExport
	// An ordered refusal advances the boundary without refreshing any layout.
	c.lastReceive, c.lastExport = receivedAt, exportTime
	if err := c.admit(receivedAt, packet, equalExport); err != nil {
		for _, definition := range packet.Templates {
			if definition.Withdrawal {
				continue
			}
			if entry, ok := c.entries[definition.ID]; ok {
				entry.uncertain, entry.refusedAt = true, exportTime
				c.entries[definition.ID] = entry
			}
		}
		return err
	}
	return nil
}

// admit stages definitions under the lock. It changes retained entries only
// after the entire batch and its final footprint have passed validation.
func (c *IPFIXTemplateCache) admit(receivedAt time.Time, packet *IPFIXRawTemplatePacket, equalExport bool) error {
	seen := make(map[uint16]IPFIXRawTemplate)
	var definitions []IPFIXRawTemplate
	for _, definition := range packet.Templates {
		if definition.Withdrawal {
			if !validIPFIXTemplateWithdrawal(definition) {
				return ErrIPFIXTemplateRedefinitionInvalid
			}
			continue
		}
		if err := validateIPFIXRawDataTemplate(definition); err != nil {
			return ErrIPFIXTemplateRedefinitionInvalid
		}
		if _, err := ipfixRawDataMinRecordLength(definition.Fields); err != nil {
			return ErrIPFIXTemplateRedefinitionInvalid
		}
		if previous, ok := seen[definition.ID]; ok {
			if !sameIPFIXTemplateLayout(previous, definition) {
				return ErrIPFIXTemplateRedefinitionInvalid
			}
			continue
		}
		seen[definition.ID] = definition
		definitions = append(definitions, definition)
	}

	pending := make(map[uint16]IPFIXTemplateSnapshot)
	generation := c.generation
	for _, definition := range definitions {
		prior, known := c.entries[definition.ID]
		identical := known && sameIPFIXTemplateLayout(prior.Template, definition)
		if known && (prior.uncertain && packet.ExportTime <= prior.refusedAt || !identical && equalExport) {
			return ErrIPFIXTemplateOrder
		}
		if identical && packet.ExportTime == prior.ExportTime {
			continue
		}

		version := prior.Generation
		if !identical || prior.uncertain || receivedAt.Sub(prior.RefreshedAt) >= c.limits.Timeout {
			if generation == math.MaxUint64 {
				return ErrIPFIXTemplateCapacity
			}
			generation++
			version = generation
		}
		pending[definition.ID] = IPFIXTemplateSnapshot{
			Template: definition, RefreshedAt: receivedAt, ExportTime: packet.ExportTime, Generation: version,
		}
	}
	if len(pending) == 0 {
		return nil
	}

	templates, fields := c.limits.MaxTemplates, c.limits.MaxFields
	var expired []uint16
	for id, entry := range c.entries {
		if _, ok := pending[id]; ok {
			continue
		}
		// Keep uncertain IDs bounded by the same limits until recovery.
		// Keep equal-time history so a repeat cannot revive an expired layout.
		if !entry.uncertain && entry.ExportTime < packet.ExportTime && receivedAt.Sub(entry.RefreshedAt) >= c.limits.Timeout {
			expired = append(expired, id)
			continue
		}
		templates--
		fields -= len(entry.Template.Fields)
	}
	for _, snapshot := range pending {
		if templates == 0 || len(snapshot.Template.Fields) > fields {
			return ErrIPFIXTemplateCapacity
		}
		templates--
		fields -= len(snapshot.Template.Fields)
	}

	for _, id := range expired {
		delete(c.entries, id)
	}
	for id, snapshot := range pending {
		snapshot.Template.Fields = slices.Clone(snapshot.Template.Fields)
		c.entries[id] = ipfixTemplateEntry{IPFIXTemplateSnapshot: snapshot}
	}
	c.generation = generation
	return nil
}

func validIPFIXTemplateWithdrawal(template IPFIXRawTemplate) bool {
	if len(template.Fields) != 0 || template.ScopeCount != 0 {
		return false
	}
	if !template.AllTemplates {
		return template.ID >= 256
	}
	if template.Options {
		return template.ID == 3
	}
	return template.ID == 2
}

func sameIPFIXTemplateLayout(a, b IPFIXRawTemplate) bool {
	return a.Options == b.Options && a.ScopeCount == b.ScopeCount && slices.Equal(a.Fields, b.Fields)
}

// LookupOrdered returns a detached layout for caller-established current data
// order and wire position. It checks the batch high-water without advancing it
// and performs no decoding or buffering.
func (c *IPFIXTemplateCache) LookupOrdered(key IPFIXTemplateStreamKey, receivedAt time.Time, exportTime uint32, id uint16) (IPFIXTemplateSnapshot, error) {
	if c == nil || c.entries == nil || receivedAt.IsZero() {
		return IPFIXTemplateSnapshot{}, ErrIPFIXTemplateConfig
	}
	if key != c.key {
		return IPFIXTemplateSnapshot{}, ErrIPFIXTemplateScope
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.beforeBoundary(receivedAt, exportTime) {
		return IPFIXTemplateSnapshot{}, ErrIPFIXTemplateOrder
	}
	entry, ok := c.entries[id]
	switch {
	case !ok:
		return IPFIXTemplateSnapshot{}, ErrIPFIXTemplateMissing
	case entry.uncertain:
		return IPFIXTemplateSnapshot{}, ErrIPFIXTemplateUncertain
	case receivedAt.Sub(entry.RefreshedAt) >= c.limits.Timeout:
		return IPFIXTemplateSnapshot{}, ErrIPFIXTemplateExpired
	}
	snapshot := entry.IPFIXTemplateSnapshot
	snapshot.Template.Fields = slices.Clone(snapshot.Template.Fields)
	return snapshot, nil
}

// beforeBoundary is called with the mutex held.
func (c *IPFIXTemplateCache) beforeBoundary(receivedAt time.Time, exportTime uint32) bool {
	return !c.lastReceive.IsZero() && (receivedAt.Before(c.lastReceive) || exportTime < c.lastExport)
}
