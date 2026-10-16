#!/usr/bin/env bash
# Issue #223: never invoke the image's run/launch/KafkaDockerWrapper setup path.
set -euo pipefail
umask 077
ROOT=/var/lib/kafka/data
LOGS=$ROOT/logs
IDENTITY=$ROOT/.platform-storage
CONFIG=/tmp/platform-kafka-server.properties

fail() {
    printf '{"component":"platform-kafka","event_code":"%s","message":"%s"}\n' "$1" "$2" >&2
    exit 1
}

configure() {
    [[ ${CLUSTER_ID:-} =~ ^[A-Za-z0-9_-]{22}$ ]] || fail KAFKA_CLUSTER_ID_REQUIRED 'Set CLUSTER_ID from verified stored identity or explicit new provisioning'
    [[ ${PLATFORM_KAFKA_STORAGE_ID:-} =~ ^[A-Za-z0-9_-]{16,80}$ ]] || fail KAFKA_STORAGE_ID_REQUIRED 'Set the independently retained volume identity'
    [[ ${KAFKA_NODE_ID:-} =~ ^(0|[1-9][0-9]*)$ ]] || fail KAFKA_NODE_ID_REQUIRED 'Set the intended node identity'
    [[ -z ${KAFKA_CLUSTER_ID+x} ]] || fail KAFKA_LEGACY_CLUSTER_ID 'KAFKA_CLUSTER_ID is not CLUSTER_ID; do not copy its old value blindly'
    [[ ${KAFKA_LOG_DIRS:-} == "$LOGS" && ${KAFKA_METADATA_LOG_DIR:-} == "$LOGS" ]] || fail KAFKA_UNEXPECTED_DIRECTORY 'Both log.dirs and metadata.log.dir must use the governed volume logs directory'
    [[ ${KAFKA_PROCESS_ROLES:-} == broker,controller ]] || fail KAFKA_UNSUPPORTED_ROLE 'This guard owns the existing standalone combined broker only'
    # Deliberately bounded to this Compose broker contract, with no config overlay.
    local mapping env_name key value
    : > "$CONFIG"
    for mapping in \
        NODE_ID:node.id PROCESS_ROLES:process.roles LISTENERS:listeners \
        ADVERTISED_LISTENERS:advertised.listeners CONTROLLER_LISTENER_NAMES:controller.listener.names \
        LISTENER_SECURITY_PROTOCOL_MAP:listener.security.protocol.map CONTROLLER_QUORUM_VOTERS:controller.quorum.voters \
        OFFSETS_TOPIC_REPLICATION_FACTOR:offsets.topic.replication.factor \
        TRANSACTION_STATE_LOG_REPLICATION_FACTOR:transaction.state.log.replication.factor \
        TRANSACTION_STATE_LOG_MIN_ISR:transaction.state.log.min.isr \
        GROUP_INITIAL_REBALANCE_DELAY_MS:group.initial.rebalance.delay.ms NUM_PARTITIONS:num.partitions \
        LOG_DIRS:log.dirs METADATA_LOG_DIR:metadata.log.dir; do
        env_name=KAFKA_${mapping%%:*}; key=${mapping#*:}; value=${!env_name:-}
        [[ -n $value && $value != *$'\n'* && $value != *$'\r'* && $value != *'\'* ]] || fail KAFKA_CONFIG_INVALID 'Required broker configuration is absent or contains property escapes'
        printf '%s=%s\n' "$key" "$value" >> "$CONFIG"
    done
}

check_mount() {
    [[ ! -L $ROOT && $(readlink -f "$ROOT") == "$ROOT" ]] || fail KAFKA_MOUNT_INVALID 'Persistent root must not be a symbolic link'
    mountpoint -q "$ROOT" || fail KAFKA_MOUNT_MISSING 'Persistent Kafka mount is absent'
    # Reject a second mount below the root: files could otherwise escape this volume.
    ! awk -v p="$ROOT/" 'index($5,p)==1 {found=1} END {exit !found}' /proc/self/mountinfo || fail KAFKA_NESTED_MOUNT 'Unexpected nested Kafka mount'
    [[ -r $ROOT && -x $ROOT ]] && { [[ ${READ_ONLY:-0} == 1 ]] || [[ -w $ROOT ]]; } || fail KAFKA_STORAGE_INACCESSIBLE 'Kafka volume is inaccessible to broker user'
}

# Distinguish retention/compaction ENOENT from true inaccessibility.
# A path listed by find may vanish before the access check; that is not corrupt storage.
# Existing paths that fail -r/-w/-x, and symlinks (-L), remain fail-closed.
log_path_accessible() {
    local p=$1 read_only=${2:-0}
    if [[ -r $p ]] && { [[ $read_only == 1 ]] || [[ -w $p ]]; } && { [[ ! -d $p ]] || [[ -x $p ]]; }; then
        return 0
    fi
    [[ -e $p || -L $p ]] || return 0
    return 1
}

check_files() {
    [[ -d $LOGS && ! -L $LOGS ]] || fail KAFKA_STORAGE_UNFORMATTED 'Persistent logs directory is missing'
    local invalid
    invalid=$(timeout 60 find "$LOGS" -type l -print -quit) || fail KAFKA_STORAGE_INACCESSIBLE 'Cannot inspect persistent files within startup bound'
    [[ -z $invalid ]] || fail KAFKA_STORAGE_INACCESSIBLE 'Persistent storage contains links or inaccessible files'
    # Single bounded walk (no rescans/retries). Vanished mutable entries are skipped;
    # EACCES, present-but-unreadable paths and symlinks still fail closed.
    timeout 60 bash -o pipefail -c '
        source "$1"
        find "$LOGS" -print0 | while IFS= read -r -d "" p; do
            log_path_accessible "$p" "$2" || exit 1
        done
    ' bash "${BASH_SOURCE[0]}" "${READ_ONLY:-0}" || fail KAFKA_STORAGE_INACCESSIBLE 'Persistent files are inaccessible to broker user'
    [[ -f $LOGS/meta.properties && -s $LOGS/bootstrap.checkpoint ]] || fail KAFKA_METADATA_MISSING 'Kafka format metadata or bootstrap checkpoint is missing'
    local key wanted actual
    for key in cluster.id node.id version directory.id; do
        case $key in
            cluster.id) wanted=$CLUSTER_ID;;
            node.id) wanted=$KAFKA_NODE_ID;;
            version) wanted=1;;
            directory.id) wanted='';;
        esac
        actual=$(awk -F= -v k="$key" '$1==k {n++; v=$2} END {if(n!=1) exit 1; print v}' "$LOGS/meta.properties") || fail KAFKA_METADATA_INVALID 'Missing or duplicate metadata identity'
        if [[ $key == directory.id ]]; then
            [[ $actual =~ ^[A-Za-z0-9_-]{22}$ ]] || fail KAFKA_METADATA_INVALID 'Invalid Kafka directory identity'
        else
            [[ $actual == "$wanted" ]] || fail KAFKA_IDENTITY_MISMATCH 'Stored cluster, node or format version differs from intended identity'
        fi
    done
    if ! timeout 30 /opt/kafka/bin/kafka-storage.sh info -c "$CONFIG" > /tmp/platform-kafka-storage-info.log 2>&1; then
        tail -n 20 /tmp/platform-kafka-storage-info.log >&2
        fail KAFKA_METADATA_INVALID 'Kafka storage info failed'
    fi
}

