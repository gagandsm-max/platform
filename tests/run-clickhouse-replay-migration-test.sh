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

# ClickHouse 24.8-alpine entrypoint creates the app user and database by
# rewriting users/config and restarting the server. SELECT 1 can succeed in a
# transient window before that init finishes; subsequent DDL then fails with
# ATTEMPT_TO_READ_AFTER_EOF. Require consecutive DDL-capable probes instead.
wait_ch() {
  local stable=0
  local probe_sql
  probe_sql=$(cat <<'SQL'
CREATE DATABASE IF NOT EXISTS platform;
CREATE TABLE IF NOT EXISTS platform.__platform_ready_probe (x UInt8) ENGINE = MergeTree() ORDER BY x;
DROP TABLE IF EXISTS platform.__platform_ready_probe;
SQL
)
  for _ in $(seq 1 90); do
    if docker exec "$SERVER" clickhouse-client \
         --user "$USER" --password "$PASSWORD" --query "SELECT 1" >/dev/null 2>&1 \
       && docker exec "$SERVER" clickhouse-client \
         --user "$USER" --password "$PASSWORD" --multiquery --query "$probe_sql" >/dev/null 2>&1 \
       && docker exec "$SERVER" clickhouse-client \
         --user "$USER" --password "$PASSWORD" --database platform \
         --query "SELECT name FROM system.databases WHERE name='platform'" >/dev/null 2>&1; then
      stable=$((stable + 1))
      if [[ "$stable" -ge 2 ]]; then
        return 0
      fi
    else
      stable=0
    fi
    sleep 1
  done
  fail "ClickHouse did not become DDL-ready (entrypoint user/database init may still be restarting)"
}

run_migration() {
  docker rm -f "$MIGRATOR" >/dev/null 2>&1 || true
  docker run --rm     --name "$MIGRATOR"     --network "$NETWORK"     -e CLICKHOUSE_HOST="$SERVER"     -e CLICKHOUSE_NATIVE_PORT=9000     -e CLICKHOUSE_DB=platform     -e CLICKHOUSE_USER="$USER"     -e CLICKHOUSE_PASSWORD="$PASSWORD"     -v "$ROOT_DIR/deploy/docker/clickhouse/migrations:/migrations:ro"     --entrypoint /bin/bash     "$CH_IMAGE"     /migrations/run-all.sh
}

run_v003_only() {
  docker rm -f "$MIGRATOR" >/dev/null 2>&1 || true
  docker run --rm     --name "$MIGRATOR"     --network "$NETWORK"     -e CLICKHOUSE_HOST="$SERVER"     -e CLICKHOUSE_NATIVE_PORT=9000     -e CLICKHOUSE_DB=platform     -e CLICKHOUSE_USER="$USER"     -e CLICKHOUSE_PASSWORD="$PASSWORD"     -v "$ROOT_DIR/deploy/docker/clickhouse/migrations:/migrations:ro"     --entrypoint /bin/bash     "$CH_IMAGE"     /migrations/V003__metric_counter_exact_uint64.sh
}

