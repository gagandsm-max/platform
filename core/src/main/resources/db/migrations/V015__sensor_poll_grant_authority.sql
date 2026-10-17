-- Issue #229 G0: inert tenant-scoped Core poll-grant issuance ledger.
-- Additive only. V001–V014 remain immutable. No operational grant seed, no activation.
-- No monitoring-profile table/FK. No Sensor FK retrofit onto legacy watermarks.
-- Numeric cadence/slot/offline-horizon defaults remain unfrozen and are not invented here.

-- Prerequisite for complete same-tenant endpoint provenance on the grant ledger.
-- endpoint_id is already globally unique (PRIMARY KEY); this tuple UNIQUE enables the composite FK.
ALTER TABLE platform.entity_management_endpoint
    DROP CONSTRAINT IF EXISTS uq_eme_tenant_entity_endpoint;
ALTER TABLE platform.entity_management_endpoint
    ADD CONSTRAINT uq_eme_tenant_entity_endpoint
        UNIQUE (tenant_id, entity_id, endpoint_id);

-- Nullable pending authorization digest/version on the existing Sensor telemetry watermark.
-- Legacy claimed/unclaimed PENDING rows remain valid with NULL/NULL; no fictional backfill.
ALTER TABLE platform.sensor_telemetry_watermark
    ADD COLUMN IF NOT EXISTS pending_authorization_digest CHAR(64),
    ADD COLUMN IF NOT EXISTS pending_authorization_version INT;

ALTER TABLE platform.sensor_telemetry_watermark
    DROP CONSTRAINT IF EXISTS chk_sensor_telemetry_pending_authorization_pair;
ALTER TABLE platform.sensor_telemetry_watermark
    ADD CONSTRAINT chk_sensor_telemetry_pending_authorization_pair CHECK (
        (
            pending_authorization_digest IS NULL
            AND pending_authorization_version IS NULL
        )
        OR
        (
            pending_authorization_digest IS NOT NULL
            AND pending_authorization_version IS NOT NULL
            AND pending_authorization_digest ~ '^[a-f0-9]{64}$'
            AND pending_authorization_version >= 1
            AND pending_authorization_version <= 2147483647
        )
    );

CREATE TABLE IF NOT EXISTS platform.sensor_poll_grant (
    grant_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    sensor_id VARCHAR(128) NOT NULL,
    entity_id UUID NOT NULL,
    endpoint_id UUID NOT NULL,

    grant_generation BIGINT NOT NULL,
    assignment_generation BIGINT NOT NULL,

    protocol VARCHAR(32) NOT NULL,

    -- Immutable issued target snapshot (independent of later endpoint address updates).
    issued_address_type VARCHAR(16) NOT NULL,
    issued_address VARCHAR(255) NOT NULL,

    -- Inert versioned profile/plan binding. No physical monitoring-profile table/FK in G0.
    profile_plan_id VARCHAR(128) NOT NULL,
    profile_plan_version VARCHAR(64) NOT NULL,

    issued_at TIMESTAMPTZ NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    window_end TIMESTAMPTZ NOT NULL,
    issued_by VARCHAR(256) NOT NULL,

    -- Governed mutable status / revocation metadata.
    status VARCHAR(32) NOT NULL,
    revoked_at TIMESTAMPTZ NULL,
    revoked_by VARCHAR(256) NULL,
    revocation_class VARCHAR(32) NULL,
    revocation_reason VARCHAR(512) NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (grant_id),

    CONSTRAINT fk_spg_sensor_same_tenant
        FOREIGN KEY (tenant_id, sensor_id)
        REFERENCES platform.sensor (tenant_id, sensor_id),
    CONSTRAINT fk_spg_entity_same_tenant
        FOREIGN KEY (tenant_id, entity_id)
        REFERENCES platform.entity (tenant_id, entity_id),
    CONSTRAINT fk_spg_endpoint_same_tenant_entity
        FOREIGN KEY (tenant_id, entity_id, endpoint_id)
        REFERENCES platform.entity_management_endpoint (tenant_id, entity_id, endpoint_id),

    CONSTRAINT chk_spg_grant_generation_max_safe CHECK (
        grant_generation >= 1
        AND grant_generation <= 9007199254740991
    ),
    CONSTRAINT chk_spg_assignment_generation_max_safe CHECK (
        assignment_generation >= 0
        AND assignment_generation <= 9007199254740991
    ),

    CONSTRAINT chk_spg_protocol CHECK (protocol IN ('ICMP', 'SNMP')),
    CONSTRAINT chk_spg_issued_address_type CHECK (
        issued_address_type IN ('IPV4', 'IPV6', 'FQDN')
    ),
    CONSTRAINT chk_spg_issued_address_nonempty CHECK (
        LENGTH(TRIM(issued_address)) > 0
    ),
    CONSTRAINT chk_spg_profile_plan_id_nonempty CHECK (
        LENGTH(TRIM(profile_plan_id)) > 0
    ),
    CONSTRAINT chk_spg_profile_plan_version_nonempty CHECK (
        LENGTH(TRIM(profile_plan_version)) > 0
    ),
    CONSTRAINT chk_spg_issued_by_len CHECK (
        char_length(issued_by) BETWEEN 1 AND 256
    ),
    CONSTRAINT chk_spg_window_order CHECK (window_end > window_start),
    CONSTRAINT chk_spg_window_contains_issued CHECK (
        issued_at >= window_start
        AND issued_at <= window_end
    ),

    CONSTRAINT chk_spg_status CHECK (
        status IN ('ISSUED', 'REVOKED', 'SUPERSEDED')
    ),
    CONSTRAINT chk_spg_revocation_coherence CHECK (
        (
            status = 'ISSUED'
            AND revoked_at IS NULL
            AND revoked_by IS NULL
            AND revocation_class IS NULL
            AND revocation_reason IS NULL
        )
        OR
        (
            status IN ('REVOKED', 'SUPERSEDED')
            AND revoked_at IS NOT NULL
            AND revoked_at >= issued_at
            AND revocation_class IS NOT NULL
            AND revocation_class IN ('SECURITY', 'ROUTINE', 'SUPERSEDED')
            AND (revoked_by IS NULL OR char_length(revoked_by) BETWEEN 1 AND 256)
            AND (revocation_reason IS NULL OR char_length(revocation_reason) <= 512)
        )
    )
);

