---
name: postgresql
description: PostgreSQL control-state schema, transactions, tenant isolation and backup/restore of authoritative data.
---

<!-- generated from project_agents/skills/postgresql/SKILL.md sha256:0089c8016f8cfd130e90f1ea255b5cc4dc0b4f52c916aa66ee39cdb7db428978 -->

# postgresql

## Purpose

Change and operate PostgreSQL as Platform’s **transactional authority** for control-state—never as an afterthought to ClickHouse analytics—with tenant predicates, lock/clock honesty and fail-closed loss behaviour.

## Use when

Schema/migrations, control APIs, watermarks/leases, advisory locks, backup/PITR, pool tuning, or any claim about HA/replication affecting PG. Required for Core ingestion stores and BUILD-003-style lease correctness.

## Authoritative inputs

Always read:

1. Flyway/migration catalogue and `schema_history` expectations
2. Table contracts for tenant_id and composite FKs
3. Mother Architecture: PG = control-state authority; exactly-two-Core HA leaves unresolved PG fencing as Decision Register / BUILD-120+ (do not invent)
4. Existing watermark/lease SQL (prefer `clock_timestamp()` for wall-clock safety where leases matter)
5. Backup/PITR runbooks and `backup_restore` Skill
6. Testcontainers/integration harness notes (`@Tag("postgres")`)

## Invariants

- PostgreSQL holds **transactional authority**; ClickHouse does not.
- Every tenant-owned query/DML includes correct tenant predicates / composite FKs.
- Migrations are forward, checksum-protected, advisory-locked; no casual down-migrations.
- Locking: row `FOR UPDATE`, transaction advisory locks, and deadlock risk must be reasoned explicitly.
- Isolation semantics (read committed vs stronger) must match the concurrency claim.
- **Clock semantics**: lease/expiry safety uses DB wall clock (`clock_timestamp()`), not transaction-start `CURRENT_TIMESTAMP`, when correctness requires it; do not mix Java `Instant.now` with PG time for ownership.
- Constraints/indexes are part of the contract; “fix in app” is insufficient for uniqueness/tenancy.
- Connection pools must not hide leaking transactions or unbounded concurrency.
- Deadlocks fail closed/retry only with proven idempotent paths.
- Replication/HA boundaries: do not invent BUILD-120 fencing/quorum; label unresolved.
- Backup/PITR is mandatory for durability claims; HA standby ≠ backup.
- Performance claims need evidence (plans, IOPS, pool wait)—not guesses.
- Integration proof prefers Testcontainers/`@Tag("postgres")` over mocks for lock/clock behaviour.
- DB loss: fail closed; do not pretend ClickHouse can reconstruct control-state.

## Required procedure

1. State whether the change is schema, query, lock/lease, backup or pool-related.
2. Ensure tenant predicates and FK/constraint coverage; add negative tests for cross-tenant writes.
3. For migrations: checksum, advisory lock, rollback-on-failure, history completeness.
4. For leases/locks: document lock keys, wall-clock predicates, and concurrent tests (including lock-delay vs expiry).
5. Avoid Java/PG clock mixing for ownership tokens.
6. Assess deadlock/retry surfaces; keep retries idempotent.
7. If HA/replication topology is touched, stop at Decision Register unless already resolved.
8. Verify backup/PITR impact; never claim instant physical erasure from immutable backups.
9. Run postgres-tagged integration tests; record plans/metrics when performance-sensitive.
10. On DB unavailable: verify API/fail-closed behaviour—no silent dual-write to analytics.

## Negative / failure cases

- Query missing tenant_id enabling cross-tenant reads.
- Lease finalize using `CURRENT_TIMESTAMP` after waiting on locks past expiry.
- Migration applied without advisory lock concurrency safety.
- Inventing logical replication fencing “for now”.
- Truncating tenant data as a “fix”.
- Pool size explosion under retry storms.

## Security / tenant implications

Tenant isolation is a hard DB property. Secrets/credentials never stored in plain PG columns without the secrets architecture. Role privileges least-privilege for app vs migrator. Audit sensitive DDL. RLS (if introduced later) must not be assumed present without Mother authority.

## Required evidence

- Migration versions/checksums
- SQL/lock/clock snippets under test
- Postgres integration test names/results
- Tenant negative-test results
- Backup/PITR notes or `UNVERIFIED`
- EXPLAIN/perf artefacts when claiming capacity
- Fail-closed behaviour under DB down (when in scope)

## Stop conditions

Stop when:

- Fix requires unresolved two-Core PG fencing design
- Tenant predicate cannot be proven
- Clock/lock correctness cannot be tested yet a safety claim is required
- Migration checksum/history would be weakened

## Anti-patterns

- Using ClickHouse for authoritative ACK/leases
- `DELETE FROM … WHERE tenant_id IS NULL`
- Sleeping in tests instead of lock/clock orchestration
- Writer self-certifying migration safety
- Disabling constraints to pass tests

## Handoff / output

Return: schema/lock/clock analysis; test evidence; HA caveats; nextOwner (`migration_safety` / DBRE verifier). Not MERGE_READY alone.

## Non-ownership

Does not invent BUILD-120 replication designs, replace `backup_restore`/`clickhouse`, or certify merge. Owns PG transactional correctness discipline.
