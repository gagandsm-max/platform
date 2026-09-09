package com.platform.core.ingestion;

import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;

@Component
public class IngestionMetrics {
    private final AtomicLong batchesReceived = new AtomicLong(0);
    private final AtomicLong batchesRejected = new AtomicLong(0);
    private final AtomicLong batchesReplayed = new AtomicLong(0);
    private final AtomicLong recordsAccepted = new AtomicLong(0);

    private final AtomicLong kafkaPublishes = new AtomicLong(0);
    private final AtomicLong kafkaPublishTotalMs = new AtomicLong(0);
    private final AtomicLong kafkaFailures = new AtomicLong(0);

    private final AtomicLong clickHouseInserts = new AtomicLong(0);
    private final AtomicLong clickHouseInsertTotalMs = new AtomicLong(0);
    private final AtomicLong clickHouseFailures = new AtomicLong(0);

    private final AtomicLong consumerPoisonEvents = new AtomicLong(0);
    private final AtomicBoolean consumerDegraded = new AtomicBoolean(false);
    private final AtomicReference<String> consumerDegradedReason = new AtomicReference<>("");

    private final AtomicReference<Instant> lastPersistenceTimestamp = new AtomicReference<>(null);
    private final AtomicReference<String> lastError = new AtomicReference<>("");

    public void recordBatchReceived() {
        batchesReceived.incrementAndGet();
    }

    public void recordBatchRejected(String errorCode) {
        batchesRejected.incrementAndGet();
        lastError.set(errorCode);
    }

    public void recordBatchReplayed() {
        batchesReplayed.incrementAndGet();
    }

    public void recordDuplicateReplay() {
        recordBatchReplayed();
    }

    public void recordKafkaPublish(int count, long durationMs) {
        recordsAccepted.addAndGet(count);
        kafkaPublishes.incrementAndGet();
        kafkaPublishTotalMs.addAndGet(durationMs);
        lastPersistenceTimestamp.set(Instant.now());
    }

    public void recordKafkaFailure(String error) {
        kafkaFailures.incrementAndGet();
        lastError.set(error);
    }

    public void recordClickHouseInsert(long count, long durationMs) {
        clickHouseInserts.addAndGet(count);
        clickHouseInsertTotalMs.addAndGet(durationMs);
        lastPersistenceTimestamp.set(Instant.now());
    }

    public void recordClickHouseFailure(String error) {
        clickHouseFailures.incrementAndGet();
        lastError.set(error);
    }

    public void recordConsumerPoisonEvent(String topic, long offset, String error) {
        consumerPoisonEvents.incrementAndGet();
        consumerDegraded.set(true);
        consumerDegradedReason.set("Poison event on " + topic + " offset " + offset + ": " + error);
        lastError.set("ERR_KAFKA_POISON_EVENT");
    }

    public void recordConsumerDegraded(String reason) {
        consumerDegraded.set(true);
        consumerDegradedReason.set(reason);
    }

    public void recordConsumerHealthy() {
        consumerDegraded.set(false);
        consumerDegradedReason.set("");
    }

    public long getBatchesReceived() { return batchesReceived.get(); }
    public long getBatchesRejected() { return batchesRejected.get(); }
    public long getBatchesReplayed() { return batchesReplayed.get(); }
    public long getRecordsAccepted() { return recordsAccepted.get(); }

    public long getKafkaPublishes() { return kafkaPublishes.get(); }
    public long getKafkaPublishTotalMs() { return kafkaPublishTotalMs.get(); }
    public long getKafkaFailures() { return kafkaFailures.get(); }

    public long getClickHouseInserts() { return clickHouseInserts.get(); }
    public long getClickHouseInsertTotalMs() { return clickHouseInsertTotalMs.get(); }
    public long getClickHouseFailures() { return clickHouseFailures.get(); }

    public long getConsumerPoisonEvents() { return consumerPoisonEvents.get(); }
    public boolean isConsumerDegraded() { return consumerDegraded.get(); }
    public String getConsumerDegradedReason() { return consumerDegradedReason.get(); }

    public Instant getLastPersistenceTimestamp() { return lastPersistenceTimestamp.get(); }
    public String getLastError() { return lastError.get(); }
}
