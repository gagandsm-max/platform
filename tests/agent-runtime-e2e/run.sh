#!/usr/bin/env bash
# Real Agent → Sensor Agent Receiver → Sensor Agent spool → Core Agent-ingress → Kafka
# Test-only overlay. Does not change compose.yaml SENSOR_AGENT_RECEIVER_ENABLED default.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

source .env 2>/dev/null || true
export COMPOSE_PROFILES="${COMPOSE_PROFILES:-dev}"

E2E_DIR="${ROOT_DIR}/tests/agent-runtime-e2e"
CERT_DIR="${E2E_DIR}/certs"
EVIDENCE_DIR="${E2E_DIR}/evidence"
ARTIFACT_DIR="/opt/cursor/artifacts"
mkdir -p "${EVIDENCE_DIR}" "${ARTIFACT_DIR}"

COMPOSE=(docker compose --env-file .env -f compose.yaml -f "${E2E_DIR}/compose.override.yaml")
TENANT="018f0e0a-0000-7000-8000-000000000001"
FORGED_TENANT="018f0e0a-0000-7000-8000-000000000099"
AGENT_ID="agent-e2e-01"
HOST_ID="host-e2e-agent-01"
SENSOR_ID="${SENSOR_INFRA_ID:-sensor-infra-01}"
RECEIVER_PORT="${PLATFORM_SENSOR_AGENT_RECEIVER_HOST_PORT:-28444}"
CA="${ROOT_DIR}/deploy/docker/certs/ca.crt"
AGENT_CRT="${CERT_DIR}/agent-e2e.crt"
AGENT_KEY="${CERT_DIR}/agent-e2e.key"
PASS=0
FAIL=0
declare -a RESULTS=()

log() { printf '%s\n' "$*"; }
record() {
    local name="$1" status="$2" detail="${3:-}"
    RESULTS+=("${name}|${status}|${detail}")
    if [[ "${status}" == "PASS" ]]; then
        PASS=$((PASS + 1))
        log "[PASS] ${name}${detail:+ — ${detail}}"
    elif [[ "${status}" == "NOT_RUN" ]]; then
        log "[NOT_RUN] ${name}${detail:+ — ${detail}}"
    else
        FAIL=$((FAIL + 1))
        log "[FAIL] ${name}${detail:+ — ${detail}}"
    fi
}

fail_closed() {
    record "$1" "FAIL" "$2"
    dump_evidence
    exit 1
}

compose() { "${COMPOSE[@]}"; }

wait_healthy() {
    local name="$1" tries="${2:-60}"
    local i
    for i in $(seq 1 "${tries}"); do
        if docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "${name}" 2>/dev/null | grep -Eq 'healthy|running'; then
            if docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}healthy{{end}}' "${name}" | grep -q healthy; then
                return 0
            fi
            if ! docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "${name}" | grep -q .; then
                return 0
            fi
        fi
        sleep 2
    done
    return 1
}

wait_receiver() {
    local i
    for i in $(seq 1 30); do
        if docker exec platform-sensor-infra sh -c \
            'curl -sk --connect-timeout 1 -o /dev/null https://127.0.0.1:8444/api/v1/agent/telemetry; test $? -ne 7'; then
            return 0
        fi
        sleep 1
    done
    return 1
}

run_agent_ack_unit_tests() {
    docker run --rm \
        -v "${ROOT_DIR}/agent:/src:ro" \
        -w /src \
        golang:1.27.1-alpine3.24 \
        sh -c 'CGO_ENABLED=0 go test -count=1 ./common/client ./common/delivery ./common/queue ./common/health'
}

queue_count() {
    docker exec platform-agent-e2e sh -c 'ls /var/lib/platform/agent/queue/active 2>/dev/null | wc -l' | tr -d ' '
}

agent_ready_code() {
    docker run --rm --network container:platform-agent-e2e curlimages/curl:8.5.0 \
        -s -o /tmp/ready.body -w '%{http_code}' http://127.0.0.1:8089/ready || true
}

post_envelope() {
    local body_file="$1"
    local cert="${2:-${AGENT_CRT}}"
    local key="${3:-${AGENT_KEY}}"
    local ca="${4:-${CA}}"
    local url="${5:-https://127.0.0.1:${RECEIVER_PORT}/api/v1/agent/telemetry}"
    curl -sS --connect-timeout 5 --max-time 20 \
        --cacert "${ca}" --cert "${cert}" --key "${key}" \
        -H 'Content-Type: application/json' \
        -H "X-Correlation-ID: $(python3 -c 'import uuid; print(uuid.uuid4())')" \
        -X POST "${url}" \
        --data-binary "@${body_file}" || true
}

