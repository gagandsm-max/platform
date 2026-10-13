#!/usr/bin/env bash
# Only unique, task-owned disposable Compose resources; no default stack access.
set -euo pipefail
REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TASK=$(mktemp -d /tmp/issue223-storage.XXXXXX)
PROJECT=issue223-storage-$(cat /proc/sys/kernel/random/uuid)
VOLUME=ci_${PROJECT}_1_kafka_data
REPLACEMENT=${PROJECT}-replacement
INTERRUPTED=${PROJECT}-interrupted
export PLATFORM_KAFKA_CLUSTER_ID
PLATFORM_KAFKA_CLUSTER_ID=$(docker run --rm --network none --tmpfs /etc/kafka/secrets \
    --tmpfs /mnt/shared/config --tmpfs /var/lib/kafka/data --entrypoint /bin/bash apache/kafka:3.8.0 -c '/opt/kafka/bin/kafka-storage.sh random-uuid')
[[ $PLATFORM_KAFKA_CLUSTER_ID =~ ^[A-Za-z0-9_-]{22}$ ]]
export PLATFORM_KAFKA_STORAGE_ID=${PROJECT}-identity
export PLATFORM_KAFKA_DATA_VOLUME=$VOLUME
# Pin merge semantics and render the base+opt-in service before extending it.
docker compose --profile dev -p "$PROJECT" -f "$REPO/compose.yaml" -f "$REPO/compose.kafka-guarded.yaml" config > "$TASK/guarded.yaml"
cat > "$TASK/compose.yaml" <<EOF_YAML
services:
  kafka:
    extends:
      file: $TASK/guarded.yaml
      service: kafka
    profiles: []
    container_name: $PROJECT-kafka
    restart: "no"
    labels:
      issue223.owner: $PROJECT
    environment:
      PLATFORM_KAFKA_NEW_VOLUME_CONFIRM: $PLATFORM_KAFKA_STORAGE_ID
    networks: [platform_net]
volumes:
  platform_kafka_data:
    external: true
    name: $VOLUME
networks:
  platform_net:
    internal: true
    labels:
      issue223.owner: $PROJECT
EOF_YAML
DC=(docker compose --profile dev -p "$PROJECT" -f "$TASK/compose.yaml")
# Default service equality is a recreation-safety invariant, not just syntax.
git -C "$REPO" show 27fbf8818ed8745fd5a38cb9521adae812224fa8:compose.yaml > "$TASK/legacy.yaml"
docker compose --profile dev -p "$PROJECT" -f "$TASK/legacy.yaml" config --format json > "$TASK/legacy.json"
docker compose --profile dev -p "$PROJECT" -f "$REPO/compose.yaml" config --format json > "$TASK/default.json"
docker compose --profile dev -p "$PROJECT" -f "$REPO/compose.yaml" -f "$REPO/compose.kafka-guarded.yaml" config --format json > "$TASK/guarded.json"
docker run --rm --network none --pull never --mount "type=bind,src=$TASK,dst=/evidence,readonly" \
    --entrypoint node node:20-alpine -e '
const fs=require("fs"),assert=require("assert");
const read=n=>JSON.parse(fs.readFileSync("/evidence/"+n+".json"));
assert.deepStrictEqual(read("default").services.kafka,read("legacy").services.kafka);
const c=read("guarded"),e=c.services.kafka.environment;
assert(!Object.hasOwn(e,"KAFKA_CLUSTER_ID"));
assert.equal(e.KAFKA_LOG_DIRS,"/var/lib/kafka/data/logs");
assert.equal(e.KAFKA_METADATA_LOG_DIR,e.KAFKA_LOG_DIRS);
assert.equal(c.volumes.platform_kafka_data.external,true);
console.log("PASS default Kafka unchanged and pinned Compose environment merge");'
for profile in dev test ha scale security dr; do
    docker compose --profile "$profile" -f "$REPO/compose.yaml" config -q
    docker compose --profile "$profile" -f "$REPO/compose.yaml" -f "$REPO/compose.kafka-guarded.yaml" config -q
