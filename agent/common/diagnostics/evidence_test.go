package diagnostics

import (
	"math"
	"strings"
	"testing"
	"time"

	"platform/agent/common/telemetry"
)

func evidenceNow() time.Time {
	return time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
}

func baseRequest(obs []telemetry.Observation) EvidenceRequest {
	return EvidenceRequest{
		TenantID:     "tenant-a",
		AgentID:      "agent-a",
		HostID:       "host-a",
		Now:          evidenceNow(),
		MaxAge:       30 * time.Second,
		MaxInput:     256,
		MaxItems:     64,
		MaxBytes:     1 << 20,
		Observations: obs,
	}
}

func obs(metric, unit, collector, id string, value float64, labels map[string]string) telemetry.Observation {
	return telemetry.Observation{
		SchemaVersion: telemetry.SchemaVersion,
		ObservationID: id,
		TenantID:      "tenant-a",
		AgentID:       "agent-a",
		HostID:        "host-a",
		ObservedAt:    evidenceNow().Add(-2 * time.Second),
		MetricKey:     metric,
		Value:         value,
		Unit:          unit,
		Labels:        labels,
		Source:        "AGENT_HOST",
		Collector:     collector,
	}
}

func cpuFixture() []telemetry.Observation {
	psi := func(metric, scope, id string, value float64) telemetry.Observation {
		return obs(metric, "%", "pressure", id, value, map[string]string{"resource": "cpu", "scope": scope})
	}
	return []telemetry.Observation{
		obs("system.cpu.utilization", "%", "cpu", "cpu-util", 91.5, nil),
		obs("system.cpu.count", "count", "cpu", "cpu-count", 8, nil),
		obs("system.load.1m", "load", "cpu", "load-1", 4.2, nil),
		obs("system.load.5m", "load", "cpu", "load-5", 3.1, nil),
		obs("system.load.15m", "load", "cpu", "load-15", 2.0, nil),
		obs("system.processes.running", "count", "osfacts", "procs-run", 12, nil),
		obs("system.processes.blocked", "count", "osfacts", "procs-block", 1, nil),
		psi("system.pressure.avg10", "some", "psi-10-some", 12.5),
		psi("system.pressure.avg10", "full", "psi-10-full", 1.5),
		psi("system.pressure.avg60", "some", "psi-60-some", 10.0),
		psi("system.pressure.avg60", "full", "psi-60-full", 1.0),
		psi("system.pressure.avg300", "some", "psi-300-some", 8.0),
		psi("system.pressure.avg300", "full", "psi-300-full", 0.5),
	}
}

func memoryFixture() []telemetry.Observation {
	psi := func(metric, scope, id string, value float64) telemetry.Observation {
		return obs(metric, "%", "pressure", id, value, map[string]string{"resource": "memory", "scope": scope})
	}
	return []telemetry.Observation{
		obs("system.memory.total", "bytes", "memory", "mem-total", 16<<30, nil),
		obs("system.memory.available", "bytes", "memory", "mem-avail", 2<<30, nil),
		obs("system.memory.used", "bytes", "memory", "mem-used", 14<<30, nil),
		obs("system.memory.utilization", "%", "memory", "mem-util", 87.5, nil),
		obs("system.memory.swap.total", "bytes", "memory", "swap-total", 4<<30, nil),
		obs("system.memory.swap.available", "bytes", "memory", "swap-avail", 1<<30, nil),
		obs("system.memory.swap.used", "bytes", "memory", "swap-used", 3<<30, nil),
		obs("system.memory.swap.utilization", "%", "memory", "swap-util", 75.0, nil),
		psi("system.pressure.avg10", "some", "mpsi-10-some", 22.0),
		psi("system.pressure.avg10", "full", "mpsi-10-full", 5.0),
		psi("system.pressure.avg60", "some", "mpsi-60-some", 18.0),
		psi("system.pressure.avg60", "full", "mpsi-60-full", 4.0),
		psi("system.pressure.avg300", "some", "mpsi-300-some", 11.0),
		psi("system.pressure.avg300", "full", "mpsi-300-full", 2.0),
	}
}

