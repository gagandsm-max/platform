-- Platform Database Bootstrap - ClickHouse
-- High-Volume Analytical / Telemetry Store
-- Matches Platform_05_LLD.md and Platform_17_API_Event_and_Data_Model_Contracts.md

CREATE DATABASE IF NOT EXISTS platform;

-- Health verification table
CREATE TABLE IF NOT EXISTS platform.health_verification (
    check_id UUID DEFAULT generateUUIDv4(),
    node_id LowCardinality(String),
    checked_at DateTime64(3, 'UTC') DEFAULT now64(3),
    status LowCardinality(String)
) ENGINE = MergeTree()
ORDER BY (status, checked_at);

-- Initial foundational telemetry table definition for dev readiness
CREATE TABLE IF NOT EXISTS platform.metric_numeric (
    tenant_id String,
    entity_id String,
    metric_id String,
    sensor_id String,
    ts DateTime64(3, 'UTC'),
    value Float64,
    quality UInt8,
    flags UInt32,
    sequence_id String,
    source_protocol LowCardinality(String)
) ENGINE = ReplacingMergeTree()
ORDER BY (tenant_id, entity_id, metric_id, ts, sensor_id, sequence_id)
SETTINGS non_replicated_deduplication_window = 10000;

-- Initial health verification record
INSERT INTO platform.health_verification (node_id, status) 
VALUES ('bootstrap-node', 'HEALTHY');
