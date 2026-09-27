package coreclient

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const goldenControlChecksum = "ec1e58221b8f3bb05ce0ddc4d8104b2e6bfce709e0de4cc07062e80919346f1d"

// controlMTLSClient builds a checked production Client via New() against an mTLS test server.
func controlMTLSClient(t *testing.T, maxBytes int, handler http.Handler) *Client {
	t.Helper()
	pki := makeTestPKI(t)
	srv := startMTLSServer(t, pki, handler)
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	return newTestClient(t, srv.URL, pki, maxBytes)
}

func TestFetchControlDesiredState204(t *testing.T) {
	client := controlMTLSClient(t, 0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != controlDesiredStatePath || r.Method != http.MethodGet {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			t.Fatal("missing client certificate")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	out, err := client.FetchControlDesiredState(context.Background())
	if err != nil || out != nil {
		t.Fatalf("out=%v err=%v", out, err)
	}
}

func TestFetchControlDesiredStateStrictParsing(t *testing.T) {
	cfg := goldenControlConfigJSON(t)
	checksum := goldenControlChecksum
	valid := mustDesiredJSON(t, 1, checksum, cfg)

	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{name: "valid", body: valid, ok: true},
		{name: "schema_v2_safe_binding", body: `{"schemaVersion":2,"desiredGeneration":1,"desiredChecksum":"` + checksum + `"}`, ok: true},
		{name: "schema_v2_future_fields", body: `{"schemaVersion":2,"desiredGeneration":1,"desiredChecksum":"` + checksum + `","future_field":"ok","nested":{"a":1}}`, ok: true},
		{name: "schema_v2_missing_gen", body: `{"schemaVersion":2,"desiredChecksum":"` + checksum + `"}`, ok: false},
		{name: "schema_v2_missing_checksum", body: `{"schemaVersion":2,"desiredGeneration":1}`, ok: false},
		{name: "schema_v2_invalid_checksum", body: `{"schemaVersion":2,"desiredGeneration":1,"desiredChecksum":"nothex"}`, ok: false},
		{name: "schema_v2_gen_negative", body: `{"schemaVersion":2,"desiredGeneration":-1,"desiredChecksum":"` + checksum + `"}`, ok: false},
		{name: "schema_v2_gen_zero", body: `{"schemaVersion":2,"desiredGeneration":0,"desiredChecksum":"` + checksum + `"}`, ok: false},
		{name: "schema_v2_gen_overflow", body: `{"schemaVersion":2,"desiredGeneration":9007199254740992,"desiredChecksum":"` + checksum + `"}`, ok: false},
		{name: "schema_zero", body: `{"schemaVersion":0,"desiredGeneration":1,"desiredChecksum":"` + checksum + `"}`, ok: false},
		{name: "schema_negative", body: `{"schemaVersion":-1,"desiredGeneration":1,"desiredChecksum":"` + checksum + `"}`, ok: false},
		{name: "schema_decimal", body: `{"schemaVersion":1.5,"desiredGeneration":1,"desiredChecksum":"` + checksum + `"}`, ok: false},
		{name: "schema_exponent", body: `{"schemaVersion":1e2,"desiredGeneration":1,"desiredChecksum":"` + checksum + `"}`, ok: false},
		{name: "schema_string", body: `{"schemaVersion":"1","desiredGeneration":1,"desiredChecksum":"` + checksum + `"}`, ok: false},
		{name: "schema_null", body: `{"schemaVersion":null,"desiredGeneration":1,"desiredChecksum":"` + checksum + `"}`, ok: false},
		{name: "unknown_top", body: strings.Replace(valid, `"schemaVersion"`, `"evil":true,"schemaVersion"`, 1), ok: false},
		{name: "trailing", body: valid + `{"x":1}`, ok: false},
		{name: "non_object", body: `[]`, ok: false},
		{name: "malformed", body: `{`, ok: false},
		{name: "gen_zero", body: mustDesiredJSON(t, 0, checksum, cfg), ok: false},
		{name: "gen_negative", body: strings.Replace(valid, `"desiredGeneration":1`, `"desiredGeneration":-1`, 1), ok: false},
		{name: "gen_over_max", body: strings.Replace(valid, `"desiredGeneration":1`, `"desiredGeneration":9007199254740992`, 1), ok: false},
		{name: "gen_decimal", body: strings.Replace(valid, `"desiredGeneration":1`, `"desiredGeneration":1.5`, 1), ok: false},
		{name: "gen_exponent", body: strings.Replace(valid, `"desiredGeneration":1`, `"desiredGeneration":1e2`, 1), ok: false},
		{name: "checksum_63", body: strings.Replace(valid, checksum, checksum[:63], 1), ok: false},
		{name: "checksum_65", body: strings.Replace(valid, checksum, checksum+"a", 1), ok: false},
		{name: "checksum_upper", body: strings.Replace(valid, checksum, strings.ToUpper(checksum), 1), ok: false},
		{name: "checksum_mixed", body: strings.Replace(valid, checksum, "Ec1e58221b8f3bb05ce0ddc4d8104b2e6bfce709e0de4cc07062e80919346f1d", 1), ok: false},
		{name: "checksum_nonhex", body: strings.Replace(valid, checksum, "gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg", 1), ok: false},
		{name: "null_desired_gen", body: strings.Replace(valid, `"desiredGeneration":1`, `"desiredGeneration":null`, 1), ok: false},
		{name: "unknown_config_field", body: strings.Replace(valid, `"logLevel"`, `"secret":"x","logLevel"`, 1), ok: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.body
			client := controlMTLSClient(t, 0, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			out, err := client.FetchControlDesiredState(context.Background())
			if tc.ok {
				if err != nil || out == nil || out.DesiredChecksum != checksum {
					t.Fatalf("expected ok, out=%v err=%v", out, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected failure, got %#v", out)
			}
		})
	}
}

func TestFetchControlDesiredStateOversized(t *testing.T) {
	client := controlMTLSClient(t, 512, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(make([]byte, 2048))
	}))
	_, err := client.FetchControlDesiredState(context.Background())
	if !IsCode(err, CodeResponseTooLarge) {
		t.Fatalf("err=%v", err)
	}
}

func TestFetchControlDesiredStateUnsupportedSchemaWire(t *testing.T) {
	checksum := goldenControlChecksum
	client := controlMTLSClient(t, 0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != controlDesiredStatePath || r.Method != http.MethodGet {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			t.Fatal("missing client certificate on unsupported-schema wire path")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"schemaVersion":2,"desiredGeneration":77,"desiredChecksum":"`+checksum+`","futureField":{"cool":true},"configuration":{"unknown":true}}`)
	}))

	out, err := client.FetchControlDesiredState(context.Background())
	if err != nil {
		t.Fatalf("FetchControlDesiredState failed: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil desired state")
	}
	if out.SchemaVersion != 2 {
		t.Fatalf("expected SchemaVersion 2, got %d", out.SchemaVersion)
	}
	if out.DesiredGeneration != 77 {
		t.Fatalf("expected DesiredGeneration 77, got %d", out.DesiredGeneration)
	}
	if out.DesiredChecksum != checksum {
		t.Fatalf("expected DesiredChecksum %s, got %s", checksum, out.DesiredChecksum)
	}
	if out.Configuration.LogLevel != "" {
		t.Fatalf("configuration should not be parsed for unsupported schema, got logLevel=%s", out.Configuration.LogLevel)
	}
	if out.Configuration.Scheduler.MaxWorkers != 0 || len(out.Configuration.Scheduler.Pools) != 0 {
		t.Fatalf("unsupported schema must not interpret v1 configuration: %+v", out.Configuration)
	}
}

func TestAckControlAppliedStateValidAndBinding(t *testing.T) {
	checksum := goldenControlChecksum
	var sawBody []byte
	client := controlMTLSClient(t, 0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != controlAppliedStatePath {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			t.Fatal("missing client certificate")
		}
		body, _ := io.ReadAll(r.Body)
		sawBody = append([]byte(nil), body...)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ACCEPTED","schemaVersion":1,"generation":3,"checksum":"`+checksum+`","outcome":"APPLIED"}`)
	}))
	resp, err := client.AckControlAppliedState(context.Background(), ControlAppliedAckRequest{
		SchemaVersion: 1, Generation: 3, Checksum: checksum, Outcome: "APPLIED",
	})
	if err != nil || resp.Status != "ACCEPTED" {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	lower := strings.ToLower(string(sawBody))
	for _, forbidden := range []string{"tenantid", "tenant_id", "sensorid", "sensor_id", "actor", "desired_by"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("identity field %q in ACK body %s", forbidden, sawBody)
		}
	}
}

