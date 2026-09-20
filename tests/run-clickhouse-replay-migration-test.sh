#!/usr/bin/env bash
# Disposable-volume ClickHouse V002 replay-identity migration test.
# Never touches developer platform_ch_data.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CH_IMAGE="clickhouse/clickhouse-server:24.8-alpine"
SUFFIX="$$"
VOLUME="platform_ch_replay_migration_${SUFFIX}"
NETWORK="platform_ch_replay_net_${SUFFIX}"
SERVER="platform-ch-replay-${SUFFIX}"
MIGRATOR="platform-ch-replay-migrate-${SUFFIX}"
USER="platform_app"
PASSWORD="platform_replay_test"

cleanup() {
  docker rm -f "$SERVER" "$MIGRATOR" >/dev/null 2>&1 || true
  docker network rm "$NETWORK" >/dev/null 2>&1 || true
  docker volume rm -f "$VOLUME" >/dev/null 2>&1 || true
}
trap cleanup EXIT

fail() { echo "[clickhouse-replay-migration] FAIL: $*" >&2; exit 1; }
log() { echo "[clickhouse-replay-migration] $*"; }

ch() {
  docker exec "$SERVER" clickhouse-client     --user "$USER" --password "$PASSWORD" --database platform --query "$1"
}
ch_raw() {
  docker exec "$SERVER" clickhouse-client     --user "$USER" --password "$PASSWORD" --query "$1"
}

wait_ch() {
  for _ in $(seq 1 90); do
    if docker exec "$SERVER" clickhouse-client         --user "$USER" --password "$PASSWORD" --query "SELECT 1" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  fail "ClickHouse did not become ready"
}

run_migration() {
  docker rm -f "$MIGRATOR" >/dev/null 2>&1 || true
  docker run --rm     --name "$MIGRATOR"     --network "$NETWORK"     -e CLICKHOUSE_HOST="$SERVER"     -e CLICKHOUSE_NATIVE_PORT=9000     -e CLICKHOUSE_DB=platform     -e CLICKHOUSE_USER="$USER"     -e CLICKHOUSE_PASSWORD="$PASSWORD"     -v "$ROOT_DIR/deploy/docker/clickhouse/migrations:/migrations:ro"     --entrypoint /bin/bash     "$CH_IMAGE"     /migrations/V002__metric_numeric_replay_identity.sh
}

assert_target() {
  [[ "$(ch "SELECT engine FROM system.tables WHERE database='platform' AND name='metric_numeric_raw' FORMAT TSV")" == "ReplacingMergeTree" ]]     || fail "metric_numeric_raw is not ReplacingMergeTree"
  [[ "$(ch "SELECT sorting_key FROM system.tables WHERE database='platform' AND name='metric_numeric_raw' FORMAT TSV")" == "tenant_id, entity_id, metric_id, ts, sensor_id, sequence_id" ]]     || fail "metric_numeric_raw sorting key mismatch"
  [[ "$(ch "SELECT engine FROM system.tables WHERE database='platform' AND name='metric_numeric' FORMAT TSV")" == "View" ]]     || fail "metric_numeric is not canonical View"
  ddl="$(ch "SELECT create_table_query FROM system.tables WHERE database='platform' AND name='metric_numeric' FORMAT TSV")"
  [[ "$ddl" == *"metric_numeric_raw FINAL"* ]] || fail "canonical metric_numeric view does not use FINAL"
}

log "create disposable ClickHouse"
docker network create "$NETWORK" >/dev/null
docker volume create "$VOLUME" >/dev/null
docker run -d   --name "$SERVER"   --network "$NETWORK"   -e CLICKHOUSE_DB=platform   -e CLICKHOUSE_USER="$USER"   -e CLICKHOUSE_PASSWORD="$PASSWORD"   -e CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1   -v "$VOLUME:/var/lib/clickhouse"   "$CH_IMAGE" >/dev/null
wait_ch

log "create exact legacy metric_numeric schema"
ch_raw "CREATE DATABASE IF NOT EXISTS platform"
ch "CREATE TABLE metric_numeric (
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
) ENGINE = MergeTree()
ORDER BY (tenant_id, entity_id, metric_id, ts)"

ch "INSERT INTO metric_numeric VALUES
('tenant-a','entity-a','legacy.one','sensor-a','2026-09-20 01:00:00.000',1,1,0,'obs-1','snmp'),
('tenant-a','entity-a','legacy.two','sensor-a','2026-09-20 01:00:01.000',2,1,0,'obs-2','snmp')"
[[ "$(ch "SELECT count() FROM metric_numeric")" == "2" ]] || fail "legacy seed count mismatch"

log "migrate legacy schema"
run_migration
assert_target
[[ "$(ch "SELECT count() FROM metric_numeric")" == "2" ]] || fail "canonical row count changed during migration"
[[ "$(ch "SELECT count() FROM metric_numeric_raw")" == "2" ]] || fail "raw row count changed during migration"

log "rerun migration to prove idempotence"
run_migration
assert_target
[[ "$(ch "SELECT count() FROM metric_numeric")" == "2" ]] || fail "idempotent rerun changed canonical count"

log "insert same durable observation twice without insert token"
ROW="('tenant-a','entity-r','replay.metric','sensor-a','2026-09-20 02:00:00.000',9,1,0,'tenant-a:sensor-a:42:0:replay.metric','snmp')"
ch "INSERT INTO metric_numeric_raw VALUES ${ROW}"
ch "INSERT INTO metric_numeric_raw VALUES ${ROW}"

RAW_DUPES="$(ch "SELECT count() FROM metric_numeric_raw WHERE metric_id='replay.metric'")"
CANONICAL="$(ch "SELECT count() FROM metric_numeric WHERE metric_id='replay.metric'")"
[[ "$RAW_DUPES" == "2" ]] || fail "test did not create two physical replay rows; got $RAW_DUPES"
[[ "$CANONICAL" == "1" ]] || fail "canonical view exposed duplicate replay rows; got $CANONICAL"

log "restart ClickHouse and verify canonical replay safety survives restart"
docker restart "$SERVER" >/dev/null
wait_ch
assert_target
[[ "$(ch "SELECT count() FROM metric_numeric WHERE metric_id='replay.metric'")" == "1" ]]   || fail "canonical replay collapse failed after restart"

log "PASS"
