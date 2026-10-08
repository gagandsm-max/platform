package ebpf

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"platform/agent/common/collector"
)

var (
	errDecoderABI        = errors.New("DECODER_REJECT_ABI")
	errDecoderSize       = errors.New("DECODER_REJECT_SIZE")
	errDecoderType       = errors.New("DECODER_REJECT_TYPE")
	errDecoderFamily     = errors.New("DECODER_REJECT_FAMILY")
	errDecoderAddress    = errors.New("DECODER_REJECT_ADDRESS")
	errDecoderComm       = errors.New("DECODER_REJECT_COMM")
	errDecoderProtocol   = errors.New("DECODER_REJECT_PROTOCOL")
	errDecoderIdentity   = errors.New("DECODER_REJECT_IDENTITY")
)

// decodedEvent is a validated userspace event. Kernel pointers are never kept.
type decodedEvent struct {
	ObservedAt time.Time
	Class      collector.ProcessNetworkEventClass
	Protocol   collector.ProcessNetworkProtocol
	Family     uint16
	LocalIP    netip.Addr
	RemoteIP   netip.Addr
	LocalPort  uint16
	RemotePort uint16
	PID        uint32
	TGID       uint32
	Comm       string
	OldState   int32
	NewState   int32
	Optional   bool
}

func decodeRawRecord(raw []byte, boot time.Time, monoBaseNs uint64) (decodedEvent, error) {
	if len(raw) < ProcessNetRecordV1Size {
		return decodedEvent{}, errDecoderSize
	}
	// Fixed ABI v1 is little-endian on governed bpfel object.
	order := binary.LittleEndian
	abi := order.Uint16(raw[0:2])
	size := order.Uint16(raw[2:4])
	if abi != ProcessNetABIV1 {
		return decodedEvent{}, errDecoderABI
	}
	if size != ProcessNetRecordV1Size || int(size) > len(raw) {
		return decodedEvent{}, errDecoderSize
	}
	ev := rawEventV1{
		ABIVersion: abi,
		RecordSize: size,
		EventType:  raw[4],
		Protocol:   raw[5],
		Family:     order.Uint16(raw[6:8]),
		TsNs:       order.Uint64(raw[8:16]),
		TGID:       order.Uint32(raw[16:20]),
		PID:        order.Uint32(raw[20:24]),
		Sport:      order.Uint16(raw[24:26]),
		Dport:      order.Uint16(raw[26:28]),
		OldState:   int32(order.Uint32(raw[28:32])),
		NewState:   int32(order.Uint32(raw[32:36])),
		Flags:      order.Uint32(raw[84:88]),
	}
	copy(ev.Saddr[:], raw[36:52])
	copy(ev.Daddr[:], raw[52:68])
	copy(ev.Comm[:], raw[68:84])

	out := decodedEvent{
		PID:        ev.PID,
		TGID:       ev.TGID,
		Family:     ev.Family,
		LocalPort:  ev.Sport,
		RemotePort: ev.Dport,
		OldState:   ev.OldState,
		NewState:   ev.NewState,
		Optional:   ev.Flags&FlagOptionalProbe != 0,
	}

	switch ev.EventType {
	case EventTypeInetState:
		out.Class = classifyInetState(ev.OldState, ev.NewState)
	case EventTypeTCPRexmit:
		out.Class = collector.ProcessNetworkEventRetransmit
	default:
		return decodedEvent{}, errDecoderType
	}

	proto, err := mapProtocol(ev.Protocol, ev.EventType)
	if err != nil {
		return decodedEvent{}, err
	}
	out.Protocol = proto

	local, remote, err := decodeAddresses(ev.Family, ev.Saddr, ev.Daddr)
	if err != nil {
		return decodedEvent{}, err
	}
	out.LocalIP = local
	out.RemoteIP = remote

	comm, err := sanitizeComm(ev.Comm[:])
	if err != nil {
		return decodedEvent{}, err
	}
	out.Comm = comm
	if out.Comm == "" && out.PID == 0 && out.TGID == 0 {
		return decodedEvent{}, errDecoderIdentity
	}

	out.ObservedAt = observeAt(boot, monoBaseNs, ev.TsNs)
	return out, nil
}

