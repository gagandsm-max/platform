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
	if err != nil || res.Action != "LOST_ACK_REPAIR" || applies != 2 || len(client.acks) != 1 {
		t.Fatalf("lost-ack res=%+v err=%v applies=%d acks=%d", res, err, applies, len(client.acks))
	}
	snap := metrics.Snapshot()
	if snap.LostAckRepairCount != 1 {
		t.Fatalf("metrics=%+v", snap)
	}
	if snap.LastAppliedGeneration != 3 {
		t.Fatalf("lost-ack must set LastAppliedGeneration=3, got %d", snap.LastAppliedGeneration)
	}
	if snap.AppliedCount != 1 {
		t.Fatalf("lost-ack must not increment AppliedCount, got %d", snap.AppliedCount)
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

func TestReconcileConfigInvalidRejected(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	cfg.Scheduler.MaxWorkers = 1
	cfg.Scheduler.Pools = map[string]control.DesiredPool{
		"NETWORK_POLL": {Workers: 2, QueueDepth: 1},
		"SYNTHETIC":    {Workers: 1, QueueDepth: 4},
	}
	client := &fakeControlClient{desired: desiredState(t, 1, cfg)}
	rec, store, _ := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		t.Fatal("applier must not run for invalid candidate")
		return nil
	}))
	res, err := rec.RunOnce(context.Background())
	if err != nil || res.Action != "CONFIG_INVALID" || len(client.acks) != 1 ||
		client.acks[0].Outcome != "REJECTED" || client.acks[0].RejectionCode != control.CodeConfigInvalid {
		t.Fatalf("res=%+v err=%v acks=%+v", res, err, client.acks)
	}
	snap, loadErr := store.Load()
	if loadErr != nil || snap != nil {
		t.Fatalf("no durable snapshot expected: snap=%#v err=%v", snap, loadErr)
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

func TestReconcileLostAckRepairDurabilityFailurePreventsAck(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	desired := desiredState(t, 3, cfg)
	client := &fakeControlClient{desired: desired}
	store := &lostAckFailingStore{
		snap: &control.AppliedSnapshot{
			SchemaVersion: 1,
			Generation:    3,
			Checksum:      desired.DesiredChecksum,
			Configuration: cfg,
			AppliedAt:     time.Now().UTC(),
		},
		err: control.NewDurabilityTestError(),
	}
	var applies int32
	metrics := &control.Metrics{}
	rec, err := control.NewReconciler(client, store, validBaseline(t), control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		atomic.AddInt32(&applies, 1)
		return nil
	}), slog.Default(), metrics)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rec.RunOnce(context.Background())
	if control.CodeOf(err) != control.CodeStateDurabilityFailed || res.Action != "DURABILITY_FAILED" || len(client.acks) != 0 || applies != 1 {
		t.Fatalf("res=%+v err=%v acks=%d applies=%d", res, err, len(client.acks), applies)
	}
	if metrics.Snapshot().DurableStoreFailures != 1 {
		t.Fatalf("expected DurableStoreFailures=1, got %d", metrics.Snapshot().DurableStoreFailures)
	}
}

type lostAckFailingStore struct {
	snap *control.AppliedSnapshot
	err  error
}

func (f *lostAckFailingStore) Load() (*control.AppliedSnapshot, error) { return f.snap, nil }
func (f *lostAckFailingStore) Commit(control.AppliedSnapshot) error   { return f.err }

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
	if err != nil || res.Action != "LOST_ACK_REPAIR" || applies != 2 || len(client.acks) != 1 {
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

func TestReconcileFetchProtocolErrorClassifiedAsInvalidDesired(t *testing.T) {
	t.Parallel()
	client := &fakeControlClient{
		fetchErr: &coreclient.Error{Code: coreclient.CodeControlProtocol, Message: "malformed JSON wire"},
	}
	metrics := &control.Metrics{}
	store, err := control.NewStore(filepath.Join(t.TempDir(), "applied.json"))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := control.NewReconciler(client, store, validBaseline(t), control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return nil
	}), slog.Default(), metrics)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rec.RunOnce(context.Background())
	if control.CodeOf(err) != control.CodeInvalidDesired || res.Action != "INVALID_DESIRED" || res.Code != control.CodeInvalidDesired {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if metrics.Snapshot().CoreTransportFailures != 0 {
		t.Fatalf("transport failure metric should NOT increment on invalid desired, got %d", metrics.Snapshot().CoreTransportFailures)
	}
}

