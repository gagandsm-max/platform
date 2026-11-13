-- Authoritative Agent↔entity binding intervals for historical Agent-ingress recovery.
-- No backfill: pre-existing entity_agent_binding rows remain current-only until a later write.
-- HOST_ID history alone never authorizes an Agent to select another entity.

CREATE TABLE platform.entity_agent_binding_history (
    history_version BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id UUID NOT NULL,
    agent_id VARCHAR(128) NOT NULL,
    entity_id UUID NOT NULL,
    certificate_identity VARCHAR(256) NOT NULL,
    binding_state VARCHAR(20) NOT NULL,
    reinstall_generation INT NOT NULL,
    change_started_at TIMESTAMPTZ NOT NULL,
    valid_from TIMESTAMPTZ,
    valid_to TIMESTAMPTZ,
    activation_state VARCHAR(32) NOT NULL,
    activated_at TIMESTAMPTZ,
    provenance VARCHAR(64) NOT NULL DEFAULT 'ENTITY_AGENT_BINDING_WRITE_V022',
    CONSTRAINT chk_agent_binding_history_interval CHECK (valid_to IS NULL OR valid_to >= valid_from),
    CONSTRAINT chk_agent_binding_history_activation CHECK (
        (activation_state = 'PENDING_COMMIT_VISIBILITY' AND valid_from IS NULL
            AND valid_to IS NULL AND activated_at IS NULL)
        OR (activation_state = 'ACTIVE' AND valid_from IS NOT NULL
            AND activated_at = valid_from)
        OR (activation_state = 'SUPERSEDED' AND valid_from IS NULL
            AND valid_to IS NULL AND activated_at IS NULL)),
    CONSTRAINT chk_agent_binding_history_state CHECK
        (binding_state IN ('PENDING_APPROVAL','ACTIVE','UNMANAGED','REVOKED','SECURITY_HOLD')),
    CONSTRAINT chk_agent_binding_history_generation CHECK (reinstall_generation >= 1),
    CONSTRAINT chk_agent_binding_history_provenance CHECK
        (provenance = 'ENTITY_AGENT_BINDING_WRITE_V022'),
    CONSTRAINT fk_agent_binding_history_entity_same_tenant FOREIGN KEY (tenant_id, entity_id)
        REFERENCES platform.entity (tenant_id, entity_id),
    CONSTRAINT fk_agent_binding_history_agent_same_tenant FOREIGN KEY (tenant_id, agent_id)
        REFERENCES platform.agent (tenant_id, agent_id)
);

CREATE UNIQUE INDEX uq_agent_binding_history_open_key
    ON platform.entity_agent_binding_history (tenant_id, agent_id)
    WHERE activation_state = 'ACTIVE' AND valid_to IS NULL;
CREATE UNIQUE INDEX uq_agent_binding_history_pending_key
    ON platform.entity_agent_binding_history (tenant_id, agent_id)
    WHERE activation_state = 'PENDING_COMMIT_VISIBILITY';
CREATE INDEX idx_agent_binding_history_interval
    ON platform.entity_agent_binding_history
    (tenant_id, agent_id, valid_from, valid_to);

CREATE FUNCTION platform.guard_entity_agent_binding_history() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'authoritative agent binding history cannot be deleted';
    END IF;
    IF TG_OP = 'UPDATE' THEN
        IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
           OR NEW.agent_id IS DISTINCT FROM OLD.agent_id
           OR NEW.entity_id IS DISTINCT FROM OLD.entity_id
           OR NEW.certificate_identity IS DISTINCT FROM OLD.certificate_identity
           OR NEW.binding_state IS DISTINCT FROM OLD.binding_state
           OR NEW.reinstall_generation IS DISTINCT FROM OLD.reinstall_generation
           OR NEW.change_started_at IS DISTINCT FROM OLD.change_started_at
           OR NEW.provenance IS DISTINCT FROM OLD.provenance
           OR NEW.history_version IS DISTINCT FROM OLD.history_version THEN
            RAISE EXCEPTION 'authoritative agent binding history is append and close only';
        END IF;
        IF OLD.activation_state = 'ACTIVE' AND OLD.valid_to IS NULL
           AND NEW.activation_state = 'ACTIVE' AND NEW.valid_from = OLD.valid_from
           AND NEW.activated_at = OLD.activated_at AND NEW.valid_to IS NOT NULL
           AND pg_trigger_depth() >= 2 THEN
            RETURN NEW;
        END IF;
        IF OLD.activation_state = 'PENDING_COMMIT_VISIBILITY'
           AND NEW.activation_state = 'SUPERSEDED' AND pg_trigger_depth() >= 2 THEN
            RETURN NEW;
        END IF;
        IF OLD.activation_state = 'PENDING_COMMIT_VISIBILITY'
           AND NEW.activation_state = 'ACTIVE' AND NEW.valid_to IS NULL
           AND NEW.valid_from = NEW.activated_at
           AND current_setting('platform.agent_binding_history_activation', true) = OLD.history_version::text THEN
            RETURN NEW;
        END IF;
        RAISE EXCEPTION 'authoritative agent binding history is append and close only';
    END IF;
    IF pg_trigger_depth() < 2 OR NEW.activation_state <> 'PENDING_COMMIT_VISIBILITY' THEN
        RAISE EXCEPTION 'agent binding history requires entity_agent_binding write provenance';
    END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(
        NEW.tenant_id::text || ':agent-binding:' || NEW.agent_id, 0));
    IF EXISTS (SELECT 1 FROM platform.entity_agent_binding_history h
                WHERE h.tenant_id = NEW.tenant_id AND h.agent_id = NEW.agent_id
                  AND h.activation_state = 'PENDING_COMMIT_VISIBILITY') THEN
        RAISE EXCEPTION 'duplicate pending agent binding history';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_guard_entity_agent_binding_history
