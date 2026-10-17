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

# TLS trust material for HTTPS probes (server SAN includes 127.0.0.1).
CERT_DIR="${REPO_ROOT}/deploy/docker/certs"
CA_CERT="${CERT_DIR}/ca.crt"
# Engineering fixture identities from DevSensorEnrollmentBootstrap (not production defaults).
EXPECTED_SENSOR_IDENTITY="sensor-net-01"
EXPECTED_TENANT_ID="018f0e0a-0000-7000-8000-000000000001"
MAX_HEARTBEAT_BYTES=8192

PROBE_BODY="$(mktemp)"
cleanup_probe_body() { rm -f "${PROBE_BODY}"; }
trap cleanup_probe_body EXIT

https_probe_status() {
    # Args: url [extra curl args...]
    # Prints HTTP status code on stdout. Returns curl exit status.
    local url="$1"
    shift
    : > "${PROBE_BODY}"
    curl -sS \
        --cacert "${CA_CERT}" \
        --connect-timeout 5 \
        --max-time 15 \
        -o "${PROBE_BODY}" \
        -w "%{http_code}" \
        "$@" \
        "${url}"
}

validate_sensor_heartbeat_body() {
    # Reads body path from $1. Exit 0 only for exact fixture authority + schema shape.
    # Never prints the body. Expected identities are constants — not derived from the body.
    local body_path="$1"
    python3 - "${body_path}" "${EXPECTED_SENSOR_IDENTITY}" "${EXPECTED_TENANT_ID}" "${MAX_HEARTBEAT_BYTES}" <<'PY'
import json
import re
import sys

body_path, expected_sensor, expected_tenant, max_bytes_s = sys.argv[1:5]
max_bytes = int(max_bytes_s)
try:
    with open(body_path, "rb") as fh:
        raw = fh.read(max_bytes + 1)
except OSError:
    sys.exit(1)
if len(raw) > max_bytes:
    sys.exit(2)
try:
    text = raw.decode("utf-8")
except UnicodeDecodeError:
    sys.exit(3)

def no_duplicate_keys(pairs):
    obj = {}
    for key, value in pairs:
        if key in obj:
            raise ValueError(f"duplicate JSON key: {key}")
        obj[key] = value
    return obj

try:
    obj = json.loads(text, object_pairs_hook=no_duplicate_keys)
except Exception:
    sys.exit(4)
if not isinstance(obj, dict):
    sys.exit(5)

required = ("status", "authenticated", "sensor_identity", "tenant_id", "received_at")
allowed = set(required)
if set(obj.keys()) != allowed:
    sys.exit(6)

status = obj.get("status")
authenticated = obj.get("authenticated")
sensor_identity = obj.get("sensor_identity")
tenant_id = obj.get("tenant_id")
received_at = obj.get("received_at")

if not isinstance(status, str) or status != "ACCEPTED":
    sys.exit(7)
if authenticated is not True:
    sys.exit(8)
if not isinstance(sensor_identity, str) or sensor_identity != expected_sensor:
    sys.exit(9)
if not isinstance(tenant_id, str) or tenant_id != expected_tenant:
    sys.exit(10)
if not isinstance(received_at, str):
    sys.exit(11)
# schemas/edge/common-definitions.schema.json#/$defs/utcTimestamp
if not re.fullmatch(
    r"[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?Z",
    received_at,
):
    sys.exit(12)
if not (20 <= len(received_at) <= 64):
    sys.exit(13)
if not (1 <= len(sensor_identity) <= 255):
    sys.exit(14)
if not re.fullmatch(
    r"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}",
    tenant_id,
):
    sys.exit(15)
sys.exit(0)
PY
}

if [ ! -f "${CA_CERT}" ]; then
    echo "[FAIL] Missing Platform CA for HTTPS verification (${CA_CERT})."
    FAILED=$((FAILED + 1))
fi

