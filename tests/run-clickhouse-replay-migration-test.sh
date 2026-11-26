#!/usr/bin/env bash
# Disposable-volume ClickHouse V002 replay-identity migration test.
# Never touches developer platform_ch_data.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CH_IMAGE="clickhouse/clickhouse-server:24.8-alpine"
validate_runtime_profile() {
  # This environment switch routes the test to its fixture; it authenticates
  # neither Docker isolation nor a real runtime.
  local profile="${AMRUDAI_REPLAY_RUNTIME_PROFILE:-}"
  if [[ -n "${REPLAY_HARNESS_MOCK:-}" && ( "$profile" != mock || "${REPLAY_HARNESS_MOCK}" != 1 ) ]]; then
    echo "[clickhouse-replay-migration] FAIL: REPLAY_HARNESS_MOCK is only valid as 1 in mock profile" >&2
    return 1
  fi
  if [[ "$profile" == mock && "${REPLAY_HARNESS_MOCK:-}" == 1 ]]; then
    local docker_bin docker_dir
    docker_bin="$(command -v docker)" || return 1
    docker_dir="$(cd "$(dirname "$docker_bin")" && pwd)"
    [[ "$docker_dir" == /tmp/* && "${DOCKER_HOST:-}" == unix://*/no-daemon.sock ]] || {
      echo "[clickhouse-replay-migration] FAIL: mock mode requires a /tmp Docker fixture and dead-end socket" >&2
      return 1
    }
    return 0
  fi
  if [[ -z "$profile" && "${REPLAY_HARNESS_MOCK:-}" != 1 ]]; then
    echo "[clickhouse-replay-migration] HOLD: shared native runtime awaits the process-bound canonical flock and trusted cooperative runtime admission" >&2
    return 1
  fi
  if [[ "$profile" != hosted-ci || "${REPLAY_HARNESS_MOCK:-}" == 1 ]]; then
    echo "[clickhouse-replay-migration] FAIL: unknown or mixed runtime profile" >&2
    return 1
  fi
  # These values are defensive routing checks only. Actual isolation authority
  # is the separately admitted ephemeral GitHub-hosted job, not environment claims.
  [[ "${GITHUB_ACTIONS:-}" == true && "${RUNNER_ENVIRONMENT:-}" == github-hosted \
     && "${RUNNER_OS:-}" == Linux && "${GITHUB_RUN_ID:-}" =~ ^[0-9]+$ \
     && "${GITHUB_RUN_ATTEMPT:-}" =~ ^[0-9]+$ && -n "${GITHUB_WORKSPACE:-}" \
     && -n "${RUNNER_TEMP:-}" && -d "$RUNNER_TEMP" && -d "$GITHUB_WORKSPACE" ]] || {
    echo "[clickhouse-replay-migration] HOLD: hosted-ci profile lacks expected GitHub-hosted job metadata" >&2
    return 1
  }
  [[ -z "${DOCKER_HOST:-}" && -z "${DOCKER_CONTEXT:-}" ]] || {
    echo "[clickhouse-replay-migration] FAIL: hosted-ci rejects remote Docker endpoint/context overrides" >&2
    return 1
  }
  local config_dir="${RUNNER_TEMP}/platform-ci-docker-cli" config_real runner_temp_real context endpoint
  [[ -d "$RUNNER_TEMP" && -d "$config_dir" && "${DOCKER_CONFIG:-}" == "$config_dir" ]] || {
    echo "[clickhouse-replay-migration] HOLD: exact workflow Docker config directory is absent or mismatched" >&2
    return 1
  }
  runner_temp_real="$(cd "$RUNNER_TEMP" && pwd -P)" || return 1
  [[ "$RUNNER_TEMP" == "$runner_temp_real" ]] || {
    echo "[clickhouse-replay-migration] FAIL: RUNNER_TEMP is not canonical" >&2
    return 1
  }
  config_real="$(cd "$config_dir" && pwd -P)" || return 1
  [[ "$config_real" == "$runner_temp_real/platform-ci-docker-cli" ]] || {
    echo "[clickhouse-replay-migration] FAIL: Docker config escapes the exact workflow job-local path" >&2
    return 1
  }
  if [[ -L "$config_dir/config.json" ]]; then
    echo "[clickhouse-replay-migration] FAIL: Docker config file must not be a symlink" >&2
    return 1
  elif [[ -f "$config_dir/config.json" ]]; then
    python3 - "$config_dir/config.json" <<'PY' || return 1
import json, sys
with open(sys.argv[1], encoding="utf-8") as stream:
    try:
        config = json.load(stream)
    except (json.JSONDecodeError, UnicodeDecodeError) as error:
        raise SystemExit(f"[clickhouse-replay-migration] FAIL: malformed Docker config: {error}")
if not isinstance(config, dict):
    raise SystemExit("[clickhouse-replay-migration] FAIL: Docker config must be a JSON object")
if config.get("currentContext", "default") not in ("", "default"):
    raise SystemExit("[clickhouse-replay-migration] FAIL: Docker config selects a non-default context")
PY
  elif [[ -e "$config_dir/config.json" || -L "$config_dir/config.json" ]]; then
    echo "[clickhouse-replay-migration] FAIL: Docker config is not a regular file" >&2
    return 1
  fi
  context="$(docker context show 2>/dev/null)" || return 1
  [[ "$context" == default ]] || {
    echo "[clickhouse-replay-migration] FAIL: hosted-ci requires the default Docker context" >&2
    return 1
  }
  endpoint="$(docker context inspect default --format '{{(index .Endpoints "docker").Host}}' 2>/dev/null)" || return 1
  [[ "$endpoint" == unix:///var/run/docker.sock ]] || {
    echo "[clickhouse-replay-migration] FAIL: hosted-ci requires the workflow-local Docker socket" >&2
    return 1
  }
}
validate_runtime_profile || exit 1
RUN_ID="$(od -An -N16 -tx1 /dev/urandom | tr -d ' \n')"
[[ "$RUN_ID" =~ ^[0-9a-f]{32}$ ]] || { echo "[clickhouse-replay-migration] FAIL: could not generate run identity" >&2; exit 1; }
OWNER_LABEL="io.amrudai.clickhouse-replay.run=$RUN_ID"
SUFFIX="$RUN_ID"
VOLUME="platform_ch_replay_migration_${SUFFIX}"
NETWORK="platform_ch_replay_net_${SUFFIX}"
SERVER="platform-ch-replay-${SUFFIX}"
MIGRATOR_PREFIX="platform-ch-replay-migrate-${SUFFIX}"
MIGRATOR_INDEX=0
MIGRATOR_RECORDS=()
NETWORK_ID=""
SERVER_ID=""
VOLUME_ID=""
VOLUME_ACQUIRED=0
NETWORK_ACQUIRED=0
SERVER_ACQUIRED=0
USER="platform_app"
PASSWORD="platform_replay_test"

fail() { echo "[clickhouse-replay-migration] FAIL: $*" >&2; exit 1; }
log() { echo "[clickhouse-replay-migration] $*"; }

# The hosted watchdog allows 50s after TERM for signal delivery and this
# 45s cleanup budget. Each Docker call includes its own escalation interval.
CLEANUP_BUDGET_SECONDS=45
CLEANUP_ACTIVE=0
CLEANUP_DEADLINE=0
bounded_docker() {
  # The per-call 15s ceiling includes timeout's TERM-to-KILL grace period.
  local budget=15 grace=2 command_seconds
  if (( CLEANUP_ACTIVE )); then
    budget=$((CLEANUP_DEADLINE - SECONDS))
    (( budget > 0 )) || return 124
    (( budget < 15 )) || budget=15
  fi
  (( budget > grace )) || return 124
  command_seconds=$((budget - grace))
  timeout --signal=TERM --kill-after="$grace" "$command_seconds" docker "$@"
}

inspect_owner() {
  local kind="$1" name="$2" expected="${3:-}" output identity
  case "$kind" in
    container) output="$(bounded_docker container inspect --format '{{.Id}} {{index .Config.Labels "io.amrudai.clickhouse-replay.run"}}' "$name")" || return 1 ;;
    network) output="$(bounded_docker network inspect --format '{{.Id}} {{index .Labels "io.amrudai.clickhouse-replay.run"}}' "$name")" || return 1 ;;
    volume) output="$(bounded_docker volume inspect --format '{{.Name}} {{index .Labels "io.amrudai.clickhouse-replay.run"}}' "$name")" || return 1 ;;
    *) return 1 ;;
  esac
  identity="${output% *}"
  [[ "$output" == "$identity $RUN_ID" && "$identity" != "$output" ]] || return 1
  case "$kind" in
    container|network) [[ "$identity" =~ ^[0-9a-f]{64}$ ]] || return 1 ;;
    volume) [[ "$identity" == "$name" ]] || return 1 ;;
  esac
  [[ -z "$expected" || "$identity" == "$expected" ]]
}

