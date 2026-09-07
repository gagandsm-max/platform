package synthetic

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"syscall"
	"testing"
	"time"

	"platform/sensor/internal/scheduler"
)

func TestExecutorActualSchedulerCompatibility(t *testing.T) {
	c := newTestCollector(t)
	e := NewExecutor(c)
	var _ scheduler.Executor = e
}

func TestExecutorWrongClassAndKind(t *testing.T) {
	c := newTestCollector(t)
	e := NewExecutor(c)
	payload := []byte(`{"schemaVersion":1,"target":"127.0.0.1","port":1,"timeoutMillis":100}`)
	_, err := e.Execute(context.Background(), scheduler.Job{Class: scheduler.ClassNetworkPoll, Kind: JobKindTCP, Payload: payload})
	if !scheduler.IsCode(err, scheduler.CodePermanent) {
		t.Fatalf("class err=%v", err)
	}
	_, err = e.Execute(context.Background(), scheduler.Job{Class: scheduler.ClassSynthetic, Kind: "SYNTHETIC_UNKNOWN", Payload: payload})
	if !scheduler.IsCode(err, scheduler.CodePermanent) {
		t.Fatalf("kind err=%v", err)
	}
}

func TestExecutorStrictPayloadUnknownTrailingAndOversized(t *testing.T) {
	c := newTestCollector(t)
	e := NewExecutor(c)
	cases := [][]byte{
		[]byte(`{"schemaVersion":1,"target":"127.0.0.1","port":1,"timeoutMillis":100,"unexpected":true}`),
		[]byte(`{"schemaVersion":1,"target":"127.0.0.1","port":1,"timeoutMillis":100} {}`),
	}
	for _, p := range cases {
		_, err := e.Execute(context.Background(), scheduler.Job{Class: scheduler.ClassSynthetic, Kind: JobKindTCP, Payload: p})
		if !scheduler.IsCode(err, scheduler.CodePermanent) {
			t.Fatalf("payload=%q err=%v", p, err)
		}
	}
	big := []byte(strings.Repeat("x", c.limits.MaxSchedulerPayloadBytes+1))
	_, err := e.Execute(context.Background(), scheduler.Job{Class: scheduler.ClassSynthetic, Kind: JobKindTCP, Payload: big})
	if !scheduler.IsCode(err, scheduler.CodePermanent) {
		t.Fatalf("oversize err=%v", err)
	}
}

func TestExecutorTransientAndPermanentMapping(t *testing.T) {
	t.Run("transient", func(t *testing.T) {
		c := newTestCollector(t)
		c.tcpDial = func(context.Context, string, string) (net.Conn, error) { return nil, syscall.ENETUNREACH }
		e := NewExecutor(c)
		p, _ := json.Marshal(tcpJobRequest{SchemaVersion: 1, Target: "127.0.0.1", Port: 443, TimeoutMillis: 100})
		_, err := e.Execute(context.Background(), scheduler.Job{Class: scheduler.ClassSynthetic, Kind: JobKindTCP, Payload: p})
		if !scheduler.IsCode(err, scheduler.CodeTransient) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("permanent", func(t *testing.T) {
		c := newTestCollector(t)
		e := NewExecutor(c)
		p, _ := json.Marshal(tcpJobRequest{SchemaVersion: 1, Target: "bad host", Port: 443, TimeoutMillis: 100})
		_, err := e.Execute(context.Background(), scheduler.Job{Class: scheduler.ClassSynthetic, Kind: JobKindTCP, Payload: p})
		if !scheduler.IsCode(err, scheduler.CodePermanent) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestExecutorAllJobKinds(t *testing.T) {
	c := newTestCollector(t)
	c.tcpDial = func(context.Context, string, string) (net.Conn, error) { return &fakeConn{}, nil }
	c.dnsLookup = func(context.Context, DNSRequest) ([]net.IP, error) { return []net.IP{net.ParseIP("192.0.2.1")}, nil }
	e := NewExecutor(c)
	tcpPayload, _ := json.Marshal(tcpJobRequest{SchemaVersion: 1, Target: "127.0.0.1", Port: 443, TimeoutMillis: 100})
	if _, err := e.Execute(context.Background(), scheduler.Job{Class: scheduler.ClassSynthetic, Kind: JobKindTCP, Payload: tcpPayload}); err != nil {
		t.Fatalf("tcp err=%v", err)
	}
	dnsPayload, _ := json.Marshal(dnsJobRequest{SchemaVersion: 1, Hostname: "example.test", RecordType: DNSRecordA, TimeoutMillis: 100, MaxAnswers: 2})
	if _, err := e.Execute(context.Background(), scheduler.Job{Class: scheduler.ClassSynthetic, Kind: JobKindDNS, Payload: dnsPayload}); err != nil {
		t.Fatalf("dns err=%v", err)
	}
}

type fakeConn struct{}

func (*fakeConn) Read([]byte) (int, error)         { return 0, nil }
func (*fakeConn) Write(p []byte) (int, error)      { return len(p), nil }
func (*fakeConn) Close() error                     { return nil }
func (*fakeConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (*fakeConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (*fakeConn) SetDeadline(time.Time) error      { return nil }
func (*fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (*fakeConn) SetWriteDeadline(time.Time) error { return nil }
