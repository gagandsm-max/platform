package syslog

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type UDPConfig struct {
	BindAddress      string
	Port             int
	MaxDatagramBytes int
	WorkerCount      int
	QueueDepth       int
	ReadTimeout      time.Duration
	ShutdownGrace    time.Duration
}

func (c UDPConfig) Validate() error {
	if strings.TrimSpace(c.BindAddress) == "" || net.ParseIP(c.BindAddress) == nil {
		return fail(CodeInvalidConfig, "Syslog UDP bind address must be an IP literal", nil)
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fail(CodeInvalidConfig, "Syslog UDP port must be in range 1..65535", nil)
	}
	if c.MaxDatagramBytes <= 0 || c.MaxDatagramBytes > 65535 {
		return fail(CodeInvalidConfig, "Syslog UDP datagram bound must be in range 1..65535 bytes", nil)
	}
	if c.WorkerCount <= 0 {
		return fail(CodeInvalidConfig, "Syslog UDP worker count must be positive", nil)
	}
	if c.QueueDepth <= 0 {
		return fail(CodeInvalidConfig, "Syslog UDP queue depth must be positive", nil)
	}
	if c.ReadTimeout <= 0 {
		return fail(CodeInvalidConfig, "Syslog UDP read timeout must be positive", nil)
	}
	if c.ShutdownGrace <= 0 {
		return fail(CodeInvalidConfig, "Syslog UDP shutdown grace must be positive", nil)
	}
	return nil
}

type EventSink interface {
	Accept(context.Context, Event) error
}

type datagram struct {
	payload    []byte
	receivedAt time.Time
	source     string
}

type UDPReceiver struct {
	cfg    UDPConfig
	parser *Parser
	sink   EventSink

	mu      sync.RWMutex
	state   ReceiverState
	lastErr ErrorCode
	started bool
	conn    *net.UDPConn
	ctx     context.Context
	cancel  context.CancelFunc

	queue          chan datagram
	readerDone     chan struct{}
	workerDone     []chan struct{}
	stopOnce       sync.Once
	queueCloseOnce sync.Once
	workersLeft    atomic.Int64

	metrics receiverMetrics
}

func NewUDPReceiver(cfg UDPConfig, parser *Parser, sink EventSink) (*UDPReceiver, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if parser == nil {
		return nil, fail(CodeInvalidConfig, "Syslog parser is required", nil)
	}
	if sink == nil {
		return nil, fail(CodeInvalidConfig, "Syslog event sink is required", nil)
	}
	r := &UDPReceiver{
		cfg:        cfg,
		parser:     parser,
		sink:       sink,
		state:      ReceiverStopped,
		queue:      make(chan datagram, cfg.QueueDepth),
		readerDone: make(chan struct{}),
		workerDone: make([]chan struct{}, cfg.WorkerCount),
	}
	for i := range r.workerDone {
		r.workerDone[i] = make(chan struct{})
	}
	return r, nil
}

func (r *UDPReceiver) Start(parent context.Context) error {
	if r == nil {
		return fail(CodeInvalidConfig, "Syslog UDP receiver is not initialized", nil)
	}
	if parent == nil {
		parent = context.Background()
	}
	if err := parent.Err(); err != nil {
		return lifecycleError(err)
	}

	r.mu.Lock()
	if r.started {
		state := r.state
		r.mu.Unlock()
		return fail(CodeInvalidConfig, "Syslog UDP receiver already started in state "+string(state), nil)
	}
	r.state = ReceiverStarting
	r.mu.Unlock()

	addr := &net.UDPAddr{IP: net.ParseIP(r.cfg.BindAddress), Port: r.cfg.Port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		r.setFailure(CodeInternal)
		return fail(CodeInternal, "Syslog UDP listener could not bind", err)
	}

	ctx, cancel := context.WithCancel(parent)
	r.mu.Lock()
	r.conn = conn
	r.ctx = ctx
	r.cancel = cancel
	r.started = true
	r.state = ReceiverRunning
	r.mu.Unlock()

	r.workersLeft.Store(int64(r.cfg.WorkerCount))
	for i := range r.workerDone {
		go r.worker(i)
	}
	go r.readLoop()
	return nil
}

func (r *UDPReceiver) Drain(ctx context.Context) error {
	return r.stopAndWait(ctx)
}

func (r *UDPReceiver) Shutdown(ctx context.Context) error {
	return r.stopAndWait(ctx)
}

func (r *UDPReceiver) stopAndWait(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if !r.isStarted() {
		r.mu.Lock()
		if r.state != ReceiverFailed {
			r.state = ReceiverStopped
		}
		r.mu.Unlock()
		return nil
	}

	r.initiateStop()
	waitCtx, cancel := r.boundedLifecycleContext(ctx)
	defer cancel()
	if err := r.waitForCompletion(waitCtx); err != nil {
		r.cancelContext()
		r.setLastError(CodeOf(err))
		return err
	}
	r.cancelContext()
	r.mu.Lock()
	if r.state != ReceiverFailed {
		r.state = ReceiverStopped
	}
	r.mu.Unlock()
	return nil
}