capture_container_id() {
  local name="$1" output id
  output="$(bounded_docker container inspect --format '{{.Id}} {{index .Config.Labels "io.amrudai.clickhouse-replay.run"}}' "$name")" || return 1
  id="${output%% *}"
  [[ "$id" =~ ^[0-9a-f]{64}$ && "${output##* }" == "$RUN_ID" ]] || return 1
  CONTAINER_CAPTURED_ID="$id"
}

capture_network_id() {
  local name="$1" output id
  output="$(bounded_docker network inspect --format '{{.Id}} {{index .Labels "io.amrudai.clickhouse-replay.run"}}' "$name")" || return 1
  id="${output%% *}"
  [[ "$id" =~ ^[0-9a-f]{64}$ && "${output##* }" == "$RUN_ID" ]] || return 1
  NETWORK_CAPTURED_ID="$id"
}

capture_volume_id() {
  local name="$1" output id
  output="$(bounded_docker volume inspect --format '{{.Name}} {{index .Labels "io.amrudai.clickhouse-replay.run"}}' "$name")" || return 1
  id="${output%% *}"
  [[ "$id" == "$name" && "${output##* }" == "$RUN_ID" ]] || return 1
  VOLUME_CAPTURED_ID="$id"
}

remove_owned() {
  local kind="$1" name="$2" ref="$3" inventory
  if ! inspect_owner "$kind" "$name" "$ref"; then
    log "cleanup refused: ownership or identity of $kind $name could not be established by successful inspect"
    return 1
  fi
  if [[ "$kind" == "network" ]]; then
    if ! bounded_docker network rm "$ref" >/dev/null 2>&1; then
      log "cleanup failed removing $kind $name"
      return 1
    fi
  elif ! bounded_docker "$kind" rm -f "$ref" >/dev/null 2>&1; then
    log "cleanup failed removing $kind $name"
    return 1
  fi
  case "$kind" in
    container) inventory="$(bounded_docker container ls -aq --no-trunc --filter "id=$ref")" || { log "cleanup absence query failed for $kind $name"; return 1; } ;;
    network) inventory="$(bounded_docker network ls -q --no-trunc --filter "id=$ref")" || { log "cleanup absence query failed for $kind $name"; return 1; } ;;
    volume) inventory="$(bounded_docker volume ls -q --filter "name=$name")" || { log "cleanup absence query failed for $kind $name"; return 1; } ;;
  esac
  if [[ "$kind" == volume ]]; then
    while IFS= read -r inventory_name; do
      [[ "$inventory_name" != "$name" ]] || { log "cleanup residue: $kind $name"; return 1; }
    done <<< "$inventory"
  else
    [[ -z "$inventory" ]] || { log "cleanup residue: $kind $name"; return 1; }
  fi
  return 0
}

