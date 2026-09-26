---
name: tenant_isolation
description: Server-side tenant isolation, RBAC/resource scope and cross-tenant negative cases.
---

# tenant_isolation

## Purpose

Enforce tenant isolation as a hard server-side property across APIs, DB, Kafka, ClickHouse, caches, jobs and exports—never trusting the UI or client-supplied tenant overrides.

## Use when

Any multi-tenant API, query, consumer, cache key, export, Sensor/Agent assignment, or admin/MSP scoped feature. Required operational-critical Skill for essentially all product changes touching tenant data.

## Authoritative inputs

Always read:

1. Architecture tenant model and RBAC/resource-scope notes
2. Existing tenant filters/interceptors on the exact head
3. `postgresql`, `clickhouse`, `kafka`, `api_event_contracts`
4. Negative-test catalogues for cross-tenant access
5. MSP/admin break-glass policies if present (`UNVERIFIED` if absent)

## Invariants

- Tenant ID authoritative source is server session/identity—not request body alone.
- Server-side enforcement on every read/write path.
- DB predicates and same-tenant FK/relationship constraints required for tenant-owned rows.
- Kafka/event payloads carry tenant scope; consumers filter fail-closed.
- ClickHouse insert/query scoped; no cross-tenant scan conveniences.
- Caches keyed by tenant; no shared poisoned entries across tenants.
- **UI is not a security boundary.**
- Credentials/profiles tenant-scoped.
- Sensor/Agent assignment cannot cross tenants.
- Cross-tenant object references rejected.
- Enumeration leakage (IDOR timing/message) minimized.
- Audit cross-tenant deny decisions where high risk.
- Background jobs and exports/reports preserve tenant scope.
- Admin/MSP scope explicit and audited—not implicit superuser in app code.
- Negative tests mandatory for new tenant surfaces.
- **Fail closed on missing tenant.**
- **No client-supplied override** of tenant authority.

## Required procedure

1. Identify every store/API touched; list tenant predicates.
2. Ensure identity→tenant binding before authorization.
3. Add/extend cross-tenant negative tests (read and write).
4. Verify cache keys, job payloads and export queries.
5. Confirm error messages do not confirm cross-tenant existence unnecessarily.
6. For MSP/admin: require explicit capability checks.
7. Refuse features that cannot express tenant scope.

## Negative / failure cases

- `WHERE id=?` without `tenant_id`.
- UI hiding buttons as the only control.
- ClickHouse query with tenant only in UI filter JSON.
- Sensor enrolled into tenant A uploading for tenant B IDs.
- Export job iterating all tenants because “batch efficiency.”

## Security / tenant implications

This Skill is operational-critical. Breaks are Sev-1 class. Pair with AppSec review on new surfaces. Support bundles must not mix tenants.

## Required evidence

- Negative-test results
- Predicate/filter references
- Job/export scope notes
- Exact SHA

## Stop conditions

Stop when:

- Client override would be honored
- Missing tenant would default to “all”
- Negative tests absent for new access path
- Cross-store scope inconsistent (API yes, CH no)

## Anti-patterns

- Shared global admin DB user in app pool without row controls
- “We’ll add tenant_id later”
- Writer self-certifying isolation

## Handoff / output

Return: isolation map; negative evidence; residual risk; nextOwner (AppSec/verifier). Not MERGE_READY alone.

## Non-ownership

Does not own IdP configuration alone or invent multi-tenant billing. Owns tenant isolation enforcement discipline.
