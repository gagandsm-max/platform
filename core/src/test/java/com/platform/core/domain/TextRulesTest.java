package com.platform.core.domain;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;

class TextRulesTest {

    @Test
    void nameBoundsEnforcedAtExactLimits() {
        String exact255 = "a".repeat(TextRules.NAME_MAX);
        assertEquals(exact255, TextRules.requireTrimmed(exact255, "name", TextRules.NAME_MAX, true));

        String over255 = "a".repeat(TextRules.NAME_MAX + 1);
        DomainException ex = assertThrows(DomainException.class,
                () -> TextRules.requireTrimmed(over255, "name", TextRules.NAME_MAX, true));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void descriptionBoundsEnforcedAtExactLimits() {
        String exact2000 = "d".repeat(TextRules.DESCRIPTION_MAX);
        assertEquals(exact2000, TextRules.requireTrimmed(exact2000, "description", TextRules.DESCRIPTION_MAX, false));

        String over2000 = "d".repeat(TextRules.DESCRIPTION_MAX + 1);
        DomainException ex = assertThrows(DomainException.class,
                () -> TextRules.requireTrimmed(over2000, "description", TextRules.DESCRIPTION_MAX, false));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void typeBoundsEnforcedAtExactLimits() {
        String exact64 = "t".repeat(TextRules.TYPE_MAX);
        assertEquals(exact64, TextRules.requireTrimmed(exact64, "type", TextRules.TYPE_MAX, true));

        String over64 = "t".repeat(TextRules.TYPE_MAX + 1);
        DomainException ex = assertThrows(DomainException.class,
                () -> TextRules.requireTrimmed(over64, "type", TextRules.TYPE_MAX, true));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void namespaceBoundsEnforcedAtExactLimits() {
        String exact128 = "n".repeat(TextRules.NAMESPACE_MAX);
        assertEquals(exact128, TextRules.requireTrimmed(exact128, "namespace", TextRules.NAMESPACE_MAX, true));

        String over128 = "n".repeat(TextRules.NAMESPACE_MAX + 1);
        DomainException ex = assertThrows(DomainException.class,
                () -> TextRules.requireTrimmed(over128, "namespace", TextRules.NAMESPACE_MAX, true));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void identifierValueBoundsEnforcedAtExactLimits() {
        String exact512 = "v".repeat(TextRules.IDENTIFIER_VALUE_MAX);
        assertEquals(exact512, TextRules.requireTrimmed(exact512, "identifierValue", TextRules.IDENTIFIER_VALUE_MAX, true));

        String over512 = "v".repeat(TextRules.IDENTIFIER_VALUE_MAX + 1);
        DomainException ex = assertThrows(DomainException.class,
                () -> TextRules.requireTrimmed(over512, "identifierValue", TextRules.IDENTIFIER_VALUE_MAX, true));
        assertEquals("VALIDATION_FAILED", ex.getCode());
    }

    @Test
    void tagBoundsEnforcedAtExactLimits() {
        String exactKey128 = "k".repeat(TextRules.TAG_KEY_MAX);
        assertEquals(exactKey128, TextRules.requireTrimmed(exactKey128, "tagKey", TextRules.TAG_KEY_MAX, true));

        String overKey128 = "k".repeat(TextRules.TAG_KEY_MAX + 1);
        assertThrows(DomainException.class,
                () -> TextRules.requireTrimmed(overKey128, "tagKey", TextRules.TAG_KEY_MAX, true));

        String exactVal512 = "v".repeat(TextRules.TAG_VALUE_MAX);
        assertEquals(exactVal512, TextRules.requireTrimmed(exactVal512, "tagValue", TextRules.TAG_VALUE_MAX, false));

        String overVal512 = "v".repeat(TextRules.TAG_VALUE_MAX + 1);
        assertThrows(DomainException.class,
                () -> TextRules.requireTrimmed(overVal512, "tagValue", TextRules.TAG_VALUE_MAX, false));
    }

    @Test
    void timezoneValidationAcceptsValidAndRejectsInvalid() {
        assertEquals("UTC", TextRules.requireTimezone("UTC"));
        assertEquals("America/New_York", TextRules.requireTimezone("America/New_York"));
        assertEquals("Asia/Kolkata", TextRules.requireTimezone("  Asia/Kolkata  "));

        assertThrows(DomainException.class, () -> TextRules.requireTimezone("Not/A_Real_Timezone"));
        assertThrows(DomainException.class, () -> TextRules.requireTimezone(""));
        assertThrows(DomainException.class, () -> TextRules.requireTimezone(null));
        assertNull(TextRules.optionalTimezone(null));
        assertNull(TextRules.optionalTimezone("   "));
        assertEquals("UTC", TextRules.optionalTimezone("UTC"));
    }

    @Test
    void controlCharactersRejected() {
        assertThrows(DomainException.class, () -> TextRules.requireTrimmed("bad\u0000name", "name", 255, true));
        assertThrows(DomainException.class, () -> TextRules.requireTrimmed("bad\u0007bell", "name", 255, true));
        assertThrows(DomainException.class, () -> TextRules.requireTrimmed("bad\u001Funit", "name", 255, true));
        assertThrows(DomainException.class, () -> TextRules.requireTrimmed("bad\u007Fdel", "name", 255, true));
    }

    @Test
    void whitespaceTrimmingAndRequiredChecks() {
        assertEquals("clean", TextRules.requireTrimmed("  clean  ", "test", 255, true));
        assertNull(TextRules.requireTrimmed(null, "test", 255, false));
        assertNull(TextRules.requireTrimmed("   ", "test", 255, false));
        assertThrows(DomainException.class, () -> TextRules.requireTrimmed(null, "test", 255, true));
        assertThrows(DomainException.class, () -> TextRules.requireTrimmed("   ", "test", 255, true));
    }
}
