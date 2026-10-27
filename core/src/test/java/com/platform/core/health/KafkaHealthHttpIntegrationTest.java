package com.platform.core.health;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.security.http.AuthorizationEnforcementFilter;
import com.platform.core.security.http.AuthorizationRouteMapper;
import com.platform.core.security.http.HttpSecurityConfiguration;
import com.platform.core.security.http.JwtDecoderConfiguration;
import com.platform.core.security.http.PlatformJwtAuthenticationConverter;
import com.platform.core.security.http.SecurityAuditEmitter;
import com.platform.core.security.http.SecurityDirectoryConfiguration;
import com.platform.core.security.http.SecurityErrorResponseWriter;
import org.apache.kafka.clients.admin.AdminClient;
import org.apache.kafka.clients.admin.DescribeClusterResult;
import org.apache.kafka.common.KafkaFuture;
import org.apache.kafka.common.Node;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import org.springframework.boot.WebApplicationType;
import org.springframework.boot.actuate.health.HealthEndpoint;
import org.springframework.boot.autoconfigure.EnableAutoConfiguration;
import org.springframework.boot.autoconfigure.data.redis.RedisAutoConfiguration;
import org.springframework.boot.autoconfigure.data.redis.RedisRepositoriesAutoConfiguration;
import org.springframework.boot.autoconfigure.jdbc.DataSourceAutoConfiguration;
import org.springframework.boot.autoconfigure.kafka.KafkaAutoConfiguration;
import org.springframework.boot.builder.SpringApplicationBuilder;
import org.springframework.boot.web.servlet.context.ServletWebServerApplicationContext;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Import;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.List;
import java.util.Map;
import java.util.Properties;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.*;

/**
 * Real TCP -> embedded servlet -> production security -> actual Actuator health endpoint
 * -> production KafkaHealthIndicator. Only AdminClient construction/results are controlled.
 * HTTP is test-only on container loopback, with no published host port or service stack.
 * This proves the controlled-constructor HTTP boundary, not physical broker recovery,
 * readiness-group membership, or forced release of permanently blocked native DNS.
 */
