#!/usr/bin/env bash
# Only unique, task-owned disposable Compose resources; no default stack access.
set -euo pipefail
REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TASK=$(mktemp -d /tmp/issue223-storage.XXXXXX)
PROJECT=issue223-storage-$(date +%s)-$$
VOLUME=${PROJECT}-data
REPLACEMENT=${PROJECT}-replacement
export PLATFORM_KAFKA_CLUSTER_ID=Zy1nT6p7Q8u9R0s1V2w3X4
export PLATFORM_KAFKA_STORAGE_ID=${PROJECT}-identity
cat > "$TASK/compose.yaml" <<EOF_YAML
services:
  kafka:
    extends:
      file: $REPO/compose.yaml
      service: kafka
    profiles: []
    container_name: $PROJECT-kafka
    restart: "no"
    environment:
      PLATFORM_KAFKA_NEW_VOLUME_CONFIRM: $PLATFORM_KAFKA_STORAGE_ID
    networks: [platform_net]
volumes:
  platform_kafka_data:
    name: $VOLUME
    labels:
      issue223.owner: $PROJECT
networks:
  platform_net:
    internal: true
EOF_YAML
DC=(docker compose --profile dev -p "$PROJECT" -f "$TASK/compose.yaml")
cleanup() {
    result=$?
    if (( result != 0 )); then
        "${DC[@]}" logs --no-color --tail 100 || true
        echo "FAIL: evidence retained at $TASK"
    fi
    "${DC[@]}" down -v --remove-orphans > /dev/null
    # This volume is created only by this harness, with its unique owner label.
    if [[ $(docker volume inspect -f '{{index .Labels "issue223.owner"}}' "$REPLACEMENT" 2>/dev/null || true) == "$PROJECT" ]]; then
        docker volume rm "$REPLACEMENT" > /dev/null
    fi
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
reject KAFKA_STORAGE_UNFORMATTED /platform-kafka/guarded-start.sh
reject KAFKA_PROVISION_AUTH_REQUIRED /platform-kafka/provision-fresh.sh
reject KAFKA_PROVISION_AUTH_REQUIRED -c 'PLATFORM_KAFKA_NEW_VOLUME_CONFIRM=wrong /platform-kafka/provision-fresh.sh --new-empty-volume'
mutate 'printf legacy-overlay > /var/lib/kafka/data/old-state'
reject KAFKA_PROVISION_NONEMPTY /platform-kafka/provision-fresh.sh --new-empty-volume
mutate 'rm /var/lib/kafka/data/old-state'
run /platform-kafka/provision-fresh.sh --new-empty-volume
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
mutate 'rm /var/lib/kafka/data/.platform-storage.pending'
mutate 'chmod 000 /var/lib/kafka/data/logs/meta.properties'
reject KAFKA_STORAGE_INACCESSIBLE /platform-kafka/guarded-start.sh
mutate 'chmod 600 /var/lib/kafka/data/logs/meta.properties'

"${DC[@]}" up -d kafka
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
"${DC[@]}" restart kafka
ready
"${DC[@]}" exec -T kafka sha256sum /var/lib/kafka/data/logs/meta.properties /var/lib/kafka/data/logs/bootstrap.checkpoint > "$TASK/identity-restart"
cmp "$TASK/identity-before" "$TASK/identity-restart"
echo 'PASS explicit fresh start and ordinary restart identity'
"${DC[@]}" logs --no-color > "$TASK/broker.log"
"${DC[@]}" down
"${DC[@]}" up -d kafka
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
"${DC[@]}" up -d kafka
ready
read_records issue223-records 2 "$TASK/records-migration"
cmp "$TASK/records-after" "$TASK/records-migration"
echo 'PASS cold-copy seal rejects partial/corrupt manifests and preserves readback'
"${DC[@]}" logs --no-color >> "$TASK/broker.log"
! grep -q 'Formatting' "$TASK/broker.log"
"${DC[@]}" down
# A replaced named volume cannot use the external identity as fresh authority.
docker volume create --label "issue223.owner=$PROJECT" "$REPLACEMENT" > /dev/null
cat > "$TASK/replacement.yaml" <<EOF_YAML
volumes:
  platform_kafka_data:
    name: $REPLACEMENT
EOF_YAML
if "${DC[@]}" -f "$TASK/replacement.yaml" run -T --rm --no-deps --entrypoint /bin/bash kafka /platform-kafka/guarded-start.sh > "$TASK/replaced.log" 2>&1; then exit 1; fi
grep -q KAFKA_STORAGE_UNFORMATTED "$TASK/replaced.log"
"${DC[@]}" -f "$TASK/replacement.yaml" down -v
# Source assertion plus direct entrypoint ensure no wrapper/format is reachable.
! grep -E 'KafkaDockerWrapper|/etc/kafka/docker/(run|launch)' "$REPO/deploy/docker/kafka/guarded-start.sh" | grep -v '^#'
echo "PASS guarded Kafka storage contract; project=$PROJECT evidence=$TASK"
