package com.platform.core.ingestion.agentingress;

import com.platform.core.ingestion.IngestionException;
import com.platform.core.ingestion.IngestionMetrics;
import com.platform.core.ingestion.KafkaTelemetryProducer;
import com.platform.core.ingestion.SensorIngestionService;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryAckResponse;
import com.platform.core.ingestion.agentingress.dto.AgentTelemetryBatchRequest;
import com.platform.core.ingestion.agentingress.dto.AgentIngressRecord;
import com.platform.core.agentcontrol.EntityAgentBinding;
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
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.HexFormat;
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

        validator.validateImmutableBatch(rawBody, request, scope);
        SensorIngestionService.requireIdempotencyKey(idempotencyKey, request.batchId());
        String requestSha256 = sha256(rawBody);

        ReplayReservation reservation = watermarkStore.reserveBatchWithBody(
                scope.tenantId(),
                scope.sensorId(),
                request.sensorAgentSpoolStreamId(),
                request.firstIngressSequence(),
                request.lastIngressSequence(),
                request.checksum(),
                request.batchId(),
                request.correlationId(), requestSha256
        );

        // Isolated E2E profile only: the reservation transaction has returned
        // (and committed), while authority validation/checkpoint has not begun.
        if (reservation.status() == ReplayReservation.Status.RESERVED
                && "agent-ingress-e2e".equals(System.getenv("SPRING_PROFILES_ACTIVE"))
                && "10,12".equals(System.getenv("PLATFORM_AGENT_INGRESS_E2E_FAIL_AFTER_RESERVE"))
                && (request.firstIngressSequence() == 10 || request.firstIngressSequence() == 12)) {
            throw new IngestionException(HttpStatus.SERVICE_UNAVAILABLE,
                    "ERR_AGENT_E2E_AFTER_RESERVE", "E2E reservation failure injection");
        }

        if (reservation.status() == ReplayReservation.Status.TERMINALLY_REJECTED) {
            if (request.records().size() != 1 || !watermarkStore.terminalReplayMatchesRecord(
                    scope.tenantId(), scope.sensorId(), reservation.terminalDispositionId(),
                    request.records().get(0))) {
                throw new IngestionException(HttpStatus.CONFLICT, "ERR_IDEMPOTENCY_CONFLICT",
                        "Terminal disposition record identity differs from the retry");
            }
            metrics.recordDuplicateReplay();
            return terminalResponse(scope, request, reservation);
        }

        if (reservation.status() == ReplayReservation.Status.ALREADY_ACKED) {
            metrics.recordDuplicateReplay();
            log.info("Idempotent agent-ingress replay tenant={} sensor={} range=[{}..{}] watermark={}",
                    scope.tenantId(), scope.sensorId(),
                    request.firstIngressSequence(), request.lastIngressSequence(),
                    reservation.ackedThroughSequence());
            return acceptedResponse(scope, request, reservation.ackedThroughSequence());
        }

        if (reservation.status() == ReplayReservation.Status.REJECTED) {
            String code = reservation.rejectionCode();
            metrics.recordBatchRejected(code);
            if ("ERR_PREVIOUS_BATCH_PENDING".equals(code)) {
                metrics.recordPreviousBatchPending();
            } else if ("ERR_IDEMPOTENCY_CONFLICT".equals(code) || "ERR_REPLAY_CONFLICT".equals(code)) {
                metrics.recordReplayConflict();
            } else if ("ERR_SEQUENCE_GAP".equals(code)) {
                metrics.recordSequenceGap();
            } else if ("ERR_SEQUENCE_REGRESSION".equals(code)) {
                metrics.recordSequenceRegression();
            } else if ("ERR_SENSOR_STREAM_CHANGED".equals(code)) {
                metrics.recordStreamMismatch();
            }
            HttpStatus status = "ERR_AGENT_LEGACY_PENDING_RECONCILIATION_REQUIRED".equals(code)
                    ? HttpStatus.SERVICE_UNAVAILABLE
                    : "ERR_SEQUENCE_GAP".equals(code) || "ERR_SEQUENCE_OUT_OF_BOUNDS".equals(code)
                    || "ERR_AGENT_STREAM_ID_INVALID".equals(code)
                    ? HttpStatus.BAD_REQUEST
                    : HttpStatus.CONFLICT;
            throw new IngestionException(status, code, reservation.rejectionMessage());
        }

        boolean legacyPending = "LEGACY_UNKNOWN".equals(reservation.pendingAuthorityState());
        boolean validatedPending = "AUTHORITY_VALIDATED".equals(reservation.pendingAuthorityState());
        List<CanonicalObservation> observations;
        try {
            if (validatedPending) {
                observations = validator.extractValidatedSnapshot(request, scope, reservation.pendingBindings());
                if (reservation.pendingCanonicalBatchSha256() == null
                        || !reservation.pendingCanonicalBatchSha256().equals(
                                CanonicalBindingDigest.combine(scope.tenantId(), scope.sensorId(),
                                        reservation.pendingReservationId(),
                                        validator.canonicalBatchDigest(observations),
                                        reservation.pendingBindings()))) {
                    throw new IngestionException(HttpStatus.SERVICE_UNAVAILABLE,
                            "ERR_AGENT_CANONICAL_DIGEST_MISMATCH",
                            "Validated canonical batch digest does not match the pending authority checkpoint");
                }
            } else {
                AgentIngressWatermarkStore.AuthorityCheckpoint checkpoint =
                        watermarkStore.checkpointAuthority(scope.tenantId(), scope.sensorId(),
                        request.sensorAgentSpoolStreamId(), request.firstIngressSequence(),
                        request.lastIngressSequence(), request.checksum(), request.batchId(),
                        request.correlationId(), requestSha256, reservation.pendingReservationId(),
                        () -> {
                            AgentIngestionValidator.ValidatedBatch validated = legacyPending
                                    ? validator.validateLegacyAndSnapshot(request, scope)
                                    : validator.validateAndSnapshot(request, scope);
                            if (validated.observations().isEmpty()) {
                                throw new IngestionException(HttpStatus.BAD_REQUEST,
                                        "ERR_NO_CANONICAL_OBSERVATIONS",
                                        "Batch contains zero supported canonical numeric observations");
                            }
                            return new AgentIngressWatermarkStore.AuthorityCheckpoint(
                                    validated.observations(), validated.bindings(),
                                    validator.canonicalBatchDigest(validated.observations()));
                        });
                if (checkpoint == null) {
                    throw new IngestionException(HttpStatus.SERVICE_UNAVAILABLE,
                            "ERR_AGENT_AUTHORITY_CHECKPOINT_UNCERTAIN",
                            "Agent-ingress authority checkpoint is unconfirmed; retain and retry exact batch");
                }
                observations = checkpoint.observations();
            }
        } catch (IngestionException validationFailure) {
            if ("ERR_AGENT_AUTHORITY_CHECKPOINT_UNCERTAIN".equals(validationFailure.getCode())) {
                metrics.recordBatchRejected(validationFailure.getCode());
                throw validationFailure;
            }
            if (legacyPending) {
                metrics.recordBatchRejected("ERR_AGENT_LEGACY_PENDING_RECONCILIATION_REQUIRED");
                throw new IngestionException(HttpStatus.SERVICE_UNAVAILABLE,
                        "ERR_AGENT_LEGACY_PENDING_RECONCILIATION_REQUIRED",
                        "Historical Agent authority cannot be proven; retain the exact pending batch",
                        validationFailure);
            }
            metrics.recordBatchRejected(validationFailure.getCode());
            if (validatedPending) {
                throw validationFailure;
            }
            if (!"ERR_AGENT_IDENTITY_CONFLICT".equals(validationFailure.getCode())
                    || request.records().size() != 1) {
                releaseRejectedPending(scope, request, requestSha256, reservation);
                throw validationFailure;
            }
            AgentIngressRecord record = request.records().get(0);
            if (record.records() == null || record.records().size() != 1) {
                releaseRejectedPending(scope, request, requestSha256, reservation);
                throw validationFailure;
            }
            try {
                EntityAgentBinding binding = validator.validateCurrentTerminalAuthority(record, scope);
                ReplayReservation terminal = watermarkStore.terminallyRejectIdentityConflict(
                        scope.tenantId(), scope.sensorId(), request.sensorAgentSpoolStreamId(),
                        record.sensorIngressSequence(), request.checksum(), request.batchId(),
                        request.correlationId(), record, binding.entityId(), binding.reinstallGeneration());
                if (terminal.status() != ReplayReservation.Status.TERMINALLY_REJECTED) {
                    throw new IngestionException(HttpStatus.CONFLICT,
                            terminal.rejectionCode(), terminal.rejectionMessage());
                }
                log.warn("Agent-ingress identity conflict durably terminally settled tenant={} sensor={} sequence={} disposition={}",
                        scope.tenantId(), scope.sensorId(), record.sensorIngressSequence(), terminal.terminalDispositionId());
                return terminalResponse(scope, request, terminal);
            } catch (IngestionException terminalAuthorityFailure) {
                releaseRejectedPending(scope, request, requestSha256, reservation);
                throw terminalAuthorityFailure;
            }
        }

        PublisherClaimResult claim = watermarkStore.claimPublisherLeaseExact(
                scope.tenantId(),
                scope.sensorId(),
                request.sensorAgentSpoolStreamId(),
                request.firstIngressSequence(),
                request.lastIngressSequence(),
                request.checksum(),
                request.batchId(),
                request.correlationId(), requestSha256, reservation.pendingReservationId()
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

        long acceptedThrough = reservation.ackedThroughSequence() == request.firstIngressSequence() - 1
                ? request.lastIngressSequence() : reservation.ackedThroughSequence();
        return acceptedResponse(scope, request, acceptedThrough);
    }

    private static AgentTelemetryAckResponse acceptedResponse(SensorScope scope,
            AgentTelemetryBatchRequest request, long acceptedThrough) {
        return new AgentTelemetryAckResponse("ACCEPTED", scope.sensorId(), request.batchId(),
                acceptedThrough, request.lastIngressSequence(), request.correlationId(),
                null, null, null, null, null, null);
    }

    private static String sha256(byte[] body) {
        if (body == null) {
            throw new IngestionException(HttpStatus.BAD_REQUEST, "ERR_EMPTY_BODY", "Request body is empty");
        }
        try {
            return HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(body));
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException("SHA-256 unavailable", e);
        }
    }

    private void releaseRejectedPending(SensorScope scope, AgentTelemetryBatchRequest request,
            String requestSha256, ReplayReservation reservation) {
        boolean released;
        try {
            released = watermarkStore.releaseUnpublishedPending(
                    scope.tenantId(), scope.sensorId(), request.sensorAgentSpoolStreamId(),
                    request.firstIngressSequence(), request.lastIngressSequence(), request.checksum(),
                    request.batchId(), request.correlationId(), requestSha256,
                    reservation.pendingReservationId());
        } catch (RuntimeException releaseFailure) {
            metrics.recordBatchRejected("ERR_AGENT_PENDING_RELEASE_UNCERTAIN");
            log.warn("code=ERR_AGENT_PENDING_RELEASE_UNCERTAIN tenant={} sensor={} range=[{}..{}] release failed",
                    scope.tenantId(), scope.sensorId(), request.firstIngressSequence(),
                    request.lastIngressSequence(), releaseFailure);
            throw new IngestionException(HttpStatus.SERVICE_UNAVAILABLE,
                    "ERR_AGENT_PENDING_RELEASE_UNCERTAIN",
                    "Agent-ingress pending cleanup is unconfirmed; retain and retry the exact batch",
                    releaseFailure);
        }
        if (!released) {
            metrics.recordBatchRejected("ERR_AGENT_PENDING_RELEASE_UNCERTAIN");
            log.warn("code=ERR_AGENT_PENDING_RELEASE_UNCERTAIN tenant={} sensor={} range=[{}..{}] exact unclaimed release not proven",
                    scope.tenantId(), scope.sensorId(), request.firstIngressSequence(),
                    request.lastIngressSequence());
            throw new IngestionException(HttpStatus.SERVICE_UNAVAILABLE,
                    "ERR_AGENT_PENDING_RELEASE_UNCERTAIN",
                    "Agent-ingress pending cleanup is unconfirmed; retain and retry the exact batch");
        }
    }

    private static AgentTelemetryAckResponse terminalResponse(SensorScope scope,
            AgentTelemetryBatchRequest request, ReplayReservation reservation) {
        return new AgentTelemetryAckResponse("TERMINALLY_REJECTED", scope.sensorId(), request.batchId(),
                reservation.ackedThroughSequence(), null, request.correlationId(),
                request.firstIngressSequence(), request.lastIngressSequence(), request.checksum(),
                reservation.settledThroughSequence(), reservation.terminalDispositionId(),
                reservation.rejectionCode());
    }
}
