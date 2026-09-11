package coreclient

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func validAgentBatch() AgentTelemetryBatch {
	return AgentTelemetryBatch{
		SchemaVersion:        1,
		SensorID:             "sensor-infra-01",
		BatchID:              "batch-agent-1",
		FirstIngressSequence: 1,
		LastIngressSequence:  1,
		RecordCount:          1,
		Checksum:             "abc123",
		CorrelationID:        "corr-agent-1",
		Records: []AgentIngressRecord{{
			SensorIngressSequence: 1,
			SensorIngressRecordID: "r-1",
			TenantID:              "tenant-1",
			AgentID:               "agent-1",
			AcceptingSensorID:     "sensor-infra-01",
			AssignmentGeneration:  1,
			AgentFirstSequence:    10,
			AgentLastSequence:     10,
			AgentBatchID:          "ab-1",
			CorrelationID:         "corr-agent-1",
			AcceptedAt:            "2026-09-11T10:00:00Z",
			Records:               json.RawMessage(`[]`),
		}},
	}
}

func TestUploadAgentTelemetryRequiresBoundAck(t *testing.T) {
	pki := makeTestPKI(t)
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != agentTelemetryPath {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                         "ACCEPTED",
			"sensorId":                       "sensor-infra-01",
			"batchId":                        "batch-agent-1",
			"acceptedThroughIngressSequence": uint64(1),
			"correlationId":                  "corr-agent-1",
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	ack, err := client.UploadAgentTelemetry(context.Background(), validAgentBatch())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ack.AcceptedThroughIngressSequence != 1 || ack.BatchID != "batch-agent-1" {
		t.Fatalf("ack=%+v", ack)
	}
}

func TestUploadAgentTelemetryRejectsHTTP200WithoutContract(t *testing.T) {
	pki := makeTestPKI(t)
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ACCEPTED"}`))
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.UploadAgentTelemetry(context.Background(), validAgentBatch())
	if !IsCode(err, CodeAckContractMissing) {
		t.Fatalf("expected ACK contract missing, got %v", err)
	}
}

func TestUploadAgentTelemetryRejectsUnknownAckFields(t *testing.T) {
	pki := makeTestPKI(t)
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                         "ACCEPTED",
			"sensorId":                       "sensor-infra-01",
			"batchId":                        "batch-agent-1",
			"acceptedThroughIngressSequence": uint64(1),
			"correlationId":                  "corr-agent-1",
			"extraField":                     true,
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.UploadAgentTelemetry(context.Background(), validAgentBatch())
	if !IsCode(err, CodeAckContractMissing) {
		t.Fatalf("expected fail-closed unknown fields, got %v", err)
	}
}

func TestUploadAgentTelemetryRejectsMismatchedBatchBinding(t *testing.T) {
	pki := makeTestPKI(t)
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                         "ACCEPTED",
			"sensorId":                       "sensor-other",
			"batchId":                        "batch-agent-1",
			"acceptedThroughIngressSequence": uint64(1),
			"correlationId":                  "corr-agent-1",
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.UploadAgentTelemetry(context.Background(), validAgentBatch())
	if !IsCode(err, CodeProtocol) {
		t.Fatalf("expected protocol bind failure, got %v", err)
	}
}

func TestUploadAgentTelemetryRejectsAckOutsideRange(t *testing.T) {
	pki := makeTestPKI(t)
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                         "ACCEPTED",
			"sensorId":                       "sensor-infra-01",
			"batchId":                        "batch-agent-1",
			"acceptedThroughIngressSequence": uint64(99),
			"correlationId":                  "corr-agent-1",
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.UploadAgentTelemetry(context.Background(), validAgentBatch())
	if !IsCode(err, CodeProtocol) {
		t.Fatalf("expected out-of-range failure, got %v", err)
	}
}

func TestUploadAgentTelemetrySurfacesTerminalCoreCode(t *testing.T) {
	pki := makeTestPKI(t)
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ERROR",
			"code":    "ERR_AGENT_ENTITY_UNRESOLVED",
			"message": "entity missing",
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.UploadAgentTelemetry(context.Background(), validAgentBatch())
	if !IsCoreAPICode(err, "ERR_AGENT_ENTITY_UNRESOLVED") {
		t.Fatalf("expected entity unresolved, got %v", err)
	}
	if IsTransient(err) {
		t.Fatalf("entity unresolved must not be transient")
	}
}
