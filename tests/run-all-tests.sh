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
if [ -d ".git" ] && [ -f ".gitignore" ] && [ -f ".env.example" ] && [ -L "AGENTS.md" ] && [ -d "project_documentation" ]; then
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
if [ -f deploy/docker/certs/platform-dev.crt ] && [ -f deploy/docker/certs/platform-dev.key ]; then
    SAN=$(openssl x509 -in deploy/docker/certs/platform-dev.crt -noout -text | grep -A1 "Subject Alternative Name" || true)
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
TEST_DESC="Compose Profiles & Deterministic Specification Validation"
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
# Test 11: Core Actuator Health Endpoint UP
# ------------------------------------------------------------------------------
TEST_NUM=11
TEST_DESC="Core Control Plane Health Endpoint (/actuator/health)"
CORE_HEALTH=$(curl -s "http://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/actuator/health" 2>/dev/null || echo "")
if echo "${CORE_HEALTH}" | grep -q '"status":"UP"'; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Core health response: ${CORE_HEALTH}"
fi

# ------------------------------------------------------------------------------
# Test 12: Core API Version & Product Identity
# ------------------------------------------------------------------------------
TEST_NUM=12
TEST_DESC="Core API Version Endpoint (/api/v1/version)"
CORE_VERSION=$(curl -s "http://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/api/v1/version" 2>/dev/null || echo "")
if echo "${CORE_VERSION}" | grep -q '"product":"Platform"'; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Core version response: ${CORE_VERSION}"
fi

# ------------------------------------------------------------------------------
# Test 13: Core Connectivity Probe (Postgres, ClickHouse, Valkey)
# ------------------------------------------------------------------------------
TEST_NUM=13
TEST_DESC="Core Tri-Datastore Connectivity Verification (/api/v1/health/connectivity)"
CORE_CONN=$(curl -s "http://127.0.0.1:${PLATFORM_CORE_HOST_PORT:-28443}/api/v1/health/connectivity" 2>/dev/null || echo "")
if echo "${CORE_CONN}" | grep -q '"postgres"' && \
   echo "${CORE_CONN}" | grep -q '"clickhouse"' && \
   echo "${CORE_CONN}" | grep -q '"valkey"' && \
   echo "${CORE_CONN}" | grep -q '"overall_status":"UP"'; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Connectivity response: ${CORE_CONN}"
fi

