package snmpintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"platform/sensor/internal/observation"
	protocolsnmp "platform/sensor/internal/protocol/snmp"
	"platform/sensor/internal/runtime"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
)

type fakeBase struct {
	mu     sync.Mutex
	result scheduler.ExecutionResult
	err    error
	calls  int
	block  bool
}

func (f *fakeBase) Execute(ctx context.Context, job scheduler.Job) (scheduler.ExecutionResult, error) {
	f.mu.Lock()
	f.calls++
	block := f.block
	res := f.result
	err := f.err
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return scheduler.ExecutionResult{}, ctx.Err()
	}
	return res, err
}

type perJobBase struct{}

func (perJobBase) Execute(ctx context.Context, job scheduler.Job) (scheduler.ExecutionResult, error) {
	text := string(job.EntityID)
	obs := valueObs("1.3.6.1.2.1.1.5.0", "1.3.6.1.2.1.1.5.0", 4, "OctetString", protocolsnmp.Value{Kind: protocolsnmp.ValueOctets, Bytes: []byte(text)})
	result := protocolsnmp.CollectionResult{SchemaVersion: protocolsnmp.ResultSchemaVersion, Version: protocolsnmp.Version2c, Operation: protocolsnmp.OperationGet, CollectedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), Observations: []protocolsnmp.Observation{obs}}
	payload, err := json.Marshal(result)
	return scheduler.ExecutionResult{Payload: payload}, err
}

type fakeSink struct {
	mu     sync.Mutex
	max    int
	err    error
	writes []observation.Envelope
}

func (s *fakeSink) MaxPayloadBytes() int { return s.max }
func (s *fakeSink) Write(ctx context.Context, e observation.Envelope) (observation.Receipt, error) {
	if err := ctx.Err(); err != nil {
		return observation.Receipt{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return observation.Receipt{}, s.err
	}
	s.writes = append(s.writes, e)
	return observation.Receipt{Sequence: uint64(len(s.writes)), RecordID: "rec"}, nil
}
func (s *fakeSink) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.writes) }
func (s *fakeSink) last() observation.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes[len(s.writes)-1]
}

type runtimeScheduler struct{}

func (runtimeScheduler) Submit(scheduler.Job) (scheduler.Submission, error) {
	return scheduler.Submission{Status: scheduler.SubmissionAccepted}, nil
}
func (runtimeScheduler) Shutdown(context.Context) error { return nil }
func (runtimeScheduler) Snapshot() scheduler.Snapshot   { return scheduler.Snapshot{} }

func sampleResult(t *testing.T, observations ...protocolsnmp.Observation) []byte {
	t.Helper()
	result := protocolsnmp.CollectionResult{SchemaVersion: protocolsnmp.ResultSchemaVersion, Version: protocolsnmp.Version2c, Operation: protocolsnmp.OperationGet, CollectedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), Observations: observations}
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
func valueObs(oid, source string, ber uint8, name string, v protocolsnmp.Value) protocolsnmp.Observation {
	return protocolsnmp.Observation{OID: oid, SourceOID: source, BERType: ber, BERTypeName: name, CollectedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), Quality: protocolsnmp.QualityGood, Status: protocolsnmp.ObservationValue, Value: v}
}
func exceptionObs(oid, source string, code protocolsnmp.ErrorCode) protocolsnmp.Observation {
	return protocolsnmp.Observation{OID: oid, SourceOID: source, BERType: 0x80, BERTypeName: string(code), CollectedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), Quality: protocolsnmp.QualityDegraded, Status: protocolsnmp.ObservationException, ErrorCode: code, Value: protocolsnmp.Value{Kind: protocolsnmp.ValueException}}
}
func jobWithPayload(payload string) scheduler.Job {
	return scheduler.Job{JobID: "job-1", AssignmentKey: "assignment-1", TenantID: "tenant-1", EntityID: "entity-1", Kind: protocolsnmp.SchedulerJobKind, Class: scheduler.ClassNetworkPoll, AssignmentVersion: 7, LeaseExpiresAt: time.Now().Add(time.Minute), Attempt: 1, Payload: []byte(payload)}
}

