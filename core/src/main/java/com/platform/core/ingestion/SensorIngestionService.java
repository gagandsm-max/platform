package com.platform.core.ingestion;

import com.platform.core.ingestion.authority.SensorAuthorityResolver;
import com.platform.core.ingestion.authority.SensorScope;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import com.platform.core.ingestion.replay.TelemetryWatermarkStore;
import com.platform.core.ingestion.replay.ReplayReservation;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Service;

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

    public TelemetryAckResponse ingestTelemetry(
            byte[] rawBody,
            TelemetryBatchRequest request,
            String idempotencyHeader,
            X509Certificate clientCert
    ) {
        metrics.recordBatchReceived();

        // 1. Resolve Sensor Authority (fingerprint → enrollment; never CN/SAN). Payload claims cannot grant authority.
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

        // 3. Durably reserve this exact sequence range/checksum in PostgreSQL BEFORE Kafka.
        // The reservation commits independently and survives a Core crash. This closes the
        // conflict hole where Kafka had committed but the ACK watermark had not: a retry with
        // different payload bytes for the same sequence range is rejected, while the exact
        // same retry is allowed to republish idempotently.
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
            HttpStatus status = "ERR_SEQUENCE_GAP".equals(code)
                    ? HttpStatus.BAD_REQUEST
                    : HttpStatus.CONFLICT;
            throw new IngestionException(status, code, reservation.rejectionMessage());
        }

        // 4. Publish atomically in a single Kafka transaction.
        // Kafka and PostgreSQL are intentionally not XA/2PC. If Core crashes after Kafka commit,
        // the pending reservation remains and the Sensor retries. Stable logical observation IDs
        // plus replay-safe ClickHouse reads make the duplicate publication harmless
        // (at-least-once, not exactly-once). Concurrent HTTP retries of the same reserved batch
        // may all republish under RETRY_PENDING; that amplification is a P1 hardening concern
        // because the normal Sensor uploader path serializes retries. Do not introduce a
        // publisher lease/new datastore here without production evidence.
        kafkaProducer.publishBatchTransactionally(observations);

        // 5. Finalize the reserved batch in a new PostgreSQL transaction only after Kafka commit.
        // If this step fails, no ACK is returned; the same reserved checksum may safely retry.
        watermarkStore.finalizeReservedBatch(
                scope.tenantId(),
                scope.sensorId(),
                request.bootEpoch(),
                request.firstSequence(),
                request.lastSequence(),
                request.checksum(),
                request.batchId()
        );

        // 6. Return durable ACK indicating accepted-through watermark
        return new TelemetryAckResponse("ACCEPTED", request.lastSequence(), scope.sensorId());
    }
}
