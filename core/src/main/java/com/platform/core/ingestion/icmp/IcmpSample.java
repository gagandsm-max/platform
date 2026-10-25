package com.platform.core.ingestion.icmp;

/**
 * One ICMP echo sample from Sensor {@code protocol/icmp.Sample}.
 * {@code rttMicros} is null when no Echo Reply was received. Successful samples may carry
 * {@code 0} when Go {@code omitempty} omitted a sub-microsecond RTT; positive RTT is never invented.
 */
public record IcmpSample(
        int sequence,
        boolean success,
        Long rttMicros,
        String errorCode
) {
}
