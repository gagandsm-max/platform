package icmp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	xicmp "golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"platform/sensor/internal/scheduler"
)

const ResultSchemaVersion = 1
const JobKind scheduler.JobKind = "ICMP"

type ErrorCode string

const (
	CodeInvalidRequest ErrorCode = "ICMP_INVALID_REQUEST"
	CodePermission     ErrorCode = "ICMP_PERMISSION_DENIED"
	CodeTimeout        ErrorCode = "ICMP_TIMEOUT"
	CodeUnreachable    ErrorCode = "ICMP_UNREACHABLE"
	CodeTransport      ErrorCode = "ICMP_TRANSPORT"
	CodeProtocol       ErrorCode = "ICMP_PROTOCOL"
	CodeResourceLimit  ErrorCode = "ICMP_RESOURCE_LIMIT"
	CodeCancelled      ErrorCode = "ICMP_CANCELLED"
	CodeDeadline       ErrorCode = "ICMP_DEADLINE_EXCEEDED"
)

type Error struct {
	Code      ErrorCode
	msg       string
	cause     error
	transient bool
}

func (e *Error) Error() string   { return string(e.Code) + ": " + e.msg }
func (e *Error) Unwrap() error   { return e.cause }
func (e *Error) Transient() bool { return e != nil && e.transient }
func newError(c ErrorCode, m string, e error, t bool) error {
	return &Error{Code: c, msg: m, cause: e, transient: t}
}
func CodeOf(err error) ErrorCode {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
func IsTransient(err error) bool { var e *Error; return errors.As(err, &e) && e.transient }

type Limits struct {
	MaxSamples               int
	MaxPayloadBytes          int
	MaxExecutionTimeout      time.Duration
	MaxReplyPacketsPerSample int
	MaxSchedulerPayloadBytes int
}

type Request struct {
	Target           string        `json:"target"`
	IPVersion        int           `json:"ipVersion"`
	Samples          int           `json:"samples"`
	Timeout          time.Duration `json:"timeout"`
	Interval         time.Duration `json:"interval"`
	PayloadBytes     int           `json:"payloadBytes"`
	ExecutionTimeout time.Duration `json:"executionTimeout"`
}

type Sample struct {
	Sequence  int       `json:"sequence"`
	Success   bool      `json:"success"`
	RTTMicros int64     `json:"rttMicros,omitempty"`
	ErrorCode ErrorCode `json:"errorCode,omitempty"`
}

type Result struct {
	SchemaVersion int       `json:"schemaVersion"`
	Target        string    `json:"target"`
	IPVersion     int       `json:"ipVersion"`
	StartedAt     time.Time `json:"startedAt"`
	CompletedAt   time.Time `json:"completedAt"`
	Sent          int       `json:"sent"`
	Received      int       `json:"received"`
	PacketLossPct float64   `json:"packetLossPct"`
	MinRTTMicros  int64     `json:"minRttMicros,omitempty"`
	AvgRTTMicros  int64     `json:"avgRttMicros,omitempty"`
	MaxRTTMicros  int64     `json:"maxRttMicros,omitempty"`
	Samples       []Sample  `json:"samples"`
}

type probeSession interface {
	Probe(context.Context, net.IP, int, int, time.Duration, int) (time.Duration, error)
	Close() error
}
type sessionFactory func(Request) (probeSession, error)

type Collector struct {
	limits  Limits
	factory sessionFactory
	now     func() time.Time
	wait    func(context.Context, time.Duration) error
}

func NewCollector(l Limits) (*Collector, error) {
	if l.MaxSamples <= 0 || l.MaxPayloadBytes <= 0 || l.MaxExecutionTimeout <= 0 || l.MaxReplyPacketsPerSample <= 0 || l.MaxSchedulerPayloadBytes <= 0 {
		return nil, newError(CodeInvalidRequest, "all ICMP limits must be explicitly positive", nil, false)
	}
	c := &Collector{limits: l, now: time.Now, wait: waitContext}
	c.factory = c.newSession
	return c, nil
}

func (c *Collector) Validate(r Request) error { return validate(c.limits, r) }

func validate(l Limits, r Request) error {
	ip := net.ParseIP(strings.TrimSpace(r.Target))
	if ip == nil {
		return newError(CodeInvalidRequest, "target must be an IP literal", nil, false)
	}
	if r.IPVersion != 4 && r.IPVersion != 6 {
		return newError(CodeInvalidRequest, "IP version must be 4 or 6", nil, false)
	}
	if r.IPVersion == 4 && ip.To4() == nil {
		return newError(CodeInvalidRequest, "target does not match IPv4 selection", nil, false)
	}
	if r.IPVersion == 6 && ip.To4() != nil {
		return newError(CodeInvalidRequest, "target does not match IPv6 selection", nil, false)
	}
	if r.Samples <= 0 || r.Samples > l.MaxSamples {
		return newError(CodeResourceLimit, "sample count outside configured bound", nil, false)
	}
	if r.PayloadBytes < 0 || r.PayloadBytes > l.MaxPayloadBytes {
		return newError(CodeResourceLimit, "payload size outside configured bound", nil, false)
	}
	if r.Timeout <= 0 {
		return newError(CodeInvalidRequest, "per-sample timeout must be positive", nil, false)
	}
	if r.Interval < 0 {
		return newError(CodeInvalidRequest, "sample interval cannot be negative", nil, false)
	}
	if r.ExecutionTimeout <= 0 || r.ExecutionTimeout > l.MaxExecutionTimeout {
		return newError(CodeResourceLimit, "execution timeout outside configured bound", nil, false)
	}
	return nil
}

func (c *Collector) Collect(parent context.Context, r Request) (Result, error) {
	if parent == nil {
		return Result{}, newError(CodeInvalidRequest, "context must not be nil", nil, false)
	}
	if err := c.Validate(r); err != nil {
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(parent, r.ExecutionTimeout)
	defer cancel()
	s, err := c.factory(r)
	if err != nil {
		return Result{}, classify(ctx, err)
	}
	defer s.Close()
	ip := net.ParseIP(r.Target)
	started := c.now().UTC()
	out := Result{SchemaVersion: ResultSchemaVersion, Target: r.Target, IPVersion: r.IPVersion, StartedAt: started, Sent: r.Samples, Samples: make([]Sample, 0, r.Samples)}
	var total, minRTT, maxRTT time.Duration
	for i := 1; i <= r.Samples; i++ {
		if err := contextErr(ctx); err != nil {
			return Result{}, err
		}
		rtt, probeErr := s.Probe(ctx, ip, i, r.PayloadBytes, r.Timeout, c.limits.MaxReplyPacketsPerSample)
		sample := Sample{Sequence: i}
		if probeErr == nil {
			sample.Success = true
			sample.RTTMicros = rtt.Microseconds()
			out.Received++
			total += rtt
			if minRTT == 0 || rtt < minRTT {
				minRTT = rtt
			}
			if rtt > maxRTT {
				maxRTT = rtt
			}
		} else {
			classified := classify(ctx, probeErr)
			sample.ErrorCode = CodeOf(classified)
			if sample.ErrorCode != CodeTimeout && sample.ErrorCode != CodeUnreachable {
				return Result{}, classified
			}
		}
		out.Samples = append(out.Samples, sample)
		if i < r.Samples && r.Interval > 0 {
			if err := c.wait(ctx, r.Interval); err != nil {
				return Result{}, contextErr(ctx)
			}
		}
	}
	out.PacketLossPct = float64(out.Sent-out.Received) * 100 / float64(out.Sent)
	if out.Received > 0 {
		out.MinRTTMicros = minRTT.Microseconds()
		out.MaxRTTMicros = maxRTT.Microseconds()
		out.AvgRTTMicros = (total / time.Duration(out.Received)).Microseconds()
	}
	out.CompletedAt = c.now().UTC()
	return out, nil
}

type realSession struct {
	conn    *xicmp.PacketConn
	version int
	id      int
	closed  sync.Once
}

var nextEchoID atomic.Uint32

func (c *Collector) newSession(r Request) (probeSession, error) {
	network, bind := "udp4", "0.0.0.0"
	if r.IPVersion == 6 {
		network, bind = "udp6", "::"
	}
	conn, err := xicmp.ListenPacket(network, bind)
	if err != nil {
		return nil, err
	}
	id := int(nextEchoID.Add(1) & 0xffff)
	if id == 0 {
		id = 1
	}
	return &realSession{conn: conn, version: r.IPVersion, id: id}, nil
}

func (s *realSession) Close() error {
	var err error
	s.closed.Do(func() { err = s.conn.Close() })
	return err
}

func (s *realSession) Probe(ctx context.Context, target net.IP, seq, payloadBytes int, timeout time.Duration, maxReplies int) (time.Duration, error) {
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := s.conn.SetDeadline(deadline); err != nil {
		return 0, err
	}
	var typ xicmp.Type = ipv4.ICMPTypeEcho
	proto := 1
	if s.version == 6 {
		typ = ipv6.ICMPTypeEchoRequest
		proto = 58
	}
	msg := xicmp.Message{Type: typ, Code: 0, Body: &xicmp.Echo{ID: s.id, Seq: seq, Data: bytes.Repeat([]byte{0x50}, payloadBytes)}}
	wire, err := msg.Marshal(nil)
	if err != nil {
		return 0, err
	}
	started := time.Now()
	if _, err = s.conn.WriteTo(wire, &net.UDPAddr{IP: target}); err != nil {
		return 0, err
	}
	watchDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = s.Close()
		case <-watchDone:
		}
	}()
	defer close(watchDone)
	buf := make([]byte, 2048+payloadBytes)
	for i := 0; i < maxReplies; i++ {
		n, peer, err := s.conn.ReadFrom(buf)
		if err != nil {
			return 0, err
		}
		parsed, err := xicmp.ParseMessage(proto, buf[:n])
		if err != nil {
			continue
		}
		echo, ok := parsed.Body.(*xicmp.Echo)
		if !ok || echo.Seq != seq || !samePeer(peer, target) {
			continue
		}
		if s.version == 4 && parsed.Type != ipv4.ICMPTypeEchoReply {
			continue
		}
		if s.version == 6 && parsed.Type != ipv6.ICMPTypeEchoReply {
			continue
		}
		return time.Since(started), nil
	}
	return 0, newError(CodeResourceLimit, "reply scan exceeded configured bound", nil, false)
}

