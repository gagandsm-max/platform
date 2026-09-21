-- Entity management endpoint — authoritative poll/management host binding (Slice A)
-- Distinct from observational entity_identifier and discovery target_address.
-- No credentials or protocol secrets.

CREATE TABLE IF NOT EXISTS platform.entity_management_endpoint (
    endpoint_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    address_type VARCHAR(16) NOT NULL,
    address VARCHAR(255) NOT NULL,
    state VARCHAR(16) NOT NULL,
    source VARCHAR(32) NOT NULL,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_by VARCHAR(256) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_by VARCHAR(256) NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (endpoint_id),
    CONSTRAINT fk_eme_tenant
        FOREIGN KEY (tenant_id) REFERENCES platform.tenant (tenant_id),
    CONSTRAINT fk_eme_entity_same_tenant
        FOREIGN KEY (tenant_id, entity_id)
        REFERENCES platform.entity (tenant_id, entity_id),
    CONSTRAINT chk_eme_address_type CHECK (address_type IN ('IPV4', 'IPV6', 'FQDN')),
    CONSTRAINT chk_eme_state CHECK (state IN ('ACTIVE', 'INACTIVE')),
    CONSTRAINT chk_eme_source CHECK (source IN ('MANUAL', 'DISCOVERY', 'PROVISIONING')),
    CONSTRAINT chk_eme_address_nonempty CHECK (LENGTH(TRIM(address)) > 0),
    CONSTRAINT chk_eme_primary_requires_active CHECK (
        (is_primary = FALSE) OR (state = 'ACTIVE')
    )
);

-- At most one ACTIVE primary management endpoint per entity.
CREATE UNIQUE INDEX IF NOT EXISTS uq_eme_one_active_primary
    ON platform.entity_management_endpoint (tenant_id, entity_id)
    WHERE is_primary = TRUE AND state = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_eme_tenant_entity
    ON platform.entity_management_endpoint (tenant_id, entity_id);

CREATE INDEX IF NOT EXISTS idx_eme_tenant_entity_state
    ON platform.entity_management_endpoint (tenant_id, entity_id, state);
