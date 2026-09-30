package com.platform.core.logging;

import org.slf4j.MDC;

/**
 * Bounded MDC helpers for certified supportability contextual fields.
 */
public final class SupportabilityMdc {

    private SupportabilityMdc() {
    }

    /**
     * Sets {@code event_code} for the duration of {@code action}, restoring prior MDC state afterward.
     */
    public static void withEventCode(String eventCode, Runnable action) {
        String previous = MDC.get(SupportabilityMdcKeys.EVENT_CODE);
        MDC.put(SupportabilityMdcKeys.EVENT_CODE, eventCode);
        try {
            action.run();
        } finally {
            if (previous == null) {
                MDC.remove(SupportabilityMdcKeys.EVENT_CODE);
            } else {
                MDC.put(SupportabilityMdcKeys.EVENT_CODE, previous);
            }
        }
    }
}
