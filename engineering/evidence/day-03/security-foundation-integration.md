# Day-03 Security Foundation Integration Evidence

**Document type:** engineering evidence (non-Mother)  
**Product:** Platform  
**Mother Reference:** v3.2.1  
**Checkpoint:** Security foundation coexistence only — **not** complete Day-03, **not** GA.

---

## Baseline

| Item | Value |
|---|---|
| Current main | `f19c58b5b714dc83f010534afb8f480adec821d9` |
| Integration branch | `feature/day-03-security-foundation-integration` |
| Worktree | `~/nms-day03-security-integration` |

Includes merged Sensor PR #5 on main.

---

## Component PR heads verified (OPEN / DRAFT / UNMERGED)

| PR | Branch | Reviewed head |
|---|---|---|
| #9 Authorization Kernel | `feature/day-03-authorization-kernel` | `1b9dea69136748b32a25b06258e5106debb954d4` |
| #10 Provider-Neutral Identity | `feature/day-03-identity-contract` | `3e52782443cd67ed0f0fce0eeed99a2e41a44038` |
| #11 Security Audit Event | `feature/day-03-security-audit-contract` | `a11f8025721b4a709d1c9ee8d3b653a903b8d41b` |

---

## Commits integrated (cherry-pick order)

1. `a033ee1` — feat(core): add provider-neutral identity contract  
2. `3e52782` — fix(core): harden provider-neutral identity contract  
3. `96872cc` — feat(core): add tenant-scoped authorization kernel  
4. `1b9dea6` — fix(core): harden authorization kernel invariants  
5. `824a62e` — feat(core): add security audit event contract  
6. `a11f802` — fix(core): harden security audit event contract  

Conflicts: **NONE**  
Integration-only code fixes: **NONE**

Packages remain separate:

- `com.platform.core.security.identity`
- `com.platform.core.security.authorization`
- `com.platform.core.security.audit`

`core/pom.xml` vs main: **unchanged** (0-line diff).

---

## Boundary documentation (future wiring — not implemented)

### Identity → Authorization

Intended mapping:

`ResolvedAuthenticatedIdentity.platformSubject`
\+ persistent Platform role/permission assignments
\+ explicit tenant/resource grants  
→ `PrincipalContext`

External provider roles/groups/claims must **not** become direct grants.

Note: authorization still uses its reviewed string subject id independently of `PlatformSubject`. Future bridge must not weaken either contract.

### Authorization → Audit

Intended pattern:

`AuthorizationDecision` + request context → `SecurityAuditEvent`

Audit is **evidence only** and must never alter the authorization decision.  
Internal deny reasons must not be blindly exposed via future HTTP responses.

### Identity → Audit

Intended emitters for authentication / identity resolution / MFA must not leak raw `ExternalSubject`, tokens, assertions, or claim maps into audit payloads.

---

## Validation (Docker / repository-approved)

| Command | Result | Counts |
|---|---|---|
| `docker compose config -q` | PASS | — |
| Targeted Maven `AuthorizationEvaluatorTest,IdentityContractTest,SecurityAuditEventTest` | PASS | Authz 37 + Identity 13 + Audit 13 = **63** |
| `make test-unit` | PASS | Core **89**; Sensor Go packages OK; Agent Go packages OK; UI typecheck OK |
| `./tests/run-core-day02-tests.sh` | PASS | **15** mandatory PostgreSQL tests |
| `make test-contract` | PASS | 4/4 contract checks |
| `make test-security-smoke` | PASS | 4/4 smoke checks |
| `make test-integration` | PASS | **20/20** |
| `make test` | PASS | unit + integration |

---

## Consistency checks

- **USE / EDIT / REVEAL:** present and distinct in both `PlatformPermissions` and `AuditActions`.
- **Tenant isolation:** authorization explicit scopes; audit tenant is recording context only; identity remains tenant-free.
- **Secret safety:** no new `Map<String,Object>` / JWT / assertion / claim-authority path into PrincipalContext / SecurityAuditEvent / ResolvedAuthenticatedIdentity.
- **CorrelationId:** audit value object mirrors Core `X-Correlation-ID` grammar; filter remains in logging package.
- **ResourceType vs AuditResourceType:** kept separate (different purpose).

---

## Known unresolved Day-03 risks

1. Dual-resource authorization (`entity.group.*` / `entity.tag.*`) still models one primary `resourceId` (ENTITY) — later API enforcement must authorize secondary targets explicitly.  
2. Identity ↔ authorization subject bridge not implemented.  
3. Authorization / identity / MFA → audit emitters not implemented.  
4. No Keycloak/OIDC/SAML/LDAP, Spring Security filters, RBAC/audit persistence, credential storage, or session management.  
5. HTTP deny-reason collapse for clients not implemented.  
6. Durable audit writer with failure surfacing not implemented.

---

## Claims

This evidence supports **security-foundation integration coexistence** only.

It does **not** claim:

- complete Day-03;
- GA readiness;
- production authentication/authorization/audit wiring.
