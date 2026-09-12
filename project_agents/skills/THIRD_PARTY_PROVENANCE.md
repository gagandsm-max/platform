# Third-party provenance — Capability Pack v1.2 UI Skills

Review date: 2026-09-12.

External repositories were inspected at exact SHAs below, including LICENSE files and Skill/knowledge bodies, not README-only. Declared licences are recorded as published by those repositories; this file is not a legal opinion.

Platform Mother Reference remains higher authority. Concepts were independently rewritten into Platform operational guidance. **No substantial upstream text was copied into canonical Skills.** Therefore no additional runtime `THIRD_PARTY_NOTICES.txt` body is required for copied Skill prose. Attribution for reviewed MIT sources is recorded here.

Do not vendor these repositories. Do not import Tailwind, Material, Bootstrap, typography/palette defaults, marketing design assumptions, framework changes, icon-library choices or npm/runtime dependencies.

## `ux_operational_design`

| Field | Record |
|---|---|
| Platform Skill | `ux_operational_design` |
| Upstream repository | `hueyexe/frontend-agent-skills` |
| Upstream exact SHA | `2841c079dd8a9c634882227194dc42e25227710d` |
| Source files reviewed | `LICENSE`; `ux-usability-foundations/SKILL.md`; `information-architecture-navigation/SKILL.md`; `accessibility-inclusive-design/SKILL.md` |
| Declared licence | MIT (Copyright (c) 2026 hueyexe) |
| Material/concepts adopted | Job-before-widgets; recognition over recall; destructive-action caution; empty/loading/error as design, not polish |
| Material rewritten | All procedure text restated against Platform_09 NOC/admin jobs, operational states and trust fields |
| Material rejected | Consumer checkout/onboarding defaults; “clever” consumer copy; framework-specific question() tooling |

| Field | Record |
|---|---|
| Platform Skill | `ux_operational_design` |
| Upstream repository | `ca-who-codes/ultimate.UIUX.design.skills` |
| Upstream exact SHA | `c87ae4e906ed4508634f1ead0f79431f5c62ff4e` |
| Source files reviewed | `LICENSE`; `agents/ui-designer.md`; `knowledge/04-interaction/states-feedback.md`; `knowledge/06-patterns/dashboards.md` |
| Declared licence | MIT (Copyright (c) 2026 UI/UX Design Pro) |
| Material/concepts adopted | Spec-before-code; non-happy-path states as first-class; one primary action per view |
| Material rewritten | Output contract mapped to Platform handoff fields; states expanded to Platform telemetry-trust vocabulary |
| Material rejected | Linear/Stripe/Vercel “elite taste” branding; marketing landing/pricing/social/print/email tracks; token hex/OKLCH invention |

| Field | Record |
|---|---|
| Platform Skill | `ux_operational_design` |
| Upstream repository | `mares29/dashboard-design-skill` |
| Upstream exact SHA | `bbed535a08f0e1768b40087996fe019370312374` |
| Source files reviewed | `LICENSE`; `SKILL.md` |
| Declared licence | MIT (Copyright (c) 2026 Karel Mares) |
| Material/concepts adopted | Decision-first dashboard pages; data-quality visibility; dense-vs-executive appropriateness |
| Material rewritten | Decision framing applied to Platform operator jobs (triage, investigate, remediate) not business-investment pages |
| Material rejected | KPI-landing/executive-glance as universal pattern; TanStack Virtual / `nuqs` / specific chart-library prescriptions; “quality score %” invention; Title Case copy rules |

## `web_interface_quality`

| Field | Record |
|---|---|
| Platform Skill | `web_interface_quality` |
| Upstream repository | `vercel-labs/web-interface-guidelines` |
| Upstream exact SHA | `e3d624baaf29dc1fc645aff3e38f03e564d2d6b1` |
| Source files reviewed | `LICENSE`; `AGENTS.md`; `command.md` |
| Declared licence | MIT (Copyright (c) 2025 Vercel Labs) |
| Material/concepts adopted | Visible focus; URL-reflected state; skeleton vs layout shift; compositor-friendly motion; honor reduced motion; do not disable zoom |
| Material rewritten | Restated as Platform operational-web quality against existing design system and large-estate query rules |
| Material rejected | Tailwind utility names (`min-w-0`, `line-clamp-*`); iOS tap-highlight cosmetics; APCA-over-WCAG mandate; React Scan; `<img>` CDN preload opinions |

| Field | Record |
|---|---|
| Platform Skill | `web_interface_quality` |
| Upstream repository | `plugin87/full-stack-design-skills` |
| Upstream exact SHA | `61311065938a3f7e978b05476c52c65385b52ba7` |
| Source files reviewed | `LICENSE`; `.claude/skills/anti-ai-design-patterns/SKILL.md`; `.claude/skills/web-accessibility-a11y/SKILL.md` |
| Declared licence | MIT (Copyright (c) 2026 plugin87) |
| Material/concepts adopted | Reject generic interchangeable dashboards; component consistency; responsive as a quality gate; performance as evidence |
| Material rewritten | Anti-generic-dashboard rule bound to Platform_09 NOC/admin specificity |
| Material rejected | Material Design / Bootstrap examples; Tailwind CSS pattern files; Figma workflows; icon-library guides; frontend-framework/state-library selection; Core Web Vitals as a Platform product KPI |

## `accessibility_inclusive_design`

| Field | Record |
|---|---|
| Platform Skill | `accessibility_inclusive_design` |
| Upstream repository | `hueyexe/frontend-agent-skills` |
| Upstream exact SHA | `2841c079dd8a9c634882227194dc42e25227710d` |
| Source files reviewed | `LICENSE`; `accessibility-inclusive-design/SKILL.md` |
| Declared licence | MIT (Copyright (c) 2026 hueyexe) |
| Material/concepts adopted | Native semantics before ARIA; keyboard and names as design; no colour-only meaning |
| Material rewritten | Tied to Platform semantic monitoring states and independent auditor certification |
| Material rejected | Consumer form/checkout-centric defaults; legal-compliance claims |

| Field | Record |
|---|---|
| Platform Skill | `accessibility_inclusive_design` |
| Upstream repository | `plugin87/full-stack-design-skills` |
| Upstream exact SHA | `61311065938a3f7e978b05476c52c65385b52ba7` |
| Source files reviewed | `LICENSE`; `.claude/skills/web-accessibility-a11y/SKILL.md` |
| Declared licence | MIT (Copyright (c) 2026 plugin87) |
| Material/concepts adopted | POUR framing as study aid; focus visible; landmark/heading discipline |
| Material rewritten | WCAG 2.2 AA engineering target per Platform_09; AT evidence may be `NOT_RUN` |
| Material rejected | WCAG 2.1-only freeze; legal ADA/EQA conclusions; framework-specific snippets |

## Repositories with no adopted Skill text

All five repositories were reviewed. Marketing, print, social, Figma, Tailwind, Material and Bootstrap tracks were rejected in full. Platform `ui/` remains the implementation source of truth and was not modified.
