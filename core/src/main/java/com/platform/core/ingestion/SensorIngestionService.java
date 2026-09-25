package com.platform.core.ingestion;

import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import com.platform.core.ingestion.replay.PublisherClaimResult;
import com.platform.core.ingestion.replay.ReplayReservation;
import com.platform.core.ingestion.replay.TelemetryWatermarkStore;
import com.platform.core.ingestion.dto.TelemetryObservationBatch;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;

import java.security.cert.X509Certificate;
import java.util.Objects;
import java.util.UUID;

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

    public TelemetryAckResponse ingestTelemetry(
            byte[] rawBody,
            TelemetryBatchRequest request,
            String idempotencyHeader,
            X509Certificate clientCert
    ) {
        metrics.recordBatchReceived();

        SensorScope scope = validator.resolveAuthority(clientCert);
        if (scope == null) {
            metrics.recordBatchRejected("ERR_SENSOR_AUTHORITY_UNAVAILABLE");
            throw new IngestionException(
                    HttpStatus.FORBIDDEN,
                    "ERR_SENSOR_AUTHORITY_UNAVAILABLE",
                    "Authoritative Sensor Scope is unavailable; telemetry ingestion is fail-closed"
            );
        }

        requireIdempotencyKey(idempotencyHeader, request.batchId());

        TelemetryObservationBatch observations = validator.validateAndExtract(rawBody, request, scope);

        ReplayReservation reservation = watermarkStore.reserveBatch(
                scope.tenantId(),
                scope.sensorId(),
                request.bootEpoch(),
                request.firstSequence(),
                request.lastSequence(),
                request.checksum(),
                request.batchId()
        );

        if (reservation.status() == ReplayReservation.Status.ALREADY_ACKED) {
            metrics.recordDuplicateReplay();
            log.info("Idempotent replay detected for tenant={} sensor={} range=[{}..{}] watermark={}",
                    scope.tenantId(), scope.sensorId(), request.firstSequence(), request.lastSequence(),
                    reservation.ackedThroughSequence());
            return new TelemetryAckResponse("ACCEPTED", reservation.ackedThroughSequence(), scope.sensorId());
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
                    ? HttpStatus.BAD_REQUEST
                    : HttpStatus.CONFLICT;
            throw new IngestionException(status, code, reservation.rejectionMessage());
        }

        PublisherClaimResult claim = watermarkStore.claimPublisherLease(
                scope.tenantId(),
                scope.sensorId(),
                request.bootEpoch(),
                request.firstSequence(),
                request.lastSequence(),
                request.checksum(),
                request.batchId()
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
            kafkaProducer.publishBatchTransactionally(observations);
            watermarkStore.finalizeReservedBatch(
                    scope.tenantId(),
                    scope.sensorId(),
                    request.bootEpoch(),
                    request.firstSequence(),
                    request.lastSequence(),
                    request.checksum(),
                    request.batchId(),
                    owner,
                    epoch
            );
        } catch (RuntimeException publishOrFinalizeFailure) {
            try {
                watermarkStore.releasePublisherLease(
                        scope.tenantId(),
                        scope.sensorId(),
                        request.bootEpoch(),
                        request.firstSequence(),
                        request.lastSequence(),
                        request.checksum(),
                        request.batchId(),
                        owner,
                        epoch
                );
            } catch (RuntimeException releaseFailure) {
                log.warn("Failed to release telemetry publisher lease after failure: {}", releaseFailure.getMessage());
            }
            if (publishOrFinalizeFailure instanceof IngestionException ingestionException
                    && "ERR_KAFKA_PUBLICATION_FAILED".equals(ingestionException.getCode())) {
                metrics.recordKafkaFailure(ingestionException.getMessage());
            } else {
                metrics.recordFinalizeFailure(publishOrFinalizeFailure.getMessage());
            }
            throw publishOrFinalizeFailure;
        }

        return new TelemetryAckResponse("ACCEPTED", request.lastSequence(), scope.sensorId());
    }

    public static void requireIdempotencyKey(String idempotencyHeader, String batchId) {
        if (batchId == null || batchId.isBlank()) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_BATCH_ID_REQUIRED",
                    "batchId is required"
            );
        }
        if (idempotencyHeader == null || idempotencyHeader.isBlank()) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_IDEMPOTENCY_KEY_REQUIRED",
                    "Idempotency-Key header is required and must equal batchId"
            );
        }
        if (!Objects.equals(idempotencyHeader, batchId)) {
            throw new IngestionException(
                    HttpStatus.BAD_REQUEST,
                    "ERR_IDEMPOTENCY_KEY_MISMATCH",
                    "Idempotency-Key must exactly equal body batchId"
            );
        }
    }
}
