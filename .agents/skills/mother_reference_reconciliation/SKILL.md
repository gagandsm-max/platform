---
name: mother_reference_reconciliation
description: Discover the Mother Reference set dynamically from Platform_00 and reconcile requirements, decisions and implementation claims without stale document ceilings.
---

<!-- generated from project_agents/skills/mother_reference_reconciliation/SKILL.md sha256:1294a75458a8f7e3ca80cb139d0b689385abd5efe72f0e42833a77b52f764a68 -->

# mother_reference_reconciliation

## Purpose
Reconcile a change against the authoritative Mother Reference set without duplicating or weakening normative requirements.

## Required procedure
1. Read `project_documentation/Platform_00_Document_Index.md` on the exact task head.
2. Discover applicable Mother Reference files from that index dynamically.
3. Read `Platform_12_Decision_Source_Register.md` and the specialized masters governing the changed subsystem.
4. Classify each proposed statement as requirement, decision, implementation status, evidence, exclusion, unresolved spike, or Founder-reserved decision.
5. Prefer references/traceability over duplicate normative prose.
6. Reject implementation/readiness claims that are supported only by requirements text.

## Guardrails
- Never hard-code a highest Platform document number.
- Never use a specialized master to override a higher-precedence contract.
- Preserve v3.2.2 unless explicitly authorized otherwise.
