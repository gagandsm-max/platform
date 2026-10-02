package supportability

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
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

// BUILD-005 Slice 5: fail-first Sensor safe_context credential/prose/PEM/non-finite bar.
func TestSlice5SafeContextCredentialProsePEMAndNonFinite(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	ctx := map[string]any{
		"status":          "ok",
		"bearer_full":     "Bearer abc123-token",
		"basic_full":      "Basic Zm9vOmJhcg==",
		"embed_bearer":    "request failed; Authorization: Bearer abc123-token",
		"embed_basic":     "upstream Authorization: Basic Zm9vOmJhcg==",
		"pem_lower":       "-----begin private key-----\nSECRET-SENTINEL\n-----end private key-----",
		"pem_mixed":       "-----Begin Private Key-----\nSECRET-SENTINEL\n-----End Private Key-----",
		"prose_bearer":    "Bearer authentication failed",
		"prose_basic":     "Basic authentication rejected",
		"outcome":         "token validation failed",
		"bad_nan":         math.NaN(),
		"bad_pos_inf":     math.Inf(1),
		"bad_neg_inf":     math.Inf(-1),
		"ok_float":        1.5,
		"ok_int":          7,
	}
	logger.Info("slice5", "safe_context", ctx)
	got := decodeOne(t, &buf)
	safe, ok := got["safe_context"].(map[string]any)
	if !ok {
		t.Fatalf("expected retained safe context: %v", got)
	}
	want := map[string]any{
		"status":       "ok",
		"prose_bearer": "Bearer authentication failed",
		"prose_basic":  "Basic authentication rejected",
		"outcome":      "token validation failed",
		"ok_float":     1.5,
		"ok_int":       float64(7),
	}
	for key, value := range want {
		if safe[key] != value {
			t.Fatalf("S5 retained key %q missing/wrong: got=%v want=%v full=%v", key, safe[key], value, safe)
		}
	}
	for _, key := range []string{
		"bearer_full", "basic_full", "embed_bearer", "embed_basic",
		"pem_lower", "pem_mixed", "bad_nan", "bad_pos_inf", "bad_neg_inf",
	} {
		if _, present := safe[key]; present {
			t.Fatalf("S5 unsafe key %q retained: %v", key, safe)
		}
	}
	raw := strings.ToLower(buf.String())
	for _, sentinel := range []string{
		"abc123-token",
		"zm9vomjhcg==",
		"secret-sentinel",
		"authorization: bearer",
		"authorization: basic",
	} {
		if strings.Contains(raw, sentinel) {
			t.Fatalf("S5 credential sentinel %q leaked: %s", sentinel, buf.String())
		}
	}
}

func assertNoDuplicateTopLevelKeys(t *testing.T, raw string) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("decode start: %v", err)
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		t.Fatalf("expected object start, got %#v", tok)
	}
	seen := map[string]struct{}{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			t.Fatalf("key token: %v", err)
		}
		key, ok := keyTok.(string)
		if !ok {
			t.Fatalf("expected string key, got %#v", keyTok)
		}
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate top-level key %q in %s", key, raw)
		}
		seen[key] = struct{}{}
		var skip any
		if err := dec.Decode(&skip); err != nil {
			t.Fatalf("skip value for %q: %v", key, err)
		}
	}
}

func TestSlice5EmittedJSONHasNoDuplicateTopLevelKeys(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf).With("component", "spoofed-component").With("sensor_id", "sensor-net-02")
	logger.Warn("dup-key-cert",
		"error_code", "CONTROL_SYNC_PENDING",
		"correlation_id", "corr-slice5",
		"service_instance_id", "sensor-instance-rebind",
		"safe_context", map[string]any{"mode": "ok"},
	)
	raw := strings.TrimSpace(buf.String())
	assertNoDuplicateTopLevelKeys(t, raw)
	got := decodeOne(t, &buf)
	if got["component"] != componentName {
		t.Fatalf("component override leaked duplicate/wrong identity: %v", got)
	}
	if got["sensor_id"] != "sensor-net-02" {
		t.Fatalf("sensor_id rebind lost: %v", got)
	}
	if got["service_instance_id"] != "sensor-instance-rebind" {
		t.Fatalf("service_instance_id rebind lost: %v", got)
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


func TestComponentIdentityCannotBeOverridden(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf).With("component", "agentreceiver")
	logger.Info("test")
	got := decodeOne(t, &buf)
	if got["component"] != componentName {
		t.Fatalf("component identity overridden: %v", got)
	}
	raw := buf.String()
	if strings.Count(raw, "\"component\"") != 1 {
		t.Fatalf("component key must be unique: %s", raw)
	}
}

func TestSafeContextRejectsPassphraseKeys(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger.Info("test", "safe_context", map[string]any{
		"mode":           "trusted_continue",
		"key_passphrase": "hunter2",
		"passphrase":     "fictional",
	})
	got := decodeOne(t, &buf)
	safe, ok := got["safe_context"].(map[string]any)
	if !ok || safe["mode"] != "trusted_continue" {
		t.Fatalf("safe context missing: %v", got)
	}
	for _, key := range []string{"key_passphrase", "passphrase"} {
		if _, exists := safe[key]; exists {
			t.Fatalf("passphrase field retained: %v", safe)
		}
	}
	if strings.Contains(strings.ToLower(buf.String()), "hunter2") ||
		strings.Contains(strings.ToLower(buf.String()), "fictional") {
		t.Fatalf("passphrase material leaked: %s", buf.String())
	}
}


