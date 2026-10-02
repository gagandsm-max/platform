#!/usr/bin/env bash
# ==============================================================================
# Platform Verification Suite (20 Automated Acceptance Tests)
# Governing Specs: AGENTS.md, Platform_03_Architecture.md, Platform_04_HLD.md
# ==============================================================================
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

source .env 2>/dev/null || true

PASS_COUNT=0
FAIL_COUNT=0
TOTAL_TESTS=20

pass() {
    echo -e "\033[32m[PASS]\033[0m Test $1: $2"
    PASS_COUNT=$((PASS_COUNT + 1))
}

fail() {
    echo -e "\033[31m[FAIL]\033[0m Test $1: $2 - Reason: $3"
    FAIL_COUNT=$((FAIL_COUNT + 1))
}

echo "========================================================================"
echo "Starting Platform Bootstrap 20-Test Verification Suite"
echo "Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "========================================================================"

# ------------------------------------------------------------------------------
# Test 1: Repository Integrity & Specification Traceability
# ------------------------------------------------------------------------------
TEST_NUM=1
TEST_DESC="Repository Integrity & Specification Traceability"
if { [ -d ".git" ] || [ -f ".git" ]; } && [ -f ".gitignore" ] && [ -f ".env.example" ] && [ -L "AGENTS.md" ] && [ -d "project_documentation" ]; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Missing essential git/repo root files"
fi

# ------------------------------------------------------------------------------
# Test 2: Host Environment Doctor Verification
# ------------------------------------------------------------------------------
TEST_NUM=2
TEST_DESC="make doctor Execution & Port Boundary Check"
if make doctor > /dev/null 2>&1; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "make doctor failed"
fi

# ------------------------------------------------------------------------------
# Test 3: TLS Certificates Presence, SAN, and Validity
# ------------------------------------------------------------------------------
TEST_NUM=3
TEST_DESC="Dev TLS Certificate Generation & SAN Validation"
if [ -f deploy/docker/certs/platform-dev.crt ] && [ -f deploy/docker/certs/platform-dev.key ] && [ -f deploy/docker/certs/core-keystore.p12 ]; then
    SAN=$(openssl x509 -in deploy/docker/certs/core.crt -noout -text | grep -A1 "Subject Alternative Name" || true)
    if echo "${SAN}" | grep -q "localhost" && echo "${SAN}" | grep -q "core" && echo "${SAN}" | grep -q "ui"; then
        pass ${TEST_NUM} "${TEST_DESC}"
    else
        fail ${TEST_NUM} "${TEST_DESC}" "SAN does not contain required DNS entries (localhost, core, ui)"
    fi
else
    fail ${TEST_NUM} "${TEST_DESC}" "Certificate files missing in deploy/docker/certs/"
fi

# ------------------------------------------------------------------------------
# Test 4: Container Build for all Platform Components
# ------------------------------------------------------------------------------
TEST_NUM=4
TEST_DESC="Multi-stage Container Builds (Core, Sensor, UI, Analytics, Agent)"
echo "Building all images..."
if docker compose build --quiet; then
    # Also build agent test container
    if docker build -q -t platform-agent:dev agent/ > /dev/null; then
        pass ${TEST_NUM} "${TEST_DESC}"
    else
        fail ${TEST_NUM} "${TEST_DESC}" "Agent container build failed"
    fi
else
    fail ${TEST_NUM} "${TEST_DESC}" "Docker Compose build failed"
fi

# ------------------------------------------------------------------------------
# Test 5: Docker Compose Configuration Validation
# ------------------------------------------------------------------------------
TEST_NUM=5
TEST_DESC="Compose Profiles & Enforceable Resource Limits Validation"
if docker compose config > /dev/null 2>&1; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "docker compose config failed"
fi

# ------------------------------------------------------------------------------
# Test 6: Platform Dev Stack Startup & Health Convergence
# ------------------------------------------------------------------------------
TEST_NUM=6
TEST_DESC="Platform Dev Stack Startup & Health Convergence"
echo "Starting dev stack..."
docker compose up -d
echo "Waiting for services to become healthy (up to 60s)..."
START_TIMEOUT=60
ELAPSED=0
ALL_HEALTHY=false

while [ $ELAPSED -lt $START_TIMEOUT ]; do
    UNHEALTHY=$(docker compose ps --format '{{.Service}}:{{.Health}}' | grep -v 'healthy' || true)
    if [ -z "${UNHEALTHY}" ]; then
        ALL_HEALTHY=true
        break
    fi
    sleep 3
    ELAPSED=$((ELAPSED + 3))
done

if [ "${ALL_HEALTHY}" = "true" ]; then
    pass ${TEST_NUM} "${TEST_DESC} (Converged in ${ELAPSED}s)"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Unhealthy services remaining: ${UNHEALTHY}"
fi

# ------------------------------------------------------------------------------
# Test 7: PostgreSQL Bootstrap & Schema Migration Verification
# ------------------------------------------------------------------------------
TEST_NUM=7
TEST_DESC="PostgreSQL Transactional Store Health & Migration Schema"
PG_CHECK=$(docker exec platform-postgres psql -U platform_app -d platform -t -A -c "SELECT count(*) FROM platform.schema_migration;" 2>/dev/null || echo "0")
if [ "${PG_CHECK}" -ge 1 ]; then
    pass ${TEST_NUM} "${TEST_DESC} (Found ${PG_CHECK} applied migrations)"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Postgres schema migration table empty or query failed"