func TestReconcileFetchOversizedClassifiedAsInvalidDesired(t *testing.T) {
	t.Parallel()
	client := &fakeControlClient{
		fetchErr: &coreclient.Error{Code: coreclient.CodeResponseTooLarge, Message: "response exceeds 16MB"},
	}
	metrics := &control.Metrics{}
	store, err := control.NewStore(filepath.Join(t.TempDir(), "applied.json"))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := control.NewReconciler(client, store, validBaseline(t), control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return nil
	}), slog.Default(), metrics)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rec.RunOnce(context.Background())
	if control.CodeOf(err) != control.CodeInvalidDesired || res.Action != "INVALID_DESIRED" || res.Code != control.CodeInvalidDesired {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if metrics.Snapshot().CoreTransportFailures != 0 {
		t.Fatalf("transport failure metric should NOT increment on oversized response, got %d", metrics.Snapshot().CoreTransportFailures)
	}
}

func TestReconcileFetchTransportClassifiedAsCoreUnavailable(t *testing.T) {
	t.Parallel()
	client := &fakeControlClient{
		fetchErr: &coreclient.Error{Code: coreclient.CodeTransport, Message: "connection refused"},
	}
	metrics := &control.Metrics{}
	store, err := control.NewStore(filepath.Join(t.TempDir(), "applied.json"))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := control.NewReconciler(client, store, validBaseline(t), control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return nil
	}), slog.Default(), metrics)
	if err != nil {
		t.Fatal(err)
	}
	res, err := rec.RunOnce(context.Background())
	if control.CodeOf(err) != control.CodeCoreUnavailable || res.Action != "CORE_UNAVAILABLE" || res.Code != control.CodeCoreUnavailable {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if metrics.Snapshot().CoreTransportFailures != 1 {
		t.Fatalf("transport failure metric MUST increment on transport error, got %d", metrics.Snapshot().CoreTransportFailures)
	}
}

func TestReconcileLostAckRestartRequiredNoAppliedAck(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	desired := desiredState(t, 7, cfg)
	client := &fakeControlClient{desired: desired}
	rec, store, _ := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return nil
	}))
	if _, err := rec.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	prior, err := store.Load()
	if err != nil || prior == nil || prior.Generation != 7 {
		t.Fatalf("prior=%#v err=%v", prior, err)
	}

	client2 := &fakeControlClient{desired: desired}
	rec2, err := control.NewReconciler(client2, store, validBaseline(t), control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return control.NewRestartRequiredError("scheduler drift")
	}), slog.Default(), &control.Metrics{})
	if err != nil {
		t.Fatal(err)
	}
	res, err := rec2.RunOnce(context.Background())
	if err != nil || res.Action != "APPLY_FAILED" || res.Code != control.CodeRestartRequired {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if len(client2.acks) != 1 || client2.acks[0].Outcome != "REJECTED" || client2.acks[0].RejectionCode != control.CodeRestartRequired {
		t.Fatalf("acks=%+v", client2.acks)
	}
	if client2.acks[0].Generation != 7 || client2.acks[0].Checksum != desired.DesiredChecksum {
		t.Fatalf("ack identity=%+v", client2.acks[0])
	}
	after, _ := store.Load()
	if after == nil || after.Generation != 7 || after.Checksum != prior.Checksum {
		t.Fatalf("durable snapshot mutated: %#v", after)
	}
}

func TestReconcileNewGenerationRestartRequiredAckCode(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	client := &fakeControlClient{desired: desiredState(t, 2, cfg)}
	rec, store, _ := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return control.NewRestartRequiredError("transport drift")
	}))
	res, err := rec.RunOnce(context.Background())
	if err != nil || res.Code != control.CodeRestartRequired || client.acks[0].RejectionCode != control.CodeRestartRequired {
		t.Fatalf("res=%+v err=%v acks=%+v", res, err, client.acks)
	}
	snap, _ := store.Load()
	if snap != nil {
		t.Fatalf("must not durable-commit on restart-required: %#v", snap)
	}
}

func TestReconcileUnknownApplyErrorMapsToApplyFailed(t *testing.T) {
	t.Parallel()
	cfg := control.GoldenConfiguration()
	client := &fakeControlClient{desired: desiredState(t, 2, cfg)}
	rec, _, _ := newTestReconciler(t, client, control.FuncApplier(func(context.Context, uint64, string, config.RuntimeConfig) error {
		return errors.New("weird-internal-XYZ")
	}))
	res, err := rec.RunOnce(context.Background())
	if err != nil || res.Code != control.CodeApplyFailed || client.acks[0].RejectionCode != control.CodeApplyFailed {
		t.Fatalf("res=%+v err=%v acks=%+v", res, err, client.acks)
	}
	if client.acks[0].RejectionCode == "weird-internal-XYZ" {
		t.Fatal("internal error leaked to ACK")
	}
}