type namedEventCode string

func TestNamedStringEventCodeIsPreserved(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger.Warn("test", "error_code", namedEventCode("SENSOR_NAMED_CODE"))
	got := decodeOne(t, &buf)
	if got["event_code"] != "SENSOR_NAMED_CODE" {
		t.Fatalf("named event code lost: %v", got)
	}
}

func TestSafeContextRejectsPwdAlias(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger.Info("test", "safe_context", map[string]any{
		"mode": "trusted_continue",
		"pwd":  "hunter2",
	})
	got := decodeOne(t, &buf)
	safe, ok := got["safe_context"].(map[string]any)
	if !ok || safe["mode"] != "trusted_continue" {
		t.Fatalf("safe context missing: %v", got)
	}
	if _, exists := safe["pwd"]; exists {
		t.Fatalf("pwd alias retained: %v", safe)
	}
	if strings.Contains(strings.ToLower(buf.String()), "hunter2") {
		t.Fatalf("pwd material leaked: %s", buf.String())
	}
}


func TestSafeContextRejectsSNMPPassAliases(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger.Info("test", "safe_context", map[string]any{
		"mode":           "trusted_continue",
		"auth_pass":      "hunter2",
		"priv_pass":      "fictional-private",
		"snmp_auth_pass": "fictional-auth",
		"snmp_priv_pass": "fictional-priv",
	})
	got := decodeOne(t, &buf)
	safe, ok := got["safe_context"].(map[string]any)
	if !ok || safe["mode"] != "trusted_continue" {
		t.Fatalf("safe context missing: %v", got)
	}
	for _, key := range []string{"auth_pass", "priv_pass", "snmp_auth_pass", "snmp_priv_pass"} {
		if _, exists := safe[key]; exists {
			t.Fatalf("SNMP pass alias retained: %s in %v", key, safe)
		}
	}
	raw := strings.ToLower(buf.String())
	for _, forbidden := range []string{"hunter2", "fictional-private", "fictional-auth", "fictional-priv"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("SNMP credential material leaked: %s", buf.String())
		}
	}
}

func TestSafeContextRejectsKeyCredentialAliases(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger.Info("test", "safe_context", map[string]any{
		"mode":       "trusted_continue",
		"auth_key":   "opaque-auth-key",
		"priv_key":   "opaque-priv-key",
		"access_key": "opaque-access-key",
	})
	got := decodeOne(t, &buf)
	safe, ok := got["safe_context"].(map[string]any)
	if !ok || safe["mode"] != "trusted_continue" {
		t.Fatalf("safe context missing: %v", got)
	}
	for _, key := range []string{"auth_key", "priv_key", "access_key"} {
		if _, exists := safe[key]; exists {
			t.Fatalf("credential key alias retained: %s in %v", key, safe)
		}
	}
	raw := strings.ToLower(buf.String())
	for _, forbidden := range []string{"opaque-auth-key", "opaque-priv-key", "opaque-access-key"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("credential material leaked: %s", buf.String())
		}
	}
}

func TestAgentIDUsesBoundedStringContract(t *testing.T) {
	for _, agentID := range []string{"agent/site 01", "Agent With Spaces"} {
		t.Run(agentID, func(t *testing.T) {
			var buf bytes.Buffer
			logger := newTestLogger(&buf)
			logger.Warn("test", "agent_id", agentID)
			got := decodeOne(t, &buf)
			if got["agent_id"] != agentID {
				t.Fatalf("contract-valid agent_id lost: %q log=%v", agentID, got)
			}
		})
	}
}

func TestSafeContextRejectsPrivacyAndBearerAliases(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger.Info("test", "safe_context", map[string]any{
		"mode":              "trusted_continue",
		"privacy_pass":      "opaque-privacy-pass",
		"snmp_privacy_pass": "opaque-snmp-privacy-pass",
		"privacy_key":       "opaque-privacy-key",
		"bearer":            "opaque-bearer-token",
	})
	got := decodeOne(t, &buf)
	safe, ok := got["safe_context"].(map[string]any)
	if !ok || safe["mode"] != "trusted_continue" {
		t.Fatalf("safe context missing: %v", got)
	}
	for _, key := range []string{"privacy_pass", "snmp_privacy_pass", "privacy_key", "bearer"} {
		if _, exists := safe[key]; exists {
			t.Fatalf("credential alias retained: %s in %v", key, safe)
		}
	}
	for _, forbidden := range []string{"opaque-privacy-pass", "opaque-snmp-privacy-pass", "opaque-privacy-key", "opaque-bearer-token"} {
		if strings.Contains(buf.String(), forbidden) {
			t.Fatalf("credential material leaked: %s", buf.String())
		}
	}
}

func TestAgentIDRejectsControlCharacters(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger.Warn("test", "agent_id", "agent\r\nid")
	got := decodeOne(t, &buf)
	if _, ok := got["agent_id"]; ok {
		t.Fatalf("control-bearing agent_id emitted: %v", got)
	}
}
