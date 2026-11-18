-- BUILD-070 / MIG-PLAN-003 persistence foundation only.
-- Empty tables: no profile templates, policy mapping, assignment, or activation is seeded.

CREATE TABLE platform.monitoring_profile (
    tenant_id UUID NOT NULL REFERENCES platform.tenant (tenant_id),
    profile_id UUID NOT NULL,
    name VARCHAR(160) NOT NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    audit_ref UUID,
    PRIMARY KEY (tenant_id, profile_id),
    UNIQUE (tenant_id, name),
    CONSTRAINT chk_monitoring_profile_name CHECK (length(btrim(name)) > 0)
);

CREATE TABLE platform.monitoring_profile_version (
    tenant_id UUID NOT NULL,
    profile_id UUID NOT NULL,
    version_no INTEGER NOT NULL,
    definition JSONB NOT NULL,
    content_hash BYTEA NOT NULL,
    capacity_estimate JSONB,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    audit_ref UUID,
    PRIMARY KEY (tenant_id, profile_id, version_no),
    CONSTRAINT fk_monitoring_profile_version_profile_same_tenant
        FOREIGN KEY (tenant_id, profile_id)
        REFERENCES platform.monitoring_profile (tenant_id, profile_id),
    CONSTRAINT chk_monitoring_profile_version_positive CHECK (version_no > 0),
    CONSTRAINT chk_monitoring_profile_definition_object CHECK (jsonb_typeof(definition) = 'object'),
    CONSTRAINT chk_monitoring_profile_definition_size CHECK (octet_length(definition::text) <= 65536),
    CONSTRAINT chk_monitoring_profile_capacity_object CHECK
        (capacity_estimate IS NULL OR jsonb_typeof(capacity_estimate) = 'object'),
    CONSTRAINT chk_monitoring_profile_capacity_size CHECK
        (capacity_estimate IS NULL OR octet_length(capacity_estimate::text) <= 16384),
    CONSTRAINT chk_monitoring_profile_content_hash_size CHECK (octet_length(content_hash) = 32)
);

CREATE INDEX idx_monitoring_profile_version_lookup
    ON platform.monitoring_profile_version (tenant_id, profile_id, version_no DESC);

CREATE FUNCTION platform.guard_monitoring_profile_version() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    latest_version INTEGER;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        RAISE EXCEPTION 'monitoring profile versions are immutable';
    END IF;

    PERFORM pg_advisory_xact_lock(hashtextextended(
        NEW.tenant_id::text || ':monitoring-profile:' || NEW.profile_id::text, 0));
    SELECT max(version_no) INTO latest_version
      FROM platform.monitoring_profile_version
     WHERE tenant_id = NEW.tenant_id AND profile_id = NEW.profile_id;
    IF latest_version IS NULL AND NEW.version_no <> 1 THEN
        RAISE EXCEPTION 'first monitoring profile version must be 1';
    END IF;
    IF latest_version IS NOT NULL AND NEW.version_no <= latest_version THEN
        RAISE EXCEPTION 'monitoring profile version must increase monotonically';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_guard_monitoring_profile_version
BEFORE INSERT OR UPDATE OR DELETE ON platform.monitoring_profile_version
FOR EACH ROW EXECUTE FUNCTION platform.guard_monitoring_profile_version();

CREATE TRIGGER trg_guard_monitoring_profile_version_truncate
BEFORE TRUNCATE ON platform.monitoring_profile_version
FOR EACH STATEMENT EXECUTE FUNCTION platform.guard_monitoring_profile_version();

CREATE TABLE platform.monitoring_profile_assignment (
    tenant_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    profile_id UUID NOT NULL,
    profile_version_no INTEGER NOT NULL,
    assignment_mode VARCHAR(24) NOT NULL,
    assigned_by UUID NOT NULL,
    effective_from TIMESTAMPTZ NOT NULL,
    row_version BIGINT NOT NULL DEFAULT 1,
    audit_ref UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (tenant_id, entity_id),
    CONSTRAINT fk_monitoring_profile_assignment_entity_same_tenant
        FOREIGN KEY (tenant_id, entity_id)
        REFERENCES platform.entity (tenant_id, entity_id),
    CONSTRAINT fk_monitoring_profile_assignment_version_same_tenant
        FOREIGN KEY (tenant_id, profile_id, profile_version_no)
        REFERENCES platform.monitoring_profile_version (tenant_id, profile_id, version_no),
    CONSTRAINT chk_monitoring_profile_assignment_mode CHECK
        (assignment_mode IN ('AUTO_FROM_CRITICALITY', 'EXPLICIT_PROFILE')),
    CONSTRAINT chk_monitoring_profile_assignment_version CHECK (row_version > 0)
);

CREATE INDEX idx_monitoring_profile_assignment_profile
    ON platform.monitoring_profile_assignment (tenant_id, profile_id);