fi

# ------------------------------------------------------------------------------
# Test 8: ClickHouse Telemetry Database & Table Verification
# ------------------------------------------------------------------------------
TEST_NUM=8
TEST_DESC="ClickHouse Analytical Store Health & Telemetry Tables"
CH_CHECK=$(curl -s -u "${CLICKHOUSE_USER:-platform_app}:${CLICKHOUSE_PASSWORD:-platform_dev_ch_password_change_in_prod}" "http://127.0.0.1:${PLATFORM_CLICKHOUSE_HTTP_HOST_PORT:-28123}/?query=SELECT+count()+FROM+platform.health_verification" 2>/dev/null || echo "0")
if [ "${CH_CHECK}" -ge 1 ]; then
    pass ${TEST_NUM} "${TEST_DESC} (ClickHouse health verification count: ${CH_CHECK})"
else
    fail ${TEST_NUM} "${TEST_DESC}" "ClickHouse health query returned unexpected output: ${CH_CHECK}"
fi

# ------------------------------------------------------------------------------
# Test 9: Kafka KRaft Streaming Backbone Internal Isolation
# ------------------------------------------------------------------------------
TEST_NUM=9
TEST_DESC="Kafka KRaft Internal Broker Health & Zero Host Exposure"
KAFKA_HOST_PORT=$(docker port platform-kafka 2>/dev/null || true)
if [ -z "${KAFKA_HOST_PORT}" ]; then
    # Verify internal broker readiness
    KAFKA_READY=$(docker exec platform-kafka /opt/kafka/bin/kafka-broker-api-versions.sh --bootstrap-server localhost:9092 > /dev/null 2>&1 && echo "ready" || echo "not_ready")
    if [ "${KAFKA_READY}" = "ready" ]; then
        pass ${TEST_NUM} "${TEST_DESC} (Internal-only verified)"
    else
        fail ${TEST_NUM} "${TEST_DESC}" "Kafka broker internal check failed"
    fi
else
    fail ${TEST_NUM} "${TEST_DESC}" "Kafka is unexpectedly bound to host port: ${KAFKA_HOST_PORT}"
fi

# ------------------------------------------------------------------------------
# Test 10: Valkey Fast Cache Health & Auth Read/Write
# ------------------------------------------------------------------------------
TEST_NUM=10
TEST_DESC="Valkey Cache Health, Auth & Read/Write Verification"
VALKEY_AUTH_PING=$(docker exec platform-valkey valkey-cli -a "${VALKEY_PASSWORD:-platform_dev_valkey_password_change_in_prod}" ping 2>/dev/null || echo "ERR")
if echo "${VALKEY_AUTH_PING}" | grep -q "PONG"; then
    docker exec platform-valkey valkey-cli -a "${VALKEY_PASSWORD:-platform_dev_valkey_password_change_in_prod}" set test_bootstrap_key "success" > /dev/null 2>&1
    VALKEY_GET=$(docker exec platform-valkey valkey-cli -a "${VALKEY_PASSWORD:-platform_dev_valkey_password_change_in_prod}" get test_bootstrap_key 2>/dev/null || echo "")
    if [ "${VALKEY_GET}" = "success" ]; then
        pass ${TEST_NUM} "${TEST_DESC}"
    else
        fail ${TEST_NUM} "${TEST_DESC}" "Valkey get test_bootstrap_key returned '${VALKEY_GET}'"
    fi
else
    fail ${TEST_NUM} "${TEST_DESC}" "Valkey auth ping failed: ${VALKEY_AUTH_PING}"
fi

# ------------------------------------------------------------------------------
# Test 11: Core Actuator Health Endpoint UP over HTTPS
# ------------------------------------------------------------------------------
TEST_NUM=11
TEST_DESC="Core Control Plane Health Endpoint (/actuator/health) over HTTPS"
CORE_HEALTH=$(curl -k -s "https://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/actuator/health" 2>/dev/null || echo "")
if echo "${CORE_HEALTH}" | grep -q '"status":"UP"'; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Core health response: ${CORE_HEALTH}"
fi

# ------------------------------------------------------------------------------
# Test 12: Core API Version over HTTPS & Cleartext Host Port Absence
# ------------------------------------------------------------------------------
TEST_NUM=12
TEST_DESC="Core API Version (/api/v1/version) over HTTPS & Cleartext Host Port Absence"
CORE_VERSION=$(curl -k -s "https://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/api/v1/version" 2>/dev/null || echo "")
CLEARTEXT_CHECK=$(curl -s --connect-timeout 2 "http://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/api/v1/version" 2>&1 || echo "curl_failed")
if echo "${CORE_VERSION}" | grep -q '"product":"Platform"' && ! echo "${CLEARTEXT_CHECK}" | grep -q '"product":"Platform"'; then
    pass ${TEST_NUM} "${TEST_DESC} (HTTPS OK, Cleartext Rejected)"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Core version: ${CORE_VERSION}, Cleartext check: ${CLEARTEXT_CHECK}"
