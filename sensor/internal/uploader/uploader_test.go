package uploader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/spool"
)

type transientErr struct{}

func (transientErr) Error() string   { return "transient" }
func (transientErr) Transient() bool { return true }

type fakeSpool struct {
	batch   spool.Batch
	stats   spool.Stats
	acks    []uint64
	readErr error
	ackErr  error
}

func (f *fakeSpool) ReadOrReserveOutboundBatch(int, int) (spool.Batch, error) {
	return f.batch, f.readErr
}
func (f *fakeSpool) CompleteOutboundBatch(v uint64) error {
	f.acks = append(f.acks, v)
	return f.ackErr
}
func (f *fakeSpool) Stats() (spool.Stats, error) { return f.stats, nil }

type fakeTransport struct {
	calls int
	fn    func(int, coreclient.Batch) (coreclient.UploadAck, error)
}

func (f *fakeTransport) Upload(ctx context.Context, b coreclient.Batch) (coreclient.UploadAck, error) {
	f.calls++
	return f.fn(f.calls, b)
}
func rec(seq uint64) spool.Record {
	return spool.Record{SchemaVersion: 1, Sequence: seq, ID: "r", Class: spool.ClassMetric, Type: "m", CreatedAt: time.Now(), ContentLength: 1, Checksum: "x", Payload: []byte("x")}
}
func cfg() Config {
	return Config{SensorID: "s1", StreamID: "stream-00112233445566778899aabbccddeeff", MaxBatchRecords: 10, MaxBatchPayloadBytes: 1000, MaxRequestBytes: 10000, MaxAttempts: 3, InitialBackoff: time.Millisecond, MaxBackoff: 10 * time.Millisecond, JitterFraction: 0, IdleInterval: time.Second}
}
func TestSuccessRequiresExactTerminalAck(t *testing.T) {
	v := uint64(2)
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 2, Records: []spool.Record{rec(1), rec(2)}, PayloadBytes: 2}}
	tr := &fakeTransport{fn: func(_ int, b coreclient.Batch) (coreclient.UploadAck, error) {
		if b.FirstSequence != 1 || b.LastSequence != 2 || b.BatchID == "" || b.CorrelationID == "" {
			t.Fatalf("bad batch %#v", b)
		}
		if b.BootEpoch != "stream-00112233445566778899aabbccddeeff" {
			t.Fatalf("wire bootEpoch must carry durable spool stream ID, got %q", b.BootEpoch)
		}
		return coreclient.UploadAck{Status: "ACCEPTED", SensorIdentity: "s1", AcceptedThroughSequence: &v}, nil
	}}
	u, _ := New(cfg(), sp, tr)
	ok, err := u.RunOnce(context.Background())
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if len(sp.acks) != 1 || sp.acks[0] != 2 {
		t.Fatalf("acks %#v", sp.acks)
	}
}

