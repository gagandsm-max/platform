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
		{"status-lowercase", map[string]any{"status": "accepted", "accepted_through_sequence": 2, "sensor_identity": "sensor-test"}, CodeProtocol},
		{"status-mixed-case", map[string]any{"status": "Accepted", "accepted_through_sequence": 2, "sensor_identity": "sensor-test"}, CodeProtocol},
		{"status-padded", map[string]any{"status": " ACCEPTED ", "accepted_through_sequence": 2, "sensor_identity": "sensor-test"}, CodeProtocol},
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

func TestUploadAckRawJSONMemberStrictness(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantError bool
		wantCode  ErrorCode
	}{
		{name: "canonical-positive", body: `{"status":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":"sensor-test"}`},
		{name: "duplicate-status-same-value", body: `{"status":"ACCEPTED","status":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "duplicate-status-different-value", body: `{"status":"REJECTED","status":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "duplicate-status-reverse-order", body: `{"status":"ACCEPTED","status":"REJECTED","accepted_through_sequence":2,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "duplicate-watermark-partial-then-terminal", body: `{"status":"ACCEPTED","accepted_through_sequence":1,"accepted_through_sequence":2,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "duplicate-watermark-terminal-then-partial", body: `{"status":"ACCEPTED","accepted_through_sequence":2,"accepted_through_sequence":1,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "duplicate-watermark-same-value", body: `{"status":"ACCEPTED","accepted_through_sequence":2,"accepted_through_sequence":2,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "duplicate-identity-different-value", body: `{"status":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":"sensor-test","sensor_identity":"other"}`, wantError: true, wantCode: CodeProtocol},
		{name: "duplicate-identity-reverse-order", body: `{"status":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":"other","sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "duplicate-identity-same-value", body: `{"status":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":"sensor-test","sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "status-case-alias", body: `{"STATUS":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "watermark-case-alias", body: `{"status":"ACCEPTED","ACCEPTED_THROUGH_SEQUENCE":2,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "identity-case-alias", body: `{"status":"ACCEPTED","accepted_through_sequence":2,"SENSOR_IDENTITY":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "escaped-duplicate-status", body: `{"status":"ACCEPTED","st\u0061tus":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "escaped-duplicate-watermark", body: `{"status":"ACCEPTED","accepted_through_sequence":2,"accepted_through_\u0073equence":2,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "escaped-duplicate-identity", body: `{"status":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":"sensor-test","sensor_\u0069dentity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "null-status", body: `{"status":null,"accepted_through_sequence":2,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "negative-watermark", body: `{"status":"ACCEPTED","accepted_through_sequence":-1,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "string-watermark", body: `{"status":"ACCEPTED","accepted_through_sequence":"2","sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "fractional-watermark", body: `{"status":"ACCEPTED","accepted_through_sequence":2.5,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "overflow-watermark", body: `{"status":"ACCEPTED","accepted_through_sequence":18446744073709551616,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeProtocol},
		{name: "null-watermark", body: `{"status":"ACCEPTED","accepted_through_sequence":null,"sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeAckContractMissing},
		{name: "missing-watermark", body: `{"status":"ACCEPTED","sensor_identity":"sensor-test"}`, wantError: true, wantCode: CodeAckContractMissing},
		{name: "null-identity", body: `{"status":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":null}`, wantError: true, wantCode: CodeAckContractMissing},
		{name: "missing-identity", body: `{"status":"ACCEPTED","accepted_through_sequence":2}`, wantError: true, wantCode: CodeAckContractMissing},
		{name: "nonobject-array", body: `[]`, wantError: true, wantCode: CodeProtocol},
		{name: "nonobject-null", body: `null`, wantError: true, wantCode: CodeProtocol},
		{name: "malformed-object", body: `{"status":`, wantError: true, wantCode: CodeProtocol},
		{name: "trailing-object", body: `{"status":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":"sensor-test"}{}`, wantError: true, wantCode: CodeProtocol},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pki := makeTestPKI(t)
			server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			client := newTestClient(t, server.URL, pki, 4096)
			ack, err := client.Upload(context.Background(), validBatch())
			if tc.wantError {
				if !IsCode(err, tc.wantCode) {
					t.Fatalf("got ack=%+v err=%v, want %s", ack, err, tc.wantCode)
				}
				return
			}
			if err != nil || ack.Status != "ACCEPTED" || ack.AcceptedThroughSequence == nil || *ack.AcceptedThroughSequence != 2 || ack.SensorIdentity != "sensor-test" {
				t.Fatalf("ack=%+v err=%v", ack, err)
			}
		})
	}
}

func TestUploadAckStrictUnmarshalFailureIsAtomic(t *testing.T) {
	initialWatermark := uint64(7)
	original := UploadAck{Status: "UNCHANGED", SensorIdentity: "sentinel", AcceptedThroughSequence: &initialWatermark}
	cases := []struct {
		name string
		body string
	}{
		{name: "unknown-field-after-valid-fields", body: `{"status":"ACCEPTED","accepted_through_sequence":2,"sensor_identity":"sensor-test","extra":true}`},
		{name: "invalid-watermark-after-valid-status", body: `{"status":"ACCEPTED","accepted_through_sequence":-1,"sensor_identity":"sensor-test"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ack := UploadAck{Status: original.Status, SensorIdentity: original.SensorIdentity, AcceptedThroughSequence: original.AcceptedThroughSequence}
			if err := json.Unmarshal([]byte(tc.body), &ack); err == nil {
				t.Fatal("expected strict decode rejection")
			}
			if ack.Status != original.Status || ack.SensorIdentity != original.SensorIdentity || ack.AcceptedThroughSequence != original.AcceptedThroughSequence || ack.AcceptedThroughSequence == nil || *ack.AcceptedThroughSequence != 7 {
				t.Fatalf("receiver changed after rejected decode: got=%+v original=%+v", ack, original)
			}
		})
	}
}
