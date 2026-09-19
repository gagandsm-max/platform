#!/usr/bin/env bash
# ==============================================================================
# Disposable-volume ClickHouse V002 migration test (PINNED 24.8).
# Does NOT touch developer platform_ch_data.
# ==============================================================================
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CH_IMAGE="clickhouse/clickhouse-server:24.8-alpine"
VOLUME_NAME="platform_ch_migration_test_$$"
NETWORK_NAME="platform_ch_migration_net_$$"
CH_NAME="platform-ch-migration-src_$$"
MIGRATE_NAME="platform-ch-migration-job_$$"
CH_USER="platform_app"
CH_PASSWORD="platform_migration_test_password"
SENTINEL_METRIC="migration.sentinel.v002"
AUTH_B64="$(printf '%s:%s' "${CH_USER}" "${CH_PASSWORD}" | base64 -w0 2>/dev/null || printf '%s:%s' "${CH_USER}" "${CH_PASSWORD}" | base64)"

cleanup() {
    docker rm -f "${CH_NAME}" "${MIGRATE_NAME}" >/dev/null 2>&1 || true
    docker network rm "${NETWORK_NAME}" >/dev/null 2>&1 || true
    docker volume rm -f "${VOLUME_NAME}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

log() { printf '[ch-migration-test] %s\n' "$*"; }
fail() { log "FAIL: $*"; exit 1; }

ch_exec() {
    docker exec "${CH_NAME}" clickhouse-client \
        --user "${CH_USER}" \
        --password "${CH_PASSWORD}" \
        --database platform \
        --query "$1"
}

ch_exec_raw() {
    docker exec "${CH_NAME}" clickhouse-client \
        --user "${CH_USER}" \
        --password "${CH_PASSWORD}" \
        --query "$1"
}

http_insert() {
    local token="$1"
    local row="$2"
    docker exec "${CH_NAME}" sh -c "
printf '%s' '${row}' > /tmp/ch_row.json
wget -q -O- --header='Authorization: Basic ${AUTH_B64}' --post-file=/tmp/ch_row.json \
  'http://127.0.0.1:8123/?query=INSERT%20INTO%20platform.metric_numeric%20FORMAT%20JSONEachRow&insert_deduplication_token=${token}'
"
}

wait_ch() {
    local i dbs
    for i in $(seq 1 90); do
        dbs="$(docker exec "${CH_NAME}" clickhouse-client \
            --user "${CH_USER}" \
            --password "${CH_PASSWORD}" \
            --query "SHOW DATABASES" 2>/dev/null || true)"
        if echo "${dbs}" | grep -qx platform; then
            return 0
        fi
        sleep 1
    done
    fail "ClickHouse failed to become ready with database platform"
}

run_migration() {
    docker rm -f "${MIGRATE_NAME}" >/dev/null 2>&1 || true
    docker run --rm \
        --name "${MIGRATE_NAME}" \
        --network "${NETWORK_NAME}" \
        -e CLICKHOUSE_HOST="${CH_NAME}" \
        -e CLICKHOUSE_NATIVE_PORT=9000 \
        -e CLICKHOUSE_USER="${CH_USER}" \
        -e CLICKHOUSE_PASSWORD="${CH_PASSWORD}" \
        -e CLICKHOUSE_DB=platform \
        -v "${ROOT_DIR}/deploy/docker/clickhouse/migrations:/migrations:ro" \
        --entrypoint /bin/bash \
        "${CH_IMAGE}" \
        /migrations/V002__metric_numeric_replay_safe.sh
}

assert_target_schema() {
    local meta engine primary_key sorting_key create_sql
    meta="$(ch_exec "SELECT engine, primary_key, sorting_key, create_table_query FROM system.tables WHERE database='platform' AND name='metric_numeric' FORMAT TSV")"
    IFS=$'\t' read -r engine primary_key sorting_key create_sql <<<"${meta}"
    [[ "${engine}" == "ReplacingMergeTree" ]] || fail "engine=${engine}"
    [[ "${primary_key}" == "tenant_id, entity_id, metric_id, ts" ]] || fail "primary_key=${primary_key}"
    [[ "${sorting_key}" == "tenant_id, entity_id, metric_id, ts, sensor_id, sequence_id" ]] || fail "sorting_key=${sorting_key}"
    [[ "${create_sql}" == *"non_replicated_deduplication_window = 10000"* ]] || fail "dedup window missing in create_table_query"
    local cols
    cols="$(ch_exec "SELECT name FROM system.columns WHERE database='platform' AND table='metric_numeric' ORDER BY position")"
    echo "${cols}" | grep -qx 'tenant_id' || fail "missing tenant_id"
    echo "${cols}" | grep -qx 'sequence_id' || fail "missing sequence_id"
    echo "${cols}" | grep -qx 'source_protocol' || fail "missing source_protocol"
}

log "1) clean disposable ClickHouse volume=${VOLUME_NAME}"
docker network create "${NETWORK_NAME}" >/dev/null
docker volume create "${VOLUME_NAME}" >/dev/null
docker run -d \
    --name "${CH_NAME}" \
    --network "${NETWORK_NAME}" \
    -e CLICKHOUSE_DB=platform \
    -e CLICKHOUSE_USER="${CH_USER}" \
    -e CLICKHOUSE_PASSWORD="${CH_PASSWORD}" \
    -e CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1 \
    -v "${VOLUME_NAME}:/var/lib/clickhouse" \
    "${CH_IMAGE}" >/dev/null
wait_ch

log "2) create OLD (main) schema exactly"
ch_exec_raw "CREATE DATABASE IF NOT EXISTS platform"
ch_exec_raw "CREATE TABLE platform.metric_numeric (tenant_id String, entity_id String, metric_id String, sensor_id String, ts DateTime64(3, 'UTC'), value Float64, quality UInt8, flags UInt32, sequence_id String, source_protocol LowCardinality(String)) ENGINE = MergeTree() ORDER BY (tenant_id, entity_id, metric_id, ts)"

log "3) insert sentinel rows"
ch_exec "INSERT INTO metric_numeric VALUES ('tenant-mig', 'entity-1', '${SENTINEL_METRIC}', 'sensor-1', '2026-09-19 12:00:00.000', 11.0, 1, 0, 'seq-1', 'snmp')"
ch_exec "INSERT INTO metric_numeric VALUES ('tenant-mig', 'entity-2', '${SENTINEL_METRIC}', 'sensor-1', '2026-09-19 12:00:01.000', 22.0, 1, 0, 'seq-2', 'snmp')"
PRE_COUNT="$(ch_exec "SELECT count() FROM platform.metric_numeric WHERE metric_id='${SENTINEL_METRIC}'")"
[[ "${PRE_COUNT}" == "2" ]] || fail "expected 2 sentinel rows before migration, got ${PRE_COUNT}"

log "4) optional restart preserving volume"
docker restart "${CH_NAME}" >/dev/null
wait_ch
POST_RESTART_COUNT="$(ch_exec "SELECT count() FROM platform.metric_numeric WHERE metric_id='${SENTINEL_METRIC}'")"
[[ "${POST_RESTART_COUNT}" == "2" ]] || fail "sentinel rows lost across restart"

log "5) run V002 migration"
run_migration

log "6) verify rows + engine + keys + dedup"
assert_target_schema
POST_COUNT="$(ch_exec "SELECT count() FROM platform.metric_numeric WHERE metric_id='${SENTINEL_METRIC}'")"
[[ "${POST_COUNT}" == "2" ]] || fail "expected 2 sentinel rows after migration, got ${POST_COUNT}"

log "7) rerun migration (idempotent)"
run_migration

log "8) no dupes / schema still correct"
assert_target_schema
RERUN_COUNT="$(ch_exec "SELECT count() FROM platform.metric_numeric WHERE metric_id='${SENTINEL_METRIC}'")"
[[ "${RERUN_COUNT}" == "2" ]] || fail "idempotent rerun changed row count to ${RERUN_COUNT}"

log "9) same-token duplicate insert deduped"
DEDUP_METRIC="migration.dedup.token"
DEDUP_ROW="{\"tenant_id\":\"tenant-mig\",\"entity_id\":\"entity-d\",\"metric_id\":\"${DEDUP_METRIC}\",\"sensor_id\":\"sensor-1\",\"ts\":\"2026-09-19 13:00:00.000\",\"value\":1,\"quality\":1,\"flags\":0,\"sequence_id\":\"tok-1\",\"source_protocol\":\"snmp\"}"
http_insert "migration-token-v002" "${DEDUP_ROW}"
http_insert "migration-token-v002" "${DEDUP_ROW}"
TOKEN_COUNT="$(ch_exec "SELECT count() FROM platform.metric_numeric WHERE metric_id='${DEDUP_METRIC}'")"
[[ "${TOKEN_COUNT}" == "1" ]] || fail "same-token dedup expected 1 row, got ${TOKEN_COUNT}"

log "10) stable-observation replay with different tokens (FINAL / PR semantics)"
REPLAY_METRIC="migration.replay.stable"
REPLAY_ROW="{\"tenant_id\":\"tenant-mig\",\"entity_id\":\"entity-r\",\"metric_id\":\"${REPLAY_METRIC}\",\"sensor_id\":\"sensor-1\",\"ts\":\"2026-09-19 14:00:00.000\",\"value\":9,\"quality\":1,\"flags\":0,\"sequence_id\":\"stable-seq\",\"source_protocol\":\"snmp\"}"
http_insert "replay-a" "${REPLAY_ROW}"
http_insert "replay-b" "${REPLAY_ROW}"
REPLAY_COUNT="$(ch_exec "SELECT count() FROM platform.metric_numeric FINAL WHERE metric_id='${REPLAY_METRIC}'")"
[[ "${REPLAY_COUNT}" == "1" ]] || fail "stable replay FINAL expected 1 row, got ${REPLAY_COUNT}"

log "PASS: V002 migration test (legacy preserve, idempotent rerun, token dedup, stable replay)"
