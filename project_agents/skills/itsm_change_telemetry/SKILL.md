---
name: itsm_change_telemetry
description: ITSM/notification/change-telemetry connectors as supporting evidence, not causal proof.
---

# itsm_change_telemetry

## Purpose

Ingest ITSM ticket/change telemetry as supporting evidence with durable identity, pagination and tenant routing—never as automatic RCA or maintenance truth without Platform policy.

## Use when

ServiceNow/Jira/etc. change or incident ticket connectors, webhook/polling ITSM feeds, maintenance-window imports from ITSM, or correlation of tickets to Platform incidents. Required for integration connector paths that touch ITSM.

## Authoritative inputs

Always read:

1. Mother rule: external provider/change evidence is supporting—not causal proof alone
2. Existing ITSM connector code on the exact head (verify implemented vendors)
3. `connector_framework`, `change_intelligence` and secrets Skills
4. Tenant routing / site binding rules
5. Maintenance-window semantics in alert masters when present

Unsupported ITSM products ⇒ `UNVERIFIED`. Do not invent certified connectors.

## Invariants

- Ticket/change identifiers from the source system are preserved with source-system identity.
- Polling and webhook modes both require auth, idempotency and replay safety as implemented.
- Pagination and retries are bounded; duplicates are deduped by stable IDs.
- Updates amend lineage—they do not invent a second unrelated change without linkage.
- Timestamps/timezones normalized; approval status mapped explicitly.
- Maintenance windows from ITSM are inputs to Platform policy—not silent suppressors without authz rules.
- Change correlation to alerts is confidence-scored evidence.
- Credential scope is least-privilege per tenant connector instance.
- Tenant routing is server-side; webhook payloads cannot pick another tenant.
- ITSM outage / stale data ⇒ connector degraded—not mass target DOWN.
- **Change/ticket record is evidence, not automatic RCA.**

## Required procedure

1. Confirm connector/vendor support on this SHA; list `UNVERIFIED`.
2. Define identity keys (sys_id, issue key, change number) + source system.
3. Implement/verify poll/webhook auth, pagination, retry and dedupe.
4. Normalize timestamps and approval/maintenance fields with fixtures.
5. Enforce tenant binding on connector config and ingress.
6. On ITSM outage: emit connector health; do not flip monitored CIs DOWN.
7. Ensure RCA/UI consumers label ITSM linkage as supporting evidence.
8. Rotate credentials via secrets architecture; prove log redaction.

## Negative / failure cases

- Auto-closing Platform incidents because a ticket auto-closed.
- Webhook without signature accepted from the Internet.
- Duplicate events creating alert storms.
- Using ITSM “affected CI” string as cross-tenant entity match.

## Security / tenant implications

ITSM tokens are secrets. Ingress webhooks authenticated. Payload may contain PII—minimize retention. Cross-tenant misrouting is a Sev-1 class defect.

## Required evidence

- Vendor matrix + `UNVERIFIED`
- Dedupe/pagination/retry tests
- Tenant negative tests
- RCA-safety statement
- Exact SHA

## Stop conditions

Stop when:

- Ticket would auto-become root cause
- Tenant routing spoofable
- Unsupported vendor claimed certified
- Outage maps to estate DOWN

## Anti-patterns

- Hard-coding one customer’s ServiceNow table names as Core globals
- Disabling webhook auth “temporarily”
- Writer self-certifying ITSM coverage

## Handoff / output

Return: connector semantics; evidence caveats; tests; nextOwner (integration verifier). Not MERGE_READY alone.

## Non-ownership

Does not own Platform incident workflow engine, invent ITSM replacement, or certify SCIM. Owns ITSM change-telemetry discipline.
