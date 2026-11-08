-- Platform_20 section 6.3: current canonical Agent/entity authority.
-- No hostId-derived backfill: existing Agents require an explicitly approved binding.
-- certificate_identity uses the existing deployment-unique certificate fingerprint.
ALTER TABLE platform.agent_certificate_credential
    ADD CONSTRAINT uq_agent_cert_tenant_agent_identity
    UNIQUE (tenant_id, agent_id, fingerprint_sha256);

CREATE TABLE platform.entity_agent_binding (
    tenant_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    agent_id VARCHAR(128) NOT NULL,
    certificate_identity CHAR(64) NOT NULL UNIQUE,
    system_name VARCHAR(255) NOT NULL,
    os_identity VARCHAR(512) NULL,
    first_enrolled_at TIMESTAMPTZ NOT NULL,
    last_heartbeat_at TIMESTAMPTZ NOT NULL,
    binding_state VARCHAR(32) NOT NULL,
    reinstall_generation BIGINT NOT NULL,
    PRIMARY KEY (tenant_id, agent_id),
    CONSTRAINT fk_entity_agent_binding_entity_same_tenant
        FOREIGN KEY (tenant_id, entity_id) REFERENCES platform.entity (tenant_id, entity_id),
    CONSTRAINT fk_entity_agent_binding_agent_same_tenant
        FOREIGN KEY (tenant_id, agent_id) REFERENCES platform.agent (tenant_id, agent_id),
    CONSTRAINT fk_entity_agent_binding_certificate_same_agent
        FOREIGN KEY (tenant_id, agent_id, certificate_identity)
        REFERENCES platform.agent_certificate_credential (tenant_id, agent_id, fingerprint_sha256),
    CONSTRAINT chk_entity_agent_binding_state CHECK
        (binding_state IN ('PENDING_APPROVAL','ACTIVE','UNMANAGED','REVOKED','SECURITY_HOLD')),
    CONSTRAINT chk_entity_agent_binding_generation CHECK
        (reinstall_generation BETWEEN 1 AND 9007199254740991),
    CONSTRAINT chk_entity_agent_binding_system_name CHECK (btrim(system_name) <> ''),
    CONSTRAINT chk_entity_agent_binding_heartbeat CHECK (last_heartbeat_at >= first_enrolled_at)
);

CREATE INDEX idx_entity_agent_binding_entity
    ON platform.entity_agent_binding (tenant_id, entity_id);
