#!/usr/bin/env bash
# ==============================================================================
# Platform API & Data Contract Verification Suite
# Governing Spec: Platform_17_API_Event_and_Data_Model_Contracts.md
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

source .env 2>/dev/null || true
CORE_PORT="${PLATFORM_CORE_HOST_PORT:-28443}"
CERT_DIR="${ROOT_DIR}/deploy/docker/certs"

echo "========================================================================"
echo "Running Platform API Contract Suite"
echo "========================================================================"

# 1. Core Health Actuator Schema Contract
echo "--> [1/4] Verifying Actuator health contract schema..."
HEALTH_JSON=$(curl -k -s "https://127.0.0.1:${CORE_PORT}/actuator/health")
for comp in "status" "components" "db" "clickHouse" "kafka" "redis" "diskSpace"; do
    if ! echo "${HEALTH_JSON}" | grep -q "\"${comp}\""; then
        echo "[ERROR] Contract violation: missing '${comp}' in health response: ${HEALTH_JSON}"
        exit 1
    fi
done
echo "[OK] Actuator health schema conforms to quad-datastore contract."

# 2. Version API Contract
echo "--> [2/4] Verifying Core version contract schema..."
VER_JSON=$(curl -k -s "https://127.0.0.1:${CORE_PORT}/api/v1/version")
if echo "${VER_JSON}" | grep -q '"product":"Platform"' && \
   echo "${VER_JSON}" | grep -q '"version"' && \
   echo "${VER_JSON}" | grep -q '"node_id"' && \
   echo "${VER_JSON}" | grep -q '"status":"UP"'; then
    echo "[OK] Version API contract schema verified."
else
    echo "[ERROR] Contract violation in version payload: ${VER_JSON}"
    exit 1
fi

# 3. Sensor Ingestion mTLS Response Contract
echo "--> [3/4] Verifying Sensor heartbeat response contract schema..."
INGEST_RESP=$(curl -k -s \
    --cert "${CERT_DIR}/sensor-net.crt" \
    --key "${CERT_DIR}/sensor-net.key" \
    -X POST "https://127.0.0.1:${CORE_PORT}/api/v1/sensor/heartbeat" \
    -H "Content-Type: application/json" \
    -d '{"sensor_id":"sensor-net-01"}')
if echo "${INGEST_RESP}" | grep -q '"status":"ACCEPTED"' && \
   echo "${INGEST_RESP}" | grep -q '"authenticated":true' && \
   echo "${INGEST_RESP}" | grep -q '"sensor_identity":"sensor-net-01.sensor.platform.local"'; then
    echo "[OK] Sensor ingestion response contract verified."
else
    echo "[ERROR] Contract violation in sensor ingestion response: ${INGEST_RESP}"
    exit 1
fi

# 4. Mandatory Correlation ID Header Contract
echo "--> [4/4] Verifying X-Correlation-ID HTTP header contract..."
CORR_ID=$(curl -k -s -I "https://127.0.0.1:${CORE_PORT}/api/v1/version" | grep -i "x-correlation-id" || true)
if [ -n "${CORR_ID}" ]; then
    echo "[OK] X-Correlation-ID header present: ${CORR_ID}"
else
    echo "[ERROR] Contract violation: X-Correlation-ID missing in response headers!"
    exit 1
fi

echo "========================================================================"
echo "ALL API CONTRACT TESTS PASSED SUCCESSFULLY"
echo "========================================================================"
