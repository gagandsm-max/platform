-- Issue #288 G1b-C3: inert Core-side Sensor execution incarnation authority.
-- Additive only. V001–V017 remain immutable. No operational seed. No legacy backfill.
-- Core generates opaque incarnation_id and MAX_SAFE-bounded monotonic incarnation_generation
-- per existing Sensor principal. At most one ACTIVE incarnation per (tenant_id, sensor_id).
-- Credential rotation, bootEpoch and normal restarts never define or mutate this table.
-- Inert control authority (not durable audit evidence): parent Sensor FK CASCADE preserves
-- existing Sensor/tenant teardown without inventing audit survival or touching V017 protections.
-- No enrollment binding, grant delivery, polling activation or telemetry authorization.

CREATE TABLE IF NOT EXISTS platform.sensor_execution_incarnation (
    incarnation_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    sensor_id VARCHAR(128) NOT NULL,
    incarnation_generation BIGINT NOT NULL,
    status VARCHAR(32) NOT NULL,
    activated_at TIMESTAMPTZ NOT NULL,
    superseded_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,

    PRIMARY KEY (incarnation_id),

    CONSTRAINT fk_sei_sensor_same_tenant
        FOREIGN KEY (tenant_id, sensor_id)
        REFERENCES platform.sensor (tenant_id, sensor_id)
        ON DELETE CASCADE,

    CONSTRAINT uq_sei_tenant_sensor_generation
        UNIQUE (tenant_id, sensor_id, incarnation_generation),

    CONSTRAINT chk_sei_generation_max_safe CHECK (
        incarnation_generation >= 1
        AND incarnation_generation <= 9007199254740991
    ),
    CONSTRAINT chk_sei_status CHECK (
        status IN ('ACTIVE', 'SUPERSEDED')
    ),
    CONSTRAINT chk_sei_status_coherence CHECK (
        (
            status = 'ACTIVE'
            AND superseded_at IS NULL
        )
        OR
        (
            status = 'SUPERSEDED'
            AND superseded_at IS NOT NULL
            AND superseded_at >= activated_at
        )
    )
);

COMMENT ON TABLE platform.sensor_execution_incarnation IS
    'Inert Core-authoritative Sensor execution incarnation. '
    'Core-minted opaque incarnation_id; monotonic incarnation_generation per principal. '
    'Absent for legacy Sensors (no backfill). Not credential identity, bootEpoch or audit evidence. '
    'CASCADE on Sensor delete preserves teardown; does not authorize grants or telemetry.';

COMMENT ON COLUMN platform.sensor_execution_incarnation.incarnation_id IS
    'Opaque Core-generated identity; never client-selected.';

COMMENT ON COLUMN platform.sensor_execution_incarnation.incarnation_generation IS
    'Per-principal monotonic generation in [1, MAX_SAFE_SEQUENCE]; allocated by Core under principal lock.';

-- At most one ACTIVE incarnation per existing Sensor principal.
CREATE UNIQUE INDEX IF NOT EXISTS uq_sei_one_active_per_principal
    ON platform.sensor_execution_incarnation (tenant_id, sensor_id)
    WHERE status = 'ACTIVE';

CREATE INDEX IF NOT EXISTS idx_sei_tenant_sensor
    ON platform.sensor_execution_incarnation (tenant_id, sensor_id);

CREATE INDEX IF NOT EXISTS idx_sei_tenant_sensor_status
    ON platform.sensor_execution_incarnation (tenant_id, sensor_id, status);

-- Identity/generation/activation are immutable; SUPERSEDED cannot reactivate to ACTIVE.
CREATE OR REPLACE FUNCTION platform.sensor_execution_incarnation_protect()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.status = 'SUPERSEDED' AND NEW.status = 'ACTIVE' THEN
        RAISE EXCEPTION 'sensor_execution_incarnation terminal status cannot reactivate to ACTIVE (trg_sei_protect)'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF NEW.incarnation_id IS DISTINCT FROM OLD.incarnation_id
        OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
        OR NEW.sensor_id IS DISTINCT FROM OLD.sensor_id
        OR NEW.incarnation_generation IS DISTINCT FROM OLD.incarnation_generation
        OR NEW.activated_at IS DISTINCT FROM OLD.activated_at
        OR NEW.created_at IS DISTINCT FROM OLD.created_at
    THEN
        RAISE EXCEPTION 'sensor_execution_incarnation identity fields are immutable (trg_sei_protect)'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    IF OLD.status = 'ACTIVE' AND NEW.status = 'SUPERSEDED' THEN
        RETURN NEW;
    END IF;
    IF NEW.status IS DISTINCT FROM OLD.status
        OR NEW.superseded_at IS DISTINCT FROM OLD.superseded_at
    THEN
        RAISE EXCEPTION 'sensor_execution_incarnation status transition is not permitted (trg_sei_protect)'
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_sei_protect ON platform.sensor_execution_incarnation;
CREATE TRIGGER trg_sei_protect
    BEFORE UPDATE ON platform.sensor_execution_incarnation
    FOR EACH ROW
    EXECUTE FUNCTION platform.sensor_execution_incarnation_protect();
