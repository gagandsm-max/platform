package com.platform.core.persistence;

import java.sql.Timestamp;
import java.time.Instant;

final class JdbcTime {
    private JdbcTime() {
    }

    static Timestamp ts(Instant instant) {
        return instant == null ? null : Timestamp.from(instant);
    }

    static Instant instant(Timestamp timestamp) {
        return timestamp == null ? null : timestamp.toInstant();
    }
}
