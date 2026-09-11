package com.platform.core.ingestion.agentingress;

import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.IngestionMetrics;
import com.platform.core.ingestion.KafkaTelemetryProducer;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryAckResponse;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryBatchRequest;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.replay.WatermarkState;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.security.cert.X509Certificate;
import java.util.List;

/**
 * Agent → Sensor → Core agent-telemetry ingestion.
 * <p>
 * Cross-system atomicity note (at-least-once, not XA): Kafka publication and PostgreSQL
 * watermark updates are not bound by a distributed 2PC/XA transaction. If Core crashes
 * or PostgreSQL rolls back after Kafka commit, the Sensor receives no ACK and retries.
 * Idempotency is guaranteed by deterministic observation sequenceId keys and watermark CAS.
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

    @Transactional("transactionManager")
    public AgentTelemetryAckResponse ingestAgentTelemetry(
            byte[] rawBody,
            AgentTelemetryBatchRequest request,
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

        List<CanonicalObservation> observations = validator.validateAndExtract(rawBody, request, scope);

        WatermarkState watermark = watermarkStore.getWatermarkForUpdate(scope.tenantId(), scope.sensorId());
        if (watermark != null) {
            if (request.firstIngressSequence() == watermark.lastFirstSequence()
                    && request.lastIngressSequence() == watermark.lastLastSequence()) {
                if (request.checksum().equalsIgnoreCase(watermark.lastChecksum())) {
                    metrics.recordDuplicateReplay();
                    log.info("Idempotent agent-ingress replay tenant={} sensor={} range=[{}..{}] watermark={}",
                            scope.tenantId(), scope.sensorId(),
                            request.firstIngressSequence(), request.lastIngressSequence(),
                            watermark.ackedThroughSequence());
                    return new AgentTelemetryAckResponse(
                            "ACCEPTED",
                            scope.sensorId(),
                            request.batchId(),
                            watermark.ackedThroughSequence(),
                            request.correlationId()
                    );
                }
                metrics.recordBatchRejected("ERR_IDEMPOTENCY_CONFLICT");
                throw new IngestionException(
                        HttpStatus.CONFLICT,
                        "ERR_IDEMPOTENCY_CONFLICT",
                        "Agent ingress sequence range [" + request.firstIngressSequence() + ".."
                                + request.lastIngressSequence()
                                + "] was previously accepted with a different payload checksum"
                );
            }

            if (request.firstIngressSequence() <= watermark.ackedThroughSequence()) {
                metrics.recordBatchRejected("ERR_SEQUENCE_REGRESSION");
                throw new IngestionException(
                        HttpStatus.CONFLICT,
                        "ERR_SEQUENCE_REGRESSION",
                        "Batch firstIngressSequence " + request.firstIngressSequence()
                                + " has already been accepted (current watermark: "
                                + watermark.ackedThroughSequence() + ")"
                );
            }

            if (request.firstIngressSequence() > watermark.ackedThroughSequence() + 1) {
                metrics.recordBatchRejected("ERR_SEQUENCE_GAP");
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_SEQUENCE_GAP",
                        "Batch firstIngressSequence " + request.firstIngressSequence()
                                + " leaves an unexpected gap from watermark "
                                + watermark.ackedThroughSequence()
                );
            }
        }

        // At-least-once across Kafka+PG (not XA); Sensor retries on missing ACK.
        kafkaProducer.publishBatchTransactionally(observations);

        watermarkStore.updateWatermark(
                scope.tenantId(),
                scope.sensorId(),
                request.lastIngressSequence(),
                request.firstIngressSequence(),
                request.lastIngressSequence(),
                request.checksum(),
                request.batchId()
        );

        return new AgentTelemetryAckResponse(
                "ACCEPTED",
                scope.sensorId(),
                request.batchId(),
                request.lastIngressSequence(),
                request.correlationId()
        );
    }
}
