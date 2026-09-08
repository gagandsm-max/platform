package com.platform.core.ingestion;

import com.platform.core.ingestion.dto.TelemetryAckResponse;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;

import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;

@Component
public class IdempotencyTracker {
    private static final int MAX_ENTRIES = 10_000;

    public record CachedEntry(
            String sensorId,
            long batchSequence,
            String checksum,
            TelemetryAckResponse ackResponse,
            Instant timestamp
    ) {}

    private final Map<String, CachedEntry> cache = new LinkedHashMap<>(1024, 0.75f, true) {
        @Override
        protected boolean removeEldestEntry(Map.Entry<String, CachedEntry> eldest) {
            return size() > MAX_ENTRIES;
        }
    };

    public synchronized TelemetryAckResponse checkDuplicate(String idempotencyKey, String checksum) {
        if (idempotencyKey == null || idempotencyKey.isBlank()) {
            return null;
        }
        CachedEntry entry = cache.get(idempotencyKey);
        if (entry == null) {
            return null;
        }
        if (!entry.checksum().equalsIgnoreCase(checksum)) {
            throw new IngestionException(
                    HttpStatus.CONFLICT,
                    "ERR_IDEMPOTENCY_CONFLICT",
                    "Idempotency key '" + idempotencyKey + "' was previously used with a different payload checksum"
            );
        }
        return entry.ackResponse();
    }

    public synchronized void recordSuccess(
            String idempotencyKey,
            String sensorId,
            long batchSequence,
            String checksum,
            TelemetryAckResponse ackResponse
    ) {
        if (idempotencyKey == null || idempotencyKey.isBlank()) {
            return;
        }
        cache.put(idempotencyKey, new CachedEntry(sensorId, batchSequence, checksum, ackResponse, Instant.now()));
    }

    public synchronized void clear() {
        cache.clear();
    }

    public synchronized int size() {
        return cache.size();
    }
}
