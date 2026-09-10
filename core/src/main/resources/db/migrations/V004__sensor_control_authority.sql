-- Sensor Control Authority Foundation (Platform v3.2.2)
-- PostgreSQL-authoritative Sensor principal, replaceable certificate bindings, and entity allowlist.
-- Capabilities / DISCOVERY / AGENT_INGRESS are derived from role in application code (mirror sensor/internal/runtime/role.go).
-- No private key material is stored.

CREATE TABLE IF NOT EXISTS platform.sensor (
    sensor_id VARCHAR(128) PRIMARY KEY,
    tenant_id UUID NOT NULL REFERENCES platform.tenant (tenant_id),
    site_id UUID NULL,
    role VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    allow_all_tenant_entities BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, sensor_id),
    CONSTRAINT chk_sensor_role CHECK (
        role IN (
            'NETWORK',
            'INFRASTRUCTURE',
            'STREAMING_EVENT',
            'SYNTHETIC',
            'CLOUD_API',
            'COMBINED'
        )
    ),
    CONSTRAINT chk_sensor_status CHECK (
        status IN ('ACTIVE', 'DISABLED', 'REVOKED')
    ),
    CONSTRAINT fk_sensor_site_same_tenant
        FOREIGN KEY (tenant_id, site_id)
        REFERENCES platform.site (tenant_id, site_id)
);

CREATE INDEX IF NOT EXISTS idx_sensor_tenant ON platform.sensor (tenant_id);
CREATE INDEX IF NOT EXISTS idx_sensor_tenant_status ON platform.sensor (tenant_id, status);

CREATE TABLE IF NOT EXISTS platform.sensor_certificate_credential (
    credential_id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    sensor_id VARCHAR(128) NOT NULL,
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
    CONSTRAINT uq_sensor_cert_fingerprint UNIQUE (fingerprint_sha256),
    CONSTRAINT chk_sensor_cert_validity_window CHECK (not_after > not_before),
    CONSTRAINT chk_sensor_cert_status CHECK (
        status IN ('ACTIVE', 'REVOKED', 'SUPERSEDED')
    ),
    CONSTRAINT chk_sensor_cert_fingerprint_hex CHECK (
        fingerprint_sha256 ~ '^[a-f0-9]{64}$'
    ),
    CONSTRAINT chk_sensor_cert_spki_hex CHECK (
        spki_sha256 IS NULL OR spki_sha256 ~ '^[a-f0-9]{64}$'
    ),
    CONSTRAINT chk_sensor_cert_issuer_fingerprint_hex CHECK (
        issuer_fingerprint_sha256 IS NULL OR issuer_fingerprint_sha256 ~ '^[a-f0-9]{64}$'
    ),
    CONSTRAINT fk_sensor_cert_sensor_same_tenant
        FOREIGN KEY (tenant_id, sensor_id)
        REFERENCES platform.sensor (tenant_id, sensor_id)
);

CREATE INDEX IF NOT EXISTS idx_sensor_cert_sensor
    ON platform.sensor_certificate_credential (sensor_id);
CREATE INDEX IF NOT EXISTS idx_sensor_cert_tenant_sensor
    ON platform.sensor_certificate_credential (tenant_id, sensor_id);

CREATE TABLE IF NOT EXISTS platform.sensor_allowed_entity (
    tenant_id UUID NOT NULL,
    sensor_id VARCHAR(128) NOT NULL,
    entity_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, sensor_id, entity_id),
    CONSTRAINT fk_sensor_allowed_sensor_same_tenant
        FOREIGN KEY (tenant_id, sensor_id)
        REFERENCES platform.sensor (tenant_id, sensor_id),
    CONSTRAINT fk_sensor_allowed_entity_same_tenant
        FOREIGN KEY (tenant_id, entity_id)
        REFERENCES platform.entity (tenant_id, entity_id)
);

CREATE INDEX IF NOT EXISTS idx_sensor_allowed_entity_sensor
    ON platform.sensor_allowed_entity (tenant_id, sensor_id);
CREATE INDEX IF NOT EXISTS idx_sensor_allowed_entity_entity
    ON platform.sensor_allowed_entity (tenant_id, entity_id);
