package com.platform.core.ingestion.icmp;

/**
 * One ICMP echo sample from Sensor {@code protocol/icmp.Sample}.
 * {@code rttMicros} is null when no Echo Reply was received; zero is never invented.
 */
public record IcmpSample(
        int sequence,
        boolean success,
        Long rttMicros,
        String errorCode
) {
}
