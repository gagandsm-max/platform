# Platform — Integration, Automation, ITSM & Ecosystem Master

**Document ID:** PLATFORM-IAX-001  
**Version:** 3.2.2  
**Status:** **Authoritative Mother Reference — Integration / Automation / ITSM / Ecosystem Specialized Master**  
**Product:** Platform  
**Change class:** Additive architecture/product clarification after independent CTO challenge; extends existing requirements without weakening any prior requirement  
**Reconciliation date:** 2026-09-19

---

## 1. Authority and purpose

This document is the authoritative specialized Mother Reference for the cross-cutting lifecycle from qualified Platform evidence through external workflow, controlled automation, validation and synchronization.

It supplements, rather than replaces:

- `Platform_02_PRD.md` — base ITSM/notification behavior;
- `Platform_04_HLD.md` — Integration Service and NCM boundaries;
- `Platform_05_LLD.md` — ITSM connector, incident and NCM models;
- `Platform_06_Dataflow.md` — ITSM, NCM and remediation sequences;
- `Platform_10_Protocol_Security_Connector_Master.md` — connector/security mechanics;
- `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` — change intelligence and controlled remediation;
- `Platform_21_Enterprise_Gap_Closure_Master.md` — declarative configuration/policy-as-code;
- `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` — evidence/confidence and tool-use safety;
- `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md` — Capability Pack / Integration Pack architecture.

The objective is a coherent, customer-configurable lifecycle:

```text
Monitoring Evidence
  -> Alert Qualification
  -> Correlation / Incident
  -> RCA / Evidence
  -> Routing Policy
  -> ITSM / Notification / Controlled Automation
  -> External Result
  -> Fresh Monitoring Revalidation
  -> Recovery / Escalation / Rollback
```

Platform monitoring truth remains authoritative. External workflow and automation systems extend Platform; they do not redefine monitored health.

---

## 2. Architecture challenge decisions

The independent recommendation was accepted only in part.

### 2.1 Accepted

The following are architecture-valid and become normative in this master:

- normalized ITSM lifecycle modes;
- connector capability and version-certification manifests;
- safe payload mapping/preview/test with connector-specific data-boundary policy;
- typed external-object binding;
- correlation-driven duplicate control;
- same-object recovery semantics and explicit reopen policy;
- explicit field/state authority for bidirectional synchronization;
- race-safe lifecycle ordering and remote-create ambiguity handling;
- CMDB/CI reconciliation without per-alert global search;
- policy-driven multi-ITSM routing;
- RCA epistemic state in external payloads;
- generic automation-executor contract;
- action risk/blast-radius classification;
- external execution success separated from monitored recovery;
- conditional rollback;
- external Agent-fleet/configuration-management interoperability;
- declarative Monitoring-as-Code through Platform APIs;
- inbound change intelligence from automation/IaC/NetOps sources;
- bounded durable queues, rate limiting, circuit breaking and self-observability;
- connector egress controls and bounded payloads;
- explicit integration/automation negative, security, race and resource tests.

### 2.2 Modified

The following recommendations are accepted only with stronger constraints:

1. **“Integration & Automation Fabric” is a logical domain, not a new mandatory microservice.** Extend the existing modular Integration Service, NCM/remediation modules and Capability Pack / Integration Pack architecture. A separate process boundary requires measured scale, security or lifecycle evidence.

2. **Do not build twenty independent day-one ITSM adapters.** Preserve the currently governed connector targets and Generic REST/Webhook. Additional vendors are Integration Pack certification candidates and must be prioritized by customer demand, API viability and support cost.

3. **Do not use root cause as the default external deduplication identity.** RCA may move from `PENDING` to `PROBABLE` to `DETERMINED` or `INCONCLUSIVE`. Default external incident identity is the stable Platform incident.

4. **Do not use unkeyed field hashing as a data-boundary control.** Low-entropy values can be dictionary-recovered. Where stable pseudonyms are required, use tenant-scoped keyed HMAC/tokenization; otherwise mask or drop.

5. **Do not implement Terraform, OpenTofu, Pulumi and Crossplane independently on day one.** First expose stable versioned APIs/declarative contracts; add an official Terraform provider after API stability. OpenTofu should consume the compatible provider contract where feasible. Pulumi/Crossplane integrations are later demand-driven adapters.

6. **Do not make Cribl, NetBox, Nornir, NAPALM, Ansible, Salt, Puppet, Chef, StackStorm or Rundeck mandatory Platform runtime dependencies.** They may integrate through governed external connectors/executors. Nornir/NAPALM may also remain approved internal NCM implementation libraries under existing architecture.

### 2.3 Rejected

The following are explicitly rejected:

- one new process/service per connector;
- one universal object model that erases distinctions between alert, incident, request, change, CI and automation job;
- arbitrary Python/JavaScript in customer mappings or policies;
- direct threshold-to-arbitrary-command execution;
- treating external automation success as service recovery;
- treating a remote ticket close as monitored recovery;
- automatic CI creation or global CMDB search on every alert;
- full native SDK/provider duplication across Terraform/OpenTofu/Pulumi/Crossplane;
- making NetBox authoritative Platform IPAM/CMDB by implication;
- making Cribl mandatory for telemetry flow;
- unrestricted autonomous destructive remediation.

---

## 3. Non-negotiable principles

1. **Monitoring truth is Platform-owned.** External workflow state cannot mark an unhealthy monitored object healthy.
2. **External failure is isolated.** ITSM, automation, CMDB, Cribl, NetBox or other external outage must not block monitoring, telemetry, qualification, incident creation or RCA.
3. **Evidence state is preserved.** RCA status, confidence, supporting evidence, contradictions and missing evidence remain distinguishable in external payloads.
4. **Correlation controls noise.** Child alerts remain inspectable while one correlated Platform incident normally maps to one remote incident.
5. **Actions are governed.** Authorization, tenant/resource scope, preconditions, risk/blast-radius policy, approval, audit, idempotency and validation precede privileged execution.
6. **Execution is not recovery.** Recovery is proved by fresh monitoring evidence and qualification.
7. **Customer flexibility is bounded.** Mappings and policies use safe declarative schemas/CEL/template primitives with cost and size limits.
8. **Connector behavior is versioned.** “Supports Vendor X” is insufficient without product/API/version/authentication/operation/limitation certification metadata.
9. **Capabilities are explicit.** UI/API expose only operations the selected connector instance/version supports.
10. **Unused integrations remain cheap.** Idle connector functionality must not materially change the certified Core baseline.

