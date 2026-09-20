# Platform — Cloud & Full-Stack Observability Master

**Document ID:** PLATFORM-CLOBS-001  
**Version:** 3.2.2  
**Status:** Authoritative Mother Reference — Specialized Product / Architecture / HLD / LLD / Data / UI / Security / Quality Supplement after merge  
**Product:** Platform  
**Scope:** Cloud, Kubernetes, OpenTelemetry, service/application observability, cloud-network observability, SLOs, cloud onboarding, cloud service packs, telemetry economics and P1 digital/application observability

---

## 1. Purpose and authority

This specialized master defines the approved additive Cloud & Full-Stack Observability capability family for Platform.

It exists because the earlier base requirement — AWS/Azure/GCP resource monitoring plus Kubernetes and OpenTelemetry metrics/logs with trace-ready architecture — is insufficient for the current product objective of a world-class enterprise monitoring and observability platform.

This document supplements, and does not replace, the base Mother References.

Authority boundaries:

- Platform_19 remains authoritative for SaaS Monitoring Packs, cloud/external change intelligence, Incident Flight Recorder, Similar Incident Memory and controlled remediation.
- Platform_20 remains authoritative for governed onboarding/identity/reconciliation. This document adds cloud-native parent-scope and ephemeral-resource rules.
- Platform_21 remains authoritative for SLI/SLO/error-budget semantics and browser synthetics. This document makes the applicable SLO capability part of P0 cloud observability and defines its cloud/service integration.
- Platform_22 remains authoritative for telemetry trust, evidence-qualified alerting, RCA/AI assurance and Platform AI observability. This document adds customer AI/LLM workload observability as P1.
- Platform_23 remains authoritative for the single Capability Pack architecture, certification model and enterprise monitoring coverage. Cloud Service Packs defined here are Capability Pack classifications/profiles, not a second pack framework.
- Platform_17 remains authoritative for cross-product API/event/data contracts. This document defines additive cloud/full-stack contract requirements that Platform_17 must reference.
- No new mandatory database or standalone microservice is selected by this document. Existing PostgreSQL, ClickHouse, Kafka, Valkey and bounded encrypted artifact-storage roles remain authoritative.

This document does not claim that the capabilities below are already implemented or certified merely because they are specified.

---

## 2. Product objective

Platform shall reason from user/business-service symptoms down through application/service/runtime/cloud/network evidence rather than stop at infrastructure resource metrics.

The target operational model is:

User / Synthetic / SLO
→ Business Service
→ Application / Service / Operation
→ Trace / Metric / Log / Event
→ Runtime dependency
→ Kubernetes / VM / Function / Process
→ Cloud managed service / resource
→ Cloud network / on-prem path
→ Change / deployment / provider event
→ Incident Flight Recorder
→ Evidence-qualified RCA
→ governed recommendation / remediation

Every material node, edge and observation shall preserve tenant, provenance, event/observation time, ingest time, freshness/quality and evidence reference where applicable.

---

## 3. Priority model

For this capability family:

- **P0** means required for the first Cloud & Full-Stack Observability GA claim. A P0 capability may still be delivered incrementally during development, but Cloud Observability GA may not claim completeness without its acceptance evidence.
- **P1** means required for the next observability expansion after the P0 foundation and must be architecturally enabled by P0.
- **Deferred / explicit boundary** means not required for P0/P1 and must not be silently pulled into scope.

This product-priority usage is distinct from existing Mother Reference **TBD/P0 engineering spikes**, which remain unresolved implementation decisions until evidence exists.

---

## 4. Formal product requirements

