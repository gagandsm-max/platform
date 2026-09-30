package com.platform.core.logging;

import ch.qos.logback.classic.Level;
import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.LoggerContext;
import ch.qos.logback.classic.spi.LoggingEvent;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import net.logstash.logback.composite.GlobalCustomFieldsJsonProvider;
import net.logstash.logback.composite.loggingevent.LogLevelJsonProvider;
import net.logstash.logback.composite.loggingevent.LoggingEventFormattedTimestampJsonProvider;
import net.logstash.logback.composite.loggingevent.MdcJsonProvider;
import net.logstash.logback.encoder.LoggingEventCompositeJsonEncoder;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;

import java.nio.charset.StandardCharsets;
import java.util.HashMap;
import java.util.Iterator;
import java.util.Map;
import java.util.Set;
import java.util.regex.Pattern;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class StructuredLogConformanceTest {

    private static final ObjectMapper MAPPER = new ObjectMapper();
    private static final Pattern TS_Z = Pattern.compile(
            "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?Z$"
    );
    private static final Set<String> FORBIDDEN = Set.of(
            "timestamp", "time", "service", "environment", "thread", "logger",
            "msg", "error_code", "stack_trace", "stackTrace"
    );

    private LoggerContext context;
    private LoggingEventCompositeJsonEncoder encoder;

    @BeforeEach
    void setUp() {
        context = (LoggerContext) LoggerFactory.getILoggerFactory();
        context.putProperty("platformNodeId", "core-node-test");
        context.putProperty("platformVersion", "0.1.0-test");
        context.putProperty("serviceInstanceId", ServiceInstanceIdPropertyDefiner.currentInstanceId());

        encoder = new LoggingEventCompositeJsonEncoder();
        encoder.setContext(context);

        LoggingEventFormattedTimestampJsonProvider timestamp = new LoggingEventFormattedTimestampJsonProvider();
        timestamp.setFieldName("ts");
        timestamp.setPattern("yyyy-MM-dd'T'HH:mm:ss.SSS'Z'");
        timestamp.setTimeZone("UTC");

        LogLevelJsonProvider level = new LogLevelJsonProvider();
        level.setFieldName("level");

        BoundedMessageJsonProvider message = new BoundedMessageJsonProvider();

        MdcJsonProvider mdc = new MdcJsonProvider();
        SupportabilityMdcKeys.ALLOWED.forEach(mdc::addIncludeMdcKeyName);

        GlobalCustomFieldsJsonProvider custom = new GlobalCustomFieldsJsonProvider();
        custom.setCustomFields(
                "{\"component\":\"platform-core\","
                        + "\"version\":\"0.1.0-test\","
                        + "\"node_id\":\"core-node-test\","
                        + "\"service_instance_id\":\"" + ServiceInstanceIdPropertyDefiner.currentInstanceId() + "\"}"
        );

        encoder.getProviders().addProvider(timestamp);
        encoder.getProviders().addProvider(level);
        encoder.getProviders().addProvider(message);
        encoder.getProviders().addProvider(mdc);
        encoder.getProviders().addProvider(custom);
        encoder.start();
    }

    @AfterEach
    void tearDown() {
        MDC.clear();
        if (encoder != null) {
            encoder.stop();
        }
    }

    @Test
    void emittedJsonContainsCanonicalRequiredFieldsOnly() throws Exception {
        JsonNode first = encode("hello supportability");
        JsonNode second = encode("second event");

        assertRequired(first);
        assertRequired(second);
        assertEquals(first.get("service_instance_id").asText(), second.get("service_instance_id").asText());
        assertEquals(ServiceInstanceIdPropertyDefiner.currentInstanceId(), first.get("service_instance_id").asText());
        assertNoForbidden(first);
        assertNoForbidden(second);
    }

    @Test
    void approvedContextualMdcFieldsAreEmittedAndArbitraryKeysAreExcluded() throws Exception {
        MDC.put(SupportabilityMdcKeys.CORRELATION_ID, "corr-test-1");
        MDC.put(SupportabilityMdcKeys.EVENT_CODE, CoreSupportabilityEventCodes.KAFKA_HEALTH_PROBE_FAILED);
        MDC.put("password", "should-not-appear");
        MDC.put("arbitrary_debug", "nope");

        JsonNode json = encode("probe failed");

        assertEquals("corr-test-1", json.get("correlation_id").asText());
        assertEquals(CoreSupportabilityEventCodes.KAFKA_HEALTH_PROBE_FAILED, json.get("event_code").asText());
        assertFalse(json.has("password"));
        assertFalse(json.has("arbitrary_debug"));
        assertNoForbidden(json);
    }

    @Test
    void messageIsBoundedToCertifiedMaximum() throws Exception {
        String oversized = "x".repeat(BoundedMessageJsonProvider.MAX_MESSAGE_LENGTH + 50);
        JsonNode json = encode(oversized);
        assertEquals(BoundedMessageJsonProvider.MAX_MESSAGE_LENGTH, json.get("message").asText().length());
    }

    @Test
    void credentialLikeMdcKeysAreNotEmittedAsTopLevelFields() throws Exception {
        MDC.put("Authorization", "Bearer fictional-token");
        MDC.put("api_token", "fictional");
        JsonNode json = encode("token validation failed");
        assertFalse(json.has("Authorization"));
        assertFalse(json.has("api_token"));
        assertEquals("token validation failed", json.get("message").asText());
    }

    private JsonNode encode(String message) throws Exception {
        ch.qos.logback.classic.Logger logger =
                context.getLogger("com.platform.core.logging.StructuredLogConformanceTest");
        LoggingEvent event = new LoggingEvent(
                Logger.class.getName(),
                logger,
                Level.INFO,
                message,
                null,
                null
        );
        event.setTimeStamp(System.currentTimeMillis());
        Map<String, String> mdcCopy = MDC.getCopyOfContextMap();
        event.setMDCPropertyMap(mdcCopy == null ? Map.of() : new HashMap<>(mdcCopy));
        byte[] bytes = encoder.encode(event);
        return MAPPER.readTree(new String(bytes, StandardCharsets.UTF_8));
    }

    private static void assertRequired(JsonNode json) {
        assertTrue(json.hasNonNull("ts"));
        assertTrue(json.hasNonNull("level"));
        assertTrue(json.hasNonNull("component"));
        assertTrue(json.hasNonNull("version"));
        assertTrue(json.hasNonNull("node_id"));
        assertTrue(json.hasNonNull("service_instance_id"));
        assertTrue(json.hasNonNull("message"));
        assertEquals("platform-core", json.get("component").asText());
        assertEquals("0.1.0-test", json.get("version").asText());
        assertEquals("core-node-test", json.get("node_id").asText());
        assertTrue(TS_Z.matcher(json.get("ts").asText()).matches());
        assertFalse(json.get("service_instance_id").asText().isBlank());
    }

    private static void assertNoForbidden(JsonNode json) {
        Iterator<String> names = json.fieldNames();
        while (names.hasNext()) {
            String name = names.next();
            assertFalse(FORBIDDEN.contains(name), "forbidden field present: " + name);
        }
    }
}
