-- Fence old writers. Dormant V020 pending work retains custody and unknown
-- publication history; only active publishers prevent the schema cutover.
LOCK TABLE platform.sensor_agent_ingress_watermark IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM platform.sensor_agent_ingress_watermark
         WHERE pending_publisher_owner IS NOT NULL
            OR pending_publisher_claimed_at IS NOT NULL
            OR pending_publisher_lease_until IS NOT NULL
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = 'P0211',
            MESSAGE = 'AGENT_INGRESS_PUBLISHER_UPGRADE_BLOCKED',
            DETAIL = 'Drain active Agent-ingress publisher ownership and leases before retrying V021';
    END IF;
END;
$$;

ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD COLUMN pending_reservation_id UUID,
    ADD COLUMN pending_authority_state VARCHAR(24) NOT NULL DEFAULT 'NONE',
    ADD COLUMN pending_authority_validated_at TIMESTAMPTZ,
    ADD COLUMN pending_entity_ids UUID[],
    ADD COLUMN pending_publication_attempted BOOLEAN DEFAULT FALSE;

-- Preserve every V020 pending identity and watermark exactly. NULL means the
-- old writer may or may not have committed Kafka before this cutover.
UPDATE platform.sensor_agent_ingress_watermark
   SET pending_reservation_id = gen_random_uuid(),
       pending_authority_state = 'LEGACY_UNKNOWN',
       pending_publication_attempted = NULL
 WHERE pending_first_sequence IS NOT NULL;

ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD CONSTRAINT chk_agent_ingress_pending_release_guard CHECK (
        (pending_first_sequence IS NULL AND pending_reservation_id IS NULL
            AND pending_authority_state = 'NONE'
            AND pending_authority_validated_at IS NULL AND pending_entity_ids IS NULL
            AND pending_publication_attempted IS FALSE
            AND pending_publisher_owner IS NULL AND pending_publisher_claimed_at IS NULL
            AND pending_publisher_lease_until IS NULL)
        OR (pending_first_sequence IS NOT NULL AND pending_reservation_id IS NOT NULL
            AND ((pending_authority_state = 'UNVALIDATED'
                    AND pending_authority_validated_at IS NULL AND pending_entity_ids IS NULL
                    AND pending_publication_attempted IS FALSE
                    AND pending_publisher_owner IS NULL)
                OR (pending_authority_state = 'LEGACY_UNKNOWN'
                    AND pending_authority_validated_at IS NULL AND pending_entity_ids IS NULL
                    AND pending_publication_attempted IS NULL
                    AND pending_publisher_owner IS NULL)
                OR (pending_authority_state = 'AUTHORITY_VALIDATED'
                    AND pending_authority_validated_at IS NOT NULL
                    AND pending_entity_ids IS NOT NULL
                    AND cardinality(pending_entity_ids) BETWEEN 1 AND 1000
                    AND cardinality(pending_entity_ids) = pending_last_sequence - pending_first_sequence + 1
                    AND array_position(pending_entity_ids, NULL) IS NULL
                    AND pending_publication_attempted IS NOT NULL)))
    );
