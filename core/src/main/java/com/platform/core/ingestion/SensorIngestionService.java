package com.platform.core.ingestion;

import com.platform.core.ingestion.dto.CanonicalObservation;
import com.platform.core.ingestion.dto.TelemetryAckResponse;
import com.platform.core.ingestion.dto.TelemetryBatchRequest;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;

import java.security.cert.X509Certificate;
import java.time.Instant;
import java.util.List;

@Service
public class SensorIngestionService {
    private static final Logger log = LoggerFactory.getLogger(SensorIngestionService.class);

    private final IngestionValidator validator;
    private final IdempotencyTracker idempotencyTracker;
    private final KafkaTelemetryProducer kafkaProducer;
    private final IngestionMetrics metrics;

    public SensorIngestionService(
            IngestionValidator validator,
            IdempotencyTracker idempotencyTracker,
            KafkaTelemetryProducer kafkaProducer,
            IngestionMetrics metrics
    ) {
        this.validator = validator;
        this.idempotencyTracker = idempotencyTracker;
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

        // 1. Validate mTLS, structure, batch bounds, checksum and parse observations
        List<CanonicalObservation> observations = validator.validateAndExtract(rawBody, request, clientCert);

        // Derive idempotency key: if Idempotency-Key is not set, use batchId, fallback to sensorId:sequence
        String idempotencyKey = idempotencyHeader;
        if (idempotencyKey == null || idempotencyKey.isBlank()) {
            idempotencyKey = request.batchId();
        }
        if (idempotencyKey == null || idempotencyKey.isBlank()) {
            idempotencyKey = request.sensorId() + ":" + request.lastSequence();
        }

        // 2. Check Idempotency for duplicate replay or checksum conflicts
        TelemetryAckResponse duplicateAck = idempotencyTracker.checkDuplicate(idempotencyKey, request.checksum());
        if (duplicateAck != null) {
            metrics.recordDuplicateReplay();
            log.info("Idempotent replay detected for sensor={} batchId={} seq={}",
                    request.sensorId(), request.batchId(), request.lastSequence());
            return duplicateAck;
        }

        // 3. Synchronous Durable Publication to Kafka (Durable Acceptance Boundary)
        // Returns accepted_through_sequence ONLY after Kafka broker confirms persistent write
        long startTime = System.currentTimeMillis();
        kafkaProducer.publishSynchronously(observations);
        metrics.recordKafkaPublish(observations.size(), System.currentTimeMillis() - startTime);

        // 4. Formulate Ack
        TelemetryAckResponse ack = new TelemetryAckResponse(
                "ACCEPTED",
                request.lastSequence(),
                request.sensorId()
        );

        // 5. Record successful publication in idempotency cache
        idempotencyTracker.recordSuccess(idempotencyKey, request.sensorId(), request.lastSequence(), request.checksum(), ack);

        return ack;
    }
}
