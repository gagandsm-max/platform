package com.platform.core.api;

import com.platform.core.domain.monitoring.MonitoringProfileEntityReadService;
import com.platform.core.domain.monitoring.MonitoringProfileEntityReadService.EntityAssignmentView;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.AssignmentMode;
import com.platform.core.security.http.PlatformSecurityContext;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

import java.time.Instant;
import java.util.UUID;

/**
 * Read-only monitoring-profile assignment API (BUILD-070).
 *
 * <p>Requires bearer authentication and entity.read scoped to the exact entity.
 * AuthorizationRouteMapper classifies this tenant/entity subresource as ENTITY_READ;
 * the service independently rechecks a trusted PlatformPrincipal from the security context.
 * No policy definition, secret, capacity estimate, or mutation authority is exposed.
 *
 * <p>200 returns an active assignment projection; 204 means there is no assignment.
 * Denied access remains masked as 404 by the existing security stack.
 */
@RestController
@RequestMapping("/api/v1/tenants/{tenantId}/entities/{entityId}/monitoring-profile-assignment")
public final class MonitoringProfileAssignmentController {

    public record AssignmentResponse(
            UUID entityId,
            UUID profileId,
            int profileVersionNo,
            AssignmentMode assignmentMode,
            long rowVersion,
            Instant effectiveFrom
    ) { }

    private final MonitoringProfileEntityReadService reader;

    public MonitoringProfileAssignmentController(MonitoringProfileEntityReadService reader) {
        this.reader = java.util.Objects.requireNonNull(reader, "reader");
    }

    @GetMapping
    public ResponseEntity<AssignmentResponse> get(
            @PathVariable UUID tenantId,
            @PathVariable UUID entityId
    ) {
        return reader.findEntityAssignment(
                        PlatformSecurityContext.requirePrincipal(), tenantId, entityId)
                .<ResponseEntity<AssignmentResponse>>map(
                        v -> ResponseEntity.ok(toResponse(v)))
                .orElseGet(() -> ResponseEntity.noContent().build());
    }

    private static AssignmentResponse toResponse(EntityAssignmentView view) {
        return new AssignmentResponse(
                view.entityId(), view.profileId(), view.profileVersionNo(),
                view.assignmentMode(), view.rowVersion(), view.effectiveFrom()
        );
    }
}
