<!-- generated from project_agents/skills/accessibility_inclusive_design/SKILL.md sha256:d2e443e6043a0529aff83c63c884584a70e506ff08703112d53875eb32214661 -->

---
name: accessibility_inclusive_design
description: Accessible Platform operational UI: keyboard/focus, semantic HTML, screen-reader intent, non-colour status, contrast, reduced motion and inclusive resilient design.
---

# accessibility_inclusive_design

## Purpose

Specify and independently verify that Platform operational screens remain operable under keyboard, zoom, assistive technology, colour-vision deficiency, motion sensitivity and operator stress. Accessibility certification is independent of the designer and the frontend writer.

## Use when

Design intent for keyboard/focus/semantics, or independent accessibility audit of changed operational screens. Required for `platform-ux-architect`, `platform-product-ui-designer`, `platform-frontend-developer`, `platform-accessibility-auditor` and `platform-ui-finish-gate-reviewer`.

## Authority

`Platform_09` accessibility/readability requirements and existing Platform a11y tests. WCAG 2.2 AA is the default engineering target unless governance specifies otherwise. Automated scores are not certification.

## Required procedure

1. Semantic HTML first: buttons for actions, links for navigation, tables for tabular telemetry, headings in order, labelled form controls. ARIA only when native semantics cannot express the pattern.
2. Keyboard: all actions reachable; visible unobscured focus; focus order matches task order; dialogs trap and restore focus; no `outline: none` without an equivalent.
3. Screen-reader intent: accessible names for icon-only controls; live regions for alert/incident updates; announce stale/degraded/error without dumping internals.
4. Status must not be colour-only. Pair colour with text/icon. Distinguish `UNKNOWN`, `STALE`, `MONITORING_DEGRADED`, `INCONCLUSIVE` from DOWN.
5. Contrast and density: readable in Light/Dark and Compact/Comfortable/Large; 200%/400% zoom/reflow; long labels and localized values must not clip the primary action.
6. Honor reduced motion. Do not autoplay non-essential motion on NOC walls.
7. Inclusive resilient design: permission-restricted and not-licensed states explain the next authorized action; do not fake empty success.
8. Independent certification: `platform-product-ui-designer` records accessibility *intent*; only `platform-accessibility-auditor` may certify. If AT evidence cannot be collected, mark `NOT_RUN`.
9. Dense tables, topology/canvas, command palette, filters, dialogs and live incident updates are high-risk; test them explicitly.

## Destructive actions

Confirm irreversible NCM/remediation with target/scope/impact visible to keyboard and AT users. Do not rely on colour-only danger.

## Required evidence

- Keyboard path for the primary job.
- Focus/name/role/state notes for custom controls.
- Contrast/non-colour status evidence, or `NOT_RUN`.
- AT evidence or explicit `NOT_RUN`.

## Non-ownership

Does not implement the UI, does not change product requirements, and does not grant the designer or frontend developer certification authority.

## Guardrails

- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.
