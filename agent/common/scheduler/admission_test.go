package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"platform/agent/common/client"
	"platform/agent/common/collector"
	"platform/agent/common/queue"
	"platform/agent/common/telemetry"
)

func admissionFixture(t *testing.T, enq Enqueuer, names ...string) *Scheduler {
	t.Helper()
	var regs []collector.Registration
	for _, name := range names {
		regs = append(regs, collector.Registration{Collector: &mockCollector{name: name}, Enabled: true, Interval: time.Hour, Timeout: time.Second})
	}
	s, err := NewScheduler(testIdent(), mustRegistry(t, regs...), enq, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func admissionObservations(t *testing.T, n int, name string) []telemetry.Observation {
	t.Helper()
	out := make([]telemetry.Observation, n)
	for i := range out {
		o, err := telemetry.Normalize(testIdent(), name, time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), telemetry.Sample{MetricKey: "system.cpu.utilization", Value: 1, Unit: "%", Labels: map[string]string{"fixture": strings.Repeat("x", 128)}})
		if err != nil {
			t.Fatal(err)
		}
		o.ObservationID = fmt.Sprintf("obs-%s-%032x", name, i)
		out[i] = o
	}
	return out
}
func depositObservations(s *Scheduler, name string, obs []telemetry.Observation, errs []telemetry.CollectorError) {
	s.deposit(pendingContribution{name: name, collectedAt: time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC), durationMs: 17, observations: obs, errors: errs})
}
func readCollections(t *testing.T, q *queue.DurableQueue) ([]queue.Record, []telemetry.Observation, []telemetry.CollectorError) {
	t.Helper()
	records, err := q.ReadBatch(10000, 10<<20)
	if err != nil {
		t.Fatal(err)
	}
	var obs []telemetry.Observation
	var errs []telemetry.CollectorError
	for i, r := range records {
		if len(r.Payload) > q.MaxRecordBytes() || r.Sequence != uint64(i+1) {
			t.Fatalf("record boundary/sequence %d %d", len(r.Payload), r.Sequence)
		}
		var b telemetry.CollectionBatch
		if err := json.Unmarshal(r.Payload, &b); err != nil {
			t.Fatal(err)
		}
		if b.AgentID != testIdent().AgentID || b.TenantID != testIdent().TenantID || b.HostID != testIdent().HostID || b.DurationMs != 17 || !b.CollectedAt.Equal(time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)) {
			t.Fatalf("metadata changed: %+v", b)
		}
		obs = append(obs, b.Observations...)
		errs = append(errs, b.Errors...)
	}
	return records, obs, errs
}
func newAdmissionQueue(t *testing.T, limit int) *queue.DurableQueue {
	t.Helper()
	q, err := queue.NewDurableQueue(t.TempDir(), 10000, 16<<20, limit)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestIssue302EncodedCollectionBoundaries(t *testing.T) {
	for _, target := range []int{65535, 65536, 65537} {
		t.Run(fmt.Sprint(target), func(t *testing.T) {
			q := newAdmissionQueue(t, 65536)
			s := admissionFixture(t, q, "cpu")
			obs := admissionObservations(t, 128, "cpu")
			b := telemetry.CollectionBatch{SchemaVersion: 1, SourceType: "AGENT", TenantID: testIdent().TenantID, AgentID: testIdent().AgentID, HostID: testIdent().HostID, Hostname: testIdent().Hostname, OS: testIdent().OS, Arch: testIdent().Arch, CollectedAt: obs[0].ObservedAt, DurationMs: 17, Observations: obs}
			// Controlled error summaries fill the valid existing envelope to an exact
			// byte boundary. Each summary remains within the producer's 256-byte cap.
			for {
				b.Errors = append(b.Errors, telemetry.CollectorError{Collector: "cpu", Code: "FIXTURE"})
				encoded, err := json.Marshal(b)
				if err != nil {
					t.Fatal(err)
				}
				gap := target - len(encoded)
				if gap < 0 {
					t.Fatalf("fixture overshot %d", gap)
				}
				if gap <= 256 {
					b.Errors[len(b.Errors)-1].Summary = strings.Repeat("x", gap)
					break
				}
				// Leave room for another complete error, avoiding a negative final gap.
				fill := 256
				if gap-256 < 100 {
					fill = gap - 100
				}
				b.Errors[len(b.Errors)-1].Summary = strings.Repeat("x", fill)
			}
			encoded, _ := json.Marshal(b)
			if len(encoded) != target {
				t.Fatalf("size=%d", len(encoded))
			}
			t.Logf("ACTUAL_ENCODED_BYTES=%d", len(encoded))
			depositObservations(s, "cpu", obs, b.Errors)
			s.flushPending()
			records, got, errs := readCollections(t, q)
			expectedRecords := 1
			if target > 65536 {
				expectedRecords = 2
			}
			if len(records) != expectedRecords || !reflect.DeepEqual(obs, got) || !reflect.DeepEqual(b.Errors, errs) {
				t.Fatalf("fragment preservation records=%d", len(records))
			}
			if s.Status().Admission.DroppedObservations+s.Status().Admission.DroppedErrors != 0 {
				t.Fatal("unexpected loss")
			}
		})
	}
}

func TestIssue302MulticollectorAndErrorOnlyFragments(t *testing.T) {
	for _, onlyErrors := range []bool{false, true} {
		t.Run(fmt.Sprint(onlyErrors), func(t *testing.T) {
			q := newAdmissionQueue(t, 1024)
			s := admissionFixture(t, q, "cpu", "mem")
			var wantObs []telemetry.Observation
			var wantErrs []telemetry.CollectorError
			for _, name := range []string{"cpu", "mem"} {
				obs := admissionObservations(t, 192, name)
				if onlyErrors {
					obs = nil
				}
				errs := []telemetry.CollectorError{{Collector: name, Code: "COLLECTION_FAILED", Summary: strings.Repeat("e", 256)}, {Collector: name, Code: "TIMEOUT", Summary: strings.Repeat("e", 256)}}
				depositObservations(s, name, obs, errs)
				wantObs = append(wantObs, obs...)
				wantErrs = append(wantErrs, errs...)
			}
			s.flushPending()
			records, got, errs := readCollections(t, q)
			if len(records) < 2 || !reflect.DeepEqual(got, wantObs) || !reflect.DeepEqual(errs, wantErrs) {
				t.Fatalf("content mismatch records=%d obs=%d errors=%d", len(records), len(got), len(errs))
			}
		})
	}
}

type failingAdmissionQueue struct {
	q                *queue.DurableQueue
	failAt, calls    int
	err              error
	entered, release chan struct{}
}

func (f *failingAdmissionQueue) MaxRecordBytes() int { return f.q.MaxRecordBytes() }
func (f *failingAdmissionQueue) Enqueue(p []byte) (*queue.Record, error) {
	f.calls++
	if f.calls == 1 && f.entered != nil {
		close(f.entered)
		<-f.release
	}
	if f.calls == f.failAt {
		return nil, f.err
	}
	return f.q.Enqueue(p)
}
func TestIssue302FirstMiddleFinalFailureRetry(t *testing.T) {
	for _, failAt := range []int{1, 3, 5} {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			q := newAdmissionQueue(t, 700)
			f := &failingAdmissionQueue{q: q, failAt: failAt, err: queue.ErrQueueFull}
			s := admissionFixture(t, f, "cpu")
			obs := admissionObservations(t, 5, "cpu")
			depositObservations(s, "cpu", obs, nil)
			s.flushPending()
			st := s.Status().Admission
			if int(st.DurableObservations) != failAt-1 || st.BacklogObservations != 6-failAt || f.calls != failAt {
				t.Fatalf("partial admission: %+v calls=%d", st, f.calls)
			}
			s.flushPending()
			_, got, _ := readCollections(t, q)
			if !reflect.DeepEqual(obs, got) || s.Status().Admission.BacklogFragments != 0 {
				t.Fatal("retry lost/duplicated/reordered observations")
			}
		})
	}
}

