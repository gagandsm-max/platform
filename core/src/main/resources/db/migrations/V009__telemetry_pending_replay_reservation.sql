-- P0 telemetry replay reservation.
-- Keeps exactly one durable in-flight Sensor batch alongside the bounded watermark.
-- This closes the Kafka-commit/PostgreSQL-watermark crash window without storing telemetry payloads in PostgreSQL.

ALTER TABLE platform.sensor_telemetry_watermark
    ADD COLUMN IF NOT EXISTS stream_id VARCHAR(128),
    ADD COLUMN IF NOT EXISTS pending_first_sequence BIGINT,
    ADD COLUMN IF NOT EXISTS pending_last_sequence BIGINT,
    ADD COLUMN IF NOT EXISTS pending_checksum VARCHAR(64),
    ADD COLUMN IF NOT EXISTS pending_batch_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS pending_reserved_at TIMESTAMPTZ;

ALTER TABLE platform.sensor_telemetry_watermark
    DROP CONSTRAINT IF EXISTS chk_sensor_telemetry_pending_complete;

ALTER TABLE platform.sensor_telemetry_watermark
    ADD CONSTRAINT chk_sensor_telemetry_pending_complete CHECK (
        (
            pending_first_sequence IS NULL
            AND pending_last_sequence IS NULL
            AND pending_checksum IS NULL
            AND pending_batch_id IS NULL
            AND pending_reserved_at IS NULL
        )
        OR
        (
            pending_first_sequence IS NOT NULL
            AND pending_last_sequence IS NOT NULL
            AND pending_checksum IS NOT NULL
            AND pending_batch_id IS NOT NULL
            AND pending_reserved_at IS NOT NULL
            AND pending_first_sequence > 0
            AND pending_last_sequence >= pending_first_sequence
        )
    );

ALTER TABLE platform.sensor_telemetry_watermark
    DROP CONSTRAINT IF EXISTS chk_sensor_telemetry_stream_id;

ALTER TABLE platform.sensor_telemetry_watermark
    ADD CONSTRAINT chk_sensor_telemetry_stream_id CHECK (
        stream_id IS NULL OR stream_id ~ '^stream-[0-9a-f]{32}$'
    );
