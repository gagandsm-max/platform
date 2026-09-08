package telemetry

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"platform/agent/common/identity"
)

const SchemaVersion = 1

const (
	maxMetricKey = 128
	maxUnit      = 32
	maxLabels    = 16
	maxLabelKey  = 64
	maxLabelVal  = 128
)

type Sample struct {
	MetricKey string
	Value     float64
	Unit      string
	Labels    map[string]string
}

type Observation struct {
	SchemaVersion int               `json:"schemaVersion"`
	ObservationID string            `json:"observationId"`
	TenantID      string            `json:"tenantId"`
	AgentID       string            `json:"agentId"`
	HostID        string            `json:"hostId"`
	ObservedAt    time.Time         `json:"observedAt"`
	MetricKey     string            `json:"metricKey"`
	Value         float64           `json:"value"`
	Unit          string            `json:"unit"`
	Labels        map[string]string `json:"labels,omitempty"`
	Source        string            `json:"source"`
	Collector     string            `json:"collector"`
}

type CollectorError struct {
	Collector string `json:"collector"`
	Code      string `json:"code"`
	Summary   string `json:"summary"`
}

type CollectionBatch struct {
	SchemaVersion int              `json:"schemaVersion"`
	SourceType    string           `json:"sourceType"`
	TenantID      string           `json:"tenantId"`
	AgentID       string           `json:"agentId"`
	SiteID        string           `json:"siteId,omitempty"`
	AgentVersion  string           `json:"agentVersion,omitempty"`
	HostID        string           `json:"hostId"`
	Hostname      string           `json:"hostname"`
	OS            string           `json:"os"`
	Arch          string           `json:"arch"`
	CollectedAt   time.Time        `json:"collectedAt"`
	DurationMs    int64            `json:"durationMs"`
	Observations  []Observation    `json:"observations"`
	Errors        []CollectorError `json:"errors,omitempty"`
}

func Normalize(ident *identity.AgentIdentity, collector string, observedAt time.Time, sample Sample) (Observation, error) {
	if ident == nil || ident.AgentID == "" || ident.TenantID == "" || ident.HostID == "" {
		return Observation{}, errors.New("TELEMETRY_IDENTITY_INVALID: Agent identity context is incomplete")
	}
	collector = strings.TrimSpace(collector)
	if collector == "" || len(collector) > 64 || hasControl(collector) {
		return Observation{}, errors.New("TELEMETRY_COLLECTOR_INVALID: collector name is outside bounds")
	}
	key := strings.TrimSpace(sample.MetricKey)
	if key == "" || len(key) > maxMetricKey || hasControl(key) {
		return Observation{}, errors.New("TELEMETRY_METRIC_INVALID: metric key is outside bounds")
	}
	unit := strings.TrimSpace(sample.Unit)
	if unit == "" || len(unit) > maxUnit || hasControl(unit) {
		return Observation{}, errors.New("TELEMETRY_UNIT_INVALID: unit is outside bounds")
	}
	if math.IsNaN(sample.Value) || math.IsInf(sample.Value, 0) {
		return Observation{}, errors.New("TELEMETRY_VALUE_INVALID: non-finite metric values are prohibited")
	}
	labels, err := normalizeLabels(sample.Labels)
	if err != nil {
		return Observation{}, err
	}
	id, err := observationID()
	if err != nil {
		return Observation{}, err
	}
	return Observation{
		SchemaVersion: SchemaVersion,
		ObservationID: id,
		TenantID:      ident.TenantID,
		AgentID:       ident.AgentID,
		HostID:        ident.HostID,
		ObservedAt:    observedAt.UTC(),
		MetricKey:     key,
		Value:         sample.Value,
		Unit:          unit,
		Labels:        labels,
		Source:        "AGENT_HOST",
		Collector:     collector,
	}, nil
}

func normalizeLabels(in map[string]string) (map[string]string, error) {
	if len(in) > maxLabels {
		return nil, errors.New("TELEMETRY_CARDINALITY_LIMIT: too many labels on one observation")
	}
	if len(in) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(in))
	for _, key := range keys {
		value := in[key]
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || len(key) > maxLabelKey || len(value) > maxLabelVal || hasControl(key) || hasControl(value) {
			return nil, errors.New("TELEMETRY_LABEL_INVALID: label key/value is outside bounds")
		}
		if _, exists := out[key]; exists {
			return nil, errors.New("TELEMETRY_LABEL_INVALID: duplicate normalized label key")
		}
		out[key] = value
	}
	return out, nil
}

func observationID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("TELEMETRY_ID_FAILED: %w", err)
	}
	return "obs-" + hex.EncodeToString(buf), nil
}

func hasControl(v string) bool {
	return strings.IndexFunc(v, unicode.IsControl) >= 0
}
