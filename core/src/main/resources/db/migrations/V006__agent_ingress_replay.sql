-- Agent Ingress Durable Drain Replay Watermark State
-- Bounded per-Sensor agent-ingress spool sequence watermark (independent of telemetry watermark)

CREATE TABLE IF NOT EXISTS platform.sensor_agent_ingress_watermark (
    tenant_id UUID NOT NULL REFERENCES platform.tenant(tenant_id) ON DELETE CASCADE,
    sensor_id VARCHAR(255) NOT NULL,
    acked_through_sequence BIGINT NOT NULL,
    last_first_sequence BIGINT NOT NULL,
    last_last_sequence BIGINT NOT NULL,
    last_checksum VARCHAR(64) NOT NULL,
    last_batch_id VARCHAR(255),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, sensor_id),
    CONSTRAINT chk_agent_ingress_acked_through_non_negative CHECK (acked_through_sequence >= 0),
    CONSTRAINT chk_agent_ingress_last_sequence_order CHECK (last_last_sequence >= last_first_sequence)
);

CREATE INDEX IF NOT EXISTS idx_sensor_agent_ingress_watermark_sensor
    ON platform.sensor_agent_ingress_watermark (sensor_id);