CREATE INDEX IF NOT EXISTS idx_spg_tenant_sensor
    ON platform.sensor_poll_grant (tenant_id, sensor_id);
CREATE INDEX IF NOT EXISTS idx_spg_tenant_sensor_status
    ON platform.sensor_poll_grant (tenant_id, sensor_id, status);
CREATE INDEX IF NOT EXISTS idx_spg_tenant_entity
    ON platform.sensor_poll_grant (tenant_id, entity_id);
CREATE INDEX IF NOT EXISTS idx_spg_tenant_endpoint
    ON platform.sensor_poll_grant (tenant_id, entity_id, endpoint_id);

-- Enforce immutability of issuance fields while permitting governed status/revocation updates.
CREATE OR REPLACE FUNCTION platform.sensor_poll_grant_protect_issued()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.grant_id IS DISTINCT FROM OLD.grant_id
        OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
        OR NEW.sensor_id IS DISTINCT FROM OLD.sensor_id
        OR NEW.entity_id IS DISTINCT FROM OLD.entity_id
        OR NEW.endpoint_id IS DISTINCT FROM OLD.endpoint_id
        OR NEW.grant_generation IS DISTINCT FROM OLD.grant_generation
        OR NEW.assignment_generation IS DISTINCT FROM OLD.assignment_generation
        OR NEW.protocol IS DISTINCT FROM OLD.protocol
        OR NEW.issued_address_type IS DISTINCT FROM OLD.issued_address_type
        OR NEW.issued_address IS DISTINCT FROM OLD.issued_address
        OR NEW.profile_plan_id IS DISTINCT FROM OLD.profile_plan_id
        OR NEW.profile_plan_version IS DISTINCT FROM OLD.profile_plan_version
        OR NEW.issued_at IS DISTINCT FROM OLD.issued_at
        OR NEW.window_start IS DISTINCT FROM OLD.window_start
        OR NEW.window_end IS DISTINCT FROM OLD.window_end
        OR NEW.issued_by IS DISTINCT FROM OLD.issued_by
    THEN
        RAISE EXCEPTION 'sensor_poll_grant issued fields are immutable (trg_spg_protect_issued)'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_spg_protect_issued ON platform.sensor_poll_grant;
CREATE TRIGGER trg_spg_protect_issued
    BEFORE UPDATE ON platform.sensor_poll_grant
    FOR EACH ROW
    EXECUTE FUNCTION platform.sensor_poll_grant_protect_issued();
