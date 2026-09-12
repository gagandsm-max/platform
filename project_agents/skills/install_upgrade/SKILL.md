---
name: install_upgrade
description: deb/rpm/MSI packaging, systemd lifecycle, upgrade/rollback and reboot auto-start evidence.
---

# install_upgrade

## Purpose
Change install/upgrade without silent data loss.

## Use when
Packaging, postinst, upgrade, rollback, or service files.

## Required procedure
1. Core/Sensor/Agents auto-start after reboot.
2. Upgrade path from supported origins.
3. Rollback/compat assessed; no assumed down migrations.
4. Crash-loop diagnostics must remain visible.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `install_upgrade` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
