package flow

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"net/netip"
	"strings"
	"sync"
	"time"
)

var (
	ErrV9StreamConfig      = errors.New("invalid NetFlow v9 stream configuration")
	ErrV9StreamScope       = errors.New("NetFlow v9 stream context mismatch")
	ErrV9LatePacket        = errors.New("late or unordered NetFlow v9 packet")
	ErrV9DuplicatePacket   = errors.New("duplicate NetFlow v9 packet")
	ErrV9SequenceCollision = errors.New("NetFlow v9 packet sequence collision")
	ErrV9EpochRequired     = errors.New("NetFlow v9 continuity requires a new epoch")
	ErrV9StreamCapacity    = errors.New("NetFlow v9 template cache capacity exceeded")
)

// V9StreamKey comes from a trusted ingress binding. The caller must assign a
// new Epoch and create an empty stream after exporter or collector restart or
// any loss of continuity. Packet timestamps/sequence alone cannot detect every
// restart. Neither SourceID nor an address establishes exporter authority.
type V9StreamKey struct {
	ExporterBinding   string
	Source, Collector netip.AddrPort
	SourceID          uint32
	Epoch             uint64
}

type V9StreamLimits struct {
	Timeout                 time.Duration
	MaxTemplates, MaxFields int // Simultaneous entries and total descriptors.
}

type v9CacheEntry struct {
	layout *V9RecordLayout
	at     time.Time
}

// V9Stream serializes offline decode calls on one volatile stream. The caller
// controls how many streams exist. A stream must not be copied after first use.
type V9Stream struct {
	mu                     sync.Mutex
	key                    V9StreamKey
	limits                 V9StreamLimits
	cache                  map[uint16]v9CacheEntry
	fields                 int
	lastReceive            time.Time
	sequence, uptime, unix uint32
	digest                 [sha256.Size]byte
	lastRefused            bool
	ended                  bool
}

func NewV9Stream(key V9StreamKey, limits V9StreamLimits) (*V9Stream, error) {
	if strings.TrimSpace(key.ExporterBinding) == "" || key.Epoch == 0 || !validV9Endpoint(key.Source) || !validV9Endpoint(key.Collector) || limits.Timeout <= 0 || limits.MaxFields <= 0 || limits.MaxTemplates <= 0 {
		return nil, ErrV9StreamConfig
	}
	return &V9Stream{key: key, limits: limits, cache: make(map[uint16]v9CacheEntry)}, nil
}

func validV9Endpoint(ep netip.AddrPort) bool {
	return ep.IsValid() && !ep.Addr().WithZone("").Unmap().IsUnspecified() && ep.Port() != 0
}

// Decode reads sets in wire order. Malformed packets commit nothing. A valid
// budget refusal records sequence progress and revokes the affected IDs without
// installing new definitions. A forward gap clears all prior authority.
// A conflicting sequence clears the cache, and a contradictory exporter clock
// ends the epoch. Exact duplicates and older packets leave the stream untouched.
func (s *V9Stream) Decode(key V9StreamKey, receivedAt time.Time, datagram []byte) (*V9RecordPacket, error) {
	if s == nil || s.limits.Timeout == 0 || receivedAt.IsZero() {
		return nil, ErrV9StreamConfig
	}
	if key != s.key {
		return nil, ErrV9StreamScope
	}
	env, err := ParseV9Envelope(datagram)
	if err != nil {
		return nil, err
	}
	if env.SourceID != key.SourceID {
		return nil, ErrV9StreamScope
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return nil, ErrV9EpochRequired
	}
	if receivedAt.Before(s.lastReceive) && !s.lastReceive.IsZero() {
		return nil, ErrV9LatePacket
	}
	digest := sha256.Sum256(datagram)
	var gap uint32
	var discontinuity error
	if !s.lastReceive.IsZero() {
		delta := env.SequenceNumber - s.sequence
		switch {
		case delta == 0:
			if s.digest == digest {
				if s.lastRefused {
					return nil, ErrV9StreamCapacity
				}
				return nil, ErrV9DuplicatePacket
			}
			discontinuity = ErrV9SequenceCollision
			if env.UnixSecs > s.unix && env.SysUpTime < s.uptime && !s.uptimeWrapped(env) {
				discontinuity = ErrV9EpochRequired
			}
		case delta >= 1<<31:
			if env.UnixSecs <= s.unix {
				return nil, ErrV9LatePacket
			}
			discontinuity = ErrV9EpochRequired
		case env.UnixSecs < s.unix || (env.SysUpTime < s.uptime && !s.uptimeWrapped(env)):
			discontinuity = ErrV9EpochRequired
		default:
			gap = delta - 1
		}
	}
	tx := v9CacheTxn{base: s.cache, entries: len(s.cache), fields: s.fields, limits: s.limits, now: receivedAt}
	if gap != 0 || discontinuity != nil {
		tx.base = nil
		tx.entries = 0
		tx.fields = 0
	}
	// A disputed packet can validate only its own definitions. Admission
	// budgets for persistent state must not mask a verified collision.
	tx.checkOnly = discontinuity != nil
	packet := &V9RecordPacket{
		Key: key, ReceivedAt: receivedAt, Timeout: s.limits.Timeout,
		SequenceNumber: env.SequenceNumber, MissingPackets: gap,
		UnixSecs: env.UnixSecs, SysUpTime: env.SysUpTime,
	}
	for _, set := range env.FlowSets {
		start := int(set.Offset)
		end := start + int(set.Length)
		body := datagram[start+v9SetHeaderLength : end]
		switch {
		case set.ID <= 1:
			if err := tx.definitions(V9TemplateKind(set.ID), body, end); err != nil {
				return nil, err
			}
		case set.ID >= 256:
			entry, _ := tx.get(set.ID)
			data, err := v9DataSet(set.ID, body, end, entry, receivedAt, s.limits.Timeout, !tx.checkOnly)
			if err != nil {
				return nil, err
			}
			if !tx.checkOnly {
				packet.Sets = append(packet.Sets, data)
			}
		}
	}
	if discontinuity != nil {
		clear(s.cache)
		s.fields = 0
		s.lastReceive = receivedAt
		s.ended = discontinuity == ErrV9EpochRequired
		return nil, discontinuity
	}
	s.lastReceive, s.sequence, s.uptime, s.unix, s.digest = receivedAt, env.SequenceNumber, env.SysUpTime, env.UnixSecs, digest
	s.lastRefused = tx.checkOnly
	if tx.base == nil {
		clear(s.cache)
		s.fields = 0
	}
	if tx.checkOnly {
		// The packet was well-formed, but its definitions could not all be
		// admitted. Prior meanings of every ID it defines are now unsafe.
		for id := range tx.pending {
			if prior, ok := s.cache[id]; ok {
				s.fields -= len(prior.layout.fields)
				delete(s.cache, id)
			}
		}
		return nil, ErrV9StreamCapacity
	}
	if tx.prune {
		for id, entry := range s.cache {
			if receivedAt.Sub(entry.at) >= s.limits.Timeout {
				delete(s.cache, id)
			}
		}
	}
	for id, entry := range tx.pending {
		s.cache[id] = entry
	}
	s.fields = tx.fields
	return packet, nil
}

