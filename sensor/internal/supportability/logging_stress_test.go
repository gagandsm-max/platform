//go:build stress

package supportability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	stressWorkers     = 32
	stressEventsEach  = 2000
	stressTotalEvents = stressWorkers * stressEventsEach // 64_000
	stressCredCases   = 10000
)

type stressCounters struct {
	events              atomic.Int64
	malformedJSON       atomic.Int64
	duplicateKeys       atomic.Int64
	identityMutation    atomic.Int64
	credentialLeaks     atomic.Int64
	nonFiniteLeaks      atomic.Int64
	allowlistViolations atomic.Int64
	panics              atomic.Int64
}

var sensorAllowedTop = map[string]struct{}{
	"ts": {}, "level": {}, "component": {}, "version": {}, "service_instance_id": {},
	"message": {}, "agent_id": {}, "sensor_id": {}, "node_id": {}, "tenant_id": {},
	"event_code": {}, "correlation_id": {}, "request_id": {}, "job_id": {}, "entity_id": {},
	"protocol": {}, "result": {}, "retry_count": {}, "safe_context": {},
}

type validatingWriter struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	t         *testing.T
	c         *stressCounters
	expect    map[string]string
	sentinels []string
}

func (w *validatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	for {
		data := w.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			break
		}
		line := append([]byte(nil), data[:idx]...)
		w.buf.Next(idx + 1)
		w.validateLine(line)
	}
	return n, err
}

func (w *validatingWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if rem := bytes.TrimSpace(w.buf.Bytes()); len(rem) > 0 {
		w.validateLine(append([]byte(nil), rem...))
		w.buf.Reset()
	}
}

func (w *validatingWriter) validateLine(raw []byte) {
	w.c.events.Add(1)
	line := bytes.TrimSpace(raw)
	if len(line) == 0 {
		return
	}
	lower := strings.ToLower(string(line))
	for _, s := range w.sentinels {
		if strings.Contains(lower, strings.ToLower(s)) {
			w.c.credentialLeaks.Add(1)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	tok, err := dec.Token()
	if err != nil {
		w.c.malformedJSON.Add(1)
		return
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		w.c.malformedJSON.Add(1)
		return
	}
	seen := map[string]struct{}{}
	obj := map[string]any{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			w.c.malformedJSON.Add(1)
			return
		}
		key, ok := keyTok.(string)
		if !ok {
			w.c.malformedJSON.Add(1)
			return
		}
		if _, exists := seen[key]; exists {
			w.c.duplicateKeys.Add(1)
		}
		seen[key] = struct{}{}
		if _, allowed := sensorAllowedTop[key]; !allowed {
			w.c.allowlistViolations.Add(1)
		}
		var val any
		if err := dec.Decode(&val); err != nil {
			w.c.malformedJSON.Add(1)
			return
		}
		obj[key] = val
		if key == "safe_context" {
			if m, ok := val.(map[string]any); ok {
				for _, v := range m {
					if f, ok := v.(float64); ok && (math.IsNaN(f) || math.IsInf(f, 0)) {
						w.c.nonFiniteLeaks.Add(1)
					}
				}
			}
		}
	}
	end, err := dec.Token()
	if err != nil || end != json.Delim('}') {
		w.c.malformedJSON.Add(1)
		return
	}
	for k, want := range w.expect {
		got, _ := obj[k].(string)
		if got != want {
			w.c.identityMutation.Add(1)
		}
	}
}

func TestStressConcurrentStructuredLogging(t *testing.T) {
	counters := &stressCounters{}
	sentinels := []string{
		"STRESS-BEARER-SENTINEL",
		"U1RSRVNTLUJBU0lD",
		"STRESS-PEM-SENTINEL",
	}
	expect := map[string]string{
		"component":           componentName,
		"version":             "0.1.0-stress",
		"service_instance_id": "sensor-stress-instance-01",
		"sensor_id":           "sensor-net-stress-01",
		"agent_id":            "agent-linux-stress-01",
	}
	w := &validatingWriter{t: t, c: counters, expect: expect, sentinels: sentinels}
	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	logger := NewJSONLogger(w, level, expect["version"], expect["service_instance_id"]).
		With("sensor_id", expect["sensor_id"]).With("agent_id", expect["agent_id"])

	var wg sync.WaitGroup
	for i := 0; i < stressWorkers; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					counters.panics.Add(1)
				}
			}()
			for n := 0; n < stressEventsEach; n++ {
				corr := fmt.Sprintf("corr-w%d-e%d", worker, n)
				ctx := map[string]any{
					"mode":   "ok",
					"worker": worker,
					"seq":    n,
				}
				switch n % 11 {
				case 0:
					ctx["detail"] = "Authorization=Bearer STRESS-BEARER-SENTINEL"
				case 1:
					ctx["detail"] = "Authorization: Basic U1RSRVNTLUJBU0lD"
				case 2:
					ctx["detail"] = "-----BEGIN PRIVATE KEY-----\nSTRESS-PEM-SENTINEL\n-----END PRIVATE KEY-----"
				case 3:
					ctx["password"] = "should-omit"
				case 4:
					ctx["prose"] = "Bearer authentication failed"
				case 5:
					ctx["prose"] = "Authorization validation failed"
				case 6:
					ctx["bad_nan"] = math.NaN()
					ctx["ok_f"] = 1.5
				case 7:
					ctx["nested"] = map[string]any{"x": 1}
				case 8:
					for k := 0; k < 20; k++ {
						ctx[fmt.Sprintf("k%d", k)] = k
					}
				}
				logger.Info("stress event",
					"error_code", "STRESS_EVENT",
					"correlation_id", corr,
					"request_id", fmt.Sprintf("req-%d-%d", worker, n),
					"job_id", fmt.Sprintf("job-%d", worker),
					"retry_count", int64(n%5),
					"component", "spoofed",
					"version", "9.9.9",
					"service_instance_id", "rebind-instance",
					"agent_id", "agent-rebind",
					"sensor_id", "sensor-rebind",
					"safe_context", ctx,
				)
			}
		}(i)
	}
	wg.Wait()
	w.flush()

	if counters.events.Load() < stressTotalEvents {
		t.Fatalf("expected >=%d events, got %d", stressTotalEvents, counters.events.Load())
	}
	assertStressClean(t, counters)
	t.Logf("SENSOR_STRESS events=%d malformed=%d dup=%d id_mut=%d leaks=%d nonfinite=%d panics=%d",
		counters.events.Load(), counters.malformedJSON.Load(), counters.duplicateKeys.Load(),
		counters.identityMutation.Load(), counters.credentialLeaks.Load(), counters.nonFiniteLeaks.Load(),
		counters.panics.Load())
}

