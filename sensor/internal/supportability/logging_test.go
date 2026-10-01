package supportability

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func decodeOne(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
		t.Fatalf("decode log: %v\n%s", err, buf.String())
	}
	return got
}

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	level := new(slog.LevelVar)
	level.Set(slog.LevelDebug)
	return NewJSONLogger(buf, level, "0.1.0-test", "sensor-instance-test").
		With("sensor_id", "sensor-net-01")
}

func TestCanonicalStructuredLogEnvelope(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger.Warn("Control sync pending",
		"error_code", "CONTROL_SYNC_PENDING",
		"correlation_id", "corr-001",
		"retry_count", int64(2),
		"role", "NETWORK",
		"error", errors.New("Bearer should-never-appear"),
		"safe_context", map[string]any{"generation": int64(12), "mode": "trusted_continue"},
	)
	got := decodeOne(t, &buf)

	for _, key := range []string{"ts", "level", "component", "version", "service_instance_id", "sensor_id", "message"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing required key %q: %v", key, got)
		}
	}
	if got["level"] != "WARN" || got["component"] != "platform-sensor" {
		t.Fatalf("unexpected canonical identity: %v", got)
	}
	if got["event_code"] != "CONTROL_SYNC_PENDING" {
		t.Fatalf("event_code=%v", got["event_code"])
	}
	if got["retry_count"] != float64(2) {
		t.Fatalf("retry_count must be JSON number: %#v", got["retry_count"])
	}
	if ts, _ := got["ts"].(string); !strings.HasSuffix(ts, "Z") {
		t.Fatalf("ts must be UTC Z: %q", ts)
	}
	for _, forbidden := range []string{"time", "timestamp", "msg", "service", "error_code", "error", "role"} {
		if _, ok := got[forbidden]; ok {
			t.Fatalf("non-canonical field %q leaked: %v", forbidden, got)
		}
	}
	raw := buf.String()
	if strings.Contains(raw, "should-never-appear") {
		t.Fatalf("raw error leaked: %s", raw)
	}
	safe, ok := got["safe_context"].(map[string]any)
	if !ok || safe["mode"] != "trusted_continue" || safe["generation"] != float64(12) {
		t.Fatalf("safe_context=%#v", got["safe_context"])
	}
}

func TestInvalidCorrelationIDIsOmitted(t *testing.T) {
	for _, value := range []string{
		"bad/id",
		"corr with space",
		"corr\r\nid",
		strings.Repeat("x", 129),
	} {
		t.Run(value[:min(len(value), 12)], func(t *testing.T) {
			var buf bytes.Buffer
			logger := newTestLogger(&buf)
			logger.Info("test", "correlation_id", value)
			got := decodeOne(t, &buf)
			if _, ok := got["correlation_id"]; ok {
				t.Fatalf("invalid correlation_id emitted: %q", value)
			}
		})
	}
}

func TestRetryCountMustBeBoundedInteger(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  bool
	}{
		{"int", int64(4), true},
		{"uint", uint64(4), true},
		{"string", "4", false},
		{"negative", int64(-1), false},
		{"overflow", uint64(9007199254740992), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := newTestLogger(&buf)
			logger.Info("test", "retry_count", tc.value)
			got := decodeOne(t, &buf)
			_, present := got["retry_count"]
			if present != tc.want {
				t.Fatalf("retry_count present=%v want=%v log=%v", present, tc.want, got)
			}
		})
	}
}

func TestSafeContextRejectsSecretsCredentialsAndUnboundedShape(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	ctx := map[string]any{
		"mode":          "trusted_continue",
		"password":      "fictional-secret",
		"community":     "private",
		"api_token":     "abc",
		"Authorization": "Bearer abc123",
		"header":        "Basic Zm9vOmJhcg==",
		"private_material": "-----BEGIN PRIVATE KEY-----abc",
		"nested":        map[string]any{"x": 1},
	}
	logger.Info("test", "safe_context", ctx)
	got := decodeOne(t, &buf)
	safe, ok := got["safe_context"].(map[string]any)
	if !ok {
		t.Fatalf("expected retained safe context: %v", got)
	}
	if safe["mode"] != "trusted_continue" {
		t.Fatalf("safe value missing: %v", safe)
	}
	for _, key := range []string{"password", "community", "api_token", "Authorization", "header", "private_material", "nested"} {
		if _, present := safe[key]; present {
			t.Fatalf("unsafe context key %q retained: %v", key, safe)
		}
	}
	raw := strings.ToLower(buf.String())
	for _, forbidden := range []string{
		"fictional-secret",
		"bearer abc123",
		"basic zm9vomjhcg==",
		"begin private key",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("credential material leaked: %s", buf.String())
		}
	}
}

func TestUnknownTopLevelAttributesAreDropped(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger.Info("test",
		"environment", "production",
		"core_endpoint", "https://user:pass@example.invalid",
		"spool_path", "/secret/path",
	)
	got := decodeOne(t, &buf)
	for _, key := range []string{"environment", "core_endpoint", "spool_path"} {
		if _, ok := got[key]; ok {
			t.Fatalf("unknown top-level key %q emitted: %v", key, got)
		}
	}
}

func TestServiceInstanceIDIsStableFormatAndChangesAcrossGeneration(t *testing.T) {
	a := NewServiceInstanceID()
	b := NewServiceInstanceID()
	if a == b {
		t.Fatalf("service instance ids must differ: %q", a)
	}
	for _, id := range []string{a, b} {
		if len(id) == 0 || len(id) > 128 || !boundedIDRE.MatchString(id) {
			t.Fatalf("invalid service instance id %q", id)
		}
	}
}