func newTestExecutor(t *testing.T, base scheduler.Executor, sink observation.Sink) *Executor {
	t.Helper()
	e, err := NewExecutor(base, sink, Config{SensorID: "sensor-1", MaxSNMPResultBytes: 64 * 1024, MaxEnvelopeBytes: 128 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestRoleRegistrationNetworkAndCombined(t *testing.T) {
	for _, role := range []runtime.Role{runtime.RoleNetwork, runtime.RoleCombined} {
		t.Run(string(role), func(t *testing.T) {
			registry := runtime.NewExecutorRegistry()
			rt, err := runtime.New(runtime.Config{Role: role, EnabledCapabilities: []runtime.Capability{runtime.CapabilitySNMP}}, runtimeScheduler{}, registry)
			if err != nil { t.Fatal(err) }
			e := newTestExecutor(t, &fakeBase{result: scheduler.ExecutionResult{Payload: sampleResult(t)}}, &fakeSink{max: 128 * 1024})
			if err := Register(rt, registry, e); err != nil { t.Fatal(err) }
			if !registry.HasCapability(runtime.CapabilitySNMP) { t.Fatal("SNMP capability not registered") }
			if !e.Component().Snapshot().Registered { t.Fatal("component not registered") }
		})
	}
}

func TestRoleRegistrationRejectsProhibitedRoleAndDuplicate(t *testing.T) {
	for _, role := range []runtime.Role{runtime.RoleInfrastructure, runtime.RoleStreamingEvent, runtime.RoleSynthetic, runtime.RoleCloudAPI} {
		registry := runtime.NewExecutorRegistry()
		rt, err := runtime.New(runtime.Config{Role: role, EnabledCapabilities: nil}, runtimeScheduler{}, registry)
		if err != nil { t.Fatal(err) }
		e := newTestExecutor(t, &fakeBase{result: scheduler.ExecutionResult{Payload: sampleResult(t)}}, &fakeSink{max: 128 * 1024})
		if err := Register(rt, registry, e); err == nil { t.Fatalf("expected prohibited role %s rejection", role) }
	}
	registry := runtime.NewExecutorRegistry()
	rt, err := runtime.New(runtime.Config{Role: runtime.RoleNetwork, EnabledCapabilities: []runtime.Capability{runtime.CapabilitySNMP}}, runtimeScheduler{}, registry)
	if err != nil { t.Fatal(err) }
	e := newTestExecutor(t, &fakeBase{result: scheduler.ExecutionResult{Payload: sampleResult(t)}}, &fakeSink{max: 128 * 1024})
	if err := Register(rt, registry, e); err != nil { t.Fatal(err) }
	other := newTestExecutor(t, &fakeBase{result: scheduler.ExecutionResult{Payload: sampleResult(t)}}, &fakeSink{max: 128 * 1024})
	if err := Register(rt, registry, other); err == nil { t.Fatal("expected duplicate executor registration rejection") }
}

func TestMissingSNMPImplementationFailsRuntimeStart(t *testing.T) {
	registry := runtime.NewExecutorRegistry()
	rt, err := runtime.New(runtime.Config{Role: runtime.RoleNetwork, EnabledCapabilities: []runtime.Capability{runtime.CapabilitySNMP}}, runtimeScheduler{}, registry)
	if err != nil { t.Fatal(err) }
	if err := rt.Start(context.Background()); err == nil { t.Fatal("expected missing SNMP implementation failure") }
}

func TestSuccessfulSNMPResultDurablySpoolsAndPreservesFields(t *testing.T) {
	max := uint64(math.MaxUint64)
	txt := "router"
	obs1 := valueObs("1.3.6.1.2.1.1.5.0", "1.3.6.1.2.1.1.5.0", 4, "OctetString", protocolsnmp.Value{Kind: protocolsnmp.ValueOctets, Bytes: []byte("router")})
	obs2 := valueObs("1.3.6.1.2.1.31.1.1.1.6.1", "1.3.6.1.2.1.31.1.1.1.6", 70, "Counter64", protocolsnmp.Value{Kind: protocolsnmp.ValueCounter64, Counter64: &max, Text: &txt})
	dir := t.TempDir()
	disk, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 256 * 1024})
	if err != nil { t.Fatal(err) }
	sink, err := observation.NewDiskSink(disk, 256*1024)
	if err != nil { t.Fatal(err) }
	e := newTestExecutor(t, &fakeBase{result: scheduler.ExecutionResult{Payload: sampleResult(t, obs1, obs2)}}, sink)
	res, err := e.Execute(context.Background(), jobWithPayload(`{"community":"TOPSECRET","authSecret":"AUTH-SECRET","privacySecret":"PRIV-SECRET","opaque":"OPAQUE-PAYLOAD"}`))
	if err != nil { t.Fatal(err) }
	if len(res.Payload) == 0 { t.Fatal("expected durable receipt") }
	batch, err := disk.ReadBatch(10, 256*1024)
	if err != nil { t.Fatal(err) }
	if len(batch.Records) != 1 { t.Fatalf("records=%d", len(batch.Records)) }
	rec := batch.Records[0]
	for _, secret := range []string{"TOPSECRET", "AUTH-SECRET", "PRIV-SECRET", "OPAQUE-PAYLOAD"} {
		if bytes.Contains(rec.Payload, []byte(secret)) { t.Fatalf("secret-bearing scheduler payload leaked into durable record: %s", secret) }
	}
	if rec.Class != spool.ClassMetric || rec.Type != durableRecordType { t.Fatalf("unexpected spool classification: %s/%s", rec.Class, rec.Type) }
	env, err := observation.DecodeBounded(rec.Payload, 256*1024)
	if err != nil { t.Fatal(err) }
	if env.Protocol != "SNMP" || env.Operation != "GET" || env.TenantID != "tenant-1" || env.EntityID != "entity-1" { t.Fatalf("bad envelope: %+v", env) }
	var wire resultWire
	if err := json.Unmarshal(env.ProtocolPayload, &wire); err != nil { t.Fatal(err) }
	if len(wire.Observations) != 2 { t.Fatalf("observations=%d", len(wire.Observations)) }
	if wire.Observations[1].OID != "1.3.6.1.2.1.31.1.1.1.6.1" || wire.Observations[1].SourceOID != "1.3.6.1.2.1.31.1.1.1.6" || wire.Observations[1].BERType != 70 { t.Fatal("OID/source/BER not preserved") }
	if wire.Observations[1].Value.Counter64 == nil || *wire.Observations[1].Value.Counter64 != "18446744073709551615" { t.Fatalf("Counter64 lost: %+v", wire.Observations[1].Value) }
	if err := disk.Close(); err != nil { t.Fatal(err) }
	disk, err = spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 256 * 1024})
	if err != nil { t.Fatal(err) }
	batch, err = disk.ReadBatch(10, 256*1024)
	if err != nil { t.Fatal(err) }
	if !bytes.Contains(batch.Records[0].Payload, []byte("18446744073709551615")) { t.Fatal("Counter64 decimal string not retained after reopen") }
}

