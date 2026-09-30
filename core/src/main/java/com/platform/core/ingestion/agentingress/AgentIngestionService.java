package com.platform.core.ingestion.agentingress;

import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.IngestionMetrics;
import com.platform.core.ingestion.KafkaTelemetryProducer;
import com.platform.core.ingestion.SensorIngestionService;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryAckResponse;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryBatchRequest;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryObservationBatch;
import com.platform.core.ingestion.replay.PublisherClaimResult;
import com.platform.core.ingestion.replay.ReplayReservation;
import com.platform.core.logging.CoreSupportabilityEventCodes;
import com.platform.core.logging.SupportabilityMdc;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;

import java.security.cert.X509Certificate;
import java.util.List;
import java.util.UUID;

/**
 * Agent → Sensor → Core agent-telemetry ingestion with reserve/claim/finalize semantics.
 * <p>
 * Delivery semantics (at-least-once, not XA): Kafka publication and PostgreSQL watermark
 * finalize are not bound by a distributed 2PC/XA transaction. Exact replay after durable
 * watermark commit is recognized via reservation and is not republished. Observation
 * {@code sequenceId} values are deterministic across Sensor retry so consumers can correlate
 * retries. Because Kafka+PostgreSQL are not XA, at-least-once delivery applies: duplicate Kafka
 * messages remain possible in the failure window between Kafka commit and durable finalize.
 * This path does not guarantee downstream physical deduplication.
 */
@Service
public class AgentIngestionService {
    private static final Logger log = LoggerFactory.getLogger(AgentIngestionService.class);

    private final AgentIngestionValidator validator;
    private final AgentIngressWatermarkStore watermarkStore;
    private final KafkaTelemetryProducer kafkaProducer;
    private final IngestionMetrics metrics;

    public AgentIngestionService(
            AgentIngestionValidator validator,
            AgentIngressWatermarkStore watermarkStore,
            KafkaTelemetryProducer kafkaProducer,
            IngestionMetrics metrics
    ) {
        this.validator = validator;
        this.watermarkStore = watermarkStore;
        this.kafkaProducer = kafkaProducer;
        this.metrics = metrics;
    }

    public AgentTelemetryAckResponse ingestAgentTelemetry(
            byte[] rawBody,
            AgentTelemetryBatchRequest request,
            String idempotencyKey,
            X509Certificate clientCert
    ) {
        metrics.recordBatchReceived();

        SensorScope scope = validator.resolveAuthority(clientCert);
        if (scope == null) {
            metrics.recordBatchRejected("ERR_SENSOR_AUTHORITY_UNAVAILABLE");
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "Authoritative Sensor Scope is unavailable; agent-telemetry ingestion is fail-closed"
            );
        }

        SensorIngestionService.requireIdempotencyKey(idempotencyKey, request.batchId());

        List<CanonicalObservation> observations = validator.validateAndExtract(rawBody, request, scope);

        ReplayReservation reservation = watermarkStore.reserveBatch(
                scope.tenantId(),
                scope.sensorId(),
                request.sensorAgentSpoolStreamId(),
                request.firstIngressSequence(),
                request.lastIngressSequence(),
                request.checksum(),
                request.batchId(),
                request.correlationId()
        );

        if (reservation.status() == ReplayReservation.Status.ALREADY_ACKED) {
            metrics.recordDuplicateReplay();
            log.info("Idempotent agent-ingress replay tenant={} sensor={} range=[{}..{}] watermark={}",
                    scope.tenantId(), scope.sensorId(),
                    request.firstIngressSequence(), request.lastIngressSequence(),
                    reservation.ackedThroughSequence());
            return new AgentTelemetryAckResponse(
                    "ACCEPTED",
                    scope.sensorId(),
                    request.batchId(),
                    reservation.ackedThroughSequence(),
                    request.correlationId()
            );
        }

