package com.platform.core.sensorcontrol;

/**
 * Sensor deployment role. Values must match {@code sensor/internal/runtime/role.go} exactly.
 */
public enum SensorRole {
    NETWORK,
    INFRASTRUCTURE,
    STREAMING_EVENT,
    SYNTHETIC,
    CLOUD_API,
    COMBINED
}
