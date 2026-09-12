<!-- generated from project_agents/skills/topology_routing/SKILL.md sha256:fb2454fb10f1cef5089f33ad6458c1c80ec649d555d9f9cf0b7dd7d8a51a38fb -->

---
name: topology_routing
description: Routing/topology discovery, network-service dependencies (DNS/DHCP/NTP/AAA) and provenance/confidence on graph edges.
---

# topology_routing

## Purpose
Build topology as evidenced relationships, not guessed drawings.

## Use when
LLDP/routing/protocol topology or shared-service dependency edges.

## Required procedure
1. Keep provenance and confidence on edges.
2. Shared services are dependency evidence, not BMS/DCIM.
3. IP/MAC are observations, not identity.
4. Do not silently merge conflicting nodes.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `topology_routing` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
