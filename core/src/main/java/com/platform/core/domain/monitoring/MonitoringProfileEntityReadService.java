package com.platform.core.domain.monitoring;

import com.platform.core.persistence.MonitoringProfileRepository;
import com.platform.core.persistence.MonitoringProfileRepository.Assignment;
import com.platform.core.security.authorization.AuthorizationEvaluator;
import com.platform.core.security.authorization.AuthorizationRequest;
import com.platform.core.security.authorization.PlatformPermissions;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.authorization.ResourceType;
import com.platform.core.security.http.UnauthorizedAccessException;
import org.springframework.stereotype.Service;

import java.time.Instant;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;

/**
 * BUILD-070 read-only entity monitoring assignment facade.
 *
 * <p>Callers must supply a trusted, authenticated Platform principal context,
 * never an identity assembled from request parameters. Authorization is always
 * evaluated before touching persistence; this facade does not disclose profile
 * definitions, capacity estimates, credentials, or assignment mutations.
 * A separate policy-management permission boundary is required before exposing
 * profile catalog/definition APIs.
 */
@Service
public final class MonitoringProfileEntityReadService {

    public record EntityAssignmentView(
            UUID entityId,
            UUID profileId,
            int profileVersionNo,
            MonitoringProfileSelection.AssignmentMode assignmentMode,
            long rowVersion,
            Instant effectiveFrom
    ) { }

    private final MonitoringProfileRepository profiles;
    private final AuthorizationEvaluator authorization = new AuthorizationEvaluator();

    public MonitoringProfileEntityReadService(MonitoringProfileRepository profiles) {
        this.profiles = Objects.requireNonNull(profiles, "profiles");
    }

    public Optional<EntityAssignmentView> findEntityAssignment(
            PrincipalContext authenticatedPrincipal, UUID tenantId, UUID entityId
    ) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(entityId, "entityId");

        if (!authorization.authorize(authenticatedPrincipal, new AuthorizationRequest(
                tenantId, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, entityId)).allowed()) {
            // One masked response for tenant, permission, and named-resource denial.
            throw new UnauthorizedAccessException("RESOURCE_NOT_FOUND", "Resource not found", 404);
        }

        Optional<Assignment> assigned = profiles.findAssignment(tenantId, entityId);
        if (assigned.isEmpty()) {
            return Optional.empty();
        }
        Assignment row = assigned.get();
        if (!tenantId.equals(row.tenantId()) || !entityId.equals(row.entityId())) {
            // Persistence-scope corruption must never become cross-tenant disclosure.
            throw new IllegalStateException("Monitoring assignment persistence boundary mismatch");
        }
        return Optional.of(new EntityAssignmentView(
                row.entityId(), row.profileId(), row.profileVersionNo(),
                row.mode(), row.rowVersion(), row.effectiveFrom()
        ));
    }
}