func TestSNMPExceptionsPersistIntentionally(t *testing.T) {
	for _, code := range []protocolsnmp.ErrorCode{protocolsnmp.CodeNoSuchObject, protocolsnmp.CodeNoSuchInstance, protocolsnmp.CodeEndOfMibView} {
		t.Run(string(code), func(t *testing.T) {
			sink := &fakeSink{max: 128 * 1024}
			e := newTestExecutor(t, &fakeBase{result: scheduler.ExecutionResult{Payload: sampleResult(t, exceptionObs("1.3.6.1.2.1.1.99.0", "1.3.6.1.2.1.1.99.0", code))}}, sink)
			if _, err := e.Execute(context.Background(), jobWithPayload(`{}`)); err != nil { t.Fatal(err) }
			env := sink.last()
			if env.Status != "COLLECTED_WITH_EXCEPTIONS" { t.Fatalf("status=%s", env.Status) }
			if !bytes.Contains(env.ProtocolPayload, []byte(code)) { t.Fatalf("exception %s not preserved", code) }
		})
	}
}

func TestProtocolFailureDoesNotFabricateObservationAndClassificationSurvives(t *testing.T) {
	cases := []struct { name string; err error; want scheduler.ErrorCode }{
		{"timeout", scheduler.Transient(&protocolsnmp.Error{Code: protocolsnmp.CodeTimeout, Message: "timeout"}), scheduler.CodeTransient},
		{"auth", scheduler.Permanent(&protocolsnmp.Error{Code: protocolsnmp.CodeAuthenticationFailed, Message: "auth failed"}), scheduler.CodePermanent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := &fakeSink{max: 128 * 1024}
			e := newTestExecutor(t, &fakeBase{err: tc.err}, sink)
			_, err := e.Execute(context.Background(), jobWithPayload(`{"community":"SECRET"}`))
			if err == nil || !scheduler.IsCode(err, tc.want) { t.Fatalf("classification lost: %v", err) }
			if sink.count() != 0 { t.Fatal("failure fabricated durable observation") }
		})
	}
}

