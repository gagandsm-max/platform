package coreclient

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const goldenControlChecksum = "ec1e58221b8f3bb05ce0ddc4d8104b2e6bfce709e0de4cc07062e80919346f1d"

func controlTestClient(t *testing.T, srv *httptest.Server, maxBytes int) *Client {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	return &Client{
		baseURL: u,
		httpClient: &http.Client{
			Transport: srv.Client().Transport,
			Timeout:   5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return newError(CodeProtocol, "Core redirects are not followed", nil, false)
			},
		},
		maxResponseBytes: maxBytes,
	}
}

func TestFetchControlDesiredState204(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != controlDesiredStatePath || r.Method != http.MethodGet {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	out, err := controlTestClient(t, srv, 0).FetchControlDesiredState(context.Background())
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
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			out, err := controlTestClient(t, srv, 0).FetchControlDesiredState(context.Background())
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
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(make([]byte, 2048))
	}))
	defer srv.Close()
	_, err := controlTestClient(t, srv, 512).FetchControlDesiredState(context.Background())
	if !IsCode(err, CodeResponseTooLarge) {
		t.Fatalf("err=%v", err)
	}
}

func TestAckControlAppliedStateValidAndBinding(t *testing.T) {
	checksum := goldenControlChecksum
	var sawBody []byte
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != controlAppliedStatePath {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		sawBody = append([]byte(nil), body...)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ACCEPTED","schemaVersion":1,"generation":3,"checksum":"`+checksum+`","outcome":"APPLIED"}`)
	}))
	defer srv.Close()
	resp, err := controlTestClient(t, srv, 0).AckControlAppliedState(context.Background(), ControlAppliedAckRequest{
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
		srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer srv.Close()
		_, err := controlTestClient(t, srv, 0).AckControlAppliedState(context.Background(), ControlAppliedAckRequest{
			SchemaVersion: 1, Generation: 1, Checksum: checksum, Outcome: "APPLIED", RejectionCode: "X",
		})
		if err == nil {
			t.Fatal("expected failure")
		}
	})
	t.Run("rejected_without_code_forbidden", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		defer srv.Close()
		_, err := controlTestClient(t, srv, 0).AckControlAppliedState(context.Background(), ControlAppliedAckRequest{
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
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			_, err := controlTestClient(t, srv, 0).AckControlAppliedState(context.Background(), ControlAppliedAckRequest{
				SchemaVersion: 1, Generation: 1, Checksum: checksum, Outcome: "APPLIED",
			})
			if err == nil {
				t.Fatal("expected ACK binding/protocol failure")
			}
		})
	}

	t.Run("valid_rejected", func(t *testing.T) {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"ACCEPTED","schemaVersion":1,"generation":1,"checksum":"`+checksum+`","outcome":"REJECTED"}`)
		}))
		defer srv.Close()
		_, err := controlTestClient(t, srv, 0).AckControlAppliedState(context.Background(), ControlAppliedAckRequest{
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
