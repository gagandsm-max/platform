package com.platform.core.sensorcontrol;

import com.platform.core.domain.DomainException;

import java.util.Map;

/**
 * Control-plane Sensor enrollment / credential errors (internal ops; not public REST).
 */
public class SensorControlException extends DomainException {

    public static final String ERR_SENSOR_NOT_FOUND = "ERR_SENSOR_NOT_FOUND";
    public static final String ERR_SENSOR_ALREADY_EXISTS = "ERR_SENSOR_ALREADY_EXISTS";
    public static final String ERR_SENSOR_INVALID_ROLE = "ERR_SENSOR_INVALID_ROLE";
    public static final String ERR_SENSOR_SITE_INVALID = "ERR_SENSOR_SITE_INVALID";
    public static final String ERR_SENSOR_CREDENTIAL_DUPLICATE = "ERR_SENSOR_CREDENTIAL_DUPLICATE";
    public static final String ERR_SENSOR_CREDENTIAL_NOT_FOUND = "ERR_SENSOR_CREDENTIAL_NOT_FOUND";
    public static final String ERR_SENSOR_CREDENTIAL_INVALID = "ERR_SENSOR_CREDENTIAL_INVALID";
    public static final String ERR_SENSOR_ENTITY_INVALID = "ERR_SENSOR_ENTITY_INVALID";
    public static final String ERR_SENSOR_STATE_INVALID = "ERR_SENSOR_STATE_INVALID";
    public static final String ERR_NO_ELIGIBLE_SENSOR = "ERR_NO_ELIGIBLE_SENSOR";
    public static final String ERR_DISCOVERY_REQUEST_NOT_PLACEABLE = "ERR_DISCOVERY_REQUEST_NOT_PLACEABLE";
    public static final String ERR_DISCOVERY_PLACEMENT_CONFLICT = "ERR_DISCOVERY_PLACEMENT_CONFLICT";

    /** Enrollment bootstrap authority (B1) — issue-time validation. */
    public static final String ERR_SENSOR_ENROLLMENT_AUTHORITY_TTL_INVALID =
            "ERR_SENSOR_ENROLLMENT_AUTHORITY_TTL_INVALID";
    public static final String ERR_SENSOR_ENROLLMENT_AUTHORITY_SCOPE_INVALID =
            "ERR_SENSOR_ENROLLMENT_AUTHORITY_SCOPE_INVALID";
    /**
     * Safe external-facing family for consume failures (unknown/expired/revoked/replay/scope).
     * Do not differentiate these for unauthenticated enrollment callers.
     */
    public static final String ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID =
            "ERR_SENSOR_ENROLLMENT_AUTHORITY_INVALID";

    /**
     * Poll-grant issuance (Issue #229 G1a). Codes are typed and nondisclosing:
     * they must not reveal cross-tenant existence or SQL/database internals.
     */
    public static final String ERR_POLL_GRANT_ENTITY_OUT_OF_SCOPE = "ERR_POLL_GRANT_ENTITY_OUT_OF_SCOPE";
    public static final String ERR_POLL_GRANT_ENDPOINT_UNAVAILABLE = "ERR_POLL_GRANT_ENDPOINT_UNAVAILABLE";
    public static final String ERR_POLL_GRANT_PLAN_INVALID = "ERR_POLL_GRANT_PLAN_INVALID";
    public static final String ERR_POLL_GRANT_GENERATION_INVALID = "ERR_POLL_GRANT_GENERATION_INVALID";
    public static final String ERR_POLL_GRANT_WINDOW_INVALID = "ERR_POLL_GRANT_WINDOW_INVALID";
    public static final String ERR_POLL_GRANT_CONFLICT = "ERR_POLL_GRANT_CONFLICT";

    public SensorControlException(String code, String message) {
        super(code, message);
    }

    public SensorControlException(String code, String message, Map<String, String> fieldErrors) {
        super(code, message, fieldErrors);
    }
}
