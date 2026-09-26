---
name: threat_modelling
description: Threat-model trust boundaries, privileged actions, Device Packs, AI tools and update channels before implementation.
---

<!-- generated from project_agents/skills/threat_modelling/SKILL.md sha256:59b59cd04a78de3b944e9886b792cdb6704828a6392930f3209a3ec9fab33bb9 -->

# threat_modelling

## Purpose

Produce actionable threat models for Platform changes: trust boundaries, assets, actors, STRIDE-style enumeration, mitigations and residual risk—feeding testable security requirements before implementation hardens the wrong thing.

## Use when

New trust boundaries, privileged remediation, Device Packs, AI tool firewalls, update channels, connectors, or authn/z redesigns. Required before implementation of security-sensitive work (`platform-security-architect`).

## Authoritative inputs

Always read:

1. `Platform_18_Security_Threat_Model.md`
2. Architecture trust boundaries (UI, Core, Sensor, Agent, IdP, connectors)
3. Task scope and data classification
4. Existing mitigations/tests on the exact head
5. Related Skills (`tenant_isolation`, `pki_mtls`, `ai_evidence_safety`, `remediation_safety`)

## Invariants

- Trust boundaries, assets and actors are named explicitly.
- Entry points enumerated (API, gRPC, UDP listeners, webhooks, packs, AI tools).
- STRIDE-style or equivalent threat enumeration covers spoofing, tampering, repudiation, info disclosure, DoS, elevation.
- Authentication, authorization and tenant isolation threats called out.
- Secrets, spoofing, tampering, replay, SSRF, injection, denial/resource exhaustion covered when relevant.
- Supply chain (deps, packs, update channels) included for those surfaces.
- Privileged remediation and AI prompt/tool boundaries modelled—prompt cannot grant privilege.
- Likelihood/impact recorded qualitatively without fake precision.
- Mitigations map to requirements/tests; residual risk explicit.
- Evidence and closure tracking for accepted risks.
- Unimplemented mitigations marked `UNVERIFIED`/open—not silently “done.”

## Required procedure

1. Draw trust-boundary diagram for the change (text acceptable).
2. List assets (keys, tenant data, remediations, packs).
3. Enumerate actors (operator, attacker-on-network, malicious tenant user, compromised Sensor).
4. Run STRIDE-style enumeration per entry point.
5. Propose mitigations with owners; link to tests/Skills.
6. Record residual risk and acceptance authority.
7. Feed AppSec/negative-test plans; do not stop at prose.
8. Revisit model when design changes materially.

## Negative / failure cases

- “HTTPS so we’re safe” as the entire model.
- Ignoring UDP syslog spoofing.
- Allowing LLM tools to call remediation without allowlist.
- Accepting critical residual risk without Founder/security authority.
- Copying a generic STRIDE table unrelated to the change.

## Security / tenant implications

Threat models may themselves be sensitive—limit distribution. Cross-tenant attacker is a first-class actor. Update-channel compromise is estate-wide risk.

## Required evidence

- Boundary/asset/actor list
- Threat table with mitigations
- Residual risk register
- Linked test IDs or `UNVERIFIED`
- Exact SHA

## Stop conditions

Stop when:

- Implementation proceeds on privileged surface with no model
- Critical threat has no mitigation or acceptance
- AI/remediation privilege escalation unaddressed
- Supply chain ignored for new OSS/pack install path

## Anti-patterns

- Checkbox STRIDE with empty cells
- Retroactive model after production incident only
- Writer self-certifying security design

## Handoff / output

Return: threat model artefact; open risks; nextOwner (AppSec/security architect verifier). Not MERGE_READY alone.

## Non-ownership

Does not replace penetration tests, invent residual-risk acceptance, or implement all mitigations alone. Owns threat-modelling discipline before sensitive builds.
