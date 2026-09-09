package com.platform.core.controller;

import org.junit.jupiter.api.Test;
import org.springframework.boot.actuate.health.CompositeHealth;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthComponent;
import org.springframework.boot.actuate.health.HealthEndpoint;
import org.springframework.boot.actuate.health.Status;
import org.springframework.http.ResponseEntity;
import org.springframework.jdbc.core.JdbcTemplate;

import java.util.LinkedHashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.ArgumentMatchers.eq;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

/**
 * Defense-in-depth: connectivity payloads must stay status-bounded even if authorization later opens.
 */
class VersionControllerConnectivitySafetyTest {

    @Test
    void connectivityNeverReturnsBackendExceptionTextOrActuatorDetails() {
        JdbcTemplate jdbc = mock(JdbcTemplate.class);
        when(jdbc.queryForObject(anyString(), eq(Integer.class)))
                .thenThrow(new RuntimeException(
                        "Connection to jdbc:postgresql://secret-host:5432/platform failed: FATAL password"
                ));

        Map<String, HealthComponent> components = new LinkedHashMap<>();
        components.put("clickHouse", Health.down()
                .withDetail("endpoint", "http://clickhouse.internal:8123")
                .withDetail("error", "ConnectException: Connection refused")
                .withDetail("path", "/var/lib/clickhouse")
                .build());
        components.put("redis", Health.down()
                .withDetail("url", "redis://valkey:6379")
                .withDetail("error", "WRONGPASS invalid password")
                .build());
        components.put("kafka", Health.up().withDetail("bootstrap", "kafka:9092").build());

        CompositeHealth composite = mock(CompositeHealth.class);
        when(composite.getStatus()).thenReturn(Status.DOWN);
        when(composite.getComponents()).thenReturn(components);

        HealthEndpoint healthEndpoint = mock(HealthEndpoint.class);
        when(healthEndpoint.health()).thenReturn(composite);

        VersionController controller = new VersionController(healthEndpoint, jdbc);
        ResponseEntity<Map<String, Object>> response = controller.getConnectivityHealth();

        assertEquals(503, response.getStatusCode().value());
        String body = String.valueOf(response.getBody());
        assertFalse(body.contains("jdbc:postgresql"));
        assertFalse(body.contains("secret-host"));
        assertFalse(body.contains("FATAL password"));
        assertFalse(body.contains("clickhouse.internal"));
        assertFalse(body.contains("ConnectException"));
        assertFalse(body.contains("/var/lib/clickhouse"));
        assertFalse(body.contains("WRONGPASS"));
        assertFalse(body.contains("redis://"));
        assertFalse(body.contains("bootstrap"));
        assertFalse(body.contains("\"details\""));
        assertFalse(body.contains("\"error\""));
        assertFalse(body.contains("\"endpoint\""));
        assertFalse(body.contains("\"url\""));
        assertFalse(body.contains("\"path\""));

        @SuppressWarnings("unchecked")
        Map<String, Object> services = (Map<String, Object>) response.getBody().get("services");
        assertEquals("DOWN", ((Map<?, ?>) services.get("postgres")).get("status"));
        assertEquals("DOWN", ((Map<?, ?>) services.get("clickhouse")).get("status"));
        assertEquals("DOWN", ((Map<?, ?>) services.get("valkey")).get("status"));
        assertEquals("UP", ((Map<?, ?>) services.get("kafka")).get("status"));
        assertEquals(1, ((Map<?, ?>) services.get("postgres")).size());
        assertEquals(1, ((Map<?, ?>) services.get("clickhouse")).size());
        assertTrue(response.getBody().containsKey("status"));
        assertFalse(response.getBody().containsKey("components"));
    }
}