func TestPartialAckRetainsSpool(t *testing.T) {
	v := uint64(1)
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 2, Records: []spool.Record{rec(1), rec(2)}, PayloadBytes: 2}}
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) {
		return coreclient.UploadAck{Status: "ACCEPTED", SensorIdentity: "s1", AcceptedThroughSequence: &v}, nil
	}}
	u, _ := New(cfg(), sp, tr)
	_, err := u.RunOnce(context.Background())
	if !isCode(err, CodeAckInvalid) || len(sp.acks) != 0 {
		t.Fatalf("err=%v acks=%v", err, sp.acks)
	}
}
func TestAckMissingRetainsSpool(t *testing.T) {
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 1, Records: []spool.Record{rec(1)}, PayloadBytes: 1}}
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) {
		return coreclient.UploadAck{}, &coreclient.Error{Code: coreclient.CodeAckContractMissing}
	}}
	u, _ := New(cfg(), sp, tr)
	_, err := u.RunOnce(context.Background())
	if !isCode(err, CodeAckContractUnavailable) || len(sp.acks) != 0 || tr.calls != 1 {
		t.Fatalf("err=%v acks=%v calls=%d", err, sp.acks, tr.calls)
	}
}
func TestInvalidAckRetainsSpool(t *testing.T) {
	v := uint64(9)
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 2, Records: []spool.Record{rec(1), rec(2)}, PayloadBytes: 2}}
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) {
		return coreclient.UploadAck{Status: "ACCEPTED", SensorIdentity: "s1", AcceptedThroughSequence: &v}, nil
	}}
	u, _ := New(cfg(), sp, tr)
	_, err := u.RunOnce(context.Background())
	if !isCode(err, CodeTransport) && !isCode(err, CodeAckInvalid) {
		t.Fatalf("err=%v", err)
	}
	if len(sp.acks) != 0 {
		t.Fatal("spool acked")
	}
}
func TestTransientRetryThenSuccess(t *testing.T) {
	v := uint64(1)
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 1, Records: []spool.Record{rec(1)}, PayloadBytes: 1}}
	tr := &fakeTransport{fn: func(n int, _ coreclient.Batch) (coreclient.UploadAck, error) {
		if n < 3 {
			return coreclient.UploadAck{}, transientErr{}
		}
		return coreclient.UploadAck{Status: "ACCEPTED", SensorIdentity: "s1", AcceptedThroughSequence: &v}, nil
	}}
	u, _ := New(cfg(), sp, tr)
	u.wait = func(context.Context, time.Duration) error { return nil }
	ok, err := u.RunOnce(context.Background())
	if err != nil || !ok || tr.calls != 3 || u.retries.Load() != 2 {
		t.Fatalf("ok=%v err=%v calls=%d retries=%d", ok, err, tr.calls, u.retries.Load())
	}
}
func TestPermanentNoRetry(t *testing.T) {
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 1, Records: []spool.Record{rec(1)}, PayloadBytes: 1}}
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) {
		return coreclient.UploadAck{}, &coreclient.Error{Code: coreclient.CodeAuthentication}
	}}
	u, _ := New(cfg(), sp, tr)
	_, err := u.RunOnce(context.Background())
	if err == nil || tr.calls != 1 || len(sp.acks) != 0 {
		t.Fatalf("err=%v calls=%d", err, tr.calls)
	}
}
func TestCancellationStopsBackoff(t *testing.T) {
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 1, Records: []spool.Record{rec(1)}, PayloadBytes: 1}}
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) {
		return coreclient.UploadAck{}, transientErr{}
	}}
	u, _ := New(cfg(), sp, tr)
	ctx, cancel := context.WithCancel(context.Background())
	u.wait = func(context.Context, time.Duration) error { cancel(); return context.Canceled }
	_, err := u.RunOnce(ctx)
	if err == nil {
		t.Fatal("want cancel")
	}
}
func TestSnapshotBacklog(t *testing.T) {
	sp := &fakeSpool{stats: spool.Stats{RecordCount: 3, SpoolBytes: 99, OldestRecordAge: 5 * time.Second}}
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) { return coreclient.UploadAck{}, nil }}
	u, _ := New(cfg(), sp, tr)
	s, err := u.Snapshot()
	if err != nil || s.BacklogRecords != 3 || s.BacklogBytes != 99 || s.UploadLag != 5*time.Second {
		t.Fatalf("%#v %v", s, err)
	}
}
func TestConfigValidation(t *testing.T) {
	_, err := New(Config{}, &fakeSpool{}, &fakeTransport{})
	if err == nil {
		t.Fatal("expected config error")
	}
}
func TestSpoolReadFailure(t *testing.T) {
	sp := &fakeSpool{readErr: errors.New("bad")}
	u, _ := New(cfg(), sp, &fakeTransport{})
	_, err := u.RunOnce(context.Background())
	if !isCode(err, CodeSpool) {
		t.Fatalf("%v", err)
	}
}
func TestSerializedRequestBound(t *testing.T) {
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 1, Records: []spool.Record{rec(1)}, PayloadBytes: 1}}
	c := cfg()
	c.MaxRequestBytes = 8
	u, _ := New(c, sp, &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) {
		t.Fatal("transport must not be called")
		return coreclient.UploadAck{}, nil
	}})
	_, err := u.RunOnce(context.Background())
	if !isCode(err, CodeInvalidConfig) {
		t.Fatalf("err=%v", err)
	}
}
func TestShutdownBeforeStartIsIdempotent(t *testing.T) {
	u, _ := New(cfg(), &fakeSpool{}, &fakeTransport{})
	if err := u.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.Start(context.Background()); err == nil {
		t.Fatal("stopped uploader restarted")
	}
}
func TestShutdownAfterStartIsIdempotent(t *testing.T) {
	sp := &fakeSpool{}
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) { return coreclient.UploadAck{}, nil }}
	u, _ := New(cfg(), sp, tr)
	if err := u.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := u.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := u.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownAckFieldsFailClosedPreventsSpoolAck(t *testing.T) {
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 2, Records: []spool.Record{rec(1), rec(2)}, PayloadBytes: 2}}
	// Test that strict JSON unmarshaling in coreclient rejects unknown fields fail-closed
	var rawAck coreclient.UploadAck
	rawJSON := []byte(`{"status":"ACCEPTED","accepted_through_sequence":2,"spool_retention_required":true}`)
	unmarshalErr := json.Unmarshal(rawJSON, &rawAck)
	if unmarshalErr == nil {
		t.Fatal("expected strict JSON unmarshaling to reject unknown field spool_retention_required")
	}
	// Transport wraps this protocol failure
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) {
		return rawAck, &coreclient.Error{Code: coreclient.CodeProtocol, Message: unmarshalErr.Error()}
	}}
	u, _ := New(cfg(), sp, tr)
	ok, err := u.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("expected uploader to fail when transport returns protocol error, got ok=%v err=%v", ok, err)
	}
	if len(sp.acks) != 0 {
		t.Fatalf("spool was acknowledged despite unknown fields: %v", sp.acks)
	}
}

