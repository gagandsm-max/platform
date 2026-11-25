// Bounded EDS evidence selection for built-in CPU/memory pressure definitions.
// Eligibility evaluation remains separate; callers still enforce job authorization
// and resource admission before invoking these selection helpers.
package diagnostics

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"platform/agent/common/telemetry"
)

// Built-in EDS definition identifiers and frozen leaf versions for S2.
const (
	DefinitionCPUPressure    = "EDS_CPU_PRESSURE"
	DefinitionMemoryPressure = "EDS_MEMORY_PRESSURE"

	DefinitionVersionCPUPressure    uint64 = 1
	DefinitionVersionMemoryPressure uint64 = 1
)

// Capture completeness subordinate to Diagnostic Job state (Platform_17 §4.6.2).
type CaptureStatus string

const (
	CaptureComplete CaptureStatus = "COMPLETE"
	CapturePartial  CaptureStatus = "PARTIAL"
	CaptureFailed   CaptureStatus = "FAILED"
)

// MissingReason values are compatible with Platform_17 §4.6.2 missing_evidence codes.
type MissingReason string

const (
	MissingNotAvailable  MissingReason = "NOT_AVAILABLE"
	MissingResourceGuard MissingReason = "RESOURCE_GUARD"
	MissingCollectorErr  MissingReason = "COLLECTOR_ERROR"
)

// SelectionReason explains fail-closed rejection of an evidence preparation call.
type SelectionReason string

const (
	ReasonOK                 SelectionReason = ""
	ReasonInvalidRequest     SelectionReason = "INVALID_REQUEST"
	ReasonInputBounded       SelectionReason = "INPUT_BOUNDED"
	ReasonIdentityMismatch   SelectionReason = "IDENTITY_MISMATCH"
	ReasonAmbiguousDuplicate SelectionReason = "AMBIGUOUS_OR_DUPLICATE_SERIES"
	ReasonNoSelectable       SelectionReason = "NO_SELECTABLE_EVIDENCE"
)

// EvidenceRequest is caller-supplied selection context. Limits and identity are
// mandatory; this leaf does not read the OS or invent observations.
type EvidenceRequest struct {
	TenantID     string
	AgentID      string
	HostID       string
	Now          time.Time
	MaxAge       time.Duration
	MaxInput     int
	MaxItems     int
	MaxBytes     int
	Observations []telemetry.Observation
}

// EvidenceItem preserves provenance from a selected normalized observation.
type EvidenceItem struct {
	SchemaVersion int
	ObservationID string
	TenantID      string
	AgentID       string
	HostID        string
	ObservedAt    time.Time
	MetricKey     string
	Value         float64
	Unit          string
	Labels        map[string]string
	Source        string
	Collector     string
}

// MissingEvidence names an expected series that could not be selected.
type MissingEvidence struct {
	MetricKey string
	Labels    map[string]string
	Reason    MissingReason
}

// EvidenceResult is the bounded selection outcome for one built-in definition.
type EvidenceResult struct {
	DefinitionID      string
	DefinitionVersion uint64
	CaptureStatus     CaptureStatus
	Reason            SelectionReason
	Items             []EvidenceItem
	Missing           []MissingEvidence
}

type seriesSpec struct {
	metricKey string
	unit      string
	collector string
	resource  string // empty for unlabeled scalars
	scope     string // empty for unlabeled scalars
}

type seriesBucket struct {
	items []telemetry.Observation
	bad   bool
}

// SelectCPUPressure selects versioned EDS_CPU_PRESSURE evidence from normalized observations.
func SelectCPUPressure(req EvidenceRequest) EvidenceResult {
	return selectDefinition(DefinitionCPUPressure, DefinitionVersionCPUPressure, cpuPressureSeries, req)
}