func TestStressSafeContextBoundaryAttack(t *testing.T) {
	var buf bytes.Buffer
	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	logger := NewJSONLogger(&buf, level, "0.1.0-stress", "sensor-stress-instance-01").
		With("sensor_id", "sensor-net-stress-01").With("agent_id", "agent-linux-stress-01")

	overKey := "k" + strings.Repeat("a", 64)
	maxKey := "k" + strings.Repeat("b", 63)
	maxVal := strings.Repeat("v", 256)
	overVal := strings.Repeat("v", 257)
	ctx := map[string]any{
		maxKey:     maxVal,
		overKey:    "x",
		"too_long": overVal,
		"nested":   map[string]any{"a": 1},
		"arr":      []any{1, 2},
		"nilv":     nil,
		"okb":      true,
		"oki":      int64(7),
		"oku":      uint64(8),
		"okf":      1.25,
		"nan":      math.NaN(),
		"pinf":     math.Inf(1),
		"ninf":     math.Inf(-1),
		"ctrl":     "bad\x00value",
		"1bad":     "no",
	}
	for i := 0; i < 20; i++ {
		ctx[fmt.Sprintf("extra%d", i)] = i
	}
	logger.Info("boundary", "safe_context", ctx)
	got := decodeOne(t, &buf)
	safe, _ := got["safe_context"].(map[string]any)
	if len(safe) > 16 {
		t.Fatalf("safe_context exceeded 16 props: %d", len(safe))
	}
	for _, forbidden := range []string{overKey, "too_long", "nested", "arr", "nan", "pinf", "ninf", "ctrl", "1bad"} {
		if _, ok := safe[forbidden]; ok {
			t.Fatalf("boundary key retained: %s in %v", forbidden, safe)
		}
	}
	raw := buf.String()
	if strings.Contains(raw, "NaN") || strings.Contains(raw, "Infinity") {
		t.Fatalf("non-finite leaked: %s", raw)
	}
}