---

## 4. Formal product requirements

| PRD ID | Requirement |
|---|---|
| `PRD-IAX-001` | Platform shall support customer-selectable ITSM delivery modes while defaulting incident-oriented ITSM integrations to one remote incident per qualified/correlated Platform incident. |
| `PRD-IAX-002` | Every connector shall expose a versioned capability manifest and certification record covering product/deployment/API/authentication/operations/limitations. |
| `PRD-IAX-003` | Platform shall provide safe payload mapping, preview, validation and lifecycle-specific test execution without arbitrary executable scripting. |
| `PRD-IAX-004` | Platform shall persist tenant-scoped typed external-object bindings instead of relying on a single opaque `itsm_ref`. |
| `PRD-IAX-005` | Correlation, recovery and reopen policy shall prevent symptom ticket storms and shall normally update the same remote object through recovery. |
| `PRD-IAX-006` | Bidirectional synchronization shall define field authority and loop/conflict prevention; remote close shall never directly change Platform monitoring health. |
| `PRD-IAX-007` | Integration lifecycle processing shall be ordered, idempotent and safe when create/update/recovery operations race or remote responses are ambiguous/lost. |
| `PRD-IAX-008` | Platform shall support cached/reconciled CI/service mappings and policy-driven routing to one or more external systems without global remote lookup on every alert. |
| `PRD-IAX-009` | External payloads containing RCA shall preserve RCA status, confidence, evidence, contradictions and missing evidence; `INCONCLUSIVE` shall remain explicit. |
| `PRD-IAX-010` | External automation shall use a normalized executor contract with external job identity, status/cancel/output/health capabilities and connector-specific extensions. |
| `PRD-IAX-011` | Every privileged action shall have a risk/blast-radius classification and corresponding authorization/approval policy. |
| `PRD-IAX-012` | External execution success shall not resolve an incident until fresh monitoring revalidation satisfies recovery qualification; rollback shall only be claimed where supported and verified. |
| `PRD-IAX-013` | Platform Agent/package lifecycle shall be consumable by external configuration-management systems through signed packages, silent-install/enrollment inputs and version/verification APIs without bundling those systems into Platform runtime. |
| `PRD-IAX-014` | Monitoring-as-Code shall be driven by versioned public APIs/declarative desired-state contracts with validation, diff, approval, audit and post-apply verification. |
| `PRD-IAX-015` | Automation, IaC, NetOps and ITSM systems may emit normalized change facts into Operations Timeline / RCA with provenance and without temporal-proximity-equals-causation assumptions. |
| `PRD-IAX-016` | Platform shall support governed NetOps/telemetry ecosystem interoperability while keeping Platform identity, monitoring truth, IPAM/CMDB governance and data-boundary authority internal. |
| `PRD-IAX-017` | Integration delivery shall use bounded durable asynchronous queues with PostgreSQL-authoritative lifecycle/outbox state for control-plane delivery, connector-specific rate limits/backoff/circuit breaking, restart-safe claim/retry/reconciliation semantics and first-class health/lag/dead-letter telemetry. Kafka may signal committed work but is not the sole external-delivery source of truth. |
| `PRD-IAX-018` | Connector credentials, inbound webhooks, mappings, payloads and automation execution shall satisfy tenant isolation, least privilege, replay protection, schema validation, egress minimization and audit requirements. |
| `PRD-IAX-019` | Integration/automation resource use and lifecycle correctness shall be release-gated by race, failure, security, tenant-isolation and performance tests. |
| `PRD-IAX-020` | A certified standard ITSM integration shall be configurable and supportable through Platform UI/API without source-code changes, direct database edits, hidden server-side file edits or customer-written executable code in the normal supported path. |
| `PRD-IAX-021` | Platform shall provide a guided connector setup and readiness workflow that validates endpoint reachability, DNS/TLS/proxy/mTLS, authentication, API/product version, required permissions/scopes, rate limits, webhook prerequisites and connector capabilities before production enablement. |
| `PRD-IAX-022` | Certified connectors shall provide versioned vendor quick-start templates and metadata discovery for remote fields, required fields, choice/state values, priorities, assignment groups/queues, CI/service identifiers and supported custom-field semantics where the remote API permits it. |
| `PRD-IAX-023` | Platform shall provide a mapping assistant that detects missing required fields, incompatible types, invalid remote choices, unmapped mandatory lifecycle states and unsafe/oversized payloads, and shall preview mappings against representative real Platform objects before activation. |
| `PRD-IAX-024` | Production enablement shall use a connector Readiness Gate that exercises all operations required by the configured delivery mode, including bidirectional/webhook paths when enabled, and returns an actionable pass/fail report with remediation rather than a generic connectivity result. |
| `PRD-IAX-025` | Connector configuration shall support safe clone, version, export/import and controlled environment promotion workflows with semantic diff, validation and rollback; secrets shall never be exported in plaintext and must be rebound through governed Credential Profiles. |
| `PRD-IAX-026` | Platform shall provide implementation-grade diagnostics with stable error codes, stage-specific evidence, sanitized remote responses, correlation IDs, permission/scope diagnosis and explicit recommended remediation; authorized support evidence shall preserve deeper technical detail without exposing secrets. |
| `PRD-IAX-027` | Platform shall provide proactive connector lifecycle management for remote API/product-version drift, expiring credentials/certificates, deprecated capabilities and connector-pack upgrades, including compatibility checks and revalidation before unsafe activation. |
| `PRD-IAX-028` | Platform shall provide operational self-service for integrations: health, queue/backlog, webhook health, rate-limit state, circuit state, last success, failed-item inspection, bounded retry/replay, pause/drain/disable, dead-letter handling and audit, all subject to RBAC. |
| `PRD-IAX-029` | ITSM implementation usability shall be a release-certification dimension measured with reproducible first-time and experienced-implementer workflows; Platform shall track setup-step completion/failure evidence and shall not make a “world-class/easy integration” claim without benchmark evidence. |

---

## 5. ITSM delivery modes

Normalized customer-visible modes are:

```text
DISABLED

EVENT_ONLY
Qualified Platform alert/event is sent to an external event/alert surface.

INCIDENT_ONLY
A correlated Platform incident creates/updates one remote incident/ticket.

EVENT_THEN_INCIDENT
A qualified event is sent first; if Platform forms a correlated incident, the remote incident is created/linked and subsequent incident lifecycle follows the Platform incident.

DUAL_LINKED
Maintain both the remote event/alert and a linked remote incident where the connector supports distinct object classes.

POLICY
Bounded policy selects one of the supported behaviors based on approved scope/fields.
```

