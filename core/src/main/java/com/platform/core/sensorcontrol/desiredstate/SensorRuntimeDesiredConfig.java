package com.platform.core.sensorcontrol.desiredstate;

import java.util.Objects;

/**
 * Operational runtime overlay (desired-config v1). Must never carry identity, endpoints,
 * TLS material, credentials, Capability Pack payloads, policy or upgrade instructions.
 */
public record SensorRuntimeDesiredConfig(
        SensorLogLevel logLevel,
        SensorSchedulerDesiredConfig scheduler,
        SensorTransportDesiredConfig transport
) {
    public SensorRuntimeDesiredConfig {
        Objects.requireNonNull(logLevel, "logLevel");
        Objects.requireNonNull(scheduler, "scheduler");
        Objects.requireNonNull(transport, "transport");
    }
}
