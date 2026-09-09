-- Day-02 / Day-03 Telemetry Ingestion Replay Watermark State
-- Bounded per-Sensor durable replay watermark protecting against duplicates, sequence regression, and gaps

CREATE TABLE IF NOT EXISTS platform.sensor_telemetry_watermark (
    tenant_id UUID NOT NULL REFERENCES platform.tenant(tenant_id) ON DELETE CASCADE,
    sensor_id VARCHAR(255) NOT NULL,
    acked_through_sequence BIGINT NOT NULL,
    last_first_sequence BIGINT NOT NULL,
    last_last_sequence BIGINT NOT NULL,
    last_checksum VARCHAR(64) NOT NULL,
    last_batch_id VARCHAR(255),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, sensor_id),
    CONSTRAINT chk_acked_through_non_negative CHECK (acked_through_sequence >= 0),
    CONSTRAINT chk_last_sequence_order CHECK (last_last_sequence >= last_first_sequence)
);

CREATE INDEX IF NOT EXISTS idx_sensor_telemetry_watermark_sensor
    ON platform.sensor_telemetry_watermark (sensor_id);
