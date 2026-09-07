package com.platform.core.api;

import com.platform.core.api.PageResult;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class PageResultTest {
    @Test
    void enforcesDefaultAndMaxLimits() {
        assertEquals(50, PageResult.normalizeLimit(null));
        assertEquals(200, PageResult.normalizeLimit(200));
        assertThrows(com.platform.core.domain.DomainException.class, () -> PageResult.normalizeLimit(0));
        assertThrows(com.platform.core.domain.DomainException.class, () -> PageResult.normalizeLimit(201));
        assertThrows(com.platform.core.domain.DomainException.class, () -> PageResult.normalizePage(0));
    }
}
