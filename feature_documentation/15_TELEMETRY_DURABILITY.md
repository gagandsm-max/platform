# Telemetry Durability

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_03, 04, 05, 06, 11, 14, 17
**Related pending work:** Agent/Sensor reliability reconciliation may strengthen generic ACK/gap semantics.

## Product objective

Ensure telemetry accepted into a certified durable Platform boundary is not silently lost within the certified buffer/replay window, while honestly representing pre-acceptance UDP/network loss and finite-storage limits.

## Boundary principle

Do not promise "zero loss" from source to database under every protocol.

Examples:

- UDP Flow/Syslog before Sensor receipt/durable acceptance: best effort.
- SNMP request before a valid response exists: no telemetry exists to preserve.
- After certified durable local acceptance: Platform owns bounded replay/accounting.

## Sensor spool

Eligible Sensor workloads write typed records to bounded durable local spool before release according to the certified acceptance contract. Spool supports:

- sequence/record identity;
- restart recovery;
- retry/replay;
- corruption detection;
- bounded storage;
- priority/class;
- oldest-age/bytes/depth health;
- 80% storage protection.

## Core acknowledgement

Sensor data is released only after the applicable Core durable-acceptance watermark/contract. Transport success alone is not necessarily durable acceptance.

## Replay and duplicates

Sensor->Core is replay-safe/at-least-once unless a narrower path is specifically certified. Stable record/observation identity allows Core consumers to avoid duplicate business effects.

## Backpressure

Core/Kafka/ClickHouse slowdown must create bounded backlog/backpressure rather than unbounded RAM growth. Monitoring/control traffic can receive priority over replay/bulk classes.

## Data gaps

When storage protection, corruption, explicit policy or terminal downstream inability creates unavoidable loss, Platform should produce explicit gap/loss accounting including source/class/time or sequence range, count/bytes, reason and recoverability where available.

## Self-monitoring

Expose:

- spool depth/bytes;
- oldest record;
- ingest/upload rate;
- retry/backoff;
- accepted/rejected;
- data freshness;
- dropped/gap records;
- estimated remaining buffer horizon;
- Core/Kafka/ClickHouse lag.

## Failure behavior

Sensor continues collecting/buffering during transient Core failover/outage while certified capacity permits. Finite disk means infinite outage cannot carry an absolute no-loss guarantee.
