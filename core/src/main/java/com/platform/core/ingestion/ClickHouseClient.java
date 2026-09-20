package com.platform.core.ingestion;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.ingestion.dto.CanonicalObservation;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.format.DateTimeFormatter;
import java.util.Base64;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;

@Component
public class ClickHouseClient {
    private static final Logger log = LoggerFactory.getLogger(ClickHouseClient.class);
    private static final DateTimeFormatter CH_TIMESTAMP_FORMAT = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss.SSS")
            .withZone(java.time.ZoneOffset.UTC);

    private final String baseUrl;
    private final String authHeader;
    private final HttpClient httpClient;
    private final ObjectMapper objectMapper;
    private final IngestionMetrics metrics;

    public ClickHouseClient(
            @Value("${platform.clickhouse.host:clickhouse}") String host,
            @Value("${platform.clickhouse.port:8123}") int port,
            @Value("${platform.clickhouse.user:platform_app}") String user,
            @Value("${platform.clickhouse.password:platform_dev_ch_password_change_in_prod}") String password,
            ObjectMapper objectMapper,
            IngestionMetrics metrics
    ) {
        this.baseUrl = "http://" + host + ":" + port;
        String creds = user + ":" + password;
        this.authHeader = "Basic " + Base64.getEncoder().encodeToString(creds.getBytes(StandardCharsets.UTF_8));
        this.httpClient = HttpClient.newBuilder()
                .connectTimeout(Duration.ofSeconds(5))
                .build();
        this.objectMapper = objectMapper;
        this.metrics = metrics;
    }

    public void insertNumericBatch(List<CanonicalObservation> observations) throws Exception {
        if (observations == null || observations.isEmpty()) {
            return;
        }

        long startTime = System.currentTimeMillis();
        StringBuilder body = new StringBuilder();
        for (CanonicalObservation obs : observations) {
            Objects.requireNonNull(obs.tenantId(), "Canonical observation tenantId must not be null");
            Objects.requireNonNull(obs.entityId(), "Canonical observation entityId must not be null");
            Objects.requireNonNull(obs.metricId(), "Canonical observation metricId must not be null");
            Objects.requireNonNull(obs.sensorId(), "Canonical observation sensorId must not be null");
            Objects.requireNonNull(obs.timestamp(), "Canonical observation timestamp must not be null");
            Objects.requireNonNull(obs.sequenceId(), "Canonical observation sequenceId must not be null");
            Objects.requireNonNull(obs.sourceProtocol(), "Canonical observation sourceProtocol must not be null");

            Map<String, Object> row = new LinkedHashMap<>();
            row.put("tenant_id", obs.tenantId());
            row.put("entity_id", obs.entityId());
            row.put("metric_id", obs.metricId());
            row.put("sensor_id", obs.sensorId());
            row.put("ts", CH_TIMESTAMP_FORMAT.format(obs.timestamp()));
            row.put("value", obs.value());
            row.put("quality", obs.quality());
            row.put("flags", obs.flags());
            row.put("sequence_id", obs.sequenceId());
            row.put("source_protocol", obs.sourceProtocol());

            body.append(objectMapper.writeValueAsString(row)).append("\n");
        }

        URI uri = URI.create(baseUrl + "/?query=INSERT+INTO+platform.metric_numeric_raw+FORMAT+JSONEachRow");
        HttpRequest request = HttpRequest.newBuilder()
                .uri(uri)
                .timeout(Duration.ofSeconds(10))
                .header("Authorization", authHeader)
                .header("Content-Type", "application/json")
                .POST(HttpRequest.BodyPublishers.ofString(body.toString(), StandardCharsets.UTF_8))
                .build();

        HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
        if (response.statusCode() != 200) {
            // ClickHouse response bodies can contain data-dependent diagnostics. Never propagate
            // those bodies into application exceptions, health state, or Kafka framework logs.
            String failureCode = "CLICKHOUSE_HTTP_" + response.statusCode();
            metrics.recordClickHouseFailure(failureCode);
            throw new RuntimeException("ClickHouse insert failed with status " + response.statusCode());
        }

        long durationMs = System.currentTimeMillis() - startTime;
        metrics.recordClickHouseInsert(observations.size(), durationMs);
        log.debug("Successfully inserted {} observations into ClickHouse platform.metric_numeric_raw ({}ms)",
                observations.size(), durationMs);
    }
}
