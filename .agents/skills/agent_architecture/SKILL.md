---
name: agent_architecture
description: Endpoint Agent identity, enrollment/mTLS, durable local collection and privacy-bounded operational telemetry.
---

<!-- generated from project_agents/skills/agent_architecture/SKILL.md sha256:93213c99e29ee2c3ff4e6dd7721b4acbca3e426e5d0a8eb7093810124622ab5c -->

# agent_architecture

## Purpose
Change Agent architecture without creating duplicate endpoints on IP change.

## Use when
Linux/Windows Agent control plane, enrollment, or local spool.

## Required procedure
1. Anchor identity to entity_id + agent_id/mTLS.
2. Reinstall/clone collisions require reconciliation.
3. Bound local storage and privacy.
4. Transport is HTTPS/gRPC over mTLS.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `agent_architecture` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
