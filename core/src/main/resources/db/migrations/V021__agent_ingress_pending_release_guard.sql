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
    ADD COLUMN pending_canonical_observation_sha256 CHAR(64),
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
            AND pending_canonical_observation_sha256 IS NULL
            AND pending_publication_attempted IS FALSE
            AND pending_publisher_owner IS NULL AND pending_publisher_claimed_at IS NULL
            AND pending_publisher_lease_until IS NULL)
        OR (pending_first_sequence IS NOT NULL AND pending_reservation_id IS NOT NULL
            AND ((pending_authority_state = 'UNVALIDATED'
                    AND pending_authority_validated_at IS NULL AND pending_canonical_batch_sha256 IS NULL
                    AND pending_canonical_observation_sha256 IS NULL
                    AND pending_publication_attempted IS FALSE
                    AND pending_publisher_owner IS NULL)
                OR (pending_authority_state = 'LEGACY_UNKNOWN'
                    AND pending_authority_validated_at IS NULL AND pending_canonical_batch_sha256 IS NULL
                    AND pending_canonical_observation_sha256 IS NULL
                    AND pending_publication_attempted IS NULL
                    AND pending_publisher_owner IS NULL)
                OR (pending_authority_state = 'AUTHORITY_VALIDATED'
                    AND pending_authority_validated_at IS NOT NULL
                    AND pending_canonical_batch_sha256 IS NOT NULL
                    AND pending_canonical_batch_sha256 ~ '^[0-9a-f]{64}$'
                    AND pending_canonical_observation_sha256 ~ '^[0-9a-f]{64}$'
                    AND pending_publication_attempted IS NOT NULL)))
    );

-- Start provenance at V021. Pre-existing identifier rows are deliberately not
-- backfilled: their earlier entity mapping cannot be proved from mutable state.
CREATE TABLE platform.entity_identifier_binding_history (
    history_version BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id UUID NOT NULL,
    namespace VARCHAR(128) NOT NULL,
    identifier_type VARCHAR(64) NOT NULL,
    identifier_value VARCHAR(512) NOT NULL,
    entity_identifier_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    confidence NUMERIC(6, 5) NOT NULL,
    valid_from TIMESTAMPTZ NOT NULL,
    valid_to TIMESTAMPTZ,
    provenance VARCHAR(64) NOT NULL DEFAULT 'ENTITY_IDENTIFIER_WRITE_V021',
    CONSTRAINT chk_identifier_history_interval CHECK (valid_to IS NULL OR valid_to > valid_from),
    CONSTRAINT chk_identifier_history_provenance CHECK (provenance = 'ENTITY_IDENTIFIER_WRITE_V021'),
    CONSTRAINT fk_identifier_history_entity_same_tenant FOREIGN KEY (tenant_id, entity_id)
        REFERENCES platform.entity (tenant_id, entity_id)
);

CREATE UNIQUE INDEX uq_identifier_history_open_key
    ON platform.entity_identifier_binding_history
    (tenant_id, namespace, identifier_type, identifier_value) WHERE valid_to IS NULL;
CREATE INDEX idx_identifier_history_interval
    ON platform.entity_identifier_binding_history
    (tenant_id, namespace, identifier_type, identifier_value, valid_from, valid_to);

