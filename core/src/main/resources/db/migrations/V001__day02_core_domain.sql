-- Day-02 Core domain model (Platform v3.2.1)
-- UUID canonical IDs, tenant-aware composite integrity, TIMESTAMPTZ throughout.

CREATE TABLE IF NOT EXISTS platform.schema_history (
    version VARCHAR(64) PRIMARY KEY,
    description VARCHAR(255) NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    application_version VARCHAR(64) NOT NULL
);

-- Safely migrate legacy Day-01 VARCHAR tenant table if present, preserving data and mapping
-- known bootstrap default row to canonical Day-02 UUID. If not present (fresh install), create table.
DO $$
DECLARE
    legacy_count INTEGER;
    migrated_count INTEGER;
    col_type TEXT;
    unsupported_count INTEGER;
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = 'platform' AND table_name = 'tenant'
    ) THEN
        SELECT data_type INTO col_type
        FROM information_schema.columns
        WHERE table_schema = 'platform' AND table_name = 'tenant' AND column_name = 'tenant_id';

        IF col_type <> 'uuid' THEN
            -- Fail closed on any unsupported legacy data before modifying schema
            SELECT COUNT(*) INTO unsupported_count
            FROM platform.tenant
            WHERE mode NOT IN ('SINGLE', 'MSP_MASTER', 'MSP_CHILD')
               OR status NOT IN ('ACTIVE', 'SUSPENDED', 'RETIRED')
               OR (tenant_id <> 'tenant-default-001' AND tenant_id !~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$');

            IF unsupported_count > 0 THEN
                RAISE EXCEPTION 'Cannot migrate legacy platform.tenant: % unsupported or corrupted row(s) detected', unsupported_count;
            END IF;

            SELECT COUNT(*) INTO legacy_count FROM platform.tenant;

            -- Rename legacy table to temporary staging table
            ALTER TABLE platform.tenant RENAME TO tenant_legacy_day01;

            -- Create normative Day-02 UUID tenant table
            CREATE TABLE platform.tenant (
                tenant_id UUID PRIMARY KEY,
                name VARCHAR(255) NOT NULL,
                mode VARCHAR(32) NOT NULL,
                status VARCHAR(32) NOT NULL,
                timezone VARCHAR(64) NOT NULL,
                created_at TIMESTAMPTZ NOT NULL,
                updated_at TIMESTAMPTZ NOT NULL,
                CONSTRAINT chk_tenant_mode CHECK (mode IN ('SINGLE', 'MSP_MASTER', 'MSP_CHILD')),
                CONSTRAINT chk_tenant_status CHECK (status IN ('ACTIVE', 'SUSPENDED', 'RETIRED'))
            );

            -- Migrate all legacy rows: deterministic map for default seed, cast for UUID-format seeds
            INSERT INTO platform.tenant (
                tenant_id, name, mode, status, timezone, created_at, updated_at
            )
            SELECT
                CASE
                    WHEN tenant_id = 'tenant-default-001' THEN '018f0e0a-0000-7000-8000-000000000001'::uuid
                    ELSE tenant_id::uuid
                END,
                name, mode, status, timezone, created_at, updated_at
            FROM platform.tenant_legacy_day01;

            -- Verify migrated row count matches legacy count
            SELECT COUNT(*) INTO migrated_count FROM platform.tenant;
            IF migrated_count <> legacy_count THEN
                RAISE EXCEPTION 'Legacy tenant migration row count mismatch: legacy=%, migrated=%', legacy_count, migrated_count;
            END IF;

            -- Drop intermediate staging table non-destructively (NO CASCADE)
            DROP TABLE platform.tenant_legacy_day01;
        END IF;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS platform.tenant (
    tenant_id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    mode VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    timezone VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CONSTRAINT chk_tenant_mode CHECK (mode IN ('SINGLE', 'MSP_MASTER', 'MSP_CHILD')),
    CONSTRAINT chk_tenant_status CHECK (status IN ('ACTIVE', 'SUSPENDED', 'RETIRED'))
);

CREATE INDEX IF NOT EXISTS idx_tenant_status ON platform.tenant (status);

CREATE TABLE IF NOT EXISTS platform.site (
    site_id UUID NOT NULL,
    tenant_id UUID NOT NULL REFERENCES platform.tenant (tenant_id),
    parent_site_id UUID NULL,
    name VARCHAR(255) NOT NULL,
    description VARCHAR(2000) NULL,
    timezone VARCHAR(64) NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (site_id),
    UNIQUE (tenant_id, site_id),
    CONSTRAINT chk_site_not_self_parent CHECK (parent_site_id IS NULL OR parent_site_id <> site_id),
    CONSTRAINT fk_site_parent_same_tenant
        FOREIGN KEY (tenant_id, parent_site_id)
        REFERENCES platform.site (tenant_id, site_id)
);

CREATE INDEX IF NOT EXISTS idx_site_tenant ON platform.site (tenant_id);
CREATE INDEX IF NOT EXISTS idx_site_tenant_parent ON platform.site (tenant_id, parent_site_id);

CREATE TABLE IF NOT EXISTS platform.entity_group (
    group_id UUID NOT NULL,
    tenant_id UUID NOT NULL REFERENCES platform.tenant (tenant_id),
    parent_group_id UUID NULL,
    name VARCHAR(255) NOT NULL,
    description VARCHAR(2000) NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (group_id),
    UNIQUE (tenant_id, group_id),
    CONSTRAINT chk_group_not_self_parent CHECK (parent_group_id IS NULL OR parent_group_id <> group_id),
    CONSTRAINT fk_group_parent_same_tenant
        FOREIGN KEY (tenant_id, parent_group_id)
        REFERENCES platform.entity_group (tenant_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_group_tenant ON platform.entity_group (tenant_id);
CREATE INDEX IF NOT EXISTS idx_group_tenant_parent ON platform.entity_group (tenant_id, parent_group_id);

CREATE TABLE IF NOT EXISTS platform.tag (
    tag_id UUID NOT NULL,
    tenant_id UUID NOT NULL REFERENCES platform.tenant (tenant_id),
    tag_key VARCHAR(128) NOT NULL,
    tag_value VARCHAR(512) NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tag_id),
    UNIQUE (tenant_id, tag_id)
);

-- Exact key/value duplicates forbidden within a tenant. NULL values are label-style tags;
-- PostgreSQL UNIQUE treats NULLs as distinct, so a partial index covers that case.
CREATE UNIQUE INDEX IF NOT EXISTS uq_tag_tenant_key_value
    ON platform.tag (tenant_id, tag_key, tag_value)
    WHERE tag_value IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_tag_tenant_key_null_value
    ON platform.tag (tenant_id, tag_key)
    WHERE tag_value IS NULL;

CREATE INDEX IF NOT EXISTS idx_tag_tenant ON platform.tag (tenant_id);
CREATE INDEX IF NOT EXISTS idx_tag_tenant_key ON platform.tag (tenant_id, tag_key);

CREATE TABLE IF NOT EXISTS platform.entity (
    entity_id UUID NOT NULL,
    tenant_id UUID NOT NULL REFERENCES platform.tenant (tenant_id),
    entity_type VARCHAR(64) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description VARCHAR(2000) NULL,
    discovered_name VARCHAR(255) NULL,
    lifecycle_state VARCHAR(32) NOT NULL,
    site_id UUID NULL,
    criticality VARCHAR(32) NULL,
    owner_team_id UUID NULL,
    device_pack_id VARCHAR(128) NULL,
    device_pack_version VARCHAR(64) NULL,
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    created_by VARCHAR(128) NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (entity_id),
    UNIQUE (tenant_id, entity_id),
    CONSTRAINT chk_entity_lifecycle CHECK (lifecycle_state IN ('DISCOVERED', 'ACTIVE', 'QUIESCED', 'RETIRED')),
    CONSTRAINT chk_entity_seen_order CHECK (last_seen_at >= first_seen_at),
    CONSTRAINT fk_entity_site_same_tenant
        FOREIGN KEY (tenant_id, site_id)
        REFERENCES platform.site (tenant_id, site_id)
);

CREATE INDEX IF NOT EXISTS idx_entity_tenant ON platform.entity (tenant_id);
CREATE INDEX IF NOT EXISTS idx_entity_tenant_type ON platform.entity (tenant_id, entity_type);
CREATE INDEX IF NOT EXISTS idx_entity_tenant_site ON platform.entity (tenant_id, site_id);

CREATE TABLE IF NOT EXISTS platform.entity_identifier (
    identifier_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    identifier_type VARCHAR(64) NOT NULL,
    namespace VARCHAR(128) NOT NULL,
    identifier_value VARCHAR(512) NOT NULL,
    confidence NUMERIC(6, 5) NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (identifier_id),
    UNIQUE (tenant_id, identifier_id),
    CONSTRAINT chk_identifier_confidence CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT chk_identifier_seen_order CHECK (last_seen_at >= first_seen_at),
    CONSTRAINT uq_identifier_resolution UNIQUE (tenant_id, namespace, identifier_type, identifier_value),
    CONSTRAINT fk_identifier_entity_same_tenant
        FOREIGN KEY (tenant_id, entity_id)
        REFERENCES platform.entity (tenant_id, entity_id)
);

CREATE INDEX IF NOT EXISTS idx_identifier_tenant_entity ON platform.entity_identifier (tenant_id, entity_id);
CREATE INDEX IF NOT EXISTS idx_identifier_lookup ON platform.entity_identifier (tenant_id, namespace, identifier_type, identifier_value);

CREATE TABLE IF NOT EXISTS platform.entity_attribute (
    attribute_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    schema_key VARCHAR(128) NOT NULL,
    value_json JSONB NOT NULL,
    source VARCHAR(64) NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (attribute_id),
    UNIQUE (tenant_id, entity_id, schema_key),
    CONSTRAINT fk_attribute_entity_same_tenant
        FOREIGN KEY (tenant_id, entity_id)
        REFERENCES platform.entity (tenant_id, entity_id)
);

CREATE INDEX IF NOT EXISTS idx_attribute_tenant_entity ON platform.entity_attribute (tenant_id, entity_id);

CREATE TABLE IF NOT EXISTS platform.entity_group_membership (
    tenant_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    group_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, entity_id, group_id),
    CONSTRAINT fk_membership_entity_same_tenant
        FOREIGN KEY (tenant_id, entity_id)
        REFERENCES platform.entity (tenant_id, entity_id),
    CONSTRAINT fk_membership_group_same_tenant
        FOREIGN KEY (tenant_id, group_id)
        REFERENCES platform.entity_group (tenant_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_membership_tenant_group ON platform.entity_group_membership (tenant_id, group_id);
CREATE INDEX IF NOT EXISTS idx_membership_tenant_entity ON platform.entity_group_membership (tenant_id, entity_id);

CREATE TABLE IF NOT EXISTS platform.entity_tag (
    tenant_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    tag_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, entity_id, tag_id),
    CONSTRAINT fk_entity_tag_entity_same_tenant
        FOREIGN KEY (tenant_id, entity_id)
        REFERENCES platform.entity (tenant_id, entity_id),
    CONSTRAINT fk_entity_tag_tag_same_tenant
        FOREIGN KEY (tenant_id, tag_id)
        REFERENCES platform.tag (tenant_id, tag_id)
);

CREATE INDEX IF NOT EXISTS idx_entity_tag_tenant_tag ON platform.entity_tag (tenant_id, tag_id);
CREATE INDEX IF NOT EXISTS idx_entity_tag_tenant_entity ON platform.entity_tag (tenant_id, entity_id);

-- Deterministic bootstrap default tenant for local/dev continuity with prior VARCHAR seed semantics.
INSERT INTO platform.tenant (
    tenant_id, name, mode, status, timezone, created_at, updated_at
) VALUES (
    '018f0e0a-0000-7000-8000-000000000001'::uuid,
    'Default Enterprise',
    'SINGLE',
    'ACTIVE',
    'UTC',
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
) ON CONFLICT (tenant_id) DO NOTHING;
