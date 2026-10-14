package com.platform.core.ingestion;

import com.platform.core.logging.CoreSupportabilityEventCodes;
import org.springframework.beans.factory.ObjectProvider;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.kafka.config.KafkaListenerEndpointRegistry;
import org.springframework.boot.actuate.health.Health;
import org.springframework.boot.actuate.health.HealthIndicator;
import org.springframework.stereotype.Component;

import java.time.Instant;

@Component("telemetryConsumer")
public class TelemetryConsumerHealthIndicator implements HealthIndicator {

    private final IngestionMetrics metrics;
    private final ObjectProvider<KafkaListenerEndpointRegistry> registryProvider;

    public TelemetryConsumerHealthIndicator(IngestionMetrics metrics) {
        this(metrics, null);
    }

    @Autowired
    public TelemetryConsumerHealthIndicator(IngestionMetrics metrics,
                                           ObjectProvider<KafkaListenerEndpointRegistry> registryProvider) {
        this.metrics = metrics;
        this.registryProvider = registryProvider;
    }

    @Override
    public Health health() {
        // Query the actual Spring registration lazily. A successful partition can clear
        // shared metrics while a sibling's poison/exhaustion stops the whole listener.
        var registry = registryProvider == null ? null : registryProvider.getIfAvailable();
        var listener = registry == null ? null : registry.getListenerContainer("telemetryNumericListener");
        boolean listenerFailed = listener != null && !listener.isInExpectedState();
        if (listenerFailed || metrics.isConsumerDegraded()) {
            String reasonCode = metrics.getConsumerDegradedReason();
            if (reasonCode == null || reasonCode.isBlank()) {
                reasonCode = CoreSupportabilityEventCodes.TELEMETRY_CONSUMER_DEGRADED;
            }
            return Health.down()
                    .withDetail("status", "DEGRADED")
                    .withDetail("reasonCode", reasonCode)
                    .withDetail("poisonEvents", metrics.getConsumerPoisonEvents())
                    .withDetail("clickHouseFailures", metrics.getClickHouseFailures())
                    .withDetail("listenerFailed", listenerFailed)
                    .build();
        }

        Instant lastPersistence = metrics.getLastPersistenceTimestamp();
        Health.Builder builder = Health.up()
                .withDetail("status", listener != null && !listener.isRunning() ? "STOPPED" : "RUNNING")
                .withDetail("recordsAccepted", metrics.getRecordsAccepted())
                .withDetail("clickHouseInserts", metrics.getClickHouseInserts());

        if (lastPersistence != null) {
            builder.withDetail("lastPersistence", lastPersistence.toString());
        }

        return builder.build();
    }
}
