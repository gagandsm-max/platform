package com.platform.core.domain.capabilitypack;

import java.io.IOException;
import java.io.InputStream;
import java.io.UncheckedIOException;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.HexFormat;
import java.util.Optional;

final class CapabilityPackTestSupport {

    private CapabilityPackTestSupport() {
    }

    static byte[] resourceBytes(String classpathResource) {
        try (InputStream in = CapabilityPackTestSupport.class.getClassLoader().getResourceAsStream(classpathResource)) {
            if (in == null) {
                throw new IllegalStateException("Missing test resource: " + classpathResource);
            }
            return in.readAllBytes();
        } catch (IOException ex) {
            throw new UncheckedIOException(ex);
        }
    }

    static String sha256(byte[] bytes) {
        try {
            return HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(bytes));
        } catch (Exception ex) {
            throw new IllegalStateException(ex);
        }
    }

    static byte[] minimalManifestBytes(String packId, String packVersion, String product) {
        String json = """
                {
                  "schemaVersion": "1.0.0",
                  "packId": "%s",
                  "packVersion": "%s",
                  "name": "Fictional Test Pack",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional Publisher"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {
                      "ruleId": "primary",
                      "priority": 10,
                      "conditions": [
                        {"field": "product", "operator": "EXACT", "value": "%s", "caseSensitive": false}
                      ]
                    }
                  ],
                  "capabilities": [],
                  "protocols": [],
                  "references": [],
                  "normalization": [],
                  "dependencies": [],
                  "provenance": [
                    {
                      "id": "fixture-source",
                      "sourceType": "CUSTOMER_SUPPLIED",
                      "sourceReference": "fictional-unit-test",
                      "redistribution": "RESTRICTED"
                    }
                  ]
                }
                """.formatted(packId, packVersion, product);
        return json.getBytes(StandardCharsets.UTF_8);
    }

    static CapabilityPackDescriptor loadMinimal(String packId, String packVersion, String product) {
        return new CapabilityPackLoader().load(minimalManifestBytes(packId, packVersion, product));
    }

    static CapabilityPackDescriptor withDependency(
            String packId, String packVersion, String product, String depPackId, String depVersion) {
        String json = """
                {
                  "schemaVersion": "1.0.0",
                  "packId": "%s",
                  "packVersion": "%s",
                  "name": "Fictional Dependent Pack",
                  "publisher": {"type": "CUSTOMER", "name": "Fictional Publisher"},
                  "certification": "EXPERIMENTAL",
                  "matchRules": [
                    {
                      "ruleId": "primary",
                      "priority": 10,
                      "conditions": [
                        {"field": "product", "operator": "EXACT", "value": "%s", "caseSensitive": false}
                      ]
                    }
                  ],
                  "capabilities": [],
                  "protocols": [],
                  "references": [],
                  "normalization": [],
                  "dependencies": [{"packId": "%s", "packVersion": "%s"}],
                  "provenance": [
                    {
                      "id": "fixture-source",
                      "sourceType": "CUSTOMER_SUPPLIED",
                      "sourceReference": "fictional-unit-test",
                      "redistribution": "RESTRICTED"
                    }
                  ]
                }
                """.formatted(packId, packVersion, product, depPackId, depVersion);
        return new CapabilityPackLoader().load(json.getBytes(StandardCharsets.UTF_8));
    }

    static CapabilityPackDescriptor unsupportedProfileDescriptor() {
        // Construct via loader then wrap is not possible for other classes; build a synthetic descriptor.
        CapabilityPackDescriptor base = loadMinimal("fixture.fictional.unsupported", "1.0.0", "Unsupported");
        return new CapabilityPackDescriptor(
                base.packId(),
                base.packVersion(),
                CapabilityPackClass.SAAS_MONITORING,
                base.schemaVersion(),
                base.name(),
                base.publisher(),
                base.certification(),
                base.metadata(),
                base.matchRules(),
                base.capabilities(),
                base.protocols(),
                base.references(),
                base.normalization(),
                base.dependencies(),
                base.provenance(),
                base.manifestSha256()
        );
    }

    static Optional<String> empty() {
        return Optional.empty();
    }
}