assert_counter_target() {
  [[ "$(ch "SELECT engine FROM system.tables WHERE database='platform' AND name='metric_counter_raw' FORMAT TSV")" == "ReplacingMergeTree" ]]     || fail "metric_counter_raw is not ReplacingMergeTree"
  [[ "$(ch "SELECT type FROM system.columns WHERE database='platform' AND table='metric_counter_raw' AND name='value' FORMAT TSV")" == "UInt64" ]]     || fail "metric_counter_raw.value is not UInt64"
  [[ "$(ch "SELECT engine FROM system.tables WHERE database='platform' AND name='metric_counter' FORMAT TSV")" == "View" ]]     || fail "metric_counter is not canonical View"
  ddl="$(ch "SELECT create_table_query FROM system.tables WHERE database='platform' AND name='metric_counter' FORMAT TSV")"
  [[ "$ddl" == *"metric_counter_raw FINAL"* ]] || fail "canonical metric_counter view does not use FINAL"
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

log "simulate interrupted cutover after writer quiesce but before copy completes"
ch "DROP VIEW metric_numeric"
ch "DROP TABLE metric_numeric_raw"
ch "CREATE TABLE metric_numeric_v002_legacy (
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
ch "INSERT INTO metric_numeric_v002_legacy VALUES
('tenant-r','entity-1','recovery.one','sensor-r','2026-09-20 03:00:00.000',1,1,0,'recover-1','snmp'),
('tenant-r','entity-2','recovery.two','sensor-r','2026-09-20 03:00:01.000',2,1,0,'recover-2','snmp')"
ch "CREATE TABLE metric_numeric_raw (
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
ch "INSERT INTO metric_numeric_raw SELECT * FROM metric_numeric_v002_legacy LIMIT 1"

if ch "INSERT INTO metric_numeric VALUES ('tenant-r','entity-old-writer','must.fail','sensor-r','2026-09-20 03:00:02.000',3,1,0,'recover-3','snmp')" >/dev/null 2>&1; then
  fail "old writer unexpectedly inserted through removed canonical table name during cutover"
fi
log "old writer path fails closed while migration is incomplete"

run_migration
assert_target
[[ "$(ch "SELECT count() FROM metric_numeric WHERE metric_id LIKE 'recovery.%'")" == "2" ]] || fail "partial-cutover recovery did not restore both legacy rows"
[[ "$(ch "SELECT count() FROM system.tables WHERE database='platform' AND name='metric_numeric_v002_legacy'")" == "0" ]] || fail "legacy backup remained after verified recovery"

log "verify exact Counter64 schema + UInt64 round-trip + replay collapse"
assert_counter_target
SEQ_C64="tenant-c64:sensor-c64:stream-c64:1:0:ifHCInOctets"
TS_C64="2026-09-21 04:00:00.000"
ch "INSERT INTO metric_counter_raw (tenant_id, entity_id, metric_id, sensor_id, ts, value, quality, flags, sequence_id, source_protocol) VALUES
('t1','e1','ifHCInOctets','s1', '${TS_C64}', 9007199254740993, 1, 0, '${SEQ_C64}', 'snmp'),
('t1','e1','ifHCInOctets','s1', '${TS_C64}', 9223372036854775808, 1, 0, '${SEQ_C64}-b', 'snmp'),
('t1','e1','ifHCInOctets','s1', '${TS_C64}', 18446744073709551615, 1, 0, '${SEQ_C64}-c', 'snmp')"
[[ "$(ch "SELECT value FROM metric_counter WHERE sequence_id='${SEQ_C64}' FORMAT TSV")" == "9007199254740993" ]] \
  || fail "Counter64 >2^53 not exact"
[[ "$(ch "SELECT value FROM metric_counter WHERE sequence_id='${SEQ_C64}-b' FORMAT TSV")" == "9223372036854775808" ]] \
  || fail "Counter64 >Long.MAX_VALUE not exact"
[[ "$(ch "SELECT value FROM metric_counter WHERE sequence_id='${SEQ_C64}-c' FORMAT TSV")" == "18446744073709551615" ]] \
  || fail "Counter64 UInt64 max not exact"

# Duplicate physical replay of max value; canonical view collapses to one.
# Identical ORDER BY key (including ts) is required for ReplacingMergeTree + FINAL.
ch "INSERT INTO metric_counter_raw (tenant_id, entity_id, metric_id, sensor_id, ts, value, quality, flags, sequence_id, source_protocol) VALUES
('t1','e1','ifHCInOctets','s1', '${TS_C64}', 18446744073709551615, 1, 0, '${SEQ_C64}-c', 'snmp')"
[[ "$(ch "SELECT count() FROM metric_counter_raw WHERE sequence_id='${SEQ_C64}-c'")" == "2" ]] \
  || fail "expected two physical Counter64 replay rows"
[[ "$(ch "SELECT count() FROM metric_counter WHERE sequence_id='${SEQ_C64}-c'")" == "1" ]] \
  || fail "canonical Counter64 view did not collapse replay"

log "restart ClickHouse and verify Counter64 canonical read survives"
docker restart "$SERVER" >/dev/null
wait_ch
assert_counter_target
[[ "$(ch "SELECT value FROM metric_counter WHERE sequence_id='${SEQ_C64}-c' FORMAT TSV")" == "18446744073709551615" ]] \
  || fail "Counter64 exact value lost after restart"
[[ "$(ch "SELECT count() FROM metric_counter WHERE sequence_id='${SEQ_C64}-c'")" == "1" ]] \
  || fail "Counter64 canonical collapse lost after restart"

log "rerun V003 idempotently"
run_v003_only
assert_counter_target

log "PASS"