| PRD ID | Priority | Requirement |
|---|---|---|
| PRD-CLOBS-001 | P0 | Platform shall accept OpenTelemetry traces as a core telemetry signal alongside metrics and logs, with W3C Trace Context-compatible correlation, span/resource identity, bounded sampling and tenant isolation. |
| PRD-CLOBS-002 | P0 | Platform shall maintain one unified resource/service identity across cloud inventory, Kubernetes, hosts, processes, metrics, logs, traces, synthetics, changes, network evidence, SLOs and incidents. |
| PRD-CLOBS-003 | P0 | Platform shall support cross-signal Metric ↔ Log ↔ Trace ↔ Event ↔ Change ↔ Topology ↔ Synthetic ↔ Network investigation without requiring a separate product. |
| PRD-CLOBS-004 | P0 | Platform shall ingest Prometheus/OpenMetrics ecosystem telemetry through approved collection/ingestion paths while retaining Platform-owned storage, policy, alerting and RCA semantics. Prometheus is not selected as the primary Platform TSDB. |
| PRD-CLOBS-005 | P0 | Kubernetes monitoring shall extend to scheduling, controller health, rollout state, resource requests/limits, OOM/termination reason, persistent storage, autoscaling, service/network relationships, ownership and events. |
| PRD-CLOBS-006 | P0 | Serverless and managed runtime monitoring shall be first-class for AWS Lambda, Azure Functions, Google Cloud Functions/Cloud Run and OCI Functions, with provider-equivalent runtimes supported through Cloud Service Packs. |
| PRD-CLOBS-007 | P0 | AWS, Azure, GCP and OCI shall all be day-one provider roots for Cloud Observability GA. Cloud Observability GA may not claim multi-cloud day-one readiness if any of these four provider roots lacks certified onboarding, inventory, health, monitoring, change/event and diagnostic coverage defined by this master. |
| PRD-CLOBS-008 | P0 | Cloud coverage shall be delivered through the single Capability Pack architecture using certified Cloud Service Pack profiles for high-value native services rather than hard-coded provider-specific Core logic. |
| PRD-CLOBS-009 | P0 | Platform shall support enterprise cloud hierarchy discovery and policy scope: AWS Organization/OU/account, Azure tenant/management-group/subscription, GCP organization/folder/project and OCI tenancy/compartment. |
| PRD-CLOBS-010 | P0 | Platform shall provide cloud-native network observability covering provider virtual networks, interfaces, routes, load balancers, NAT, private connectivity, peering, VPN/dedicated connectivity, DNS and flow evidence where available, correlated with on-prem/WAN/path evidence. |
| PRD-CLOBS-011 | P0 | Platform shall ingest provider-native control-plane, audit, deployment, service-health and resource-change evidence and reconcile event-driven updates with periodic authoritative inventory. |
| PRD-CLOBS-012 | P0 | Cloud collection shall implement per-provider/account/API-family rate budgets, adaptive scheduling, retry/backoff, pagination/cursors, throttling visibility and freshness semantics so monitoring cannot silently self-degrade through provider API exhaustion. |
| PRD-CLOBS-013 | P0 | Platform shall monitor provider/service quotas, throttling and capacity exhaustion and forecast approaching limits where provider data permits. |
| PRD-CLOBS-014 | P0 | Platform shall monitor autoscaling state and failure for provider autoscaling groups, Kubernetes autoscaling and serverless concurrency where APIs permit, including oscillation, under-scaling and failed scale actions. |
| PRD-CLOBS-015 | P0 | Cloud-native logs shall be ingestible from provider-native and OTel paths with canonical entity/service correlation, trace/span correlation, operational search, pattern/anomaly support and privacy/cardinality controls. |
| PRD-CLOBS-016 | P0 | SLI/SLO/error-budget and burn-rate evaluation from Platform_21 shall be integrated with service/cloud entities, traces, synthetics, metrics and incidents and shall be visible in cloud/service workspaces. |
| PRD-CLOBS-017 | P0 | Platform shall expose observability coverage and blind spots by service/resource, including signal availability/freshness, ownership, SLO state, credential/connector health and material missing evidence. |
| PRD-CLOBS-018 | P0 | Cloud connectors and Cloud/API Sensors shall self-monitor API latency/errors, authorization failures, throttling, backlog, cursor/reconciliation age, partial coverage, dropped/rejected observations and last successful collection. |
| PRD-CLOBS-019 | P0 | Cloud onboarding shall be UI-driven, least-privilege, preflight-validated and capable of bulk provider-scope onboarding. Ephemeral child resources inherit governed parent-scope policy rather than requiring manual approval per pod/function/task/short-lived instance. |
| PRD-CLOBS-020 | P0 | Evidence-qualified RCA shall correlate cloud resource state, service graph, traces, logs, metrics, synthetics, provider health, quotas, scaling, network path and changes while preserving contradictions and permitting INCONCLUSIVE. |
| PRD-CLOBS-021 | P0 | Telemetry cardinality, ingest volume, sampling and retention shall be governed per tenant/signal/source with explainable cost/resource controls and protection against noisy-neighbor or unbounded high-cardinality telemetry. |
| PRD-CLOBS-022 | P1 | Platform shall support browser Real User Monitoring with privacy-bounded Web Vitals, navigation/request timing, JavaScript errors, resource timing and frontend-to-backend trace correlation. |
| PRD-CLOBS-023 | P1 | Platform shall support mobile RUM and browser journey synthetics after P0 signal/service identity is stable, without weakening privacy or tenant controls. |
| PRD-CLOBS-024 | P1 | Platform shall support continuous profiling and trace-to-profile correlation for CPU, wall time, allocation/memory and supported contention/I/O signals, with bounded retention. |
| PRD-CLOBS-025 | P1 | Platform shall support eBPF-assisted service/process/network dependency discovery where technically and legally appropriate, as evidence enrichment rather than an uncontrolled privileged collector. |
| PRD-CLOBS-026 | P1 | Platform shall support cloud cost/FinOps observability using provider billing exports and a FOCUS-compatible normalized cost model where practical, correlating cost with resource, workload, service, owner, utilization and change. |
| PRD-CLOBS-027 | P1 | Platform shall support customer AI/LLM workload observability using OpenTelemetry-compatible GenAI semantics where available, including provider/model/agent/tool identity, latency, token usage, errors, retries, cost metadata and evaluation signals. |
| PRD-CLOBS-028 | P1 | Platform shall support GPU/accelerator observability, observability-as-code, advanced service ownership/catalog linkage, signal-specific data-sovereignty controls and deeper query/database trace correlation. |

---

## 5. Day-one provider baseline

“Ready from day one” means ready for the first certified Cloud Observability GA, not that every service ever released by every cloud provider is automatically certified.

The four provider roots below are mandatory P0. Each provider shall have:

1. UI onboarding and least-privilege preflight.
2. hierarchy discovery.
3. inventory reconciliation.
4. core metrics/logs/events/change evidence.
5. provider/service health.
6. quotas/throttling.
7. cloud-network topology/evidence.
8. mandatory high-value Cloud Service Packs.
9. connector self-monitoring.
10. dimensional certification evidence and compatibility manifest.

### 5.1 AWS mandatory initial service families

Organization/IAM/STS; EC2/EBS; Elastic Load Balancing including ALB/NLB; Auto Scaling; VPC/subnet/route/security-group/NACL/ENI; NAT Gateway; Transit Gateway; Direct Connect; VPN; Route 53; CloudFront; S3; RDS/Aurora; DynamoDB; ElastiCache; EKS; ECS/Fargate; Lambda; API Gateway; SQS; SNS; EventBridge; Kinesis; CloudWatch; CloudTrail; AWS Health; Service Quotas.

### 5.2 Azure mandatory initial service families

Entra/tenant and management-group/subscription hierarchy; Virtual Machines/VM Scale Sets/disks; VNet/subnet/NIC/routes/NSG; Load Balancer; Application Gateway; Front Door; NAT Gateway; VPN; ExpressRoute; DNS/CDN; Storage; Azure SQL/Managed Instance and major managed PostgreSQL/MySQL offerings; Cosmos DB; managed Redis where available; AKS; Container Apps/ACI; Functions; App Service; API Management; Service Bus; Event Grid; Event Hubs; Azure Monitor; Activity/Resource Health/Service Health; quotas/limits.

### 5.3 GCP mandatory initial service families

Organization/folder/project hierarchy; Compute Engine/MIG/disks; VPC/subnet/firewall/routes; load balancing; Cloud NAT; VPN; Interconnect; Cloud DNS/CDN; Cloud Storage; Cloud SQL; Spanner; Firestore/Bigtable where certified; Memorystore; GKE; Cloud Run; Cloud Functions; API Gateway; Pub/Sub; supported streaming/data-service operational signals where certified; Cloud Monitoring; Cloud Logging; Audit Logs; provider service-health and quotas.

### 5.4 OCI mandatory initial service families

Tenancy/compartment hierarchy; Compute/autoscaling; Block/Object Storage; VCN/subnet/route/NSG/security-list/NIC; load balancing; NAT/DRG; VPN/FastConnect; DNS; core managed database families including Autonomous Database and supported DB systems; OKE; Functions; API Gateway; Streaming/Queue where certified; Monitoring; Logging; Audit; Events; provider health/limits/quotas.