func TestAckControlAppliedStateRejectedAndNegatives(t *testing.T) {
	checksum := goldenControlChecksum

	t.Run("applied_with_rejection_forbidden", func(t *testing.T) {
		client := controlMTLSClient(t, 0, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		_, err := client.AckControlAppliedState(context.Background(), ControlAppliedAckRequest{
			SchemaVersion: 1, Generation: 1, Checksum: checksum, Outcome: "APPLIED", RejectionCode: "X",
		})
		if err == nil {
			t.Fatal("expected failure")
		}
	})
	t.Run("rejected_without_code_forbidden", func(t *testing.T) {
		client := controlMTLSClient(t, 0, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		_, err := client.AckControlAppliedState(context.Background(), ControlAppliedAckRequest{
			SchemaVersion: 1, Generation: 1, Checksum: checksum, Outcome: "REJECTED",
		})
		if err == nil {
			t.Fatal("expected failure")
		}
	})

	negatives := []struct {
		name string
		body string
	}{
		{name: "generation_mismatch", body: `{"status":"ACCEPTED","schemaVersion":1,"generation":9,"checksum":"` + checksum + `","outcome":"APPLIED"}`},
		{name: "checksum_mismatch", body: `{"status":"ACCEPTED","schemaVersion":1,"generation":1,"checksum":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","outcome":"APPLIED"}`},
		{name: "outcome_mismatch", body: `{"status":"ACCEPTED","schemaVersion":1,"generation":1,"checksum":"` + checksum + `","outcome":"REJECTED"}`},
		{name: "unknown_field", body: `{"status":"ACCEPTED","schemaVersion":1,"generation":1,"checksum":"` + checksum + `","outcome":"APPLIED","extra":1}`},
		{name: "trailing", body: `{"status":"ACCEPTED","schemaVersion":1,"generation":1,"checksum":"` + checksum + `","outcome":"APPLIED"}{}`},
		{name: "malformed", body: `{`},
	}
	for _, tc := range negatives {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.body
			client := controlMTLSClient(t, 0, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			_, err := client.AckControlAppliedState(context.Background(), ControlAppliedAckRequest{
				SchemaVersion: 1, Generation: 1, Checksum: checksum, Outcome: "APPLIED",
			})
			if err == nil {
				t.Fatal("expected ACK binding/protocol failure")
			}
		})
	}

	t.Run("valid_rejected", func(t *testing.T) {
		client := controlMTLSClient(t, 0, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"ACCEPTED","schemaVersion":1,"generation":1,"checksum":"`+checksum+`","outcome":"REJECTED"}`)
		}))
		_, err := client.AckControlAppliedState(context.Background(), ControlAppliedAckRequest{
			SchemaVersion: 1, Generation: 1, Checksum: checksum, Outcome: "REJECTED", RejectionCode: "SENSOR_CONTROL_CONFIG_INVALID",
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestLocalChecksumParityMatchesGolden(t *testing.T) {
	// Independent framing check inside coreclient tests without importing control package.
	digest := computeTestChecksum(map[string]any{
		"logLevel": "INFO",
		"scheduler": map[string]any{
			"maxWorkers": 4, "maxQueueDepth": 16, "maxTrackedAssignments": 32, "maxTrackedJobs": 64,
			"maxPayloadBytes": 1048576, "defaultExecutionTimeoutMs": 5000, "shutdownGracePeriodMs": 10000,
			"retryMaxAttempts": 3, "retryInitialBackoffMs": 100, "retryMaxBackoffMs": 1000, "retryJitterBasisPoints": 500,
			"pools": map[string]any{
				"SYNTHETIC":    map[string]any{"workers": 1, "queueDepth": 4},
				"NETWORK_POLL": map[string]any{"workers": 2, "queueDepth": 8},
			},
		},
		"transport": map[string]any{
			"requestTimeoutMs": 15000, "maxRequestBytes": 1048576, "maxResponseBytes": 1048576,
			"uploadBatchRecords": 100, "uploadBatchBytes": 524288,
		},
	})
	if digest != goldenControlChecksum {
		t.Fatalf("got=%s want=%s", digest, goldenControlChecksum)
	}
}

func goldenControlConfigJSON(t *testing.T) string {
	t.Helper()
	cfg := map[string]any{
		"logLevel": "INFO",
		"scheduler": map[string]any{
			"maxWorkers": 4, "maxQueueDepth": 16, "maxTrackedAssignments": 32, "maxTrackedJobs": 64,
			"maxPayloadBytes": 1048576, "defaultExecutionTimeoutMs": 5000, "shutdownGracePeriodMs": 10000,
			"retryMaxAttempts": 3, "retryInitialBackoffMs": 100, "retryMaxBackoffMs": 1000, "retryJitterBasisPoints": 500,
			"pools": map[string]any{
				"SYNTHETIC":    map[string]any{"workers": 1, "queueDepth": 4},
				"NETWORK_POLL": map[string]any{"workers": 2, "queueDepth": 8},
			},
		},
		"transport": map[string]any{
			"requestTimeoutMs": 15000, "maxRequestBytes": 1048576, "maxResponseBytes": 1048576,
			"uploadBatchRecords": 100, "uploadBatchBytes": 524288,
		},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustDesiredJSON(t *testing.T, generation uint64, checksum, cfgJSON string) string {
	t.Helper()
	gen, err := json.Marshal(generation)
	if err != nil {
		t.Fatal(err)
	}
	return `{"schemaVersion":1,"desiredGeneration":` + string(gen) + `,"desiredChecksum":"` + checksum + `","configuration":` + cfgJSON + `,"appliedGeneration":0,"appliedChecksum":null,"observedGeneration":0,"observedChecksum":null,"observedOutcome":null}`
}

func computeTestChecksum(cfg map[string]any) string {
	h := sha256.New()
	h.Write([]byte("platform.sensor.control.desired.v1"))
	h.Write([]byte{0x00})
	frame := func(value string) {
		b := []byte(value)
		var lenBuf [4]byte
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(b)))
		_, _ = h.Write(lenBuf[:])
		_, _ = h.Write(b)
	}
	write := func(name, value string) {
		frame(name)
		frame(value)
	}
	scheduler := cfg["scheduler"].(map[string]any)
	transport := cfg["transport"].(map[string]any)
	write("logLevel", cfg["logLevel"].(string))
	for _, key := range []string{
		"maxWorkers", "maxQueueDepth", "maxTrackedAssignments", "maxTrackedJobs", "maxPayloadBytes",
		"defaultExecutionTimeoutMs", "shutdownGracePeriodMs", "retryMaxAttempts", "retryInitialBackoffMs",
		"retryMaxBackoffMs", "retryJitterBasisPoints",
	} {
		write(key, strconv.Itoa(scheduler[key].(int)))
	}
	pools := scheduler["pools"].(map[string]any)
	names := make([]string, 0, len(pools))
	for name := range pools {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pool := pools[name].(map[string]any)
		write("pool", name)
		write("workers", strconv.Itoa(pool["workers"].(int)))
		write("queueDepth", strconv.Itoa(pool["queueDepth"].(int)))
	}
	for _, key := range []string{"requestTimeoutMs", "maxRequestBytes", "maxResponseBytes", "uploadBatchRecords", "uploadBatchBytes"} {
		write(key, strconv.Itoa(transport[key].(int)))
	}
	return hex.EncodeToString(h.Sum(nil))
}
