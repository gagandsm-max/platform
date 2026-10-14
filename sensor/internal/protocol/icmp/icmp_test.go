package icmp

import (
	"context"
	"encoding/json"
	"net"
	"syscall"
	"testing"
	"time"

	xicmp "golang.org/x/net/icmp"

	"platform/sensor/internal/scheduler"
)

type fakeSession struct {
	results []struct {
		rtt time.Duration
		err error
	}
	calls  int
	closed int
}

func (f *fakeSession) Probe(context.Context, net.IP, int, int, time.Duration, int) (time.Duration, error) {
	v := f.results[f.calls]
	f.calls++
	return v.rtt, v.err
}
func (f *fakeSession) Close() error { f.closed++; return nil }

func testLimits() Limits {
	return Limits{MaxSamples: 8, MaxPayloadBytes: 128, MaxExecutionTimeout: time.Second, MaxReplyPacketsPerSample: 4, MaxSchedulerPayloadBytes: 2048}
}
func collectorWith(f *fakeSession) *Collector {
	c, _ := NewCollector(testLimits())
	c.factory = func(Request) (probeSession, error) { return f, nil }
	c.now = time.Now
	c.wait = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestValidationIPv4IPv6AndBounds(t *testing.T) {
	c, _ := NewCollector(testLimits())
	base := Request{Target: "192.0.2.1", IPVersion: 4, Samples: 2, Timeout: 10 * time.Millisecond, PayloadBytes: 8, ExecutionTimeout: 100 * time.Millisecond}
	if err := c.Validate(base); err != nil {
		t.Fatal(err)
	}
	v6 := base
	v6.Target = "2001:db8::1"
	v6.IPVersion = 6
	if err := c.Validate(v6); err != nil {
		t.Fatal(err)
	}
	bad := []Request{
		func() Request { x := base; x.Target = "host"; return x }(),
		func() Request { x := base; x.Samples = 9; return x }(),
		func() Request { x := base; x.PayloadBytes = 129; return x }(),
		func() Request { x := base; x.IPVersion = 6; return x }(),
	}
	for _, r := range bad {
		if err := c.Validate(r); err == nil {
			t.Fatalf("accepted %+v", r)
		}
	}
}

func TestAggregationLossAndRTT(t *testing.T) {
	f := &fakeSession{results: []struct {
		rtt time.Duration
		err error
	}{{10 * time.Millisecond, nil}, {0, &net.DNSError{IsTimeout: true}}, {30 * time.Millisecond, nil}}}
	c := collectorWith(f)
	r := Request{Target: "192.0.2.1", IPVersion: 4, Samples: 3, Timeout: 20 * time.Millisecond, PayloadBytes: 8, ExecutionTimeout: 200 * time.Millisecond}
	out, err := c.Collect(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if out.Received != 2 || out.PacketLossPct < 33 || out.PacketLossPct > 34 || out.MinRTTMicros != 10000 || out.MaxRTTMicros != 30000 || out.AvgRTTMicros != 20000 {
		t.Fatalf("%+v", out)
	}
	if f.closed != 1 {
		t.Fatalf("close=%d", f.closed)
	}
}

func TestPermissionAndUnreachableClassification(t *testing.T) {
	c := collectorWith(&fakeSession{results: []struct {
		rtt time.Duration
		err error
	}{{0, syscall.EPERM}}})
	r := Request{Target: "192.0.2.1", IPVersion: 4, Samples: 1, Timeout: 10 * time.Millisecond, PayloadBytes: 1, ExecutionTimeout: 100 * time.Millisecond}
	_, err := c.Collect(context.Background(), r)
	if CodeOf(err) != CodePermission || IsTransient(err) {
		t.Fatalf("%v", err)
	}
	c = collectorWith(&fakeSession{results: []struct {
		rtt time.Duration
		err error
	}{{0, syscall.EHOSTUNREACH}}})
	out, err := c.Collect(context.Background(), r)
	if err != nil || out.Samples[0].ErrorCode != CodeUnreachable {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestCancellation(t *testing.T) {
	c := collectorWith(&fakeSession{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := Request{Target: "192.0.2.1", IPVersion: 4, Samples: 1, Timeout: 10 * time.Millisecond, PayloadBytes: 1, ExecutionTimeout: 100 * time.Millisecond}
	_, err := c.Collect(ctx, r)
	if CodeOf(err) != CodeCancelled {
		t.Fatalf("%v", err)
	}
}

func TestExecutorRoutingAndPayload(t *testing.T) {
	f := &fakeSession{results: []struct {
		rtt time.Duration
		err error
	}{{time.Millisecond, nil}}}
	c := collectorWith(f)
	e := NewExecutor(c)
	p := jobRequest{SchemaVersion: 1, Target: "192.0.2.1", IPVersion: 4, Samples: 1, TimeoutMillis: 10, PayloadBytes: 1, ExecutionTimeoutMillis: 100}
	b, _ := json.Marshal(p)
	res, err := e.Execute(context.Background(), scheduler.Job{Kind: JobKind, Class: scheduler.ClassNetworkPoll, Payload: b})
	if err != nil || len(res.Payload) == 0 {
		t.Fatalf("res=%s err=%v", res.Payload, err)
	}
	var out Result
	if err = json.Unmarshal(res.Payload, &out); err != nil || out.Received != 1 {
		t.Fatalf("%+v %v", out, err)
	}
	_, err = e.Execute(context.Background(), scheduler.Job{Kind: JobKind, Class: "SYNTHETIC", Payload: b})
	if err == nil {
		t.Fatal("wrong class accepted")
	}
}

func TestFactoryFailureClassification(t *testing.T) {
	c, _ := NewCollector(testLimits())
	c.factory = func(Request) (probeSession, error) { return nil, syscall.EACCES }
	r := Request{Target: "192.0.2.1", IPVersion: 4, Samples: 1, Timeout: 10 * time.Millisecond, PayloadBytes: 1, ExecutionTimeout: 100 * time.Millisecond}
	_, err := c.Collect(context.Background(), r)
	if CodeOf(err) != CodePermission {
		t.Fatalf("%v", err)
	}
}

func TestVerifyLocalCapabilityPermissionAndNoProbe(t *testing.T) {
	c, _ := NewCollector(testLimits())
	var opens, probes, closes int
	c.factory = func(r Request) (probeSession, error) {
		opens++
		if r.IPVersion == 6 {
			return nil, syscall.EAFNOSUPPORT
		}
		return nil, syscall.EPERM
	}
	err := c.VerifyLocalCapability(context.Background())
	if CodeOf(err) != CodePermission {
		t.Fatalf("err=%v", err)
	}
	if opens != 1 {
		t.Fatalf("opens=%d want 1 (IPv4 fail before IPv6)", opens)
	}
	if probes != 0 || closes != 0 {
		t.Fatalf("probes=%d closes=%d", probes, closes)
	}

	session := &fakeSession{}
	opens, probes, closes = 0, 0, 0
	c.factory = func(r Request) (probeSession, error) {
		opens++
		if r.IPVersion == 6 {
			return nil, syscall.EAFNOSUPPORT
		}
		return session, nil
	}
	if err := c.VerifyLocalCapability(context.Background()); err != nil {
		t.Fatal(err)
	}
	if opens != 2 || session.calls != 0 || session.closed != 1 {
		t.Fatalf("opens=%d probes=%d closed=%d", opens, session.calls, session.closed)
	}
}

func TestVerifyLocalCapabilityProtocolUnsupported(t *testing.T) {
	c, _ := NewCollector(testLimits())
	c.factory = func(Request) (probeSession, error) {
		return nil, newError(CodeProtocol, "ping socket Echo identifier semantics unsupported", nil, false)
	}
	if err := c.VerifyLocalCapability(context.Background()); CodeOf(err) != CodeProtocol {
		t.Fatalf("%v", err)
	}
}

func TestVerifyLocalCapabilityRespectsContext(t *testing.T) {
	c, _ := NewCollector(testLimits())
	c.factory = func(Request) (probeSession, error) {
		time.Sleep(50 * time.Millisecond)
		return &fakeSession{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	err := c.VerifyLocalCapability(ctx)
	if CodeOf(err) != CodeDeadline {
		t.Fatalf("%v", err)
	}
}

func TestUnknownSchedulerFieldRejected(t *testing.T) {
	c := collectorWith(&fakeSession{})
	e := NewExecutor(c)
	_, err := e.Execute(context.Background(), scheduler.Job{Kind: JobKind, Class: scheduler.ClassNetworkPoll, Payload: []byte(`{"schemaVersion":1,"target":"192.0.2.1","unknown":1}`)})
	if err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestEchoReplyRequiresMatchingIDAndSeq(t *testing.T) {
	if !echoReplyMatches(&xicmp.Echo{ID: 7, Seq: 3}, 7, 3) {
		t.Fatal("matching id/seq rejected")
	}
	if echoReplyMatches(&xicmp.Echo{ID: 8, Seq: 3}, 7, 3) {
		t.Fatal("foreign echo id accepted")
	}
	if echoReplyMatches(&xicmp.Echo{ID: 7, Seq: 4}, 7, 3) {
		t.Fatal("mismatched seq accepted")
	}
	if echoReplyMatches(nil, 7, 3) {
		t.Fatal("nil echo accepted")
	}
}