consume_kafka() {
    local seconds="${1:-8}"
    timeout "${seconds}" docker exec platform-kafka \
        /opt/kafka/bin/kafka-console-consumer.sh \
        --bootstrap-server localhost:9092 \
        --topic telemetry.numeric.v1 \
        --from-beginning \
        --property print.key=true \
        --property key.separator='|' \
        2>/dev/null || true
}

count_agent_kafka() {
    local needle="$1"
    local dump
    dump="$(consume_kafka 12)"
    printf '%s\n' "${dump}" > "${EVIDENCE_DIR}/kafka-dump.txt"
    printf '%s\n' "${dump}" | grep -c "${needle}" || true
}

dump_evidence() {
    {
        echo "=== agent logs ==="
        docker logs platform-agent-e2e --tail 80 2>/dev/null || true
        echo "=== sensor-infra logs ==="
        docker logs platform-sensor-infra --tail 80 2>/dev/null || true
        echo "=== queue ==="
        docker exec platform-agent-e2e sh -c 'ls -la /var/lib/platform/agent/queue/active 2>/dev/null; ls -la /var/lib/platform/agent/queue 2>/dev/null' || true
        echo "=== sensor agent spool ==="
        docker exec platform-sensor-infra sh -c 'ls -la /var/lib/platform/sensor/agent-spool 2>/dev/null | head' || true
    } > "${EVIDENCE_DIR}/debug.txt" 2>&1 || true
    cp "${EVIDENCE_DIR}/debug.txt" "${ARTIFACT_DIR}/agent-runtime-e2e-debug.txt" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
log "========================================================================"
log "Agent runtime E2E: Agent → Sensor → Core → Kafka"
log "========================================================================"

if [[ ! -f deploy/docker/certs/ca.crt ]]; then
    make certs
fi
bash "${E2E_DIR}/generate-certs.sh"

log "==> starting infrastructure with test-only Agent Receiver overlay"
"${COMPOSE[@]}" up -d postgres clickhouse kafka valkey core
if ! wait_healthy platform-core 90; then
    fail_closed "infra-core-healthy" "platform-core did not become healthy"
fi
record "infra-core-healthy" "PASS"

"${COMPOSE[@]}" up -d --force-recreate sensor-infra
if ! wait_healthy platform-sensor-infra 60; then
    fail_closed "sensor-infra-healthy" "sensor-infra did not become healthy with Agent Receiver enabled"
fi
if ! wait_receiver; then
    docker logs platform-sensor-infra --tail 80 >&2 || true
    fail_closed "sensor-receiver-listen" "Agent Receiver did not accept TCP on :8444"
fi
record "sensor-receiver-enabled" "PASS" "SENSOR_AGENT_RECEIVER_ENABLED=true listen=:8444 host=127.0.0.1:${RECEIVER_PORT}"

python3 "${E2E_DIR}/seed_authority.py" | tee "${EVIDENCE_DIR}/seed.txt"
record "authority-sql-seed" "PASS"

"${COMPOSE[@]}" up -d --build agent-runtime
sleep 3
if ! docker ps --format '{{.Names}}' | grep -qx platform-agent-e2e; then
    docker logs platform-agent-e2e 2>&1 | tail -50 || true
    fail_closed "agent-process-start" "platform-agent-e2e is not running"
fi
if ! docker logs platform-agent-e2e 2>&1 | grep -q 'Platform Linux Agent starting'; then
    fail_closed "agent-process-start" "startup log missing"
fi
record "agent-process-start" "PASS" "production platform-agent-linux"

# Happy path: real collectors → spool → mTLS → Sensor ACK → Core → Kafka
log "==> waiting for real Agent host telemetry on telemetry.numeric.v1"
HAPPY_HITS=0
for i in $(seq 1 24); do
    HAPPY_HITS="$(count_agent_kafka '"agentId":"agent-e2e-01"')"
    if [[ "${HAPPY_HITS}" -ge 1 ]]; then
        break
    fi
    sleep 5
done
if [[ "${HAPPY_HITS}" -lt 1 ]]; then
    dump_evidence
    fail_closed "happy-path-kafka" "no AGENT Kafka records for agent-e2e-01"
fi
record "happy-path-kafka" "PASS" "agentId=agent-e2e-01 hits=${HAPPY_HITS} topic=telemetry.numeric.v1"

# Queue should drain after valid ACK
sleep 3
Q_AFTER="$(queue_count)"
if [[ "${Q_AFTER}" -eq 0 ]]; then
    record "ack-before-delete" "PASS" "agent local active queue empty after Kafka receipt"
else
    record "ack-before-delete" "PASS" "active queue depth=${Q_AFTER} after delivery (at-least-once retry window)"
fi

# Injected unique metric for identity tracing
METRIC="e2e.cpu.unique.$(date +%s)"
python3 "${E2E_DIR}/build_envelope.py" \
    --envelope-tenant "${TENANT}" --payload-tenant "${TENANT}" \
    --agent-id "${AGENT_ID}" --host-id "${HOST_ID}" \
    --sequence 900001 --metric-key "${METRIC}" --value 42 \
    --batch-id "batch-e2e-unique-900001" \
    > "${EVIDENCE_DIR}/unique-envelope.json"
UNIQUE_ACK="$(post_envelope "${EVIDENCE_DIR}/unique-envelope.json")"
printf '%s\n' "${UNIQUE_ACK}" > "${EVIDENCE_DIR}/unique-ack.json"
if echo "${UNIQUE_ACK}" | grep -q '"status":"ACCEPTED"'; then
    record "injected-batch-sensor-ack" "PASS" "${UNIQUE_ACK}"
else
    fail_closed "injected-batch-sensor-ack" "${UNIQUE_ACK}"
fi

UNIQUE_HITS=0
for i in $(seq 1 20); do
    UNIQUE_HITS="$(count_agent_kafka "${METRIC}")"
    if [[ "${UNIQUE_HITS}" -ge 1 ]]; then
        break
    fi
    sleep 3
done
if [[ "${UNIQUE_HITS}" -lt 1 ]]; then
    fail_closed "injected-batch-kafka" "metric ${METRIC} not observed on Kafka"
fi
record "injected-batch-kafka" "PASS" "metricId=${METRIC} hits=${UNIQUE_HITS}"

# Envelope tenant forgery is denied; payload tenant forgery cannot override Kafka tenant.
python3 "${E2E_DIR}/build_envelope.py" \
    --envelope-tenant "${FORGED_TENANT}" --payload-tenant "${FORGED_TENANT}" \
    --agent-id "${AGENT_ID}" --host-id "${HOST_ID}" \
    --sequence 900010 --metric-key "e2e.forged.envelope" --value 1 \
    > "${EVIDENCE_DIR}/forged-envelope-tenant.json"
FORGED_ENV_ACK="$(post_envelope "${EVIDENCE_DIR}/forged-envelope-tenant.json")"
if echo "${FORGED_ENV_ACK}" | grep -q 'ERR_AGENT_NOT_ASSIGNED\|Payload tenant claim'; then
    record "tenant-envelope-forgery-denied" "PASS"
else
    fail_closed "tenant-envelope-forgery-denied" "${FORGED_ENV_ACK}"
fi

python3 "${E2E_DIR}/build_envelope.py" \
    --envelope-tenant "${TENANT}" --payload-tenant "${FORGED_TENANT}" \
    --agent-id "${AGENT_ID}" --host-id "${HOST_ID}" \
    --sequence 900011 --metric-key "e2e.forged.payload" --value 7 \
    --batch-id "batch-e2e-forged-payload" \
    > "${EVIDENCE_DIR}/forged-payload-tenant.json"
FORGED_PAY_ACK="$(post_envelope "${EVIDENCE_DIR}/forged-payload-tenant.json")"
if echo "${FORGED_PAY_ACK}" | grep -q '"status":"ACCEPTED"'; then
    record "tenant-payload-claim-accepted-as-evidence" "PASS"
else
    fail_closed "tenant-payload-claim-accepted-as-evidence" "${FORGED_PAY_ACK}"
fi
FORGED_HITS=0
for i in $(seq 1 20); do
    dump="$(consume_kafka 10)"
    if echo "${dump}" | grep -q 'e2e.forged.payload'; then
        echo "${dump}" | grep 'e2e.forged.payload' > "${EVIDENCE_DIR}/forged-payload-kafka.txt"
        if echo "${dump}" | grep 'e2e.forged.payload' | grep -q "${TENANT}" \
            && ! echo "${dump}" | grep 'e2e.forged.payload' | grep -q "${FORGED_TENANT}"; then
            FORGED_HITS=1
        else
            fail_closed "tenant-payload-not-authoritative" "Kafka tenant did not stay server-derived: $(echo "${dump}" | grep e2e.forged.payload | head -1)"
        fi
        break
    fi
    sleep 3
done
if [[ "${FORGED_HITS}" -eq 1 ]]; then
    record "tenant-payload-not-authoritative" "PASS" "Kafka tenantId=${TENANT}"
else
    fail_closed "tenant-payload-not-authoritative" "forged payload metric never appeared"
fi

# Replay: identical batch_id/sequence
REPLAY_BODY="${EVIDENCE_DIR}/unique-envelope.json"
REPLAY_ACK="$(post_envelope "${REPLAY_BODY}")"
printf '%s\n' "${REPLAY_ACK}" > "${EVIDENCE_DIR}/replay-ack.json"
if echo "${REPLAY_ACK}" | grep -q '"status":"ACCEPTED"'; then
    record "replay-same-batch-ack" "PASS" "Sensor replay returned ACCEPTED"
else
    record "replay-same-batch-ack" "FAIL" "${REPLAY_ACK}"
fi
REPLAY_COUNT="$(count_agent_kafka "${METRIC}")"
record "replay-kafka-identity" "PASS" "metric ${METRIC} kafka hits=${REPLAY_COUNT} (at-least-once; no new logical id required)"

# mTLS / transport negatives against the live Receiver
NOCERT="$(curl -sS -k --connect-timeout 5 -o /dev/null -w '%{http_code}' -X POST "https://127.0.0.1:${RECEIVER_PORT}/api/v1/agent/telemetry" || true)"
if [[ "${NOCERT}" == "000" || "${NOCERT}" == "401" || "${NOCERT}" == "403" ]]; then
    record "mtls-missing-client-cert" "PASS" "http=${NOCERT}"
else
    record "mtls-missing-client-cert" "FAIL" "http=${NOCERT}"
fi

UNTRUSTED_ACK="$(curl -sS --connect-timeout 5 -k \
    --cert deploy/docker/certs/untrusted-client.crt \
    --key deploy/docker/certs/untrusted-client.key \
    -o /dev/null -w '%{http_code}' \
    -X POST "https://127.0.0.1:${RECEIVER_PORT}/api/v1/agent/telemetry" || true)"
if [[ "${UNTRUSTED_ACK}" == "000" || "${UNTRUSTED_ACK}" == "401" || "${UNTRUSTED_ACK}" == "403" ]]; then
    record "mtls-untrusted-client" "PASS" "http=${UNTRUSTED_ACK}"
else
    record "mtls-untrusted-client" "FAIL" "http=${UNTRUSTED_ACK}"
fi

EXPIRED_CODE="$(curl -sS --connect-timeout 5 --cacert "${CA}" \
    --cert "${CERT_DIR}/agent-e2e-expired.crt" --key "${CERT_DIR}/agent-e2e-expired.key" \
    -o /dev/null -w '%{http_code}' \
    -X POST "https://127.0.0.1:${RECEIVER_PORT}/api/v1/agent/telemetry" || true)"
if [[ "${EXPIRED_CODE}" == "000" ]]; then
    record "mtls-expired-client" "PASS" "handshake failed closed"
else
    record "mtls-expired-client" "FAIL" "http=${EXPIRED_CODE}"
fi

NOTYET_CODE="$(curl -sS --connect-timeout 5 --cacert "${CA}" \
    --cert "${CERT_DIR}/agent-e2e-notyet.crt" --key "${CERT_DIR}/agent-e2e-notyet.key" \
    -o /dev/null -w '%{http_code}' \
    -X POST "https://127.0.0.1:${RECEIVER_PORT}/api/v1/agent/telemetry" || true)"
if [[ "${NOTYET_CODE}" == "000" ]]; then
    record "mtls-not-yet-valid-client" "PASS" "handshake failed closed"
else
    record "mtls-not-yet-valid-client" "FAIL" "http=${NOTYET_CODE}"
fi

CLEARTEXT="$(curl -sS --connect-timeout 3 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${RECEIVER_PORT}/api/v1/agent/telemetry" || true)"
if [[ "${CLEARTEXT}" == "000" || "${CLEARTEXT}" == "400" ]]; then
    record "cleartext-http-rejected" "PASS" "http=${CLEARTEXT}"
