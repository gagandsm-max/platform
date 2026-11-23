package flow

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"net/netip"
	"syscall"
)

var (
	ErrUDPOriginConfig    = errors.New("invalid UDP origin reader configuration")
	ErrUDPOriginMetadata  = errors.New("missing or invalid UDP origin metadata")
	ErrUDPOriginInterface = errors.New("unexpected UDP receiving interface")
	ErrUDPOriginTruncated = errors.New("truncated UDP datagram or control metadata")
)

// UDPOrigin is kernel-observed transport evidence for a single datagram.
// Addresses are unzoned; IfIndex identifies the interface in the socket's
// network namespace. Neither the tuple nor IfIndex authenticates the sender.
type UDPOrigin struct {
	Source    netip.AddrPort
	Collector netip.AddrPort
	IfIndex   int
}

// UDPOriginReader receives from a caller-owned Linux UDP socket. The caller
// retains responsibility for deadlines, closure, and the socket's namespace
// and interface configuration throughout the reader's lifetime.
type UDPOriginReader struct {
	conn            *net.UDPConn
	expectedIfIndex int
	family          int
	local           netip.AddrPort
}

// NewUDPOriginReader enables packet information on an already-bound socket.
// Use a socket dedicated to this receive path. Other control messages (for
// example UDP GRO) can change reception semantics and are rejected on read.
func NewUDPOriginReader(conn *net.UDPConn, expectedIfIndex int) (*UDPOriginReader, error) {
	if conn == nil || expectedIfIndex <= 0 || expectedIfIndex > math.MaxInt32 {
		return nil, ErrUDPOriginConfig
	}
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUDPOriginConfig, err)
	}
	reader := &UDPOriginReader{conn: conn, expectedIfIndex: expectedIfIndex}
	var setupErr error
	err = raw.Control(func(fd uintptr) {
		addr, err := syscall.Getsockname(int(fd))
		if err != nil {
			setupErr = err
			return
		}
		var level, option int
		switch a := addr.(type) {
		case *syscall.SockaddrInet4:
			reader.family = syscall.AF_INET
			reader.local = netip.AddrPortFrom(netip.AddrFrom4(a.Addr), uint16(a.Port))
			level, option = syscall.IPPROTO_IP, syscall.IP_PKTINFO
		case *syscall.SockaddrInet6:
			if a.ZoneId != 0 && a.ZoneId != uint32(expectedIfIndex) {
				setupErr = ErrUDPOriginInterface
				return
			}
			reader.family = syscall.AF_INET6
			reader.local = netip.AddrPortFrom(netip.AddrFrom16(a.Addr).Unmap(), uint16(a.Port))
			level, option = syscall.IPPROTO_IPV6, syscall.IPV6_RECVPKTINFO
		default:
			setupErr = errors.New("unsupported socket family")
			return
		}
		if reader.local.Port() == 0 {
			setupErr = errors.New("socket has no bound UDP port")
			return
		}
		setupErr = syscall.SetsockoptInt(int(fd), level, option, 1)
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUDPOriginConfig, err)
	}
	if setupErr != nil {
		return nil, fmt.Errorf("%w: %w", ErrUDPOriginConfig, setupErr)
	}
	return reader, nil
}

// ReadDatagram returns the payload length and origin for one message. A
// zero-length datagram is valid. On error it returns zero, an empty origin,
// and clears any prefix written by this receive. Socket errors retain their
// original identity so the caller can distinguish a deadline or closure.
func (reader *UDPOriginReader) ReadDatagram(payload []byte) (int, UDPOrigin, error) {
	if reader == nil || reader.conn == nil || len(payload) == 0 {
		return 0, UDPOrigin{}, ErrUDPOriginConfig
	}
	var control [128]byte // Fits either packet-info message including native CMSG alignment.
	n, oobn, flags, source, err := reader.conn.ReadMsgUDPAddrPort(payload, control[:])
	var origin UDPOrigin
	if err == nil {
		if flags&(syscall.MSG_TRUNC|syscall.MSG_CTRUNC) != 0 {
			err = ErrUDPOriginTruncated
		} else {
			origin, err = reader.origin(source, control[:oobn])
		}
	}
	if err != nil {
		clear(payload[:min(max(n, 0), len(payload))])
		return 0, UDPOrigin{}, err
	}
	return n, origin, nil
}

func (reader *UDPOriginReader) origin(source netip.AddrPort, control []byte) (UDPOrigin, error) {
	dst, ifIndex, err := parseUDPOriginControl(reader.family, control)
	if err != nil {
		return UDPOrigin{}, err
	}
	if ifIndex != reader.expectedIfIndex {
		return UDPOrigin{}, ErrUDPOriginInterface
	}
	src := source.Addr().WithZone("").Unmap()
	if !src.IsValid() || src.IsUnspecified() || src.IsMulticast() || src == netip.AddrFrom4([4]byte{255, 255, 255, 255}) ||
		source.Port() == 0 || src.Is4() != dst.Is4() || (!reader.local.Addr().IsUnspecified() && reader.local.Addr() != dst) {
		return UDPOrigin{}, ErrUDPOriginMetadata
	}
	return UDPOrigin{
		Source:    netip.AddrPortFrom(src, source.Port()),
		Collector: netip.AddrPortFrom(dst, reader.local.Port()),
		IfIndex:   ifIndex,
	}, nil
}

func parseUDPOriginControl(family int, control []byte) (netip.Addr, int, error) {
	messages, err := syscall.ParseSocketControlMessage(control)
	if err != nil || len(messages) != 1 {
		return netip.Addr{}, 0, ErrUDPOriginMetadata
	}
	msg := messages[0]
	var dst netip.Addr
	var index uint32
	switch {
	case family == syscall.AF_INET && msg.Header.Level == syscall.IPPROTO_IP && msg.Header.Type == syscall.IP_PKTINFO && len(msg.Data) == syscall.SizeofInet4Pktinfo:
		index = binary.NativeEndian.Uint32(msg.Data[:4])
		// in_pktinfo.ipi_addr is the on-wire destination. ipi_spec_dst is
		// the local address selected for replying; it can be different.
		dst = netip.AddrFrom4([4]byte(msg.Data[8:12]))
	case family == syscall.AF_INET6 && msg.Header.Level == syscall.IPPROTO_IPV6 && msg.Header.Type == syscall.IPV6_PKTINFO && len(msg.Data) == syscall.SizeofInet6Pktinfo:
		dst = netip.AddrFrom16([16]byte(msg.Data[:16])).Unmap()
		index = binary.NativeEndian.Uint32(msg.Data[16:20])
	default:
		return netip.Addr{}, 0, ErrUDPOriginMetadata
	}
	// ParseSocketControlMessage ignores a tail shorter than a second header.
	// Accept only the single message's normal unpadded or padded extent.
	size := len(msg.Data)
	if (len(control) != syscall.CmsgLen(size) && len(control) != syscall.CmsgSpace(size)) ||
		index == 0 || index > math.MaxInt32 || dst.IsUnspecified() {
		return netip.Addr{}, 0, ErrUDPOriginMetadata
	}
	return dst, int(index), nil
}