func TestUploaderFrozenWindowSurvivesGrowthRestartAndExactChecksum(t *testing.T) {
	dir := t.TempDir()
	sp, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.Close() })
	spSpace := func(int64) (spool.Stats, error) { return spool.Stats{}, nil }
	_ = spSpace
	// Bypass 80% protection in unit test by using high free capacity via unexported hook:
	// DiskSpool.space is package-private to spool; Append uses real FS which is fine in TempDir.

	for i := 1; i <= 5; i++ {
		payload := []byte(fmt.Sprintf(`{"n":%d}`, i))
		if _, err := sp.Append(spool.AppendRequest{Class: spool.ClassMetric, Type: "m", Payload: payload, CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	var firstBatch coreclient.Batch
	failOnce := true
	tr := &fakeTransport{fn: func(call int, b coreclient.Batch) (coreclient.UploadAck, error) {
		if failOnce {
			failOnce = false
			firstBatch = b
			return coreclient.UploadAck{}, &transientErr{}
		}
		if b.FirstSequence != firstBatch.FirstSequence || b.LastSequence != firstBatch.LastSequence ||
			b.Checksum != firstBatch.Checksum || b.BatchID != firstBatch.BatchID {
			t.Fatalf("retry must replay exact frozen batch\n first=%+v\n retry=%+v", firstBatch, b)
		}
		v := b.LastSequence
		return coreclient.UploadAck{Status: "ACCEPTED", SensorIdentity: "s1", AcceptedThroughSequence: &v}, nil
	}}

	cfg := cfg()
	cfg.StreamID = sp.StreamID()
	cfg.MaxAttempts = 1
	u1, err := New(cfg, sp, tr)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := u1.RunOnce(context.Background())
	if ok || err == nil {
		t.Fatalf("first RunOnce must fail transport, ok=%v err=%v", ok, err)
	}
	if firstBatch.FirstSequence != 1 || firstBatch.LastSequence != 5 {
		t.Fatalf("first submission range=%d..%d", firstBatch.FirstSequence, firstBatch.LastSequence)
	}

	for i := 6; i <= 10; i++ {
		payload := []byte(fmt.Sprintf(`{"n":%d}`, i))
		if _, err := sp.Append(spool.AppendRequest{Class: spool.ClassMetric, Type: "m", Payload: payload, CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	if err := sp.Close(); err != nil {
		t.Fatal(err)
	}
	sp2, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp2.Close() })

	cfg2 := cfg
	cfg2.StreamID = sp2.StreamID()
	cfg2.MaxAttempts = 1
	u2, err := New(cfg2, sp2, tr)
	if err != nil {
		t.Fatal(err)
	}
	ok, err = u2.RunOnce(context.Background())
	if err != nil || !ok {
		t.Fatalf("retry after growth/restart must succeed: ok=%v err=%v", ok, err)
	}

	next, err := sp2.ReadOrReserveOutboundBatch(10, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if next.FirstSequence != 6 || next.LastSequence != 10 {
		t.Fatalf("after ACK next outbound must be 6..10, got %#v", next)
	}
}

func TestUploaderAckStatusExactAccepted(t *testing.T) {
	v := uint64(1)
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 1, Records: []spool.Record{rec(1)}, PayloadBytes: 1}}
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) {
		return coreclient.UploadAck{Status: "accepted", SensorIdentity: "s1", AcceptedThroughSequence: &v}, nil
	}}
	u, _ := New(cfg(), sp, tr)
	_, err := u.RunOnce(context.Background())
	if !isCode(err, CodeAckInvalid) || len(sp.acks) != 0 {
		t.Fatalf("Equal-fold ACCEPTED must be rejected: err=%v acks=%v", err, sp.acks)
	}
}

