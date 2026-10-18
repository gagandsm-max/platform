package com.platform.core.telemetry.read;

import com.fasterxml.jackson.core.JsonParser;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.JsonToken;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.platform.core.domain.DomainException;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Component;

import java.io.BufferedReader;
import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.io.StringReader;
import java.math.BigInteger;
import java.net.ConnectException;
import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.net.http.HttpTimeoutException;
import java.nio.ByteBuffer;
import java.nio.charset.CharacterCodingException;
import java.nio.charset.CharsetDecoder;
import java.nio.charset.CodingErrorAction;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneOffset;
import java.time.format.DateTimeFormatter;
import java.time.format.DateTimeParseException;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Objects;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.Flow;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * Bounded ClickHouse read accessor for canonical FINAL views only.
 * Does not share the ingestion write client; never queries {@code *_raw} tables.
 * Selects the true latest N+1 observations (DESC), then reorders ASC for presentation.
 *
 * <p>Response exchange uses an absolute completion deadline covering headers and body,
 * plus a hard retained-byte ceiling. {@link HttpRequest#timeout()} alone is insufficient
 * on the evidenced Java 21.0.7 HttpClient (request timer is cleared before body read).
 */
@Component
public class ClickHouseMetricObservationReader {

    private static final Logger log = LoggerFactory.getLogger(ClickHouseMetricObservationReader.class);
    private static final DateTimeFormatter CH_TS = DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss.SSS")
            .withZone(ZoneOffset.UTC);
    private static final Duration DEFAULT_TIMEOUT = Duration.ofSeconds(5);
    private static final BigInteger MAX_UINT64 = new BigInteger("18446744073709551615");
    private static final BigInteger MAX_UINT32 = BigInteger.valueOf(4_294_967_295L);
    private static final long NANOS_PER_MILLI = 1_000_000L;

    /**
     * Hard ceiling on retained ClickHouse HTTP response bytes for this reader.
     * Phase-1 {@link MetricObservationQuery#MAX_LIMIT} is 500; selection fetches N+1 = 501
     * JSONEachRow lines. Allowlisted observation projections are a bounded field set; a
     * conservative ~4 KiB/line envelope for long OID/sensor/sequence identifiers yields
     * {@code 501 × 4 KiB ≈ 2 MiB}. This ceiling admits a valid maximum-page positive control
     * with headroom while failing closed on multi-megabyte dumps.
     */
    static final int MAX_RESPONSE_BYTES = 2 * 1024 * 1024;

    static final String SELECTION_ORDERING = "ts DESC, sensor_id DESC, sequence_id DESC";
    static final String PRESENTATION_ORDERING = "ts ASC, sensor_id ASC, sequence_id ASC";
    private static final String CH_EXCEPTION_CODE_HEADER = "X-ClickHouse-Exception-Code";

    private final String baseUrl;
    private final String authHeader;
    private final HttpClient httpClient;
    private final ObjectMapper objectMapper;
    private final Duration requestTimeout;

    /**
     * Production constructor selected by Spring DI. The package-visible test constructor
     * must not compete for autowiring (Spring 6.1+ counts all constructors).
     */
    @Autowired
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

    public MetricObservationPage read(MetricObservationQuery query) {
        int fetchLimit = query.limit() + 1; // N+1 to detect truthful truncation
        String sql = buildStaticSql(query.selector().valueKind());
        String uri = baseUrl
                + "/?param_tenant_id=" + enc(query.tenantId().toString())
                + "&param_entity_id=" + enc(query.entityId().toString())
                + "&param_metric_id=" + enc(query.selector().metricId())
                + "&param_from_ts=" + enc(formatClickHouseBound(query.fromInclusive()))
                + "&param_to_ts=" + enc(formatClickHouseBound(query.toExclusive()))
                + "&param_row_limit=" + fetchLimit
                + "&output_format_json_quote_64bit_integers=1"
                + "&max_execution_time=4";

        // Absolute deadline is enforced below via CompletableFuture.get; HttpRequest.timeout
        // is intentionally omitted because JDK 21.0.7 clears that timer before body completion.
        HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(uri))
                .header("Authorization", authHeader)
                .header("Content-Type", "text/plain; charset=UTF-8")
                .POST(HttpRequest.BodyPublishers.ofString(sql, StandardCharsets.UTF_8))
                .build();

        long deadlineNanos = System.nanoTime() + requestTimeout.toNanos();
        CompletableFuture<HttpResponse<String>> future = httpClient.sendAsync(
                request, boundedStringHandler(MAX_RESPONSE_BYTES));
        try {
            long remaining = deadlineNanos - System.nanoTime();
            if (remaining <= 0L) {
                cancelExchange(future);
                throw unavailable();
            }
            HttpResponse<String> response = future.get(remaining, TimeUnit.NANOSECONDS);
            if (response.statusCode() != 200) {
                log.warn("ClickHouse metric read failed status={}", response.statusCode());
                throw unavailable();
            }
            assertNoClickHouseExceptionMetadata(response);
            return parseLatestPage(response.body(), query.selector().valueKind(), query.limit());
        } catch (DomainException ex) {
            throw ex;
        } catch (TimeoutException ex) {
            cancelExchange(future);
            log.warn("ClickHouse metric read absolute deadline exceeded failureClass={}",
                    ex.getClass().getSimpleName());
            throw unavailable();
        } catch (InterruptedException ex) {
            cancelExchange(future);
            Thread.currentThread().interrupt();
            throw unavailable();
        } catch (ExecutionException ex) {
            throw mapExecutionFailure(ex);
        } catch (Exception ex) {
            cancelExchange(future);
            log.warn("ClickHouse metric read unavailable failureClass={}", ex.getClass().getSimpleName());
            throw unavailable();
        }
    }

    /**
     * Package-visible: static SQL uses canonical FINAL views only; parameters are never concatenated.
     * Selection is newest-first with deterministic sensor/sequence ties; callers reorder for presentation.
     */
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
                ORDER BY ts DESC, sensor_id DESC, sequence_id DESC
                LIMIT {row_limit:UInt32}
                FORMAT JSONEachRow
                """.formatted(view).trim();
    }

    /**
     * Parses ClickHouse JSONEachRow body already ordered newest-first.
     * Validates the complete bounded response (including any content beyond the N+1 probe),
     * then keeps at most {@code limit} rows, sets truncation from the N+1 probe row,
     * and reorders ascending for presentation only.
     */
    MetricObservationPage parseLatestPage(String body, MetricValueKind kind, int limit) {
        List<MetricObservationRow> newestFirst = parseRowsNewestFirst(body, kind, limit + 1);
        boolean truncated = newestFirst.size() > limit;
        List<MetricObservationRow> selected = truncated
                ? new ArrayList<>(newestFirst.subList(0, limit))
                : new ArrayList<>(newestFirst);
        Collections.reverse(selected); // presentation: oldest → newest within the selected latest window
        return new MetricObservationPage(selected, truncated);
    }

    private List<MetricObservationRow> parseRowsNewestFirst(String body, MetricValueKind kind, int maxRows) {
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
                if (rows.size() >= maxRows) {
                    // Extra nonblank content beyond the allowed N+1 probe rows — fail closed.
                    throw new DomainException(
                            "DATA_INTEGRITY_ERROR",
                            "Telemetry store returned malformed observation data");
                }
                rows.add(parseStrictJsonLine(line, kind));
            }
        } catch (DomainException ex) {
            throw ex;
        } catch (IOException ex) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        return rows;
    }

    private MetricObservationRow parseStrictJsonLine(String line, MetricValueKind kind) {
        try (JsonParser parser = objectMapper.getFactory().createParser(line)) {
            JsonNode node = objectMapper.readTree(parser);
            if (node == null) {
                throw new DomainException(
                        "DATA_INTEGRITY_ERROR",
                        "Telemetry store returned malformed observation data");
            }
            JsonToken trailing = parser.nextToken();
            if (trailing != null) {
                throw new DomainException(
                        "DATA_INTEGRITY_ERROR",
                        "Telemetry store returned malformed observation data");
            }
            return parseRow(node, kind);
        } catch (DomainException ex) {
            throw ex;
        } catch (JsonProcessingException | IllegalArgumentException | DateTimeParseException ex) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        } catch (IOException ex) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
    }

    private MetricObservationRow parseRow(JsonNode node, MetricValueKind kind) {
        Instant ts = parseTimestamp(node.get("ts"));
        int quality = requireQuality(node);
        long flags = requireUInt32(node, "flags");
        String sensorId = textOrEmpty(node, "sensor_id");
        String sequenceId = textOrEmpty(node, "sequence_id");
        String sourceProtocol = textOrEmpty(node, "source_protocol");
        if (kind == MetricValueKind.COUNTER64) {
            String counter = parseCounter64(node.get("value"));
            return new MetricObservationRow(
                    ts, null, counter, quality, flags, sensorId, sequenceId, sourceProtocol);
        }
        if (!node.has("value") || node.get("value").isNull() || !node.get("value").isNumber()) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        double value = node.get("value").asDouble();
        return new MetricObservationRow(
                ts, value, null, quality, flags, sensorId, sequenceId, sourceProtocol);
    }

    /**
     * Formats an API Instant as a DateTime64(3) bound string.
     * Sub-millisecond instants round UP to the next millisecond so the SQL
     * {@code [from,to)} interval preserves inclusive-from / exclusive-to semantics
     * against millisecond-stored observations. Already-aligned bounds are unchanged.
     * Original API request Instants remain unmodified on the query/response.
     */
    static String formatClickHouseBound(Instant instant) {
        return CH_TS.format(ceilToMillis(instant));
    }

    static Instant ceilToMillis(Instant instant) {
        int nano = instant.getNano();
        int subMilli = nano % (int) NANOS_PER_MILLI;
        if (subMilli == 0) {
            return instant;
        }
        return instant.plusNanos(NANOS_PER_MILLI - subMilli);
    }

    private static Instant parseTimestamp(JsonNode node) {
        if (node == null || node.isNull()) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        String raw = node.asText();
        // ClickHouse JSONEachRow DateTime64 may arrive as "YYYY-MM-DD HH:mm:ss.SSS".
        if (raw.indexOf('T') < 0) {
            raw = raw.replace(' ', 'T') + "Z";
        } else if (!raw.endsWith("Z") && raw.indexOf('+') < 0) {
            raw = raw + "Z";
        }
        try {
            return Instant.parse(raw);
        } catch (DateTimeParseException ex) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
    }

    static String parseCounter64(JsonNode valueNode) {
        if (valueNode == null || valueNode.isNull()) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        String decimal;
        if (valueNode.isTextual()) {
            decimal = valueNode.asText();
        } else if (valueNode.isIntegralNumber()) {
            // Only safe for values that fit Java long; still normalize via BigInteger.
            decimal = valueNode.bigIntegerValue().toString();
        } else {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        if (!decimal.matches("^[0-9]+$")) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        BigInteger exact = new BigInteger(decimal);
        if (exact.signum() < 0 || exact.compareTo(MAX_UINT64) > 0) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        return exact.toString();
    }

    /**
     * Populated quality must be UNKNOWN=0, GOOD=1 or DEGRADED=2.
     * Absent/null defaults to UNKNOWN. Unsupported/malformed values fail closed.
     */
    private static int requireQuality(JsonNode node) {
        JsonNode child = node.get("quality");
        if (child == null || child.isNull()) {
            return 0;
        }
        if (!child.isIntegralNumber()) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        BigInteger exact = child.bigIntegerValue();
        if (exact.compareTo(BigInteger.ZERO) < 0 || exact.compareTo(BigInteger.valueOf(2L)) > 0) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        return exact.intValueExact();
    }

    /** ClickHouse flags are UInt32: preserve the full 0..4294967295 range as Java long. */
    private static long requireUInt32(JsonNode node, String field) {
        JsonNode child = node.get(field);
        if (child == null || child.isNull()) {
            return 0L;
        }
        BigInteger exact;
        if (child.isIntegralNumber()) {
            exact = child.bigIntegerValue();
        } else if (child.isTextual() && child.asText().matches("^[0-9]+$")) {
            exact = new BigInteger(child.asText());
        } else {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        if (exact.signum() < 0 || exact.compareTo(MAX_UINT32) > 0) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        return exact.longValueExact();
    }

    private static String textOrEmpty(JsonNode node, String field) {
        JsonNode child = node.get(field);
        return child == null || child.isNull() ? "" : child.asText();
    }

    private static void assertNoClickHouseExceptionMetadata(HttpResponse<?> response) {
        // Scalar field: JDK HttpHeaders preserves every occurrence; firstValue alone can hide a
        // later nonzero/malformed code behind an earlier zero. Absent and a single well-formed
        // zero remain the only non-error controls; duplicates/conflicts fail closed.
        List<String> codeHeaders = response.headers().allValues(CH_EXCEPTION_CODE_HEADER);
        if (codeHeaders.isEmpty()) {
            return;
        }
        if (codeHeaders.size() != 1) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        String raw = codeHeaders.get(0).trim();
        if (raw.isEmpty()) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        if (!raw.matches("^[0-9]+$")) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        BigInteger code = new BigInteger(raw);
        if (!BigInteger.ZERO.equals(code)) {
            throw new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
    }

    private static DomainException mapExecutionFailure(ExecutionException ex) {
        Throwable cause = ex.getCause() == null ? ex : ex.getCause();
        if (cause instanceof DomainException domain) {
            return domain;
        }
        if (cause instanceof ResponseBoundExceededException) {
            return new DomainException(
                    "DATA_INTEGRITY_ERROR",
                    "Telemetry store returned malformed observation data");
        }
        if (cause instanceof InterruptedException) {
            Thread.currentThread().interrupt();
            return unavailable();
        }
        if (cause instanceof HttpTimeoutException || cause instanceof ConnectException
                || cause instanceof TimeoutException) {
            log.warn("ClickHouse metric read unavailable failureClass={}", cause.getClass().getSimpleName());
            return unavailable();
        }
        log.warn("ClickHouse metric read unavailable failureClass={}", cause.getClass().getSimpleName());
        return unavailable();
    }

    private static DomainException unavailable() {
        return new DomainException("DATABASE_UNAVAILABLE", "Telemetry store temporarily unavailable");
    }

    private static void cancelExchange(CompletableFuture<?> future) {
        if (future != null) {
            future.cancel(true);
        }
    }

    static HttpResponse.BodyHandler<String> boundedStringHandler(int maxBytes) {
        return responseInfo -> new BoundedStringSubscriber(maxBytes);
    }

    private static String enc(String value) {
        return URLEncoder.encode(value, StandardCharsets.UTF_8);
    }

    private static String basicAuth(String user, String password) {
        String creds = user + ":" + password;
        return "Basic " + java.util.Base64.getEncoder().encodeToString(creds.getBytes(StandardCharsets.UTF_8));
    }

    /** Fail-closed signal when retained ClickHouse body exceeds {@link #MAX_RESPONSE_BYTES}. */
    static final class ResponseBoundExceededException extends RuntimeException {
        ResponseBoundExceededException() {
            super("ClickHouse response exceeded retained byte ceiling");
        }
    }

    /**
     * Retains at most {@code maxBytes} of response body on the HttpClient executor / calling
     * subscriber path. Overflow cancels the subscription immediately and does not grow the buffer.
     */
    static final class BoundedStringSubscriber implements HttpResponse.BodySubscriber<String> {
        private final int maxBytes;
        private final ByteArrayOutputStream buffer = new ByteArrayOutputStream();
        private final CompletableFuture<String> result = new CompletableFuture<>();
        private final AtomicBoolean done = new AtomicBoolean(false);
        private volatile Flow.Subscription subscription;

        BoundedStringSubscriber(int maxBytes) {
            if (maxBytes < 1) {
                throw new IllegalArgumentException("maxBytes must be positive");
            }
            this.maxBytes = maxBytes;
        }

        @Override
        public void onSubscribe(Flow.Subscription subscription) {
            this.subscription = subscription;
            subscription.request(Long.MAX_VALUE);
        }

        @Override
        public void onNext(List<ByteBuffer> items) {
            if (done.get()) {
                return;
            }
            for (ByteBuffer item : items) {
                int remaining = item.remaining();
                if (remaining <= 0) {
                    continue;
                }
                if (buffer.size() + remaining > maxBytes) {
                    failOverflow();
                    return;
                }
                byte[] chunk = new byte[remaining];
                item.get(chunk);
                buffer.write(chunk, 0, chunk.length);
            }
        }

        @Override
        public void onError(Throwable throwable) {
            if (done.compareAndSet(false, true)) {
                buffer.reset();
                result.completeExceptionally(throwable);
            }
        }

        @Override
        public void onComplete() {
            if (!done.compareAndSet(false, true)) {
                return;
            }
            try {
                // ByteArrayOutputStream#toString(Charset) replaces malformed input; report instead
                // so altered provenance cannot be admitted as a GOOD observation.
                CharsetDecoder decoder = StandardCharsets.UTF_8.newDecoder()
                        .onMalformedInput(CodingErrorAction.REPORT)
                        .onUnmappableCharacter(CodingErrorAction.REPORT);
                String body = decoder.decode(ByteBuffer.wrap(buffer.toByteArray())).toString();
                result.complete(body);
            } catch (CharacterCodingException ex) {
                result.completeExceptionally(new DomainException(
                        "DATA_INTEGRITY_ERROR",
                        "Telemetry store returned malformed observation data"));
            } finally {
                buffer.reset();
            }
        }

        @Override
        public CompletableFuture<String> getBody() {
            return result;
        }

        private void failOverflow() {
            if (!done.compareAndSet(false, true)) {
                return;
            }
            Flow.Subscription sub = subscription;
            if (sub != null) {
                sub.cancel();
            }
            buffer.reset();
            result.completeExceptionally(new ResponseBoundExceededException());
        }
    }
}