cleanup() {
  local failed=0
  local i record name id
  CLEANUP_ACTIVE=1
  CLEANUP_DEADLINE=$((SECONDS + CLEANUP_BUDGET_SECONDS))
  for i in "${!MIGRATOR_RECORDS[@]}"; do
    record="${MIGRATOR_RECORDS[$i]}"
    name="${record%%|*}"
    id="${record#*|}"
    if [[ -z "$id" ]] && capture_container_id "$name"; then
      id="$CONTAINER_CAPTURED_ID"
      MIGRATOR_RECORDS[$i]="$name|$id"
    fi
    if [[ -z "$id" ]]; then
      log "cleanup refused: immutable identity unavailable for $name"
      failed=1
    else
      remove_owned container "$name" "$id" || failed=1
    fi
  done
  if (( SERVER_ACQUIRED != 0 )); then
    [[ -n "$SERVER_ID" ]] || { capture_container_id "$SERVER" && SERVER_ID="$CONTAINER_CAPTURED_ID"; }
    if [[ -z "$SERVER_ID" ]]; then log "cleanup refused: container identity unavailable for $SERVER"; failed=1
    else remove_owned container "$SERVER" "$SERVER_ID" || failed=1; fi
  fi
  if (( NETWORK_ACQUIRED != 0 )); then
    [[ -n "$NETWORK_ID" ]] || { capture_network_id "$NETWORK" && NETWORK_ID="$NETWORK_CAPTURED_ID"; }
    if [[ -z "$NETWORK_ID" ]]; then log "cleanup refused: network identity unavailable for $NETWORK"; failed=1
    else remove_owned network "$NETWORK" "$NETWORK_ID" || failed=1; fi
  fi
  if (( VOLUME_ACQUIRED != 0 )); then
    if [[ -z "$VOLUME_ID" ]]; then
      capture_volume_id "$VOLUME" && VOLUME_ID="$VOLUME_CAPTURED_ID" || { log "cleanup refused: volume identity unavailable for $VOLUME"; failed=1; }
    fi
    if [[ -z "$VOLUME_ID" ]]; then log "cleanup refused: volume identity unavailable for $VOLUME"; failed=1
    else remove_owned volume "$VOLUME" "$VOLUME_ID" || failed=1; fi
  fi
  return "$failed"
}