Provider service names evolve. The machine-readable Capability Pack compatibility/certification registry is authoritative for exact service/version/region coverage at release time.

---

## 6. Customer cloud integration through the UI

Cloud integration shall not require customers to hand-edit Core configuration files for normal onboarding.

Primary path:

**Administration → Integrations → Cloud → Add Cloud Environment**

Wizard stages:

1. **Choose provider** — AWS, Azure, GCP or OCI.
2. **Choose scope** — organization/tenant/project hierarchy or one bounded account/subscription/project/compartment.
3. **Choose authentication** — provider-native temporary/delegated identity preferred; long-lived reusable secrets are fallback only where unavoidable.
4. **Generate/Show least-privilege requirements** — exact required and optional permissions by selected Cloud Service Packs.
5. **Test identity** — authenticate from the selected Cloud/API Sensor path and verify caller identity.
6. **Discover hierarchy** — show candidate accounts/subscriptions/projects/compartments and regions.
7. **Select monitoring scope** — include/exclude organization units, subscriptions/projects, regions, tags/labels and services.
8. **Select service packs** — recommended mandatory packs preselected; optional packs show required permission/API/telemetry implications.
9. **Collection plan** — metrics, logs, traces, events, audit/change, network flow and provider-health sources; show unavailable signals and prerequisites.
10. **Sensor placement** — select or recommend Cloud/API Sensor pool; test DNS/TLS/proxy/egress/provider API reachability.
11. **Rate/capacity preview** — projected API call budget, expected telemetry volume/cardinality, Sensor/Core load class and material provider-side telemetry/API cost caveats where estimable.
12. **Ownership/policy mapping** — site/region/business unit/team/service/tags, monitoring profile, alert evidence policy, retention, SLO template and notification routing.
13. **Ephemeral-resource policy** — define approved parent scope whose short-lived children are automatically monitored/reconciled without per-child approval.
14. **Discovery preview** — counts by type/region/account; duplicates/conflicts; permission gaps; unsupported resources; expected coverage.
15. **Activate** — explicit authorized action with audit record.
16. **Verify** — first collection, freshness, connector health, entity relationships and coverage/blind-spot report.
17. **Continuous reconciliation** — provider hierarchy/resource drift is reconciled automatically; destructive scope/credential changes follow governed review.

UI states shall distinguish at minimum:

CONNECTED, PARTIAL, AUTHORIZATION_DEGRADED, THROTTLED, STALE, RECONCILING, MISCONFIGURED, DISCONNECTED, SUSPENDED.

No state may convert provider API failure into resource-down truth without corroborating evidence.

---

## 7. Authentication and credential principles

Preferred patterns:

- AWS: delegated/cross-account role and short-lived STS credentials; External ID/organization policy where appropriate.
- Azure: managed identity/workload identity or Entra application/service principal using least privilege and short-lived token flows.
- GCP: workload identity federation/service-account impersonation preferred; static service-account key material only as a controlled fallback.
- OCI: workload/resource/instance-principal style identity where available; protected API signing credentials only where required.
- Kubernetes: service account/client identity scoped to required resources; read-only by default.

Platform shall store credential metadata/reference and authorization scope, not display reusable secrets after creation. Rotation/expiry/last-verified state is visible and auditable.

---

## 8. Unified resource and service identity

Cloud/provider IDs are evidence and aliases, not tenant authority.

Canonical identity shall support:

tenant
→ cloud provider
→ organization/tenant
→ management hierarchy
→ account/subscription/project/compartment
→ region/zone
→ network
→ resource
→ runtime/workload
→ service/application
→ business service.

The same runtime must not become duplicate canonical entities merely because it is seen through cloud API, Kubernetes, Agent, OTel, logs, flow or synthetics.

Relevant identity attributes include provider resource ID/ARN/URI, cloud account/project scope, region/zone, Kubernetes UID, host/agent identity, OTel resource attributes, service.name/service.namespace/service.instance.id equivalents, deployment/environment/version and customer-defined ownership.

Identity conflicts are explicit reconciliation cases.

---

## 9. Service graph

P0 service graph relationships may be derived from:

- OTel spans and resource attributes;
- Kubernetes Service/Ingress/Endpoint/workload relationships;
- provider load-balancer/backend target relationships;
- API gateway routes;
- queue/topic producer-consumer evidence;
- database/cache client evidence;
- cloud virtual-network relationships;
- Network Flow where available;
- synthetics;
- customer-defined authoritative relationships;
- Platform_19 cloud/deployment change evidence.

Derived relationships record source, confidence, first/last observation, age and evidence reference.

Inferred runtime relationships may not silently overwrite customer-governed authoritative relationships.

---

## 10. OpenTelemetry traces — P0

Platform shall accept OTLP traces over approved secure endpoints.

Trace model shall support:

- trace ID and span ID;
- parent and span links;
- service/resource identity;
- span kind;
- operation name;
- start/end/duration;
- status/error;
- bounded attributes/events;
- HTTP/RPC/database/messaging/cloud/Kubernetes semantic fields where provided;
- deployment version/environment;
- tenant scope;
- sampling metadata;
- event and ingest time;
- provenance/collector identity.

Sampling:

- head sampling supported;
- tail sampling supported where deployment architecture permits;
- adaptive policy may retain a higher proportion of errors, high-latency, SLO-impacting or incident-correlated traces;
- sampling must be explainable and tenant-scoped;
- 100% indefinite trace retention is not a default assumption.

Trace-derived RED metrics may be generated for request rate, errors and duration without replacing original evidence semantics.

---

## 11. Prometheus / OpenMetrics — P0

Platform shall interoperate with the Prometheus/OpenMetrics ecosystem without making Prometheus the primary Platform TSDB or alert authority.

Supported patterns may include:

- Sensor/OTel Collector scrape of Prometheus/OpenMetrics endpoints;
- approved OTel Prometheus receiver paths;
- a bounded remote-write-compatible ingestion surface where architecture/performance testing approves it;
- Kubernetes service/pod discovery feeding governed scrape targets.

Prometheus labels are subject to tenant/cardinality policy and canonical identity mapping.

Alertmanager is not the Platform alert engine.

---

## 12. Deep Kubernetes observability — P0

In addition to existing cluster/node/namespace/workload/pod/container/service/ingress entities, monitoring shall cover where supported:

