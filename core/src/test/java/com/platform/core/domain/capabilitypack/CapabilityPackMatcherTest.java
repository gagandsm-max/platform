package com.platform.core.domain.capabilitypack;

import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

class CapabilityPackMatcherTest {

    private final CapabilityPackLoader loader = new CapabilityPackLoader();

    @Test
    void exactMatchWinsOverGlobAtHigherPriority() {
        CapabilityPackDescriptor pack = loader.load(
                CapabilityPackTestSupport.resourceBytes("capabilitypack/examples/multi-protocol-manifest.json"));
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        registry.replaceAll(List.of(pack));

        CapabilityPackMatchResult result = registry.match(new CapabilityPackIdentity(
                "Fictional Networks", null, null, "Example 9000", null, null, null, null, null));

        assertEquals(CapabilityPackMatchOutcome.MATCH, result.outcome());
        assertEquals("example.fictional.multi-protocol", result.packId().orElseThrow());
        assertEquals("example-model-exact", result.ruleId().orElseThrow());
    }

    @Test
    void globMatchWhenExactDoesNotApply() {
        CapabilityPackDescriptor pack = loader.load(
                CapabilityPackTestSupport.resourceBytes("capabilitypack/examples/multi-protocol-manifest.json"));
        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        registry.replaceAll(List.of(pack));

        CapabilityPackMatchResult result = registry.match(new CapabilityPackIdentity(
                "Fictional Networks", null, null, "Example-ABC", null, null, null, null, null));

        assertEquals(CapabilityPackMatchOutcome.MATCH, result.outcome());
        assertEquals("example-model-family", result.ruleId().orElseThrow());
    }

