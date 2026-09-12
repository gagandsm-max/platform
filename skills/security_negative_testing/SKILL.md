<!-- generated from project_agents/skills/security_negative_testing/SKILL.md sha256:9aae151a1215d2c319754332b1b1d7dec96ebb55460c2fa2d06dafdc8c5c5e4c -->

# security_negative_testing

## Purpose
Prove that security-sensitive paths fail closed and do not leak protected material.

## Required coverage where applicable
- Unauthorized, cross-tenant and wrong-scope access.
- Malformed/untrusted input and boundary values.
- Direct logs and structured log attributes.
- Exception messages and every nested cause crossing framework boundaries.
- Framework error handlers/listener containers and retry/recovery wrappers.
- Support bundles, traces and telemetry/export paths.
- ACK/offset/idempotency behavior after rejection.

## Guardrails
- A sanitized top-level exception is insufficient if a raw cause survives.
- Do not assert absence of leakage without exercising the configured framework path.
- Security fixes must not create fail-open acknowledgement or offset advancement.