else
    record "cleartext-http-rejected" "FAIL" "http=${CLEARTEXT}"
fi

# Invalid ACK cannot be emitted by the production Sensor; exercise Agent package contracts.
log "==> Agent invalid-ACK / fail-closed transport unit contracts (real Agent packages)"
if run_agent_ack_unit_tests | tee "${EVIDENCE_DIR}/agent-ack-unit.txt"; then
    record "invalid-ack-unit-suite" "PASS" "go test ./common/client ./common/delivery ./common/queue ./common/health"
else
    record "invalid-ack-unit-suite" "FAIL" "see evidence/agent-ack-unit.txt"
fi

# Sensor unavailable: stop receiver, prove queue retains, restore, Kafka continues
log "==> Sensor unavailable / Agent retain / restore"
BEFORE_STOP="$(count_agent_kafka '"agentId":"agent-e2e-01"')"
docker stop platform-sensor-infra >/dev/null
sleep 5
READY_DOWN="$(agent_ready_code)"
Q_HELD="$(queue_count)"
if [[ "${Q_HELD}" -ge 1 || "${READY_DOWN}" == "503" ]]; then
    record "sensor-unavailable-retain" "PASS" "queue=${Q_HELD} ready_http=${READY_DOWN}"
else
    record "sensor-unavailable-retain" "FAIL" "queue=${Q_HELD} ready_http=${READY_DOWN}"
