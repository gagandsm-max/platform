//go:build linux

package flow

import (
	"bytes"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// The wildcard receiver does not identify the packet's destination. Derive the
// expected loopback interface independently, without a privileged netlink dump.
func loopbackOriginIndex(t *testing.T) int {
	t.Helper()
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fd)
	request, err := unix.NewIfreq("lo")
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.IoctlIfreq(fd, unix.SIOCGIFINDEX, request); err != nil {
		t.Fatalf("get loopback ifindex: %v", err)
	}
	return int(request.Uint32())
}

type originSockets struct {
	receiver *net.UDPConn
	sender   *net.UDPConn
	port     uint16
	index    int
}

func openOriginSockets(t *testing.T, network string) originSockets {
	t.Helper()
	listen := func(ip net.IP) *net.UDPConn {
		t.Helper()
		conn, err := net.ListenUDP(network, &net.UDPAddr{IP: ip})
		if err != nil {
			if network == "udp6" && (errors.Is(err, unix.EAFNOSUPPORT) || errors.Is(err, unix.EPROTONOSUPPORT) || errors.Is(err, unix.EADDRNOTAVAIL)) {
				t.Skipf("IPv6 loopback unavailable: %v", err)
			}
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		return conn
	}
	wildcard, loopback := net.IPv4zero, net.IPv4(127, 0, 0, 1)
	if network == "udp6" {
		wildcard, loopback = net.IPv6zero, net.IPv6loopback
	}
	r, s := listen(wildcard), listen(loopback)
	bound := r.LocalAddr().(*net.UDPAddr).AddrPort()
	if !bound.Addr().IsUnspecified() || bound.Port() == 0 {
		t.Fatalf("expected wildcard with assigned port, got %v", bound)
	}
	return originSockets{receiver: r, sender: s, port: bound.Port(), index: loopbackOriginIndex(t)}
}

func (s originSockets) reader(t *testing.T) *UDPOriginReader {
	t.Helper()
	r, err := NewUDPOriginReader(s.receiver, s.index)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (s originSockets) send(t *testing.T, dst netip.Addr, payload []byte) UDPOrigin {
	t.Helper()
	collector := netip.AddrPortFrom(dst, s.port)
	if err := s.receiver.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	n, err := s.sender.WriteToUDPAddrPort(payload, collector)
	if err != nil || n != len(payload) {
		t.Fatalf("send to %v: n=%d err=%v", collector, n, err)
	}
	peer := s.sender.LocalAddr().(*net.UDPAddr).AddrPort()
	return UDPOrigin{Source: netip.AddrPortFrom(peer.Addr().Unmap(), peer.Port()), Collector: collector, IfIndex: s.index}
}

func assertOriginRead(t *testing.T, reader *UDPOriginReader, buf, packet []byte, want UDPOrigin) {
	t.Helper()
	n, origin, err := reader.ReadDatagram(buf)
	if err != nil || n != len(packet) || origin != want {
		t.Fatalf("read n=%d origin=%+v err=%v; want n=%d origin=%+v", n, origin, err, len(packet), want)
	}
	if !bytes.Equal(buf[:n], packet) {
		t.Fatal("received payload differs from sender bytes")
	}
}

func assertOriginRejected(t *testing.T, reader *UDPOriginReader, buf []byte, received int, reason error) {
	t.Helper()
	n, origin, err := reader.ReadDatagram(buf)
	if n != 0 || origin != (UDPOrigin{}) || !errors.Is(err, reason) {
		t.Fatalf("read rejected packet: n=%d origin=%+v err=%v; want no bytes or origin and %v", n, origin, err, reason)
	}
	if !bytes.Equal(buf[:received], make([]byte, received)) {
		t.Fatalf("rejected packet bytes remain in receive buffer: %x", buf[:received])
	}
}

func TestUDPOriginLinuxWildcardDestinationAndPeer(t *testing.T) {
	packet := []byte{0, 9, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 42, 1, 0, 0, 8, 1, 2, 3, 4}
	for _, tc := range []struct {
		network string
		addrs   []string
	}{
		{"udp4", []string{"127.0.0.1", "127.0.0.2"}},
		{"udp6", []string{"::1"}},
	} {
		t.Run(tc.network, func(t *testing.T) {
			s := openOriginSockets(t, tc.network)
			r := s.reader(t)
			buffer := make([]byte, len(packet)) // An exactly full datagram is valid.
			for _, addr := range tc.addrs {
				want := s.send(t, netip.MustParseAddr(addr), packet)
				assertOriginRead(t, r, buffer, packet, want)
				envelope, err := ParseV9Envelope(buffer)
				if err != nil || envelope.SourceID != 42 || len(envelope.FlowSets) != 1 {
					t.Fatalf("unchanged v9 parser: envelope=%+v err=%v", envelope, err)
				}
			}
		})
	}
}

func TestUDPOriginLinuxTruncationAndRecovery(t *testing.T) {
	s := openOriginSockets(t, "udp4")
	r := s.reader(t)
	dst := netip.MustParseAddr("127.0.0.1")
	buffer := bytes.Repeat([]byte{0x66}, 16)
	s.send(t, dst, bytes.Repeat([]byte{0xa7}, len(buffer)+1))
	assertOriginRejected(t, r, buffer, len(buffer), ErrUDPOriginTruncated)
	packet := []byte{0x35, 0x6b}
	want := s.send(t, dst, packet)
	assertOriginRead(t, r, buffer, packet, want)
	want = s.send(t, dst, nil)
	assertOriginRead(t, r, buffer, nil, want)
}

func TestUDPOriginLinuxRejectsOtherInterface(t *testing.T) {
	s := openOriginSockets(t, "udp4")
	const wrongIndex = 1<<31 - 1
	if s.index == wrongIndex {
		t.Fatalf("unexpected loopback index %d", s.index)
	}
	r, err := NewUDPOriginReader(s.receiver, wrongIndex)
	if err != nil {
		if !errors.Is(err, ErrUDPOriginConfig) && !errors.Is(err, ErrUDPOriginInterface) {
			t.Fatalf("wrong interface returned unexpected configuration error: %v", err)
		}
		return
	}
	packet := []byte{0xde, 0xad, 0xbe, 0xef}
	s.send(t, netip.MustParseAddr("127.0.0.1"), packet)
	assertOriginRejected(t, r, make([]byte, len(packet)), len(packet), ErrUDPOriginInterface)
}

func TestUDPOriginLinuxMissingKernelEvidenceAndRecovery(t *testing.T) {
	s := openOriginSockets(t, "udp4")
	r := s.reader(t)
	raw, err := s.receiver.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	setPacketInfo := func(value int) {
		t.Helper()
		var sockErr error
		if err := raw.Control(func(fd uintptr) { sockErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_PKTINFO, value) }); err != nil {
			t.Fatal(err)
		}
		if sockErr != nil {
			t.Fatal(sockErr)
		}
	}
	packet := []byte{0xb0, 0x0b, 0x33}
	dst := netip.MustParseAddr("127.0.0.2")
	buffer := make([]byte, len(packet))
	setPacketInfo(0)
	s.send(t, dst, packet)
	assertOriginRejected(t, r, buffer, len(packet), ErrUDPOriginMetadata)
	setPacketInfo(1)
	want := s.send(t, dst, packet)
	assertOriginRead(t, r, buffer, packet, want)
}

func TestUDPOriginLinuxRejectsExtraAncillaryData(t *testing.T) {
	s := openOriginSockets(t, "udp4")
	r := s.reader(t)
	raw, err := s.receiver.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	// Request unrelated ancillary messages. The kernel may return the complete
	// set or truncate it to fit the reader's control buffer. Neither can be used.
	options := [][2]int{
		{unix.SOL_SOCKET, unix.SO_TIMESTAMPNS},
		{unix.IPPROTO_IP, unix.IP_RECVTTL},
		{unix.IPPROTO_IP, unix.IP_RECVTOS},
		{unix.IPPROTO_IP, unix.IP_RECVORIGDSTADDR},
	}
	var sockErr error
	if err := raw.Control(func(fd uintptr) {
		for _, opt := range options {
			if sockErr = unix.SetsockoptInt(int(fd), opt[0], opt[1], 1); sockErr != nil {
				return
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if sockErr != nil {
		t.Fatal(sockErr)
	}
	packet := []byte{1, 2, 3}
	s.send(t, netip.MustParseAddr("127.0.0.1"), packet)
	buffer := make([]byte, len(packet))
	n, origin, err := r.ReadDatagram(buffer)
	if n != 0 || origin != (UDPOrigin{}) || (!errors.Is(err, ErrUDPOriginTruncated) && !errors.Is(err, ErrUDPOriginMetadata)) || !bytes.Equal(buffer, []byte{0, 0, 0}) {
		t.Fatalf("unusable ancillary data: n=%d origin=%+v err=%v buffer=%x", n, origin, err, buffer)
	}
}

func TestUDPOriginLinuxControlValidation(t *testing.T) {
	const index = 7
	v4 := unix.Inet4Pktinfo{Ifindex: index, Spec_dst: [4]byte{192, 0, 2, 1}, Addr: [4]byte{198, 51, 100, 18}}
	v6 := unix.Inet6Pktinfo{Ifindex: index, Addr: netip.MustParseAddr("2001:db8::2").As16()}
	info4, info6 := unix.PktInfo4(&v4), unix.PktInfo6(&v6)
	addr, gotIndex, err := parseUDPOriginControl(unix.AF_INET, info4)
	if err != nil || addr != netip.AddrFrom4(v4.Addr) || gotIndex != index {
		t.Fatalf("IPv4 packet-header destination: address=%v index=%d err=%v", addr, gotIndex, err)
	}
	addr, gotIndex, err = parseUDPOriginControl(unix.AF_INET6, info6)
	if err != nil || addr != netip.AddrFrom16(v6.Addr) || gotIndex != index {
		t.Fatalf("IPv6 packet-header destination: address=%v index=%d err=%v", addr, gotIndex, err)
	}
	empty4 := unix.Inet4Pktinfo{Ifindex: index}
	noIndex := unix.Inet6Pktinfo{Addr: v6.Addr}
	negativeIndex := unix.Inet4Pktinfo{Ifindex: -1, Addr: v4.Addr}
	for _, tc := range []struct {
		name    string
		family  int
		control []byte
	}{
		{"absent", unix.AF_INET, nil},
		{"short_header", unix.AF_INET, info4[:1]},
		{"short_payload", unix.AF_INET, info4[:unix.CmsgLen(1)]},
		{"wrong_family", unix.AF_INET6, info4},
		{"duplicate", unix.AF_INET, append(append([]byte{}, info4...), info4...)},
		{"unknown_type", unix.AF_INET, unix.UnixRights(0)},
		{"unspecified_destination", unix.AF_INET, unix.PktInfo4(&empty4)},
		{"zero_index", unix.AF_INET6, unix.PktInfo6(&noIndex)},
		{"negative_index", unix.AF_INET, unix.PktInfo4(&negativeIndex)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr, gotIndex, err := parseUDPOriginControl(tc.family, tc.control)
			if addr.IsValid() || gotIndex != 0 || !errors.Is(err, ErrUDPOriginMetadata) {
				t.Fatalf("invalid evidence: addr=%v index=%d err=%v", addr, gotIndex, err)
			}
		})
	}
}
