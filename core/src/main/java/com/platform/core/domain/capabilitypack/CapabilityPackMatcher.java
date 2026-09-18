package com.platform.core.domain.capabilitypack;

import java.util.ArrayList;
import java.util.Comparator;
import java.util.List;
import java.util.Locale;
import java.util.Optional;
import java.util.regex.Pattern;

/**
 * Deterministic Capability Pack matching.
 *
 * <p>Rank within a rule and across packs: (1) lower priority wins, (2) greater EXACT count,
 * (3) greater total conditions. Equal-strength survivors yield {@link CapabilityPackMatchOutcome#AMBIGUOUS}.
 */
public final class CapabilityPackMatcher {

    private CapabilityPackMatcher() {
    }

    public static CapabilityPackMatchResult match(List<CapabilityPackDescriptor> packs, CapabilityPackIdentity identity) {
        if (packs == null || packs.isEmpty()) {
            return new CapabilityPackMatchResult(
                    CapabilityPackMatchOutcome.NO_MATCH,
                    Optional.empty(),
                    Optional.empty(),
                    Optional.empty(),
                    0,
                    0,
                    0,
                    List.of()
            );
        }
        if (identity == null) {
            return new CapabilityPackMatchResult(
                    CapabilityPackMatchOutcome.UNKNOWN,
                    Optional.empty(),
                    Optional.empty(),
                    Optional.empty(),
                    0,
                    0,
                    0,
                    List.of()
            );
        }

        List<RankedCandidate> candidates = new ArrayList<>();
        boolean sawUnknown = false;

        for (CapabilityPackDescriptor pack : packs) {
            for (MatchRule rule : pack.matchRules()) {
                RuleEval eval = evaluateRule(rule, identity);
                if (eval == RuleEval.UNKNOWN) {
                    sawUnknown = true;
                } else if (eval == RuleEval.MATCH) {
                    candidates.add(new RankedCandidate(
                            pack.packId(),
                            pack.packVersion(),
                            rule.ruleId(),
                            rule.priority(),
                            rule.exactCount(),
                            rule.conditions().size()
                    ));
                }
            }
        }

        List<CapabilityPackMatchResult.CandidateDiagnostic> diagnostics = candidates.stream()
                .map(c -> new CapabilityPackMatchResult.CandidateDiagnostic(
                        c.packId, c.packVersion, c.ruleId, c.priority, c.exactCount, c.conditionCount))
                .toList();

        if (candidates.isEmpty()) {
            CapabilityPackMatchOutcome outcome = sawUnknown
                    ? CapabilityPackMatchOutcome.UNKNOWN
                    : CapabilityPackMatchOutcome.NO_MATCH;
            return new CapabilityPackMatchResult(
                    outcome, Optional.empty(), Optional.empty(), Optional.empty(), 0, 0, 0, diagnostics);
        }

        candidates.sort(RANK_ORDER);
        RankedCandidate best = candidates.get(0);
        boolean ambiguous = false;
        for (int i = 1; i < candidates.size(); i++) {
            RankedCandidate other = candidates.get(i);
            if (sameRank(best, other)) {
                ambiguous = true;
            } else {
                break;
            }
        }

        if (ambiguous) {
            List<CapabilityPackMatchResult.CandidateDiagnostic> tied = candidates.stream()
                    .filter(c -> sameRank(best, c))
                    .map(c -> new CapabilityPackMatchResult.CandidateDiagnostic(
                            c.packId, c.packVersion, c.ruleId, c.priority, c.exactCount, c.conditionCount))
                    .toList();
            return new CapabilityPackMatchResult(
                    CapabilityPackMatchOutcome.AMBIGUOUS,
                    Optional.empty(),
                    Optional.empty(),
                    Optional.empty(),
                    best.priority,
                    best.exactCount,
                    best.conditionCount,
                    tied
            );
        }

        return new CapabilityPackMatchResult(
                CapabilityPackMatchOutcome.MATCH,
                Optional.of(best.packId),
                Optional.of(best.packVersion),
                Optional.of(best.ruleId),
                best.priority,
                best.exactCount,
                best.conditionCount,
                diagnostics
        );
    }

    private static final Comparator<RankedCandidate> RANK_ORDER = Comparator
            .comparingInt((RankedCandidate c) -> c.priority)
            .thenComparing(Comparator.comparingInt((RankedCandidate c) -> c.exactCount).reversed())
            .thenComparing(Comparator.comparingInt((RankedCandidate c) -> c.conditionCount).reversed())
            .thenComparing(c -> c.packId)
            .thenComparing(c -> c.packVersion)
            .thenComparing(c -> c.ruleId);

    private static boolean sameRank(RankedCandidate a, RankedCandidate b) {
        return a.priority == b.priority
                && a.exactCount == b.exactCount
                && a.conditionCount == b.conditionCount;
    }

    private static RuleEval evaluateRule(MatchRule rule, CapabilityPackIdentity identity) {
        boolean unknown = false;
        for (MatchCondition condition : rule.conditions()) {
            String observed = identity.value(condition.field());
            if (observed == null) {
                unknown = true;
                continue;
            }
            if (!matches(condition, observed)) {
                return RuleEval.NO_MATCH;
            }
        }
        return unknown ? RuleEval.UNKNOWN : RuleEval.MATCH;
    }

    static boolean matches(MatchCondition condition, String observed) {
        String expected = condition.value();
        String left = condition.caseSensitive() ? observed : observed.toLowerCase(Locale.ROOT);
        String right = condition.caseSensitive() ? expected : expected.toLowerCase(Locale.ROOT);
        return switch (condition.operator()) {
            case EXACT -> left.equals(right);
            case GLOB -> globMatch(left, right);
        };
    }

    /**
     * Bounded glob: {@code *} and {@code ?} only. Pattern is treated as literal otherwise.
     */
    static boolean globMatch(String text, String pattern) {
        StringBuilder regex = new StringBuilder("^");
        for (int i = 0; i < pattern.length(); i++) {
            char c = pattern.charAt(i);
            switch (c) {
                case '*' -> regex.append(".*");
                case '?' -> regex.append('.');
                default -> regex.append(Pattern.quote(String.valueOf(c)));
            }
        }
        regex.append('$');
        return Pattern.compile(regex.toString(), Pattern.DOTALL).matcher(text).matches();
    }

    private enum RuleEval {
        MATCH,
        NO_MATCH,
        UNKNOWN
    }

    private record RankedCandidate(
            String packId,
            String packVersion,
            String ruleId,
            int priority,
            int exactCount,
            int conditionCount
    ) {
    }
}
