#!/usr/bin/env bash
# V002 — migrate metric_numeric to replay-safe raw storage + canonical FINAL view.
set -euo pipefail

CH_HOST="${CLICKHOUSE_HOST:-clickhouse}"
CH_PORT="${CLICKHOUSE_NATIVE_PORT:-9000}"
CH_USER="${CLICKHOUSE_USER:-platform_app}"
CH_PASSWORD="${CLICKHOUSE_PASSWORD:-platform_dev_ch_password_change_in_prod}"
CH_DB="${CLICKHOUSE_DB:-platform}"

RAW="metric_numeric_raw"
VIEW="metric_numeric"
STAGING="metric_numeric_raw_v002_new"
LEGACY="metric_numeric_v002_legacy"
EXPECTED_SORT="tenant_id, entity_id, metric_id, ts, sensor_id, sequence_id"

log() { printf '[ch-migrate-v002] %s\n' "$*"; }
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
  local name="$1"
  ch "CREATE TABLE ${name} (
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
  ORDER BY (tenant_id, entity_id, metric_id, ts, sensor_id, sequence_id)"
}

create_view() {
  ch "CREATE VIEW ${VIEW} AS
      SELECT tenant_id, entity_id, metric_id, sensor_id, ts, value, quality, flags, sequence_id, source_protocol
      FROM ${RAW} FINAL"
}

validate_raw() {
  exists "$RAW" || die "missing ${CH_DB}.${RAW}"
  [[ "$(engine "$RAW")" == "ReplacingMergeTree" ]] || die "${RAW} is not ReplacingMergeTree"
  local sorting
  sorting="$(ch "SELECT sorting_key FROM system.tables WHERE database=currentDatabase() AND name='${RAW}' FORMAT TSV")"
  [[ "$sorting" == "$EXPECTED_SORT" ]] || die "unexpected ${RAW} sorting key: $sorting"
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
  [[ "$ddl" == *"FROM platform.metric_numeric_raw FINAL"* || "$ddl" == *"FROM metric_numeric_raw FINAL"* ]] \
    || die "${VIEW} does not enforce FINAL replay collapse"
}

finish_partial_state() {
  validate_raw
  if exists "$VIEW"; then
    [[ "$(engine "$VIEW")" == "View" ]] || die "${VIEW} exists but is not canonical View"
  else
    create_view
  fi
  validate_view
  if exists "$LEGACY"; then
    local legacy_count raw_count
    legacy_count="$(ch "SELECT count() FROM ${LEGACY}")"
    raw_count="$(ch "SELECT count() FROM ${RAW}")"
    [[ "$legacy_count" == "$raw_count" ]] || die "legacy/raw count mismatch; preserving backup"
    ch "DROP TABLE ${LEGACY}"
  fi
}

migrate_legacy() {
  [[ "$(engine "$VIEW")" == "MergeTree" ]] || die "unsupported pre-V002 engine for ${VIEW}: $(engine "$VIEW")"
  exists "$STAGING" && ch "DROP TABLE ${STAGING}"
  create_raw "$STAGING"

  local before staged after
  before="$(ch "SELECT count() FROM ${VIEW}")"
  ch "INSERT INTO ${STAGING} SELECT * FROM ${VIEW}"
  staged="$(ch "SELECT count() FROM ${STAGING}")"
  after="$(ch "SELECT count() FROM ${VIEW}")"
  [[ "$before" == "$staged" && "$before" == "$after" ]] || {
    ch "DROP TABLE IF EXISTS ${STAGING}"
    die "source changed during migration or copy mismatch (before=$before staged=$staged after=$after)"
  }

  ch "RENAME TABLE ${VIEW} TO ${LEGACY}, ${STAGING} TO ${RAW}"
  create_view
  validate_raw
  validate_view

  local raw_count
  raw_count="$(ch "SELECT count() FROM ${RAW}")"
  [[ "$raw_count" == "$before" ]] || die "post-migration raw row count mismatch; legacy backup retained"
  ch "DROP TABLE ${LEGACY}"
}

main() {
  wait_for_clickhouse
  ch_raw "CREATE DATABASE IF NOT EXISTS ${CH_DB}"

  if exists "$RAW"; then
    finish_partial_state
    log "PASS: replay-safe raw store + canonical view already present/recovered"
    exit 0
  fi

  if exists "$VIEW"; then
    migrate_legacy
    log "PASS: legacy metric_numeric migrated without row loss"
    exit 0
  fi

  create_raw "$RAW"
  create_view
  validate_raw
  validate_view
  log "PASS: fresh replay-safe schema created"
}

main "$@"