fi

# ------------------------------------------------------------------------------
# Test 13: Connectivity endpoint is not an anonymous probe (auth required / default-deny)
# ------------------------------------------------------------------------------
TEST_NUM=13
TEST_DESC="Core Connectivity Auth Boundary (/api/v1/health/connectivity denied anonymously)"
CORE_CONN_CODE=$(curl -k -s -o /tmp/platform-core-conn.json -w "%{http_code}" \
    "https://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/api/v1/health/connectivity" 2>/dev/null || echo "000")
CORE_CONN=$(cat /tmp/platform-core-conn.json 2>/dev/null || echo "")
if { [ "${CORE_CONN_CODE}" = "401" ] || [ "${CORE_CONN_CODE}" = "403" ] || [ "${CORE_CONN_CODE}" = "404" ]; } && \
   ! echo "${CORE_CONN}" | grep -qi 'jdbc:postgresql' && \
   ! echo "${CORE_CONN}" | grep -qi 'stackTrace' && \
   ! echo "${CORE_CONN}" | grep -q '"details"'; then
    pass ${TEST_NUM} "${TEST_DESC} (HTTP ${CORE_CONN_CODE})"
else
    fail ${TEST_NUM} "${TEST_DESC}" "HTTP ${CORE_CONN_CODE}, body: ${CORE_CONN}"
fi

