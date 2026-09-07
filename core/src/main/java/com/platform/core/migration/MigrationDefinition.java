package com.platform.core.migration;

public record MigrationDefinition(
        String version,
        String description,
        String resourcePath,
        String checksum
) {
}
