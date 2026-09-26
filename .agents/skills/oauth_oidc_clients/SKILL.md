---
name: oauth_oidc_clients
description: OAuth/OIDC client integrations, token handling, redirect safety and secret storage.
---

<!-- generated from project_agents/skills/oauth_oidc_clients/SKILL.md sha256:d1fa3b8bb75cdf3391c855a91add0354f3f5c18246d0d754d877dae6b1071b18 -->

# oauth_oidc_clients

## Purpose

Integrate OAuth2/OIDC clients safely: validate issuer/audience, protect redirects, handle token lifetime/refresh and store secrets correctly—without inventing provider-specific support or conflating SAML/SCIM.

## Use when

UI/API OIDC login, service-client credentials, token validation changes, JWKS rotation, or logout/session work. Required for identity client paths (`platform-identity-access-engineer`).

## Authoritative inputs

Always read:

1. Existing IdP/Keycloak integration code on the exact head—verify implemented flows
2. `pki_mtls` / `secrets_handling` for related material
3. Mother identity notes; SCIM/SAML remain separate Skills/docs where applicable
4. Redirect URI allowlists and cookie/session policies in-tree
5. Clock skew tolerances actually configured

Do not invent Okta/Entra-specific behaviours unless code exists; mark `UNVERIFIED`.

## Invariants

- OAuth2/OIDC boundaries clear (auth code vs client credentials, etc.).
- Issuer and audience validated on tokens.
- `state` / `nonce` / PKCE used where the flow requires them.
- Redirect URIs allowlisted exactly—no open redirects.
- Access token lifetime short; refresh tokens stored as secrets with rotation rules.
- JWKS/key rotation handled; reject wrong `kid`/alg.
- Clock skew bounded; expired tokens fail closed.
- Logout/session invalidation behaviour defined for the app.
- Confidential vs public clients distinguished; secrets never in public clients.
- Token leakage via logs/URLs/localStorage forbidden for confidential flows.
- Cross-tenant identity mapping is server-authoritative.
- Failure behaviour: unauthenticated/unauthorized—not “empty tenant.”
- SAML/SCIM not silently folded into OIDC claims without explicit design.

## Required procedure

1. Confirm which OAuth/OIDC flows exist on this SHA; list `UNVERIFIED` providers.
2. Validate discovery/issuer configuration; pin audiences.
3. Review redirect URI allowlist and PKCE/state/nonce.
4. Ensure token storage/rotation via secrets architecture.
5. Test expired/wrong-aud/wrong-iss tokens; JWKS key rotation case.
6. Verify logout clears server session as designed.
7. Keep SAML/SCIM changes out of scope unless assigned.

## Negative / failure cases

- Accepting tokens with unverified signature.
- Open redirect via `redirect_uri` query tampering.
- Putting refresh tokens in frontend localStorage for confidential apps.
- Mapping IdP `org_id` client claim directly to Platform tenant without server map.
- Disabling audience checks “temporarily.”

## Security / tenant implications

Identity is a security boundary. Client secrets are Credential Profiles. Cross-tenant login must fail closed. Audit admin IdP config changes. Prompt/LLM cannot mint tokens.

## Required evidence

- Flow matrix + `UNVERIFIED` providers
- Negative token tests
- Redirect allowlist references
- Exact SHA

## Stop conditions

Stop when:

- Signature/aud/iss validation missing
- Open redirect possible
- Secrets exposed to browser for confidential clients
- Provider support over-claimed

## Anti-patterns

- Rolling custom crypto instead of approved libraries
- Logging full JWT payloads with PII
- Writer self-certifying IdP integration

## Handoff / output

Return: flow security notes; test evidence; gaps; nextOwner (IAM/AppSec verifier). Not MERGE_READY alone.

## Non-ownership

Does not own SAML/SCIM productization, invent IdP vendors, or replace `tenant_isolation`. Owns OAuth/OIDC client discipline.