done
echo 'PASS all six default/guarded Compose profiles'
gate() { local action=$1; shift; bash "$REPO/deploy/docker/kafka/deployment-preflight.sh" "$action" "${DC[@]:2}" "$@"; }
new_volume() {
    local names containers
    names=$(docker volume ls --format '{{.Name}}') || return 1
    ! grep -Fxq "$1" <<< "$names" || { echo 'Primary test-volume collision rejected' >&2; return 1; }
    containers=$(docker ps -aq --filter "name=$PROJECT") || return 1
    [[ -z $containers ]] || return 1
    containers=$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT") || return 1
    [[ -z $containers ]] || return 1
    containers=$(docker ps -aq --filter "label=issue223.owner=$PROJECT") || return 1
    [[ -z $containers ]] || return 1
}
owned_volume() {
    local volume=$1 owner=${2:-$PROJECT} storage=${3:-$PLATFORM_KAFKA_STORAGE_ID}
    [[ $(docker volume inspect -f '{{index .Labels "issue223.owner"}}|{{index .Labels "platform.kafka.project"}}|{{index .Labels "platform.kafka.storage-id"}}|{{index .Labels "platform.kafka.resource"}}' "$volume" 2>/dev/null) == "$owner|$owner|$storage|platform_kafka_data" ]]
}
remove_owned_volume() {
    owned_volume "$@" || { echo "Refuse deletion: unowned volume $1" >&2; return 1; }
    local users
    users=$(docker ps -aq --filter "volume=$1") || return 1
    [[ -z $users ]] || return 1
    docker volume rm "$1" > /dev/null
}
cleanup() {
    result=$?
    if (( result != 0 )); then
        "${DC[@]}" logs --no-color --tail 100 || true
        echo "FAIL: evidence retained at $TASK"
    fi
    # An API failure is unknown provenance, never an empty resource inventory.
    local volume_names network_ids container_ids
    volume_names=$(docker volume ls --format '{{.Name}}') || exit 1
    network_ids=$(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT") || exit 1
    container_ids=$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT") || exit 1
    # Verify every resource before teardown; never delete volumes through Compose.
    for volume in "$VOLUME" "$REPLACEMENT" "$INTERRUPTED"; do
        storage=$PLATFORM_KAFKA_STORAGE_ID; [[ $volume != "$INTERRUPTED" ]] || storage=$INTERRUPTED
        if grep -Fxq "$volume" <<< "$volume_names"; then owned_volume "$volume" "$PROJECT" "$storage" || { echo 'Foreign volume: cleanup refused' >&2; exit 1; }; fi
    done
    for network in $network_ids; do
        [[ $(docker network inspect -f '{{index .Labels "issue223.owner"}}|{{index .Labels "com.docker.compose.project"}}' "$network") == "$PROJECT|$PROJECT" ]] || { echo 'Foreign network: cleanup refused' >&2; exit 1; }
    done
    for cid in $container_ids; do
        [[ $(docker inspect -f '{{index .Config.Labels "issue223.owner"}}|{{index .Config.Labels "com.docker.compose.project"}}' "$cid") == "$PROJECT|$PROJECT" ]] || { echo 'Foreign container: cleanup refused' >&2; exit 1; }
    done
    "${DC[@]}" down > /dev/null || exit 1
    for volume in "$VOLUME" "$REPLACEMENT" "$INTERRUPTED"; do
        storage=$PLATFORM_KAFKA_STORAGE_ID; [[ $volume != "$INTERRUPTED" ]] || storage=$INTERRUPTED
        if grep -Fxq "$volume" <<< "$volume_names"; then remove_owned_volume "$volume" "$PROJECT" "$storage" || exit 1; fi
    done
    exit "$result"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
run() { "${DC[@]}" run -T --rm --no-deps --entrypoint /bin/bash kafka "$@"; }
cli() { "${DC[@]}" exec -T kafka /opt/kafka/bin/"$1" --bootstrap-server kafka:9092 "${@:2}"; }
mutate() { run -c "$1"; }
hash_storage() { "${DC[@]}" run -T --rm --no-deps --user 0 --entrypoint /bin/bash kafka -c 'set -o pipefail; cd /var/lib/kafka/data; find . -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum' > "$1"; }
reject() {
    local code=$1; shift
    hash_storage "$TASK/before.sha256"
    if run "$@" > "$TASK/reject.log" 2>&1; then
        echo "Expected rejection: $code" >&2; exit 1
    fi
    grep -q "\"event_code\":\"$code\"" "$TASK/reject.log"
    hash_storage "$TASK/after.sha256"
    cmp "$TASK/before.sha256" "$TASK/after.sha256"
    echo "PASS rejection $code; storage unchanged"
}
ready() {
    for ((i=0; i<60; i++)); do
        if cli kafka-broker-api-versions.sh > /dev/null 2>&1; then return; fi
        if [[ $(docker inspect -f '{{.State.Running}}' "$PROJECT-kafka") != true ]]; then break; fi
        sleep 1
    done
    echo 'Broker failed bounded readiness' >&2; exit 1
}
read_records() {
    "${DC[@]}" exec -T kafka timeout 30 /opt/kafka/bin/kafka-console-consumer.sh \
        --bootstrap-server kafka:9092 --topic "$1" --from-beginning \
        --isolation-level read_committed --max-messages "$2" > "$3"
}
offsets() { cli kafka-consumer-groups.sh --describe --group issue223-group | awk '$2=="issue223-records" {print $2,$3,$4,$5}' | LC_ALL=C sort; }
transaction() { cli kafka-transactions.sh describe --transactional-id issue223-transaction | awk 'NR>1 {print $1,$2,$3,$4,$5,$6,$7}'; }

"${DC[@]}" config -q
[[ -z $(docker network ls -q --filter "label=com.docker.compose.project=$PROJECT") ]]
new_volume "$VOLUME"
new_volume "$REPLACEMENT"
# Legacy overlay fixture: a sleeping synthetic container, never the live broker.
docker run -d --rm --name "$PROJECT-kafka" --network none --tmpfs /etc/kafka/secrets \
    --tmpfs /mnt/shared/config --tmpfs /var/lib/kafka/data \
    --label "issue223.owner=$PROJECT" --label "com.docker.compose.project=$PROJECT" \
    --label com.docker.compose.service=kafka --entrypoint /bin/bash apache/kafka:3.8.0 \
    -c 'mkdir -p /tmp/kafka-logs; printf "cluster.id=legacy-fixture\nnode.id=1\n" > /tmp/kafka-logs/meta.properties; sleep 300' > "$TASK/legacy-id"
sleep 1
if gate up > "$TASK/legacy-reject.log" 2>&1; then exit 1; fi
grep -q KAFKA_DEPLOYMENT_MIGRATION_REQUIRED "$TASK/legacy-reject.log"
[[ $(docker inspect -f '{{.Id}}' "$PROJECT-kafka") == "$(cat "$TASK/legacy-id")" ]]
docker cp "$PROJECT-kafka:/tmp/kafka-logs/meta.properties" - | tar -xOf - | grep -q legacy-fixture
echo 'PASS legacy overlay container retained, no Compose recreation'
[[ $(docker inspect -f '{{index .Config.Labels "issue223.owner"}}' "$PROJECT-kafka") == "$PROJECT" ]]
docker stop "$PROJECT-kafka" >/dev/null
if gate up > "$TASK/missing-reject.log" 2>&1; then exit 1; fi
grep -q KAFKA_DEPLOYMENT_VOLUME_MISSING "$TASK/missing-reject.log"
[[ -z $(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT") ]]
! docker volume inspect "$VOLUME" >/dev/null 2>&1
echo 'PASS missing state rejected before Compose mutation'
if PLATFORM_KAFKA_NEW_VOLUME_CONFIRM=wrong gate provision > "$TASK/auth-reject.log" 2>&1; then exit 1; fi
! docker volume inspect "$VOLUME" >/dev/null 2>&1
PLATFORM_KAFKA_NEW_VOLUME_CONFIRM=$PLATFORM_KAFKA_STORAGE_ID gate provision
owned_volume "$VOLUME"
if new_volume "$VOLUME" > "$TASK/collision.log" 2>&1; then exit 1; fi
grep -q 'collision rejected' "$TASK/collision.log"
if PLATFORM_KAFKA_NEW_VOLUME_CONFIRM=$PLATFORM_KAFKA_STORAGE_ID gate provision > "$TASK/collision-provision.log" 2>&1; then exit 1; fi
grep -q KAFKA_DEPLOYMENT_COLLISION "$TASK/collision-provision.log"
echo 'PASS explicit provisioning and primary volume collision rejection'
echo 'PASS CI-scoped volume name and exact deployment ownership'
# Stopped, not-yet-started container: desired env alone is not effective evidence.
"${DC[@]}" create kafka
UNKNOWN_ID=$(docker inspect -f '{{.Id}}' "$PROJECT-kafka")
if gate up > "$TASK/unknown-reject.log" 2>&1; then exit 1; fi
grep -q KAFKA_DEPLOYMENT_UNKNOWN "$TASK/unknown-reject.log"
[[ $(docker inspect -f '{{.Id}}' "$PROJECT-kafka") == "$UNKNOWN_ID" ]]
[[ $(docker inspect -f '{{index .Config.Labels "issue223.owner"}}' "$PROJECT-kafka") == "$PROJECT" ]]
"${DC[@]}" down
echo 'PASS unknown effective state rejected with existing container retained'
# Kill the actual provisioner after reservation, before formatting can execute.
# This isolated format-tool pause is fault injection, not successful formatting.
new_volume "$INTERRUPTED"
docker volume create --label "issue223.owner=$PROJECT" --label "platform.kafka.project=$PROJECT" \
    --label "platform.kafka.storage-id=$INTERRUPTED" --label platform.kafka.resource=platform_kafka_data "$INTERRUPTED" >/dev/null
owned_volume "$INTERRUPTED" "$PROJECT" "$INTERRUPTED"
docker run --rm --network none --user 0 --tmpfs /etc/kafka/secrets --tmpfs /mnt/shared/config \
    --mount "type=volume,src=$INTERRUPTED,dst=/var/lib/kafka/data" --entrypoint /bin/bash apache/kafka:3.8.0 \
    -c 'test -z "$(find /var/lib/kafka/data -mindepth 1 -maxdepth 1 -print -quit)" && chown 1000:1000 /var/lib/kafka/data'
printf '#!/usr/bin/env bash\n[[ $1 == format ]] || exit 1\nexec sleep 600\n' > "$TASK/pause-format.sh"
chmod 755 "$TASK/pause-format.sh"
cat > "$TASK/interrupted.yaml" <<EOF_YAML
services:
  kafka:
    environment:
      PLATFORM_KAFKA_STORAGE_ID: $INTERRUPTED
      PLATFORM_KAFKA_NEW_VOLUME_CONFIRM: $INTERRUPTED
    volumes:
      - $TASK/pause-format.sh:/opt/kafka/bin/kafka-storage.sh:ro
volumes:
  platform_kafka_data:
    name: $INTERRUPTED
EOF_YAML
"${DC[@]}" -f "$TASK/interrupted.yaml" run -d --rm --no-deps --user 1000:1000 \
    --name "$PROJECT-interrupt" --entrypoint /bin/bash kafka /platform-kafka/provision-fresh.sh --new-empty-volume >/dev/null
reserved=0
for ((i=0; i<30; i++)); do
    if docker exec "$PROJECT-interrupt" test -f /var/lib/kafka/data/.platform-storage.pending; then reserved=1; break; fi
    sleep 1
done
[[ $reserved == 1 ]]
[[ $(docker inspect -f '{{index .Config.Labels "issue223.owner"}}|{{index .Config.Labels "com.docker.compose.project"}}' "$PROJECT-interrupt") == "$PROJECT|$PROJECT" ]]
docker kill "$PROJECT-interrupt" >/dev/null
if gate up -f "$TASK/interrupted.yaml" > "$TASK/killed-provisioner.log" 2>&1; then exit 1; fi
grep -q KAFKA_PROVISION_INCOMPLETE "$TASK/killed-provisioner.log"
owned_volume "$INTERRUPTED" "$PROJECT" "$INTERRUPTED"
remove_owned_volume "$INTERRUPTED" "$PROJECT" "$INTERRUPTED"
echo 'PASS killed provisioning process leaves reserved state; startup fails before Compose mutation'
reject KAFKA_PROVISION_AUTH_REQUIRED /platform-kafka/provision-fresh.sh
reject KAFKA_PROVISION_AUTH_REQUIRED -c 'PLATFORM_KAFKA_NEW_VOLUME_CONFIRM=wrong /platform-kafka/provision-fresh.sh --new-empty-volume'
mutate 'printf legacy-overlay > /var/lib/kafka/data/old-state'
reject KAFKA_PROVISION_NONEMPTY /platform-kafka/provision-fresh.sh --new-empty-volume
mutate 'rm /var/lib/kafka/data/old-state'
reject KAFKA_PROVISION_NONEMPTY /platform-kafka/provision-fresh.sh --new-empty-volume
reject KAFKA_IDENTITY_MISMATCH -c 'CLUSTER_ID=Ab1nT6p7Q8u9R0s1V2w3X4 /platform-kafka/guarded-start.sh'
reject KAFKA_IDENTITY_MISMATCH -c 'KAFKA_NODE_ID=2 /platform-kafka/guarded-start.sh'
reject KAFKA_IDENTITY_MISMATCH -c 'PLATFORM_KAFKA_STORAGE_ID=another-storage-identity /platform-kafka/guarded-start.sh'
reject KAFKA_CLUSTER_ID_REQUIRED -c 'unset CLUSTER_ID; /platform-kafka/guarded-start.sh'
reject KAFKA_UNEXPECTED_DIRECTORY -c 'KAFKA_LOG_DIRS=/tmp/kafka-logs /platform-kafka/guarded-start.sh'
reject KAFKA_LEGACY_CLUSTER_ID -c 'KAFKA_CLUSTER_ID=legacy /platform-kafka/guarded-start.sh'
reject KAFKA_MOUNT_MISSING -c 'source /platform-kafka/guarded-start.sh; ROOT=/tmp; check_mount'
# Previous overlay location cannot satisfy persistence, even with formatted bytes.
mutate 'mv /var/lib/kafka/data/logs /var/lib/kafka/data/logs.saved'
reject KAFKA_STORAGE_UNFORMATTED -c 'mkdir /tmp/kafka-logs; cp /var/lib/kafka/data/logs.saved/meta.properties /tmp/kafka-logs/; /platform-kafka/guarded-start.sh'
mutate 'mv /var/lib/kafka/data/logs.saved /var/lib/kafka/data/logs'
mutate 'cp /var/lib/kafka/data/logs/bootstrap.checkpoint /var/lib/kafka/data/bootstrap.saved; printf corrupt > /var/lib/kafka/data/logs/bootstrap.checkpoint'
reject KAFKA_IDENTITY_MISMATCH /platform-kafka/guarded-start.sh
mutate 'mv /var/lib/kafka/data/bootstrap.saved /var/lib/kafka/data/logs/bootstrap.checkpoint'
mutate 'mv /var/lib/kafka/data/.platform-storage /var/lib/kafka/data/identity.saved'
reject KAFKA_STORAGE_UNATTESTED /platform-kafka/guarded-start.sh
mutate 'mv /var/lib/kafka/data/identity.saved /var/lib/kafka/data/.platform-storage'
mutate 'printf interrupted > /var/lib/kafka/data/.platform-storage.pending'
reject KAFKA_PROVISION_INCOMPLETE /platform-kafka/guarded-start.sh
if gate up > "$TASK/interrupted-host.log" 2>&1; then exit 1; fi
grep -q KAFKA_PROVISION_INCOMPLETE "$TASK/interrupted-host.log"
[[ -z $(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT") ]]
mutate 'rm /var/lib/kafka/data/.platform-storage.pending'
mutate 'chmod 000 /var/lib/kafka/data/logs/meta.properties'
reject KAFKA_STORAGE_INACCESSIBLE /platform-kafka/guarded-start.sh
mutate 'chmod 600 /var/lib/kafka/data/logs/meta.properties'

gate up
ready
cli kafka-topics.sh --create --topic issue223-records --partitions 1 --replication-factor 1
cli kafka-topics.sh --create --topic issue223-transactions --partitions 1 --replication-factor 1
printf 'acknowledged-record-1\nacknowledged-record-2\n' | "${DC[@]}" exec -T kafka /opt/kafka/bin/kafka-console-producer.sh \
    --bootstrap-server kafka:9092 --topic issue223-records --producer-property acks=all
"${DC[@]}" exec -T kafka timeout 30 /opt/kafka/bin/kafka-console-consumer.sh \
    --bootstrap-server kafka:9092 --topic issue223-records --group issue223-group --from-beginning \
    --consumer-property enable.auto.commit=true --max-messages 2 > "$TASK/consumed-before"
offsets > "$TASK/offsets-before"
grep -q '^issue223-records 0 2 2$' "$TASK/offsets-before"
"${DC[@]}" exec -T kafka bash -c 'printf "committed-transaction\n" > /tmp/issue223-payload; /opt/kafka/bin/kafka-producer-perf-test.sh --topic issue223-transactions --num-records 8 --throughput -1 --payload-file /tmp/issue223-payload --transactional-id issue223-transaction --transaction-duration-ms 1000 --producer-props bootstrap.servers=kafka:9092 acks=all'
read_records issue223-transactions 8 "$TASK/transactions-before"
[[ $(grep -c '^committed-transaction$' "$TASK/transactions-before") == 8 ]]
transaction > "$TASK/transaction-before"
grep -q 'CompleteCommit' "$TASK/transaction-before"
cli kafka-topics.sh --describe --topic issue223-records > "$TASK/topic-before"
"${DC[@]}" exec -T kafka sha256sum /var/lib/kafka/data/logs/meta.properties /var/lib/kafka/data/logs/bootstrap.checkpoint > "$TASK/identity-before"
"${DC[@]}" exec -T kafka bash -c 'test -s /var/lib/kafka/data/logs/meta.properties; test -d /var/lib/kafka/data/logs/__cluster_metadata-0; test -d /var/lib/kafka/data/logs/issue223-records-0; compgen -G "/var/lib/kafka/data/logs/__consumer_offsets-*"; compgen -G "/var/lib/kafka/data/logs/__transaction_state-*"; test ! -e /tmp/kafka-logs; test ! -e /tmp/kraft-combined-logs' > "$TASK/physical-paths"
gate restart
ready
"${DC[@]}" exec -T kafka sha256sum /var/lib/kafka/data/logs/meta.properties /var/lib/kafka/data/logs/bootstrap.checkpoint > "$TASK/identity-restart"
cmp "$TASK/identity-before" "$TASK/identity-restart"
echo 'PASS explicit fresh start and ordinary restart identity'
"${DC[@]}" logs --no-color > "$TASK/broker.log"
gate down
gate up
ready
"${DC[@]}" exec -T kafka sha256sum /var/lib/kafka/data/logs/meta.properties /var/lib/kafka/data/logs/bootstrap.checkpoint > "$TASK/identity-after"
cmp "$TASK/identity-before" "$TASK/identity-after"
cli kafka-topics.sh --describe --topic issue223-records > "$TASK/topic-after"
cmp "$TASK/topic-before" "$TASK/topic-after"
read_records issue223-records 2 "$TASK/records-after"
cmp "$TASK/consumed-before" "$TASK/records-after"
read_records issue223-transactions 8 "$TASK/transactions-after"
cmp "$TASK/transactions-before" "$TASK/transactions-after"
offsets > "$TASK/offsets-after"
cmp "$TASK/offsets-before" "$TASK/offsets-after"
transaction > "$TASK/transaction-after"
cmp "$TASK/transaction-before" "$TASK/transaction-after"
echo 'PASS recreation: topic ID, committed records, consumer offsets and transaction producer ID/epoch/state'
"${DC[@]}" stop kafka
reject KAFKA_PROVISION_NONEMPTY /platform-kafka/provision-fresh.sh --new-empty-volume
mutate 'mv /var/lib/kafka/data/logs/__cluster_metadata-0 /var/lib/kafka/data/metadata.saved'
reject KAFKA_MIGRATION_INCOMPLETE /platform-kafka/guarded-start.sh
mutate 'mv /var/lib/kafka/data/metadata.saved /var/lib/kafka/data/logs/__cluster_metadata-0'
# Exercise cold-migration acceptance against our stopped synthetic broker only.
mutate 'rm /var/lib/kafka/data/.platform-storage; cd /var/lib/kafka/data; find logs -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum > .migration.sha256; cp .migration.sha256 manifest.saved; sed -i "1d" .migration.sha256'
reject KAFKA_MIGRATION_INVALID /platform-kafka/guarded-start.sh --seal-migration
mutate 'mv /var/lib/kafka/data/manifest.saved /var/lib/kafka/data/.migration.sha256; cp /var/lib/kafka/data/logs/issue223-records-0/00000000000000000000.log /var/lib/kafka/data/record.saved; printf corrupt >> /var/lib/kafka/data/logs/issue223-records-0/00000000000000000000.log'
reject KAFKA_MIGRATION_INVALID /platform-kafka/guarded-start.sh --seal-migration
mutate 'mv /var/lib/kafka/data/record.saved /var/lib/kafka/data/logs/issue223-records-0/00000000000000000000.log'
run /platform-kafka/guarded-start.sh --seal-migration
mutate 'cp /var/lib/kafka/data/logs/issue223-records-0/00000000000000000000.log /var/lib/kafka/data/record.saved; printf corrupt >> /var/lib/kafka/data/logs/issue223-records-0/00000000000000000000.log'
reject KAFKA_MIGRATION_INVALID /platform-kafka/guarded-start.sh
mutate 'mv /var/lib/kafka/data/record.saved /var/lib/kafka/data/logs/issue223-records-0/00000000000000000000.log'
run /platform-kafka/guarded-start.sh --check
gate up
ready
read_records issue223-records 2 "$TASK/records-migration"
cmp "$TASK/records-after" "$TASK/records-migration"
echo 'PASS cold-copy seal rejects partial/corrupt manifests and preserves readback'
"${DC[@]}" logs --no-color >> "$TASK/broker.log"
! grep -q 'Formatting' "$TASK/broker.log"
"${DC[@]}" down
# A replaced named volume cannot use the external identity as fresh authority.
new_volume "$REPLACEMENT"
docker volume create --label "issue223.owner=$PROJECT" --label "platform.kafka.project=$PROJECT" \
    --label "platform.kafka.storage-id=$PLATFORM_KAFKA_STORAGE_ID" --label platform.kafka.resource=platform_kafka_data "$REPLACEMENT" > /dev/null
cat > "$TASK/replacement.yaml" <<EOF_YAML
volumes:
  platform_kafka_data:
    name: $REPLACEMENT
EOF_YAML
if "${DC[@]}" -f "$TASK/replacement.yaml" run -T --rm --no-deps --entrypoint /bin/bash kafka /platform-kafka/guarded-start.sh > "$TASK/replaced.log" 2>&1; then exit 1; fi
grep -q KAFKA_STORAGE_UNFORMATTED "$TASK/replaced.log"
"${DC[@]}" -f "$TASK/replacement.yaml" down
if gate up -f "$TASK/replacement.yaml" > "$TASK/missing-storage.log" 2>&1; then exit 1; fi
grep -q KAFKA_STORAGE_UNFORMATTED "$TASK/missing-storage.log"
# Independent foreign-provenance fixture: refusal first, verified fixture cleanup later.
FOREIGN=$PROJECT-foreign
docker volume create --label "issue223.owner=$FOREIGN" --label "platform.kafka.project=$FOREIGN" \
    --label "platform.kafka.storage-id=$FOREIGN" --label platform.kafka.resource=platform_kafka_data "$FOREIGN" > /dev/null
if remove_owned_volume "$FOREIGN" > "$TASK/foreign-cleanup.log" 2>&1; then exit 1; fi
docker volume inspect "$FOREIGN" >/dev/null
remove_owned_volume "$FOREIGN" "$FOREIGN" "$FOREIGN"
echo 'PASS cleanup refuses foreign provenance; exact labels required for both primary and replacement'
# Source assertion plus direct entrypoint ensure no wrapper/format is reachable.
! grep -E 'KafkaDockerWrapper|/etc/kafka/docker/(run|launch)' "$REPO/deploy/docker/kafka/guarded-start.sh" | grep -v '^#'
echo "PASS guarded Kafka storage contract; project=$PROJECT evidence=$TASK"
