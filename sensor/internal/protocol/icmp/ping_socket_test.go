package icmp

import (
	"context"
	xicmp "golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
	"net"
	"runtime"
	"syscall"
	"testing"
	"time"
)

func TestEffectiveEchoIDFailsClosed(t *testing.T) {
	for _, addr := range []net.Addr{nil, &net.IPAddr{}, &net.UDPAddr{}, &net.UDPAddr{Port: -1}, &net.UDPAddr{Port: 65536}} {
		if _, err := effectiveEchoID(addr, "linux"); CodeOf(err) != CodeProtocol {
			t.Fatalf("addr=%v err=%v", addr, err)
		}
	}
	if id, err := effectiveEchoID(&net.UDPAddr{Port: 14362}, "linux"); err != nil || id != 14362 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	if _, err := effectiveEchoID(nil, "unsupported"); CodeOf(err) != CodeProtocol {
		t.Fatal(err)
	}
}

func TestPingSocketLoopback(t *testing.T) {
	for _, tc := range []struct {
		version int
		target  string
	}{{4, "127.0.0.1"}, {6, "::1"}} {
		t.Run(tc.target, func(t *testing.T) {
			c, _ := NewCollector(testLimits())
			s, err := c.newSession(Request{IPVersion: tc.version})
			if err != nil {
				if tc.version == 6 && (err == syscall.EAFNOSUPPORT || err == syscall.EPROTONOSUPPORT) {
					t.Skipf("IPv6 socket unsupported: %v", err)
				}
				t.Fatal(err)
			}
			real := s.(*realSession)
			if runtime.GOOS == "linux" && real.id != real.conn.LocalAddr().(*net.UDPAddr).Port {
				t.Fatal("effective socket ID not used")
			}
			t.Logf("socket=%v effective_id=%d", real.conn.LocalAddr(), real.id)
			s.Close()
			out, err := c.Collect(context.Background(), Request{Target: tc.target, IPVersion: tc.version, Samples: 3, Timeout: 100 * time.Millisecond, PayloadBytes: 16, ExecutionTimeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("live loopback result=%+v", out)
			if out.Sent != 3 || out.Received != 3 || out.PacketLossPct != 0 || len(out.Samples) != 3 {
				t.Fatalf("%+v", out)
			}
			for _, sample := range out.Samples {
				if !sample.Success || sample.RTTMicros <= 0 {
					t.Fatalf("invalid RTT: %+v", sample)
				}
			}
			if out.MinRTTMicros <= 0 || out.AvgRTTMicros < out.MinRTTMicros || out.MaxRTTMicros < out.AvgRTTMicros {
				t.Fatal("invalid RTT aggregation")
			}
		})
	}
}

func TestStrictReplyValidation(t *testing.T) {
	for _, version := range []int{4, 6} {
		proto := 1
		var typ xicmp.Type = ipv4.ICMPTypeEchoReply
		if version == 6 {
			proto = 58
			typ = ipv6.ICMPTypeEchoReply
		}
		target := net.ParseIP("127.0.0.1")
		if version == 6 {
			target = net.ParseIP("::1")
		}
		payload := []byte{0x50, 0x50}
		for _, tc := range []string{"valid", "sequence", "identifier", "peer", "type", "code", "payload", "malformed"} {
			t.Run(tc+string(rune('0'+version)), func(t *testing.T) {
				echo := &xicmp.Echo{ID: 123, Seq: 3, Data: append([]byte(nil), payload...)}
				msg := xicmp.Message{Type: typ, Body: echo}
				var peer net.Addr = &net.UDPAddr{IP: target}
				switch tc {
				case "sequence":
					echo.Seq++
				case "identifier":
					echo.ID++
				case "peer":
					peer = &net.UDPAddr{IP: net.ParseIP("192.0.2.1")}
				case "type":
					if version == 4 {
						msg.Type = ipv4.ICMPTypeEcho
					} else {
						msg.Type = ipv6.ICMPTypeEchoRequest
					}
				case "code":
					msg.Code = 1
				case "payload":
					echo.Data[0]++
				}
				wire, err := msg.Marshal(nil)
				if err != nil {
					t.Fatal(err)
				}
				if tc == "malformed" {
					wire = wire[:3]
				}
				if got := validEchoReply(wire, proto, version, 123, 3, peer, target, payload); got != (tc == "valid") {
					t.Fatalf("accepted=%v", got)
				}
			})
		}
	}
}

// Socket fixture drives the real Probe deadline/read/cancellation path without
// depending on external routing or privileged raw packet injection.
type silentPacketConn struct {
	closed   chan struct{}
	wrote    chan struct{}
	deadline time.Time
}

func (s *silentPacketConn) LocalAddr() net.Addr                { return &net.UDPAddr{Port: 123} }
func (s *silentPacketConn) SetDeadline(d time.Time) error      { s.deadline = d; return nil }
func (s *silentPacketConn) SetReadDeadline(d time.Time) error  { return s.SetDeadline(d) }
func (s *silentPacketConn) SetWriteDeadline(d time.Time) error { return s.SetDeadline(d) }
func (s *silentPacketConn) WriteTo(b []byte, _ net.Addr) (int, error) {
	close(s.wrote)
	return len(b), nil
}
func (s *silentPacketConn) ReadFrom([]byte) (int, net.Addr, error) {
	timer := time.NewTimer(time.Until(s.deadline))
	defer timer.Stop()
	select {
	case <-s.closed:
		return 0, nil, net.ErrClosed
	case <-timer.C:
		return 0, nil, &net.DNSError{IsTimeout: true}
	}
}
func (s *silentPacketConn) Close() error { close(s.closed); return nil }
func TestProbeTimeoutCancellationAndDeadline(t *testing.T) {
	for _, tc := range []string{"timeout", "cancel", "deadline"} {
		t.Run(tc, func(t *testing.T) {
			conn := &silentPacketConn{closed: make(chan struct{}), wrote: make(chan struct{})}
			s := &realSession{conn: conn, version: 4, id: 123}
			defer s.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			timeout := 20 * time.Millisecond
			want := CodeTimeout
			if tc == "cancel" {
				timeout = time.Second
				want = CodeCancelled
				go func() { <-conn.wrote; cancel() }()
			}
			if tc == "deadline" {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 10*time.Millisecond)
				defer stop()
				want = CodeDeadline
				timeout = time.Second
			}
			_, err := s.Probe(ctx, net.ParseIP("127.0.0.1"), 1, 8, timeout, 4)
			// Allow the context timer to fire when the socket deadline wins the race.
			if tc == "deadline" {
				<-ctx.Done()
			}
			if CodeOf(classify(ctx, err)) != want {
				t.Fatalf("err=%v code=%s want=%s", err, CodeOf(classify(ctx, err)), want)
			}
		})
	}
}