    @Test
    void unknownWhenRequiredIdentityFieldMissing() {
        CapabilityPackDescriptor pack = CapabilityPackTestSupport.loadMinimal(
                "fixture.fictional.match-unknown", "1.0.0", "Fictional Device");
        CapabilityPackMatchResult result = CapabilityPackMatcher.match(
                List.of(pack),
                new CapabilityPackIdentity(null, null, null, null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.UNKNOWN, result.outcome());
    }

    @Test
    void noMatchWhenIdentityPresentButDifferent() {
        CapabilityPackDescriptor pack = CapabilityPackTestSupport.loadMinimal(
                "fixture.fictional.match-nomatch", "1.0.0", "Fictional Device");
        CapabilityPackMatchResult result = CapabilityPackMatcher.match(
                List.of(pack),
                new CapabilityPackIdentity(null, null, "Other Device", null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.NO_MATCH, result.outcome());
    }

    @Test
    void ambiguousWhenEqualStrengthAcrossPacks() {
        CapabilityPackDescriptor a = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.amb-a",
                  "packVersion": "1.0.0",
                  "name": "Amb A",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "a", "priority": 10, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Shared", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "a", "redistribution": "RESTRICTED"}]
                }
                """);
        CapabilityPackDescriptor b = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.amb-b",
                  "packVersion": "1.0.0",
                  "name": "Amb B",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "b", "priority": 10, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Shared", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "b", "redistribution": "RESTRICTED"}]
                }
                """);

        CapabilityPackMatchResult result = CapabilityPackMatcher.match(
                List.of(a, b),
                new CapabilityPackIdentity(null, null, "Shared", null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.AMBIGUOUS, result.outcome());
        assertTrue(result.packId().isEmpty());
        assertEquals(2, result.candidates().size());
    }

    @Test
    void strongerUnknownOutranksWeakerMatch() {
        CapabilityPackDescriptor unknownPack = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.unk-strong",
                  "packVersion": "1.0.0",
                  "name": "Unknown Strong",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "needs-model", "priority": 5, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Shared", "caseSensitive": false},
                      {"field": "model", "operator": "EXACT", "value": "X1", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "u", "redistribution": "RESTRICTED"}]
                }
                """);
        CapabilityPackDescriptor matchPack = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.match-weak",
                  "packVersion": "1.0.0",
                  "name": "Match Weak",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "product-only", "priority": 20, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Shared", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "m", "redistribution": "RESTRICTED"}]
                }
                """);

        CapabilityPackMatchResult result = CapabilityPackMatcher.match(
                List.of(unknownPack, matchPack),
                new CapabilityPackIdentity(null, null, "Shared", null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.UNKNOWN, result.outcome());
        assertTrue(result.packId().isEmpty());
    }

    @Test
    void equalRankUnknownTiesMatchYieldsUnknown() {
        CapabilityPackDescriptor unknownPack = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.unk-tie",
                  "packVersion": "1.0.0",
                  "name": "Unknown Tie",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "needs-model", "priority": 10, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Shared", "caseSensitive": false},
                      {"field": "model", "operator": "EXACT", "value": "X1", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "u", "redistribution": "RESTRICTED"}]
                }
                """);
        CapabilityPackDescriptor matchPack = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.match-tie",
                  "packVersion": "1.0.0",
                  "name": "Match Tie",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "two-exact", "priority": 10, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Shared", "caseSensitive": false},
                      {"field": "vendor", "operator": "EXACT", "value": "Labs", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "m", "redistribution": "RESTRICTED"}]
                }
                """);

        CapabilityPackMatchResult result = CapabilityPackMatcher.match(
                List.of(unknownPack, matchPack),
                new CapabilityPackIdentity("Labs", null, "Shared", null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.UNKNOWN, result.outcome());
    }

    @Test
    void weakerUnknownDoesNotDisplaceStrongerMatch() {
        CapabilityPackDescriptor unknownPack = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.unk-weak",
                  "packVersion": "1.0.0",
                  "name": "Unknown Weak",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "needs-model", "priority": 30, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Shared", "caseSensitive": false},
                      {"field": "model", "operator": "EXACT", "value": "X1", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "u", "redistribution": "RESTRICTED"}]
                }
                """);
        CapabilityPackDescriptor matchPack = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.match-strong",
                  "packVersion": "1.0.0",
                  "name": "Match Strong",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "product-only", "priority": 10, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Shared", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "m", "redistribution": "RESTRICTED"}]
                }
                """);

        CapabilityPackMatchResult result = CapabilityPackMatcher.match(
                List.of(unknownPack, matchPack),
                new CapabilityPackIdentity(null, null, "Shared", null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.MATCH, result.outcome());
        assertEquals("fixture.fictional.match-strong", result.packId().orElseThrow());
    }

    @Test
    void strongerUnknownOverAmbiguousMatchesYieldsUnknown() {
        CapabilityPackDescriptor ambA = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.amb-unk-a",
                  "packVersion": "1.0.0",
                  "name": "Amb A",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "a", "priority": 20, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Shared", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "a", "redistribution": "RESTRICTED"}]
                }
                """);
        CapabilityPackDescriptor ambB = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.amb-unk-b",
                  "packVersion": "1.0.0",
                  "name": "Amb B",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "b", "priority": 20, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Shared", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "b", "redistribution": "RESTRICTED"}]
                }
                """);
        CapabilityPackDescriptor unknownStrong = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.amb-unk-strong",
                  "packVersion": "1.0.0",
                  "name": "Unknown Strong",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "needs-model", "priority": 5, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Shared", "caseSensitive": false},
                      {"field": "model", "operator": "EXACT", "value": "X1", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "u", "redistribution": "RESTRICTED"}]
                }
                """);

        CapabilityPackMatchResult result = CapabilityPackMatcher.match(
                List.of(ambA, ambB, unknownStrong),
                new CapabilityPackIdentity(null, null, "Shared", null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.UNKNOWN, result.outcome());
    }

    @Test
    void exactCountTieBreakerPrefersMoreExactConditions() {
        CapabilityPackDescriptor moreExact = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.exact-a",
                  "packVersion": "1.0.0",
                  "name": "Exact A",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "two-exact", "priority": 10, "conditions": [
                      {"field": "vendor", "operator": "EXACT", "value": "Labs", "caseSensitive": false},
                      {"field": "product", "operator": "EXACT", "value": "Router", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "a", "redistribution": "RESTRICTED"}]
                }
                """);
        CapabilityPackDescriptor fewerExact = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.exact-b",
                  "packVersion": "1.0.0",
                  "name": "Exact B",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "one-exact", "priority": 10, "conditions": [
                      {"field": "vendor", "operator": "EXACT", "value": "Labs", "caseSensitive": false},
                      {"field": "product", "operator": "GLOB", "value": "R*", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "b", "redistribution": "RESTRICTED"}]
                }
                """);

        CapabilityPackMatchResult result = CapabilityPackMatcher.match(
                List.of(fewerExact, moreExact),
                new CapabilityPackIdentity("Labs", null, "Router", null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.MATCH, result.outcome());
        assertEquals("fixture.fictional.exact-a", result.packId().orElseThrow());
        assertEquals(2, result.rankExactCount());
    }

    @Test
    void conditionCountTieBreakerPrefersMoreConditions() {
        CapabilityPackDescriptor moreConditions = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.cond-a",
                  "packVersion": "1.0.0",
                  "name": "Cond A",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "three", "priority": 10, "conditions": [
                      {"field": "vendor", "operator": "EXACT", "value": "Labs", "caseSensitive": false},
                      {"field": "product", "operator": "GLOB", "value": "R*", "caseSensitive": false},
                      {"field": "model", "operator": "GLOB", "value": "M*", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "a", "redistribution": "RESTRICTED"}]
                }
                """);
        CapabilityPackDescriptor fewerConditions = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.cond-b",
                  "packVersion": "1.0.0",
                  "name": "Cond B",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "two", "priority": 10, "conditions": [
                      {"field": "vendor", "operator": "EXACT", "value": "Labs", "caseSensitive": false},
                      {"field": "product", "operator": "GLOB", "value": "R*", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "b", "redistribution": "RESTRICTED"}]
                }
                """);

        CapabilityPackMatchResult result = CapabilityPackMatcher.match(
                List.of(fewerConditions, moreConditions),
                new CapabilityPackIdentity("Labs", null, "Router", "M1", null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.MATCH, result.outcome());
        assertEquals("fixture.fictional.cond-a", result.packId().orElseThrow());
        assertEquals(3, result.rankConditionCount());
    }

    @Test
    void caseSensitiveTrueTreatsRouterAndRouterAsDistinct() {
        CapabilityPackDescriptor pack = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.case-sensitive",
                  "packVersion": "1.0.0",
                  "name": "Case Sensitive",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "exact-case", "priority": 10, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Router", "caseSensitive": true}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "c", "redistribution": "RESTRICTED"}]
                }
                """);

        CapabilityPackMatchResult mismatch = CapabilityPackMatcher.match(
                List.of(pack),
                new CapabilityPackIdentity(null, null, "router", null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.NO_MATCH, mismatch.outcome());

        CapabilityPackMatchResult match = CapabilityPackMatcher.match(
                List.of(pack),
                new CapabilityPackIdentity(null, null, "Router", null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.MATCH, match.outcome());
    }

    @Test
    void caseSensitiveFalseTreatsRouterAndRouterAsEqual() {
        CapabilityPackDescriptor pack = loadPack("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.case-insensitive",
                  "packVersion": "1.0.0",
                  "name": "Case Insensitive",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "fold-case", "priority": 10, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "Router", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "c", "redistribution": "RESTRICTED"}]
                }
                """);

        CapabilityPackMatchResult result = CapabilityPackMatcher.match(
                List.of(pack),
                new CapabilityPackIdentity(null, null, "router", null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.MATCH, result.outcome());
    }

    @Test
    void lowerPriorityWinsThenExactCountThenConditionCount() {
        String json = """
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.rank",
                  "packVersion": "1.0.0",
                  "name": "Rank",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "loose", "priority": 20, "conditions": [
                      {"field": "vendor", "operator": "EXACT", "value": "Labs", "caseSensitive": false}
                    ]},
                    {"ruleId": "tight", "priority": 10, "conditions": [
                      {"field": "vendor", "operator": "EXACT", "value": "Labs", "caseSensitive": false},
                      {"field": "model", "operator": "GLOB", "value": "M*", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "r", "redistribution": "RESTRICTED"}]
                }
                """;
        CapabilityPackDescriptor pack = loader.load(json.getBytes(StandardCharsets.UTF_8));
        CapabilityPackMatchResult result = CapabilityPackMatcher.match(
                List.of(pack),
                new CapabilityPackIdentity("Labs", null, null, "M1", null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.MATCH, result.outcome());
        assertEquals("tight", result.ruleId().orElseThrow());
    }

    private CapabilityPackDescriptor loadPack(String json) {
        return loader.load(json.getBytes(StandardCharsets.UTF_8));
    }
}
