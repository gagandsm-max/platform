package uploader

import (
	"context"
	"encoding/json"
	"errors"
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

func (f *fakeSpool) ReadBatch(int, int) (spool.Batch, error) { return f.batch, f.readErr }
func (f *fakeSpool) AcknowledgeThrough(v uint64) error       { f.acks = append(f.acks, v); return f.ackErr }
func (f *fakeSpool) Stats() (spool.Stats, error)             { return f.stats, nil }

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
	return Config{SensorID: "s1", BootEpoch: "b1", MaxBatchRecords: 10, MaxBatchPayloadBytes: 1000, MaxRequestBytes: 10000, MaxAttempts: 3, InitialBackoff: time.Millisecond, MaxBackoff: 10 * time.Millisecond, JitterFraction: 0, IdleInterval: time.Second}
}
func TestSuccessAndPartialAck(t *testing.T) {
	v := uint64(1)
	sp := &fakeSpool{batch: spool.Batch{FirstSequence: 1, LastSequence: 2, Records: []spool.Record{rec(1), rec(2)}, PayloadBytes: 2}}
	tr := &fakeTransport{fn: func(_ int, b coreclient.Batch) (coreclient.UploadAck, error) {
		if b.FirstSequence != 1 || b.LastSequence != 2 || b.BatchID == "" || b.CorrelationID == "" {
			t.Fatalf("bad batch %#v", b)
		}
		return coreclient.UploadAck{AcceptedThroughSequence: &v}, nil
	}}
	u, _ := New(cfg(), sp, tr)
	ok, err := u.RunOnce(context.Background())
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if len(sp.acks) != 1 || sp.acks[0] != 1 {
		t.Fatalf("acks %#v", sp.acks)
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
		return coreclient.UploadAck{AcceptedThroughSequence: &v}, nil
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
		return coreclient.UploadAck{AcceptedThroughSequence: &v}, nil
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
