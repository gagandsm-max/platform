#!/usr/bin/env bash
# ==============================================================================
# Platform Containerized Unit Test Runner
# Runs inside Docker build containers (zero host runtimes required)
# ==============================================================================
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

echo "========================================================================"
echo "Running Platform Containerized Unit Tests"
echo "========================================================================"

echo "--> [1/3] Running Go Unit Tests (Sensor & Agent)..."
docker run --rm \
    -v "${ROOT_DIR}/sensor:/app/sensor:ro" \
    -w /app/sensor \
    golang:1.27.1-alpine3.24 \
    go test -v ./...

docker run --rm \
    -v "${ROOT_DIR}/agent:/app/agent:ro" \
    -w /app/agent \
    golang:1.27.1-alpine3.24 \
    go test -v ./...

echo "--> [2/3] Running Java Core Unit Tests..."
docker run --rm \
    -v "${ROOT_DIR}/core:/src:ro" \
    -v "${HOME}/.m2:/root/.m2" \
    maven:3.9.9-eclipse-temurin-21-alpine \
    sh -c "cp -r /src /build && cd /build && mvn test -B -DexcludedGroups=postgres"

echo "--> [3/3] Running UI Typecheck..."
docker run --rm \
    -v "${ROOT_DIR}/ui:/src:ro" \
    node:20-alpine \
    sh -c "cp -r /src /app && cd /app && npm ci --silent && npx tsc --noEmit"

echo "========================================================================"
echo "ALL CONTAINERIZED UNIT TESTS PASSED SUCCESSFULLY"
echo "========================================================================"
