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

    public SensorControlException(String code, String message) {
        super(code, message);
    }

    public SensorControlException(String code, String message, Map<String, String> fieldErrors) {
        super(code, message, fieldErrors);
    }
}