func TestSpoolFailuresPreventSchedulerSuccess(t *testing.T) {
	cases := []struct { name string; spoolCode spool.ErrorCode; want ErrorCode }{
		{"protection", spool.CodeProtection, CodeStorageProtection},
		{"corruption", spool.CodeCorruption, CodeStorageCorruption},
		{"io", spool.CodeIO, CodeStorageUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := &fakeSink{max: 128 * 1024, err: &spool.Error{Code: tc.spoolCode}}
			e := newTestExecutor(t, &fakeBase{result: scheduler.ExecutionResult{Payload: sampleResult(t, valueObs("1.3", "1.3", 2, "Integer", protocolsnmp.Value{Kind: protocolsnmp.ValueInteger}))}}, sink)
			_, err := e.Execute(context.Background(), jobWithPayload(`{}`))
			if err == nil || !scheduler.IsCode(err, scheduler.CodePermanent) { t.Fatalf("expected permanent storage failure: %v", err) }
			if CodeOf(err) != tc.want { t.Fatalf("code=%s want=%s", CodeOf(err), tc.want) }
			if e.Component().Snapshot().SpoolWriteFailures != 1 { t.Fatal("spool failure metric not updated") }
		})
	}
}

func TestCancellationPropagatesWithoutSpoolWrite(t *testing.T) {
	sink := &fakeSink{max: 128 * 1024}
	base := &fakeBase{block: true}
	e := newTestExecutor(t, base, sink)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := e.Execute(ctx, jobWithPayload(`{}`)); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) { t.Fatalf("err=%v", err) }
	case <-time.After(time.Second):
		t.Fatal("cancellation did not propagate")
	}
	if sink.count() != 0 { t.Fatal("cancelled job wrote spool record") }
}

