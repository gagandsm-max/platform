package com.platform.core.domain.capabilitypack;

import java.util.Objects;

/**
 * Outcome of registering an immutable tenant pack version.
 *
 * @param created {@code true} when a new row was inserted; {@code false} on same-SHA idempotent hit
 */
public record CapabilityPackRegistrationResult(StoredCapabilityPack pack, boolean created) {
    public CapabilityPackRegistrationResult {
        Objects.requireNonNull(pack, "pack");
    }
}