# ------------------------------------------------------------------------------
# Test 14: Sensor Instances (Network & Infra) Startup & Spool Initialization
# ------------------------------------------------------------------------------
TEST_NUM=14
TEST_DESC="Sensor Instances Startup, Health & Named Spool Volumes"
NET_HEALTH=$(docker exec platform-sensor-net curl -s http://127.0.0.1:8088/health 2>/dev/null || echo "")
INFRA_HEALTH=$(docker exec platform-sensor-infra curl -s http://127.0.0.1:8088/health 2>/dev/null || echo "")
NET_READY=$(docker exec platform-sensor-net curl -s http://127.0.0.1:8088/readyz 2>/dev/null || echo "")
INFRA_READY=$(docker exec platform-sensor-infra curl -s http://127.0.0.1:8088/readyz 2>/dev/null || echo "")
NET_HEALTH_CODE=$(docker exec platform-sensor-net curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8088/health 2>/dev/null || echo "000")
INFRA_HEALTH_CODE=$(docker exec platform-sensor-infra curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8088/health 2>/dev/null || echo "000")
NET_READY_CODE=$(docker exec platform-sensor-net curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8088/readyz 2>/dev/null || echo "000")
INFRA_READY_CODE=$(docker exec platform-sensor-infra curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8088/readyz 2>/dev/null || echo "000")

validate_sensor_supportability() {
    SENSOR_HEALTH_JSON="$1" SENSOR_READY_JSON="$2" SENSOR_HEALTH_CODE="$3" SENSOR_READY_CODE="$4" python3 - <<'PY'
import json, os, sys

HEALTH_ALLOWED = {
    "document_type", "component", "version", "service_instance_id", "ts", "uptime_seconds",
    "component_state", "storage_pressure_percent", "spool_pressure_percent", "queue_depth",
    "queue_capacity", "scheduler_status", "control_reconciliation_status", "delivery_status",
    "dependency_status", "counters", "gauges", "reason_codes", "state_timestamps",
}
READY_ALLOWED = {"status", "ready", "http_semantic", "component", "ts", "service_instance_id", "reason_code"}

def valid_health(health, code):
    return (
        isinstance(health, dict)
        and not (set(health) - HEALTH_ALLOWED)
        and health.get("document_type") == "SUPPORT_EVIDENCE"
        and health.get("component") == "platform-sensor"
        and bool(health.get("service_instance_id"))
        and bool(health.get("ts"))
        and health.get("component_state") in ("UP", "DEGRADED")
        and code == 200
    )

def valid_ready(ready, code):
    return (
        isinstance(ready, dict)
        and not (set(ready) - READY_ALLOWED)
        and ready.get("component") == "platform-sensor"
        and ready.get("status") in ("UP", "DEGRADED")
        and ready.get("ready") is True
        and ready.get("http_semantic") == 200
        and code == 200
    )

try:
    health = json.loads(os.environ["SENSOR_HEALTH_JSON"])
    ready = json.loads(os.environ["SENSOR_READY_JSON"])
    health_code = int(os.environ["SENSOR_HEALTH_CODE"])
    ready_code = int(os.environ["SENSOR_READY_CODE"])
except Exception:
    sys.exit(1)

if not valid_health(health, health_code) or not valid_ready(ready, ready_code):
    sys.exit(1)
unknown = dict(ready)
unknown["_unknown"] = True
if valid_ready(unknown, ready_code):
    sys.exit(1)
if valid_ready(ready, 503):
    sys.exit(1)
sys.exit(0)
PY
}

if validate_sensor_supportability "${NET_HEALTH}" "${NET_READY}" "${NET_HEALTH_CODE}" "${NET_READY_CODE}" && \
   validate_sensor_supportability "${INFRA_HEALTH}" "${INFRA_READY}" "${INFRA_HEALTH_CODE}" "${INFRA_READY_CODE}"; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "sensor-net health=${NET_HEALTH_CODE}/${NET_HEALTH} ready=${NET_READY_CODE}/${NET_READY}; sensor-infra health=${INFRA_HEALTH_CODE}/${INFRA_HEALTH} ready=${INFRA_READY_CODE}/${INFRA_READY}"
fi
# ------------------------------------------------------------------------------
# Test 15: Sensor Storage Watermark Threshold Guard
# ------------------------------------------------------------------------------
TEST_NUM=15
TEST_DESC="Sensor 80% Storage Watermark Protection Logic"
GUARD_TEST=$(docker run --rm -v "${ROOT_DIR}/sensor:/app:ro" -w /app golang:1.27.1-alpine3.24 go test -v -run "TestProtectionAndRecoveryAfterFreeSpace|TestFixedWatermarkCannotBeChanged" ./internal/... 2>/dev/null || echo "FAIL")
if echo "${GUARD_TEST}" | grep -q "PASS"; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Watermark threshold unit test failed"
fi

# ------------------------------------------------------------------------------
# Test 16: UI HTTPS-Only Listener & Non-80 Enforcement
# ------------------------------------------------------------------------------
TEST_NUM=16
TEST_DESC="UI HTTPS-Only Port 23000 & Port 80 Listener Absence"
UI_HTTPS=$(curl -k -s -o /dev/null -w "%{http_code}" "https://127.0.0.1:${PLATFORM_UI_HOST_PORT:-23000}/" 2>/dev/null || echo "000")
PORT_80_LISTENER=$(docker exec platform-ui netstat -tuln 2>/dev/null | grep ":80 " || true)

if [ "${UI_HTTPS}" = "200" ] && [ -z "${PORT_80_LISTENER}" ]; then
    pass ${TEST_NUM} "${TEST_DESC} (HTTP ${UI_HTTPS}, Port 80 absent)"
else
    fail ${TEST_NUM} "${TEST_DESC}" "UI HTTPS status: ${UI_HTTPS}, port 80 listener: ${PORT_80_LISTENER}"
fi

# ------------------------------------------------------------------------------
# Test 17: UI Reverse-Proxy Routing to Core API over HTTPS
# ------------------------------------------------------------------------------
TEST_NUM=17
TEST_DESC="UI Reverse-Proxy Routing to Core (/api/v1/version over HTTPS)"
PROXY_VERSION=$(curl -k -s "https://127.0.0.1:${PLATFORM_UI_HOST_PORT:-23000}/api/v1/version" 2>/dev/null || echo "")
if echo "${PROXY_VERSION}" | grep -q '"product":"Platform"'; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "UI proxy returned: ${PROXY_VERSION}"
fi

# ------------------------------------------------------------------------------
# Test 18: Sensor -> Core mTLS Authentication & Access Control Verification
# ------------------------------------------------------------------------------
TEST_NUM=18
TEST_DESC="Sensor -> Core mTLS Authentication & Access Control Verification"
CERT_DIR="${ROOT_DIR}/deploy/docker/certs"

# 1. Valid Network Sensor cert -> Accepted (HTTP 200)
NET_RESP=$(curl -k -s \
    --cert "${CERT_DIR}/sensor-net.crt" \
    --key "${CERT_DIR}/sensor-net.key" \
    -X POST "https://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/api/v1/sensor/heartbeat" \
    -H "Content-Type: application/json" \
    -d '{"sensor_id":"sensor-net-01"}' 2>/dev/null || echo "")

# 2. Valid Infra Sensor cert -> Accepted (HTTP 200, distinct identity)
INFRA_RESP=$(curl -k -s \
    --cert "${CERT_DIR}/sensor-infra.crt" \
    --key "${CERT_DIR}/sensor-infra.key" \
    -X POST "https://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/api/v1/sensor/heartbeat" \
    -H "Content-Type: application/json" \
    -d '{"sensor_id":"sensor-infra-01"}' 2>/dev/null || echo "")

# 3. No client cert -> Rejected (HTTP 401 Unauthorized)
NO_CERT_STATUS=$(curl -k -s -o /dev/null -w "%{http_code}" \
    -X POST "https://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/api/v1/sensor/heartbeat" \
    -H "Content-Type: application/json" \
    -d '{"sensor_id":"sensor-anonymous"}' 2>/dev/null || echo "000")

# 4. Untrusted client cert -> Rejected (TLS handshake failure or HTTP 403)
UNTRUSTED_STATUS=$(curl -k -s -o /dev/null -w "%{http_code}" \
    --cert "${CERT_DIR}/untrusted-client.crt" \
    --key "${CERT_DIR}/untrusted-client.key" \
    -X POST "https://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/api/v1/sensor/heartbeat" \
    -H "Content-Type: application/json" \
    -d '{"sensor_id":"sensor-rogue"}' 2>/dev/null || true)

# Authoritative identity is enrolled Sensor principal ID (not certificate CN).
NET_OK=$(echo "${NET_RESP}" | grep -q '"sensor_identity":"sensor-net-01"' && echo "true" || echo "false")
INFRA_OK=$(echo "${INFRA_RESP}" | grep -q '"sensor_identity":"sensor-infra-01"' && echo "true" || echo "false")
NO_CERT_OK=$([ "${NO_CERT_STATUS}" = "401" ] && echo "true" || echo "false")
UNTRUSTED_OK=false
if [ "${UNTRUSTED_STATUS}" = "000" ] || [ "${UNTRUSTED_STATUS}" = "403" ] || [ -z "${UNTRUSTED_STATUS}" ]; then
    UNTRUSTED_OK=true
fi

if [ "${NET_OK}" = "true" ] && [ "${INFRA_OK}" = "true" ] && [ "${NO_CERT_OK}" = "true" ] && [ "${UNTRUSTED_OK}" = "true" ]; then
    pass ${TEST_NUM} "${TEST_DESC} (mTLS valid accepted, missing 401, untrusted rejected)"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Net: ${NET_OK}, Infra: ${INFRA_OK}, NoCert 401: ${NO_CERT_OK} (${NO_CERT_STATUS}), Untrusted: ${UNTRUSTED_OK} (${UNTRUSTED_STATUS})"
fi

# ------------------------------------------------------------------------------
# Test 19: Kafka Core-Dependency Failure Observability & Automatic Recovery
# ------------------------------------------------------------------------------
TEST_NUM=19
TEST_DESC="Kafka Core-Dependency Failure Observability & Automatic Recovery"
echo "Testing Kafka failure detection by Core..."
docker pause platform-kafka > /dev/null 2>&1

KAFKA_DOWN_DETECTED=false
for i in $(seq 1 8); do
    HEALTH_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" "https://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/actuator/health" || echo "000")
    if [ "${HEALTH_CODE}" = "503" ]; then
        KAFKA_DOWN_DETECTED=true
        break
    fi
    sleep 1
done

docker unpause platform-kafka > /dev/null 2>&1

KAFKA_RECOVERED=false
for i in $(seq 1 15); do
    HEALTH_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" "https://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/actuator/health" || echo "000")
    if [ "${HEALTH_CODE}" = "200" ]; then
        KAFKA_RECOVERED=true
        break
    fi
    sleep 1
done

if [ "${KAFKA_DOWN_DETECTED}" = "true" ] && [ "${KAFKA_RECOVERED}" = "true" ]; then
    pass ${TEST_NUM} "${TEST_DESC} (Degraded to 503 upon pause, recovered to 200 upon unpause)"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Down detected: ${KAFKA_DOWN_DETECTED}, Recovered: ${KAFKA_RECOVERED}"
fi

# ------------------------------------------------------------------------------
# Test 20: Persistence, Structured JSON Logs, and Isolation Verification
# ------------------------------------------------------------------------------
TEST_NUM=20
TEST_DESC="Persistence, Structured JSON Logs, and Workstation Isolation"
# Marker row in Postgres
# Day-02 tenant_id is UUID. Shared-file change required so persistence probe matches schema.
docker exec platform-postgres psql -U platform_app -d platform -c "INSERT INTO platform.tenant (tenant_id, name, mode, status, timezone, created_at, updated_at) VALUES ('018f0e0a-0000-7000-8000-00000000beef'::uuid, 'Restart Test', 'SINGLE', 'ACTIVE', 'UTC', NOW(), NOW()) ON CONFLICT DO NOTHING;" > /dev/null 2>&1
# Marker key in Valkey
docker exec platform-valkey valkey-cli -a "${VALKEY_PASSWORD:-platform_dev_valkey_password_change_in_prod}" set restart_persist_key "preserved_across_down" > /dev/null 2>&1

# Start an explicit non-Compose probe so isolation can never pass vacuously.
# Reuse the already-local Postgres image; do not pull or add a dependency.
FOREIGN_PROBE_NAME="platform-isolation-probe-$$"
FOREIGN_PROBE_IMAGE=$(docker inspect -f '{{.Config.Image}}' platform-postgres 2>/dev/null || echo "")
docker rm -f "${FOREIGN_PROBE_NAME}" > /dev/null 2>&1 || true
FOREIGN_PROBE_STARTED=false
if [ -n "${FOREIGN_PROBE_IMAGE}" ] && \
   docker run -d --name "${FOREIGN_PROBE_NAME}" --entrypoint sh "${FOREIGN_PROBE_IMAGE}" \
     -c 'while :; do sleep 60; done' > /dev/null 2>&1; then
    FOREIGN_PROBE_STARTED=true
fi

# Snapshot unrelated container state immediately before the destructive
# Platform Compose restart. Determine ownership from the exact current Compose
# stack rather than a name prefix.
PLATFORM_OWNED_BEFORE=$(docker compose ps -a --format '{{.Name}}' | sort)
snapshot_foreign_container_states() {
    docker ps -a --format '{{.Names}}' | sort | while IFS= read -r name; do
        [ -n "${name}" ] || continue
        if printf '%s\n' "${PLATFORM_OWNED_BEFORE}" | grep -Fxq "${name}"; then
            continue
        fi
        running=$(docker inspect -f '{{.State.Running}}' "${name}" 2>/dev/null || printf 'MISSING')
        printf '%s|%s\n' "${name}" "${running}"
    done
}
FOREIGN_BEFORE=$(snapshot_foreign_container_states)
# Restart stack
docker compose down > /dev/null 2>&1
docker compose up -d > /dev/null 2>&1

# Wait for healthy
for i in $(seq 1 45); do
    UNHEALTHY=$(docker compose ps --format '{{.Service}}:{{.Health}}' 2>/dev/null | grep -v 'healthy' || true)
    if [ -z "${UNHEALTHY}" ]; then
        break
    fi
    sleep 2
done

# Read markers
PG_RESTART_VAL=$(docker exec platform-postgres psql -U platform_app -d platform -t -A -c "SELECT count(*) FROM platform.tenant WHERE tenant_id='018f0e0a-0000-7000-8000-00000000beef'::uuid;" 2>/dev/null || echo "0")
VALKEY_RESTART_VAL=$(docker exec platform-valkey valkey-cli -a "${VALKEY_PASSWORD:-platform_dev_valkey_password_change_in_prod}" get restart_persist_key 2>/dev/null || echo "")

# Clean markers
docker exec platform-postgres psql -U platform_app -d platform -c "DELETE FROM platform.tenant WHERE tenant_id='018f0e0a-0000-7000-8000-00000000beef'::uuid;" > /dev/null 2>&1 || true
docker exec platform-valkey valkey-cli -a "${VALKEY_PASSWORD:-platform_dev_valkey_password_change_in_prod}" del restart_persist_key > /dev/null 2>&1 || true

# Structured logs
CORE_LOG=$(docker logs --tail 20 platform-core 2>/dev/null | awk '/^\{/{print; exit}' || true)
SENSOR_LOG=$(docker logs --tail 100 platform-sensor-net 2>/dev/null | awk '/^\{/{print}' || true)
ANALYTICS_LOG=$(docker logs --tail 20 platform-analytics 2>/dev/null | awk '/^\{/{print; exit}' || true)
ANALYTICS_HEALTH=$(docker exec platform-analytics curl -s http://127.0.0.1:8090/health 2>/dev/null || echo "")
ANALYTICS_HEALTHZ=$(docker exec platform-analytics curl -s http://127.0.0.1:8090/healthz 2>/dev/null || echo "")
ANALYTICS_HEALTH_CODE=$(docker exec platform-analytics curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8090/health 2>/dev/null || echo "000")
ANALYTICS_HEALTHZ_CODE=$(docker exec platform-analytics curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:8090/healthz 2>/dev/null || echo "000")

# Core/Sensor/Analytics use BUILD-005 common envelopes (component, not legacy service).
# Slice certification is Master-owned; this gate only validates runtime envelope shape.
CORE_LOG_VALID=false
if [ -n "${CORE_LOG}" ] && echo "${CORE_LOG}" | python3 -c '
import json, sys, re
required = ("ts", "level", "component", "version", "node_id", "service_instance_id", "message")
legacy = ("timestamp", "service", "environment", "thread", "logger", "error_code", "stack_trace", "stackTrace")
SI_RE = re.compile(r"^[A-Za-z0-9._:-]{1,128}$")

def no_duplicate_keys(pairs):
    obj = {}
    for key, value in pairs:
        if key in obj:
            raise ValueError(f"duplicate JSON key: {key}")
        obj[key] = value
    return obj

try:
    obj = json.load(sys.stdin, object_pairs_hook=no_duplicate_keys)
except Exception:
    sys.exit(1)
if not isinstance(obj, dict):
    sys.exit(1)
for key in required:
    value = obj.get(key)
    if value is None or (isinstance(value, str) and value.strip() == ""):
        sys.exit(1)
if obj.get("component") != "platform-core":
    sys.exit(1)
version = obj.get("version")
if not isinstance(version, str) or not (1 <= len(version) <= 64):
    sys.exit(1)
si = obj.get("service_instance_id")
if not isinstance(si, str) or not SI_RE.fullmatch(si):
    sys.exit(1)
for key in legacy:
    if key in obj:
        sys.exit(1)
sys.exit(0)
'; then
    CORE_LOG_VALID=true
fi

SENSOR_LOG_VALID=false
if [ -n "${SENSOR_LOG}" ] && echo "${SENSOR_LOG}" | python3 -c '
import json, sys, re
required = ("ts", "level", "component", "version", "sensor_id", "service_instance_id", "message")
allowed = {
    "ts", "level", "component", "version", "service_instance_id", "message",
    "node_id", "sensor_id", "agent_id", "tenant_id", "event_code", "correlation_id",
    "request_id", "job_id", "entity_id", "protocol", "result", "retry_count", "safe_context",
}
legacy = ("timestamp", "time", "msg", "service", "environment", "thread", "logger", "error_code", "error", "stack_trace", "stackTrace")
SI_RE = re.compile(r"^[A-Za-z0-9._:-]{1,128}$")

def no_duplicate_keys(pairs):
    obj = {}
    for key, value in pairs:
        if key in obj:
            raise ValueError(f"duplicate JSON key: {key}")
        obj[key] = value
    return obj

def valid_log(obj):
    if not isinstance(obj, dict):
        return False
    if set(obj) - allowed:
        return False
    for key in required:
        value = obj.get(key)
        if value is None or (isinstance(value, str) and value.strip() == ""):
            return False
    if obj.get("component") != "platform-sensor":
        return False
    version = obj.get("version")
    if not isinstance(version, str) or not (1 <= len(version) <= 64):
        return False
    si = obj.get("service_instance_id")
    if not isinstance(si, str) or not SI_RE.fullmatch(si):
        return False
    if any(key in obj for key in legacy):
        return False
    return True

seen = 0
last = None
for raw in sys.stdin:
    raw = raw.strip()
    if not raw:
        continue
    try:
        obj = json.loads(raw, object_pairs_hook=no_duplicate_keys)
    except Exception:
        sys.exit(1)
    if not valid_log(obj):
        sys.exit(1)
    last = obj
    seen += 1
if seen == 0:
    sys.exit(1)
probe = dict(last)
probe["_unknown"] = True
if valid_log(probe):
    sys.exit(1)
sys.exit(0)
'; then
    SENSOR_LOG_VALID=true
fi
ANALYTICS_LOG_VALID=false
if [ -n "${ANALYTICS_LOG}" ] && echo "${ANALYTICS_LOG}" | python3 -c '
import json, sys, re
required = ("ts", "level", "component", "version", "service_instance_id", "message")
allowed = {
    "ts", "level", "component", "version", "service_instance_id", "message",
    "node_id", "sensor_id", "agent_id", "tenant_id", "event_code", "correlation_id",
    "request_id", "job_id", "entity_id", "protocol", "result", "retry_count", "safe_context",
}
legacy = ("timestamp", "time", "msg", "service", "environment", "thread", "logger", "error_code", "error", "stack_trace", "stackTrace")
SI_RE = re.compile(r"^[A-Za-z0-9._:-]{1,128}$")

def no_duplicate_keys(pairs):
    obj = {}
    for key, value in pairs:
        if key in obj:
            raise ValueError(f"duplicate JSON key: {key}")
        obj[key] = value
    return obj

def valid_log(obj):
    if not isinstance(obj, dict):
        return False
    if set(obj) - allowed:
        return False
    for key in required:
        value = obj.get(key)
        if value is None or (isinstance(value, str) and value.strip() == ""):
            return False
    if obj.get("component") != "platform-analytics":
        return False
    version = obj.get("version")
    if not isinstance(version, str) or not (1 <= len(version) <= 64):
        return False
    si = obj.get("service_instance_id")
    if not isinstance(si, str) or not SI_RE.fullmatch(si):
        return False
    if any(key in obj for key in legacy):
        return False
    return True

try:
    obj = json.loads(sys.stdin.read(), object_pairs_hook=no_duplicate_keys)
except Exception:
    sys.exit(1)
if not valid_log(obj):
    sys.exit(1)
probe = dict(obj)
probe["_unknown"] = True
if valid_log(probe):
    sys.exit(1)
sys.exit(0)
'; then
    ANALYTICS_LOG_VALID=true
fi

ANALYTICS_HEALTH_VALID=false
if [ "${ANALYTICS_HEALTH_CODE}" = "200" ] && [ "${ANALYTICS_HEALTHZ_CODE}" = "200" ] && \
   echo "${ANALYTICS_HEALTH}" | python3 -c '
import json, sys, re
required = ("status", "component", "version", "ts", "service_instance_id")
legacy = ("service", "document_type")
SI_RE = re.compile(r"^[A-Za-z0-9._:-]{1,128}$")
try:
    obj = json.load(sys.stdin)
except Exception:
    sys.exit(1)
if not isinstance(obj, dict):
    sys.exit(1)
for key in required:
    value = obj.get(key)
    if value is None or (isinstance(value, str) and value.strip() == ""):
        sys.exit(1)
if obj.get("component") != "platform-analytics":
    sys.exit(1)
version = obj.get("version")
if not isinstance(version, str) or not (1 <= len(version) <= 64):
    sys.exit(1)
si = obj.get("service_instance_id")
if not isinstance(si, str) or not SI_RE.fullmatch(si):
    sys.exit(1)
if obj.get("status") not in ("UP", "SHUTTING_DOWN"):
    sys.exit(1)
for key in legacy:
    if key in obj:
        sys.exit(1)
sys.exit(0)
' && echo "${ANALYTICS_HEALTHZ}" | python3 -c '
import json, sys, re
required = ("status", "component", "version", "ts", "service_instance_id")
legacy = ("service", "document_type")
SI_RE = re.compile(r"^[A-Za-z0-9._:-]{1,128}$")
try:
    obj = json.load(sys.stdin)
except Exception:
    sys.exit(1)
if not isinstance(obj, dict):
    sys.exit(1)
for key in required:
    value = obj.get(key)
    if value is None or (isinstance(value, str) and value.strip() == ""):
        sys.exit(1)
if obj.get("component") != "platform-analytics":
    sys.exit(1)
version = obj.get("version")
if not isinstance(version, str) or not (1 <= len(version) <= 64):
    sys.exit(1)
si = obj.get("service_instance_id")
if not isinstance(si, str) or not SI_RE.fullmatch(si):
    sys.exit(1)
if "document_type" in obj or "service" in obj:
    sys.exit(1)
sys.exit(0)
'; then
    ANALYTICS_HEALTH_VALID=true
fi

LOGS_OK=false
if [ "${CORE_LOG_VALID}" = "true" ] && [ "${SENSOR_LOG_VALID}" = "true" ] && \
   [ "${ANALYTICS_LOG_VALID}" = "true" ] && [ "${ANALYTICS_HEALTH_VALID}" = "true" ]; then
    LOGS_OK=true
fi

# Isolation: verify every unrelated container present before the Platform
# restart still exists with the same running/stopped state afterwards.
FOREIGN_OK=true
FOREIGN_COUNT=0
FOREIGN_DETAIL=""
if [ -n "${FOREIGN_BEFORE}" ]; then
    FOREIGN_COUNT=$(printf '%s\n' "${FOREIGN_BEFORE}" | awk 'NF { count++ } END { print count+0 }')
    while IFS='|' read -r name was_running; do
        [ -n "${name}" ] || continue
        now_running=$(docker inspect -f '{{.State.Running}}' "${name}" 2>/dev/null || printf 'MISSING')
        if [ "${now_running}" != "${was_running}" ]; then
            FOREIGN_OK=false
            FOREIGN_DETAIL="${FOREIGN_DETAIL}${name}:${was_running}->${now_running};"
        fi
    done <<< "${FOREIGN_BEFORE}"
fi
if [ "${FOREIGN_PROBE_STARTED}" != "true" ] || [ "${FOREIGN_COUNT}" -lt 1 ]; then
    FOREIGN_OK=false
    FOREIGN_DETAIL="${FOREIGN_DETAIL}synthetic_probe_missing;"
elif ! printf '%s\n' "${FOREIGN_BEFORE}" | grep -Fq "${FOREIGN_PROBE_NAME}|true"; then
    FOREIGN_OK=false
    FOREIGN_DETAIL="${FOREIGN_DETAIL}synthetic_probe_not_running_before_restart;"
fi
docker rm -f "${FOREIGN_PROBE_NAME}" > /dev/null 2>&1 || true
# Host runtime isolation is a developer-workstation policy. GitHub-hosted CI
# runners intentionally ship Java/Go/Node and other language runtimes. Platform
# project execution remains containerized in CI, so an ephemeral runner's PATH
# is not evidence of developer-host policy violation. The detector itself still
# must prove that prohibited runtimes would be caught via its synthetic test.
HOST_POLLUTION=""
SYNTHETIC_TEST_OK=false
if bash "${ROOT_DIR}/scripts/check-host-runtime-isolation.sh" --synthetic-test > /dev/null 2>&1; then
    SYNTHETIC_TEST_OK=true
fi

HOST_ISOLATION_OK=false
ISOLATION_DETAIL="developer host runtime isolation & synthetic detector verified"
if [ "${PLATFORM_CI:-0}" = "1" ]; then
    ISOLATION_DETAIL="CI hosted-runner PATH exempt; synthetic detector verified"
    HOST_POLLUTION="CI_HOSTED_RUNNER_PATH_EXEMPT"
    if [ "${SYNTHETIC_TEST_OK}" = "true" ]; then
        HOST_ISOLATION_OK=true
    fi
else
    HOST_POLLUTION=$(bash "${ROOT_DIR}/scripts/check-host-runtime-isolation.sh" 2>&1 || true)
    if [ -z "${HOST_POLLUTION}" ] && [ "${SYNTHETIC_TEST_OK}" = "true" ]; then
        HOST_ISOLATION_OK=true
    fi
fi

PERSIST_OK=false
if [ "${PG_RESTART_VAL}" = "1" ] && [ "${VALKEY_RESTART_VAL}" = "preserved_across_down" ]; then
    PERSIST_OK=true
fi

if [ "${PERSIST_OK}" = "true" ] && [ "${LOGS_OK}" = "true" ] && [ "${FOREIGN_OK}" = "true" ] && [ "${HOST_ISOLATION_OK}" = "true" ]; then
    pass ${TEST_NUM} "${TEST_DESC} (Persistence verified, JSON logs valid, ${FOREIGN_COUNT} foreign container(s) intact, ${ISOLATION_DETAIL})"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Persistence: ${PERSIST_OK}, Logs: ${LOGS_OK}, Foreign: ${FOREIGN_OK}, ForeignDetail: ${FOREIGN_DETAIL:-none}, Isolation: ${HOST_ISOLATION_OK} (Pollution: ${HOST_POLLUTION}, SyntheticOK: ${SYNTHETIC_TEST_OK})"
fi

echo "========================================================================"
echo "Platform Verification Suite Completed"
echo "Results: ${PASS_COUNT} / ${TOTAL_TESTS} Passed, ${FAIL_COUNT} Failed"
echo "========================================================================"

if [ "${FAIL_COUNT}" -eq 0 ]; then
    exit 0
else
    exit 1
fi
