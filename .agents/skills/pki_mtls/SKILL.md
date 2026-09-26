---
name: pki_mtls
description: PKI, mTLS enrollment, certificate identity and untrusted-cert rejection.
---

<!-- generated from project_agents/skills/pki_mtls/SKILL.md sha256:2562cf948f78398efa0d37a8ee20d40cb015559cfadbb8f462dc76aadcbd2076 -->

# pki_mtls

## Purpose

Govern PKI and mTLS identity for Core/Sensor/Agent: trust roots, certificate-to-principal binding, private-key protection, rotation/expiry/revocation and fail-closed rejection of wrong/missing/revoked certificates.

## Use when

Enrollment, cert rotation, TLS client/server config, trust-store changes, or identity bugs involving certificates. Required operational-critical Skill for Sensor/Agent/Core transport security.

## Authoritative inputs

Always read:

1. Existing PKI/enrollment/mTLS code and cert paths on the exact head
2. `Platform_18` / protocol security notes; `agent_architecture` for Agent identity
3. `secrets_handling` for key material
4. Dev cert generation (`make certs`) vs production issuance boundaries
5. Documented overlap/rotation windows if any (`UNVERIFIED` if absent)

## Invariants

- CA/trust roots are explicit; unknown CAs fail closed.
- Certificate subject/SAN identity binds to Platform principal (`sensor_id`/`agent_id`/etc.)—not IP.
- Private keys protected (filesystem ACLs/HSMs as implemented); never logged.
- Bootstrap/enrollment issues short-lived credentials via approved flows.
- Rotation, expiry and revocation (as implemented) are enforced; expired/revoked rejected.
- Replacement does not silently create a second inventory identity (`Platform_20` reconciliation).
- Certificate-to-principal binding is server-authoritative.
- Tenant/site authority not derived from client-supplied headers when mTLS is present.
- Wrong/missing/revoked cert ⇒ transport failure, not degraded anonymous access.
- Clock validity checked; skew tolerances documented.
- TLS verification stays enabled; no `InsecureSkipVerify` in production paths.
- **No proxy-header identity override** bypassing mTLS.
- Secret logging prevention mandatory.
- Race during cert rotation handled; old/new cert overlap only per documented rules.
- Evidence and negative tests required for identity accept/reject matrix.

## Required procedure

1. Inventory trust anchors and identity fields used on this SHA.
2. Verify private key permissions and redaction in logs.
3. Test accept path for valid cert; reject paths for expired, wrong SAN, untrusted CA, revoked (if CRL/OCSP exists—else `UNVERIFIED`).
4. Document rotation procedure and overlap; add race tests if changing rotator.
5. Ensure enrollment cannot attach cross-tenant.
6. Confirm proxy headers cannot override peer certificate identity.
7. Never commit production private keys; keep `.env`/certs untracked.

## Negative / failure cases

- Accepting self-signed random certs because verify was disabled.
- Binding identity to CN IP after DHCP change.
- Logging PEM private keys on enrollment error.
- Allowing old revoked cert during “grace” without policy.
- Trusting `X-Forwarded-Client-Cert` without mTLS terminate policy.

## Security / tenant implications

Compromise of Sensor/Agent key is tenant-scoped breach potential—rotate and inventory. Trust-store updates are high-risk changes. Support bundles exclude private keys. Operational-critical: incomplete procedure escalates P0/P1.

## Required evidence

- Accept/reject matrix tests
- Key permission / redaction proof
- Rotation notes or `UNVERIFIED`
- Exact SHA

## Stop conditions

Stop when:

- TLS verify would be disabled
- Identity unbound from certificate
- Private key exposure path exists
- Cross-tenant enroll possible via cert

## Anti-patterns

- Sharing one client cert across tenants
- Checking certs into git
- Writer self-certifying PKI safety

## Handoff / output

Return: trust model; negative-test evidence; rotation gaps; nextOwner (secrets/AppSec verifier). Not MERGE_READY alone.

## Non-ownership

Does not own OAuth token flows alone (`oauth_oidc_clients`) or invent public CA commerce. Owns PKI/mTLS identity discipline.
