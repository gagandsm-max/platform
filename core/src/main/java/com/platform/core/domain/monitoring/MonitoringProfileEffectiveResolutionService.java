package com.platform.core.domain.monitoring;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.AssignmentMode;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.ProfileRef;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.Request;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.Result;
import com.platform.core.persistence.MonitoringProfileRepository;
import com.platform.core.persistence.MonitoringProfileRepository.Assignment;
import com.platform.core.persistence.MonitoringProfileRepository.Version;
import com.platform.core.security.authorization.AuthorizationEvaluator;
import com.platform.core.security.authorization.AuthorizationRequest;
import com.platform.core.security.authorization.PlatformPermissions;
import com.platform.core.security.authorization.PrincipalContext;
import com.platform.core.security.authorization.ResourceType;
import com.platform.core.security.http.UnauthorizedAccessException;
import org.springframework.stereotype.Service;

import java.util.Map;
import java.util.Objects;
import java.util.Optional;
import java.util.UUID;

/** Dormant, read-only resolution of a tenant-scoped entity's effective monitoring profile. */
@Service
public final class MonitoringProfileEffectiveResolutionService {

    private final MonitoringProfileRepository profiles;
    private final AuthorizationEvaluator authorization = new AuthorizationEvaluator();

    public MonitoringProfileEffectiveResolutionService(MonitoringProfileRepository profiles) {
        this.profiles = Objects.requireNonNull(profiles, "profiles");
    }

    /**
     * Resolves assignment mode against the caller's already-derived suggestion. No suggestion is
     * inferred here. Authorization precedes every persistence lookup.
     */
    public Optional<Result> resolve(
            PrincipalContext trustedPrincipal, UUID tenantId, UUID entityId, ProfileRef suggestedProfile
    ) {
        Objects.requireNonNull(tenantId, "tenantId");
        Objects.requireNonNull(entityId, "entityId");
        if (!authorization.authorize(trustedPrincipal, new AuthorizationRequest(
                tenantId, PlatformPermissions.ENTITY_READ, ResourceType.ENTITY, entityId)).allowed()) {
            throw new UnauthorizedAccessException("RESOURCE_NOT_FOUND", "Resource not found", 404);
        }

        Optional<Assignment> persisted = profiles.findAssignment(tenantId, entityId);
        if (persisted.isEmpty()) {
            return Optional.empty();
        }
        Assignment assignment = persisted.get();
        if (!tenantId.equals(assignment.tenantId()) || !entityId.equals(assignment.entityId())) {
            throw new IllegalStateException("Monitoring assignment persistence boundary mismatch");
        }
        if (assignment.profileId() == null || assignment.profileVersionNo() < 1 || assignment.mode() == null) {
            throw new IllegalStateException("Monitoring assignment persistence record is invalid");
        }

        ProfileRef supplied = validateReference(tenantId, suggestedProfile, "suggestedProfile");
        ProfileRef pinned = new ProfileRef(tenantId, assignment.profileId().toString(),
                assignment.profileVersionNo());
        Result selected = MonitoringProfileSelection.select(new Request(
                tenantId, assignment.mode(), supplied,
                assignment.mode() == AssignmentMode.EXPLICIT_PROFILE ? pinned : null));
        if (!selected.effectiveProfile().equals(supplied)) {
            validateReference(tenantId, selected.effectiveProfile(), "effectiveProfile");
        }
        return Optional.of(selected);
    }

    private ProfileRef validateReference(UUID tenantId, ProfileRef reference, String field) {
        if (reference == null) {
            throw invalid(field, "required");
        }
        if (!tenantId.equals(reference.tenantId())) {
            throw invalid(field, "crossTenant");
        }
        if (reference.profileVersionNo() < 1) {
            throw invalid(field, "invalidVersion");
        }
        final UUID profileId;
        try {
            profileId = UUID.fromString(reference.profileId());
        } catch (IllegalArgumentException ex) {
            throw invalid(field, "invalidProfileId");
        }
        if (!profileId.toString().equals(reference.profileId())) {
            throw invalid(field, "nonCanonicalProfileId");
        }
        Optional<Version> found = profiles.findVersion(tenantId, profileId, reference.profileVersionNo());
        if (found.isEmpty()) {
            throw invalid(field, "unknownVersion");
        }
        Version version = found.get();
        if (!tenantId.equals(version.tenantId()) || !profileId.equals(version.profileId())
                || reference.profileVersionNo() != version.versionNo()) {
            throw new IllegalStateException("Monitoring profile version persistence boundary mismatch");
        }
        return reference;
    }

    private static DomainException invalid(String field, String reason) {
        return new DomainException(MonitoringProfileSelection.ERR_VALIDATION,
                "Invalid monitoring profile reference", Map.of(field, reason));
    }
}
