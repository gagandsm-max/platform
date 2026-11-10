package main

import (
	"context"
	"encoding/json"
	"log/slog"
	icmpintegration "platform/sensor/internal/integration/icmp"
	"platform/sensor/internal/observation"
	protocolicmp "platform/sensor/internal/protocol/icmp"
	sensorruntime "platform/sensor/internal/runtime"
	"platform/sensor/internal/spool"
	"strings"
	"testing"
	"time"
)

func TestBootstrapUnsupportedCapabilityFailClosed(t *testing.T) {
	b, _, _ := bootstrapFixture(t, sensorruntime.CapabilitySNMP, sensorruntime.CapabilityICMP, sensorruntime.CapabilitySSH)
	err := b.Runtime.Start(context.Background())
	if err == nil || !strings.Contains(err.Error(), "SENSOR_RUNTIME_CAPABILITY_UNIMPLEMENTED") || !strings.Contains(err.Error(), "SSH") || b.Runtime.Snapshot().State != sensorruntime.StateFailed {
		t.Fatalf("err=%v", err)
	}
	if len(b.Runtime.Snapshot().EnabledCapabilities) != 3 {
		t.Fatal("capability silently removed")
	}
}

func TestICMPBootstrapLiveLoopbackToDurableSpool(t *testing.T) {
	for _, caps := range [][]sensorruntime.Capability{{sensorruntime.CapabilityICMP}, {sensorruntime.CapabilitySNMP, sensorruntime.CapabilityICMP}} {
		t.Run(strings.Join(capStrings(caps), "+"), func(t *testing.T) {
			b, disk, dir := bootstrapFixture(t, caps...)
			if err := b.Runtime.Start(context.Background()); err != nil {
				t.Fatal(err)
			}
			job := bootstrapJob([]byte(`{"schemaVersion":1,"target":"127.0.0.1","ipVersion":4,"samples":3,"timeoutMillis":100,"intervalMillis":0,"payloadBytes":16,"executionTimeoutMillis":500}`))
			job.Kind = protocolicmp.JobKind
			if _, err := b.Runtime.Submit(job); err != nil {
				t.Fatal(err)
			}
			waitBootstrapJob(t, b, false)
			if s := b.ICMP.Snapshot(); s.Successful != 1 || s.Executions != 1 || s.Failed != 0 {
				t.Fatalf("ICMP=%+v", s)
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := b.Runtime.Shutdown(ctx); err != nil {
				t.Fatal(err)
			}
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
			env, err := observation.DecodeBounded(batch.Records[0].Payload, 1<<20)
			if err != nil {
				t.Fatal(err)
			}
			if env.RecordType != icmpintegration.RecordType || env.Protocol != "ICMP" || env.ProtocolVersion != "4" || env.SensorID != "sensor-001" || env.TenantID != "tenant-bootstrap" || env.EntityID != "entity-bootstrap" || env.Job.JobID != string(job.JobID) || env.Job.AssignmentVersion != 1 || env.Quality != "GOOD" || env.Status != "COLLECTED" {
				t.Fatalf("env=%+v", env)
			}
			var evidence icmpintegration.Evidence
			if err := json.Unmarshal(env.ProtocolPayload, &evidence); err != nil {
				t.Fatal(err)
			}
			if evidence.Received != 3 || evidence.PacketLossPct == nil || *evidence.PacketLossPct != 0 || evidence.AvgRTTMicros == nil || *evidence.AvgRTTMicros <= 0 || evidence.Source != "SENSOR_PING_SOCKET" || evidence.StartedAt.IsZero() || evidence.CompletedAt.Before(evidence.StartedAt) {
				t.Fatalf("evidence=%+v", evidence)
			}
			t.Logf("live IPv4 ping socket: received=%d loss=%g RTT=%d us; durable sequence=%d", evidence.Received, *evidence.PacketLossPct, *evidence.AvgRTTMicros, batch.Records[0].Sequence)
		})
	}
}
func capStrings(caps []sensorruntime.Capability) []string {
	out := make([]string, len(caps))
	for i, c := range caps {
		out[i] = string(c)
	}
	return out
}
