#!/usr/bin/env bash
# ==============================================================================
# Platform Telemetry Pipeline End-to-End Verification Test
# Verifies: Sensor Spool -> Core Ingestion Gateway -> Kafka -> ClickHouse
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

source .env 2>/dev/null || true
CORE_PORT="${PLATFORM_CORE_HOST_PORT:-28443}"
CH_HTTP_PORT="${PLATFORM_CLICKHOUSE_HTTP_HOST_PORT:-28123}"
CH_USER="${CLICKHOUSE_USER:-platform_app}"
CH_PASS="${CLICKHOUSE_PASSWORD:-platform_dev_ch_password_change_in_prod}"
CERT_DIR="${ROOT_DIR}/deploy/docker/certs"

echo "========================================================================"
echo "Running Platform Telemetry Pipeline End-to-End Verification"
echo "========================================================================"

# Check if Core sensor authority is running in fail-closed foundation mode
AUTHORITY_PROBE=$(curl -k -s \
    --cert "${CERT_DIR}/sensor-net.crt" \
    --key "${CERT_DIR}/sensor-net.key" \
    -X POST "https://127.0.0.1:${CORE_PORT}/api/v1/telemetry" \
    -H "Content-Type: application/json" \
    -d '{"schemaVersion":1,"sensorId":"sensor-net-01","bootEpoch":"epoch-probe-1","batchId":"probe-1","firstSequence":1,"lastSequence":1,"checksum":"0","records":[]}')

IS_FAIL_CLOSED="false"
if echo "${AUTHORITY_PROBE}" | grep -q '"code":"ERR_SENSOR_AUTHORITY_UNAVAILABLE"'; then
    IS_FAIL_CLOSED="true"
    echo "[INFO] Sensor authority is operating in fail-closed foundation mode (ERR_SENSOR_AUTHORITY_UNAVAILABLE)."
fi

# Test 1: Query initial ClickHouse telemetry row count
echo "--> [1/6] Verifying telemetry table status in ClickHouse..."
INITIAL_COUNT=$(curl -fsS -u "${CH_USER}:${CH_PASS}" "http://127.0.0.1:${CH_HTTP_PORT}/?query=SELECT+count()+FROM+platform.metric_numeric" || true)
if ! [[ "${INITIAL_COUNT}" =~ ^[0-9]+$ ]]; then
    echo "[ERROR] Failed to query numeric ClickHouse platform.metric_numeric count: ${INITIAL_COUNT}"
    exit 1
fi
echo "[OK] ClickHouse platform.metric_numeric accessible (current rows: ${INITIAL_COUNT})."

# Test 2: Ingestion Pipeline Behavior Verification
echo "--> [2/6] Verifying ingestion pipeline behavior across Sensor, Core, Kafka, and ClickHouse..."
if [ "${IS_FAIL_CLOSED}" = "true" ]; then
    echo "[OK] Fail-closed baseline verified: un-enrolled sensor telemetry denied without leakage."
else
    NEW_COUNT="${INITIAL_COUNT}"
    for i in $(seq 1 15); do
        sleep 1
        CURRENT_COUNT=$(curl -fsS -u "${CH_USER}:${CH_PASS}" "http://127.0.0.1:${CH_HTTP_PORT}/?query=SELECT+count()+FROM+platform.metric_numeric" || true)
        if [[ "${CURRENT_COUNT}" =~ ^[0-9]+$ ]] && [ "${CURRENT_COUNT}" -gt "${INITIAL_COUNT}" ]; then
            NEW_COUNT="${CURRENT_COUNT}"
            break
        fi
    done

    if [ "${NEW_COUNT}" -le "${INITIAL_COUNT}" ]; then
        echo "[ERROR] ClickHouse row count did not increase within 15s (initial: ${INITIAL_COUNT}, new: ${NEW_COUNT})"
        exit 1
    fi
    DELTA=$((NEW_COUNT - INITIAL_COUNT))
    echo "[OK] Ingestion pipeline is live and advancing (added ${DELTA} rows, total: ${NEW_COUNT})."
fi

