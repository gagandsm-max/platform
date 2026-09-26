package coreclient

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func validAgentBatch() AgentTelemetryBatch {
	return AgentTelemetryBatch{
		SchemaVersion:            2,
		SensorAgentSpoolStreamID: "stream-00112233445566778899aabbccddeeff",
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

func multiRecordAgentBatch() AgentTelemetryBatch {
	return AgentTelemetryBatch{
		SchemaVersion:            2,
		SensorAgentSpoolStreamID: "stream-00112233445566778899aabbccddeeff",
		SensorID:             "sensor-infra-01",
		BatchID:              "batch-agent-multi",
		FirstIngressSequence: 10,
		LastIngressSequence:  15,
		RecordCount:          6,
		Checksum:             "checksum-multi",
		CorrelationID:        "corr-agent-multi",
		Records: []AgentIngressRecord{
			{
				SensorIngressSequence: 10,
				SensorIngressRecordID: "r-10",
				TenantID:              "tenant-1",
				AgentID:               "agent-1",
				AcceptingSensorID:     "sensor-infra-01",
				AssignmentGeneration:  1,
				AgentFirstSequence:    10,
				AgentLastSequence:     10,
				AgentBatchID:          "ab-10",
				CorrelationID:         "corr-10",
				AcceptedAt:            "2026-09-11T10:00:00Z",
				Records:               json.RawMessage(`[]`),
			},
			{
				SensorIngressSequence: 15,
				SensorIngressRecordID: "r-15",
				TenantID:              "tenant-1",
				AgentID:               "agent-1",
				AcceptingSensorID:     "sensor-infra-01",
				AssignmentGeneration:  1,
				AgentFirstSequence:    15,
				AgentLastSequence:     15,
				AgentBatchID:          "ab-15",
				CorrelationID:         "corr-15",
				AcceptedAt:            "2026-09-11T10:00:00Z",
				Records:               json.RawMessage(`[]`),
			},
		},
	}
}

func TestUploadAgentTelemetryExactLastSequenceValid(t *testing.T) {
	pki := makeTestPKI(t)
	batch := multiRecordAgentBatch()
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                         "ACCEPTED",
			"sensorId":                       batch.SensorID,
			"batchId":                        batch.BatchID,
			"acceptedThroughIngressSequence": uint64(15),
			"correlationId":                  batch.CorrelationID,
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	ack, err := client.UploadAgentTelemetry(context.Background(), batch)
	if err != nil {
		t.Fatalf("expected successful ACK, got %v", err)
	}
	if ack.AcceptedThroughIngressSequence != 15 {
		t.Fatalf("expected acceptedThrough=15, got %d", ack.AcceptedThroughIngressSequence)
	}
}

func TestUploadAgentTelemetryRejectsAcceptedThroughBelowLastInsideRange(t *testing.T) {
	pki := makeTestPKI(t)
	batch := multiRecordAgentBatch()
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                         "ACCEPTED",
			"sensorId":                       batch.SensorID,
			"batchId":                        batch.BatchID,
			"acceptedThroughIngressSequence": uint64(12),
			"correlationId":                  batch.CorrelationID,
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.UploadAgentTelemetry(context.Background(), batch)
	if !IsCode(err, CodeProtocol) {
		t.Fatalf("expected CodeProtocol for sequence below last inside range, got %v", err)
	}
}

func TestUploadAgentTelemetryRejectsAcceptedThroughBelowFirst(t *testing.T) {
	pki := makeTestPKI(t)
	batch := multiRecordAgentBatch()
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                         "ACCEPTED",
			"sensorId":                       batch.SensorID,
			"batchId":                        batch.BatchID,
			"acceptedThroughIngressSequence": uint64(9),
			"correlationId":                  batch.CorrelationID,
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.UploadAgentTelemetry(context.Background(), batch)
	if !IsCode(err, CodeProtocol) {
		t.Fatalf("expected CodeProtocol for sequence below first, got %v", err)
	}
}

func TestUploadAgentTelemetryRejectsAcceptedThroughAboveLast(t *testing.T) {
	pki := makeTestPKI(t)
	batch := multiRecordAgentBatch()
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                         "ACCEPTED",
			"sensorId":                       batch.SensorID,
			"batchId":                        batch.BatchID,
			"acceptedThroughIngressSequence": uint64(16),
			"correlationId":                  batch.CorrelationID,
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.UploadAgentTelemetry(context.Background(), batch)
	if !IsCode(err, CodeProtocol) {
		t.Fatalf("expected CodeProtocol for sequence above last, got %v", err)
	}
}

func TestUploadAgentTelemetryRejectsWrongBatchID(t *testing.T) {
	pki := makeTestPKI(t)
	batch := multiRecordAgentBatch()
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                         "ACCEPTED",
			"sensorId":                       batch.SensorID,
			"batchId":                        "wrong-batch-id",
			"acceptedThroughIngressSequence": uint64(15),
			"correlationId":                  batch.CorrelationID,
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.UploadAgentTelemetry(context.Background(), batch)
	if !IsCode(err, CodeProtocol) {
		t.Fatalf("expected CodeProtocol for wrong batchId, got %v", err)
	}
}

func TestUploadAgentTelemetryRejectsWrongSensorID(t *testing.T) {
	pki := makeTestPKI(t)
	batch := multiRecordAgentBatch()
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                         "ACCEPTED",
			"sensorId":                       "sensor-wrong-id",
			"batchId":                        batch.BatchID,
			"acceptedThroughIngressSequence": uint64(15),
			"correlationId":                  batch.CorrelationID,
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.UploadAgentTelemetry(context.Background(), batch)
	if !IsCode(err, CodeProtocol) {
		t.Fatalf("expected CodeProtocol for wrong sensorId, got %v", err)
	}
}

func TestUploadAgentTelemetryRejectsWrongCorrelationID(t *testing.T) {
	pki := makeTestPKI(t)
	batch := multiRecordAgentBatch()
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":                         "ACCEPTED",
			"sensorId":                       batch.SensorID,
			"batchId":                        batch.BatchID,
			"acceptedThroughIngressSequence": uint64(15),
			"correlationId":                  "wrong-correlation-id",
		})
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.UploadAgentTelemetry(context.Background(), batch)
	if !IsCode(err, CodeProtocol) {
		t.Fatalf("expected CodeProtocol for wrong correlationId, got %v", err)
	}
}

