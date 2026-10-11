package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
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

// Execute the actual production main, including Agent receiver/drain startup,
// in a subprocess so signal handling and bounded termination are exercised.
func TestProductionEntrypoint(t *testing.T) {
	if os.Getenv("BUILD020_STARTUP_CHILD") == "1" {
		main()
		os.Exit(0)
	}
	for _, role := range []string{"NETWORK", "INFRASTRUCTURE"} {
		t.Run(role, func(t *testing.T) {
			var discoveryRequests, controlRequests, uploadRequests atomic.Int64
			core := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/sensor/discovery/tasks":
					discoveryRequests.Add(1)
				case "/api/v1/sensor/control/desired-state":
					controlRequests.Add(1)
				case "/api/v1/telemetry":
					uploadRequests.Add(1)
				}
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer core.Close()
			dir := t.TempDir()
			certPath, keyPath := filepath.Join(dir, "fixture.crt"), filepath.Join(dir, "fixture.key")
			key, err := x509.MarshalPKCS8PrivateKey(core.TLS.Certificates[0].PrivateKey)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: core.TLS.Certificates[0].Certificate[0]}), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600); err != nil {
				t.Fatal(err)
			}
			address := func() string {
				ln, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				addr := ln.Addr().String()
				ln.Close()
				return addr
			}
			healthAddr, agentAddr := address(), address()
			caps := "SNMP,ICMP"
			if role == "INFRASTRUCTURE" {
				caps = "INFRA,AGENT_INGRESS"
			}
			values := map[string]string{
				"BUILD020_STARTUP_CHILD": "1",
				"PLATFORM_SENSOR_ID":     "sensor-001", "PLATFORM_SENSOR_ROLE": role,
				"PLATFORM_SENSOR_ENABLED_CAPABILITIES": caps,
				"PLATFORM_SENSOR_CORE_ENDPOINT":        core.URL,
				"PLATFORM_SENSOR_SPOOL_PATH":           filepath.Join(dir, "telemetry"),
				"PLATFORM_SENSOR_HEALTH_ADDRESS":       healthAddr,
				"PLATFORM_SENSOR_TLS_CERT_FILE":        certPath, "PLATFORM_SENSOR_TLS_KEY_FILE": keyPath, "PLATFORM_SENSOR_CA_FILE": certPath,
				"PLATFORM_SENSOR_CONFIG_VERSION": "1", "PLATFORM_SENSOR_LOG_LEVEL": "INFO",
				"PLATFORM_SENSOR_STORAGE_WATERMARK_PERCENT": "80",
				"PLATFORM_SENSOR_SCHEDULER_MAX_WORKERS":     "8", "PLATFORM_SENSOR_SCHEDULER_MAX_QUEUE_DEPTH": "16",
				"PLATFORM_SENSOR_SCHEDULER_MAX_TRACKED_ASSIGNMENTS": "64", "PLATFORM_SENSOR_SCHEDULER_MAX_TRACKED_JOBS": "24",
				"PLATFORM_SENSOR_SCHEDULER_MAX_PAYLOAD_BYTES": "1048576", "PLATFORM_SENSOR_SCHEDULER_DEFAULT_EXECUTION_TIMEOUT": "10s",
				"PLATFORM_SENSOR_SCHEDULER_SHUTDOWN_GRACE": "1s", "PLATFORM_SENSOR_SCHEDULER_RETRY_MAX_ATTEMPTS": "3",
				"PLATFORM_SENSOR_SCHEDULER_RETRY_INITIAL_BACKOFF": "100ms", "PLATFORM_SENSOR_SCHEDULER_RETRY_MAX_BACKOFF": "2s", "PLATFORM_SENSOR_SCHEDULER_RETRY_JITTER": "0.2",
				"PLATFORM_SENSOR_SCHEDULER_POOLS_JSON":      `{"NETWORK_POLL":{"workers":4,"queueDepth":8},"STREAMING":{"workers":2,"queueDepth":4}}`,
				"PLATFORM_SENSOR_TRANSPORT_REQUEST_TIMEOUT": "1s", "PLATFORM_SENSOR_TRANSPORT_MAX_REQUEST_BYTES": "2097152",
				"PLATFORM_SENSOR_TRANSPORT_MAX_RESPONSE_BYTES": "2097152", "PLATFORM_SENSOR_UPLOAD_BATCH_RECORDS": "100", "PLATFORM_SENSOR_UPLOAD_BATCH_BYTES": "1048576",
				"SENSOR_POLL_INTERVAL_SECONDS": "1", "SENSOR_CONTROL_RECONCILE_INTERVAL_SECONDS": "5",
				"SENSOR_CAPABILITY_PACK_DIR":    filepath.Join(dir, "packs"),
				"SENSOR_AGENT_RECEIVER_ENABLED": "false",
			}
			if role == "INFRASTRUCTURE" {
				// Agent listener identity is separate from the Sensor→Core client.
				agentKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				template := x509.Certificate{
					SerialNumber: big.NewInt(2), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
					IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
					IsCA:        true, BasicConstraintsValid: true,
					KeyUsage:    x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
					ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
				}
				agentDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &agentKey.PublicKey, agentKey)
				if err != nil {
					t.Fatal(err)
				}
				agentKeyDER, err := x509.MarshalPKCS8PrivateKey(agentKey)
				if err != nil {
					t.Fatal(err)
				}
				agentCertPath, agentKeyPath := filepath.Join(dir, "agent.crt"), filepath.Join(dir, "agent.key")
				if err := os.WriteFile(agentCertPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: agentDER}), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(agentKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: agentKeyDER}), 0600); err != nil {
					t.Fatal(err)
				}
				values["SENSOR_AGENT_RECEIVER_ENABLED"] = "true"
				values["SENSOR_AGENT_RECEIVER_LISTEN_ADDR"] = agentAddr
				values["SENSOR_AGENT_SERVER_CERT_FILE"] = agentCertPath
				values["SENSOR_AGENT_SERVER_KEY_FILE"] = agentKeyPath
				values["SENSOR_AGENT_CLIENT_CA_FILE"] = agentCertPath
				values["SENSOR_AGENT_SPOOL_DIR"] = filepath.Join(dir, "agents")
				values["SENSOR_AGENT_DRAIN_REQUEST_TIMEOUT_SECONDS"] = "1"
				values["SENSOR_AGENT_RECEIVER_SHUTDOWN_GRACE_SECONDS"] = "1"
				values["SENSOR_AGENT_DRAIN_SHUTDOWN_GRACE_SECONDS"] = "1"
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestProductionEntrypoint$")
			for _, entry := range os.Environ() {
				name, _, _ := strings.Cut(entry, "=")
				if strings.HasPrefix(name, "PLATFORM_") || strings.HasPrefix(name, "SENSOR_") || name == "BUILD020_STARTUP_CHILD" {
					continue
				}
				cmd.Env = append(cmd.Env, entry)
			}
			for name, value := range values {
				cmd.Env = append(cmd.Env, name+"="+value)
			}
			logPath := filepath.Join(dir, "startup.log")
			logs, err := os.Create(logPath)
			if err != nil {
				t.Fatal(err)
			}
			defer logs.Close()
			cmd.Stdout, cmd.Stderr = logs, logs
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			stopped := false
			t.Cleanup(func() {
				if !stopped {
					_ = cmd.Process.Signal(syscall.SIGTERM)
					select {
					case <-done:
					case <-time.After(10 * time.Second):
						t.Error("production child did not stop")
					}
				}
			})
			client := &http.Client{Timeout: 200 * time.Millisecond}
			deadline := time.Now().Add(8 * time.Second)
			live := false
			for time.Now().Before(deadline) {
				select {
				case err := <-done:
					stopped = true
					data, _ := os.ReadFile(logPath)
					t.Fatalf("production startup exited: %v\n%s", err, data)
				default:
				}
				resp, err := client.Get("http://" + healthAddr + "/livez")
				if err == nil {
					resp.Body.Close()
					live = resp.StatusCode == http.StatusOK
				}
				if live && discoveryRequests.Load() > 0 && controlRequests.Load() > 0 && uploadRequests.Load() > 0 {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if !live {
				t.Fatal("production health listener did not start")
			}
			if discoveryRequests.Load() == 0 || controlRequests.Load() == 0 || uploadRequests.Load() == 0 {
				t.Fatalf("production paths missing: discovery=%d control=%d upload=%d", discoveryRequests.Load(), controlRequests.Load(), uploadRequests.Load())
			}
			if role == "INFRASTRUCTURE" {
				conn, err := net.DialTimeout("tcp", agentAddr, time.Second)
				if err != nil {
					t.Fatalf("Agent receiver not listening: %v", err)
				}
				conn.Close()
			}
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				stopped = true
				if err != nil {
					t.Fatalf("production shutdown: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("unbounded production shutdown")
			}
			data, _ := os.ReadFile(logPath)
			text := string(data)
			if !strings.Contains(text, "Platform Sensor shutdown complete") || strings.Contains(text, "SENSOR_RUNTIME_START_FAILED") {
				t.Fatalf("startup/shutdown failure:\n%s", text)
			}
			if role == "NETWORK" && !strings.Contains(text, "SENSOR_RUNTIME_STARTED") {
				t.Fatal("NETWORK runtime not activated")
			}
			if role == "INFRASTRUCTURE" && (!strings.Contains(text, "SENSOR_RUNTIME_ROLE_NOT_INTEGRATED") || strings.Contains(text, "SENSOR_RUNTIME_STARTED")) {
				t.Fatal("incorrect INFRASTRUCTURE runtime claim")
			}
			disk, err := spool.Open(spool.Config{Dir: filepath.Join(dir, "telemetry"), MaxRecordBytes: 1 << 20, Logger: slog.Default()})
			if err != nil {
				t.Fatal(err)
			}
			defer disk.Close()
			batch, err := disk.ReadBatch(10, 1<<20)
			if err != nil || len(batch.Records) == 0 || !strings.Contains(string(batch.Records[0].Payload), "sensor.heartbeat") {
				t.Fatalf("production heartbeat not durable: %+v %v", batch, err)
			}
			t.Logf("production main %s (%s): live health, durable heartbeat, bounded SIGTERM exit 0; Agent receiver enabled=%t", role, caps, role == "INFRASTRUCTURE")
		})
	}
}

