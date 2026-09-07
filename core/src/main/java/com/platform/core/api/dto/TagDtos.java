package com.platform.core.api.dto;

import java.time.Instant;
import java.util.UUID;

public final class TagDtos {
    private TagDtos() {}

    public record CreateTagRequest(String key, String value) {}

    public record TagResponse(
            UUID tagId,
            UUID tenantId,
            String key,
            String value,
            Instant createdAt
    ) {}
}
