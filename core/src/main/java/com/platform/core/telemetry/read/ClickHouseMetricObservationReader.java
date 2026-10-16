package com.platform.core.telemetry.read;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.domain.DomainException;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.io.BufferedReader;
import java.io.StringReader;
import java.math.BigInteger;
import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.List;
import java.util.Objects;

/**
 * Bounded ClickHouse read accessor for canonical FINAL views only.
 * Does not share the ingestion write client; never queries {@code *_raw} tables.
 */
@Component
public class ClickHouseMetricObservationReader {

    private static final Logger log = LoggerFactory.getLogger(ClickHouseMetricObservationReader.class);
    private static final DateTimeFormatter CH_TS = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss.SSS")
            .withZone(ZoneOffset.UTC);
    private static final Duration DEFAULT_TIMEOUT = Duration.ofSeconds(5);
    private static final BigInteger MAX_UINT64 = new BigInteger("18446744073709551615");

    private final String baseUrl;
    private final String authHeader;
    private final HttpClient httpClient;
    private final ObjectMapper objectMapper;
    private final Duration requestTimeout;

    public ClickHouseMetricObservationReader(
            @Value("${platform.clickhouse.host:clickhouse}") String host,
            @Value("${platform.clickhouse.port:8123}") int port,
            @Value("${platform.clickhouse.user:platform_app}") String user,
            @Value("${platform.clickhouse.password:platform_dev_ch_password_change_in_prod}") String password,
            ObjectMapper objectMapper
    ) {
        this(
                "http://" + host + ":" + port,
                basicAuth(user, password),
                HttpClient.newBuilder().connectTimeout(Duration.ofSeconds(3)).build(),
                objectMapper,
                DEFAULT_TIMEOUT
        );
    }

    /** Package-visible for direct unit tests. */
    ClickHouseMetricObservationReader(
            String baseUrl,
            String authHeader,
            HttpClient httpClient,
            ObjectMapper objectMapper,
            Duration requestTimeout
    ) {
        this.baseUrl = Objects.requireNonNull(baseUrl, "baseUrl");
        this.authHeader = Objects.requireNonNull(authHeader, "authHeader");
        this.httpClient = Objects.requireNonNull(httpClient, "httpClient");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper");
        this.requestTimeout = Objects.requireNonNull(requestTimeout, "requestTimeout");
    }

    public List<MetricObservationRow> read(MetricObservationQuery query) {
        String sql = buildStaticSql(query.selector().valueKind());
        String uri = baseUrl
                + "/?param_tenant_id=" + enc(query.tenantId().toString())
                + "&param_entity_id=" + enc(query.entityId().toString())
                + "&param_metric_id=" + enc(query.selector().metricId())
                + "&param_from_ts=" + enc(CH_TS.format(query.fromInclusive()))
                + "&param_to_ts=" + enc(CH_TS.format(query.toExclusive()))
                + "&param_row_limit=" + query.limit()
                + "&output_format_json_quote_64bit_integers=1"
                + "&max_execution_time=4";

        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(uri))
                .timeout(requestTimeout)
                .header("Authorization", authHeader)
                .header("Content-Type", "text/plain; charset=UTF-8")
                .POST(HttpRequest.BodyPublishers.ofString(sql, StandardCharsets.UTF_8))
                .build();