# ------------------------------------------------------------------------------
# Test 14: Sensor Instances (Network & Infra) Startup & Spool Initialization
# ------------------------------------------------------------------------------
TEST_NUM=14
TEST_DESC="Sensor Instances Startup, Health & Named Spool Volumes"
NET_HEALTH=$(docker exec platform-sensor-net curl -s http://127.0.0.1:8088/health 2>/dev/null || echo "")
INFRA_HEALTH=$(docker exec platform-sensor-infra curl -s http://127.0.0.1:8088/health 2>/dev/null || echo "")

if echo "${NET_HEALTH}" | grep -q '"status":"UP"' && echo "${INFRA_HEALTH}" | grep -q '"status":"UP"'; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "sensor-net: ${NET_HEALTH}, sensor-infra: ${INFRA_HEALTH}"
fi

# ------------------------------------------------------------------------------
# Test 15: Sensor Storage Watermark Threshold Guard
# ------------------------------------------------------------------------------
TEST_NUM=15
TEST_DESC="Sensor 80% Storage Watermark Protection Logic"
# Run isolated unit test inside Go container verifying watermark threshold enforcement
GUARD_TEST=$(docker run --rm -v "${ROOT_DIR}/sensor:/app:ro" -w /app golang:1.23-alpine go test -v -run TestWatermarkThreshold ./internal/spool 2>/dev/null || echo "FAIL")
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
# Check container for port 80 listener
PORT_80_LISTENER=$(docker exec platform-ui netstat -tuln 2>/dev/null | grep ":80 " || true)

if [ "${UI_HTTPS}" = "200" ] && [ -z "${PORT_80_LISTENER}" ]; then
    pass ${TEST_NUM} "${TEST_DESC} (HTTP ${UI_HTTPS}, Port 80 absent)"
else
    fail ${TEST_NUM} "${TEST_DESC}" "UI HTTPS status: ${UI_HTTPS}, port 80 listener: ${PORT_80_LISTENER}"
fi

# ------------------------------------------------------------------------------
# Test 17: UI Reverse-Proxy Routing to Core API
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
# Test 18: Graceful Shutdown (dev-down) & Restart Data Persistence
# ------------------------------------------------------------------------------
TEST_NUM=18
TEST_DESC="dev-down & dev-up Lifecycle with Data Persistence"
echo "Testing persistence across restart..."
# Write marker row to Postgres
docker exec platform-postgres psql -U platform_app -d platform -c "INSERT INTO platform.tenant (tenant_id, name) VALUES ('restart-test-tenant', 'Restart Test') ON CONFLICT DO NOTHING;" > /dev/null 2>&1
# Write marker key to Valkey
docker exec platform-valkey valkey-cli -a "${VALKEY_PASSWORD:-platform_dev_valkey_password_change_in_prod}" set restart_persist_key "preserved_across_down" > /dev/null 2>&1

# Stop and restart
docker compose down > /dev/null 2>&1
docker compose up -d > /dev/null 2>&1

# Wait for healthy
sleep 15

# Verify marker row and key
PG_RESTART_VAL=$(docker exec platform-postgres psql -U platform_app -d platform -t -A -c "SELECT count(*) FROM platform.tenant WHERE tenant_id='restart-test-tenant';" 2>/dev/null || echo "0")
VALKEY_RESTART_VAL=$(docker exec platform-valkey valkey-cli -a "${VALKEY_PASSWORD:-platform_dev_valkey_password_change_in_prod}" get restart_persist_key 2>/dev/null || echo "")

if [ "${PG_RESTART_VAL}" = "1" ] && [ "${VALKEY_RESTART_VAL}" = "preserved_across_down" ]; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "PG count: ${PG_RESTART_VAL}, Valkey key: ${VALKEY_RESTART_VAL}"
fi

# ------------------------------------------------------------------------------
# Test 19: Structured JSON Logging Verification
# ------------------------------------------------------------------------------
TEST_NUM=19
TEST_DESC="Structured JSON Logging Verification Across Components"
CORE_LOG=$(docker logs --tail 20 platform-core 2>/dev/null | grep '^{' | head -n 1 || true)
SENSOR_LOG=$(docker logs --tail 20 platform-sensor-net 2>/dev/null | grep '^{' | head -n 1 || true)
ANALYTICS_LOG=$(docker logs --tail 20 platform-analytics 2>/dev/null | grep '^{' | head -n 1 || true)

LOGS_VALID=true
if ! echo "${CORE_LOG}" | grep -q '"service"'; then LOGS_VALID=false; fi
if ! echo "${SENSOR_LOG}" | grep -q '"service"'; then LOGS_VALID=false; fi
if ! echo "${ANALYTICS_LOG}" | grep -q '"service"'; then LOGS_VALID=false; fi

if [ "${LOGS_VALID}" = "true" ]; then
    pass ${TEST_NUM} "${TEST_DESC}"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Structured log check failed. Core: ${CORE_LOG}, Sensor: ${SENSOR_LOG}, Analytics: ${ANALYTICS_LOG}"
fi

# ------------------------------------------------------------------------------
# Test 20: Isolation Verification (Zero Foreign Container Interference & Zero Host Runtimes)
# ------------------------------------------------------------------------------
TEST_NUM=20
TEST_DESC="Isolation Verification (Foreign Containers Untouched, Zero Host Project Runtimes)"
# Check foreign container screensmart status
FOREIGN_RUNNING=$(docker ps --filter "name=screensmart" --format '{{.Names}}' | wc -l)
# Verify zero host project-specific runtimes installed in PATH
HOST_RUNTIMES=$(which java go node npm mvn clickhouse-server valkey-server 2>/dev/null || true)

if [ "${FOREIGN_RUNNING}" -gt 0 ] && [ -z "${HOST_RUNTIMES}" ]; then
    pass ${TEST_NUM} "${TEST_DESC} (${FOREIGN_RUNNING} foreign containers intact, zero host runtimes)"
else
    fail ${TEST_NUM} "${TEST_DESC}" "Foreign containers: ${FOREIGN_RUNNING}, Host runtimes: ${HOST_RUNTIMES}"
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
