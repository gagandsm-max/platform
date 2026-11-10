-- V020 pending work has unknown publication state. Quiesce admission and drain it
-- with the old Core before upgrading. Keep the table locked through this migration
-- so an in-flight old writer cannot reserve after the precondition is checked.
LOCK TABLE platform.sensor_agent_ingress_watermark IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM platform.sensor_agent_ingress_watermark
         WHERE pending_first_sequence IS NOT NULL
            OR pending_publisher_owner IS NOT NULL
            OR pending_publisher_claimed_at IS NOT NULL
            OR pending_publisher_lease_until IS NOT NULL
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = 'P0211',
            MESSAGE = 'AGENT_INGRESS_PENDING_UPGRADE_BLOCKED',
            DETAIL = 'Drain or reconcile Agent-ingress pending work with the old Core, verify zero pending reservations and publisher leases, then retry V021';
    END IF;
END;
$$;

ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD COLUMN pending_reservation_id UUID,
    ADD COLUMN pending_publication_attempted BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD CONSTRAINT chk_agent_ingress_pending_release_guard CHECK (
        (pending_first_sequence IS NULL AND pending_reservation_id IS NULL
            AND pending_publication_attempted = FALSE)
        OR (pending_first_sequence IS NOT NULL AND pending_reservation_id IS NOT NULL)
    );
