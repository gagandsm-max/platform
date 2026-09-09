package com.platform.core.ingestion.replay;

import org.springframework.jdbc.core.JdbcTemplate;
import org.springframework.stereotype.Repository;

import java.sql.ResultSet;
import java.sql.SQLException;
import java.util.List;
import java.util.UUID;

@Repository
public class PostgresTelemetryWatermarkStore implements TelemetryWatermarkStore {

    private final JdbcTemplate jdbcTemplate;

    public PostgresTelemetryWatermarkStore(JdbcTemplate jdbcTemplate) {
        this.jdbcTemplate = jdbcTemplate;
    }

    @Override
    public WatermarkState getWatermarkForUpdate(String sensorId) {
        // Advisory transaction lock guarantees serialize access for this sensor
        // even when the row has not yet been inserted.
        jdbcTemplate.queryForObject(
                "SELECT pg_advisory_xact_lock(hashtext(?))",
                Void.class,
                "sensor_telemetry_watermark:" + sensorId
        );

        List<WatermarkState> list = jdbcTemplate.query(
                """
                SELECT sensor_id, tenant_id, acked_through_sequence, last_first_sequence,
                       last_last_sequence, last_checksum, last_batch_id, updated_at
                  FROM platform.sensor_telemetry_watermark
                 WHERE sensor_id = ?
                 FOR UPDATE
                """,
                (rs, rowNum) -> mapRow(rs),
                sensorId
        );
        return list.isEmpty() ? null : list.get(0);
    }

    @Override
    public void updateWatermark(
            String sensorId,
            UUID tenantId,
            long ackedThrough,
            long firstSeq,
            long lastSeq,
            String checksum,
            String batchId
    ) {
        jdbcTemplate.update(
                """
                INSERT INTO platform.sensor_telemetry_watermark (
                    sensor_id, tenant_id, acked_through_sequence, last_first_sequence,
                    last_last_sequence, last_checksum, last_batch_id, updated_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
                ON CONFLICT (sensor_id) DO UPDATE SET
                    tenant_id = EXCLUDED.tenant_id,
                    acked_through_sequence = EXCLUDED.acked_through_sequence,
                    last_first_sequence = EXCLUDED.last_first_sequence,
                    last_last_sequence = EXCLUDED.last_last_sequence,
                    last_checksum = EXCLUDED.last_checksum,
                    last_batch_id = EXCLUDED.last_batch_id,
                    updated_at = CURRENT_TIMESTAMP
                """,
                sensorId, tenantId, ackedThrough, firstSeq, lastSeq, checksum, batchId
        );
    }

    private static WatermarkState mapRow(ResultSet rs) throws SQLException {
        return new WatermarkState(
                rs.getString("sensor_id"),
                rs.getObject("tenant_id", UUID.class),
                rs.getLong("acked_through_sequence"),
                rs.getLong("last_first_sequence"),
                rs.getLong("last_last_sequence"),
                rs.getString("last_checksum"),
                rs.getString("last_batch_id"),
                rs.getTimestamp("updated_at").toInstant()
        );
    }
}