func TestOuterLoopPermanentFailurePacedAtMaxBackoff(t *testing.T) {
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 1, Records: []spool.Record{rec(1)}, PayloadBytes: 1}}
	var seen []coreclient.Batch
	tr := &fakeTransport{fn: func(_ int, b coreclient.Batch) (coreclient.UploadAck, error) {
		seen = append(seen, b)
		return coreclient.UploadAck{}, &coreclient.Error{Code: coreclient.CodeAuthentication}
	}}
	c := cfg()
	c.IdleInterval = time.Second
	c.MaxBackoff = 5 * time.Second
	c.MaxAttempts = 3
	u, err := New(c, sp, tr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var waits []time.Duration
	var nextAttempts []time.Time
	finished := make(chan struct{})
	u.wait = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		snap, snapErr := u.Snapshot()
		if snapErr != nil {
			t.Fatalf("snapshot: %v", snapErr)
		}
		nextAttempts = append(nextAttempts, snap.NextAttempt)
		if len(waits) >= 3 {
			cancel()
			select {
			case <-finished:
			default:
				close(finished)
			}
			return context.Canceled
		}
		return nil
	}
	if err := u.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for 3 permanent failure outer cycles")
	}
	shutCtx, shutCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutCancel()
	if err := u.Shutdown(shutCtx); err != nil {
		t.Fatal(err)
	}
	if len(waits) != 3 {
		t.Fatalf("want 3 outer waits, got %d (%v)", len(waits), waits)
	}
	for i, d := range waits {
		if d != c.MaxBackoff {
			t.Fatalf("wait[%d]=%s want MaxBackoff %s (not IdleInterval %s)", i, d, c.MaxBackoff, c.IdleInterval)
		}
		if nextAttempts[i].IsZero() {
			t.Fatalf("NextAttempt[%d] must be set for truthful failure pacing", i)
		}
	}
	if tr.calls != 3 {
		t.Fatalf("want 3 outer transport calls (no inner permanent retry), got %d", tr.calls)
	}
	if len(sp.acks) != 0 {
		t.Fatalf("permanent rejection must not ACK/delete: %v", sp.acks)
	}
	snap, err := u.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !snap.LastSuccessfulUpload.IsZero() {
		t.Fatalf("last-success must not promote on permanent failure: %v", snap.LastSuccessfulUpload)
	}
	if snap.Uploads != 3 || snap.Retries != 0 {
		t.Fatalf("Uploads/Retries semantics: uploads=%d retries=%d want 3/0", snap.Uploads, snap.Retries)
	}
	if snap.State != StateStopped && snap.State != StateDegraded {
		t.Fatalf("state=%s", snap.State)
	}
	if len(seen) != 3 {
		t.Fatalf("seen batches=%d", len(seen))
	}
	for i := 1; i < len(seen); i++ {
		if seen[i].BatchID != seen[0].BatchID || seen[i].Checksum != seen[0].Checksum ||
			seen[i].FirstSequence != seen[0].FirstSequence || seen[i].LastSequence != seen[0].LastSequence ||
			seen[i].CorrelationID != seen[0].CorrelationID {
			t.Fatalf("frozen replay diverged\n first=%+v\n later=%+v", seen[0], seen[i])
		}
	}
}