        try {
            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
            if (response.statusCode() != 200) {
                log.warn("ClickHouse metric read failed status={}", response.statusCode());
                throw new DomainException("DATABASE_UNAVAILABLE", "Telemetry store temporarily unavailable");
            }
            return parseRows(response.body(), query.selector().valueKind(), query.limit());
        } catch (DomainException ex) {
            throw ex;
        } catch (InterruptedException ex) {
            Thread.currentThread().interrupt();
            throw new DomainException("DATABASE_UNAVAILABLE", "Telemetry store temporarily unavailable");
        } catch (Exception ex) {
            log.warn("ClickHouse metric read unavailable failureClass={}", ex.getClass().getSimpleName());
            throw new DomainException("DATABASE_UNAVAILABLE", "Telemetry store temporarily unavailable");
        }
    }

    /** Package-visible: static SQL uses canonical FINAL views only; parameters are never concatenated. */
    static String buildStaticSql(MetricValueKind kind) {
        String view = kind == MetricValueKind.COUNTER64
                ? "platform.metric_counter"
                : "platform.metric_numeric";
        return """
                SELECT tenant_id, entity_id, metric_id, sensor_id, ts, value, quality, flags, sequence_id, source_protocol
                FROM %s
                WHERE tenant_id = {tenant_id:String}
                  AND entity_id = {entity_id:String}
                  AND metric_id = {metric_id:String}
                  AND ts >= toDateTime64({from_ts:String}, 3, 'UTC')
                  AND ts < toDateTime64({to_ts:String}, 3, 'UTC')
                ORDER BY ts ASC, sequence_id ASC
                LIMIT {row_limit:UInt32}
                FORMAT JSONEachRow
                """.formatted(view).trim();
    }

    List<MetricObservationRow> parseRows(String body, MetricValueKind kind, int limit) throws Exception {
        List<MetricObservationRow> rows = new ArrayList<>();
        if (body == null || body.isBlank()) {
            return rows;
        }
        try (BufferedReader reader = new BufferedReader(new StringReader(body))) {
            String line;
            while ((line = reader.readLine()) != null) {
                if (line.isBlank()) {
                    continue;
                }
                if (rows.size() >= limit) {
                    break;
                }
                JsonNode node = objectMapper.readTree(line);
                Instant ts = parseTimestamp(node.get("ts"));
                int quality = node.path("quality").asInt(0);
                int flags = node.path("flags").asInt(0);
                String sensorId = textOrEmpty(node, "sensor_id");
                String sequenceId = textOrEmpty(node, "sequence_id");
                String sourceProtocol = textOrEmpty(node, "source_protocol");
                if (kind == MetricValueKind.COUNTER64) {
                    String counter = parseCounter64(node.get("value"));
                    rows.add(new MetricObservationRow(
                            ts, null, counter, quality, flags, sensorId, sequenceId, sourceProtocol));
                } else {
                    if (!node.has("value") || !node.get("value").isNumber()) {
                        throw new DomainException("VALIDATION_FAILED", "numeric observation value is malformed");
                    }
                    double value = node.get("value").asDouble();
                    rows.add(new MetricObservationRow(
                            ts, value, null, quality, flags, sensorId, sequenceId, sourceProtocol));
                }
            }
        }
        return rows;
    }

    private static Instant parseTimestamp(JsonNode node) {
        if (node == null || node.isNull()) {
            throw new DomainException("VALIDATION_FAILED", "observation timestamp missing");
        }
        String raw = node.asText();
        // ClickHouse JSONEachRow DateTime64 may arrive as "YYYY-MM-DD HH:mm:ss.SSS".
        if (raw.indexOf('T') < 0) {
            raw = raw.replace(' ', 'T') + "Z";
        } else if (!raw.endsWith("Z") && raw.indexOf('+') < 0) {
            raw = raw + "Z";
        }
        return Instant.parse(raw);
    }

    static String parseCounter64(JsonNode valueNode) {
        if (valueNode == null || valueNode.isNull()) {
            throw new DomainException("VALIDATION_FAILED", "counter observation value missing");
        }
        String decimal;
        if (valueNode.isTextual()) {
            decimal = valueNode.asText();
        } else if (valueNode.isIntegralNumber()) {
            // Only safe for values that fit Java long; still normalize via BigInteger.
            decimal = valueNode.bigIntegerValue().toString();
        } else {
            throw new DomainException("VALIDATION_FAILED", "counter observation value must be unsigned decimal");
        }
        if (!decimal.matches("^[0-9]+$")) {
            throw new DomainException("VALIDATION_FAILED", "counter observation value must be unsigned decimal");
        }
        BigInteger exact = new BigInteger(decimal);
        if (exact.signum() < 0 || exact.compareTo(MAX_UINT64) > 0) {
            throw new DomainException("VALIDATION_FAILED", "counter observation value outside UInt64");
        }
        return exact.toString();
    }

    private static String textOrEmpty(JsonNode node, String field) {
        JsonNode child = node.get(field);
        return child == null || child.isNull() ? "" : child.asText();
    }

    private static String enc(String value) {
        return URLEncoder.encode(value, StandardCharsets.UTF_8);
    }

    private static String basicAuth(String user, String password) {
        String creds = user + ":" + password;
        return "Basic " + java.util.Base64.getEncoder().encodeToString(creds.getBytes(StandardCharsets.UTF_8));
    }
}
