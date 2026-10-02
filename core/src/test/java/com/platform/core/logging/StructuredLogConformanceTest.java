package com.platform.core.logging;

import ch.qos.logback.classic.Level;
import ch.qos.logback.classic.Logger;
import ch.qos.logback.classic.LoggerContext;
import ch.qos.logback.classic.spi.LoggingEvent;
import com.fasterxml.jackson.core.JsonFactory;
import com.fasterxml.jackson.core.StreamReadFeature;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import net.logstash.logback.composite.GlobalCustomFieldsJsonProvider;
import net.logstash.logback.composite.loggingevent.LogLevelJsonProvider;
import net.logstash.logback.composite.loggingevent.LoggingEventFormattedTimestampJsonProvider;
import net.logstash.logback.encoder.LoggingEventCompositeJsonEncoder;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.slf4j.LoggerFactory;
import org.slf4j.MDC;

import java.io.InputStream;
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

    private static final ObjectMapper MAPPER = new ObjectMapper(
            JsonFactory.builder()
                    .configure(StreamReadFeature.STRICT_DUPLICATE_DETECTION, true)
                    .build()
    );
    private static final Pattern TS_Z = Pattern.compile(
            "^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\\.[0-9]+)?Z$"
    );
    private static final Set<String> FORBIDDEN = Set.of(
            "timestamp", "time", "service", "environment", "thread", "logger",
            "msg", "error_code", "stack_trace", "stackTrace"
    );
    private static final Set<String> ALLOWED_TOP_LEVEL = Set.of(
            "ts", "level", "component", "version", "node_id", "service_instance_id", "message",
            "tenant_id", "event_code", "correlation_id", "request_id", "job_id", "entity_id",
            "protocol", "result", "retry_count", "safe_context", "sensor_id", "agent_id"
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
        SupportabilityMdcJsonProvider mdc = new SupportabilityMdcJsonProvider();

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
    void runtimeLogbackWiresTypedSupportabilityMdcProvider() throws Exception {
        String xml;
        try (InputStream in = getClass().getClassLoader().getResourceAsStream("logback-spring.xml")) {
            assertTrue(in != null, "logback-spring.xml must be on classpath");
            xml = new String(in.readAllBytes(), StandardCharsets.UTF_8);
        }
        assertTrue(
                xml.contains("com.platform.core.logging.SupportabilityMdcJsonProvider"),
                "runtime XML must wire typed SupportabilityMdcJsonProvider"
        );
        assertTrue(xml.contains("com.platform.core.logging.BoundedMessageJsonProvider"));
        assertTrue(xml.contains("<fieldName>ts</fieldName>"));
        assertFalse(xml.contains("<mdc>"), "runtime XML must not use generic unrestricted MDC provider");
        assertFalse(xml.contains("thread_name") || xml.contains("<threadName"));
        assertFalse(xml.contains("<loggerName") || xml.contains("logger_name"));
        assertFalse(xml.contains("\"timestamp\"") && xml.contains("fieldName>timestamp"));
        assertFalse(xml.contains("stackTrace") || xml.contains("stack_trace"));
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
    void allApprovedContextualFieldsEmitWithCorrectJsonTypes() throws Exception {
        MDC.put(SupportabilityMdcKeys.TENANT_ID, "550e8400-e29b-41d4-a716-446655440000");
        MDC.put(SupportabilityMdcKeys.EVENT_CODE, CoreSupportabilityEventCodes.KAFKA_PUBLICATION_FAILED);
        MDC.put(SupportabilityMdcKeys.CORRELATION_ID, "abc-123_TEST.trace:1");
        MDC.put(SupportabilityMdcKeys.REQUEST_ID, "req-1");
        MDC.put(SupportabilityMdcKeys.JOB_ID, "job-9");
        MDC.put(SupportabilityMdcKeys.ENTITY_ID, "entity-42");
        MDC.put(SupportabilityMdcKeys.PROTOCOL, "snmp");
        MDC.put(SupportabilityMdcKeys.RESULT, "FAILED");
        MDC.put(SupportabilityMdcKeys.RETRY_COUNT, "2");
        MDC.put("password", "should-not-appear");
        MDC.put("arbitrary_debug", "nope");

        JsonNode json = encode("probe failed");

        assertEquals("550e8400-e29b-41d4-a716-446655440000", json.get("tenant_id").asText());
        assertEquals(CoreSupportabilityEventCodes.KAFKA_PUBLICATION_FAILED, json.get("event_code").asText());
        assertEquals("abc-123_TEST.trace:1", json.get("correlation_id").asText());
        assertEquals("req-1", json.get("request_id").asText());
        assertEquals("job-9", json.get("job_id").asText());
        assertEquals("entity-42", json.get("entity_id").asText());
        assertEquals("snmp", json.get("protocol").asText());
        assertEquals("FAILED", json.get("result").asText());
        assertTrue(json.get("retry_count").isIntegralNumber(), "retry_count must be JSON integer");
        assertEquals(2, json.get("retry_count").asLong());
        assertFalse(json.has("password"));
        assertFalse(json.has("arbitrary_debug"));
        assertOnlyAllowedTopLevel(json);
        assertNoForbidden(json);
    }

    @Test
    void invalidContextualValuesAreOmitted() throws Exception {
        MDC.put(SupportabilityMdcKeys.TENANT_ID, "not-a-uuid");
        MDC.put(SupportabilityMdcKeys.CORRELATION_ID, "bad id with spaces");
        MDC.put(SupportabilityMdcKeys.REQUEST_ID, "x".repeat(129));
        MDC.put(SupportabilityMdcKeys.PROTOCOL, "bad protocol!");
        MDC.put(SupportabilityMdcKeys.RESULT, "x".repeat(33));
        MDC.put(SupportabilityMdcKeys.RETRY_COUNT, "-1");
        MDC.put(SupportabilityMdcKeys.EVENT_CODE, CoreSupportabilityEventCodes.CLICKHOUSE_INSERT_FAILED);

        JsonNode withNegative = encode("invalid context");
        assertFalse(withNegative.has("tenant_id"));
        assertFalse(withNegative.has("correlation_id"));
        assertFalse(withNegative.has("request_id"));
        assertFalse(withNegative.has("protocol"));
        assertFalse(withNegative.has("result"));
        assertFalse(withNegative.has("retry_count"));
        assertEquals(CoreSupportabilityEventCodes.CLICKHOUSE_INSERT_FAILED, withNegative.get("event_code").asText());

        MDC.clear();
        MDC.put(SupportabilityMdcKeys.RETRY_COUNT, "not-a-number");
        assertFalse(encode("non numeric retry").has("retry_count"));

        MDC.clear();
        MDC.put(SupportabilityMdcKeys.RETRY_COUNT, "9007199254740992");
        assertFalse(encode("overflow retry").has("retry_count"));
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

    @Test
    void emittedJsonHasNoDuplicateTopLevelKeysUnderStrictDecode() throws Exception {
        MDC.put(SupportabilityMdcKeys.EVENT_CODE, CoreSupportabilityEventCodes.KAFKA_PUBLICATION_FAILED);
        MDC.put(SupportabilityMdcKeys.CORRELATION_ID, "corr-slice5-core");
        String raw = encodeRaw("duplicate-key certification");
        // STRICT_DUPLICATE_DETECTION on MAPPER fails closed if any top-level key repeats.
        JsonNode json = MAPPER.readTree(raw);
        assertRequired(json);
        assertOnlyAllowedTopLevel(json);
        assertEquals(1, raw.split("\"component\"", -1).length - 1);
        assertEquals(1, raw.split("\"service_instance_id\"", -1).length - 1);
        assertEquals(1, raw.split("\"node_id\"", -1).length - 1);
    }

    @Test
    void controlCharactersInMessageRemainStructurallyValidJson() throws Exception {
        // BUILD-005 Slice 5: JSON encoder structural escaping is the Core bar.
        // Do not require a universal message scrubber; prove emitted bytes remain
        // parseable as a single JSON object under strict duplicate detection.
        String raw = encodeRaw("line1\r\nline2\twith\u0000null");
        JsonNode json = MAPPER.readTree(raw);
        assertTrue(json.isObject());
        assertTrue(json.hasNonNull("message"));
        assertRequired(json);
        assertOnlyAllowedTopLevel(json);
    }

    private JsonNode encode(String message) throws Exception {
        return MAPPER.readTree(encodeRaw(message));
    }

    private String encodeRaw(String message) {
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
        return new String(bytes, StandardCharsets.UTF_8);
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

    private static void assertOnlyAllowedTopLevel(JsonNode json) {
        Iterator<String> names = json.fieldNames();
        while (names.hasNext()) {
            String name = names.next();
            assertTrue(ALLOWED_TOP_LEVEL.contains(name), "unexpected top-level field: " + name);
        }
    }
}
