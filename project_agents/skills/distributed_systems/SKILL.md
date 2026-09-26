---
name: distributed_systems
description: Distributed-systems discipline for Platform ownership, durability, fencing, retries and partial failure without inventing unresolved HA designs.
---

# distributed_systems

## Purpose

Apply distributed-systems discipline to Platform Core/Sensor/Agent paths: clear ownership, durable vs ephemeral state, idempotent retries, fencing/leases and honest partial-failure behaviour—without inventing BUILD-120+ HA topology.

## Use when

Multi-node ownership, lease/fencing, queue/backpressure, retry storms, split-brain risk, clock-sensitive expiry, or consistency-boundary questions across PG/Kafka/spool. Required for runtime engineers touching shared mutable authority.

## Authoritative inputs

Always read:

1. Architecture: exactly-two-Core HA remains Decision Register / BUILD-120+ where unresolved
2. Existing lease/watermark/spool/Kafka code on the exact head
3. `postgresql`, `kafka`, `replay_idempotency`, `ha_dr` Skills
4. Task stopConditions for unresolved distributed primitives
5. Failure-injection evidence expectations

Do not treat blog consensus patterns as Platform authority before Decision Register approval.

## Invariants

- Ownership and write authority are explicit; dual-active writers without fencing are defects.
- Durable vs ephemeral state is labelled (PG control-state ≠ Valkey cache ≠ CH analytics).
- At-least-once delivery is the default assumption unless end-to-end proof exists.
- Idempotency keys/effects protect retries.
- Fencing/leases exclude stale writers; clock semantics prefer DB wall clock where leases matter.
- Split-brain scenarios are named and tested or marked unresolved/`UNVERIFIED`.
- Retries use bounded backoff; partial failure is classified per subsystem.
- Queues expose backpressure; ordering guarantees are scoped (partition/key), not global myths.
- Crash windows leave recoverable durable state—not silent loss of authority tokens.
- Consistency boundaries are stated; no distributed transaction assumption without proof.
- Failure injection evidence is required for material distributed claims.

## Required procedure

1. Name the authority object (lease, offset, spool ACK, config version) and its store.
2. Document failure modes: crash mid-write, network partition, clock skew, retry after success.
3. Define fencing/idempotency; add concurrent tests where code changes.
4. Bound retries/backoff; prove no unbounded amplification.
5. Separate ephemeral cache loss from durable control-state loss.
6. If HA fencing/replication is required beyond implemented code, stop for Decision Register—do not invent BUILD-120+.
7. Record failure-injection or mark `UNVERIFIED`.

## Negative / failure cases

- Two Sensors owning one work unit after partition.
- Retry creating duplicate side effects without idempotency.
- Using `Instant.now` vs PG clock for lease expiry incorrectly.
- Claiming exactly-once across Kafka+PG without proof.
- Inventing three-node quorum “for safety.”

## Security / tenant implications

Distributed bugs often become cross-tenant or privilege bugs (stolen leases, replayed admin commands). Tenant scope must be part of ownership keys. Audit fencing losses.

## Required evidence

- Ownership/fencing design notes
- Idempotency/retry tests
- Clock/lease evidence where applicable
- Failure-injection results or `UNVERIFIED`
- Exact SHA

## Stop conditions

Stop when:

- Dual-active write possible without detection
- Unresolved HA primitive would be coded as truth
- Idempotency absent on mutating retry paths
- Distributed TX assumed without proof

## Anti-patterns

- “Kafka is the source of truth for leases”
- Sleep-based distributed tests only
- Writer self-certifying distributed safety

## Handoff / output

Return: authority model; failure matrix; test evidence; nextOwner (runtime/SRE verifier). Not MERGE_READY alone.

## Non-ownership

Does not invent BUILD-120+ HA designs or replace domain Skills (`kafka`, `postgresql`). Owns distributed correctness discipline.
