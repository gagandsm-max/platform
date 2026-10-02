#!/usr/bin/env bash
# ==============================================================================
# BUILD-005 Slice 5 — Sensor cross-runtime certification evidence (bounded)
#
# Path-constrained writer scope: sensor/ + tests/ only.
# Certifies Sensor alignment to the frozen Slice-5 safe_context bar and
# duplicate-key / JSON-injection evidence. Does not certify BUILD-005 closure.
# ==============================================================================
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GO_TEST_TMPFS="/tmp:rw,exec,size=1073741824,mode=1777"

SLICE5_TESTS=(
  TestSafeContextRejectsSecretsCredentialsAndUnboundedShape
  TestSafeContextRejectsEmbeddedAuthorizationAndMixedCasePEM
  TestSafeContextOmitsNonFiniteFloats
  TestEmittedStructuredLogHasNoDuplicateTopLevelKeys
  TestMessageJSONEscapingPreventsLogInjection
  TestCanonicalStructuredLogEnvelope
  TestComponentIdentityCannotBeOverridden
  TestInvalidCorrelationIDIsOmitted
)

JOINED="$(IFS='|'; echo "${SLICE5_TESTS[*]}")"

echo "========================================================================"
echo "BUILD-005 Slice 5 Sensor certification (supportability package)"
echo "========================================================================"

docker run --rm \
  --tmpfs "${GO_TEST_TMPFS}" \
  -v "${ROOT_DIR}/sensor:/app/sensor:ro" \
  -w /app/sensor \
  golang:1.27.1-alpine3.24 \
  go test -v -count=1 "./internal/supportability/" -run "^(${JOINED})$"

# Genuine request/response correlation on existing Core client paths (no fake
# Agent→Sensor→Core telemetry lineage).
docker run --rm \
  --tmpfs "${GO_TEST_TMPFS}" \
  -v "${ROOT_DIR}/sensor:/app/sensor:ro" \
  -w /app/sensor \
  golang:1.27.1-alpine3.24 \
  go test -v -count=1 "./internal/coreclient/" -run 'Correlation|Heartbeat|UploadAgentTelemetry'

echo "========================================================================"
echo "BUILD-005 Slice 5 Sensor certification evidence PASSED"
echo "========================================================================"
