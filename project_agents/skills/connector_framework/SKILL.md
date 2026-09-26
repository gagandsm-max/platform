---
name: connector_framework
description: Connector framework auth, rate limiting, retry, idempotency, outbound durability and certification boundaries.
---

# connector_framework

## Purpose

Build and operate outbound/inbound connectors so credential isolation, rate limits and error normalization hold—and connector failure never becomes target DOWN by itself.

## Use when

ITSM, SaaS, webhook, carrier/change-telemetry, SCIM or other connector work; credential profiles for connectors; outbound durability; certification of a connector pack. Required for `platform-integration-connector-engineer`.

## Authoritative inputs

Always read:

1. Mother Architecture / `Platform_07` for approved connector dependencies
2. Credential Profile / secrets Skills and connector secret storage contracts
3. Existing connector SDK/interfaces on the exact head (verify what is implemented)
4. Rate-limit, retry and idempotency patterns already in-tree
5. `Platform_19`/`Platform_21` when the connector feeds OIV or enterprise-gap surfaces

If a circuit breaker, outbound spool or certification harness is not present in code, treat it as `UNVERIFIED`/future—do not claim it ships.

## Invariants

- Credential isolation: connector secrets are tenant-scoped Credential Profiles—never embedded in pack code or logs.
- Tenant/site ownership of connector instances is authoritative; vendor IDs never override Platform entity authority.
- Timeout, retry and backoff are bounded and idempotent where side effects exist.
- Rate limits and (when implemented) circuit breaking protect Core and the remote API.
- Pagination of remote APIs must be complete and resumable without silent gaps.
- API drift: remote schema changes fail closed or degrade with explicit health—not silent mis-parse as success.
- Error normalization maps vendor errors to Platform classes without leaking secrets.
- Secret rotation must not require downtime procedures that copy secrets into tickets/chat.
- Observability: connector health, lag, error rate and auth failures are first-class.
- **Connector failure ≠ monitored target DOWN.** Collector/connector health is evaluated separately from resource operational state.

## Required procedure

1. Confirm the connector type and whether code paths exist on this SHA; list `UNVERIFIED` gaps.
2. Wire credentials only through approved secret/profile mechanisms; prove redaction in logs.
3. Define timeout/retry/backoff and idempotency keys for mutating calls.
4. Enforce rate limits; document remote quota assumptions.
5. Implement pagination/resume; test empty, partial and error mid-page.
6. Normalize errors; add tests for auth failure, 429, 5xx, malformed payload.
7. Separate connector/collector health signals from target alert semantics in any alert path touched.
8. On secret rotation, verify old/new credential cutover without logging secret material.
9. Record outbound durability behaviour if present; otherwise mark durability claims `UNVERIFIED`.

## Negative / failure cases

- Declaring device/service DOWN solely because Jira/ServiceNow/SaaS API failed.
- Storing API tokens in connector YAML committed to git.
- Unbounded retry storms against a failing vendor.
- Treating HTTP 200 with wrong schema as success.
- Cross-tenant webhook delivery from a shared misconfigured endpoint.

## Security / tenant implications

Least-privilege OAuth/scopes. Tenant isolation on ingress webhooks (signature + tenant binding). No SSRF via user-controlled connector URLs without allowlist policy. Support bundles redact connector secrets.

## Required evidence

- Credential handling proof (no secret in git/logs)
- Retry/rate-limit/timeout configuration + tests
- Error-normalization fixtures
- Health-vs-target-DOWN separation evidence
- Exact SHA; `UNVERIFIED` capability list

## Stop conditions

Stop when:

- Secrets cannot be isolated
- Failure would collapse into target DOWN without a design fix
- Remote API contract is unknown and fixtures cannot be produced
- Required Mother/OSS approval for a new dependency is missing

## Anti-patterns

- “Best effort sync” without idempotency
- Copying vendor status text as Platform severity
- Writer self-certifying connector certification without harness evidence
- Disabling TLS verify to “make the demo work”

## Handoff / output

Return: connector health model; secret/retry evidence; DOWN-separation statement; nextOwner (integration verifier / AppSec). Not MERGE_READY alone.

## Non-ownership

Does not own Core alert policy engine, invent SaaS product packs as certified, or replace `secrets_handling`. Owns connector runtime discipline.