- Deployment, StatefulSet, DaemonSet, Job and CronJob;
- desired/current/available replica state;
- rollout/revision/image changes;
- pending/unschedulable reason;
- taints/tolerations/affinity scheduling evidence;
- requests/limits vs actual utilization;
- OOMKill, CrashLoopBackOff and termination reason;
- HPA/VPA and cluster-autoscaler state/events;
- PVC/PV/storage capacity, latency and mount/attachment failures;
- service/endpoints/ingress relationships;
- DNS/network-policy/service-mesh evidence where available;
- node pressure and eviction;
- Kubernetes events;
- labels/annotations/team/service ownership mapping;
- cluster/provider relationship for EKS/AKS/GKE/OKE.

Short-lived Pods/Jobs/containers use parent-scope lifecycle policy and do not create manual approval floods.

---

## 13. Serverless observability — P0

For supported providers Platform shall expose:

- function/service identity and version/alias/revision;
- invocation/request rate;
- duration/latency;
- errors/failures;
- throttling/rate limiting;
- concurrency and saturation;
- cold-start/init indicators where provider/runtime exposes them;
- memory/resource use where available;
- trigger/source relationships;
- downstream dependency evidence;
- logs and traces;
- deployment/change evidence;
- quota state;
- cost metadata in P1 where billing correlation is enabled.

Ephemeral execution instances are observations of the canonical function/service, not permanent inventory entities unless provider semantics require otherwise.

---

## 14. Cloud network observability — P0

Canonical cloud-network entities include, where provider supports them:

virtual network/VPC/VCN, subnet, route table/route, network interface, security/NACL-style policy relation, load balancer/listener/target, NAT, gateway, peering, private endpoint/private service connectivity, transit hub, VPN, dedicated connectivity, DNS zone/record/resolver and flow-log source.

Platform shall correlate:

cloud flow evidence
+ provider routes
+ provider load-balancer targets
+ Kubernetes service/network evidence
+ on-prem route/path/flow evidence
+ synthetics
+ circuit/ISP inventory
+ change evidence.

The UI should allow an operator to traverse from a slow/error service to the relevant cloud network path and then, when known, into on-prem WAN/router/circuit state.

---

## 15. Cloud change, service-health and control-plane evidence — P0

In addition to Platform_19 normalized change intelligence, each provider integration shall support where available:

- audit/control-plane operations;
- deployment/revision changes;
- networking/security/control-plane changes;
- autoscaling actions;
- quota changes;
- managed-service maintenance/failover;
- provider service-health/regional degradation;
- resource create/delete/move/tag/config changes.

Provider status is supporting evidence, not causal proof.

Event-driven ingestion shall be reconciled by periodic authoritative inventory so missed events do not create permanent state divergence.

---

## 16. Quotas, throttling and autoscaling — P0

Quota and capacity objects shall capture:

limit, current usage where available, scope, reset/window semantics, last verified time and forecast risk.

Autoscaling state shall capture:

desired/current/min/max, triggering policy, recent scale actions, failed actions, stabilization/cooldown, oscillation indicators and relevant quota/resource blockers.

Alerts distinguish workload failure from inability to observe scaling or quota state.

---

## 17. SLI/SLO/error-budget integration — P0

Platform_21 owns the canonical SLI/SLO/error-budget semantics.

Cloud/full-stack integration shall allow SLIs to be backed by:

- trace-derived success/latency;
- OTel/provider metrics;
- synthetics;
- API/transaction outcomes;
- queue/job outcomes;
- custom governed telemetry.

Service and Cloud views shall show:

objective, current attainment, error budget remaining, burn rate, active incidents, top contributing services/dependencies and recent changes.

No SLO definition becomes a contractual customer commitment merely by being configured in the product.

---

## 18. Observability coverage and blind spots — P0

Coverage shall be calculated as evidence facts, not one misleading universal score.

For each applicable resource/service scope Platform shall show:

- discovered resources;
- monitored resources;
- metrics available/fresh;
- logs available/fresh;
- traces available/fresh;
- synthetics present;
- change/audit telemetry present;
- network visibility present;
- ownership known;
- SLO present where required;
- connector/credential health;
- unsupported/unavailable provider telemetry;
- last successful reconciliation;
- known evidence gaps affecting RCA confidence.

Example UI statement:

“RCA confidence limited: database traces are unavailable and Azure Activity telemetry is stale.”

---

## 19. Cloud connector self-observability — P0

Every provider connection shall expose:

authentication state; permission drift; token/credential expiry; API calls/success/error/throttle; retry/backoff; pages/cursors; reconciliation age; inventory lag; metric/log/event lag; rejected observations; rate-budget utilization; Sensor health; provider endpoint health; data freshness and last-good timestamps.

Cloud connector health is itself monitored and can create MONITORING_DEGRADED/STALE states without declaring managed resources down.

---

## 20. Database and storage architecture

### 20.1 Governing principle

Do not add a new mandatory database merely to implement Cloud Observability.

Use existing storage roles by data semantics.

### 20.2 PostgreSQL — authoritative control and current-state metadata

PostgreSQL owns transactional/control-plane state such as:

- cloud provider connections and tenant ownership;
- organization/account/subscription/project/compartment hierarchy metadata;
- credential references and permission profiles;
- provider/service-pack bindings and versions;
- monitoring/profile assignments;
- durable collection/reconciliation cursors when correctness requires persistence;
- canonical resource/service identity and stable aliases;
- current governed ownership/group/tag metadata;
- ephemeral-parent activation policy;
- SLO definitions and policy;
- coverage policy/requirements;
- connector configuration and desired state;
- compatibility/certification references;
- investigation/annotation/control metadata where existing domains already use PostgreSQL.

High-rate telemetry does not belong in PostgreSQL merely because it is relationally convenient.

### 20.3 ClickHouse — high-volume observability telemetry

ClickHouse owns high-volume analytical observations including, through versioned logical schemas:

- metric samples and rollups;
- operational logs/events;
- trace spans and derived trace metrics;
- cloud audit/control-plane event facts;
- normalized cloud changes;
- cloud network-flow facts/summaries;
- provider service-health observations;
- quota/autoscaling observations when high-rate/historical;
- coverage-history facts where trend analysis is required;
- P1 RUM events;
- P1 profiling samples/aggregates;
- P1 normalized cloud-cost usage facts where volume requires analytical storage.

Tables are tenant-partitioned/logically tenant-keyed, time-partitioned and ordered for dominant query paths. Retention/TTL/rollup is signal-specific. High-cardinality attribute maps require bounded indexing/materialized-column strategy; arbitrary attributes must not create unbounded schema or index explosion.

### 20.4 Kafka — internal event backbone

Versioned topic families shall cover, as applicable:

