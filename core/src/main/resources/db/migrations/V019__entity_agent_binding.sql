-- Platform_17 MIG-PLAN-002 / Platform_20 §6.3: current canonical Agent/entity authority.
-- No hostId-derived backfill: existing Agents require an explicitly approved binding.
-- certificate_identity remains the deployment-unique certificate fingerprint (VARCHAR(256)
-- physical type per Platform_17; values remain 64-hex fingerprints from agent credentials).
ALTER TABLE platform.agent_certificate_credential
    ADD CONSTRAINT uq_agent_cert_tenant_agent_identity
    UNIQUE (tenant_id, agent_id, fingerprint_sha256);

CREATE TABLE platform.entity_agent_binding (
    tenant_id UUID NOT NULL,
    entity_id UUID NOT NULL,
    agent_id VARCHAR(128) NOT NULL,
    certificate_identity VARCHAR(256) NOT NULL,
    system_name VARCHAR(255) NOT NULL,
    os_identity JSONB NULL,
    first_enrolled_at TIMESTAMPTZ NOT NULL,
    last_heartbeat_at TIMESTAMPTZ NOT NULL,
    binding_state VARCHAR(20) NOT NULL,
    reinstall_generation INT NOT NULL,
    PRIMARY KEY (tenant_id, entity_id),
    CONSTRAINT uq_entity_agent_binding_agent UNIQUE (agent_id),
    CONSTRAINT uq_entity_agent_binding_certificate UNIQUE (certificate_identity),
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
        (reinstall_generation >= 1),
    CONSTRAINT chk_entity_agent_binding_system_name CHECK (btrim(system_name) <> ''),
    CONSTRAINT chk_entity_agent_binding_heartbeat CHECK (last_heartbeat_at >= first_enrolled_at),
    CONSTRAINT chk_entity_agent_binding_os_identity_bound CHECK
        (os_identity IS NULL OR octet_length(os_identity::text) <= 512)
);

CREATE INDEX idx_entity_agent_binding_state
    ON platform.entity_agent_binding (tenant_id, binding_state);
