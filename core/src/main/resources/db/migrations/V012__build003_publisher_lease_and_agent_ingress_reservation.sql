-- BUILD-003: publisher lease for telemetry pending reservation + Agent-ingress
-- reserve/finalize parity, stream lineage, and MAX_SAFE transport sequence constraints.
-- Do not rewrite V002/V006/V009. No telemetry payloads stored in PostgreSQL.

-- ============================================================================
-- Direct telemetry: publisher lease + MAX_SAFE / coherence
-- ============================================================================

ALTER TABLE platform.sensor_telemetry_watermark
    ADD COLUMN IF NOT EXISTS pending_publisher_owner UUID,
    ADD COLUMN IF NOT EXISTS pending_publisher_epoch BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pending_publisher_claimed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS pending_publisher_lease_until TIMESTAMPTZ;

ALTER TABLE platform.sensor_telemetry_watermark
    DROP CONSTRAINT IF EXISTS chk_sensor_telemetry_publisher_lease_coherent;

ALTER TABLE platform.sensor_telemetry_watermark
    ADD CONSTRAINT chk_sensor_telemetry_publisher_lease_coherent CHECK (
        (
            pending_publisher_owner IS NULL
            AND pending_publisher_claimed_at IS NULL
            AND pending_publisher_lease_until IS NULL
        )
        OR
        (
            pending_first_sequence IS NOT NULL
            AND pending_publisher_owner IS NOT NULL
            AND pending_publisher_claimed_at IS NOT NULL
            AND pending_publisher_lease_until IS NOT NULL
            AND pending_publisher_lease_until >= pending_publisher_claimed_at
            AND pending_publisher_epoch > 0
        )
    );

ALTER TABLE platform.sensor_telemetry_watermark
    DROP CONSTRAINT IF EXISTS chk_sensor_telemetry_acked_max_safe;
ALTER TABLE platform.sensor_telemetry_watermark
    ADD CONSTRAINT chk_sensor_telemetry_acked_max_safe CHECK (
        acked_through_sequence >= 0
        AND acked_through_sequence <= 9007199254740991
    );

ALTER TABLE platform.sensor_telemetry_watermark
    DROP CONSTRAINT IF EXISTS chk_sensor_telemetry_last_max_safe;
ALTER TABLE platform.sensor_telemetry_watermark
    ADD CONSTRAINT chk_sensor_telemetry_last_max_safe CHECK (
        last_first_sequence >= 0
        AND last_first_sequence <= 9007199254740991
        AND last_last_sequence >= 0
        AND last_last_sequence <= 9007199254740991
    );

ALTER TABLE platform.sensor_telemetry_watermark
    DROP CONSTRAINT IF EXISTS chk_sensor_telemetry_pending_max_safe;
ALTER TABLE platform.sensor_telemetry_watermark
    ADD CONSTRAINT chk_sensor_telemetry_pending_max_safe CHECK (
        pending_first_sequence IS NULL
        OR (
            pending_first_sequence >= 1
            AND pending_first_sequence <= 9007199254740991
            AND pending_last_sequence >= pending_first_sequence
            AND pending_last_sequence <= 9007199254740991
        )
    );

-- ============================================================================
-- Agent-ingress: stream + pending reservation + publisher lease + MAX_SAFE
-- ============================================================================

ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD COLUMN IF NOT EXISTS stream_id VARCHAR(128),
    ADD COLUMN IF NOT EXISTS pending_first_sequence BIGINT,
    ADD COLUMN IF NOT EXISTS pending_last_sequence BIGINT,
    ADD COLUMN IF NOT EXISTS pending_checksum VARCHAR(64),
    ADD COLUMN IF NOT EXISTS pending_batch_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS pending_correlation_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS pending_reserved_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS pending_publisher_owner UUID,
    ADD COLUMN IF NOT EXISTS pending_publisher_epoch BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pending_publisher_claimed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS pending_publisher_lease_until TIMESTAMPTZ;

ALTER TABLE platform.sensor_agent_ingress_watermark
    DROP CONSTRAINT IF EXISTS chk_agent_ingress_stream_id;
ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD CONSTRAINT chk_agent_ingress_stream_id CHECK (
        stream_id IS NULL OR stream_id ~ '^stream-[0-9a-f]{32}$'
    );

ALTER TABLE platform.sensor_agent_ingress_watermark
    DROP CONSTRAINT IF EXISTS chk_agent_ingress_pending_complete;
ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD CONSTRAINT chk_agent_ingress_pending_complete CHECK (
        (
            pending_first_sequence IS NULL
            AND pending_last_sequence IS NULL
            AND pending_checksum IS NULL
            AND pending_batch_id IS NULL
            AND pending_correlation_id IS NULL
            AND pending_reserved_at IS NULL
        )
        OR
        (
            pending_first_sequence IS NOT NULL
            AND pending_last_sequence IS NOT NULL
            AND pending_checksum IS NOT NULL
            AND pending_batch_id IS NOT NULL
            AND pending_correlation_id IS NOT NULL
            AND pending_reserved_at IS NOT NULL
            AND pending_first_sequence > 0
            AND pending_last_sequence >= pending_first_sequence
        )
    );

ALTER TABLE platform.sensor_agent_ingress_watermark
    DROP CONSTRAINT IF EXISTS chk_agent_ingress_publisher_lease_coherent;
ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD CONSTRAINT chk_agent_ingress_publisher_lease_coherent CHECK (
        (
            pending_publisher_owner IS NULL
            AND pending_publisher_claimed_at IS NULL
            AND pending_publisher_lease_until IS NULL
        )
        OR
        (
            pending_first_sequence IS NOT NULL
            AND pending_publisher_owner IS NOT NULL
            AND pending_publisher_claimed_at IS NOT NULL
            AND pending_publisher_lease_until IS NOT NULL
            AND pending_publisher_lease_until >= pending_publisher_claimed_at
            AND pending_publisher_epoch > 0
        )
    );

ALTER TABLE platform.sensor_agent_ingress_watermark
    DROP CONSTRAINT IF EXISTS chk_agent_ingress_acked_max_safe;
ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD CONSTRAINT chk_agent_ingress_acked_max_safe CHECK (
        acked_through_sequence >= 0
        AND acked_through_sequence <= 9007199254740991
    );

ALTER TABLE platform.sensor_agent_ingress_watermark
    DROP CONSTRAINT IF EXISTS chk_agent_ingress_last_max_safe;
ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD CONSTRAINT chk_agent_ingress_last_max_safe CHECK (
        last_first_sequence >= 0
        AND last_first_sequence <= 9007199254740991
        AND last_last_sequence >= 0
        AND last_last_sequence <= 9007199254740991
    );

ALTER TABLE platform.sensor_agent_ingress_watermark
    DROP CONSTRAINT IF EXISTS chk_agent_ingress_pending_max_safe;
ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD CONSTRAINT chk_agent_ingress_pending_max_safe CHECK (
        pending_first_sequence IS NULL
        OR (
            pending_first_sequence >= 1
            AND pending_first_sequence <= 9007199254740991
            AND pending_last_sequence >= pending_first_sequence
            AND pending_last_sequence <= 9007199254740991
        )
    );
