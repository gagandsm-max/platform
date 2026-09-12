<!-- generated from project_agents/skills/virtualization_storage/SKILL.md sha256:ac27beeabfab85877add4a8c5281e20d7295e9797d44fad13b03a8ca21c69e2f -->

---
name: virtualization_storage
description: Virtualization, datastore and storage-target monitoring without turning Platform into DCIM/BMS.
---

# virtualization_storage

## Purpose
Monitor virt/storage as infrastructure dependencies.

## Use when
VM, hypervisor, SAN/NAS, or volume metrics.

## Required procedure
1. Normalize vendor IDs into Platform entities.
2. Facilities/power may explain, not expand product scope.
3. Preserve tenant isolation on shared hypervisors.
4. Capacity evidence must include protection thresholds.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `virtualization_storage` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
