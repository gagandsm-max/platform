package telemetry

import (
	"math"
	"testing"
	"time"

	"platform/agent/common/identity"
)

func TestTelemetryNormalizeSuccess(t *testing.T) {
	ident := &identity.AgentIdentity{
		AgentID:  "agent-01",
		TenantID: "tenant-01",
		HostID:   "host-01",
		Hostname: "test-node",
		OS:       "linux",
		Arch:     "amd64",
	}

	now := time.Now()
	sample := Sample{
		MetricKey: "system.cpu.utilization",
		Value:     42.5,
		Unit:      "%",
		Labels: map[string]string{
			"core": "0",
		},
	}

	obs, err := Normalize(ident, "cpu_collector", now, sample)
	if err != nil {
		t.Fatalf("unexpected normalization error: %v", err)
	}

	if obs.AgentID != "agent-01" || obs.TenantID != "tenant-01" || obs.HostID != "host-01" {
		t.Fatalf("identity context mismatch: %+v", obs)
	}
	if obs.MetricKey != "system.cpu.utilization" || obs.Value != 42.5 || obs.Unit != "%" {
		t.Fatalf("sample data mismatch: %+v", obs)
	}
	if obs.Labels["core"] != "0" {
		t.Fatalf("expected label core=0, got %v", obs.Labels)
	}
	if obs.ObservedAt != now.UTC() {
		t.Fatalf("expected UTC timestamp: %v vs %v", obs.ObservedAt, now.UTC())
	}
}

func TestTelemetryNormalizeRejections(t *testing.T) {
	validIdent := &identity.AgentIdentity{
		AgentID:  "agent-01",
		TenantID: "tenant-01",
		HostID:   "host-01",
	}

	// 1. Missing identity fields
	badIdent := &identity.AgentIdentity{AgentID: "a", TenantID: ""}
	_, err := Normalize(badIdent, "cpu", time.Now(), Sample{MetricKey: "m", Value: 1, Unit: "u"})
	if err == nil {
		t.Fatal("expected error on incomplete identity")
	}

	// 2. Non-finite values
	_, err = Normalize(validIdent, "cpu", time.Now(), Sample{MetricKey: "m", Value: math.NaN(), Unit: "u"})
	if err == nil {
		t.Fatal("expected error on NaN")
	}
	_, err = Normalize(validIdent, "cpu", time.Now(), Sample{MetricKey: "m", Value: math.Inf(1), Unit: "u"})
	if err == nil {
		t.Fatal("expected error on +Inf")
	}

	// 3. Control characters in metric key
	_, err = Normalize(validIdent, "cpu", time.Now(), Sample{MetricKey: "bad\nkey", Value: 1, Unit: "u"})
	if err == nil {
		t.Fatal("expected error on control character in metric key")
	}

	// 4. Too many labels
	tooMany := make(map[string]string)
	for i := 0; i < 20; i++ {
		tooMany[string(rune('a'+i))] = "val"
	}
	_, err = Normalize(validIdent, "cpu", time.Now(), Sample{MetricKey: "m", Value: 1, Unit: "u", Labels: tooMany})
	if err == nil {
		t.Fatal("expected error on cardinality limit exceeded")
	}
}
