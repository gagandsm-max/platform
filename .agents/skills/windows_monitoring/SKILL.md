<!-- generated from project_agents/skills/windows_monitoring/SKILL.md sha256:0c43ce22d155f97de0358b0b5aff61eeb9f015b724016bdf9632c8529deefefa -->

---
name: windows_monitoring
description: Windows Agent service, MSI/BAT install, event/perf evidence and privacy-bounded endpoint collection.
---

# windows_monitoring

## Purpose
Monitor Windows endpoints as operational experience, not surveillance.

## Use when
Windows Agent, MSI, event logs, or endpoint DEX-adjacent metrics.

## Required procedure
1. Keep the Windows Agent a signed service.
2. Prohibit content/surveillance collection.
3. Identity is agent_id/entity_id, not DHCP IP.
4. Prove install/uninstall/reboot start.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `windows_monitoring` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