telemetry.metric.v1
telemetry.log.v1
telemetry.trace.v1
cloud.inventory.v1
cloud.event.v1
cloud.change.v1
cloud.network_flow.v1
cloud.connector_health.v1
cloud.coverage.v1
slo.evaluation.v1

Exact topic naming may follow existing repository conventions, but schemas remain versioned and replay/idempotency aware.

Sensors/collectors do not use Kafka directly over customer WAN links.

### 20.5 Valkey — ephemeral/reconstructable state only

Valkey may cache:

recent query results, transient aggregation windows, token buckets/rate budgets, bounded correlation state and short-lived lookup caches.

Provider inventory authority, durable cursors, credentials, SLO definitions and canonical identity must not rely solely on Valkey.

### 20.6 Bounded artifact storage

Existing encrypted artifact-storage abstraction may hold bounded diagnostic artifacts or P1 profile payloads when ClickHouse rows are not the correct representation.

Session replay is deferred and must not be enabled by implication.

### 20.7 Trace retention and indexing

Trace storage shall use:

- time partitioning;
- tenant/service/resource/time sort keys;
- selective materialized searchable fields;
- bounded attribute indexing;
- sampling/retention classes;
- incident pinning via Incident Flight Recorder where approved;
- rollup/derived RED metrics for long-range analysis.

Trace attributes can contain sensitive data. Default semantic-convention allowlists/redaction and customer controls are required before storage/indexing.

### 20.8 Cost data — P1

Cloud cost facts shall be normalized to a FOCUS-compatible logical model where practical. PostgreSQL stores billing-source configuration/allocation policy; ClickHouse stores high-volume usage/cost facts. Provider billing export remains source evidence and may arrive delayed; cost data must never be treated as real-time availability truth.

---

### 20.9 Logical PostgreSQL table families

The names below are **logical schema contracts**, not permission to bypass the repository's migration/schema naming conventions. Physical names are frozen through normal migration review.

| Logical family | Key data | Required integrity/index behavior |
|---|---|---|
| `cloud_connection` | tenant, provider, root scope, auth-mode/credential reference, Sensor pool, desired state, status | unique tenant/provider/root-scope identity; tenant-first access; no secret value columns |
| `cloud_scope` | organization/tenant/folder/OU/account/subscription/project/compartment hierarchy, parent, display metadata, enabled regions | unique canonical scope identity within tenant/provider; parent/child traversal index |
| `cloud_pack_binding` | connection/scope, Capability Pack ID/version, effective policy, enabled state | FK to connection/scope/pack registry; versioned/auditable |
| `cloud_reconcile_cursor` | provider/API family/scope cursor or watermark, last success, last full reconciliation, error state | durable update semantics; tenant/provider/scope lookup; never stored only in Valkey |
| `cloud_external_identity_binding` | canonical entity/service ID ↔ provider external ID/ARN/URI/K8s UID/OTel resource identity | unique active binding within tenant/source scope; conflict state instead of silent merge |
| `cloud_parent_activation_policy` | approved durable scope, include/exclude predicates, inherited profile/retention/alert/SLO policy | versioned, auditable and linked to Platform_20 authorization decision |
| `cloud_coverage_expectation` | scope/service class and required metric/log/trace/change/network/SLO evidence | versioned policy; supports explicit unsupported/not-required state |
| `cloud_connector_desired_state` | rate budgets, collection classes, regions, service packs, reconciliation cadence | versioned desired state with observed/applied version |
| `service_catalog_binding` | canonical service/business-service ownership, environment, version/repository/runbook references where governed | tenant/service unique active identity; ownership history preserved |
| `slo_definition/reference` | use/refer to Platform_21 authoritative SLO domain | do not fork a second SLO definition table solely for cloud |

Cloud current-state materialization may reuse existing canonical inventory/entity/relationship tables rather than duplicate one PostgreSQL table for every provider resource type. Provider-specific payload fragments are retained only when needed for provenance/diagnostics and are bounded by schema/security policy.

### 20.10 Logical ClickHouse table families

| Logical family | Core fields/query keys | Storage rules |
|---|---|---|
| `trace_span` | tenant, trace_id, span_id, parent/link IDs, service/entity IDs, environment/version, start/end/duration, status, span kind, bounded attributes/events, source/collector | time partition; tenant + service/entity + time ordering; trace-id lookup acceleration; materialize frequently queried semantic fields; do not index arbitrary attributes indiscriminately |
| `cloud_operational_event` | tenant, provider, scope/entity/service, event class, source event ID, event/ingest time, actor/service identity where available, normalized state/result, raw evidence reference | dedupe/idempotency by source identity; time-partitioned; provider/source provenance retained |
| `cloud_connector_health_history` | tenant, connection, API family, auth/permission state, API latency/error/throttle, rate-budget usage, cursor/reconcile age, lag/freshness | bounded operational history; current authoritative desired/config state stays in PostgreSQL |
| `cloud_quota_sample` | tenant, provider/scope/resource/service quota, limit, observed usage, forecast risk, observation time | time series/rollup; unknown usage remains unknown rather than fabricated |
| `cloud_autoscaling_event` | tenant, scaling target, desired/current/min/max, policy, action/result, blocker/quota evidence, time | event-time ordering and incident/change correlation |
| `observability_coverage_history` | tenant, scope/service/entity, expected signal, observed signal/freshness/quality, connector state, gap reason | use for trend/coverage history; current expectations/config remain PostgreSQL |
| `rum_event` (P1) | tenant, app/service/session-safe pseudonymous IDs, page/navigation/Web-Vitals/error/request timing, trace correlation | privacy minimization, high-cardinality limits, short/default-bounded retention |
| `profile_sample/aggregate` (P1) | tenant, service/version/runtime/profile type, stack/function symbol reference, value/time, trace correlation where supported | prefer aggregated/bounded representation; artifact store permitted for payloads not suited to rows |
| `cloud_cost_usage` (P1) | tenant, billing period/time, provider/account, service/resource/workload/owner allocation, normalized FOCUS-compatible dimensions, amount/currency/unit, source/estimate flag | billing facts are delayed analytical data; partition by billing/event period; never operational availability truth |

Existing Platform tables shall be reused rather than forked where semantics already exist:

- network flow data remains the existing `network_flow` family enriched with cloud/provider/interface/subnet/service identity;
- Platform_19 normalized change evidence remains the authoritative cross-domain change model;
- generic metric/log/event/audit table families remain reusable where their schema satisfies Platform_26 identity/provenance requirements;
- Incident Flight Recorder pins/references existing evidence instead of copying every signal into a second incident database.