func TestEnvelopeBoundAndSecretBoundary(t *testing.T) {
	sink := &fakeSink{max: 300}
	e, err := NewExecutor(&fakeBase{result: scheduler.ExecutionResult{Payload: sampleResult(t, valueObs("1.3.6.1", "1.3.6.1", 4, "OctetString", protocolsnmp.Value{Kind: protocolsnmp.ValueOctets, Bytes: bytes.Repeat([]byte("x"), 512)}))}}, sink, Config{SensorID: "sensor-1", MaxSNMPResultBytes: 64 * 1024, MaxEnvelopeBytes: 300})
	if err != nil { t.Fatal(err) }
	_, err = e.Execute(context.Background(), jobWithPayload(`{"community":"COMMUNITY-SECRET","authSecret":"AUTH-SECRET","privacySecret":"PRIV-SECRET","opaque":"OPAQUE-PAYLOAD"}`))
	if err == nil || !scheduler.IsCode(err, scheduler.CodePermanent) { t.Fatalf("expected resource limit: %v", err) }
	if sink.count() != 0 { t.Fatal("oversized envelope reached sink") }
	sink = &fakeSink{max: 128 * 1024}
	e = newTestExecutor(t, &fakeBase{result: scheduler.ExecutionResult{Payload: sampleResult(t, valueObs("1.3.6.1", "1.3.6.1", 2, "Integer", protocolsnmp.Value{Kind: protocolsnmp.ValueInteger}))}}, sink)
	_, err = e.Execute(context.Background(), jobWithPayload(`{"community":"COMMUNITY-SECRET","authSecret":"AUTH-SECRET","privacySecret":"PRIV-SECRET","opaque":"OPAQUE-PAYLOAD"}`))
	if err != nil { t.Fatal(err) }
	encoded, err := json.Marshal(sink.last())
	if err != nil { t.Fatal(err) }
	for _, secret := range []string{"COMMUNITY-SECRET", "AUTH-SECRET", "PRIV-SECRET", "OPAQUE-PAYLOAD"} {
		if bytes.Contains(encoded, []byte(secret)) { t.Fatalf("secret leaked: %s", secret) }
	}
}

func TestComponentMetricsLowCardinalityAndRecovery(t *testing.T) {
	sink := &fakeSink{max: 128 * 1024, err: &spool.Error{Code: spool.CodeProtection}}
	base := &fakeBase{result: scheduler.ExecutionResult{Payload: sampleResult(t, valueObs("1.3", "1.3", 2, "Integer", protocolsnmp.Value{Kind: protocolsnmp.ValueInteger}))}}
	e := newTestExecutor(t, base, sink)
	e.Component().now = func() time.Time { return time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC) }
	_, _ = e.Execute(context.Background(), jobWithPayload(`{}`))
	snap := e.Component().Snapshot()
	if snap.Executions != 1 || snap.Failed != 1 || !snap.Degraded || snap.LastErrorCode == "" { t.Fatalf("bad failure snapshot: %+v", snap) }
	raw, _ := json.Marshal(snap)
	for _, forbidden := range []string{"tenant-1", "entity-1", "1.3", "community", "target"} {
		if strings.Contains(string(raw), forbidden) { t.Fatalf("high-cardinality/secret metric leaked: %s", forbidden) }
	}
	sink.err = nil
	if _, err := e.Execute(context.Background(), jobWithPayload(`{}`)); err != nil { t.Fatal(err) }
	snap = e.Component().Snapshot()
	if snap.Successful != 1 || snap.Degraded || snap.LastErrorCode != "" { t.Fatalf("health did not recover: %+v", snap) }
}

func TestConcurrentJobsDoNotCrossContaminateAndBatchIDIsStable(t *testing.T) {
	sink := &fakeSink{max: 128 * 1024}
	e := newTestExecutor(t, perJobBase{}, sink)
	jobs := make([]scheduler.Job, 20)
	for i := range jobs {
		jobs[i] = jobWithPayload(`{"community":"secret-` + string(rune('a'+i%20)) + `"}`)
		jobs[i].JobID = scheduler.JobID("job-" + time.Unix(int64(i+1), 0).UTC().Format("150405"))
		jobs[i].EntityID = scheduler.EntityID("entity-" + time.Unix(int64(i+1), 0).UTC().Format("150405"))
	}
	var wg sync.WaitGroup
	for _, job := range jobs {
		job := job
		wg.Add(1)
		go func() { defer wg.Done(); if _, err := e.Execute(context.Background(), job); err != nil { t.Errorf("execute: %v", err) } }()
	}
	wg.Wait()
	if sink.count() != len(jobs) { t.Fatalf("writes=%d", sink.count()) }
	seen := map[string]bool{}
	for _, env := range sink.writes {
		if seen[env.BatchID] { t.Fatalf("unexpected batch ID collision %s", env.BatchID) }
		seen[env.BatchID] = true
		var wire resultWire
		if err := json.Unmarshal(env.ProtocolPayload, &wire); err != nil { t.Fatal(err) }
		if len(wire.Observations) != 1 || !bytes.Equal(wire.Observations[0].Value.Bytes, []byte(env.EntityID)) { t.Fatalf("cross-job result contamination for entity %s", env.EntityID) }
	}
	job := jobWithPayload(`{"community":"one"}`)
	id1 := deterministicBatchID(job)
	job.Attempt = 99
	job.Payload = []byte(`{"community":"two"}`)
	id2 := deterministicBatchID(job)
	if id1 != id2 { t.Fatal("logical retry identity changed batch ID") }
}

