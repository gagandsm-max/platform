package com.platform.core.domain.capabilitypack;

import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;
import java.util.Arrays;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class CapabilityPackLoaderTest {

    private final CapabilityPackLoader loader = new CapabilityPackLoader();

    @Test
    void loadsValidMinimalManifestAndRecordsSha256() {
        byte[] bytes = CapabilityPackTestSupport.resourceBytes("capabilitypack/examples/minimal-manifest.json");
        CapabilityPackDescriptor descriptor = loader.load(bytes);

        assertEquals("example.fictional.minimal", descriptor.packId());
        assertEquals("1.0.0", descriptor.packVersion());
        assertEquals(CapabilityPackClass.DEVICE_NETWORK, descriptor.packClass());
        assertEquals("1.0.0", descriptor.schemaVersion());
        assertFalse(descriptor.platformVersionRangePresent());
        assertTrue(descriptor.platformVersionRange().isEmpty());
        assertEquals(CapabilityPackTestSupport.sha256(bytes), descriptor.manifestSha256());
    }

    @Test
    void rejectsManifestOneByteOverMaxBound() {
        byte[] base = CapabilityPackTestSupport.minimalManifestBytes(
                "fixture.fictional.oversized", "1.0.0", "Fictional");
        assertTrue(base.length < CapabilityPackLoader.MAX_MANIFEST_BYTES);

        byte[] oversized = new byte[CapabilityPackLoader.MAX_MANIFEST_BYTES + 1];
        Arrays.fill(oversized, (byte) ' ');
        System.arraycopy(base, 0, oversized, 0, base.length);
        // Keep JSON valid-ish padding after object would fail parse; oversize checked first.
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class, () -> loader.load(oversized));
        assertEquals("MANIFEST_TOO_LARGE", ex.getCode());
    }

    @Test
    void acceptsManifestAtExactMaxBoundWhenValidJsonFits() {
        // Build a valid manifest padded via a long description within STRING_BOUNDS (512),
        // then verify the constant boundary using a crafted payload just under the limit that still parses.
        byte[] base = CapabilityPackTestSupport.minimalManifestBytes(
                "fixture.fictional.exact-bound", "1.0.0", "Fictional");
        int pad = CapabilityPackLoader.MAX_MANIFEST_BYTES - base.length;
        assertTrue(pad > 10);
        // Exact-bound rejection is for > MAX; equal length must still parse if JSON is valid.
        // Construct whitespace-padded JSON: leading spaces + body, total == MAX.
        byte[] padded = new byte[CapabilityPackLoader.MAX_MANIFEST_BYTES];
        Arrays.fill(padded, (byte) ' ');
        System.arraycopy(base, 0, padded, pad, base.length);
        CapabilityPackDescriptor descriptor = loader.load(padded);
        assertEquals("fixture.fictional.exact-bound", descriptor.packId());
        assertEquals(CapabilityPackLoader.MAX_MANIFEST_BYTES, padded.length);
    }

    @Test
    void rejectsUnknownTopLevelField() {
        String base = new String(CapabilityPackTestSupport.minimalManifestBytes(
                "fixture.fictional.unknown-field", "1.0.0", "Fictional"), StandardCharsets.UTF_8);
        byte[] bytes = (base.substring(0, base.lastIndexOf('}')) + ",\"runtimeMagic\":\"no\"}")
                .getBytes(StandardCharsets.UTF_8);
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load(bytes));
        assertTrue(ex.getIssues().stream().anyMatch(i -> "UNKNOWN_FIELD".equals(i.code())));
    }

    @Test
    void rejectsUnsupportedSchemaVersion() {
        String json = new String(CapabilityPackTestSupport.minimalManifestBytes(
                "fixture.fictional.schema", "1.0.0", "Fictional"), StandardCharsets.UTF_8)
                .replace("\"schemaVersion\": \"1.0.0\"", "\"schemaVersion\": \"9.9.9\"");
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load(json.getBytes(StandardCharsets.UTF_8)));
        assertTrue(ex.getIssues().stream().anyMatch(i -> "UNSUPPORTED_SCHEMA_VERSION".equals(i.code())));
    }

    @Test
    void rejectsTrailingTokens() {
        byte[] body = CapabilityPackTestSupport.minimalManifestBytes(
                "fixture.fictional.trailing", "1.0.0", "Fictional");
        byte[] trailing = (new String(body, StandardCharsets.UTF_8) + " {}").getBytes(StandardCharsets.UTF_8);
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class, () -> loader.load(trailing));
        assertTrue("TRAILING_TOKENS".equals(ex.getCode()) || "JSON_PARSE_ERROR".equals(ex.getCode()));
    }

    @Test
    void rejectsDuplicateKeys() {
        String json = """
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.dup-key",
                  "packId": "fixture.fictional.dup-key-b",
                  "packVersion": "1.0.0",
                  "name": "Dup",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "r", "priority": 1, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "X", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [],
                  "protocols": [],
                  "references": [],
                  "normalization": [],
                  "dependencies": [],
                  "provenance": [
                    {"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "x", "redistribution": "RESTRICTED"}
                  ]
                }
                """;
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load(json.getBytes(StandardCharsets.UTF_8)));
        assertEquals("DUPLICATE_KEY", ex.getCode());
        assertEquals("Manifest JSON contains duplicate object keys.", ex.getMessage());
        assertTrue(ex.getIssues().stream().allMatch(i ->
                "Manifest JSON contains duplicate object keys.".equals(i.message())));
    }

    @Test
    void rejectsMalformedJson() {
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load("{not-json".getBytes(StandardCharsets.UTF_8)));
        assertEquals("JSON_PARSE_ERROR", ex.getCode());
        assertEquals("Manifest JSON is malformed.", ex.getMessage());
    }

    @Test
    void rejectsNonStringContractFields() {
        assertTrue(loadFailsWithInvalidType("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": 12345,
                  "packVersion": "1.0.0",
                  "name": "Numeric PackId",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "r", "priority": 1, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "X", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "x", "redistribution": "RESTRICTED"}]
                }
                """));

        assertTrue(loadFailsWithInvalidType("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.bool-name",
                  "packVersion": "1.0.0",
                  "name": true,
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "r", "priority": 1, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "X", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "x", "redistribution": "RESTRICTED"}]
                }
                """));

        assertTrue(loadFailsWithInvalidType("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.num-publisher",
                  "packVersion": "1.0.0",
                  "name": "Pub",
                  "publisher": {"type": "CUSTOMER", "name": 99},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "r", "priority": 1, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "X", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "x", "redistribution": "RESTRICTED"}]
                }
                """));

        assertTrue(loadFailsWithInvalidType("""
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.num-match-value",
                  "packVersion": "1.0.0",
                  "name": "Match",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "r", "priority": 1, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": 42, "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                  "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "x", "redistribution": "RESTRICTED"}]
                }
                """));
    }

    @Test
    void rejectsStringWhereBooleanOrIntegerExpected() {
        CapabilityPackValidationException boolEx = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load("""
                        {
                          "schemaVersion": "1.0.0",
                          "packId": "fixture.fictional.string-bool",
                          "packVersion": "1.0.0",
                          "name": "Bool",
                          "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                          "certification": "EXPERIMENTAL",
                          "matchRules": [
                            {"ruleId": "r", "priority": 1, "conditions": [
                              {"field": "product", "operator": "EXACT", "value": "X", "caseSensitive": "false"}
                            ]}
                          ],
                          "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                          "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "x", "redistribution": "RESTRICTED"}]
                        }
                        """.getBytes(StandardCharsets.UTF_8)));
        assertTrue(boolEx.getIssues().stream().anyMatch(i ->
                "INVALID_TYPE".equals(i.code()) && i.path().contains("caseSensitive")));

        CapabilityPackValidationException intEx = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load("""
                        {
                          "schemaVersion": "1.0.0",
                          "packId": "fixture.fictional.string-int",
                          "packVersion": "1.0.0",
                          "name": "Int",
                          "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                          "certification": "EXPERIMENTAL",
                          "matchRules": [
                            {"ruleId": "r", "priority": "1", "conditions": [
                              {"field": "product", "operator": "EXACT", "value": "X", "caseSensitive": false}
                            ]}
                          ],
                          "capabilities": [], "protocols": [], "references": [], "normalization": [], "dependencies": [],
                          "provenance": [{"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "x", "redistribution": "RESTRICTED"}]
                        }
                        """.getBytes(StandardCharsets.UTF_8)));
        assertTrue(intEx.getIssues().stream().anyMatch(i ->
                "INVALID_MATCH_PRIORITY".equals(i.code()) || "INVALID_TYPE".equals(i.code())));
    }

    private boolean loadFailsWithInvalidType(String json) {
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load(json.getBytes(StandardCharsets.UTF_8)));
        return ex.getIssues().stream().anyMatch(i ->
                "INVALID_TYPE".equals(i.code()) || "INVALID_PACK_ID".equals(i.code()));
    }
}