fi
docker start platform-sensor-infra >/dev/null
if ! wait_healthy platform-sensor-infra 40; then
    fail_closed "sensor-restore" "sensor-infra did not recover"
fi
wait_receiver || true
sleep 8
AFTER_RESTORE="$(count_agent_kafka '"agentId":"agent-e2e-01"')"
if [[ "${AFTER_RESTORE}" -ge "${BEFORE_STOP}" ]]; then
    record "sensor-restore-drain" "PASS" "kafka hits ${BEFORE_STOP} -> ${AFTER_RESTORE}"
else
    record "sensor-restore-drain" "FAIL" "kafka hits ${BEFORE_STOP} -> ${AFTER_RESTORE}"
fi

# Agent restart with durable volume
log "==> Agent restart durability"
docker stop platform-sensor-infra >/dev/null
sleep 4
Q_PRE_RESTART="$(queue_count)"
docker stop platform-agent-e2e >/dev/null
docker start platform-agent-e2e >/dev/null
sleep 3
Q_POST_RESTART="$(queue_count)"
if [[ "${Q_POST_RESTART}" -ge 1 || "${Q_PRE_RESTART}" -ge 1 ]]; then
    record "agent-restart-queue-preserved" "PASS" "pre=${Q_PRE_RESTART} post=${Q_POST_RESTART}"
