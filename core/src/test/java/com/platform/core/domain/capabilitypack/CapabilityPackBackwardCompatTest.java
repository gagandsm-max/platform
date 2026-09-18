package com.platform.core.domain.capabilitypack;

import org.junit.jupiter.api.Test;

import java.util.List;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Backward-compat: existing Device Pack schemaVersion 1.0.0 examples load as DEVICE_NETWORK.
 */
class CapabilityPackBackwardCompatTest {

    private final CapabilityPackLoader loader = new CapabilityPackLoader();

    @Test
    void devicePackExamplesMapOnlyToDeviceNetworkProfile() {
        CapabilityPackDescriptor minimal = loader.load(
                CapabilityPackTestSupport.resourceBytes("capabilitypack/examples/minimal-manifest.json"));
        CapabilityPackDescriptor multi = loader.load(
                CapabilityPackTestSupport.resourceBytes("capabilitypack/examples/multi-protocol-manifest.json"));

        assertEquals(CapabilityPackClass.DEVICE_NETWORK, minimal.packClass());
        assertEquals(CapabilityPackClass.DEVICE_NETWORK, multi.packClass());
        assertFalse(minimal.platformVersionRangePresent());
        assertFalse(multi.platformVersionRangePresent());

        CapabilityPackRegistry registry = new CapabilityPackRegistry();
        registry.replaceAll(List.of(minimal, multi));
        assertEquals(2, registry.size());

        CapabilityPackMatchResult result = registry.match(new CapabilityPackIdentity(
                "Other Networks", null, "Fictional Device", null, null, null, null, null, null));
        assertEquals(CapabilityPackMatchOutcome.MATCH, result.outcome());
        assertEquals("example.fictional.minimal", result.packId().orElseThrow());
        assertTrue(result.candidates().stream().noneMatch(c -> c.packId().contains("password")));
    }
}
