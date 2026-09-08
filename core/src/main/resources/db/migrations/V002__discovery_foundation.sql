-- Discovery Foundation (Platform v3.2.2)
-- Core-orchestrated, Sensor-executed, tenant-isolated discovery tables and constraints.

CREATE TABLE IF NOT EXISTS platform.discovery_request (
    request_id UUID NOT NULL,
    tenant_id UUID NOT NULL REFERENCES platform.tenant (tenant_id),
    name VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL,
    correlation_id VARCHAR(128) NOT NULL,
    idempotency_key VARCHAR(128) NULL,
    site_id UUID NULL,
    sensor_pool_id UUID NULL,
    assigned_sensor_id VARCHAR(128) NULL,
    requested_methods JSONB NOT NULL,
    credential_profile_ref VARCHAR(128) NULL,
    snmp_port INT NOT NULL DEFAULT 161,
    timeout_ms INT NOT NULL DEFAULT 3000,
    max_retries INT NOT NULL DEFAULT 2,
    created_by VARCHAR(128) NULL,
    created_at TIMESTAMPTZ NOT NULL,
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    summary_json JSONB NULL,
    failure_reason VARCHAR(512) NULL,
    PRIMARY KEY (request_id),
    UNIQUE (tenant_id, request_id),
    CONSTRAINT chk_discovery_status CHECK (
        status IN ('PENDING', 'ASSIGNED', 'RUNNING', 'COMPLETED', 'FAILED', 'CANCELLED')
    ),
    CONSTRAINT fk_discovery_site_same_tenant
        FOREIGN KEY (tenant_id, site_id)
        REFERENCES platform.site (tenant_id, site_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_discovery_tenant_idempotency
    ON platform.discovery_request (tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_discovery_request_tenant_status
    ON platform.discovery_request (tenant_id, status);

CREATE INDEX IF NOT EXISTS idx_discovery_request_sensor_status
    ON platform.discovery_request (assigned_sensor_id, status);

CREATE TABLE IF NOT EXISTS platform.discovery_target (
    target_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    request_id UUID NOT NULL,
    target_address VARCHAR(512) NOT NULL,
    target_type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (target_id),
    UNIQUE (tenant_id, target_id),
    UNIQUE (tenant_id, request_id, target_address),
    CONSTRAINT chk_discovery_target_status CHECK (
        status IN ('PENDING', 'RUNNING', 'PROBED', 'FAILED', 'SKIPPED')
    ),
    CONSTRAINT chk_discovery_target_type CHECK (
        target_type IN ('IPV4', 'IPV6', 'HOSTNAME')
    ),
    CONSTRAINT fk_target_request_same_tenant
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES platform.discovery_request (tenant_id, request_id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_discovery_target_lookup
    ON platform.discovery_target (tenant_id, request_id, status);

CREATE TABLE IF NOT EXISTS platform.discovery_evidence (
    evidence_id UUID NOT NULL,
    tenant_id UUID NOT NULL,
    request_id UUID NOT NULL,
    target_id UUID NOT NULL,
    sensor_id VARCHAR(128) NOT NULL,
    protocol VARCHAR(32) NOT NULL,
    method VARCHAR(32) NOT NULL,
    status VARCHAR(32) NOT NULL,
    reachability_kind VARCHAR(32) NOT NULL,
    technical_code VARCHAR(128) NULL,
    attributes_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    raw_evidence_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    observed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (evidence_id),
    UNIQUE (tenant_id, evidence_id),
    CONSTRAINT chk_evidence_protocol CHECK (
        protocol IN ('ICMP', 'SNMP')
    ),
    CONSTRAINT chk_evidence_status CHECK (
        status IN ('SUCCESS', 'FAILED', 'INCONCLUSIVE')
    ),
    CONSTRAINT chk_evidence_reachability CHECK (
        reachability_kind IN ('OBSERVED', 'DERIVED', 'SUSPECTED', 'UNKNOWN')
    ),
    CONSTRAINT fk_evidence_request_same_tenant
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES platform.discovery_request (tenant_id, request_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_evidence_target_same_tenant
        FOREIGN KEY (tenant_id, target_id)
        REFERENCES platform.discovery_target (tenant_id, target_id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_discovery_evidence_request
    ON platform.discovery_evidence (tenant_id, request_id);

CREATE INDEX IF NOT EXISTS idx_discovery_evidence_target
    ON platform.discovery_evidence (tenant_id, target_id);

CREATE TABLE IF NOT EXISTS platform.discovery_candidate (
    candidate_id UUID NOT NULL,
    tenant_id UUID NOT NULL REFERENCES platform.tenant (tenant_id),
    request_id UUID NOT NULL,
    target_address VARCHAR(512) NOT NULL,
    discovered_name VARCHAR(255) NULL,
    sys_object_id VARCHAR(255) NULL,
    sys_descr VARCHAR(2000) NULL,
    evidenced_protocols JSONB NOT NULL,
    attributes_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    confidence VARCHAR(32) NOT NULL,
    review_status VARCHAR(32) NOT NULL DEFAULT 'PENDING_REVIEW',
    conflict_detected BOOLEAN NOT NULL DEFAULT FALSE,
    conflict_details VARCHAR(1000) NULL,
    site_id UUID NULL,
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (candidate_id),
    UNIQUE (tenant_id, candidate_id),
    UNIQUE (tenant_id, request_id, target_address),
    CONSTRAINT chk_candidate_confidence CHECK (
        confidence IN ('HIGH', 'MEDIUM', 'LOW', 'INCONCLUSIVE')
    ),
    CONSTRAINT chk_candidate_review_status CHECK (
        review_status IN ('PENDING_REVIEW', 'APPROVED_FOR_PROVISIONING', 'LEFT_UNMANAGED', 'IGNORED', 'EXCLUDED_PERMANENTLY')
    ),
    CONSTRAINT fk_candidate_request_same_tenant
        FOREIGN KEY (tenant_id, request_id)
        REFERENCES platform.discovery_request (tenant_id, request_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_candidate_site_same_tenant
        FOREIGN KEY (tenant_id, site_id)
        REFERENCES platform.site (tenant_id, site_id)
);

CREATE INDEX IF NOT EXISTS idx_discovery_candidate_review
    ON platform.discovery_candidate (tenant_id, review_status);

CREATE INDEX IF NOT EXISTS idx_discovery_candidate_request
    ON platform.discovery_candidate (tenant_id, request_id);