// SelectMemoryPressure selects versioned EDS_MEMORY_PRESSURE evidence from normalized observations.
func SelectMemoryPressure(req EvidenceRequest) EvidenceResult {
	return selectDefinition(DefinitionMemoryPressure, DefinitionVersionMemoryPressure, memoryPressureSeries, req)
}

func selectDefinition(definitionID string, version uint64, catalog []seriesSpec, req EvidenceRequest) EvidenceResult {
	base := EvidenceResult{DefinitionID: definitionID, DefinitionVersion: version}
	if reason := requestProblem(req); reason != "" {
		base.CaptureStatus = CaptureFailed
		base.Reason = reason
		return base
	}
	if len(req.Observations) > req.MaxInput {
		base.CaptureStatus = CaptureFailed
		base.Reason = ReasonInputBounded
		return base
	}

	index := make(map[string]int, len(catalog))
	for i, spec := range catalog {
		index[seriesID(spec)] = i
	}
	buckets := make([]seriesBucket, len(catalog))
	interested := metricInterest(catalog)
	seenObsIDs := make(map[string]int) // ObservationID -> catalog series index

	for i := range req.Observations {
		obs := req.Observations[i]
		// Reject malformed UTF-8 before any selection; never replace or invent identity.
		if !utf8.ValidString(obs.TenantID) || !utf8.ValidString(obs.AgentID) ||
			!utf8.ValidString(obs.HostID) || !utf8.ValidString(obs.ObservationID) {
			base.CaptureStatus = CaptureFailed
			base.Reason = ReasonInvalidRequest
			return base
		}
		if obs.TenantID != req.TenantID || obs.AgentID != req.AgentID || obs.HostID != req.HostID {
			base.CaptureStatus = CaptureFailed
			base.Reason = ReasonIdentityMismatch
			return base
		}
		if !interested[obs.MetricKey] {
			continue
		}
		spec, ok := lookupSeries(catalog, index, obs)
		if !ok {
			// Wrong label shape for an interested key (or other-resource PSI) is not selected.
			continue
		}
		idx := index[seriesID(spec)]
		if observationProblem(req, obs, spec) != "" {
			buckets[idx].bad = true
			continue
		}
		if prev, exists := seenObsIDs[obs.ObservationID]; exists && prev != idx {
			base.CaptureStatus = CaptureFailed
			base.Reason = ReasonAmbiguousDuplicate
			return base
		}
		seenObsIDs[obs.ObservationID] = idx
		buckets[idx].items = append(buckets[idx].items, obs)
	}

	for _, b := range buckets {
		if len(b.items) > 1 {
			base.CaptureStatus = CaptureFailed
			base.Reason = ReasonAmbiguousDuplicate
			return base
		}
	}

	selected := make([]EvidenceItem, 0, len(catalog))
	missing := make([]MissingEvidence, 0)
	bytesUsed := 0
	truncated := false

	for i, spec := range catalog {
		b := buckets[i]
		if truncated {
			missing = append(missing, missingOf(spec, MissingResourceGuard))
			continue
		}
		if len(b.items) != 1 {
			reason := MissingNotAvailable
			if b.bad {
				reason = MissingCollectorErr
			}
			missing = append(missing, missingOf(spec, reason))
			continue
		}
		item := itemFrom(b.items[0])
		size, ok := itemBytes(item)
		// Fail closed on accounting overflow or overbudget; never wrap to admit.
		if !ok || len(selected) >= req.MaxItems || size > req.MaxBytes || bytesUsed > req.MaxBytes-size {
			truncated = true
			missing = append(missing, missingOf(spec, MissingResourceGuard))
			continue
		}
		selected = append(selected, item)
		bytesUsed += size
	}

	base.Items = selected
	base.Missing = missing
	switch {
	case len(selected) == 0:
		base.CaptureStatus = CaptureFailed
		base.Reason = ReasonNoSelectable
	case len(missing) == 0:
		base.CaptureStatus = CaptureComplete
		base.Reason = ReasonOK
	default:
		base.CaptureStatus = CapturePartial
		base.Reason = ReasonOK
	}
	return base
}

