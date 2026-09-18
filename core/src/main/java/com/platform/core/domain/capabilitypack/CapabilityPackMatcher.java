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
 * (3) greater total conditions. Lexical pack identity is diagnostic-only and never a semantic winner.
 *
 * <p>Ranked {@code UNKNOWN} candidates are retained. An unresolved UNKNOWN that can outrank or tie
 * the strongest actual MATCH forces {@link CapabilityPackMatchOutcome#UNKNOWN} rather than an
 * arbitrary MATCH/AMBIGUOUS winner.
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

        List<RankedCandidate> matchCandidates = new ArrayList<>();
        List<RankedCandidate> unknownCandidates = new ArrayList<>();

        for (CapabilityPackDescriptor pack : packs) {
            for (MatchRule rule : pack.matchRules()) {
                RuleEval eval = evaluateRule(rule, identity);
                RankedCandidate ranked = new RankedCandidate(
                        pack.packId(),
                        pack.packVersion(),
                        rule.ruleId(),
                        rule.priority(),
                        rule.exactCount(),
                        rule.conditions().size()
                );
                if (eval == RuleEval.UNKNOWN) {
                    unknownCandidates.add(ranked);
                } else if (eval == RuleEval.MATCH) {
                    matchCandidates.add(ranked);
                }
            }
        }

        List<CapabilityPackMatchResult.CandidateDiagnostic> diagnostics = new ArrayList<>();
        for (RankedCandidate c : matchCandidates) {
            diagnostics.add(toDiagnostic(c));
        }
        for (RankedCandidate c : unknownCandidates) {
            diagnostics.add(toDiagnostic(c));
        }

        if (matchCandidates.isEmpty()) {
            CapabilityPackMatchOutcome outcome = unknownCandidates.isEmpty()
                    ? CapabilityPackMatchOutcome.NO_MATCH
                    : CapabilityPackMatchOutcome.UNKNOWN;
            return new CapabilityPackMatchResult(
                    outcome, Optional.empty(), Optional.empty(), Optional.empty(), 0, 0, 0, List.copyOf(diagnostics));
        }

        matchCandidates.sort(DIAGNOSTIC_ORDER);
        RankedCandidate best = matchCandidates.get(0);

        // Any UNKNOWN that can outrank or tie the strongest MATCH blocks an arbitrary winner.
        for (RankedCandidate unknown : unknownCandidates) {
            if (compareRank(unknown, best) <= 0) {
                return new CapabilityPackMatchResult(
                        CapabilityPackMatchOutcome.UNKNOWN,
                        Optional.empty(),
                        Optional.empty(),
                        Optional.empty(),
                        best.priority,
                        best.exactCount,
                        best.conditionCount,
                        List.copyOf(diagnostics)
                );
            }
        }

        boolean ambiguous = false;
        for (int i = 1; i < matchCandidates.size(); i++) {
            RankedCandidate other = matchCandidates.get(i);
            if (sameRank(best, other)) {
                ambiguous = true;
            } else {
                break;
            }
        }

        if (ambiguous) {
            List<CapabilityPackMatchResult.CandidateDiagnostic> tied = matchCandidates.stream()
                    .filter(c -> sameRank(best, c))
                    .map(CapabilityPackMatcher::toDiagnostic)
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
                List.copyOf(diagnostics)
        );
    }

    /** Semantic rank only: priority asc, EXACT count desc, condition count desc. */
    private static int compareRank(RankedCandidate a, RankedCandidate b) {
        int byPriority = Integer.compare(a.priority, b.priority);
        if (byPriority != 0) {
            return byPriority;
        }
        int byExact = Integer.compare(b.exactCount, a.exactCount);
        if (byExact != 0) {
            return byExact;
        }
        return Integer.compare(b.conditionCount, a.conditionCount);
    }

    /** Diagnostic sort: semantic rank, then lexical identity for stable listing only. */
    private static final Comparator<RankedCandidate> DIAGNOSTIC_ORDER = Comparator
            .comparingInt((RankedCandidate c) -> c.priority)
            .thenComparing(Comparator.comparingInt((RankedCandidate c) -> c.exactCount).reversed())
            .thenComparing(Comparator.comparingInt((RankedCandidate c) -> c.conditionCount).reversed())
            .thenComparing(c -> c.packId)
            .thenComparing(c -> c.packVersion)
            .thenComparing(c -> c.ruleId);

    private static boolean sameRank(RankedCandidate a, RankedCandidate b) {
        return compareRank(a, b) == 0;
    }

    private static CapabilityPackMatchResult.CandidateDiagnostic toDiagnostic(RankedCandidate c) {
        return new CapabilityPackMatchResult.CandidateDiagnostic(
                c.packId, c.packVersion, c.ruleId, c.priority, c.exactCount, c.conditionCount);
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