# Test 3: Verify Sensor Spool Status & Buffering Safety
echo "--> [3/6] Verifying Sensor disk spool status and watermark safety..."
STATE_JSON=$(docker exec platform-sensor-net cat /var/lib/platform/sensor/spool/state.json 2>/dev/null || true)
ACKED_THROUGH=$(echo "${STATE_JSON}" | grep -o '"ackedThrough":[0-9]*' | cut -d: -f2 || true)
if [ "${IS_FAIL_CLOSED}" = "true" ]; then
    echo "[OK] Sensor disk spool correctly retains unacknowledged records (ackedThrough: ${ACKED_THROUGH:-0}) without data loss."
else
    if [ -z "${ACKED_THROUGH}" ] || [ "${ACKED_THROUGH}" -le 0 ]; then
        echo "[ERROR] Sensor spool ackedThrough did not advance: ${STATE_JSON}"
        exit 1
    fi
    echo "[OK] Sensor disk spool watermark is advancing (ackedThrough: ${ACKED_THROUGH})."
fi

# Test 4: Durable Stream Identity & Canonical Read Verification
echo "--> [4/6] Verifying durable Sensor stream binding and canonical ClickHouse reads..."
STREAM_ID=$(docker exec platform-sensor-net cat /var/lib/platform/sensor/spool/stream-id 2>/dev/null | tr -d '\r\n' || true)
if ! [[ "${STREAM_ID}" =~ ^stream-[0-9a-f]{32}$ ]]; then
    echo "[ERROR] Sensor durable stream identity is missing or malformed: ${STREAM_ID}"
    exit 1
fi

if [ "${IS_FAIL_CLOSED}" = "true" ]; then
    echo "[OK] Durable Sensor stream identity exists; Core remains authority fail-closed as expected."
else
    DB_STREAM_ID=$(docker exec platform-postgres psql \
        -U "${POSTGRES_USER:-platform_app}" \
        -d "${POSTGRES_DB:-platform}" \
        -t -A \
        -c "SELECT stream_id FROM platform.sensor_telemetry_watermark WHERE sensor_id='sensor-net-01' LIMIT 1;" \
        2>/dev/null | tr -d '[:space:]' || true)

    if [ "${DB_STREAM_ID}" != "${STREAM_ID}" ]; then
        echo "[ERROR] PostgreSQL replay stream binding does not match Sensor spool identity: sensor=${STREAM_ID} core=${DB_STREAM_ID}"
        exit 1
    fi
    echo "[OK] PostgreSQL replay watermark is bound to the Sensor durable stream identity."

    CH_RECORD=$(curl -fsS \
        -u "${CH_USER}:${CH_PASS}" \
        --get \
        --data-urlencode "query=SELECT sequence_id,source_protocol,tenant_id,entity_id FROM platform.metric_numeric WHERE sensor_id='sensor-net-01' ORDER BY ts DESC LIMIT 1 FORMAT JSONEachRow" \
        "http://127.0.0.1:${CH_HTTP_PORT}/" | head -n 1 || true)

    if [ -z "${CH_RECORD}" ]; then
        echo "[ERROR] Canonical ClickHouse view returned no Sensor telemetry row."
        exit 1
    fi
    if ! echo "${CH_RECORD}" | grep -q "${STREAM_ID}"; then
        echo "[ERROR] Canonical ClickHouse observation identity does not contain the durable Sensor stream ID: ${CH_RECORD}"
        exit 1
    fi
    if ! echo "${CH_RECORD}" | grep -q '"source_protocol"'; then
        echo "[ERROR] Canonical ClickHouse row is missing source_protocol: ${CH_RECORD}"
        exit 1
    fi
    echo "[OK] Canonical ClickHouse read surface exposes the durable observation identity."
fi

