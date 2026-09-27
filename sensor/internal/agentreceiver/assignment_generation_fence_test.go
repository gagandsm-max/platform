package agentreceiver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"platform/sensor/internal/runtime"
	"platform/sensor/internal/spool"
)

func rewriteOwnerGeneration(t *testing.T, spoolDir, tenantID, agentID string, generation int64) {
	t.Helper()
	path := filepath.Join(spoolDir, streamStateDirName, lineageOwnerDirName, sourceRangeStreamKey(tenantID, agentID)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var owner LineageOwnerRecord
	if err := decodeStrictJSONObject(data, &owner); err != nil {
		t.Fatal(err)
	}
	owner.ActivatedUnderAssignmentGeneration = generation
	out, err := json.Marshal(owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o640); err != nil {
		t.Fatal(err)
	}
}

func reopenSink(t *testing.T, s *spool.DiskSpool, stateDir string) *SpoolAcceptanceSink {
	t.Helper()
	sink, err := NewSpoolAcceptanceSink(s, stateDir)
	if err != nil {
		t.Fatal(err)
	}
	return sink
}

func TestAssignmentFenceFirstOwnerPersistsPositiveGeneration(t *testing.T) {
	s, sink, dir := openTestSink(t)
	defer s.Close()
	env := sampleV2Envelope("agent-a", "tenant-1", testStreamA(), sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&env)
	env.AssignmentGeneration = 7
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	if got := sink.OwnerAssignmentGenerationForTest("tenant-1", "agent-a"); got != 7 {
		t.Fatalf("owner generation=%d", got)
	}
	// Restart reload
	sink2 := reopenSink(t, s, dir)
	if got := sink2.OwnerAssignmentGenerationForTest("tenant-1", "agent-a"); got != 7 {
		t.Fatalf("reloaded owner generation=%d", got)
	}
}

func TestAssignmentFencePreFenceGenerationZeroLoadsAndPromotes(t *testing.T) {
	s, sink, dir := openTestSink(t)
	defer s.Close()
	env := sampleV2Envelope("agent-a", "tenant-1", testStreamA(), sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&env)
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	rewriteOwnerGeneration(t, dir, "tenant-1", "agent-a", 0)
	sink2 := reopenSink(t, s, dir)
	if got := sink2.OwnerAssignmentGenerationForTest("tenant-1", "agent-a"); got != 0 {
		t.Fatalf("pre-fence owner must load with generation 0, got %d", got)
	}
	before, _ := s.Stats()
	next := sampleV2Envelope("agent-a", "tenant-1", testStreamA(), sampleRecord(2, "r2", []byte(`{"v":2}`)))
	stampEnvelope(&next)
	next.AssignmentGeneration = 5
	next.BatchID = "batch-promote"
	if _, err := sink2.AcceptBatch(context.Background(), next); err != nil {
		t.Fatal(err)
	}
	if got := sink2.OwnerAssignmentGenerationForTest("tenant-1", "agent-a"); got != 5 {
		t.Fatalf("owner must promote 0→5 before acceptance, got %d", got)
	}
	after, _ := s.Stats()
	if after.RecordCount != before.RecordCount+1 {
		t.Fatalf("expected one new spool record, before=%d after=%d", before.RecordCount, after.RecordCount)
	}
}

func TestAssignmentFenceCorruptOwnerGenerationFailsLoad(t *testing.T) {
	s, sink, dir := openTestSink(t)
	defer s.Close()
	env := sampleV2Envelope("agent-a", "tenant-1", testStreamA(), sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&env)
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}

	t.Run("negative", func(t *testing.T) {
		rewriteOwnerGeneration(t, dir, "tenant-1", "agent-a", -1)
		if _, err := NewSpoolAcceptanceSink(s, dir); err == nil || !strings.Contains(err.Error(), ErrCodeStreamOwnerCorrupt) {
			t.Fatalf("expected corrupt owner load failure, got %v", err)
		}
	})
	t.Run("above_max_safe", func(t *testing.T) {
		rewriteOwnerGeneration(t, dir, "tenant-1", "agent-a", MAX_SAFE_SEQUENCE+1)
		if _, err := NewSpoolAcceptanceSink(s, dir); err == nil || !strings.Contains(err.Error(), ErrCodeStreamOwnerCorrupt) {
			t.Fatalf("expected corrupt owner load failure, got %v", err)
		}
	})
}