CREATE FUNCTION platform.guard_entity_identifier_binding_history() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF pg_trigger_depth() < 2 THEN
        RAISE EXCEPTION 'identifier history requires entity_identifier write provenance';
    END IF;
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'authoritative identifier history cannot be deleted';
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF OLD.valid_to IS NOT NULL OR NEW.valid_to IS NULL
           OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
           OR NEW.namespace IS DISTINCT FROM OLD.namespace
           OR NEW.identifier_type IS DISTINCT FROM OLD.identifier_type
           OR NEW.identifier_value IS DISTINCT FROM OLD.identifier_value
           OR NEW.entity_identifier_id IS DISTINCT FROM OLD.entity_identifier_id
           OR NEW.entity_id IS DISTINCT FROM OLD.entity_id
           OR NEW.confidence IS DISTINCT FROM OLD.confidence
           OR NEW.valid_from IS DISTINCT FROM OLD.valid_from
           OR NEW.provenance IS DISTINCT FROM OLD.provenance
           OR NEW.history_version IS DISTINCT FROM OLD.history_version THEN
            RAISE EXCEPTION 'authoritative identifier history is append and close only';
        END IF;
        RETURN NEW;
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(
        NEW.tenant_id::text || ':' || NEW.namespace || ':' || NEW.identifier_type || ':' || NEW.identifier_value, 0));
    IF EXISTS (SELECT 1 FROM platform.entity_identifier_binding_history h
                WHERE h.tenant_id = NEW.tenant_id AND h.namespace = NEW.namespace
                  AND h.identifier_type = NEW.identifier_type AND h.identifier_value = NEW.identifier_value
                  AND (h.valid_to IS NULL OR h.valid_to > NEW.valid_from)
                  AND (NEW.valid_to IS NULL OR NEW.valid_to > h.valid_from)) THEN
        RAISE EXCEPTION 'overlapping authoritative identifier history';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_guard_entity_identifier_binding_history
BEFORE INSERT OR UPDATE OR DELETE ON platform.entity_identifier_binding_history
FOR EACH ROW EXECUTE FUNCTION platform.guard_entity_identifier_binding_history();

CREATE FUNCTION platform.record_entity_identifier_binding_history() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    boundary TIMESTAMPTZ;
    previous_from TIMESTAMPTZ;
BEGIN
    -- Serialize all writers of one resolution key, including delete/reinsert.
    IF TG_OP <> 'INSERT' THEN
        PERFORM pg_advisory_xact_lock(hashtextextended(
            OLD.tenant_id::text || ':' || OLD.namespace || ':' || OLD.identifier_type || ':' || OLD.identifier_value, 0));
        SELECT valid_from INTO previous_from
          FROM platform.entity_identifier_binding_history
         WHERE tenant_id = OLD.tenant_id AND namespace = OLD.namespace
           AND identifier_type = OLD.identifier_type AND identifier_value = OLD.identifier_value
           AND valid_to IS NULL FOR UPDATE;
        IF previous_from IS NOT NULL THEN
            boundary := greatest(clock_timestamp(), previous_from + interval '1 microsecond');
            UPDATE platform.entity_identifier_binding_history SET valid_to = boundary
             WHERE tenant_id = OLD.tenant_id AND namespace = OLD.namespace
               AND identifier_type = OLD.identifier_type AND identifier_value = OLD.identifier_value
               AND valid_to IS NULL;
        END IF;
    END IF;
    IF TG_OP <> 'DELETE' THEN
        PERFORM pg_advisory_xact_lock(hashtextextended(
            NEW.tenant_id::text || ':' || NEW.namespace || ':' || NEW.identifier_type || ':' || NEW.identifier_value, 0));
        boundary := greatest(clock_timestamp(), coalesce(boundary, '-infinity'::timestamptz));
        IF EXISTS (SELECT 1 FROM platform.entity_identifier_binding_history
                   WHERE tenant_id = NEW.tenant_id AND namespace = NEW.namespace
                     AND identifier_type = NEW.identifier_type AND identifier_value = NEW.identifier_value
                     AND valid_from < boundary AND (valid_to IS NULL OR valid_to > boundary)) THEN
            RAISE EXCEPTION 'overlapping authoritative identifier history';
        END IF;
        INSERT INTO platform.entity_identifier_binding_history
            (tenant_id, namespace, identifier_type, identifier_value, entity_identifier_id,
             entity_id, confidence, valid_from)
        VALUES (NEW.tenant_id, NEW.namespace, NEW.identifier_type, NEW.identifier_value,
                NEW.identifier_id, NEW.entity_id, NEW.confidence, boundary);
    END IF;
    RETURN NULL;
END;
$$;

CREATE TRIGGER trg_entity_identifier_binding_history
AFTER INSERT OR UPDATE OF tenant_id, namespace, identifier_type, identifier_value,
    identifier_id, entity_id, confidence OR DELETE ON platform.entity_identifier
FOR EACH ROW EXECUTE FUNCTION platform.record_entity_identifier_binding_history();

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
