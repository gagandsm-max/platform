package com.platform.core.domain.capabilitypack;

import java.util.List;
import java.util.Objects;
import java.util.Optional;

/**
 * Diagnostics for a match evaluation. Contains pack/rule identifiers only — never secrets.
 */
public record CapabilityPackMatchResult(
        CapabilityPackMatchOutcome outcome,
        Optional<String> packId,
        Optional<String> packVersion,
        Optional<String> ruleId,
        int rankPriority,
        int rankExactCount,
        int rankConditionCount,
        List<CandidateDiagnostic> candidates
) {
    public CapabilityPackMatchResult {
        Objects.requireNonNull(outcome, "outcome");
        packId = packId == null ? Optional.empty() : packId;
        packVersion = packVersion == null ? Optional.empty() : packVersion;
        ruleId = ruleId == null ? Optional.empty() : ruleId;
        candidates = candidates == null ? List.of() : List.copyOf(candidates);
    }

    public record CandidateDiagnostic(
            String packId,
            String packVersion,
            String ruleId,
            int priority,
            int exactCount,
            int conditionCount
    ) {
        public CandidateDiagnostic {
            Objects.requireNonNull(packId, "packId");
            Objects.requireNonNull(packVersion, "packVersion");
            Objects.requireNonNull(ruleId, "ruleId");
        }
    }
}