Default for ITSM incident connectors is `INCIDENT_ONLY`.

`EVENT_ONLY` remains available for customers whose operating model explicitly requires “Platform alert -> external alert/event”.

`POLICY` may use safe expressions over allowlisted Platform fields; it may not execute code or bypass connector/data-boundary policy.

---

## 6. Connector strategy, capabilities and certification

### 6.1 Connector-pack strategy

Capability Pack / Integration Pack remains the only extensibility architecture.

The existing governed architecture targets remain:

- ServiceNow;
- Jira Service Management;
- BMC Helix/Remedy;
- ManageEngine ServiceDesk Plus;
- Freshservice/Freshworks;
- SummitAI / SymphonyAI Summit naming mapped by connector certification;
- Generic REST/Webhook.

The following are **candidate Integration Pack certification backlog**, not day-one product commitments merely because they appeared in an audit:

- Ivanti Neurons for ITSM;
- OpenText SMAX;
- SolarWinds Service Desk;
- HaloITSM;
- SysAid;
- TOPdesk;
- Matrix42 ESM;
- Xurrent / 4me;
- Hornbill Service Management;
- IFS assyst;
- TeamDynamix;
- EasyVista Service Manager;
- InvGate Service Management;
- Broadcom CA Service Desk Manager.

Adding a vendor to the candidate list does not mean implemented, tested, certified, supported or commercially committed.

### 6.2 Capability manifest

Each connector/version declares typed capability groups, for example:

```text
authentication:
  BASIC | OAUTH2 | API_TOKEN | MTLS | VENDOR_EXTENSION

objects:
  EVENT | ALERT | INCIDENT | REQUEST | PROBLEM | CHANGE | CI | SERVICE | AUTOMATION_JOB

outbound:
  CREATE | UPDATE | RESOLVE | CLOSE | REOPEN | COMMENT | WORKNOTE | ATTACHMENT

inbound:
  WEBHOOK | POLL | STATE | ASSIGNMENT | COMMENT | WORKNOTE | CHANGE

metadata:
  CUSTOM_FIELDS
  CI_LOOKUP
  PAGINATION
  RATE_LIMIT
  REMOTE_IDEMPOTENCY
  ETAG_OR_VERSION
  ATTACHMENT_LIMITS
  MAX_PAYLOAD
```

Vendor-specific semantics are represented through versioned extension capabilities; common contracts must not erase vendor-specific behavior.

### 6.3 Certification record

For every certified connector release record:

```text
vendor
product
deployment_mode       # cloud / on-prem / both
supported_product_versions
supported_api_versions
authentication_modes
supported_operations
known_limitations
certified_at
connector_pack_version
test_evidence_ref
deprecation_state
```

Unsupported capabilities degrade visibly and cannot appear enabled in UI/API.

---

## 7. Payload mapping, transformation and sandbox

### 7.1 Lifecycle templates

Mappings may be defined separately for:

```text
CREATE
UPDATE
SEVERITY_CHANGE
CORRELATION_UPDATE
RCA_UPDATE
RECOVERY
RESOLVE
CLOSE
REOPEN
COMMENT
WORKNOTE
```

Available source fields are typed, authorization-filtered Platform fields from entity, alert, incident, RCA, change, recovery and action domains.

### 7.2 Transformation safety

Allowed mechanisms are bounded template primitives, schema mappings and approved CEL expressions/functions.

Prohibited:

- arbitrary JavaScript/Python;
- shell/SQL execution;
- filesystem/network access from mappings;
- secret reveal functions;
- cross-tenant lookups;
- unbounded recursion/loops;
- dynamic code loading.

### 7.3 Data-boundary actions

Per connector/field policy supports:

```text
ALLOW
MASK
DROP
TOKENIZE_HMAC
```

`TOKENIZE_HMAC` uses tenant-scoped governed keys where stable pseudonyms are required. Plain unkeyed hashing is not a sufficient protection for low-entropy values.

### 7.4 Preview/test

Before production enablement:

```text
Platform object
 -> mapping
 -> transformation
 -> authorization/data-boundary filtering
 -> exact final payload preview
 -> connector schema/size validation
 -> TEST
 -> sanitized remote response
```

The sandbox tests the lifecycle operations required by the selected mode, not only network connectivity.

No test may create uncontrolled production objects; connector-specific test-object cleanup/marking is required where remote APIs permit it.

---

## 8. External-object binding and synchronization model

### 8.1 Binding

Replace single `itsm_ref` as the long-term model with:

```text
external_object_binding
  binding_id
  tenant_id

  connector_instance_id

  platform_object_type
  platform_object_id

  external_object_type
  external_object_id
  external_object_number?
  external_url?

  correlation_key
  remote_dedup_key?

  normalized_remote_state?
  remote_state_raw?
  remote_version?

  lifecycle_version
  sync_status

  created_at
  updated_at
  last_outbound_sync?
  last_inbound_sync?
  last_result?
  last_error_code?
```

Platform authoritative state is referenced from the Platform object; it is not redundantly copied into the binding as a second source of truth.

Use typed child tables only where domain-specific integrity requires fields that do not belong in the generic binding. Connector-specific opaque metadata must be bounded/versioned and must not replace typed core identity/state.

### 8.2 Unique/consistency constraints

At minimum enforce tenant + connector instance + platform object identity uniqueness according to the configured delivery mode.

External IDs are unique within the connector instance/object type where the remote system guarantees that property.

All reads/writes include tenant scope.

### 8.3 Field authority

Default authority:

| Field family | Authority |
|---|---|
| Monitoring condition / telemetry / recovery evidence | Platform |
| Alert qualification / incident correlation / RCA evidence | Platform |
| Remote ticket/object number and immutable remote identifiers | External system |
| Assignment group / assignee | Connector policy: external or controlled shared |
| Comments / work notes | Bidirectional where supported, origin-tagged |
| Remote workflow state | External/shared mapping policy |
| Platform incident state | Platform; may react to remote workflow policy but never to override monitored health |

Every synchronized mutation carries origin/version metadata so echo updates do not loop.

### 8.4 External close conflict

If the external ticket is closed while Platform condition remains active:

```text
Platform condition = ACTIVE
external binding = CLOSED_EXTERNALLY / CONFLICT
```

Policy may:

- respect the remote close without changing health;
- add warning/work note;
- reopen the same object;
- create a new linked object;
- raise a synchronization conflict.

---

## 9. Correlation, recovery and reopen semantics

### 9.1 External duplicate-control modes

Supported initial modes:

```text
PER_PLATFORM_INCIDENT        # default for incident mode
PER_QUALIFIED_ALERT          # event/alert workflows only
PER_ENTITY_CONDITION
CUSTOM_SAFE_KEY
```

`CUSTOM_SAFE_KEY` is a bounded CEL-derived key over allowlisted fields with size/cardinality controls.

`PER_ROOT_CAUSE` is not an initial authoritative binding mode because RCA may change as evidence arrives. A determined stable RCA may influence routing/update policy without replacing the Platform incident identity.

Service-level grouping should normally be produced by Platform incident correlation rather than an independent external deduplication algorithm.

### 9.2 Same-object recovery invariant

Normal recovery updates the existing binding:

```text
Platform incident OPEN
 -> Remote Incident #123
 -> Qualified recovery
 -> update/resolve/close Remote Incident #123
```

A separate “recovered” ticket is prohibited unless an explicit customer workflow requires it.

Recovery actions:

```text
PLATFORM_ONLY
ADD_RECOVERY_NOTE
UPDATE_REMOTE_STATE
RESOLVE_REMOTE
CLOSE_REMOTE
```

subject to connector capability.

### 9.3 Reopen

If the condition returns, policy selects:

- reopen the existing remote object when supported and inside the reopen window; or
- create a new linked object referencing the prior binding.

Flapping/hysteresis and existing Platform recovery qualification remain authoritative.

---

## 10. Race-safe lifecycle and ambiguous remote create

### 10.1 Internal delivery states

Customer UI may simplify these, but persistence must distinguish at least:

```text
QUEUED
CREATE_PENDING
CREATE_AMBIGUOUS
CREATE_CONFIRMED
UPDATE_PENDING
RECOVERY_PENDING
RESOLVE_PENDING
RESOLVED
REOPEN_PENDING
FAILED_RETRYABLE
FAILED_TERMINAL
DEAD_LETTER
CANCELLED
```

Operations for one binding are ordered by monotonically increasing `lifecycle_version`.

A later recovery cannot be lost because an earlier remote CREATE completed late.

### 10.2 Remote-create ambiguity

If a remote CREATE may have succeeded but the response is lost:

1. use remote idempotency header/key when supported;
2. otherwise query by a dedicated Platform correlation/custom field when supported;
3. otherwise perform bounded safe reconciliation/lookup before recreate;
4. only recreate when connector policy can establish that no prior object exists.

Blind CREATE retry is prohibited when duplicate risk is unresolved.

---

## 11. CI/CMDB mapping and multi-system routing

### 11.1 CI/service mapping

Maintain reconciled mappings:

```text
Platform entity  <-> remote CI
Platform service <-> remote service/business-service object
```

Mappings retain stable remote IDs, provenance, last reconciliation and status.

Do not issue broad global CMDB searches per alert/incident.

If no mapping exists, customer policy may:

- create ticket without CI;
- perform bounded fallback lookup;
- queue a reconciliation task;
- reject/hold external creation with visible reason.

Automatic CI creation is a separate governed capability and is not implied.

### 11.2 Routing

Routing may use:

- tenant;
- region/site/business unit;
- entity/group/technology;
- severity;
- business service;
- support team;
- alert/incident class.

A Platform incident may hold multiple external bindings only where an explicit routing policy requires it. Each binding remains independently auditable and data-boundary controlled.

---

## 12. RCA payload contract

If RCA data is sent externally, the minimum semantic contract is:

```text
rca_status          # PENDING | PROBABLE | DETERMINED | INCONCLUSIVE
root_candidate?
confidence?
supporting_evidence_summary
contradictions
missing_evidence
causal_chain_summary?
blast_radius?
recommended_next_validation_or_action?
```

A `PROBABLE` candidate must never be serialized as a determined fact.

If `INCONCLUSIVE`, external payloads say so explicitly.

Large/raw evidence remains in Platform; external payloads use bounded operational summaries plus deep links and controlled attachments.

---

## 13. Automation executor architecture

### 13.1 Executor contract

External automation systems use a common executor contract:

```text
testConnection
discoverCapabilities
execute
readStatus
cancel
fetchOutput
health
```

Connector-specific extensions may add capabilities without changing the core contract.

Candidate external executor packs include Ansible Automation Platform/Event-Driven Ansible, StackStorm, PagerDuty Process Automation/Rundeck and ServiceNow Flow/ITOM where customer demand/API capability justify certification. This list is not a day-one support commitment.

### 13.2 Execution record

Canonical execution state extends the existing remediation model:

```text
action_execution
  execution_id
  tenant_id
  action_id
  action_version

  trigger_source
  alert_ids?
  incident_id?

  executor_type
  connector_instance_id?
  external_job_id?

  targets
  preconditions

  risk_class
  blast_radius

  requested_by
  approval_policy
  approval_state

  requested_at
  started_at?
  completed_at?

  execution_status
  execution_result_code?

  validation_policy
  validation_status

  rollback_available
  rollback_status?

  evidence_refs
  correlation_id
  audit_id
```

### 13.3 Risk classes

Use a five-level product risk model:

| Class | Meaning | Default |
|---|---|---|
| `R0_DIAGNOSTIC` | Read-only/diagnostic, no intended state mutation | May auto-run when authorized policy allows |
| `R1_LOW_REVERSIBLE` | Bounded low-risk reversible Platform/service action | Policy-controlled auto-run permitted |
| `R2_OPERATIONAL_CHANGE` | Material application/operational state change | Strong policy; approval normally required |
| `R3_INFRASTRUCTURE_CHANGE` | Network/firewall/routing/IaC/cluster or broad infrastructure mutation | Approval required by default |
| `R4_DESTRUCTIVE` | Destructive/high-blast-radius action | Disabled by default; separate explicit engineering/authorization required |

Examples are guidance only; the action definition declares the risk class and blast-radius policy.

### 13.4 Recovery validation

```text
external execution SUCCESS
  != incident RESOLVED
```

External success means `ACTION_EXECUTION_SUCCESS`.

Platform then requests fresh polling/telemetry/synthetic evidence. Only recovery qualification may set the monitored condition/incident to recovered/resolved.

If the executor reports success but monitoring still fails:

```text
execution = SUCCEEDED
remediation_effect = FAILED_TO_RESTORE
incident = ACTIVE / ESCALATED
```

### 13.5 Rollback

Rollback is exposed only if the action/executor has a verified rollback definition.

Where available:

```text
capture/pre-state
 -> action
 -> monitoring validation failure
 -> rollback
 -> post-rollback monitoring validation
 -> recovered or escalated
```

No UI/API may imply rollback capability that the underlying action/executor cannot safely provide.

---

## 14. Agent fleet and configuration-management interoperability

Platform supplies:

- signed Core/Sensor/Agent packages;
- checksums/signatures/provenance;
- silent/non-interactive install parameters;
- bounded enrollment/bootstrap inputs;
- desired-version/status APIs;
- installation/upgrade verification APIs;
- certificate rotation/re-enrollment workflows.

Ansible, SaltStack, Puppet, Chef and other customer configuration-management systems may consume these interfaces.

Platform does **not** embed those products as mandatory runtime components.

Dedicated vendor modules/collections may be certified later where they materially improve customer adoption; generic package/API automation remains sufficient architectural interoperability.

---

## 15. Monitoring-as-Code

### 15.1 Canonical path

Versioned Platform REST/OpenAPI and declarative desired-state contracts are authoritative.

Candidate managed resources include tenant-scoped:

- site/group/entity/discovery;
- monitoring profile;
- threshold/alert policy;
- ITSM route/integration;
- maintenance;
- dashboard/report definitions where safe;
- Sensor assignment policy;
- synthetic monitor.

Apply path:

```text
declaration
 -> schema validation
 -> tenant/RBAC
 -> semantic validation
 -> current-vs-desired diff
 -> conflict/safety checks
 -> approval when required
 -> bounded apply
 -> audit
 -> post-apply readback/verification
```

### 15.2 Provider strategy

1. Stabilize public API/declarative resource contracts first.
2. Add an official Terraform provider only after API/resource lifecycle semantics are stable enough to avoid provider churn.
3. Support OpenTofu through compatible provider protocol/artifacts where technically valid.
4. Pulumi and Crossplane remain later integrations using the same public APIs/provider foundations when justified.

Do not build four independent control planes.

---

## 16. IaC, NetOps and telemetry ecosystem

### 16.1 Change intelligence

Automation/IaC sources may emit:

```text
change_id
source
external_ref
actor_or_service
target_entities/services
change_type
before_after_ref?
version_or_commit?
start_time
end_time?
result
ticket_or_change_ref?
evidence_link?
provenance
```

This maps to the existing Platform canonical change fact and feeds Operations Timeline / RCA.

Temporal proximity is evidence, not causal proof.

### 16.2 NetOps

- Nornir/NAPALM may remain internal NCM implementation primitives where already approved.
- Customer-owned Nornir/NAPALM automation integrates through the external executor/API contract; no duplicate NetOps control plane is created.
- NetBox may integrate through an Integration Pack for inventory/IPAM/change/event exchange.
- NetBox is not authoritative Platform IPAM/CMDB unless a future explicit architecture decision changes authority.

### 16.3 OpenTelemetry and Cribl

- OTLP ingestion into Platform remains first-class existing architecture.
- Platform-to-OTel export is optional and must be justified by customer workflow; it uses an explicit export connector/data-boundary contract rather than becoming a second internal event backbone.
- Cribl interoperability is an optional Integration Pack candidate for inbound/outbound governed telemetry routing.
- Cribl is never a mandatory dependency or source of Platform monitoring truth.

---

## 17. Queueing, health, rate and failure isolation

### 17.1 Delivery

Outbound integration/automation delivery is durable, asynchronous and bounded.

Connector failure does not block:

- collection;
- ingestion;
- alert qualification;
- incident correlation/creation;
- RCA;
- Platform recovery evaluation.

### 17.2 Health states

Connector instance health exposes:

```text
HEALTHY
DEGRADED
RATE_LIMITED
AUTH_FAILED
QUEUE_BACKED_UP
CIRCUIT_OPEN
DISABLED
FAILED
```

### 17.3 Metrics

At minimum:

```text
queue_depth
oldest_item_age
success_rate
failure_rate
retry_rate
rate_limit_events
remote_latency
last_success_at
dead_letter_count
circuit_state
inflight_count
payload_rejected_count
```

### 17.4 Retry/circuit policy

Use classified retryability, bounded exponential backoff with jitter, `Retry-After` when valid, connector-specific concurrency/rate budgets and circuit breaking.

Authentication/authorization/schema failures are not blindly retried as transient failures.


### 17.5 Durable state ownership and delivery boundary

Integration control/lifecycle state is **PostgreSQL-authoritative**. At minimum, connector instances/configuration versions, typed external-object bindings, lifecycle versions, outbound delivery intents, attempt state, dead-letter state, reconciliation tasks and replay-job metadata are durably persisted there. Valkey may accelerate reconstructable lookup/rate state but is never the only copy of a pending external operation.

For incident/ticket/automation lifecycle delivery, creation of an authoritative Platform state transition and its outbound integration intent shall use a **transactional outbox or equivalent single-PostgreSQL atomic boundary**. A Core crash after committing the Platform state but before remote delivery must leave a durable delivery intent that a restarted worker can reclaim. A Core crash before that transaction commits must not expose a phantom queued operation.

Workers claim bounded work with concurrency-safe leasing/locking, persist attempt/result state, and only mark an item delivered when the connector contract proves the required remote acceptance boundary. HTTP/TLS write success alone is not remote-object confirmation when the response is ambiguous. Ambiguous create/update outcomes enter reconciliation rather than blind duplicate creation.

Kafka may carry internal notifications/wakeups derived from committed integration state, but it is **not the sole source of truth for external-object binding or lifecycle delivery state**. ClickHouse may retain high-volume historical delivery/health/audit analytics after the authoritative state transition; it is not the control queue. High-rate telemetry/log forwarding uses its separately governed streaming/queue contracts and must not overload the transactional ITSM/control outbox.

Dead-letter and replay are state transitions over the original durable delivery identity. Replay creates a new bounded attempt/replay job while preserving the original binding, lifecycle version, actor/reason, provenance and audit linkage. Deleting or expiring queue records before their terminal/audited disposition is prohibited.

The externally visible success boundary is explicit per connector capability:

```text
PLATFORM_STATE_COMMITTED
 -> DELIVERY_INTENT_DURABLE
 -> WORK_CLAIMED
 -> REMOTE_REQUEST_SENT
 -> REMOTE_ACCEPTANCE_CONFIRMED | AMBIGUOUS | RETRYABLE_FAILURE | TERMINAL_FAILURE
 -> BINDING/DELIVERY_STATE_COMMITTED
```

No UI/API may report an external object as created/updated merely because the request was queued or a socket/HTTP write completed.

---

## 18. Security and egress controls

Connector credentials:

- encrypted governed secret storage;
- tenant/resource scoped use;
- least privilege;
- rotation/expiry metadata;
- no ordinary browser plaintext;
- no logging/support-bundle leakage;
- full audit of use/config changes.

Inbound webhooks:

- authentication/signature validation;
- tenant/source binding;
- timestamp/freshness validation;
- nonce/event-ID replay defense where supported;
- schema and content-size validation;
- bounded rate;
- provenance/correlation ID;
- no trust in caller-supplied tenant identity without server-side binding.

Automation additionally requires:

- action allowlist/version;
- target scope;
- preconditions;
- risk/blast-radius policy;
- approval;
- idempotency;
- timeout/cancel behavior;
- immutable audit;
- result/output redaction;
- post-action monitoring validation.

---

## 19. Large-payload and evidence policy

Remote workflow systems receive the minimum operationally useful payload:

- concise incident/alert summary;
- current severity and state;
- affected scope;
- RCA status/confidence and critical evidence;
- contradictions/missing evidence summary;
- recommended next action/validation;
- Platform deep links.

Raw telemetry, logs, packet payload, large configuration snapshots, full RCA graph and unrestricted automation output remain in Platform unless an authorized bounded attachment policy explicitly permits transfer.

Every connector has maximum payload/attachment limits and deterministic oversize behavior.

---

## 20. UI/UX requirements

### 20.1 ITSM implementation experience contract

For certified standard connectors, the supported implementation path must be **no-code/low-friction by default**:

- no Platform source-code change;
- no direct PostgreSQL/ClickHouse manipulation;
- no undocumented environment-variable or server-file editing;
- no requirement for customer-written JavaScript/Python;
- no vendor-specific tribal knowledge that Platform could have discovered or validated itself;
- no production activation before the required readiness checks are visible.

A connector setup wizard shall use progressive disclosure and retain a resumable draft. The recommended flow is:

```text
Select Vendor/Product
 -> Select Cloud / On-Prem + Product/API Version
 -> Endpoint / Proxy / Trust / mTLS
 -> Credential Profile / OAuth Authorization
 -> Permission & Scope Preflight
 -> Discover Capabilities / Remote Metadata
 -> Choose Delivery Mode
 -> Routing / Correlation Policy
 -> CI / Service Mapping
 -> Field / State / Priority Mapping
 -> Data-Boundary Policy
 -> Exact Payload Preview
 -> End-to-End Lifecycle Tests
 -> Readiness Report
 -> Activate
 -> Observe
```

Every step shows `PASS`, `WARNING`, `FAIL`, or `NOT_APPLICABLE`, with the exact reason and remediation path.

### 20.2 Connection and permission preflight

Before mapping work begins, Platform should automatically validate as applicable:

- DNS resolution and selected egress path/Sensor/Core placement;
- TCP/HTTPS reachability;
- proxy configuration;
- TLS chain, hostname, expiry and customer CA trust;
- mTLS client certificate validity;
- clock/skew issues relevant to signed requests;
- OAuth/token/API-key authentication;
- remote product/API version;
- required API permissions/scopes/roles;
- create/update/comment/resolve/reopen/read permissions for the configured mode;
- webhook callback reachability/signature requirements where applicable;
- remote rate-limit/quota information where exposed.

The user receives an **Implementation Readiness Report** suitable for sharing with ITSM/customer administrators. It lists required endpoints, ports/proxy path, certificate/trust requirements, authentication method, scopes/roles, webhook callback requirements, tested capabilities, known limitations and failures. Secrets are excluded.

### 20.3 Vendor quick starts and remote metadata discovery

Certified Integration Packs should ship versioned vendor/product quick-start profiles covering safe defaults for supported object classes and workflows.

Where APIs permit, Platform discovers and caches remote metadata including:

- required/custom fields and data types;
- choice lists/enumerations;
- workflow/state values and legal transitions;
- priority/urgency/impact values;
- assignment groups/queues;
- CI/service classes and identifiers;
- attachment/payload limits;
- supported API capabilities.

Defaults accelerate setup but never silently override existing customer customization. Discovered metadata carries fetch time, product/API version and freshness.

### 20.4 Mapping assistant

The mapping experience must support:

- searchable Platform and remote fields;
- type-aware drag/select mapping;
- lifecycle-specific mappings;
- required-field completeness;
- state/priority enumeration mapping;
- conditional mappings through bounded safe expressions;
- defaults/fallbacks with explicit precedence;
- representative real Platform object preview;
- exact post-egress-filter payload preview;
- validation of length, enum, required, nullability and remote schema constraints;
- detection of fields removed or changed after remote-version drift.

AI may suggest mappings only as advisory assistance. Suggestions require deterministic validation and user confirmation and cannot access/reveal connector secrets.

### 20.5 End-to-end Readiness Gate

The connector cannot be marked `PRODUCTION_READY` until the operations required by the selected delivery mode pass or an explicitly authorized exception is recorded.

The gate tests, where applicable:

```text
AUTHENTICATE
DISCOVER_METADATA
CREATE
READ_BACK / FIND_BY_CORRELATION_KEY
UPDATE
COMMENT / WORKNOTE
SEVERITY_OR_STATE_UPDATE
CI / SERVICE LOOKUP
RECOVERY
RESOLVE / CLOSE
REOPEN
INBOUND WEBHOOK / POLLING
DUPLICATE / IDEMPOTENCY BEHAVIOR
CLEANUP TEST OBJECT
```

Test artifacts are clearly marked as Platform test objects when the remote system supports it and are cleaned up or linked for administrator cleanup. Platform shall never generate uncontrolled production ticket spam during testing.

The final report answers:

- what passed;
- what failed;
- exact failing stage;
- expected versus observed behavior;
- relevant sanitized remote status/error;
- required customer-side remediation;
- whether production activation is safe.

### 20.6 Configuration portability and environment promotion

Connector instances, mappings, policies and routes are versioned configuration objects.

Platform supports:

- clone connector configuration;
- export/import a redacted, versioned configuration bundle;
- semantic diff before import/apply;
- validation against the destination connector/product/API version;
- explicit Credential Profile rebinding;
- controlled Dev/Test/UAT -> Production promotion where customers use separate environments;
- rollback to the previous valid configuration version.

Plaintext credentials, tokens, private keys and secret values are never exported.

### 20.7 Troubleshooting and support experience

Implementation teams must not need packet captures, database queries or source-code access for routine integration failures.

For each failed stage Platform presents:

```text
stable_error_code
human_summary
failing_stage
connector_instance
remote_endpoint
remote_status_code?
sanitized_remote_error?
correlation_id
last_success_at?
retryability
likely_cause
recommended_fix
test_again_action
support_evidence_ref
```

Authorized support users may access deeper sanitized protocol/request/response evidence subject to RBAC and data-boundary policy. Secrets remain redacted.

A one-click support bundle for a connector contains version/capability metadata, health history, readiness results, recent sanitized errors, queue statistics, configuration checksums and relevant audit references — not credentials or unrestricted customer payloads.

### 20.8 Lifecycle and upgrade assistance

Platform proactively warns about:

- expiring OAuth/token/certificate credentials where expiry is knowable;
- remote API deprecation/version incompatibility;
- connector-pack version drift;
- remote metadata/schema drift;
- newly required fields or permissions;
- revoked webhook registration;
- sustained rate limiting or quota exhaustion.

Connector upgrades run compatibility/preflight tests against the existing instance before activation. Breaking mapping/capability changes require explicit review.

### 20.9 Operational self-service

Integration administrators can, subject to RBAC:

- test connection/readiness;
- inspect remote binding;
- inspect/retry a failed item;
- replay a bounded dead-letter selection;
- pause new delivery;
- drain existing queue;
- disable connector;
- rotate/rebind credentials;
- refresh remote metadata;
- compare configuration versions;
- rollback connector configuration;
- re-run certification/readiness;
- generate a support bundle.

Bulk retry/replay always shows scope/count/blast-radius and requires confirmation/policy.

### 20.10 Usability and implementation-quality certification

ITSM integration usability is a release-certification dimension, not an informal UX aspiration.

Certification includes reproducible scenarios for:

1. a first-time implementation engineer configuring a certified connector from documentation/UI only;
2. an experienced engineer configuring the same connector;
3. a wrong endpoint;
4. TLS/customer-CA failure;
5. bad credentials;
6. insufficient remote permissions;
7. required custom field missing;
8. invalid state/priority mapping;
9. webhook signature/reachability failure;
10. remote rate limiting;
11. API/version drift;
12. migration of an integration configuration between environments;
13. diagnosis and recovery from a failed queued lifecycle event.

Measure at minimum:

- setup completion rate;
- time and step count to first successful end-to-end lifecycle test;
- number of undocumented/manual interventions;
- failed-step diagnostic accuracy;
- remediation success without source/DB intervention;
- configuration promotion success;
- post-upgrade revalidation success.

Numeric release targets must be frozen from usability benchmarking and pilot evidence rather than invented in advance. A connector cannot be called “easy”, “zero-touch” or “world-class” merely because a demo succeeds.

Integrations workspace must expose:

- connector instances;
- capability/version certification;
- delivery mode;
- routing policy;
- payload mapping studio;
- exact payload preview after data-boundary filtering;
- lifecycle test sandbox;
- external-object bindings;
- queue/dead-letter/retry;
- health/rate/circuit state;
- state-authority/conflict policy;
- CI/service mappings;
- automation executors/actions/approvals/validation;
- audit.

UI renders only supported connector capabilities.

Customer-visible lifecycle state is simplified; internal delivery/race states are available in diagnostics/audit.

No UI may imply “recovered” based solely on remote workflow/automation state.

---

## 21. Performance and resource requirements

When integrations are unused:

- no one-process-per-connector model;
- no continuously polling worker per disabled connector;
- no heavyweight third-party integration engine;
- no material baseline CPU/RAM increase beyond measured tolerance.

Use shared bounded HTTP/runtime primitives, lazy/conditional workers and bounded queues/thread pools.

Benchmark at minimum:

```text
idle integration RAM
CPU per 1,000 lifecycle operations
queue storage per operation
payload transform latency
connector concurrency
retry/replay throughput
remote-timeout degradation
dead-letter growth
UI query cost for queue/binding views
```

Resource regressions participate in normal Core release certification.

---

## 22. Mandatory test/certification matrix

### 22.1 ITSM lifecycle

Test create/update/recovery/resolve/close/reopen, duplicate alert/incident, correlated child alert, manual remote close, assignment/comment/state inbound sync, connector disable/re-enable, long outage and queued replay.

### 22.2 Race/idempotency

Test:

- remote CREATE succeeds but response is lost;
- recovery arrives while CREATE is pending;
- duplicate trigger;
- out-of-order remote webhook;
- stale lifecycle version;
- replay after Core restart;
- remote object manually deleted;
- two routes bind one Platform incident intentionally.

### 22.3 Correlation/RCA

Test one device outage producing many symptoms -> intended one Platform incident/remote incident; independent faults remain separate; root-cause hypothesis changes update the same binding; `DETERMINED`, `PROBABLE`, `INCONCLUSIVE`, contradictions and missing evidence serialize correctly.

### 22.4 Automation

Test approval denied, precondition fail, executor unavailable, timeout/cancel, duplicate trigger, success, success-without-recovery, validation success/failure, rollback success/failure, unsupported rollback, blast-radius denial and emergency suspension.

### 22.5 Security/tenant isolation

Test cross-tenant binding/routing attempts, forged/replayed webhook, invalid/expired credentials, connector secret rotation/revocation, malicious template/CEL input, command/parameter injection, oversized payload, data-boundary enforcement and output redaction.

### 22.6 Resource/failure

Test 401/403/409/429/5xx/timeouts, `Retry-After`, circuit open/recovery, queue saturation, dead-letter handling, remote outage while monitoring remains normal, and performance under certified connector workload.

### 22.7 Implementation usability and supportability

For each GA-certified ITSM connector/version, execute a clean-environment usability certification using the setup and failure scenarios in §20.10.

The test must prove that the normal supported implementation, validation, activation and routine troubleshooting path does not require:

- source changes;
- direct database edits;
- hidden configuration-file changes;
- plaintext secret handling;
- vendor-specific manual steps omitted from Platform;
- support-only backend intervention for routine failures.

The evidence bundle retains the setup path, test results, failures encountered, diagnostic output, remediation actions and final readiness report.

