#!/usr/bin/env bash
# ==============================================================================
# Platform Containerized Unit Test Runner
# Runs inside Docker build containers (zero host runtimes required)
# ==============================================================================
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

# Sensor spool tests intentionally enforce the fixed 80% storage-protection guard.
# A GitHub-hosted runner's overlay filesystem can already be above that watermark,
# which makes otherwise deterministic unit tests depend on runner image pressure.
# Give Go tests a clean bounded executable tmpfs for t.TempDir()/go-build without
# changing product storage-protection behavior.
GO_TEST_TMPFS="/tmp:rw,exec,size=1073741824,mode=1777"

echo "========================================================================"
echo "Running Platform Containerized Unit Tests"
echo "========================================================================"

echo "--> [1/3] Running Go Unit Tests (Sensor & Agent)..."
docker run --rm \
    --tmpfs "${GO_TEST_TMPFS}" \
    -v "${ROOT_DIR}/sensor:/app/sensor:ro" \
    -w /app/sensor \
    golang:1.27.1-alpine3.24 \
    go test -v ./...

docker run --rm \
    --tmpfs "${GO_TEST_TMPFS}" \
    -v "${ROOT_DIR}/agent:/app/agent:ro" \
    -v "${ROOT_DIR}/deploy:/deploy:ro" \
    -w /app/agent \
    golang:1.27.1-alpine3.24 \
    go test -v ./...

# Cross-module Agent↔Sensor receiver contract (build tag pipeline).
# Keep production sensor/go.mod free of platform/agent replace (Docker build isolation).
echo "--> [1b/3] Running Agent receiver pipeline contract tests (-tags pipeline)..."
docker run --rm \
    --tmpfs "${GO_TEST_TMPFS}" \
    -v "${ROOT_DIR}/sensor:/src/sensor:ro" \
    -v "${ROOT_DIR}/agent:/src/agent:ro" \
    -w /build \
    golang:1.27.1-alpine3.24 \
    sh -c 'cp -a /src/sensor /build/sensor && cd /build/sensor && go mod edit -require=platform/agent@v0.0.0 -replace=platform/agent=/src/agent && go mod tidy && go test -tags=pipeline -v -count=1 ./internal/agentreceiver/'

echo "--> [2/3] Running Java Core Unit Tests..."
docker run --rm \
    -v "${ROOT_DIR}/core:/src:ro" \
    -v "${HOME}/.m2:/root/.m2" \
    maven:3.9.9-eclipse-temurin-21-alpine \
    sh -c "cp -r /src /build && cd /build && mvn test -B -DexcludedGroups=postgres"

echo "--> [3/4] Running UI Typecheck..."
docker run --rm \
    -v "${ROOT_DIR}/ui:/src:ro" \
    node:20-alpine \
    sh -c "cp -r /src /app && cd /app && npm ci --silent && npx tsc --noEmit"

echo "--> [4/4] Running UI Core-Unavailable Resilience Test..."
bash "${ROOT_DIR}/tests/test-ui-core-unavailable-resilience.sh"

echo "========================================================================"
echo "ALL CONTAINERIZED UNIT TESTS PASSED SUCCESSFULLY"
echo "========================================================================"
