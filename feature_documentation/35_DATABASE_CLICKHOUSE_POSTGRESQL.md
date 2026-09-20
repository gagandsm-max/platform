# Databases — ClickHouse & PostgreSQL

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_03, 04, 05, 11, 14, 16, 17

## Product objective

Use PostgreSQL and ClickHouse for clearly separated workloads so control state remains transactionally correct while high-volume telemetry remains efficient and queryable.

## PostgreSQL — authoritative control state

PostgreSQL owns transactional/current-state domains such as:

- tenants/customers;
- users/RBAC metadata;
- inventory/current entity state;
- policies/profiles;
- Credential Profile metadata/references;
- Sensor/Agent assignments/control state;
- workflow/job state;
- licensing;
- integration configuration;
- monitoring definitions;
- Capability Pack metadata/compatibility state;
- saved/configuration objects;
- current reconciliation state;
- audit references where transactional ownership is required.

Use explicit transactions and schema migrations.

## ClickHouse — high-volume historical analytics

ClickHouse owns high-volume historical/analytical data such as:

- numeric/state telemetry history;
- Flow;
- Logs/events;
- selected trap/event analytics;
- telemetry quality;
- alert/incident/audit history analytics where appropriate;
- exact counter history/derived rates when the final schema requires it;
- rollups;
- capacity/SLA facts;
- queryable incident evidence;
- analytics features.

ClickHouse is not the authoritative RBAC/configuration database.

## Kafka and Valkey relationship

Kafka is the internal event transport/backbone between producers/consumers; it is not a database exposed to customers.

Valkey is cache/ephemeral state only; loss of Valkey must not destroy authoritative configuration/business state.

## Schema design

Use typed stable columns for common fields and controlled extension attributes for vendor/high-cardinality data. Avoid dynamic-column explosion.

Physical table/topic choices remain implementation delegated where the Mother Reference defines only a logical data family.

## Precision

Source values that cannot be represented exactly as Float64, such as full SNMP Counter64, require an exact representation before derived calculations. Do not sacrifice source truth for one universal numeric column.

## Tenant isolation

Every telemetry/control record carries or derives trusted tenant scope. Payload-provided tenant identifiers never override authenticated/server-side authority.

## Retention

ClickHouse TTL/rollups and raw/aggregate retention are data-class specific. PostgreSQL authoritative state is not purged using high-volume telemetry rules.

## HA / recovery

Each datastore has explicit replication/promotion/fencing/backup/restore behavior compatible with the exactly-two-Core product architecture. Do not assume textbook three-node quorum designs.

## Migrations and upgrades

PostgreSQL migrations and ClickHouse schema evolution are versioned, tested, backward/rollback assessed and verified against representative data. A successful service restart alone does not prove data integrity.

## Self-monitoring

Expose connection health, replication/failover state, disk usage, insert/query latency, Kafka consumer lag, migration version, backup status, retention cleanup and errors.

## No extra database by default

Do not add another telemetry/log/Flow database unless benchmark evidence proves PostgreSQL+ClickHouse cannot meet a certified requirement.