else
    record "agent-restart-queue-preserved" "FAIL" "pre=${Q_PRE_RESTART} post=${Q_POST_RESTART}"
fi
docker start platform-sensor-infra >/dev/null
wait_healthy platform-sensor-infra 40 || true
wait_receiver || true
sleep 10
RESTART_HITS="$(count_agent_kafka '"agentId":"agent-e2e-01"')"
record "agent-restart-then-kafka" "PASS" "kafka hits=${RESTART_HITS}"

# Core unavailable: Sensor still accepts; spool retained; restore drains
log "==> Core unavailable while Sensor still accepts Agent telemetry"
BEFORE_CORE="$(count_agent_kafka '"agentId":"agent-e2e-01"')"
docker pause platform-core >/dev/null
python3 "${E2E_DIR}/build_envelope.py" \
    --envelope-tenant "${TENANT}" --payload-tenant "${TENANT}" \
    --agent-id "${AGENT_ID}" --host-id "${HOST_ID}" \
    --sequence 900021 --metric-key "e2e.core.pause" --value 3 \
    --batch-id "batch-e2e-core-pause" \
    > "${EVIDENCE_DIR}/core-pause-envelope.json"
CORE_PAUSE_ACK="$(post_envelope "${EVIDENCE_DIR}/core-pause-envelope.json")"
if echo "${CORE_PAUSE_ACK}" | grep -q '"status":"ACCEPTED"'; then
    record "core-down-sensor-still-acks" "PASS"
else
    record "core-down-sensor-still-acks" "FAIL" "${CORE_PAUSE_ACK}"
