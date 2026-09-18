package com.platform.core.domain.capabilitypack;

/** Runtime match outcome. Never invents an arbitrary winner. */
public enum CapabilityPackMatchOutcome {
    MATCH,
    NO_MATCH,
    UNKNOWN,
    AMBIGUOUS
}
