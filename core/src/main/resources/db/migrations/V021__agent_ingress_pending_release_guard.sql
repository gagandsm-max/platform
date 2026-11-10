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
    ADD COLUMN pending_canonical_batch_sha256 CHAR(64),
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
            AND pending_authority_validated_at IS NULL AND pending_canonical_batch_sha256 IS NULL
            AND pending_publication_attempted IS FALSE
            AND pending_publisher_owner IS NULL AND pending_publisher_claimed_at IS NULL
            AND pending_publisher_lease_until IS NULL)
        OR (pending_first_sequence IS NOT NULL AND pending_reservation_id IS NOT NULL
            AND ((pending_authority_state = 'UNVALIDATED'
                    AND pending_authority_validated_at IS NULL AND pending_canonical_batch_sha256 IS NULL
                    AND pending_publication_attempted IS FALSE
                    AND pending_publisher_owner IS NULL)
                OR (pending_authority_state = 'LEGACY_UNKNOWN'
                    AND pending_authority_validated_at IS NULL AND pending_canonical_batch_sha256 IS NULL
                    AND pending_publication_attempted IS NULL
                    AND pending_publisher_owner IS NULL)
                OR (pending_authority_state = 'AUTHORITY_VALIDATED'
                    AND pending_authority_validated_at IS NOT NULL
                    AND pending_canonical_batch_sha256 IS NOT NULL
                    AND pending_canonical_batch_sha256 ~ '^[0-9a-f]{64}$'
                    AND pending_publication_attempted IS NOT NULL)))
    );

-- One typed immutable canonical authority row per Sensor ingress sequence.
-- The identifier ID records the resolution evidence, but is deliberately not
-- FK-bound: a live A→B mapping may change while this pending A snapshot survives.
CREATE TABLE platform.sensor_agent_ingress_pending_canonical_binding (
    tenant_id UUID NOT NULL,
    sensor_id VARCHAR(255) NOT NULL,
    reservation_id UUID NOT NULL,
    sensor_ingress_sequence BIGINT NOT NULL,
    agent_id VARCHAR(128) NOT NULL,
    assignment_generation BIGINT NOT NULL,
    host_id VARCHAR(255) NOT NULL,
    entity_identifier_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    binding_version BIGINT NOT NULL,
    validated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (tenant_id, sensor_id, reservation_id, sensor_ingress_sequence),
    CONSTRAINT fk_pending_canonical_watermark_same_sensor
        FOREIGN KEY (tenant_id, sensor_id)
        REFERENCES platform.sensor_agent_ingress_watermark (tenant_id, sensor_id) ON DELETE CASCADE,
    CONSTRAINT fk_pending_canonical_entity_same_tenant
        FOREIGN KEY (tenant_id, entity_id) REFERENCES platform.entity (tenant_id, entity_id),
    CONSTRAINT fk_pending_canonical_agent_same_tenant
        FOREIGN KEY (tenant_id, agent_id) REFERENCES platform.agent (tenant_id, agent_id),
    CONSTRAINT chk_pending_canonical_sequence CHECK
        (sensor_ingress_sequence BETWEEN 1 AND 9007199254740991),
    CONSTRAINT chk_pending_canonical_generation CHECK
        (assignment_generation BETWEEN 1 AND 9007199254740991),
    CONSTRAINT chk_pending_canonical_binding_version CHECK (binding_version > 0),
    CONSTRAINT chk_pending_canonical_host CHECK (btrim(host_id) <> '')
);

CREATE INDEX idx_pending_canonical_reservation
    ON platform.sensor_agent_ingress_pending_canonical_binding (tenant_id, sensor_id, reservation_id);
