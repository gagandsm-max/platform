package coreclient

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestUploadAckNegativeMatrix(t *testing.T) {
	cases := []struct {
		name string
		body map[string]any
		code ErrorCode
	}{
		{"wrong-status", map[string]any{"status": "OK", "accepted_through_sequence": 2, "sensor_identity": "sensor-test"}, CodeProtocol},
		{"wrong-sensor", map[string]any{"status": "ACCEPTED", "accepted_through_sequence": 2, "sensor_identity": "other"}, CodeProtocol},
		{"missing-watermark", map[string]any{"status": "ACCEPTED", "sensor_identity": "sensor-test"}, CodeAckContractMissing},
		{"below-first", map[string]any{"status": "ACCEPTED", "accepted_through_sequence": 0, "sensor_identity": "sensor-test"}, CodeProtocol},
		{"inside-not-terminal", map[string]any{"status": "ACCEPTED", "accepted_through_sequence": 1, "sensor_identity": "sensor-test"}, CodeProtocol},
		{"above-last", map[string]any{"status": "ACCEPTED", "accepted_through_sequence": 99, "sensor_identity": "sensor-test"}, CodeProtocol},
		{"unknown-field", map[string]any{"status": "ACCEPTED", "accepted_through_sequence": 2, "sensor_identity": "sensor-test", "extra": true}, CodeProtocol},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pki := makeTestPKI(t)
			server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			client := newTestClient(t, server.URL, pki, 4096)
			_, err := client.Upload(context.Background(), validBatch())
			if !IsCode(err, tc.code) {
				t.Fatalf("got %v want %s", err, tc.code)
			}
		})
	}

	t.Run("trailing-json", func(t *testing.T) {
		pki := makeTestPKI(t)
		server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"status":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":"sensor-test"}{}`))
		}))
		client := newTestClient(t, server.URL, pki, 4096)
		_, err := client.Upload(context.Background(), validBatch())
		if !IsCode(err, CodeProtocol) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("malformed-json", func(t *testing.T) {
		pki := makeTestPKI(t)
		server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{not-json`))
		}))
		client := newTestClient(t, server.URL, pki, 4096)
		_, err := client.Upload(context.Background(), validBatch())
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("oversized-response", func(t *testing.T) {
		pki := makeTestPKI(t)
		server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(strings.Repeat("a", 8192)))
		}))
		client := newTestClient(t, server.URL, pki, 64)
		_, err := client.Upload(context.Background(), validBatch())
		if !IsCode(err, CodeResponseTooLarge) {
			t.Fatalf("got %v", err)
		}
	})
}
