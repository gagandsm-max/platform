---
name: ha_dr
description: Exactly-two-Core Active/Standby HA/DR operability, failover evidence and Sensor continue-to-collect behavior without inventing unresolved replication designs.
---

<!-- generated from project_agents/skills/ha_dr/SKILL.md sha256:7c8b8c9e292c4d361df2a6ec0cf2b29465546fad60efd80b33c7b6feeda5ae24 -->

# ha_dr

## Purpose

Preserve Platform’s certified HA shape—exactly two full Core nodes in Active/Standby—and produce failover evidence without inventing unresolved replication, fencing or quorum primitives.

## Use when

Any change to Core failover, standby promotion, lease/fencing language, HA runbooks, DR drills, Core restart under load, or claims about RPO/RTO / Sensor behaviour during Core outage. Required for `platform-sre`, HA-touched Core work and BUILD-120+ dependency discussions.

## Authoritative inputs

Always read:

1. `project_documentation/Platform_03_Architecture.md` — exactly-two-Core Active/Standby; Sensors continue collecting/buffering
2. Root `AGENTS.md` HA non-negotiables (<60s failover target; 80% storage protection; absolute zero-loss not promised)
3. `project_documentation/Platform_11_Sizing_Capacity_Storage_Master.md` for capacity during continue-to-collect
4. Task `stopConditions`, migration reservations and any BUILD-120+ Decision Register entries for unresolved HA primitives
5. Existing failover/drill evidence for the exact head under test

Do not treat blog HA patterns, three-node quorums or invented Kafka/CH/PG fencing designs as authoritative before Decision Register approval.

## Invariants

- HA target is **exactly two full Core nodes**: Active/Standby. Standalone labs may run one Core; production HA is not “N+1 of anything”.
- Split-brain prevention and fencing/stale-writer exclusion are mandatory topics; unresolved mechanisms stay labelled P0 spike—not silently coded as truth.
- Failover target remains **<60 seconds** where claimed; claims require measurement, not aspiration.
- Backup/restore is **not** HA. Standby availability is **not** a substitute for backup/PITR.
- Sensors continue collecting/buffering during Core failover while capacity permits (80% storage protection). Absolute zero-loss is not promised.
- DB (PostgreSQL), Kafka and ClickHouse dependency failures must be classified separately from Core process death.
- Former-active return after promotion must not create dual-active writers without fencing/exclusion evidence.
- Unresolved HA primitives must not be invented before BUILD-120+ / Decision Register decisions.

## Required procedure

1. State whether the task touches standalone (1 Core) or HA (exactly two full Cores) topology and name active vs standby roles.
2. Enumerate failure modes in scope: active process loss, active loss during write, standby crash, network partition, PostgreSQL/Kafka/ClickHouse dependency failure, and planned failover.
3. For each mode, document expected fencing / stale-writer exclusion / promotion trigger and who may write.
4. Prove or explicitly `UNVERIFIED` the <60s failover target with timed drill evidence on the exact head.
5. Record Sensor continue-to-collect/buffer behaviour, spool pressure and 80% protection interaction during the Core outage window.
6. Separate RPO/RTO evidence (measured) from marketing language; tie RPO to durable boundaries (spool/Kafka/PG), not hopes.
7. On standby promotion, verify former-active rejoin path: demotion, catch-up, and exclusion of stale writers.
8. If the change needs unresolved replication/fencing primitives, stop and route to Decision Register / BUILD-120+—do not ship a speculative design.

## Negative / failure cases

- Dual-active acceptance of writes (split-brain) after partition.
- Standby promoted while old active still commits without fencing.
- Declaring HA healthy because backups exist.
- Inventing three-node quorum, etcd, or undocumented Kafka fencing “for now”.
- Ignoring Sensor buffer overflow / 80% protection during prolonged Core outage.
- Counting dependency outage (PG/Kafka/CH) as identical to Core process failover without separate evidence.
- Publishing RPO=0 or RTO claims without drill measurement.

## Security / tenant implications

Failover must not drop tenant isolation, mTLS identity or RBAC enforcement on the promoted node. Secrets/Credential Profiles needed on standby follow `secrets_handling`—no emergency plaintext. Audit which node was active across the cutover. Cross-tenant leakage during catch-up is a stop condition.

## Required evidence

- Topology declaration (exactly two Cores) and role timeline
- Timed failover/promotion drill (<60s or explicit miss)
- Fencing/stale-writer exclusion result or `UNVERIFIED` + Decision Register pointer
- Sensor continue-to-collect/spool evidence during outage
- Dependency-failure classification (PG/Kafka/CH) when in scope
- RPO/RTO measurement method and numbers, or `NOT_RUN`
- Exact SHA/commands; claim types via `evidence_bundle`

## Stop conditions

Stop when:

- Design requires a third Core, novel quorum or unresolved fencing primitive without BUILD-120+ authority
- Split-brain cannot be excluded from the proposed change
- Failover evidence cannot be produced yet a <60s / zero-loss claim is requested
- Mother documents conflict on HA shape and precedence is unresolved

## Anti-patterns

- “We’ll use the standby as the backup”
- Coding speculative replication and calling it certified HA
- Silencing Sensor backpressure alerts during drills
- Equating `mergeable` CI green with HA readiness
- Writer self-certifying HA drill evidence

## Handoff / output

Return: failure-mode matrix; drill timings; fencing status; Sensor buffer outcome; RPO/RTO evidence or `UNVERIFIED`; Decision Register gaps; next verifier (`platform-sre` / independent auditor). Do not mark MERGE_READY from the HA implementer alone.

## Non-ownership

Does not invent unresolved two-Core replication/fencing designs, authorize production failover, replace `backup_restore`, or certify MERGE_READY/COMPLETE.
