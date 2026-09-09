package com.platform.core.ingestion;

import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Component;

import java.time.Instant;

@Component("telemetryConsumer")
public class TelemetryConsumerHealthIndicator implements HealthIndicator {

    private final IngestionMetrics metrics;

    public TelemetryConsumerHealthIndicator(IngestionMetrics metrics) {
        this.metrics = metrics;
    }

    @Override
    public Health health() {
        if (metrics.isConsumerDegraded()) {
            return Health.down()
                    .withDetail("status", "DEGRADED")
                    .withDetail("reason", metrics.getConsumerDegradedReason())
                    .withDetail("poisonEvents", metrics.getConsumerPoisonEvents())
                    .withDetail("clickHouseFailures", metrics.getClickHouseFailures())
                    .build();
        }

        Instant lastPersistence = metrics.getLastPersistenceTimestamp();
        Health.Builder builder = Health.up()
                .withDetail("status", "RUNNING")
                .withDetail("recordsAccepted", metrics.getRecordsAccepted())
                .withDetail("clickHouseInserts", metrics.getClickHouseInserts());

        if (lastPersistence != null) {
            builder.withDetail("lastPersistence", lastPersistence.toString());
        }

        return builder.build();
    }
}
