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

# 1. Core Health Contracts (exact public probe boundary; connectivity is not anonymous)
echo "--> [1/4] Verifying anonymous actuator health + connectivity auth boundary..."
HEALTH_JSON=$(curl -k -s "https://127.0.0.1:${CORE_PORT}/actuator/health")
if ! echo "${HEALTH_JSON}" | grep -q '"status"'; then
    echo "[ERROR] Contract violation: missing 'status' in anonymous health response: ${HEALTH_JSON}"
    exit 1
fi
# Anonymous health must not dump internal component graphs (security boundary).
for leak in '"components"' '"db"' '"clickHouse"' '"kafka"' '"redis"'; do
    if echo "${HEALTH_JSON}" | grep -q "${leak}"; then
        echo "[ERROR] Contract violation: anonymous /actuator/health must not expose ${leak}: ${HEALTH_JSON}"
        exit 1
    fi
done
echo "[OK] Anonymous actuator health is status-only."

for probe in "/actuator/health/liveness" "/actuator/health/readiness"; do
    PROBE_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" "https://127.0.0.1:${CORE_PORT}${probe}")
    if [ "${PROBE_CODE}" != "200" ] && [ "${PROBE_CODE}" != "503" ]; then
        echo "[ERROR] Contract violation: anonymous ${probe} must be reachable, got HTTP ${PROBE_CODE}"
        exit 1
    fi
done
echo "[OK] Anonymous actuator liveness/readiness probes are reachable."

CONN_CODE=$(curl -k -s -o /tmp/platform-conn-body.json -w "%{http_code}" \
    "https://127.0.0.1:${CORE_PORT}/api/v1/health/connectivity")
CONN_JSON=$(cat /tmp/platform-conn-body.json 2>/dev/null || true)
if [ "${CONN_CODE}" = "200" ] || [ "${CONN_CODE}" = "503" ]; then
    echo "[ERROR] Contract violation: anonymous /api/v1/health/connectivity must be denied, got HTTP ${CONN_CODE}: ${CONN_JSON}"
    exit 1
fi
if [ "${CONN_CODE}" != "401" ] && [ "${CONN_CODE}" != "403" ] && [ "${CONN_CODE}" != "404" ]; then
    echo "[ERROR] Contract violation: unexpected connectivity status HTTP ${CONN_CODE}: ${CONN_JSON}"
    exit 1
fi
for leak in 'jdbc:postgresql' 'password' 'stackTrace' '"details"' 'Exception'; do
    if echo "${CONN_JSON}" | grep -qi "${leak}"; then
        echo "[ERROR] Contract violation: connectivity denial body leaked '${leak}': ${CONN_JSON}"
        exit 1
    fi
done
echo "[OK] Anonymous connectivity is denied without backend leak material."

UNKNOWN_HEALTH_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" \
    "https://127.0.0.1:${CORE_PORT}/api/v1/health/unknown-bypass")
if [ "${UNKNOWN_HEALTH_CODE}" = "200" ] || [ "${UNKNOWN_HEALTH_CODE}" = "503" ]; then
    echo "[ERROR] Contract violation: unknown /api/v1/health/* must not bypass auth (HTTP ${UNKNOWN_HEALTH_CODE})"
    exit 1
fi
echo "[OK] Unknown /api/v1/health/* cannot bypass authentication."

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
# Authoritative sensor_identity is the enrolled Sensor principal ID (not certificate CN).
if echo "${INGEST_RESP}" | grep -q '"status":"ACCEPTED"' && \
   echo "${INGEST_RESP}" | grep -q '"authenticated":true' && \
   echo "${INGEST_RESP}" | grep -q '"sensor_identity":"sensor-net-01"'; then
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
