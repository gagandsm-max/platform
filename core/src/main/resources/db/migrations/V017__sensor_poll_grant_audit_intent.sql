-- Issue #294 G1b-C2: inert same-transaction poll-grant issuance audit intent.
-- Additive only. V001–V016 remain immutable. No operational seed. No historical backfill.
-- Captures grant_id + tenant_id only from the AFTER INSERT NEW grant row (FK-validated by V015).
-- No FK to sensor_poll_grant or tenant: audit intent must survive parent grant/tenant deletion.
-- Not BUILD-073, not cryptographic tamper evidence, not an authenticated human actor,
-- not retention/offboarding policy. Residual risk: privileged table-owner/DDL can still tamper.
-- No address, credential, secret, arbitrary text, or unrestricted JSON payload.

CREATE TABLE IF NOT EXISTS platform.sensor_poll_grant_audit_intent (
    grant_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (grant_id)
);

CREATE INDEX IF NOT EXISTS idx_spgai_tenant
    ON platform.sensor_poll_grant_audit_intent (tenant_id);

COMMENT ON TABLE platform.sensor_poll_grant_audit_intent IS
    'Append-only issuance audit intent for sensor_poll_grant. '
    'grant_id/tenant_id copied from the validated grant row in the same transaction. '
    'No FK to grant/tenant so intent survives parent deletion. '
    'Not cryptographic tamper evidence; not BUILD-073.';

CREATE OR REPLACE FUNCTION platform.sensor_poll_grant_audit_intent_capture()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO platform.sensor_poll_grant_audit_intent (grant_id, tenant_id, recorded_at)
    VALUES (NEW.grant_id, NEW.tenant_id, NEW.issued_at);
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