func TestDuplicateExecutionUsesStableReplayIdentity(t *testing.T) {
	sink := &fakeSink{max: 128 * 1024}
	base := &fakeBase{result: scheduler.ExecutionResult{Payload: sampleResult(t, valueObs("1.3", "1.3", 2, "Integer", protocolsnmp.Value{Kind: protocolsnmp.ValueInteger}))}}
	e := newTestExecutor(t, base, sink)
	job := jobWithPayload(`{"community":"secret-one"}`)
	if _, err := e.Execute(context.Background(), job); err != nil { t.Fatal(err) }
	job.Attempt = 2
	job.Payload = []byte(`{"community":"secret-two"}`)
	if _, err := e.Execute(context.Background(), job); err != nil { t.Fatal(err) }
	if sink.count() != 2 { t.Fatalf("writes=%d", sink.count()) }
	if sink.writes[0].BatchID != sink.writes[1].BatchID { t.Fatal("same logical scheduler job produced different replay/dedup identities") }
}

func TestExecutorInterfaceCompatibility(t *testing.T) {
	var _ scheduler.Executor = (*Executor)(nil)
	typ := reflect.TypeOf((*scheduler.Executor)(nil)).Elem()
	if !reflect.TypeOf((*Executor)(nil)).Implements(typ) { t.Fatal("Executor does not satisfy scheduler.Executor") }
}

func TestWrongRoutingRejected(t *testing.T) {
	sink := &fakeSink{max: 128 * 1024}
	e := newTestExecutor(t, &fakeBase{result: scheduler.ExecutionResult{Payload: sampleResult(t)}}, sink)
	job := jobWithPayload(`{}`)
	job.Class = scheduler.ClassSynthetic
	if _, err := e.Execute(context.Background(), job); err == nil || !scheduler.IsCode(err, scheduler.CodePermanent) { t.Fatalf("wrong class accepted: %v", err) }
	job = jobWithPayload(`{}`)
	job.Kind = "OTHER"
	if _, err := e.Execute(context.Background(), job); err == nil || !scheduler.IsCode(err, scheduler.CodePermanent) { t.Fatalf("wrong kind accepted: %v", err) }
}

func TestResultDecoderRejectsUnknownTrailingAndInvalidEnums(t *testing.T) {
	base := sampleResult(t)
	var m map[string]any
	if err := json.Unmarshal(base, &m); err != nil { t.Fatal(err) }
	m["credentials"] = "must-not-be-accepted"
	unknown, _ := json.Marshal(m)
	if _, err := decodeResult(unknown, 64*1024); err == nil { t.Fatal("unknown result field accepted") }
	trailing := append(append([]byte(nil), base...), []byte(` {}`)...)
	if _, err := decodeResult(trailing, 64*1024); err == nil { t.Fatal("trailing result document accepted") }
	m = map[string]any{"schemaVersion": 1, "version": "bogus", "operation": "GET", "collectedAt": "2026-09-07T00:00:00Z", "observations": []any{}}
	bad, _ := json.Marshal(m)
	if _, err := decodeResult(bad, 64*1024); err == nil { t.Fatal("invalid protocol enum accepted") }
}

func TestMain(m *testing.M) { os.Exit(m.Run()) }
