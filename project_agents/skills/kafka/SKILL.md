---
name: kafka
description: Internal Kafka transport, consumer fail-closed ACK/offset, poison parking and lag evidence.
---

# kafka

## Purpose

Operate Kafka as Platform’s **internal** event backbone with correct partition keys, offset-vs-business-ACK discipline, lag visibility and poison handling—without exactly-once claims lacking end-to-end proof or inventing BUILD-122 HA topology.

## Use when

Producer/consumer changes, topic design, schema evolution, lag incidents, rebalance storms, or replay tooling. Required for telemetry pipeline and Core messaging paths.

## Authoritative inputs

Always read:

1. Architecture: Kafka is internal; Sensor/Agent external transport is HTTPS/gRPC mTLS
2. Existing topic configs, serializers and consumer code on the exact head
3. `replay_idempotency`, `distributed_systems`, `api_event_contracts`
4. Schema registry / compatibility policy if present (`UNVERIFIED` if absent)
5. Decision Register before inventing multi-cluster HA (BUILD-122+)

## Invariants

- Internal event backbone only—not a public client protocol.
- Partitioning/key choice defines ordering scope; document it.
- Producer `acks` and idempotent producer settings are intentional.
- Transactions used only where actually implemented and tested.
- Producer fencing considered for transactional IDs when used.
- Retries bounded; poison events parked—not infinite retry loops.
- **Consumer offset commit ≠ business ACK** unless proven identical effect store.
- Replay/duplicates expected under at-least-once; consumers must be idempotent at effect layer.
- Lag/backpressure visible; retention sufficient for stated replay windows.
- Schema compatibility enforced for material events.
- Tenant/event scope fields required on tenant-owned payloads.
- Broker outage and rebalance behaviour documented; fail closed on authz/tenant parse errors.
- **No exactly-once claim without end-to-end proof.**

## Required procedure

1. State topic, key, ordering scope and compatibility mode.
2. Separate produce durability from consumer effect durability.
3. Implement/verify idempotent effect handling and poison parking.
4. Ensure offset commit happens only after durable effect (or document intentional at-least-once gap).
5. Add lag metrics/alerts considerations for changed consumers.
6. Test rebalance and duplicate delivery where feasible.
7. Do not invent broker HA topologies beyond implemented ops docs.

## Negative / failure cases

- Committing offsets before PG write succeeds.
- Using null keys then assuming global order.
- Dropping poison messages without parking/metrics.
- Exposing Kafka to Sensors over the Internet.
- Claiming EOS because `enable.idempotence=true` alone.

## Security / tenant implications

ACLs/auth between internal principals. Payloads may carry tenant data—encryption in transit as deployed. No cross-tenant consume via shared consumer group misbind. Secrets not in message headers.

## Required evidence

- Topic/key/compatibility notes
- Offset vs ACK design
- Duplicate/poison tests
- Lag or `UNVERIFIED`
- Exact SHA

## Stop conditions

Stop when:

- Offset commit precedes durable effect without disclosure
- Exactly-once claimed without proof
- BUILD-122 HA invented
- Tenant field missing on tenant events

## Anti-patterns

- One giant topic for everything with random keys
- Manual offset hacks in prod without audit
- Writer self-certifying messaging correctness

## Handoff / output

Return: messaging design; idempotency evidence; risks; nextOwner (telemetry verifier). Not MERGE_READY alone.

## Non-ownership

Does not own external mTLS transport, invent multi-cluster HA, or replace CH/PG authority. Owns Kafka usage discipline.