func irrelevantNoise() []telemetry.Observation {
	return []telemetry.Observation{
		obs("system.filesystem.utilization", "%", "filesystem", "fs-util", 90, map[string]string{"mountpoint": "/"}),
		obs("system.network.rx.bytes", "bytes", "net", "net-rx", 1000, map[string]string{"device": "eth0"}),
		obs("system.pressure.avg10", "%", "pressure", "psi-io", 50, map[string]string{"resource": "io", "scope": "some"}),
		obs("system.pressure.total", "us", "pressure", "psi-total", 99, map[string]string{"resource": "cpu", "scope": "some"}),
		obs("system.pressure.available", "bool", "pressure", "psi-avail", 1, map[string]string{"resource": "cpu"}),
		obs("system.uptime", "s", "cpu", "uptime", 1000, nil),
		obs("system.cpu.utilization", "%", "cpu", "other-tenant-should-not-reach", 1, nil), // overwritten in mismatch tests
	}
}

func TestCPUPressureSelectsExactSeries(t *testing.T) {
	req := baseRequest(append(cpuFixture(), irrelevantNoise()[:6]...))
	got := SelectCPUPressure(req)
	if got.DefinitionID != DefinitionCPUPressure || got.DefinitionVersion != DefinitionVersionCPUPressure {
		t.Fatalf("definition identity: %+v", got)
	}
	if got.CaptureStatus != CaptureComplete || got.Reason != ReasonOK {
		t.Fatalf("status=%s reason=%s missing=%v", got.CaptureStatus, got.Reason, got.Missing)
	}
	if len(got.Items) != len(cpuPressureSeries) {
		t.Fatalf("selected %d, want %d", len(got.Items), len(cpuPressureSeries))
	}
	for i, item := range got.Items {
		spec := cpuPressureSeries[i]
		if item.MetricKey != spec.metricKey || item.Unit != spec.unit || item.Collector != spec.collector {
			t.Fatalf("item %d mismatch: %+v vs %+v", i, item, spec)
		}
		if item.Source != "AGENT_HOST" || item.TenantID != "tenant-a" || item.AgentID != "agent-a" || item.HostID != "host-a" {
			t.Fatalf("provenance lost on item %d: %+v", i, item)
		}
		if spec.resource != "" {
			if item.Labels["resource"] != spec.resource || item.Labels["scope"] != spec.scope {
				t.Fatalf("labels lost on item %d: %+v", i, item.Labels)
			}
		} else if len(item.Labels) != 0 {
			t.Fatalf("unexpected labels on scalar item %d: %+v", i, item.Labels)
		}
	}
	for _, item := range got.Items {
		if item.MetricKey == "system.uptime" || item.MetricKey == "system.pressure.total" || item.Labels["resource"] == "io" {
			t.Fatalf("irrelevant observation leaked: %+v", item)
		}
	}
}

func TestMemoryPressureSelectsExactSeries(t *testing.T) {
	req := baseRequest(append(memoryFixture(), irrelevantNoise()[:6]...))
	got := SelectMemoryPressure(req)
	if got.DefinitionID != DefinitionMemoryPressure || got.DefinitionVersion != DefinitionVersionMemoryPressure {
		t.Fatalf("definition identity: %+v", got)
	}
	if got.CaptureStatus != CaptureComplete || len(got.Items) != len(memoryPressureSeries) {
		t.Fatalf("status=%s items=%d missing=%v", got.CaptureStatus, len(got.Items), got.Missing)
	}
	foundSwap := false
	for _, item := range got.Items {
		if item.MetricKey == "system.memory.swap.utilization" {
			foundSwap = true
			if item.Unit != "%" || item.Collector != "memory" {
				t.Fatalf("swap utilization provenance: %+v", item)
			}
		}
		if item.Labels["resource"] == "cpu" {
			t.Fatalf("cpu pressure leaked into memory definition: %+v", item)
		}
	}
	if !foundSwap {
		t.Fatal("expected system.memory.swap.utilization")
	}
}

func TestIrrelevantDataExcludedFromEachDefinition(t *testing.T) {
	mixed := append(cpuFixture()[:1], memoryFixture()[:1]...)
	mixed = append(mixed, irrelevantNoise()[:6]...)
	cpu := SelectCPUPressure(baseRequest(mixed))
	if cpu.CaptureStatus != CapturePartial {
		t.Fatalf("cpu status=%s", cpu.CaptureStatus)
	}
	for _, item := range cpu.Items {
		if item.MetricKey == "system.memory.total" {
			t.Fatal("memory fact selected by CPU definition")
		}
	}
	mem := SelectMemoryPressure(baseRequest(mixed))
	if mem.CaptureStatus != CapturePartial {
		t.Fatalf("memory status=%s", mem.CaptureStatus)
	}
	for _, item := range mem.Items {
		if item.MetricKey == "system.cpu.utilization" {
			t.Fatal("cpu fact selected by memory definition")
		}
	}
}

