-- Agent Control Authority Foundation (Platform v3.2.2)
-- PostgreSQL-authoritative Agent principal, replaceable certificate bindings, and Sensor assignment fence.
-- agent_id is case-sensitive VARCHAR (PostgreSQL default collation).
-- No private key material is stored.

CREATE TABLE IF NOT EXISTS platform.agent (
    agent_id VARCHAR(128) PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES platform.tenant (tenant_id),
    site_id UUID NULL,
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, agent_id),
    CONSTRAINT chk_agent_status CHECK (
        status IN ('ACTIVE', 'DISABLED', 'REVOKED')
    ),
    CONSTRAINT fk_agent_site_same_tenant
        FOREIGN KEY (tenant_id, site_id)
        REFERENCES platform.site (tenant_id, site_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_tenant ON platform.agent (tenant_id);
CREATE INDEX IF NOT EXISTS idx_agent_tenant_status ON platform.agent (tenant_id, status);

CREATE TABLE IF NOT EXISTS platform.agent_certificate_credential (
    credential_id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    agent_id VARCHAR(128) NOT NULL,
    fingerprint_sha256 CHAR(64) NOT NULL,
    spki_sha256 CHAR(64) NULL,
    serial_number VARCHAR(128) NOT NULL,
    issuer_dn VARCHAR(512) NOT NULL,
    issuer_fingerprint_sha256 CHAR(64) NULL,
    not_before TIMESTAMPTZ NOT NULL,
    not_after TIMESTAMPTZ NOT NULL,
    status VARCHAR(32) NOT NULL,
    activated_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT uq_agent_cert_fingerprint UNIQUE (fingerprint_sha256),
    CONSTRAINT chk_agent_cert_validity_window CHECK (not_after > not_before),
    CONSTRAINT chk_agent_cert_status CHECK (
        status IN ('ACTIVE', 'REVOKED', 'SUPERSEDED')
    ),
    CONSTRAINT chk_agent_cert_fingerprint_hex CHECK (
        fingerprint_sha256 ~ '^[a-f0-9]{64}$'
    ),
    CONSTRAINT chk_agent_cert_spki_hex CHECK (
        spki_sha256 IS NULL OR spki_sha256 ~ '^[a-f0-9]{64}$'
    ),
    CONSTRAINT chk_agent_cert_issuer_fingerprint_hex CHECK (
        issuer_fingerprint_sha256 IS NULL OR issuer_fingerprint_sha256 ~ '^[a-f0-9]{64}$'
    ),
    CONSTRAINT fk_agent_cert_agent_same_tenant
        FOREIGN KEY (tenant_id, agent_id)
        REFERENCES platform.agent (tenant_id, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_cert_agent
    ON platform.agent_certificate_credential (agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_cert_tenant_agent
    ON platform.agent_certificate_credential (tenant_id, agent_id);

CREATE TABLE IF NOT EXISTS platform.agent_sensor_assignment (
    assignment_id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    agent_id VARCHAR(128) NOT NULL,
    sensor_id VARCHAR(128) NOT NULL,
    generation BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    CONSTRAINT chk_agent_sensor_assignment_generation CHECK (generation >= 1),
    CONSTRAINT chk_agent_sensor_assignment_status CHECK (
        status IN ('ACTIVE', 'REVOKED')
    ),
    CONSTRAINT uq_agent_sensor_assignment_generation UNIQUE (tenant_id, agent_id, generation),
    CONSTRAINT fk_agent_sensor_assignment_agent_same_tenant
        FOREIGN KEY (tenant_id, agent_id)
        REFERENCES platform.agent (tenant_id, agent_id),
    CONSTRAINT fk_agent_sensor_assignment_sensor_same_tenant
        FOREIGN KEY (tenant_id, sensor_id)
        REFERENCES platform.sensor (tenant_id, sensor_id)
);

-- One ACTIVE assignment per agent (monotonic fence via generation in application code).
CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_sensor_assignment_active_agent
    ON platform.agent_sensor_assignment (tenant_id, agent_id)
    WHERE status = 'ACTIVE';

-- Sensor-side lookup of active Agent assignment.
CREATE INDEX IF NOT EXISTS idx_agent_sensor_assignment_active_sensor
    ON platform.agent_sensor_assignment (tenant_id, sensor_id, status)
    WHERE status = 'ACTIVE';
