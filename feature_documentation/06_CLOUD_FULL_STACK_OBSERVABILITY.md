# Cloud / Full-Stack Observability

**Class:** Derived feature brief — non-authoritative
**Primary sources:** Platform_02, 03, 19, 21, 22, 23

## Product objective

Correlate cloud, infrastructure, Kubernetes, endpoint, application-facing, log, metric, flow, change and service-dependency evidence into one operational model without requiring a monolithic "full-stack" agent everywhere.

## Cloud coverage

AWS, Azure and GCP integrations normalize accounts/subscriptions/projects, regions, networks, compute, storage, databases, load balancers and other supported service resources through provider APIs/SDKs and Capability Packs.

Future provider-specific coverage remains version/certification based.

## Kubernetes/container coverage

Canonical hierarchy includes Cluster -> Node -> Namespace -> Workload -> Pod -> Container -> Service -> Ingress plus storage/network/dependency relationships where observable.

## Signals

Full-stack views may combine:

- infrastructure/host/Agent metrics;
- SNMP/gNMI/network metrics;
- Flow;
- logs/events;
- OTLP metrics/logs;
- selected traces when trace support is actually implemented;
- synthetics;
- cloud-provider health/events;
- deployment/config/change evidence;
- service and topology graph relationships.

## APM boundary

Current architecture is trace-ready and accepts OTLP metrics/logs. Full bytecode APM is not an initial native product claim. Platform must not market "full APM" until trace storage, analysis, dependency extraction, retention, query and UX are implemented and accepted.

## Dependency graph

Provider/cluster/application/service identities normalize into the temporal canonical graph. Cloud IDs are strong identity inputs, but tenant authority remains server-side.

## Change intelligence

Correlate cloud changes, deployments, scaling, configuration and provider events with telemetry shifts. Temporal proximity alone is not causation.

## Operations

Cloud connector/API failure is `MONITORING_DEGRADED`, not proof a cloud resource failed. Rate limits, API version changes and permission problems are visible as collection health.

## Security

Use least-privilege provider roles, encrypted credentials/tokens, tenant isolation, egress controls and audit. Provider text or metadata is untrusted evidence, not AI/tool authority.