func TestOuterLoopIdleIntervalGreaterThanMaxBackoffPolicy(t *testing.T) {
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 1, Records: []spool.Record{rec(1)}, PayloadBytes: 1}}
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) {
		return coreclient.UploadAck{}, &coreclient.Error{Code: coreclient.CodeAuthentication}
	}}
	c := cfg()
	c.IdleInterval = 10 * time.Second
	c.MaxBackoff = 2 * time.Second
	u, err := New(c, sp, tr)
	if err != nil {
		t.Fatal(err)
	}
	if got := u.outerCycleDelay(errors.New("fail")); got != c.MaxBackoff {
		t.Fatalf("IdleInterval>MaxBackoff policy: failure delay=%s want MaxBackoff %s", got, c.MaxBackoff)
	}
	if got := u.outerCycleDelay(nil); got != c.IdleInterval {
		t.Fatalf("success/empty delay=%s want IdleInterval %s", got, c.IdleInterval)
	}
}

func TestOuterLoopSuccessThenIdleThenPermanentFailurePacing(t *testing.T) {
	v := uint64(1)
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 1, Records: []spool.Record{rec(1)}, PayloadBytes: 1}}
	phase := 0
	tr := &fakeTransport{fn: func(_ int, b coreclient.Batch) (coreclient.UploadAck, error) {
		phase++
		if phase == 1 {
			return coreclient.UploadAck{Status: "ACCEPTED", SensorIdentity: "s1", AcceptedThroughSequence: &v}, nil
		}
		return coreclient.UploadAck{}, &coreclient.Error{Code: coreclient.CodeAuthentication}
	}}
	c := cfg()
	c.IdleInterval = 100 * time.Millisecond
	c.MaxBackoff = 500 * time.Millisecond
	u, err := New(c, sp, tr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var waits []time.Duration
	finished := make(chan struct{})
	u.wait = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		if len(waits) == 1 {
			sp.acks = nil
		}
		if len(waits) >= 3 {
			cancel()
			select {
			case <-finished:
			default:
				close(finished)
			}
			return context.Canceled
		}
		return nil
	}
	if err := u.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for success+failure pacing cycles")
	}
	shutCtx, shutCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutCancel()
	if err := u.Shutdown(shutCtx); err != nil {
		t.Fatal(err)
	}
	if len(waits) < 3 {
		t.Fatalf("waits=%v", waits)
	}
	if waits[0] != c.IdleInterval {
		t.Fatalf("after success want IdleInterval %s, got %s", c.IdleInterval, waits[0])
	}
	if waits[1] != c.MaxBackoff || waits[2] != c.MaxBackoff {
		t.Fatalf("after permanent failure want MaxBackoff pacing, got %v", waits)
	}
	snap, err := u.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.LastSuccessfulUpload.IsZero() {
		t.Fatal("successful ACK must record last-success before later permanent failures")
	}
	if snap.AcknowledgedThrough != 1 {
		t.Fatalf("acked=%d", snap.AcknowledgedThrough)
	}
}

func TestOuterLoopEmptyIdleCadence(t *testing.T) {
	sp := &fakeSpool{}
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) {
		t.Fatal("empty spool must not call transport")
		return coreclient.UploadAck{}, nil
	}}
	c := cfg()
	c.IdleInterval = 250 * time.Millisecond
	c.MaxBackoff = time.Second
	u, err := New(c, sp, tr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var waits []time.Duration
	finished := make(chan struct{})
	u.wait = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		if len(waits) >= 2 {
			cancel()
			select {
			case <-finished:
			default:
				close(finished)
			}
			return context.Canceled
		}
		return nil
	}
	if err := u.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for empty idle cycles")
	}
	shutCtx, shutCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutCancel()
	if err := u.Shutdown(shutCtx); err != nil {
		t.Fatal(err)
	}
	for i, d := range waits {
		if d != c.IdleInterval {
			t.Fatalf("empty idle wait[%d]=%s want %s", i, d, c.IdleInterval)
		}
	}
	if tr.calls != 0 {
		t.Fatalf("transport calls=%d", tr.calls)
	}
}

