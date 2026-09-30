package com.platform.core.logging;

import java.util.Set;

/**
 * Approved contextual MDC keys for BUILD-005 Core structured logging.
 * Arbitrary MDC keys must not be emitted as top-level JSON fields.
 */
public final class SupportabilityMdcKeys {

    public static final String CORRELATION_ID = "correlation_id";
    public static final String TENANT_ID = "tenant_id";
    public static final String EVENT_CODE = "event_code";
    public static final String REQUEST_ID = "request_id";
    public static final String JOB_ID = "job_id";
    public static final String ENTITY_ID = "entity_id";
    public static final String PROTOCOL = "protocol";
    public static final String RESULT = "result";
    public static final String RETRY_COUNT = "retry_count";

    public static final Set<String> ALLOWED = Set.of(
            CORRELATION_ID,
            TENANT_ID,
            EVENT_CODE,
            REQUEST_ID,
            JOB_ID,
            ENTITY_ID,
            PROTOCOL,
            RESULT,
            RETRY_COUNT
    );

    private SupportabilityMdcKeys() {
    }
}
