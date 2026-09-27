package com.platform.core.sensorcontrol.desiredstate;

public enum SensorLogLevel {
    DEBUG,
    INFO,
    WARN,
    ERROR;

    public static SensorLogLevel parse(String raw) {
        if (raw == null || raw.isBlank()) {
            throw new IllegalArgumentException("logLevel is required");
        }
        try {
            return SensorLogLevel.valueOf(raw.trim());
        } catch (IllegalArgumentException ex) {
            throw new IllegalArgumentException("logLevel must be DEBUG|INFO|WARN|ERROR");
        }
    }
}
