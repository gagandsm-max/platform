#!/usr/bin/env bash
# Explicit one-time action. Not a Compose dependency or ordinary startup path.
set -euo pipefail
source /platform-kafka/guarded-start.sh
[[ $# == 1 && $1 == --new-empty-volume ]] || fail KAFKA_PROVISION_AUTH_REQUIRED 'Explicit --new-empty-volume authorization required'
configure
check_mount
[[ ${PLATFORM_KAFKA_NEW_VOLUME_CONFIRM:-} == "$PLATFORM_KAFKA_STORAGE_ID" ]] || fail KAFKA_PROVISION_AUTH_REQUIRED 'Confirm the intended NEW volume identity independently'
# Empty is a precondition, never the authority to provision.
[[ -z $(find "$ROOT" -mindepth 1 -maxdepth 1 -print -quit) ]] || fail KAFKA_PROVISION_NONEMPTY 'Refusing to format any nonempty volume'
# Reserve before formatting. Interrupted actions cannot be silently retried.
(set -o noclobber; printf 'provisioning\n' > "$IDENTITY.pending") || fail KAFKA_PROVISION_INCOMPLETE 'Another action reserved the volume'
OWN_PENDING=1
sync
mkdir "$LOGS"
if ! timeout 60 /opt/kafka/bin/kafka-storage.sh format --cluster-id "$CLUSTER_ID" -c "$CONFIG" > /tmp/platform-kafka-format.log 2>&1; then
    tail -n 20 /tmp/platform-kafka-format.log >&2
    fail KAFKA_PROVISION_FAILED 'Formatting failed; retain volume for operator review'
fi
check_files
# Fresh permission is complete before the identity is sealed; pending prevents
# startup after any interrupted write. No --ignore-formatted, no vendor setup.
identity_text > "$ROOT/.fresh"
seal_identity
check_identity
printf '{"component":"platform-kafka","event_code":"KAFKA_FRESH_PROVISIONED","message":"New persistent storage formatted, validated and sealed; no broker started"}\n'
