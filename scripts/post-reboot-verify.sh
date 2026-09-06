#!/usr/bin/env bash
# ==============================================================================
# Platform Post-Host-Reboot Structural Verification Script
# Governing Specs: AGENTS.md, Platform_13, Platform_14
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

echo "========================================================================"
echo "Platform Post-Host-Reboot Structural Verification"
echo "========================================================================"

FAILED=0

# 1. Verify Docker Engine is running
echo "--> [1/7] Verifying Docker Engine..."
if ! docker info > /dev/null 2>&1; then
    echo "[FAIL] Docker daemon is not responding."
    exit 1
fi
echo "[OK] Docker Engine is active."

# 2. Check Platform containers status and health
echo "--> [2/7] Checking Platform containers and health convergence..."
REQUIRED_CONTAINERS=(
    "platform-postgres"
    "platform-clickhouse"
    "platform-kafka"
    "platform-valkey"
    "platform-core"
    "platform-ui"
    "platform-sensor-net"
    "platform-sensor-infra"
    "platform-analytics"
)

for c in "${REQUIRED_CONTAINERS[@]}"; do
    STATUS=$(docker inspect "$c" -f '{{.State.Status}}' 2>/dev/null || echo "not_found")
    HEALTH=$(docker inspect "$c" -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}no_healthcheck{{end}}' 2>/dev/null || echo "unknown")
    if [ "$STATUS" != "running" ]; then
        echo "[FAIL] Container $c is not running (status: $STATUS)"
        FAILED=$((FAILED + 1))
    elif [ "$HEALTH" != "healthy" ] && [ "$HEALTH" != "no_healthcheck" ]; then
        echo "[FAIL] Container $c is not healthy (health: $HEALTH)"
        FAILED=$((FAILED + 1))
    else
        echo "[OK] Container $c is running ($HEALTH)"
    fi
done

# 3. Verify Foreign Workstation Containers Undisturbed
echo "--> [3/7] Verifying foreign workstation containers are intact..."
FOREIGN_ALL=$(docker ps -a --format '{{.Names}}' | grep -v '^platform' || true)
if [ -n "${FOREIGN_ALL}" ]; then
    FOREIGN_COUNT=$(echo "${FOREIGN_ALL}" | wc -l)
    FOREIGN_RUNNING=$(docker ps --format '{{.Names}}' | grep -v '^platform' | wc -l)
    if [ "${FOREIGN_RUNNING}" -lt "${FOREIGN_COUNT}" ]; then
        echo "[FAIL] Foreign containers were disturbed: running ${FOREIGN_RUNNING} / total ${FOREIGN_COUNT}"
        FAILED=$((FAILED + 1))
    else
        echo "[OK] All ${FOREIGN_COUNT} foreign container(s) intact and running."
    fi
else
    echo "[OK] No foreign containers on workstation."
fi

# 4. Verify Persistent Data Integrity (Read authoritative bootstrap records)
echo "--> [4/7] Verifying datastore persistence..."
PG_TENANTS=$(docker exec platform-postgres psql -U platform_app -d platform -t -A -c "SELECT count(*) FROM platform.tenant;" 2>/dev/null || echo "0")
if [ "${PG_TENANTS}" -ge 1 ]; then
    echo "[OK] PostgreSQL persistent state verified (${PG_TENANTS} tenant records)."
else
    echo "[FAIL] PostgreSQL persistent state missing."
    FAILED=$((FAILED + 1))
fi

CH_HEALTH=$(docker exec platform-clickhouse clickhouse-client --database platform --query "SELECT count(*) FROM health_verification;" 2>/dev/null || echo "0")
if [ "${CH_HEALTH}" -ge 1 ]; then
    echo "[OK] ClickHouse persistent state verified (${CH_HEALTH} verification records)."
else
    echo "[FAIL] ClickHouse persistent state missing."
    FAILED=$((FAILED + 1))
fi

VALKEY_PING=$(docker exec platform-valkey valkey-cli -a "${VALKEY_PASSWORD:-platform_dev_valkey_password_change_in_prod}" ping 2>/dev/null || echo "FAIL")
if [ "${VALKEY_PING}" = "PONG" ]; then
    echo "[OK] Valkey cache operational (PONG)."
else
    echo "[FAIL] Valkey cache failed ping."
    FAILED=$((FAILED + 1))
fi

# 5. Verify Platform Web UI HTTPS Endpoint
echo "--> [5/7] Verifying Platform Web UI HTTPS endpoint..."
UI_STATUS=$(curl -k -s -o /dev/null -w "%{http_code}" https://127.0.0.1:23000/ || echo "000")
if [ "${UI_STATUS}" = "200" ]; then
    echo "[OK] Web UI HTTPS endpoint responds with HTTP 200."
else
    echo "[FAIL] Web UI HTTPS endpoint returned HTTP ${UI_STATUS}."
    FAILED=$((FAILED + 1))
fi

# 6. Verify Platform Core HTTPS Actuator Health Endpoint
echo "--> [6/7] Verifying Platform Core direct HTTPS Actuator health..."
CORE_HEALTH=$(curl -k -s -o /dev/null -w "%{http_code}" https://127.0.0.1:28443/actuator/health || echo "000")
if [ "${CORE_HEALTH}" = "200" ]; then
    echo "[OK] Core direct HTTPS endpoint responds with HTTP 200."
else
    echo "[FAIL] Core direct HTTPS endpoint returned HTTP ${CORE_HEALTH}."
    FAILED=$((FAILED + 1))
fi

# 7. Verify Sensor mTLS Ingestion Transport
echo "--> [7/7] Verifying Sensor -> Core mTLS transport..."
CERT_DIR="${REPO_ROOT}/deploy/docker/certs"
MTLS_RESP=$(curl -k -s \
    --cert "${CERT_DIR}/sensor-net.crt" \
    --key "${CERT_DIR}/sensor-net.key" \
    -X POST https://127.0.0.1:28443/api/v1/sensor/heartbeat \
    -H "Content-Type: application/json" \
    -d '{"sensor_id":"sensor-net-01"}' || echo '{"status":"FAIL"}')

if echo "${MTLS_RESP}" | grep -q '"ACCEPTED"'; then
    echo "[OK] Sensor mTLS client certificate successfully authenticated by Core."
else
    echo "[FAIL] Sensor mTLS authentication failed. Response: ${MTLS_RESP}"
    FAILED=$((FAILED + 1))
fi

echo "========================================================================"
if [ ${FAILED} -eq 0 ]; then
    echo "VERIFICATION RESULT: ALL STRUCTURAL CHECKS PASSED"
    echo "REBOOT READINESS STATUS:"
    echo "STRUCTURALLY READY — ACTUAL HOST REBOOT TEST PENDING"
    exit 0
else
    echo "VERIFICATION RESULT: ${FAILED} CHECK(S) FAILED"
    exit 1
fi
