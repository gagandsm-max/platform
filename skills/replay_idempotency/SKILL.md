---
name: replay_idempotency
description: Replay, dedup, late-data and exactly-once-effect evidence across Sensor spool, Kafka and datastores.
---

<!-- generated from project_agents/skills/replay_idempotency/SKILL.md sha256:b5a791d93f7c8e9c2bd79cfcdf919d561122134237ebe18143a6b107f411d42f -->

# replay_idempotency

## Purpose

Prove that Sensor spool upload, Kafka consume and datastore apply paths tolerate retry/replay without duplicating or dropping authoritative effects—and that durable ACK is the only licence to forget data.

## Use when

Uploader replay, consumer retry, watermark/idempotency-key changes, late data, duplicate delivery, fencing of stale publishers, or any claim of exactly-once effect across spool → transport → store. Required for `platform-sensor-runtime-engineer`, `platform-telemetry-pipeline-engineer` and BUILD transport slices.

## Authoritative inputs

Always read:

1. Governing transport/spool contracts in Architecture/LLD for the touched path
2. Exact code owning reserve → publish → finalize → ACK (Sensor uploader / Core ingress)
3. Existing watermark / idempotency / checksum / batch-identity fields in schema
4. Task scope files and `replay_idempotency` / `runtime_failure_injection` pairing when crash points matter
5. Prior EXECUTION_EVIDENCE for the exact head—stale SHA evidence is invalid

## Invariants

- Durable ACK boundary is authoritative: **no spool deletion / offset advance / watermark move before exact ACK** of the durable success criteria for that hop.
- Request identity, sequence lineage, checksum and batch identity must make **exact replay** distinguishable from **conflicting replay**.
- Pending reservation must not be treated as committed effect.
- Crash before publish, after publish before ACK, lost ACK, restart and duplicate downstream delivery must leave a deterministic recoverable state.
- Stale publishers are fenced; a superseded epoch/generation cannot silently win.
- Exactly-once *effect* is evidenced, not asserted from at-least-once transport alone.
- Deterministic reproducer evidence is mandatory for durability claims.

## Required procedure

1. Identify the idempotency key(s), watermark, sequence/lineage fields, checksum and batch identity for the hop under change.
2. Draw the reserve → publish → finalize → ACK state machine; mark the durable ACK boundary.
3. Enumerate cases: exact replay (same identity), conflicting replay (same key, different payload/checksum), pending reservation timeout, crash before publish, crash after publish before ACK, lost HTTP/gRPC ACK, process restart, duplicate downstream delivery, late data, stale publisher after fencing.
4. Implement or extend tests that drive each case to an externally observable store/spool state.
5. Prove spool entries are retained until exact ACK; deletion earlier is a defect.
6. Prove consumers fail closed on poison/conflict (no silent offset advance).
7. Record a deterministic reproducer (commands, fixtures, expected states) so verifiers can re-run on the exact head.

## Negative / failure cases

- Deleting spool batches because upload returned 200 without durable Core/Kafka criteria.
- Treating pending reservation as success after crash.
- Accepting conflicting replay as idempotent success.
- Double-apply to PostgreSQL/ClickHouse from duplicate delivery without dedup evidence.
- Lost ACK causing permanent gap because local state was erased.
- Stale publisher after promotion/fencing still appending as authority.
- “Idempotent” claimed solely because HTTP client retries.

## Security / tenant implications

Idempotency keys and watermarks are tenant-scoped where the data is tenant-scoped. Replay fixtures must not cross tenants. Logs must not dump full payloads that contain secrets. Fencing tokens/epochs are integrity-sensitive—treat forgery as a security defect.

## Required evidence

- State-machine diagram or equivalent bullet timeline for the hop
- Matrix of exact/conflict/pending/crash/lost-ACK/restart/duplicate/stale-publisher results
- Proof spool retention until ACK
- Watermark/idempotency schema identifiers
- Deterministic reproducer commands on exact SHA
- `UNVERIFIED` where exactly-once effect is not evidenced
- Claim labelling via `evidence_bundle`

## Stop conditions

Stop when:

- ACK boundary is ambiguous or disagreed across components
- Conflicting replay cannot be detected
- Change requires silent data drop to “recover”
- Required crash/replay cases cannot be executed and the task claims durability anyway

## Anti-patterns

- Best-effort delete-on-send
- Reusing idempotency keys across tenants or epochs
- Hand-waving “Kafka is durable so we are exactly once”
- Erasing failure evidence to green CI
- Writer self-certifying transport durability

## Handoff / output

Return: identity fields; ACK boundary; case matrix; reproducer; residual `UNVERIFIED`; next owner (`platform-telemetry-pipeline-engineer` / independent verifier). Pair with `runtime_failure_injection` when crash points are new.

## Non-ownership

Does not invent unresolved multi-Core fencing, authorize production replay storms, or certify MERGE_READY. Complements `kafka`, `sensor_edge`, `runtime_failure_injection` and `ha_dr` without replacing them.