        if (reservation.status() == ReplayReservation.Status.REJECTED) {
            String code = reservation.rejectionCode();
            metrics.recordBatchRejected(code);
            if ("ERR_PREVIOUS_BATCH_PENDING".equals(code)) {
                metrics.recordPreviousBatchPending();
            } else if ("ERR_IDEMPOTENCY_CONFLICT".equals(code)) {
                metrics.recordReplayConflict();
            } else if ("ERR_SEQUENCE_GAP".equals(code)) {
                metrics.recordSequenceGap();
            } else if ("ERR_SEQUENCE_REGRESSION".equals(code)) {
                metrics.recordSequenceRegression();
            } else if ("ERR_SENSOR_STREAM_CHANGED".equals(code)) {
                metrics.recordStreamMismatch();
            }
            HttpStatus status = "ERR_SEQUENCE_GAP".equals(code) || "ERR_SEQUENCE_OUT_OF_BOUNDS".equals(code)
                    || "ERR_AGENT_STREAM_ID_INVALID".equals(code)
                    ? HttpStatus.BAD_REQUEST
                    : HttpStatus.CONFLICT;
            throw new IngestionException(status, code, reservation.rejectionMessage());
        }

        PublisherClaimResult claim = watermarkStore.claimPublisherLease(
                scope.tenantId(),
                scope.sensorId(),
                request.sensorAgentSpoolStreamId(),
                request.firstIngressSequence(),
                request.lastIngressSequence(),
                request.checksum(),
                request.batchId(),
                request.correlationId()
        );
        if (claim.status() == PublisherClaimResult.Status.BUSY) {
            metrics.recordPublisherLeaseContention();
            throw new IngestionException(
                    HttpStatus.TOO_MANY_REQUESTS,
                    claim.rejectionCode(),
                    claim.rejectionMessage()
            );
        }
        if (claim.status() == PublisherClaimResult.Status.REJECTED) {
            metrics.recordBatchRejected(claim.rejectionCode());
            throw new IngestionException(HttpStatus.CONFLICT, claim.rejectionCode(), claim.rejectionMessage());
        }

        UUID owner = claim.ownerToken();
        long epoch = claim.publisherEpoch();
        try {
            // At-least-once across Kafka+PG (not XA); Sensor retries on missing ACK.
            kafkaProducer.publishBatchTransactionally(TelemetryObservationBatch.ofNumeric(observations));
            watermarkStore.finalizeReservedBatch(
                    scope.tenantId(),
                    scope.sensorId(),
                    request.sensorAgentSpoolStreamId(),
                    request.firstIngressSequence(),
                    request.lastIngressSequence(),
                    request.checksum(),
                    request.batchId(),
                    request.correlationId(),
                    owner,
                    epoch
            );
        } catch (RuntimeException publishOrFinalizeFailure) {
            try {
                watermarkStore.releasePublisherLease(
                        scope.tenantId(),
                        scope.sensorId(),
                        request.sensorAgentSpoolStreamId(),
                        request.firstIngressSequence(),
                        request.lastIngressSequence(),
                        request.checksum(),
                        request.batchId(),
                        request.correlationId(),
                        owner,
                        epoch
                );
            } catch (RuntimeException releaseFailure) {
                SupportabilityMdc.withEventCode(CoreSupportabilityEventCodes.PUBLISHER_LEASE_RELEASE_FAILED, () ->
                        log.warn(
                                "Failed to release agent-ingress publisher lease after failure failureClass={}",
                                releaseFailure.getClass().getSimpleName()
                        )
                );
            }
            if (publishOrFinalizeFailure instanceof IngestionException ingestionException
                    && "ERR_KAFKA_PUBLICATION_FAILED".equals(ingestionException.getCode())) {
                metrics.recordKafkaFailure(CoreSupportabilityEventCodes.KAFKA_PUBLICATION_FAILED);
            } else {
                metrics.recordFinalizeFailure(CoreSupportabilityEventCodes.PUBLISHER_FINALIZE_FAILED);
            }
            throw publishOrFinalizeFailure;
        }

        return new AgentTelemetryAckResponse(
                "ACCEPTED",
                scope.sensorId(),
                request.batchId(),
                request.lastIngressSequence(),
                request.correlationId()
        );
    }
}
