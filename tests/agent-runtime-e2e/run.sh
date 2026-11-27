#!/usr/bin/env bash
# Real Agent → Sensor Agent Receiver → Sensor Agent spool → Core Agent-ingress → Kafka
# Test-only overlay. Does not change compose.yaml SENSOR_AGENT_RECEIVER_ENABLED default.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

# Fail closed before contacting Docker: the old fixed-name harness is unsafe.
: "${PLATFORM_E2E_ISOLATED_CONFIG:?Use run-isolated.sh after runtime admission}"
: "${PLATFORM_E2E_PREFIX:?Missing isolated project identity}"
python3 tests/agent-runtime-e2e/isolation.py verify "$PLATFORM_E2E_ISOLATED_CONFIG" "$PLATFORM_E2E_PREFIX"
export COMPOSE_PROFILES="${COMPOSE_PROFILES:-dev}"

E2E_DIR="${ROOT_DIR}/tests/agent-runtime-e2e"
CERT_DIR="${PLATFORM_E2E_CERT_DIR:?}"
EVIDENCE_DIR="${PLATFORM_E2E_RUN_DIR:?}/evidence"
ARTIFACT_DIR="${PLATFORM_E2E_ARTIFACT_DIR:-${EVIDENCE_DIR}/artifacts}"
mkdir -p "${EVIDENCE_DIR}" "${ARTIFACT_DIR}"

COMPOSE=(docker compose -p "$PLATFORM_E2E_PREFIX" -f "$PLATFORM_E2E_ISOLATED_CONFIG")
# Validate this exact requested overlay before test resets/certs or Compose mutation.
bash deploy/docker/kafka/deployment-preflight.sh check "${COMPOSE[@]:2}"
TENANT="018f0e0a-0000-7000-8000-000000000001"
FORGED_TENANT="018f0e0a-0000-7000-8000-000000000099"
AGENT_ID="agent-e2e-01"
HOST_ID="host-e2e-agent-01"
RUN_ID="$(date +%s)"
SENSOR_ID="${SENSOR_INFRA_ID:-sensor-infra-01}"
RECEIVER_PORT="${PLATFORM_SENSOR_AGENT_RECEIVER_HOST_PORT:-28444}"
CA="${PLATFORM_E2E_CA_DIR}/ca.crt"
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
    local name="$1" tries="${2:-60}" state i
    for i in $(seq 1 "$tries"); do
        state=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$name" 2>/dev/null || true)
        [[ "$state" == healthy ]] && return 0
        sleep 2
    done
    return 1
}