func TestIssue302ConcurrentFlushOrder(t *testing.T) {
	q := newAdmissionQueue(t, 700)
	f := &failingAdmissionQueue{q: q, entered: make(chan struct{}), release: make(chan struct{})}
	s := admissionFixture(t, f, "cpu")
	first := admissionObservations(t, 3, "cpu")
	second := admissionObservations(t, 2, "cpu")
	for i := range second {
		second[i].ObservationID += "-later"
	}
	depositObservations(s, "cpu", first, nil)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.flushPending() }()
	<-f.entered
	depositObservations(s, "cpu", second, nil)
	go func() { defer wg.Done(); s.flushPending() }()
	close(f.release)
	wg.Wait()
	_, got, _ := readCollections(t, q)
	if !reflect.DeepEqual(got, append(first, second...)) {
		t.Fatal("flush admission reordered")
	}
}

func TestIssue302UncertainAdmissionNeverRetried(t *testing.T) {
	for _, d := range []queue.AdmissionDisposition{queue.NotAdmitted, queue.DurablyAdmitted, queue.AdmissionUncertain} {
		t.Run(string(d), func(t *testing.T) {
			q := newAdmissionQueue(t, 700)
			f := &failingAdmissionQueue{q: q, failAt: 1, err: &queue.AdmissionError{Disposition: d, RecoveryRequired: true, Err: queue.ErrMetaUncertain}}
			s := admissionFixture(t, f, "cpu")
			depositObservations(s, "cpu", admissionObservations(t, 3, "cpu"), nil)
			s.flushPending()
			s.flushPending()
			st := s.Status().Admission
			if f.calls != 1 || !st.RecoveryRequired {
				t.Fatalf("blind retry: calls=%d %+v", f.calls, st)
			}
			if d == queue.DurablyAdmitted {
				if st.DurableObservations != 1 || st.BacklogObservations != 2 {
					t.Fatalf("custody: %+v", st)
				}
			} else if st.DurableObservations != 0 || st.BacklogObservations != 3 {
				t.Fatalf("false durable receipt: %+v", st)
			}
			if err := s.StopContext(context.Background()); !errors.Is(err, ErrPendingAdmission) {
				t.Fatalf("shutdown hid recovery: %v", err)
			}
		})
	}
	q := newAdmissionQueue(t, 700)
	f := &failingAdmissionQueue{q: q, failAt: 1, err: errors.New("unknown persistence failure")}
	s := admissionFixture(t, f, "cpu")
	depositObservations(s, "cpu", admissionObservations(t, 1, "cpu"), nil)
	s.flushPending()
	s.flushPending()
	if f.calls != 1 || s.Status().Admission.UncertainObservations != 1 {
		t.Fatal("unknown error retried")
	}
}