func TestProductionBootstrapRoleCompatibility(t *testing.T) {
	for _, role := range []sensorruntime.Role{sensorruntime.RoleNetwork, sensorruntime.RoleInfrastructure, sensorruntime.RoleStreamingEvent, sensorruntime.RoleSynthetic, sensorruntime.RoleCloudAPI, sensorruntime.RoleCombined} {
		t.Run(string(role), func(t *testing.T) {
			cfg := testRuntime()
			cfg.Role = role
			cfg.EnabledCapabilities = sensorruntime.CapabilitiesForRole(role)
			if role == sensorruntime.RoleNetwork {
				cfg.EnabledCapabilities = []sensorruntime.Capability{sensorruntime.CapabilitySNMP, sensorruntime.CapabilityICMP}
			} else if role == sensorruntime.RoleInfrastructure {
				cfg.EnabledCapabilities = []sensorruntime.Capability{sensorruntime.CapabilityInfra, sensorruntime.CapabilityAgentIngress}
			}
			disk, err := spool.Open(spool.Config{Dir: t.TempDir(), MaxRecordBytes: 1 << 20, Logger: slog.Default()})
			if err != nil {
				t.Fatal(err)
			}
			defer disk.Close()
			sink, err := observation.NewDiskSink(disk, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			var logs bytes.Buffer
			b, err := startProductionBootstrap(context.Background(), cfg, sink, slog.New(slog.NewJSONHandler(&logs, nil)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := b.shutdown(ctx); err != nil {
					t.Error(err)
				}
			})
			if role == sensorruntime.RoleNetwork {
				if b.Runtime == nil || b.Runtime.Snapshot().State != sensorruntime.StateReady || b.SNMP == nil || b.ICMP == nil || !b.registry.HasCapability(sensorruntime.CapabilitySNMP) || !b.registry.HasCapability(sensorruntime.CapabilityICMP) {
					t.Fatal("NETWORK did not activate both real components")
				}
				if b.Runtime.Snapshot().Scheduler.Submitted != b.Scheduler.Snapshot().Submitted {
					t.Fatal("scheduler mismatch")
				}
			} else {
				if b.Runtime != nil || b.SNMP != nil || b.ICMP != nil {
					t.Fatal("unintegrated role activated runtime")
				}
				for _, cap := range cfg.EnabledCapabilities {
					if b.registry.HasCapability(cap) {
						t.Fatal("fake capability registration")
					}
				}
				if !strings.Contains(logs.String(), "SENSOR_RUNTIME_ROLE_NOT_INTEGRATED") || !strings.Contains(logs.String(), `"runtime_activated":false`) {
					t.Fatal("interim limitation not observable")
				}
			}
			if b.Scheduler.Snapshot().Submitted != 0 {
				t.Fatal("bootstrap admitted recurring work")
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := b.shutdown(ctx); err != nil {
				t.Fatal(err)
			}
			if b.Scheduler.Snapshot().State != scheduler.StateStopped || b.Scheduler.Snapshot().ActiveWorkers != 0 {
				t.Fatal("scheduler leaked")
			}
		})
	}
}

func TestProductionBootstrapUnsupportedNetworkFailsClosed(t *testing.T) {
	cfg := testRuntime()
	cfg.EnabledCapabilities = []sensorruntime.Capability{sensorruntime.CapabilitySNMP, sensorruntime.CapabilityICMP, sensorruntime.CapabilitySSH}
	disk, err := spool.Open(spool.Config{Dir: t.TempDir(), MaxRecordBytes: 1 << 20, Logger: slog.Default()})
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	sink, err := observation.NewDiskSink(disk, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	b, err := startProductionBootstrap(context.Background(), cfg, sink, slog.Default())
	if b != nil || err == nil || !strings.Contains(err.Error(), "SENSOR_RUNTIME_CAPABILITY_UNIMPLEMENTED") || !strings.Contains(err.Error(), "SSH") {
		t.Fatalf("bootstrap=%v err=%v", b, err)
	}
	if len(cfg.EnabledCapabilities) != 3 {
		t.Fatal("capability silently filtered")
	}
}

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

func TestSNMPBootstrapDefaultAndICMPOnlyCapabilities(t *testing.T) {
	data, err := os.ReadFile("../../../compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "PLATFORM_SENSOR_ENABLED_CAPABILITIES: SNMP,ICMP") {
		t.Fatal("default changed; reassess compatibility test")
	}
	for _, caps := range [][]sensorruntime.Capability{{sensorruntime.CapabilitySNMP, sensorruntime.CapabilityICMP}, {sensorruntime.CapabilityICMP}} {
		b, _, _ := bootstrapFixture(t, caps...)
		if err := b.Runtime.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		snap := b.Runtime.Snapshot()
		if snap.State != sensorruntime.StateReady || len(snap.EnabledCapabilities) != len(caps) || b.ICMP == nil || !b.registry.HasCapability(sensorruntime.CapabilityICMP) {
			t.Fatalf("runtime=%+v", snap)
		}
		if b.Scheduler.Snapshot().Submitted != 0 {
			t.Fatal("bootstrap admitted recurring work")
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
			b, disk, dir := bootstrapFixture(t, sensorruntime.CapabilitySNMP, sensorruntime.CapabilityICMP)
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

// Startup failure injection is a lifecycle test only; no fake SSH executor is
// registered in production or represented as a successful poll.
type failingBootstrapComponent struct{}

func (failingBootstrapComponent) Name() string { return "startup-failure-fixture" }
func (failingBootstrapComponent) Capability() sensorruntime.Capability {
	return sensorruntime.CapabilitySSH
}
func (failingBootstrapComponent) Start(context.Context) error {
	return fmt.Errorf("injected startup failure")
}
func (failingBootstrapComponent) Ready(context.Context) error    { return nil }
func (failingBootstrapComponent) Drain(context.Context) error    { return nil }
func (failingBootstrapComponent) Shutdown(context.Context) error { return nil }
func TestSNMPBootstrapComponentStartupFailure(t *testing.T) {
	b, _, _ := bootstrapFixture(t, sensorruntime.CapabilitySNMP, sensorruntime.CapabilitySSH)
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
