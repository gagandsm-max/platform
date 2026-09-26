---
name: mother_reference_reconciliation
description: Discover the Mother Reference set dynamically from Platform_00 and reconcile requirements, decisions and implementation claims without stale document ceilings.
---

<!-- generated from project_agents/skills/mother_reference_reconciliation/SKILL.md sha256:72c7530f3ebb88e925ad16bb30357a5a2cc62dc1588f9f1fa68b1aa123dc36c2 -->

# mother_reference_reconciliation

## Purpose

Keep implementation and certification claims aligned with the authoritative Mother Reference set discovered from `Platform_00_Document_Index.md`, resolving conflicts through documented precedence—not silent reinterpretation.

## Use when

Any task citing BRD/PRD/Architecture/HLD/LLD/OSS catalogue/Decision Register; discovering contradictory requirements; claiming “docs say X”; changing Mother References; or when code reality diverges from desired architecture. Mandatory before material BUILD design claims and after `source_truth_rebaseline`.

## Authoritative inputs

Always read (from the **exact task head** after rebaseline):

1. `project_documentation/Platform_00_Document_Index.md` — exact Mother Reference set and precedence
2. Governing BRD/PRD / specialized masters named by the index for this task class
3. `Platform_03_Architecture.md` and relevant HLD/LLD/Dataflow sections
4. `Platform_07_OSS_Catalog.md` when dependencies are in scope
5. `Platform_12_Decision_Source_Register.md` when authority/ownership is unclear
6. Applicable Platform_19–23 specialized masters per `AGENTS.md` preflight rules
7. Current Decision Register / change-control entries for Founder-reserved items

Do not invent a frozen “N-file Mother set”; discover it from `Platform_00` at the exact SHA.

## Invariants

- Document precedence is defined by `Platform_00`; lower documents cannot silently override higher ones.
- Exact canonical document set is discovered dynamically—no historical file-count ceiling.
- Conflicts are classified: requirement↔requirement, requirement↔architecture, architecture↔code, docs↔implemented reality.
- **Founder-reserved** decisions (pricing, contractual SLA invention, etc.) stay Founder-owned; CTO/control-plane owns technical sequencing within Mother constraints.
- No silent rewriting of Mother References during feature work; Mother updates require deliberate change control.
- Traceability (requirement IDs, Decision Register) must be updated when an approved decision changes.
- Contradictory requirements block implementation until precedence resolves them.
- Stale document versions (wrong SHA / outdated branch copy) are not authoritative.
- Implemented reality vs desired architecture must be labelled honestly (`SOURCE_FACT` vs desired).
- Evidence before reconciliation: cite exact sections/SHAs; do not reconcile from memory.

## Required procedure

1. Confirm exact-head rebaseline; open `Platform_00` and list the current Mother Reference files on disk.
2. Select governing documents for the task class using the index + `AGENTS.md` specialized-master preflight rules.
3. Extract the specific requirement/architecture statements in conflict or under change; quote paths/sections.
4. Classify each conflict (docs↔docs, docs↔code, desired↔implemented) and apply `Platform_00` precedence.
5. Identify Founder-reserved vs CTO/control-plane ownership; escalate Founder-reserved gaps—do not invent policy.
6. If Mother docs must change: stop feature coding; open deliberate change-control (cross-doc reconciliation + Decision Register).
7. If code is wrong: route to diagnosis/implementation with requirement IDs preserved.
8. Record reconciliation outcome: resolved interpretation, open P0/TBD, or BLOCKED pending Decision Register.
9. Update traceability notes for the task; do not leave “we decided in chat” as the only record.

## Negative / failure cases

- Implementing from a remembered doc set that omits a specialized master required by `AGENTS.md`.
- Quietly editing Mother docs to match buggy code.
- Treating Desire/Roadmap prose as certified architecture.
- Ignoring Decision Register for Founder-reserved topics.
- Resolving contradictions by picking the convenient paragraph.
- Citing Mother docs from `main` while implementing on a drifted feature SHA (or vice versa) without disclosure.

## Security / tenant implications

Mother security/threat-model documents (`Platform_18`, etc.) constrain auth, tenancy and secrets. Reconciliation must not weaken tenant isolation, RBAC or data-egress rules to make an implementation “fit”. Security-sensitive reinterpretations require security-architecture review and Decision Register when they change boundaries.

## Required evidence

- Exact SHA and `Platform_00` discovery list used
- Governing document paths + section pointers
- Conflict classification table
- Precedence application notes
- Decision Register / Founder-escalation pointers when applicable
- Explicit `UNVERIFIED` where docs are missing or TBD/P0 remains
- Traceability IDs touched

## Stop conditions

Stop implementation/certification when:

- Precedence cannot resolve a contradiction
- Required specialized master is missing from the governing branch/index
- Founder-reserved decision is required but absent
- Mother change is needed but change-control is not authorized in this lane
- Exact-head docs differ from the docs the PR claims to follow

## Anti-patterns

- “Docs are outdated, code wins” without change-control
- Freezing a historical Mother file count into validators
- Rewriting requirements in the Skill or agent prompt instead of Mother docs
- Papering TBD/P0 as done
- Cross-linking Slack/chat as precedence over `Platform_00`

## Handoff / output

Return: discovery list; conflict matrix; resolved reading or BLOCKED; Decision Register actions; nextOwner (writer / security / Founder / control plane). Does not self-certify MERGE_READY.

## Non-ownership

Does not implement product code, invent Founder-reserved policy, certify merge, or replace `source_truth_rebaseline` / `exact_head_merge_gate`. Owns reconciliation discipline only.
