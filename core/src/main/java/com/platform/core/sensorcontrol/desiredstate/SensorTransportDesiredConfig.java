package com.platform.core.sensorcontrol.desiredstate;

public record SensorTransportDesiredConfig(
        int requestTimeoutMs,
        int maxRequestBytes,
        int maxResponseBytes,
        int uploadBatchRecords,
        int uploadBatchBytes
) {
}