A connector/version is not “certified” until its declared capabilities pass the applicable contract/integration/security suite.

---

## 23. Implementation sequence

This is a dependency sequence for this capability family, not permission to bypass the overall Platform roadmap.

### P0 — foundation

1. finalize versioned connector capability/certification schema;
2. replace long-term `itsm_ref` design with external binding model and migration compatibility;
3. define delivery-mode/routing/correlation/recovery/reopen/state-authority contracts;
4. implement durable ordered delivery, idempotency, ambiguity reconciliation and connector health;
5. implement safe mapping/data-boundary/preview/test foundation;
6. implement guided setup/preflight, remote metadata discovery, quick-start templates, Readiness Gate and implementation diagnostics;
7. implement versioned configuration portability/promotion and connector lifecycle/upgrade checks;
8. implement tenant/security/audit controls and race/security/usability tests.

### P1 — first operational integrations

ITSM is not a late-stage “connector polish” item. As soon as the Platform Incident lifecycle and P0 integration contracts are stable enough to bind safely, the first certified ITSM path is an immediate P1 deliverable.

Use a reference-implementation strategy:

- **Reference-grade first:** ServiceNow + Generic REST/Webhook. These must exercise the complete connector capability, binding, mapping, readiness, diagnostics, recovery/reopen, bidirectional and supportability contracts.
- **Certified baseline next:** Jira Service Management, BMC Helix/Remedy, ManageEngine ServiceDesk Plus, Freshservice/Freshworks and SummitAI/SymphonyAI Summit, reusing the same runtime/contracts rather than separate subsystems.
- **Expansion after:** additional Integration Packs based on customer demand/API viability, only after the baseline certification mechanism proves maintainable.

9. implement/certify ServiceNow and Generic REST/Webhook as reference-grade connectors using the full implementation-experience certification;
10. certify the remaining already-governed baseline ITSM connectors against the same common runtime and tests;
11. implement CI/service mapping and bidirectional state/conflict rules;
12. implement normalized automation executor/action/risk contract;
13. integrate controlled-remediation validation/rollback with fresh monitoring evidence;
14. ingest executor/IaC/ITSM change facts.

### P2 — ecosystem expansion

15. expand ITSM/automation Integration Packs by measured customer demand;
16. expose stable Monitoring-as-Code API/declarative resources;
17. add Terraform provider after API lifecycle stability; verify OpenTofu compatibility;
18. add external NetBox/NetOps interoperability where required;
19. add optional OTel-export/Cribl packs when customer use cases justify them;
20. add Pulumi/Crossplane integrations only when they avoid duplicated control-plane logic.

---

## 24. Formal requirements traceability supplement

| Requirement ID | PRD ID | Primary contract |
|---|---|---|
| `REQ-IAX-001` | `PRD-IAX-001` | §5 ITSM delivery modes/default |
| `REQ-IAX-002` | `PRD-IAX-002` | §6 capability/version certification |
| `REQ-IAX-003` | `PRD-IAX-003` | §7 mapping/data-boundary/sandbox |
| `REQ-IAX-004` | `PRD-IAX-004` | §8 external bindings |
| `REQ-IAX-005` | `PRD-IAX-005` | §9 correlation/recovery/reopen |
| `REQ-IAX-006` | `PRD-IAX-006` | §8.3-8.4 authority/conflict |
| `REQ-IAX-007` | `PRD-IAX-007` | §10 race/idempotency/ambiguity |
| `REQ-IAX-008` | `PRD-IAX-008` | §11 CI mapping/routing |
| `REQ-IAX-009` | `PRD-IAX-009` | §12 RCA payload |
| `REQ-IAX-010` | `PRD-IAX-010` | §13 executor contract |
| `REQ-IAX-011` | `PRD-IAX-011` | §13.3 risk classes |
| `REQ-IAX-012` | `PRD-IAX-012` | §13.4-13.5 validation/rollback |
| `REQ-IAX-013` | `PRD-IAX-013` | §14 Agent/config-management interop |
| `REQ-IAX-014` | `PRD-IAX-014` | §15 Monitoring-as-Code |
| `REQ-IAX-015` | `PRD-IAX-015` | §16.1 change intelligence |
| `REQ-IAX-016` | `PRD-IAX-016` | §16.2-16.3 NetOps/telemetry |
| `REQ-IAX-017` | `PRD-IAX-017` | §17 queue/health/rate/circuit |
| `REQ-IAX-018` | `PRD-IAX-018` | §18 security/egress |
| `REQ-IAX-019` | `PRD-IAX-019` | §21-22 resource/test gates |
| `REQ-IAX-020` | `PRD-IAX-020` | §20.1 no-code supported implementation path |
| `REQ-IAX-021` | `PRD-IAX-021` | §20.2 guided setup/readiness preflight |
| `REQ-IAX-022` | `PRD-IAX-022` | §20.3 quick starts/metadata discovery |
| `REQ-IAX-023` | `PRD-IAX-023` | §20.4 mapping assistant |
| `REQ-IAX-024` | `PRD-IAX-024` | §20.5 end-to-end Readiness Gate |
| `REQ-IAX-025` | `PRD-IAX-025` | §20.6 configuration portability/promotion |
| `REQ-IAX-026` | `PRD-IAX-026` | §20.7 diagnostics/support experience |
| `REQ-IAX-027` | `PRD-IAX-027` | §20.8 lifecycle/upgrade assistance |
| `REQ-IAX-028` | `PRD-IAX-028` | §20.9 operational self-service |
| `REQ-IAX-029` | `PRD-IAX-029` | §20.10 + §22.7 usability certification |

---

## 25. Definition of done

This master is specification/change-control evidence only.

No capability in this document is implemented, production-ready or certified merely because the requirement exists.

Implementation is complete only when applicable code, migrations, compatibility, tests, tenant isolation, security, observability, resource evidence, UI, upgrade/rollback behavior, runbooks, connector certification evidence and documentation are complete.

For a certified standard ITSM connector, Definition of Done additionally requires guided setup, preflight/readiness reporting, metadata discovery where supported, safe mapping/preview, end-to-end lifecycle tests, implementation diagnostics, configuration portability, upgrade/revalidation behavior, self-service operations and usability-certification evidence. Routine supported integration must not require source changes, direct database edits or undocumented server-side configuration.

Named candidate vendors/tools in this document are not support claims until their connector/version has passed the certification matrix and the release documentation explicitly lists them as supported.