func TestIssue302CapacityBacklogShutdownAndBounds(t *testing.T) {
	q, err := queue.NewDurableQueue(t.TempDir(), 1, 1<<20, 65536)
	if err != nil {
		t.Fatal(err)
	}
	s := admissionFixture(t, q, "cpu")
	total := 0
	for i := 0; i < 24; i++ {
		obs := admissionObservations(t, 128, "cpu")
		for j := range obs {
			obs[j].ObservationID += fmt.Sprint(i)
		}
		total += len(obs)
		depositObservations(s, "cpu", obs, nil)
		s.flushPending()
	}
	st := s.Status().Admission
	if st.BacklogBytes > maxAdmissionBacklogBytes || st.BacklogFragments > maxAdmissionBacklogFragments || st.DroppedObservations == 0 {
		t.Fatalf("bound missing: %+v", st)
	}
	if int(st.DurableObservations+st.DroppedObservations)+st.BacklogObservations != total {
		t.Fatalf("loss accounting total=%d %+v", total, st)
	}
	if err := s.StopContext(context.Background()); !errors.Is(err, ErrPendingAdmission) {
		t.Fatalf("false clean shutdown: %v", err)
	}
}

func TestIssue302OversizedAndInvalidItemDoNotStarveValid(t *testing.T) {
	q := newAdmissionQueue(t, 700)
	s := admissionFixture(t, q, "cpu")
	obs := admissionObservations(t, 3, "cpu")
	obs[0].Labels["oversized"] = strings.Repeat("x", 1000)
	obs[1].Value = math.NaN()
	errs := []telemetry.CollectorError{{Collector: "cpu", Code: "TOO_LARGE", Summary: strings.Repeat("e", 1000)}, {Collector: "cpu", Code: "VALID", Summary: "retained"}}
	depositObservations(s, "cpu", obs, errs)
	s.flushPending()
	_, got, gotErrors := readCollections(t, q)
	if len(got) != 1 || got[0].ObservationID != obs[2].ObservationID || len(gotErrors) != 1 || gotErrors[0].Code != "VALID" {
		t.Fatal("valid siblings starved")
	}
	st := s.Status().Admission
	if st.DroppedObservations != 2 || st.DroppedErrors != 1 {
		t.Fatalf("loss accounting %+v", st)
	}
}

