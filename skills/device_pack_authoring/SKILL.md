---
name: device_pack_authoring
description: Device Pack schema, identification, mappings, versioning, certification/provenance and test-lab semantics.
---

<!-- generated from project_agents/skills/device_pack_authoring/SKILL.md sha256:6343442cb5f29800316bc8a61a88194490bf0095f49e1e18b3a952979f7c90a4 -->

# device_pack_authoring

## Purpose
Author Device Packs as Platform-owned identification contracts.

## Use when
Pack schema, vendor mappings, pack tests, or certification.

## Required procedure
1. Identification is pack-defined, not IP.
2. Version and provenance are mandatory.
3. Protocol specialists do not own pack schema.
4. Test-lab fixtures required for pack claims.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `device_pack_authoring` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
