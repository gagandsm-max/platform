-- Sensor enrollment bootstrap authority (B1)
-- Short-lived, single-use, tenant/deployment-scoped token authority.
-- Persist SHA-256 digest only — never plaintext token or private-key material.

CREATE TABLE IF NOT EXISTS platform.sensor_enrollment_authority (
    authority_id UUID NOT NULL,
    token_hash_sha256 CHAR(64) NOT NULL,
    tenant_id UUID NOT NULL,
    deployment_id VARCHAR(128) NOT NULL,
    site_id UUID NULL,
    role VARCHAR(32) NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (authority_id),
    CONSTRAINT uq_sensor_enrollment_token_hash UNIQUE (token_hash_sha256),
    CONSTRAINT fk_sea_tenant
        FOREIGN KEY (tenant_id) REFERENCES platform.tenant (tenant_id),
    CONSTRAINT fk_sea_site_same_tenant
        FOREIGN KEY (tenant_id, site_id)
        REFERENCES platform.site (tenant_id, site_id),
    CONSTRAINT chk_sea_token_hash_hex CHECK (
        token_hash_sha256 ~ '^[a-f0-9]{64}$'
    ),
    CONSTRAINT chk_sea_deployment_nonempty CHECK (
        LENGTH(TRIM(deployment_id)) > 0
    ),
    CONSTRAINT chk_sea_role CHECK (
        role IS NULL OR role IN (
            'NETWORK',
            'INFRASTRUCTURE',
            'STREAMING_EVENT',
            'SYNTHETIC',
            'CLOUD_API',
            'COMBINED'
        )
    ),
    CONSTRAINT chk_sea_expires_after_issued CHECK (expires_at > issued_at),
    CONSTRAINT chk_sea_consumed_after_issued CHECK (
        consumed_at IS NULL OR consumed_at >= issued_at
    ),
    CONSTRAINT chk_sea_revoked_after_issued CHECK (
        revoked_at IS NULL OR revoked_at >= issued_at
    ),
    CONSTRAINT chk_sea_terminal_xor CHECK (
        NOT (consumed_at IS NOT NULL AND revoked_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_sea_token_hash
    ON platform.sensor_enrollment_authority (token_hash_sha256);

CREATE INDEX IF NOT EXISTS idx_sea_tenant_deployment
    ON platform.sensor_enrollment_authority (tenant_id, deployment_id);

CREATE INDEX IF NOT EXISTS idx_sea_expires_at
    ON platform.sensor_enrollment_authority (expires_at)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;