func TestAssignmentFenceIncomingGenerationBounds(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()
	for _, gen := range []int64{0, MAX_SAFE_SEQUENCE + 1} {
		env := sampleV2Envelope("agent-a", "tenant-1", testStreamA(), sampleRecord(1, "r1", []byte(`{"v":1}`)))
		stampEnvelope(&env)
		env.AssignmentGeneration = gen
		before, _ := s.Stats()
		_, err := sink.AcceptBatch(context.Background(), env)
		if err == nil || !strings.Contains(err.Error(), ErrCodeInvalidField) {
			t.Fatalf("gen=%d expected invalid-field rejection, got %v", gen, err)
		}
		after, _ := s.Stats()
		if after.RecordCount != before.RecordCount {
			t.Fatalf("gen=%d must not mutate spool", gen)
		}
		if sink.OwnerAssignmentGenerationForTest("tenant-1", "agent-a") != -1 {
			t.Fatalf("gen=%d must not create owner", gen)
		}
	}
}

func TestAssignmentFenceEqualHigherLowerAndRestart(t *testing.T) {
	s, sink, dir := openTestSink(t)
	defer s.Close()
	stream := testStreamA()
	first := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	first.AssignmentGeneration = 3
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	equal := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(2, "r2", []byte(`{"v":2}`)))
	stampEnvelope(&equal)
	equal.AssignmentGeneration = 3
	equal.BatchID = "eq"
	if _, err := sink.AcceptBatch(context.Background(), equal); err != nil {
		t.Fatal(err)
	}
	if sink.OwnerAssignmentGenerationForTest("tenant-1", "agent-a") != 3 {
		t.Fatal("equal generation must leave owner unchanged")
	}

	higher := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(3, "r3", []byte(`{"v":3}`)))
	stampEnvelope(&higher)
	higher.AssignmentGeneration = 8
	higher.BatchID = "hi"
	if _, err := sink.AcceptBatch(context.Background(), higher); err != nil {
		t.Fatal(err)
	}
	if sink.OwnerAssignmentGenerationForTest("tenant-1", "agent-a") != 8 {
		t.Fatal("higher generation must advance owner before acceptance")
	}

	before, _ := s.Stats()
	wmBefore := sink.ContiguousWatermarkForStream("tenant-1", "agent-a", stream)
	lower := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(4, "r4", []byte(`{"v":4}`)))
	stampEnvelope(&lower)
	lower.AssignmentGeneration = 7
	lower.BatchID = "lo"
	_, err := sink.AcceptBatch(context.Background(), lower)
	if !IsAssignmentGenerationStale(err) {
		t.Fatalf("expected stale generation error, got %v", err)
	}
	after, _ := s.Stats()
	if after.RecordCount != before.RecordCount {
		t.Fatal("stale generation must not append spool")
	}
	if sink.ContiguousWatermarkForStream("tenant-1", "agent-a", stream) != wmBefore {
		t.Fatal("stale generation must not mutate watermark")
	}
	if sink.OwnerAssignmentGenerationForTest("tenant-1", "agent-a") != 8 {
		t.Fatal("stale generation must not change owner")
	}

	sink2 := reopenSink(t, s, dir)
	if sink2.OwnerAssignmentGenerationForTest("tenant-1", "agent-a") != 8 {
		t.Fatal("restart must reload highest generation")
	}
	_, err = sink2.AcceptBatch(context.Background(), lower)
	if !IsAssignmentGenerationStale(err) {
		t.Fatalf("lower generation must remain rejected after restart, got %v", err)
	}
}

func TestAssignmentFenceOwnerPublicationFailurePreventsAcceptance(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()
	stream := testStreamA()
	first := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	first.AssignmentGeneration = 2
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	sink.SetOwnerPublisherForTest(func(dir, path, tmpPattern string, v any) error {
		return errors.New("injected owner publication failure")
	})
	before, _ := s.Stats()
	higher := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(2, "r2", []byte(`{"v":2}`)))
	stampEnvelope(&higher)
	higher.AssignmentGeneration = 4
	higher.BatchID = "fail-pub"
	_, err := sink.AcceptBatch(context.Background(), higher)
	if err == nil {
		t.Fatal("expected publication failure")
	}
	after, _ := s.Stats()
	if after.RecordCount != before.RecordCount {
		t.Fatal("failed owner publication must not accept batch")
	}
	if !sink.ReconcileRequiredForTest() {
		t.Fatal("reconcileRequired must be set")
	}
	if sink.OwnerAssignmentGenerationForTest("tenant-1", "agent-a") != 2 {
		t.Fatal("in-memory owner must not advance on failed publish")
	}
}

