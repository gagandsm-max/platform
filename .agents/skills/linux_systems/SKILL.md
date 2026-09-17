---
name: linux_systems
description: Linux OS, systemd, process, disk, and host-path semantics for Core/Sensor/Agent packaging and monitoring targets.
---

<!-- generated from project_agents/skills/linux_systems/SKILL.md sha256:add2d9121a9473b5d90b8b123a21f4d5ebebd712230db29ecbd72fd4ffb70e8e -->

# linux_systems

## Purpose
Change Linux-facing behavior with service lifecycle and disk protection in mind.

## Use when
systemd units, host metrics, package postinst, or Linux monitoring.

## Required procedure
1. Services must auto-start and supervise recovery.
2. Honor 80% disk protection.
3. Do not require host Python/Java/Go installs for tests.
4. Prove reboot/start evidence when lifecycle changes.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `linux_systems` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