### 20.11 Partitioning, indexing and retention invariants

- PostgreSQL indexes begin with tenant/scope dimensions for tenant-owned high-selectivity access paths; provider external identifiers are never globally trusted without tenant/source scope.
- ClickHouse partitioning is time-oriented and designed to avoid per-tenant/per-resource partition explosion. Dominant ORDER BY/query keys begin with tenant plus stable service/entity/signal dimensions and time as appropriate.
- Trace-ID, error/status and selected semantic-convention fields may use materialized/data-skipping indexes after benchmark; arbitrary attribute keys must not generate unbounded indexes/materialized columns.
- Every high-volume table has a signal-specific TTL/retention class compatible with Platform_11 storage protection. Incident/legal-hold pinning uses the existing protected-evidence mechanism.
- Late/out-of-order, replay and duplicate cloud/trace events are handled using existing ingestion idempotency/event-time semantics; a provider retry must not create duplicate incident effects.
- P1 cost/RUM/profile retention and access policies are independent from operational metrics/logs/traces and may be disabled without breaking P0 monitoring.
- Schema evolution uses versioned events and normal database migrations; provider API field changes do not directly mutate canonical schemas without normalization/version handling.

### 20.12 P0 end-to-end dataflows

**Cloud onboarding / inventory:**

```text
UI Add Cloud Environment
→ Core validates tenant/RBAC + stores protected credential reference
→ Cloud/API Sensor executes identity/permission/network preflight
→ provider hierarchy/resource discovery
→ normalize identity + reconcile canonical entities/relationships
→ PostgreSQL current/control state
→ inventory/change events through Kafka where applicable
→ ClickHouse historical evidence
→ Graph / Coverage / UI verification
```

**OpenTelemetry trace ingestion:**

```text
Application / OTel SDK or Collector
→ approved OTLP endpoint / Sensor path
→ tenant/source/schema/size/cardinality validation
→ sampling/backpressure policy
→ Kafka internal telemetry.trace event
→ trace normalization + canonical service/resource mapping
→ ClickHouse trace_span
→ derived RED metrics / service graph / SLO / investigation / RCA
```

**Provider event/change path:**

```text
Provider event/audit source
→ Cloud/API Sensor or approved connector
→ source authentication + replay/idempotency validation
→ normalized cloud event/change
→ Kafka
→ ClickHouse evidence history
→ canonical current-state reconciliation where applicable
→ Change Intelligence / Incident Flight Recorder / RCA
→ periodic inventory reconciliation repairs missed-event drift
```

**Query/investigation path:**

```text
UI Service / Trace / Cloud / Incident view
→ Query API enforces tenant/resource/time/cardinality limits
→ PostgreSQL current identity/control + ClickHouse historical signals
→ Graph/Correlation joins by canonical entity/service/evidence references
→ bounded response with freshness/quality/provenance
→ UI exposes evidence, contradictions and missing coverage
```

## 21. API and event contracts

P0 API surfaces shall exist for at least:

- cloud provider connections;
- scope/hierarchy discovery;
- permission/preflight tests;
- cloud resource/service query;
- connector health;
- coverage/blind spots;
- traces and cross-signal pivots;
- service graph/dependency query;
- cloud changes/events;
- quota/autoscaling;
- SLO/service status;
- Cloud Service Pack compatibility/certification status.

All APIs enforce tenant/resource scope server-side, paginate large collections and expose stable error/correlation IDs.

Observation/event envelopes preserve:

event time, ingest time, tenant, provider/source, canonical entity/service references, schema version, provenance, quality/freshness, correlation IDs and idempotency identity where needed.

---

## 22. UI / UX product surfaces

### 22.1 Multi-Cloud Overview — enhanced

Add views for:

Overview; Providers; Organizations/Accounts; Regions; Services; Kubernetes; Serverless; Network; Databases; Queues/Streams; SLOs; Changes; Quotas; Coverage; Connector Health; Costs (P1).

### 22.2 Cloud Provider Workspace

Per AWS/Azure/GCP/OCI:

hierarchy, service-family health, region health, active incidents, SLO impact, quota risk, connector health, recent changes and coverage gaps.

### 22.3 Service Overview

For a canonical application/service:

health, request rate, error rate, latency, SLO/error budget, traces, logs, metrics, dependencies, runtime instances, cloud resources, network path, deployments/changes, active incidents and RCA.

### 22.4 Trace Explorer — P0

Search/filter by tenant/service/environment/version/operation/status/duration/resource/cloud/Kubernetes attributes within policy limits.

Trace detail shows waterfall, span tree, critical path indicators, related logs/metrics/events, resource identity, deployments/changes and service graph links.

### 22.5 Kubernetes Workspace — enhanced

Cluster; Nodes; Namespaces; Workloads; Controllers; Pods; Containers; Services; Ingress; Storage; Autoscaling; Events; Changes; SLO; Coverage.

### 22.6 Serverless Workspace

Provider/function/service; versions/revisions; invocations; latency; errors; throttles; concurrency; cold-start indicators; triggers/dependencies; logs; traces; changes; quotas; SLO.

### 22.7 Cloud Network Workspace

Provider network graph; route/path; LB/target relationships; NAT/transit/private connectivity; flow analytics; DNS; synthetics; on-prem path correlation; changes; RCA.

### 22.8 Observability Coverage Workspace

Scope tree with evidence availability/freshness, connector/credential health, unsupported signals, policy expectation, blind spots and recommended remediation.

### 22.9 Cloud Integration Administration

List provider connections with scope, owner, Sensor pool, auth mode, selected packs, state, last verified, last inventory, API budget, coverage and actions: Test, Reconcile Now, Rotate/Rebind Credential, Edit Scope, View Permission Diff, Suspend, Remove.

### 22.10 P1 surfaces

RUM, Mobile Experience, Profiling, Cloud Cost/FinOps, AI/LLM Observability and GPU/Accelerator workspaces remain P1 and use the same global shell/service identity rather than independent products.

Every view supports Loading, Empty, Partial Data, Stale Data, Permission Restricted, Not Licensed, Query Limited, Error, Monitoring Degraded and Normal states.

---

## 23. P1 capability details

### 23.1 Real User Monitoring

Browser RUM supports privacy-bounded page/navigation timing, Core Web Vitals, resource timing, JavaScript errors, fetch/XHR timing, geography/device/browser class and trace-context correlation.