func TestMissingAndPartialReasons(t *testing.T) {
	partial := []telemetry.Observation{
		obs("system.cpu.utilization", "%", "cpu", "cpu-util", 80, nil),
		obs("system.load.1m", "load", "cpu", "load-1", 1, nil),
	}
	got := SelectCPUPressure(baseRequest(partial))
	if got.CaptureStatus != CapturePartial || len(got.Items) != 2 {
		t.Fatalf("partial result: status=%s items=%d", got.CaptureStatus, len(got.Items))
	}
	if len(got.Missing) == 0 {
		t.Fatal("expected missing evidence entries")
	}
	for _, m := range got.Missing {
		if m.Reason != MissingNotAvailable {
			t.Fatalf("missing reason=%s want NOT_AVAILABLE for absent series", m.Reason)
		}
	}

	stale := obs("system.cpu.utilization", "%", "cpu", "stale", 90, nil)
	stale.ObservedAt = evidenceNow().Add(-2 * time.Minute)
	got = SelectCPUPressure(baseRequest([]telemetry.Observation{stale}))
	if got.CaptureStatus != CaptureFailed || got.Reason != ReasonNoSelectable {
		t.Fatalf("stale-only: %+v", got)
	}
	if len(got.Missing) == 0 || got.Missing[0].Reason != MissingCollectorErr {
		t.Fatalf("stale series missing reason: %+v", got.Missing)
	}

	empty := SelectCPUPressure(baseRequest(nil))
	if empty.CaptureStatus != CaptureFailed || empty.Reason != ReasonNoSelectable {
		t.Fatalf("empty input: %+v", empty)
	}
}

func TestRejectsFutureStaleNonfiniteAndMalformed(t *testing.T) {
	future := obs("system.cpu.utilization", "%", "cpu", "future", 90, nil)
	future.ObservedAt = evidenceNow().Add(time.Second)
	badUnit := obs("system.cpu.utilization", "ratio", "cpu", "bad-unit", 90, nil)
	badCollector := obs("system.cpu.utilization", "%", "memory", "bad-collector", 90, nil)
	nonfinite := obs("system.cpu.count", "count", "cpu", "nan", math.NaN(), nil)
	badSource := obs("system.load.1m", "load", "cpu", "bad-source", 1, nil)
	badSource.Source = "SENSOR"
	labeledScalar := obs("system.load.5m", "load", "cpu", "labeled", 1, map[string]string{"core": "0"})

	got := SelectCPUPressure(baseRequest([]telemetry.Observation{
		future, badUnit, badCollector, nonfinite, badSource, labeledScalar,
		obs("system.load.15m", "load", "cpu", "ok", 1.5, nil),
	}))
	if got.CaptureStatus != CapturePartial || len(got.Items) != 1 || got.Items[0].ObservationID != "ok" {
		t.Fatalf("malformed handling: %+v", got)
	}
	for _, m := range got.Missing {
		if m.MetricKey == "system.cpu.utilization" && m.Reason != MissingCollectorErr {
			t.Fatalf("util missing reason=%s", m.Reason)
		}
	}
}

func TestRejectsWrongTenantAgentHost(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(*telemetry.Observation)
	}{
		{"tenant", func(o *telemetry.Observation) { o.TenantID = "other" }},
		{"agent", func(o *telemetry.Observation) { o.AgentID = "other" }},
		{"host", func(o *telemetry.Observation) { o.HostID = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sample := obs("system.cpu.utilization", "%", "cpu", "x", 1, nil)
			tc.mut(&sample)
			got := SelectCPUPressure(baseRequest([]telemetry.Observation{sample}))
			if got.CaptureStatus != CaptureFailed || got.Reason != ReasonIdentityMismatch {
				t.Fatalf("got %+v", got)
			}
			if len(got.Items) != 0 {
				t.Fatal("identity mismatch must not emit evidence")
			}
		})
	}
}

