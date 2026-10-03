package collector

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// MaxRegisteredCollectors bounds startup registry size. This is a hard
// construction limit, not a runtime plugin capacity knob.
const MaxRegisteredCollectors = 32

// Registration describes one collector and its startup scheduling metadata.
// Interval/Timeout may be zero; callers such as NewDefaultRegistry fill defaults.
type Registration struct {
	Collector HostCollector
	Enabled   bool
	Interval  time.Duration
	Timeout   time.Duration
}

// Entry is an immutable snapshot of a registered collector after freeze.
type Entry struct {
	Name      string
	Collector HostCollector
	Enabled   bool
	Interval  time.Duration
	Timeout   time.Duration
}

// Registry is a startup-immutable collector catalogue. After NewRegistry returns
// successfully the set of collectors and metadata cannot change.
type Registry struct {
	entries []Entry
}

// NewRegistry validates and freezes collector registrations in deterministic
// registration order. Duplicate names, empty/invalid names, nil collectors and
// oversize catalogues are rejected.
func NewRegistry(regs []Registration) (*Registry, error) {
	if len(regs) == 0 {
		return nil, errors.New("COLLECTOR_REGISTRY_EMPTY: at least one collector registration is required")
	}
	if len(regs) > MaxRegisteredCollectors {
		return nil, fmt.Errorf(
			"COLLECTOR_REGISTRY_BOUND_EXCEEDED: at most %d collectors may be registered, got %d",
			MaxRegisteredCollectors,
			len(regs),
		)
	}

	seen := make(map[string]struct{}, len(regs))
	entries := make([]Entry, 0, len(regs))
	for i, reg := range regs {
		if reg.Collector == nil {
			return nil, fmt.Errorf("COLLECTOR_REGISTRY_NIL: registration[%d] collector is nil", i)
		}
		name := strings.TrimSpace(reg.Collector.Name())
		if err := validateCollectorName(name); err != nil {
			return nil, fmt.Errorf("COLLECTOR_REGISTRY_NAME_INVALID: registration[%d]: %w", i, err)
		}
		if _, dup := seen[name]; dup {
			return nil, fmt.Errorf("COLLECTOR_REGISTRY_DUPLICATE_NAME: collector name %q already registered", name)
		}
		if reg.Interval < 0 {
			return nil, fmt.Errorf("COLLECTOR_REGISTRY_INTERVAL_INVALID: collector %q interval must be non-negative", name)
		}
		if reg.Timeout < 0 {
			return nil, fmt.Errorf("COLLECTOR_REGISTRY_TIMEOUT_INVALID: collector %q timeout must be non-negative", name)
		}
		seen[name] = struct{}{}
		entries = append(entries, Entry{
			Name:      name,
			Collector: reg.Collector,
			Enabled:   reg.Enabled,
			Interval:  reg.Interval,
			Timeout:   reg.Timeout,
		})
	}

	return &Registry{entries: entries}, nil
}

// NewDefaultRegistry wraps NewDefaultCollectors with shared interval/timeout
// defaults while preserving the architecture for distinct per-collector values.
func NewDefaultRegistry(maxFilesystems int, interval, timeout time.Duration) (*Registry, error) {
	collectors := NewDefaultCollectors(maxFilesystems)
	regs := make([]Registration, 0, len(collectors))
	for _, c := range collectors {
		regs = append(regs, Registration{
			Collector: c,
			Enabled:   true,
			Interval:  interval,
			Timeout:   timeout,
		})
	}
	return NewRegistry(regs)
}

// Len returns the frozen registration count.
func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.entries)
}

// Entries returns a defensive copy of the frozen catalogue in registration order.
func (r *Registry) Entries() []Entry {
	if r == nil || len(r.entries) == 0 {
		return nil
	}
	out := make([]Entry, len(r.entries))
	copy(out, r.entries)
	return out
}

// EnabledEntries returns enabled collectors only, preserving registration order.
func (r *Registry) EnabledEntries() []Entry {
	if r == nil || len(r.entries) == 0 {
		return nil
	}
	out := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		if e.Enabled {
			out = append(out, e)
		}
	}
	return out
}

func validateCollectorName(name string) error {
	if name == "" {
		return errors.New("collector name must be non-empty")
	}
	if len(name) > 64 {
		return errors.New("collector name exceeds 64 characters")
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return errors.New("collector name must not contain control characters")
		}
	}
	return nil
}
