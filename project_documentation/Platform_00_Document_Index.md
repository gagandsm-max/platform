# Platform — Mother Reference Documentation Constitution

**Document ID:** PLATFORM-INDEX-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Documentation Constitution (A1 NORMALIZED_V1)**
**Product:** Platform (internal engineering/build identifier) / AmrudAi (commercial customer-facing product name)
**Project independence:** Clean-room product; separate from ThoughtData, Enterprise360 and NetAI
**Last reconciliation:** 2026-09-24 — A2.3 OSS catalogue normalization and formal OSS-008→REQ-OSS-008 migration (Issue #108)
**Normalization state:** NORMALIZED_V1. Platform_01..08 are NORMALIZED_V1 after A2.3; Platform_09..23 remain TRANSITIONAL_LEGACY until later A3–A5 tranches.

## 1. Authority

1. The canonical GitHub repository `https://github.com/gagandsm-max/platform` (`main` after merge) is the authoritative system of record for Platform Mother References and implementation.
2. Current Mother Reference version is **3.2.2**.
3. The exact authoritative Mother Reference set is the **25-file set** listed in §5 (`project_documentation/AGENTS.md` plus `Platform_00` through `Platform_23`). Supporting artifacts such as `mother_reference_manifest.json` and `project_documentation/tools/validate_mother_references.py` are governance tooling, **not** additional Mother References and must not be numbered as `Platform_24+`.
4. Source precedence is defined in §6. Conflicts require documented change control; implementation must not silently choose a convenient interpretation.
5. Mother documents are READ-MOSTLY. Deliberate change control is mandatory (see §12).
6. Repository implementation does **not** silently override an explicit approved product decision recorded in the Mother References or Decision Register.
7. If implementation discovers false, stale or contradictory documentation, engineering must correct the Mother References through change control rather than knowingly preserve a false specification.
8. v3.2.2 supersedes v3.2.1 as the authoritative Platform mother-reference set after merge to canonical GitHub `main`. Earlier v1/v2/v3.0/v3.1 generated master packages and the legacy `Platform_Master_Architecture_v1.0.docx` are superseded as Platform engineering sources of truth. Original ThoughtData/Enterprise360, NetAI, SolarWinds, ManageEngine and other supplied reference material remains reference/evidence only and never overrides this set.
9. Open/draft PR filenames, branch names, titles or draft document headers do **not** reserve a `Platform_NN` number and do not change the authoritative Mother Reference set or version.

## 2. No-assumption rule

- Founder-fixed / explicit product-owner decisions remain fixed until deliberate change control changes them.
- Architecture-delegated decisions require recorded evidence and Decision Register updates; they are not free-form invention.
- Unresolved values remain explicit `TBD — Decision Required` or `P0 ENGINEERING SPIKE`. No inference may convert TBD into implementation truth.
- Capacity statements remain engineering targets until benchmark-certified. A capacity target is not benchmark certification.
- Security/compliance readiness must never be represented as achieved certification until independently achieved. Security readiness is not compliance certification.
- Customer-value/MTTR percentages are not product claims until supported by disclosed reproducible customer/pilot evidence.
- A1 does not resolve Founder/commercial/legal-reserved conflicts (including any licence-grace day-count conflict between Mother References and external handover notes). Encountered conflicts remain unresolved until Founder change control.

## 3. Maturity model

The following maturity vocabulary is normative. A lower state never implies a higher state. AI agents and humans must not casually upgrade maturity without physical evidence.

| State | Meaning |
|---|---|
| **SPECIFIED** | An approved normative design exists in the Mother References (or an approved Decision Register entry). |
| **IMPLEMENTED** | Physical production-path source exists and is source-verified (migration, package, service, API implementation, or equivalent). |
| **TESTED** | Required automated and/or runtime tests for the implemented boundary pass with reproducible evidence. |
| **BENCHMARKED** | Applicable workload/resource/performance evidence exists for the declared profile/envelope. |
| **CERTIFIED** | All applicable architecture, security, tenancy, failure, scale, lifecycle, upgrade/recovery and operability gates are proven for the declared certification profile. |
| **DEPLOYED** | The certified artifact/version is actually installed in the stated environment. |

Canonical inequality:

`SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED`

Unsupported combined spellings that accidentally imply a higher maturity without evidence are prohibited as certification claims. Phrases such as `IMPLEMENTED+TESTED` may describe concurrent evidenced states only when each component is independently true; they do not imply `BENCHMARKED`, `CERTIFIED` or `DEPLOYED`.

## 4. Logical design versus physical implementation

A logical table, API, topic, service or interface described in a Mother Reference does **not** prove it physically exists.

Where implementation exists, documentation should eventually point to physical source evidence such as:

- migration;
- schema;
- package;
- service;
- source path;
- API implementation;
- test;
- benchmark;
- CI evidence.

Where absent, status remains `SPECIFIED` / `NOT IMPLEMENTED` (or equivalent). A1 establishes this rule; A2–A5 progressively attach evidence pointers without inventing architecture.

## 5. Exact Mother Reference set

| # | File | Governing scope |
|---:|---|---|
| 1 | `AGENTS.md` | Coding-agent/engineer navigation and non-negotiable repository rules |
| 2 | `Platform_00_Document_Index.md` | Authority, precedence, file map, change/completeness rules |
| 3 | `Platform_01_BRD.md` | Business intent, outcomes, scope, principles, commercial/security/continuity requirements |
| 4 | `Platform_02_PRD.md` | Base customer-visible functional and non-functional product requirements |
| 5 | `Platform_03_Architecture.md` | Governing architecture, technology, deployment, HA/data/Sensor/AI/security decisions |
| 6 | `Platform_04_HLD.md` | Subsystems, deployment units, interfaces, ownership and failure boundaries |
| 7 | `Platform_05_LLD.md` | Concrete schemas, topics, state machines, adapters, algorithms and implementation rules |
| 8 | `Platform_06_Dataflow.md` | End-to-end telemetry/control/HA/RCA/NCM/storage/integration sequences |
| 9 | `Platform_07_OSS_Catalog.md` | Authoritative OSS allow/candidate list, licences, custom boundaries and governance |
| 10 | `Platform_08_Requirements_Traceability.md` | Global requirements, product-requirement and evidence traceability index |
| 11 | `Platform_09_UI_UX_Master.md` | UI shell, navigation, Studios, RBAC UX and out-of-box dashboards/workspaces |
| 12 | `Platform_10_Protocol_Security_Connector_Master.md` | Protocols, authentication, credentials, ports, connector and provisioning diagnostics |
| 13 | `Platform_11_Sizing_Capacity_Storage_Master.md` | Core/Sensor sizing, MWU, placement, retention, rollups and storage protection |
| 14 | `Platform_12_Decision_Source_Register.md` | Fixed/delegated/reference/TBD decision provenance |
| 15 | `Platform_13_Engineering_Development_and_Docker_Standards.md` | Engineering execution + Docker Compose development/test standard |
| 16 | `Platform_14_Test_Performance_and_Release_Quality.md` | Test strategy + benchmarks/performance + Alpha/Beta/RC/GA quality gates |
| 17 | `Platform_15_CI_CD_Packaging_and_Distribution.md` | CI/CD + SBOM/signing + `.deb`/`.rpm`/Windows packaging/distribution |
| 18 | `Platform_16_Installation_Upgrade_Rollback_and_Operations.md` | Install/HA enrollment/upgrade/rollback + operational runbooks |
| 19 | `Platform_17_API_Event_and_Data_Model_Contracts.md` | API/event compatibility + PostgreSQL/ClickHouse/Kafka/Valkey data contracts |
| 20 | `Platform_18_Security_Threat_Model.md` | Trust boundaries, threats, abuse cases and mitigations |
| 21 | `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` | Authoritative OIV specialized master: SaaS/carrier/change intelligence, Flight Recorder, Similar Incident Memory, triggered packet evidence, controlled remediation, MTTR/value engine, formal `PRD-OIV-*` + `REQ-OIV-*` supplement |
| 22 | `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md` | Authoritative onboarding specialized master: discovery review, Agent enrollment/identity continuity, approval/audit, grouping/policy preview, Notification Center, formal `PRD-ONB-*` + `REQ-ONB-*` supplement |
| 23 | `Platform_21_Enterprise_Gap_Closure_Master.md` | Authoritative enterprise gap-closure specialized master: normalized SD-WAN, SCIM lifecycle target, SLA/OLA/Provider-SLA service assurance, SLI/SLO/error budgets, audit tamper evidence, enterprise connector/offboarding/licensing/policy-as-code requirements, formal `PRD-EGC-*` + `REQ-EGC-*` supplement |
| 24 | `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` | Authoritative monitoring-intelligence specialized master: class-specific Alert Evidence Contracts, telemetry trust, semantic monitoring states, Endpoint Diagnostic Snapshot (EDS), tiered local intelligence, RCA/AI assurance, Incident Copilot, AI safety/provenance, formal `PRD-MIA-*` + `REQ-MIA-*` supplement |
| 25 | `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md` | Authoritative enterprise-monitoring-coverage specialized master: Capability Pack product contract, L1/L2/L3 dimensional certification, Linux/Windows Agent and agentless infrastructure coverage, technology-family catalogue, Customer Acceptance Profiles, Real Customer Requirements normalization, formal `PRD-EMC-*` + `REQ-EMC-*` supplement |

### 5.1 Specialized-master number allocation

The canonical Mother Reference remains the actual merged set listed in §5 on current GitHub `main`. Today the numbered specialized-master sequence ends at the highest merged master (`Platform_23`). Candidate or open PR filenames, branch names, titles or draft document headers do **not** reserve a `Platform_NN` number and do not enlarge the authoritative set.

A Platform number becomes canonical only when the corresponding specialized master is finally reconciled and merged. At each final specialized-master reconciliation:

1. fetch exact current `main`;
2. enumerate merged canonical `Platform_NN_*` specialized masters;
3. determine the highest canonical number;
4. assign the next available number to the master currently being finalized;
5. rename the candidate document to that number;
6. update Index, Decision Register, RTM, AGENTS and other cross-references where applicable;
7. make those changes atomically in the same final reconciliation;
8. run exact-head validation/review/CI;
9. merge;
10. only then does that number become canonical.

Multiple parallel candidate PRs must not each claim the same next number. Merge order against then-current `main` determines allocation. Candidate documents remain **unnumbered/unallocated until final reconciliation**; temporary internal names are allowed, but they confer no reservation authority. Number assignment, rename and cross-reference updates must occur atomically before merge. A candidate specialized master must not silently advance the set-wide Mother Reference version (currently **v3.2.2**); version changes require separate set-wide change control.

## 6. Source precedence

When requirements appear to conflict:

1. BRD (`Platform_01`) — business intent/non-negotiable outcomes.
2. PRD (`Platform_02` and authoritative specialized `PRD-*` supplements) — customer-visible behavior.
3. Architecture (`Platform_03`) — technology/deployment/security constraints.
4. HLD (`Platform_04`) — subsystem boundaries.
5. LLD (`Platform_05`) — implementation contracts.
6. Dataflow (`Platform_06`) — end-to-end sequencing consistent with the above.
7. Specialized masters (`Platform_19`..`Platform_23`) — detailed normative contracts in their assigned domain.
8. Decision Register (`Platform_12`) — provenance/TBD control.

**Specialized-master special cases (unchanged):**

- **OIV:** `Platform_19` may add behavior where existing files are silent, but may not silently override an explicit higher-precedence requirement. Formal `PRD-OIV-*` / `REQ-OIV-*` are authoritative supplements.
- **Onboarding:** `Platform_20` `PRD-ONB-*` / `REQ-ONB-*` are authoritative supplements for discovery/onboarding/identity/notification.
- **Enterprise gap-closure:** `Platform_21` `PRD-EGC-*` / `REQ-EGC-*` govern that enterprise capability family.
- **Monitoring-intelligence:** `Platform_22` `PRD-MIA-*` / `REQ-MIA-*` govern alert quality, evidence qualification and AI/RCA assurance without silently overriding Platform_19/21 outside assigned scope.
- **Enterprise monitoring coverage:** `Platform_23` `PRD-EMC-*` / `REQ-EMC-*` govern Capability Pack / dimensional certification without silently overriding Platform_10/19/20/21/22 outside assigned scope.

Conflicts require documented change control.

## 7. Normalized document ownership

| Document | Owns |
|---|---|
| `Platform_01` | Business intent/outcomes/scope. |
| `Platform_02` | Product capability and customer-visible behavior. |
| `Platform_03` | System architecture/deployment/trust/data/control planes/HA. |
| `Platform_04` | Subsystems/services/modules/interfaces/failure boundaries. |
| `Platform_05` | Executable low-level technical contracts including data/index/cache/concurrency algorithms. |
| `Platform_06` | Success/failure/retry/failover dataflows. |
| `Platform_07` | Dependency/provenance/licensing authority. |
| `Platform_08` | **Global** requirements/product-requirement/evidence traceability index. |
| `Platform_09` | UI/UX/workspace/workflow contract. |
| `Platform_10` | Protocol/transport/credential/connector mechanics. |
| `Platform_11` | Capacity/cardinality/retention/storage/resource budgets. |
| `Platform_12` | Decisions/TBD/provenance/implementation-truth deltas. |
| `Platform_13` | Engineering/repository/development standards. |
| `Platform_14` | Test/benchmark/certification/release quality gates. |
| `Platform_15` | CI/CD/artifacts/signing/SBOM/packaging/distribution. |
| `Platform_16` | Install/upgrade/rollback/backup/restore/operations/runbooks. |
| `Platform_17` | API/event/data/schema/compatibility authority. |
| `Platform_18` | Security threat/risk/control/verification authority. |
| `Platform_19`..`Platform_23` | Specialized product domains using the **same** capability-contract discipline and cross-referencing base mechanics rather than inventing duplicates. |
| `AGENTS.md` | Coding-agent/engineer navigation and non-negotiable repository rules (not a numbered Platform Document ID). |

## 8. Mandatory capability specification contract

Every significant capability/sub-capability must eventually be specified against the following contract fields where applicable. Fields that are genuinely irrelevant may state **N/A with reason**. The contract must not encourage meaningless boilerplate.

1. capability ID/name
2. customer/operational outcome
3. scope
4. explicit exclusions
5. subfeatures
6. owning subsystem/module/service
7. owning engineering agent/domain
8. upstream dependencies
9. downstream dependencies
10. deployment/execution location
11. control-plane boundary
12. data-plane boundary
13. public/internal interfaces
14. APIs/actions
15. transport
16. protocol
17. source→destination direction
18. ports/defaults/override rules
19. authentication
20. authorization
21. PostgreSQL ownership
22. ClickHouse ownership
23. Kafka ownership
24. Valkey/cache ownership
25. object/table/entity model
26. primary/natural keys
27. same-tenant relationships/FKs
28. indexes
29. primary query/access patterns
30. pagination model
31. cache rules/invalidation/degradation
32. state machine/lifecycle
33. transactional boundaries
34. idempotency
35. replay/deduplication
36. ordering
37. concurrency/race behavior
38. tenant isolation
39. RBAC/resource scope
40. credential handling
41. secret handling
42. PKI/certificate behavior
43. normal success flow
44. failure states
45. degraded states
46. retry/backoff
47. recovery semantics
48. HA/fencing/ownership
49. capacity limits
50. cardinality limits
51. concurrency/rate limits
52. backpressure
53. storage budget/horizon
54. logs
55. metrics
56. health/self-monitoring
57. audit
58. customer-visible diagnostics
59. retention
60. purge
61. legal hold/protected evidence
62. backup
63. restore
64. DR implications
65. install/package implications
66. migration
67. upgrade
68. backward compatibility
69. rollback
70. UI/workflow behavior
71. stable customer errors/remediation
72. security threats/abuse cases
73. negative/failure tests
74. integration tests
75. performance/scale tests
76. certification profile/evidence
77. current maturity
78. physical implementation/evidence pointers
79. requirement IDs
80. decision/source references
81. open decisions/TBD
82. owner of each unresolved decision
83. blocking impact

A1 makes this contract normative. Completing the field set for every capability across Platform_01..23 is A2–A5 work, not an A1 claim.

## 9. Fixed product/architecture decisions

- Product name: **Platform** (internal engineering/build identifier) / **AmrudAi** (commercial customer-facing product name).
- Commercial branding and reports: Customer-facing UI displays effective commercial branding (defaulting to AmrudAi, or licensed tenant/MSP branding where white-label is active); report artifacts are **PDF and/or CSV only**; generated PDF reports prominently display the effective commercial logo (defaulting to the governed AmrudAi logo, or licensed tenant/MSP logo where white-label is active); CSV reports contain tabular data only.
- Clean-room implementation; no ThoughtData/NetAI proprietary implementation reuse.
- Core/Sensor: **Ubuntu 24.x LTS family** and **RHEL 9.x family** first (subject to certification matrix); Windows Core/Sensor not initial scope.
- Standalone: one Core. HA: exactly two full Core nodes, Active/Standby, target <60s Core-service restoration.
- Sensors continue polling and locally buffer through Core failure/WAN loss while capacity permits; 80% storage protection may reclaim/shed eligible oldest lower-priority data and must expose data-loss risk. Absolute zero-loss is not promised.
- Sensor/Agent external channel: HTTPS/gRPC over mTLS; Kafka stays internal.
- Data stack: PostgreSQL authoritative control state, ClickHouse high-volume telemetry analytics, Kafka streaming backbone, Valkey cache/ephemeral state.
- Go Sensors/Agents; Java 21 Core; React/TypeScript UI; Python for selected NCM/AI/analytics workers.
- Core ≤2,500 reference devices: 8 vCPU/16 GB mandatory engineering target under SNWP-2500-v1.
- UI-driven provisioning: individual, CSV/XLSX bulk and network auto-discovery.
- Placement is locality + reachability + capacity + HA + security aware.
- Full always-on DPI/full APM remain excluded initially; **bounded triggered packet evidence** is an approved diagnostic capability and does not reopen the full-DPI decision.
- Graph-grounded hybrid RCA; GNN is an augmenting scorer, not causal proof; `INCONCLUSIVE` is required.
- Native Platform dashboard/UI; no embedded Grafana.
- Core and Sensor managed-storage 80% protection watermark is mandatory.
- SNMP is a typed evidence source rather than a numeric-only transport. Exact raw counters, state/inventory/topology facts and collection-quality outcomes remain semantically distinct; a failed SNMP poll alone does not prove device failure.
- Routine SNMP monitoring is read-only. SNMP SET, when implemented, is a privileged NCM/remediation action subject to authorization, policy/approval, pre/post verification and audit rather than an ordinary polling operation.
- Customer controls raw retention, rollups, archive and allowed purge priority.
- DR capability required in v1.0; contractual RPO/RTO remain TBD until frozen.
- Gemini through AI Gateway initially; deterministic/causal RCA remains independent.
- HTTPS-only Platform UI; no application listener on TCP/80.
- Development/test: Docker Compose standard; host laptop kept free of project-specific runtimes.
- Production packaging: Core/Sensor/Linux Agent `.deb` + `.rpm`; Windows Agent signed service EXE + MSI + BAT wrappers.
- Core, Sensors and Agents auto-start after reboot and emit structured diagnostics/logs.
- Canonical source repository: `https://github.com/gagandsm-max/platform`.
- First-class SaaS monitoring, cloud-native runtime dependency discovery, external deployment/change telemetry and carrier/ISP operational integration are approved product scope.
- Incident Flight Recorder, Similar Incident Memory and cross-domain Change Intelligence are approved MTTR-reduction capabilities.
- Controlled remediation is approved only with deterministic action definitions, authorization, preconditions, blast-radius controls, approval/policy, audit, post-action validation and rollback/fail-safe handling; no direct LLM privileged execution.
- Platform Value / MTTR Improvement measurement and the four-layer customer-value dashboard are approved product scope; unsupported causal or financial claims are prohibited.
- Network auto-discovery and Windows/Linux Agent enrollment use a governed review lifecycle; full monitoring/licence activation requires review by default unless authorized trusted auto-approval policy applies.
- Approve/reject/unmanaged/exclude onboarding decisions require operator comment/reason and audit; pre-activation review supports metadata, group/sub-group and monitoring/alert-policy preview.
- Agent-backed endpoint identity is anchored to canonical `entity_id` plus unique `agent_id`/mTLS identity; system/host name is human-readable technical identity and dynamic IP/MAC values are mutable observations.
- Platform exposes one global per-user Notification Center/bell with server-side tenant/RBAC/resource scoping; operational Alerts/Incidents remain distinct from platform/workflow notifications.
- Enterprise integrations normalize vendor-specific evidence into Platform-owned canonical contracts; vendor identifiers do not override tenant/entity authority.
- Existing seven-day licence grace remains governed behavior. Commercial quantities, packaging and pricing remain Founder-reserved.
- Declarative policy/configuration inputs cannot bypass Platform RBAC, validation, approvals, audit or runtime safety.
- Endpoint DEX is bounded operational-experience monitoring, not employee surveillance.
- Future regional federation does not reopen the v1 exactly-two-Core Active/Standby topology by implication.
- Material alert classes use class-specific evidence contracts and telemetry-quality trust checks before authoritative operational-failure assertions.
- Alert severity and evidentiary confidence are independent; semantic states such as `MONITORING_DEGRADED`, `REDUNDANCY_LOST`, `SLA_DEGRADED`, `EXPECTED_STATE`, `UNKNOWN` and `STALE` are preserved where applicable rather than collapsed into generic DOWN.
- GNN/LLM outputs are evidence/hypothesis assistance, never causal authority; `INCONCLUSIVE` remains valid and prompt/model text cannot grant privileged execution authority.
- Core monitoring and deterministic RCA/alerting continue without Internet/hosted AI availability; hosted/private/local/no-LLM operation is governed by explicit data-egress and fallback controls.
- Capability Pack is the canonical enterprise extension architecture. Technology Monitoring Pack, Device / Network Pack, SaaS Monitoring Pack, Integration Pack and optional Customer Extension Pack are classifications/profiles of that architecture, not independent frameworks. Existing Device Pack contracts remain valid under this model.
- Monitoring-technology certification is dimensional (Discovery/L1/L2/L3/Topology/NCM/Security/version). A family listed in `Platform_23` is a coverage target, not an implemented or certified pack.
- Windows/Linux server monitoring certifies Agent-based and agentless/remote modes separately; collection/credential/path failure is monitoring-health evidence and does not automatically assert host/service failure.
- Endpoint Diagnostic Snapshot is a bounded typed evidence-acquisition capability triggered by qualified policy state or authorized investigation; endpoint evidence informs but does not replace Core-authoritative cross-domain RCA.
- Normal monitored Agents and Sensors do not host an LLM/GNN/foundation-model runtime for EDS; endpoint-local intelligence is deterministic/bounded statistical logic only.
- Optional dedicated local/private inference is an AI-Gateway provider, not a third Core/HA authority or authoritative data store; deterministic Platform operation does not depend on it.
- Authoritative system/event/telemetry instants are absolute UTC; interactive display uses IANA zones with precedence override → user preference → browser local → tenant.timezone → UTC; display preference never rewrites operational policy timezones; host OS timezone is non-authoritative.
- Recurring monitoring uses Core schedule/placement authority plus Sensor durable local desired-state execution; Monitor Group, Scheduling Class and Execution Class remain distinct; cadence and freshness are both required; deterministic phase spreading and failure-domain backoff are mandatory architecture.
- Exact raw Counter64 Sensor→Core→Kafka→ClickHouse transport/persistence is **IMPLEMENTED+TESTED** on current main after PR #84, including full unsigned-64 boundary coverage and replay-safe `UInt64` persistence; benchmarking/certification/deployment remain open, and derived delta/rate/reset/wrap/discontinuity semantics remain a P0 implementation gap.
- Exactly-two-Core Active/Standby HA remains P0 and not production-certified; no Patroni/repmgr/ClickHouse Keeper/Raft/Consul/etcd third-coordination selection is made by this baseline.
- Standard production Sensor roles are NETWORK, INFRASTRUCTURE, FLOW and LOG; SYNTHETIC and CLOUD_API are conditional dedicated roles; COMBINED is lab/dev/small-deployment unless separately certified; runtime STREAMING_EVENT requires product/runtime reconciliation toward FLOW/LOG.
- PR #81 Sensor telemetry durability is canonical at-least-once; concurrent pending-reservation republish amplification is P1 hardening, not a new datastore.

## 10. Completeness and global traceability

`Platform_08_Requirements_Traceability.md` is the **single global index** for formal `REQ-*` and `PRD-*` identifiers.

- Detailed normative requirements remain in their owning documents.
- Specialized-master `REQ-*` / `PRD-*` supplements remain authoritative in their owners and are indexed globally by Platform_08.
- A new requirement is incorporated only when it has an ID, affected-layer mapping and sufficient normative behavior for implementation without inventing a load-bearing decision.
- Documentation mapping is not implementation evidence.

Machine-enforced structural protection is provided by:

- `project_documentation/mother_reference_manifest.json`
- `project_documentation/tools/validate_mother_references.py`

These are supporting governance artifacts, not Mother References.

## 11. Explicit unresolved/P0 items

Authoritative list is `Platform_12_Decision_Source_Register.md` §5 and the 2026-09-21 handoff priority freeze. Restated here for navigation:

- exact Counter64 typed pipeline (Sensor→Core→Kafka→ClickHouse) preserving full unsigned 64-bit source truth;
- display timezone persistence/API/UI behavior (`display_timezone` preference, precedence, DST/report correctness);
- generic recurring scheduling/freshness/deterministic spreading and production scheduler wiring (including SNMP);
- failure-domain backoff, recovery ramp and resource-pressure deferral behavior;
- exact two-Core Kafka implementation/failover while honoring no third full Core;
- exact two-Core ClickHouse replication/promotion/fencing design;
- exact PostgreSQL two-Core failover/fencing/rewind mechanics;
- exact Core service fencing/VIP implementation and failure matrix;
- Sensor ownership/lease/fencing and push-protocol HA continuity;
- Flow/NTA production runtime and FLOW-role certification; LOG production role/runtime reconciliation;
- final contractual DR RPO/RTO and data-loss objectives;
- final SNWP-2500-v1 metric/interface/event mix and certified performance results;
- Streaming/Flow Sensor certified Flow records/sec and Syslog EPS;
- exact GNN model architecture, training labels, inference latency, CPU/GPU footprint and production enablement threshold;
- final compliance certification program/scope and achieved certifications;
- exact product SKUs/licensing quantities beyond the agreed behavioral rules.

These remain unresolved until formally approved; no coding tool may invent them. Specialized-master additions do not resolve these P0/TBD items by declaration.

## 12. Deliberate change control (READ-MOSTLY)

Mother documents are **READ-MOSTLY** and authoritative.
Coding agents must **not casually** modify them while implementing features.

A mother document **MUST** be updated when:

1. a factual/documentation error is identified;
2. two mother documents contradict one another;
3. an approved product decision changes;
4. an approved architecture decision changes;
5. an implementation dependency becomes formally selected;
6. a P0/TBD item is resolved;
7. a requirement was accidentally omitted;
8. an implemented behavior would otherwise conflict with documented truth.

Such changes require deliberate change control, cross-document reconciliation, Decision Register update where a decision changes, and review through the canonical GitHub repository. No implementation is allowed to permanently diverge from the mother-reference truth. If implementation discovers an error in the mother documents, engineering must correct the mother documents rather than knowingly preserve a false specification.

## 13. Canonical repository and working clone

**USER-FIXED:** Canonical Platform source repository is `https://github.com/gagandsm-max/platform`.

- GitHub is the authoritative repository/system of record.
- Local `~/nms` (or any other clone path) is a **working clone**, not an independent source of truth.
- Code, build, test and mother-document changes become authoritative only after being pushed and merged into the canonical GitHub repository.
- Normal development uses short-lived branches.
- Material changes use pull requests.
- Do not directly push normal development changes to the protected/default branch.
- Commits must be traceable to the implementation/change being made.
- Do not force-push shared/default branches.
- GitHub history must preserve architecture/document decision provenance.
- Selecting GitHub as source of record does **not** by itself freeze a CI provider. GitHub Actions may be evaluated later; it is not a fixed architecture decision unless separately approved.

## 14. Development-stage readiness

Before coding a module, an engineer or coding agent reads `AGENTS.md`, this Index, the relevant BRD/PRD requirement, Architecture/HLD/LLD/Dataflow sections, OSS Catalogue, **all task-relevant specialized masters discovered from this Index**, and the Decision Register. Do not hard-code an upper specialized-master document number in agent/tooling logic.

For any OIV task, `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` is mandatory preflight reading.
For discovery review, Agent onboarding/enrollment identity, dynamic endpoint identity/reconciliation or in-product Notification Center work, `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md` is mandatory preflight reading.
For normalized SD-WAN, SCIM lifecycle, SLA/OLA/Provider-SLA service assurance, SLI/SLO/error budgets, audit tamper evidence, middleware/backup/SASE connectors, tenant offboarding/data portability, licensing safety, declarative configuration/policy-as-code, Incident Command Workspace, privacy-bounded endpoint DEX, browser synthetics, customer-operated probes or future regional federation, `Platform_21_Enterprise_Gap_Closure_Master.md` is mandatory preflight reading.
For alert evidence/qualification, semantic monitoring states, alert-policy lifecycle/quality, active evidence acquisition, RCA confidence/contradictions, AI evaluation/safety, Incident Copilot, AI provider/egress controls, prompt-injection/tool-use protection, tenant-scoped learning, shared-service dependency evidence, PKI assurance or AI observability, `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md` is mandatory preflight reading.
For enterprise monitoring-domain catalogue, Capability Pack contract/lifecycle, dimensional L1/L2/L3 certification, Customer Acceptance Profiles, Real Customer Requirements normalization, operational-graph vs governed-CMDB semantics, or coverage-measurement work, `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md` is mandatory preflight reading.

A feature is not complete when it only compiles: tests, observability, error/failure behavior, upgrade/migration impact, security/OSS implications, traceability and acceptance evidence are part of Definition of Done.

## 15. Historical reconciliation policy

A1 does **not** claim to have normalized all of Platform_01..23.

Current Mother References contain substantial historical reconciliation material (chronological amendment blocks, handoff appendices and duplicated/out-of-order structure in transitional documents).

Policy:

1. Current normative constitution is this document, sections 1–16. Appendix A is transitional historical/reconciliation material retained for zero-loss migration and is not the destination for new normative requirements.
2. Transitional historical/reconciliation material is retained in **Appendix A** so A1 does not recklessly delete content and risk requirement loss.
3. No new product requirement may be appended to Appendix A or to any historical reconciliation block.
4. Historical sections are retained temporarily for zero-loss migration.
5. A2–A5 will absorb valid current requirements into proper owning documents before historical blocks are removed.
6. Git history, PRs and the Decision Register are ultimately the history source.

## 16. Repository placement and agent navigation

For ChatGPT Project use, these **25 Markdown mother-reference files** are sufficient as the authoritative source set.
For code-agent enforcement, `AGENTS.md` should also be available at repository root (`~/nms/AGENTS.md` in a working clone), preferably as the existing symlink to `project_documentation/AGENTS.md`, so tools that auto-discover root instructions see the same content.
Authority of the clone follows §12 after merge to the canonical GitHub repository.

OSS governance remains governed by `Platform_07_OSS_Catalog.md`:

Only components admitted by `Platform_07_OSS_Catalog.md` at an approved pinned version may ship.
Top-level licence alone is insufficient; transitive/bundled licences, notices, vulnerabilities and exact artifacts must be scanned.
Every release produces SPDX and CycloneDX SBOMs, third-party notices/licence texts, version locks, checksums and provenance.
Implementation dependencies must match the catalogue; catalogue omissions discovered in implementation require catalogue/change-control updates before milestone/release.

`Platform_19`, `Platform_20`, `Platform_21`, `Platform_22` and `Platform_23` do not by themselves select new runtime dependencies. Any new vector database, packet-analysis product, SaaS SDK, browser automation engine, probe runtime, AI/model runtime, vendor SDK, pack runtime or other dependency requires normal OSS catalogue/security/performance approval.

---

## Appendix A — Transitional historical / reconciliation material (zero-loss retention)

> **TRANSITIONAL.** Retained for zero-loss migration under Issue #94 / programme #93.
> **Forbidden destination for new product requirements.**
> A2–A5 must absorb valid current requirements into proper owners before removal.
> Git history / PR / Decision Register remains the long-term history source.

### A.1 Consolidation and version map (historical)

### 4.1 v3.1 → v3.2 consolidation

| v3.1 document(s) | v3.2 destination |
|---|---|
| `Platform_13_Engineering_Execution_Standards.md` + `Platform_14_Docker_Development_Environment.md` | `Platform_13_Engineering_Development_and_Docker_Standards.md` |
| `Platform_15_Test_Strategy_and_Quality_Gates.md` + `Platform_22_Performance_and_Benchmark_Plan.md` + `Platform_24_Release_Acceptance_Criteria.md` | `Platform_14_Test_Performance_and_Release_Quality.md` |
| `Platform_16_CI_CD_and_Release_Engineering.md` + `Platform_18_Packaging_and_Distribution.md` | `Platform_15_CI_CD_Packaging_and_Distribution.md` |
| `Platform_17_Installation_Upgrade_Rollback_Runbook.md` + `Platform_23_Operational_Runbooks.md` | `Platform_16_Installation_Upgrade_Rollback_and_Operations.md` |
| `Platform_19_API_and_Event_Contract_Master.md` + `Platform_20_Data_Model_and_Schema_Master.md` | `Platform_17_API_Event_and_Data_Model_Contracts.md` |
| `Platform_21_Security_Threat_Model.md` | `Platform_18_Security_Threat_Model.md` |

Every source section is retained in the destination document. Consolidation is not permission to drop a requirement.

### 4.2 v3.2.1 → v3.2.2 additions

v3.2.2 adds specialized masters under a consolidated-amendment pattern instead of duplicating the same normative text across many base files.

`Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` carries OIV business/product, architecture, HLD, LLD/data, dataflow, UI, security, capacity, quality, operations and RTM authority. Its formal `PRD-OIV-*` and `REQ-OIV-*` IDs are authoritative supplements to the base PRD/RTM catalogues.

The 2026-09-11 reconciliation adds `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md` under the same pattern. Its §§3–12 carry product/architecture/HLD/LLD/dataflow/UI/security/quality/operations authority for the onboarding capability family, and §13 provides formal `REQ-ONB-*` traceability.

The enterprise gap-closure reconciliation adds `Platform_21_Enterprise_Gap_Closure_Master.md` under the same pattern. Its `PRD-EGC-*` and `REQ-EGC-*` requirements govern the enterprise capability family while explicitly reserving monitoring-intelligence/alert-quality/AI-assurance requirements for a separate specialized master.

The 2026-09-12 monitoring-intelligence reconciliation adds `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md`. Its `PRD-MIA-*` and `REQ-MIA-*` requirements govern alert evidence/quality, semantic monitoring states, RCA/AI assurance, bounded AI assistance and related dependency evidence. This remains **v3.2.2**; no v3.2.3 product-version number is created.

The 2026-09-13 enterprise-monitoring-coverage reconciliation adds `Platform_23_Enterprise_Monitoring_Coverage_and_Technology_Pack_Master.md` under the same pattern. Its `PRD-EMC-*` and `REQ-EMC-*` requirements govern the Capability Pack architecture, dimensional certification, enterprise technology coverage and Customer Acceptance Profiles. Technology Monitoring Pack, Device / Network Pack, SaaS Monitoring Pack, Integration Pack and optional Customer Extension Pack are classifications/profiles of Capability Pack, not a competing framework. This remains **v3.2.2**; no v3.2.3 product-version number is created. `Platform_23` does not replace Device Pack contracts, `Platform_10` connector/security mechanics, `Platform_19` OIV, `Platform_20` onboarding, `Platform_21` enterprise-gap or `Platform_22` alert/AI-assurance authority.

The older files remain authoritative for their existing scope and are explicitly supplemented by specialized masters; they are not considered obsolete. This packaging choice is deliberate change control, not a waiver of traceability.

### 4.3 2026-09-19 NOC troubleshooting reconciliation

This v3.2.2 reconciliation adds a first-class **no-context-switch NOC troubleshooting experience** without adding a new Mother Reference file, mandatory database, mandatory standalone microservice or nested monitoring product. Existing browser CLI, Sensor/Agent diagnostics, Alert Evidence/Active Evidence Acquisition, NCM, Query/Graph/RCA and incident capabilities are composed into one governed operator workflow.

The approved scope includes persistent contextual action bars; a Troubleshoot drawer/workspace; embedded governed CLI; on-demand ICMP/traceroute/SNMP/TCP/DNS/SSH/API/domain tests; multi-vantage verification; deterministic `Verify Now`; Live Evidence distinguishing monitored state from fresh operator-triggered verification; `Why?`, `What Changed?` and `Compare`; Investigation Sessions; deterministic diagnostic packs; and risk-proportionate controls. Platform_22 remains authoritative for planner-selected Active Evidence Acquisition/AI investigation safety, and Platform_21 remains authoritative for the broader Incident Command Workspace.

This reconciliation updates existing base/specialized masters and keeps the authoritative set at **25 files** and version **v3.2.2**.

### 4.4 2026-09-19 NOC integration-audit closure

A post-merge cross-layer audit of the NOC troubleshooting reconciliation identified four documentation-integration defects: the API/event/data-contract master did not yet define the diagnostic/investigation contracts; the sizing master did not explicitly account for interactive diagnostics and multi-vantage fan-out; Interface Detail and RCA/Investigation screen contracts retained weaker legacy action lists; and the phrase **diagnostic pack** could be misread as a second packaging framework beside the single Capability Pack architecture.

This corrective reconciliation closes those defects without adding product scope, a new database, a new mandatory service, a new pack framework or a new dependency. `Platform_17` owns the versioned API/event/data contracts; `Platform_11` owns interactive-diagnostic capacity accounting; `Platform_09` owns explicit screen behavior; and `Platform_23` clarifies that a diagnostic workflow definition is not a second Capability Pack/registry/SDK.

### 4.5 2026-09-19 endpoint diagnostics and local-intelligence reconciliation

This v3.2.2 reconciliation formalizes comprehensive Windows/Linux infrastructure monitoring and a bounded **Endpoint Diagnostic Snapshot (EDS)** without adding a new Mother Reference file, mandatory endpoint interpreter, mandatory AI appliance, new causal authority or second diagnostics framework.

EDS is the product-facing name for a specialized diagnostic type/profile (`ENDPOINT_DIAGNOSTIC_SNAPSHOT`) of the existing Diagnostic Job / Live Evidence / Investigation Session architecture. It does not create a second diagnostic registry, scheduler, authorization system, investigation state machine or evidence store.

`Platform_23` owns separate Agent-based versus agentless/remote infrastructure coverage and certification, including CPU/resource pressure, memory/swap, filesystem/inode, block I/O, NIC, process, service and applicable OS/event/hardware evidence. `Platform_22` owns EDS capture-eligibility/evidence semantics and tiered local intelligence: deterministic/model-free endpoint logic, bounded Core analytics and optional LLM/embedding assistance behind AI Gateway. No LLM/GNN/foundation-model runtime is placed on normal monitored Agents or Sensors for EDS.

An optional dedicated local/private inference provider is a **conditional** compute-isolation boundary: **if** that provider mode is supported/enabled, its isolation/security/non-authority requirements are mandatory. It is not required to deliver Linux/Windows monitoring, SSH/WinRM/WMI/CIM, EDS, deterministic Core RCA or normal Platform operation. It is not a third Core, owns no authoritative Platform state and may fail or be absent without affecting deterministic monitoring/RCA. Existing on-Core local inference remains optional and allowed only inside the certified `Platform_11` non-interference envelope. All-AI-disabled remains a supported deterministic mode.

This reconciliation updates existing base/specialized masters and keeps the authoritative set at **25 files** and version **v3.2.2**. No `Platform_24` / `Platform_25` document is introduced.

### 4.6 2026-09-20 enterprise SNMP correctness and usability reconciliation

This v3.2.2 reconciliation strengthens the existing SNMP product contract **without adding a new Mother Reference file, database, protocol engine, microservice or extensibility framework**. SNMP remains governed through the existing base masters: `Platform_02` product behavior; `Platform_03` architecture; `Platform_04` subsystem ownership; `Platform_05` concrete SNMP/data contracts; `Platform_06` dataflows; `Platform_09` UI/UX; `Platform_10` protocol/security/credential rules; `Platform_11` capacity; `Platform_14` quality/certification; `Platform_17` API/event/data ownership; `Platform_18` threat model; `Platform_22` evidence-qualified alerting; and `Platform_23` Capability Pack certification.

The reconciliation closes correctness gaps around exact Counter64 handling, typed SNMP evidence, counter wrap/reset/discontinuity and rate semantics, stable interface identity, production scheduler/runtime wiring, governed credential resolution, adaptive/bulk poll planning, trap/inform durable-acceptance semantics and storm control, MIB/Capability Pack execution, customer SNMP diagnostics and privileged separation of SNMP SET from routine polling.

The normal scheduled SNMP **operational metric** default is **5 minutes**. That default does **not** require polling every available OID every five minutes; collection remains monitor-definition / Capability-Pack driven. Availability/liveness remains a separate plane and may normally use approximately 60 seconds according to policy. Event/streaming telemetry remains event/stream driven. Broader cross-technology freshness/cost scheduling architecture is out of scope for this reconciliation.

The authoritative file count remains **25 files**, the highest merged specialized master remains `Platform_23`, and the product-document version remains **v3.2.2**. `SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED`. Specification does not imply implementation or certification.

### 4.7 2026-09-21 final handoff architecture reconciliation

This v3.2.2 documentation baseline freezes implementation-honest architecture before control-plane handover. It does **not** add a specialized master, runtime feature, migration, dependency, datastore or mandatory microservice.

Closed-domain reconciliation decision: Flow/NTA, Log/SIEM, Cloud/full-stack, Extensibility/API Studio/Webhook and ITSM/Automation requirements remain inside existing owners (`REQ-NET-*`, `REQ-INF-*`, `REQ-EXT-*`, `REQ-INT-*` and related PRD/Architecture sections). **SPECIALIZED MASTER NOT REQUIRED AT THIS BASELINE** for those domains. This does not permanently prohibit a later master if future scope genuinely justifies one under PR #78 number-allocation governance.

Founder-fixed **Display Time Zone** behavior is specified as UTC source-of-truth instants plus per-user IANA display preference with explicit precedence and strict separation from operational-policy timezones. Generic recurring **scheduling/freshness** architecture extends Core placement/Sensor scheduler/Capability Pack contracts without a scheduler microservice. Exact Counter64 remains **SPECIFIED / P0 / NOT END-TO-END IMPLEMENTED**. Exactly-two-Core Active/Standby HA remains **SPECIFIED / P0 / NOT PRODUCTION CERTIFIED**. Sensor production-role direction reconciles NETWORK/INFRASTRUCTURE/FLOW/LOG with conditional SYNTHETIC/CLOUD_API and lab COMBINED; runtime `STREAMING_EVENT` requires implementation reconciliation and is not the final product substitute for FLOW/LOG.

PR #81 telemetry durability (durable stream identity + reserve→Kafka→finalize→ACK) is canonical **at-least-once**. Concurrent HTTP retry amplification is recorded as P1 hardening. Old PR #59 insert-token/dedupe-window correctness is superseded; only bounded Kafka→ClickHouse micro-batching remains as future P1 performance work after benchmark.

Authoritative file count remains **25**, highest specialized master remains `Platform_23`, product-document version remains **v3.2.2**. `SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED`.

### 4.8 2026-09-21 unified RCA and regional Sensor/provisioning reconciliation

This v3.2.2 reconciliation independently challenged the external **Unified Correlation & RCA** and **Core Efficiency / Regional Sensor / Provisioning / Transport** handoffs against current canonical architecture.

Accepted RCA deltas strengthen the existing `Platform_22` authority: one unified RCA engine over the canonical temporal graph; explicit causal roles/hypotheses; immutable progressive revisions; bounded counterfactual and recovery validation; redundancy/shared-risk semantics; progressive RCA UI; deterministic cross-domain/fault-injection certification and tenant-scoped human ground truth. Existing evidence qualification, contradictions, `INCONCLUSIVE`, Flight Recorder, Similar Incident, GNN/LLM boundaries and graph architecture remain authoritative rather than being duplicated.

Accepted Sensor/provisioning deltas strengthen the existing `Platform_20` onboarding/control family: optional customer-defined Region, first-class Sensor Pools, Core-authoritative capacity/locality placement, generation/fencing, short-lived single-use Sensor/Agent bootstrap, endpoint-local key generation, normal Agent→Sensor→Core telemetry and explicit bootstrap endpoint/certificate validation.

The proposed plaintext Platform transport direction was **rejected** for production Platform-owned Sensor↔Core and Agent↔Sensor trust boundaries. Existing authenticated encrypted transport/mTLS requirements remain. Legacy insecure monitored-device protocols remain separately governed compatibility modes and cannot downgrade Platform-owned channels.

The resource-efficiency principle is accepted as an engineering constraint: optimize measured algorithms/data paths/queries/schemas/batching/unnecessary work before increasing certified hardware, without weakening correctness, durability, security or freshness.

Implementation truth is also reconciled to verified current `main`: PR #84 established the exact raw Counter64 Sensor→Core→Kafka→ClickHouse path as **IMPLEMENTED+TESTED**. Derived counter delta/rate/reset/wrap/discontinuity behavior remains **SPECIFIED but NOT IMPLEMENTED/CERTIFIED**.

This reconciliation updates existing Mother References and keeps the authoritative set at **25 files**, version **v3.2.2**, highest specialized master **Platform_23**. No `Platform_24` is introduced.
