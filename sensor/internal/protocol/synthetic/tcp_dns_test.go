package synthetic

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestTCPValidLocalhostSuccessAndClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan error, 1)
	go func() {
		conn, e := ln.Accept()
		if e != nil {
			accepted <- e
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 1)
		_, e = conn.Read(buf)
		accepted <- e
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	c := newTestCollector(t)
	out, err := c.TCP(context.Background(), TCPRequest{Target: "127.0.0.1", Port: port, Timeout: time.Second})
	if err != nil || !out.Success || out.DurationMicros < 0 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	select {
	case e := <-accepted:
		if !errors.Is(e, io.EOF) {
			t.Fatalf("server did not observe close: %v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not observe socket close")
	}
}

func TestTCPConnectionRefusedClassification(t *testing.T) {
	c := newTestCollector(t)
	c.tcpDial = func(context.Context, string, string) (net.Conn, error) { return nil, syscall.ECONNREFUSED }
	_, err := c.TCP(context.Background(), TCPRequest{Target: "127.0.0.1", Port: 9, Timeout: time.Second})
	if CodeOf(err) != CodeConnectionRefused || IsTransient(err) {
		t.Fatalf("code=%s transient=%v err=%v", CodeOf(err), IsTransient(err), err)
	}
}

func TestTCPTimeoutAndCancellation(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		c := newTestCollector(t)
		c.tcpDial = func(ctx context.Context, _ string, _ string) (net.Conn, error) { <-ctx.Done(); return nil, ctx.Err() }
		_, err := c.TCP(context.Background(), TCPRequest{Target: "example.test", Port: 443, Timeout: 20 * time.Millisecond})
		if CodeOf(err) != CodeTimeout || !IsTransient(err) {
			t.Fatalf("code=%s err=%v", CodeOf(err), err)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		c := newTestCollector(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := c.TCP(ctx, TCPRequest{Target: "example.test", Port: 443, Timeout: time.Second})
		if CodeOf(err) != CodeCancelled {
			t.Fatalf("code=%s err=%v", CodeOf(err), err)
		}
	})
}

func TestDNSValidationAAndAAAAContracts(t *testing.T) {
	c := newTestCollector(t)
	if _, err := c.DNS(context.Background(), DNSRequest{Hostname: "", RecordType: DNSRecordA, Timeout: time.Second, MaxAnswers: 2}); CodeOf(err) != CodeInvalidRequest {
		t.Fatalf("empty host err=%v", err)
	}
	var seen []DNSRecordType
	c.dnsLookup = func(ctx context.Context, r DNSRequest) ([]net.IP, error) {
		seen = append(seen, r.RecordType)
		if r.RecordType == DNSRecordA {
			return []net.IP{net.ParseIP("192.0.2.1")}, nil
		}
		return []net.IP{net.ParseIP("2001:db8::1")}, nil
	}
	a, err := c.DNS(context.Background(), DNSRequest{Hostname: "example.test", RecordType: DNSRecordA, Timeout: time.Second, MaxAnswers: 2})
	if err != nil || len(a.Answers) != 1 || a.Answers[0] != "192.0.2.1" {
		t.Fatalf("a=%+v err=%v", a, err)
	}
	aaaa, err := c.DNS(context.Background(), DNSRequest{Hostname: "example.test", RecordType: DNSRecordAAAA, Timeout: time.Second, MaxAnswers: 2})
	if err != nil || len(aaaa.Answers) != 1 || aaaa.Answers[0] != "2001:db8::1" {
		t.Fatalf("aaaa=%+v err=%v", aaaa, err)
	}
	if len(seen) != 2 || seen[0] != DNSRecordA || seen[1] != DNSRecordAAAA {
		t.Fatalf("record types=%v", seen)
	}
}

func TestDNSNotFoundAndBoundedAnswers(t *testing.T) {
	t.Run("not-found", func(t *testing.T) {
		c := newTestCollector(t)
		c.dnsLookup = func(context.Context, DNSRequest) ([]net.IP, error) {
			return nil, &net.DNSError{Err: "no such host", Name: "missing.test", IsNotFound: true}
		}
		out, err := c.DNS(context.Background(), DNSRequest{Hostname: "missing.test", RecordType: DNSRecordA, Timeout: time.Second, MaxAnswers: 2})
		if CodeOf(err) != CodeDNSFailure || out.Status != DNSStatusNotFound || out.RCode != DNSRCodeUnavailable {
			t.Fatalf("out=%+v err=%v", out, err)
		}
	})
	t.Run("bound", func(t *testing.T) {
		c := newTestCollector(t)
		c.dnsLookup = func(context.Context, DNSRequest) ([]net.IP, error) {
			return []net.IP{net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), net.ParseIP("192.0.2.3")}, nil
		}
		_, err := c.DNS(context.Background(), DNSRequest{Hostname: "example.test", RecordType: DNSRecordA, Timeout: time.Second, MaxAnswers: 2})
		if CodeOf(err) != CodeResourceLimit {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestDNSTimeoutAndCancellation(t *testing.T) {
	c := newTestCollector(t)
	c.dnsLookup = func(ctx context.Context, _ DNSRequest) ([]net.IP, error) { <-ctx.Done(); return nil, ctx.Err() }
	_, err := c.DNS(context.Background(), DNSRequest{Hostname: "example.test", RecordType: DNSRecordA, Timeout: 20 * time.Millisecond, MaxAnswers: 2})
	if CodeOf(err) != CodeTimeout {
		t.Fatalf("timeout err=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.DNS(ctx, DNSRequest{Hostname: "example.test", RecordType: DNSRecordAAAA, Timeout: time.Second, MaxAnswers: 2})
	if CodeOf(err) != CodeCancelled {
		t.Fatalf("cancel err=%v", err)
	}
}

func TestDNSResolverAddressValidation(t *testing.T) {
	c := newTestCollector(t)
	if _, err := c.DNS(context.Background(), DNSRequest{Hostname: "example.test", RecordType: DNSRecordA, ResolverAddress: "127.0.0.1", Timeout: time.Second, MaxAnswers: 1}); CodeOf(err) != CodeInvalidRequest {
		t.Fatalf("err=%v", err)
	}
}

func TestTCPHostnameIPv4IPv6AddressConstruction(t *testing.T) {
	c := newTestCollector(t)
	var got []string
	c.tcpDial = func(ctx context.Context, network, address string) (net.Conn, error) {
		got = append(got, address)
		return &fakeConn{}, nil
	}
	for _, tc := range []TCPRequest{{Target: "localhost", Port: 443, Timeout: time.Second}, {Target: "192.0.2.10", Port: 443, Timeout: time.Second}, {Target: "2001:db8::10", Port: 443, Timeout: time.Second}} {
		if _, err := c.TCP(context.Background(), tc); err != nil {
			t.Fatalf("request=%+v err=%v", tc, err)
		}
	}
	want := []string{"localhost:443", "192.0.2.10:443", "[2001:db8::10]:443"}
	if len(got) != len(want) {
		t.Fatalf("got=%v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("address[%d]=%q want %q", i, got[i], want[i])
		}
	}
}