# 5. Verify Platform Web UI HTTPS Endpoint
echo "--> [5/7] Verifying Platform Web UI HTTPS endpoint..."
if [ ! -f "${CA_CERT}" ]; then
    echo "[FAIL] Web UI HTTPS probe skipped: CA certificate missing."
    FAILED=$((FAILED + 1))
else
    set +e
    UI_STATUS="$(https_probe_status "https://127.0.0.1:23000/")"
    UI_CURL_EC=$?
    set -e
    if [ "${UI_CURL_EC}" -ne 0 ]; then
        echo "[FAIL] Web UI HTTPS transport failed (curl_exit=${UI_CURL_EC})."
        FAILED=$((FAILED + 1))
    elif [ "${UI_STATUS}" != "200" ]; then
        echo "[FAIL] Web UI HTTPS endpoint returned HTTP ${UI_STATUS}."
        FAILED=$((FAILED + 1))
    else
        echo "[OK] Web UI HTTPS endpoint responds with HTTP 200."
    fi
fi

# 6. Verify Platform Core HTTPS Actuator Health Endpoint
echo "--> [6/7] Verifying Platform Core direct HTTPS Actuator health..."
if [ ! -f "${CA_CERT}" ]; then
    echo "[FAIL] Core HTTPS probe skipped: CA certificate missing."
    FAILED=$((FAILED + 1))
else
    set +e
    CORE_HEALTH="$(https_probe_status "https://127.0.0.1:28443/actuator/health")"
    CORE_CURL_EC=$?
    set -e
    if [ "${CORE_CURL_EC}" -ne 0 ]; then
        echo "[FAIL] Core HTTPS transport failed (curl_exit=${CORE_CURL_EC})."
        FAILED=$((FAILED + 1))
    elif [ "${CORE_HEALTH}" != "200" ]; then
        echo "[FAIL] Core direct HTTPS endpoint returned HTTP ${CORE_HEALTH}."
        FAILED=$((FAILED + 1))
    else
        echo "[OK] Core direct HTTPS endpoint responds with HTTP 200."
    fi
fi

# 7. Verify Sensor mTLS Ingestion Transport + typed heartbeat authority
echo "--> [7/7] Verifying Sensor -> Core mTLS transport..."
SENSOR_CERT="${CERT_DIR}/sensor-net.crt"
SENSOR_KEY="${CERT_DIR}/sensor-net.key"
if [ ! -f "${CA_CERT}" ] || [ ! -f "${SENSOR_CERT}" ] || [ ! -f "${SENSOR_KEY}" ]; then
    echo "[FAIL] Sensor heartbeat probe skipped: TLS material missing."
    FAILED=$((FAILED + 1))
else
    set +e
    HB_STATUS="$(https_probe_status "https://127.0.0.1:28443/api/v1/sensor/heartbeat" \
        --cert "${SENSOR_CERT}" \
        --key "${SENSOR_KEY}" \
        -X POST \
        -H "Content-Type: application/json" \
        -d '{"sensor_id":"sensor-net-01"}')"
    HB_CURL_EC=$?
    set -e
    if [ "${HB_CURL_EC}" -ne 0 ]; then
        echo "[FAIL] Sensor heartbeat transport failed (curl_exit=${HB_CURL_EC})."
        FAILED=$((FAILED + 1))
    elif [ "${HB_STATUS}" != "200" ]; then
        echo "[FAIL] Sensor heartbeat returned HTTP ${HB_STATUS}."
        FAILED=$((FAILED + 1))
    else
        set +e
        validate_sensor_heartbeat_body "${PROBE_BODY}"
        HB_VALID_EC=$?
        set -e
        if [ "${HB_VALID_EC}" -eq 0 ]; then
            echo "[OK] Sensor mTLS heartbeat accepted with expected fixture authority."
        else
            echo "[FAIL] Sensor heartbeat response failed authority/schema validation (code=${HB_VALID_EC})."
            FAILED=$((FAILED + 1))
        fi
    fi
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
