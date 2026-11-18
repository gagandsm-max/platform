package com.platform.core.domain.monitoring;

import com.platform.core.domain.DomainException;

import java.util.Map;
import java.util.Objects;
import java.util.UUID;

/**
 * Pure dormant Criticality/Profile assignment resolution ({@code REQ-INF-016} / BUILD-070).
 *
 * <p>Deterministic selection only: no persistence, no API, no collector activation, and no
 * hardcoded Criticality→profile mapping. Callers supply the suggested profile reference; this
 * component chooses the effective profile from assignment mode alone.
 */
public final class MonitoringProfileSelection {

    public static final String ERR_VALIDATION = "VALIDATION_FAILED";

    private MonitoringProfileSelection() {
    }

    /** Assignment mode vocabulary from Platform_17 {@code monitoring_profile_assignment}. */
    public enum AssignmentMode {
        AUTO_FROM_CRITICALITY,
        EXPLICIT_PROFILE
    }

    /**
     * Immutable Monitoring Profile reference. {@code profileVersionNo} is the immutable version
     * identity supplied by the caller (never remapped here).
     */
    public record ProfileRef(UUID tenantId, String profileId, int profileVersionNo) {
        public ProfileRef {
            Objects.requireNonNull(tenantId, "tenantId");
            Objects.requireNonNull(profileId, "profileId");
        }
    }

    /**
     * Selection request. {@code suggestedProfile} is always the caller-supplied Criticality
     * suggestion; under {@link AssignmentMode#EXPLICIT_PROFILE} it must not replace the pinned
     * explicit profile.
     */
    public record Request(
            UUID tenantId,
            AssignmentMode assignmentMode,
            ProfileRef suggestedProfile,
            ProfileRef explicitProfile
    ) {
        public Request {
            Objects.requireNonNull(tenantId, "tenantId");
            Objects.requireNonNull(assignmentMode, "assignmentMode");
        }
    }

    /** Deterministic selection outcome: suggested vs effective profile/version. */
    public record Result(
            AssignmentMode assignmentMode,
            ProfileRef suggestedProfile,
            ProfileRef effectiveProfile
    ) {
        public Result {
            Objects.requireNonNull(assignmentMode, "assignmentMode");
            Objects.requireNonNull(suggestedProfile, "suggestedProfile");
            Objects.requireNonNull(effectiveProfile, "effectiveProfile");
        }
    }

    /**
     * Resolves suggested and effective Monitoring Profile references.
     *
     * <ul>
     *   <li>{@code AUTO_FROM_CRITICALITY}: effective = caller-supplied suggested profile+version</li>
     *   <li>{@code EXPLICIT_PROFILE}: effective = pinned explicit profile+version; suggestion may
     *       differ after a Criticality change</li>
     * </ul>
     *
     * <p>Every non-null supplied {@link ProfileRef} is validated for tenant match, non-blank
     * {@code profileId} and positive {@code profileVersionNo}, including an unused
     * {@code explicitProfile} under {@link AssignmentMode#AUTO_FROM_CRITICALITY}. Invalid unused
     * references are rejected rather than silently ignored; a valid unused explicit reference is
     * accepted and does not change the AUTO effective selection.
     *
     * @throws DomainException when a required reference is missing, a supplied reference has a
     *                         blank id / non-positive version, or any supplied reference is
     *                         cross-tenant
     */
    public static Result select(Request request) {
        Objects.requireNonNull(request, "request");

        ProfileRef suggested = validateSuggested(request);
        ProfileRef explicit = validateSuppliedExplicit(request);
        return switch (request.assignmentMode()) {
            case AUTO_FROM_CRITICALITY -> new Result(
                    AssignmentMode.AUTO_FROM_CRITICALITY,
                    suggested,
                    copy(suggested));
            case EXPLICIT_PROFILE -> {
                if (explicit == null) {
                    throw missing("explicitProfile");
                }
                yield new Result(AssignmentMode.EXPLICIT_PROFILE, suggested, explicit);
            }
        };
    }

    private static ProfileRef validateSuggested(Request request) {
        ProfileRef suggested = request.suggestedProfile();
        if (suggested == null) {
            throw missing("suggestedProfile");
        }
        return validateRef(request.tenantId(), suggested, "suggestedProfile");
    }

    /**
     * Validates a non-null explicit reference even when assignment mode will not select it.
     * Returns {@code null} when no explicit reference was supplied.
     */
    private static ProfileRef validateSuppliedExplicit(Request request) {
        ProfileRef explicit = request.explicitProfile();
        if (explicit == null) {
            return null;
        }
        return validateRef(request.tenantId(), explicit, "explicitProfile");
    }

    private static ProfileRef validateRef(UUID tenantId, ProfileRef ref, String field) {
        String profileId = ref.profileId();
        if (profileId.isBlank()) {
            throw missing(field);
        }
        if (ref.profileVersionNo() < 1) {
            throw new DomainException(
                    ERR_VALIDATION,
                    field + " profileVersionNo must be >= 1",
                    Map.of(field, "invalidVersion"));
        }
        if (!tenantId.equals(ref.tenantId())) {
            throw new DomainException(
                    ERR_VALIDATION,
                    field + " tenant does not match request tenant",
                    Map.of(field, "crossTenant"));
        }
        return copy(ref);
    }

    private static ProfileRef copy(ProfileRef ref) {
        // Defensive snapshot so callers cannot observe shared mutable identity assumptions.
        return new ProfileRef(ref.tenantId(), ref.profileId(), ref.profileVersionNo());
    }

    private static DomainException missing(String field) {
        return new DomainException(
                ERR_VALIDATION,
                field + " is required",
                Map.of(field, "required"));
    }
}
