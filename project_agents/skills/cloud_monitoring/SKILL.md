---
name: cloud_monitoring
description: Cloud-account/target monitoring, OpenTelemetry import boundaries and vendor-ID normalization.
---

# cloud_monitoring

## Purpose

Monitor cloud accounts/subscriptions/projects with correct provider identity, quota safety and inventory honesty—without inventing unsupported connectors or equating cloud API outage with resource DOWN.

## Use when

AWS/Azure/GCP (or other) cloud account onboarding, cloud metrics/logs/events ingest, discovery of cloud resources, or OTel import boundaries for cloud. Required for infrastructure cloud paths when code exists.

## Authoritative inputs

Always read:

1. Mother Architecture / `Platform_07` for approved cloud dependencies
2. Existing cloud connector/collector code on the exact head—**verify what is actually implemented**
3. Credential Profile / least-privilege role guidance
4. `connector_framework` and `tenant_isolation` Skills
5. Cardinality/retention notes in `Platform_11`

Do not invent unsupported cloud connectors. Absent providers/features ⇒ `UNVERIFIED`.

## Invariants

- Provider identities (AWS account, Azure subscription, GCP project) are first-class and tenant-scoped.
- Account/subscription/project isolation is hard; no cross-account scrape via shared creds without explicit design.
- API credentials/roles are least-privilege Credential Profiles—never long-lived keys in git.
- API quotas, rate limits and pagination must be respected; backoff is mandatory.
- Region scope is explicit; global inventory claims need evidence.
- Resource discovery is eventual-consistency aware; stale inventory is labelled.
- Metrics/logs/events are distinct pipelines with their own failure modes.
- Tag/label cardinality is bounded.
- **Cloud control-plane/API outage ≠ resource DOWN**; collector health is separate.
- Vendor resource IDs never override Platform `entity_id` authority.
- Secret handling: rotate without logging secrets; support bundles redact.

## Required procedure

1. Confirm which providers/APIs exist on this SHA; list `UNVERIFIED`.
2. Bind cloud account objects to tenant; enforce authz on register.
3. Configure roles/creds via secrets architecture; prove redaction.
4. Implement/verify pagination, retries and quota handling with tests.
5. Define discovery consistency (list vs watch) and stale TTL behaviour.
6. Separate API/auth failures from instance/VM/function operational state in alerts.
7. Bound tags/dimensions; reject unbounded custom-metric explosions where controlled.
8. Normalize vendor IDs into Platform entities without silent duplicates.

## Negative / failure cases

- Declaring all VMs DOWN because CloudWatch/Azure Monitor API 429s.
- Using root/owner keys for collectors.
- Unbounded tag ingest blowing CH cardinality.
- Claiming GCP support when only AWS stubs exist.
- Cross-tenant access via shared org credentials.

## Security / tenant implications

Cloud creds are high-value secrets. STS/assume-role preferred where implemented. Tenant isolation on every cloud API proxy. No SSRF via user-supplied endpoints without allowlist.

## Required evidence

- Implemented provider matrix + `UNVERIFIED`
- Quota/pagination/retry tests
- DOWN-separation statement
- Secret handling proof
- Exact SHA

## Stop conditions

Stop when:

- Unsupported connector would be claimed implemented
- API outage maps to resource DOWN
- Secrets cannot be isolated
- Tenant/account isolation fails

## Anti-patterns

- Copying vendor console status as Platform severity
- Disabling TLS to a “custom” cloud endpoint
- Writer self-certifying multi-cloud coverage from one provider lab

## Handoff / output

Return: provider matrix; isolation/quota evidence; gap list; nextOwner (cloud/infra verifier). Not MERGE_READY alone.

## Non-ownership

Does not own Kubernetes-in-cloud specifics (`kubernetes_monitoring`), invent CSPM product, or select unapproved SDKs. Owns cloud-target monitoring discipline for implemented connectors.
