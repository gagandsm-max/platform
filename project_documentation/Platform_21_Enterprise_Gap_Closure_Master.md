# Platform — Enterprise Gap Closure Master

**Document ID:** PLATFORM-EGC-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Enterprise Gap Closure**
**Product:** Platform
**Change class:** Additive v3.2.2 product/engineering requirements; no runtime implementation or readiness claim

---

## 1. Authority and purpose

This document is the consolidated specialized master for enterprise capability gaps that are material to Platform adoption but are not owned by the monitoring-intelligence/alert-quality/AI-assurance family reserved for `Platform_22`.

`Platform_00_Document_Index.md` defines precedence. This master supplements existing v3.2.2 requirements and must not silently override a higher-precedence explicit requirement. Contradictions require change control before implementation.

Nothing in this document selects a new OSS dependency, database, mandatory microservice, commercial SKU, price, licence quantity, compliance certification or vendor commitment. Capability statements below are **requirements/targets**, not implementation or production-readiness claims.

---

## 2. Non-negotiable enterprise principles

1. **One canonical model, multiple vendors.** Vendor-specific integrations normalize into Platform-owned contracts rather than leaking vendor schemas through the product.
2. **Control-plane authority stays with Platform.** Git/policy declarations may express desired configuration, but Platform RBAC, validation, approvals, audit and runtime safety remain authoritative.
3. **Compliance claims require evidence.** Protocol support or tamper evidence must not be marketed as certification.
4. **Privacy by design.** Endpoint experience monitoring is operational telemetry, not employee surveillance.
5. **Deletion is a lifecycle, not a button.** Active data, backups, archives, legal holds and final destruction evidence are distinct states.
6. **Existing HA is not reopened.** v1 remains exactly two full Core nodes in Active/Standby; regional federation is future scope.
7. **Integration over imitation.** Incident collaboration integrates with existing enterprise communication/ITSM tools; Platform does not become a chat product.
8. **Future capabilities do not preselect dependencies.** Browser engines, probe SaaS, federation transport and vendor SDKs require later OSS/security/performance review.

---

## 3. Formal product requirements

| PRD ID | Release | Requirement |
|---|---|---|
| `PRD-EGC-001` | P0 | Platform shall expose a normalized SD-WAN model spanning controller/fabric, site, edge, underlay, overlay/tunnel, path, SLA/SLO class, application/path policy and health, with vendor adapters mapped into the canonical model. |
| `PRD-EGC-002` | P1 | Platform shall target SCIM v2-compatible identity lifecycle integration for governed user/group provisioning and deprovisioning, while making no SCIM compliance/certification claim until independently validated. |
| `PRD-EGC-003` | P0 | Platform shall support service-level objectives using defined SLIs, SLO targets, error budgets and multi-window burn-rate evaluation, with customer-visible evidence and policy-driven notification/incident integration. |
| `PRD-EGC-004` | P0 | Security-relevant audit records shall provide independently verifiable tamper evidence, retention and export. The required outcome is tamper evidence; a particular cryptographic construction is not preselected here. |
| `PRD-EGC-005` | P1 | Platform shall provide middleware monitoring Packs/connectors for common enterprise middleware using supported APIs, metrics, logs, JMX/standard interfaces or bounded synthetic checks; full bytecode-instrumentation APM is not implied. |
| `PRD-EGC-006` | P0 | Tenant offboarding shall provide governed export/portability, active-data deletion, backup/archive expiry handling, legal-hold exceptions, status visibility and final destruction evidence without falsely claiming immediate physical erasure from immutable backups. |
| `PRD-EGC-007` | P0 | Licence/entitlement enforcement shall fail safely, preserve the existing seven-day grace requirement where already governed, expose expiry/capacity state, and never invent commercial quantities or pricing without Founder decision. |
| `PRD-EGC-008` | P1 | Platform shall support declarative configuration/policy-as-code workflows with versioned desired state, validation, diff, approval and audit. External repositories may supply desired declarations but cannot bypass Platform runtime authority or safety controls. |
| `PRD-EGC-009` | P1 | Enterprise backup products shall be monitorable through the Pack/API connector architecture for job outcome, SLA, capacity, repository/media health and recoverability evidence where exposed by the product API; Platform does not become a backup engine. |
| `PRD-EGC-010` | P1 | SASE/SSE/ZTNA integrations shall normalize service/connector/tunnel/session/policy/experience evidence into Platform inventory, topology, health and incident context without claiming to replace the security control plane. |
| `PRD-EGC-011` | P1 | Platform shall provide a lean Incident Command Workspace that assembles incident state, evidence, timeline, ownership, actions and links to ITSM/collaboration systems; it shall not duplicate Slack/Teams/chat functionality. |
| `PRD-EGC-012` | V1.x | Optional endpoint Digital Experience Monitoring may collect bounded device/application/network experience signals needed for service quality, with explicit tenant policy, transparency, minimization, retention controls and prohibitions on keystroke/content/user-surveillance collection. |
| `PRD-EGC-013` | V1.x | Platform may add multi-step browser synthetics with scripted journeys, assertions, timing and evidence capture. No browser runtime or automation dependency is selected by this requirement. |
| `PRD-EGC-014` | V1.x | Platform may support customer-operated/outside-in probes for externally observed reachability and service experience. This does not commit Platform to operating a global managed probe SaaS. |
| `PRD-EGC-015` | P2 | Future regional Core federation may provide governed cross-region inventory/service/incident summarization and continuity, but shall not alter the v1 exactly-two-Core Active/Standby contract without separate architecture change control. |

