# UI-009 / UI-029 presentation evidence (local, pre-push)

Captured against Vite production preview of the bounded Phase 1 health-overview implementation.
Preview is HTTP for local visual capture only; production UI remains HTTPS-only (nginx, no port 80).

## Commands
- `docker run ... node:20-alpine ... npm ci && npm run test:unit && npm run build` → 35 pass
- Playwright screenshots against `http://127.0.0.1:4173`

## Observed
- UI-009 / UI-029 render `source-unavailable` with empty entity rows
- No fabricated device/host UP/DOWN or metric counters
- Light/dark and compact/comfortable/large density exercised
- Keyboard ArrowRight on workspace tabs exercised
- H1 computed font-size ~29–33px across densities (readable)

## Not claimed
- Production Core API integration
- Accessibility certification
- MERGE_READY / REALITY_GATE
