package control_test

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"platform/sensor/internal/config"
	"platform/sensor/internal/control"
	"platform/sensor/internal/coreclient"
)

type fakeControlClient struct {
	mu       sync.Mutex
	desired  *coreclient.ControlDesiredState
	fetchErr error
	acks     []coreclient.ControlAppliedAckRequest
	ackErr   error
	ackFn    func(coreclient.ControlAppliedAckRequest) error
}

func (f *fakeControlClient) FetchControlDesiredState(context.Context) (*coreclient.ControlDesiredState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	if f.desired == nil {
		return nil, nil
	}
	cp := *f.desired
	return &cp, nil
}

func (f *fakeControlClient) AckControlAppliedState(_ context.Context, req coreclient.ControlAppliedAckRequest) (coreclient.ControlAppliedAckResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acks = append(f.acks, req)
	if f.ackFn != nil {
		if err := f.ackFn(req); err != nil {
			return coreclient.ControlAppliedAckResponse{}, err
		}
	}
	if f.ackErr != nil {
		return coreclient.ControlAppliedAckResponse{}, f.ackErr
	}
	return coreclient.ControlAppliedAckResponse{
		Status: "ACCEPTED", SchemaVersion: 1, Generation: req.Generation, Checksum: req.Checksum, Outcome: req.Outcome,
	}, nil
}

func desiredState(t *testing.T, generation uint64, cfg control.DesiredConfiguration) *coreclient.ControlDesiredState {
	t.Helper()
	checksum, err := control.ComputeDesiredChecksum(cfg)
	if err != nil {
		t.Fatal(err)
	}
	pools := map[string]coreclient.ControlDesiredPool{}
	for name, pool := range cfg.Scheduler.Pools {
		pools[name] = coreclient.ControlDesiredPool{Workers: pool.Workers, QueueDepth: pool.QueueDepth}
	}
	return &coreclient.ControlDesiredState{
		SchemaVersion:     1,
		DesiredGeneration: generation,
		DesiredChecksum:   checksum,
		Configuration: coreclient.ControlDesiredConfiguration{
			LogLevel: cfg.LogLevel,
			Scheduler: coreclient.ControlDesiredScheduler{
				MaxWorkers: cfg.Scheduler.MaxWorkers, MaxQueueDepth: cfg.Scheduler.MaxQueueDepth,
				MaxTrackedAssignments: cfg.Scheduler.MaxTrackedAssignments, MaxTrackedJobs: cfg.Scheduler.MaxTrackedJobs,
				MaxPayloadBytes: cfg.Scheduler.MaxPayloadBytes, DefaultExecutionTimeoutMs: cfg.Scheduler.DefaultExecutionTimeoutMs,
				ShutdownGracePeriodMs: cfg.Scheduler.ShutdownGracePeriodMs, RetryMaxAttempts: cfg.Scheduler.RetryMaxAttempts,
				RetryInitialBackoffMs: cfg.Scheduler.RetryInitialBackoffMs, RetryMaxBackoffMs: cfg.Scheduler.RetryMaxBackoffMs,
				RetryJitterBasisPoints: cfg.Scheduler.RetryJitterBasisPoints, Pools: pools,
			},
			Transport: coreclient.ControlDesiredTransport{
				RequestTimeoutMs: cfg.Transport.RequestTimeoutMs, MaxRequestBytes: cfg.Transport.MaxRequestBytes,
				MaxResponseBytes: cfg.Transport.MaxResponseBytes, UploadBatchRecords: cfg.Transport.UploadBatchRecords,
				UploadBatchBytes: cfg.Transport.UploadBatchBytes,
			},
		},
		AppliedGeneration:  0,
		AppliedChecksum:    nil,
		ObservedGeneration: 0,
		ObservedChecksum:   nil,
		ObservedOutcome:    nil,
	}
}

func newTestReconciler(t *testing.T, client *fakeControlClient, apply control.Applier) (*control.Reconciler, *control.Store, *control.Metrics) {
	t.Helper()
	store, err := control.NewStore(filepath.Join(t.TempDir(), "applied.json"))
	if err != nil {
		t.Fatal(err)
	}
	metrics := &control.Metrics{}
	rec, err := control.NewReconciler(client, store, validBaseline(t), apply, slog.Default(), metrics)
	if err != nil {
		t.Fatal(err)
	}
	return rec, store, metrics
}

