package syslogintegration

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"platform/sensor/internal/observation"
	protocolsyslog "platform/sensor/internal/protocol/syslog"
	sensorruntime "platform/sensor/internal/runtime"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
)

func parserConfig() protocolsyslog.ParserConfig {
	return protocolsyslog.ParserConfig{
		MaxInputMessageBytes: 4096, MaxRawEvidenceBytes: 1024,
		MaxHostnameBytes: 128, MaxAppNameBytes: 64, MaxProcessIDBytes: 32,
		MaxMessageIDBytes: 64, MaxMessageBytes: 2048,
		MaxStructuredDataElements: 8, MaxStructuredDataParamsPerElement: 8,
		MaxStructuredDataIDBytes: 64, MaxStructuredDataParamNameBytes: 64,
		MaxStructuredDataParamValueBytes: 256, MaxSourceAddressBytes: 256,
	}
}

func sinkConfig() SinkConfig {
	return SinkConfig{SensorID: "sensor-test", TenantID: "tenant-test", MaxEventBytes: 8192, MaxEnvelopeBytes: 16384}
}

func udpConfig(port int) protocolsyslog.UDPConfig {
	return protocolsyslog.UDPConfig{BindAddress: "127.0.0.1", Port: port, MaxDatagramBytes: 2048, WorkerCount: 2, QueueDepth: 64, ReadTimeout: 20 * time.Millisecond, ShutdownGrace: time.Second}
}

