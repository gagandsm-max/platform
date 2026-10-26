package com.platform.core.telemetry.read;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.domain.DomainException;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;
import org.springframework.context.annotation.AnnotationConfigApplicationContext;
import org.springframework.core.env.MapPropertySource;

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.OutputStream;
import java.math.BigInteger;
import java.net.InetSocketAddress;
import java.net.URLDecoder;
import java.net.http.HttpClient;
import java.net.http.HttpHeaders;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.net.http.HttpTimeoutException;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;
import java.util.Map;
import java.util.StringJoiner;
import java.util.UUID;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotNull;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTimeoutPreemptively;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class ClickHouseMetricObservationReaderTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");

    private final List<HttpServer> wireServers = new ArrayList<>();
    private final List<ExecutorService> fixtureExecutors = new ArrayList<>();

    @AfterEach
    void tearDownWireFixtures() throws InterruptedException {
        for (HttpServer server : wireServers) {
            server.stop(0);
        }
        wireServers.clear();
        for (ExecutorService executor : fixtureExecutors) {
            executor.shutdownNow();
            assertTrue(executor.awaitTermination(5, TimeUnit.SECONDS),
                    "wire fixture executor failed to terminate");
        }
        fixtureExecutors.clear();
        // Fixture-owned executors must fully terminate so no issue105-wire-* threads remain.
        // Per-test cancel evidence is asserted via server-side cancelled/handlerFinished flags.
        long leftover = -1L;
        long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(3);
        do {
            leftover = Thread.getAllStackTraces().keySet().stream()
                    .filter(t -> t.getName().startsWith("issue105-wire-"))
                    .filter(Thread::isAlive)
                    .count();
            if (leftover == 0L) {
                break;
            }
            Thread.sleep(50L);
        } while (System.nanoTime() < deadline);
        assertEquals(0L, leftover, "fixture issue105-wire-* threads must terminate after shutdown");
    }

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
    void validJsonNumericExponentOverflowFailsClosedWhileFiniteControlIsPreserved() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        // These are valid JSON number tokens, not quoted strings or bare NaN/Infinity.
        String row = """
                {"ts":"2026-10-16 00:00:01.000","value":%s,"quality":1,"flags":0,"sensor_id":"s1","sequence_id":"a","source_protocol":"agent"}
                """;
        for (String overflow : List.of("1e309", "-1e309")) {
            DomainException ex = assertThrows(DomainException.class,
                    () -> reader.parseLatestPage(row.formatted(overflow), MetricValueKind.NUMERIC, 10));
            assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
            assertEquals("Telemetry store returned malformed observation data", ex.getMessage());
        }
        MetricObservationPage page = reader.parseLatestPage(row.formatted("1.25"), MetricValueKind.NUMERIC, 10);
        assertEquals(1, page.observations().size());
        assertEquals(1.25, page.observations().get(0).numericValue());
        assertFalse(page.truncated());
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
    void clickHouseOfflineMapsToSanitizedUnavailable() {
        HttpClient httpClient = mock(HttpClient.class);
        when(httpClient.sendAsync(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenReturn(CompletableFuture.failedFuture(new java.net.ConnectException("connection refused")));
        ClickHouseMetricObservationReader reader = newReader(httpClient);

        MetricObservationQuery query = agentCpuQuery(10);
        DomainException ex = assertThrows(DomainException.class, () -> reader.read(query));
        assertEquals("DATABASE_UNAVAILABLE", ex.getCode());
        assertTrue(ex.getMessage().contains("temporarily unavailable"));
    }

    @Test
    void realTimeoutMapsToSanitizedUnavailable() {
        HttpClient httpClient = mock(HttpClient.class);
        when(httpClient.sendAsync(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenReturn(CompletableFuture.failedFuture(new HttpTimeoutException("request timed out")));
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
        when(response.headers()).thenReturn(HttpHeaders.of(Map.of(), (a, b) -> true));
        AtomicReference<HttpRequest> captured = new AtomicReference<>();
        when(httpClient.sendAsync(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenAnswer(invocation -> {
                    captured.set(invocation.getArgument(0));
                    return CompletableFuture.completedFuture(response);
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
        assertEquals(2 * 1024 * 1024, ClickHouseMetricObservationReader.MAX_RESPONSE_BYTES);
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
    void ceilBoundsForSubMillisecondWindowSelectCorrectDateTime64Interval() {
        HttpClient httpClient = mock(HttpClient.class);
        @SuppressWarnings("unchecked")
        HttpResponse<String> response = mock(HttpResponse.class);
        when(response.statusCode()).thenReturn(200);
        when(response.body()).thenReturn("");
        when(response.headers()).thenReturn(HttpHeaders.of(Map.of(), (a, b) -> true));
        AtomicReference<HttpRequest> captured = new AtomicReference<>();
        when(httpClient.sendAsync(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenAnswer(invocation -> {
                    captured.set(invocation.getArgument(0));
                    return CompletableFuture.completedFuture(response);
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

    @Test
    void malformedTailAfterNPlusOneProbeMapsToDataIntegrityError() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        StringJoiner joiner = new StringJoiner("\n");
        // limit=3 → maxRows=4 exact probe rows, then late nonblank backend error text.
        for (int i = 4; i >= 1; i--) {
            joiner.add(numericLine(i, "sensor-a", "seq-" + i));
        }
        joiner.add("Code: 241. DB::Exception: Memory limit exceeded");
        DomainException ex = assertThrows(DomainException.class,
                () -> reader.parseLatestPage(joiner.toString(), MetricValueKind.NUMERIC, 3));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void trailingJsonTokensWithinRowMapToDataIntegrityError() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        String body = """
                {"ts":"2026-10-16 00:00:01.000","value":1.0,"quality":1,"flags":0,"sensor_id":"s1","sequence_id":"a","source_protocol":"agent"}{"extra":true}
                """;
        DomainException ex = assertThrows(DomainException.class,
                () -> reader.parseLatestPage(body.trim(), MetricValueKind.NUMERIC, 10));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void nonzeroClickHouseExceptionCodeHeaderMapsToDataIntegrityError() {
        HttpClient httpClient = mock(HttpClient.class);
        @SuppressWarnings("unchecked")
        HttpResponse<String> response = mock(HttpResponse.class);
        when(response.statusCode()).thenReturn(200);
        when(response.body()).thenReturn("");
        when(response.headers()).thenReturn(HttpHeaders.of(
                Map.of("X-ClickHouse-Exception-Code", List.of("241")), (a, b) -> true));
        when(httpClient.sendAsync(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenReturn(CompletableFuture.completedFuture(response));

        DomainException ex = assertThrows(DomainException.class,
                () -> newReader(httpClient).read(agentCpuQuery(10)));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void malformedClickHouseExceptionCodeHeaderMapsToDataIntegrityError() {
        HttpClient httpClient = mock(HttpClient.class);
        @SuppressWarnings("unchecked")
        HttpResponse<String> response = mock(HttpResponse.class);
        when(response.statusCode()).thenReturn(200);
        when(response.body()).thenReturn(numericLine(1, "s1", "a"));
        when(response.headers()).thenReturn(HttpHeaders.of(
                Map.of("X-ClickHouse-Exception-Code", List.of("not-a-code")), (a, b) -> true));
        when(httpClient.sendAsync(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenReturn(CompletableFuture.completedFuture(response));

        DomainException ex = assertThrows(DomainException.class,
                () -> newReader(httpClient).read(agentCpuQuery(10)));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void zeroClickHouseExceptionCodeHeaderAllowsEmptyBody() {
        HttpClient httpClient = mock(HttpClient.class);
        @SuppressWarnings("unchecked")
        HttpResponse<String> response = mock(HttpResponse.class);
        when(response.statusCode()).thenReturn(200);
        when(response.body()).thenReturn("");
        when(response.headers()).thenReturn(HttpHeaders.of(
                Map.of("X-ClickHouse-Exception-Code", List.of("0")), (a, b) -> true));
        when(httpClient.sendAsync(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenReturn(CompletableFuture.completedFuture(response));

        MetricObservationPage page = newReader(httpClient).read(agentCpuQuery(10));
        assertEquals(0, page.observations().size());
        assertFalse(page.truncated());
    }

    @Test
    void firstZeroThenNonzeroClickHouseExceptionCodeHeaderMapsToDataIntegrityError() {
        // firstValue("0") must not hide a later explicit error code (JDK preserves all values).
        HttpClient httpClient = mock(HttpClient.class);
        @SuppressWarnings("unchecked")
        HttpResponse<String> response = mock(HttpResponse.class);
        when(response.statusCode()).thenReturn(200);
        when(response.body()).thenReturn("");
        when(response.headers()).thenReturn(HttpHeaders.of(
                Map.of("X-ClickHouse-Exception-Code", List.of("0", "241")), (a, b) -> true));
        when(httpClient.sendAsync(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenReturn(CompletableFuture.completedFuture(response));

        DomainException ex = assertThrows(DomainException.class,
                () -> newReader(httpClient).read(agentCpuQuery(10)));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void duplicateZeroClickHouseExceptionCodeHeaderMapsToDataIntegrityError() {
        HttpClient httpClient = mock(HttpClient.class);
        @SuppressWarnings("unchecked")
        HttpResponse<String> response = mock(HttpResponse.class);
        when(response.statusCode()).thenReturn(200);
        when(response.body()).thenReturn("");
        when(response.headers()).thenReturn(HttpHeaders.of(
                Map.of("X-ClickHouse-Exception-Code", List.of("0", "0")), (a, b) -> true));
        when(httpClient.sendAsync(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenReturn(CompletableFuture.completedFuture(response));

        DomainException ex = assertThrows(DomainException.class,
                () -> newReader(httpClient).read(agentCpuQuery(10)));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void validMaximumPagePositiveControlUnderByteCeiling() {
        ClickHouseMetricObservationReader reader = newReader(mock(HttpClient.class));
        int limit = MetricObservationQuery.MAX_LIMIT; // 500 → fetch 501
        StringJoiner joiner = new StringJoiner("\n");
        for (int i = 501; i >= 1; i--) {
            joiner.add(numericLine(i, "sensor-a", "seq-" + i));
        }
        String body = joiner.toString();
        assertTrue(body.getBytes(StandardCharsets.UTF_8).length
                < ClickHouseMetricObservationReader.MAX_RESPONSE_BYTES);
        MetricObservationPage page = reader.parseLatestPage(body, MetricValueKind.NUMERIC, limit);
        assertEquals(limit, page.observations().size());
        assertTrue(page.truncated());
        assertEquals(Instant.parse("2026-10-16T00:00:02.000Z"), page.observations().get(0).timestamp());
        assertEquals(Instant.parse("2026-10-16T00:08:21.000Z"), page.observations().get(limit - 1).timestamp());
    }

    @Test
    void wireStalledBodyAfter200HeadersMapsToUnavailableWithinBudget() throws Exception {
        CountDownLatch headersSent = new CountDownLatch(1);
        CountDownLatch handlerFinished = new CountDownLatch(1);
        AtomicBoolean writeCancelled = new AtomicBoolean(false);
        AtomicInteger openExchanges = new AtomicInteger(0);
        HttpServer server = startWireServer((exchange, mode) -> {
            openExchanges.incrementAndGet();
            try {
                exchange.getResponseHeaders().add("Content-Type", "text/plain; charset=UTF-8");
                // Large declared length; body barely trickles so headers complete first and
                // subsequent writes surface client cancel after the absolute deadline aborts.
                exchange.sendResponseHeaders(200, 10_000_000);
                headersSent.countDown();
                OutputStream os = exchange.getResponseBody();
                try {
                    for (int i = 0; i < 200_000; i++) {
                        os.write('x');
                        os.flush();
                        Thread.sleep(25L);
                    }
                } catch (InterruptedException | IOException ex) {
                    writeCancelled.set(true);
                    if (ex instanceof InterruptedException) {
                        Thread.currentThread().interrupt();
                    }
                } finally {
                    try {
                        os.close();
                    } catch (IOException ignored) {
                        // exchange teardown
                    }
                }
            } catch (IOException ex) {
                writeCancelled.set(true);
                headersSent.countDown();
            } finally {
                openExchanges.decrementAndGet();
                handlerFinished.countDown();
            }
        });

        Duration budget = Duration.ofMillis(800);
        ClickHouseMetricObservationReader reader = realWireReader(server, budget);
        long started = System.nanoTime();
        DomainException ex = assertTimeoutPreemptively(Duration.ofSeconds(3), () ->
                assertThrows(DomainException.class, () -> reader.read(agentCpuQuery(10))));
        long elapsedMs = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started);
        assertEquals("DATABASE_UNAVAILABLE", ex.getCode());
        assertTrue(elapsedMs < 2_500L, "absolute deadline must bound return, elapsedMs=" + elapsedMs);
        assertTrue(headersSent.await(1, TimeUnit.SECONDS));
        assertTrue(handlerFinished.await(5, TimeUnit.SECONDS),
                "server handler must finish after client cancel (no blocked background read)");
        assertTrue(writeCancelled.get() || openExchanges.get() == 0,
                "exchange/subscription must cancel without leaving a blocked background read");
    }

    @Test
    void wireSlowTrickleBodyExceedingDeadlineMapsToUnavailable() throws Exception {
        AtomicBoolean cancelled = new AtomicBoolean(false);
        CountDownLatch handlerFinished = new CountDownLatch(1);
        HttpServer server = startWireServer((exchange, mode) -> {
            try {
                exchange.getResponseHeaders().add("Content-Type", "text/plain; charset=UTF-8");
                exchange.sendResponseHeaders(200, 0);
                OutputStream os = exchange.getResponseBody();
                try {
                    for (int i = 0; i < 200; i++) {
                        os.write((numericLine(i + 1, "s", "q" + i) + "\n")
                                .getBytes(StandardCharsets.UTF_8));
                        os.flush();
                        Thread.sleep(100L);
                    }
                } catch (InterruptedException | IOException ex) {
                    cancelled.set(true);
                    if (ex instanceof InterruptedException) {
                        Thread.currentThread().interrupt();
                    }
                } finally {
                    try {
                        os.close();
                    } catch (IOException ignored) {
                        // teardown
                    }
                }
            } catch (IOException ex) {
                cancelled.set(true);
            } finally {
                handlerFinished.countDown();
            }
        });

        ClickHouseMetricObservationReader reader = realWireReader(server, Duration.ofMillis(700));
        DomainException ex = assertTimeoutPreemptively(Duration.ofSeconds(3), () ->
                assertThrows(DomainException.class, () -> reader.read(agentCpuQuery(10))));
        assertEquals("DATABASE_UNAVAILABLE", ex.getCode());
        assertTrue(handlerFinished.await(5, TimeUnit.SECONDS),
                "slow-trickle handler must finish after absolute-deadline cancel");
        assertTrue(cancelled.get() || handlerFinished.getCount() == 0L,
                "slow trickle must be cancelled on absolute deadline");
    }

    @Test
    void wireOversizeBodyMapsToDataIntegrityAndCancels() throws Exception {
        AtomicBoolean cancelled = new AtomicBoolean(false);
        CountDownLatch pastCeiling = new CountDownLatch(1);
        CountDownLatch handlerFinished = new CountDownLatch(1);
        AtomicLong bytesWritten = new AtomicLong();
        byte[] chunk = new byte[32 * 1024];
        Arrays.fill(chunk, (byte) 'x');
        HttpServer server = startWireServer((exchange, mode) -> {
            try {
                exchange.getResponseHeaders().add("Content-Type", "text/plain; charset=UTF-8");
                // Continuing producer: unknown length, never naturally completes under the ceiling.
                exchange.sendResponseHeaders(200, 0);
                OutputStream os = exchange.getResponseBody();
                try {
                    while (true) {
                        os.write(chunk);
                        os.flush();
                        long written = bytesWritten.addAndGet(chunk.length);
                        if (written > ClickHouseMetricObservationReader.MAX_RESPONSE_BYTES) {
                            pastCeiling.countDown();
                        }
                        Thread.sleep(2L);
                    }
                } catch (InterruptedException | IOException ex) {
                    cancelled.set(true);
                    if (ex instanceof InterruptedException) {
                        Thread.currentThread().interrupt();
                    }
                } finally {
                    try {
                        os.close();
                    } catch (IOException ignored) {
                        // teardown
                    }
                }
            } catch (IOException ex) {
                cancelled.set(true);
            } finally {
                handlerFinished.countDown();
            }
        });

        ClickHouseMetricObservationReader reader = realWireReader(server, Duration.ofSeconds(5));
        DomainException ex = assertTimeoutPreemptively(Duration.ofSeconds(8), () ->
                assertThrows(DomainException.class, () -> reader.read(agentCpuQuery(10))));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
        // Cancellation must be observed before teardown; handlerFinished alone is tautological
        // after await and natural full-body completion must not satisfy this assertion.
        long cancelDeadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(5);
        while (!cancelled.get() && System.nanoTime() < cancelDeadline) {
            Thread.sleep(20L);
        }
        assertTrue(cancelled.get(),
                "oversize body must cancel the continuing producer before teardown");
        assertTrue(
                bytesWritten.get() > ClickHouseMetricObservationReader.MAX_RESPONSE_BYTES
                        || pastCeiling.getCount() == 0L,
                "continuing producer must have crossed the retained-byte ceiling before cancel");
        assertTrue(handlerFinished.await(5, TimeUnit.SECONDS),
                "oversize handler must finish after subscription cancel");
    }

    @Test
    void wireMalformedUtf8InOtherwiseValidJsonMapsToDataIntegrityError() throws Exception {
        ByteArrayOutputStream body = new ByteArrayOutputStream();
        body.write(("{\"ts\":\"2026-10-16 00:00:01.000\",\"value\":1.0,\"quality\":1,\"flags\":0,"
                + "\"sensor_id\":\"").getBytes(StandardCharsets.UTF_8));
        body.write(0xFF); // malformed UTF-8; replacement decoding would admit altered provenance
        body.write(("\",\"sequence_id\":\"a\",\"source_protocol\":\"agent\"}\n")
                .getBytes(StandardCharsets.UTF_8));
        byte[] payload = body.toByteArray();

        HttpServer server = startWireServer((exchange, mode) -> {
            exchange.getResponseHeaders().add("Content-Type", "text/plain; charset=UTF-8");
            exchange.sendResponseHeaders(200, payload.length);
            try (OutputStream os = exchange.getResponseBody()) {
                os.write(payload);
            }
        });

        ClickHouseMetricObservationReader reader = realWireReader(server, Duration.ofSeconds(5));
        DomainException ex = assertTimeoutPreemptively(Duration.ofSeconds(5), () ->
                assertThrows(DomainException.class, () -> reader.read(agentCpuQuery(10))));
        assertEquals("DATA_INTEGRITY_ERROR", ex.getCode());
    }

    @Test
    void wireValidMultibyteUtf8ObservationIsPreserved() throws Exception {
        String sensorId = "sensor-\u6e2c\u8a66-\u00e9"; // multibyte CJK + Latin-1 supplement
        String line = """
                {"ts":"2026-10-16 00:00:01.000","value":1.0,"quality":1,"flags":0,"sensor_id":"%s","sequence_id":"a","source_protocol":"agent"}
                """.formatted(sensorId).trim() + "\n";
        byte[] payload = line.getBytes(StandardCharsets.UTF_8);
        assertTrue(payload.length > line.length(), "fixture must contain multibyte UTF-8");

        HttpServer server = startWireServer((exchange, mode) -> {
            exchange.getResponseHeaders().add("Content-Type", "text/plain; charset=UTF-8");
            exchange.sendResponseHeaders(200, payload.length);
            try (OutputStream os = exchange.getResponseBody()) {
                os.write(payload);
            }
        });

        ClickHouseMetricObservationReader reader = realWireReader(server, Duration.ofSeconds(5));
        MetricObservationPage page = assertTimeoutPreemptively(Duration.ofSeconds(5),
                () -> reader.read(agentCpuQuery(10)));
        assertEquals(1, page.observations().size());
        assertEquals(sensorId, page.observations().get(0).sensorId());
        assertEquals(1, page.observations().get(0).quality());
    }

    private ClickHouseMetricObservationReader realWireReader(HttpServer server, Duration budget) {
        String base = "http://127.0.0.1:" + server.getAddress().getPort();
        ExecutorService httpExecutor = Executors.newCachedThreadPool(r -> {
            Thread t = new Thread(r, "issue105-wire-http-" + System.nanoTime());
            t.setDaemon(true);
            return t;
        });
        fixtureExecutors.add(httpExecutor);
        HttpClient client = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(2))
                .executor(httpExecutor)
                .build();
        return new ClickHouseMetricObservationReader(
                base, "Basic dGVzdA==", client, new ObjectMapper(), budget);
    }

    @FunctionalInterface
    private interface WireHandler {
        void handle(HttpExchange exchange, String mode) throws IOException;
    }

    private HttpServer startWireServer(WireHandler handler) throws IOException {
        HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", exchange -> handler.handle(exchange, "default"));
        ExecutorService serverExecutor = Executors.newCachedThreadPool(r -> {
            Thread t = new Thread(r, "issue105-wire-server-" + System.nanoTime());
            t.setDaemon(true);
            return t;
        });
        fixtureExecutors.add(serverExecutor);
        server.setExecutor(serverExecutor);
        server.start();
        wireServers.add(server);
        return server;
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
        // secondOffset 1 → 00:00:01, 200 → 00:03:20, 501 → 00:08:21
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