---

## 4. Governing architecture contracts

### 4.1 Normalized SD-WAN

The canonical contract shall separate vendor identity from normalized entities and relationships. At minimum adapters map controller/fabric, sites, edges, circuits/underlays, overlays/tunnels, paths, SLA classes, application/policy bindings and material health/change evidence. Vendor-specific fields may remain namespaced extension attributes. Cross-vendor dashboards, topology, alerts and service views must consume canonical fields first.

### 4.2 SCIM lifecycle

SCIM integration is an identity-governance target, not a certification claim. Provisioning/deprovisioning must be tenant scoped, idempotent, auditable and reconciled with SSO/LDAP/AD/RBAC authority. Deprovisioning must remove effective access promptly while preserving audit history. Unsupported SCIM features must fail explicitly rather than be silently ignored.

### 4.3 SLI/SLO and error budgets

An SLO definition binds service/resource scope, SLI expression, evaluation window, target, exclusions/maintenance semantics and ownership. Burn-rate evaluation must support more than one time horizon so short severe breaches and sustained degradation can be distinguished. Threshold values remain policy/customer decisions unless an existing requirement fixes them.

### 4.4 Audit tamper evidence

Audit records require immutable identity, tenant/actor/action/object/time/result context, ordered export and verifiable integrity evidence. Key custody, signing/hash construction and external anchoring remain engineering decisions subject to threat modelling and performance review; this master mandates the security outcome, not a premature algorithm.

### 4.5 Middleware and backup Packs

Packs use the governed connector framework, credential vault, rate/bound controls, schema/version compatibility, health telemetry and least privilege. They must expose collector health separately from monitored-system health. Backup integrations must distinguish job success from proven restore/recoverability evidence.

### 4.6 Tenant offboarding/data portability

Offboarding is an explicit state machine: `REQUESTED → EXPORT_PREPARED/EXPORT_DELIVERED (when requested) → ACTIVE_DATA_DELETION → BACKUP_ARCHIVE_EXPIRY_PENDING (if applicable) → FINALIZED`, with `LEGAL_HOLD` capable of pausing eligible deletion. Final evidence records what was deleted, what remains under governed retention/hold, scheduled expiry and who authorized/finalized the process. Secrets/credentials are revoked independently and early.

