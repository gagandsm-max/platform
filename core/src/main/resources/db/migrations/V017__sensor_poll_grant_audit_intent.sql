-- Issue #294 G1b-C2: inert same-transaction poll-grant issuance audit intent.
-- Additive only. V001–V016 remain immutable. No operational seed. No historical backfill.
-- Captures grant_id + tenant_id + issued_at only from the AFTER INSERT NEW grant row (FK-validated by V015).
-- recorded_at is database-observed trigger execution time via clock_timestamp() (NOT transaction commit time).
-- No FK to sensor_poll_grant or tenant: audit intent must survive parent grant/tenant deletion.
-- Not BUILD-073, not cryptographic tamper evidence, not an authenticated human actor,
-- not retention/offboarding policy. Residual risk: privileged table-owner/DDL can still tamper.
-- No address, credential, secret, arbitrary text, or unrestricted JSON payload.

CREATE TABLE IF NOT EXISTS platform.sensor_poll_grant_audit_intent (
    grant_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    issued_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (grant_id)
);

CREATE INDEX IF NOT EXISTS idx_spgai_tenant
    ON platform.sensor_poll_grant_audit_intent (tenant_id);

COMMENT ON TABLE platform.sensor_poll_grant_audit_intent IS
    'Append-only issuance audit intent for sensor_poll_grant. '
    'grant_id/tenant_id/issued_at copied from the validated grant row in the same transaction. '
    'recorded_at is clock_timestamp() at AFTER INSERT trigger execution (DB observation time, not commit time). '
    'No FK to grant/tenant so intent survives parent deletion. '
    'Not cryptographic tamper evidence; not BUILD-073.';

COMMENT ON COLUMN platform.sensor_poll_grant_audit_intent.issued_at IS
    'Source grant event timestamp copied from NEW.issued_at.';

COMMENT ON COLUMN platform.sensor_poll_grant_audit_intent.recorded_at IS
    'Database-observed trigger execution time from clock_timestamp(); not transaction commit time.';

CREATE OR REPLACE FUNCTION platform.sensor_poll_grant_audit_intent_capture()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO platform.sensor_poll_grant_audit_intent (grant_id, tenant_id, issued_at, recorded_at)
    VALUES (NEW.grant_id, NEW.tenant_id, NEW.issued_at, clock_timestamp());
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_spg_audit_intent_after_insert ON platform.sensor_poll_grant;
CREATE TRIGGER trg_spg_audit_intent_after_insert
    AFTER INSERT ON platform.sensor_poll_grant
    FOR EACH ROW
    EXECUTE FUNCTION platform.sensor_poll_grant_audit_intent_capture();

CREATE OR REPLACE FUNCTION platform.sensor_poll_grant_audit_intent_deny_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'sensor_poll_grant_audit_intent is append-only (trg_spgai_deny_mutation)'
        USING ERRCODE = 'integrity_constraint_violation';
END;
$$;

DROP TRIGGER IF EXISTS trg_spgai_deny_mutation ON platform.sensor_poll_grant_audit_intent;
CREATE TRIGGER trg_spgai_deny_mutation
    BEFORE UPDATE OR DELETE ON platform.sensor_poll_grant_audit_intent
    FOR EACH ROW
    EXECUTE FUNCTION platform.sensor_poll_grant_audit_intent_deny_mutation();

DROP TRIGGER IF EXISTS trg_spgai_deny_truncate ON platform.sensor_poll_grant_audit_intent;
CREATE TRIGGER trg_spgai_deny_truncate
    BEFORE TRUNCATE ON platform.sensor_poll_grant_audit_intent
    FOR EACH STATEMENT
    EXECUTE FUNCTION platform.sensor_poll_grant_audit_intent_deny_mutation();
