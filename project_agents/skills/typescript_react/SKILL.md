---
name: typescript_react
description: TypeScript/React UI implementation discipline—contracts, states, safety and testable operational screens.
---

# typescript_react

## Purpose

Implement Platform UI in TypeScript/React with strict typing, honest async states, XSS-safe rendering and no browser-exposed secrets—aligned to server-driven authz and shared contracts.

## Use when

Building or changing React/TypeScript UI packages, hooks, data fetchers, tables or forms. Required for `platform-frontend-developer` after UX contracts are clear.

## Authoritative inputs

Always read:

1. Existing UI package layout, lint/typecheck scripts on the exact head
2. Shared/generated API contract types when present
3. `ux_operational_design`, `web_interface_quality`, `accessibility_inclusive_design`
4. Auth/session handling patterns already in the app
5. Mother UI HTTPS-only / no port-80 constraints for deployment claims

If React Compiler memo guidance or specific data libraries differ in-repo, follow the repo—do not invent a parallel stack.

## Invariants

- Strict typing enabled; **no unchecked `any`** for API contracts.
- Prefer generated/shared contract types where applicable; drift is a defect.
- Loading / error / empty states are mandatory for async views.
- Permission/unauthorized state is first-class—not a blank page.
- Abort/cancellation on unmount/navigation; no setState on unmounted paths.
- Stale request races resolved (ignore outdated responses).
- Pagination/large lists use paging or virtualization as appropriate.
- State ownership is clear (server authz truth > client cache).
- Accessibility basics respected (labels, keyboard)—coordinate with a11y Skill.
- XSS-safe rendering; no `dangerouslySetInnerHTML` with untrusted HTML unless sanitized by approved policy.
- **Secrets never browser-exposed** (no tokens in localStorage dumps, URLs, or bundle literals).
- Tests for critical flows; build/typecheck/lint must pass on the exact SHA.

## Required procedure

1. Locate contract types; update codegen if API changed.
2. Implement UI with explicit loading/error/empty/permission states.
3. Wire fetch with abort and race guards.
4. Type responses; eliminate `any` on boundaries.
5. Handle large collections with pagination/virtualization.
6. Ensure destructive actions use confirm patterns from UX Skill.
7. Run typecheck/lint/unit (and component tests if present).
8. Grep for secret patterns and unsafe HTML; fix before handoff.

## Negative / failure cases

- Showing another tenant’s cached row after switch.
- Swallowing 403 as empty data.
- Embedding API tokens in frontend env committed to git.
- Unbounded render of 100k rows freezing NOC browsers.
- `as any` to silence contract breakages.

## Security / tenant implications

UI is not the authority—server enforces tenant/RBAC. XSS is a tenant data breach class. CSP/headers follow platform defaults when present. Never log tokens in the browser console for “debug builds” that ship.

## Required evidence

- typecheck/lint/test commands + results
- Contract type references
- Race/abort notes for changed fetchers
- Exact SHA

## Stop conditions

Stop when:

- Unchecked `any` on API boundaries required to compile
- Secrets would ship in the bundle
- Permission failures hidden
- Typecheck/lint failing

## Anti-patterns

- Fetch in render without cancellation
- Global mutable caches without tenant key
- Writer self-certifying UI correctness from visual glance

## Handoff / output

Return: screens touched; test/typecheck evidence; residual UX risks; nextOwner (UI finish / a11y). Not MERGE_READY alone.

## Non-ownership

Does not own UX information architecture alone, invent backend fields, or declare WCAG certification. Owns TS/React implementation discipline.
