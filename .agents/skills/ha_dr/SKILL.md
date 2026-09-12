<!-- generated from project_agents/skills/ha_dr/SKILL.md sha256:cd54c12a8b6cd1191d8be512b66f5e70cf7f684fb6738f21895afd68bd5120d4 -->

---
name: ha_dr
description: Exactly-two-Core Active/Standby HA/DR operability, failover evidence and Sensor continue-to-collect behavior without inventing unresolved replication designs.
---

# ha_dr

## Purpose
Preserve the certified HA shape and make failover evidence explicit.

## Use when
Failover, standby promotion, Core restart, or DR documentation/tests.

## Required procedure
1. Keep standalone=1 Core and HA=exactly two full Cores.
2. Label unresolved Kafka/CH/PG fencing as P0 spike.
3. Sensors continue collecting/buffering while capacity permits.
4. Record RPO/RTO only when evidenced.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `ha_dr` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