func TestOuterLoopTransientExhaustionThenSuccessResumesIdle(t *testing.T) {
	v := uint64(1)
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 1, Records: []spool.Record{rec(1)}, PayloadBytes: 1}}
	c := cfg()
	c.MaxAttempts = 2
	c.InitialBackoff = time.Millisecond
	c.MaxBackoff = 40 * time.Millisecond
	c.IdleInterval = 15 * time.Millisecond
	c.JitterFraction = 0
	phase := 0
	tr := &fakeTransport{fn: func(_ int, b coreclient.Batch) (coreclient.UploadAck, error) {
		phase++
		if phase <= c.MaxAttempts {
			return coreclient.UploadAck{}, transientErr{}
		}
		return coreclient.UploadAck{Status: "ACCEPTED", SensorIdentity: "s1", AcceptedThroughSequence: &v}, nil
	}}
	u, err := New(c, sp, tr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var outerWaits []time.Duration
	var innerWaits []time.Duration
	finished := make(chan struct{})
	u.wait = func(_ context.Context, d time.Duration) error {
		if d == c.InitialBackoff {
			innerWaits = append(innerWaits, d)
			return nil
		}
		outerWaits = append(outerWaits, d)
		if len(outerWaits) >= 2 {
			cancel()
			select {
			case <-finished:
			default:
				close(finished)
			}
			return context.Canceled
		}
		return nil
	}
	if err := u.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for transient exhaustion then success")
	}
	shutCtx, shutCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutCancel()
	if err := u.Shutdown(shutCtx); err != nil {
		t.Fatal(err)
	}
	if len(innerWaits) != 1 || innerWaits[0] != c.InitialBackoff {
		t.Fatalf("inner backoff waits=%v", innerWaits)
	}
	if len(outerWaits) < 2 {
		t.Fatalf("outer waits=%v", outerWaits)
	}
	if outerWaits[0] != c.MaxBackoff {
		t.Fatalf("after transient exhaustion want MaxBackoff %s, got %s", c.MaxBackoff, outerWaits[0])
	}
	if outerWaits[1] != c.IdleInterval {
		t.Fatalf("after success want IdleInterval %s, got %s", c.IdleInterval, outerWaits[1])
	}
	snap, err := u.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.LastSuccessfulUpload.IsZero() || snap.AcknowledgedThrough != 1 {
		t.Fatalf("success recovery snapshot=%+v", snap)
	}
	if snap.Retries != 1 {
		t.Fatalf("Retries must count inner transient retries only, got %d", snap.Retries)
	}
	if len(sp.acks) != 1 || sp.acks[0] != 1 {
		t.Fatalf("acks=%v", sp.acks)
	}
}

func TestOuterLoopCancelDuringFailureWaitStopsWithoutNextUpload(t *testing.T) {
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 1, Records: []spool.Record{rec(1)}, PayloadBytes: 1}}
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) {
		return coreclient.UploadAck{}, &coreclient.Error{Code: coreclient.CodeAuthentication}
	}}
	c := cfg()
	c.MaxBackoff = 5 * time.Second
	c.IdleInterval = time.Second
	u, err := New(c, sp, tr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sawWait := make(chan struct{})
	u.wait = func(_ context.Context, d time.Duration) error {
		if d != c.MaxBackoff {
			t.Fatalf("cancel-during-failure wait=%s want MaxBackoff %s", d, c.MaxBackoff)
		}
		cancel()
		select {
		case <-sawWait:
		default:
			close(sawWait)
		}
		return context.Canceled
	}
	if err := u.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sawWait:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for outer failure wait")
	}
	shutCtx, shutCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutCancel()
	if err := u.Shutdown(shutCtx); err != nil {
		t.Fatal(err)
	}
	if tr.calls != 1 {
		t.Fatalf("cancel during outer wait must not start next upload, calls=%d", tr.calls)
	}
	if len(sp.acks) != 0 {
		t.Fatalf("acks=%v", sp.acks)
	}
}