func classifyInetState(oldState, newState int32) collector.ProcessNetworkEventClass {
	// TCP state numbers match include/net/tcp_states.h.
	const (
		tcpEstablished = 1
		tcpSynSent     = 2
		tcpSynRecv     = 3
		tcpFinWait1    = 4
		tcpFinWait2    = 5
		tcpTimeWait    = 6
		tcpClose       = 7
		tcpCloseWait   = 8
		tcpLastAck     = 9
		tcpListen      = 10
		tcpClosing     = 11
	)
	switch {
	case newState == tcpEstablished && (oldState == tcpSynSent || oldState == tcpSynRecv):
		return collector.ProcessNetworkEventOpen
	case newState == tcpClose || newState == tcpTimeWait || newState == tcpCloseWait ||
		newState == tcpFinWait1 || newState == tcpFinWait2 || newState == tcpLastAck || newState == tcpClosing:
		return collector.ProcessNetworkEventClose
	case newState == tcpClose && (oldState == tcpSynSent || oldState == tcpSynRecv):
		return collector.ProcessNetworkEventFail
	default:
		// Non-terminal transitions remain observational OPEN for aggregation
		// identity establishment without fabricating bytes/RTT.
		if newState == tcpSynSent || newState == tcpSynRecv || newState == tcpListen {
			return collector.ProcessNetworkEventOpen
		}
		return collector.ProcessNetworkEventOpen
	}
}

func mapProtocol(p uint8, eventType uint8) (collector.ProcessNetworkProtocol, error) {
	switch p {
	case 6:
		return collector.ProcessNetworkProtocolTCP, nil
	case 17:
		if eventType == EventTypeTCPRexmit {
			return "", errDecoderProtocol
		}
		return collector.ProcessNetworkProtocolUDP, nil
	default:
		return "", errDecoderProtocol
	}
}

func decodeAddresses(family uint16, saddr, daddr [16]byte) (netip.Addr, netip.Addr, error) {
	switch family {
	case 2: // AF_INET
		var local4, remote4 [4]byte
		copy(local4[:], saddr[:4])
		copy(remote4[:], daddr[:4])
		local := netip.AddrFrom4(local4)
		remote := netip.AddrFrom4(remote4)
		if !remote.IsValid() || remote.IsUnspecified() {
			return netip.Addr{}, netip.Addr{}, errDecoderAddress
		}
		return local, remote, nil
	case 10: // AF_INET6
		local := netip.AddrFrom16(saddr)
		remote := netip.AddrFrom16(daddr)
		if !remote.IsValid() || remote.IsUnspecified() {
			return netip.Addr{}, netip.Addr{}, errDecoderAddress
		}
		// Prefer canonical unmapped form when IPv4-mapped.
		if local.Is4In6() {
			local = local.Unmap()
		}
		if remote.Is4In6() {
			remote = remote.Unmap()
		}
		return local, remote, nil
	default:
		return netip.Addr{}, netip.Addr{}, errDecoderFamily
	}
}

func sanitizeComm(raw []byte) (string, error) {
	n := 0
	for n < len(raw) && raw[n] != 0 {
		n++
	}
	b := raw[:n]
	if len(b) == 0 {
		return "", nil
	}
	if !utf8.Valid(b) {
		return "", errDecoderComm
	}
	s := string(b)
	for _, r := range s {
		if r == 0 || r == '\n' || r == '\r' || (!unicode.IsPrint(r) && !unicode.IsSpace(r)) {
			return "", errDecoderComm
		}
	}
	s = strings.TrimSpace(s)
	if len(s) > ProcessNetCommLen {
		return "", errDecoderComm
	}
	return s, nil
}

func observeAt(boot time.Time, monoBaseNs, eventNs uint64) time.Time {
	if boot.IsZero() || eventNs == 0 {
		return time.Now().UTC()
	}
	// Convert monotonic ktime to wall clock relative to process boot sample.
	if eventNs >= monoBaseNs {
		return boot.Add(time.Duration(eventNs-monoBaseNs) * time.Nanosecond).UTC()
	}
	return boot.UTC()
}

func toSynthetic(ev decodedEvent) (collector.ProcessNetworkSyntheticEvent, error) {
	if ev.Comm == "" {
		// Workload identity not defensible without a process name when PID is
		// ephemeral-only — drop and account at caller.
		return collector.ProcessNetworkSyntheticEvent{}, errDecoderIdentity
	}
	pid := ev.PID
	localPort := ev.LocalPort
	out := collector.ProcessNetworkSyntheticEvent{
		ObservedAt:         ev.ObservedAt,
		Class:              ev.Class,
		ExecutableIdentity: ev.Comm,
		ProcessName:        ev.Comm,
		PID:                &pid,
		Protocol:           ev.Protocol,
		RemoteIP:           ev.RemoteIP.String(),
		RemotePort:         ev.RemotePort,
		LocalIP:            ev.LocalIP.String(),
		LocalPort:          &localPort,
	}
	if ev.Class == collector.ProcessNetworkEventRetransmit {
		one := uint32(1)
		out.RetransmitCount = &one
	}
	return out, nil
}

func backendDropEvent(reason string, n uint32, at time.Time) collector.ProcessNetworkSyntheticEvent {
	_ = reason
	count := n
	if count == 0 {
		count = 1
	}
	return collector.ProcessNetworkSyntheticEvent{
		ObservedAt:   at,
		Class:        collector.ProcessNetworkEventBackendDrop,
		DroppedCount: &count,
	}
}

func formatDecoderErr(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
