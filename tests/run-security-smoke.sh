#!/usr/bin/env bash
# ==============================================================================
# Platform Security Smoke Verification Suite
# Governing Spec: Platform_18_Security_Threat_Model.md
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

source .env 2>/dev/null || true
CORE_PORT="${PLATFORM_CORE_HOST_PORT:-28443}"
UI_PORT="${PLATFORM_UI_HOST_PORT:-23000}"
CERT_DIR="${ROOT_DIR}/deploy/docker/certs"

echo "========================================================================"
echo "Running Platform Security Smoke Suite"
echo "========================================================================"

# 1. Cleartext HTTP rejection on Core port
echo "--> [1/4] Verifying Core cleartext rejection..."
CLEARTEXT_RESP=$(curl -s --connect-timeout 2 "http://127.0.0.1:${CORE_PORT}/api/v1/version" 2>&1 || true)
if echo "${CLEARTEXT_RESP}" | grep -q "requires TLS\|400 Bad Request"; then
    echo "[OK] Core rejects cleartext HTTP with 400 Bad Request."
else
    echo "[ERROR] Core unexpectedly accepted cleartext HTTP: ${CLEARTEXT_RESP}"
    exit 1
fi

# 2. Port 80 absence in UI container
echo "--> [2/4] Verifying UI port 80 listener absence..."
PORT80=$(docker exec platform-ui netstat -tuln 2>/dev/null | grep ":80 " || true)
if [ -z "${PORT80}" ]; then
    echo "[OK] UI has zero port 80 listener (HTTPS-only confirmed)."
else
    echo "[ERROR] UI container is listening on port 80: ${PORT80}"
    exit 1
fi

# 3. mTLS enforcement on Core ingestion
echo "--> [3/4] Verifying mTLS enforcement and untrusted cert rejection..."
NO_CERT=$(curl -k -s -o /dev/null -w "%{http_code}" -X POST "https://127.0.0.1:${CORE_PORT}/api/v1/sensor/heartbeat" 2>/dev/null || echo "000")
if [ "${NO_CERT}" = "401" ]; then
    echo "[OK] Ingestion endpoint rejects missing client cert with HTTP 401."
else
    echo "[ERROR] Expected 401 for unauthenticated request, got: ${NO_CERT}"
    exit 1
fi

UNTRUSTED=$(curl -k -s -o /dev/null -w "%{http_code}" \
    --cert "${CERT_DIR}/untrusted-client.crt" \
    --key "${CERT_DIR}/untrusted-client.key" \
    -X POST "https://127.0.0.1:${CORE_PORT}/api/v1/sensor/heartbeat" 2>/dev/null || true)
if [ "${UNTRUSTED}" = "000" ] || [ "${UNTRUSTED}" = "403" ] || [ -z "${UNTRUSTED}" ]; then
    echo "[OK] Ingestion endpoint rejects untrusted client cert at TLS handshake."
else
    echo "[ERROR] Untrusted client cert was not rejected, status: ${UNTRUSTED}"
    exit 1
fi

# 4. Developer-host isolation policy plus synthetic detector verification.
# GitHub-hosted CI runners intentionally contain preinstalled language runtimes.
# Platform code/tests still run through Docker; therefore CI validates the detector
# itself without misclassifying the runner image as a developer-policy violation.
echo "--> [4/4] Verifying host runtime isolation policy and synthetic negative detector..."
if [ "${PLATFORM_CI:-0}" = "1" ]; then
    echo "[INFO] PLATFORM_CI=1: skipping ephemeral runner PATH policy enforcement; running detector regression only."
else
    bash "${ROOT_DIR}/scripts/check-host-runtime-isolation.sh"
fi
bash "${ROOT_DIR}/scripts/check-host-runtime-isolation.sh" --synthetic-test

echo "========================================================================"
echo "SECURITY SMOKE SUITE PASSED SUCCESSFULLY"
echo "========================================================================"
