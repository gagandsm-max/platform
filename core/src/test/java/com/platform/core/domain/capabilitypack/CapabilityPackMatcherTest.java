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
        String packA = """
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
                """;
        String packB = packA
                .replace("fixture.fictional.amb-a", "fixture.fictional.amb-b")
                .replace("\"ruleId\": \"a\"", "\"ruleId\": \"b\"")
                .replace("\"sourceReference\": \"a\"", "\"sourceReference\": \"b\"");
        CapabilityPackDescriptor a = loader.load(packA.getBytes(StandardCharsets.UTF_8));
        CapabilityPackDescriptor b = loader.load(packB.getBytes(StandardCharsets.UTF_8));

        CapabilityPackMatchResult result = CapabilityPackMatcher.match(
                List.of(a, b),
                new CapabilityPackIdentity(null, null, "Shared", null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.AMBIGUOUS, result.outcome());
        assertTrue(result.packId().isEmpty());
        assertEquals(2, result.candidates().size());
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
}
