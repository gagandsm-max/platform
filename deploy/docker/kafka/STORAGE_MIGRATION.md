# Guarded Kafka storage — Issue #223

This is a source/development-Compose change for the existing standalone combined
Kafka node. It does not execute a customer migration, certify native installation,
HA, host-reboot durability, catastrophic storage loss, or acknowledged-telemetry
RPO. Master/independent review remains required. Governing references:
Platform_00 §6, Platform_02 PRD-DEV-001/002 and §5.4, Platform_03 §4,
Platform_04 §10, Platform_05 §6, Platform_07 OSS-002, Platform_13,
Platform_16 upgrade/rollback/runbooks and Platform_17 Kafka contracts.

## Verified image and startup model

Inspected `apache/kafka:3.8.0` image digest:
`sha256:c89f315cff967322c5d2021434b32271393cb193aa7ec1d43e97341924e57069`.
The existing image tag remains unchanged; this inspected digest is evidence,
not a new image selection or assurance that a mutable tag cannot drift.

Image entrypoint `/__cacert_entrypoint.sh` ultimately execs its command.
Default command `/etc/kafka/docker/run` sources `configureDefaults`, `configure`,
then `launch`. `configureDefaults` supplies `CLUSTER_ID=5L6g3nShT-eMCtK--X86sw`
if unset. `launch` calls `kafka.docker.KafkaDockerWrapper setup`, which prepares
properties then calls `StorageTool.main(format --cluster-id=CLUSTER_ID -c ...)`,
before broker start. The launcher tolerates an "already formatted" error.
See the pinned upstream [wrapper source](https://github.com/apache/kafka/blob/3.8.0/core/src/main/scala/kafka/docker/KafkaDockerWrapper.scala)
and [launch source](https://github.com/apache/kafka/blob/3.8.0/docker/jvm/launch).

When KAFKA_* properties are supplied, the wrapper writes those properties rather
than merging the default server file. With `log.dirs` absent, Kafka uses its
`/tmp/kafka-logs` default. The image's shipped default server file separately
names `/tmp/kraft-combined-logs`; neither path is the intended persistent mount.
`KAFKA_CLUSTER_ID` becomes a broker property, not the format tool's `CLUSTER_ID`.
**Do not substitute the old Compose value for a captured stored cluster ID.**

Only the explicit `compose.kafka-guarded.yaml` overlay overrides the entrypoint.
Default `compose.yaml` retains the exact pre-PR Kafka service configuration;
applying this source patch alone cannot introduce a legacy service recreation.
`guarded-start.sh` builds the bounded
existing broker configuration itself and invokes `kafka-server-start.sh`
directly. Neither normal startup nor validation invokes vendor setup or format.
Both `log.dirs` and `metadata.log.dir` are `/var/lib/kafka/data/logs`. This stores
KRaft metadata, topic segments, consumer offsets and transaction state together
inside the explicitly provisioned/migrated external named volume. Listener ports,
network, profiles, replication
settings and image are retained. Unknown broker tuning requires a separately
reviewed change to the explicit configuration mapping; mounted vendor config
files are not applied by this path.

## Host gate and staged deployment

Use Compose **5.5.0**, whose `!reset null` environment merge behavior is tested:
the overlay removes inherited `KAFKA_CLUSTER_ID` entirely. An empty string or an
additional `CLUSTER_ID` alongside the inherited key is not equivalent.
No host installation/version change is performed by this work. The host gate
uses the existing governed `node:20-alpine` tooling image as a networkless JSON
parser with CPU/memory bounds, not a host Node runtime or a new dependency.
Docker may fetch these already approved images on a fresh host; it never installs
host runtimes. Unused image-declared config/secrets volumes are backed by tmpfs
in the guarded service and helpers, avoiding unlabelled anonymous-volume cleanup.

`deployment-preflight.sh check|provision|up|restart|down` accepts Compose
global options after the action. `.env`/`COMPOSE_FILE` selection also works.
Volume deletion is **not** a production preflight action: matching ownership
labels or `PLATFORM_KAFKA_STORAGE_ID` is never deletion authorization.
Every governed startup checks the exact rendered configuration first. The gate
inspects both project/service and fixed-name container identities (running or
stopped), mounted volume, stored cluster/node/directory seal, effective properties,
legacy overlay metadata and deployment/volume ownership. Unknown configuration,
missing storage, any legacy container or mismatched identity rejects before
Compose mutation. A separate read-only inspector container uses the approved
Kafka image and a read-only data mount; it cannot format or consume first-start
authorization. Neither telemetry topic offsets nor destination emptiness proves
that an installation is fresh. Inspection never removes a container. `down`
tears down Compose services without removing the external Kafka volume.

Direct ungoverned `docker compose up` cannot be made safe by an in-container
guard. Do not apply this overlay to an existing installation until the separate
verified cold migration/cutover below is accepted. One deployment operator/writer
must hold exclusive lifecycle ownership throughout check and mutation; a Docker
administrator racing mounts/labels/source edits is outside this gate's authority.

## Identity and failure behavior

Retain these deployment values independently of the Kafka volume (e.g. approved
installation configuration and backup record):

- `PLATFORM_KAFKA_DATA_VOLUME`: approved named volume.
- `PLATFORM_KAFKA_CLUSTER_ID`: actual stored Kafka cluster ID; fresh installations
  generate a new Kafka UUID with the pinned image's `kafka-storage.sh random-uuid`.
- `PLATFORM_KAFKA_STORAGE_ID`: unique 16–80 character `[A-Za-z0-9_-]` volume
  identity, selected and retained by the operator; never reuse for a replacement.
- Kafka node ID (currently `1`) and matching quorum/listener configuration.

There is no fallback identity. Empty means uninitialized, never authorized.
The guard requires an exact mount at `/var/lib/kafka/data`, rejects nested mounts,
links and inaccessible files, enforces the sole log directory, validates Kafka
metadata with `kafka-storage.sh info`, and compares the complete sealed identity
including SHA-256 of immutable `meta.properties` and `bootstrap.checkpoint`.
KRaft v1 cluster/node/directory identity is checked explicitly. A valid identity
is required in addition to a mount; the image's automatic anonymous volume is
not accepted merely because it is mounted.

Errors have stable `KAFKA_*` event codes. Enumeration and Kafka storage-info calls
have finite timeouts; Docker logs retain existing size/count limits. The existing
restart policy remains: retrying a failed guard cannot call the format tool or
broker. Inspect failure logs and stop retrying the failed deployment pending
operator recovery. Do not disable the guard to satisfy readiness.

A `.platform-storage.pending` marker means interrupted provisioning/sealing and
requires manual review. A `.fresh` authorization is consumed before the first
broker invocation. If startup crashes before creating the KRaft metadata log,
subsequent startup fails closed; inspect and recover that explicitly new resource
under operator approval. No automatic reset or format is available.

The seal proves the verified cold-copy inventory at acceptance time, not permanent
checksums of mutable topic logs. Subsequent runtime segment recovery/integrity is
Kafka's responsibility. A malicious actor with write access to both external
identity configuration and volume contents is outside this guard's trust boundary.
This is not a signature, an effect-store ACK proof, or a substitute for backups.

## A. Explicit fresh deployment (never for an existing installation)

1. Confirm the deployment is intentionally new, identify the exact volume and
   ownership, and confirm no broker mounts it. Retain intended cluster/node/storage
   identities outside the volume. Set the three PLATFORM_KAFKA_* values above in
   deployment configuration. Do not use defaults from the former Compose file.
   Generate the new cluster UUID without selecting or mounting any broker state:

   ```sh
   docker run --rm --network none --tmpfs /etc/kafka/secrets \
     --tmpfs /mnt/shared/config --tmpfs /var/lib/kafka/data --entrypoint /bin/bash apache/kafka:3.8.0 \
     -c '/opt/kafka/bin/kafka-storage.sh random-uuid'
   ```

   Retain the returned UUID and a separately generated storage identity in `.env`.
2. Set `COMPOSE_FILE=compose.yaml:compose.kafka-guarded.yaml` in `.env`, with
   the retained unique identities and Compose project. The target volume must
   **not exist**. Explicitly authorize its creation and provisioning:

   ```sh
   PLATFORM_KAFKA_NEW_VOLUME_CONFIRM=<retained-new-storage-identity> make kafka-provision-fresh
   bash deploy/docker/kafka/deployment-preflight.sh check
   make dev-up
   ```

   The confirmation variable is a deliberate invoking-shell authorization;
   it is not a saved default or ordinary startup action. The host gate refuses
   existing volumes/containers, then creates the volume with exact
   `issue223.owner`, `platform.kafka.project`, `platform.kafka.storage-id` and
   `platform.kafka.resource` labels. It checks ownership before initializing
   permissions and invokes the explicit provisioner as UID/GID 1000:1000.
   Provisioning formats only an empty
   mounted root after explicit authorization, without `--ignore-formatted`, then
   validates and seals identity. Any nonempty volume is rejected without mutation.
3. Confirm readiness, cluster/node identity, persistent metadata path, and initial
   topic creation. Archive provisioning evidence. Do not provision on restart.

CI performs this action only after verifying its `ci_<run>_<attempt>_kafka_data`
volume name and rejecting a preexisting volume. It generates a new cluster ID,
records cluster/storage identities in its run-local environment, provisions once,
then invokes the existing integration suite. Its Compose down/up retains that
same named volume and identities; no provisioning hook runs on container recreation.

## B. Existing deployment migration — operator runbook, NOT executed here

### Preconditions and capture

1. Schedule downtime and establish the rollback owner, scope, approved source
   broker/container, target volume and available space. Keep version 3.8.0 and
   node ID unchanged. Do not use fresh provisioning. Do not delete/recreate the
   old container: its overlay may hold the only complete Kafka state.
2. Capture live Admin/Kafka evidence before quiescence: actual cluster ID and node
   identity, effective `log.dirs` and `metadata.log.dir`, all `meta.properties`
   (including directory IDs), metadata version/quorum state, exact topic IDs,
   partition assignments/configs, start/end offsets, consumer-group committed
   offsets, lag, transactional IDs/producer IDs/epochs/states and active producers.
   Include internal `__consumer_offsets` and `__transaction_state` topics.
   Capture Core ingress watermark/ACK evidence and selected acknowledged telemetry
   identifiers/readback baselines, plus PostgreSQL/ClickHouse reconciliation state.
3. Quiesce Core producers AND consumers using the governed service procedure.
   Stop new Core acceptance, finish or explicitly resolve in-flight transactions,
   drain outstanding client operations and record the final offsets/state snapshot.
   Ensure no clients can reconnect during migration. Preserve Sensor/Agent spools,
   identities and replay windows; never delete spooled telemetry to make a check pass.
4. Stop the broker gracefully **without deleting its container or storage**.
   Verify process termination and no open writer/second mount. Retain its logs,
   image digest/config and container inventory. Never `compose down -v`, `rm -v`,
   or reset the deployment as a migration step.

### Cold backup and copy

5. Make complete cold backups of every configured Kafka log directory and the
   KRaft metadata directory, including hidden files, bootstrap/meta/checkpoint
   files, indexes, segments, producer snapshots, partition metadata, recovery
   checkpoints and ALL internal topics. Back up configuration and identity
   evidence separately. Backup Core datastore state consistently where required
   by the approved DR profile. Do not back up only user-topic `.log` files.
6. Generate an independent full file inventory and SHA-256 manifest from the cold
   source; verify backup inventory/hashes and restore readability. Store the
   backup plus manifest off the source and target volume, under approved custody.
   Record source digest, identities, sizes, ownership, time and command exits.
7. If all state is in one source directory (the inspected CI model), copy its
   entire contents into `<approved-volume>/logs`, preserving bytes and relevant
   ownership/modes; broker runs as image UID/GID `1000:1000`. Ensure root/logs are
   accessible. Only approved, independently verified offline permissions repairs
   are permitted. Never change metadata identity to make validation pass.
   **STOP** if the source has multiple log dirs, distinct metadata dirs, symlinks,
   mismatched identities or missing files: this bounded single-directory path
   does not authorize flattening or merging layouts. Obtain a reviewed mapping.
8. Generate the mapped source manifest using paths relative to the target root:
   every regular file starts with `logs/`. Sort NUL-delimited paths with
   `LC_ALL=C sort -z` and hash with `xargs -0 -r sha256sum`. Compare the complete
   target inventory/hashes to that independently retained source manifest. No
   subset check. Place the verified manifest at target `.migration.sha256`.
   No `.fresh`, `.platform-storage` or pending identity may be copied from a
   different deployment. Retain the source `meta.properties` and bootstrap bytes
   unchanged. Do not run a format command.

### Seal and guarded start

The cold target must have the four deployment ownership labels described above,
with a separately retained NEW storage identity and the captured ACTUAL cluster
and node IDs. Establish these as part of the independently approved offline copy,
never by adopting arbitrary unlabelled existing storage in the fresh provisioner.
Before cutover, retain the stopped legacy container and all original bytes for
rollback. A reviewed cutover may archive/rename that stopped container and select
a distinct Compose project, retaining the original project/config/identity record;
do not delete it. The host gate intentionally refuses any remaining legacy broker
under the requested project/service/name or any foreign mount of the target.
Archival/cutover is a separate approved operation, not an automatic preflight action.

9. Set external expected cluster ID to the **captured actual stored value**, node
   ID to the captured node, and storage ID to a new retained migration identity.
   With source/target brokers stopped and no other writers, run:

   ```sh
   # Only after approved cold-copy/provenance validation and source archival:
   # explicitly select the guarded overlay and the NEW deployment project.
   docker compose -f compose.yaml -f compose.kafka-guarded.yaml \
     run -T --rm --no-deps --entrypoint /bin/bash kafka \
     /platform-kafka/guarded-start.sh --seal-migration
   bash deploy/docker/kafka/deployment-preflight.sh check
   make dev-up
   ```

   Sealing validates identity, presence of KRaft logs, and exact cold-copy manifest
   inventory/hashes (300-second bound), then atomically publishes the identity.
   It neither formats nor starts Kafka. A `.migration-ready` authorization also
   requires full inventory/hash verification on `--check` and immediately before
   the first guarded broker start, when that one-time authorization is consumed.
   Thus damage between sealing and first startup is rejected. A timeout requires operator review and a
   separately reviewed capacity adjustment, never a skipped check. Archive the
   seal and manifest externally. After broker startup, mutable-file hashes change;
   the cold-copy manifest remains acceptance evidence, not a recurring runtime check.
10. Keep Core traffic quiesced. Verify exact cluster/node/directory/topic identities,
    topic partition counts/configs, start/end offsets, committed group offsets,
    transaction producer IDs/epochs/state and committed readback. Confirm KRaft and
    all topic paths physically reside under the named volume. Verify captured
    acknowledged telemetry readback against Kafka and the relevant Core durable
    watermark/effect store; metadata identity alone does not prove that data.
    Compare the full inventories, not a single healthcheck. Any discrepancy is HOLD.
11. Obtain governed acceptance, reopen Core consumers/producers in controlled order,
    permit Sensor spool replay and watch lag/duplicates/errors/space. Verify new
    accepted telemetry readback and known idempotent effects. Capture new baseline.

### Rollback boundaries

- **Before accepting new traffic:** stop the guarded broker, preserve target
  evidence, confirm no target writes beyond broker recovery/verification, and
  restart the retained original container/storage/config under its original
  identities. Never allow both brokers to own the same cluster. Reconcile readback
  and offsets before reopening clients. The verified cold backup is the fallback
  if original bytes changed or original storage is unavailable.
- **After accepting new traffic:** the original cold source is stale. Do not
  switch back blindly or copy only user-topic records. Quiesce Core again, resolve
  transactions, capture the new offsets/watermarks, stop the target and take a
  complete verified cold backup of the new authoritative state. Obtain a reviewed
  reverse migration/restore with compatible Kafka/config and all KRaft/internal
  topics plus Core watermark/effect reconciliation. Preserve Sensor spools. If new
  ACK-authorized records cannot be accounted for, HOLD and escalate; do not discard
  them, reset offsets or fabricate continuity. Retain both copies until governed
  reconciliation and acceptance permit archival disposal.

## Local verification and limits

`tests/run-kafka-storage-contract-test.sh` creates a unique isolated Compose
project, no host broker ports, and only uniquely named owner-labelled disposable
volumes. It tests fresh authorization, nonempty rejection without byte mutation,
identity/path/mount/permission failures, restart/recreation, topic IDs, committed
record readback, committed offsets and transaction ID/producer ID/epoch/state.
It kills an actual disposable provisioner after reservation using a paused
format-tool fixture and proves the retained pending state prevents startup.
It also verifies cold-copy sealing rejects incomplete inventories and corrupted
files, and verifies readback after sealing. Cleanup is limited to its project.
No developer, customer or running CI broker is selected by the harness.
It rejects a pre-existing primary name and verifies exact owner/project/resource/
storage labels before any volume removal. Teardown never uses `down -v` or global
prune. Negative coverage proves `cleanup` is rejected by the production host gate
even when `PLATFORM_KAFKA_DISPOSABLE_CONFIRM` equals the retained storage identity,
and that `down` leaves the owned volume intact. A foreign-provenance fixture proves
refusal before independently verified fixture cleanup. CI uses the same explicit
provisioning and non-deleting preflight (`check|provision|up|restart|down`) with
run/attempt-scoped identities; Kafka and other CI volumes are removed only by
strictly CI-scoped job logic after `GITHUB_ACTIONS` proof, exact run identity
binding, ownership/Compose provenance and no remaining container attachments.
Incomplete/unattested provisioning is retained for review, not silently cleaned up.
Archival disposal of a real installation remains a separate reviewed operator
procedure outside `deployment-preflight.sh`.

Full Platform CI and the existing 20-test suite must run on the proposed commit
in an isolated CI job. Native packaging, host reboot/crash/fsync durability,
80% storage protection, HA/DR certification and real installation migration are
separate gates; this source change must not be used to close them.
