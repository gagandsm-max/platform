package collector

import (
	"context"
	"strings"
	"testing"
	"time"

	"platform/agent/common/telemetry"
)

type stubCollector struct {
	name    string
	samples []telemetry.Sample
}

func (s *stubCollector) Name() string { return s.name }
func (s *stubCollector) Collect(ctx context.Context) ([]telemetry.Sample, error) {
	return s.samples, nil
}

func TestRegistryValidConstructionAndEnumeration(t *testing.T) {
	regs := []Registration{
		{Collector: &stubCollector{name: "cpu"}, Enabled: true, Interval: time.Second, Timeout: 100 * time.Millisecond},
		{Collector: &stubCollector{name: "memory"}, Enabled: true, Interval: 2 * time.Second, Timeout: 200 * time.Millisecond},
		{Collector: &stubCollector{name: "filesystem"}, Enabled: false, Interval: 3 * time.Second, Timeout: 300 * time.Millisecond},
	}

	reg, err := NewRegistry(regs)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if reg.Len() != 3 {
		t.Fatalf("Len=%d want 3", reg.Len())
	}

	entries := reg.Entries()
	if len(entries) != 3 {
		t.Fatalf("Entries len=%d", len(entries))
	}
	if entries[0].Name != "cpu" || entries[1].Name != "memory" || entries[2].Name != "filesystem" {
		t.Fatalf("enumeration order not stable: %+v", namesOf(entries))
	}

	// Deterministic across repeated reads.
	again := reg.Entries()
	for i := range entries {
		if again[i].Name != entries[i].Name || again[i].Enabled != entries[i].Enabled {
			t.Fatalf("enumeration not stable across reads")
		}
		if again[i].Interval != entries[i].Interval || again[i].Timeout != entries[i].Timeout {
			t.Fatalf("scheduling metadata not stable across reads")
		}
	}

	enabled := reg.EnabledEntries()
	if len(enabled) != 2 {
		t.Fatalf("EnabledEntries=%d want 2", len(enabled))
	}
	if enabled[0].Name != "cpu" || enabled[1].Name != "memory" {
		t.Fatalf("enabled order=%v", namesOf(enabled))
	}
}

func TestRegistryRejectsDuplicateName(t *testing.T) {
	_, err := NewRegistry([]Registration{
		{Collector: &stubCollector{name: "cpu"}, Enabled: true},
		{Collector: &stubCollector{name: "cpu"}, Enabled: true},
	})
	if err == nil || !strings.Contains(err.Error(), "COLLECTOR_REGISTRY_DUPLICATE_NAME") {
		t.Fatalf("expected duplicate rejection, got %v", err)
	}
}

func TestRegistryRejectsEmptyAndInvalidName(t *testing.T) {
	_, err := NewRegistry([]Registration{
		{Collector: &stubCollector{name: "  "}, Enabled: true},
	})
	if err == nil || !strings.Contains(err.Error(), "COLLECTOR_REGISTRY_NAME_INVALID") {
		t.Fatalf("expected empty name rejection, got %v", err)
	}

	_, err = NewRegistry([]Registration{
		{Collector: &stubCollector{name: "bad\nname"}, Enabled: true},
	})
	if err == nil || !strings.Contains(err.Error(), "COLLECTOR_REGISTRY_NAME_INVALID") {
		t.Fatalf("expected control-char rejection, got %v", err)
	}
}

func TestRegistryRejectsNilCollector(t *testing.T) {
	_, err := NewRegistry([]Registration{
		{Collector: nil, Enabled: true},
	})
	if err == nil || !strings.Contains(err.Error(), "COLLECTOR_REGISTRY_NIL") {
		t.Fatalf("expected nil rejection, got %v", err)
	}
}

func TestRegistryEnforcesMaximumBound(t *testing.T) {
	regs := make([]Registration, 0, MaxRegisteredCollectors+1)
	for i := 0; i < MaxRegisteredCollectors+1; i++ {
		regs = append(regs, Registration{
			Collector: &stubCollector{name: "c" + strings.Repeat("x", 0) + itoa(i)},
			Enabled:   true,
		})
	}
	_, err := NewRegistry(regs)
	if err == nil || !strings.Contains(err.Error(), "COLLECTOR_REGISTRY_BOUND_EXCEEDED") {
		t.Fatalf("expected bound rejection, got %v", err)
	}
}

func TestRegistryImmutableAfterConstruction(t *testing.T) {
	regs := []Registration{
		{Collector: &stubCollector{name: "cpu"}, Enabled: true, Interval: time.Second, Timeout: time.Millisecond},
		{Collector: &stubCollector{name: "memory"}, Enabled: true, Interval: time.Second, Timeout: time.Millisecond},
	}
	reg, err := NewRegistry(regs)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	// Mutating the input slice must not affect the frozen registry.
	regs[0].Enabled = false
	regs[0].Interval = time.Hour
	regs = append(regs, Registration{Collector: &stubCollector{name: "extra"}, Enabled: true})

	entries := reg.Entries()
	if len(entries) != 2 {
		t.Fatalf("registry grew after construction: %d", len(entries))
	}
	if !entries[0].Enabled || entries[0].Interval != time.Second {
		t.Fatalf("registry mutated via input slice: %+v", entries[0])
	}

	// Mutating returned slice must not affect subsequent reads.
	entries[0].Enabled = false
	entries[0].Name = "mutated"
	again := reg.Entries()
	if again[0].Name != "cpu" || !again[0].Enabled {
		t.Fatalf("Entries() did not return a defensive copy: %+v", again[0])
	}
}

func TestNewDefaultRegistry(t *testing.T) {
	reg, err := NewDefaultRegistry(8, 15*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("NewDefaultRegistry: %v", err)
	}
	if reg.Len() < 1 {
		t.Fatal("expected default collectors")
	}
	for _, e := range reg.Entries() {
		if e.Name == "" {
			t.Fatal("empty default collector name")
		}
		if !e.Enabled {
			t.Fatalf("default collector %q should be enabled", e.Name)
		}
		if e.Interval != 15*time.Second || e.Timeout != 5*time.Second {
			t.Fatalf("default metadata mismatch for %q: interval=%v timeout=%v", e.Name, e.Interval, e.Timeout)
		}
	}
}

func namesOf(entries []Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Name
	}
	return out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
