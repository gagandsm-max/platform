# Database Monitoring

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 09, 10, 11, 22, 23

## Product objective

Monitor database availability, workload, capacity, replication and failure evidence through certified Capability Packs without embedding vendor-specific database logic into Core.

## Target database families

The Mother Reference includes major relational/NoSQL/data-platform targets such as PostgreSQL, MySQL/MariaDB, Microsoft SQL Server, Oracle, SAP HANA, Db2, MongoDB, ClickHouse and Elasticsearch/OpenSearch where supported interfaces/licensing permit.

Listing a product is a certification target, not a shipment/certification claim.

## Canonical entities

Normalize vendor models into entities such as:

- instance/cluster/node;
- database/schema;
- tablespace/storage;
- replica;
- connection pool;
- transaction log;
- query/workload class.

## Monitoring capabilities

Depending on the certified Pack/interface:

- availability and connection health;
- CPU/memory/storage pressure attributable to the DB;
- sessions/connections;
- transaction/commit/rollback rates;
- replication state/lag;
- locks/blocking/deadlocks;
- tablespace/data/log growth;
- cache/buffer efficiency;
- long/slow/query workload evidence;
- queue/thread/worker pressure;
- backup/recovery state where the product exposes it;
- capacity/trend/forecast evidence.

## Collection architecture

Prefer supported vendor APIs/views/protocols and least-privilege read-only accounts. Database Packs normalize into Platform metrics, state, inventory, relationships and event/evidence contracts. High-cardinality query text/labels require bounded treatment.

## Security

Credential Profiles are tenant/site scoped and encrypted. Avoid storing passwords, connection strings with secrets or unrestricted query text in ordinary telemetry/logs. Monitoring accounts receive only required read permissions.

## Alert/RCA behavior

A failed DB query alone is not automatically a database-down assertion. Correlate DB evidence with host, storage, network, cluster/replication and application/dependency evidence. RCA may identify locks, storage exhaustion or replica failure as hypotheses only when evidence supports them.

## UX

Database detail should expose Overview, Availability, Performance, Sessions/Locks, Replication, Storage/Capacity, Workload/Queries, Dependencies, Alerts/Incidents, Changes and evidence-quality state.

## Certification

Each Capability Pack declares supported product/version, discovery, L1/L2/L3 metrics, topology/dependencies, diagnostics, security method and negative/failure cases. No database family is called certified merely because this document names it.
