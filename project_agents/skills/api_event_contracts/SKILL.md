---
name: api_event_contracts
description: HTTPS/gRPC/REST and Kafka event schemas, compatibility, error codes and correlation IDs.
---

# api_event_contracts

## Purpose

Change HTTPS/gRPC/REST and Kafka event contracts without silent breaks—versioning, tenant scope, idempotency and error classes stay explicit and testable.

## Use when

Core APIs, Sensor/Agent control planes, ingestion envelopes, Kafka topics/schemas, pagination contracts, or any claim about client/server compatibility across releases. Required for `platform-api-platform-engineer` and contract-touching BUILD work.

## Authoritative inputs

Always read:

1. `project_documentation/Platform_17_API_Event_and_Data_Model_Contracts.md` when present
2. Existing OpenAPI/proto/Avro/JSON Schema artefacts and fixture catalogues for the exact head
3. Mother Architecture transport rules (external HTTPS/gRPC mTLS; Kafka internal)
4. Error-code / correlation-ID conventions already used by Core
5. Task `stopConditions` and compatibility window for the change

Do not invent a new public API surface or event family without Mother/Decision Register authority. If a fixture or schema file is absent, mark `UNVERIFIED`—do not claim the product has it.

## Invariants

- Schema/versioning compatibility is explicit (additive preferred; breaking changes require version bump + migration plan).
- Stable resource/event IDs and correlation IDs are mandatory on material paths.
- Tenant scope is server-enforced on every request/event; clients cannot assert cross-tenant access.
- Timestamps use documented semantics (event time vs ingest time); do not mix silently.
- Error classes are stable codes/bodies—do not leak internals on anonymous or unauthenticated paths.
- Idempotency keys/effects are defined where retries are expected (Sensor upload, control actions, connectors).
- Pagination contracts (cursor/limit/continuation) must not skip/duplicate under concurrent mutation without documented behaviour.
- Unknown fields: forward-compatible readers ignore or reject per stated policy—never crash Core.
- Backward compatibility with in-field Sensors/Agents in the supported skew window is required unless an explicit break is approved.
- Contract fixtures and negative tests are part of Definition of Done for contract changes.

## Required procedure

1. Identify every consumer (UI, Sensor, Agent, connector, Kafka consumer) of the changed contract.
2. State versioning strategy and whether the change is additive, breaking or dual-write transitional.
3. Preserve/extend correlation IDs, tenant fields and idempotency semantics; document timestamp fields.
4. Enumerate error classes; ensure unauthenticated surfaces stay information-poor.
5. Add/update fixtures for golden request/response/event shapes.
6. Add negative tests: missing tenant, unknown fields, stale version, bad pagination cursor, duplicate idempotency key.
7. Run contract/static tests on the exact SHA; record results.
8. If runtime consumers are not yet built for a field, mark behaviour `UNVERIFIED` rather than asserting production support.

## Negative / failure cases

- Silent field rename/type change without version bump.
- Accepting cross-tenant IDs from the client body.
- Dropping correlation IDs on error paths.
- “Compatible” claim without fixtures or negative tests.
- Treating Kafka offset commit as business idempotency without an effect key.

## Security / tenant implications

Authn/z is server-side. Tokens/certs never appear in error payloads or logs. Tenant isolation is a contract property, not a UI filter. Public health endpoints must not expose inventory or secrets.

## Required evidence

- Schema/proto/OpenAPI diff summary
- Fixture paths and negative test names/results
- Compatibility notes for Sensor/Agent skew
- Exact SHA under test
- `UNVERIFIED` list for unimplemented consumers

## Stop conditions

Stop when:

- Breaking change lacks approved version/migration plan
- Tenant or auth boundary cannot be expressed in the contract
- Fixtures/negative tests cannot be added yet a compatibility claim is required
- Mother documents contradict the proposed surface—route to reconciliation first

## Anti-patterns

- Hand-waving “clients will adapt”
- Logging full request bodies with secrets
- Using HTTP 200 for all failures
- Writer self-certifying API readiness from happy-path only

## Handoff / output

Return: compatibility statement; fixture/test evidence; residual risks; nextOwner (API verifier / AppSec). Not MERGE_READY alone.

## Non-ownership

Does not own UI layout, Kafka broker ops, or invent product APIs. Owns contract-change discipline for declared surfaces.
