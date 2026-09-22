package com.platform.core.domain.inventory;

import com.platform.core.domain.DomainException;
import com.platform.core.id.UuidV7;
import com.platform.core.persistence.EntityManagementEndpointRepository;
import com.platform.core.persistence.EntityRepository;
import com.platform.core.security.audit.AuditActions;
import com.platform.core.security.audit.AuditCategory;
import com.platform.core.security.audit.AuditOutcome;
import com.platform.core.security.http.PlatformSecurityContext;
import com.platform.core.security.http.SecurityAuditEmitter;
import org.springframework.dao.DuplicateKeyException;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;

/**
 * Authoritative entity management-endpoint control plane.
 * Discovery observations never become authority without an explicit governed create call.
 */
@Service
public class EntityManagementEndpointService {

    public static final String ERR_NOT_FOUND = "MANAGEMENT_ENDPOINT_NOT_FOUND";
    public static final String ERR_PRIMARY_CONFLICT = "MANAGEMENT_ENDPOINT_PRIMARY_CONFLICT";

    private final EntityRepository entities;
    private final EntityManagementEndpointRepository endpoints;
    private final SecurityAuditEmitter auditEmitter;

    public EntityManagementEndpointService(
            EntityRepository entities,
            EntityManagementEndpointRepository endpoints,
            SecurityAuditEmitter auditEmitter
    ) {
        this.entities = Objects.requireNonNull(entities, "entities");
        this.endpoints = Objects.requireNonNull(endpoints, "endpoints");
        this.auditEmitter = Objects.requireNonNull(auditEmitter, "auditEmitter");
    }

    /**
     * Authoritative primary ACTIVE management endpoint for scheduler/poll consumers.
     * Returns endpoint identity + canonical address only — never credentials.
     */
    @Transactional(value = "transactionManager", readOnly = true)
    public Optional<EntityManagementEndpoint> resolvePrimaryManagementEndpoint(UUID tenantId, UUID entityId) {
        requireEntity(tenantId, entityId);
        return endpoints.findActivePrimary(tenantId, entityId);
    }

    @Transactional(value = "transactionManager", readOnly = true)
    public List<EntityManagementEndpoint> list(UUID tenantId, UUID entityId) {
        requireEntity(tenantId, entityId);
        return endpoints.listByEntity(tenantId, entityId);
    }

    @Transactional("transactionManager")
    public EntityManagementEndpoint createManual(
            UUID tenantId,
            UUID entityId,
            ManagementEndpointAddressType addressType,
            String address,
            boolean makePrimary
    ) {
        return create(tenantId, entityId, addressType, address, ManagementEndpointSource.MANUAL, makePrimary);
    }

    /**
     * Explicit governed boundary for future discovery/provisioning approval workflows.
     * Must be called with an approved source — never auto-invoked from discovery ingestion.
     */
    @Transactional("transactionManager")
    public EntityManagementEndpoint createFromApprovedSource(
            UUID tenantId,
            UUID entityId,
            ManagementEndpointAddressType addressType,
            String address,
            ManagementEndpointSource source,
            boolean makePrimary
    ) {
        if (source == null || source == ManagementEndpointSource.MANUAL) {
            throw new DomainException("VALIDATION_FAILED", "approved source must be DISCOVERY or PROVISIONING");
        }
        return create(tenantId, entityId, addressType, address, source, makePrimary);
    }

