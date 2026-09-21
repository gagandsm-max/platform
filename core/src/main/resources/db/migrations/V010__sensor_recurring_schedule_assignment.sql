-- Sensor recurring monitoring schedule assignment and desired-state (Slice A)
-- Desired assignment is Core-authoritative; Sensors pull and ACK applied generation/checksum.
-- No credentials/secrets. Generation 0 means never mutated.
-- protocol_params_json is bounded non-secret SNMP collection params only.

CREATE TABLE IF NOT EXISTS platform.sensor_recurring_assignment (
    tenant_id UUID NOT NULL,
    sensor_id VARCHAR(128) NOT NULL,
    assignment_id VARCHAR(128) NOT NULL,
    entity_id VARCHAR(128) NOT NULL,
    monitor_definition_id VARCHAR(128) NOT NULL,
    scheduling_class VARCHAR(64) NOT NULL,
    execution_class VARCHAR(64) NOT NULL,
    cadence_ms BIGINT NOT NULL,
    freshness_ms BIGINT NOT NULL,
    phase_version INT NOT NULL,
    execution_timeout_ms BIGINT NOT NULL,
    credential_profile_ref VARCHAR(256) NOT NULL,
    protocol VARCHAR(32) NOT NULL,
    protocol_params_json TEXT NOT NULL,
    assigned_by VARCHAR(256) NOT NULL,
    assigned_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, sensor_id, assignment_id),
    CONSTRAINT fk_sra_sensor_same_tenant
        FOREIGN KEY (tenant_id, sensor_id)
        REFERENCES platform.sensor (tenant_id, sensor_id),
    CONSTRAINT chk_sra_cadence_ms CHECK (cadence_ms > 0),
    CONSTRAINT chk_sra_freshness_ms CHECK (freshness_ms > 0),
    CONSTRAINT chk_sra_phase_version CHECK (phase_version >= 1),
    CONSTRAINT chk_sra_execution_timeout_ms CHECK (execution_timeout_ms > 0),
    CONSTRAINT chk_sra_protocol CHECK (protocol IN ('SNMP')),
    CONSTRAINT chk_sra_scheduling_class_nonempty CHECK (LENGTH(TRIM(scheduling_class)) > 0),
    CONSTRAINT chk_sra_execution_class_nonempty CHECK (LENGTH(TRIM(execution_class)) > 0)
);

CREATE INDEX IF NOT EXISTS idx_sra_tenant_sensor
    ON platform.sensor_recurring_assignment (tenant_id, sensor_id);
CREATE INDEX IF NOT EXISTS idx_sra_tenant_entity
    ON platform.sensor_recurring_assignment (tenant_id, entity_id);

CREATE TABLE IF NOT EXISTS platform.sensor_recurring_state (
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
    CONSTRAINT fk_srs_sensor_same_tenant
        FOREIGN KEY (tenant_id, sensor_id)
        REFERENCES platform.sensor (tenant_id, sensor_id),
    CONSTRAINT chk_srs_desired_generation CHECK (desired_generation >= 0),
    CONSTRAINT chk_srs_applied_generation CHECK (applied_generation >= 0),
    CONSTRAINT chk_srs_applied_le_desired CHECK (applied_generation <= desired_generation),
    CONSTRAINT chk_srs_desired_checksum_hex CHECK (
        desired_checksum IS NULL OR desired_checksum ~ '^[a-f0-9]{64}$'
    ),
    CONSTRAINT chk_srs_applied_checksum_hex CHECK (
        applied_checksum IS NULL OR applied_checksum ~ '^[a-f0-9]{64}$'
    )
);

CREATE INDEX IF NOT EXISTS idx_srs_tenant
    ON platform.sensor_recurring_state (tenant_id);
