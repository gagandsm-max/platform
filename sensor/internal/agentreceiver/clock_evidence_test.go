package agentreceiver

import (
	"context"
	"errors"
	"testing"
	"time"

	"platform/sensor/internal/agentacceptance"
	"platform/sensor/internal/spool"
)

type fixedClockEvidenceProvider struct{ sample ClockSample }

func (p fixedClockEvidenceProvider) Sample(time.Time) ClockSample { return p.sample }

func TestSensorAcceptanceEvidencePersistsQualifiedAndUnqualifiedClockProvenance(t *testing.T) {
	for _, tc := range []struct {
		name   string
		sample ClockSample
	}{
		{"qualified", func() ClockSample {
			errorMs, syncAgeMs := int64(37), int64(240)
			return ClockSample{Quality: "QUALIFIED", MaxErrorMs: &errorMs,
				SyncAgeMs: &syncAgeMs, Source: "TEST_KERNEL_SYNC"}
		}()},
		{"unqualified", unqualifiedClock("TEST_SYNC_UNPROVEN")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sp, _, dir := openTestSink(t)
			defer sp.Close()
			sink, err := NewSpoolAcceptanceSink(sp, dir,
				WithClockEvidenceProvider(fixedClockEvidenceProvider{tc.sample}),
				WithServiceInstanceID("test-sensor-incarnation"))
			if err != nil {
				t.Fatal(err)
			}
			env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r1", []byte(`{"v":1}`)))
			stampEnvelope(&env)
			if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
				t.Fatal(err)
			}
			batch, err := sp.ReadOrReserveOutboundBatch(1, 1024*1024)
			if err != nil || len(batch.Records) != 1 {
				t.Fatalf("spool read: records=%d err=%v", len(batch.Records), err)
			}
			guard, err := agentacceptance.Lock(dir)
			if err != nil {
				t.Fatal(err)
			}
			persisted, err := guard.Resolve(batch.Records[0], sp.StreamID())
			_ = guard.Close()
			if err != nil {
				t.Fatal(err)
			}
			evidence := persisted.Evidence
			if evidence.EvidenceVersion != 1 || evidence.ClockQuality != tc.sample.Quality ||
				evidence.ClockSource != tc.sample.Source ||
				evidence.SensorServiceInstanceID != "test-sensor-incarnation" ||
				evidence.AcceptedAtUTC != persisted.AcceptedAtUTC {
				t.Fatalf("durable Sensor acceptance evidence mismatch: %+v", evidence)
			}
			if tc.sample.Quality == "QUALIFIED" {
				if evidence.ClockMaxErrorMs == nil || *evidence.ClockMaxErrorMs != 37 ||
					evidence.ClockSyncAgeMs == nil || *evidence.ClockSyncAgeMs != 240 {
					t.Fatalf("qualified bounds missing: %+v", evidence)
				}
			} else if evidence.ClockMaxErrorMs != nil || evidence.ClockSyncAgeMs != nil {
				t.Fatalf("unqualified evidence fabricated bounds: %+v", evidence)
			}
		})
	}
}

func TestAcceptanceEvidenceFollowsDurableAppendAndRevocationRace(t *testing.T) {
	sp, _, dir := openTestSink(t)
	defer sp.Close()
	sink, err := NewSpoolAcceptanceSink(sp, dir,
		WithClockEvidenceProvider(fixedClockEvidenceProvider{ClockSample{Quality: "QUALIFIED", Source: "TEST_SYNC"}}))
	if err != nil {
		t.Fatal(err)
	}
	var revokedAt, appendCompleted time.Time
	sink.SetAppendHookForTest(func(DeliveryEnvelope) error { revokedAt = time.Now().UTC(); return nil })
	sink.SetPostAppendHookForTest(func(spool.Record) error { appendCompleted = time.Now().UTC(); return nil })
	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&env)
	if _, err := sink.AcceptBatch(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	batch, err := sp.ReadOrReserveOutboundBatch(1, 1024*1024)
	if err != nil || len(batch.Records) != 1 {
		t.Fatalf("spool read: %v", err)
	}
	guard, err := agentacceptance.Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := guard.Resolve(batch.Records[0], sp.StreamID())
	_ = guard.Close()
	if err != nil {
		t.Fatal(err)
	}
	acceptedAt, err := time.Parse(time.RFC3339Nano, entry.AcceptedAtUTC)
	if err != nil {
		t.Fatal(err)
	}
	if acceptedAt.Before(revokedAt) || acceptedAt.Before(appendCompleted) {
		t.Fatalf("acceptance predates revocation/append: accepted=%v revoked=%v durable=%v",
			acceptedAt, revokedAt, appendCompleted)
	}
	t.Logf("revocation=%s durable_append_completed=%s accepted_at=%s spool_sequence=%d",
		revokedAt.Format(time.RFC3339Nano), appendCompleted.Format(time.RFC3339Nano),
		acceptedAt.Format(time.RFC3339Nano), batch.Records[0].Sequence)
}

func TestCrashAfterAppendRetainsBatchAndRecoversOnlyUnqualifiedEvidence(t *testing.T) {
	sp, _, dir := openTestSink(t)
	sink, err := NewSpoolAcceptanceSink(sp, dir)
	if err != nil {
		t.Fatal(err)
	}
	sink.SetPostAppendHookForTest(func(spool.Record) error { return errors.New("injected crash window") })
	env := sampleEnvelope("agent-a", "tenant-1", sampleRecord(1, "r1", []byte(`{"v":1}`)))
	stampEnvelope(&env)
	if _, err := sink.AcceptBatch(context.Background(), env); err == nil {
		t.Fatal("expected post-append failure")
	}
	if err := sp.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := spool.Open(spool.Config{Dir: dir, MaxRecordBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if _, err := NewSpoolAcceptanceSink(restarted, dir); err != nil {
		t.Fatal(err)
	}
	batch, err := restarted.ReadOrReserveOutboundBatch(1, 1024*1024)
	if err != nil || len(batch.Records) != 1 {
		t.Fatalf("durable telemetry lost: %v", err)
	}
	guard, err := agentacceptance.Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := guard.Resolve(batch.Records[0], restarted.StreamID())
	_ = guard.Close()
	if err != nil {
		t.Fatal(err)
	}
	if entry.Evidence.ClockQuality != "UNQUALIFIED" || entry.Evidence.ClockSource != "POST_CRASH_RECOVERY" {
		t.Fatalf("recovery fabricated qualified evidence: %+v", entry.Evidence)
	}
	t.Logf("recovered_spool_sequence=%d clock_quality=%s source=%s",
		batch.Records[0].Sequence, entry.Evidence.ClockQuality, entry.Evidence.ClockSource)
}