    @Transactional("transactionManager")
    public EntityManagementEndpoint updateState(
            UUID tenantId,
            UUID entityId,
            UUID endpointId,
            ManagementEndpointState newState,
            Boolean makePrimary
    ) {
        requireEntity(tenantId, entityId);
        EntityManagementEndpoint existing = endpoints.findByTenantEntityAndId(tenantId, entityId, endpointId)
                .orElseThrow(() -> new DomainException(ERR_NOT_FOUND, "Management endpoint not found"));

        String actor = currentActor();
        Instant now = Instant.now();
        ManagementEndpointState state = newState != null ? newState : existing.state();
        boolean primary = makePrimary != null ? makePrimary : existing.primary();
        if (state != ManagementEndpointState.ACTIVE) {
            primary = false;
        }
        if (Boolean.TRUE.equals(makePrimary) && state != ManagementEndpointState.ACTIVE) {
            throw new DomainException("VALIDATION_FAILED", "primary endpoint must be ACTIVE");
        }

        if (primary) {
            endpoints.clearPrimaryFlags(tenantId, entityId, endpointId, actor, now);
        }

        EntityManagementEndpoint updated = new EntityManagementEndpoint(
                existing.endpointId(),
                existing.tenantId(),
                existing.entityId(),
                existing.addressType(),
                existing.address(),
                state,
                existing.source(),
                primary,
                existing.createdBy(),
                existing.createdAt(),
                actor,
                now
        );
        endpoints.update(updated);

        if (existing.state() != state) {
            emit(tenantId, state == ManagementEndpointState.ACTIVE
                            ? AuditActions.ENTITY_MANAGEMENT_ENDPOINT_ACTIVATE
                            : AuditActions.ENTITY_MANAGEMENT_ENDPOINT_DEACTIVATE,
                    "ENTITY_MANAGEMENT_ENDPOINT_STATE",
                    Map.of(
                            "entity_id", entityId.toString(),
                            "endpoint_id", endpointId.toString(),
                            "address_type", existing.addressType().name(),
                            "state", state.name()
                    ));
        }
        if (primary && !existing.primary()) {
            emit(tenantId, AuditActions.ENTITY_MANAGEMENT_ENDPOINT_PRIMARY_CHANGE,
                    "ENTITY_MANAGEMENT_ENDPOINT_PRIMARY",
                    Map.of(
                            "entity_id", entityId.toString(),
                            "endpoint_id", endpointId.toString(),
                            "address_type", existing.addressType().name(),
                            "address", existing.address()
                    ));
        }
        return updated;
    }

    private EntityManagementEndpoint create(
            UUID tenantId,
            UUID entityId,
            ManagementEndpointAddressType addressType,
            String address,
            ManagementEndpointSource source,
            boolean makePrimary
    ) {
        requireEntity(tenantId, entityId);
        ManagementEndpointAddressValidator.NormalizedAddress normalized =
                ManagementEndpointAddressValidator.normalizeAndValidate(addressType, address);

        String actor = currentActor();
        Instant now = Instant.now();
        UUID endpointId = UuidV7.generate();
        boolean primary = makePrimary;
        if (primary) {
            endpoints.clearPrimaryFlags(tenantId, entityId, null, actor, now);
        }

        EntityManagementEndpoint created = new EntityManagementEndpoint(
                endpointId,
                tenantId,
                entityId,
                normalized.type(),
                normalized.address(),
                ManagementEndpointState.ACTIVE,
                source,
                primary,
                actor,
                now,
                actor,
                now
        );
        try {
            endpoints.insert(created);
        } catch (DuplicateKeyException ex) {
            throw new DomainException(ERR_PRIMARY_CONFLICT, "Entity already has an ACTIVE primary endpoint");
        }

        emit(tenantId, AuditActions.ENTITY_MANAGEMENT_ENDPOINT_CREATE,
                "ENTITY_MANAGEMENT_ENDPOINT_CREATE",
                Map.of(
                        "entity_id", entityId.toString(),
                        "endpoint_id", endpointId.toString(),
                        "address_type", normalized.type().name(),
                        "address", normalized.address(),
                        "source", source.name(),
                        "is_primary", String.valueOf(primary)
                ));
        if (primary) {
            emit(tenantId, AuditActions.ENTITY_MANAGEMENT_ENDPOINT_PRIMARY_CHANGE,
                    "ENTITY_MANAGEMENT_ENDPOINT_PRIMARY",
                    Map.of(
                            "entity_id", entityId.toString(),
                            "endpoint_id", endpointId.toString(),
                            "address_type", normalized.type().name(),
                            "address", normalized.address()
                    ));
        }
        return created;
    }

    private Entity requireEntity(UUID tenantId, UUID entityId) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(entityId, "entityId");
        return entities.findByTenantAndId(tenantId, entityId)
                .orElseThrow(() -> new DomainException("ENTITY_NOT_FOUND", "Entity not found"));
    }

    private static String currentActor() {
        try {
            return PlatformSecurityContext.requirePrincipal().subjectId();
        } catch (RuntimeException ex) {
            return "system";
        }
    }

    private void emit(UUID tenantId, com.platform.core.security.audit.AuditAction action,
                      String eventType, Map<String, String> metadata) {
        auditEmitter.emitControlEvent(
                AuditCategory.SECURITY_ADMINISTRATION,
                action,
                AuditOutcome.SUCCESS,
                tenantId,
                eventType,
                metadata
        );
    }
}
