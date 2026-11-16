package com.platform.core.ingestion;

import org.apache.kafka.clients.consumer.ConsumerConfig;
import org.junit.jupiter.api.Test;
import org.springframework.boot.autoconfigure.kafka.KafkaProperties;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

class TelemetryProcessingBudgetTest {
    @Test
    void defaultsMatchEffectiveKafkaPropertiesAndHttpTimeout() {
        var factory = IngestionKafkaConfiguration.createPinnedTelemetryConsumerFactory(new KafkaProperties());
        var budget = new TelemetryProcessingBudget(factory.getConfigurationProperties(), 5, 200);
        assertEquals(300_000, budget.maxPollIntervalMs);
        assertEquals(1, budget.maxPollRecords);
        assertEquals(59_000, budget.recordProcessingMs);
        assertEquals(10_000, ClickHouseClient.INSERT_REQUEST_TIMEOUT_MS);
    }

    @Test
    void effectiveConsumerOverridesTakePrecedenceOverGlobalProperties() {
        var props = new KafkaProperties();
        props.getProperties().put(ConsumerConfig.MAX_POLL_INTERVAL_MS_CONFIG, "600000");
        props.getConsumer().getProperties().put(ConsumerConfig.MAX_POLL_INTERVAL_MS_CONFIG, "500");
        var factory = IngestionKafkaConfiguration.createPinnedTelemetryConsumerFactory(props);
        assertThrows(IllegalArgumentException.class, () ->
                new TelemetryProcessingBudget(factory.getConfigurationProperties(), 2, 2000));
    }

    @Test
    void wholePollAndCommitReserveMustFitStrictly() {
        // 1 attempt = 12s processing, plus 5s commit, plus 5s poll = 22s.
        assertThrows(IllegalArgumentException.class, () -> budget(22_000, 1, 1, 0));
        assertDoesNotThrow(() -> budget(22_001, 1, 1, 0));
        assertDoesNotThrow(() -> budget(300_000, 4, 5, 200));
        assertThrows(IllegalArgumentException.class, () -> budget(300_000, 5, 5, 200));
    }

    @Test
    void safeLargerAttemptsAndBackoffsAreNotArbitrarilyCapped() {
        assertDoesNotThrow(() -> budget(300_000, 1, 6, 1001));
        assertDoesNotThrow(() -> budget(Integer.MAX_VALUE, 1, 100_000, 0));
        assertDoesNotThrow(() -> budget(300_000, 1, 1, Long.MAX_VALUE));
    }

    @Test
    void extremeValuesRejectWithoutOverflowOrAttemptSizedLoop() {
        assertTimeoutPreemptively(java.time.Duration.ofSeconds(2), () -> {
            for (int attempts : new int[] {64, 65, Integer.MAX_VALUE}) {
                for (long delay : new long[] {Long.MAX_VALUE, Long.MAX_VALUE / 2, 1}) {
                    assertThrows(IllegalArgumentException.class, () -> budget(Integer.MAX_VALUE, 1, attempts, delay));
                }
            }
            assertThrows(IllegalArgumentException.class, () -> budget(Integer.MAX_VALUE, 1, 2, Long.MAX_VALUE));
            assertThrows(IllegalArgumentException.class, () -> budget(Integer.MAX_VALUE, Integer.MAX_VALUE, Integer.MAX_VALUE, 0));
        });
    }

    private TelemetryProcessingBudget budget(int interval, int records, int attempts, long backoff) {
        return new TelemetryProcessingBudget(Map.of(ConsumerConfig.MAX_POLL_INTERVAL_MS_CONFIG, interval,
                ConsumerConfig.MAX_POLL_RECORDS_CONFIG, records), attempts, backoff);
    }
}