func samePeer(a net.Addr, target net.IP) bool {
	switch v := a.(type) {
	case *net.UDPAddr:
		return v.IP.Equal(target)
	case *net.IPAddr:
		return v.IP.Equal(target)
	}
	return false
}

func classify(ctx context.Context, err error) error {
	if ce := contextErr(ctx); ce != nil {
		return ce
	}
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		return newError(CodePermission, "ICMP socket permission denied; configure supported ping socket capability/policy", err, false)
	}
	if errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) {
		return newError(CodeUnreachable, "ICMP target/network unreachable", err, true)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return newError(CodeTimeout, "ICMP request timed out", err, true)
	}
	return newError(CodeTransport, "ICMP transport failed", err, true)
}

func contextErr(ctx context.Context) error {
	if ctx == nil {
		return newError(CodeInvalidRequest, "context must not be nil", nil, false)
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return newError(CodeDeadline, "ICMP execution deadline exceeded", err, false)
		}
		return newError(CodeCancelled, "ICMP execution cancelled", err, false)
	}
	return nil
}

func waitContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

type Executor struct{ collector *Collector }

func NewExecutor(c *Collector) *Executor { return &Executor{collector: c} }

type jobRequest struct {
	SchemaVersion          int    `json:"schemaVersion"`
	Target                 string `json:"target"`
	IPVersion              int    `json:"ipVersion"`
	Samples                int    `json:"samples"`
	TimeoutMillis          int64  `json:"timeoutMillis"`
	IntervalMillis         int64  `json:"intervalMillis"`
	PayloadBytes           int    `json:"payloadBytes"`
	ExecutionTimeoutMillis int64  `json:"executionTimeoutMillis"`
}