### 4.7 Licensing safety

Entitlement evaluation must be deterministic and observable. Existing seven-day grace behavior remains unchanged unless Founder-approved change control says otherwise. Grace expiry must not corrupt data or silently disable security controls; customer/admin state and remediation path must be visible. Commercial quantities, packaging and prices remain Founder-reserved.

### 4.8 Declarative configuration/policy-as-code

Declarations are versioned inputs to Platform validation. Apply requires schema validation, tenant/RBAC authorization, semantic diff, conflict detection, policy/safety checks, approval where required, audit, bounded rollout and post-apply verification. Git is not a privileged back door and cannot directly mutate runtime stores.

### 4.9 SASE/SSE/ZTNA

Connectors may ingest vendor API/event/telemetry evidence and normalize service edges, connectors, tunnels/sessions, policy references and experience/availability. Authentication secrets remain in the governed credential system. Platform observes/correlates; enforcement changes require separately governed remediation/NCM actions.

### 4.10 Incident Command Workspace

The workspace is a projection over authoritative Incident, Alert, Change, topology/evidence, remediation and ITSM objects. It provides timeline, ownership, current hypothesis/evidence, actions/approvals and deep links. Messages remain in external collaboration tools except bounded integration metadata required for incident context.

Base Platform no-context-switch troubleshooting and Investigation Sessions feed this workspace when linked to an incident. The Incident Command Workspace does not create a second diagnostic, CLI or evidence authority: it projects the same governed diagnostic results, CLI-session references, Live Evidence, comparisons and investigation evidence defined by the base PRD/UI and Platform_22. An Investigation Session may exist before an Incident and later be linked without changing its original evidence provenance.

### 4.11 Endpoint DEX privacy boundary

DEX collection is opt-in/policy-controlled and limited to operational experience signals such as device resource pressure, boot/login timing where lawful/approved, application responsiveness/crash metadata, network reachability/latency and synthetic experience. Platform shall not capture keystrokes, message/document contents, screenshots, microphone/camera data, browsing content or productivity scoring for employee surveillance. Regional/legal requirements remain customer responsibility and must be supported by configurable minimization/retention controls.

### 4.12 Future synthetics/probes/federation

Multi-step browser synthetics, customer-operated probes and regional federation are capability requirements only. Detailed runtimes, protocols, topology, data residency and cost models require explicit architecture/OSS/security/performance decisions before implementation.

---

## 5. Security, reliability and operability requirements

- All new connectors are tenant scoped, least-privilege, credential-vault integrated and rate/bound controlled.
- External identities and vendor object IDs never override Platform canonical tenant/entity authority.
- SLO, licence, connector and offboarding state changes are auditable.
- Connector failure must surface as monitoring/integration degradation and must not be misrepresented as monitored-service failure.
- Export/offboarding artifacts require authorization, encryption in transit, bounded lifetime and audit.
- Policy-as-code changes require dry-run/diff and post-apply verification; partial failure is explicit.
- Incident workspace integrations must tolerate external ITSM/chat outage without blocking core monitoring.
- Future probe/federation designs must address authentication, replay, tenant isolation, data residency, bounded buffering and failure partitions before implementation.

---

## 6. Quality and release gates

P0/P1 capability implementation is not complete until the relevant requirements have source traceability, negative security tests, tenant-isolation tests, upgrade/backward-compatibility coverage, failure-mode tests and operational telemetry/runbook evidence. Protocol or vendor compatibility claims require reproducible contract/integration tests against supported versions.

`V1.x` and `P2` requirements are not v1 GA blockers unless separately promoted through change control. A capability may not be advertised as implemented merely because this master defines it.

---

## 7. Formal traceability requirements