@Timeout(30)
class KafkaHealthHttpIntegrationTest {
    @Test
    void completedHttp503WhileConstructionIsHeldThen200AfterFreshValidatedRecovery() throws Exception {
        try (var context = (ServletWebServerApplicationContext) new SpringApplicationBuilder(TestApplication.class)
                .web(WebApplicationType.SERVLET)
                .run("--server.address=127.0.0.1", "--server.port=0", "--server.ssl.enabled=false",
                        "--server.forward-headers-strategy=none", "--management.server.port=0",
                        "--management.health.defaults.enabled=false",
                        "--platform.security.directories.persistence-enabled=false",
                        "--platform.security.oidc.issuer-uri=", "--platform.security.oidc.audience=")) {
            var fixture = context.getBean(ControlledFactory.class);
            var indicator = context.getBean("kafka", KafkaHealthIndicator.class);
            assertFalse(mockingDetails(indicator).isMock());
            assertFalse(mockingDetails(context.getBean(HealthEndpoint.class)).isMock());
            // management.server.port=0 equals server.port=0: one isolated embedded server.
            int port = context.getWebServer().getPort();
            try (HttpClient client = HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(2))
                    .followRedirects(HttpClient.Redirect.NEVER).build()) {
                try {
                    assertWireStatus(client, port, "/actuator/health", 503, "OUT_OF_SERVICE");
                    assertEquals(0, fixture.entered.getCount(), "production worker must enter controlled create");
                    assertEquals(1, fixture.release.getCount(), "503 must complete while create is still held");
                    assertEquals(1, fixture.creates.get());
                    assertTrue(indicator.hasInFlightProbe());
                    assertWireStatus(client, port, "/actuator/health", 503, "OUT_OF_SERVICE");
                    assertEquals(1, fixture.creates.get(), "abandoned create must not multiply workers");

                    fixture.release.countDown();
                    assertTrue(fixture.closed.await(2, TimeUnit.SECONDS), "late returned client must close");
                    long deadline = System.nanoTime() + TimeUnit.SECONDS.toNanos(2);
                    while (indicator.hasInFlightProbe() && System.nanoTime() < deadline) Thread.sleep(10);
                    assertFalse(indicator.hasInFlightProbe(), "abandoned probe must finish before fresh recovery");
                    assertWireStatus(client, port, "/actuator/health", 200, "UP");
                    assertEquals(2, fixture.creates.get(), "200 requires a fresh validated probe");
                    assertEquals(2, fixture.closes.get());
                    assertTrue(indicator.probePoolSize() <= 1);

                    // Production exact public-probe allowlist and hidden details stay effective.
                    assertEquals(401, request(client, port, "/api/v1/health/connectivity").statusCode());
                    assertEquals(403, request(client, port, "/actuator/health/kafka").statusCode());
                    verify(fixture.firstAdmin, never()).describeCluster();
                    verify(fixture.firstAdmin).close(any(Duration.class));
                    verify(fixture.recoveryAdmin).describeCluster();
                    verify(fixture.recoveryAdmin).close(any(Duration.class));
                    System.out.println("ISSUE260_HTTP_SECURITY connectivity=401 component=403 details=hidden");
                } finally {
                    fixture.release.countDown();
                }
            }
        }
    }

    private static void assertWireStatus(HttpClient client, int port, String path, int status, String state)
            throws Exception {
        long started = System.nanoTime();
        HttpResponse<String> response = request(client, port, path);
        long elapsed = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started);
        System.out.printf("ISSUE260_HTTP completed=true port=%d path=%s status=%d elapsed_ms=%d body=%s%n",
                port, path, response.statusCode(), elapsed, response.body());
        assertEquals(status, response.statusCode());
        assertTrue(elapsed < 3500, "controlled fixture response must complete within its finite allowance");
        assertEquals(new ObjectMapper().valueToTree(Map.of("status", state)),
                new ObjectMapper().readTree(response.body()), "anonymous probe exposes status only");
        assertTrue(response.headers().firstValue("content-type").orElse("").contains("json"));
    }

    private static HttpResponse<String> request(HttpClient client, int port, String path) throws Exception {
        return client.send(HttpRequest.newBuilder(URI.create("http://127.0.0.1:" + port + path))
                .timeout(Duration.ofSeconds(5)).GET().build(), HttpResponse.BodyHandlers.ofString());
    }

    @Configuration(proxyBeanMethods = false)
    @EnableAutoConfiguration(exclude = {DataSourceAutoConfiguration.class, KafkaAutoConfiguration.class,
            RedisAutoConfiguration.class, RedisRepositoriesAutoConfiguration.class})
    @Import({HttpSecurityConfiguration.class, PlatformJwtAuthenticationConverter.class,
            AuthorizationRouteMapper.class, AuthorizationEnforcementFilter.class,
            SecurityErrorResponseWriter.class, SecurityAuditEmitter.class,
            SecurityDirectoryConfiguration.class, JwtDecoderConfiguration.class})
    static class TestApplication {
        @Bean ControlledFactory controlledFactory() { return new ControlledFactory(); }
        @Bean(name = "kafka")
        KafkaHealthIndicator kafka(ControlledFactory factory) {
            return new KafkaHealthIndicator("private-bootstrap-never-resolved:9092", factory::create);
        }
    }

    static final class ControlledFactory {
        final CountDownLatch entered = new CountDownLatch(1);
        final CountDownLatch release = new CountDownLatch(1);
        final CountDownLatch closed = new CountDownLatch(1);
        final AtomicInteger creates = new AtomicInteger();
        final AtomicInteger closes = new AtomicInteger();
        final AdminClient firstAdmin = mock(AdminClient.class);
        final AdminClient recoveryAdmin = mock(AdminClient.class);

        ControlledFactory() {
            DescribeClusterResult cluster = mock(DescribeClusterResult.class);
            when(recoveryAdmin.describeCluster()).thenReturn(cluster);
            when(cluster.nodes()).thenReturn(KafkaFuture.completedFuture(List.of(new Node(1, "private-node", 9092))));
            when(cluster.clusterId()).thenReturn(KafkaFuture.completedFuture("private-cluster-id"));
            doAnswer(call -> { closes.incrementAndGet(); closed.countDown(); return null; })
                    .when(firstAdmin).close(any(Duration.class));
            doAnswer(call -> { closes.incrementAndGet(); return null; })
                    .when(recoveryAdmin).close(any(Duration.class));
        }

        AdminClient create(Properties properties) {
            if (creates.incrementAndGet() == 1) {
                entered.countDown();
                try {
                    if (!release.await(10, TimeUnit.SECONDS)) throw new IllegalStateException("fixture release expired");
                } catch (InterruptedException interrupted) {
                    Thread.currentThread().interrupt();
                    throw new IllegalStateException("fixture interrupted", interrupted);
                }
                return firstAdmin;
            }
            return recoveryAdmin;
        }
    }
}
