package com.platform.core.ingestion.agentingress;

import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.IngestionMetrics;
import com.platform.core.ingestion.KafkaTelemetryProducer;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryAckResponse;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryBatchRequest;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryObservationBatch;
import com.platform.core.ingestion.replay.PublisherClaimResult;
import com.platform.core.ingestion.replay.ReplayReservation;
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

    private static final String STREAM = "stream-00112233445566778899aabbccddeeff";

    private AgentIngestionValidator validator;
    private AgentIngressWatermarkStore watermarkStore;
    private KafkaTelemetryProducer kafkaProducer;
    private IngestionMetrics metrics;
    private AgentIngestionService service;

    private final UUID tenantId = UUID.fromString("018f0e0a-0000-7000-8000-000000000001");
    private final UUID entityId = UUID.fromString("018f0e0a-0000-7000-8000-000000000011");
    private final SensorScope scope = new SensorScope("sensor-infra-01", tenantId, null, Set.of(entityId), true);
    private final UUID ownerToken = UUID.fromString("018f0e0a-0000-7000-8000-00000000beef");
    private static final long PUBLISHER_EPOCH = 1L;
    private static final String BODY_SHA = "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a";

    @BeforeEach
    void setUp() {
        validator = mock(AgentIngestionValidator.class);
        watermarkStore = mock(AgentIngressWatermarkStore.class);
        kafkaProducer = mock(KafkaTelemetryProducer.class);
        metrics = new IngestionMetrics();
        service = new AgentIngestionService(validator, watermarkStore, kafkaProducer, metrics);
        when(watermarkStore.reserveBatchWithBody(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), anyString(), anyString()))
                .thenReturn(ReplayReservation.reserved(0));
        when(watermarkStore.claimPublisherLease(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), anyString()))
                .thenReturn(PublisherClaimResult.claimed(ownerToken, PUBLISHER_EPOCH));
    }

    private X509Certificate cert() {
        return mock(X509Certificate.class);
    }

    private AgentTelemetryBatchRequest req(long first, long last, String checksum) {
        return new AgentTelemetryBatchRequest(
                2, "sensor-infra-01", STREAM, "batch-1", first, last, (int) (last - first + 1),
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
        when(watermarkStore.reserveBatchWithBody(
                tenantId, "sensor-infra-01", STREAM, 10L, 12L, "chk-abc", "batch-1", "corr-1", BODY_SHA))
                .thenReturn(ReplayReservation.alreadyAcked(12L));

        AgentTelemetryAckResponse ack = service.ingestAgentTelemetry("{}".getBytes(), request, "batch-1", c);
        assertEquals("ACCEPTED", ack.status());
        assertEquals(12L, ack.acceptedThroughIngressSequence());
        assertEquals("batch-1", ack.batchId());
        assertEquals("corr-1", ack.correlationId());
        verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        verify(watermarkStore, never()).claimPublisherLease(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), anyString());
        verify(watermarkStore, never()).finalizeReservedBatch(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), anyString(),
                any(), anyLong());
    }

    @Test
    void idempotencyKeyMismatchRejectsBeforeReserve() {
        X509Certificate c = cert();
        AgentTelemetryBatchRequest request = req(10, 12, "chk-abc");
        when(validator.resolveAuthority(c)).thenReturn(scope);

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestAgentTelemetry("{}".getBytes(), request, "wrong-key", c));
        assertEquals("ERR_IDEMPOTENCY_KEY_MISMATCH", ex.getCode());
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
        verify(watermarkStore, never()).reserveBatchWithBody(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), anyString(), anyString());
    }

    @Test
    void checksumConflictOnSameRange() {
        X509Certificate c = cert();
        AgentTelemetryBatchRequest request = req(10, 12, "chk-new");
        when(validator.resolveAuthority(c)).thenReturn(scope);
        when(validator.validateAndExtract(any(), eq(request), eq(scope))).thenReturn(obs());
        when(watermarkStore.reserveBatchWithBody(
                tenantId, "sensor-infra-01", STREAM, 10L, 12L, "chk-new", "batch-1", "corr-1", BODY_SHA))
                .thenReturn(ReplayReservation.rejected(
                        12L, "ERR_IDEMPOTENCY_CONFLICT",
                        "sequence range was accepted/reserved with a different checksum"));

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestAgentTelemetry("{}".getBytes(), request, "batch-1", c));
        assertEquals("ERR_IDEMPOTENCY_CONFLICT", ex.getCode());
        assertEquals(HttpStatus.CONFLICT, ex.getStatus());
        verify(kafkaProducer, never()).publishBatchTransactionally(any(TelemetryObservationBatch.class));
    }

    @Test
    void sequenceRegressionRejected() {
        X509Certificate c = cert();
        AgentTelemetryBatchRequest request = req(5, 8, "chk");
        when(validator.resolveAuthority(c)).thenReturn(scope);
        when(validator.validateAndExtract(any(), eq(request), eq(scope))).thenReturn(obs());
        when(watermarkStore.reserveBatchWithBody(
                tenantId, "sensor-infra-01", STREAM, 5L, 8L, "chk", "batch-1", "corr-1", BODY_SHA))
                .thenReturn(ReplayReservation.rejected(
                        12L, "ERR_SEQUENCE_REGRESSION", "sequence already accepted"));

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestAgentTelemetry("{}".getBytes(), request, "batch-1", c));
        assertEquals("ERR_SEQUENCE_REGRESSION", ex.getCode());
    }

    @Test
    void sequenceGapRejected() {
        X509Certificate c = cert();
        AgentTelemetryBatchRequest request = req(20, 22, "chk");
        when(validator.resolveAuthority(c)).thenReturn(scope);
        when(validator.validateAndExtract(any(), eq(request), eq(scope))).thenReturn(obs());
        when(watermarkStore.reserveBatchWithBody(
                tenantId, "sensor-infra-01", STREAM, 20L, 22L, "chk", "batch-1", "corr-1", BODY_SHA))
                .thenReturn(ReplayReservation.rejected(
                        12L, "ERR_SEQUENCE_GAP", "unexpected sequence gap"));

        IngestionException ex = assertThrows(IngestionException.class, () ->
                service.ingestAgentTelemetry("{}".getBytes(), request, "batch-1", c));
        assertEquals("ERR_SEQUENCE_GAP", ex.getCode());
        assertEquals(HttpStatus.BAD_REQUEST, ex.getStatus());
    }

    @Test
    void happyPathPublishesAndFinalizesWatermark() {
        X509Certificate c = cert();
        AgentTelemetryBatchRequest request = req(13, 15, "chk-new");
        when(validator.resolveAuthority(c)).thenReturn(scope);
        when(validator.validateAndExtract(any(), eq(request), eq(scope))).thenReturn(obs());
        when(watermarkStore.reserveBatchWithBody(
                tenantId, "sensor-infra-01", STREAM, 13L, 15L, "chk-new", "batch-1", "corr-1", BODY_SHA))
                .thenReturn(ReplayReservation.reserved(12L));

        AgentTelemetryAckResponse ack = service.ingestAgentTelemetry("{}".getBytes(), request, "batch-1", c);
        assertEquals(15L, ack.acceptedThroughIngressSequence());
        assertEquals("sensor-infra-01", ack.sensorId());
        verify(kafkaProducer).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        verify(watermarkStore).claimPublisherLease(
                tenantId, "sensor-infra-01", STREAM, 13L, 15L, "chk-new", "batch-1", "corr-1");
        verify(watermarkStore).finalizeReservedBatch(
                tenantId, "sensor-infra-01", STREAM, 13L, 15L, "chk-new", "batch-1", "corr-1",
                ownerToken, PUBLISHER_EPOCH);
        verify(watermarkStore, never()).releasePublisherLease(
                any(), anyString(), anyString(), anyLong(), anyLong(), anyString(), anyString(), anyString(),
                any(), anyLong());
    }

    @Test
    void kafkaCommitThenWatermarkFailureAllowsRetryRepublish() {
        X509Certificate c = cert();
        AgentTelemetryBatchRequest request = req(13, 15, "chk-new");
        when(validator.resolveAuthority(c)).thenReturn(scope);
        when(validator.validateAndExtract(any(), eq(request), eq(scope))).thenReturn(obs());
        when(watermarkStore.reserveBatchWithBody(
                tenantId, "sensor-infra-01", STREAM, 13L, 15L, "chk-new", "batch-1", "corr-1", BODY_SHA))
                .thenReturn(ReplayReservation.reserved(12L))
                .thenReturn(ReplayReservation.retryPending(12L));

        doThrow(new RuntimeException("PostgreSQL connection terminated unexpectedly"))
                .doNothing()
                .when(watermarkStore).finalizeReservedBatch(
                        eq(tenantId), eq("sensor-infra-01"), eq(STREAM), eq(13L), eq(15L),
                        eq("chk-new"), eq("batch-1"), eq("corr-1"), eq(ownerToken), eq(PUBLISHER_EPOCH));

        RuntimeException first = assertThrows(RuntimeException.class, () ->
                service.ingestAgentTelemetry("{}".getBytes(), request, "batch-1", c));
        assertTrue(first.getMessage().contains("PostgreSQL connection terminated"));

        AgentTelemetryAckResponse ack = service.ingestAgentTelemetry("{}".getBytes(), request, "batch-1", c);
        assertEquals("ACCEPTED", ack.status());
        assertEquals(15L, ack.acceptedThroughIngressSequence());
        verify(kafkaProducer, times(2)).publishBatchTransactionally(any(TelemetryObservationBatch.class));
        verify(watermarkStore, times(2)).claimPublisherLease(
                tenantId, "sensor-infra-01", STREAM, 13L, 15L, "chk-new", "batch-1", "corr-1");
        verify(watermarkStore, times(1)).releasePublisherLease(
                eq(tenantId), eq("sensor-infra-01"), eq(STREAM), eq(13L), eq(15L),
                eq("chk-new"), eq("batch-1"), eq("corr-1"), eq(ownerToken), eq(PUBLISHER_EPOCH));
    }
}
