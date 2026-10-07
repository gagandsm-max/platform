---
name: virtualization_storage
description: Virtualization, datastore and storage-target monitoring without turning Platform into DCIM/BMS.
---

<!-- generated from project_agents/skills/virtualization_storage/SKILL.md sha256:73ecbc262a2c20a633ea0d39dfc9a4de19a1e1af87c3d8e6765c0483cc7a9145 -->

# virtualization_storage

## Purpose

Monitor virtualization and storage targets (where actually supported) with stable VM/datastore identity, capacity/latency evidence and migration-safe topology—without claiming unsupported platforms or becoming DCIM/BMS.

## Use when

VMware/Hyper-V/storage collector work, datastore capacity/latency, VM inventory, or dependency impact from storage paths—only for platforms implemented on the exact head.

## Authoritative inputs

Always read:

1. Existing virtualization/storage connector code on the exact head—**verify support**
2. Mother exclusions: Platform is not broad BMS/DCIM
3. `Platform_07` approved dependencies
4. Credential and rate-limit patterns (`connector_framework`)
5. Topology identity rules (`topology_routing`, `Platform_20` where relevant)
6. `references/vmware-first-ga.md` and fixtures/evals for vCenter/ESXi/vMotion and stale inventory

Unsupported hypervisors/arrays ⇒ `UNVERIFIED`. Do not claim them as implemented.

## Invariants

- Supported platforms only (e.g. VMware/Hyper-V/storage arrays **where code exists**).
- vCenter/host/VM/datastore (or equivalents) identities are stable; VM mobility must not duplicate entities.
- Cluster relationships and datastore attachments carry provenance/timestamps.
- Stale topology after vMotion/storage vMotion is detected and reconciled—not silently duplicated.
- Capacity and datastore latency are measured with stated units/intervals.
- Path/multipath and storage controller health are separate evidence classes when implemented.
- Credentials least-privilege; API rate limits honored.
- Discovery consistency: list pagination/eventual consistency handled.
- Dependency impact may explain incidents but does not expand Platform into facilities monitoring.
- **No duplicate VM identity after migration.**

## Required procedure

1. Build the supported-platform matrix from code; mark others `UNVERIFIED`.
2. Define identity keys (BIOS UUID, instance UUID, datastore moRef, etc.) and migration reconcile rules.
3. Wire credentials via secrets; test auth failure vs target failure separation.
4. Implement/verify rate limits and pagination.
5. Collect capacity/latency with cardinality bounds.
6. On topology churn (vMotion), prove no duplicate entity; add tests if changing identity code.
7. Keep facilities/power out of scope except as optional external evidence per Mother.
8. Refuse DCIM feature creep in the same change.

## Negative / failure cases

- New entity_id on every vMotion.
- Declaring all VMs DOWN because vCenter API is down.
- Claiming NetApp/Pure/Hyper-V support from VMware-only stubs.
- Using overly broad vCenter admin roles permanently.

## Security / tenant implications

Hypervisor creds are crown jewels. Tenant isolation if multi-tenant vCenter folding is ever allowed (default: careful binding). Redact connection strings. No guest credential harvesting.

## Required evidence

- Platform support matrix + `UNVERIFIED`
- Identity/migration tests
- Rate-limit/auth failure classification
- Exact SHA

## Stop conditions

Stop when:

- Unsupported platform claimed live
- Migration duplicates identities
- API outage maps to all-VM DOWN improperly
- DCIM/BMS scope creep requested

## Anti-patterns

- Scraping HTML clients instead of supported APIs
- “Storage latency high ⇒ datacenter power issue” as Platform assertion
- Writer self-certifying virtualization coverage

## Handoff / output

Return: support matrix; identity rules; gaps; nextOwner (infra verifier). Not MERGE_READY alone.

## Non-ownership

Does not own pure network topology, invent SAN fabric controller product, or BMS. Owns virtualization/storage monitoring discipline for implemented targets.
