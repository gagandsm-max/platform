package com.platform.core.ingestion;

import com.platform.core.logging.CoreSupportabilityEventCodes;
import org.apache.kafka.clients.consumer.CommitFailedException;
import org.apache.kafka.common.errors.AuthenticationException;
import org.apache.kafka.common.errors.AuthorizationException;
import org.apache.kafka.common.errors.FencedInstanceIdException;
import org.apache.kafka.common.errors.IllegalGenerationException;
import org.apache.kafka.common.errors.RebalanceInProgressException;
import org.apache.kafka.common.errors.UnknownMemberIdException;

/** Consumer-local reasons, classified from trusted types/HTTP metadata before redaction. */
enum TelemetryConsumerFailure {
    DEPENDENCY(CoreSupportabilityEventCodes.TELEMETRY_CLICKHOUSE_UNAVAILABLE, true),
    AUTHORIZATION("ERR_CLICKHOUSE_AUTHORIZATION", false),
    SCHEMA("ERR_CLICKHOUSE_SCHEMA", false),
    CONFIGURATION("ERR_CLICKHOUSE_CONFIGURATION", false),
    PERMANENT("ERR_CLICKHOUSE_PERMANENT", false),
    UNKNOWN("ERR_CLICKHOUSE_UNKNOWN", false),
    POISON(CoreSupportabilityEventCodes.TELEMETRY_POISON_EVENT, false),
    ACK_UNCERTAIN("ERR_KAFKA_ACK_UNCERTAIN", false),
    MEMBERSHIP_LOST("ERR_KAFKA_GROUP_MEMBERSHIP_LOST", false),
    KAFKA_CONFIGURATION("ERR_KAFKA_CONFIGURATION", false),
    POLL_BUDGET("ERR_KAFKA_POLL_BUDGET", false),
    SHUTDOWN("ERR_KAFKA_CONSUMER_SHUTDOWN", false),
    DISABLED("ERR_KAFKA_CONSUMER_DISABLED", false),
    CONSUMER_FAILED("ERR_KAFKA_CONSUMER_FAILED", false);

    final String reasonCode;
    final boolean retryableInsert;

    TelemetryConsumerFailure(String reasonCode, boolean retryableInsert) {
        this.reasonCode = reasonCode;
        this.retryableInsert = retryableInsert;
    }

    static TelemetryConsumerFailure insertion(Throwable failure) {
        // Bounded traversal also handles wrappers introduced by resolveInsert. Never match messages.
        for (int depth = 0; failure != null && depth < 8; depth++, failure = failure.getCause()) {
            if (failure instanceof InsertFailure typed) return typed.reason;
            if (failure instanceof InterruptedException) return SHUTDOWN;
            if (failure instanceof javax.net.ssl.SSLException) return CONFIGURATION;
            if (failure instanceof SecurityException) return AUTHORIZATION;
            if (failure instanceof IllegalArgumentException) return CONFIGURATION;
            if (failure instanceof java.net.SocketException
                    || failure instanceof java.net.SocketTimeoutException
                    || failure instanceof java.net.http.HttpTimeoutException
                    || failure instanceof java.net.UnknownHostException
                    || failure instanceof java.io.EOFException) return DEPENDENCY;
        }
        // Unknown/local/schema failures are never assumed transient.
        return PERMANENT;
    }

    static TelemetryConsumerFailure acknowledgment(Throwable failure) {
        for (int depth = 0; failure != null && depth < 8; depth++, failure = failure.getCause()) {
            if (failure instanceof CommitFailedException || failure instanceof RebalanceInProgressException
                    || failure instanceof FencedInstanceIdException || failure instanceof UnknownMemberIdException
                    || failure instanceof IllegalGenerationException) return MEMBERSHIP_LOST;
            if (failure instanceof AuthenticationException || failure instanceof AuthorizationException) {
                return KAFKA_CONFIGURATION;
            }
            if (failure instanceof InterruptedException) return SHUTDOWN;
        }
        return ACK_UNCERTAIN;
    }

    static TelemetryConsumerFailure http(int status, String exceptionCode) {
        if (status == 401 || status == 403) return AUTHORIZATION;
        if (exceptionCode != null && !exceptionCode.isBlank()) {
            final int code;
            try {
                code = Integer.parseInt(exceptionCode);
            } catch (NumberFormatException invalid) {
                return UNKNOWN;
            }
            // ClickHouse 24.8 numeric error codes. Do not parse or retain response text.
            if (code != 0) return switch (code) {
                case 192, 193, 194, 195, 291, 497, 516 -> AUTHORIZATION;
                case 7, 8, 10, 16, 20, 27, 41, 43, 44, 47, 50, 53, 60, 62, 70, 72, 81, 117, 122 -> SCHEMA;
                case 36, 115, 137, 318 -> CONFIGURATION;
                case 159, 202, 203, 209, 210, 279 -> DEPENDENCY;
                default -> UNKNOWN;
            };
        }
        return switch (status) {
            case 429, 502, 503, 504 -> DEPENDENCY;
            case 400, 404, 422 -> SCHEMA;
            case 405 -> CONFIGURATION;
            default -> UNKNOWN;
        };
    }

    // Only the HttpClient.send catch site may grant generic IOException transport provenance.
    // Jackson/local IOException never enters this method. Preserve TLS/configuration distinctions.
    static TelemetryConsumerFailure transport(java.io.IOException failure) {
        var known = insertion(failure);
        return known == PERMANENT ? DEPENDENCY : known;
    }

    /** Carries an allowlisted reason/status, never response text. Transport causes require consumer redaction. */
    static final class InsertFailure extends RuntimeException {
        final TelemetryConsumerFailure reason;
        final int httpStatus;

        InsertFailure(TelemetryConsumerFailure reason, int status) {
            super(reason.reasonCode + ": ClickHouse insert rejected; HTTP status=" + status);
            this.reason = reason;
            this.httpStatus = status;
        }
    }
}
