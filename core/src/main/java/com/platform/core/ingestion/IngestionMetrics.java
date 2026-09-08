package com.platform.core.ingestion;

import org.springframework.stereotype.Component;

import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.AtomicReference;

@Component
public class IngestionMetrics {
    private final AtomicLong batchesReceived = new AtomicLong(0);
    private final AtomicLong batchesRejected = new AtomicLong(0);
    private final AtomicLong batchesReplayed = new AtomicLong(0);
    private final AtomicLong recordsAccepted = new AtomicLong(0);
    private final AtomicLong kafkaFailures = new AtomicLong(0);
    private final AtomicLong clickHouseInserts = new AtomicLong(0);
    private final AtomicLong clickHouseFailures = new AtomicLong(0);
    private final AtomicReference<String> lastError = new AtomicReference<>("");

    public void recordBatchReceived() { batchesReceived.incrementAndGet(); }
    public void recordBatchRejected(String error) {
        batchesRejected.incrementAndGet();
        lastError.set(error);
    }
    public void recordBatchReplayed() { batchesReplayed.incrementAndGet(); }
    public void recordDuplicateReplay() { recordBatchReplayed(); }
    public void recordRecordsAccepted(long count) { recordsAccepted.addAndGet(count); }
    public void recordKafkaPublish(int count, long durationMs) { recordRecordsAccepted(count); }
    public void recordKafkaFailure(String error) {
        kafkaFailures.incrementAndGet();
        lastError.set(error);
    }
    public void recordClickHouseInsert(long count) { clickHouseInserts.addAndGet(count); }
    public void recordClickHouseFailure(String error) {
        clickHouseFailures.incrementAndGet();
        lastError.set(error);
    }

    public long getBatchesReceived() { return batchesReceived.get(); }
    public long getBatchesRejected() { return batchesRejected.get(); }
    public long getBatchesReplayed() { return batchesReplayed.get(); }
    public long getRecordsAccepted() { return recordsAccepted.get(); }
    public long getKafkaFailures() { return kafkaFailures.get(); }
    public long getClickHouseInserts() { return clickHouseInserts.get(); }
    public long getClickHouseFailures() { return clickHouseFailures.get(); }
    public String getLastError() { return lastError.get(); }
}
