---
name: device_control_protocols
description: SSH/CLI, NETCONF/YANG and gNMI/gRPC device control/telemetry with credential, timeout and blast-radius constraints.
---

<!-- generated from project_agents/skills/device_control_protocols/SKILL.md sha256:d3e91ace11989303e4d1b22665ebda377164961e7f8da7f8cfb7b79da595ae03 -->

# device_control_protocols

## Purpose
Talk to devices through approved control protocols without unsafe command execution.

## Use when
CLI/NETCONF/gNMI collection or NCM execution adapters.

## Required procedure
1. Use catalogue protocol libraries.
2. Credentials from Credential Profiles only.
3. Timeouts/retries bounded; no interactive shells.
4. Privileged commands require NCM remediation workflow.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `device_control_protocols` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