func TestIssue302DeliveryEnvelopeBudgetAndReplay(t *testing.T) {
	q := newAdmissionQueue(t, 65536)
	s := admissionFixture(t, q, "cpu")
	obs := admissionObservations(t, 192, "cpu")
	depositObservations(s, "cpu", obs, nil)
	s.flushPending()
	records, _, _ := readCollections(t, q)
	env := client.DeliveryEnvelope{SchemaVersion: 2, AgentID: testIdent().AgentID, TenantID: testIdent().TenantID, CorrelationID: "corr-fixture", AgentStreamID: q.AgentStreamID(), BatchID: client.BuildLineageBatchID(q.AgentStreamID(), testIdent().AgentID, records), FirstSequence: 1, LastSequence: uint64(len(records)), RecordCount: len(records)}
	for _, r := range records {
		env.Records = append(env.Records, client.DeliveryRecord{Sequence: r.Sequence, ID: r.ID, CreatedAt: r.CreatedAt, Checksum: r.Checksum, Payload: json.RawMessage(r.Payload)})
	}
	encoded, err := json.Marshal(env)
	if err != nil || len(encoded) > client.SensorReceiverMaxBodyBytes {
		t.Fatalf("wire budget %d: %v", len(encoded), err)
	}
	replay, err := q.ReadBatch(100, 384*1024)
	if err != nil || !reflect.DeepEqual(replay, records) {
		t.Fatal("replay changed")
	}
	if err := q.CommitAccepted(records, 1); err != nil {
		t.Fatal(err)
	}
	tail, err := q.ReadBatch(100, 384*1024)
	if err != nil || len(tail) != len(records)-1 || tail[0].Sequence != 2 {
		t.Fatalf("partial ACK %v %v", tail, err)
	}
	t.Logf("delivery_envelope_bytes=%d queue_records=%d", len(encoded), len(records))
}

func TestIssue302DiskByteBudgetDoesNotStrandLargeFragment(t *testing.T) {
	q, err := queue.NewDurableQueue(t.TempDir(), 100, 2048, 65536)
	if err != nil {
		t.Fatal(err)
	}
	s := admissionFixture(t, q, "cpu")
	obs := admissionObservations(t, 5, "cpu")
	depositObservations(s, "cpu", obs, nil)
	s.flushPending()
	if s.Status().Admission.DurableFragments == 0 {
		t.Fatal("first fragment cannot fit empty queue")
	}
	for s.Status().Admission.BacklogFragments > 0 {
		records, err := q.ReadBatch(100, 1<<20)
		if err != nil || len(records) == 0 {
			t.Fatalf("stranded backlog: %v", err)
		}
		if err := q.CommitAccepted(records, records[len(records)-1].Sequence); err != nil {
			t.Fatal(err)
		}
		s.flushPending()
	}
	st := s.Status().Admission
	if st.DurableObservations != 5 || st.DroppedObservations != 0 {
		t.Fatalf("lost under disk pressure: %+v", st)
	}
}

func TestIssue302PendingByteBoundAccountsRejectedContribution(t *testing.T) {
	q := newAdmissionQueue(t, 65536)
	s := admissionFixture(t, q, "cpu")
	obs := admissionObservations(t, 2, "cpu")
	obs[0].Labels["fixture"] = strings.Repeat("x", maxPendingContributionBytes)
	depositObservations(s, "cpu", obs, nil)
	if s.PendingItemCountForTest() != 1 || s.Status().Admission.DroppedObservations != 1 {
		t.Fatal("unbounded deposit or missing loss")
	}
	valid := admissionObservations(t, 1, "cpu")
	depositObservations(s, "cpu", valid, nil)
	s.flushPending()
	_, got, _ := readCollections(t, q)
	if !reflect.DeepEqual(append(obs[1:], valid...), got) {
		t.Fatal("large rejected contribution starved sibling or later data")
	}
}

