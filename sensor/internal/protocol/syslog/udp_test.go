package syslog

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type recordingSink struct {
	mu      sync.Mutex
	events  []Event
	eventCh chan Event
	err     error
	block   <-chan struct{}
	entered chan struct{}
	active  atomic.Int64
	max     atomic.Int64
}

func newRecordingSink() *recordingSink {
	return &recordingSink{eventCh: make(chan Event, 256)}
}

func (s *recordingSink) Accept(ctx context.Context, event Event) error {
	current := s.active.Add(1)
	defer s.active.Add(-1)
	for {
		old := s.max.Load()
		if current <= old || s.max.CompareAndSwap(old, current) {
			break
		}
	}
	if s.entered != nil {
		select {
		case s.entered <- struct{}{}:
		default:
		}
	}
	if s.block != nil {
		select {
		case <-s.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if s.err != nil {
		return s.err
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	select {
	case s.eventCh <- event:
	default:
	}
	return nil
}

func testParserConfig() ParserConfig {
	return ParserConfig{
		MaxInputMessageBytes: 4096, MaxRawEvidenceBytes: 2048,
		MaxHostnameBytes: 255, MaxAppNameBytes: 128, MaxProcessIDBytes: 128,
		MaxMessageIDBytes: 128, MaxMessageBytes: 3072,
		MaxStructuredDataElements: 16, MaxStructuredDataParamsPerElement: 32,
		MaxStructuredDataIDBytes: 128, MaxStructuredDataParamNameBytes: 128,
		MaxStructuredDataParamValueBytes: 512, MaxSourceAddressBytes: 256,
	}
}

func testUDPConfig(port int) UDPConfig {
	return UDPConfig{
		BindAddress:      "127.0.0.1",
		Port:             port,
		MaxDatagramBytes: 2048,
		WorkerCount:      2,
		QueueDepth:       16,
		ReadTimeout:      20 * time.Millisecond,
		ShutdownGrace:    time.Second,
	}
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func sendUDP(t *testing.T, port int, payload []byte) int {
	t.Helper()
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	localPort := conn.LocalAddr().(*net.UDPAddr).Port
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	return localPort
}

func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("condition not reached before test deadline")
}

func newTestReceiver(t *testing.T, cfg UDPConfig, sink EventSink) *UDPReceiver {
	t.Helper()
	parser, err := NewParser(testParserConfig())
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewUDPReceiver(cfg, parser, sink)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestUDPConfigValidation(t *testing.T) {
	cfg := testUDPConfig(5514)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	cases := []struct {
		name string
		edit func(*UDPConfig)
	}{
		{"invalid host", func(c *UDPConfig) { c.BindAddress = "not-a-host" }},
		{"invalid port zero", func(c *UDPConfig) { c.Port = 0 }},
		{"invalid port high", func(c *UDPConfig) { c.Port = 65536 }},
		{"invalid datagram", func(c *UDPConfig) { c.MaxDatagramBytes = 0 }},
		{"invalid workers", func(c *UDPConfig) { c.WorkerCount = 0 }},
		{"invalid queue", func(c *UDPConfig) { c.QueueDepth = -1 }},
		{"invalid read timeout", func(c *UDPConfig) { c.ReadTimeout = 0 }},
		{"invalid shutdown grace", func(c *UDPConfig) { c.ShutdownGrace = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := cfg
			tc.edit(&candidate)
			if err := candidate.Validate(); CodeOf(err) != CodeInvalidConfig {
				t.Fatalf("expected invalid config, got %v", err)
			}
		})
	}
}

func TestUDPReceiverStartsOnLocalhost(t *testing.T) {
	port := freeUDPPort(t)
	r := newTestReceiver(t, testUDPConfig(port), newRecordingSink())
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := r.Snapshot().State; got != ReceiverRunning {
		t.Fatalf("state=%s", got)
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestUDPReceiverParsesRFC3164AndPreservesSource(t *testing.T) {
	port := freeUDPPort(t)
	sink := newRecordingSink()
	r := newTestReceiver(t, testUDPConfig(port), sink)
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	sourcePort := sendUDP(t, port, []byte("<13>Sep  7 11:00:00 host app[123]: hello"))
	var event Event
	select {
	case event = <-sink.eventCh:
	case <-time.After(time.Second):
		t.Fatal("RFC3164 event did not reach sink")
	}
	if event.Format != FormatRFC3164 || event.Source.Transport != TransportUDP {
		t.Fatalf("unexpected event: %+v", event)
	}
	host, portText, err := net.SplitHostPort(event.Source.Address)
	if err != nil {
		t.Fatalf("source address=%q: %v", event.Source.Address, err)
	}
	if host != "127.0.0.1" || portText != strconv.Itoa(sourcePort) {
		t.Fatalf("source address=%q want 127.0.0.1:%d", event.Source.Address, sourcePort)
	}
	waitFor(t, func() bool { return r.Snapshot().SinkSuccess == 1 })
	if s := r.Snapshot(); s.ParsedRFC3164 != 1 || s.SinkSuccess != 1 {
		t.Fatalf("snapshot=%+v", s)
	}
	_ = r.Shutdown(context.Background())
}

func TestUDPReceiverParsesRFC5424AndPartial(t *testing.T) {
	port := freeUDPPort(t)
	sink := newRecordingSink()
	r := newTestReceiver(t, testUDPConfig(port), sink)
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	sendUDP(t, port, []byte(`<34>1 2026-09-07T05:30:00Z host app 123 ID47 [meta a="b"] hello`))
	sendUDP(t, port, []byte(`<13>partial-content`))
	waitFor(t, func() bool { return r.Snapshot().SinkSuccess == 2 })
	s := r.Snapshot()
	if s.ParsedRFC5424 != 1 || s.ParsedPartial != 1 || s.SinkSuccess != 2 {
		t.Fatalf("snapshot=%+v", s)
	}
	_ = r.Shutdown(context.Background())
}

func TestUDPMalformedDoesNotReachSink(t *testing.T) {
	port := freeUDPPort(t)
	sink := newRecordingSink()
	r := newTestReceiver(t, testUDPConfig(port), sink)
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	sendUDP(t, port, []byte("not-syslog"))
	waitFor(t, func() bool { return r.Snapshot().Malformed == 1 })
	if s := r.Snapshot(); s.SinkSuccess != 0 || s.Malformed != 1 {
		t.Fatalf("snapshot=%+v", s)
	}
	select {
	case <-sink.eventCh:
		t.Fatal("malformed datagram reached sink")
	default:
	}
	_ = r.Shutdown(context.Background())
}

func TestUDPOversizedRejectedWithoutTruncatedParse(t *testing.T) {
	port := freeUDPPort(t)
	cfg := testUDPConfig(port)
	cfg.MaxDatagramBytes = 64
	sink := newRecordingSink()
	r := newTestReceiver(t, cfg, sink)
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	payload := append([]byte("<13>"), make([]byte, 100)...)
	sendUDP(t, port, payload)
	waitFor(t, func() bool { return r.Snapshot().Oversized == 1 })
	s := r.Snapshot()
	if s.Oversized != 1 || s.SinkSuccess != 0 || s.LastErrorCode != CodeMessageTooLarge {
		t.Fatalf("snapshot=%+v", s)
	}
	_ = r.Shutdown(context.Background())
}

func TestUDPQueueFullDropsNewDatagram(t *testing.T) {
	port := freeUDPPort(t)
	cfg := testUDPConfig(port)
	cfg.WorkerCount = 1
	cfg.QueueDepth = 1
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	sink := newRecordingSink()
	sink.block = release
	sink.entered = entered
	r := newTestReceiver(t, cfg, sink)
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	sendUDP(t, port, []byte("<13>first"))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not enter sink")
	}
	sendUDP(t, port, []byte("<13>second"))
	waitFor(t, func() bool { return r.Snapshot().QueueDepth == 1 })
	sendUDP(t, port, []byte("<13>third"))
	waitFor(t, func() bool { return r.Snapshot().QueueFullDrops == 1 })
	if s := r.Snapshot(); s.QueueCapacity != 1 || s.QueueFullDrops != 1 || s.LastErrorCode != CodeQueueFull {
		t.Fatalf("snapshot=%+v", s)
	}
	close(release)
	_ = r.Shutdown(context.Background())
}

func TestUDPFixedWorkerLimit(t *testing.T) {
	port := freeUDPPort(t)
	cfg := testUDPConfig(port)
	cfg.WorkerCount = 2
	cfg.QueueDepth = 16
	release := make(chan struct{})
	entered := make(chan struct{}, 16)
	sink := newRecordingSink()
	sink.block = release
	sink.entered = entered
	r := newTestReceiver(t, cfg, sink)
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		sendUDP(t, port, []byte("<13>work"))
	}
	waitFor(t, func() bool { return sink.max.Load() == int64(cfg.WorkerCount) })
	if got := sink.max.Load(); got > int64(cfg.WorkerCount) {
		t.Fatalf("max concurrency=%d workers=%d", got, cfg.WorkerCount)
	}
	close(release)
	waitFor(t, func() bool { return r.Snapshot().SinkSuccess == 8 })
	_ = r.Shutdown(context.Background())
}

func TestUDPConcurrentDatagramsRaceSafe(t *testing.T) {
	port := freeUDPPort(t)
	cfg := testUDPConfig(port)
	cfg.WorkerCount = 4
	cfg.QueueDepth = 128
	sink := newRecordingSink()
	r := newTestReceiver(t, cfg, sink)
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	const count = 32
	var wg sync.WaitGroup
	errCh := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
			if err != nil {
				errCh <- err
				return
			}
			defer conn.Close()
			_, err = conn.Write([]byte("<13>concurrent"))
			if err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}
	waitFor(t, func() bool { return r.Snapshot().SinkSuccess == count })
	if s := r.Snapshot(); s.QueueFullDrops != 0 || s.SinkSuccess != count {
		t.Fatalf("snapshot=%+v", s)
	}
	_ = r.Shutdown(context.Background())
}

func TestUDPContextCancellationClosesReceiver(t *testing.T) {
	port := freeUDPPort(t)
	ctx, cancel := context.WithCancel(context.Background())
	r := newTestReceiver(t, testUDPConfig(port), newRecordingSink())
	if err := r.Start(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	waitFor(t, func() bool { return r.Snapshot().State == ReceiverStopped })
	probe, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		t.Fatalf("socket not released after cancellation: %v", err)
	}
	_ = probe.Close()
}

func TestUDPShutdownReleasesSocketAndIsIdempotent(t *testing.T) {
	port := freeUDPPort(t)
	r := newTestReceiver(t, testUDPConfig(port), newRecordingSink())
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := r.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	probe, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		t.Fatalf("socket not released: %v", err)
	}
	_ = probe.Close()
}

func TestUDPDrainPreventsNewInputAndCompletesQueuedWork(t *testing.T) {
	port := freeUDPPort(t)
	cfg := testUDPConfig(port)
	cfg.WorkerCount = 1
	cfg.QueueDepth = 4
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	sink := newRecordingSink()
	sink.block = release
	sink.entered = entered
	r := newTestReceiver(t, cfg, sink)
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	sendUDP(t, port, []byte("<13>first"))
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("first event did not reach sink")
	}
	sendUDP(t, port, []byte("<13>second"))
	waitFor(t, func() bool { return r.Snapshot().QueueDepth == 1 })
	drainDone := make(chan error, 1)
	go func() { drainDone <- r.Drain(context.Background()) }()
	waitFor(t, func() bool { return r.Snapshot().State == ReceiverDraining })
	before := r.Snapshot().DatagramsReceived
	sendUDP(t, port, []byte("<13>after-drain"))
	close(release)
	select {
	case err := <-drainDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("drain did not complete")
	}
	if r.Snapshot().SinkSuccess != 2 {
		t.Fatalf("queued work was not drained: %+v", r.Snapshot())
	}
	time.Sleep(2 * cfg.ReadTimeout)
	if r.Snapshot().DatagramsReceived != before {
		t.Fatalf("drain accepted new input: before=%d after=%d", before, r.Snapshot().DatagramsReceived)
	}
}

func TestUDPSinkFailureAndSuccessCounters(t *testing.T) {
	port := freeUDPPort(t)
	failedSink := newRecordingSink()
	failedSink.err = errors.New("sink failed")
	r := newTestReceiver(t, testUDPConfig(port), failedSink)
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	sendUDP(t, port, []byte("<13>failure"))
	waitFor(t, func() bool { return r.Snapshot().SinkFailure == 1 })
	if s := r.Snapshot(); s.SinkFailure != 1 || s.SinkSuccess != 0 || s.LastErrorCode != CodeInternal {
		t.Fatalf("snapshot=%+v", s)
	}
	_ = r.Shutdown(context.Background())

	port = freeUDPPort(t)
	successSink := newRecordingSink()
	r = newTestReceiver(t, testUDPConfig(port), successSink)
	if err := r.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	sendUDP(t, port, []byte("<13>success"))
	waitFor(t, func() bool { return r.Snapshot().SinkSuccess == 1 })
	if r.Snapshot().SinkFailure != 0 {
		t.Fatalf("snapshot=%+v", r.Snapshot())
	}
	_ = r.Shutdown(context.Background())
}

func TestUDPErrorDoesNotEchoInboundPayload(t *testing.T) {
	secret := "ATTACKER-CONTROLLED-COMPLETE-SYSLOG-PAYLOAD"
	err := fail(CodeMalformedPRI, "malformed Syslog PRI", errors.New(secret))
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked inbound payload: %v", err)
	}
}

func TestUDPReceiverSnapshotHasNoHighCardinalityMaps(t *testing.T) {
	typ := reflect.TypeOf(ReceiverSnapshot{})
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Type.Kind() == reflect.Map {
			t.Fatalf("high-cardinality map field %s", typ.Field(i).Name)
		}
	}
}
