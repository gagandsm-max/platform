---
name: web_interface_quality
description: Platform web implementation quality: existing design-system use, component consistency, responsive behaviour, state completeness, runtime/visual validation and frontend performance.
---

# web_interface_quality

## Purpose

Specify and verify that a Platform screen is implementable on the current React/TypeScript design system without inventing a parallel visual language or pulling a 200K-device table into the browser.

## Use when

Frontend implementation, design-system application, component composition, responsive behaviour, visual/runtime validation, or frontend performance/density work. Required for `platform-product-ui-designer`, `platform-frontend-developer`, `platform-accessibility-auditor` and `platform-ui-finish-gate-reviewer`.

## Authority

Follow `Platform_09` screen contracts, existing `ui/` components, and `typescript_react`. Do not select new CSS frameworks, chart libraries, icon packs or npm packages. OSS remains `Platform_07`.

## Required procedure

1. Reuse existing Platform components and tokens. Flag a missing primitive; do not add a second button/table/status language.
2. Specify component composition: page chrome, first-read object, primary action, filters, dense table/visualization, evidence pane, and contextual actions.
3. URL/state must preserve tenant, time range, filters, selected entity and drill-down context. Links are shareable; back must not drop investigation context.
4. Every data region has independent state. One failed widget must not blank the dashboard. Keep stale data visible while refreshing; label it stale.
5. Large estates: server-side filter/query/pagination/aggregation; virtualize long lists; cancel abandoned queries; bounded ClickHouse rollups per `Platform_09` performance notes.
6. Responsive: desktop-first admin/studios; mobile/tablet for dashboards, alerts, incidents, acknowledgement and investigation summaries. NOC TV/full-screen is a first-class layout, not an afterthought.
7. Theme/density: Light/Dark/System and Compact/Comfortable/Large as Platform already requires. Do not invent a new palette.
8. Runtime/visual validation: exercise loading/empty/partial/stale/error/permission states in browser evidence. A single happy-path screenshot is not verification.
9. Performance: measure query/render cost for the operator job. Do not claim “fast” without evidence. Prefer tabular numerals for comparable metrics.
10. Frontend developers must implement the approved specification faithfully and must not redesign material UX while coding. Return to `platform-product-ui-designer` if the spec is insufficient.

## State completeness

Required where the contract can produce them: loading, empty/no-data, normal, partial, degraded, stale, error, permission-restricted, not-licensed, query-limited, monitoring-degraded, action-in-progress, success, failed/recoverable.

Never present missing telemetry as a green zero. Show freshness and source.

## Required evidence

- Component names from current `ui/` (reality comparison only in this pack).
- State matrix and large-estate query/pagination notes.
- Browser/test evidence after implementation; `NOT_RUN` if not executed.

## Non-ownership

Does not own product requirements, accessibility certification, final finish approval, or dependency selection.

## Guardrails

- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.
