#!/usr/bin/env bash
# Day-02 Core PostgreSQL integration gate (Docker-in-Docker / docker.sock required).
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo "========================================================================"
echo "Running Day-02 Core PostgreSQL Integration Gate"
echo "========================================================================"

LOG_FILE=$(mktemp /tmp/day02-pg-tests-XXXXXX.log)
trap 'rm -f "${LOG_FILE}"' EXIT

docker run --rm \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "${ROOT_DIR}/core:/app" -w /app \
  -v "${HOME}/.m2:/root/.m2" \
  --network host \
  maven:3.9.9-eclipse-temurin-21-alpine \
  sh -c 'apk add --no-cache docker-cli >/dev/null && mvn -B -Dgroups=postgres -DfailIfNoSpecifiedTests=true test' | tee "${LOG_FILE}"

SUMMARY_LINE=$(grep -E "Tests run: [0-9]+, Failures: [0-9]+, Errors: [0-9]+, Skipped: [0-9]+" "${LOG_FILE}" | tail -n 1 || true)

if [ -z "${SUMMARY_LINE}" ]; then
  echo "[ERROR] Could not find test execution summary in Maven output!"
  exit 1
fi

TESTS_RUN=$(echo "${SUMMARY_LINE}" | sed -E 's/.*Tests run: ([0-9]+),.*/\1/')
FAILURES=$(echo "${SUMMARY_LINE}" | sed -E 's/.*Failures: ([0-9]+),.*/\1/')
ERRORS=$(echo "${SUMMARY_LINE}" | sed -E 's/.*Errors: ([0-9]+),.*/\1/')
SKIPPED=$(echo "${SUMMARY_LINE}" | sed -E 's/.*Skipped: ([0-9]+).*/\1/')

echo "------------------------------------------------------------------------"
echo "Execution Verification: Tests run=${TESTS_RUN}, Failures=${FAILURES}, Errors=${ERRORS}, Skipped=${SKIPPED}"

if [ "${TESTS_RUN}" -le 0 ]; then
  echo "[ERROR] Zero mandatory PostgreSQL tests were executed!"
  exit 1
fi

if [ "${SKIPPED}" -ne 0 ]; then
  echo "[ERROR] Mandatory PostgreSQL tests were skipped (${SKIPPED} skipped)! A skipped mandatory gate is NOT PASS."
  exit 1
fi

if [ "${FAILURES}" -ne 0 ] || [ "${ERRORS}" -ne 0 ]; then
  echo "[ERROR] PostgreSQL tests failed (${FAILURES} failures, ${ERRORS} errors)!"
  exit 1
fi

echo "[PASS] Verified: ${TESTS_RUN} mandatory PostgreSQL integration tests executed (0 failures, 0 errors, 0 skipped)."
echo "Day-02 Core PostgreSQL integration tests PASSED"