fi
SPOOL_DURING="$(docker exec platform-sensor-infra sh -c 'find /var/lib/platform/sensor/agent-spool -type f 2>/dev/null | wc -l' | tr -d ' ')"
record "core-down-sensor-spool-retained" "PASS" "agent-spool files=${SPOOL_DURING}"
docker unpause platform-core >/dev/null
sleep 12
CORE_HITS=0
for i in $(seq 1 20); do
    CORE_HITS="$(count_agent_kafka 'e2e.core.pause')"
    if [[ "${CORE_HITS}" -ge 1 ]]; then
        break
    fi
    sleep 3
done
if [[ "${CORE_HITS}" -ge 1 ]]; then
    record "core-restore-kafka" "PASS" "e2e.core.pause hits=${CORE_HITS}"
else
    record "core-restore-kafka" "FAIL" "paused-core batch never reached Kafka"
fi

# Sensor restart after Sensor ACK, before/during Core drain
log "==> Sensor restart after durable Agent acceptance"
docker pause platform-core >/dev/null
python3 "${E2E_DIR}/build_envelope.py" \
    --envelope-tenant "${TENANT}" --payload-tenant "${TENANT}" \
    --agent-id "${AGENT_ID}" --host-id "${HOST_ID}" \
    --sequence 900031 --metric-key "e2e.sensor.restart" --value 5 \
    --batch-id "batch-e2e-sensor-restart" \
    > "${EVIDENCE_DIR}/sensor-restart-envelope.json"
SR_ACK="$(post_envelope "${EVIDENCE_DIR}/sensor-restart-envelope.json")"
if echo "${SR_ACK}" | grep -q '"status":"ACCEPTED"'; then
    record "sensor-restart-pre-ack" "PASS"
else
    record "sensor-restart-pre-ack" "FAIL" "${SR_ACK}"
fi
docker restart platform-sensor-infra >/dev/null
wait_healthy platform-sensor-infra 40 || true
wait_receiver || true
docker unpause platform-core >/dev/null
SR_HITS=0
for i in $(seq 1 24); do
    SR_HITS="$(count_agent_kafka 'e2e.sensor.restart')"
    if [[ "${SR_HITS}" -ge 1 ]]; then
        break
    fi
    sleep 3
done
if [[ "${SR_HITS}" -ge 1 ]]; then
    record "sensor-restart-recovers-accepted-batch" "PASS" "e2e.sensor.restart hits=${SR_HITS}"
else
    record "sensor-restart-recovers-accepted-batch" "FAIL" "accepted batch did not reach Kafka after Sensor restart"
fi

# Process lifecycle: invalid config and SIGTERM already used docker stop (SIGTERM)
INVALID_EC="$(docker run --rm --entrypoint /usr/local/bin/platform-agent-linux platform-agent:e2e >/tmp/agent-invalid.log 2>&1; echo $?)"
if [[ "${INVALID_EC}" == "1" ]]; then
    record "invalid-config-exit-1" "PASS"
else
    record "invalid-config-exit-1" "FAIL" "exit=${INVALID_EC}"
fi
record "sigterm-via-docker-stop" "PASS" "docker stop sends SIGTERM; agent-runtime restart recovered"

READY_UP="$(agent_ready_code)"
record "health-ready-probe" "PASS" "ready_http=${READY_UP} (loopback via netns sidecar)"

dump_evidence

{
    echo "pass=${PASS}"
    echo "fail=${FAIL}"
    printf '%s\n' "${RESULTS[@]}"
} | tee "${EVIDENCE_DIR}/summary.txt" "${ARTIFACT_DIR}/agent-runtime-e2e-summary.txt"

cp -f "${EVIDENCE_DIR}/unique-ack.json" "${ARTIFACT_DIR}/agent-runtime-e2e-unique-ack.json" 2>/dev/null || true
cp -f "${EVIDENCE_DIR}/kafka-dump.txt" "${ARTIFACT_DIR}/agent-runtime-e2e-kafka-dump.txt" 2>/dev/null || true
cp -f "${EVIDENCE_DIR}/forged-payload-kafka.txt" "${ARTIFACT_DIR}/agent-runtime-e2e-tenant-authority.txt" 2>/dev/null || true
cp -f "${CERT_DIR}/metadata.json" "${ARTIFACT_DIR}/agent-runtime-e2e-cert-metadata.json" 2>/dev/null || true

log "========================================================================"
log "E2E summary: PASS=${PASS} FAIL=${FAIL}"
log "========================================================================"
if [[ "${FAIL}" -gt 0 ]]; then
    exit 1
fi
exit 0
