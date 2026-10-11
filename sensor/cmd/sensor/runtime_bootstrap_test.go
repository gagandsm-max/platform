package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"platform/sensor/internal/collector"
	"platform/sensor/internal/observation"
	protocolsnmp "platform/sensor/internal/protocol/snmp"
	sensorruntime "platform/sensor/internal/runtime"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
)

func bootstrapFixture(t *testing.T, caps ...sensorruntime.Capability) (*snmpBootstrap, *spool.DiskSpool, string) {
	t.Helper()
	cfg := testRuntime()
	cfg.EnabledCapabilities = caps
	dir := t.TempDir()
	disk, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1 << 20, Logger: slog.Default()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disk.Close() })
	sink, err := observation.NewDiskSink(disk, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	b, err := newSNMPBootstrap(cfg, sink, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := b.Runtime.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return b, disk, dir
}

func TestSNMPBootstrapStartOwnershipAndShutdown(t *testing.T) {
	b, _, _ := bootstrapFixture(t, sensorruntime.CapabilitySNMP)
	if !b.registry.HasCapability(sensorruntime.CapabilitySNMP) || b.registry.HasCapability(sensorruntime.CapabilityICMP) {
		t.Fatal("incorrect real executor registry")
	}
	if err := b.Runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	snap := b.Runtime.Snapshot()
	if snap.State != sensorruntime.StateReady || len(snap.Components) != 1 || snap.Components[0].Name != "snmp" || snap.Components[0].State != sensorruntime.ComponentReady {
		t.Fatalf("runtime=%+v", snap)
	}
	if snap.AppliedConfigVersion != 0 || snap.DesiredConfigVersion != 1 {
		t.Fatal("bootstrap falsely applied control configuration")
	}
	if err := b.Runtime.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Runtime.Submit(bootstrapJob(nil)); err == nil {
		t.Fatal("draining accepted work")
	}
	for i := 0; i < 2; i++ {
		if err := b.Runtime.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if b.Scheduler.Snapshot().State != scheduler.StateStopped || b.Runtime.Snapshot().Scheduler.State != scheduler.StateStopped {
		t.Fatal("runtime did not stop the shared production scheduler")
	}
}

func TestSNMPBootstrapDefaultCapabilitiesFailClosed(t *testing.T) {
	data, err := os.ReadFile("../../../compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "PLATFORM_SENSOR_ENABLED_CAPABILITIES: SNMP,ICMP") {
		t.Fatal("default changed; reassess compatibility test")
	}
	for _, caps := range [][]sensorruntime.Capability{{sensorruntime.CapabilitySNMP, sensorruntime.CapabilityICMP}, {sensorruntime.CapabilityICMP}} {
		b, _, _ := bootstrapFixture(t, caps...)
		err := b.Runtime.Start(context.Background())
		if err == nil || !strings.Contains(err.Error(), "SENSOR_RUNTIME_CAPABILITY_UNIMPLEMENTED") || !strings.Contains(err.Error(), "ICMP") {
			t.Fatalf("start=%v", err)
		}
		snap := b.Runtime.Snapshot()
		if snap.State != sensorruntime.StateFailed || len(snap.EnabledCapabilities) != len(caps) {
			t.Fatal("unsupported requested capability hidden")
		}
		if _, err := b.Runtime.Submit(bootstrapJob(nil)); err == nil {
			t.Fatal("failed runtime admitted work")
		}
	}
}

func bootstrapJob(payload []byte) scheduler.Job {
	return scheduler.Job{JobID: "snmp-bootstrap-job", AssignmentKey: "snmp-bootstrap-assignment", TenantID: "tenant-bootstrap", EntityID: "entity-bootstrap", Kind: protocolsnmp.SchedulerJobKind, Class: scheduler.ClassNetworkPoll, AssignmentVersion: 1, LeaseExpiresAt: time.Now().Add(30 * time.Second), Timeout: time.Second, Payload: payload}
}

func waitBootstrapJob(t *testing.T, b *snmpBootstrap, failed bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snap := b.Runtime.Snapshot().Scheduler
		if snap.TrackedJobs == 0 && ((failed && snap.Failed == 1) || (!failed && snap.Completed == 1)) {
			actual := b.Scheduler.Snapshot()
			if actual.Submitted != 1 || actual.Completed != snap.Completed || actual.Failed != snap.Failed {
				t.Fatal("runtime uses a different scheduler")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job did not finish: %+v", b.Scheduler.Snapshot())
}

// A loopback UDP peer exchanges real BER packets with the production GoSNMP
// client. Only the managed device is a fixture; the entire Sensor path is real.
func localBootstrapSNMP(t *testing.T, typ gosnmp.Asn1BER, value any) (int, <-chan struct{}) {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	received := make(chan struct{}, 1)
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, peer, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			req, err := (&gosnmp.GoSNMP{}).UnmarshalTrap(buf[:n], false)
			if err != nil {
				continue
			}
			if req.PDUType != gosnmp.GetRequest || req.Community != "fixture-only" || len(req.Variables) != 1 || req.Variables[0].Name != ".1.3.6.1.2.1.1.3.0" {
				continue
			}
			response := &gosnmp.SnmpPacket{Version: gosnmp.Version2c, Community: req.Community, PDUType: gosnmp.GetResponse, RequestID: req.RequestID, Variables: []gosnmp.SnmpPDU{{Name: req.Variables[0].Name, Type: typ, Value: value}}}
			wire, err := response.MarshalMsg()
			if err != nil {
				continue
			}
			if _, err := conn.WriteToUDP(wire, peer); err == nil {
				select {
				case received <- struct{}{}:
				default:
				}
			}
		}
	}()
	t.Cleanup(func() { _ = conn.Close(); <-done })
	return conn.LocalAddr().(*net.UDPAddr).Port, received
}

func TestSNMPBootstrapRealUDPToDurableSpool(t *testing.T) {
	for _, tc := range []struct {
		name            string
		typ             gosnmp.Asn1BER
		value           any
		status, quality string
	}{{"value", gosnmp.TimeTicks, uint32(1234567), "COLLECTED", "GOOD"}, {"no_such_object", gosnmp.NoSuchObject, nil, "COLLECTED_WITH_EXCEPTIONS", "UNKNOWN"}} {
		t.Run(tc.name, func(t *testing.T) {
			b, disk, dir := bootstrapFixture(t, sensorruntime.CapabilitySNMP)
			port, received := localBootstrapSNMP(t, tc.typ, tc.value)
			req := protocolsnmp.Request{Target: "127.0.0.1", Port: port, Version: protocolsnmp.Version2c, Operation: protocolsnmp.OperationGet, OIDs: []string{".1.3.6.1.2.1.1.3.0"}, Credentials: protocolsnmp.Credentials{Community: &protocolsnmp.CommunityCredentials{Community: protocolsnmp.Secret("fixture-only")}}, Timeout: 200 * time.Millisecond, ExecutionTimeout: time.Second, Retry: protocolsnmp.RetryPolicy{MaxAttempts: 1}, Limits: protocolsnmp.Limits{MaxOIDs: 1, MaxWalkEntries: 1, MaxBulkRepetitions: 1, MaxResults: 1, MaxResultBytes: 1 << 20}}
			payload, err := protocolsnmp.EncodeSchedulerPayload(req)
			if err != nil {
				t.Fatal(err)
			}
			if err := b.Runtime.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			if sub, err := b.Runtime.Submit(bootstrapJob(payload)); err != nil || sub.Status != scheduler.SubmissionAccepted {
				t.Fatalf("submit=%+v %v", sub, err)
			}
			waitBootstrapJob(t, b, false)
			select {
			case <-received:
			default:
				t.Fatal("no actual UDP request/response")
			}
			if snap := b.SNMP.Snapshot(); snap.Executions != 1 || snap.Successful != 1 || snap.Failed != 0 {
				t.Fatalf("SNMP=%+v", snap)
			}
			if err := b.Runtime.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			// Close/reopen proves a durable record, beyond an in-memory sink receipt.

			if err := disk.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1 << 20, Logger: slog.Default()})
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			batch, err := reopened.ReadBatch(10, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			if len(batch.Records) != 1 {
				t.Fatalf("records=%d", len(batch.Records))
			}
			rec := batch.Records[0]
			var env observation.Envelope
			if err := json.Unmarshal(rec.Payload, &env); err != nil {
				t.Fatal(err)
			}
			if rec.Sequence != 1 || env.Protocol != "SNMP" || env.SensorID != "sensor-001" || env.TenantID != "tenant-bootstrap" || env.EntityID != "entity-bootstrap" || env.Job.AssignmentVersion != 1 || env.Status != tc.status || env.Quality != tc.quality {
				t.Fatalf("envelope=%+v", env)
			}
			if strings.Contains(string(rec.Payload), "fixture-only") || strings.Contains(string(rec.Payload), "DOWN") {
				t.Fatal("secret or false device-down in durable evidence")
			}
			if tc.name == "value" && !strings.Contains(string(env.ProtocolPayload), "1234567") {
				t.Fatal("actual polled value missing")
			}
		})
	}
}

func TestSNMPBootstrapUnknownJobRejected(t *testing.T) {
	b, _, _ := bootstrapFixture(t, sensorruntime.CapabilitySNMP)
	if err := b.Runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	job := bootstrapJob([]byte(`{}`))
	job.Kind = "UNKNOWN_KIND_NO_ROUTE"
	if _, err := b.Runtime.Submit(job); err != nil {
		t.Fatal(err)
	}
	waitBootstrapJob(t, b, true)
	if b.Scheduler.Snapshot().Retries != 0 || b.SNMP.Snapshot().Executions != 0 {
		t.Fatal("unknown route executed or retried")
	}
}

func TestSNMPBootstrapHeartbeatStillSpools(t *testing.T) {
	b, disk, _ := bootstrapFixture(t, sensorruntime.CapabilitySNMP)
	if err := b.Runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	sink, err := observation.NewDiskSink(disk, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	engine := collector.NewCollectorEngine("sensor-001", "NETWORK", sink, 5*time.Millisecond, slog.Default())
	engine.Start(context.Background())
	defer engine.Stop()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		batch, err := disk.ReadBatch(1, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		if len(batch.Records) > 0 {
			if !strings.Contains(string(batch.Records[0].Payload), "sensor.heartbeat") || b.Scheduler.Snapshot().Submitted != 0 {
				t.Fatal("heartbeat changed or duplicate polling admitted")
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("heartbeat not spooled")
}

func TestSNMPBootstrapRejectsMissingSink(t *testing.T) {
	if _, err := newSNMPBootstrap(testRuntime(), nil, slog.Default()); err == nil {
		t.Fatal("missing sink accepted")
	}
	if _, err := newProductionScheduler(testRuntime(), nil, slog.Default()); err == nil {
		t.Fatal("missing registry accepted")
	}
}

// Startup failure injection is a lifecycle test only; no fake ICMP executor is
// registered in production or represented as a successful poll.
type failingBootstrapComponent struct{}

func (failingBootstrapComponent) Name() string { return "startup-failure-fixture" }
func (failingBootstrapComponent) Capability() sensorruntime.Capability {
	return sensorruntime.CapabilityICMP
}
func (failingBootstrapComponent) Start(context.Context) error {
	return fmt.Errorf("injected startup failure")
}
func (failingBootstrapComponent) Ready(context.Context) error    { return nil }
func (failingBootstrapComponent) Drain(context.Context) error    { return nil }
func (failingBootstrapComponent) Shutdown(context.Context) error { return nil }
func TestSNMPBootstrapComponentStartupFailure(t *testing.T) {
	b, _, _ := bootstrapFixture(t, sensorruntime.CapabilitySNMP, sensorruntime.CapabilityICMP)
	if err := b.Runtime.RegisterComponent(failingBootstrapComponent{}); err != nil {
		t.Fatal(err)
	}
	err := b.Runtime.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "SENSOR_RUNTIME_COMPONENT_START_FAILED") || b.Runtime.Snapshot().State != sensorruntime.StateFailed {
		t.Fatalf("start=%v", err)
	}
	if err := b.Runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if b.Scheduler.Snapshot().State != scheduler.StateStopped {
		t.Fatal("startup failure leaked scheduler")
	}
}

func TestSNMPBootstrapActivePollShutdown(t *testing.T) {
	b, disk, _ := bootstrapFixture(t, sensorruntime.CapabilitySNMP)
	peer, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	// The peer receives the actual SNMP request but sends no reply.
	req := protocolsnmp.Request{Target: "127.0.0.1", Port: peer.LocalAddr().(*net.UDPAddr).Port, Version: protocolsnmp.Version2c, Operation: protocolsnmp.OperationGet, OIDs: []string{".1.3.6.1.2.1.1.3.0"}, Credentials: protocolsnmp.Credentials{Community: &protocolsnmp.CommunityCredentials{Community: protocolsnmp.Secret("fixture-only")}}, Timeout: 2 * time.Second, ExecutionTimeout: 3 * time.Second, Retry: protocolsnmp.RetryPolicy{MaxAttempts: 1}, Limits: protocolsnmp.Limits{MaxOIDs: 1, MaxWalkEntries: 1, MaxBulkRepetitions: 1, MaxResults: 1, MaxResultBytes: 1 << 20}}
	payload, err := protocolsnmp.EncodeSchedulerPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Runtime.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	job := bootstrapJob(payload)
	job.Timeout = 3 * time.Second
	if _, err := b.Runtime.Submit(job); err != nil {
		t.Fatal(err)
	}
	if err := peer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := peer.ReadFromUDP(make([]byte, 4096)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := b.Runtime.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	snap := b.Scheduler.Snapshot()
	if snap.State != scheduler.StateStopped || snap.ActiveWorkers != 0 || snap.Completed != 0 {
		t.Fatalf("shutdown=%+v", snap)
	}
	batch, err := disk.ReadBatch(10, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Records) != 0 || b.SNMP.Snapshot().Successful != 0 {
		t.Fatal("cancelled poll fabricated success or device-down evidence")
	}
}