func TestUploadAgentTelemetryRejectsMalformedAck(t *testing.T) {
	pki := makeTestPKI(t)
	batch := multiRecordAgentBatch()
	server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	client := newTestClient(t, server.URL, pki, 4096)
	_, err := client.UploadAgentTelemetry(context.Background(), batch)
	if !IsCode(err, CodeAckContractMissing) {
		t.Fatalf("expected CodeAckContractMissing for malformed json, got %v", err)
	}
}

func TestUploadAgentTelemetryRejectsNonExactAcceptedStatus(t *testing.T) {
	for _, status := range []string{"accepted", "Accepted", " ACCEPTED ", "OK"} {
		t.Run(status, func(t *testing.T) {
			pki := makeTestPKI(t)
			batch := multiRecordAgentBatch()
			server := startMTLSServer(t, pki, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"status":                         status,
					"sensorId":                       batch.SensorID,
					"batchId":                        batch.BatchID,
					"acceptedThroughIngressSequence": batch.LastIngressSequence,
					"correlationId":                  batch.CorrelationID,
				})
			}))
			client := newTestClient(t, server.URL, pki, 4096)
			_, err := client.UploadAgentTelemetry(context.Background(), batch)
			if !IsCode(err, CodeProtocol) {
				t.Fatalf("status %q must be rejected exactly, got %v", status, err)
			}
		})
	}
}

func TestClassifyAgentTelemetryFailureRetryableAndSpoolRetaining(t *testing.T) {
	pending := ClassifyAgentTelemetryFailure(409, []byte(`{"code":"ERR_PREVIOUS_BATCH_PENDING","message":"x"}`))
	if !IsTransient(pending) {
		t.Fatalf("ERR_PREVIOUS_BATCH_PENDING must be transient, got %v", pending)
	}
	busy := ClassifyAgentTelemetryFailure(429, []byte(`{"code":"ERR_BATCH_PUBLICATION_IN_PROGRESS","message":"x"}`))
	if !IsTransient(busy) {
		t.Fatalf("ERR_BATCH_PUBLICATION_IN_PROGRESS must be transient, got %v", busy)
	}
	for _, code := range []string{
		"ERR_SENSOR_STREAM_CHANGED",
		"ERR_IDEMPOTENCY_CONFLICT",
		"ERR_SEQUENCE_GAP",
		"ERR_SEQUENCE_REGRESSION",
		"ERR_SEQUENCE_OUT_OF_BOUNDS",
		"ERR_AGENT_STREAM_ID_INVALID",
	} {
		err := ClassifyAgentTelemetryFailure(409, []byte(`{"code":"`+code+`","message":"x"}`))
		if IsTransient(err) {
			t.Fatalf("%s must fail closed (non-transient)", code)
		}
		if !IsAgentIngressSpoolRetainingCode(code) {
			t.Fatalf("%s must retain spool", code)
		}
	}
}
