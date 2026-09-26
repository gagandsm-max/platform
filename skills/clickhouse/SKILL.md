---
name: clickhouse
description: ClickHouse historical analytics, insert idempotency, retention and lag without treating CH as control state.
---

<!-- generated from project_agents/skills/clickhouse/SKILL.md sha256:cb3b7718f8056ac792237cbfc277aea5f8a9f9c6a5803ed4897ef240b82dca97 -->

# clickhouse

## Purpose

Operate ClickHouse strictly as telemetry/analytics storage—never transactional authority—with tenant scoping, cardinality control, retention and honest lag/failure behaviour.

## Use when

CH schema/migrations, Kafka→CH consumers, analytics queries, retention/TTL, cardinality explosions, legacy CH upgrades, or any temptation to ACK/control-plane on CH success.

## Authoritative inputs

Always read:

1. Mother Architecture: CH = historical analytics; PG = control-state
2. Existing CH migration scripts and table engines/ORDER BY/partition keys
3. Kafka consumer contracts and replay/idempotency Skills
4. `Platform_11` sizing/retention guidance
5. Storage-protection / capacity expectations (80% themes where applicable to analytics hosts)
6. Legacy upgrade/replay-identity tests when touching historical tables

## Invariants

- Telemetry/analytics role **only**—no transactional authority, no lease/ACK source of truth.
- Tenant scoping on insert and query paths; no cross-tenant scan “for convenience”.
- Partition/ORDER BY/index (primary key) design must match query and lifecycle claims.
- Cardinality of tags/labels/entities is bounded; high-cardinality dumps are defects.
- Retention/TTL is explicit; do not promise infinite hot storage.
- Ingestion failure must not corrupt control-plane watermarks; CH lag ≠ PG truth.
- Query latency claims need evidence under realistic cardinality.
- Schema evolution is planned (materialized migrations / compatible reads); no silent break of replay identity.
- Dedupe expectations are explicit (at-least-once from Kafka is normal).
- Kafka replay into CH must be safe/idempotent at the CH table design level claimed.
- Backup/recovery implications differ from PG PITR—state them honestly.
- Legacy upgrades need proven replay-identity tests.
- Storage protection/capacity: refuse unbounded retention growth without headroom evidence.

## Required procedure

1. Confirm the change does not move ACK/control authority into CH.
2. State table engine, partition, ORDER BY, and tenant key columns.
3. Assess cardinality of new dimensions; add bounds or reject the schema.
4. Define retention/TTL and storage impact; tie to capacity_performance when large.
5. Align Kafka consumer commit semantics with CH insert durability claims (`replay_idempotency`).
6. Add/extend tests for ingest failure, duplicate delivery and legacy upgrade when touched.
7. Measure or mark `UNVERIFIED` query latency/lag under non-trivial data.
8. Document backup/restore expectations (what is lost if CH volume dies).
9. Never instruct operators to “fix tenancy by rewriting CH and ignoring PG”.

## Negative / failure cases

- Treating CH insert success as Sensor upload ACK.
- Omitting tenant_id from ORDER BY/query filters.
- Introducing unbounded map keys/labels.
- Dropping partitions that break required replay windows without Decision Register.
- Claiming exactly-once CH without proof.
- Ignoring Kafka lag while declaring analytics “caught up”.

## Security / tenant implications

Cross-tenant CH reads are a security incident class. Do not put secrets into CH columns. Support dumps redact payloads. Multi-tenant operators require server-side enforcement, not UI-only filters.

## Required evidence

- Schema DDL + engine/partition/order justification
- Tenant filter proof (query + test)
- Cardinality/retention analysis
- Kafka consumer/replay test results
- Lag/latency measurements or `UNVERIFIED`
- Legacy upgrade test results when applicable
- Backup/loss narrative

## Stop conditions

Stop when:

- Design requires CH as transactional source of truth
- Tenant scoping cannot be enforced
- Cardinality/retention would breach capacity without a plan
- Replay identity would break without migration proof

## Anti-patterns

- “Just query CH for the watermark”
- SELECT without tenant predicate in multi-tenant admin tools
- Infinite TTL “until we decide”
- Writer self-certifying analytics correctness from a 100-row lab table

## Handoff / output

Return: schema rationale; tenant/cardinality/retention notes; replay evidence; lag; nextOwner (telemetry pipeline verifier). Not control-plane certification.

## Non-ownership

Does not replace PostgreSQL authority, invent BUILD-120 CH clusters, or certify MERGE_READY. Owns analytics-store discipline.
