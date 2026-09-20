# HA / DR / Backup / Restore / Upgrades

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 03, 14, 15, 16, 17, 18

## Product objective

Keep Platform operational through expected node/service failures and planned maintenance, preserve recoverable authoritative state and support safe upgrade/rollback without corrupting monitoring or tenant data.

## Core HA

Customer-facing HA is exactly **two full Core nodes**:

- Active;
- Standby;
- one stable Core endpoint/VIP;
- continuous synchronization appropriate to each datastore/service;
- failover target below the existing governed objective;
- no third customer-visible full Core introduced merely to satisfy textbook quorum patterns.

Internal technologies are adapted to this two-node product topology and require explicit fencing/failover validation.

## Sensor behavior during Core failure

Sensors continue collection and bounded local buffering during transient Core failover/outage while certified storage capacity permits. Sensor health exposes backlog and remaining buffer horizon.

## Data-service HA

PostgreSQL, ClickHouse, Kafka, Valkey and Core application state each have explicit failure/promotion/recovery behavior. Do not assume one generic replication method applies to every service.

## DR

DR is mandatory in v1 architecture. DR design covers:

- authoritative control/config state;
- credentials metadata/references;
- inventory/identity;
- policies/workflows;
- telemetry according to configured protection/retention;
- package/config versions;
- recovery runbooks;
- post-recovery integrity validation.

Exact contractual RPO/RTO remains governed by the current Decision Register and evidence; do not invent values.

## Backup

Backups include the data classes required to rebuild control state and applicable historical evidence. Backup jobs are monitored, encrypted as required, validated and periodically restore-tested.

A "backup successful" status without restore evidence is insufficient.

## Restore

Restore validates:

- schema/version compatibility;
- tenant and identity integrity;
- credential references;
- entity relationships;
- policy/workflow state;
- database consistency;
- service startup;
- data freshness;
- HA role;
- audit continuity.

## Upgrades

Upgrade workflow supports:

- compatibility/preflight;
- backup/checkpoint;
- schema migrations;
- staged/canary rollout where applicable;
- Core Active/Standby sequencing;
- Sensor/Agent rollout;
- Pack compatibility;
- health verification;
- rollback/fail-safe.

No component is called upgraded until post-upgrade monitoring and data integrity are verified.

## Air gap

Release media includes pinned packages/images/artifacts, checksums, SBOMs and notices. Upgrade must work without public runtime downloads in air-gapped environments.

## Failure testing

Certification includes Core node failure, split-brain/fencing cases, service crash loops, datastore failover, network partition, Sensor backlog/replay, failed migration, rollback, backup corruption and restore drills.
