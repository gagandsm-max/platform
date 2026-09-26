package com.platform.core.control;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ControlGenerationLimitsTest {

    @Test
    void maxSafeConstantMatchesFrozenBuild000Ceiling() {
        assertEquals(9_007_199_254_740_991L, ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION);
        assertEquals(9007199254740991L, ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION);
        assertEquals("ERR_CONTROL_GENERATION_EXHAUSTED", ControlGenerationLimits.ERR_CONTROL_GENERATION_EXHAUSTED);
    }

    @Test
    void jsonSafeAndPositiveBounds() {
        assertTrue(ControlGenerationLimits.isJsonSafe(0L));
        assertTrue(ControlGenerationLimits.isJsonSafe(ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION));
        assertFalse(ControlGenerationLimits.isJsonSafe(-1L));
        assertFalse(ControlGenerationLimits.isJsonSafe(ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION + 1L));

        assertFalse(ControlGenerationLimits.isPositiveJsonSafe(0L));
        assertTrue(ControlGenerationLimits.isPositiveJsonSafe(1L));
        assertTrue(ControlGenerationLimits.isPositiveJsonSafe(ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION));
        assertFalse(ControlGenerationLimits.isPositiveJsonSafe(ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION + 1L));
    }

    @Test
    void canIncrementAndNextOrThrow() {
        assertTrue(ControlGenerationLimits.canIncrement(0L));
        assertTrue(ControlGenerationLimits.canIncrement(ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION - 1L));
        assertFalse(ControlGenerationLimits.canIncrement(ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION));
        assertFalse(ControlGenerationLimits.canIncrement(-1L));

        assertEquals(
                ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION,
                ControlGenerationLimits.nextOrThrow(
                        ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION - 1L,
                        new IllegalStateException("unused")));

        IllegalStateException exhausted = assertThrows(
                IllegalStateException.class,
                () -> ControlGenerationLimits.nextOrThrow(
                        ControlGenerationLimits.MAX_SAFE_CONTROL_GENERATION,
                        new IllegalStateException("exhausted")));
        assertEquals("exhausted", exhausted.getMessage());
    }
}
