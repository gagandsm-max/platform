---
name: kubernetes_monitoring
description: Kubernetes/container target monitoring, workload identity and collector-health separation.
---

# kubernetes_monitoring

## Purpose
Monitor k8s as a target, not as a new Platform runtime dependency.

## Use when
k8s/container collectors, kube-state, or workload mapping.

## Required procedure
1. Do not select a new cluster runtime for Platform Core.
2. Separate collector health from workload health.
3. Map identities without using pod IP as sole identity.
4. Bound cardinality of container metrics.

## Guardrails
- Keep Platform v3.2.2 product requirements; do not invent TBD/P0 designs.
- Do not bypass tests, weaken tenant isolation, write to `main`, or force-push.
- Do not disable TLS validation, embed secrets, or use destructive Docker cleanup.
- LLM output is not privileged authority; writer cannot self-certify merge readiness.

## Required evidence
- Record commands, tests, logs or CI pointers that prove `kubernetes_monitoring` procedure ran.
- Label missing runtime proof as UNVERIFIED. Do not convert inference into SOURCE_FACT.

## Non-ownership
- Does not override Mother Reference product requirements or invent TBD/P0 designs.
- Does not certify merge readiness or execute privileged actions from LLM output.
