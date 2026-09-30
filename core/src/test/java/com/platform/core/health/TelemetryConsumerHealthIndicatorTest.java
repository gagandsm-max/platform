package com.platform.core.health;

import com.platform.core.ingestion.IngestionMetrics;
import com.platform.core.ingestion.TelemetryConsumerHealthIndicator;
import com.platform.core.logging.CoreSupportabilityEventCodes;
import org.junit.jupiter.api.Test;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.Status;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;

class TelemetryConsumerHealthIndicatorTest {

    @Test
    void degradedHealthUsesStableReasonCodeNotRawDetail() {
        IngestionMetrics metrics = new IngestionMetrics();
        metrics.recordConsumerDegraded("Poison event on secret-topic offset 9: Authorization Bearer abc");
        TelemetryConsumerHealthIndicator indicator = new TelemetryConsumerHealthIndicator(metrics);

        Health health = indicator.health();
        assertEquals(Status.DOWN, health.getStatus());
        assertEquals(CoreSupportabilityEventCodes.TELEMETRY_POISON_EVENT, health.getDetails().get("reasonCode"));
        assertFalse(health.getDetails().containsKey("reason"));
        assertFalse(health.getDetails().toString().contains("Bearer"));
        assertFalse(health.getDetails().toString().contains("secret-topic"));
    }

    @Test
    void clickHouseUnavailableMapsToStableCode() {
        IngestionMetrics metrics = new IngestionMetrics();
        metrics.recordConsumerDegraded("ClickHouse unavailable after 2 attempts: connection refused");
        TelemetryConsumerHealthIndicator indicator = new TelemetryConsumerHealthIndicator(metrics);
        assertEquals(
                CoreSupportabilityEventCodes.TELEMETRY_CLICKHOUSE_UNAVAILABLE,
                indicator.health().getDetails().get("reasonCode")
        );
    }
}
