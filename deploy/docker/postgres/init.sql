-- Platform Database Bootstrap - PostgreSQL
-- Minimal init only. Day-02 domain schema is applied by Core MigrationRunner
-- (platform.schema_history + versioned SQL under classpath db/migrations).

CREATE SCHEMA IF NOT EXISTS platform;

-- Legacy bootstrap marker retained for existing health/integration probes that
-- count applied bootstrap records. Application-owned migrations use schema_history.
CREATE TABLE IF NOT EXISTS platform.schema_migration (
    migration_id SERIAL PRIMARY KEY,
    version VARCHAR(64) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    checksum VARCHAR(64) NOT NULL
);

CREATE TABLE IF NOT EXISTS platform.deployment_metadata (
    deployment_id VARCHAR(64) PRIMARY KEY,
    platform_version VARCHAR(32) NOT NULL,
    cluster_mode VARCHAR(32) NOT NULL DEFAULT 'STANDALONE',
    active_epoch BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO platform.deployment_metadata (deployment_id, platform_version, cluster_mode, active_epoch)
VALUES ('platform-dev-deployment-001', '0.1.0-dev', 'STANDALONE', 1)
ON CONFLICT (deployment_id) DO NOTHING;

INSERT INTO platform.schema_migration (version, name, checksum)
VALUES ('001_initial_bootstrap', 'Initial bootstrap migration', 'bootstrap-hash-v0.1.0')
ON CONFLICT (version) DO NOTHING;

CREATE OR REPLACE VIEW platform.v_health_check AS
SELECT
    d.deployment_id,
    d.platform_version,
    d.cluster_mode,
    d.active_epoch,
    (SELECT COUNT(*) FROM platform.schema_migration) AS migrations_count,
    CURRENT_TIMESTAMP AS checked_at
FROM platform.deployment_metadata d;
