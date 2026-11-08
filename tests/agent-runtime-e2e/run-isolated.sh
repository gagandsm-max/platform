#!/usr/bin/env bash
# Invoke only after Issue #211 runtime admission. Never reuse a prior run namespace.
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
: "${PLATFORM_E2E_RUN_DIR:?Set a new durable evidence directory for this run}"
[[ $PLATFORM_E2E_RUN_DIR == /* && ! -e $PLATFORM_E2E_RUN_DIR ]] || { echo 'Run directory must be absolute and new' >&2; exit 1; }
mkdir -m 0700 "$PLATFORM_E2E_RUN_DIR"
export PLATFORM_E2E_PREFIX="agent-e2e-$(date -u +%Y%m%d%H%M%S)-$$"
export PLATFORM_E2E_CA_DIR="$PLATFORM_E2E_RUN_DIR/pki/deploy/docker/certs"
export PLATFORM_E2E_CERT_DIR="$PLATFORM_E2E_RUN_DIR/agent-certs"
export PLATFORM_E2E_ISOLATED_CONFIG="$PLATFORM_E2E_RUN_DIR/compose.json"
export PLATFORM_KAFKA_DATA_VOLUME="$PLATFORM_E2E_PREFIX-platform_kafka_data"
export PLATFORM_KAFKA_STORAGE_ID="$PLATFORM_E2E_PREFIX"
export PLATFORM_KAFKA_NEW_VOLUME_CONFIRM="$PLATFORM_KAFKA_STORAGE_ID"
export PLATFORM_KAFKA_CLUSTER_ID
PLATFORM_KAFKA_CLUSTER_ID=$(openssl rand -base64 16 | tr '+/' '-_' | tr -d '=\n')
export COMPOSE_PROFILES=dev COMPOSE_PARALLEL_LIMIT=1
cd "$ROOT"
git rev-parse HEAD 'HEAD^{tree}' > "$PLATFORM_E2E_RUN_DIR/source-head.txt"
git status --porcelain=v1 > "$PLATFORM_E2E_RUN_DIR/source-status.txt"
git diff --binary > "$PLATFORM_E2E_RUN_DIR/source.diff"
git ls-files -co --exclude-standard -z | xargs -0 sha256sum > "$PLATFORM_E2E_RUN_DIR/source-files.sha256"
date -u +%FT%TZ > "$PLATFORM_E2E_RUN_DIR/start.utc"
docker compose --env-file .env.example -f compose.yaml -f compose.kafka-guarded.yaml -f tests/agent-runtime-e2e/compose.override.yaml config --format json > "$PLATFORM_E2E_RUN_DIR/rendered-original.json"
python3 tests/agent-runtime-e2e/isolation.py render "$PLATFORM_E2E_RUN_DIR/rendered-original.json" "$PLATFORM_E2E_PREFIX" "$ROOT" "$PLATFORM_E2E_RUN_DIR" > "$PLATFORM_E2E_ISOLATED_CONFIG"
python3 tests/agent-runtime-e2e/isolation.py verify "$PLATFORM_E2E_ISOLATED_CONFIG" "$PLATFORM_E2E_PREFIX"
DC=(docker compose -p "$PLATFORM_E2E_PREFIX" -f "$PLATFORM_E2E_ISOLATED_CONFIG")
"${DC[@]}" config --quiet
# Fresh-name checks precede cleanup registration and every runtime mutation.
[[ -z $(docker ps -aq --filter "label=com.docker.compose.project=$PLATFORM_E2E_PREFIX") ]]
[[ -z $(docker volume ls -q --filter "name=^$PLATFORM_E2E_PREFIX-") ]]
[[ -z $(docker network ls -q --filter "name=^$PLATFORM_E2E_PREFIX-") ]]
[[ -z $(docker ps -aq --filter "name=^/$PLATFORM_E2E_PREFIX-") ]]
cleanup() {
    local rc=$? cleanup_rc=0
    trap - EXIT
    "${DC[@]}" logs --no-color > "$PLATFORM_E2E_RUN_DIR/stack.log" 2>&1 || cleanup_rc=1
    mkdir -p "$PLATFORM_E2E_RUN_DIR/container-state"
    for cid in $(docker ps -aq --filter "label=com.docker.compose.project=$PLATFORM_E2E_PREFIX"); do
        docker inspect "$cid" > "$PLATFORM_E2E_RUN_DIR/container-state/$cid.json" || cleanup_rc=1
    done
    if docker container inspect "$PLATFORM_E2E_PREFIX-clickhouse" >/dev/null 2>&1; then
        docker cp "$PLATFORM_E2E_PREFIX-clickhouse:/var/log/clickhouse-server" "$PLATFORM_E2E_RUN_DIR/container-state/clickhouse-logs" >> "$PLATFORM_E2E_RUN_DIR/cleanup.log" 2>&1 || cleanup_rc=1
    fi
    for cid in $(docker ps -q --filter "label=com.docker.compose.project=$PLATFORM_E2E_PREFIX" --filter status=paused); do docker unpause "$cid" || cleanup_rc=1; done
    "${DC[@]}" down --volumes --timeout 20 >> "$PLATFORM_E2E_RUN_DIR/cleanup.log" 2>&1 || cleanup_rc=1
    if docker volume inspect "$PLATFORM_KAFKA_DATA_VOLUME" >/dev/null 2>&1; then
        if [[ $(docker volume inspect -f '{{index .Labels "platform.kafka.project"}}' "$PLATFORM_KAFKA_DATA_VOLUME") == "$PLATFORM_E2E_PREFIX" ]]; then
            docker volume rm "$PLATFORM_KAFKA_DATA_VOLUME" >> "$PLATFORM_E2E_RUN_DIR/cleanup.log" 2>&1 || cleanup_rc=1
        else cleanup_rc=1; fi
    fi
    # A successful down command alone is not cleanup evidence.
    docker ps -a --filter "label=com.docker.compose.project=$PLATFORM_E2E_PREFIX" --format '{{.ID}} {{.Names}} {{.Status}}' > "$PLATFORM_E2E_RUN_DIR/remaining-containers.txt" || cleanup_rc=1
    docker volume ls -q --filter "name=^$PLATFORM_E2E_PREFIX-" > "$PLATFORM_E2E_RUN_DIR/remaining-volumes.txt" || cleanup_rc=1
    docker network ls -q --filter "name=^$PLATFORM_E2E_PREFIX-" > "$PLATFORM_E2E_RUN_DIR/remaining-networks.txt" || cleanup_rc=1
    for kind in containers volumes networks; do
        [[ ! -s "$PLATFORM_E2E_RUN_DIR/remaining-$kind.txt" ]] || cleanup_rc=1
    done
    printf 'workload_exit=%s\ncleanup_exit=%s\n' "$rc" "$cleanup_rc" > "$PLATFORM_E2E_RUN_DIR/exits.txt"
    date -u +%FT%TZ > "$PLATFORM_E2E_RUN_DIR/end.utc"
    if [[ $cleanup_rc != 0 ]]; then exit 1; fi
    exit "$rc"
}
trap cleanup EXIT
# Use the existing PKI generator in a private projection; no shared cert mutation.
mkdir -p "$PLATFORM_E2E_RUN_DIR/pki/scripts"
cp scripts/generate-dev-certs.sh "$PLATFORM_E2E_RUN_DIR/pki/scripts/"
bash "$PLATFORM_E2E_RUN_DIR/pki/scripts/generate-dev-certs.sh" > "$PLATFORM_E2E_RUN_DIR/pki.log" 2>&1
bash tests/agent-runtime-e2e/generate-certs.sh >> "$PLATFORM_E2E_RUN_DIR/pki.log" 2>&1
bash deploy/docker/kafka/deployment-preflight.sh provision -p "$PLATFORM_E2E_PREFIX" -f "$PLATFORM_E2E_ISOLATED_CONFIG" > "$PLATFORM_E2E_RUN_DIR/kafka-provision.log" 2>&1
# Buildx may parallelize a multi-service build even with COMPOSE_PARALLEL_LIMIT.
for service in core sensor-infra agent-runtime; do
    "${DC[@]}" build "$service" > "$PLATFORM_E2E_RUN_DIR/build-$service.log" 2>&1
done
bash tests/agent-runtime-e2e/run.sh
