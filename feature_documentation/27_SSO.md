# Single Sign-On (SSO)

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_02, 03, 07, 09, 10

## Product objective

Provide enterprise authentication federation while keeping Platform tenant/resource authorization under Platform control.

## Supported federation model

The architecture supports enterprise identity integration through the selected identity layer, including OIDC, SAML and LDAP/AD federation where configured.

Keycloak is the selected identity/federation foundation; Platform owns tenant mapping, RBAC and resource authorization.

## Login flow

```text
User
 -> Platform tenant/customer entry context
 -> configured identity provider/federation
 -> authentication/MFA
 -> identity claims
 -> Platform tenant/user mapping
 -> effective RBAC/resource scope
 -> UI/API session
```

## Tenant safety

Authentication success does not determine tenant/resource access by itself. Claims/groups are mapped through governed Platform configuration. A claim from one IdP/tenant cannot grant another tenant's access.

## MFA

MFA may be enforced by the configured IdP/identity layer and Platform policy. Privileged actions may additionally require step-up/JIT controls where governed.

## Home realm / provider selection

Customer-specific URLs or tenant context may route users to the correct enterprise IdP. Provider discovery must not leak sensitive tenant configuration unnecessarily.

## Session security

Use secure cookies/tokens, HTTPS only, bounded session lifetime, logout/session revocation and auditable authentication events.

## Break-glass

Production design should preserve governed local/break-glass administration appropriate for IdP outages, with strong protection/audit and no broad bypass of normal authorization.

## Failure behavior

IdP/LDAP outage is authentication-service degradation. Existing valid sessions follow configured lifetime/revalidation rules; the failure does not alter monitored infrastructure state.

## Separation from RBAC

SSO answers "who authenticated?" RBAC answers "what may this authenticated identity do?" See `28_RBAC.md`.
