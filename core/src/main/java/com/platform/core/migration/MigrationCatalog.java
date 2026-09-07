package com.platform.core.migration;

import org.springframework.core.io.Resource;
import org.springframework.core.io.support.PathMatchingResourcePatternResolver;
import org.springframework.stereotype.Component;
import org.springframework.util.StreamUtils;

import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.HexFormat;
import java.util.List;
import java.util.Locale;

@Component
public class MigrationCatalog {

    public static final String MANIFEST_RESOURCE = "classpath:db/migrations/manifest.txt";
    public static final String MIGRATION_LOCATION = "classpath:db/migrations/";

    public List<MigrationDefinition> load() {
        try {
            PathMatchingResourcePatternResolver resolver = new PathMatchingResourcePatternResolver();
            Resource manifest = resolver.getResource(MANIFEST_RESOURCE);
            if (!manifest.exists()) {
                throw new MigrationException("MIGRATION_MANIFEST_MISSING", "Migration manifest is missing");
            }
            String manifestText = StreamUtils.copyToString(manifest.getInputStream(), StandardCharsets.UTF_8);
            List<MigrationDefinition> definitions = new ArrayList<>();
            for (String rawLine : manifestText.split("\n")) {
                String line = rawLine.trim();
                if (line.isEmpty() || line.startsWith("#")) {
                    continue;
                }
                String[] parts = line.split("\\|", 2);
                if (parts.length != 2) {
                    throw new MigrationException("MIGRATION_MANIFEST_INVALID", "Invalid migration manifest line");
                }
                String fileName = parts[0].trim();
                String description = parts[1].trim();
                if (!fileName.matches("^V\\d{3}__[a-z0-9_]+\\.sql$")) {
                    throw new MigrationException("MIGRATION_MANIFEST_INVALID", "Migration file name is invalid: " + fileName);
                }
                String version = fileName.substring(0, fileName.indexOf("__"));
                Resource sql = resolver.getResource(MIGRATION_LOCATION + fileName);
                if (!sql.exists()) {
                    throw new MigrationException("MIGRATION_RESOURCE_MISSING", "Migration SQL missing: " + fileName);
                }
                String body;
                try (InputStream in = sql.getInputStream()) {
                    body = StreamUtils.copyToString(in, StandardCharsets.UTF_8);
                }
                definitions.add(new MigrationDefinition(version, description, MIGRATION_LOCATION + fileName, sha256(body)));
            }
            return List.copyOf(definitions);
        } catch (IOException e) {
            throw new MigrationException("MIGRATION_CATALOG_FAILED", "Failed to load migration catalog", e);
        }
    }

    public String loadSql(MigrationDefinition definition) {
        try {
            PathMatchingResourcePatternResolver resolver = new PathMatchingResourcePatternResolver();
            Resource sql = resolver.getResource(definition.resourcePath());
            try (InputStream in = sql.getInputStream()) {
                return StreamUtils.copyToString(in, StandardCharsets.UTF_8);
            }
        } catch (IOException e) {
            throw new MigrationException("MIGRATION_RESOURCE_MISSING", "Failed to read migration SQL", e);
        }
    }

    static String sha256(String body) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] hash = digest.digest(body.getBytes(StandardCharsets.UTF_8));
            return HexFormat.of().formatHex(hash).toLowerCase(Locale.ROOT);
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException("SHA-256 not available", e);
        }
    }
}
