package com.platform.core.api;

import com.platform.core.domain.DomainException;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;

class PageResultTest {

    @Test
    void normalizesLimitsAndPages() {
        assertEquals(50, PageResult.normalizeLimit(null));
        assertEquals(1, PageResult.normalizeLimit(1));
        assertEquals(10, PageResult.normalizeLimit(10));
        assertEquals(50, PageResult.normalizeLimit(50));
        assertEquals(200, PageResult.normalizeLimit(200));

        assertEquals(1, PageResult.normalizePage(1));
        assertEquals(10_000, PageResult.normalizePage(10_000));
        assertEquals(Integer.MAX_VALUE, PageResult.normalizePage(Integer.MAX_VALUE));

        DomainException lim0 = assertThrows(DomainException.class, () -> PageResult.normalizeLimit(0));
        assertEquals("QUERY_LIMIT_EXCEEDED", lim0.getCode());

        DomainException limNeg = assertThrows(DomainException.class, () -> PageResult.normalizeLimit(-1));
        assertEquals("QUERY_LIMIT_EXCEEDED", limNeg.getCode());

        DomainException lim201 = assertThrows(DomainException.class, () -> PageResult.normalizeLimit(201));
        assertEquals("QUERY_LIMIT_EXCEEDED", lim201.getCode());

        DomainException page0 = assertThrows(DomainException.class, () -> PageResult.normalizePage(0));
        assertEquals("QUERY_LIMIT_EXCEEDED", page0.getCode());

        DomainException pageNeg = assertThrows(DomainException.class, () -> PageResult.normalizePage(-1));
        assertEquals("QUERY_LIMIT_EXCEEDED", pageNeg.getCode());

        DomainException pageNegLarge = assertThrows(DomainException.class, () -> PageResult.normalizePage(-99));
        assertEquals("QUERY_LIMIT_EXCEEDED", pageNegLarge.getCode());
    }

    @Test
    void offsetCalculationPreventsArithmeticOverflow() {
        assertEquals(0L, PageResult.offset(1, 50));
        assertEquals(50L, PageResult.offset(2, 50));
        assertEquals(1_999_800L, PageResult.offset(10_000, 200));
        // (Integer.MAX_VALUE - 1) * 200 = 2147483646 * 200 = 429496729200
        assertEquals(429_496_729_200L, PageResult.offset(Integer.MAX_VALUE, 200));
        // Ensure result is strictly positive and fits in standard signed long
        long maxOffset = PageResult.offset(Integer.MAX_VALUE, 200);
        org.junit.jupiter.api.Assertions.assertTrue(maxOffset > 0, "Offset must never overflow to negative");
    }
}
