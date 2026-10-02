package com.platform.core.logging;

import jakarta.servlet.FilterChain;
import org.junit.jupiter.api.Test;
import org.slf4j.MDC;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;

import java.util.HashSet;
import java.util.Set;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * BUILD-005 supportability correlation stress (not ordinary unit CI default).
 * Invoked explicitly by tests/stress/run-build005-supportability-stress.sh.
 */
class CorrelationIdFilterStressTest {

    private final CorrelationIdFilter filter = new CorrelationIdFilter();

    @Test
    void twoThousandCorrelatedRequestsRemainIsolatedAndBounded() throws Exception {
        Set<String> generated = new HashSet<>();
        for (int i = 0; i < 2000; i++) {
            MockHttpServletRequest request = new MockHttpServletRequest();
            MockHttpServletResponse response = new MockHttpServletResponse();
            if (i % 5 == 0) {
                request.addHeader(CorrelationIdFilter.CORRELATION_ID_HEADER, "bad id " + i);
            } else if (i % 5 == 1) {
                request.addHeader(CorrelationIdFilter.CORRELATION_ID_HEADER, "corr-stress-" + i);
            }
            AtomicReference<String> observed = new AtomicReference<>();
            FilterChain chain = (req, res) -> observed.set(MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY));
            filter.doFilter(request, response, chain);
            String header = response.getHeader(CorrelationIdFilter.CORRELATION_ID_HEADER);
            assertTrue(header != null && !header.isBlank());
            assertTrue(header.length() <= 128);
            assertEquals(header, observed.get());
            if (i % 5 == 1) {
                assertEquals("corr-stress-" + i, header);
            } else if (i % 5 == 0) {
                assertNotEquals("bad id " + i, header);
            }
            generated.add(header);
            assertTrue(MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY) == null);
        }
        assertTrue(generated.size() > 1000, "expected many distinct correlation ids, got " + generated.size());
        System.out.println("CORR_CORE_STRESS ops=2000 distinct=" + generated.size());
    }
}
