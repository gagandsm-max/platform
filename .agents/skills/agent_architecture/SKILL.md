---
name: agent_architecture
description: Endpoint Agent identity, enrollment/mTLS, durable local collection and privacy-bounded operational telemetry.
---

<!-- generated from project_agents/skills/agent_architecture/SKILL.md sha256:66279da6fb5f10f31cf2ffb55a30571ee6048e2e236aa739a13a7dca21abfdd8 -->

# agent_architecture

## Purpose

Govern Endpoint Agent identity, enrollment, mTLS, durable local spool and lifecycle so Agents remain tenant-bound, replay-safe and privacy-minimized—without treating IP as identity.

## Use when

Agent install/enroll/reinstall, mTLS identity, Agent→Sensor ownership, local spool/retry, Agent upgrade/restart, privacy-bounded telemetry, or endpoint lifecycle/reconciliation work. Required for `platform-` Agent paths and `Platform_20` identity concerns.

## Authoritative inputs

Always read:

1. `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md` (mandatory for Agent identity/onboarding)
2. Root `AGENTS.md` Agent identity / mTLS / enrollment non-negotiables
3. Existing Agent enroll, spool and transport code on the exact head
4. `pki_mtls`, `replay_idempotency` and `secrets_handling` Skills
5. Task stopConditions for unimplemented control-plane functions

If a control-plane function (remote wipe, live process kill, etc.) is not in code, mark `UNVERIFIED`—do not claim it.

## Invariants

- **`agent_id` vs `entity_id`:** Agent has unique `agent_id`/mTLS identity; endpoint inventory anchors to canonical `entity_id`. They are related, not interchangeable.
- Enrollment is governed; newly enrolled Agents do not silently become licensed/actively monitored without authorized policy.
- mTLS identity is the trust anchor—not hostname or IP.
- Reinstall/clone/image-collision requires explicit reconciliation; never silent merge or duplicate inventory.
- Identity continuity survives IP/DHCP/VPN/Wi-Fi changes; **no IP-address identity**.
- Tenant/site binding is server-authoritative at enrollment and must not be client-asserted alone.
- Agent→Sensor ownership/assignment is explicit; orphaned Agents are degraded, not reinvented.
- Durable local Agent spool retains telemetry until durable application ACK; **no telemetry deletion before ACK**.
- Sequence lineage supports retry/replay without silent loss or reorder corruption.
- Local disk bounds + storage protection apply; spool must not fill the host unbounded.
- Credentials/private keys stay in protected stores—never logs, support chat or world-readable files.
- Upgrade/restart preserves identity and unacked spool (within bounds).
- Privacy/data minimization: operational telemetry only—not employee surveillance content.
- Lost Sensor/Core connectivity: continue collect/buffer within bounds; fail closed on privilege.
- Unimplemented control-plane functions remain `UNVERIFIED`.

## Required procedure

1. State whether the change touches identity, enrollment, transport, spool or privacy surface; verify code exists on this SHA.
2. Confirm `agent_id`/`entity_id` separation and reconciliation rules for reinstall/clone.
3. Verify mTLS enroll/rotate paths; reject untrusted certs; never log private keys.
4. Prove IP/hostname changes do not mint a new endpoint solely from network observations.
5. Check tenant/site binding and Agent→Sensor ownership on assign/reconnect.
6. Exercise spool: write, retry, replay, ACK delete; inject lost connectivity.
7. Enforce disk bounds; record behaviour at protection threshold.
8. Review telemetry fields for minimization; remove/reject prohibited content classes.
9. Mark any requested remote-control capability `UNVERIFIED` if absent.
10. Add negative tests: clone collision, cross-tenant enroll, delete-before-ACK regression.

## Negative / failure cases

- Creating a new entity because DHCP changed the IP.
- Silent merge of two Agents after clone.
- Deleting spool on upload HTTP error without durable ACK.
- Embedding Agent private key in enrollment logs.
- Treating Agent disconnect as host DOWN without corroboration policy.

## Security / tenant implications

Enrollment tokens and keys are secrets. Cross-tenant Agent attach is prohibited. Support bundles redact key material. Privacy-bounded collection cannot be widened by prompt/LLM. Reconciliation decisions are audited (`Platform_20`).

## Required evidence

- Identity/enrollment test results (including collision cases)
- Spool ACK/retry/connectivity evidence
- Disk-bound / protection notes
- Privacy field review
- Exact SHA; `UNVERIFIED` control-plane list

## Stop conditions

Stop when:

- Identity would be IP-keyed
- Spool can delete pre-ACK
- Cross-tenant enrollment possible
- Clone collision would silent-merge
- Required `Platform_20` controls cannot be met

## Anti-patterns

- “Hostname is unique enough”
- Disabling mTLS in lab and shipping the same config
- Writer self-certifying Agent identity safety
- Inventing surveillance collection “for RCA”

## Handoff / output

Return: identity model; spool/ACK evidence; reconciliation notes; nextOwner (identity/AppSec/Agent verifier). Not MERGE_READY alone.

## Non-ownership

Does not own Core HA, Device Packs, or invent Agent remote shell productization. Owns Agent identity and durable collection discipline.
