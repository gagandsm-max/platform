package com.platform.core.ingestion.agentingress;

import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.IngestionMetrics;
import com.platform.core.ingestion.KafkaTelemetryProducer;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryAckResponse;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryBatchRequest;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.replay.WatermarkState;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.http.HttpStatus;

import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.List;
import java.util.Set;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

class AgentIngestionServiceTest {

    private AgentIngestionValidator validator;
    private AgentIngressWatermarkStore watermarkStore;
    private KafkaTelemetryProducer kafkaProducer;
    private IngestionMetrics metrics;
    private AgentIngestionService service;

    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private final UUID entityId = UUID.fromString("018f0e0a-0000-7000-8000-000000000011");
    private final SensorScope scope = new SensorScope("sensor-infra-01", tenantId, null, Set.of(entityId), true);

    @BeforeEach
    void setUp() {
        validator = mock(AgentIngestionValidator.class);
        watermarkStore = mock(AgentIngressWatermarkStore.class);
        kafkaProducer = mock(KafkaTelemetryProducer.class);
        metrics = new IngestionMetrics();
        service = new AgentIngestionService(validator, watermarkStore, kafkaProducer, metrics);
    }

    private X509Certificate cert() {
        return mock(X509Certificate.class);
    }

    private AgentTelemetryBatchRequest req(long first, long last, String checksum) {
        return new AgentTelemetryBatchRequest(
                1, "sensor-infra-01", "batch-1", first, last, (int) (last - first + 1),
                checksum, "corr-1", List.of()
        );
    }

    private List<CanonicalObservation> obs() {
        return List.of(new CanonicalObservation(
                tenantId.toString(), entityId.toString(), "cpu.util", "sensor-infra-01",
                Instant.now(), 1.0, 1, 0, "seq-1", "AGENT_HOST"
        ));
    }

    @Test
    void exactReplayReturnsAckWithoutRepublish() {
        X509Certificate c = cert();
        AgentTelemetryBatchRequest request = req(10, 12, "chk-abc");
        when(validator.resolveAuthority(c)).thenReturn(scope);
        when(validator.validateAndExtract(any(), eq(request), eq(scope))).thenReturn(obs());
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-infra-01"))
                .thenReturn(new WatermarkState(
                        "sensor-infra-01", tenantId, 12, 10, 12, "chk-abc", "batch-1", Instant.now()
                ));

        AgentTelemetryAckResponse ack = service.ingestAgentTelemetry("{}".getBytes(), request, c);
        assertEquals("ACCEPTED", ack.status());
        assertEquals(12L, ack.acceptedThroughIngressSequence());
        assertEquals("batch-1", ack.batchId());
        assertEquals("corr-1", ack.correlationId());
        verify(kafkaProducer, never()).publishBatchTransactionally(any());
        verify(watermarkStore, never()).updateWatermark(any(), any(), anyLong(), anyLong(), anyLong(), any(), any());
    }

    @Test
    void checksumConflictOnSameRange() {
        X509Certificate c = cert();
        AgentTelemetryBatchRequest request = req(10, 12, "chk-new");
        when(validator.resolveAuthority(c)).thenReturn(scope);
        when(validator.validateAndExtract(any(), eq(request), eq(scope))).thenReturn(obs());
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-infra-01"))
                .thenReturn(new WatermarkState(
                        "sensor-infra-01", tenantId, 12, 10, 12, "chk-old", "batch-1", Instant.now()
                ));

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestAgentTelemetry("{}".getBytes(), request, c));
        assertEquals("ERR_IDEMPOTENCY_CONFLICT", ex.getCode());
        assertEquals(HttpStatus.CONFLICT, ex.getStatus());
    }

    @Test
    void sequenceRegressionRejected() {
        X509Certificate c = cert();
        AgentTelemetryBatchRequest request = req(5, 8, "chk");
        when(validator.resolveAuthority(c)).thenReturn(scope);
        when(validator.validateAndExtract(any(), eq(request), eq(scope))).thenReturn(obs());
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-infra-01"))
                .thenReturn(new WatermarkState(
                        "sensor-infra-01", tenantId, 12, 10, 12, "chk-old", "batch-1", Instant.now()
                ));

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestAgentTelemetry("{}".getBytes(), request, c));
        assertEquals("ERR_SEQUENCE_REGRESSION", ex.getCode());
    }

    @Test
    void sequenceGapRejected() {
        X509Certificate c = cert();
        AgentTelemetryBatchRequest request = req(20, 22, "chk");
        when(validator.resolveAuthority(c)).thenReturn(scope);
        when(validator.validateAndExtract(any(), eq(request), eq(scope))).thenReturn(obs());
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-infra-01"))
                .thenReturn(new WatermarkState(
                        "sensor-infra-01", tenantId, 12, 10, 12, "chk-old", "batch-1", Instant.now()
                ));

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestAgentTelemetry("{}".getBytes(), request, c));
        assertEquals("ERR_SEQUENCE_GAP", ex.getCode());
    }

    @Test
    void happyPathPublishesAndUpdatesWatermark() {
        X509Certificate c = cert();
        AgentTelemetryBatchRequest request = req(13, 15, "chk-new");
        when(validator.resolveAuthority(c)).thenReturn(scope);
        when(validator.validateAndExtract(any(), eq(request), eq(scope))).thenReturn(obs());
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-infra-01"))
                .thenReturn(new WatermarkState(
                        "sensor-infra-01", tenantId, 12, 10, 12, "chk-old", "batch-1", Instant.now()
                ));

        AgentTelemetryAckResponse ack = service.ingestAgentTelemetry("{}".getBytes(), request, c);
        assertEquals(15L, ack.acceptedThroughIngressSequence());
        assertEquals("sensor-infra-01", ack.sensorId());
        verify(kafkaProducer).publishBatchTransactionally(any());
        verify(watermarkStore).updateWatermark(tenantId, "sensor-infra-01", 15L, 13L, 15L, "chk-new", "batch-1");
    }

    @Test
    void kafkaCommitThenWatermarkFailureAllowsRetryRepublish() {
        X509Certificate c = cert();
        AgentTelemetryBatchRequest request = req(13, 15, "chk-new");
        when(validator.resolveAuthority(c)).thenReturn(scope);
        when(validator.validateAndExtract(any(), eq(request), eq(scope))).thenReturn(obs());
        when(watermarkStore.getWatermarkForUpdate(tenantId, "sensor-infra-01")).thenReturn(null);

        doThrow(new RuntimeException("PostgreSQL connection terminated unexpectedly"))
                .doNothing()
                .when(watermarkStore).updateWatermark(
                        eq(tenantId), eq("sensor-infra-01"), eq(15L), eq(13L), eq(15L), eq("chk-new"), eq("batch-1"));

        RuntimeException first = assertThrows(RuntimeException.class, () ->
                service.ingestAgentTelemetry("{}".getBytes(), request, c));
        assertTrue(first.getMessage().contains("PostgreSQL connection terminated"));

        AgentTelemetryAckResponse ack = service.ingestAgentTelemetry("{}".getBytes(), request, c);
        assertEquals("ACCEPTED", ack.status());
        assertEquals(15L, ack.acceptedThroughIngressSequence());
        verify(kafkaProducer, times(2)).publishBatchTransactionally(any());
    }
}
