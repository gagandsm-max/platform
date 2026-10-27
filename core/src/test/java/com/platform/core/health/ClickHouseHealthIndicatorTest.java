package com.platform.core.health;

import com.platform.core.logging.CoreSupportabilityEventCodes;
import org.junit.jupiter.api.Test;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.Status;
import org.springframework.test.util.ReflectionTestUtils;

import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertNotEquals;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

class ClickHouseHealthIndicatorTest {

    @Test
    void downHealthExposesStableReasonCodeWithoutEndpointOrBody() throws Exception {
        HttpClient client = mock(HttpClient.class);
        when(client.send(any(HttpRequest.class), any(HttpResponse.BodyHandler.class)))
                .thenThrow(new RuntimeException("secret connection detail"));

        ClickHouseHealthIndicator indicator = new ClickHouseHealthIndicator(client, "http://clickhouse:8123");
        Health health = indicator.health();

        assertEquals(Status.DOWN, health.getStatus());
        assertEquals("clickHouse", health.getDetails().get("dependency"));
        assertEquals(CoreSupportabilityEventCodes.CLICKHOUSE_HEALTH_PROBE_FAILED, health.getDetails().get("reasonCode"));
        assertFalse(health.getDetails().containsKey("endpoint"));
        assertFalse(health.getDetails().containsKey("response"));
        assertFalse(health.getDetails().containsKey("error"));
        assertFalse(health.getDetails().toString().toLowerCase().contains("password"));
        assertFalse(health.getDetails().toString().contains("secret connection detail"));
    }

    @Test
    void kafkaFailedOrIncompleteHealthOmitsBootstrapServers() {
        KafkaHealthIndicator indicator = new KafkaHealthIndicator();
        try {
            ReflectionTestUtils.setField(indicator, "bootstrapServers", "kafka:9092");
            // Unreachable/slow bootstrap in the unit harness must not publish UP or leak
            // bootstrap identity. DOWN = confirmed failure; UNKNOWN = incomplete probe evidence.
            Health health = indicator.health();
            assertNotEquals(Status.UP, health.getStatus());
            assertEquals(CoreSupportabilityEventCodes.KAFKA_HEALTH_PROBE_FAILED, health.getDetails().get("reasonCode"));
            assertFalse(health.getDetails().containsKey("bootstrapServers"));
            assertFalse(health.getDetails().containsKey("error"));
            assertFalse(health.toString().contains("kafka:9092"));
        } finally {
            indicator.destroy();
        }
    }
}
