---
name: monitoring_work_units
description: Monitoring work-unit placement, Sensor assignment, capacity impact and locality constraints.
---

# monitoring_work_units

## Purpose

Schedule and own monitoring work units with fair leases, bounded concurrency and honest UNKNOWN/maintenance semantics—without poll storms or silent lease theft.

## Use when

Poll scheduling, Sensor assignment, work-unit leases/fencing, adaptive intervals, maintenance windows, or capacity placement changes. Required for Sensor runtime and Core scheduling paths (`platform-sensor-runtime-engineer`).

## Authoritative inputs

Always read:

1. Existing scheduler/lease/watermark code on the exact head (e.g. wall-clock lease semantics)
2. Architecture notes on Sensor locality and work-unit placement
3. `Platform_11` capacity and polling-interval guidance
4. Maintenance window / expected-state semantics in alert masters when present
5. Storage-protection interaction when backlog grows

If adaptive ML-based intervals or advanced fair-queuing are not implemented, mark `UNVERIFIED`.

## Invariants

- Poll scheduling is deterministic enough to reason about; jitter is intentional to prevent stampedes.
- Lease/ownership of work units is fenced; dual-active pollers for the same unit are defects.
- Adaptive intervals (if present) stay within safety bounds—never starve critical polls silently.
- Fairness across tenants/devices prevents one hot target from dominating a Sensor.
- Jitter/storm prevention on restart and mass rediscovery is mandatory.
- Retries are budgeted; they must not amplify into poll storms.
- Backpressure interacts with Sensor capacity and 80% storage protection.
- **UNKNOWN** / collector-degraded semantics are distinct from DOWN.
- Maintenance windows produce EXPECTED_STATE (or equivalent)—not false outages—when implemented; otherwise `UNVERIFIED`.
- Bounded concurrency per Sensor/protocol worker.

## Required procedure

1. Identify work-unit types in scope and their lease/ownership mechanism in code.
2. Verify clock/lease predicates (prefer DB wall clock where leases matter).
3. Define interval, jitter and concurrency caps; simulate restart stampede.
4. Confirm retry budgets and backoff; test failure amplification.
5. Separate scheduling failure (UNKNOWN/degraded) from resource DOWN in alert paths touched.
6. Apply maintenance-window behaviour if present; else document gap.
7. Evaluate assignment locality (Sensor site/network) and capacity impact.
8. Record metrics: queue depth, overdue polls, lease steal attempts.

## Negative / failure cases

- Two Sensors polling the same unit after partition (split brain).
- Removing jitter so all SNMP walks align on the minute.
- Infinite retry on timeout collapsing the worker pool.
- Marking device DOWN because scheduler fell behind.
- Starving tenant B because tenant A has 100k interfaces.

## Security / tenant implications

Work-unit metadata is tenant-scoped. Lease keys must not permit cross-tenant steal. Operators cannot reassign across tenants without authz. Logs include correlation IDs, not credentials.

## Required evidence

- Lease/ownership test results
- Concurrency/jitter/retry settings
- UNKNOWN vs DOWN separation
- Maintenance behaviour or `UNVERIFIED`
- Capacity impact note
- Exact SHA

## Stop conditions

Stop when:

- Dual ownership possible without detection
- Storm prevention absent for mass reassignment
- DOWN collapsed into scheduler lag
- Capacity impact unassessed for large enablement

## Anti-patterns

- Sleep-based tests instead of lease/clock orchestration
- “Run polls as fast as possible”
- Writer self-certifying scheduler safety without concurrent tests

## Handoff / output

Return: scheduling model; lease evidence; storm/capacity notes; nextOwner (Sensor runtime verifier). Not MERGE_READY alone.

## Non-ownership

Does not own protocol semantics (`snmp_mib`), HA Core fencing designs, or alert policy authoring. Owns work-unit scheduling discipline.
