-- V016: bounded Platform identity + permission-grant directory persistence (Issue #249).
-- Additive only. No seed principals, no admin grants, no bootstrap bypass.
-- Does not implement roles/groups/conditions/management/provisioning from the larger IAM model.
-- G0 retains V015 separately; this branch may apply after V014 with a catalog gap until G0 merges.
-- Flag-off rollback is configuration/restart (Empty directories), not schema down-migration.

CREATE TABLE IF NOT EXISTS platform.identity_provider_instance (
    provider_instance_id VARCHAR(128) PRIMARY KEY,
    provider_type VARCHAR(32) NOT NULL,
    issuer_uri TEXT,
    audience TEXT,
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_identity_provider_type
        CHECK (provider_type IN ('LOCAL', 'OIDC', 'SAML', 'LDAP')),
    CONSTRAINT chk_identity_provider_status
        CHECK (status IN ('ACTIVE', 'DISABLED', 'RETIRED')),
    CONSTRAINT chk_identity_provider_instance_id_format
        CHECK (provider_instance_id ~ '^[a-z0-9]+([._-][a-z0-9]+)*$'),
    CONSTRAINT chk_identity_provider_oidc_trust
        CHECK (
            provider_type <> 'OIDC'
            OR (
                issuer_uri IS NOT NULL AND btrim(issuer_uri) <> '' AND issuer_uri = btrim(issuer_uri)
                AND audience IS NOT NULL AND btrim(audience) <> '' AND audience = btrim(audience)
            )
        )
);

COMMENT ON TABLE platform.identity_provider_instance IS
    'Configured authentication-provider trust. provider_instance_id is immutable configuration identity; OIDC issuer/audience are immutable once inserted. RETIRED cannot be reactivated. No hard delete.';

CREATE TABLE IF NOT EXISTS platform.platform_subject (
    subject_id UUID PRIMARY KEY,
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_platform_subject_status
        CHECK (status IN ('ACTIVE', 'DISABLED', 'RETIRED'))
);

COMMENT ON TABLE platform.platform_subject IS
    'Stable Platform-owned subject. Lifecycle is independent of provider mapping disabled flag.';

CREATE TABLE IF NOT EXISTS platform.identity_mapping (
    provider_type VARCHAR(32) NOT NULL,
    provider_instance_id VARCHAR(128) NOT NULL
        REFERENCES platform.identity_provider_instance (provider_instance_id),
    external_subject TEXT NOT NULL,
    subject_id UUID NOT NULL
        REFERENCES platform.platform_subject (subject_id),
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (provider_type, provider_instance_id, external_subject),
    CONSTRAINT chk_identity_mapping_provider_type
        CHECK (provider_type IN ('LOCAL', 'OIDC', 'SAML', 'LDAP')),
    CONSTRAINT chk_identity_mapping_external_subject
        CHECK (
            external_subject <> ''
            AND external_subject = btrim(external_subject)
            AND char_length(external_subject) <= 512
        )
);

COMMENT ON TABLE platform.identity_mapping IS
    'Exact (provider_type, provider_instance_id, external_subject) equality map. No trim/case fold/email identity. Mapping.disabled is distinct from subject.status and provider.status.';

CREATE INDEX IF NOT EXISTS idx_identity_mapping_subject
    ON platform.identity_mapping (subject_id);

CREATE INDEX IF NOT EXISTS idx_identity_mapping_provider_instance
    ON platform.identity_mapping (provider_instance_id);

CREATE TABLE IF NOT EXISTS platform.subject_tenant_scope (
    subject_id UUID NOT NULL
        REFERENCES platform.platform_subject (subject_id),
    tenant_id UUID NOT NULL
        REFERENCES platform.tenant (tenant_id),
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (subject_id, tenant_id),
    CONSTRAINT chk_subject_tenant_scope_status
        CHECK (status IN ('ACTIVE', 'REVOKED'))
);

COMMENT ON TABLE platform.subject_tenant_scope IS
    'Explicit authoritative tenant membership. Grants require an ACTIVE membership row for the same (subject, tenant). Does not redefine tenant ACTIVE/SUSPENDED/RETIRED policy.';

CREATE INDEX IF NOT EXISTS idx_subject_tenant_scope_subject_active
    ON platform.subject_tenant_scope (subject_id)
    WHERE status = 'ACTIVE';

CREATE TABLE IF NOT EXISTS platform.permission_grant (
    grant_id UUID PRIMARY KEY,
    subject_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    permission VARCHAR(96) NOT NULL,
    resource_type VARCHAR(64) NOT NULL,
    scope_kind VARCHAR(32) NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_permission_grant_membership
        FOREIGN KEY (subject_id, tenant_id)
        REFERENCES platform.subject_tenant_scope (subject_id, tenant_id),
    CONSTRAINT chk_permission_grant_scope_kind
        CHECK (scope_kind IN ('TENANT_WIDE', 'NAMED_RESOURCES')),
    CONSTRAINT chk_permission_grant_resource_type
        CHECK (resource_type IN (
            'TENANT', 'SITE', 'GROUP', 'TAG', 'ENTITY', 'ENTITY_IDENTIFIER', 'CREDENTIAL'
        )),
    CONSTRAINT chk_permission_grant_permission_format
        CHECK (permission ~ '^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+$')
);

COMMENT ON TABLE platform.permission_grant IS
    'Platform-owned permission grants. JWT role/permission claims never populate this table. revoked_at set means omitted on load. TENANT_WIDE must be explicit; empty NAMED_RESOURCES is invalid.';

CREATE INDEX IF NOT EXISTS idx_permission_grant_subject_active
    ON platform.permission_grant (subject_id)
    WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_permission_grant_subject_tenant_active
    ON platform.permission_grant (subject_id, tenant_id)
    WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS platform.permission_grant_resource (
    grant_id UUID NOT NULL
        REFERENCES platform.permission_grant (grant_id) ON DELETE CASCADE,
    resource_id UUID NOT NULL,
    PRIMARY KEY (grant_id, resource_id)
);

COMMENT ON TABLE platform.permission_grant_resource IS
    'Named resource ids for NAMED_RESOURCES grants. Empty child set must fail closed on load and must never become a wildcard.';

CREATE INDEX IF NOT EXISTS idx_permission_grant_resource_grant
    ON platform.permission_grant_resource (grant_id);

-- Immutable OIDC/provider trust binding: prevent retargeting that would revive old mappings under a new issuer/audience.
CREATE OR REPLACE FUNCTION platform.prevent_identity_provider_trust_retarget()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'identity_provider_instance rows must not be deleted; retire the instance instead';
    END IF;
    IF OLD.provider_type IS DISTINCT FROM NEW.provider_type
       OR OLD.issuer_uri IS DISTINCT FROM NEW.issuer_uri
       OR OLD.audience IS DISTINCT FROM NEW.audience
       OR OLD.provider_instance_id IS DISTINCT FROM NEW.provider_instance_id THEN
        RAISE EXCEPTION 'identity_provider_instance trust binding is immutable';
    END IF;
    IF OLD.status = 'RETIRED' AND NEW.status IS DISTINCT FROM 'RETIRED' THEN
        RAISE EXCEPTION 'retired identity_provider_instance cannot be reactivated';
    END IF;
    NEW.updated_at := CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_identity_provider_trust_immutable ON platform.identity_provider_instance;
CREATE TRIGGER trg_identity_provider_trust_immutable
    BEFORE UPDATE OR DELETE ON platform.identity_provider_instance
    FOR EACH ROW
    EXECUTE FUNCTION platform.prevent_identity_provider_trust_retarget();
