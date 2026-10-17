package com.platform.core.ingestion;

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
        // Query the actual registration; metrics alone cannot prove listener lifecycle health.
        var registry = registryProvider == null ? null : registryProvider.getIfAvailable();
        var listener = registry == null ? null : registry.getListenerContainer("telemetryNumericListener");
        boolean listenerFailed = listener != null && !listener.isInExpectedState();
        if (listenerFailed || metrics.isConsumerDegraded()) {
            String reasonCode = metrics.getConsumerDegradedReason();
            if (reasonCode == null || reasonCode.isBlank()) {
                reasonCode = TelemetryConsumerFailure.CONSUMER_FAILED.reasonCode;
            }
            var health = Health.down()
                    .withDetail("status", "DEGRADED")
                    .withDetail("reasonCode", reasonCode)
                    .withDetail("poisonEvents", metrics.getConsumerPoisonEvents())
                    .withDetail("clickHouseFailures", metrics.getClickHouseFailures())
                    .withDetail("listenerFailed", listenerFailed);
            var stop = metrics.getConsumerTerminalStop();
            if (stop != null) {
                var failure = stop.failure();
                health.withDetail("terminalStopStatus", stop.status())
                        .withDetail("terminalStopCompleted", stop.stopCompleted())
                        .withDetail("terminalStopEpoch", failure.stopEpoch())
                        .withDetail("terminalListener", failure.listener())
                        .withDetail("terminalChild", failure.child())
                        .withDetail("terminalTopic", failure.topic())
                        .withDetail("terminalPartition", failure.partition())
                        .withDetail("terminalOffset", failure.offset())
                        .withDetail("terminalReasonCode", failure.reason().reasonCode)
                        .withDetail("terminalInsertCompleted", failure.completion().insertCompleted())
                        .withDetail("terminalPersistenceUncertain", failure.completion().persistenceUncertain());
            }
            return health.build();
        }

        if (listener != null && !listener.isRunning()) {
            var reason = listener.isAutoStartup() ? TelemetryConsumerFailure.SHUTDOWN : TelemetryConsumerFailure.DISABLED;
            return (reason == TelemetryConsumerFailure.DISABLED ? Health.unknown() : Health.down())
                    .withDetail("status", reason == TelemetryConsumerFailure.DISABLED ? "DISABLED" : "STOPPED")
                    .withDetail("reasonCode", reason.reasonCode)
                    .withDetail("listenerFailed", false)
                    .build();
        }

        if (listener == null) {
            return Health.unknown().withDetail("status", "UNAVAILABLE")
                    .withDetail("reasonCode", TelemetryConsumerFailure.CONSUMER_FAILED.reasonCode).build();
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
