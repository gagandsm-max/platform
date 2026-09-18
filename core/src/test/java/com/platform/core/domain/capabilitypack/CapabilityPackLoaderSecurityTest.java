package com.platform.core.domain.capabilitypack;

import org.junit.jupiter.api.Test;

import java.nio.charset.StandardCharsets;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class CapabilityPackLoaderSecurityTest {

    private final CapabilityPackLoader loader = new CapabilityPackLoader();

    @Test
    void rejectsSecretFieldAndPrivateKeyMaterial() {
        String json = """
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.secrets",
                  "packVersion": "1.0.0",
                  "name": "Secret Fixture",
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
                  ],
                  "password": "fictional-placeholder",
                  "privateKey": "-----BEGIN PRIVATE KEY----- fictional -----END PRIVATE KEY-----"
                }
                """;
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load(json.getBytes(StandardCharsets.UTF_8)));
        assertTrue(ex.getIssues().stream().anyMatch(i -> "FORBIDDEN_SECRET_FIELD".equals(i.code())));
        assertTrue(ex.getIssues().stream().anyMatch(i -> "FORBIDDEN_SECRET_VALUE".equals(i.code())));
    }

    @Test
    void rejectsPathTraversalAndAbsoluteReference() {
        String json = """
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.paths",
                  "packVersion": "1.0.0",
                  "name": "Path Fixture",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "r", "priority": 1, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "X", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [],
                  "protocols": [],
                  "references": [
                    {"id": "unsafe", "kind": "MIB", "packagePath": "../fictional.mib", "provenanceId": "p"}
                  ],
                  "normalization": [],
                  "dependencies": [],
                  "provenance": [
                    {"id": "p", "sourceType": "CUSTOMER_SUPPLIED",
                     "sourceReference": "/fictional/absolute", "redistribution": "RESTRICTED"}
                  ]
                }
                """;
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load(json.getBytes(StandardCharsets.UTF_8)));
        assertTrue(ex.getIssues().stream().anyMatch(i -> "PATH_TRAVERSAL".equals(i.code())));
        assertTrue(ex.getIssues().stream().anyMatch(i -> "ABSOLUTE_FILESYSTEM_REFERENCE".equals(i.code())));
    }

    @Test
    void rejectsEncodedTraversalAndCredentialUrl() {
        String json = """
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.encoded",
                  "packVersion": "1.0.0",
                  "name": "Encoded Fixture",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "r", "priority": 1, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "X", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [],
                  "protocols": [],
                  "references": [
                    {"id": "unsafe", "kind": "MIB", "packagePath": "refs/%2e%2e/secret.mib", "provenanceId": "p"}
                  ],
                  "normalization": [],
                  "dependencies": [],
                  "provenance": [
                    {"id": "p", "sourceType": "CUSTOMER_SUPPLIED",
                     "sourceReference": "https://user:pass@example.invalid/path", "redistribution": "RESTRICTED"}
                  ]
                }
                """;
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load(json.getBytes(StandardCharsets.UTF_8)));
        assertTrue(ex.getIssues().stream().anyMatch(i -> "PATH_TRAVERSAL".equals(i.code())));
        assertTrue(ex.getIssues().stream().anyMatch(i -> "EMBEDDED_URL_CREDENTIAL".equals(i.code())));
    }

    @Test
    void rejectsExecutableFields() {
        String json = """
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.exec",
                  "packVersion": "1.0.0",
                  "name": "Exec Fixture",
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
                  ],
                  "script": "fictional",
                  "command": "fictional"
                }
                """;
        CapabilityPackValidationException ex = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load(json.getBytes(StandardCharsets.UTF_8)));
        assertTrue(ex.getIssues().stream().anyMatch(i -> "FORBIDDEN_EXECUTABLE_FIELD".equals(i.code())));
    }

    @Test
    void rejectsLegacyProtocolFalselyNonLegacyAndRequiredLegacy() {
        String missingLegacyFlag = """
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.legacy-flag",
                  "packVersion": "1.0.0",
                  "name": "Legacy Flag",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "r", "priority": 1, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "X", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [],
                  "protocols": [{"family": "TELNET", "requirement": "OPTIONAL", "legacy": false}],
                  "references": [],
                  "normalization": [],
                  "dependencies": [],
                  "provenance": [
                    {"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "x", "redistribution": "RESTRICTED"}
                  ]
                }
                """;
        CapabilityPackValidationException ex1 = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load(missingLegacyFlag.getBytes(StandardCharsets.UTF_8)));
        assertTrue(ex1.getIssues().stream().anyMatch(i -> "LEGACY_PROTOCOL_NOT_FLAGGED".equals(i.code())));

        String requiredLegacy = """
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.legacy-req",
                  "packVersion": "1.0.0",
                  "name": "Legacy Required",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "r", "priority": 1, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "X", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [],
                  "protocols": [{"family": "FTP", "requirement": "REQUIRED", "legacy": true}],
                  "references": [],
                  "normalization": [],
                  "dependencies": [],
                  "provenance": [
                    {"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "x", "redistribution": "RESTRICTED"}
                  ]
                }
                """;
        CapabilityPackValidationException ex2 = assertThrows(
                CapabilityPackValidationException.class,
                () -> loader.load(requiredLegacy.getBytes(StandardCharsets.UTF_8)));
        assertTrue(ex2.getIssues().stream().anyMatch(i -> "LEGACY_PROTOCOL_CANNOT_BE_REQUIRED".equals(i.code())));
    }

    @Test
    void acceptsOptionalLegacyWhenExplicitlyFlagged() {
        String json = """
                {
                  "schemaVersion": "1.0.0",
                  "packId": "fixture.fictional.valid-legacy",
                  "packVersion": "1.0.0",
                  "name": "Valid Legacy",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {"ruleId": "r", "priority": 1, "conditions": [
                      {"field": "product", "operator": "EXACT", "value": "X", "caseSensitive": false}
                    ]}
                  ],
                  "capabilities": [],
                  "protocols": [{"family": "TELNET", "requirement": "OPTIONAL", "legacy": true, "transportModes": ["TCP"]}],
                  "references": [],
                  "normalization": [],
                  "dependencies": [],
                  "provenance": [
                    {"id": "p", "sourceType": "CUSTOMER_SUPPLIED", "sourceReference": "x", "redistribution": "RESTRICTED"}
                  ]
                }
                """;
        CapabilityPackDescriptor descriptor = loader.load(json.getBytes(StandardCharsets.UTF_8));
        assertEquals(1, descriptor.protocols().size());
        assertTrue(descriptor.protocols().get(0).legacy());
    }
}
