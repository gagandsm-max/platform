---
name: change_intelligence
description: Change and configuration evidence correlation with provenance—supporting incident context, not automatic root cause.
---

<!-- generated from project_agents/skills/change_intelligence/SKILL.md sha256:da325d279e185ecd2a565233c8ddb99542058472edf38353c627a2280ad05db6 -->

# change_intelligence

## Purpose

Correlate configuration/change signals (NCM, deployments, ITSM) into incident context with honest provenance and confidence—never treating change proximity as automatic root cause.

## Use when

Change feeds, NCM diffs, deployment hooks, ITSM change linkage, pre/post-incident timelines, or RCA context panels that cite changes. Required when wiring change evidence into alerts/incidents.

## Authoritative inputs

Always read:

1. Mother notes on change as supporting evidence (not causal proof)
2. Existing NCM/ITSM/deployment connectors and schemas on the exact head
3. `itsm_change_telemetry`, `connector_framework` and RCA Skills
4. Tenant isolation rules for change records
5. Timestamp/clock assumptions in source systems

If a change source is not implemented, mark `UNVERIFIED`—do not invent connectors.

## Invariants

- Configuration/change sources are named (NCM, deploy pipeline, ITSM, operator) with provenance.
- NCM and deployment/change events are distinct classes when both exist.
- ITSM changes are evidence feeds—not Platform’s authoritative config store.
- Timestamp alignment (source TZ vs Platform) is explicit; skew is documented.
- Actor/source attribution is preserved; anonymous “system” only when truly unknown.
- Correlation windows are bounded and stated; wide windows lower confidence.
- Confidence is separate from severity; weak correlation stays low-confidence.
- Duplicate changes are deduped by source identity keys—not dropped silently without record.
- Missing/stale sources degrade confidence (`STALE`/`UNKNOWN` evidence)—they do not invent changes.
- **Change ≠ root cause automatically**; temporal proximity is not proof.
- Pre/post incident relationship is labelled hypothesis-support.
- Tenant isolation on ingest and query is mandatory.

## Required procedure

1. Inventory change sources in scope; verify implementation on this SHA (`UNVERIFIED` gaps).
2. Define identity keys (change_id, ticket_id, commit SHA, device+version).
3. Normalize timestamps; record timezone handling tests.
4. Set correlation window and confidence rules for consumers (RCA/UI).
5. Deduplicate updates; preserve lineage of amend/cancel.
6. Ensure RCA/alert copy cannot auto-promote change to cause without corroboration.
7. Enforce tenant predicates; add cross-tenant negative tests.
8. Document stale/missing feed behaviour.

## Negative / failure cases

- “Change 3 minutes before alert ⇒ root cause.”
- Mixing tenants’ ServiceNow feeds.
- Dropping actor fields for “cleaner UI.”
- Using deployment webhook time as device apply time without caveat.

## Security / tenant implications

Change payloads may contain config fragments—redact secrets. RBAC on change evidence. Connector credentials scoped least-privilege. Exports audited.

## Required evidence

- Source inventory + `UNVERIFIED` list
- Correlation/confidence rules
- Tenant negative tests
- Dedupe/timestamp fixtures
- Exact SHA

## Stop conditions

Stop when:

- Design asserts automatic causality from change alone
- Tenant isolation missing
- Source provenance cannot be retained
- Unimplemented connector claimed as live

## Anti-patterns

- Scraping email as authoritative change
- Overwriting NCM truth with ITSM text
- Writer self-certifying change-intelligence completeness

## Handoff / output

Return: source model; confidence rules; RCA-safety statement; nextOwner (integration/RCA verifier). Not MERGE_READY alone.

## Non-ownership

Does not own remediation execution, NCM apply engine, or invent causal ML. Owns change-evidence discipline.
