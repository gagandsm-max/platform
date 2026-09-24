#!/usr/bin/env bash
# ==============================================================================
# Platform UI Runtime Resilience Test (Issue #100)
# Verifies UI availability when Core DNS/upstream is absent, late, or untrusted
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

TEST_NET="ui-f2-test-net-$$"
UI_CONTAINER="ui-f2-test-ui-$$"
TRUSTED_CORE_CONTAINER="ui-f2-trusted-core-$$"
UNTRUSTED_CORE_CONTAINER="ui-f2-untrusted-core-$$"
TEST_PORT="${PLATFORM_UI_TEST_PORT:-23099}"

TMP_DIR="$(mktemp -d)"

cleanup() {
    echo "==> Cleaning up test containers and networks..."
    docker rm -f "${UI_CONTAINER}" "${TRUSTED_CORE_CONTAINER}" "${UNTRUSTED_CORE_CONTAINER}" >/dev/null 2>&1 || true
    docker network rm "${TEST_NET}" >/dev/null 2>&1 || true
    rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

echo "========================================================================"
echo "Starting UI Core-Unavailable Runtime Resilience Regression Suite (#100)"
echo "Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
echo "========================================================================"

# Step 0: Prepare network, certs, and fresh UI image
echo "--> [0/5] Creating isolated Docker network ${TEST_NET} and building platform-ui:dev..."
docker network create "${TEST_NET}" >/dev/null

if [ ! -f "deploy/docker/certs/platform-dev.crt" ]; then
    echo "--> Dev certs missing, generating via make certs..."
    make certs
fi

docker build -q -t platform-ui:dev "${ROOT_DIR}/ui" >/dev/null

# Step A: Core absent at UI startup
echo "--> [A/5] Testing Matrix A: Core absent at UI startup..."
docker run -d \
    --name "${UI_CONTAINER}" \
    --network "${TEST_NET}" \
    -v "${ROOT_DIR}/deploy/docker/certs:/etc/nginx/certs:ro" \
    -p "127.0.0.1:${TEST_PORT}:443" \
    platform-ui:dev >/dev/null

# Verify UI stays running
if ! docker ps --format '{{.Names}}' | grep -q "^${UI_CONTAINER}$"; then
    echo "[FAIL] Matrix A: UI container exited on startup when Core DNS was absent!"
    echo "Logs:"
    docker logs "${UI_CONTAINER}" || true
    exit 1
fi

# Wait for Nginx container readiness (up to 10s)
SHELL_CODE="000"
for i in $(seq 1 10); do
    SHELL_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" "https://127.0.0.1:${TEST_PORT}/" || echo "000")
    if [ "${SHELL_CODE}" = "200" ]; then
        break
    fi
    sleep 1
done

if [ "${SHELL_CODE}" != "200" ]; then
    echo "[FAIL] Matrix A: Static GET / returned ${SHELL_CODE}, expected 200"
    docker logs "${UI_CONTAINER}" || true
    exit 1
fi

# Verify static asset loading (index.html contains root div)
SHELL_BODY=$(curl -k -s "https://127.0.0.1:${TEST_PORT}/")
if ! echo "${SHELL_BODY}" | grep -q '<div id="root"></div>'; then
    echo "[FAIL] Matrix A: Static index.html root div missing"
    exit 1
fi

# Verify controlled API gateway failure (502 or 504)
API_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" "https://127.0.0.1:${TEST_PORT}/api/v1/version" || echo "000")
if [ "${API_CODE}" != "502" ] && [ "${API_CODE}" != "504" ]; then
    echo "[FAIL] Matrix A: /api/v1/version returned ${API_CODE}, expected 502/504 controlled failure"
    exit 1
fi

# Verify no port 80 listener inside UI container
PORT_80=$(docker exec "${UI_CONTAINER}" netstat -tuln 2>/dev/null | grep ":80 " || true)
if [ -n "${PORT_80}" ]; then
    echo "[FAIL] Matrix A: Forbidden TCP/80 listener found: ${PORT_80}"
    exit 1
fi

echo "[PASS] Matrix A: UI started, served HTTPS 200 SPA shell, and returned controlled ${API_CODE} API error when Core was absent."

# Step B: Core appears after UI startup (without UI restart)
echo "--> [B/5] Testing Matrix B: Core late arrival (without UI container restart)..."

# Create Nginx config for trusted Core stub
cat <<EOF > "${TMP_DIR}/trusted_core.conf"
events { worker_connections 1024; }
http {
    server {
        listen 8443 ssl;
        server_name core localhost;
        ssl_certificate     /etc/nginx/certs/core.crt;
        ssl_certificate_key /etc/nginx/certs/core.key;

        location /api/v1/version {
            default_type application/json;
            return 200 '{"product":"Platform","status":"UP","version":"1.0.0-stub"}';
        }
        location /actuator/health {
            default_type application/json;
            return 200 '{"status":"UP"}';
        }
    }
}
EOF

docker run -d \
    --name "${TRUSTED_CORE_CONTAINER}" \
    --network "${TEST_NET}" \
    --network-alias core \
    -v "${TMP_DIR}/trusted_core.conf:/etc/nginx/nginx.conf:ro" \
    -v "${ROOT_DIR}/deploy/docker/certs:/etc/nginx/certs:ro" \
    nginx:1.27-alpine >/dev/null

# Wait for DNS propagation / Nginx resolver window (up to 12s for valid=10s)
RECOVERED=false
RECOVERED_BODY=""
for i in $(seq 1 15); do
    RECOVERED_BODY=$(curl -k -s "https://127.0.0.1:${TEST_PORT}/api/v1/version" || echo "")
    if echo "${RECOVERED_BODY}" | grep -q '"product":"Platform"'; then
        RECOVERED=true
        break
    fi
    sleep 1
done

if [ "${RECOVERED}" = "true" ]; then
    echo "[PASS] Matrix B: Dynamic proxying recovered automatically without UI restart. Response: ${RECOVERED_BODY}"
else
    echo "[FAIL] Matrix B: Dynamic proxying failed to recover after Core started. Response: ${RECOVERED_BODY}"
    docker logs "${UI_CONTAINER}" || true
    exit 1
fi

# Step C: Core disappears again
echo "--> [C/5] Testing Matrix C: Core disappearance..."
docker rm -f "${TRUSTED_CORE_CONTAINER}" >/dev/null

# Allow DNS record caching/expiry
DISAPPEARED=false
API_AFTER_DISAPPEAR="000"
for i in $(seq 1 15); do
    API_AFTER_DISAPPEAR=$(curl -k -s -o /dev/null -w "%{http_code}" "https://127.0.0.1:${TEST_PORT}/api/v1/version" || echo "000")
    if [ "${API_AFTER_DISAPPEAR}" = "502" ] || [ "${API_AFTER_DISAPPEAR}" = "504" ]; then
        DISAPPEARED=true
        break
    fi
    sleep 1
done

SHELL_AFTER_DISAPPEAR=$(curl -k -s -o /dev/null -w "%{http_code}" "https://127.0.0.1:${TEST_PORT}/")

if [ "${DISAPPEARED}" = "true" ] && [ "${SHELL_AFTER_DISAPPEAR}" = "200" ]; then
    echo "[PASS] Matrix C: UI shell remained HTTPS 200, API returned controlled ${API_AFTER_DISAPPEAR} after Core disappeared."
else
    echo "[FAIL] Matrix C: Disappearance check failed. Shell: ${SHELL_AFTER_DISAPPEAR}, API: ${API_AFTER_DISAPPEAR}"
    exit 1
fi

# Step D: TLS negative test
echo "--> [D/5] Testing Matrix D: Untrusted Core TLS certificate rejection..."

# Generate self-signed untrusted certificate for badcore
openssl req -x509 -nodes -days 1 -newkey rsa:2048 \
    -keyout "${TMP_DIR}/untrusted.key" \
    -out "${TMP_DIR}/untrusted.crt" \
    -subj "/CN=core.untrusted.local" >/dev/null 2>&1

cat <<EOF > "${TMP_DIR}/untrusted_core.conf"
events { worker_connections 1024; }
http {
    server {
        listen 8443 ssl;
        server_name core localhost;
        ssl_certificate     /etc/nginx/certs/untrusted.crt;
        ssl_certificate_key /etc/nginx/certs/untrusted.key;

        location /api/v1/version {
            default_type application/json;
            return 200 '{"product":"RoguePlatform","status":"UP"}';
        }
    }
}
EOF

docker run -d \
    --name "${UNTRUSTED_CORE_CONTAINER}" \
    --network "${TEST_NET}" \
    --network-alias core \
    -v "${TMP_DIR}/untrusted_core.conf:/etc/nginx/nginx.conf:ro" \
    -v "${TMP_DIR}/untrusted.crt:/etc/nginx/certs/untrusted.crt:ro" \
    -v "${TMP_DIR}/untrusted.key:/etc/nginx/certs/untrusted.key:ro" \
    nginx:1.27-alpine >/dev/null

sleep 2

# Verify proxy connection rejected (502) due to TLS verification failure
UNTRUSTED_API_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" "https://127.0.0.1:${TEST_PORT}/api/v1/version" || echo "000")
SHELL_UNTRUSTED_CODE=$(curl -k -s -o /dev/null -w "%{http_code}" "https://127.0.0.1:${TEST_PORT}/")

if [ "${UNTRUSTED_API_CODE}" = "502" ] && [ "${SHELL_UNTRUSTED_CODE}" = "200" ]; then
    echo "[PASS] Matrix D: Untrusted Core TLS certificate rejected (HTTP ${UNTRUSTED_API_CODE}), UI shell remains HTTPS 200."
else
    echo "[FAIL] Matrix D: TLS negative test failed. API code: ${UNTRUSTED_API_CODE}, Shell code: ${SHELL_UNTRUSTED_CODE}"
    docker logs "${UI_CONTAINER}" || true
    exit 1
fi

docker rm -f "${UNTRUSTED_CORE_CONTAINER}" >/dev/null

# Step E: Security headers check
echo "--> [E/5] Testing Matrix E: Security headers and HTTPS-only enforcement..."
HEADERS=$(curl -k -s -I "https://127.0.0.1:${TEST_PORT}/")
if echo "${HEADERS}" | grep -qi "X-Frame-Options: DENY" && \
   echo "${HEADERS}" | grep -qi "Strict-Transport-Security" && \
   echo "${HEADERS}" | grep -qi "X-Content-Type-Options: nosniff"; then
    echo "[PASS] Matrix E: All security headers verified."
else
    echo "[FAIL] Matrix E: Missing required security headers in response:"
    echo "${HEADERS}"
    exit 1
fi

echo "========================================================================"
echo "ALL MATRICES (A, B, C, D, E) PASSED SUCCESSFULLY FOR ISSUE #100"
echo "========================================================================"