func TestIssue302SustainedPressureRemainsBounded(t *testing.T) {
	enq := &mockEnqueuer{err: queue.ErrQueueFull}
	s := admissionFixture(t, enq, "cpu")
	obs := admissionObservations(t, 192, "cpu")
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	goroutines := runtime.NumGoroutine()
	started := time.Now()
	const cycles = 256
	for i := 0; i < cycles; i++ {
		depositObservations(s, "cpu", obs, nil)
		s.flushPending()
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	st := s.Status().Admission
	if st.BacklogBytes > maxAdmissionBacklogBytes || st.BacklogFragments > maxAdmissionBacklogFragments || st.FailedAttempts != cycles {
		t.Fatalf("unbounded retry/backlog %+v", st)
	}
	if st.DurableObservations+st.DroppedObservations+uint64(st.BacklogObservations) != cycles*192 {
		t.Fatalf("accounting %+v", st)
	}
	if runtime.NumGoroutine() > goroutines+2 {
		t.Fatal("pressure accumulated goroutines")
	}
	t.Logf("cycles=%d input_observations=%d elapsed_ms=%d heap_before=%d heap_after=%d goroutines_before=%d goroutines_after=%d backlog_bytes=%d dropped=%d failed_attempts=%d", cycles, cycles*192, time.Since(started).Milliseconds(), before.HeapAlloc, after.HeapAlloc, goroutines, runtime.NumGoroutine(), st.BacklogBytes, st.DroppedObservations, st.FailedAttempts)
}

func TestIssue302WriteRejectionHealthStateAndRecovery(t *testing.T) {
	for _, cause := range []error{syscall.ENOSPC, syscall.EROFS, syscall.EIO} {
		t.Run(cause.Error(), func(t *testing.T) {
			q := newAdmissionQueue(t, 700)
			f := &failingAdmissionQueue{q: q, failAt: 1, err: &queue.AdmissionError{Disposition: queue.NotAdmitted, Err: cause}}
			s := admissionFixture(t, f, "cpu")
			depositObservations(s, "cpu", admissionObservations(t, 1, "cpu"), nil)
			s.flushPending()
			if st := s.Status().Admission; !st.WriteBlocked || st.BacklogObservations != 1 || st.DurableObservations != 0 || st.RecoveryRequired {
				t.Fatalf("local write failure masked: %+v", st)
			}
			s.flushPending()
			if st := s.Status().Admission; st.WriteBlocked || st.BacklogObservations != 0 || st.DurableObservations != 1 {
				t.Fatalf("successful retry did not clear write block: %+v", st)
			}
		})
	}
	t.Run("serialized_byte_capacity", func(t *testing.T) {
		q, err := queue.NewDurableQueue(t.TempDir(), 100, 2000, 700)
		if err != nil {
			t.Fatal(err)
		}
		s := admissionFixture(t, q, "cpu")
		depositObservations(s, "cpu", admissionObservations(t, 3, "cpu"), nil)
		s.flushPending()
		if st := s.Status().Admission; !st.WriteBlocked || st.BacklogObservations == 0 || q.Stats().AtCapacity {
			t.Fatalf("byte rejection below exact capacity not exposed: %+v queue=%+v", st, q.Stats())
		}
		records, err := q.ReadBatch(100, 10000)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) == 0 {
			t.Fatal("capacity fixture admitted no prefix")
		}
		if err := q.CommitAccepted(records, records[len(records)-1].Sequence); err != nil {
			t.Fatal(err)
		}
		s.flushPending()
		if st := s.Status().Admission; st.WriteBlocked || st.BacklogObservations != 0 || st.DurableObservations != 3 {
			t.Fatalf("capacity recovery failed: %+v", st)
		}
	})
}

func TestIssue302DroppedContributionCannotAnchorCycle(t *testing.T) {
	q := newAdmissionQueue(t, 65536)
	s := admissionFixture(t, q, "cpu")
	valid := admissionObservations(t, 1, "cpu")
	invalid := valid[0]
	invalid.Value = math.NaN()
	s.deposit(pendingContribution{name: "cpu", collectedAt: valid[0].ObservedAt.Add(-time.Hour), observations: []telemetry.Observation{invalid}})
	if len(s.pending) != 0 || !s.cycleStart.IsZero() {
		t.Fatal("dropped contribution retained cycle metadata")
	}
	depositObservations(s, "cpu", valid, nil)
	s.flushPending()
	_, got, _ := readCollections(t, q)
	if !reflect.DeepEqual(valid, got) || s.Status().Admission.DroppedObservations != 1 {
		t.Fatal("valid contribution metadata or loss accounting changed")
	}
}

func TestIssue302InFlightFragmentCustodySnapshot(t *testing.T) {
	q := newAdmissionQueue(t, 700)
	f := &failingAdmissionQueue{q: q, entered: make(chan struct{}), release: make(chan struct{})}
	s := admissionFixture(t, f, "cpu")
	depositObservations(s, "cpu", admissionObservations(t, 1, "cpu"), nil)
	done := make(chan struct{})
	go func() { s.flushPending(); close(done) }()
	<-f.entered
	st := s.Status().Admission
	close(f.release)
	<-done
	if st.BacklogObservations != 1 || st.BacklogFragments != 1 || st.DurableObservations != 0 {
		t.Fatalf("in-flight serialized custody omitted: %+v", st)
	}
	st = s.Status().Admission
	if st.BacklogObservations != 0 || st.BacklogFragments != 0 || st.DurableObservations != 1 {
		t.Fatalf("completed custody double-counted: %+v", st)
	}
}
