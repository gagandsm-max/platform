---
name: icmp_path
description: ICMP/path reachability, RTT/loss/timeout evidence and path diagnostics as corroboration, not sole identity.
---

<!-- generated from project_agents/skills/icmp_path/SKILL.md sha256:8aa102c28bb1764e1e90a575bf6e5f4df4d66e72bd9902673792a8456e941be5 -->

# icmp_path

## Purpose
Use ICMP as evidence, not as inventory identity.

## Use when
Ping/path checks, loss/latency monitors, or reachability corroboration.

## Required procedure
1. Bound probe rate and timeouts.
2. Record RTT/loss/timeout distinctly.
3. Do not create entities from changing IPs.
4. Combine with other evidence before DOWN.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `icmp_path` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