func requestProblem(req EvidenceRequest) SelectionReason {
	if strings.TrimSpace(req.TenantID) == "" || strings.TrimSpace(req.AgentID) == "" || strings.TrimSpace(req.HostID) == "" {
		return ReasonInvalidRequest
	}
	// Exact identity bytes are matched later; reject malformed UTF-8 without replacement.
	if !utf8.ValidString(req.TenantID) || !utf8.ValidString(req.AgentID) || !utf8.ValidString(req.HostID) {
		return ReasonInvalidRequest
	}
	if req.Now.IsZero() || req.MaxAge <= 0 || req.MaxInput <= 0 || req.MaxItems <= 0 || req.MaxBytes <= 0 {
		return ReasonInvalidRequest
	}
	return ReasonOK
}

func observationProblem(req EvidenceRequest, obs telemetry.Observation, spec seriesSpec) string {
	if obs.SchemaVersion != telemetry.SchemaVersion {
		return "schema"
	}
	if strings.TrimSpace(obs.ObservationID) == "" || hasControlRune(obs.ObservationID) {
		return "id"
	}
	if obs.Source != "AGENT_HOST" {
		return "source"
	}
	if obs.Collector != spec.collector {
		return "collector"
	}
	if obs.Unit != spec.unit {
		return "unit"
	}
	if obs.MetricKey != spec.metricKey {
		return "metric"
	}
	if !finiteFloat(obs.Value) {
		return "value"
	}
	observedAt := obs.ObservedAt
	if observedAt.IsZero() || observedAt.After(req.Now) {
		return "time"
	}
	if req.Now.Sub(observedAt) > req.MaxAge {
		return "stale"
	}
	if !labelsMatch(obs.Labels, spec) {
		return "labels"
	}
	return ""
}

func lookupSeries(catalog []seriesSpec, index map[string]int, obs telemetry.Observation) (seriesSpec, bool) {
	resource, scope := "", ""
	if len(obs.Labels) > 0 {
		resource, scope = obs.Labels["resource"], obs.Labels["scope"]
	}
	id := seriesID(seriesSpec{metricKey: obs.MetricKey, resource: resource, scope: scope})
	idx, ok := index[id]
	if !ok {
		return seriesSpec{}, false
	}
	return catalog[idx], true
}

func labelsMatch(labels map[string]string, spec seriesSpec) bool {
	if spec.resource == "" && spec.scope == "" {
		return len(labels) == 0
	}
	if len(labels) != 2 {
		return false
	}
	return labels["resource"] == spec.resource && labels["scope"] == spec.scope
}

func missingOf(spec seriesSpec, reason MissingReason) MissingEvidence {
	out := MissingEvidence{MetricKey: spec.metricKey, Reason: reason}
	if spec.resource != "" || spec.scope != "" {
		out.Labels = map[string]string{"resource": spec.resource, "scope": spec.scope}
	}
	return out
}

func itemFrom(obs telemetry.Observation) EvidenceItem {
	var labels map[string]string
	if len(obs.Labels) > 0 {
		labels = make(map[string]string, len(obs.Labels))
		keys := make([]string, 0, len(obs.Labels))
		for k := range obs.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			labels[k] = obs.Labels[k]
		}
	}
	return EvidenceItem{
		SchemaVersion: obs.SchemaVersion,
		ObservationID: obs.ObservationID,
		TenantID:      obs.TenantID,
		AgentID:       obs.AgentID,
		HostID:        obs.HostID,
		ObservedAt:    obs.ObservedAt.UTC(),
		MetricKey:     obs.MetricKey,
		Value:         obs.Value,
		Unit:          obs.Unit,
		Labels:        labels,
		Source:        obs.Source,
		Collector:     obs.Collector,
	}
}

