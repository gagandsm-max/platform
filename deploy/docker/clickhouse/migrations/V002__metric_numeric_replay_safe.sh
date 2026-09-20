#!/usr/bin/env bash
# ==============================================================================
# V002: metric_numeric → ReplacingMergeTree (stable observation replay protection)
#
# One-shot idempotent migration for existing ClickHouse volumes where init.sql
# already ran with legacy MergeTree ORDER BY (tenant_id, entity_id, metric_id, ts).
# CREATE TABLE IF NOT EXISTS cannot alter engine/keys; this script migrates in place.
#
# Exit 0 only after system-metadata schema gate passes (Core may then start).
# On failure the legacy table is left recoverable; incomplete staging tables may
# remain and are cleaned on a successful retry.
# ==============================================================================
set -euo pipefail

CH_HOST="${CLICKHOUSE_HOST:-clickhouse}"
CH_PORT="${CLICKHOUSE_NATIVE_PORT:-9000}"
CH_USER="${CLICKHOUSE_USER:-platform_app}"
CH_PASSWORD="${CLICKHOUSE_PASSWORD:-platform_dev_ch_password_change_in_prod}"
CH_DB="${CLICKHOUSE_DB:-platform}"

TABLE="metric_numeric"
STAGING_TABLE="metric_numeric_v002_new"

TARGET_ENGINE="ReplacingMergeTree"
TARGET_PRIMARY_KEY="tenant_id, entity_id, metric_id, ts"
TARGET_SORTING_KEY="tenant_id, entity_id, metric_id, ts, sensor_id, sequence_id"
TARGET_DEDUP_SETTING="non_replicated_deduplication_window = 10000"
REQUIRED_COLUMNS="tenant_id entity_id metric_id sensor_id ts value quality flags sequence_id source_protocol"

LEGACY_ENGINE="MergeTree"
LEGACY_SORTING_KEY="tenant_id, entity_id, metric_id, ts"

log() { printf '[ch-migrate-v002] %s\n' "$*"; }
die() { log "ERROR: $*"; exit 1; }

ch_query_raw() {
    clickhouse-client \
        --host "${CH_HOST}" \
        --port "${CH_PORT}" \
        --user "${CH_USER}" \
        --password "${CH_PASSWORD}" \
        --query "$1"
}

ch_query() {
    clickhouse-client \
        --host "${CH_HOST}" \
        --port "${CH_PORT}" \
        --user "${CH_USER}" \
        --password "${CH_PASSWORD}" \
        --database "${CH_DB}" \
        --query "$1"
}

wait_for_clickhouse() {
    local attempt dbs
    for attempt in $(seq 1 90); do
        dbs="$(clickhouse-client \
            --host "${CH_HOST}" \
            --port "${CH_PORT}" \
            --user "${CH_USER}" \
            --password "${CH_PASSWORD}" \
            --query "SHOW DATABASES" 2>/dev/null || true)"
        if echo "${dbs}" | grep -qx "${CH_DB}"; then
            return 0
        fi
        # Database may not exist yet on a totally empty volume; connectivity is enough to proceed
        # once SELECT 1 works — CREATE DATABASE follows in main().
        if clickhouse-client \
            --host "${CH_HOST}" \
            --port "${CH_PORT}" \
            --user "${CH_USER}" \
            --password "${CH_PASSWORD}" \
            --query "SELECT 1" >/dev/null 2>&1; then
            # Prefer waiting briefly for entrypoint-created DB when CLICKHOUSE_DB is set on server.
            if [[ "${attempt}" -ge 5 ]]; then
                return 0
            fi
        fi
        sleep 1
    done
    die "ClickHouse not reachable at ${CH_HOST}:${CH_PORT} after 90s"
}

create_target_table() {
    local name="$1"
    ch_query "CREATE TABLE ${name} (tenant_id String, entity_id String, metric_id String, sensor_id String, ts DateTime64(3, 'UTC'), value Float64, quality UInt8, flags UInt32, sequence_id String, source_protocol LowCardinality(String)) ENGINE = ReplacingMergeTree() PRIMARY KEY (tenant_id, entity_id, metric_id, ts) ORDER BY (tenant_id, entity_id, metric_id, ts, sensor_id, sequence_id) SETTINGS non_replicated_deduplication_window = 10000"
}

table_exists() {
    local name="$1"
    local count
    count="$(ch_query "SELECT count() FROM system.tables WHERE database = currentDatabase() AND name = '${name}'")"
    [[ "${count}" == "1" ]]
}

read_table_meta() {
    local name="$1"
    ch_query "SELECT engine, primary_key, sorting_key, create_table_query FROM system.tables WHERE database = currentDatabase() AND name = '${name}' FORMAT TSV"
}

columns_ok() {
    local name="$1"
    local col
    for col in ${REQUIRED_COLUMNS}; do
        local present
        present="$(ch_query "SELECT count() FROM system.columns WHERE database = currentDatabase() AND table = '${name}' AND name = '${col}'")"
        [[ "${present}" == "1" ]] || return 1
    done
    return 0
}

