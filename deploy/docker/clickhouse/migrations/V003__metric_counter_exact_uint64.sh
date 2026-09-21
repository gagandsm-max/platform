#!/usr/bin/env bash
# V003 — exact Counter64 raw store (UInt64) + canonical FINAL view.
# Additive / idempotent. Does not modify metric_numeric_raw or its view.
set -euo pipefail

CH_HOST="${CLICKHOUSE_HOST:-clickhouse}"
CH_PORT="${CLICKHOUSE_NATIVE_PORT:-9000}"
CH_USER="${CLICKHOUSE_USER:-platform_app}"
CH_PASSWORD="${CLICKHOUSE_PASSWORD:-platform_dev_ch_password_change_in_prod}"
CH_DB="${CLICKHOUSE_DB:-platform}"

RAW="metric_counter_raw"
VIEW="metric_counter"
EXPECTED_SORT="tenant_id, entity_id, metric_id, ts, sensor_id, sequence_id"

log() { printf '[ch-migrate-v003] %s\n' "$*"; }
die() { log "ERROR: $*"; exit 1; }

ch_raw() {
  clickhouse-client --host "$CH_HOST" --port "$CH_PORT" --user "$CH_USER" --password "$CH_PASSWORD" --query "$1"
}

ch() {
  clickhouse-client --host "$CH_HOST" --port "$CH_PORT" --user "$CH_USER" --password "$CH_PASSWORD" --database "$CH_DB" --query "$1"
}

wait_for_clickhouse() {
  for _ in $(seq 1 90); do
    if ch_raw "SELECT 1" >/dev/null 2>&1; then return 0; fi
    sleep 1
  done
  die "ClickHouse not reachable at ${CH_HOST}:${CH_PORT}"
}

exists() {
  [[ "$(ch "SELECT count() FROM system.tables WHERE database=currentDatabase() AND name='$1'")" == "1" ]]
}

engine() {
  ch "SELECT engine FROM system.tables WHERE database=currentDatabase() AND name='$1' FORMAT TSV"
}

create_raw() {
  ch "CREATE TABLE IF NOT EXISTS ${RAW} (
    tenant_id String,
    entity_id String,
    metric_id String,
    sensor_id String,
    ts DateTime64(3, 'UTC'),
    value UInt64,
    quality UInt8,
    flags UInt32,
    sequence_id String,
    source_protocol LowCardinality(String)
  ) ENGINE = ReplacingMergeTree()
  ORDER BY (tenant_id, entity_id, metric_id, ts, sensor_id, sequence_id)"
}

create_view() {
  ch "CREATE VIEW IF NOT EXISTS ${VIEW} AS
      SELECT tenant_id, entity_id, metric_id, sensor_id, ts, value, quality, flags, sequence_id, source_protocol
      FROM ${RAW} FINAL"
}

validate_raw() {
  exists "$RAW" || die "missing ${CH_DB}.${RAW}"
  [[ "$(engine "$RAW")" == "ReplacingMergeTree" ]] || die "${RAW} is not ReplacingMergeTree"
  local sorting
  sorting="$(ch "SELECT sorting_key FROM system.tables WHERE database=currentDatabase() AND name='${RAW}' FORMAT TSV")"
  [[ "$sorting" == "$EXPECTED_SORT" ]] || die "unexpected ${RAW} sorting key: $sorting"
  local value_type
  value_type="$(ch "SELECT type FROM system.columns WHERE database=currentDatabase() AND table='${RAW}' AND name='value' FORMAT TSV")"
  [[ "$value_type" == "UInt64" ]] || die "${RAW}.value must be UInt64, got: $value_type"
  for col in tenant_id entity_id metric_id sensor_id ts value quality flags sequence_id source_protocol; do
    [[ "$(ch "SELECT count() FROM system.columns WHERE database=currentDatabase() AND table='${RAW}' AND name='${col}'")" == "1" ]] \
      || die "${RAW} missing required column ${col}"
  done
}

validate_view() {
  exists "$VIEW" || die "missing canonical ${CH_DB}.${VIEW} view"
  [[ "$(engine "$VIEW")" == "View" ]] || die "${VIEW} is not a View"
  local ddl
  ddl="$(ch "SELECT create_table_query FROM system.tables WHERE database=currentDatabase() AND name='${VIEW}' FORMAT TSV")"
  [[ "$ddl" == *"FROM platform.metric_counter_raw FINAL"* || "$ddl" == *"FROM metric_counter_raw FINAL"* ]] \
    || die "${VIEW} does not enforce FINAL replay collapse"
}

wait_for_clickhouse
ch_raw "CREATE DATABASE IF NOT EXISTS ${CH_DB}"

if exists "$RAW"; then
  validate_raw
else
  create_raw
  validate_raw
fi

if exists "$VIEW"; then
  validate_view
else
  create_view
  validate_view
fi

log "PASS: exact Counter64 raw store + canonical view present"
