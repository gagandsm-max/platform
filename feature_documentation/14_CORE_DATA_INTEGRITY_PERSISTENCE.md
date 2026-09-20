# Core Data Integrity & Persistence

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_03, 04, 05, 06, 14, 17, 18

## Product objective

Preserve authoritative control state and accepted telemetry through typed schemas, tenant isolation, idempotency, versioned contracts, migrations, recovery and auditable lifecycle behavior.

## Datastore ownership

- **PostgreSQL** — authoritative transactional/control state: tenants, users/RBAC metadata, inventory/current entity state, policies, credentials metadata/references, licensing, workflow/job state and configuration.
- **ClickHouse** — high-volume historical telemetry/analytics such as metrics, flow, logs/events, state/history, incident evidence and audit analytics where appropriate.
- **Kafka** — internal event transport/backbone; not browser/customer transport.
- **Valkey** — bounded cache/ephemeral state only; never sole authority for durable business state.

## Integrity principles

- server-side tenant/resource scope;
- stable entity/event/job IDs;
- versioned schema;
- explicit state transitions;
- foreign-key/logical ownership rules;
- idempotent/replay-safe writes where applicable;
- ordering/sequence semantics where needed;
- no secret material in analytics/audit payloads;
- no silent data coercion/loss;
- migration compatibility and rollback plan.

## Ingestion accounting

Accepted telemetry must have a clear transition from produced -> accepted -> normalized -> persisted/consumed, with reject/drop/error accounting. Acknowledgement must not advance past data that became unaccounted unless the durability contract explicitly records a terminal gap/loss.

## Transactions

PostgreSQL mutations requiring atomicity use explicit transaction boundaries. Multi-datastore workflows do not pretend to be one distributed ACID transaction; use durable state/event/outbox/reconciliation patterns as required.

## Migrations

Schema migrations are versioned, ordered, tested against upgrade/rollback scenarios and must not silently reinterpret historical telemetry. Data conversion includes validation/reconciliation evidence.

## Protection

Core fixed 80% storage protection preserves essential control/current-state data before reclaiming eligible high-volume raw data. Protected incident/legal-hold evidence follows policy.

## Recovery

Backup/restore and DR verify data consistency, not merely service startup. Restoration must preserve tenant identity, authorization state, schema version and auditable continuity.
