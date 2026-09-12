<!-- generated from project_agents/skills/ux_operational_design/SKILL.md sha256:1344bfee6d4a78ef8d625d4af607f0617a93c9af7584a116d819e680794a9f8f -->

---
name: ux_operational_design
description: Operational UX for Platform NOC/admin screens: job-to-be-done, hierarchy, dense-data presentation, microcopy intent and implementation-ready design handoff.
---

# ux_operational_design

## Purpose

Turn an operator job into a concrete Platform screen/interaction specification that a frontend engineer can implement without redesigning the product.

## Use when

Material UI/UX work: new operational screens, dashboard decision design, NOC wallboards, investigation workspaces, Notification Center surfaces, NCM/remediation UX, or large-estate tables. Required for `platform-ux-architect`, `platform-product-ui-designer` and `platform-ui-finish-gate-reviewer`.

## Authority

Platform sources override external design guidance:

1. `project_documentation/Platform_09_UI_UX_Master.md`
2. `project_documentation/Platform_02_PRD.md`
3. `project_documentation/Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md`
4. `project_documentation/Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md`
5. `project_documentation/Platform_08_Requirements_Traceability.md`

Do not import Tailwind, Material, Bootstrap, marketing landing-page patterns, icon libraries or new npm dependencies.

## Required procedure

1. State the operator job and the single primary decision/action.
2. List information required for that decision, including freshness/confidence/completeness/source when the contract exposes them.
3. Define information hierarchy then visual hierarchy then layout. First-read object and primary action must be obvious without hover.
4. Preserve context across dashboard → alert → incident → entity → topology → metric → change → RCA evidence → remediation.
5. Specify progressive disclosure for rare/advanced controls; keep monitoring density for trained operators.
6. Design every applicable operational state (see below). Never treat missing/stale telemetry as healthy zero.
7. Write microcopy intent in operator language: what happened, why it matters, next action, correlation/error ID. Do not blame the operator.
8. For destructive/high-impact NCM/remediation, expose target, scope, impact, preconditions, approval, planned action, execution state, validation, rollback and audit evidence. No direct LLM privileged action.
9. Complete the handoff contract and the acceptance matrix. Record `NOT_RUN` rather than omitting a row.

## Operational states

Every material screen must specify: loading, empty/no-data, normal, partial, degraded, stale, error, permission-restricted, action-in-progress, success, failed/recoverable. Distinguish first-use empty, filter/search no-results, and “no matching authorised entities”.

## UI Design Handoff Contract

Goal / user job; Primary decision/action; Information architecture; Layout/hierarchy; Key components; Relevant existing design tokens; Interaction model; Applicable UI states; Empty/error/recovery behaviour; Responsive behaviour; Accessibility intent; Large-estate considerations; Performance considerations; RBAC/tenant considerations; Open assumptions; Acceptance criteria.

## UI Acceptance Matrix

Each row is `PASS`, `FAIL`, `NOT_APPLICABLE` or `NOT_RUN`. Never hide `NOT_RUN`.

Rows: user/operator goal; primary decision/action; information hierarchy; navigation/context; visual hierarchy; information density; component consistency; operational states; empty/loading/error recovery; stale/partial/degraded representation; freshness/trust; keyboard; focus; screen-reader/native semantics; contrast/non-colour status; responsive behavior; laptop/NOC display; long labels/localization/data values; large-estate behavior; filtering/pagination/virtualization; render/query performance; tenant/RBAC visibility; destructive-action safety; microcopy/actionability; theme support where applicable; visual consistency; final polish.

Tiny copy/spacing fixes may skip this skill’s full pipeline; record that as risk-based routing with `NOT_APPLICABLE` plus reason.

## Required evidence

- Platform_09/PRD/22 identifiers used.
- Completed handoff contract and matrix.
- selectedAgents and selectedSkills for the task.

## Non-ownership

Does not implement React, certify WCAG, approve final finish, change product requirements, or select frameworks.

## Guardrails

- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.
