package com.platform.core.telemetry.read;

import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

/**
 * Source contract: readers must use canonical FINAL views, never {@code *_raw}.
 */
class ClickHouseMetricObservationSqlContractTest {

    @Test
    void numericSqlUsesCanonicalViewOnly() {
        String sql = ClickHouseMetricObservationReader.buildStaticSql(MetricValueKind.NUMERIC);
        assertTrue(sql.contains("FROM platform.metric_numeric"));
        assertFalse(sql.contains("_raw"));
        assertTrue(sql.contains("ORDER BY ts ASC, sequence_id ASC"));
        assertTrue(sql.contains("LIMIT {row_limit:UInt32}"));
    }

    @Test
    void counterSqlUsesCanonicalViewOnly() {
        String sql = ClickHouseMetricObservationReader.buildStaticSql(MetricValueKind.COUNTER64);
        assertTrue(sql.contains("FROM platform.metric_counter"));
        assertFalse(sql.contains("_raw"));
    }
}