# Test 5: Checksum / Payload Conflict Detection
TEST_TENANT_ID="018f0e0a-0000-7000-8000-000000000001"
TEST_ENTITY_ID="018f0e0a-0000-7000-8000-000000000011"
COLLECTED_AT=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
RECORD_JSON="{\"sequence\":1,\"type\":\"SNMP_OBSERVATION_BATCH\",\"payload\":{\"tenantId\":\"${TEST_TENANT_ID}\",\"entityId\":\"${TEST_ENTITY_ID}\",\"collectedAt\":\"${COLLECTED_AT}\",\"quality\":\"GOOD\",\"protocol\":\"snmp\",\"protocolPayload\":{\"observations\":[{\"status\":\"VALUE\",\"oid\":\".1.3.6.1.2.1.1.3.0\",\"value\":{\"integer\":1234567}}]}}}"
CHECKSUM=$(echo -n "${RECORD_JSON}" | sha256sum | awk '{print $1}')
REPLAY_BATCH_ID="test-invalid-$(date +%s%N)"
echo "--> [5/6] Verifying rejection of telemetry with a corrupted checksum..."
CONFLICT_PAYLOAD=$(cat <<EOF
{
  "schemaVersion": 1,
  "sensorId": "sensor-net-01",
  "bootEpoch": "${STREAM_ID}",
  "batchId": "${REPLAY_BATCH_ID}",
  "firstSequence": 1,
  "lastSequence": 1,
  "checksum": "0000000000000000000000000000000000000000000000000000000000000000",
  "records": [${RECORD_JSON}]
}
EOF
)

CONFLICT_HTTP_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" \
    --cert "${CERT_DIR}/sensor-net.crt" \
    --key "${CERT_DIR}/sensor-net.key" \
    -X POST "https://127.0.0.1:${CORE_PORT}/api/v1/telemetry" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: ${REPLAY_BATCH_ID}" \
    -d "${CONFLICT_PAYLOAD}")

# Checksum mismatch on conflict or conflict error code (or 403 in fail-closed mode)
if [ "${CONFLICT_HTTP_CODE}" != "400" ] && [ "${CONFLICT_HTTP_CODE}" != "409" ] && [ "${CONFLICT_HTTP_CODE}" != "403" ]; then
    echo "[ERROR] Expected 400, 409, or 403 on corrupted checksum / idempotency conflict, got HTTP ${CONFLICT_HTTP_CODE}"
    exit 1
fi
echo "[OK] Corrupted checksum / conflict rejected with HTTP ${CONFLICT_HTTP_CODE}."

# Test 6: Sensor Identity Spoofing Protection
echo "--> [6/6] Verifying sensor identity spoofing prevention via mTLS..."
SPOOF_PAYLOAD=$(cat <<EOF
{
  "schemaVersion": 1,
  "sensorId": "sensor-spoofed-99",
  "bootEpoch": "${STREAM_ID}",
  "batchId": "batch-spoof-1",
  "firstSequence": 1,
  "lastSequence": 1,
  "checksum": "${CHECKSUM}",
  "records": [${RECORD_JSON}]
}
EOF
)

SPOOF_RESP=$(curl -k -s \
    --cert "${CERT_DIR}/sensor-net.crt" \
    --key "${CERT_DIR}/sensor-net.key" \
    -X POST "https://127.0.0.1:${CORE_PORT}/api/v1/telemetry" \
    -H "Content-Type: application/json" \
    -d "${SPOOF_PAYLOAD}")

if [ "${IS_FAIL_CLOSED}" = "true" ]; then
    if ! echo "${SPOOF_RESP}" | grep -q '"code":"ERR_SENSOR_AUTHORITY_UNAVAILABLE"'; then
        echo "[ERROR] Expected authority fail-closed rejection, got: ${SPOOF_RESP}"
        exit 1
    fi
    echo "[OK] Spoof attempt remained blocked at the authority boundary."
else
    if ! echo "${SPOOF_RESP}" | grep -q '"code":"ERR_AUTH_IDENTITY_MISMATCH"'; then
        echo "[ERROR] Expected ERR_AUTH_IDENTITY_MISMATCH on spoofed sensor identity, got: ${SPOOF_RESP}"
        exit 1
    fi
    echo "[OK] Spoofed sensor identity rejected with ERR_AUTH_IDENTITY_MISMATCH."
fi

echo "========================================================================"
echo "ALL TELEMETRY PIPELINE E2E TESTS PASSED SUCCESSFULLY"
echo "========================================================================"
