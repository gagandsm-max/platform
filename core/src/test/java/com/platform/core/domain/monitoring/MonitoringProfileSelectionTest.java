package com.platform.core.domain.monitoring;

import com.platform.core.domain.DomainException;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.AssignmentMode;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.ProfileRef;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.Request;
import com.platform.core.domain.monitoring.MonitoringProfileSelection.Result;
import org.junit.jupiter.api.Test;

import java.util.UUID;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotSame;
import static org.junit.jupiter.api.Assertions.assertThrows;

class MonitoringProfileSelectionTest {

    private static final UUID TENANT = UUID.fromString("11111111-1111-1111-1111-111111111111");
    private static final UUID OTHER_TENANT = UUID.fromString("22222222-2222-2222-2222-222222222222");

    @Test
    void automaticUsesCallerSuppliedSuggestedProfileAndVersion() {
        ProfileRef suggested = new ProfileRef(TENANT, "STANDARD", 3);
        Result result = MonitoringProfileSelection.select(new Request(
                TENANT,
                AssignmentMode.AUTO_FROM_CRITICALITY,
                suggested,
                null));

        assertEquals(AssignmentMode.AUTO_FROM_CRITICALITY, result.assignmentMode());
        assertEquals(suggested, result.suggestedProfile());
        assertEquals(suggested, result.effectiveProfile());
        assertEquals(3, result.effectiveProfile().profileVersionNo());
    }

    @Test
    void explicitPreservesPinnedProfileAndVersion() {
        ProfileRef suggested = new ProfileRef(TENANT, "STANDARD", 1);
        ProfileRef explicit = new ProfileRef(TENANT, "CUSTOM_EDGE", 7);
        Result result = MonitoringProfileSelection.select(new Request(
                TENANT,
                AssignmentMode.EXPLICIT_PROFILE,
                suggested,
                explicit));

        assertEquals(AssignmentMode.EXPLICIT_PROFILE, result.assignmentMode());
        assertEquals(suggested, result.suggestedProfile());
        assertEquals(explicit, result.effectiveProfile());
        assertEquals(7, result.effectiveProfile().profileVersionNo());
    }

    @Test
    void criticalitySuggestionChangeDoesNotReplaceExplicitEffectiveProfile() {
        ProfileRef pinned = new ProfileRef(TENANT, "CUSTOM_EDGE", 7);
        ProfileRef beforeSuggestion = new ProfileRef(TENANT, "STANDARD", 1);
        ProfileRef afterSuggestion = new ProfileRef(TENANT, "CRITICAL", 2);

        Result before = MonitoringProfileSelection.select(new Request(
                TENANT, AssignmentMode.EXPLICIT_PROFILE, beforeSuggestion, pinned));
        Result after = MonitoringProfileSelection.select(new Request(
                TENANT, AssignmentMode.EXPLICIT_PROFILE, afterSuggestion, pinned));

        assertEquals(beforeSuggestion, before.suggestedProfile());
        assertEquals(afterSuggestion, after.suggestedProfile());
        assertEquals(pinned, before.effectiveProfile());
        assertEquals(pinned, after.effectiveProfile());
        assertEquals(before.effectiveProfile(), after.effectiveProfile());
    }

    @Test
    void rejectsMissingSelectedReference() {
        DomainException autoMissing = assertThrows(DomainException.class,
                () -> MonitoringProfileSelection.select(new Request(
                        TENANT, AssignmentMode.AUTO_FROM_CRITICALITY, null, null)));
        assertEquals(MonitoringProfileSelection.ERR_VALIDATION, autoMissing.getCode());
        assertEquals("required", autoMissing.getFieldErrors().get("suggestedProfile"));

        DomainException explicitMissing = assertThrows(DomainException.class,
                () -> MonitoringProfileSelection.select(new Request(
                        TENANT,
                        AssignmentMode.EXPLICIT_PROFILE,
                        new ProfileRef(TENANT, "STANDARD", 1),
                        null)));
        assertEquals(MonitoringProfileSelection.ERR_VALIDATION, explicitMissing.getCode());
        assertEquals("required", explicitMissing.getFieldErrors().get("explicitProfile"));

        DomainException blankId = assertThrows(DomainException.class,
                () -> MonitoringProfileSelection.select(new Request(
                        TENANT,
                        AssignmentMode.AUTO_FROM_CRITICALITY,
                        new ProfileRef(TENANT, "   ", 1),
                        null)));
        assertEquals("required", blankId.getFieldErrors().get("suggestedProfile"));
    }

    @Test
    void rejectsCrossTenantProfileReference() {
        DomainException ex = assertThrows(DomainException.class,
                () -> MonitoringProfileSelection.select(new Request(
                        TENANT,
                        AssignmentMode.AUTO_FROM_CRITICALITY,
                        new ProfileRef(OTHER_TENANT, "STANDARD", 1),
                        null)));
        assertEquals(MonitoringProfileSelection.ERR_VALIDATION, ex.getCode());
        assertEquals("crossTenant", ex.getFieldErrors().get("suggestedProfile"));
    }

    @Test
    void autoRejectsCrossTenantUnusedExplicitReference() {
        DomainException ex = assertThrows(DomainException.class,
                () -> MonitoringProfileSelection.select(new Request(
                        TENANT,
                        AssignmentMode.AUTO_FROM_CRITICALITY,
                        new ProfileRef(TENANT, "STANDARD", 1),
                        new ProfileRef(OTHER_TENANT, "CUSTOM_EDGE", 2))));
        assertEquals(MonitoringProfileSelection.ERR_VALIDATION, ex.getCode());
        assertEquals("crossTenant", ex.getFieldErrors().get("explicitProfile"));
    }

