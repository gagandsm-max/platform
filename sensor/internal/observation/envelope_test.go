package observation

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestEnvelopeStrictBoundedRoundTrip(t *testing.T) {
	e := Envelope{SchemaVersion: SchemaVersion, BatchID: "b", RecordType: "SNMP_OBSERVATION_BATCH", CollectedAt: time.Now().UTC(), TenantID: "t", Job: JobCorrelation{JobID: "j", AssignmentKey: "a", AssignmentVersion: 1}, Protocol: "SNMP", ProtocolVersion: "2c", Operation: "GET", Status: "COLLECTED", Quality: "GOOD", ProtocolSchemaVersion: 1, ProtocolPayload: json.RawMessage(`{"ok":true}`)}
	b, err := EncodeBounded(context.Background(), e, 4096)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeBounded(b, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if got.BatchID != e.BatchID {
		t.Fatal("roundtrip failed")
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m["unknown"] = "x"
	bad, _ := json.Marshal(m)
	if _, err := DecodeBounded(bad, 4096); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := EncodeBounded(context.Background(), e, 10); err == nil {
		t.Fatal("bound not enforced")
	}
}
