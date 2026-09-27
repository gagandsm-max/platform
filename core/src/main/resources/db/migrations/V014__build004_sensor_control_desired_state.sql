-- BUILD-004 Slice 2: Core Sensor control desired-state authority.
-- Additive only. V001–V013 remain immutable. No clamp/rewrite of historical rows.

CREATE TABLE IF NOT EXISTS platform.sensor_control_state (
    tenant_id UUID NOT NULL,
    sensor_id VARCHAR(128) NOT NULL,

    desired_schema_version INT NOT NULL,
    desired_generation BIGINT NOT NULL,
    desired_checksum CHAR(64) NOT NULL,
    desired_config_json JSONB NOT NULL,
    desired_by VARCHAR(256) NOT NULL,
    desired_at TIMESTAMPTZ NOT NULL,

    applied_generation BIGINT NOT NULL,
    applied_checksum CHAR(64) NULL,
    applied_at TIMESTAMPTZ NULL,

    observed_generation BIGINT NOT NULL,
    observed_checksum CHAR(64) NULL,
    observed_outcome VARCHAR(32) NULL,
    rejection_code VARCHAR(128) NULL,
    rejection_message VARCHAR(512) NULL,
    observed_at TIMESTAMPTZ NULL,

    updated_at TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (tenant_id, sensor_id),

    CONSTRAINT fk_scs_sensor_same_tenant
        FOREIGN KEY (tenant_id, sensor_id)
        REFERENCES platform.sensor (tenant_id, sensor_id),

    CONSTRAINT chk_scs_desired_schema_version CHECK (desired_schema_version = 1),

    CONSTRAINT chk_scs_desired_generation CHECK (
        desired_generation >= 1
        AND desired_generation <= 9007199254740991
    ),
    CONSTRAINT chk_scs_applied_generation CHECK (
        applied_generation >= 0
        AND applied_generation <= 9007199254740991
    ),
    CONSTRAINT chk_scs_observed_generation CHECK (
        observed_generation >= 0
        AND observed_generation <= 9007199254740991
    ),
    CONSTRAINT chk_scs_applied_le_desired CHECK (applied_generation <= desired_generation),
    CONSTRAINT chk_scs_observed_le_desired CHECK (observed_generation <= desired_generation),

    CONSTRAINT chk_scs_desired_checksum_hex CHECK (desired_checksum ~ '^[a-f0-9]{64}$'),
    CONSTRAINT chk_scs_applied_checksum_hex CHECK (
        applied_checksum IS NULL OR applied_checksum ~ '^[a-f0-9]{64}$'
    ),
    CONSTRAINT chk_scs_observed_checksum_hex CHECK (
        observed_checksum IS NULL OR observed_checksum ~ '^[a-f0-9]{64}$'
    ),

    CONSTRAINT chk_scs_applied_checksum_presence CHECK (
        (applied_generation = 0 AND applied_checksum IS NULL)
        OR (applied_generation > 0 AND applied_checksum IS NOT NULL)
    ),
    CONSTRAINT chk_scs_observed_checksum_presence CHECK (
        (observed_generation = 0 AND observed_checksum IS NULL)
        OR (observed_generation > 0 AND observed_checksum IS NOT NULL)
    ),

    CONSTRAINT chk_scs_observed_outcome CHECK (
        observed_outcome IS NULL
        OR observed_outcome IN ('APPLIED', 'REJECTED')
    ),
    CONSTRAINT chk_scs_observed_outcome_presence CHECK (
        (observed_generation = 0 AND observed_outcome IS NULL)
        OR (observed_generation > 0 AND observed_outcome IS NOT NULL)
    ),

    CONSTRAINT chk_scs_rejection_coherence CHECK (
        (
            observed_outcome IS NULL
            AND rejection_code IS NULL
            AND rejection_message IS NULL
        )
        OR (
            observed_outcome = 'APPLIED'
            AND rejection_code IS NULL
            AND rejection_message IS NULL
        )
        OR (
            observed_outcome = 'REJECTED'
            AND rejection_code IS NOT NULL
            AND char_length(rejection_code) BETWEEN 1 AND 128
            AND (rejection_message IS NULL OR char_length(rejection_message) <= 512)
        )
    ),

    CONSTRAINT chk_scs_desired_config_object CHECK (jsonb_typeof(desired_config_json) = 'object'),
    CONSTRAINT chk_scs_desired_config_size CHECK (octet_length(desired_config_json::text) <= 65536),
    CONSTRAINT chk_scs_desired_by_len CHECK (char_length(desired_by) BETWEEN 1 AND 256)
);

CREATE INDEX IF NOT EXISTS idx_scs_tenant
    ON platform.sensor_control_state (tenant_id);
CREATE INDEX IF NOT EXISTS idx_scs_tenant_desired_generation
    ON platform.sensor_control_state (tenant_id, desired_generation);
