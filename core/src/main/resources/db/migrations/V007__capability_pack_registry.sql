-- Tenant-scoped Capability Pack registry (EMC1.2)
-- Immutable pack artifacts + activation history. No credentials/secrets.

CREATE TABLE IF NOT EXISTS platform.tenant_capability_pack (
    tenant_id UUID NOT NULL REFERENCES platform.tenant (tenant_id),
    pack_id VARCHAR(128) NOT NULL,
    pack_version VARCHAR(64) NOT NULL,
    schema_version VARCHAR(32) NOT NULL,
    pack_class VARCHAR(64) NOT NULL,
    name VARCHAR(160) NOT NULL,
    publisher_type VARCHAR(32) NOT NULL,
    publisher_name VARCHAR(160) NOT NULL,
    certification VARCHAR(64) NOT NULL,
    manifest_sha256 CHAR(64) NOT NULL,
    manifest_bytes BYTEA NOT NULL,
    created_by VARCHAR(256) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, pack_id, pack_version),
    CONSTRAINT chk_tcp_schema_version CHECK (schema_version = '1.0.0'),
    CONSTRAINT chk_tcp_pack_class CHECK (pack_class = 'DEVICE_NETWORK'),
    CONSTRAINT chk_tcp_manifest_sha256_hex CHECK (manifest_sha256 ~ '^[a-f0-9]{64}$'),
    CONSTRAINT chk_tcp_manifest_bytes_size CHECK (
        octet_length(manifest_bytes) > 0
        AND octet_length(manifest_bytes) <= 262144
    )
);

CREATE INDEX IF NOT EXISTS idx_tcp_tenant
    ON platform.tenant_capability_pack (tenant_id);
CREATE INDEX IF NOT EXISTS idx_tcp_tenant_pack
    ON platform.tenant_capability_pack (tenant_id, pack_id);

CREATE TABLE IF NOT EXISTS platform.tenant_capability_pack_activation (
    activation_id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    pack_id VARCHAR(128) NOT NULL,
    pack_version VARCHAR(64) NOT NULL,
    generation BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL,
    activated_by VARCHAR(256) NOT NULL,
    activated_at TIMESTAMPTZ NOT NULL,
    revoked_by VARCHAR(256) NULL,
    revoked_at TIMESTAMPTZ NULL,
    correlation_id VARCHAR(128) NULL,
    CONSTRAINT chk_tcpa_generation CHECK (generation >= 1),
    CONSTRAINT chk_tcpa_status CHECK (status IN ('ACTIVE', 'REVOKED')),
    CONSTRAINT uq_tcpa_tenant_pack_generation UNIQUE (tenant_id, pack_id, generation),
    CONSTRAINT fk_tcpa_pack_same_tenant
        FOREIGN KEY (tenant_id, pack_id, pack_version)
        REFERENCES platform.tenant_capability_pack (tenant_id, pack_id, pack_version)
);

-- One ACTIVE activation per (tenant, pack_id).
CREATE UNIQUE INDEX IF NOT EXISTS uq_tcpa_active_pack
    ON platform.tenant_capability_pack_activation (tenant_id, pack_id)
    WHERE status = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_tcpa_tenant
    ON platform.tenant_capability_pack_activation (tenant_id);
CREATE INDEX IF NOT EXISTS idx_tcpa_tenant_pack_status
    ON platform.tenant_capability_pack_activation (tenant_id, pack_id, status);
