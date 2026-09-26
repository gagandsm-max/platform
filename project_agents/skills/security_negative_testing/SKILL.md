---
name: security_negative_testing
description: Prove security boundaries with adversarial and negative cases rather than success-path evidence alone.
---

# security_negative_testing

## Purpose

Prove that security-sensitive Platform paths fail closed, reject unauthorized callers and do not leak protected material—using adversarial and negative cases, not success-path tests alone.

## Use when

Authn/authz, tenant isolation, mTLS enrollment, API/gRPC ingress, Kafka consumers, connector credentials, AI tool firewalls, support bundles, admin actions, or any change that could widen trust boundaries. Required for `platform-appsec-engineer`, `platform-security-architect` and risk-triggered review on privileged paths.

## Authoritative inputs

Always read:

1. `project_documentation/Platform_18_Security_Threat_Model.md`
2. `project_documentation/Platform_10_Protocol_Security_Connector_Master.md` when protocols/connectors change
3. `project_agents/tool_authority.json` and task `securityBoundaries[]`
4. Exact authn vs authz contracts for the surface (Keycloak/OIDC, mTLS SPIFFE-like sensor/agent identity, API tokens)
5. Existing negative test harnesses for the module under change

Threat-model assets/actors/boundaries for the change class before writing “happy path green” claims.

## Invariants

- Authentication and authorization are distinct: proving identity ≠ proving permission on a tenant/resource.
- Fail closed on authn/authz/policy errors; never fail open to keep availability cosmetics.
- Negative coverage is mandatory for new or changed trust boundaries.
- Secrets must not appear in logs, exception cause chains, traces, metrics labels, support bundles or AI prompts.
- Replay/idempotency after rejection must not ACK/advance offsets or commit side effects.
- Tests themselves stay bounded: no production blast radius, no cross-tenant live data, no destructive Docker prune as a shortcut.

## Required procedure

1. Name the trust boundary and protected asset (tenant data, credential, admin action, offset, cert).
2. Build a negative matrix covering, as applicable:
   - unauthenticated calls
   - wrong identity (valid cert/token for a different principal)
   - revoked/expired credentials
   - cross-tenant resource identifiers
   - wrong resource scope / RBAC role
   - malformed and oversized payloads
   - injection (SQL/LDAP/command/log/prompt) where the surface parses untrusted input
   - replay of captured requests and duplicate idempotency keys after denial
   - privilege escalation (user→admin, sensor→core admin, AI tool escalate)
   - secret leakage across logs/exceptions/bundles/traces
   - rate/resource exhaustion and storm controls
   - mTLS missing/untrusted/expired client certs
   - fail-open vs fail-closed configurations
3. Distinguish expected authn failure (401/unauthorized transport) from authz failure (403/permission) in assertions.
4. Exercise framework error handlers, nested causes, retry wrappers and listener containers—not only the top-level sanitized message.
5. Verify audit evidence: actor, tenant, decision, correlation id, timestamp without secret material.
6. Record blast-radius controls used for the test (fixtures, ephemeral tenants, Docker Compose—not production).

## Negative / failure cases

- Success-path-only tests used as “security evidence”.
- Sanitized top-level exception while `getCause()` / suppressed exceptions still leak secrets.
- Consumer ACKs poison/unauthorized messages.
- TLS validation disabled in tests and left disabled in sample config.
- Cross-tenant IDOR via UUID enumeration succeeding.
- AI/tool path that treats prompt text as authorization.
- Rate-limit tests that only check metrics, not rejection behavior.
- “Unauthorized” asserted without proving which layer failed (authn vs authz).

## Security / tenant implications

Negative tests must not become an attack tool against shared environments. Use isolated Compose projects, synthetic tenants and disposable credentials. Never commit real certs/keys. Redact captured artifacts before attaching to `evidence_bundle`. Cross-tenant cases use two synthetic tenants—not production customer ids.

## Required evidence

- Negative matrix rows with PASS/FAIL/NOT_RUN
- Exact commands/test ids and SHA
- Authn vs authz distinction proof
- Leakage scan results for logs/exceptions/bundles (or `NOT_RUN` with reason)
- Replay/idempotency-after-reject proof for messaging paths
- mTLS failure cases when transport is mTLS
- Audit event samples (redacted)
- Fail-closed configuration evidence

## Stop conditions

Stop claiming security readiness when:

- Any applicable matrix row for a changed boundary is missing without `NOT_RUN` + owner
- Fail-open behavior is required to pass tests
- Threat model lists an unresolved P0 for the boundary
- Evidence would require production attack traffic
- Writer asks to self-certify security review

## Anti-patterns

- `assertEquals(200)` only
- Catch-and-ignore authorization exceptions in tests
- Using admin tokens for “everyone is authorized” fixtures
- Copying real customer payloads into unit tests
- Disabling security filters in `@Test` configuration that can leak into main

## Handoff / output

Return: boundary list; negative matrix; leakage/replay/mTLS results; residual `UNVERIFIED`; next owner (`platform-appsec-engineer` / independent verifier). Security review remains independent from the implementing writer.

## Non-ownership

Does not grant production pentest authority, invent crypto constructions, weaken TLS to green CI, or close MERGE_READY. Complements `tenant_isolation`, `pki_mtls`, `secrets_handling` and `ai_evidence_safety` rather than replacing them.
