-- Sensor Capability Pack assignment and desired-state (EMC1.3)
-- Desired assignment is Core-authoritative; Sensors pull and ACK applied generation/checksum.
-- No credentials/secrets. Generation 0 means never mutated.

CREATE TABLE IF NOT EXISTS platform.sensor_capability_pack_assignment (
    tenant_id UUID NOT NULL,
    sensor_id VARCHAR(128) NOT NULL,
    pack_id VARCHAR(128) NOT NULL,
    pack_version VARCHAR(64) NOT NULL,
    assigned_by VARCHAR(256) NOT NULL,
    assigned_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, sensor_id, pack_id),
    CONSTRAINT fk_scpa_sensor_same_tenant
        FOREIGN KEY (tenant_id, sensor_id)
        REFERENCES platform.sensor (tenant_id, sensor_id),
    CONSTRAINT fk_scpa_pack_same_tenant
        FOREIGN KEY (tenant_id, pack_id, pack_version)
        REFERENCES platform.tenant_capability_pack (tenant_id, pack_id, pack_version)
);

CREATE INDEX IF NOT EXISTS idx_scpa_tenant_sensor
    ON platform.sensor_capability_pack_assignment (tenant_id, sensor_id);
CREATE INDEX IF NOT EXISTS idx_scpa_tenant_pack
    ON platform.sensor_capability_pack_assignment (tenant_id, pack_id);

CREATE TABLE IF NOT EXISTS platform.sensor_capability_pack_state (
    tenant_id UUID NOT NULL,
    sensor_id VARCHAR(128) NOT NULL,
    desired_generation BIGINT NOT NULL,
    desired_checksum CHAR(64) NULL,
    applied_generation BIGINT NOT NULL,
    applied_checksum CHAR(64) NULL,
    applied_at TIMESTAMPTZ NULL,
    updated_by VARCHAR(256) NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, sensor_id),
    CONSTRAINT fk_scps_sensor_same_tenant
        FOREIGN KEY (tenant_id, sensor_id)
        REFERENCES platform.sensor (tenant_id, sensor_id),
    CONSTRAINT chk_scps_desired_generation CHECK (desired_generation >= 0),
    CONSTRAINT chk_scps_applied_generation CHECK (applied_generation >= 0),
    CONSTRAINT chk_scps_applied_le_desired CHECK (applied_generation <= desired_generation),
    CONSTRAINT chk_scps_desired_checksum_hex CHECK (
        desired_checksum IS NULL OR desired_checksum ~ '^[a-f0-9]{64}$'
    ),
    CONSTRAINT chk_scps_applied_checksum_hex CHECK (
        applied_checksum IS NULL OR applied_checksum ~ '^[a-f0-9]{64}$'
    )
);

CREATE INDEX IF NOT EXISTS idx_scps_tenant
    ON platform.sensor_capability_pack_state (tenant_id);
