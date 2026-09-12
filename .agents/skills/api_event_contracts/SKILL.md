<!-- generated from project_agents/skills/api_event_contracts/SKILL.md sha256:ac80065c7d2a9210d33b395097dc80ec56ce8137afabe53a9fdee21e2968d5e2 -->

---
name: api_event_contracts
description: HTTPS/gRPC/REST and Kafka event contracts, compatibility, error codes and correlation IDs.
---

# api_event_contracts

## Purpose
Change APIs/events without silent contract breaks.

## Use when
Core APIs, Sensor/Agent control, or event schemas.

## Required procedure
1. Version and compatibility-test the contract.
2. Keep stable error codes and correlation IDs.
3. Authn/z is server-side.
4. Do not leak internals on anonymous health.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `api_event_contracts` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