// Whole-second export times permit up to 999 ms of skew in elapsed time.
func (s *V9Stream) uptimeWrapped(env *V9Envelope) bool {
	delta := env.SysUpTime - s.uptime
	elapsed := int64(env.UnixSecs-s.unix) * 1000
	difference := int64(delta) - elapsed
	return s.uptime >= 1<<31 && env.SysUpTime < 1<<31 && delta < 1<<31 && difference >= -999 && difference <= 999
}

// Stage only changed IDs. Unchanged templates are never copied per packet.
type v9CacheTxn struct {
	base, pending   map[uint16]v9CacheEntry
	entries, fields int
	now             time.Time
	limits          V9StreamLimits
	prune           bool
	checkOnly       bool
}

func (tx *v9CacheTxn) get(id uint16) (v9CacheEntry, bool) {
	if e, ok := tx.pending[id]; ok {
		return e, true
	}
	e, ok := tx.base[id]
	if ok && tx.prune && tx.now.Sub(e.at) >= tx.limits.Timeout {
		return v9CacheEntry{}, false
	}
	return e, ok
}
func (tx *v9CacheTxn) fits(id uint16, n int) bool {
	old, ok := tx.get(id)
	fields := tx.fields
	if ok {
		fields -= len(old.layout.fields)
	} else if tx.entries >= tx.limits.MaxTemplates {
		return false
	}
	return n <= tx.limits.MaxFields-fields
}
func (tx *v9CacheTxn) definitions(kind V9TemplateKind, body []byte, end int) error {
	for {
		id, scopes, specs, tail, err := nextV9Template(kind, body, end)
		if err != nil {
			return err
		}
		n := len(specs) / 4
		if !tx.checkOnly && !tx.fits(id, n) {
			if !tx.prune {
				for k, e := range tx.base {
					if _, replaced := tx.pending[k]; !replaced && tx.now.Sub(e.at) >= tx.limits.Timeout {
						tx.fields -= len(e.layout.fields)
						tx.entries--
					}
				}
				tx.prune = true
			}
			if !tx.fits(id, n) {
				tx.checkOnly = true
			}
		}
		layout := &V9RecordLayout{kind: kind, scopes: scopes}
		for i := 2; i < len(specs); i += 4 {
			layout.width += int(binary.BigEndian.Uint16(specs[i:]))
		}
		// A zero aggregate width has no row boundaries. Individual zero-width
		// descriptors remain evidence when another field gives forward progress.
		if layout.width == 0 {
			return ErrMalformedV9Template
		}
		if !tx.checkOnly {
			layout.fields = make([]V9TemplateField, n)
			copyV9Fields(layout.fields, specs)
			old, ok := tx.get(id)
			if ok {
				tx.fields -= len(old.layout.fields)
			} else {
				tx.entries++
			}
			tx.fields += n
		}
		if tx.pending == nil {
			tx.pending = make(map[uint16]v9CacheEntry)
		}
		tx.pending[id] = v9CacheEntry{layout: layout, at: tx.now}
		if len(tail) == 0 {
			return nil
		}
		body = tail
	}
}
