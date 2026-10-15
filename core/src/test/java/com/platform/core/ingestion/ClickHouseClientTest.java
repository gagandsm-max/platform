package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import com.sun.net.httpserver.HttpServer;
import org.junit.jupiter.params.ParameterizedTest;
import org.junit.jupiter.params.provider.CsvSource;

import java.net.InetSocketAddress;
import java.time.Instant;
import java.util.List;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;

class ClickHouseClientTest {
    @org.junit.jupiter.api.Test
    void realConnectionFailureIsRecoverable() throws Exception {
        int closedPort;
        try (var socket = new java.net.ServerSocket(0, 1, java.net.InetAddress.getLoopbackAddress())) {
            closedPort = socket.getLocalPort();
        }
        var metrics = new IngestionMetrics();
        var client = new ClickHouseClient("127.0.0.1", closedPort, "user", "PRIVATE_SENTINEL",
                new ObjectMapper().findAndRegisterModules(), metrics);
        var failure = assertThrows(java.net.ConnectException.class, () -> client.insertNumericBatch(rows()));
        assertEquals(TelemetryConsumerFailure.DEPENDENCY, TelemetryConsumerFailure.insertion(failure));
        assertEquals(1, metrics.getClickHouseFailures());
        assertEquals(TelemetryConsumerFailure.DEPENDENCY.reasonCode, metrics.getLastError());
    }

    @org.junit.jupiter.api.Test
    void realRequestTimeoutIsRecoverable() throws Exception {
        var release = new java.util.concurrent.CountDownLatch(1);
        var server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", exchange -> {
            try {
                exchange.getRequestBody().readAllBytes();
                release.await(15, java.util.concurrent.TimeUnit.SECONDS);
            } catch (InterruptedException interrupted) {
                Thread.currentThread().interrupt();
            } finally {
                exchange.close();
            }
        });
        server.start();
        try {
            var metrics = new IngestionMetrics();
            var client = new ClickHouseClient("127.0.0.1", server.getAddress().getPort(), "user", "PRIVATE_SENTINEL",
                    new ObjectMapper().findAndRegisterModules(), metrics);
            var failure = assertThrows(java.net.http.HttpTimeoutException.class, () -> client.insertNumericBatch(rows()));
            assertEquals(TelemetryConsumerFailure.DEPENDENCY, TelemetryConsumerFailure.insertion(failure));
            assertEquals(1, metrics.getClickHouseFailures());
            assertEquals(0, metrics.getClickHouseInserts());
        } finally {
            release.countDown();
            server.stop(0);
        }
    }

    private List<CanonicalObservation> rows() {
        return List.of(new CanonicalObservation("tenant", "entity", "metric", "sensor", Instant.now(),
                1, 1, 0, "1", "snmp"));
    }

    @ParameterizedTest
    @CsvSource({
            "503, , CORE_TELEMETRY_CLICKHOUSE_UNAVAILABLE",
            "502, , CORE_TELEMETRY_CLICKHOUSE_UNAVAILABLE",
            "504, , CORE_TELEMETRY_CLICKHOUSE_UNAVAILABLE",
            "429, , CORE_TELEMETRY_CLICKHOUSE_UNAVAILABLE",
            "500, 209, CORE_TELEMETRY_CLICKHOUSE_UNAVAILABLE",
            "401, , ERR_CLICKHOUSE_AUTHORIZATION",
            "403, , ERR_CLICKHOUSE_AUTHORIZATION",
            "500, 516, ERR_CLICKHOUSE_AUTHORIZATION",
            "500, 497, ERR_CLICKHOUSE_AUTHORIZATION",
            "404, 60, ERR_CLICKHOUSE_SCHEMA",
            "500, 53, ERR_CLICKHOUSE_SCHEMA",
            "503, 60, ERR_CLICKHOUSE_SCHEMA",
            "200, 60, ERR_CLICKHOUSE_SCHEMA",
            "500, , ERR_CLICKHOUSE_PERMANENT",
            "503, 999999, ERR_CLICKHOUSE_PERMANENT",
            "503, secret-header, ERR_CLICKHOUSE_PERMANENT"
    })
    void classifiesHttpFailureWithoutExposingResponse(int status, String code, String expected) throws Exception {
        var requests = new AtomicInteger();
        var server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", exchange -> {
            requests.incrementAndGet();
            exchange.getRequestBody().readAllBytes();
            if (code != null) {
                exchange.getResponseHeaders().set("X-ClickHouse-Exception-Code", code);
            }
            byte[] body = "Bearer PRIVATE_SENTINEL password=PRIVATE_SENTINEL payload=PRIVATE_SENTINEL".getBytes();
            exchange.sendResponseHeaders(status, body.length);
            try (var out = exchange.getResponseBody()) {
                out.write(body);
            }
        });
        server.start();
        try {
            var metrics = new IngestionMetrics();
            var client = new ClickHouseClient("127.0.0.1", server.getAddress().getPort(), "user", "PRIVATE_SENTINEL",
                    new ObjectMapper().findAndRegisterModules(), metrics);
            var failure = assertThrows(RuntimeException.class, () -> client.insertNumericBatch(List.of(
                    new CanonicalObservation("tenant", "entity", "metric", "sensor", Instant.now(),
                            1, 1, 0, "1", "snmp"))));
            assertTrue(failure.getMessage().contains(expected), failure.getMessage());
            assertFalse(failure.toString().contains("PRIVATE_SENTINEL"));
            assertFalse(failure.toString().contains("secret-header"));
            assertNull(failure.getCause());
            assertEquals(1, requests.get());
            assertEquals(1, metrics.getClickHouseFailures());
            assertEquals(0, metrics.getClickHouseInserts());
        } finally {
            server.stop(0);
        }
    }
}
