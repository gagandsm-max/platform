---
name: secrets_handling
description: Credential Profiles, connector secrets, CI credentials, redaction and support-bundle safety.
---

<!-- generated from project_agents/skills/secrets_handling/SKILL.md sha256:f56c1b4acdb28f727de4fe93f8d94f4b1694e92a7123df099eaebb40d9bca85a -->

# secrets_handling

## Purpose

Keep secret material out of git, logs, traces, metrics, support bundles, AI prompts/tool args and evidence bundles—using references, least privilege, rotation/revocation and tenant boundaries.

## Use when

Credential Profiles, connector credentials, mTLS private keys, API tokens, CI secrets, support bundles, env/config changes, AI tool payloads, or any path that might print exception causes. Required for `platform-secrets-credential-engineer`, AppSec and any security-sensitive change.

## Authoritative inputs

Always read:

1. `project_documentation/Platform_18_Security_Threat_Model.md` for secret assets
2. Credential Profile / secret-service contracts in Architecture/LLD for the surface
3. `pki_mtls` when certificates/keys are involved
4. CI masking/OIDC rules for the workflow under change
5. Task `securityBoundaries` and evidence_bundle redaction rules

## Invariants

- **Never commit secrets** (keys, tokens, passwords, raw cert private keys, cloud creds) to git.
- Prefer **secret references** (ids/handles) over secret material in config, Kafka messages, UI props and evidence.
- Logs, errors (including nested causes), traces, metrics labels and support bundles must not emit secret material.
- Env/config files in repos carry references or placeholders—not live values.
- mTLS private keys stay permission-restricted on disk and out of logs/bundles.
- Connector credentials use Credential Profiles; least privilege scopes only.
- Rotation/revocation must be operable; compromised credentials get incident handling.
- Test fixtures use synthetic secrets clearly marked; never production values.
- CI secrets remain masked; do not echo them in scripts.
- AI prompts/tools must not receive secret material (`ai_evidence_safety` tool firewall).
- Tenant boundary: one tenant’s secrets never appear in another tenant’s context.
- Filesystem permissions on key material are restrictive (no world-readable keys).

## Required procedure

1. Inventory secret classes touched (CI, connector, mTLS key, token, DB password, AI provider key).
2. Ensure each is stored in the approved secret service / Credential Profile / CI secret store—not in source.
3. Trace output paths: app logs, structured fields, exception mappers, support bundle collectors, traces, metrics—add redaction or remove fields.
4. Verify config uses references; scan diffs for accidental material (cloud access-key prefixes, PEM private-key headers, chatbot tokens, etc.).
5. Confirm filesystem mode/owner for key material on Sensor/Core/Agent paths under change.
6. Exercise rotation/revocation path or document `NOT_RUN` with owner.
7. For AI surfaces, prove prompts/tool args exclude secrets (negative test).
8. If leakage is detected: revoke/rotate, purge from git history if committed (incident process), invalidate evidence that captured it, notify gatekeeper—do not quietly edit and continue.

## Negative / failure cases

- Sanitized top-level message with secret still in `getCause()` / suppressed exceptions.
- Support bundle shipping `.pem` keys or `.env`.
- Unit tests hard-coding production-like credentials.
- CI printing `env` or curl headers with tokens.
- AI Copilot/tool dump of Credential Profile payloads.
- World-readable private keys on disk after install.
- Cross-tenant credential resolution succeeding.
- “Fix” by disabling TLS or embedding keys “temporarily”.

## Security / tenant implications

This Skill *is* the security/tenant control for secret material. Combine with `tenant_isolation`, `security_negative_testing` and `pki_mtls`. Fail closed if the secret store is unavailable—do not fall back to plaintext defaults.

## Required evidence

- Diff scan result for secret material (PASS/FAIL)
- Redaction tests for logs/exceptions/bundles
- Reference-vs-material config proof
- Key filesystem permission check when files touched
- AI prompt/tool negative case when AI in scope
- Rotation/revocation note or `NOT_RUN`
- Incident actions if leakage found
- Exact SHA; no secret bytes in the evidence bundle itself

## Stop conditions

Stop when:

- Live secret must be committed or pasted into chat/evidence to proceed
- Redaction cannot cover nested framework error paths in scope
- Production credentials are requested as fixtures
- Leakage incident is open and unrevoked

## Anti-patterns

- Base64 as “encryption”
- Sharing `.env` via ticket attachments
- Logging Authorization headers for “debug”
- Committing `*.pem` with “example” names that are real
- Writer self-certifying secret hygiene

## Handoff / output

Return: inventory; redaction/scan results; rotation status; residual risks; next AppSec/secrets verifier. On leakage, hand to incident/gatekeeper immediately.

## Non-ownership

Does not select KMS vendors outside catalogue, invent cryptography, authorize production key export, or close MERGE_READY. Complements—not replaces—`pki_mtls`, `security_negative_testing` and `ai_evidence_safety`.
