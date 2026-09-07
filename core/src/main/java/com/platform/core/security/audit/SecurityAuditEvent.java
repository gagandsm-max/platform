package com.platform.core.security.audit;

import com.platform.core.id.UuidV7;

import java.time.Instant;
import java.util.Map;
import java.util.Objects;
import java.util.UUID;

/**
 * Immutable security/control-plane audit fact contract.
 *
 * <p>Persistence, transport, retry, and durability are deliberately out of scope.
 * Future writers must surface write failures and must not silently discard security-sensitive facts.
 *
 * <p>Audit records authorization outcomes; they never grant authority.
 * Deny/reason detail may be richer than future HTTP client responses should expose.
 */
public final class SecurityAuditEvent {
    private final UUID eventId;
    private final Instant occurredAt;
    private final CorrelationId correlationId;
    private final AuditCategory category;
    private final AuditAction action;
    private final AuditOutcome outcome;
    private final ActorType actorType;
    private final UUID actorSubjectId;
    private final UUID tenantId;
    private final NetworkSourceAddress sourceAddress;
    private final AuditResourceType resourceType;
    private final UUID resourceId;
    private final AuditReasonCode reasonCode;
    private final AuditMetadata metadata;

    public SecurityAuditEvent(UUID eventId, Instant occurredAt, CorrelationId correlationId,
                              AuditCategory category, AuditAction action, AuditOutcome outcome,
                              ActorType actorType, UUID actorSubjectId, UUID tenantId, NetworkSourceAddress sourceAddress,
                              AuditResourceType resourceType, UUID resourceId,
                              AuditReasonCode reasonCode, AuditMetadata metadata) {
        this.eventId = Objects.requireNonNull(eventId, "eventId");
        this.occurredAt = Objects.requireNonNull(occurredAt, "occurredAt");
        this.correlationId = Objects.requireNonNull(correlationId, "correlationId");
        this.category = Objects.requireNonNull(category, "category");
        this.action = Objects.requireNonNull(action, "action");
        this.outcome = Objects.requireNonNull(outcome, "outcome");
        this.actorType = Objects.requireNonNull(actorType, "actorType");
        validateActor(actorType, actorSubjectId);
        if ((resourceType == null) != (resourceId == null)) {
            throw new IllegalArgumentException("resourceType and resourceId must both be present or absent");
        }
        this.actorSubjectId = actorSubjectId;
        this.tenantId = tenantId;
        this.sourceAddress = sourceAddress;
        this.resourceType = resourceType;
        this.resourceId = resourceId;
        this.reasonCode = reasonCode;
        this.metadata = metadata == null ? AuditMetadata.empty() : metadata;
    }

    public static SecurityAuditEvent create(Instant occurredAt, CorrelationId correlationId,
                                            AuditCategory category, AuditAction action, AuditOutcome outcome,
                                            ActorType actorType, UUID actorSubjectId, UUID tenantId, NetworkSourceAddress sourceAddress,
                                            AuditResourceType resourceType, UUID resourceId,
                                            AuditReasonCode reasonCode, Map<String, String> metadata) {
        return new SecurityAuditEvent(UuidV7.generate(), occurredAt, correlationId, category, action, outcome,
                actorType, actorSubjectId, tenantId, sourceAddress, resourceType, resourceId, reasonCode,
                metadata == null ? AuditMetadata.empty() : new AuditMetadata(metadata));
    }

    private static void validateActor(ActorType actorType, UUID actorSubjectId) {
        if (actorType == ActorType.SYSTEM && actorSubjectId != null) {
            throw new IllegalArgumentException("SYSTEM actor must not carry a Platform subject id");
        }
    }

    public UUID eventId() { return eventId; }
    public Instant occurredAt() { return occurredAt; }
    public CorrelationId correlationId() { return correlationId; }
    public AuditCategory category() { return category; }
    public AuditAction action() { return action; }
    public AuditOutcome outcome() { return outcome; }
    public ActorType actorType() { return actorType; }
    public UUID actorSubjectId() { return actorSubjectId; }
    public UUID tenantId() { return tenantId; }
    public NetworkSourceAddress sourceAddress() { return sourceAddress; }
    public AuditResourceType resourceType() { return resourceType; }
    public UUID resourceId() { return resourceId; }
    public AuditReasonCode reasonCode() { return reasonCode; }
    public AuditMetadata metadata() { return metadata; }

    /** Equality is by event identity only; identical facts with distinct IDs remain distinct. */
    @Override
    public boolean equals(Object other) {
        return this == other || (other instanceof SecurityAuditEvent that && eventId.equals(that.eventId));
    }

    @Override
    public int hashCode() {
        return eventId.hashCode();
    }

    @Override
    public String toString() {
        return "SecurityAuditEvent[eventId=" + eventId
                + ", occurredAt=" + occurredAt
                + ", correlationId=" + correlationId
                + ", category=" + category
                + ", action=" + action
                + ", outcome=" + outcome
                + ", actorType=" + actorType
                + ", actorSubjectId=" + actorSubjectId
                + ", tenantId=" + tenantId
                + ", sourceAddress=" + sourceAddress
                + ", resourceType=" + resourceType
                + ", resourceId=" + resourceId
                + ", reasonCode=" + reasonCode
                + ", metadataEntries=" + metadata.size() + "]";
    }
}