func TestReconcileNoDesiredNoOp(t *testing.T) {
	t.Parallel()
	client := &fakeControlClient{}
	var applies int32
	rec, store, metrics := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		atomic.AddInt32(&applies, 1)
		return nil
	}))
	res, err := rec.RunOnce(context.Background())
	if err != nil || res.Action != "NO_DESIRED" || applies != 0 || len(client.acks) != 0 {
		t.Fatalf("res=%+v err=%v applies=%d acks=%d", res, err, applies, len(client.acks))
	}
	snap, _ := store.Load()
	if snap != nil {
		t.Fatal("must not mutate store")
	}
	if metrics.Snapshot().NoDesiredCount != 1 {
		t.Fatalf("metrics=%+v", metrics.Snapshot())
	}
}

func TestReconcileFirstApplyAndCoalesce(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	client := &fakeControlClient{desired: desiredState(t, 5, cfg)}
	var applies int32
	var lastGen uint64
	rec, store, _ := newTestReconciler(t, client, control.FuncApplier(func(_ context.Context, generation uint64, _ string, _ config.RuntimeConfig) error {
		atomic.AddInt32(&applies, 1)
		lastGen = generation
		return nil
	}))
	res, err := rec.RunOnce(context.Background())
	if err != nil || res.Action != "APPLIED" || res.Generation != 5 || applies != 1 {
		t.Fatalf("first apply res=%+v err=%v applies=%d", res, err, applies)
	}
	snap, err := store.Load()
	if err != nil || snap == nil || snap.Generation != 5 {
		t.Fatalf("snapshot=%#v err=%v", snap, err)
	}
	if len(client.acks) != 1 || client.acks[0].Outcome != "APPLIED" {
		t.Fatalf("acks=%+v", client.acks)
	}
	// Coalesce N -> N+k
	client.desired = desiredState(t, 9, cfg)
	res, err = rec.RunOnce(context.Background())
	if err != nil || res.Action != "APPLIED" || res.Generation != 9 || applies != 2 || lastGen != 9 {
		t.Fatalf("coalesce res=%+v err=%v applies=%d last=%d", res, err, applies, lastGen)
	}
}

func TestReconcileLostAckRepair(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	client := &fakeControlClient{desired: desiredState(t, 3, cfg)}
	var applies int32
	rec, _, metrics := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		atomic.AddInt32(&applies, 1)
		return nil
	}))
	if _, err := rec.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.acks = nil
	res, err := rec.RunOnce(context.Background())
	if err != nil || res.Action != "LOST_ACK_REPAIR" || applies != 1 || len(client.acks) != 1 {
		t.Fatalf("lost-ack res=%+v err=%v applies=%d acks=%d", res, err, applies, len(client.acks))
	}
	if metrics.Snapshot().LostAckRepairCount != 1 {
		t.Fatalf("metrics=%+v", metrics.Snapshot())
	}
}

func TestReconcileCoreRegressionAndConflict(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	client := &fakeControlClient{desired: desiredState(t, 4, cfg)}
	rec, _, _ := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return nil
	}))
	if _, err := rec.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.desired = desiredState(t, 2, cfg)
	client.acks = nil
	res, err := rec.RunOnce(context.Background())
	if control.CodeOf(err) != control.CodeCoreRegression || res.Action != "CORE_REGRESSION" || len(client.acks) != 0 {
		t.Fatalf("regression res=%+v err=%v acks=%d", res, err, len(client.acks))
	}

	mutated := cfg
	mutated.LogLevel = "ERROR"
	client.desired = desiredState(t, 4, mutated)
	res, err = rec.RunOnce(context.Background())
	if control.CodeOf(err) != control.CodeGenerationConflict || res.Action != "GENERATION_CONFLICT" || len(client.acks) != 0 {
		t.Fatalf("conflict res=%+v err=%v acks=%d", res, err, len(client.acks))
	}
}

func TestReconcileChecksumMismatchRejected(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	desired := desiredState(t, 1, cfg)
	desired.DesiredChecksum = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	client := &fakeControlClient{desired: desired}
	rec, store, _ := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		t.Fatal("must not apply")
		return nil
	}))
	res, err := rec.RunOnce(context.Background())
	if err != nil || res.Action != "CHECKSUM_MISMATCH" || len(client.acks) != 1 || client.acks[0].Outcome != "REJECTED" {
		t.Fatalf("res=%+v err=%v acks=%+v", res, err, client.acks)
	}
	if client.acks[0].RejectionCode != control.CodeChecksumMismatch {
		t.Fatalf("rejection=%+v", client.acks[0])
	}
	snap, _ := store.Load()
	if snap != nil {
		t.Fatal("must preserve empty prior state")
	}
}

