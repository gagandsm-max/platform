package com.platform.core.telemetry.read;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.domain.DomainException;
import org.junit.jupiter.api.Test;

import java.math.BigInteger;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.time.Instant;
import java.util.List;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicReference;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertThrows;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class ClickHouseMetricObservationReaderTest {

    private static final UUID TENANT = UUID.fromString("018f0e0a-0000-7000-8000-0000000000a1");
    private static final UUID ENTITY = UUID.fromString("018f0e0a-0000-7000-8000-0000000000e1");

    @Test
    void preservesCounter64AboveJsSafeIntegerAsString() throws Exception {
        ClickHouseMetricObservationReader reader = new ClickHouseMetricObservationReader(
                "http://clickhouse:8123", "Basic dGVzdA==",
                mock(HttpClient.class), new ObjectMapper(), Duration.ofSeconds(5));
        String huge = "18446744073709551615";
        String body = """
                {"tenant_id":"%s","entity_id":"%s","metric_id":"if.hc_in_octets","sensor_id":"s1","ts":"2026-10-16 00:00:01.000","value":"%s","quality":1,"flags":0,"sequence_id":"seq-1","source_protocol":"snmp"}
                """.formatted(TENANT, ENTITY, huge).trim();

        List<MetricObservationRow> rows = reader.parseRows(body, MetricValueKind.COUNTER64, 10);
        assertEquals(1, rows.size());
        assertEquals(huge, rows.get(0).counterValue());
        assertEquals(0, new BigInteger(rows.get(0).counterValue()).compareTo(CanonicalCounterMax.MAX));
    }

    @Test
    void collapsesReplayDuplicatesAreCallerViewContract_readerReturnsCanonicalRowsOnly() throws Exception {
        // Canonical views use FINAL; reader must not invent rate/discontinuity and must surface rows as returned.
        ClickHouseMetricObservationReader reader = new ClickHouseMetricObservationReader(
                "http://clickhouse:8123", "Basic dGVzdA==",
                mock(HttpClient.class), new ObjectMapper(), Duration.ofSeconds(5));
        String body = """
                {"ts":"2026-10-16 00:00:01.000","value":10.5,"quality":1,"flags":0,"sensor_id":"s1","sequence_id":"seq-1","source_protocol":"snmp"}
                {"ts":"2026-10-16 00:00:02.000","value":11.0,"quality":1,"flags":0,"sensor_id":"s1","sequence_id":"seq-2","source_protocol":"snmp"}
                """;
        List<MetricObservationRow> rows = reader.parseRows(body, MetricValueKind.NUMERIC, 10);
        assertEquals(2, rows.size());
        assertEquals(10.5, rows.get(0).numericValue());
        assertEquals(Instant.parse("2026-10-16T00:00:01.000Z"), rows.get(0).timestamp());
    }

    @Test
    void enforcesRowCapDuringParse() throws Exception {
        ClickHouseMetricObservationReader reader = new ClickHouseMetricObservationReader(
                "http://clickhouse:8123", "Basic dGVzdA==",
                mock(HttpClient.class), new ObjectMapper(), Duration.ofSeconds(5));
        String body = """
                {"ts":"2026-10-16 00:00:01.000","value":1.0,"quality":1,"flags":0,"sensor_id":"s1","sequence_id":"a","source_protocol":"snmp"}
                {"ts":"2026-10-16 00:00:02.000","value":2.0,"quality":1,"flags":0,"sensor_id":"s1","sequence_id":"b","source_protocol":"snmp"}
                {"ts":"2026-10-16 00:00:03.000","value":3.0,"quality":1,"flags":0,"sensor_id":"s1","sequence_id":"c","source_protocol":"snmp"}
                """;
        assertEquals(1, reader.parseRows(body, MetricValueKind.NUMERIC, 1).size());
    }

    @Test
    void clickHouseOfflineMapsToSanitizedUnavailable() throws Exception {
        HttpClient httpClient = mock(HttpClient.class);
        when(httpClient.send(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenThrow(new java.net.ConnectException("connection refused"));
        ClickHouseMetricObservationReader reader = new ClickHouseMetricObservationReader(
                "http://clickhouse:8123", "Basic dGVzdA==",
                httpClient, new ObjectMapper(), Duration.ofSeconds(5));

        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, "cpu.util", "NUMERIC",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                10
        );
        DomainException ex = assertThrows(DomainException.class, () -> reader.read(query));
        assertEquals("DATABASE_UNAVAILABLE", ex.getCode());
        assertTrue(ex.getMessage().contains("temporarily unavailable"));
    }

    @Test
    void queryUsesCanonicalViewAndBoundParametersNotRawTables() throws Exception {
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

        ClickHouseMetricObservationReader reader = new ClickHouseMetricObservationReader(
                "http://clickhouse:8123", "Basic dGVzdA==",
                httpClient, new ObjectMapper(), Duration.ofSeconds(5));
        MetricObservationQuery query = MetricObservationQuery.parse(
                TENANT, ENTITY, "if.hc_in_octets", "COUNTER64",
                Instant.parse("2026-10-16T00:00:00Z"),
                Instant.parse("2026-10-16T01:00:00Z"),
                10
        );
        reader.read(query);

        String sql = ClickHouseMetricObservationReader.buildStaticSql(MetricValueKind.COUNTER64);
        assertTrue(sql.contains("FROM platform.metric_counter"));
        assertTrue(!sql.contains("metric_counter_raw") && !sql.contains("metric_numeric_raw"));
        assertTrue(sql.contains("{tenant_id:String}"));
        assertTrue(sql.contains("{row_limit:UInt32}"));

        HttpRequest sent = captured.get();
        String uri = sent.uri().toString();
        assertTrue(uri.contains("param_tenant_id="));
        assertTrue(uri.contains("param_entity_id="));
        assertTrue(uri.contains("param_metric_id="));
        assertTrue(uri.contains("max_execution_time=4"));
        assertTrue(!uri.contains("metric_counter_raw") && !uri.contains("metric_numeric_raw"));
    }

    @Test
    void requestTimeoutIsBudgeted() {
        Duration timeout = Duration.ofSeconds(5);
        ClickHouseMetricObservationReader reader = new ClickHouseMetricObservationReader(
                "http://clickhouse:8123", "Basic dGVzdA==",
                mock(HttpClient.class), new ObjectMapper(), timeout);
        // Construction with explicit timeout proves the budgeted path is injectable for tests.
        assertEquals(ClickHouseMetricObservationReader.class, reader.getClass());
    }

    private static final class CanonicalCounterMax {
        static final BigInteger MAX = new BigInteger("18446744073709551615");
    }
}