on_exit() {
  local original_status=$? cleanup_status=0
  trap - EXIT
  trap '' INT TERM
  cleanup || cleanup_status=$?
  if (( cleanup_status != 0 )); then
    log "cleanup incomplete: owned resource absence is unverified"
  fi
  if (( original_status != 0 )); then
    exit "$original_status"
  fi
  if (( cleanup_status != 0 )); then
    exit 1
  fi
  log "PASS (cooperative cleanup checks only; runtime isolation is not certified; hostile-daemon safety is not certified)"
  exit 0
}
trap on_exit EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

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
  MIGRATOR_INDEX=$((MIGRATOR_INDEX + 1))
  local migrator="${MIGRATOR_PREFIX}-${MIGRATOR_INDEX}"
  local status=0
  MIGRATOR_RECORDS+=("$migrator|")
  docker run --label "$OWNER_LABEL" --name "$migrator" --network "$NETWORK" -e CLICKHOUSE_HOST="$SERVER" -e CLICKHOUSE_NATIVE_PORT=9000 -e CLICKHOUSE_DB=platform -e CLICKHOUSE_USER="$USER" -e CLICKHOUSE_PASSWORD="$PASSWORD" -v "$ROOT_DIR/deploy/docker/clickhouse/migrations:/migrations:ro" --entrypoint /bin/bash "$CH_IMAGE" /migrations/run-all.sh || status=$?
  if capture_container_id "$migrator"; then MIGRATOR_RECORDS[$(( ${#MIGRATOR_RECORDS[@]} - 1 ))]="$migrator|$CONTAINER_CAPTURED_ID"; fi
  return "$status"
}

run_v003_only() {
  MIGRATOR_INDEX=$((MIGRATOR_INDEX + 1))
  local migrator="${MIGRATOR_PREFIX}-${MIGRATOR_INDEX}"
  local status=0
  MIGRATOR_RECORDS+=("$migrator|")
  docker run --label "$OWNER_LABEL" --name "$migrator" --network "$NETWORK" -e CLICKHOUSE_HOST="$SERVER" -e CLICKHOUSE_NATIVE_PORT=9000 -e CLICKHOUSE_DB=platform -e CLICKHOUSE_USER="$USER" -e CLICKHOUSE_PASSWORD="$PASSWORD" -v "$ROOT_DIR/deploy/docker/clickhouse/migrations:/migrations:ro" --entrypoint /bin/bash "$CH_IMAGE" /migrations/V003__metric_counter_exact_uint64.sh || status=$?
  if capture_container_id "$migrator"; then MIGRATOR_RECORDS[$(( ${#MIGRATOR_RECORDS[@]} - 1 ))]="$migrator|$CONTAINER_CAPTURED_ID"; fi
  return "$status"
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
NETWORK_ACQUIRED=1
network_status=0
docker network create --label "$OWNER_LABEL" "$NETWORK" >/dev/null || network_status=$?
if (( network_status != 0 )); then
  log "network acquisition failed with status $network_status"
  exit "$network_status"
fi
if ! capture_network_id "$NETWORK"; then fail "network identity or ownership could not be established"; fi
NETWORK_ID="$NETWORK_CAPTURED_ID"
inspect_owner network "$NETWORK" || fail "network ownership could not be established"
VOLUME_ACQUIRED=1
volume_status=0
docker volume create --label "$OWNER_LABEL" "$VOLUME" >/dev/null || volume_status=$?
if (( volume_status != 0 )); then
  log "volume acquisition failed with status $volume_status"
  exit "$volume_status"
fi
if ! capture_volume_id "$VOLUME"; then fail "volume identity could not be captured"; fi
VOLUME_ID="$VOLUME_CAPTURED_ID"
inspect_owner volume "$VOLUME" "$VOLUME_ID" || fail "volume ownership could not be established"
SERVER_ACQUIRED=1
server_status=0
docker run -d --label "$OWNER_LABEL" --name "$SERVER" --network "$NETWORK" -e CLICKHOUSE_DB=platform -e CLICKHOUSE_USER="$USER" -e CLICKHOUSE_PASSWORD="$PASSWORD" -e CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1 -v "$VOLUME:/var/lib/clickhouse" "$CH_IMAGE" >/dev/null || server_status=$?
if (( server_status != 0 )); then
  log "server acquisition failed with status $server_status"
  exit "$server_status"
fi
if ! capture_container_id "$SERVER"; then fail "server identity or ownership could not be established"; fi
SERVER_ID="$CONTAINER_CAPTURED_ID"
[[ "$SERVER_ID" =~ ^[0-9a-f]{64}$ ]] || fail "server identity is invalid"
inspect_owner container "$SERVER" || fail "server ownership could not be established"
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