func TestOuterLoopPermanentFailureReplaysFrozenBatchWhileBacklogGrows(t *testing.T) {
	dir := t.TempDir()
	sp, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.Close() })
	for i := 1; i <= 3; i++ {
		payload := []byte(fmt.Sprintf(`{"n":%d}`, i))
		if _, err := sp.Append(spool.AppendRequest{Class: spool.ClassMetric, Type: "m", Payload: payload, CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	var seen []coreclient.Batch
	tr := &fakeTransport{fn: func(_ int, b coreclient.Batch) (coreclient.UploadAck, error) {
		seen = append(seen, b)
		return coreclient.UploadAck{}, &coreclient.Error{Code: coreclient.CodeAuthentication}
	}}
	c := cfg()
	c.StreamID = sp.StreamID()
	c.MaxAttempts = 1
	c.IdleInterval = time.Second
	c.MaxBackoff = 5 * time.Second
	u, err := New(c, sp, tr)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	growSeq := 4
	var waits []time.Duration
	finished := make(chan struct{})
	u.wait = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		payload := []byte(fmt.Sprintf(`{"n":%d}`, growSeq))
		growSeq++
		if _, err := sp.Append(spool.AppendRequest{Class: spool.ClassMetric, Type: "m", Payload: payload, CreatedAt: time.Now()}); err != nil {
			t.Fatalf("append during failure pacing: %v", err)
		}
		if len(waits) >= 3 {
			cancel()
			select {
			case <-finished:
			default:
				close(finished)
			}
			return context.Canceled
		}
		return nil
	}
	if err := u.Start(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for frozen replay cycles with growth")
	}
	shutCtx, shutCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutCancel()
	if err := u.Shutdown(shutCtx); err != nil {
		t.Fatal(err)
	}
	if len(waits) != 3 {
		t.Fatalf("waits=%v", waits)
	}
	for i, d := range waits {
		if d != c.MaxBackoff {
			t.Fatalf("wait[%d]=%s want MaxBackoff", i, d)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("uploads=%d", len(seen))
	}
	for i := 1; i < len(seen); i++ {
		if seen[i].BatchID != seen[0].BatchID || seen[i].Checksum != seen[0].Checksum ||
			seen[i].FirstSequence != seen[0].FirstSequence || seen[i].LastSequence != seen[0].LastSequence {
			t.Fatalf("frozen window expanded under growth\n first=%+v\n later=%+v", seen[0], seen[i])
		}
	}
	if seen[0].FirstSequence != 1 || seen[0].LastSequence != 3 {
		t.Fatalf("frozen range=%d..%d", seen[0].FirstSequence, seen[0].LastSequence)
	}
	st, err := sp.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.RecordCount < 6 {
		t.Fatalf("backlog must grow while frozen window retained, records=%d", st.RecordCount)
	}
	snap, err := u.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !snap.LastSuccessfulUpload.IsZero() {
		t.Fatal("no last-success on permanent reject path")
	}
	pending, err := sp.ReadOrReserveOutboundBatch(10, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if pending.FirstSequence != 1 || pending.LastSequence != 3 {
		t.Fatalf("durable custody must retain frozen 1..3, got %#v", pending)
	}
}

func TestBackoffClampsJitterAboveMaxAndNonPositive(t *testing.T) {
	max := 5 * time.Second
	initial := 500 * time.Millisecond
	over := backoff(initial, max, 5, func(time.Duration, float64) time.Duration {
		return max + time.Second
	}, 1)
	if over != max {
		t.Fatalf("jitter above max: got %s want %s", over, max)
	}
	zero := backoff(initial, max, 1, func(time.Duration, float64) time.Duration {
		return 0
	}, 1)
	if zero <= 0 {
		t.Fatalf("jitter zero must become positive, got %s", zero)
	}
	if zero > max {
		t.Fatalf("positive clamp escaped max: %s", zero)
	}
	neg := backoff(initial, max, 1, func(time.Duration, float64) time.Duration {
		return -time.Second
	}, 1)
	if neg <= 0 || neg > max {
		t.Fatalf("jitter negative clamp out of bounds: %s", neg)
	}
	atCap := backoff(initial, max, 5, func(d time.Duration, _ float64) time.Duration {
		if d != max {
			t.Fatalf("pre-jitter base at attempt 5 want max %s got %s", max, d)
		}
		return d
	}, 0)
	if atCap != max {
		t.Fatalf("cap reached got %s", atCap)
	}
}

func TestClampBackoffDelayEdgeArithmetic(t *testing.T) {
	if got := clampBackoffDelay(0, time.Second); got <= 0 || got > time.Second {
		t.Fatalf("zero input: %s", got)
	}
	if got := clampBackoffDelay(-5, time.Second); got <= 0 || got > time.Second {
		t.Fatalf("negative input: %s", got)
	}
	if got := clampBackoffDelay(2*time.Second, time.Second); got != time.Second {
		t.Fatalf("above max: %s", got)
	}
	if got := clampBackoffDelay(time.Millisecond, 0); got <= 0 {
		t.Fatalf("non-positive max floor: %s", got)
	}
	if got := clampBackoffDelay(time.Second, time.Second); got != time.Second {
		t.Fatalf("exact max: %s", got)
	}
}

func TestJitterFractionNaNAndInfRejected(t *testing.T) {
	tr := &fakeTransport{fn: func(int, coreclient.Batch) (coreclient.UploadAck, error) {
		return coreclient.UploadAck{}, nil
	}}
	sp := &fakeSpool{}
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		c := cfg()
		c.JitterFraction = f
		u, err := New(c, sp, tr)
		if err == nil || u != nil {
			t.Fatalf("JitterFraction=%v must be rejected by New (got u=%v err=%v); NaN must not pass range predicate", f, u, err)
		}
		if !isCode(err, CodeInvalidConfig) {
			t.Fatalf("JitterFraction=%v want CodeInvalidConfig, got %v", f, err)
		}
	}
	c := cfg()
	c.JitterFraction = 0.5
	if _, err := New(c, sp, tr); err != nil {
		t.Fatalf("finite in-range JitterFraction must remain accepted: %v", err)
	}
}

func TestJitterDurationSaturatesMaxInt64TinyFraction(t *testing.T) {
	// float64(MaxInt64) rounds to 2^63. With math.SmallestNonzeroFloat64 the
	// jitter band cannot move v off that boundary for any rand draw, so the
	// cast must saturate to MaxInt64 — never overflow into a 1ns tight retry.
	d := time.Duration(math.MaxInt64)
	f := math.SmallestNonzeroFloat64
	bound := float64(math.MaxInt64)
	if bound != math.Ldexp(1, 63) {
		t.Fatalf("diagnostic: float64(MaxInt64)=%v want 2^63=%v (rounded boundary)", bound, math.Ldexp(1, 63))
	}
	for i := 0; i < 64; i++ {
		delta := float64(d) * f
		// Pre-image of the conversion: independent of the consumed rand draw,
		// v remains at the 2^63 boundary for this (d,f) pair.
		vProbe := float64(d) - delta + 0*2*delta
		if !(vProbe >= bound) {
			t.Fatalf("draw-prep %d: expected v>=float64(MaxInt64) independent of rand; v=%v delta=%v", i, vProbe, delta)
		}
		got := jitterDuration(d, f)
		if got == time.Nanosecond || got <= 0 {
			t.Fatalf("draw %d: saturated jitter must not collapse to tight retry; got %d (%s)", i, got, got)
		}
		if got != time.Duration(math.MaxInt64) {
			t.Fatalf("draw %d: jitter=%d (%s) want MaxInt64 saturate %d", i, got, got, int64(math.MaxInt64))
		}
		clamped := clampBackoffDelay(got, d)
		if clamped != d {
			t.Fatalf("draw %d: positive max clamp must preserve MaxInt64, got %d", i, clamped)
		}
	}
}

func TestJitterDurationOrdinaryPositive(t *testing.T) {
	d := time.Second
	f := 0.5
	lo := time.Duration(float64(d) * (1 - f))
	hi := time.Duration(float64(d) * (1 + f))
	for i := 0; i < 64; i++ {
		got := jitterDuration(d, f)
		if got <= 0 {
			t.Fatalf("ordinary jitter draw %d non-positive: %s", i, got)
		}
		if got < lo || got > hi {
			t.Fatalf("ordinary jitter draw %d out of [%s,%s]: %s", i, lo, hi, got)
		}
	}
}