func TestAssignmentFenceExactReplayUnderHigherAndLowerGeneration(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()
	stream := testStreamA()
	env := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(1, "r1", []byte(`{"exact":true}`)))
	stampEnvelope(&env)
	env.AssignmentGeneration = 2
	env.BatchID = "exact-replay"
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Stats()

	// Exact durable replay under higher generation: advance owner, no duplicate append.
	replayHigher := env
	replayHigher.AssignmentGeneration = 9
	res, err := sink.AcceptBatch(context.Background(), replayHigher)
	if err != nil {
		t.Fatal(err)
	}
	if res.AcceptedThrough != 1 {
		t.Fatalf("acceptedThrough=%d", res.AcceptedThrough)
	}
	if sink.OwnerAssignmentGenerationForTest("tenant-1", "agent-a") != 9 {
		t.Fatal("exact replay under higher generation must advance owner first")
	}
	after, _ := s.Stats()
	if after.RecordCount != before.RecordCount {
		t.Fatal("exact replay must not duplicate spool append")
	}

	// Exact durable replay under lower generation: stale despite receipt.
	replayLower := env
	replayLower.AssignmentGeneration = 3
	_, err = sink.AcceptBatch(context.Background(), replayLower)
	if !IsAssignmentGenerationStale(err) {
		t.Fatalf("exact replay under lower generation must be stale, got %v", err)
	}
	if sink.OwnerAssignmentGenerationForTest("tenant-1", "agent-a") != 9 {
		t.Fatal("owner must remain at highest generation")
	}
}

func TestAssignmentFenceReplacementAndRetiredStream(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()
	a, b := testStreamA(), testStreamB()
	first := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	first.AssignmentGeneration = 4
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	// Replacement with lower generation rejected.
	staleReplace := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "rb", []byte(`{"v":2}`)))
	stampEnvelope(&staleReplace)
	staleReplace.AssignmentGeneration = 3
	staleReplace.BatchID = "replace-stale"
	if _, err := sink.AcceptBatch(context.Background(), staleReplace); !IsAssignmentGenerationStale(err) {
		t.Fatalf("replacement under lower generation must be stale, got %v", err)
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != a {
		t.Fatal("active stream must remain A")
	}

	// Replacement equal/newer retains transition semantics.
	replace := sampleV2Envelope("agent-a", "tenant-1", b, sampleRecord(1, "rb", []byte(`{"v":2}`)))
	stampEnvelope(&replace)
	replace.AssignmentGeneration = 6
	replace.BatchID = "replace-ok"
	if _, err := sink.AcceptBatch(context.Background(), replace); err != nil {
		t.Fatal(err)
	}
	if sink.ActiveOwnerStreamID("tenant-1", "agent-a") != b {
		t.Fatal("active stream must become B")
	}
	if sink.OwnerAssignmentGenerationForTest("tenant-1", "agent-a") != 6 {
		t.Fatal("replacement owner must persist current generation")
	}

	// Retired stream with newer generation: remember generation, still reject stream.
	retired := sampleV2Envelope("agent-a", "tenant-1", a, sampleRecord(1, "ra", []byte(`{"v":9}`)))
	stampEnvelope(&retired)
	retired.AssignmentGeneration = 10
	retired.BatchID = "retired-newer"
	_, err := sink.AcceptBatch(context.Background(), retired)
	if !IsStreamStaleOrFenced(err) {
		t.Fatalf("retired stream must remain fenced, got %v", err)
	}
	if sink.OwnerAssignmentGenerationForTest("tenant-1", "agent-a") != 10 {
		t.Fatal("newer generation must be durably remembered before retired rejection")
	}
}

