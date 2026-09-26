---
name: sensor_edge
description: Sensor edge runtime—scheduling, durable spool, uploader/replay, resource lifecycle and self-observability.
---

<!-- generated from project_agents/skills/sensor_edge/SKILL.md sha256:1c3aeccada49e1284fea3883c5e85b026ce524b6feb2f0b091e082b1c92938f4 -->

# sensor_edge

## Purpose

Operate Sensors as edge collectors with durable spool, correct ACK boundaries, ownership/assignment, backpressure and storage protection—separating collector health from target DOWN, without inventing desired-state authority beyond BUILD-004.

## Use when

Sensor workers, spool/uploader, assignment/leases, protocol isolation, restart recovery, or Sensor self-health. Required for `platform-sensor-runtime-engineer`.

## Authoritative inputs

Always read:

1. Existing Sensor runtime/spool/uploader code on the exact head
2. Architecture: Sensors continue collecting during Core failover within capacity
3. 80% storage protection rules in `AGENTS.md`
4. `go_runtime`, `replay_idempotency`, `pki_mtls`, `monitoring_work_units`
5. BUILD-004 / desired-state boundaries—do not invent unassigned authority

## Invariants

- Sensor roles: collect, buffer, upload—not Core policy authority.
- Collection vs target state separated; **No Data ≠ DOWN**.
- Durable spool retains until durable application ACK; no pre-ACK delete.
- Ownership/assignment of work units fenced; scheduling fair and jittered.
- Backpressure and **80% storage protection** enforced.
- Protocol isolation: one flood (flow/syslog) must not kill all workers.
- Restart recovery replays unacked spool.
- Network/Core outage ⇒ continue collect/buffer within bounds.
- Agent ingress (if present) is tenant-scoped and mTLS-authenticated.
- Local health/self-monitoring exported.
- Fail-closed on authz/identity failures.
- Do not invent desired-state / config authority beyond assigned BUILD scope.

## Required procedure

1. Identify runtime surfaces touched (scheduler, spool, uploader, listeners).
2. Verify ACK vs delete semantics with tests.
3. Confirm storage-protection behaviour under fill.
4. Separate collector errors from target alert semantics in any touched paths.
5. Test restart with pending spool; Core unreachable scenario.
6. Bound concurrency per protocol worker.
7. Keep desired-state features out unless BUILD task explicitly includes them.

## Negative / failure cases

- Deleting spool on HTTP 500 without ACK.
- Declaring devices DOWN because Sensor CPU saturated.
- One UDP flood starving SNMP polls.
- Using Sensor as unconstrained remote shell host.
- Inventing BUILD-004 desired-state APIs early.

## Security / tenant implications

Sensor holds credentials for polling—Credential Profiles, least privilege. mTLS to Core. Tenant assignment enforced. Spool may contain sensitive telemetry—protect disk ACLs. Compromise of Sensor is site-scoped risk.

## Required evidence

- Spool ACK/restart tests
- Storage-protection notes
- Isolation/backpressure evidence or `UNVERIFIED`
- Exact SHA

## Stop conditions

Stop when:

- Pre-ACK delete possible
- No Data collapsed to DOWN improperly
- Storage protection absent under load claims
- Unassigned desired-state authority introduced

## Anti-patterns

- Infinite in-memory queues “for speed”
- Disabling mTLS to Core
- Writer self-certifying Sensor runtime safety

## Handoff / output

Return: runtime model; durability evidence; gaps; nextOwner (Sensor verifier/SRE). Not MERGE_READY alone.

## Non-ownership

Does not own Core HA design, invent BUILD-004 scope, or replace protocol Skills. Owns Sensor edge runtime discipline.
