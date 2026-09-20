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

-- Physical telemetry storage. Replays may create duplicate physical rows after
-- Kafka/Core crash windows, so durable logical identity (sequence_id) is part of the
-- replacement key. Readers use the canonical FINAL view below, never the raw table.
CREATE TABLE IF NOT EXISTS platform.metric_numeric_raw (
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
ORDER BY (tenant_id, entity_id, metric_id, ts, sensor_id, sequence_id);

-- Canonical read surface: duplicate replays are collapsed immediately at query time,
-- independent of merge timing or finite insert-deduplication windows.
CREATE VIEW IF NOT EXISTS platform.metric_numeric AS
SELECT
    tenant_id,
    entity_id,
    metric_id,
    sensor_id,
    ts,
    value,
    quality,
    flags,
    sequence_id,
    source_protocol
FROM platform.metric_numeric_raw FINAL;

-- Initial health verification record
INSERT INTO platform.health_verification (node_id, status) 
VALUES ('bootstrap-node', 'HEALTHY');