identity_text() {
    printf 'version=1\nstorage.id=%s\ncluster.id=%s\nnode.id=%s\n' "$PLATFORM_KAFKA_STORAGE_ID" "$CLUSTER_ID" "$KAFKA_NODE_ID"
    sha256sum "$LOGS/meta.properties" "$LOGS/bootstrap.checkpoint"
}

check_identity() {
    [[ -f $IDENTITY && ! -L $IDENTITY && -r $IDENTITY ]] || fail KAFKA_STORAGE_UNATTESTED 'No sealed fresh-provisioning or verified-migration identity'
    cmp -s "$IDENTITY" <(identity_text) || fail KAFKA_IDENTITY_MISMATCH 'Volume identity or immutable Kafka metadata checksum differs'
}

require_metadata_log() {
    [[ -d $LOGS/__cluster_metadata-0 ]] || fail KAFKA_MIGRATION_INCOMPLETE 'Previously started or migrated storage lacks KRaft metadata log'
    local segment
    segment=$(find "$LOGS/__cluster_metadata-0" -maxdepth 1 -name '*.log' -type f -print -quit)
    [[ -n $segment ]] || fail KAFKA_MIGRATION_INCOMPLETE 'KRaft metadata segments are missing'
}

seal_identity() {
    if [[ ${OWN_PENDING:-0} != 1 ]]; then
        (set -o noclobber; printf 'sealing\n' > "$IDENTITY.pending") || fail KAFKA_PROVISION_INCOMPLETE 'Another action reserved the volume'
    fi
    identity_text > "$IDENTITY.pending"
    sync
    # Atomic publish without replacing a seal created by a competing action.
    ln "$IDENTITY.pending" "$IDENTITY" || fail KAFKA_PROVISION_INCOMPLETE 'Identity already exists; refusing to replace it'
    rm "$IDENTITY.pending"
    sync
}