func TestRejectsDuplicateSeries(t *testing.T) {
	dup := []telemetry.Observation{
		obs("system.cpu.utilization", "%", "cpu", "a", 10, nil),
		obs("system.cpu.utilization", "%", "cpu", "b", 20, nil),
	}
	got := SelectCPUPressure(baseRequest(dup))
	if got.CaptureStatus != CaptureFailed || got.Reason != ReasonAmbiguousDuplicate {
		t.Fatalf("duplicate scalars: %+v", got)
	}

	labels := map[string]string{"resource": "memory", "scope": "some"}
	dupLabels := []telemetry.Observation{
		obs("system.pressure.avg10", "%", "pressure", "p1", 1, labels),
		obs("system.pressure.avg10", "%", "pressure", "p2", 2, map[string]string{"resource": "memory", "scope": "some"}),
	}
	got = SelectMemoryPressure(baseRequest(dupLabels))
	if got.CaptureStatus != CaptureFailed || got.Reason != ReasonAmbiguousDuplicate {
		t.Fatalf("duplicate labeled series: %+v", got)
	}
}

func TestBoundsInputItemsAndBytes(t *testing.T) {
	req := baseRequest(cpuFixture())
	req.MaxInput = 2
	got := SelectCPUPressure(req)
	if got.CaptureStatus != CaptureFailed || got.Reason != ReasonInputBounded {
		t.Fatalf("input bound: %+v", got)
	}

	req = baseRequest(cpuFixture())
	req.MaxItems = 3
	got = SelectCPUPressure(req)
	if got.CaptureStatus != CapturePartial || len(got.Items) != 3 {
		t.Fatalf("item bound: status=%s items=%d", got.CaptureStatus, len(got.Items))
	}
	for _, m := range got.Missing {
		if m.Reason != MissingResourceGuard {
			t.Fatalf("truncated missing reason=%s", m.Reason)
		}
	}

	req = baseRequest(cpuFixture())
	firstSize, ok := itemBytes(itemFrom(cpuFixture()[0]))
	if !ok {
		t.Fatal("itemBytes overflow on ASCII fixture")
	}
	req.MaxBytes = firstSize + 1
	got = SelectCPUPressure(req)
	if got.CaptureStatus != CapturePartial || len(got.Items) != 1 {
		t.Fatalf("byte bound: status=%s items=%d", got.CaptureStatus, len(got.Items))
	}
}

func TestDeterministicOrdering(t *testing.T) {
	forward := cpuFixture()
	reversed := make([]telemetry.Observation, len(forward))
	for i := range forward {
		reversed[len(forward)-1-i] = forward[i]
	}
	a := SelectCPUPressure(baseRequest(forward))
	b := SelectCPUPressure(baseRequest(reversed))
	if a.CaptureStatus != CaptureComplete || b.CaptureStatus != CaptureComplete {
		t.Fatalf("expected complete selections")
	}
	if len(a.Items) != len(b.Items) {
		t.Fatal("length drift")
	}
	for i := range a.Items {
		if a.Items[i].ObservationID != b.Items[i].ObservationID || a.Items[i].MetricKey != b.Items[i].MetricKey {
			t.Fatalf("order drift at %d: %+v vs %+v", i, a.Items[i], b.Items[i])
		}
	}
}