func (r *UDPReceiver) Snapshot() ReceiverSnapshot {
	if r == nil {
		return ReceiverSnapshot{State: ReceiverStopped}
	}
	r.mu.RLock()
	state, lastErr := r.state, r.lastErr
	r.mu.RUnlock()
	return ReceiverSnapshot{
		State:             state,
		DatagramsReceived: r.metrics.datagramsReceived.Load(),
		ParsedRFC3164:     r.metrics.parsedRFC3164.Load(),
		ParsedRFC5424:     r.metrics.parsedRFC5424.Load(),
		ParsedPartial:     r.metrics.parsedPartial.Load(),
		Malformed:         r.metrics.malformed.Load(),
		Oversized:         r.metrics.oversized.Load(),
		QueueFullDrops:    r.metrics.queueFullDrops.Load(),
		SinkSuccess:       r.metrics.sinkSuccess.Load(),
		SinkFailure:       r.metrics.sinkFailure.Load(),
		QueueDepth:        len(r.queue),
		QueueCapacity:     cap(r.queue),
		WorkerCount:       r.cfg.WorkerCount,
		LastErrorCode:     lastErr,
	}
}

func (r *UDPReceiver) worker(index int) {
	defer func() {
		close(r.workerDone[index])
		if r.workersLeft.Add(-1) == 0 {
			r.mu.Lock()
			if r.state != ReceiverFailed {
				r.state = ReceiverStopped
			}
			r.mu.Unlock()
		}
	}()
	for d := range r.queue {
		r.processDatagram(r.context(), d)
	}
}

func (r *UDPReceiver) processDatagram(ctx context.Context, d datagram) {
	if err := ctx.Err(); err != nil {
		r.setLastError(contextCode(err))
		return
	}
	event, err := r.parser.Parse(d.payload, d.receivedAt, SourceMetadata{Transport: TransportUDP, Address: d.source})
	if err != nil {
		r.metrics.malformed.Add(1)
		code := CodeOf(err)
		if code == "" {
			code = CodeInternal
		}
		r.setLastError(code)
		return
	}

	if event.Format == FormatUnknown || event.Quality == QualityPartial {
		r.metrics.parsedPartial.Add(1)
	} else {
		switch event.Format {
		case FormatRFC3164:
			r.metrics.parsedRFC3164.Add(1)
		case FormatRFC5424:
			r.metrics.parsedRFC5424.Add(1)
		default:
			r.metrics.malformed.Add(1)
			r.setLastError(CodeInternal)
			return
		}
	}

	if err := ctx.Err(); err != nil {
		r.setLastError(contextCode(err))
		return
	}
	if err := r.sink.Accept(ctx, event); err != nil {
		r.metrics.sinkFailure.Add(1)
		if ctxErr := ctx.Err(); ctxErr != nil {
			r.setLastError(contextCode(ctxErr))
		} else {
			r.setLastError(CodeInternal)
		}
		return
	}
	r.metrics.sinkSuccess.Add(1)
}

func (r *UDPReceiver) initiateStop() {
	r.stopOnce.Do(func() {
		r.mu.Lock()
		if r.state != ReceiverStopped && r.state != ReceiverFailed {
			r.state = ReceiverDraining
		}
		conn := r.conn
		r.mu.Unlock()
		if conn != nil {
			_ = conn.Close()
		}
	})
}

func (r *UDPReceiver) waitForCompletion(ctx context.Context) error {
	if err := waitChannel(ctx, r.readerDone); err != nil {
		return lifecycleError(err)
	}
	for _, done := range r.workerDone {
		if err := waitChannel(ctx, done); err != nil {
			return lifecycleError(err)
		}
	}
	return nil
}

func waitChannel(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func lifecycleError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fail(CodeTimeout, "Syslog UDP lifecycle deadline exceeded", err)
	}
	return fail(CodeCancelled, "Syslog UDP lifecycle was cancelled", err)
}

func contextCode(err error) ErrorCode {
	if errors.Is(err, context.DeadlineExceeded) {
		return CodeTimeout
	}
	return CodeCancelled
}

func (r *UDPReceiver) boundedLifecycleContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) <= r.cfg.ShutdownGrace {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, r.cfg.ShutdownGrace)
}

func (r *UDPReceiver) closeQueue() {
	r.queueCloseOnce.Do(func() { close(r.queue) })
}

func (r *UDPReceiver) closeConn() {
	r.mu.RLock()
	conn := r.conn
	r.mu.RUnlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (r *UDPReceiver) cancelContext() {
	r.mu.RLock()
	cancel := r.cancel
	r.mu.RUnlock()
	if cancel != nil {
		cancel()
	}
}

func (r *UDPReceiver) context() context.Context {
	r.mu.RLock()
	ctx := r.ctx
	r.mu.RUnlock()
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (r *UDPReceiver) isStarted() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.started
}

func (r *UDPReceiver) isStopping() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state == ReceiverDraining || r.state == ReceiverStopped || r.state == ReceiverFailed
}

func (r *UDPReceiver) setFailure(code ErrorCode) {
	if code == "" {
		code = CodeInternal
	}
	r.mu.Lock()
	r.lastErr = code
	r.state = ReceiverFailed
	r.mu.Unlock()
}

func (r *UDPReceiver) setLastError(code ErrorCode) {
	if code == "" {
		code = CodeInternal
	}
	r.mu.Lock()
	r.lastErr = code
	r.mu.Unlock()
}
