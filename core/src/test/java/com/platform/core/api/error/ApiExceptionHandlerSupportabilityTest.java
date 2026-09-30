package com.platform.core.api.error;

import ch.qos.logback.classic.Level;
import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.spi.ILoggingEvent;
import ch.qos.logback.core.read.ListAppender;
import com.platform.core.logging.CoreSupportabilityEventCodes;
import com.platform.core.logging.SupportabilityMdcKeys;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;
import org.springframework.dao.DataAccessResourceFailureException;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNull;
import static org.junit.jupiter.api.Assertions.assertTrue;

class ApiExceptionHandlerSupportabilityTest {

    private ApiExceptionHandler handler;
    private Logger logger;
    private ListAppender<ILoggingEvent> appender;
    private Level previousLevel;

    @BeforeEach
    void setUp() {
        handler = new ApiExceptionHandler();
        logger = (Logger) LoggerFactory.getLogger(ApiExceptionHandler.class);
        appender = new ListAppender<>();
        appender.start();
        previousLevel = logger.getLevel();
        logger.setLevel(Level.ERROR);
        logger.addAppender(appender);
        MDC.clear();
    }

    @AfterEach
    void tearDown() {
        logger.detachAppender(appender);
        logger.setLevel(previousLevel);
        appender.stop();
        MDC.clear();
    }

    @Test
    void handleDataAccessEmitsSafeEventWithoutExceptionSentinel() {
        String bearer = "Bearer FAKESECRET_DB_123";
        String password = "password=FAKE_DB_PASSWORD";
        String jdbc = "jdbc:postgresql://user:FAKEPASS@example.invalid/db";
        DataAccessResourceFailureException ex = new DataAccessResourceFailureException(
                bearer + " " + password + " " + jdbc
        );

        ResponseEntity<Map<String, Object>> response = handler.handleDataAccess(ex);

        assertEquals(HttpStatus.SERVICE_UNAVAILABLE, response.getStatusCode());
        assertEquals("DATABASE_UNAVAILABLE", response.getBody().get("code"));
        assertNull(MDC.get(SupportabilityMdcKeys.EVENT_CODE));

        assertEquals(1, appender.list.size());
        ILoggingEvent event = appender.list.get(0);
        assertEquals(
                CoreSupportabilityEventCodes.DATABASE_ACCESS_FAILED,
                event.getMDCPropertyMap().get(SupportabilityMdcKeys.EVENT_CODE)
        );
        String message = event.getFormattedMessage();
        assertTrue(message.contains("failureClass=DataAccessResourceFailureException"), message);
        assertFalse(message.contains(bearer), message);
        assertFalse(message.contains(password), message);
        assertFalse(message.contains(jdbc), message);
        assertFalse(message.contains("FAKESECRET_DB_123"), message);
        assertFalse(message.contains("FAKE_DB_PASSWORD"), message);
        assertNull(event.getThrowableProxy());
    }

    @Test
    void handleGenericEmitsSafeEventWithoutExceptionSentinel() {
        String bearer = "Bearer FAKESECRET_API_123";
        RuntimeException ex = new RuntimeException(bearer);

        ResponseEntity<Map<String, Object>> response = handler.handleGeneric(ex);

        assertEquals(HttpStatus.INTERNAL_SERVER_ERROR, response.getStatusCode());
        assertEquals("INTERNAL_ERROR", response.getBody().get("code"));
        assertNull(MDC.get(SupportabilityMdcKeys.EVENT_CODE));

        assertEquals(1, appender.list.size());
        ILoggingEvent event = appender.list.get(0);
        assertEquals(
                CoreSupportabilityEventCodes.UNHANDLED_API_FAILURE,
                event.getMDCPropertyMap().get(SupportabilityMdcKeys.EVENT_CODE)
        );
        String message = event.getFormattedMessage();
        assertTrue(message.contains("failureClass=RuntimeException"), message);
        assertFalse(message.contains(bearer), message);
        assertFalse(message.contains("FAKESECRET_API_123"), message);
        assertNull(event.getThrowableProxy());
    }
}