    @Test
    void explicitRejectsCrossTenantPinnedReference() {
        DomainException ex = assertThrows(DomainException.class,
                () -> MonitoringProfileSelection.select(new Request(
                        TENANT,
                        AssignmentMode.EXPLICIT_PROFILE,
                        new ProfileRef(TENANT, "STANDARD", 1),
                        new ProfileRef(OTHER_TENANT, "CUSTOM_EDGE", 7))));
        assertEquals(MonitoringProfileSelection.ERR_VALIDATION, ex.getCode());
        assertEquals("crossTenant", ex.getFieldErrors().get("explicitProfile"));
    }

    @Test
    void autoAcceptsValidUnusedExplicitAndSelectsSuggested() {
        ProfileRef suggested = new ProfileRef(TENANT, "STANDARD", 3);
        ProfileRef unusedExplicit = new ProfileRef(TENANT, "CUSTOM_EDGE", 7);
        Result result = MonitoringProfileSelection.select(new Request(
                TENANT,
                AssignmentMode.AUTO_FROM_CRITICALITY,
                suggested,
                unusedExplicit));

        assertEquals(AssignmentMode.AUTO_FROM_CRITICALITY, result.assignmentMode());
        assertEquals(suggested, result.suggestedProfile());
        assertEquals(suggested, result.effectiveProfile());
        assertEquals(3, result.effectiveProfile().profileVersionNo());
        assertEquals("STANDARD", result.effectiveProfile().profileId());
    }

    @Test
    void autoRejectsInvalidUnusedExplicitReference() {
        DomainException blankId = assertThrows(DomainException.class,
                () -> MonitoringProfileSelection.select(new Request(
                        TENANT,
                        AssignmentMode.AUTO_FROM_CRITICALITY,
                        new ProfileRef(TENANT, "STANDARD", 1),
                        new ProfileRef(TENANT, "  ", 2))));
        assertEquals(MonitoringProfileSelection.ERR_VALIDATION, blankId.getCode());
        assertEquals("required", blankId.getFieldErrors().get("explicitProfile"));

        DomainException invalidVersion = assertThrows(DomainException.class,
                () -> MonitoringProfileSelection.select(new Request(
                        TENANT,
                        AssignmentMode.AUTO_FROM_CRITICALITY,
                        new ProfileRef(TENANT, "STANDARD", 1),
                        new ProfileRef(TENANT, "CUSTOM_EDGE", 0))));
        assertEquals(MonitoringProfileSelection.ERR_VALIDATION, invalidVersion.getCode());
        assertEquals("invalidVersion", invalidVersion.getFieldErrors().get("explicitProfile"));
    }

    @Test
    void rejectsInvalidProfileVersion() {
        DomainException zero = assertThrows(DomainException.class,
                () -> MonitoringProfileSelection.select(new Request(
                        TENANT,
                        AssignmentMode.AUTO_FROM_CRITICALITY,
                        new ProfileRef(TENANT, "STANDARD", 0),
                        null)));
        assertEquals("invalidVersion", zero.getFieldErrors().get("suggestedProfile"));

        DomainException negative = assertThrows(DomainException.class,
                () -> MonitoringProfileSelection.select(new Request(
                        TENANT,
                        AssignmentMode.EXPLICIT_PROFILE,
                        new ProfileRef(TENANT, "STANDARD", 1),
                        new ProfileRef(TENANT, "CUSTOM_EDGE", -1))));
        assertEquals("invalidVersion", negative.getFieldErrors().get("explicitProfile"));
    }

    @Test
    void preservesExplicitVersionWhenSuggestionVersionChanges() {
        ProfileRef pinned = new ProfileRef(TENANT, "CUSTOM_EDGE", 4);
        Result result = MonitoringProfileSelection.select(new Request(
                TENANT,
                AssignmentMode.EXPLICIT_PROFILE,
                new ProfileRef(TENANT, "CRITICAL", 9),
                pinned));

        assertEquals(4, result.effectiveProfile().profileVersionNo());
        assertEquals(9, result.suggestedProfile().profileVersionNo());
        assertEquals("CUSTOM_EDGE", result.effectiveProfile().profileId());
    }

    @Test
    void doesNotMutateRequestInputs() {
        ProfileRef suggested = new ProfileRef(TENANT, "STANDARD", 2);
        ProfileRef explicit = new ProfileRef(TENANT, "CUSTOM_EDGE", 5);
        Request request = new Request(TENANT, AssignmentMode.EXPLICIT_PROFILE, suggested, explicit);

        Result result = MonitoringProfileSelection.select(request);

        assertEquals(TENANT, request.tenantId());
        assertEquals(AssignmentMode.EXPLICIT_PROFILE, request.assignmentMode());
        assertEquals(suggested, request.suggestedProfile());
        assertEquals(explicit, request.explicitProfile());
        assertEquals(2, request.suggestedProfile().profileVersionNo());
        assertEquals(5, request.explicitProfile().profileVersionNo());

        assertNotSame(request.suggestedProfile(), result.suggestedProfile());
        assertNotSame(request.explicitProfile(), result.effectiveProfile());
        assertEquals(suggested, result.suggestedProfile());
        assertEquals(explicit, result.effectiveProfile());
    }
}
