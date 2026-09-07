package syslogintegration

import (
	"context"
	"net"
	"strconv"
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

func parseEvent(t *testing.T, message string, source string) protocolsyslog.Event {
	t.Helper()
	p, err := protocolsyslog.NewParser(parserConfig())
	if err != nil {
		t.Fatal(err)
	}
	e, err := p.Parse([]byte(message), time.Date(2026, 9, 7, 7, 0, 0, 0, time.UTC), protocolsyslog.SourceMetadata{Transport: protocolsyslog.TransportUDP, Address: source})
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

func TestDurableSinkRoundTripPreservesFieldsAndRepeatedEvents(t *testing.T) {
	dir := t.TempDir()
	s, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDurableSink(sinkConfig(), parserConfig(), s)
	if err != nil {
		t.Fatal(err)
	}
	e3164 := parseEvent(t, "<13>Sep  7 12:30:00 host app[123]: hello", "127.0.0.1:55123")
	e5424 := parseEvent(t, "<34>1 2026-09-07T07:00:00Z host app 123 ID47 [meta a=\"b\"][other c=\"d\"] hello", "127.0.0.1:55124")
	for _, e := range []protocolsyslog.Event{e3164, e5424, e5424} {
		if err := d.Accept(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	batch, err := reopened.ReadBatch(8, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Records) != 3 {
		t.Fatalf("records=%d", len(batch.Records))
	}
	for i, rec := range batch.Records {
		if rec.Class != spool.ClassSyslog || rec.Type != RecordTypeSyslogEvent {
			t.Fatalf("record %d class/type=%s/%s", i, rec.Class, rec.Type)
		}
	}
	if batch.Records[1].Sequence == batch.Records[2].Sequence {
		t.Fatal("identical receptions were deduplicated")
	}
	env3164, got3164 := decodeRecord(t, batch.Records[0])
	if env3164.Source.SensorID != "sensor-test" || env3164.Source.TenantID != "tenant-test" || env3164.Source.Transport != "UDP" || env3164.Source.SourceAddress != "127.0.0.1:55123" {
		t.Fatalf("source=%+v", env3164.Source)
	}
	if got3164.Facility != 1 || got3164.Severity != 5 || got3164.TimestampQuality != protocolsyslog.TimestampRFC3164Unresolved || got3164.EventTime != nil || got3164.ProcessID != "123" {
		t.Fatalf("RFC3164=%+v", got3164)
	}
	_, got5424 := decodeRecord(t, batch.Records[1])
	if got5424.Facility != 4 || got5424.Severity != 2 || len(got5424.StructuredData) != 2 || got5424.StructuredData[0].Params[0].Value != "b" || got5424.StructuredData[1].ID != "other" {
		t.Fatalf("RFC5424=%+v", got5424)
	}
}

type controlledAppender struct {
	mu       sync.Mutex
	err      error
	requests []spool.AppendRequest
}

func (f *controlledAppender) Append(req spool.AppendRequest) (spool.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return spool.Record{}, f.err
	}
	f.requests = append(f.requests, req)
	return spool.Record{Sequence: uint64(len(f.requests))}, nil
}
func (f *controlledAppender) set(err error) { f.mu.Lock(); f.err = err; f.mu.Unlock() }

func TestStorageErrorsBoundsCancellationAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		in   error
		want ErrorCode
	}{
		{&spool.Error{Code: spool.CodeProtection}, CodeStorageProtection},
		{&spool.Error{Code: spool.CodeClosed}, CodeStorageUnavailable},
		{&spool.Error{Code: spool.CodeCorruption}, CodeStorageCorruption},
	} {
		f := &controlledAppender{err: tc.in}
		d, err := NewDurableSink(sinkConfig(), parserConfig(), f)
		if err != nil {
			t.Fatal(err)
		}
		if got := CodeOf(d.Accept(context.Background(), parseEvent(t, "<13>partial", "127.0.0.1:1"))); got != tc.want {
			t.Fatalf("code=%s want=%s", got, tc.want)
		}
		if !d.Snapshot().Degraded {
			t.Fatal("storage failure did not degrade sink")
		}
		f.set(nil)
		if err := d.Accept(context.Background(), parseEvent(t, "<13>partial", "127.0.0.1:1")); err != nil {
			t.Fatal(err)
		}
		if d.Snapshot().Degraded {
			t.Fatal("successful append did not clear storage degradation")
		}
	}

	f := &controlledAppender{}
	cfg := sinkConfig()
	cfg.MaxEventBytes = 64
	cfg.MaxEnvelopeBytes = 128
	d, err := NewDurableSink(cfg, parserConfig(), f)
	if err != nil {
		t.Fatal(err)
	}
	if got := CodeOf(d.Accept(context.Background(), parseEvent(t, "<13>Sep  7 12:30:00 host app: message-exceeding-small-bound", "127.0.0.1:1"))); got != CodeResourceLimit {
		t.Fatalf("bound code=%s", got)
	}

	cfg = sinkConfig()
	d, _ = NewDurableSink(cfg, parserConfig(), f)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := CodeOf(d.Accept(ctx, parseEvent(t, "<13>partial", "127.0.0.1:1"))); got != CodeCancelled {
		t.Fatalf("cancel code=%s", got)
	}
	deadline, cancelDeadline := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelDeadline()
	if got := CodeOf(d.Accept(deadline, parseEvent(t, "<13>partial", "127.0.0.1:1"))); got != CodeDeadlineExceeded {
		t.Fatalf("deadline code=%s", got)
	}
}

func TestDurableOutputContainsNoUnrelatedSecrets(t *testing.T) {
	f := &controlledAppender{}
	d, err := NewDurableSink(sinkConfig(), parserConfig(), f)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Accept(context.Background(), parseEvent(t, "<13>partial", "127.0.0.1:1")); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	payload := string(f.requests[0].Payload)
	f.mu.Unlock()
	for _, forbidden := range []string{"community-secret", "auth-secret", "privacy-secret", "private-key-secret", "bearer-token-secret", "KAFKA"} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("durable output leaked %q", forbidden)
		}
	}
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
	sourcePort := sendUDP(t, port, "<34>1 2026-09-07T07:00:00Z host app 123 ID47 [meta a=\"b\"] hello")
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
	if env.Source.Transport != "UDP" || !strings.HasSuffix(env.Source.SourceAddress, ":"+strconv.Itoa(sourcePort)) {
		t.Fatalf("source=%+v", env.Source)
	}
	if got.Source.Transport != protocolsyslog.TransportUDP {
		t.Fatalf("event source=%+v", got.Source)
	}
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
		f := &controlledAppender{}
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
	if _, err := RegisterUDP(rt, AssemblyConfig{UDP: udpConfig(freeUDPPort(t)), Parser: parserConfig(), Sink: sinkConfig()}, &controlledAppender{}); err == nil {
		t.Fatal("Syslog registered in prohibited role")
	}
}
