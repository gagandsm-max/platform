package icmpintegration

import (
	"context"
	"encoding/json"
	"log/slog"
	"platform/sensor/internal/observation"
	protocolicmp "platform/sensor/internal/protocol/icmp"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
	"strings"
	"testing"
	"time"
)

type fixtureExecutor struct {
	result  protocolicmp.Result
	err     error
	payload []byte
}

func (f fixtureExecutor) Execute(context.Context, scheduler.Job) (scheduler.ExecutionResult, error) {
	if f.err != nil {
		return scheduler.ExecutionResult{}, f.err
	}
	if f.payload != nil {
		return scheduler.ExecutionResult{Payload: f.payload}, nil
	}
	p, _ := json.Marshal(f.result)
	return scheduler.ExecutionResult{Payload: p}, nil
}
func fixtureResult() protocolicmp.Result {
	now := time.Now().UTC()
	return protocolicmp.Result{SchemaVersion: 1, Target: "127.0.0.1", IPVersion: 4, StartedAt: now, CompletedAt: now.Add(time.Millisecond), Sent: 2, Received: 1, PacketLossPct: 50, MinRTTMicros: 1000, AvgRTTMicros: 1000, MaxRTTMicros: 1000, Samples: []protocolicmp.Sample{{Sequence: 1, Success: true, RTTMicros: 1000}, {Sequence: 2, ErrorCode: protocolicmp.CodeTimeout}}}
}
func fixtureJob() scheduler.Job {
	return scheduler.Job{Kind: protocolicmp.JobKind, Class: scheduler.ClassNetworkPoll, JobID: "job", AssignmentKey: "assignment", AssignmentVersion: 1, TenantID: "tenant", EntityID: "entity", Payload: []byte(`{"schemaVersion":1,"target":"127.0.0.1","ipVersion":4}`)}
}
func integrationFixture(t *testing.T, base scheduler.Executor) (*Executor, *spool.DiskSpool) {
	t.Helper()
	disk, err := spool.Open(spool.Config{Dir: t.TempDir(), MaxRecordBytes: 8192, Logger: slog.Default()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { disk.Close() })
	sink, err := observation.NewDiskSink(disk, 8192)
	if err != nil {
		t.Fatal(err)
	}
	e, err := NewExecutor(base, sink, Config{SensorID: "sensor", MaxResultBytes: 4096, MaxEnvelopeBytes: 8192, MaxSamples: 8})
	if err != nil {
		t.Fatal(err)
	}
	return e, disk
}
func readEvidence(t *testing.T, disk *spool.DiskSpool) (observation.Envelope, Evidence) {
	t.Helper()
	batch, err := disk.ReadBatch(10, 8192)
	if err != nil || len(batch.Records) != 1 {
		t.Fatalf("batch=%+v err=%v", batch, err)
	}
	env, err := observation.DecodeBounded(batch.Records[0].Payload, 8192)
	if err != nil {
		t.Fatal(err)
	}
	var evidence Evidence
	if err := json.Unmarshal(env.ProtocolPayload, &evidence); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(batch.Records[0].Payload), "DEVICE_DOWN") {
		t.Fatal("invented device-down")
	}
	return env, evidence
}
func TestFixtureLossAndUnknownObservations(t *testing.T) {
	for _, tc := range []string{"partial_loss", "all_loss", "unreachable", "permission", "protocol", "transport"} {
		t.Run(tc, func(t *testing.T) {
			base := fixtureExecutor{result: fixtureResult()}
			switch tc {
			case "all_loss":
				base.result.Received = 0
				base.result.PacketLossPct = 100
				base.result.MinRTTMicros = 0
				base.result.AvgRTTMicros = 0
				base.result.MaxRTTMicros = 0
				base.result.Samples[0] = protocolicmp.Sample{Sequence: 1, ErrorCode: protocolicmp.CodeTimeout}
			case "unreachable":
				base.result.Samples[1].ErrorCode = protocolicmp.CodeUnreachable
			case "permission":
				base.err = scheduler.Permanent(&protocolicmp.Error{Code: protocolicmp.CodePermission})
			case "protocol":
				base.err = scheduler.Permanent(&protocolicmp.Error{Code: protocolicmp.CodeProtocol})
			case "transport":
				base.err = scheduler.Transient(&protocolicmp.Error{Code: protocolicmp.CodeTransport})
			}
			e, disk := integrationFixture(t, base)
			e.component.markRegistered()
			_, err := e.Execute(context.Background(), fixtureJob())
			if (err != nil) != (base.err != nil) {
				t.Fatalf("err=%v", err)
			}
			env, obs := readEvidence(t, disk)
			snap := e.Component().Snapshot()
			if tc == "permission" || tc == "protocol" || tc == "transport" {
				if env.Status != "COLLECTION_FAILED" || env.Quality != "UNKNOWN" || obs.ErrorCode == "" || obs.PacketLossPct != nil || obs.AvgRTTMicros != nil || len(obs.Samples) != 0 || snap.Successful != 0 {
					t.Fatalf("env=%+v obs=%+v snap=%+v", env, obs, snap)
				}
				wantDegraded := tc == "permission" || tc == "protocol"
				if snap.Degraded != wantDegraded || snap.SpoolWriteFailures != 0 {
					t.Fatalf("collector health snap=%+v wantDegraded=%v", snap, wantDegraded)
				}
				if wantDegraded {
					if err := e.Component().Ready(context.Background()); err == nil || !strings.Contains(err.Error(), "ICMP_COMPONENT_DEGRADED") {
						t.Fatalf("ready err=%v", err)
					}
					if snap.LastErrorCode != obs.ErrorCode {
						t.Fatalf("lastError=%q obs=%q", snap.LastErrorCode, obs.ErrorCode)
					}
				} else if err := e.Component().Ready(context.Background()); err != nil {
					t.Fatalf("transient transport must not degrade Ready: %v", err)
				}
			} else if tc == "unreachable" {
				if env.Quality != "UNKNOWN" || env.Status != "PARTIAL" || obs.PacketLossPct != nil || snap.Degraded {
					t.Fatalf("%+v %+v snap=%+v", env, obs, snap)
				}
			} else if env.Quality != "PARTIAL" || obs.PacketLossPct == nil || *obs.PacketLossPct != base.result.PacketLossPct || (tc == "all_loss" && obs.AvgRTTMicros != nil) || snap.Degraded {
				t.Fatalf("%+v %+v snap=%+v", env, obs, snap)
			}
		})
	}
}