Keystroke capture, arbitrary page-content capture and employee-surveillance behavior are prohibited by default. Session replay remains deferred pending separate privacy/security/storage change control.

### 23.2 Mobile RUM and browser journeys

Mobile SDK strategy/runtime is not selected by this specification. Browser journey synthetics build on Platform_21 browser-synthetic governance.

### 23.3 Continuous profiling

Profiles may include CPU, wall time, memory/allocation and supported lock/contention/I/O signals. Trace-to-profile correlation should preserve service/version/runtime identity.

### 23.4 eBPF-assisted discovery

eBPF may enrich process/service/network relationships, but requires explicit privilege, kernel compatibility, overhead, security and packaging certification. It is not a mandatory P0 dependency.

### 23.5 FinOps / cloud cost observability

Capabilities include:

- provider billing-export ingestion;
- FOCUS-compatible normalization where practical;
- cost by provider/account/region/service/resource/workload/business service/team/cost center;
- change-to-cost correlation;
- budget and anomaly facts where configured;
- rightsizing recommendation inputs;
- idle/orphaned resource candidates;
- Kubernetes workload allocation;
- commitment/discount opportunity evidence where provider data supports it.

Platform shall distinguish measured provider billing facts from estimates/recommendations.

### 23.6 Customer AI/LLM observability

Where OpenTelemetry or provider APIs expose data, capture:

model/provider; agent/workflow; request; first-token/total latency; input/output/cache tokens; tool calls; retries/errors; finish reason; RAG/retrieval references where permitted; deployment/model change; evaluation signal; estimated/provider-reported cost metadata.

This does not weaken Platform_22 AI provider/egress/tool-use controls for Platform's own AI.

### 23.7 GPU/accelerator

Monitor utilization, memory, temperature/power where exposed, error state, saturation and mapping to workload/service.

### 23.8 Observability-as-code

Declarative monitor/SLO/dashboard/service metadata may be managed through governed APIs/Git/policy-as-code but cannot bypass RBAC, validation, approval, audit or compatibility checks.

---

## 24. Security, privacy and tenancy

Mandatory controls:

- least privilege provider roles;
- tenant-scoped credentials and resource ownership;
- no browser exposure of reusable cloud credentials;
- secret redaction in logs/traces/events;
- sensitive attribute classification/drop/hash/mask/store policy;
- provider API egress allowlisting/proxy support;
- TLS validation;
- rate limiting and anti-abuse controls;
- trace/log/RUM payload size and attribute limits;
- audit of cloud connection creation/change/test/remove;
- audit of credential binding/rotation and permission drift;
- no cross-tenant resource identity merging;
- no cross-tenant trace/log search leakage;
- data-sovereignty policy must apply per signal/source when P1 regional controls are enabled.

Cloud audit logs and provider events are untrusted external evidence until schema/auth/source validation passes.

---

## 25. Capacity, performance and telemetry economics

Cloud workload planning shall include:

- resources by provider/account/region/type;
- provider API calls/minute and quota;
- metric samples/sec;
- log bytes/events/sec;
- spans/sec and attributes/span;
- flow records/sec;
- audit/change events/sec;
- Kubernetes object/event churn;
- serverless invocations;
- number of SLOs and evaluation windows;
- concurrent Trace/Log/Service-graph queries;
- retention and rollup;
- P1 RUM/profile/cost volumes.

P0 acceptance requires cardinality-bomb tests, provider-throttling tests, partial-permission tests and high-churn resource tests.

Collection policy should prefer delta/event/stream methods where correct, but periodic reconciliation remains mandatory where needed for authoritative state.

Telemetry-volume optimization recommendations are advisory unless explicit safe automatic policy is enabled.

---

## 26. Failure and degradation behavior

Examples:

- provider auth revoked → AUTHORIZATION_DEGRADED; do not mark resources down;
- API throttled → THROTTLED/MONITORING_DEGRADED; back off and preserve last-good state with freshness;
- event stream missed → periodic reconciliation repairs drift;
- trace backend pressure → apply governed sampling/backpressure; surface reduced trace coverage;
- log source inaccessible → coverage gap; service may still be healthy;
- one cloud account unavailable while others work → partial provider state, not global cloud failure;
- provider health reports outage but customer synthetics are healthy → preserve contradiction; do not assert provider outage as root cause automatically;
- metric/log/trace identities conflict → reconciliation finding; no silent merge;
- cost feed delayed → cost stale, never operational-down.

---

## 27. Quality and certification gates

Cloud Observability GA requires evidence for all P0 requirements.

At minimum test:

- AWS/Azure/GCP/OCI onboarding happy path;
- wrong/expired/insufficient permissions;
- organization-scale hierarchy discovery;
- provider API throttling and Retry-After behavior;
- cursor/pagination interruption and resume;
- event loss followed by reconciliation repair;
- duplicate/replayed provider events;
- region/account partial failure;
- high-cardinality metrics/logs/traces;
- trace sampling correctness;
- cross-signal trace-log-metric-service identity;
- Kubernetes high-churn/ephemeral identity;
- serverless correlation;
- cloud network route/flow relationships;
- quota/autoscaling alert semantics;
- SLO/error-budget evaluation;
- connector self-monitoring;
- coverage blind-spot accuracy;
- tenant isolation;
- secrets/sensitive-field redaction;
- ClickHouse/PostgreSQL/Kafka failure/replay behavior under existing HA contracts;
- upgrade/rollback/pack compatibility;
- data-retention/80% storage protection;
- UI partial/stale/error states.

Certification is dimensional and uses Platform_23. “Specified” does not mean “implemented”; “implemented” does not mean “certified”; “certified” does not mean “deployed in every customer environment.”

---

## 28. Implementation sequencing

| Stage | Scope |
|---|---|
| CLOBS-0 | This specification, cross-document reconciliation, contracts, compatibility model |
| CLOBS-1 | Unified service/resource identity; OTLP traces; trace storage/query; cross-signal correlation; Prometheus/OpenMetrics ingestion |
| CLOBS-2 | Cloud onboarding framework; hierarchy discovery; connector health/rate budgets; AWS/Azure/GCP/OCI provider roots |
| CLOBS-3 | Mandatory Cloud Service Packs; deep Kubernetes; serverless; provider events/change/service health; quotas/autoscaling |
| CLOBS-4 | Cloud network observability; SLO integration; coverage/blind spots; cloud-aware RCA/Flight Recorder |
| CLOBS-5 | P1 RUM/browser/mobile/profiling/eBPF |
| CLOBS-6 | P1 FinOps/FOCUS, AI/LLM/GPU observability and observability-as-code expansion |

