---
name: database_monitoring
description: BUILD-035 customer monitored-target database evidence for PostgreSQL and Microsoft SQL Server—distinct from Platform control-plane postgresql Skill.
---

# database_monitoring

## Purpose

Own customer monitored-target database evidence (BUILD-035) for PostgreSQL and Microsoft SQL Server: availability, connectivity, sessions, waits, blocking, capacity, storage/file evidence, authorized query/load evidence, permissions, timeout, source quality and version compatibility—with least-privilege credentials and no DBA automation.

## Use when

Monitoring customer PostgreSQL or MSSQL as targets (not Platform's own control-state database). Owner `platform-infrastructure-cloud-engineer`.

## Authoritative inputs

Always read:

1. BUILD-035 scope and Mother References for monitored databases
2. `references/postgresql-target.md`, `references/mssql-target.md`
3. Credential Profiles with least-privilege DB roles
4. Fixtures for success, timeout, permission denied, blocking, waits, missing metrics, malformed results
5. Do **not** confuse with the `postgresql` Skill (Platform authoritative control-state engineering)

## Invariants

- **Database collector failure ≠ database service DOWN** without sufficient corroborating evidence.
- Platform `postgresql` Skill ≠ customer target monitoring.
- Least-privilege credentials only; no superuser-by-default.
- Sessions/waits/blocking/capacity are evidence classes with source quality and freshness.
- Authorized query/load evidence is bounded; no arbitrary SQL remediation.
- Permission denied and timeout are collector/auth evidence classes.
- Version compatibility is explicit; unsupported version ⇒ UNSUPPORTED/UNVERIFIED.
- Malformed results fail closed—do not invent metrics.

## Required procedure

1. Confirm target type (PostgreSQL vs MSSQL) and version compatibility gate.
2. Bind Credential Profile with least privilege.
3. Separate connectivity/collector health from database service operational state.
4. Collect sessions/waits/blocking/capacity with bounded queries.
5. Handle permission denied, timeout, missing metrics and malformed results explicitly.
6. Never propose arbitrary SQL remediation or DBA automation from this Skill.
7. Document source quality and freshness on every material assertion.

## Negative / failure cases

- Declaring DB DOWN solely because the collector timed out.
- Using Platform control-plane `postgresql` runbooks on customer DBs.
- Requesting sa/superuser for routine metrics.
- Executing LLM-authored SQL against customer databases.
- Treating blocking session evidence as automatic kill authorization.

## Security / tenant implications

DB credentials and result sets may contain sensitive data. Tenant isolation, redaction, least privilege and no cross-tenant query. Support bundles must not include connection secrets or full query text with secrets.

## Required evidence

- Connectivity vs service-state separation tests
- Permission/timeout/malformed fixtures
- Version compatibility notes
- Exact SHA

## Stop conditions

Stop when:

- Collector failure is collapsed into DOWN without evidence policy
- DBA automation / arbitrary SQL remediation is requested
- Superuser credentials are required for routine monitoring
- Scope expands into full APM/query-store productization without authority

## Anti-patterns

- “Login failed ⇒ database down”
- Copying production query text into alerts unbounded
- Writer self-certifying database pack coverage

## Handoff / output

Return: target model; evidence classes; gaps; nextOwner (infra verifier). Not MERGE_READY alone.

## Non-ownership

Does not own Platform control-state PostgreSQL (`postgresql` Skill), DBA remediation, or arbitrary schema change. Does not add MySQL/Oracle in this tranche.
