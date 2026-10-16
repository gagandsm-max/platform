#!/usr/bin/env bash
# Host gate. No Compose mutation before identity, provenance and legacy checks.
set -euo pipefail
REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)
fail() { printf 'KAFKA_DEPLOYMENT_%s: %s\n' "$1" "$2" >&2; exit 1; }
ACTION=${1:-check}; shift || true
DC=(docker compose "$@")
case $ACTION in check|provision|up|restart|down) ;; *) fail ARGUMENT 'Use check/provision/up/restart/down followed by Compose global options';; esac
[[ $(docker compose version --short) == 5.5.0 ]] || fail COMPOSE_VERSION 'Validated Compose version is 5.5.0; do not upgrade/downgrade the host automatically'
# Use the already governed Node tooling image, never install a host parser.
# Rendering and this networkless parser do not create/use Kafka resources.
CONFIG=$("${DC[@]}" config --format json) || fail CONFIG 'Cannot render requested configuration'
FIELDS=$(printf '%s' "$CONFIG" | docker run --rm -i --network none --cpus 0.25 --memory 128m node:20-alpine node -e '
  const fs=require("fs"), c=JSON.parse(fs.readFileSync(0,"utf8")), k=c.services.kafka;
  function requireThat(ok,msg){if(!ok)throw Error(msg)}
  requireThat(k && JSON.stringify(k.entrypoint)===JSON.stringify(["/bin/bash","/platform-kafka/guarded-start.sh"]),"Select compose.kafka-guarded.yaml explicitly; legacy deployment requires verified cold migration");
  requireThat(k.image==="apache/kafka:3.8.0" && (!k.command || k.command.length===0),"Unexpected image/command");
  const e=k.environment, v=c.volumes.platform_kafka_data;
  requireThat(!Object.hasOwn(e,"KAFKA_CLUSTER_ID"),"Inherited KAFKA_CLUSTER_ID was not removed");
  requireThat(e.KAFKA_LOG_DIRS==="/var/lib/kafka/data/logs" && e.KAFKA_METADATA_LOG_DIR===e.KAFKA_LOG_DIRS,"Unexpected persistent paths");
  requireThat(v.external===true && /^[A-Za-z0-9][A-Za-z0-9_.-]+$/.test(v.name) && v.name!=="platform_kafka_data","Use a deliberately identified external volume");
  requireThat(/^[A-Za-z0-9_-]{16,80}$/.test(e.PLATFORM_KAFKA_STORAGE_ID) && /^[A-Za-z0-9_-]{22}$/.test(e.CLUSTER_ID) && e.KAFKA_NODE_ID==="1","Retain explicit storage, actual cluster and node identity");
  requireThat(k.volumes.filter(x=>x.target==="/var/lib/kafka/data").length===1 && k.volumes.find(x=>x.target==="/var/lib/kafka/data").source==="platform_kafka_data","Unexpected storage mount");
  requireThat(k.volumes.filter(x=>x.target.startsWith("/var/lib/kafka/data/")).length===0,"Nested storage mount");
  const a=[c.name,k.container_name,v.name,e.PLATFORM_KAFKA_STORAGE_ID,e.CLUSTER_ID,e.KAFKA_NODE_ID];
  requireThat(a.every(x=>typeof x==="string" && /^[A-Za-z0-9_.-]+$/.test(x)),"Invalid deployment identity");
  for(const [key,value] of Object.entries(e).sort()){
    requireThat(/^[A-Z][A-Z0-9_]*$/.test(key) && !/[\n\r\x00]/.test(String(value)),"Invalid environment");
    a.push(key+"="+value);
  }
  process.stdout.write(a.join("\n"));
') || fail CONFIG 'Guarded configuration/identity is missing or invalid; see STORAGE_MIGRATION.md'
mapfile -t VALUES <<< "$FIELDS"
PROJECT=${VALUES[0]}; CONTAINER=${VALUES[1]}; VOLUME=${VALUES[2]}
STORAGE=${VALUES[3]}; CLUSTER=${VALUES[4]}; NODE=${VALUES[5]}
ENV_ARGS=()
for value in "${VALUES[@]:6}"; do ENV_ARGS+=(-e "$value"); done
inspect_volume() {
    docker volume inspect "$VOLUME" >/dev/null 2>&1 || fail VOLUME_MISSING 'Provision explicitly or recover the verified migrated volume; startup never creates it'
    local labels
    labels=$(docker volume inspect -f '{{index .Labels "issue223.owner"}}|{{index .Labels "platform.kafka.project"}}|{{index .Labels "platform.kafka.storage-id"}}|{{index .Labels "platform.kafka.resource"}}' "$VOLUME")
    [[ $labels == "$PROJECT|$PROJECT|$STORAGE|platform_kafka_data" ]] || fail OWNER 'Volume provenance does not match this deployment'
}
check_containers() {
    local cid info mounted running
    # Both service label and fixed name: never overlook a foreign named broker.
    local candidates by_project by_name by_volume
    by_project=$(docker ps -aq --filter "label=com.docker.compose.project=$PROJECT" --filter label=com.docker.compose.service=kafka) || fail INSPECTION 'Cannot inspect project containers'
    by_name=$(docker ps -aq --filter "name=^/${CONTAINER}$") || fail INSPECTION 'Cannot inspect named container'
    by_volume=$(docker ps -aq --filter "volume=$VOLUME") || fail INSPECTION 'Cannot inspect mounted volume users'
    candidates=$(printf '%s\n' "$by_project" "$by_name" "$by_volume" | sort -u)
    for cid in $candidates; do
        info=$(docker inspect -f '{{.Name}}|{{index .Config.Labels "com.docker.compose.project"}}|{{index .Config.Labels "com.docker.compose.service"}}|{{join .Config.Entrypoint " "}}|{{.Config.Image}}|{{.State.Status}}' "$cid")
        printf 'KAFKA_DEPLOYMENT_CONTAINER: %s %s\n' "$cid" "$info"
        # Read evidence without stopping/removing the container, including stopped ones.
        for path in /tmp/kafka-logs/meta.properties /tmp/kraft-combined-logs/meta.properties /tmp/platform-kafka-server.properties /opt/kafka/config/server.properties; do
            docker cp "$cid:$path" - 2>/dev/null | tar -xOf - 2>/dev/null | awk '/^(log.dirs|metadata.log.dir|cluster.id|node.id)=/ {print "KAFKA_DEPLOYMENT_STORED: " $0}' || true
        done
        [[ $ACTION != provision ]] || fail EXISTING_CONTAINER 'Fresh provisioning cannot adopt any existing broker/container'
        [[ $info == "/$CONTAINER|$PROJECT|kafka|/bin/bash /platform-kafka/guarded-start.sh|apache/kafka:3.8.0|"* ]] || fail MIGRATION_REQUIRED 'Legacy/foreign/unknown broker must be cold-backed-up and verified; no Compose mutation permitted'
        mounted=$(docker inspect -f '{{range .Mounts}}{{if eq .Destination "/var/lib/kafka/data"}}{{.Type}}|{{.Name}}{{end}}{{end}}' "$cid")
        [[ $mounted == "volume|$VOLUME" ]] || fail MOUNT 'Existing container uses different storage; governed cutover required'
        [[ -z $(docker inspect -f '{{range .Mounts}}{{println .Destination}}{{end}}' "$cid" | awk 'index($0,"/var/lib/kafka/data/")==1') ]] || fail MOUNT 'Existing container has a nested storage mount'
        for value in "${VALUES[@]:6}"; do
            docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$cid" | grep -Fxq -- "$value" || fail CONFIG_DRIFT 'Existing broker environment differs; reviewed cutover required'
        done
        if docker cp "$cid:/tmp/kafka-logs/meta.properties" - >/dev/null 2>&1 || docker cp "$cid:/tmp/kraft-combined-logs/meta.properties" - >/dev/null 2>&1; then
            fail OVERLAY_STATE 'Legacy container-overlay broker state is present; retain the container for migration'
        fi
        # Effective configuration must be inspectable, not inferred from desired env.
        local effective
        effective=$(docker cp "$cid:/tmp/platform-kafka-server.properties" - 2>/dev/null | tar -xOf - 2>/dev/null) || fail UNKNOWN 'Cannot read existing effective broker configuration'
        for value in log.dirs=/var/lib/kafka/data/logs metadata.log.dir=/var/lib/kafka/data/logs "node.id=$NODE"; do
            grep -Fxq -- "$value" <<< "$effective" || fail UNKNOWN 'Effective log/metadata/node configuration differs'
        done
    done
}
check_storage() {
    # Read-only mount; the inspection mode neither consumes permission nor formats.
    docker run --rm --network none --cpus 1 --memory 512m --user 1000:1000 \
        --tmpfs /etc/kafka/secrets --tmpfs /mnt/shared/config \
        --label "issue223.owner=$PROJECT" "${ENV_ARGS[@]}" \
        --mount "type=volume,src=$VOLUME,dst=/var/lib/kafka/data,readonly" \
        --mount "type=bind,src=$REPO/deploy/docker/kafka,dst=/platform-kafka,readonly" \
        --entrypoint /bin/bash apache/kafka:3.8.0 /platform-kafka/guarded-start.sh --inspect \
        || fail STORAGE 'Storage is unprovisioned, interrupted, inaccessible or mismatched; retain it for review'
}
check_containers
if [[ $ACTION == provision ]]; then
    [[ ${PLATFORM_KAFKA_NEW_VOLUME_CONFIRM:-} == "$STORAGE" ]] || fail AUTHORIZATION 'Export PLATFORM_KAFKA_NEW_VOLUME_CONFIRM equal to the retained new storage identity'
    VOLUME_NAMES=$(docker volume ls --format '{{.Name}}') || fail INSPECTION 'Cannot inspect existing volumes'
    if grep -Fxq "$VOLUME" <<< "$VOLUME_NAMES"; then fail COLLISION 'Fresh volume name already exists; never adopt or reformat it'; fi
    # No automatic retry/removal: interrupted creation is intentionally retained.
    docker volume create --label "issue223.owner=$PROJECT" --label "platform.kafka.project=$PROJECT" \
        --label "platform.kafka.storage-id=$STORAGE" --label platform.kafka.resource=platform_kafka_data "$VOLUME" >/dev/null
    inspect_volume
    check_containers
    docker run --rm --network none --cpus 0.25 --memory 128m --user 0 \
        --tmpfs /etc/kafka/secrets --tmpfs /mnt/shared/config \
        --label "issue223.owner=$PROJECT" --mount "type=volume,src=$VOLUME,dst=/var/lib/kafka/data" \
        --entrypoint /bin/bash apache/kafka:3.8.0 -c \
        'test -z "$(find /var/lib/kafka/data -mindepth 1 -maxdepth 1 -print -quit)" && chown 1000:1000 /var/lib/kafka/data' \
        || fail NONEMPTY 'New volume is no longer empty; retain for review'
    "${DC[@]}" run -T --rm --no-deps --user 1000:1000 -e "PLATFORM_KAFKA_NEW_VOLUME_CONFIRM=$STORAGE" \
        --entrypoint /bin/bash kafka /platform-kafka/provision-fresh.sh --new-empty-volume
    check_storage
    exit 0
fi
inspect_volume
check_storage
printf 'KAFKA_DEPLOYMENT_VERIFIED: project=%s volume=%s cluster=%s node=%s storage=%s\n' "$PROJECT" "$VOLUME" "$CLUSTER" "$NODE" "$STORAGE"
case $ACTION in
    check) ;;
    up) "${DC[@]}" up -d;;
    restart) "${DC[@]}" restart;;
    # Non-deleting teardown only. Volume removal is never a production preflight
    # action: matching STORAGE identity/labels is not disposable-resource authority.
    down) "${DC[@]}" down;;
esac
