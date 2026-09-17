---
name: snmp_mib
description: SNMP/MIB collection, SNMPv3 security, counter semantics and Device Pack mapping without declaring device-down from SNMP alone.
---

<!-- generated from project_agents/skills/snmp_mib/SKILL.md sha256:c09055536d04ef62bd1e90f82d0697c09d749a5a96d59647f2f1b02cbce18d54 -->

# snmp_mib

## Purpose
Collect SNMP evidence with protocol-correct failure classification.

## Use when
SNMP polling, MIB mapping, or SNMP auth/timeout defects.

## Required procedure
1. Separate SNMP failure from device failure.
2. Preserve SNMPv3 auth/privacy constraints.
3. Map via Device Pack identifiers, not ad-hoc OIDs.
4. Corroborate reachability before DOWN.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `snmp_mib` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