schema_gate_ok() {
    local name="$1"
    local meta engine primary_key sorting_key create_sql
    meta="$(read_table_meta "${name}")" || return 1
    [[ -n "${meta}" ]] || return 1
    IFS=$'\t' read -r engine primary_key sorting_key create_sql <<<"${meta}"
    [[ "${engine}" == "${TARGET_ENGINE}" ]] || return 1
    [[ "${primary_key}" == "${TARGET_PRIMARY_KEY}" ]] || return 1
    [[ "${sorting_key}" == "${TARGET_SORTING_KEY}" ]] || return 1
    [[ "${create_sql}" == *"${TARGET_DEDUP_SETTING}"* ]] || return 1
    columns_ok "${name}" || return 1
    return 0
}

assert_schema_gate() {
    local name="$1"
    if schema_gate_ok "${name}"; then
        log "schema gate PASS for ${CH_DB}.${name}"
        return 0
    fi
    local meta
    meta="$(read_table_meta "${name}" 2>/dev/null || true)"
    die "schema gate FAIL for ${CH_DB}.${name}; metadata='${meta}'"
}

is_legacy_merge_tree() {
    local name="$1"
    local meta engine primary_key sorting_key create_sql
    meta="$(read_table_meta "${name}")" || return 1
    IFS=$'\t' read -r engine primary_key sorting_key create_sql <<<"${meta}"
    [[ "${engine}" == "${LEGACY_ENGINE}" ]] || return 1
    [[ "${sorting_key}" == "${LEGACY_SORTING_KEY}" ]] || return 1
    columns_ok "${name}" || return 1
    return 0
}

drop_table_if_exists() {
    local name="$1"
    ch_query "DROP TABLE IF EXISTS ${name}"
}

migrate_legacy() {
    local source_count staging_count source_count_after

    log "legacy MergeTree detected; migrating with preserve→copy→EXCHANGE→verify"

    if table_exists "${STAGING_TABLE}"; then
        log "removing incomplete staging table ${STAGING_TABLE} from a prior attempt"
        drop_table_if_exists "${STAGING_TABLE}"
    fi

    create_target_table "${STAGING_TABLE}"
    source_count="$(ch_query "SELECT count() FROM ${TABLE}")"
    log "copying ${source_count} row(s) into staging ${STAGING_TABLE}"
    ch_query "INSERT INTO ${STAGING_TABLE} SELECT * FROM ${TABLE}"
    staging_count="$(ch_query "SELECT count() FROM ${STAGING_TABLE}")"
    source_count_after="$(ch_query "SELECT count() FROM ${TABLE}")"
    if [[ "${staging_count}" != "${source_count}" || "${source_count_after}" != "${source_count}" ]]; then
        drop_table_if_exists "${STAGING_TABLE}"
        die "row-count mismatch after copy: source_before=${source_count} staging=${staging_count} source_after=${source_count_after}; legacy ${TABLE} left untouched (retry when writers are idle)"
    fi

    # Atomic name swap (verified on clickhouse/clickhouse-server:24.8-alpine).
    # After EXCHANGE: TABLE holds target data; STAGING_TABLE holds legacy backup.
    log "EXCHANGE TABLES ${TABLE} AND ${STAGING_TABLE}"
    ch_query "EXCHANGE TABLES ${TABLE} AND ${STAGING_TABLE}"

    assert_schema_gate "${TABLE}"
    local final_count
    final_count="$(ch_query "SELECT count() FROM ${TABLE}")"
    [[ "${final_count}" == "${source_count}" ]] \
        || die "post-swap row-count mismatch: expected=${source_count} actual=${final_count}; legacy backup remains as ${STAGING_TABLE}"

    log "post-swap verify succeeded; dropping legacy backup ${STAGING_TABLE}"
    drop_table_if_exists "${STAGING_TABLE}"
    log "legacy→target migration complete (${final_count} row(s))"
}

main() {
    log "starting (host=${CH_HOST}:${CH_PORT} db=${CH_DB})"
    wait_for_clickhouse
    ch_query_raw "CREATE DATABASE IF NOT EXISTS ${CH_DB}"

    if ! table_exists "${TABLE}"; then
        log "table missing; creating target schema (fresh volume path)"
        create_target_table "${TABLE}"
        assert_schema_gate "${TABLE}"
        log "done (created)"
        exit 0
    fi

    if schema_gate_ok "${TABLE}"; then
        log "already at target schema; noop verify"
        # Stale staging/backup from a previous successful swap is safe to remove.
        if table_exists "${STAGING_TABLE}"; then
            log "removing leftover staging/backup ${STAGING_TABLE}"
            drop_table_if_exists "${STAGING_TABLE}"
        fi
        assert_schema_gate "${TABLE}"
        log "done (noop)"
        exit 0
    fi

    if is_legacy_merge_tree "${TABLE}"; then
        migrate_legacy
        assert_schema_gate "${TABLE}"
        log "done (migrated)"
        exit 0
    fi

    die "unsupported ${CH_DB}.${TABLE} schema; refusing to mutate. Core must not start. metadata='$(read_table_meta "${TABLE}" 2>/dev/null || true)'"
}

main "$@"