verify_migration_manifest() {
    [[ -f $ROOT/.migration.sha256 && ! -L $ROOT/.migration.sha256 ]] || fail KAFKA_MIGRATION_MANIFEST_REQUIRED 'Supply the independently verified complete cold-copy manifest'
    # Exact inventory and hashes, not merely a subset accepted by sha256sum -c.
    (cd "$ROOT"; timeout 300 bash -o pipefail -c 'find logs -type f -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum') > /tmp/platform-kafka-migration.sha256 || fail KAFKA_MIGRATION_INVALID 'Cold-copy inventory hashing failed'
    cmp -s "$ROOT/.migration.sha256" /tmp/platform-kafka-migration.sha256 || fail KAFKA_MIGRATION_INVALID 'Cold-copy manifest, file inventory or hashes differ'
}

seal_migration() {
    [[ ! -e $IDENTITY && ! -e $IDENTITY.pending && ! -e $ROOT/.fresh && ! -e $ROOT/.migration-ready ]] || fail KAFKA_MIGRATION_ALREADY_SEALED 'Migration sealing requires an unsealed cold copy'
    check_files
    require_metadata_log
    verify_migration_manifest
    identity_text > "$ROOT/.migration-ready"
    seal_identity
    printf '{"component":"platform-kafka","event_code":"KAFKA_MIGRATION_SEALED","message":"Cold-copy hashes and identity verified; no broker started"}\n'
}

main() {
    [[ ${1:-} != --inspect ]] || READ_ONLY=1
    [[ $# -le 1 ]] || fail KAFKA_ARGUMENT_INVALID 'Guard accepts only --check, --inspect or --seal-migration'
    configure
    check_mount
    case ${1:-} in
        --seal-migration) seal_migration; return;;
        ''|--check|--inspect) ;;
        *) fail KAFKA_ARGUMENT_INVALID 'Guard accepts only --check, --inspect or --seal-migration';;
    esac
    [[ ! -e $IDENTITY.pending ]] || fail KAFKA_PROVISION_INCOMPLETE 'Interrupted provisioning or migration requires operator review'
    check_files
    check_identity
    if [[ -e $ROOT/.migration-ready ]]; then
        [[ ! -e $ROOT/.fresh && ! -L $ROOT/.migration-ready ]] && cmp -s "$ROOT/.migration-ready" "$IDENTITY" || fail KAFKA_MIGRATION_INVALID 'Migration startup authorization is invalid'
        verify_migration_manifest
    fi
    if [[ -e $ROOT/.fresh ]]; then
        [[ ! -L $ROOT/.fresh ]] && cmp -s "$ROOT/.fresh" "$IDENTITY" || fail KAFKA_PROVISION_INCOMPLETE 'Fresh-start authorization is invalid'
    else
        require_metadata_log
    fi
    [[ ${1:-} != --check && ${1:-} != --inspect ]] || return 0
    # Consume the explicit one-time permission before starting. A crash before
    # KRaft log creation fails closed on retry; never infer freshness again.
    if [[ -e $ROOT/.fresh ]]; then
        rm "$ROOT/.fresh"
        sync
    fi
    if [[ -e $ROOT/.migration-ready ]]; then
        rm "$ROOT/.migration-ready"
        sync
    fi
    printf '{"component":"platform-kafka","event_code":"KAFKA_STORAGE_VERIFIED","message":"Starting directly without vendor setup or formatting"}\n'
    exec /opt/kafka/bin/kafka-server-start.sh "$CONFIG"
}

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then main "$@"; fi
