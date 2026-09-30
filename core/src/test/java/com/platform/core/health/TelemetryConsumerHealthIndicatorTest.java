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
    void degradedHealthUsesStoredStableReasonCode() {
        IngestionMetrics metrics = new IngestionMetrics();
        metrics.recordConsumerDegraded(CoreSupportabilityEventCodes.TELEMETRY_POISON_EVENT);
        TelemetryConsumerHealthIndicator indicator = new TelemetryConsumerHealthIndicator(metrics);

        Health health = indicator.health();
        assertEquals(Status.DOWN, health.getStatus());
        assertEquals(CoreSupportabilityEventCodes.TELEMETRY_POISON_EVENT, health.getDetails().get("reasonCode"));
        assertFalse(health.getDetails().containsKey("reason"));
        assertFalse(health.getDetails().toString().contains("Bearer"));
        assertFalse(health.getDetails().toString().contains("secret-topic"));
    }

    @Test
    void clickHouseUnavailableUsesStableCode() {
        IngestionMetrics metrics = new IngestionMetrics();
        metrics.recordConsumerDegraded(CoreSupportabilityEventCodes.TELEMETRY_CLICKHOUSE_UNAVAILABLE);
        TelemetryConsumerHealthIndicator indicator = new TelemetryConsumerHealthIndicator(metrics);
        assertEquals(
                CoreSupportabilityEventCodes.TELEMETRY_CLICKHOUSE_UNAVAILABLE,
                indicator.health().getDetails().get("reasonCode")
        );
    }

    @Test
    void poisonEventRecorderStoresStableCodesOnly() {
        IngestionMetrics metrics = new IngestionMetrics();
        metrics.recordConsumerPoisonEvent("secret-topic", 9L, "Authorization Bearer abc");
        TelemetryConsumerHealthIndicator indicator = new TelemetryConsumerHealthIndicator(metrics);
        Health health = indicator.health();
        assertEquals(CoreSupportabilityEventCodes.TELEMETRY_POISON_EVENT, health.getDetails().get("reasonCode"));
        assertEquals(CoreSupportabilityEventCodes.TELEMETRY_POISON_EVENT, metrics.getLastError());
        assertFalse(health.getDetails().toString().contains("Bearer"));
        assertFalse(health.getDetails().toString().contains("secret-topic"));
        assertFalse(metrics.getConsumerDegradedReason().contains("Bearer"));
    }
}