// itemBytes returns selected-item budget accounting for one EvidenceItem.
//
// Accounting (UTF-8 byte lengths, not Unicode runes; not a wire/envelope size):
//
//	len(ObservationID)+len(TenantID)+len(AgentID)+len(HostID)+
//	len(MetricKey)+len(Unit)+len(Source)+len(Collector)+
//	sum(len(labelKey)+len(labelVal)) + fixedOverhead(16)
//
// where fixedOverhead covers value + timestamps + schemaVersion fields only.
// This is an admission bound for deterministic selection; final transport payload
// size is owned by later BUILD wire encoding and is not claimed here.
// ok=false if the sum would overflow int (caller must fail closed / not admit).
func itemBytes(item EvidenceItem) (n int, ok bool) {
	n = 16 // value + timestamps + schemaVersion overhead
	add := func(s string) bool {
		l := len(s)
		if n > math.MaxInt-l {
			return false
		}
		n += l
		return true
	}
	if !add(item.ObservationID) || !add(item.TenantID) || !add(item.AgentID) ||
		!add(item.HostID) || !add(item.MetricKey) || !add(item.Unit) ||
		!add(item.Source) || !add(item.Collector) {
		return 0, false
	}
	for k, v := range item.Labels {
		if !add(k) || !add(v) {
			return 0, false
		}
	}
	return n, true
}

func seriesID(spec seriesSpec) string {
	if spec.resource == "" && spec.scope == "" {
		return spec.metricKey
	}
	return spec.metricKey + "|resource=" + spec.resource + "|scope=" + spec.scope
}

func metricInterest(catalog []seriesSpec) map[string]bool {
	out := make(map[string]bool, len(catalog))
	for _, spec := range catalog {
		out[spec.metricKey] = true
	}
	return out
}

func finiteFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func hasControlRune(v string) bool {
	return strings.IndexFunc(v, func(r rune) bool {
		return r < 0x20 || r == 0x7f
	}) >= 0
}

func pressureSeries(resource string) []seriesSpec {
	out := make([]seriesSpec, 0, 6)
	for _, metric := range []string{
		"system.pressure.avg10",
		"system.pressure.avg60",
		"system.pressure.avg300",
	} {
		for _, scope := range []string{"some", "full"} {
			out = append(out, seriesSpec{
				metricKey: metric,
				unit:      "%",
				collector: "pressure",
				resource:  resource,
				scope:     scope,
			})
		}
	}
	return out
}

// Exact keys/units/collectors verified against agent/common/collector sources.
var cpuPressureSeries = append([]seriesSpec{
	{metricKey: "system.cpu.utilization", unit: "%", collector: "cpu"},
	{metricKey: "system.cpu.count", unit: "count", collector: "cpu"},
	{metricKey: "system.load.1m", unit: "load", collector: "cpu"},
	{metricKey: "system.load.5m", unit: "load", collector: "cpu"},
	{metricKey: "system.load.15m", unit: "load", collector: "cpu"},
	{metricKey: "system.processes.running", unit: "count", collector: "osfacts"},
	{metricKey: "system.processes.blocked", unit: "count", collector: "osfacts"},
}, pressureSeries("cpu")...)

var memoryPressureSeries = append([]seriesSpec{
	{metricKey: "system.memory.total", unit: "bytes", collector: "memory"},
	{metricKey: "system.memory.available", unit: "bytes", collector: "memory"},
	{metricKey: "system.memory.used", unit: "bytes", collector: "memory"},
	{metricKey: "system.memory.utilization", unit: "%", collector: "memory"},
	{metricKey: "system.memory.swap.total", unit: "bytes", collector: "memory"},
	{metricKey: "system.memory.swap.available", unit: "bytes", collector: "memory"},
	{metricKey: "system.memory.swap.used", unit: "bytes", collector: "memory"},
	{metricKey: "system.memory.swap.utilization", unit: "%", collector: "memory"},
}, pressureSeries("memory")...)
