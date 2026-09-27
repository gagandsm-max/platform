package coreclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestResolveAgentAuthorityAcceptedContract(t *testing.T) {
	pki := makeTestPKI(t)
	var gotBody map[string]any
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != agentAuthorityResolvePath || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                "ACCEPTED",
			"agent_id":              "agent-01",
			"tenant_id":             "11111111-1111-1111-1111-111111111111",
			"site_id":               "22222222-2222-2222-2222-222222222222",
			"sensor_id":             "sensor-infra-01",
			"assignment_generation": 3,
			"resolved_at":           time.Now().UTC().Format(time.RFC3339Nano),
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	view, err := client.ResolveAgentAuthority(context.Background(), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if gotBody["fingerprint_sha256"] != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("fingerprint not normalized/sent: %#v", gotBody)
	}
	if view.AgentID != "agent-01" || view.SensorID != "sensor-infra-01" || view.AssignmentGeneration != 3 {
		t.Fatalf("unexpected view: %+v", view)
	}
}

func TestResolveAgentAuthorityDefinitiveVsTransient(t *testing.T) {
	pki := makeTestPKI(t)
	cases := []struct {
		name      string
		status    int
		body      map[string]any
		transient bool
		code      string
	}{
		{
			name:      "definitive_missing",
			status:    http.StatusForbidden,
			body:      map[string]any{"status": "ERROR", "code": "ERR_AGENT_ASSIGNMENT_MISSING", "message": "missing"},
			transient: false,
			code:      "ERR_AGENT_ASSIGNMENT_MISSING",
		},
		{
			name:      "transient_unavailable_on_403",
			status:    http.StatusForbidden,
			body:      map[string]any{"status": "ERROR", "code": "ERR_AGENT_AUTHORITY_UNAVAILABLE", "message": "db"},
			transient: true,
			code:      "ERR_AGENT_AUTHORITY_UNAVAILABLE",
		},
		{
			name:      "transient_503",
			status:    http.StatusServiceUnavailable,
			body:      map[string]any{"status": "ERROR", "code": "ERR_INTERNAL", "message": "down"},
			transient: true,
			code:      "ERR_AGENT_AUTHORITY_UNAVAILABLE",
		},
		{
			name:      "transient_429",
			status:    http.StatusTooManyRequests,
			body:      map[string]any{"status": "ERROR", "code": "ERR_RATE", "message": "slow"},
			transient: true,
			code:      "ERR_AGENT_AUTHORITY_UNAVAILABLE",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			client := newTestClient(t, server.URL, pki, 4096)
			_, err := client.ResolveAgentAuthority(context.Background(), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
			var authErr *AgentAuthorityError
			if !errors.As(err, &authErr) {
				t.Fatalf("expected AgentAuthorityError, got %v", err)
			}
			if authErr.Transient != tc.transient {
				t.Fatalf("transient=%v want %v code=%s", authErr.Transient, tc.transient, authErr.Code)
			}
			if authErr.Code != tc.code {
				t.Fatalf("code=%s want %s", authErr.Code, tc.code)
			}
		})
	}
}

func TestResolveAgentAuthorityRejectsGenerationAboveMaxSafe(t *testing.T) {
	pki := makeTestPKI(t)
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                "ACCEPTED",
			"agent_id":              "agent-01",
			"tenant_id":             "11111111-1111-1111-1111-111111111111",
			"site_id":               "22222222-2222-2222-2222-222222222222",
			"sensor_id":             "sensor-infra-01",
			"assignment_generation": MaxSafeSequence + 1,
			"resolved_at":           time.Now().UTC().Format(time.RFC3339Nano),
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.ResolveAgentAuthority(context.Background(), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	var authErr *AgentAuthorityError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected AgentAuthorityError, got %v", err)
	}
	if authErr.Transient {
		t.Fatal("out-of-range generation must be definitive")
	}
}

func TestResolveAgentAuthorityRejectsGenerationZero(t *testing.T) {
	pki := makeTestPKI(t)
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                "ACCEPTED",
			"agent_id":              "agent-01",
			"tenant_id":             "11111111-1111-1111-1111-111111111111",
			"site_id":               "22222222-2222-2222-2222-222222222222",
			"sensor_id":             "sensor-infra-01",
			"assignment_generation": 0,
			"resolved_at":           time.Now().UTC().Format(time.RFC3339Nano),
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.ResolveAgentAuthority(context.Background(), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA")
	if err == nil {
		t.Fatal("expected rejection for generation 0")
	}
}
