package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalCounterObservation;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.time.Instant;
import java.util.List;

import static org.junit.jupiter.api.Assertions.*;

/** Opt-in fixture: isolated ClickHouse 24.8 with deploy/docker/clickhouse/init.sql only. */
@EnabledIfEnvironmentVariable(named = "K11_CLICKHOUSE_HOST", matches = ".+")
class ClickHouseInsertCompletionTest {
    private final String host = System.getenv("K11_CLICKHOUSE_HOST");
    private final HttpClient http = HttpClient.newHttpClient();
    private final String authorization = "Basic " + java.util.Base64.getEncoder().encodeToString(
            "k11_fixture:k11_fixture_only".getBytes(java.nio.charset.StandardCharsets.UTF_8));

    private HttpResponse<String> query(String sql, String settings) throws Exception {
        return http.send(HttpRequest.newBuilder(URI.create("http://" + host + ":8123/?" + settings))
                .timeout(Duration.ofSeconds(10)).header("Authorization", authorization)
                .POST(HttpRequest.BodyPublishers.ofString(sql)).build(), HttpResponse.BodyHandlers.ofString());
    }

    @Test
    void synchronousInsertEmptyResponseExactCounterAndCanonicalReplay() throws Exception {
        assertTrue(query("SELECT version()", "").body().startsWith("24.8."));
        var raw = query("INSERT INTO platform.metric_numeric_raw VALUES "
                + "('fixture','entity','raw','sensor','2026-10-15 00:00:00',1,1,0,'raw','snmp')",
                "wait_end_of_query=1&async_insert=0");
        assertEquals(200, raw.statusCode());
        assertEquals("", raw.body(), "Successful synchronous INSERT produces no result body");
        assertEquals("0", raw.headers().firstValue("X-ClickHouse-Exception-Code").orElse("0"));
        var mapper = new ObjectMapper().findAndRegisterModules();
        var metrics = new IngestionMetrics();
        var client = new ClickHouseClient(host, 8123, "k11_fixture", "k11_fixture_only", mapper, metrics);
        var numeric = List.of(new CanonicalObservation("fixture", "entity", "numeric", "sensor",
                Instant.parse("2026-10-15T00:00:00Z"), 42, 1, 0, "exact-replay", "snmp"));
        var counter = List.of(new CanonicalCounterObservation("fixture", "entity", "counter", "sensor",
                Instant.parse("2026-10-15T00:00:00Z"), CanonicalCounterObservation.MAX_UINT64,
                1, 0, "exact-replay", "snmp"));
        client.insertNumericBatch(numeric);
        client.insertNumericBatch(numeric);
        client.insertCounterBatch(counter);
        client.insertCounterBatch(counter);
        assertEquals(4, metrics.getClickHouseInserts());
        assertEquals(0, metrics.getClickHouseFailures());
        assertEquals("1\t42\n", query("SELECT count(), any(value) FROM platform.metric_numeric "
                + "WHERE tenant_id='fixture' AND metric_id='numeric'", "").body());
        assertEquals("1\t18446744073709551615\n", query("SELECT count(), any(value) FROM platform.metric_counter "
                + "WHERE tenant_id='fixture' AND metric_id='counter'", "").body());
        System.out.println("K11_REAL_CH synchronous_insert status=200 body_bytes=0 numeric=42 counter=18446744073709551615 canonical_replay_count=1");
    }

    @Test
    void realLateExceptionAndWaitEndOfQueryComparison() throws Exception {
        String sql = "SELECT sleepEachRow(0.15), throwIf(number=3) FROM numbers(5)";
        var late = query(sql, "max_block_size=1&buffer_size=1&wait_end_of_query=0");
        System.out.printf("K11_REAL_CH late_select status=%d body_bytes=%d exception=%s%n",
                late.statusCode(), late.body().length(), late.body().contains("DB::Exception"));
        assertEquals(200, late.statusCode());
        assertTrue(late.body().contains("DB::Exception"));
        var buffered = query(sql, "max_block_size=1&wait_end_of_query=1");
        assertNotEquals(200, buffered.statusCode());
        assertTrue(buffered.body().contains("DB::Exception"));
        // INSERT has no normal result bytes; force progress headers before a later execution error.
        String insert = "INSERT INTO platform.metric_numeric_raw SELECT 'fixture','entity','late','sensor',"
                + "now64(3), sleepEachRow(0.15)+throwIf(number=3),1,0,toString(number),'snmp' FROM numbers(5)";
        var insertLate = query(insert, "max_block_size=1&max_threads=1&wait_end_of_query=0&"
                + "send_progress_in_http_headers=1&http_headers_progress_interval_ms=1");
        assertEquals(200, insertLate.statusCode());
        assertTrue(insertLate.body().contains("DB::Exception"));
        System.out.printf("K11_REAL_CH failed_insert status=%d body_bytes=%d exception=true%n",
                insertLate.statusCode(), insertLate.body().length());
        var insertBuffered = query(insert, "max_block_size=1&max_threads=1&wait_end_of_query=1&"
                + "send_progress_in_http_headers=1&http_headers_progress_interval_ms=1");
        assertTrue(insertBuffered.body().contains("DB::Exception"));
        System.out.printf("K11_REAL_CH wait_end_failed_insert status=%d body_bytes=%d exception=true%n",
                insertBuffered.statusCode(), insertBuffered.body().length());
    }
}
