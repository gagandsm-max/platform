#!/usr/bin/env bash
# Day-02 Core PostgreSQL integration gate (Docker-in-Docker / docker.sock required).
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "${ROOT_DIR}/core:/app" -w /app \
  -v "${HOME}/.m2:/root/.m2" \
  --network host \
  maven:3.9.9-eclipse-temurin-21-alpine \
  sh -c 'apk add --no-cache docker-cli >/dev/null && mvn -q -Dtest=MigrationIntegrationTest,TenantIsolationIntegrationTest test'
echo "Day-02 Core PostgreSQL integration tests PASSED"
