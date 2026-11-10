package agentreceiver

import (
	"context"
	"encoding/json"
	"testing"
	"time"
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
			var persisted durableBatchPayload
			if err := json.Unmarshal(batch.Records[0].Payload, &persisted); err != nil {
				t.Fatal(err)
			}
			evidence := persisted.AcceptanceEvidence
			if evidence.EvidenceVersion != 1 || evidence.ClockQuality != tc.sample.Quality ||
				evidence.ClockSource != tc.sample.Source ||
				evidence.SensorServiceInstanceID != "test-sensor-incarnation" ||
				evidence.AcceptedAtUTC != persisted.Timestamp.UTC().Format(time.RFC3339Nano) {
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