func parsedEvent(t *testing.T, message string) protocolsyslog.Event {
	t.Helper()
	p, err := protocolsyslog.NewParser(parserConfig())
	if err != nil {
		t.Fatal(err)
	}
	e, err := p.Parse([]byte(message), time.Date(2026, 9, 7, 7, 0, 0, 0, time.UTC), protocolsyslog.SourceMetadata{Transport: protocolsyslog.TransportUDP, Address: "127.0.0.1:55123"})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func decodeRecord(t *testing.T, rec spool.Record) (observation.EventEnvelope, protocolsyslog.Event) {
	t.Helper()
	env, err := observation.DecodeEventEnvelope(rec.Payload, sinkConfig().MaxEnvelopeBytes, sinkConfig().MaxEventBytes)
	if err != nil {
		t.Fatal(err)
	}
	e, err := protocolsyslog.DecodeEvent(env.Payload, sinkConfig().MaxEventBytes, parserConfig())
	if err != nil {
		t.Fatal(err)
	}
	return env, e
}

func TestDurableSinkRoundTripPreservesSyslogFields(t *testing.T) {
	dir := t.TempDir()
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	durable, err := NewDurableSink(sinkConfig(), parserConfig(), s)
	if err != nil {
		t.Fatal(err)
	}
	if err := durable.Accept(context.Background(), parsedEvent(t, "<13>Sep  7 12:30:00 host app[123]: hello")); err != nil {
		t.Fatal(err)
	}
	if err := durable.Accept(context.Background(), parsedEvent(t, `<34>1 2026-09-07T07:00:00Z host app 123 ID47 [meta a="b"][other c="d"] hello`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	batch, err := reopened.ReadBatch(4, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Records) != 2 {
		t.Fatalf("records=%d", len(batch.Records))
	}
	for _, rec := range batch.Records {
		if rec.Class != spool.ClassSyslog || rec.Type != RecordTypeSyslogEvent {
			t.Fatalf("record class/type=%s/%s", rec.Class, rec.Type)
		}
	}
	env3164, got3164 := decodeRecord(t, batch.Records[0])
	if env3164.Source.SensorID != "sensor-test" || env3164.Source.TenantID != "tenant-test" || env3164.Source.Transport != "UDP" || env3164.Source.SourceAddress != "127.0.0.1:55123" {
		t.Fatalf("RFC3164 source=%+v", env3164.Source)
	}
	if got3164.Facility != 1 || got3164.Severity != 5 || got3164.TimestampQuality != protocolsyslog.TimestampRFC3164Unresolved || got3164.EventTime != nil || got3164.ProcessID != "123" {
		t.Fatalf("RFC3164 event=%+v", got3164)
	}
	_, got5424 := decodeRecord(t, batch.Records[1])
	if got5424.Facility != 4 || got5424.Severity != 2 || len(got5424.StructuredData) != 2 || got5424.StructuredData[0].ID != "meta" || got5424.StructuredData[0].Params[0].Value != "b" || got5424.StructuredData[1].ID != "other" {
		t.Fatalf("RFC5424 event=%+v", got5424)
	}
}

func TestRepeatedIdenticalMessagesCreateSeparateDurableRecords(t *testing.T) {
	dir := t.TempDir()
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	durable, err := NewDurableSink(sinkConfig(), parserConfig(), s)
	if err != nil {
		t.Fatal(err)
	}
	e := parsedEvent(t, "<13>partial")
	if err := durable.Accept(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if err := durable.Accept(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	batch, err := s.ReadBatch(4, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Records) != 2 || batch.Records[0].Sequence == batch.Records[1].Sequence {
		t.Fatalf("duplicate receptions were not preserved independently: %+v", batch.Records)
	}
}

type failingAppender struct {
	mu  sync.Mutex
	err error
}

func (f *failingAppender) Append(spool.AppendRequest) (spool.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return spool.Record{}, f.err
	}
	return spool.Record{Sequence: 1}, nil
}
func (f *failingAppender) set(err error) { f.mu.Lock(); f.err = err; f.mu.Unlock() }

func TestStorageErrorClassificationAndRecovery(t *testing.T) {
	cases := []struct {
		err  error
		want ErrorCode
	}{
		{&spool.Error{Code: spool.CodeProtection}, CodeStorageProtection},
		{&spool.Error{Code: spool.CodeClosed}, CodeStorageUnavailable},
		{&spool.Error{Code: spool.CodeCorruption}, CodeStorageCorruption},
	}
	for _, tc := range cases {
		f := &failingAppender{err: tc.err}
		d, err := NewDurableSink(sinkConfig(), parserConfig(), f)
		if err != nil {
			t.Fatal(err)
		}
		if got := CodeOf(d.Accept(context.Background(), parsedEvent(t, "<13>partial"))); got != tc.want {
			t.Fatalf("got=%s want=%s", got, tc.want)
		}
		if !d.Snapshot().Degraded {
			t.Fatal("storage failure did not degrade sink")
		}
		f.set(nil)
		if err := d.Accept(context.Background(), parsedEvent(t, "<13>partial")); err != nil {
			t.Fatal(err)
		}
		if d.Snapshot().Degraded {
			t.Fatal("successful durable append did not clear storage degradation")
		}
	}
}

func TestEnvelopeBoundAndNoSecretSerialization(t *testing.T) {
	f := &captureAppender{}
	cfg := sinkConfig()
	cfg.MaxEventBytes = 64
	cfg.MaxEnvelopeBytes = 128
	d, err := NewDurableSink(cfg, parserConfig(), f)
	if err != nil {
		t.Fatal(err)
	}
	if got := CodeOf(d.Accept(context.Background(), parsedEvent(t, "<13>Sep  7 12:30:00 host app: this-message-is-long-enough-to-exceed-the-small-event-bound"))); got != CodeResourceLimit {
		t.Fatalf("got=%s", got)
	}

	cfg = sinkConfig()
	d, _ = NewDurableSink(cfg, parserConfig(), f)
	if err := d.Accept(context.Background(), parsedEvent(t, "<13>partial")); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	payload := string(f.requests[len(f.requests)-1].Payload)
	f.mu.Unlock()
	for _, forbidden := range []string{"community-secret", "auth-secret", "privacy-secret", "private-key-secret", "bearer-token-secret", "KAFKA"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("durable output leaked %q", forbidden)
		}
	}
}

func TestDurableSinkCancellationAndDeadline(t *testing.T) {
	f := &captureAppender{}
	d, err := NewDurableSink(sinkConfig(), parserConfig(), f)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := CodeOf(d.Accept(ctx, parsedEvent(t, "<13>partial"))); got != CodeCancelled {
		t.Fatalf("cancel code=%s", got)
	}
	deadline, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()
	if got := CodeOf(d.Accept(deadline, parsedEvent(t, "<13>partial"))); got != CodeDeadlineExceeded {
		t.Fatalf("deadline code=%s", got)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) != 0 {
		t.Fatal("cancelled/deadline sink attempt reached durable append")
	}
}

type captureAppender struct {
	mu       sync.Mutex
	requests []spool.AppendRequest
}

func (c *captureAppender) Append(req spool.AppendRequest) (spool.Record, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, req)
	return spool.Record{Sequence: uint64(len(c.requests))}, nil
}

func freeUDPPort(t *testing.T) int {
	t.Helper()
	c, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	p := c.LocalAddr().(*net.UDPAddr).Port
	_ = c.Close()
	return p
}
func sendUDP(t *testing.T, port int, payload string) int {
	t.Helper()
	c, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p := c.LocalAddr().(*net.UDPAddr).Port
	if _, err := c.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	return p
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
	t.Fatal("condition not reached")
}

func TestUDPToDurableSpoolRoundTripAndConcurrency(t *testing.T) {
	dir := t.TempDir()
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	port := freeUDPPort(t)
	component, err := NewUDPComponent(AssemblyConfig{UDP: udpConfig(port), Parser: parserConfig(), Sink: sinkConfig()}, s)
	if err != nil {
		t.Fatal(err)
	}
	if err := component.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	sourcePort := sendUDP(t, port, `<34>1 2026-09-07T07:00:00Z host app 123 ID47 [meta a="b"] hello`)
	waitFor(t, func() bool { return component.Snapshot().Receiver.SinkSuccess == 1 })
	const extra = 16
	var wg sync.WaitGroup
	errCh := make(chan error, extra)
	for i := 0; i < extra; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
			if err != nil {
				errCh <- err
				return
			}
			defer c.Close()
			if _, err := c.Write([]byte("<13>partial")); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent UDP send: %v", err)
		}
	}
	waitFor(t, func() bool { return component.Snapshot().Receiver.SinkSuccess == extra+1 })
	if err := component.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	batch, err := reopened.ReadBatch(extra+2, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Records) != extra+1 {
		t.Fatalf("records=%d", len(batch.Records))
	}
	env, got := decodeRecord(t, batch.Records[0])
	if env.Source.Transport != "UDP" || !strings.HasSuffix(env.Source.SourceAddress, ":"+itoa(sourcePort)) {
		t.Fatalf("source=%+v", env.Source)
	}
	if got.Source.Transport != protocolsyslog.TransportUDP {
		t.Fatalf("event source=%+v", got.Source)
	}
}
func itoa(v int) string {
	const digits = "0123456789"
	if v == 0 {
		return "0"
	}
	b := make([]byte, 0, 6)
	for v > 0 {
		b = append([]byte{digits[v%10]}, b...)
		v /= 10
	}
	return string(b)
}

