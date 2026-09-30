package com.platform.core.logging;

import jakarta.servlet.ServletException;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;
import org.slf4j.MDC;
import org.springframework.mock.web.MockFilterChain;
import org.springframework.mock.web.MockHttpServletRequest;
import org.springframework.mock.web.MockHttpServletResponse;

import java.io.IOException;
import java.util.concurrent.atomic.AtomicReference;
import java.util.regex.Pattern;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;

class CorrelationIdFilterTest {

    private static final Pattern SAFE = Pattern.compile("^[A-Za-z0-9._:-]{1,128}$");
    private final CorrelationIdFilter filter = new CorrelationIdFilter();

    @AfterEach
    void clearMdc() {
        MDC.clear();
    }

    @Test
    void validCorrelationIdIsPreservedInMdcAndResponse() throws Exception {
        String valid = "abc-123_TEST.trace:1";
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.addHeader(CorrelationIdFilter.CORRELATION_ID_HEADER, valid);
        MockHttpServletResponse response = new MockHttpServletResponse();
        AtomicReference<String> observed = new AtomicReference<>();

        filter.doFilter(request, response, (req, res) ->
                observed.set(MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY)));

        assertEquals(valid, observed.get());
        assertEquals(valid, response.getHeader(CorrelationIdFilter.CORRELATION_ID_HEADER));
        assertNull(MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY));
    }

    @Test
    void missingCorrelationIdIsReplacedWithSafeGeneratedValue() throws Exception {
        assertReplaced(new MockHttpServletRequest());
    }

    @Test
    void blankCorrelationIdIsReplaced() throws Exception {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.addHeader(CorrelationIdFilter.CORRELATION_ID_HEADER, "   ");
        assertReplaced(request);
    }

    @Test
    void whitespaceCorrelationIdIsReplaced() throws Exception {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.addHeader(CorrelationIdFilter.CORRELATION_ID_HEADER, "bad id");
        assertReplaced(request);
    }

    @Test
    void illegalCharactersAreReplaced() throws Exception {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.addHeader(CorrelationIdFilter.CORRELATION_ID_HEADER, "bad/id");
        assertReplaced(request);
    }

    @Test
    void length129IsReplaced() throws Exception {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.addHeader(CorrelationIdFilter.CORRELATION_ID_HEADER, "a".repeat(129));
        assertReplaced(request);
    }

    @Test
    void crlfStyleValueIsReplaced() throws Exception {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.addHeader(CorrelationIdFilter.CORRELATION_ID_HEADER, "corr\r\nid");
        assertReplaced(request);
    }

    @Test
    void mdcClearedAfterSuccessfulChain() throws Exception {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.addHeader(CorrelationIdFilter.CORRELATION_ID_HEADER, "keep-me");
        MockHttpServletResponse response = new MockHttpServletResponse();
        filter.doFilter(request, response, new MockFilterChain());
        assertNull(MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY));
    }

    @Test
    void mdcClearedAfterDownstreamException() {
        MockHttpServletRequest request = new MockHttpServletRequest();
        request.addHeader(CorrelationIdFilter.CORRELATION_ID_HEADER, "keep-me");
        MockHttpServletResponse response = new MockHttpServletResponse();

        assertThrows(ServletException.class, () ->
                filter.doFilter(request, response, (req, res) -> {
                    assertEquals("keep-me", MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY));
                    throw new ServletException("downstream failure");
                }));

        assertNull(MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY));
    }

    private void assertReplaced(MockHttpServletRequest request) throws ServletException, IOException {
        MockHttpServletResponse response = new MockHttpServletResponse();
        AtomicReference<String> observed = new AtomicReference<>();
        filter.doFilter(request, response, (req, res) ->
                observed.set(MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY)));

        String generated = response.getHeader(CorrelationIdFilter.CORRELATION_ID_HEADER);
        assertNotNull(generated);
        assertFalse(generated.isBlank());
        assertTrue(generated.length() <= 128);
        assertTrue(SAFE.matcher(generated).matches());
        assertEquals(generated, observed.get());
        assertNull(MDC.get(CorrelationIdFilter.MDC_CORRELATION_ID_KEY));
    }
}