func TestInvalidRequestRejected(t *testing.T) {
	valid := baseRequest(cpuFixture()[:1])
	for _, tc := range []struct {
		name string
		mut  func(*EvidenceRequest)
	}{
		{"empty tenant", func(r *EvidenceRequest) { r.TenantID = "" }},
		{"zero now", func(r *EvidenceRequest) { r.Now = time.Time{} }},
		{"zero age", func(r *EvidenceRequest) { r.MaxAge = 0 }},
		{"zero input", func(r *EvidenceRequest) { r.MaxInput = 0 }},
		{"zero items", func(r *EvidenceRequest) { r.MaxItems = 0 }},
		{"zero bytes", func(r *EvidenceRequest) { r.MaxBytes = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := valid
			tc.mut(&req)
			got := SelectMemoryPressure(req)
			if got.CaptureStatus != CaptureFailed || got.Reason != ReasonInvalidRequest {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestEligibilityRegressionStillSeparate(t *testing.T) {
	// S1 eligibility API remains independently usable; evidence selection does not
	// assert DOWN/cause or mutate eligibility state.
	policy := testPolicy()
	authority := &Authority{Binding: policy.Binding, ExpiresAt: at(100)}
	state, decision := Evaluate(State{}, policy, authority, at(0), sample(policy, 0, 90))
	check(t, decision, Qualifying, ReasonQualifying)
	_ = SelectCPUPressure(baseRequest(cpuFixture()[:1]))
	state2, decision2 := Evaluate(state, policy, authority, at(1), sample(policy, 1, 90))
	check(t, decision2, Qualifying, ReasonQualifying)
	if state2.phase != Qualifying {
		t.Fatal("evidence selection disturbed eligibility state machine")
	}
}

func TestSelectedLabelsAreCopied(t *testing.T) {
	labels := map[string]string{"resource": "cpu", "scope": "some"}
	sample := obs("system.pressure.avg10", "%", "pressure", "psi", 3, labels)
	got := SelectCPUPressure(baseRequest([]telemetry.Observation{sample}))
	if len(got.Items) != 1 {
		t.Fatalf("items=%d missing=%v", len(got.Items), got.Missing)
	}
	got.Items[0].Labels["resource"] = "mutated"
	if labels["resource"] != "cpu" {
		t.Fatal("selection must copy labels")
	}
}

func TestItemBytesUsesUTF8ByteLengthsNotRunes(t *testing.T) {
	// 界 is U+754C (3 UTF-8 bytes). 40 repetitions => 40 runes / 120 bytes.
	// ObservationID "cpu-util" (8) + ASCII provenance + metric system.cpu.utilization (22):
	//   string runes=97 + fixed 16 = 113 (rune-era false admit at MaxBytes=113)
	//   string UTF-8 bytes=177 + fixed 16 = 193 (correct selected-item accounting)
	tenant := strings.Repeat("界", 40)
	sample := obs("system.cpu.utilization", "%", "cpu", "cpu-util", 91.5, nil)
	sample.TenantID = tenant

	item := itemFrom(sample)
	size, ok := itemBytes(item)
	if !ok {
		t.Fatal("itemBytes overflow")
	}
	stringBytes := len(item.ObservationID) + len(item.TenantID) + len(item.AgentID) +
		len(item.HostID) + len(item.MetricKey) + len(item.Unit) + len(item.Source) +
		len(item.Collector)
	if stringBytes != 177 {
		t.Fatalf("string UTF-8 bytes=%d want 177", stringBytes)
	}
	if size != 193 {
		t.Fatalf("selected-item accounting=%d want 193 (177 string bytes + 16 fixed)", size)
	}

	req := baseRequest([]telemetry.Observation{sample})
	req.TenantID = tenant
	req.MaxBytes = 113 // rune-era false admit boundary; must reject under byte accounting
	got := SelectCPUPressure(req)
	if got.CaptureStatus != CaptureFailed || got.Reason != ReasonNoSelectable || len(got.Items) != 0 {
		t.Fatalf("MaxBytes=113 must fail-closed: status=%s reason=%s items=%d", got.CaptureStatus, got.Reason, len(got.Items))
	}
	if len(got.Missing) == 0 || got.Missing[0].Reason != MissingResourceGuard {
		t.Fatalf("overbudget series must be RESOURCE_GUARD: %+v", got.Missing)
	}

	// Inclusive boundary: size == MaxBytes admits exactly one item.
	req.MaxBytes = size
	got = SelectCPUPressure(req)
	if got.CaptureStatus != CapturePartial || len(got.Items) != 1 {
		t.Fatalf("inclusive MaxBytes=%d: status=%s items=%d", size, got.CaptureStatus, len(got.Items))
	}
	if got.Items[0].TenantID != tenant || got.Items[0].ObservationID != "cpu-util" {
		t.Fatalf("provenance drift: %+v", got.Items[0])
	}
}

func TestValidMultibyteIdentitiesExactMatch(t *testing.T) {
	// Supplementary-plane and non-ASCII identities must be preserved with exact match.
	tenant := "テナント-\U0001F4BB" // includes U+1F4BB (supplementary)
	agent := "エージェント-α"
	host := "ホスト-β"
	sample := obs("system.cpu.utilization", "%", "cpu", "id-日本語", 42, nil)
	sample.TenantID = tenant
	sample.AgentID = agent
	sample.HostID = host

	req := baseRequest([]telemetry.Observation{sample})
	req.TenantID = tenant
	req.AgentID = agent
	req.HostID = host
	got := SelectCPUPressure(req)
	if got.CaptureStatus != CapturePartial || len(got.Items) != 1 {
		t.Fatalf("valid multibyte identities rejected: status=%s items=%d reason=%s", got.CaptureStatus, len(got.Items), got.Reason)
	}
	if got.Items[0].TenantID != tenant || got.Items[0].AgentID != agent ||
		got.Items[0].HostID != host || got.Items[0].ObservationID != "id-日本語" {
		t.Fatalf("identity not preserved exactly: %+v", got.Items[0])
	}

	// Distinct code points must not alias (no trimming/normalization).
	near := sample
	near.TenantID = tenant + " "
	req2 := req
	req2.Observations = []telemetry.Observation{near}
	got = SelectCPUPressure(req2)
	if got.CaptureStatus != CaptureFailed || got.Reason != ReasonIdentityMismatch || len(got.Items) != 0 {
		t.Fatalf("trailing-space tenant must not alias: %+v", got)
	}
}

func TestRejectsInvalidUTF8IdentitiesAndObservationIDs(t *testing.T) {
	invalid := string([]byte{0xff, 0xfe, 0xfd}) // malformed UTF-8; must not be replaced

	t.Run("request tenant", func(t *testing.T) {
		req := baseRequest(cpuFixture()[:1])
		req.TenantID = invalid
		got := SelectCPUPressure(req)
		if got.CaptureStatus != CaptureFailed || got.Reason != ReasonInvalidRequest || len(got.Items) != 0 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("request agent", func(t *testing.T) {
		req := baseRequest(cpuFixture()[:1])
		req.AgentID = invalid
		got := SelectCPUPressure(req)
		if got.CaptureStatus != CaptureFailed || got.Reason != ReasonInvalidRequest || len(got.Items) != 0 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("request host", func(t *testing.T) {
		req := baseRequest(cpuFixture()[:1])
		req.HostID = invalid
		got := SelectCPUPressure(req)
		if got.CaptureStatus != CaptureFailed || got.Reason != ReasonInvalidRequest || len(got.Items) != 0 {
			t.Fatalf("got %+v", got)
		}
	})
	for _, tc := range []struct {
		name string
		mut  func(*telemetry.Observation)
	}{
		{"obs tenant", func(o *telemetry.Observation) { o.TenantID = invalid }},
		{"obs agent", func(o *telemetry.Observation) { o.AgentID = invalid }},
		{"obs host", func(o *telemetry.Observation) { o.HostID = invalid }},
		{"obs id", func(o *telemetry.Observation) { o.ObservationID = invalid }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sample := obs("system.cpu.utilization", "%", "cpu", "ok-id", 1, nil)
			tc.mut(&sample)
			// Keep request identity valid ASCII so only observation malformation is under test
			// (except when mutating tenant/agent/host to invalid, which also mismatches).
			req := baseRequest([]telemetry.Observation{sample})
			if tc.name == "obs tenant" || tc.name == "obs agent" || tc.name == "obs host" {
				// Align request to the invalid value so UTF-8 rejection fires before identity mismatch.
				req.TenantID = sample.TenantID
				req.AgentID = sample.AgentID
				req.HostID = sample.HostID
			}
			got := SelectCPUPressure(req)
			if got.CaptureStatus != CaptureFailed || got.Reason != ReasonInvalidRequest || len(got.Items) != 0 {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestRejectsDuplicateObservationIDAcrossSeries(t *testing.T) {
	dupID := []telemetry.Observation{
		obs("system.cpu.utilization", "%", "cpu", "shared-id", 10, nil),
		obs("system.cpu.count", "count", "cpu", "shared-id", 8, nil),
	}
	got := SelectCPUPressure(baseRequest(dupID))
	if got.CaptureStatus != CaptureFailed || got.Reason != ReasonAmbiguousDuplicate || len(got.Items) != 0 {
		t.Fatalf("duplicate ObservationID across series: %+v", got)
	}

	// Preserve same-series duplicate regression (distinct IDs, same series).
	sameSeries := []telemetry.Observation{
		obs("system.cpu.utilization", "%", "cpu", "a", 10, nil),
		obs("system.cpu.utilization", "%", "cpu", "b", 20, nil),
	}
	got = SelectCPUPressure(baseRequest(sameSeries))
	if got.CaptureStatus != CaptureFailed || got.Reason != ReasonAmbiguousDuplicate || len(got.Items) != 0 {
		t.Fatalf("duplicate series regression broken: %+v", got)
	}
}
