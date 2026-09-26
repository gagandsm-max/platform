---
name: kubernetes_monitoring
description: Kubernetes/container target monitoring, workload identity and collector-health separation.
---

# kubernetes_monitoring

## Purpose

Monitor Kubernetes clusters with correct cluster/namespace identity, API safety and cardinality control—separating API/collector failure from workload DOWN, and never deleting pods unless explicitly authorized remediation exists.

## Use when

Cluster onboarding, kube-state/metrics/event collectors, namespace tenancy mapping, watch/list clients, or Kubernetes dependency evidence in RCA. Required for infra K8s paths when implemented.

## Authoritative inputs

Always read:

1. Existing Kubernetes collector/connector code on the exact head—verify real support
2. Mother notes on collector-health separation and Capability Pack boundaries
3. `Platform_07` for approved K8s client dependencies
4. RBAC/service-account guidance; `secrets_handling`
5. Cardinality guidance (`Platform_11`)

If only partial controllers exist, mark other resources `UNVERIFIED`. Do not claim full APM/bytecode.

## Invariants

- Cluster identity is first-class; namespaces map to tenant/scopes per Platform policy—not assumed 1:1 without config.
- API access uses least-privilege RBAC; service-account secrets protected.
- Watch/list semantics honor resourceVersion reconnect; stale cache is labelled.
- Kube-state vs metrics vs events are distinct evidence classes.
- Pods/nodes/workloads/services modelled without treating ephemeral objects as permanent inventory without rules.
- Cardinality of labels/pods is bounded.
- API throttling/backoff required; watch storms avoided.
- **Cluster API outage ≠ workload/resource DOWN** automatically.
- No pod deletion/remediation unless explicitly authorized allowlisted remediation path exists (usually out of scope here).

## Required procedure

1. Inventory implemented collectors (state/metrics/events) on this SHA; `UNVERIFIED` gaps.
2. Bind cluster credentials via secrets; verify RBAC scope.
3. Implement/verify list/watch reconnect and resourceVersion handling tests.
4. Define namespace→tenant mapping rules; negative-test cross-namespace leak.
5. Bound label cardinality; drop or hash high-cardinality keys per policy.
6. Separate API/auth/throttle errors from Deployment/Pod operational states in alerts.
7. Refuse destructive K8s actions unless remediation_safety allowlist applies—and usually stop.
8. Document ephemeral object lifecycle (Completed pods, Jobs) in inventory rules.

## Negative / failure cases

- Marking all namespaces DOWN on apiserver timeout.
- Infinite watch reopen without backoff.
- Ingesting `pod_template_hash` unbounded as metric dimensions.
- Collector using cluster-admin “for convenience.”
- Auto-deleting CrashLoop pods as “remediation.”

## Security / tenant implications

Kubeconfig/tokens are secrets. Multi-tenant clusters require strict namespace isolation in Platform. Audit access to exec/logs if ever exposed (default: do not). Supply-chain: only approved client libs.

## Required evidence

- Resource coverage matrix + `UNVERIFIED`
- Watch/reconnect and throttle tests
- Tenant/namespace isolation proof
- DOWN-separation statement
- Exact SHA

## Stop conditions

Stop when:

- API outage maps to workload DOWN without corroboration policy
- Secrets/RBAC overly broad
- Destructive actions proposed without remediation controls
- Unsupported resources claimed monitored

## Anti-patterns

- Shipping kubeconfig in git
- Treating metrics-server absence as node death without corroboration
- Writer self-certifying “full Kubernetes support”

## Handoff / output

Return: coverage matrix; API-safety evidence; risks; nextOwner (infra/K8s verifier). Not MERGE_READY alone.

## Non-ownership

Does not own container image scanning productization, full APM, or invent cluster autoscaler control. Owns K8s monitoring evidence discipline.