Stages are engineering sequencing, not commercial SKUs or release-date commitments.

---

## 29. Explicit boundaries

The following are not implied by this master:

- unrestricted SIEM replacement;
- CNAPP/CSPM replacement;
- full always-on DPI;
- mandatory proprietary bytecode APM agents for every language;
- browser session replay;
- employee surveillance;
- autonomous LLM/SRE privileged remediation;
- broad cloud configuration-management replacement;
- universal support for every cloud service without pack certification;
- new mandatory database merely for observability;
- a second pack framework outside Capability Pack.

---

## 30. Cross-document reconciliation requirements

On merge:

- Platform_00 shall register this specialized master and precedence.
- Platform_01/02 shall replace the old metrics/logs-only trace-ready scope wording with P0 OpenTelemetry traces while retaining the boundary against mandatory full proprietary bytecode APM.
- Platform_03/07 shall update OpenTelemetry architecture/OSS wording.
- Platform_08 shall update the base Infrastructure/Cloud traceability summary and reference REQ-CLOBS.
- Platform_10 shall include OCI and the cloud-auth/preflight contract.
- Platform_12 shall record the Founder-approved product-scope change and P0/P1 classification.
- Platform_17 shall define/reference additive cloud/trace/coverage contracts and storage ownership.
- Platform_18 shall reference cloud credential, telemetry-content and tenant-isolation risks.
- Platform_20 shall reconcile parent-scope/ephemeral child onboarding.
- Platform_21/22/23 shall cross-reference this master where SLO/browser, AI observability and Cloud Capability Pack authority overlap.
- Platform_14 shall add P0 cloud certification/release gates.

---

## 31. Formal traceability catalogue

| REQ ID | Owning PRD | Required acceptance evidence |
|---|---|---|
| REQ-CLOBS-001 | PRD-CLOBS-001 | OTLP trace ingest/query/sampling/tenant-isolation tests |
| REQ-CLOBS-002 | PRD-CLOBS-002 | same resource/service correlates across provider/K8s/Agent/OTel signals without duplicate canonical identity |
| REQ-CLOBS-003 | PRD-CLOBS-003 | metric/log/trace/event/change/topology pivot integration tests |
| REQ-CLOBS-004 | PRD-CLOBS-004 | Prometheus/OpenMetrics scrape/ingest/cardinality tests without Prometheus becoming authoritative TSDB |
| REQ-CLOBS-005 | PRD-CLOBS-005 | deep K8s scheduling/controller/storage/autoscaling/event tests |
| REQ-CLOBS-006 | PRD-CLOBS-006 | serverless invocation/error/throttle/concurrency/log/trace/change tests |
| REQ-CLOBS-007 | PRD-CLOBS-007 | AWS/Azure/GCP/OCI P0 provider-root certification matrix |
| REQ-CLOBS-008 | PRD-CLOBS-008 | Cloud Service Packs use Capability Pack loader/schema/certification; no hard-coded Core fork |
| REQ-CLOBS-009 | PRD-CLOBS-009 | organization/tenant hierarchy discovery, policy and tenant-boundary tests |
| REQ-CLOBS-010 | PRD-CLOBS-010 | cloud network graph/route/flow/LB/on-prem-path correlation tests |
| REQ-CLOBS-011 | PRD-CLOBS-011 | provider event/change ingestion plus missed-event reconciliation tests |
| REQ-CLOBS-012 | PRD-CLOBS-012 | throttling/rate-budget/backoff/pagination/freshness tests |
| REQ-CLOBS-013 | PRD-CLOBS-013 | quota/limit observation and forecast tests with missing-data semantics |
| REQ-CLOBS-014 | PRD-CLOBS-014 | autoscaling state/failure/oscillation tests |
| REQ-CLOBS-015 | PRD-CLOBS-015 | cloud log ingestion/correlation/redaction/cardinality tests |
| REQ-CLOBS-016 | PRD-CLOBS-016 | cloud/service SLI/SLO/error-budget and burn-rate integration tests |
| REQ-CLOBS-017 | PRD-CLOBS-017 | coverage/blind-spot calculations correctly reflect missing/stale/unsupported evidence |
| REQ-CLOBS-018 | PRD-CLOBS-018 | connector self-monitoring and MONITORING_DEGRADED behavior tests |
| REQ-CLOBS-019 | PRD-CLOBS-019 | complete UI onboarding/preflight/preview/activate/verify flow; ephemeral-parent policy tests |
| REQ-CLOBS-020 | PRD-CLOBS-020 | cloud-aware RCA shows supporting/contradictory/missing evidence and INCONCLUSIVE path |
| REQ-CLOBS-021 | PRD-CLOBS-021 | signal volume/cardinality/sampling/noisy-neighbor protection tests |
| REQ-CLOBS-022 | PRD-CLOBS-022 | P1 browser RUM privacy/performance/trace-correlation acceptance |
| REQ-CLOBS-023 | PRD-CLOBS-023 | P1 mobile/browser-journey compatibility and privacy tests |
| REQ-CLOBS-024 | PRD-CLOBS-024 | P1 profile capture/query/trace correlation and overhead tests |
| REQ-CLOBS-025 | PRD-CLOBS-025 | P1 eBPF privilege/kernel/overhead/security and relationship-quality tests |
| REQ-CLOBS-026 | PRD-CLOBS-026 | P1 cost normalization/allocation/correlation with measured-vs-estimated distinction |
| REQ-CLOBS-027 | PRD-CLOBS-027 | P1 GenAI telemetry identity/token/latency/tool/cost/evaluation tests with data-egress controls |
| REQ-CLOBS-028 | PRD-CLOBS-028 | P1 accelerator/observability-as-code/service-ownership/data-sovereignty/deep-DB acceptance evidence |

---

## 32. Decision summary

The approved architectural shift is:

**Old boundary:** OpenTelemetry metrics/logs initially; trace-ready; no full APM.

**New boundary:** Metrics + Logs + Traces + Events + Topology + Changes + Synthetics are P0 observability signals. Platform remains independent of mandatory proprietary full-bytecode APM agents; RUM/profiling/eBPF/FinOps/customer-AI observability are P1.

The product differentiation target is not “more cloud dashboards.” It is one evidence-preserving operational graph and investigation model spanning network, infrastructure, Kubernetes, cloud, services, telemetry, changes, SLOs and business impact.