BEFORE INSERT OR UPDATE OR DELETE ON platform.entity_agent_binding_history
FOR EACH ROW EXECUTE FUNCTION platform.guard_entity_agent_binding_history();

CREATE FUNCTION platform.record_entity_agent_binding_history() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    boundary TIMESTAMPTZ;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        PERFORM pg_advisory_xact_lock(hashtextextended(
            OLD.tenant_id::text || ':agent-binding:' || OLD.agent_id, 0));
        boundary := clock_timestamp();
        UPDATE platform.entity_agent_binding_history
           SET valid_to = greatest(boundary, valid_from)
         WHERE tenant_id = OLD.tenant_id AND agent_id = OLD.agent_id
           AND activation_state = 'ACTIVE' AND valid_to IS NULL;
        UPDATE platform.entity_agent_binding_history
           SET activation_state = 'SUPERSEDED'
         WHERE tenant_id = OLD.tenant_id AND agent_id = OLD.agent_id
           AND activation_state = 'PENDING_COMMIT_VISIBILITY';
    END IF;
    IF TG_OP <> 'DELETE' THEN
        PERFORM pg_advisory_xact_lock(hashtextextended(
            NEW.tenant_id::text || ':agent-binding:' || NEW.agent_id, 0));
        boundary := greatest(clock_timestamp(), coalesce(boundary, '-infinity'::timestamptz));
        INSERT INTO platform.entity_agent_binding_history
            (tenant_id, agent_id, entity_id, certificate_identity, binding_state,
             reinstall_generation, change_started_at, activation_state)
        VALUES (NEW.tenant_id, NEW.agent_id, NEW.entity_id, NEW.certificate_identity,
                NEW.binding_state, NEW.reinstall_generation, boundary,
                'PENDING_COMMIT_VISIBILITY');
    END IF;
    RETURN NULL;
END;
$$;

CREATE TRIGGER trg_entity_agent_binding_history
AFTER INSERT OR UPDATE OF tenant_id, entity_id, agent_id, certificate_identity,
    binding_state, reinstall_generation OR DELETE ON platform.entity_agent_binding
FOR EACH ROW EXECUTE FUNCTION platform.record_entity_agent_binding_history();

CREATE FUNCTION platform.activate_entity_agent_binding_history(
    p_history_version BIGINT, p_tenant_id UUID, p_agent_id VARCHAR,
    p_entity_id UUID, p_certificate_identity VARCHAR, p_binding_state VARCHAR,
    p_reinstall_generation BIGINT) RETURNS BOOLEAN
LANGUAGE plpgsql AS $$
DECLARE
    pending_count BIGINT;
    current_count BIGINT;
    activation_time TIMESTAMPTZ;
BEGIN
    SELECT count(*) INTO current_count FROM (
        SELECT 1 FROM platform.entity_agent_binding
         WHERE tenant_id = p_tenant_id AND agent_id = p_agent_id
           AND entity_id = p_entity_id
           AND certificate_identity = p_certificate_identity
           AND binding_state = p_binding_state
           AND reinstall_generation = p_reinstall_generation
         FOR UPDATE) current_binding;
    IF current_count <> 1 THEN RETURN FALSE; END IF;
    PERFORM pg_advisory_xact_lock(hashtextextended(
        p_tenant_id::text || ':agent-binding:' || p_agent_id, 0));
    SELECT count(*) INTO pending_count FROM (
        SELECT 1 FROM platform.entity_agent_binding_history
         WHERE history_version = p_history_version AND tenant_id = p_tenant_id
           AND agent_id = p_agent_id AND entity_id = p_entity_id
           AND certificate_identity = p_certificate_identity
           AND binding_state = p_binding_state
           AND reinstall_generation = p_reinstall_generation
           AND activation_state = 'PENDING_COMMIT_VISIBILITY' FOR UPDATE
    ) pending_binding;
    IF pending_count <> 1 OR EXISTS (
        SELECT 1 FROM platform.entity_agent_binding_history
         WHERE tenant_id = p_tenant_id AND agent_id = p_agent_id
           AND history_version > p_history_version) THEN
        RETURN FALSE;
    END IF;
    IF EXISTS (
        SELECT 1 FROM platform.entity_agent_binding_history
         WHERE history_version = p_history_version
           AND xmin::text = (txid_current() % 4294967296)::text) THEN
        RETURN FALSE;
    END IF;
    activation_time := greatest(clock_timestamp(),
        (SELECT change_started_at + interval '1 microsecond'
           FROM platform.entity_agent_binding_history
          WHERE history_version = p_history_version));
    PERFORM set_config('platform.agent_binding_history_activation', p_history_version::text, true);
    UPDATE platform.entity_agent_binding_history
       SET activation_state = 'ACTIVE', valid_from = activation_time,
           activated_at = activation_time
     WHERE history_version = p_history_version;
    RETURN TRUE;
END;
$$;
