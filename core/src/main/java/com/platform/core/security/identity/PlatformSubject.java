package com.platform.core.security.identity;

import java.util.Objects;
import java.util.UUID;

/** Stable Platform-owned identity used after provider identity resolution. */
public record PlatformSubject(UUID subjectId) {
    public PlatformSubject {
        Objects.requireNonNull(subjectId, "subjectId");
    }
}
