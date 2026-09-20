# Active Directory Integration

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_02, 10, 20, 21, 22
**Pending enrichment:** Flow user/endpoint correlation described in `02_FLOW_MONITORING.md`.

## Product objective

Use Microsoft Active Directory as an enterprise identity/directory integration for authentication/federation, user/group context, dependency monitoring and governed endpoint/user enrichment.

## Identity/federation

AD/LDAP may be integrated through the Platform identity layer for authentication and group/role mapping according to configured tenant policy. SSO-specific behavior is documented separately.

## Directory data

Where authorized, Platform may consume:

- user identity;
- group membership;
- computer accounts;
- domain/OU context;
- directory attributes required for Platform workflows.

Directory attributes are tenant scoped and minimized.

## Monitoring AD as a service

AD-related infrastructure may also be monitored operationally through supported Packs:

- domain controllers;
- LDAP/LDAPS reachability;
- DNS dependency;
- replication/health evidence where supported;
- authentication/service availability;
- certificate/PKI dependencies.

AD monitoring failure is distinct from user-authentication failure and from monitored endpoint failure.

## Flow/user enrichment

AD alone does **not** prove which user owned an IP at a historical time. Flow user attribution requires temporal evidence such as Agent login sessions, VPN, RADIUS/802.1X, NAC, WLAN controller, firewall user mapping, authentication events or equivalent.

A user association records:

- canonical user/directory ID;
- source;
- valid/observed time;
- endpoint/entity;
- confidence;
- conflicting evidence if present.

Do not perform a simple current-IP-to-current-AD-user join for historical Flow.

## Provisioning / SCIM relationship

SCIM v2-compatible lifecycle is a future enterprise target and is not the same thing as raw AD/LDAP integration. Do not claim SCIM certification until validated.

## Security/privacy

Use secure LDAP/TLS where applicable, least privilege, tenant-scoped queries, credential protection and audit. User-attributed Flow/DEX data is operational telemetry and requires privacy-aware RBAC; it must not become employee surveillance.

## Failure behavior

Directory/connector failure surfaces as identity/integration degradation and cannot silently remove access or rewrite historical user/entity attribution without governed reconciliation.