func TestAssignmentFenceConcurrentAcceptBatchSerialization(t *testing.T) {
	s, sink, _ := openTestSink(t)
	defer s.Close()
	stream := testStreamA()
	first := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&first)
	first.AssignmentGeneration = 5
	if _, err := sink.AcceptBatch(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var staleHits atomic.Int64
	var okHits atomic.Int64
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			env := sampleV2Envelope("agent-a", "tenant-1", stream, sampleRecord(uint64(2+i), "r", []byte(`{"n":1}`)))
			stampEnvelope(&env)
			env.BatchID = "c-" + string(rune('a'+i))
			if i%2 == 0 {
				env.AssignmentGeneration = 4 // lower
			} else {
				env.AssignmentGeneration = 5 // equal
			}
			_, err := sink.AcceptBatch(context.Background(), env)
			if IsAssignmentGenerationStale(err) {
				staleHits.Add(1)
				return
			}
			if err == nil {
				okHits.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if sink.OwnerAssignmentGenerationForTest("tenant-1", "agent-a") < 5 {
		t.Fatal("concurrent AcceptBatch must not regress owner generation")
	}
	if staleHits.Load() == 0 {
		t.Fatal("expected some stale rejections from lower-generation racers")
	}
}

func TestReceiverMapsAssignmentGenerationStaleTo409(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-a", "agent-a")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.AllowGeneration("agent-a", "sensor-infra-01", "tenant-1", "site-1", 1)

	sink := NewInMemoryAcceptanceSink()
	sink.SetInjectedError(&AssignmentGenerationStaleError{
		TenantID: "tenant-1", AgentID: "agent-a", CurrentGeneration: 2, DurableGeneration: 5,
	})
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)
	srv, client := mockTLSServer(rcv, "agent-a")
	defer srv.Close()
	env := sampleV2Envelope("agent-a", "tenant-1", testStreamA(), sampleRecord(2, "r2", []byte(`{"v":2}`)))
	env.SiteID = "site-1"
	body, _ := json.Marshal(env)
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("HTTP %d", resp.StatusCode)
	}
	var er ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		t.Fatal(err)
	}
	if er.Code != ErrCodeAssignmentGenerationStale {
		t.Fatalf("code=%s", er.Code)
	}
	if rcv.Metrics().Snapshot().AssignmentGenerationStale != 1 {
		t.Fatalf("assignment_generation_stale metric=%d", rcv.Metrics().Snapshot().AssignmentGenerationStale)
	}
}

func TestReceiverAssignmentGenerationFenceCannotBypassViaReplayCache(t *testing.T) {
	ident := NewStaticTestIdentityVerifier()
	ident.AllowCommonName("agent-a", "agent-a")
	auth := NewInMemoryAssignmentAuthorizer()
	auth.AllowGeneration("agent-a", "sensor-infra-01", "tenant-1", "site-1", 2)

	s, sink, _ := openTestSink(t)
	defer s.Close()
	rcv := newTestReceiver(t, runtime.RoleInfrastructure, ident, auth, sink)
	srv, client := mockTLSServer(rcv, "agent-a")
	defer srv.Close()

	env := sampleV2Envelope("agent-a", "tenant-1", testStreamA(), sampleRecord(1, "r1", []byte(`{"v":1}`)))
	env.SiteID = "site-1"
	body, _ := json.Marshal(env)
	resp := postJSON(t, client, srv.URL+EndpointPath, body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first accept HTTP %d", resp.StatusCode)
	}

	auth.AllowGeneration("agent-a", "sensor-infra-01", "tenant-1", "site-1", 1) // Core now resolves lower
	resp2 := postJSON(t, client, srv.URL+EndpointPath, body)
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("replay under lower generation HTTP %d", resp2.StatusCode)
	}
	var er ErrorResponse
	_ = json.NewDecoder(resp2.Body).Decode(&er)
	if er.Code != ErrCodeAssignmentGenerationStale {
		t.Fatalf("code=%s", er.Code)
	}
}

func TestValidateLiveAssignmentGenerationBounds(t *testing.T) {
	if err := validateLiveAssignmentGeneration(1); err != nil {
		t.Fatal(err)
	}
	if err := validateLiveAssignmentGeneration(MAX_SAFE_SEQUENCE); err != nil {
		t.Fatal(err)
	}
	if err := validateLiveAssignmentGeneration(0); err == nil {
		t.Fatal("expected rejection")
	}
	if err := validateLiveAssignmentGeneration(MAX_SAFE_SEQUENCE + 1); err == nil {
		t.Fatal("expected rejection")
	}
}

func TestValidateAssignmentContextGenerationBounds(t *testing.T) {
	err := validateAssignmentContext(AssignmentContext{
		TenantID: "t", SiteID: "s", SensorID: "sensor-infra-01", AgentID: "agent-a",
		Generation: MAX_SAFE_SEQUENCE + 1, Assigned: true,
	}, AuthenticatedAgentIdentity{AgentID: "agent-a"}, "sensor-infra-01")
	if err == nil {
		t.Fatal("expected MAX_SAFE rejection")
	}
	err = validateAssignmentContext(AssignmentContext{
		TenantID: "t", SiteID: "s", SensorID: "sensor-infra-01", AgentID: "agent-a",
		Generation: 0, Assigned: true,
	}, AuthenticatedAgentIdentity{AgentID: "agent-a"}, "sensor-infra-01")
	if err == nil {
		t.Fatal("expected generation 0 rejection")
	}
	if err := validateAssignmentContext(AssignmentContext{
		TenantID: "t", SiteID: "s", SensorID: "sensor-infra-01", AgentID: "agent-a",
		Generation: 3, Assigned: true,
	}, AuthenticatedAgentIdentity{AgentID: "agent-a"}, "sensor-infra-01"); err != nil {
		t.Fatal(err)
	}
}
