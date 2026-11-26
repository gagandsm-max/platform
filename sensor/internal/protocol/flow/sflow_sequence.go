package flow

import (
	"crypto/sha256"
	"errors"
	"net/netip"
	"strings"
	"sync"
)

// SFlowV5StreamKey identifies one authorized exporter stream. The receiving
// collector must resolve TenantID, ExporterBinding, AgentAddress and SubAgentID
// from current trusted configuration independently of the datagram. Source
// and Collector are the observed UDP endpoints at the named vantage.
type SFlowV5StreamKey struct {
	TenantID        string
	ExporterBinding string
	VantageID       string
	Source          netip.AddrPort
	Collector       netip.AddrPort
	AgentAddress    netip.Addr
	SubAgentID      uint32
}

func (key SFlowV5StreamKey) valid() bool {
	return strings.TrimSpace(key.TenantID) != "" &&
		strings.TrimSpace(key.ExporterBinding) != "" &&
		strings.TrimSpace(key.VantageID) != "" &&
		key.Source.Port() != 0 && concreteSFlowAddress(key.Source.Addr()) &&
		key.Collector.Port() != 0 && concreteSFlowAddress(key.Collector.Addr()) &&
		concreteSFlowAddress(key.AgentAddress) && key.AgentAddress.Zone() == ""
}

func concreteSFlowAddress(addr netip.Addr) bool {
	return addr.IsValid() && !addr.WithZone("").Unmap().IsUnspecified()
}

// ErrSFlowV5StreamScope indicates missing or mismatched trusted stream identity.
var ErrSFlowV5StreamScope = errors.New("sFlow v5: invalid or mismatched exporter stream binding")

type SFlowV5SequenceStatus string

const (
	SFlowV5Baseline      SFlowV5SequenceStatus = "BASELINE"
	SFlowV5Contiguous    SFlowV5SequenceStatus = "CONTIGUOUS"
	SFlowV5SequenceGap   SFlowV5SequenceStatus = "SEQUENCE_GAP"
	SFlowV5Duplicate     SFlowV5SequenceStatus = "DUPLICATE_DATAGRAM"
	SFlowV5Collision     SFlowV5SequenceStatus = "SEQUENCE_COLLISION"
	SFlowV5Old           SFlowV5SequenceStatus = "OUT_OF_ORDER_OR_OLD_DATAGRAM"
	SFlowV5Discontinuity SFlowV5SequenceStatus = "EXPORTER_RESTART_OR_DISCONTINUITY"
	SFlowV5Unknown       SFlowV5SequenceStatus = "SEQUENCE_DISCONTINUITY_UNKNOWN"
)

// SFlowV5SequencePoint holds the immutable identity and header evidence for
// one datagram. Uptime is in milliseconds, including when its value is zero.
type SFlowV5SequencePoint struct {
	SequenceNumber uint32
	Uptime         uint32
	UptimeTrusted  bool
	Digest         [sha256.Size]byte
}

// SFlowV5Observation describes the decision against the previous accepted
// point. Datagram is set only for an admitted packet. MissingDatagrams counts
// missing UDP datagrams within an epoch, never missing records or device health.
type SFlowV5Observation struct {
	Key              SFlowV5StreamKey
	Status           SFlowV5SequenceStatus
	Epoch            uint64
	HasPrevious      bool
	Previous         SFlowV5SequencePoint
	Current          SFlowV5SequencePoint
	MissingDatagrams uint32
	Datagram         *SFlowV5Datagram
}

// SFlowV5Stream tracks one trusted stream in constant memory. Allocate a new
// tracker for each binding and on collector restart. Use the returned pointer
// without copying it. Mutable decoded samples are never retained here.
type SFlowV5Stream struct {
	key   SFlowV5StreamKey
	mu    sync.Mutex
	epoch uint64
	last  SFlowV5SequencePoint
}

func NewSFlowV5Stream(key SFlowV5StreamKey) (*SFlowV5Stream, error) {
	if !key.valid() {
		return nil, ErrSFlowV5StreamScope
	}
	return &SFlowV5Stream{key: key}, nil
}

// Observe parses and fingerprints the same UDP payload, then serializes the
// admission decision. The caller supplies its current trusted binding each
// time and must keep datagram unchanged for the duration of this call.
func (stream *SFlowV5Stream) Observe(key SFlowV5StreamKey, uptimeTrusted bool, datagram []byte) (*SFlowV5Observation, error) {
	if stream == nil || stream.key != key || !key.valid() {
		return nil, ErrSFlowV5StreamScope
	}
	packet, err := ParseSFlowV5(datagram)
	if err != nil {
		return nil, err
	}
	if packet.AgentAddress != key.AgentAddress || packet.SubAgentID != key.SubAgentID {
		return nil, ErrSFlowV5StreamScope
	}
	current := SFlowV5SequencePoint{
		SequenceNumber: packet.SequenceNumber,
		Uptime:         packet.Uptime,
		UptimeTrusted:  uptimeTrusted,
		Digest:         sha256.Sum256(datagram),
	}

	stream.mu.Lock()
	defer stream.mu.Unlock()
	obs := &SFlowV5Observation{
		Key: key, Epoch: stream.epoch, HasPrevious: stream.epoch != 0,
		Previous: stream.last, Current: current,
	}
	distance := current.SequenceNumber - stream.last.SequenceNumber
	switch {
	case !obs.HasPrevious:
		obs.Status = SFlowV5Baseline
		obs.Epoch = 1
	case current.UptimeTrusted && stream.last.UptimeTrusted && current.Uptime < stream.last.Uptime:
		// A trusted uptime decrease takes precedence over serial arithmetic,
		// including apparent wrap, backtrack and equal sequence.
		obs.Status = SFlowV5Discontinuity
		obs.Epoch++
	case distance == 0:
		obs.Status = SFlowV5Collision
		if current.Digest == stream.last.Digest {
			obs.Status = SFlowV5Duplicate
		}
		return obs, nil
	case distance >= 1<<31:
		obs.Status = SFlowV5Old
		// A later exporter timestamp contradicts an older serial value.
		// Equal trusted uptime is consistent with reordering inside one tick.
		if !current.UptimeTrusted || !stream.last.UptimeTrusted || current.Uptime > stream.last.Uptime {
			obs.Status = SFlowV5Unknown
		}
		return obs, nil
	case distance == 1:
		obs.Status = SFlowV5Contiguous
	default:
		obs.Status = SFlowV5SequenceGap
		obs.MissingDatagrams = distance - 1
	}

	stream.epoch = obs.Epoch
	stream.last = current
	obs.Datagram = packet
	return obs, nil
}
