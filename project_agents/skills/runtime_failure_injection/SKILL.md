---
name: runtime_failure_injection
description: Exercise crash, retry, timeout, durability and partial-failure paths and distinguish runtime evidence from source inference.
---

# runtime_failure_injection

## Purpose

Convert durability/retry/timeout assumptions into **runtime** EXECUTION_EVIDENCE by injecting the narrowest safe failure at the boundary that can violate the invariant—never by destructive environment vandalism.

## Use when

Proving crash/retry behaviour for Sensor spool/uploader, Core ingress, Kafka consumers, DB transactions, or any task that claims fail-closed ACK/offset/finalize semantics. Pair with `replay_idempotency` for transport hops. Required before durability or exactly-once claims advance past LOCAL_VALIDATION.

## Authoritative inputs

Always read:

1. The exact state machine under test (reserve → publish → commit/finalize → ACK)
2. `replay_idempotency` and owning module tests already covering happy path
3. Compose/lab topology for the drill (never production)
4. Task `securityBoundaries` and stop conditions forbidding destructive Docker shortcuts
5. Prior runtime evidence SHA—invalidate on head drift
6. `references/failure-modes-recovery-oracle.md` and fixtures/evals for lost ACK, dependency outage and recovery oracle

Source inspection alone is `SOURCE_FACT`, not runtime proof.

## Invariants

- Inject at a **named** boundary; record process, host/container and clock.
- Acknowledgements, spool deletions and offset advances occur only after the required durable boundary.
- Production-destructive testing is prohibited. No arbitrary `docker system prune`, `docker compose down -v`, disk wipe, or shared-environment kill storms.
- Dependency unavailability (DB/Kafka/ClickHouse) is distinct from process kill and must be asserted separately.
- Queue saturation, timeouts and backpressure are first-class cases, not afterthoughts.
- Evidence must be externally observable after recovery (store contents, spool files, offsets, metrics)—not only log grepping.

## Required procedure

1. Name the invariant (e.g. “no spool delete before Core durable ACK”).
2. Select the narrowest injection point from the safe catalogue below.
3. Prepare fixtures and expected post-conditions before injecting.
4. Inject once; capture commands, pids, timestamps and outcomes.
5. Restore/retry per product logic; verify observable state and absence of duplication/loss beyond the contract.
6. Label each catalogue row PASS / FAIL / NOT_RUN. Do not silently skip crash-before-ACK.
7. Package EXECUTION_EVIDENCE pointers for the exact head.

### Safe concrete injection points (catalogue)

Use only lab/Compose fixtures:

| Point | Intent |
|---|---|
| Before reserve | Caller fails before local reservation exists |
| After reserve | Reservation held; process dies before publish |
| Before publish | Reserved but not on wire |
| Kafka commit ambiguity | Produce/commit unclear; consumer/uploader must not double-apply blindly |
| Before finalize | Downstream accepted bytes; local finalize not done |
| After finalize / before ACK | Durable success local; peer ACK not yet observed |
| Lost HTTP/gRPC ACK | Peer succeeded; ACK dropped on network |
| Process kill | Hard kill of Sensor/Core worker mid-hop |
| DB unavailable | PostgreSQL control-state unreachable |
| Kafka unavailable | Internal bus unreachable |
| ClickHouse unavailable | Analytics insert path down |
| Sensor/Core restart | Clean/unclean restart with spool/WAL present |
| Queue saturation | Bounded queue full; admission/backpressure |
| Timeouts | Deadline exceeded on publish/consume/DB |
| Backpressure | 80% storage / ingress reject paths |

## Negative / failure cases

- Claiming runtime proof from code reading alone.
- Do not use `docker compose down -v` or broad prune to “simulate” failure (destroys evidence volumes).
- Injecting in production or shared customer tenants.
- Only testing happy-path retry without crash-before-ACK.
- Passing because metrics incremented while durable state duplicated silently.
- Skipping DB/Kafka/CH unavailability because “unit mocks cover it”.

## Security / tenant implications

Failure drills stay inside disposable tenants/fixtures. Do not dump secrets from crashed process cores into evidence. Cross-tenant fixtures remain isolated. Rate-limit injection automation so lab Kafka/DB are not accidentally DoSed beyond the test’s need.

## Required evidence

- Invariant statement and injection-point row ids exercised
- Exact commands, container names, exit codes, timestamps
- Post-recovery observable state (spool/DB/offset/CH)
- Catalogue rows marked PASS/FAIL/NOT_RUN
- Proof that destructive Docker shortcuts were not used
- Exact SHA; `SOURCE_FACT` vs `EXECUTION_EVIDENCE` labelling

## Stop conditions

Stop when:

- Only destructive shortcuts can create the failure
- Injection point cannot be named against a real boundary
- Production or shared mutable state would be touched
- Head drifts mid-drill—invalidate and restart

## Anti-patterns

- Chaos-without-oracle (crash everything, squint at logs)
- Equating mock `raise Exception` with process-kill evidence
- Deleting volumes that held the spool under study
- Writer self-certifying durability from a single local run without verifier replay

## Handoff / output

Return: catalogue coverage matrix; reproducer; residual NOT_RUN rows; next verifier. Escalate durability gaps through `replay_idempotency` / `ha_dr` as applicable.

## Non-ownership

Does not authorize production chaos, invent HA fencing, weaken tests to green, or close MERGE_READY. Distinguishes runtime evidence from source inference only—it does not replace domain Skills.