wait_receiver() {
    local i code endpoint
    # Docker can assign a different ephemeral host port on restart/start.
    endpoint=$("${COMPOSE[@]}" port sensor-infra 8444) || return 1
    [[ "$endpoint" == 127.0.0.1:* ]] || return 1
    RECEIVER_PORT=${endpoint##*:}
    [[ "$RECEIVER_PORT" =~ ^[0-9]+$ ]] || return 1
    printf '%s %s\n' "$(date -u +%FT%TZ)" "$endpoint" >> "${EVIDENCE_DIR}/receiver-endpoints.txt"
    for i in $(seq 1 30); do
        code=$(curl -sS --connect-timeout 1 --max-time 2 --cacert "$CA" --cert "$AGENT_CRT" --key "$AGENT_KEY" \
            -o /dev/null -w '%{http_code}' "https://127.0.0.1:${RECEIVER_PORT}/api/v1/agent/telemetry" 2>/dev/null || true)
        [[ "$code" == 405 ]] && return 0
        sleep 1
    done
    return 1
}

run_agent_ack_unit_tests() {
    "${COMPOSE[@]}" run --rm --no-deps unit-contracts
}

queue_count() {
    docker exec "${PLATFORM_E2E_PREFIX}-agent-runtime" sh -ec 'test -d /var/lib/platform/agent/queue/active; find /var/lib/platform/agent/queue/active -maxdepth 1 -type f -name "rec_*.json" -print' | wc -l | tr -d ' '
}

agent_ready_code() {
    docker exec "${PLATFORM_E2E_PREFIX}-agent-runtime" sh -c \
        'wget -S -O /dev/null http://127.0.0.1:8089/ready 2>&1 | sed -n "s/.*HTTP\/1.[01] \([0-9]*\).*/\1/p"'
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

kafka_end_offsets() {
    docker exec ${PLATFORM_E2E_PREFIX}-kafka /opt/kafka/bin/kafka-topics.sh \
        --bootstrap-server localhost:9092 \
        --create --if-not-exists \
        --topic telemetry.numeric.v1 \
        --partitions 3 \
        --replication-factor 1 >/dev/null 2>&1 || true
    docker exec ${PLATFORM_E2E_PREFIX}-kafka /opt/kafka/bin/kafka-get-offsets.sh \
        --bootstrap-server localhost:9092 \
        --topic telemetry.numeric.v1 \
        --time -1
}

consume_since_offsets() {
    # The captured log-end offset is the next record offset, inclusive. Reading
    # one earlier would falsely count old telemetry as post-recovery delivery.
    local offset_file="$1"
    local idle_seconds="${2:-6}"
    local idle_ms=$((idle_seconds * 1000))
    local topic part off start
    while IFS=: read -r topic part off; do
        [[ -n "${part}" && -n "${off}" ]] || continue
        start="${off}"
        [[ "$off" =~ ^[0-9]+$ ]] || return 1
        docker exec ${PLATFORM_E2E_PREFIX}-kafka \
            /opt/kafka/bin/kafka-console-consumer.sh \
            --bootstrap-server localhost:9092 \
            --topic "${topic}" \
            --partition "${part}" \
            --offset "${start}" \
            --timeout-ms "${idle_ms}" \
            --property print.key=true \
            --property key.separator='|' \
            2>/dev/null || true
    done < "${offset_file}"
}

count_kafka_since() {
    local needle="$1"
    local offset_file="$2"
    local dump
    dump="$(consume_since_offsets "${offset_file}" 8)"
    printf '%s\n' "${dump}" > "${EVIDENCE_DIR}/kafka-dump.txt"
    printf '%s\n' "${dump}" | grep -c "${needle}" || true
}

dump_evidence() {
    {
        echo "=== agent logs ==="
        docker logs ${PLATFORM_E2E_PREFIX}-agent-runtime --tail 80 2>/dev/null || true
        echo "=== sensor-infra logs ==="
        docker logs ${PLATFORM_E2E_PREFIX}-sensor-infra --tail 120 2>/dev/null || true
        echo "=== queue ==="
        if [[ "${IDENTITY_PROBE_FROZEN:-0}" == 1 ]]; then
            echo "Agent queue exec skipped: frozen identity probe; retained sequence snapshot and container state/logs are the evidence."
        else
            docker exec ${PLATFORM_E2E_PREFIX}-agent-runtime sh -c 'ls -la /var/lib/platform/agent/queue/active 2>/dev/null; ls -la /var/lib/platform/agent/queue 2>/dev/null' || true
        fi
        echo "=== sensor agent spool ==="
        docker exec ${PLATFORM_E2E_PREFIX}-sensor-infra sh -c 'ls -la /var/lib/platform/sensor/agent-spool 2>/dev/null | head -40; echo --- poison ---; ls -la /var/lib/platform/sensor/agent-spool/agent-poison 2>/dev/null | head' || true
        echo "=== core agent-ingress ==="
        docker logs ${PLATFORM_E2E_PREFIX}-core --tail 80 2>/dev/null | grep -E 'agent-telemetry|RECORD_TOO_LARGE|agent-e2e' || true
    } > "${EVIDENCE_DIR}/debug.txt" 2>&1 || true
    cp "${EVIDENCE_DIR}/debug.txt" "${ARTIFACT_DIR}/agent-runtime-e2e-debug.txt" 2>/dev/null || true
}

sample_agent_resources() {
    local phase="$1"
    date -u +%FT%TZ > "${EVIDENCE_DIR}/resources-${phase}.utc"
    docker stats --no-stream --format '{{json .}}' "${PLATFORM_E2E_PREFIX}-agent-runtime" > "${EVIDENCE_DIR}/resources-${phase}.json"
    docker exec "${PLATFORM_E2E_PREFIX}-agent-runtime" sh -c 'cat /proc/1/status /proc/1/io; echo open_fds; ls /proc/1/fd | wc -l; echo queue_bytes; du -sk /var/lib/platform/agent/queue' > "${EVIDENCE_DIR}/resources-${phase}.txt"
}

# ---------------------------------------------------------------------------
log "========================================================================"
log "Agent runtime E2E: Agent → Sensor → Core → Kafka"
log "========================================================================"

# Invalid ACK cannot be emitted by the production Sensor; exercise Agent package contracts.
log "==> Agent invalid-ACK / fail-closed transport unit contracts (real Agent packages)"
if run_agent_ack_unit_tests | tee "${EVIDENCE_DIR}/agent-ack-unit.txt"; then
    record "invalid-ack-unit-suite" "PASS" "go test ./common/client ./common/delivery ./common/queue ./common/health ./common/config"
else
    record "invalid-ack-unit-suite" "FAIL" "see evidence/agent-ack-unit.txt"
fi

log "==> starting infrastructure with test-only Agent Receiver overlay"
bash deploy/docker/kafka/deployment-preflight.sh check "${COMPOSE[@]:2}"
"${COMPOSE[@]}" up -d postgres clickhouse kafka valkey core
if ! wait_healthy ${PLATFORM_E2E_PREFIX}-core 90; then
    fail_closed "infra-core-healthy" "${PLATFORM_E2E_PREFIX}-core did not become healthy"
fi
record "infra-core-healthy" "PASS"

log "==> rebuilding Sensor with Agent Receiver overlay (does not flip compose.yaml default)"
true # Images built from the exact checkout by run-isolated.sh
"${COMPOSE[@]}" up -d --no-build --force-recreate sensor-infra
if ! wait_healthy ${PLATFORM_E2E_PREFIX}-sensor-infra 90; then
    fail_closed "sensor-infra-healthy" "sensor-infra did not become healthy with Agent Receiver enabled"
fi
RECEIVER_PORT=$("${COMPOSE[@]}" port sensor-infra 8444)
RECEIVER_PORT=${RECEIVER_PORT##*:}
if ! wait_receiver; then
    docker logs ${PLATFORM_E2E_PREFIX}-sensor-infra --tail 80 >&2 || true
    fail_closed "sensor-receiver-listen" "Agent Receiver did not accept TCP on :8444"
fi
record "sensor-receiver-enabled" "PASS" "SENSOR_AGENT_RECEIVER_ENABLED=true listen=:8444 host=127.0.0.1:${RECEIVER_PORT}"

python3 "${E2E_DIR}/seed_authority.py" | tee "${EVIDENCE_DIR}/seed.txt"
python3 "${E2E_DIR}/seed_authority.py" --probe | tee "${EVIDENCE_DIR}/seed-probe.txt"
record "authority-sql-seed" "PASS"

log "==> capturing Kafka end offsets before Agent start"
kafka_end_offsets | tee "${EVIDENCE_DIR}/offsets-before-agent.txt"

if [[ "${PLATFORM_E2E_SEQUENCE_DISPOSITION:-0}" == "1" ]]; then
    python3 "${E2E_DIR}/sequence_disposition.py" "$PLATFORM_E2E_PREFIX" "$RECEIVER_PORT" "$EVIDENCE_DIR"
    exit 0
fi
if [[ "${PLATFORM_E2E_REAL_SENSOR_GAP:-0}" == "1" ]]; then
    python3 "${E2E_DIR}/real_sensor_commit_gap.py" "$PLATFORM_E2E_PREFIX" "$RECEIVER_PORT" "$EVIDENCE_DIR"
    exit 0
fi

true # Agent image built by run-isolated.sh
"${COMPOSE[@]}" up -d --force-recreate --no-deps agent-runtime-init
if ! docker wait ${PLATFORM_E2E_PREFIX}-agent-runtime-init >/dev/null; then
    docker logs ${PLATFORM_E2E_PREFIX}-agent-runtime-init 2>&1 || true
    fail_closed "agent-runtime-init" "init container failed"
fi
init_ec="$(docker inspect --format '{{.State.ExitCode}}' ${PLATFORM_E2E_PREFIX}-agent-runtime-init)"
if [[ "${init_ec}" != "0" ]]; then
    docker logs ${PLATFORM_E2E_PREFIX}-agent-runtime-init 2>&1 || true
    fail_closed "agent-runtime-init" "init exit=${init_ec}"
fi
"${COMPOSE[@]}" up -d --no-deps --no-build --force-recreate agent-runtime
sleep 3
if ! docker ps --format '{{.Names}}' | grep -qx ${PLATFORM_E2E_PREFIX}-agent-runtime; then
    docker logs ${PLATFORM_E2E_PREFIX}-agent-runtime 2>&1 | tail -50 || true
    fail_closed "agent-process-start" "${PLATFORM_E2E_PREFIX}-agent-runtime is not running"
fi
if ! docker logs ${PLATFORM_E2E_PREFIX}-agent-runtime 2>&1 | grep -Fq '"event_code":"AGENT_SERVICE_RUNNING"'; then
    fail_closed "agent-process-start" "startup log missing"
fi
record "agent-process-start" "PASS" "production platform-agent-linux"
sample_agent_resources started

# Happy path: real collectors → spool → mTLS → Sensor ACK → Core → Kafka
log "==> waiting for real Agent host telemetry on telemetry.numeric.v1"
HAPPY_HITS=0
for i in $(seq 1 24); do
    HAPPY_HITS="$(count_kafka_since '"agentId":"agent-e2e-01"' "${EVIDENCE_DIR}/offsets-before-agent.txt")"
    if [[ "${HAPPY_HITS}" -ge 1 ]]; then
        break
    fi
    sleep 5
done
if [[ "${HAPPY_HITS}" -lt 1 ]]; then
    dump_evidence
    fail_closed "happy-path-kafka" "no new AGENT Kafka records for agent-e2e-01 since offsets $(tr '\n' ' ' < "${EVIDENCE_DIR}/offsets-before-agent.txt")"
fi
record "happy-path-kafka" "PASS" "agentId=agent-e2e-01 new_hits=${HAPPY_HITS} topic=telemetry.numeric.v1 partitions=$(tr '\n' ',' < "${EVIDENCE_DIR}/offsets-before-agent.txt")"

# Retain and compare one actual production collector observation all the way to
# the canonical ClickHouse view, separately from injected transport fixtures.
cp "${EVIDENCE_DIR}/kafka-dump.txt" "${EVIDENCE_DIR}/real-agent-kafka.txt"
python3 "${E2E_DIR}/readback.py" prepare "${EVIDENCE_DIR}/real-agent-kafka.txt" "${EVIDENCE_DIR}/real-agent-expected.json" > "${EVIDENCE_DIR}/real-agent-query.sql"
REAL_QUERY=$(cat "${EVIDENCE_DIR}/real-agent-query.sql")
for _ in $(seq 1 20); do
    docker exec "${PLATFORM_E2E_PREFIX}-clickhouse" sh -c 'exec clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --query "$1"' sh "$REAL_QUERY" > "${EVIDENCE_DIR}/real-agent-clickhouse.jsonl"
    [[ -s "${EVIDENCE_DIR}/real-agent-clickhouse.jsonl" ]] && break
    sleep 1
done
python3 "${E2E_DIR}/readback.py" verify "${EVIDENCE_DIR}/real-agent-expected.json" "${EVIDENCE_DIR}/real-agent-clickhouse.jsonl" || fail_closed "real-agent-clickhouse-readback" "Canonical stored fields differ"
record "real-agent-clickhouse-readback" "PASS" "exact stored scalar fields at ClickHouse millisecond precision; original Kafka timestamp retained for comparison"

# Queue should drain after valid ACK
sleep 3
Q_AFTER="$(queue_count)"
if [[ "${Q_AFTER}" -eq 0 ]]; then
    record "ack-before-delete" "PASS" "agent local active queue empty after Kafka receipt"
else
    record "ack-before-delete" "FAIL" "active queue depth=${Q_AFTER} after delivery (at-least-once retry window)"
fi

# Pause host collectors so injected batches are not buried under filesystem cardinality
# or Core Kafka 503 head-of-line stalls.
log "==> pausing Agent collectors for injected identity/authority batches"
docker pause ${PLATFORM_E2E_PREFIX}-agent-runtime >/dev/null
sleep 2
# Keep synthetic legacy-stream probes away from the running Agent's native-v2
# custody and sequence space. Both identities have separately seeded credentials.
AGENT_ID=agent-e2e-probe-01
HOST_ID=host-e2e-probe-01
AGENT_CRT="${CERT_DIR}/agent-probe.crt"
AGENT_KEY="${CERT_DIR}/agent-probe.key"

# Injected unique metric for identity tracing
METRIC="e2e.cpu.unique.$(date +%s)"
kafka_end_offsets > "${EVIDENCE_DIR}/offsets-unique.txt"
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

# Unique batch must reach Kafka (Sensor drain) before lost-ACK replay.
UNIQUE_HITS=0
for i in $(seq 1 20); do
    UNIQUE_HITS="$(count_kafka_since "${METRIC}" "${EVIDENCE_DIR}/offsets-unique.txt")"
    if [[ "${UNIQUE_HITS}" -ge 1 ]]; then
        break
    fi
    sleep 3
done
if [[ "${UNIQUE_HITS}" -lt 1 ]]; then
    fail_closed "injected-batch-kafka" "metric ${METRIC} not observed on Kafka"
fi
record "injected-batch-kafka" "PASS" "metricId=${METRIC} hits=${UNIQUE_HITS}"
sleep 5

# Query the canonical FINAL view, checking exact content and source sequence.
# Agent observationId is Kafka-only in the current Core schema; do not claim
# ClickHouse provenance parity beyond these actually stored columns.
verify_clickhouse_unique() {
    local row query
    query="SELECT tenant_id, entity_id, metric_id, sensor_id, toString(ts) AS ts, value, sequence_id, source_protocol FROM platform.metric_numeric WHERE tenant_id='${TENANT}' AND metric_id='${METRIC}' FORMAT JSONEachRow"
    for _ in $(seq 1 20); do
        docker exec "${PLATFORM_E2E_PREFIX}-clickhouse" sh -c 'exec clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --query "$1"' sh "$query" > "${EVIDENCE_DIR}/clickhouse-unique.jsonl"
        if [[ -s "${EVIDENCE_DIR}/clickhouse-unique.jsonl" ]]; then break; fi
        sleep 1
    done
    python3 - "${EVIDENCE_DIR}/clickhouse-unique.jsonl" "$TENANT" "$SENSOR_ID" "$METRIC" <<'PY'
import json, sys
rows = [json.loads(line) for line in open(sys.argv[1]) if line.strip()]
assert len(rows) == 1, rows
r = rows[0]
assert r['tenant_id'] == sys.argv[2] and r['sensor_id'] == sys.argv[3], r
assert r['entity_id'] == '018f0e0a-0000-7000-8000-00000000ae02', r
assert r['metric_id'] == sys.argv[4] and r['value'] == 42, r
assert r['ts'] == '2026-09-12 19:00:00.000', r
assert r['sequence_id'].endswith(':900001:0:' + sys.argv[4]), r
assert r['source_protocol'] == 'AGENT_HOST', r
PY
}
verify_clickhouse_unique || fail_closed "clickhouse-canonical-readback" "Exact metric/tenant/entity/time/sequence comparison failed"
record "clickhouse-canonical-readback" "PASS" "exact canonical fields; Agent observationId remains Kafka-only"

# TEST A — lost ACK after durable accept + drain: identical retry must ACK without a second logical batch.
PRE_REPLAY_HITS="$(count_kafka_since "${METRIC}" "${EVIDENCE_DIR}/offsets-unique.txt")"
REPLAY_BODY="${EVIDENCE_DIR}/unique-envelope.json"
REPLAY_ACK="$(post_envelope "${REPLAY_BODY}")"
printf '%s\n' "${REPLAY_ACK}" > "${EVIDENCE_DIR}/replay-ack.json"
if echo "${REPLAY_ACK}" | grep -q '"status":"ACCEPTED"' && echo "${REPLAY_ACK}" | grep -q '"accepted_through_sequence":900001'; then
    record "lost-ack-after-drain-replay" "PASS" "${REPLAY_ACK}"
else
    fail_closed "lost-ack-after-drain-replay" "${REPLAY_ACK}"
fi
sleep 4
POST_REPLAY_HITS="$(count_kafka_since "${METRIC}" "${EVIDENCE_DIR}/offsets-unique.txt")"
if [[ "${POST_REPLAY_HITS}" -gt "${PRE_REPLAY_HITS}" ]]; then
    fail_closed "lost-ack-after-drain-no-second-logical" "Kafka hits increased from ${PRE_REPLAY_HITS} to ${POST_REPLAY_HITS} after identical replay"
fi
record "lost-ack-after-drain-no-second-logical" "PASS" "metric ${METRIC} kafka hits stayed at ${POST_REPLAY_HITS} (pre=${PRE_REPLAY_HITS})"

# TEST B — same identity after Sensor restart.
log "==> Sensor restart between drain and duplicate retry"
PRE_RESTART_REPLAY_HITS="$(count_kafka_since "${METRIC}" "${EVIDENCE_DIR}/offsets-unique.txt")"
docker restart ${PLATFORM_E2E_PREFIX}-sensor-infra >/dev/null
if ! wait_healthy ${PLATFORM_E2E_PREFIX}-sensor-infra 40; then
    fail_closed "lost-ack-sensor-restart" "sensor-infra did not recover"
fi
wait_receiver || fail_closed "lost-ack-sensor-restart" "receiver not ready after restart"
RESTART_REPLAY_ACK="$(post_envelope "${REPLAY_BODY}")"
printf '%s\n' "${RESTART_REPLAY_ACK}" > "${EVIDENCE_DIR}/replay-after-sensor-restart-ack.json"
if echo "${RESTART_REPLAY_ACK}" | grep -q '"status":"ACCEPTED"' && echo "${RESTART_REPLAY_ACK}" | grep -q '"accepted_through_sequence":900001'; then
    record "lost-ack-after-drain-and-sensor-restart" "PASS" "${RESTART_REPLAY_ACK}"
else
    fail_closed "lost-ack-after-drain-and-sensor-restart" "${RESTART_REPLAY_ACK}"
fi
sleep 4
POST_RESTART_REPLAY_HITS="$(count_kafka_since "${METRIC}" "${EVIDENCE_DIR}/offsets-unique.txt")"
if [[ "${POST_RESTART_REPLAY_HITS}" -gt "${PRE_RESTART_REPLAY_HITS}" ]]; then
    fail_closed "lost-ack-after-restart-no-second-logical" "Kafka hits increased from ${PRE_RESTART_REPLAY_HITS} to ${POST_RESTART_REPLAY_HITS} after restart replay"
fi
record "lost-ack-after-restart-no-second-logical" "PASS" "metric ${METRIC} kafka hits stayed at ${POST_RESTART_REPLAY_HITS} (pre=${PRE_RESTART_REPLAY_HITS})"
verify_clickhouse_unique || fail_closed "clickhouse-replay-identity" "Replay changed canonical identity/content/count"
record "clickhouse-replay-identity" "PASS" "one canonical row after identical replay and Sensor restart"

# TEST C — mutated replay of the accepted sequence/range.
kafka_end_offsets > "${EVIDENCE_DIR}/offsets-before-mutated.txt"
python3 "${E2E_DIR}/build_envelope.py" \
    --envelope-tenant "${TENANT}" --payload-tenant "${TENANT}" \
    --agent-id "${AGENT_ID}" --host-id "${HOST_ID}" \
    --sequence 900001 --metric-key "${METRIC}" --value 99 \
    --batch-id "batch-e2e-unique-900001" \
    > "${EVIDENCE_DIR}/unique-envelope-mutated.json"
MUTATED_ACK="$(post_envelope "${EVIDENCE_DIR}/unique-envelope-mutated.json")"
printf '%s\n' "${MUTATED_ACK}" > "${EVIDENCE_DIR}/replay-mutated-ack.json"
if echo "${MUTATED_ACK}" | grep -q 'ERR_REPLAY_CONFLICT'; then
    record "mutated-replay-conflict" "PASS" "${MUTATED_ACK}"
else
    fail_closed "mutated-replay-conflict" "${MUTATED_ACK}"
fi
MUTATED_KAFKA="$(consume_since_offsets "${EVIDENCE_DIR}/offsets-before-mutated.txt" 6 || true)"
if echo "${MUTATED_KAFKA}" | grep -q '"value":99'; then
    fail_closed "mutated-replay-no-new-downstream" "mutated replay produced a new Kafka payload"
fi
record "mutated-replay-no-new-downstream" "PASS" "no new value=99 event after conflict"

# Envelope tenant forgery is denied; payload tenant forgery cannot override Kafka tenant.
python3 "${E2E_DIR}/build_envelope.py" \
    --envelope-tenant "${FORGED_TENANT}" --payload-tenant "${FORGED_TENANT}" \
    --agent-id "${AGENT_ID}" --host-id "${HOST_ID}" \
    --sequence 900010 --metric-key "e2e.forged.envelope.${RUN_ID}" --value 1 \
    > "${EVIDENCE_DIR}/forged-envelope-tenant.json"
FORGED_ENV_ACK="$(post_envelope "${EVIDENCE_DIR}/forged-envelope-tenant.json")"
if echo "${FORGED_ENV_ACK}" | grep -q 'ERR_AGENT_NOT_ASSIGNED\|Payload tenant claim'; then
    record "tenant-envelope-forgery-denied" "PASS"
else
    fail_closed "tenant-envelope-forgery-denied" "${FORGED_ENV_ACK}"
fi

kafka_end_offsets > "${EVIDENCE_DIR}/offsets-forged.txt"
python3 "${E2E_DIR}/build_envelope.py" \
    --envelope-tenant "${TENANT}" --payload-tenant "${FORGED_TENANT}" \
    --agent-id "${AGENT_ID}" --host-id "${HOST_ID}" \
    --sequence 900011 --metric-key "e2e.forged.payload.${RUN_ID}" --value 7 \
    --batch-id "batch-e2e-forged-payload-${RUN_ID}" \
    > "${EVIDENCE_DIR}/forged-payload-tenant.json"
FORGED_PAY_ACK="$(post_envelope "${EVIDENCE_DIR}/forged-payload-tenant.json")"
if echo "${FORGED_PAY_ACK}" | grep -q '"status":"ACCEPTED"'; then
    record "tenant-payload-claim-accepted-as-evidence" "PASS"
else
    fail_closed "tenant-payload-claim-accepted-as-evidence" "${FORGED_PAY_ACK}"
fi
FORGED_HITS=0
FORGED_METRIC="e2e.forged.payload.${RUN_ID}"
for i in $(seq 1 20); do
    dump="$(consume_since_offsets "${EVIDENCE_DIR}/offsets-forged.txt" 8)"
    if echo "${dump}" | grep -q "${FORGED_METRIC}"; then
        echo "${dump}" | grep "${FORGED_METRIC}" > "${EVIDENCE_DIR}/forged-payload-kafka.txt"
        if echo "${dump}" | grep "${FORGED_METRIC}" | grep -q "${TENANT}" \
            && ! echo "${dump}" | grep "${FORGED_METRIC}" | grep -q "${FORGED_TENANT}"; then
            FORGED_HITS=1
        else
            fail_closed "tenant-payload-not-authoritative" "Kafka tenant did not stay server-derived: $(echo "${dump}" | grep "${FORGED_METRIC}" | head -1)"
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
REPLAY_COUNT="$(count_kafka_since "${METRIC}" "${EVIDENCE_DIR}/offsets-unique.txt")"
if [[ "${REPLAY_COUNT}" -gt "${PRE_REPLAY_HITS}" ]]; then
    fail_closed "replay-kafka-identity" "Kafka hits increased from ${PRE_REPLAY_HITS} to ${REPLAY_COUNT} across replays"
fi
record "replay-kafka-identity" "PASS" "metric ${METRIC} kafka hits=${REPLAY_COUNT} (at-least-once; no new logical id required)"

log "==> resuming Agent collectors for failure-path scenarios"
docker unpause ${PLATFORM_E2E_PREFIX}-agent-runtime >/dev/null || true

# Only a verified server TLS rejection counts. Connection failure, missing
# fixture files or a timeout must not masquerade as certificate rejection.
tls_rejection_probe() {
    local name="$1" code rc
    shift
    set +e
    code=$(curl -sS --connect-timeout 5 --max-time 10 --cacert "$CA" "$@" \
        -o /dev/null -w '%{http_code}' -X POST "https://127.0.0.1:${RECEIVER_PORT}/api/v1/agent/telemetry" \
        2> "${EVIDENCE_DIR}/${name}.stderr")
    rc=$?
    set -e
    printf 'curl_exit=%s http=%s\n' "$rc" "$code" > "${EVIDENCE_DIR}/${name}.result"
    if [[ ( "$rc" == 35 || "$rc" == 56 ) && "$code" == 000 ]] && grep -Eiq 'alert|certificate' "${EVIDENCE_DIR}/${name}.stderr"; then
        record "$name" "PASS" "verified server TLS rejection; curl_exit=$rc"
    else
        record "$name" "FAIL" "curl_exit=$rc http=$code; inspect retained stderr"
    fi
}
tls_rejection_probe mtls-missing-client-cert
tls_rejection_probe mtls-untrusted-client --cert "${PLATFORM_E2E_CA_DIR}/untrusted-client.crt" --key "${PLATFORM_E2E_CA_DIR}/untrusted-client.key"
tls_rejection_probe mtls-expired-client --cert "${CERT_DIR}/agent-e2e-expired.crt" --key "${CERT_DIR}/agent-e2e-expired.key"
tls_rejection_probe mtls-not-yet-valid-client --cert "${CERT_DIR}/agent-e2e-notyet.crt" --key "${CERT_DIR}/agent-e2e-notyet.key"
wait_receiver || fail_closed "mtls-receiver-sanity" "Valid client could not reach the verified TLS server after negative probes"

CLEARTEXT="$(curl -sS --connect-timeout 3 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${RECEIVER_PORT}/api/v1/agent/telemetry" || true)"
if [[ "${CLEARTEXT}" == "000" || "${CLEARTEXT}" == "400" ]]; then
    record "cleartext-http-rejected" "PASS" "http=${CLEARTEXT}"
else
    record "cleartext-http-rejected" "FAIL" "http=${CLEARTEXT}"
fi

# Sensor unavailable: stop receiver, prove queue retains, restore, Kafka continues
log "==> Sensor unavailable / Agent retain / restore"
sample_agent_resources steady
kafka_end_offsets > "${EVIDENCE_DIR}/offsets-sensor-down.txt"
docker stop ${PLATFORM_E2E_PREFIX}-sensor-infra >/dev/null
Q_HELD=0
READY_DOWN=""
HEALTH_DOWN_OK=0
for i in $(seq 1 12); do
    sleep 3
    READY_DOWN="$(agent_ready_code)"
    Q_HELD="$(queue_count)"
    docker exec "${PLATFORM_E2E_PREFIX}-agent-runtime" wget -q -O - http://127.0.0.1:8089/ready > "${EVIDENCE_DIR}/ready-down-${i}.json" || true
    docker exec "${PLATFORM_E2E_PREFIX}-agent-runtime" wget -q -O - http://127.0.0.1:8089/health > "${EVIDENCE_DIR}/health-down-${i}.json" || true
    if python3 "${E2E_DIR}/readback.py" health-outage "${EVIDENCE_DIR}/ready-down-${i}.json" "${EVIDENCE_DIR}/health-down-${i}.json" "${READY_DOWN:-0}" "$Q_HELD"; then
        HEALTH_DOWN_OK=1
        break
    fi
done
if [[ "$HEALTH_DOWN_OK" == 1 ]]; then
    record "sensor-unavailable-retain" "PASS" "queue=${Q_HELD}; HTTP200 DEGRADED/ready=true, failed/down transport, safe durable buffering"
else
    record "sensor-unavailable-retain" "FAIL" "queue=${Q_HELD} ready_http=${READY_DOWN}"
fi
docker start ${PLATFORM_E2E_PREFIX}-sensor-infra >/dev/null
if ! wait_healthy ${PLATFORM_E2E_PREFIX}-sensor-infra 40; then
    fail_closed "sensor-restore" "sensor-infra did not recover"
fi
wait_receiver || true
kafka_end_offsets > "${EVIDENCE_DIR}/offsets-sensor-restore.txt"
AFTER_RESTORE=0
for i in $(seq 1 20); do
    AFTER_RESTORE="$(count_kafka_since '"agentId":"agent-e2e-01"' "${EVIDENCE_DIR}/offsets-sensor-restore.txt")"
    if [[ "${AFTER_RESTORE}" -ge 1 ]]; then
        break
    fi
    sleep 3
done
if [[ "${AFTER_RESTORE}" -ge 1 ]]; then
    record "sensor-restore-drain" "PASS" "new kafka hits after restore=${AFTER_RESTORE}"
else
    record "sensor-restore-drain" "FAIL" "no new agent Kafka records after Sensor restore"
fi

# Agent restart with durable volume
log "==> Agent restart durability"
docker stop ${PLATFORM_E2E_PREFIX}-sensor-infra >/dev/null
kafka_end_offsets > "${EVIDENCE_DIR}/offsets-agent-restart.txt"
for _ in $(seq 1 15); do
    Q_PRE_RESTART="$(queue_count)"
    [[ "$Q_PRE_RESTART" -ge 1 ]] && break
    sleep 2
done
[[ "$Q_PRE_RESTART" -ge 1 ]] || fail_closed "agent-restart-queue-preserved" "No pre-restart durable record"
docker exec "${PLATFORM_E2E_PREFIX}-agent-runtime" sh -c 'sha256sum /var/lib/platform/agent/identity.json /var/lib/platform/agent/queue/active/rec_*.json' > "${EVIDENCE_DIR}/queue-pre-restart.sha256"
docker stop ${PLATFORM_E2E_PREFIX}-agent-runtime >/dev/null
AGENT_STOP_EXIT=$(docker inspect -f '{{.State.ExitCode}}' "${PLATFORM_E2E_PREFIX}-agent-runtime")
docker logs "${PLATFORM_E2E_PREFIX}-agent-runtime" > "${EVIDENCE_DIR}/agent-stop.log" 2>&1
[[ "$AGENT_STOP_EXIT" == 0 ]] || fail_closed "agent-graceful-stop" "exit=$AGENT_STOP_EXIT"
printf '%s\n' "$AGENT_STOP_EXIT" > "${EVIDENCE_DIR}/agent-stop.exit"
docker start ${PLATFORM_E2E_PREFIX}-agent-runtime >/dev/null
sleep 3
Q_POST_RESTART="$(queue_count)"
docker exec "${PLATFORM_E2E_PREFIX}-agent-runtime" sh -c 'sha256sum /var/lib/platform/agent/identity.json /var/lib/platform/agent/queue/active/rec_*.json' > "${EVIDENCE_DIR}/queue-post-restart.sha256"
if python3 - "${EVIDENCE_DIR}/queue-pre-restart.sha256" "${EVIDENCE_DIR}/queue-post-restart.sha256" <<'CHECK_QUEUE'
import sys
before, after = [set(open(p).read().splitlines()) for p in sys.argv[1:]]
assert len(before) >= 2 and before.issubset(after), (before, after)
CHECK_QUEUE
then
    record "agent-restart-queue-preserved" "PASS" "Exact identity and every pre-restart durable record retained; pre=${Q_PRE_RESTART} post=${Q_POST_RESTART}"
else
    fail_closed "agent-restart-queue-preserved" "Durable record or identity changed/disappeared"
fi
docker start ${PLATFORM_E2E_PREFIX}-sensor-infra >/dev/null
wait_healthy ${PLATFORM_E2E_PREFIX}-sensor-infra 40 || true
wait_receiver || true
sleep 10
RESTART_HITS="$(count_kafka_since '"agentId":"agent-e2e-01"' "${EVIDENCE_DIR}/offsets-agent-restart.txt")"
[[ "$RESTART_HITS" -ge 1 ]] || fail_closed "agent-restart-then-kafka" "No replay record observed"
record "agent-restart-then-kafka" "PASS" "kafka hits since sensor-down offset=${RESTART_HITS}"

# Core unavailable: authority for new POSTs requires Core (fail closed).
# Already-accepted Sensor Agent spool must be retained and drain after restore.
log "==> Core unavailable: fail-closed new authority; retain already-accepted spool"
kafka_end_offsets > "${EVIDENCE_DIR}/offsets-core-pause.txt"
python3 "${E2E_DIR}/build_envelope.py" \
    --envelope-tenant "${TENANT}" --payload-tenant "${TENANT}" \
    --agent-id "${AGENT_ID}" --host-id "${HOST_ID}" \
    --sequence 900021 --metric-key "e2e.core.pause.${RUN_ID}" --value 3 \
    --batch-id "batch-e2e-core-pause-${RUN_ID}" \
    > "${EVIDENCE_DIR}/core-pause-envelope.json"
docker pause "${PLATFORM_E2E_PREFIX}-kafka" >/dev/null
CORE_PAUSE_ACK="$(post_envelope "${EVIDENCE_DIR}/core-pause-envelope.json")"
if echo "${CORE_PAUSE_ACK}" | grep -q '"status":"ACCEPTED"'; then
    record "core-down-pre-ack" "PASS" "${CORE_PAUSE_ACK}"
else
    record "core-down-pre-ack" "FAIL" "${CORE_PAUSE_ACK}"
fi
docker stop ${PLATFORM_E2E_PREFIX}-sensor-infra >/dev/null
docker pause ${PLATFORM_E2E_PREFIX}-core >/dev/null
docker unpause "${PLATFORM_E2E_PREFIX}-kafka" >/dev/null
docker start ${PLATFORM_E2E_PREFIX}-sensor-infra >/dev/null
wait_healthy ${PLATFORM_E2E_PREFIX}-sensor-infra 40 || true
wait_receiver || true
CORE_DOWN_NEW="$(post_envelope "${EVIDENCE_DIR}/core-pause-envelope.json")"
if echo "${CORE_DOWN_NEW}" | grep -q 'ERR_AGENT_AUTHORITY_UNAVAILABLE'; then
    record "core-down-new-post-fail-closed" "PASS" "new Agent POST requires Core authority"
else
    record "core-down-new-post-fail-closed" "FAIL" "${CORE_DOWN_NEW}"
fi
SPOOL_DURING="$(docker exec ${PLATFORM_E2E_PREFIX}-sensor-infra sh -c 'find /var/lib/platform/sensor/agent-spool -maxdepth 1 -name "record-*.json" -type f 2>/dev/null | wc -l' | tr -d ' ')"
[[ "$SPOOL_DURING" -ge 1 ]] || fail_closed "core-down-sensor-spool-retained" "Empty spool"
record "core-down-sensor-spool-retained" "PASS" "agent-spool files=${SPOOL_DURING}"
docker unpause ${PLATFORM_E2E_PREFIX}-core >/dev/null
sleep 8
CORE_HITS=0
for i in $(seq 1 20); do
    CORE_HITS="$(count_kafka_since "e2e.core.pause.${RUN_ID}" "${EVIDENCE_DIR}/offsets-core-pause.txt")"
    if [[ "${CORE_HITS}" -ge 1 ]]; then
        break
    fi
    sleep 3
done
if [[ "${CORE_HITS}" -ge 1 ]]; then
    record "core-restore-kafka" "PASS" "e2e.core.pause.${RUN_ID} hits=${CORE_HITS}"
else
    record "core-restore-kafka" "FAIL" "accepted-before-core-outage batch never reached Kafka"
fi

# Sensor restart after Sensor ACK, with Core paused so drain cannot complete first.
log "==> Sensor restart after durable Agent acceptance"
kafka_end_offsets > "${EVIDENCE_DIR}/offsets-sensor-restart.txt"
python3 "${E2E_DIR}/build_envelope.py" \
    --envelope-tenant "${TENANT}" --payload-tenant "${TENANT}" \
    --agent-id "${AGENT_ID}" --host-id "${HOST_ID}" \
    --sequence 900031 --metric-key "e2e.sensor.restart.${RUN_ID}" --value 5 \
    --batch-id "batch-e2e-sensor-restart-${RUN_ID}" \
    > "${EVIDENCE_DIR}/sensor-restart-envelope.json"
docker pause "${PLATFORM_E2E_PREFIX}-kafka" >/dev/null
SR_ACK="$(post_envelope "${EVIDENCE_DIR}/sensor-restart-envelope.json")"
if echo "${SR_ACK}" | grep -q '"status":"ACCEPTED"'; then
    record "sensor-restart-pre-ack" "PASS" "${SR_ACK}"
else
    record "sensor-restart-pre-ack" "FAIL" "${SR_ACK}"
fi
docker pause ${PLATFORM_E2E_PREFIX}-core >/dev/null
docker restart ${PLATFORM_E2E_PREFIX}-sensor-infra >/dev/null
wait_healthy ${PLATFORM_E2E_PREFIX}-sensor-infra 40 || true
wait_receiver || true
docker unpause ${PLATFORM_E2E_PREFIX}-core >/dev/null
docker unpause "${PLATFORM_E2E_PREFIX}-kafka" >/dev/null
SR_HITS=0
for i in $(seq 1 24); do
    SR_HITS="$(count_kafka_since "e2e.sensor.restart.${RUN_ID}" "${EVIDENCE_DIR}/offsets-sensor-restart.txt")"
    if [[ "${SR_HITS}" -ge 1 ]]; then
        break
    fi
    sleep 3
done
if [[ "${SR_HITS}" -ge 1 ]]; then
    record "sensor-restart-recovers-accepted-batch" "PASS" "e2e.sensor.restart.${RUN_ID} hits=${SR_HITS}"
else
    record "sensor-restart-recovers-accepted-batch" "FAIL" "accepted batch did not reach Kafka after Sensor restart"
fi

# Process lifecycle: invalid config and SIGTERM already used docker stop (SIGTERM)
INVALID_EC="$(docker run --rm --name "${PLATFORM_E2E_PREFIX}-invalid-config" --label "com.docker.compose.project=${PLATFORM_E2E_PREFIX}" --network none --read-only --cpus 0.25 --memory 128m --pids-limit 32 --cap-drop ALL --security-opt no-new-privileges:true --entrypoint /usr/local/bin/platform-agent-linux "${PLATFORM_E2E_PREFIX}:agent-runtime" >"${EVIDENCE_DIR}/agent-invalid.log" 2>&1; echo $?)"
if [[ "${INVALID_EC}" == "1" ]]; then
    record "invalid-config-exit-1" "PASS"
else
    record "invalid-config-exit-1" "FAIL" "exit=${INVALID_EC}"
fi
record "sigterm-via-docker-stop" "PASS" "docker stop exited Agent with 0; exact durable record hashes survived restart; this is not host reboot"

HEALTH_UP_OK=0
for i in $(seq 1 10); do
    READY_UP="$(agent_ready_code)"
    docker exec "${PLATFORM_E2E_PREFIX}-agent-runtime" wget -q -O - http://127.0.0.1:8089/ready > "${EVIDENCE_DIR}/ready-up-${i}.json" || true
    docker exec "${PLATFORM_E2E_PREFIX}-agent-runtime" wget -q -O - http://127.0.0.1:8089/health > "${EVIDENCE_DIR}/health-up-${i}.json" || true
    if python3 "${E2E_DIR}/readback.py" health-recovered "${EVIDENCE_DIR}/ready-up-${i}.json" "${EVIDENCE_DIR}/health-up-${i}.json" "${READY_UP:-0}" 0; then
        HEALTH_UP_OK=1
        break
    fi
    sleep 2
done
[[ "$HEALTH_UP_OK" == 1 ]] || fail_closed "health-ready-probe" "HTTP=$READY_UP; inspect retained readiness/support bodies"
record "health-ready-probe" "PASS" "HTTP200 UP/ready=true with recovered Sensor transport (loopback probe inside Agent container)"
sample_agent_resources recovered

# Explicit A/B private identities, each authenticated at Sensor before the same
# forged record is challenged at Core. Sensor custody is distinct from Core accept.
kafka_end_offsets > "${EVIDENCE_DIR}/offsets-forged-host.txt"
# This terminal probe deliberately freezes the Agent until teardown. Even if
# the probe fails early, diagnostics must not docker exec into that fixture.
IDENTITY_PROBE_FROZEN=1
python3 "${E2E_DIR}/identity_probe.py" "$PLATFORM_E2E_PREFIX" \
    "https://127.0.0.1:${RECEIVER_PORT}/api/v1/agent/telemetry" "$RUN_ID" "$EVIDENCE_DIR" \
    || fail_closed "agent-canonical-entity-binding" "See retained Sensor ACK and Core typed rejection evidence"
consume_since_offsets "${EVIDENCE_DIR}/offsets-forged-host.txt" 8 > "${EVIDENCE_DIR}/forged-host-kafka.txt"
docker exec "${PLATFORM_E2E_PREFIX}-clickhouse" sh -c \
    'exec clickhouse-client --user "$CLICKHOUSE_USER" --password "$CLICKHOUSE_PASSWORD" --query "$1"' sh \
    "SELECT tenant_id, entity_id, metric_id FROM platform.metric_numeric WHERE startsWith(metric_id,'e2e.forged.') FORMAT JSONEachRow" \
    > "${EVIDENCE_DIR}/forged-host-clickhouse.jsonl"
if python3 - "$EVIDENCE_DIR" <<'PYPROBE'
import json, sys
from pathlib import Path
p = Path(sys.argv[1])
evidence = json.loads((p / 'identity-fixtures.json').read_text())
metrics = {r['metric'] for r in evidence['results']}
for file, key in (('forged-host-kafka.txt', 'metricId'), ('forged-host-clickhouse.jsonl', 'metric_id')):
    for line in (p / file).read_text().splitlines():
        try: row = json.loads(line.split('|', 1)[-1])
        except ValueError: continue
        assert row.get(key) not in metrics, row
assert len(evidence['results']) == 2
PYPROBE
then
    record "agent-canonical-entity-binding" "PASS" "A claims HostB and B claims HostA: durable Core terminal ERR_AGENT_IDENTITY_CONFLICT, no Kafka/ClickHouse metric for either entity; Sensor custody ACK preserved"
else
    fail_closed "agent-canonical-entity-binding" "Forged metric reached downstream storage"
fi

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
