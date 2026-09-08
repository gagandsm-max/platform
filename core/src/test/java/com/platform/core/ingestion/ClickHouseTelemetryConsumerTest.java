package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.apache.kafka.clients.consumer.ConsumerRecord;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.time.Instant;

import static org.mockito.ArgumentMatchers.anyList;
import static org.mockito.Mockito.*;

class ClickHouseTelemetryConsumerTest {

    private ClickHouseClient clickHouseClient;
    private ObjectMapper objectMapper;
    private IngestionMetrics metrics;
    private ClickHouseTelemetryConsumer consumer;

    @BeforeEach
    void setUp() {
        clickHouseClient = mock(ClickHouseClient.class);
        objectMapper = new ObjectMapper().findAndRegisterModules();
        metrics = new IngestionMetrics();
        consumer = new ClickHouseTelemetryConsumer(clickHouseClient, objectMapper, metrics);
    }

    @Test
    void testConsumeInsertsIntoClickHouse() throws Exception {
        CanonicalObservation obs = new CanonicalObservation(
                "default", "entity-1", "metric-1", "sensor-net-01",
                Instant.now(), 99.5, 1, 0, "1", "snmp"
        );
        String json = objectMapper.writeValueAsString(obs);

        ConsumerRecord<String, String> record = new ConsumerRecord<>(
                "telemetry.numeric.v1", 0, 100L, "default:entity-1", json
        );

        consumer.consume(record);

        verify(clickHouseClient, times(1)).insertNumericBatch(anyList());
    }
}
