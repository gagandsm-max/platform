# ITSM Integration

**Class:** Derived feature brief — non-authoritative
**Canonical sources:** Platform_02, 10, 17, 19, 21, 23
**Draft enhancement:** PR #61

## Product objective

Integrate Platform alerts/incidents/changes with customer ITSM systems through one reusable connector runtime, stable object binding and explicit lifecycle authority.

## Connector model

A connector defines:

- vendor/product/version capability;
- endpoint/proxy/TLS/mTLS;
- authentication Credential Profile;
- field/state mapping;
- retry/backoff/rate limits;
- idempotency/correlation;
- durable outbound queue;
- dead-letter/replay;
- health and audit;
- test sandbox and readiness checks.

Vendor-specific logic belongs in Integration/Capability Packs, not Core forks.

## Reference implementation order

Draft PR #61 proposes ServiceNow + Generic REST/Webhook as reference-grade first implementations, followed through the same runtime by JSM, BMC, ManageEngine/ServiceDesk Plus, Freshservice/Freshworks, SummitAI and other governed targets.

This is draft architecture, not a certification claim.

## Lifecycle

Platform maintains a stable binding between its authoritative Alert/Incident/Change object and the remote ITSM object. Create/update/comment/recovery/resolve/reopen behavior is connector-capability aware.

Remote ticket closure does **not** change monitored health. External job success does **not** prove operational recovery.

## Mapping and setup UX

Guided setup should provide:

- vendor/version selection;
- DNS/TLS/proxy/auth/permission/scope preflight;
- metadata/schema discovery where API permits;
- type-aware required-field/state mapping;
- exact post-egress payload preview;
- Dev/Test/UAT/Production promotion with semantic diff and credential rebinding;
- readiness report;
- configuration drift warnings.

Arbitrary customer Python/JavaScript mapping code is not the default extension model.

## Reliability

Queues, circuit breakers, idempotency, retry, dead letter, pause/drain/replay and sanitized support evidence are first-class. One failed connector cannot block another.

## Security

Tenant/resource scope, least-privilege remote credentials, outbound destination validation, secret redaction and audit are mandatory. Plain unkeyed hashing is not treated as a privacy/security boundary.

## Implementation note

PR #61 is open/draft and currently conflicts with newer main. Use its architecture as review input only until rebased/reconciled and merged.
