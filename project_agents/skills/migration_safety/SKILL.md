---
name: migration_safety
description: Protect schema/data migrations with compatibility, rollback, idempotency, tenant isolation and failure-path evidence.
---

# migration_safety

## Purpose

Protect Platform control-state and analytics compatibility when a task touches schema, storage format, indexes, constraints, retention or data backfill—especially PostgreSQL Flyway-style migrations and related ClickHouse/retention changes.

## Use when

Adding/changing SQL migrations, ORM schema, idempotency keys, watermark tables, FK/check/null/default behavior, large-table backfills, retention jobs, or upgrade/rollback notes that mutate durable state. Required for `platform-database-reliability-engineer` and any Core/Sensor change that alters persisted contracts.

## Authoritative inputs

Always read:

1. Existing migration history under the owning module (never rewrite published migrations)
2. `project_documentation/Platform_03_Architecture.md` storage roles (PostgreSQL control state vs ClickHouse analytics)
3. `project_documentation/Platform_11_Sizing_Capacity_Storage_Master.md` for retention/80% protection and capacity impact
4. Task `migrationReservations[]`, parallel branch reservations, and HA notes for two-Core Active/Standby
5. Backup/restore/PITR runbooks applicable to the environment under test

Platform owns schema. Do not invent automatic down-migration requirements the governing process does not support.

## Invariants

- Forward-only migration discipline: historical migration scripts are immutable once shipped; never reuse or alter an occupied migration id without proof it never shipped.
- Compatibility window: expand/contract (dual-write/dual-read) as required so mixed-version Core/Sensor/Agent during rollout does not corrupt data.
- Tenant isolation columns/predicates remain enforced; migrations must not create cross-tenant leakage paths.
- Lock duration, large-table rewrites and backfills stay bounded; 80% storage protection and HA failover targets remain respected.
- Restart/idempotency behavior of the migration runner is understood; partial failure has an explicit roll-forward or recovery decision.
- Defaults/nulls/checks/FKs are evaluated for existing rows, not only empty databases.
- Backup/restore/PITR and standby apply paths remain valid; destructive data loss is never an implicit rollback plan.

## Required procedure

1. Classify the change: schema-only, data backfill, index/constraint, retention, or contract/idempotency semantic.
2. Reserve a new forward migration id; confirm no collision across branches.
3. Assess expand/contract steps and mixed-version readers/writers during deployment.
4. Estimate lock duration and table size; choose online techniques or staged backfill with progress/observability.
5. Define tenant-isolation and authorization impact; verify predicates/indexes still enforce tenant scope.
6. Specify default/null/check/FK behavior on existing rows; add validation queries.
7. Document restart/idempotency: what happens if the process crashes mid-migration; whether retry is safe; whether a partial apply requires roll-forward.
8. Decide rollback vs roll-forward explicitly (restore from backup/PITR, feature flag, compensating forward migration)—do not claim a fictional down script if unsupported.
9. Add exact migration tests: empty DB, representative fixture, failure injection, repeat apply, and tenant isolation queries.
10. Record HA implications: apply on active, standby catch-up, and Sensor continue-to-collect expectations during Core upgrade.

## Negative / failure cases

- Editing a checksummed historical migration to “fix CI”.
- Long ACCESS EXCLUSIVE locks on hot control tables without measurement.
- Backfill that scans unbounded history in one transaction and blows storage protection.
- Dropping columns/tables still read by a mixed-version peer.
- Nullable→NOT NULL without backfill; FK add without verifying orphans.
- Silent cross-tenant unique constraints or global sequences used as tenant authority.
- Assuming standby apply equals restore validation without a restore/PITR exercise when risk warrants it.
- Treating schema success alone as data-invariant proof.

## Security / tenant implications

Migrations run with appropriate DB privileges (runtime vs migration role separation where required). Do not embed secrets in migration SQL. Tenant id remains server-enforced. Audit who applied what version in which environment. Fail closed if migration privilege or connectivity is wrong—do not fall back to permissive DDL from the app role.

## Required evidence

- Migration ids added and immutability check on history
- Compatibility/mixed-version analysis
- Lock/backfill strategy and measured or bounded estimates
- Tenant-isolation verification queries
- Restart/partial-failure notes and rollback vs roll-forward decision
- Exact test commands/ids (empty, fixture, failure, repeat)
- Backup/PITR/HA impact statement or explicit `NOT_RUN` with risk acceptance owner
- Exact SHA and claim types via `evidence_bundle`

## Stop conditions

Stop when:

- Required migration id is already occupied on another lane without reservation resolution
- Compatibility window cannot be articulated
- Only recovery path is untested destructive DROP
- Mother Reference storage role would be violated (e.g., using PostgreSQL as primary high-volume telemetry store)
- Parallel worktree writes the same migration space without coordination

## Anti-patterns

- Squashing or rewriting published migrations on shared branches
- “We’ll fix data in prod manually” as the plan
- Disabling FK/checks to get green
- Skipping tenant predicates on new tables “temporarily”
- Inventing down-migrations that the Flyway/process policy forbids and then claiming rollback safety

## Handoff / output

Return: migration ids; expand/contract plan; lock/backfill bounds; failure/restart behavior; test matrix; rollback/roll-forward decision; residual HA/PITR risks; next verifier (`platform-database-reliability-engineer` / independent reviewer).

## Non-ownership

Does not authorize production apply, invent unresolved two-Core replication designs, select new databases, or certify MERGE_READY alone. Does not replace `postgresql`/`clickhouse`/`backup_restore` Skills—it governs safety procedure when those stores change.