type fakeScheduler struct{}

func (fakeScheduler) Submit(scheduler.Job) (scheduler.Submission, error) {
	return scheduler.Submission{}, nil
}
func (fakeScheduler) Shutdown(context.Context) error { return nil }
func (fakeScheduler) Snapshot() scheduler.Snapshot   { return scheduler.Snapshot{} }

func TestRuntimeRegistrationLifecycleAndHealthRecovery(t *testing.T) {
	for _, role := range []sensorruntime.Role{sensorruntime.RoleNetwork, sensorruntime.RoleStreamingEvent, sensorruntime.RoleCombined} {
		port := freeUDPPort(t)
		f := &failingAppender{}
		rt, err := sensorruntime.New(sensorruntime.Config{Role: role, EnabledCapabilities: []sensorruntime.Capability{sensorruntime.CapabilitySyslog}}, fakeScheduler{}, sensorruntime.NewExecutorRegistry())
		if err != nil {
			t.Fatal(err)
		}
		component, err := RegisterUDP(rt, AssemblyConfig{UDP: udpConfig(port), Parser: parserConfig(), Sink: sinkConfig()}, f)
		if err != nil {
			t.Fatalf("role %s: %v", role, err)
		}
		if _, err := RegisterUDP(rt, AssemblyConfig{UDP: udpConfig(freeUDPPort(t)), Parser: parserConfig(), Sink: sinkConfig()}, f); err == nil {
			t.Fatal("duplicate registration accepted")
		}
		if err := rt.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		if component.Snapshot().Receiver.State != protocolsyslog.ReceiverRunning {
			t.Fatalf("snapshot=%+v", component.Snapshot())
		}
		f.set(&spool.Error{Code: spool.CodeProtection})
		sendUDP(t, port, "<13>partial")
		waitFor(t, func() bool { return component.Snapshot().Receiver.SinkFailure == 1 })
		snap := component.Snapshot()
		if snap.Receiver.SinkSuccess != 0 || !snap.Sink.Degraded || snap.Sink.LastErrorCode != CodeStorageProtection {
			t.Fatalf("storage failure snapshot=%+v", snap)
		}
		if err := rt.RefreshHealth(context.Background()); err == nil {
			t.Fatal("storage failure did not degrade runtime health")
		}
		f.set(nil)
		sendUDP(t, port, "<13>partial")
		waitFor(t, func() bool { return component.Snapshot().Receiver.SinkSuccess == 1 })
		if err := rt.RefreshHealth(context.Background()); err != nil {
			t.Fatalf("health did not recover: %v", err)
		}
		if err := rt.Drain(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := rt.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProhibitedRoleRejected(t *testing.T) {
	rt, err := sensorruntime.New(sensorruntime.Config{Role: sensorruntime.RoleSynthetic}, fakeScheduler{}, sensorruntime.NewExecutorRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterUDP(rt, AssemblyConfig{UDP: udpConfig(freeUDPPort(t)), Parser: parserConfig(), Sink: sinkConfig()}, &captureAppender{}); err == nil {
		t.Fatal("Syslog registered in prohibited role")
	}
}
