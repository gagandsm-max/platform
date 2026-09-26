---
name: incident_flight_recorder
description: Bounded incident evidence capture, correlation, retention and export safety without unbounded DPI.
---

# incident_flight_recorder

## Purpose

Capture a **bounded** incident evidence window—correlated telemetry, config/change context and identifiers—with tenant separation, redaction and export safety. Not a full packet recorder by default.

## Use when

Incident evidence packages, flight-recorder buffers, correlation-ID stitching, incident export, or OIV evidence features governed by `Platform_19`. Required when touching IFR-like stores or incident evidence APIs.

## Authoritative inputs

Always read:

1. `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` (mandatory for OIV/IFR)
2. Existing IFR/evidence APIs and retention code on the exact head
3. Privacy/redaction and support-bundle rules
4. Alert/incident correlation-ID conventions
5. Triggered packet-evidence policy if packet artefacts are in scope (bounded, privileged)

If IFR product surfaces or packet slices are not implemented yet, write procedures that **verify** and mark `UNVERIFIED`—do not claim GA capture.

## Invariants

- Evidence window is **bounded** (time/bytes/objects)—not infinite always-on capture.
- Correlation IDs stitch alerts, polls, changes and operator actions.
- Timestamps distinguish event vs capture vs export time.
- Config/change/telemetry context is included when available; absence is explicit.
- Tenant separation is hard; no cross-tenant evidence in one package.
- Redaction of secrets/PII before export/support.
- Immutability: exported evidence is integrity-protected as implemented; if not built, `UNVERIFIED`.
- Retention follows policy; legal hold differs from active IFR window.
- Export safety: RBAC, audit, and no silent external egress.

## Required procedure

1. Verify which IFR capabilities exist on this SHA; list `UNVERIFIED` gaps.
2. Define window bounds (duration, size, entity scope) and enforcement points.
3. Require correlation IDs on capture triggers and package manifests.
4. Collect allowed context classes only; refuse unbounded PCAP/DPI expansion.
5. Apply tenant checks and redaction before persist/export.
6. Document retention and deletion/hold behaviour honestly (backups may remain).
7. Test export authz negative cases and oversized window rejection.
8. Tie value/MTTR metrics to evidence quality—no fabricated savings (`customer_value_mttr`).

## Negative / failure cases

- Turning IFR into always-on full DPI.
- Mixing two tenants’ incidents in one export.
- Shipping packages with credentials from config snapshots.
- Claiming immutability without hash/signature implementation.
- Unbounded retention “for forensics” without capacity plan.

## Security / tenant implications

Privileged evidence. Access audited. Packet artefacts (if any) remain tenant-scoped and resource-limited. External sharing requires explicit authorization. Prompt/LLM cannot authorize export.

## Required evidence

- Capability inventory vs `UNVERIFIED`
- Window bound enforcement tests
- Redaction/tenant negative tests
- Retention notes
- Exact SHA

## Stop conditions

Stop when:

- Bounds cannot be enforced
- Tenant separation fails
- Packet capture scope exceeds authorized triggered-evidence policy
- Mother `Platform_19` requirements conflict unresolved

## Anti-patterns

- “Keep everything forever”
- Using IFR dumps as casual debug logs
- Writer self-certifying OIV readiness without `Platform_19` tests

## Handoff / output

Return: window model; security evidence; gap list; nextOwner (OIV verifier / AppSec). Not MERGE_READY alone.

## Non-ownership

Does not own Similar Incident Memory causality, remediation execution, or SIEM replacement. Owns bounded incident evidence discipline.
