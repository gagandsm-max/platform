package com.platform.core.ingestion;

import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import com.platform.core.ingestion.replay.TelemetryWatermarkStore;
import com.platform.core.ingestion.replay.WatermarkState;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.security.cert.X509Certificate;
import java.util.List;

@Service
public class SensorIngestionService {
    private static final Logger log = LoggerFactory.getLogger(SensorIngestionService.class);

    private final IngestionValidator validator;
    private final SensorAuthorityResolver authorityResolver;
    private final TelemetryWatermarkStore watermarkStore;
    private final KafkaTelemetryProducer kafkaProducer;
    private final IngestionMetrics metrics;

    public SensorIngestionService(
            IngestionValidator validator,
            SensorAuthorityResolver authorityResolver,
            TelemetryWatermarkStore watermarkStore,
            KafkaTelemetryProducer kafkaProducer,
            IngestionMetrics metrics
    ) {
        this.validator = validator;
        this.authorityResolver = authorityResolver;
        this.watermarkStore = watermarkStore;
        this.kafkaProducer = kafkaProducer;
        this.metrics = metrics;
    }

    @Transactional
    public TelemetryAckResponse ingestTelemetry(
            byte[] rawBody,
            TelemetryBatchRequest request,
            String idempotencyHeader,
            X509Certificate clientCert
    ) {
        metrics.recordBatchReceived();

        // 1. Resolve Sensor Authority (fails closed if authority unavailable)
        SensorScope scope = validator.resolveAuthority(clientCert);
        if (scope == null) {
            metrics.recordBatchRejected("ERR_SENSOR_AUTHORITY_UNAVAILABLE");
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "Authoritative Sensor Scope is unavailable; telemetry ingestion is fail-closed"
            );
        }

        // 2. Validate payload bounds, checksum, entity/tenant scope, and extract observations
        List<CanonicalObservation> observations = validator.validateAndExtract(rawBody, request, scope);

        // 3. Concurrency-safe Watermark Check from durable PostgreSQL store
        WatermarkState watermark = watermarkStore.getWatermarkForUpdate(scope.sensorId());
        if (watermark != null) {
            // Case A: Exact retry of last accepted batch range and checksum
            if (request.firstSequence() == watermark.lastFirstSequence()
                    && request.lastSequence() == watermark.lastLastSequence()) {
                if (request.checksum().equalsIgnoreCase(watermark.lastChecksum())) {
                    metrics.recordDuplicateReplay();
                    log.info("Idempotent replay detected for sensor={} range=[{}..{}] watermark={}",
                            scope.sensorId(), request.firstSequence(), request.lastSequence(), watermark.ackedThroughSequence());
                    return new TelemetryAckResponse("ACCEPTED", watermark.ackedThroughSequence(), scope.sensorId());
                } else {
                    // Conflicting checksum for same accepted sequence range
                    metrics.recordBatchRejected("ERR_IDEMPOTENCY_CONFLICT");
                    throw new IngestionException(
                            HttpStatus.CONFLICT,
                            "ERR_IDEMPOTENCY_CONFLICT",
                            "Batch sequence range [" + request.firstSequence() + ".." + request.lastSequence()
                                    + "] was previously accepted with a different payload checksum"
                    );
                }
            }

            // Case B: Sequence regression (firstSequence is at or below previously accepted watermark)
            if (request.firstSequence() <= watermark.ackedThroughSequence()) {
                metrics.recordBatchRejected("ERR_SEQUENCE_REGRESSION");
                throw new IngestionException(
                        HttpStatus.CONFLICT,
                        "ERR_SEQUENCE_REGRESSION",
                        "Batch firstSequence " + request.firstSequence()
                                + " has already been accepted (current watermark: " + watermark.ackedThroughSequence() + ")"
                );
            }

            // Case C: Sequence gap (firstSequence skips beyond next expected sequence)
            if (request.firstSequence() > watermark.ackedThroughSequence() + 1) {
                metrics.recordBatchRejected("ERR_SEQUENCE_GAP");
                throw new IngestionException(
                        HttpStatus.BAD_REQUEST,
                        "ERR_SEQUENCE_GAP",
                        "Batch firstSequence " + request.firstSequence()
                                + " leaves an unexpected gap from watermark " + watermark.ackedThroughSequence()
                );
            }
        }

        // 4. Publish atomically in a single Kafka transaction
        kafkaProducer.publishBatchTransactionally(observations);

        // 5. Update durable watermark state in PostgreSQL
        watermarkStore.updateWatermark(
                scope.sensorId(),
                scope.tenantId(),
                request.lastSequence(),
                request.firstSequence(),
                request.lastSequence(),
                request.checksum(),
                request.batchId()
        );

        // 6. Return durable ACK indicating accepted-through watermark
        return new TelemetryAckResponse("ACCEPTED", request.lastSequence(), scope.sensorId());
    }
}
