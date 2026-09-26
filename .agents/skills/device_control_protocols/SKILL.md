---
name: device_control_protocols
description: SSH/CLI, NETCONF/YANG and gNMI/gRPC device control/telemetry with credential, timeout and blast-radius constraints.
---

<!-- generated from project_agents/skills/device_control_protocols/SKILL.md sha256:1888f3e00bb3bd5f28af79595a54fe3cdfb1b2643bf60778193e8de11295c165 -->

# device_control_protocols

## Purpose

Execute device control and privileged telemetry (SSH/CLI, NETCONF/YANG, gNMI) with strict read/write separation, authorization, blast-radius limits and fail-closed behaviour.

## Use when

On-demand diagnostics that touch device control channels, NCM-related command execution, gNMI/NETCONF collectors, or any write/config action toward a device. Required for `device_control` paths and NOC troubleshooting workflows that use Platform-controlled channels.

## Authoritative inputs

Always read:

1. `Platform_10_Protocol_Security_Connector_Master.md` and Architecture trust boundaries
2. `Platform_18_Security_Threat_Model.md` for privileged actions
3. Credential Profile contracts; never browser-supplied reusable device passwords
4. Existing Core/Sensor/Agent control path code on the exact head
5. Approval/RBAC/JIT requirements for write actions in Mother UI/security docs

If dry-run, maker-checker or automated rollback is not implemented for a path, mark `UNVERIFIED`—do not claim it.

## Invariants

- **Read vs write** operations are classified before execution; writes require elevated authz.
- Authorization is server-side (tenant, RBAC, resource scope); prompt/LLM text cannot grant privilege.
- Device capability checks precede commands (features/yang models/pack support)—no blind exec.
- Protocol timeout/retry are bounded; no infinite hang on SSH/NETCONF sessions.
- Command safety: deny destructive patterns unless explicitly approved and allowlisted.
- Blast-radius limits: target set, concurrency and rate are capped.
- Approvals/audit where Mother requires maker-checker or reason codes.
- Rollback/fail-closed: failed writes do not leave silent partial config without recorded state; if rollback automation is absent, stop and escalate rather than invent it.
- Sessions run through Platform Core/Sensor/Agent paths—not direct browser-to-device.

## Required procedure

1. Classify the action as read-only diagnostic vs write/config/remediation.
2. Verify caller authorization, tenant scope and (for writes) approval/JIT/reason requirements.
3. Resolve credentials via Credential Profiles; confirm protocol (SSH/NETCONF/gNMI) and capability.
4. Set timeouts, retry budget and concurrency caps; refuse unbounded fan-out.
5. For writes: confirm allowlisted command/template, preconditions and blast-radius envelope.
6. Execute via Platform-controlled path; capture transcript/evidence with redaction.
7. On failure: fail closed; do not retry blindly; record partial state; escalate if rollback is required but `UNVERIFIED`.
8. Emit audit events (actor, target, action, result, correlation ID).

## Negative / failure cases

- LLM-proposed CLI executed without allowlist/authz.
- Treating SSH timeout as device DOWN without corroboration.
- Broad “run on all devices in site” without blast-radius cap.
- Storing enable passwords in chat or tickets.
- Browser connecting straight to device with pasted credentials.

## Security / tenant implications

Privileged credentials never logged. Cross-tenant device control is prohibited. Export of session transcripts follows redaction/retention rules. Break-glass paths remain audited.

## Required evidence

- Authz/approval decision record
- Capability check proof
- Timeout/retry/blast-radius settings
- Audit/transcript references (redacted)
- Rollback status or explicit `UNVERIFIED`
- Exact SHA

## Stop conditions

Stop when:

- Write path lacks required approval controls
- Capability unknown and command is destructive
- Rollback required but not implemented (`UNVERIFIED` block)
- Direct-to-device networking is requested

## Anti-patterns

- “Just SSH and see”
- Disabling command allowlists for speed
- Writer self-certifying NCM safety
- Using remediation Skill output as auto-approve

## Handoff / output

Return: action class; authz proof; evidence IDs; residual risk; nextOwner (`remediation_safety` / NCM verifier / AppSec). Not MERGE_READY alone.

## Non-ownership

Does not own alert semantics, pack authoring, or invent automated rollback engines. Owns protocol control-path safety discipline.
