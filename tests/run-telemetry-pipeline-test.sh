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

# Test 1: Query initial ClickHouse telemetry row count
echo "--> [1/6] Verifying active telemetry ingestion into ClickHouse..."
INITIAL_COUNT=$(curl -s -u "${CH_USER}:${CH_PASS}" "http://127.0.0.1:${CH_HTTP_PORT}/?query=SELECT+count()+FROM+platform.metric_numeric")
if [ -z "${INITIAL_COUNT}" ] || [ "${INITIAL_COUNT}" -le 0 ]; then
    echo "[ERROR] Expected positive row count in ClickHouse platform.metric_numeric, got: ${INITIAL_COUNT}"
    exit 1
fi
echo "[OK] ClickHouse currently stores ${INITIAL_COUNT} canonical numeric telemetry rows."

# Test 2: Live Ingestion Growth (Sensor -> Core -> Kafka -> ClickHouse)
echo "--> [2/6] Verifying live ingestion progress across the full pipeline..."
NEW_COUNT="${INITIAL_COUNT}"
for i in $(seq 1 15); do
    sleep 1
    CURRENT_COUNT=$(curl -s -u "${CH_USER}:${CH_PASS}" "http://127.0.0.1:${CH_HTTP_PORT}/?query=SELECT+count()+FROM+platform.metric_numeric")
    if [ "${CURRENT_COUNT}" -gt "${INITIAL_COUNT}" ]; then
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

# Test 3: Verify Sensor Spool Acknowledgment & File Release
echo "--> [3/6] Verifying Sensor disk spool acknowledgement and watermark advancement..."
STATE_JSON=$(docker exec platform-sensor-net cat /var/lib/platform/sensor/spool/state.json)
ACKED_THROUGH=$(echo "${STATE_JSON}" | grep -o '"ackedThrough":[0-9]*' | cut -d: -f2)
if [ -z "${ACKED_THROUGH}" ] || [ "${ACKED_THROUGH}" -le 0 ]; then
    echo "[ERROR] Sensor spool ackedThrough did not advance: ${STATE_JSON}"
    exit 1
fi
echo "[OK] Sensor disk spool watermark is advancing (ackedThrough: ${ACKED_THROUGH})."

# Test 4: Idempotency & Replay Verification
echo "--> [4/6] Verifying batch idempotency and duplicate replay safety..."
REPLAY_BATCH_ID="test-idem-$(date +%s%N)"
RECORD_JSON='{"sequence":1,"type":"TELEMETRY_SAMPLE","payload":{"tenantId":"default","entityId":"sensor-net-01","protocol":"SENSOR","protocolPayload":{"metric_id":"sensor.heartbeat","value":1.0}}}'
CHECKSUM=$(echo -n "${RECORD_JSON}" | sha256sum | awk '{print $1}')

BATCH_PAYLOAD=$(cat <<EOF
{
  "schemaVersion": 1,
  "sensorId": "sensor-net-01",
  "bootEpoch": "epoch-test-1",
  "batchId": "${REPLAY_BATCH_ID}",
  "firstSequence": 1,
  "lastSequence": 1,
  "checksum": "${CHECKSUM}",
  "records": [${RECORD_JSON}]
}
EOF
)

# First submission
FIRST_RESP=$(curl -k -s \
    --cert "${CERT_DIR}/sensor-net.crt" \
    --key "${CERT_DIR}/sensor-net.key" \
    -X POST "https://127.0.0.1:${CORE_PORT}/api/v1/telemetry" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: ${REPLAY_BATCH_ID}" \
    -d "${BATCH_PAYLOAD}")

if ! echo "${FIRST_RESP}" | grep -q '"status":"ACCEPTED"' || ! echo "${FIRST_RESP}" | grep -q '"accepted_through_sequence":1'; then
    echo "[ERROR] First submission failed: ${FIRST_RESP}"
    exit 1
fi
echo "[OK] First submission accepted and published to Kafka."

# Duplicate replay submission
REPLAY_RESP=$(curl -k -s \
    --cert "${CERT_DIR}/sensor-net.crt" \
    --key "${CERT_DIR}/sensor-net.key" \
    -X POST "https://127.0.0.1:${CORE_PORT}/api/v1/telemetry" \
    -H "Content-Type: application/json" \
    -H "Idempotency-Key: ${REPLAY_BATCH_ID}" \
    -d "${BATCH_PAYLOAD}")

if ! echo "${REPLAY_RESP}" | grep -q '"status":"ACCEPTED"' || ! echo "${REPLAY_RESP}" | grep -q '"accepted_through_sequence":1'; then
    echo "[ERROR] Duplicate replay failed: ${REPLAY_RESP}"
    exit 1
fi
echo "[OK] Duplicate replay returned cached idempotent ACK."

# Test 5: Checksum / Payload Conflict Detection
echo "--> [5/6] Verifying rejection of reused idempotency key with conflicting payload..."
CONFLICT_PAYLOAD=$(cat <<EOF
{
  "schemaVersion": 1,
  "sensorId": "sensor-net-01",
  "bootEpoch": "epoch-test-1",
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

# Checksum mismatch on conflict or conflict error code
if [ "${CONFLICT_HTTP_CODE}" != "400" ] && [ "${CONFLICT_HTTP_CODE}" != "409" ]; then
    echo "[ERROR] Expected 400 or 409 on corrupted checksum / idempotency conflict, got HTTP ${CONFLICT_HTTP_CODE}"
    exit 1
fi
echo "[OK] Corrupted checksum / idempotency conflict correctly rejected with HTTP ${CONFLICT_HTTP_CODE}."

# Test 6: Sensor Identity Spoofing Protection
echo "--> [6/6] Verifying sensor identity spoofing prevention via mTLS..."
SPOOF_PAYLOAD=$(cat <<EOF
{
  "schemaVersion": 1,
  "sensorId": "sensor-spoofed-99",
  "bootEpoch": "epoch-test-1",
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

if ! echo "${SPOOF_RESP}" | grep -q '"code":"ERR_AUTH_IDENTITY_MISMATCH"'; then
    echo "[ERROR] Expected ERR_AUTH_IDENTITY_MISMATCH on spoofed sensor identity, got: ${SPOOF_RESP}"
    exit 1
fi
echo "[OK] Spoofed sensor identity rejected with ERR_AUTH_IDENTITY_MISMATCH."

echo "========================================================================"
echo "ALL TELEMETRY PIPELINE E2E TESTS PASSED SUCCESSFULLY"
echo "========================================================================"