func TestCollectorUnusableDegradesAndRecovers(t *testing.T) {
	e, _ := integrationFixture(t, fixtureExecutor{
		err: scheduler.Permanent(&protocolicmp.Error{Code: protocolicmp.CodePermission}),
	})
	e.component.markRegistered()
	if _, err := e.Execute(context.Background(), fixtureJob()); err == nil {
		t.Fatal("permission failure expected")
	}
	if snap := e.Component().Snapshot(); !snap.Degraded || snap.LastErrorCode != string(protocolicmp.CodePermission) || snap.SpoolWriteFailures != 0 {
		t.Fatalf("permission snap=%+v", snap)
	}
	if err := e.Component().Ready(context.Background()); err == nil {
		t.Fatal("permission must fail Ready")
	}
	e.base = fixtureExecutor{result: fixtureResult()}
	if _, err := e.Execute(context.Background(), fixtureJob()); err != nil {
		t.Fatal(err)
	}
	snap := e.Component().Snapshot()
	if snap.Degraded || snap.Successful != 1 || snap.LastErrorCode != "" {
		t.Fatalf("recovery snap=%+v", snap)
	}
	if err := e.Component().Ready(context.Background()); err != nil {
		t.Fatalf("ready after recovery: %v", err)
	}
}
func TestMalformedResultsFailClosed(t *testing.T) {
	for _, tc := range []string{"schema", "count", "loss", "sequence", "time", "target", "unknown", "trailing", "oversize"} {
		t.Run(tc, func(t *testing.T) {
			base := fixtureExecutor{result: fixtureResult()}
			switch tc {
			case "schema":
				base.result.SchemaVersion = 2
			case "count":
				base.result.Received = 2
			case "loss":
				base.result.PacketLossPct = 100
			case "sequence":
				base.result.Samples[0].Sequence = 9
			case "time":
				base.result.CompletedAt = time.Time{}
			case "target":
				base.result.IPVersion = 6
			case "unknown":
				base.payload = []byte(`{"unknown":1}`)
			case "trailing":
				p, _ := json.Marshal(base.result)
				base.payload = append(p, []byte(` {}`)...)
			case "oversize":
				base.payload = []byte(strings.Repeat("x", 4097))
			}
			e, disk := integrationFixture(t, base)
			if _, err := e.Execute(context.Background(), fixtureJob()); err == nil {
				t.Fatal("invalid result accepted")
			}
			batch, _ := disk.ReadBatch(10, 8192)
			if len(batch.Records) != 0 {
				t.Fatal("invalid result spooled")
			}
		})
	}
}
func TestSinkFailureAndCancellation(t *testing.T) {
	e, disk := integrationFixture(t, fixtureExecutor{result: fixtureResult()})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.Execute(ctx, fixtureJob()); err != context.Canceled {
		t.Fatal(err)
	}
	disk.Close()
	if _, err := e.Execute(context.Background(), fixtureJob()); err == nil {
		t.Fatal("closed spool accepted")
	}
	snap := e.Component().Snapshot()
	if !snap.Degraded || snap.SpoolWriteFailures != 1 || snap.Successful != 0 {
		t.Fatalf("%+v", snap)
	}
}
func TestInvalidRouteAndConfiguration(t *testing.T) {
	e, _ := integrationFixture(t, fixtureExecutor{result: fixtureResult()})
	job := fixtureJob()
	job.Kind = "WRONG"
	if _, err := e.Execute(context.Background(), job); err == nil {
		t.Fatal("wrong route accepted")
	}
	if _, err := NewExecutor(nil, e.sink, e.cfg); err == nil {
		t.Fatal("nil executor accepted")
	}
	if _, err := NewExecutor(e.base, nil, e.cfg); err == nil {
		t.Fatal("nil sink accepted")
	}
	cfg := e.cfg
	cfg.SensorID = ""
	if _, err := NewExecutor(e.base, e.sink, cfg); err == nil {
		t.Fatal("missing identity accepted")
	}
	if e.Component().Start(context.Background()) == nil {
		t.Fatal("unregistered component started")
	}
	if batchID(fixtureJob()) == batchID(scheduler.Job{}) {
		t.Fatal("batch identity lacks scope")
	}
}