| REQ ID | Release | Requirement / acceptance evidence |
|---|---|---|
| `REQ-EGC-001` | P0 | SD-WAN adapters prove canonical mapping, vendor-extension isolation, tenant scoping and topology/health consumption using supported vendor fixtures/contracts. |
| `REQ-EGC-002` | P1 | SCIM lifecycle tests cover create/update/disable/delete/reconcile, duplicate/retry behavior, RBAC convergence and explicit unsupported-feature behavior; compliance is not claimed by test existence alone. |
| `REQ-EGC-003` | P0 | SLO tests prove SLI calculation, exclusions/maintenance, error-budget consumption, multi-window burn behavior and incident/notification integration using deterministic fixtures. |
| `REQ-EGC-004` | P0 | Audit tamper-evidence tests prove alteration/deletion/reordering is detectable within the selected design and that export verification is independently reproducible. |
| `REQ-EGC-005` | P1 | Middleware/backup Packs prove least privilege, version/error handling, collector-health separation, bounded polling and no bytecode instrumentation unless separately approved. |
| `REQ-EGC-006` | P0 | Offboarding tests prove authorization, export, active deletion, secret revocation, legal-hold handling, backup/archive pending state, scheduled expiry and final evidence without false immediate-erasure claims. |
| `REQ-EGC-007` | P0 | Licensing tests preserve the governed seven-day grace, expose deterministic state transitions, avoid data corruption/security bypass and leave commercial quantities unspecified unless Founder-approved. |
| `REQ-EGC-008` | P1 | Declarative policy tests prove schema/semantic validation, RBAC, diff, approval, conflict handling, bounded apply, audit and post-apply verification; external Git cannot bypass Platform authority. |
| `REQ-EGC-009` | P1 | SASE/SSE/ZTNA tests prove normalized identity/relationship mapping, secret handling, API rate/failure behavior and separation of observation from enforcement. |
| `REQ-EGC-010` | P1 | Incident Command Workspace tests prove authoritative-object linkage, role/tenant scoping, external-integration degradation behavior and absence of an independent chat authority. |
| `REQ-EGC-011` | V1.x | DEX privacy tests prove prohibited content/surveillance fields are not collected, tenant policy gates collection, and retention/export honor configured privacy controls. |
| `REQ-EGC-012` | V1.x | Browser-synthetic implementation requires an approved dependency/security/performance decision plus deterministic journey, secret, evidence-retention and failure-isolation tests. |
| `REQ-EGC-013` | V1.x | Outside-in probe implementation proves customer/probe identity, tenant scope, replay resistance, bounded buffering, time quality and explicit ownership/operating model. |
| `REQ-EGC-014` | P2 | Regional federation requires separate architecture approval and tests for partition, authority, data residency, reconciliation and compatibility while preserving v1 two-Core HA semantics. |
| `REQ-EGC-015` | P0 | Every capability introduced under this master exposes health/lag/error/capacity telemetry sufficient to distinguish Platform collection/control degradation from monitored-environment failure. |

---

## 8. Explicit exclusions and unresolved engineering spikes

This master does **not** approve full bytecode APM, employee surveillance, Platform-operated global probe SaaS, multi-region active/active Core, a chat product, a backup engine, a SASE enforcement plane, or any new OSS/runtime by implication.

Engineering decisions still requiring bounded spikes before implementation include: SCIM compatibility matrix; audit tamper-evidence construction/key custody; first middleware/backup/SASE vendor priorities; browser synthetic runtime; customer-probe trust/bootstrap model; and regional federation authority/data-residency model.

---

## 9. Founder-reserved decisions

Founder approval remains required for commercial licence quantities/pricing/SKUs, contractual SLO/RPO/RTO commitments, claims of certification/compliance, operation of a managed global probe network, material new vendor/cloud spend, and any change to the v1 exactly-two-Core Active/Standby topology.

---

## 10. Implementation-status disclaimer

At creation, this document is a requirements/architecture authority only. It does not assert that any `PRD-EGC-*` or `REQ-EGC-*` capability has been implemented, benchmarked, certified or released. Implementation status must be established from exact source, executable tests, CI/runtime evidence and release records.