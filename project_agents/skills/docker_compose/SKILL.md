---
name: docker_compose
description: Docker Compose development/test hygiene: named volumes, healthchecks, no destructive prune, reboot-safe local stack.
---

# docker_compose

## Purpose
Use Compose as the normal dev/test environment.

## Use when
compose files, local ports, or test containers.

## Required procedure
1. Do not install project runtimes on the host.
2. No routine `down -v` or docker prune.
3. Keep healthchecks and named volumes.
4. Do not start the full stack unless required.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `docker_compose` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