func TestStressCredentialRedactionRepetition(t *testing.T) {
	counters := &stressCounters{}
	sentinels := []string{
		"STRESS-BEARER-SENTINEL",
		"U1RSRVNTLUJBU0lD",
		"STRESS-PEM-SENTINEL",
		"stress-password-sentinel",
	}
	expect := map[string]string{
		"component":           componentName,
		"version":             "0.1.0-stress",
		"service_instance_id": "sensor-stress-instance-01",
		"sensor_id":           "sensor-net-stress-01",
		"agent_id":            "agent-linux-stress-01",
	}
	w := &validatingWriter{t: t, c: counters, expect: expect, sentinels: sentinels}
	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	logger := NewJSONLogger(w, level, expect["version"], expect["service_instance_id"]).
		With("sensor_id", expect["sensor_id"]).With("agent_id", expect["agent_id"])

	cases := []map[string]any{
		{"detail": "Bearer STRESS-BEARER-SENTINEL"},
		{"detail": "Basic U1RSRVNTLUJBU0lD"},
		{"detail": "Authorization: Bearer STRESS-BEARER-SENTINEL"},
		{"detail": "Authorization = Bearer STRESS-BEARER-SENTINEL"},
		{"detail": "Authorization: Basic U1RSRVNTLUJBU0lD"},
		{"detail": "Authorization = Basic U1RSRVNTLUJBU0lD"},
		{"detail": "authorization=bearer STRESS-BEARER-SENTINEL"},
		{"detail": "authorization=basic U1RSRVNTLUJBU0lD"},
		{"pem": "-----begin private key-----\nSTRESS-PEM-SENTINEL\n-----end private key-----"},
		{"password": "stress-password-sentinel"},
		{"community": "stress-password-sentinel"},
		{"api_token": "stress-password-sentinel"},
		{"cookie": "stress-password-sentinel"},
		{"passphrase": "stress-password-sentinel"},
		{"auth_key": "stress-password-sentinel"},
		{"privacy_key": "stress-password-sentinel"},
		{"prose": "Bearer authentication failed"},
		{"prose": "Basic authentication rejected"},
		{"outcome": "token validation failed"},
		{"prose": "Authorization validation failed"},
		{"prose": "authentication scheme Bearer rejected"},
	}
	for i := 0; i < stressCredCases; i++ {
		ctx := cases[i%len(cases)]
		copyCtx := map[string]any{"status": "ok"}
		for k, v := range ctx {
			copyCtx[k] = v
		}
		logger.Info("cred-stress", "safe_context", copyCtx)
	}
	w.flush()
	if counters.credentialLeaks.Load() != 0 {
		t.Fatalf("credential sentinel leaks=%d", counters.credentialLeaks.Load())
	}
	assertStressClean(t, counters)
	// Prove prose retained on a dedicated emit.
	var buf bytes.Buffer
	logger2 := NewJSONLogger(&buf, level, expect["version"], expect["service_instance_id"]).
		With("sensor_id", expect["sensor_id"]).With("agent_id", expect["agent_id"])
	logger2.Info("prose", "safe_context", map[string]any{
		"a": "Bearer authentication failed",
		"b": "Basic authentication rejected",
		"c": "token validation failed",
		"d": "Authorization validation failed",
		"e": "authentication scheme Bearer rejected",
	})
	got := decodeOne(t, &buf)
	safe := got["safe_context"].(map[string]any)
	for _, k := range []string{"a", "b", "c", "d", "e"} {
		if _, ok := safe[k]; !ok {
			t.Fatalf("benign prose key %s omitted: %v", k, safe)
		}
	}
}

func TestStressMemoryWindows(t *testing.T) {
	level := new(slog.LevelVar)
	level.Set(slog.LevelInfo)
	var discard countingDiscard
	logger := NewJSONLogger(&discard, level, "0.1.0-stress", "sensor-stress-instance-01").
		With("sensor_id", "sensor-net-stress-01").With("agent_id", "agent-linux-stress-01")

	runtime.GC()
	var before, after1, after2 runtime.MemStats
	runtime.ReadMemStats(&before)

	runWindow := func() {
		for i := 0; i < 20000; i++ {
			logger.Info("mem",
				"error_code", "STRESS_MEM",
				"correlation_id", fmt.Sprintf("c-%d", i),
				"safe_context", map[string]any{"i": i, "mode": "ok"},
			)
		}
	}
	runWindow()
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	runtime.ReadMemStats(&after1)
	runWindow()
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	runtime.ReadMemStats(&after2)

	growth := int64(after2.HeapAlloc) - int64(after1.HeapAlloc)
	t.Logf("SENSOR_MEM before=%d after1=%d after2=%d growth=%d bytes discard_bytes=%d",
		before.HeapAlloc, after1.HeapAlloc, after2.HeapAlloc, growth, discard.n.Load())
	const limit = 32 * 1024 * 1024
	if growth > limit {
		t.Fatalf("post-GC heap growth across equal windows %d exceeds 32MiB", growth)
	}
}

type countingDiscard struct{ n atomic.Int64 }

func (c *countingDiscard) Write(p []byte) (int, error) {
	c.n.Add(int64(len(p)))
	return len(p), nil
}

func assertStressClean(t *testing.T, c *stressCounters) {
	t.Helper()
	if c.panics.Load() != 0 {
		t.Fatalf("panics=%d", c.panics.Load())
	}
	if c.malformedJSON.Load() != 0 {
		t.Fatalf("malformedJSON=%d", c.malformedJSON.Load())
	}
	if c.duplicateKeys.Load() != 0 {
		t.Fatalf("duplicateKeys=%d", c.duplicateKeys.Load())
	}
	if c.identityMutation.Load() != 0 {
		t.Fatalf("identityMutation=%d", c.identityMutation.Load())
	}
	if c.credentialLeaks.Load() != 0 {
		t.Fatalf("credentialLeaks=%d", c.credentialLeaks.Load())
	}
	if c.nonFiniteLeaks.Load() != 0 {
		t.Fatalf("nonFiniteLeaks=%d", c.nonFiniteLeaks.Load())
	}
	if c.allowlistViolations.Load() != 0 {
		t.Fatalf("allowlistViolations=%d", c.allowlistViolations.Load())
	}
}