func TestReconcileUnsupportedSchema(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	desired := desiredState(t, 1, cfg)
	desired.SchemaVersion = 99
	client := &fakeControlClient{desired: desired}
	rec, _, _ := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return nil
	}))
	res, err := rec.RunOnce(context.Background())
	if err != nil || res.Action != "UNSUPPORTED_SCHEMA" || client.acks[0].RejectionCode != control.CodeUnsupportedSchema {
		t.Fatalf("res=%+v err=%v acks=%+v", res, err, client.acks)
	}
}

func TestReconcileApplyFailurePreservesPrior(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	client := &fakeControlClient{desired: desiredState(t, 1, cfg)}
	rec, store, _ := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return nil
	}))
	if _, err := rec.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.desired = desiredState(t, 2, cfg)
	client.acks = nil
	rec2, err := control.NewReconciler(client, store, validBaseline(t), control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return errors.New("boom")
	}), slog.Default(), &control.Metrics{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := rec2.RunOnce(context.Background())
	if err != nil || res.Action != "APPLY_FAILED" || client.acks[0].Outcome != "REJECTED" {
		t.Fatalf("res=%+v err=%v acks=%+v", res, err, client.acks)
	}
	snap, _ := store.Load()
	if snap == nil || snap.Generation != 1 {
		t.Fatalf("prior generation must remain: %#v", snap)
	}
}

func TestReconcileDurabilityFailurePreventsAppliedAck(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	client := &fakeControlClient{desired: desiredState(t, 1, cfg)}
	failing := &failingStore{err: control.NewDurabilityTestError()}
	rec, err := control.NewReconciler(client, failing, validBaseline(t), control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return nil
	}), slog.Default(), &control.Metrics{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := rec.RunOnce(context.Background())
	if control.CodeOf(err) != control.CodeStateDurabilityFailed || res.Action != "DURABILITY_FAILED" || len(client.acks) != 0 {
		t.Fatalf("res=%+v err=%v acks=%d", res, err, len(client.acks))
	}
}

type failingStore struct {
	err error
}

func (f *failingStore) Load() (*control.AppliedSnapshot, error) { return nil, nil }
func (f *failingStore) Commit(control.AppliedSnapshot) error   { return f.err }


func TestReconcileAckFailureRetriesWithoutReapply(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	client := &fakeControlClient{desired: desiredState(t, 1, cfg), ackErr: errors.New("ack down")}
	var applies int32
	rec, _, _ := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		atomic.AddInt32(&applies, 1)
		return nil
	}))
	res, err := rec.RunOnce(context.Background())
	if control.CodeOf(err) != control.CodeAckFailed || res.Action != "ACK_FAILED" || applies != 1 {
		t.Fatalf("res=%+v err=%v applies=%d", res, err, applies)
	}
	client.ackErr = nil
	client.acks = nil
	res, err = rec.RunOnce(context.Background())
	if err != nil || res.Action != "LOST_ACK_REPAIR" || applies != 1 || len(client.acks) != 1 {
		t.Fatalf("retry res=%+v err=%v applies=%d acks=%d", res, err, applies, len(client.acks))
	}
}

func TestReconcileCoreUnavailablePreservesPrior(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	client := &fakeControlClient{desired: desiredState(t, 1, cfg)}
	rec, store, _ := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return nil
	}))
	if _, err := rec.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	client.desired = nil
	client.fetchErr = errors.New("core transport down")
	res, err := rec.RunOnce(context.Background())
	if control.CodeOf(err) != control.CodeCoreUnavailable || res.Action != "CORE_UNAVAILABLE" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	snap, _ := store.Load()
	if snap == nil || snap.Generation != 1 {
		t.Fatalf("prior must remain %#v", snap)
	}
}

func TestReconcileConcurrentSerialized(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	client := &fakeControlClient{desired: desiredState(t, 1, cfg)}
	var concurrent int32
	var maxConcurrent int32
	rec, _, _ := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		cur := atomic.AddInt32(&concurrent, 1)
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if cur <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&concurrent, -1)
		return nil
	}))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = rec.RunOnce(context.Background())
		}()
	}
	wg.Wait()
	if maxConcurrent != 1 {
		t.Fatalf("expected serialized apply, maxConcurrent=%d", maxConcurrent)
	}
}

func TestReconcileCancellation(t *testing.T) {
	t.Parallel()
	client := &fakeControlClient{
		fetchErr: context.Canceled,
	}
	rec, _, _ := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return nil
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := rec.RunOnce(ctx)
	if control.CodeOf(err) != control.CodeCoreUnavailable {
		t.Fatalf("err=%v", err)
	}
}
