package com.platform.core.telemetry.read;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.domain.DomainException;
import org.junit.jupiter.api.Test;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;
import org.springframework.core.env.MapPropertySource;

import java.math.BigInteger;
import java.net.URLDecoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.net.http.HttpTimeoutException;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.util.Map;
import java.util.StringJoiner;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class ClickHouseMetricObservationReaderTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");

    @Test
    void preservesCounter64AboveJsSafeIntegerAsString() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        String huge = "18446744073709551615";
        String body = """
                {"tenant_id":"%s","entity_id":"%s","metric_id":".1.3.6.1.2.1.31.1.1.1.6.1","sensor_id":"s1","ts":"2026-10-16 00:00:01.000","value":"%s","quality":1,"flags":0,"sequence_id":"seq-1","source_protocol":"snmp"}
                """.formatted(TENANT, ENTITY, huge).trim();

        MetricObservationPage page = reader.parseLatestPage(body, MetricValueKind.COUNTER64, 10);
        assertEquals(1, page.observations().size());
        assertEquals(huge, page.observations().get(0).counterValue());
        assertEquals(0, new BigInteger(page.observations().get(0).counterValue()).compareTo(CanonicalCounterMax.MAX));
        assertFalse(page.truncated());
    }

    @Test
    void selectsLatestFiftyOfTwoHundredWithTruthfulTruncation() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        // Simulate CH newest-first response of 51 rows for limit=50 (N+1 probe).
        StringJoiner joiner = new StringJoiner("\n");
        for (int i = 200; i >= 150; i--) {
            joiner.add(numericLine(i, "sensor-a", "seq-" + i));
        }
        MetricObservationPage page = reader.parseLatestPage(joiner.toString(), MetricValueKind.NUMERIC, 50);
        assertEquals(50, page.observations().size());
        assertTrue(page.truncated());
        // Presentation order is ascending within the selected latest window.
        assertEquals(Instant.parse("2026-10-16T00:02:31.000Z"), page.observations().get(0).timestamp()); // t=151
        assertEquals(Instant.parse("2026-10-16T00:03:20.000Z"), page.observations().get(49).timestamp()); // t=200
        assertEquals(200.0, page.observations().get(49).numericValue());
    }

    @Test
    void nPlusOneProbeWithoutExtraRowsIsNotTruncated() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        String body = """
                {"ts":"2026-10-16 00:00:02.000","value":2.0,"quality":1,"flags":0,"sensor_id":"s1","sequence_id":"b","source_protocol":"agent"}
                {"ts":"2026-10-16 00:00:01.000","value":1.0,"quality":1,"flags":0,"sensor_id":"s1","sequence_id":"a","source_protocol":"agent"}
                """;
        MetricObservationPage page = reader.parseLatestPage(body, MetricValueKind.NUMERIC, 10);
        assertEquals(2, page.observations().size());
        assertFalse(page.truncated());
        assertEquals(1.0, page.observations().get(0).numericValue());
        assertEquals(2.0, page.observations().get(1).numericValue());
    }

    @Test
    void deterministicSensorSequenceTieBreakPreservedThenReordered() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        // Same ts; CH returns sensor/sequence DESC: s2/seq-b then s1/seq-a
        String body = """
                {"ts":"2026-10-16 00:00:01.000","value":2.0,"quality":1,"flags":0,"sensor_id":"s2","sequence_id":"seq-b","source_protocol":"agent"}
                {"ts":"2026-10-16 00:00:01.000","value":1.0,"quality":1,"flags":0,"sensor_id":"s1","sequence_id":"seq-a","source_protocol":"agent"}
                """;
        MetricObservationPage page = reader.parseLatestPage(body, MetricValueKind.NUMERIC, 2);
        assertEquals(2, page.observations().size());
        // Presentation ASC: s1 before s2
        assertEquals("s1", page.observations().get(0).sensorId());
        assertEquals("s2", page.observations().get(1).sensorId());
    }

    @Test
    void malformedNumericValueMapsToDataIntegrityError() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        String body = """
                {"ts":"2026-10-16 00:00:01.000","value":"not-a-number","quality":1,"flags":0,"sensor_id":"s1","sequence_id":"a","source_protocol":"agent"}
                """;
        DomainException ex = assertThrows(DomainException.class,
                () -> reader.parseLatestPage(body, MetricValueKind.NUMERIC, 10));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
        assertTrue(ex.getMessage().contains("malformed"));
    }

    @Test
    void malformedJsonLineMapsToDataIntegrityError() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        DomainException ex = assertThrows(DomainException.class,
                () -> reader.parseLatestPage("{not-json", MetricValueKind.NUMERIC, 10));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void malformedCounterMapsToDataIntegrityError() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        String body = """
                {"ts":"2026-10-16 00:00:01.000","value":"-1","quality":1,"flags":0,"sensor_id":"s1","sequence_id":"a","source_protocol":"snmp"}
                """;
        DomainException ex = assertThrows(DomainException.class,
                () -> reader.parseLatestPage(body, MetricValueKind.COUNTER64, 10));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void clickHouseOfflineMapsToSanitizedUnavailable() throws Exception {
        HttpClient httpClient = mock(HttpClient.class);
        when(httpClient.send(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenThrow(new java.net.ConnectException("connection refused"));
        ClickHouseMetricObservationReader reader = newReader(httpClient);

        MetricObservationQuery query = agentCpuQuery(10);
        DomainException ex = assertThrows(DomainException.class, () -> reader.read(query));
        assertEquals("DATABASE_UNAVAILABLE", ex.getCode());
        assertTrue(ex.getMessage().contains("temporarily unavailable"));
    }

    @Test
    void realTimeoutMapsToSanitizedUnavailable() throws Exception {
        HttpClient httpClient = mock(HttpClient.class);
        when(httpClient.send(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenThrow(new HttpTimeoutException("request timed out"));
        ClickHouseMetricObservationReader reader = newReader(httpClient);

        DomainException ex = assertThrows(DomainException.class, () -> reader.read(agentCpuQuery(10)));
        assertEquals("DATABASE_UNAVAILABLE", ex.getCode());
        assertTrue(ex.getMessage().contains("temporarily unavailable"));
    }

    @Test
    void queryUsesCanonicalViewBoundParametersAndNPlusOneLimit() throws Exception {
        HttpClient httpClient = mock(HttpClient.class);
        @SuppressWarnings("unchecked")
        HttpResponse<String> response = mock(HttpResponse.class);
        when(response.statusCode()).thenReturn(200);
        when(response.body()).thenReturn("");
        AtomicReference<HttpRequest> captured = new AtomicReference<>();
        when(httpClient.send(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenAnswer(invocation -> {
                    captured.set(invocation.getArgument(0));
                    return response;
                });

        ClickHouseMetricObservationReader reader = newReader(httpClient);
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, ".1.3.6.1.2.1.31.1.1.1.6.1", "COUNTER64",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                50
        );
        reader.read(query);

        String sql = ClickHouseMetricObservationReader.buildStaticSql(MetricValueKind.COUNTER64);
        assertTrue(sql.contains("FROM platform.metric_counter"));
        assertTrue(!sql.contains("metric_counter_raw") && !sql.contains("metric_numeric_raw"));
        assertTrue(sql.contains("{tenant_id:String}"));
        assertTrue(sql.contains("ORDER BY ts DESC, sensor_id DESC, sequence_id DESC"));

        HttpRequest sent = captured.get();
        String uri = sent.uri().toString();
        assertTrue(uri.contains("param_tenant_id="));
        assertTrue(uri.contains("param_entity_id="));
        assertTrue(uri.contains("param_metric_id="));
        assertTrue(uri.contains("param_row_limit=51")); // N+1
        assertTrue(uri.contains("max_execution_time=4"));
        assertTrue(uri.contains(".1.3.6.1.2.1.31.1.1.1.6.1")
                || uri.contains("%2E1.3.6.1.2.1.31.1.1.1.6.1")
                || uri.contains("%2e1.3.6.1.2.1.31.1.1.1.6.1"));
    }

    @Test
    void requestTimeoutIsBudgeted() {
        Duration timeout = Duration.ofSeconds(5);
        ClickHouseMetricObservationReader reader = new ClickHouseMetricObservationReader(
                "http://clickhouse:8123", "Basic dGVzdA==",
                mock(HttpClient.class), new ObjectMapper(), timeout);
        assertEquals(ClickHouseMetricObservationReader.class, reader.getClass());
    }

    @Test
    void springSelectsProductionConstructorWithObjectMapperAndProperties() {
        assertEquals(2, ClickHouseMetricObservationReader.class.getDeclaredConstructors().length);

        AnnotationConfigApplicationContext context = new AnnotationConfigApplicationContext();
        context.getEnvironment().getPropertySources().addFirst(new MapPropertySource(
                "issue105-clickhouse-reader",
                Map.of(
                        "platform.clickhouse.host", "clickhouse",
                        "platform.clickhouse.port", "8123",
                        "platform.clickhouse.user", "platform_app",
                        "platform.clickhouse.password", "platform_dev_ch_password_change_in_prod"
                )));
        context.getDefaultListableBeanFactory().registerSingleton("objectMapper", new ObjectMapper());
        context.register(ClickHouseMetricObservationReader.class);
        context.refresh();

        assertNotNull(context.getBean(ClickHouseMetricObservationReader.class));
        context.close();
    }

    @Test
    void preservesUInt32FlagsMaxThroughRow() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        String body = """
                {"ts":"2026-10-16 00:00:01.000","value":1.0,"quality":1,"flags":4294967295,"sensor_id":"s1","sequence_id":"a","source_protocol":"agent"}
                """;
        MetricObservationPage page = reader.parseLatestPage(body, MetricValueKind.NUMERIC, 10);
        assertEquals(1, page.observations().size());
        assertEquals(4_294_967_295L, page.observations().get(0).flags());
        assertEquals(1, page.observations().get(0).quality());
    }

    @Test
    void rejectsOutOfRangeFlagsWithDataIntegrityError() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        String body = """
                {"ts":"2026-10-16 00:00:01.000","value":1.0,"quality":1,"flags":4294967296,"sensor_id":"s1","sequence_id":"a","source_protocol":"agent"}
                """;
        DomainException ex = assertThrows(DomainException.class,
                () -> reader.parseLatestPage(body, MetricValueKind.NUMERIC, 10));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void rejectsUnsupportedQualityThreeWithDataIntegrityError() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        String body = """
                {"ts":"2026-10-16 00:00:01.000","value":1.0,"quality":3,"flags":0,"sensor_id":"s1","sequence_id":"a","source_protocol":"agent"}
                """;
        DomainException ex = assertThrows(DomainException.class,
                () -> reader.parseLatestPage(body, MetricValueKind.NUMERIC, 10));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void rejectsQualityIntegralOverflowThatWouldWrapToGood() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        // 4294967297 as signed int truncates/wraps toward 1; must not become GOOD.
        String body = """
                {"ts":"2026-10-16 00:00:01.000","value":1.0,"quality":4294967297,"flags":0,"sensor_id":"s1","sequence_id":"a","source_protocol":"agent"}
                """;
        DomainException ex = assertThrows(DomainException.class,
                () -> reader.parseLatestPage(body, MetricValueKind.NUMERIC, 10));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void ceilBoundsForSubMillisecondWindowSelectCorrectDateTime64Interval() throws Exception {
        HttpClient httpClient = mock(HttpClient.class);
        @SuppressWarnings("unchecked")
        HttpResponse<String> response = mock(HttpResponse.class);
        when(response.statusCode()).thenReturn(200);
        when(response.body()).thenReturn("");
        AtomicReference<HttpRequest> captured = new AtomicReference<>();
        when(httpClient.send(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenAnswer(invocation -> {
                    captured.set(invocation.getArgument(0));
                    return response;
                });

        Instant from = Instant.parse("2026-10-16T00:00:00.000500Z");
        Instant to = Instant.parse("2026-10-16T00:00:00.001500Z");
        ClickHouseMetricObservationReader reader = newReader(httpClient);
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, "system.cpu.utilization", "NUMERIC", from, to, 10);
        reader.read(query);

        // Original API request bounds preserved on the query object.
        assertEquals(from, query.fromInclusive());
        assertEquals(to, query.toExclusive());

        String uri = URLDecoder.decode(captured.get().uri().toString(), StandardCharsets.UTF_8);
        assertTrue(uri.contains("param_from_ts=2026-10-16 00:00:00.001"), uri);
        assertTrue(uri.contains("param_to_ts=2026-10-16 00:00:00.002"), uri);
        // Floored SQL [.000,.001) must not be used — that would admit .000 and exclude .001.
        assertFalse(uri.contains("param_from_ts=2026-10-16 00:00:00.000"));
    }

    @Test
    void alreadyAlignedMillisecondBoundsAreUnchanged() {
        Instant aligned = Instant.parse("2026-10-16T00:00:00.001Z");
        assertEquals(aligned, ClickHouseMetricObservationReader.ceilToMillis(aligned));
        assertEquals("2026-10-16 00:00:00.001",
                ClickHouseMetricObservationReader.formatClickHouseBound(aligned));
        Instant zero = Instant.parse("2026-10-16T00:00:00.000Z");
        assertEquals("2026-10-16 00:00:00.000",
                ClickHouseMetricObservationReader.formatClickHouseBound(zero));
    }

    private static ClickHouseMetricObservationReader newReader(HttpClient httpClient) {
        return new ClickHouseMetricObservationReader(
                "http://clickhouse:8123", "Basic dGVzdA==",
                httpClient, new ObjectMapper(), Duration.ofSeconds(5));
    }

    private static MetricObservationQuery agentCpuQuery(int limit) {
        return MetricObservationQuery.parse(
                TENANT, ENTITY, "system.cpu.utilization", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                limit
        );
    }

    private static String numericLine(int secondOffset, String sensor, String seq) {
        // secondOffset 1 → 00:00:01, 200 → 00:03:20
        int totalSeconds = secondOffset;
        int minutes = totalSeconds / 60;
        int seconds = totalSeconds % 60;
        String ts = "2026-10-16 00:%02d:%02d.000".formatted(minutes, seconds);
        return """
                {"ts":"%s","value":%d.0,"quality":1,"flags":0,"sensor_id":"%s","sequence_id":"%s","source_protocol":"agent"}
                """.formatted(ts, secondOffset, sensor, seq).trim();
    }

    private static final class CanonicalCounterMax {
        static final BigInteger MAX = new BigInteger("18446744073709551615");
    }
}