func (e *Executor) Execute(ctx context.Context, j scheduler.Job) (scheduler.ExecutionResult, error) {
	if e == nil || e.collector == nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "ICMP executor is not initialized", nil, false))
	}
	if j.Class != scheduler.ClassNetworkPoll || j.Kind != JobKind {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "ICMP job must use NETWORK_POLL class and ICMP kind", nil, false))
	}
	if len(j.Payload) == 0 || len(j.Payload) > e.collector.limits.MaxSchedulerPayloadBytes {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeResourceLimit, "ICMP scheduler payload exceeds configured bound", nil, false))
	}
	var p jobRequest
	dec := json.NewDecoder(bytes.NewReader(j.Payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "invalid ICMP scheduler payload", nil, false))
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "ICMP scheduler payload contains trailing data", nil, false))
	}
	if p.SchemaVersion != 1 {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeInvalidRequest, "unsupported ICMP scheduler payload schema", nil, false))
	}
	r := Request{Target: p.Target, IPVersion: p.IPVersion, Samples: p.Samples, Timeout: time.Duration(p.TimeoutMillis) * time.Millisecond, Interval: time.Duration(p.IntervalMillis) * time.Millisecond, PayloadBytes: p.PayloadBytes, ExecutionTimeout: time.Duration(p.ExecutionTimeoutMillis) * time.Millisecond}
	out, err := e.collector.Collect(ctx, r)
	if err != nil {
		if IsTransient(err) {
			return scheduler.ExecutionResult{}, scheduler.Transient(err)
		}
		return scheduler.ExecutionResult{}, scheduler.Permanent(err)
	}
	payload, err := json.Marshal(out)
	if err != nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(newError(CodeProtocol, "failed encoding ICMP result", err, false))
	}
	return scheduler.ExecutionResult{Payload: payload}, nil
}
