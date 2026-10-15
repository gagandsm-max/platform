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
        var failure = assertThrows(TelemetryConsumerFailure.InsertFailure.class, () -> client.insertNumericBatch(rows()));
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
            var failure = assertThrows(TelemetryConsumerFailure.InsertFailure.class, () -> client.insertNumericBatch(rows()));
            assertEquals(TelemetryConsumerFailure.DEPENDENCY, TelemetryConsumerFailure.insertion(failure));
            assertEquals(1, metrics.getClickHouseFailures());
            assertEquals(0, metrics.getClickHouseInserts());
        } finally {
            release.countDown();
            server.stop(0);
        }
    }

    @org.junit.jupiter.api.Test
    void lateExceptionAfter200NeverCountsSuccessOrAcknowledges() throws Exception {
        var server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", exchange -> {
            exchange.getRequestBody().readAllBytes();
            exchange.sendResponseHeaders(200, 0);
            exchange.getResponseBody().flush(); // Commit the status before the exception.
            exchange.getResponseBody().write("Code: 60. DB::Exception: PRIVATE_SENTINEL".getBytes());
            exchange.close();
        });
        server.start();
        try {
            var mapper = new ObjectMapper().findAndRegisterModules();
            var metrics = new IngestionMetrics();
            var client = new ClickHouseClient("127.0.0.1", server.getAddress().getPort(), "user", "secret", mapper, metrics);
            var consumer = new ClickHouseTelemetryConsumer(client, mapper, metrics, 2, 0);
            var ack = org.mockito.Mockito.mock(org.springframework.kafka.support.Acknowledgment.class);
            var record = new org.apache.kafka.clients.consumer.ConsumerRecord<String, String>(
                    "telemetry.numeric.v1", 0, 7L, "key", mapper.writeValueAsString(rows().getFirst()));
            var failure = assertThrows(RuntimeException.class, () -> consumer.consume(record, ack));
            assertEquals("ERR_CLICKHOUSE_UNKNOWN", metrics.getConsumerDegradedReason());
            assertEquals(0, metrics.getClickHouseInserts());
            assertEquals(1, metrics.getClickHouseFailures());
            org.mockito.Mockito.verifyNoInteractions(ack);
            assertFalse(failure.toString().contains("PRIVATE_SENTINEL"));
        } finally {
            server.stop(0);
        }
    }

    @org.junit.jupiter.api.Test
    void genericSendIOExceptionHasTrustedTransportProvenance() throws Exception {
        // Incomplete response headers cause HttpClient.send itself to throw a plain IOException.
        var server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", exchange -> {
            exchange.getRequestBody().readAllBytes();
            exchange.close();
        });
        server.start();
        try {
            var metrics = new IngestionMetrics();
            var client = new ClickHouseClient("127.0.0.1", server.getAddress().getPort(), "user", "secret",
                    new ObjectMapper().findAndRegisterModules(), metrics);
            var failure = assertThrows(Exception.class, () -> client.insertNumericBatch(rows()));
            assertEquals(TelemetryConsumerFailure.DEPENDENCY, TelemetryConsumerFailure.insertion(failure));
            assertEquals(0, metrics.getClickHouseInserts());
        } finally {
            server.stop(0);
        }
    }

    @org.junit.jupiter.api.Test
    void plainIOExceptionFromSendIsRetryableButJacksonIOExceptionIsNot() throws Exception {
        var http = org.mockito.Mockito.mock(java.net.http.HttpClient.class);
        org.mockito.Mockito.when(http.send(org.mockito.ArgumentMatchers.any(), org.mockito.ArgumentMatchers.any()))
                .thenThrow(new java.io.IOException("PRIVATE_SENTINEL"));
        var mapper = new ObjectMapper().findAndRegisterModules();
        var client = new ClickHouseClient("127.0.0.1", 1, "user", "secret", mapper, new IngestionMetrics());
        org.springframework.test.util.ReflectionTestUtils.setField(client, "httpClient", http);
        var failure = assertThrows(Exception.class, () -> client.insertNumericBatch(rows()));
        assertEquals(TelemetryConsumerFailure.DEPENDENCY, TelemetryConsumerFailure.insertion(failure));

        var brokenMapper = org.mockito.Mockito.mock(ObjectMapper.class);
        org.mockito.Mockito.when(brokenMapper.writeValueAsString(org.mockito.ArgumentMatchers.any()))
                .thenThrow(new com.fasterxml.jackson.core.JsonProcessingException("PRIVATE_SENTINEL") { });
        var local = new ClickHouseClient("127.0.0.1", 1, "user", "secret", brokenMapper, new IngestionMetrics());
        org.springframework.test.util.ReflectionTestUtils.setField(local, "httpClient", http);
        var serialization = assertThrows(Exception.class, () -> local.insertNumericBatch(rows()));
        assertEquals(TelemetryConsumerFailure.PERMANENT, TelemetryConsumerFailure.insertion(serialization));
        org.mockito.Mockito.verify(http, org.mockito.Mockito.times(1))
                .send(org.mockito.ArgumentMatchers.any(), org.mockito.ArgumentMatchers.any());
    }

    @org.junit.jupiter.api.Test
    void emptyCompleted200SucceedsForNumericAndExactCounter() throws Exception {
        var queries = new java.util.concurrent.CopyOnWriteArrayList<String>();
        var payloads = new java.util.concurrent.CopyOnWriteArrayList<String>();
        var server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", exchange -> {
            queries.add(exchange.getRequestURI().getQuery());
            payloads.add(new String(exchange.getRequestBody().readAllBytes(), java.nio.charset.StandardCharsets.UTF_8));
            exchange.getResponseHeaders().set("X-ClickHouse-Exception-Code", "0");
            exchange.sendResponseHeaders(200, -1);
            exchange.close();
        });
        server.start();
        try {
            var metrics = new IngestionMetrics();
            var client = new ClickHouseClient("127.0.0.1", server.getAddress().getPort(), "user", "secret",
                    new ObjectMapper().findAndRegisterModules(), metrics);
            client.insertNumericBatch(rows());
            client.insertCounterBatch(List.of(new com.platform.core.ingestion.dto.CanonicalCounterObservation(
                    "tenant", "entity", "metric", "sensor", Instant.now(),
                    new java.math.BigInteger("18446744073709551615"), 1, 0, "1", "snmp")));
            assertEquals(2, metrics.getClickHouseInserts());
            assertEquals(0, metrics.getClickHouseFailures());
            assertTrue(queries.stream().allMatch(q -> q.contains("wait_end_of_query=1") && q.contains("async_insert=0")));
            assertTrue(payloads.get(1).contains("\"value\":\"18446744073709551615\""));
        } finally {
            server.stop(0);
        }
    }

    @org.junit.jupiter.api.Test
    void stalledBodyAfter200HeadersStillHasWholeRequestDeadline() throws Exception {
        var release = new java.util.concurrent.CountDownLatch(1);
        var server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", exchange -> {
            exchange.getRequestBody().readAllBytes();
            exchange.sendResponseHeaders(200, 0);
            exchange.getResponseBody().flush();
            try { release.await(15, java.util.concurrent.TimeUnit.SECONDS); }
            catch (InterruptedException interrupted) { Thread.currentThread().interrupt(); }
            finally { exchange.close(); }
        });
        server.start();
        try {
            var metrics = new IngestionMetrics();
            var client = new ClickHouseClient("127.0.0.1", server.getAddress().getPort(), "user", "secret",
                    new ObjectMapper().findAndRegisterModules(), metrics);
            long start = System.nanoTime();
            var failure = assertThrows(Exception.class, () -> client.insertNumericBatch(rows()));
            assertEquals(TelemetryConsumerFailure.DEPENDENCY, TelemetryConsumerFailure.insertion(failure));
            assertTrue(java.util.concurrent.TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - start) < 12_000);
            assertEquals(0, metrics.getClickHouseInserts());
            assertEquals(1, metrics.getClickHouseFailures());
        } finally {
            release.countDown();
            server.stop(0);
        }
    }

    @org.junit.jupiter.params.ParameterizedTest
    @org.junit.jupiter.params.provider.ValueSource(strings = {"\n", "Code: 60. DB::Exception: PRIVATE_SENTINEL", "Ok."})
    void anyUnexpected200BodyFailsClosed(String text) throws Exception {
        var server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", exchange -> {
            exchange.getRequestBody().readAllBytes();
            exchange.sendResponseHeaders(200, 0);
            try (var out = exchange.getResponseBody()) {
                // A large or endless error must not force the client to buffer/drain it.
                for (int i = 0; i < 100_000; i++) out.write(text.getBytes());
            } catch (java.io.IOException cancelled) { /* Expected client cancellation. */ }
            finally { exchange.close(); }
        });
        server.start();
        try {
            var metrics = new IngestionMetrics();
            var client = new ClickHouseClient("127.0.0.1", server.getAddress().getPort(), "user", "secret",
                    new ObjectMapper().findAndRegisterModules(), metrics);
            var failure = assertThrows(TelemetryConsumerFailure.InsertFailure.class, () -> client.insertNumericBatch(rows()));
            assertEquals(TelemetryConsumerFailure.UNKNOWN, failure.reason);
            assertFalse(failure.toString().contains("PRIVATE_SENTINEL"));
            assertEquals(0, metrics.getClickHouseInserts());
        } finally {
            server.stop(0);
        }
    }

    @org.junit.jupiter.api.Test
    void noSuccessOrAckBeforeEmptyResponseCompletes() throws Exception {
        var headersSent = new java.util.concurrent.CountDownLatch(1);
        var release = new java.util.concurrent.CountDownLatch(1);
        var server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", exchange -> {
            exchange.getRequestBody().readAllBytes();
            exchange.sendResponseHeaders(200, 0);
            exchange.getResponseBody().flush();
            headersSent.countDown();
            try { release.await(5, java.util.concurrent.TimeUnit.SECONDS); }
            catch (InterruptedException interrupted) { Thread.currentThread().interrupt(); }
            finally { exchange.close(); }
        });
        server.start();
        try {
            var mapper = new ObjectMapper().findAndRegisterModules();
            var metrics = new IngestionMetrics();
            var client = new ClickHouseClient("127.0.0.1", server.getAddress().getPort(), "user", "secret", mapper, metrics);
            var consumer = new ClickHouseTelemetryConsumer(client, mapper, metrics, 1, 0);
            var ack = org.mockito.Mockito.mock(org.springframework.kafka.support.Acknowledgment.class);
            var record = new org.apache.kafka.clients.consumer.ConsumerRecord<String, String>(
                    "telemetry.numeric.v1", 0, 7L, "key", mapper.writeValueAsString(rows().getFirst()));
            var work = java.util.concurrent.CompletableFuture.runAsync(() -> consumer.consume(record, ack));
            assertTrue(headersSent.await(3, java.util.concurrent.TimeUnit.SECONDS));
            assertFalse(work.isDone());
            assertEquals(0, metrics.getClickHouseInserts());
            org.mockito.Mockito.verifyNoInteractions(ack);
            release.countDown();
            work.get(5, java.util.concurrent.TimeUnit.SECONDS);
            assertEquals(1, metrics.getClickHouseInserts());
            org.mockito.Mockito.verify(ack).acknowledge();
        } finally {
            release.countDown();
            server.stop(0);
        }
    }

    @org.junit.jupiter.api.Test
    void sendTlsFailureRetainsConfigurationClassification() throws Exception {
        var http = org.mockito.Mockito.mock(java.net.http.HttpClient.class);
        org.mockito.Mockito.when(http.send(org.mockito.ArgumentMatchers.any(), org.mockito.ArgumentMatchers.any()))
                .thenThrow(new javax.net.ssl.SSLHandshakeException("PRIVATE_SENTINEL"));
        var mapper = new ObjectMapper().findAndRegisterModules();
        var metrics = new IngestionMetrics();
        var client = new ClickHouseClient("127.0.0.1", 1, "user", "secret", mapper, metrics);
        org.springframework.test.util.ReflectionTestUtils.setField(client, "httpClient", http);
        var consumer = new ClickHouseTelemetryConsumer(client, mapper, metrics, 2, 0);
        var ack = org.mockito.Mockito.mock(org.springframework.kafka.support.Acknowledgment.class);
        var record = new org.apache.kafka.clients.consumer.ConsumerRecord<String, String>(
                "telemetry.numeric.v1", 0, 7L, "key", mapper.writeValueAsString(rows().getFirst()));
        var failure = assertThrows(RuntimeException.class, () -> consumer.consume(record, ack));
        assertEquals(TelemetryConsumerFailure.CONFIGURATION.reasonCode, metrics.getConsumerDegradedReason());
        org.mockito.Mockito.verify(http).send(org.mockito.ArgumentMatchers.any(), org.mockito.ArgumentMatchers.any());
        org.mockito.Mockito.verifyNoInteractions(ack);
        for (Throwable cursor = failure; cursor != null; cursor = cursor.getCause()) {
            assertFalse(cursor.toString().contains("PRIVATE_SENTINEL"));
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
            "500, , ERR_CLICKHOUSE_UNKNOWN",
            "500, 0, ERR_CLICKHOUSE_UNKNOWN",
            "500, 999999, ERR_CLICKHOUSE_UNKNOWN",
            "503, 999999, ERR_CLICKHOUSE_UNKNOWN",
            "503, secret-header, ERR_CLICKHOUSE_UNKNOWN"
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
