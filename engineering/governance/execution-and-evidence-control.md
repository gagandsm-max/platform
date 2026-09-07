# Platform Engineering Execution and Evidence Control

**Status:** NON-MOTHER — OPERATIONAL ENGINEERING CONTROL  
**Applies to:** Platform v1 engineering programme  
**Authoritative Mother Reference:** Platform v3.2.1  
**Canonical system of record:** GitHub repository `gagandsm-max/platform`  
**Purpose:** control engineering sequence, maturity claims, evidence, merge/release gates, and visibility of unresolved P0/TBD decisions.

> This file is not part of the Mother Reference and cannot override it. It exists to prevent documentation coverage, branch code, local tests, spikes, or calendar progress from being misrepresented as implementation completeness or GA certification.

## Reference set

This control is subordinate to the authoritative Mother Reference, especially:

- [`../../AGENTS.md`](../../AGENTS.md)
- [`../../project_documentation/Platform_00_Document_Index.md`](../../project_documentation/Platform_00_Document_Index.md)
- [`../../project_documentation/Platform_01_BRD.md`](../../project_documentation/Platform_01_BRD.md)
- [`../../project_documentation/Platform_02_PRD.md`](../../project_documentation/Platform_02_PRD.md)
- [`../../project_documentation/Platform_03_Architecture.md`](../../project_documentation/Platform_03_Architecture.md)
- [`../../project_documentation/Platform_04_HLD.md`](../../project_documentation/Platform_04_HLD.md)
- [`../../project_documentation/Platform_05_LLD.md`](../../project_documentation/Platform_05_LLD.md)
- [`../../project_documentation/Platform_06_Dataflow.md`](../../project_documentation/Platform_06_Dataflow.md)
- [`../../project_documentation/Platform_07_OSS_Catalog.md`](../../project_documentation/Platform_07_OSS_Catalog.md)
- [`../../project_documentation/Platform_08_Requirements_Traceability.md`](../../project_documentation/Platform_08_Requirements_Traceability.md)
- [`../../project_documentation/Platform_10_Protocol_Security_Connector_Master.md`](../../project_documentation/Platform_10_Protocol_Security_Connector_Master.md)
- [`../../project_documentation/Platform_11_Sizing_Capacity_Storage_Master.md`](../../project_documentation/Platform_11_Sizing_Capacity_Storage_Master.md)
- [`../../project_documentation/Platform_12_Decision_Source_Register.md`](../../project_documentation/Platform_12_Decision_Source_Register.md)
- [`../../project_documentation/Platform_13_Engineering_Development_and_Docker_Standards.md`](../../project_documentation/Platform_13_Engineering_Development_and_Docker_Standards.md)
- [`../../project_documentation/Platform_14_Test_Performance_and_Release_Quality.md`](../../project_documentation/Platform_14_Test_Performance_and_Release_Quality.md)
- [`../../project_documentation/Platform_15_CI_CD_Packaging_and_Distribution.md`](../../project_documentation/Platform_15_CI_CD_Packaging_and_Distribution.md)
- [`../../project_documentation/Platform_16_Installation_Upgrade_Rollback_and_Operations.md`](../../project_documentation/Platform_16_Installation_Upgrade_Rollback_and_Operations.md)
- [`../../project_documentation/Platform_17_API_Event_and_Data_Model_Contracts.md`](../../project_documentation/Platform_17_API_Event_and_Data_Model_Contracts.md)
- [`../../project_documentation/Platform_18_Security_Threat_Model.md`](../../project_documentation/Platform_18_Security_Threat_Model.md)

---

## 1. Authority and precedence

Platform Mother Reference v3.2.1 remains the architectural and product authority.

This document controls only:

- engineering sequence;
- evidence recording;
- implementation maturity;
- validation maturity;
- merge gating;
- release/GA evidence gating.

This document cannot:

- change a product requirement;
- override the BRD, PRD, Architecture, HLD, LLD, Dataflow, specialized masters, or Decision Register;
- close a Mother P0/TBD item by declaration;
- change a USER-FIXED or otherwise fixed decision;
- defer agreed v1 scope;
- create or alter commercial commitments;
- claim certification that has not independently been achieved;
- declare GA without the evidence required by the Mother quality/release contracts.

If this file conflicts with the Mother Reference, **the Mother Reference wins** and this file must be corrected through normal GitHub change control.

GitHub is the canonical system of record. Local work, agent output, unpushed commits, or unpublished artifacts are not authoritative repository state.

---

## 2. RTM semantics: documentation coverage is not implementation state

[`Platform_08_Requirements_Traceability.md`](../../project_documentation/Platform_08_Requirements_Traceability.md) is an authoritative **requirements traceability and completeness** control.

A `✓` in that matrix means:

> **The requirement is represented/mapped in that documentation layer.**

It does **not** mean that the requirement is:

- implemented;
- committed;
- merged;
- tested;
- integration validated;
- failure validated;
- benchmark certified;
- production ready;
- GA ready.

The RTM itself states that it maps requirement families across the Mother documents and treats a missing implementation mapping as a **documentation defect**, not permission to ignore the requirement. The checkmarks are therefore documentation coverage evidence, not software-delivery evidence.

No agent may use RTM coverage alone to promote implementation or validation maturity.

---

## 3. Three independent maturity dimensions

Every material work item may be tracked independently across specification, implementation, and validation. Progress in one dimension does not imply progress in another.

### 3.1 Specification maturity

| Value | Meaning |
|---|---|
| `SPECIFIED` | Normative behavior is sufficiently defined for implementation without inventing a load-bearing decision. |
| `PARTIALLY_SPECIFIED` | Some implementation detail remains unresolved, but engineering can proceed safely around the unresolved boundary without pretending to resolve it. |
| `TBD_P0` | A load-bearing engineering/business decision remains open. Agents may spike/test candidates but must not invent or freeze the answer. |

### 3.2 Implementation maturity

| Value | Meaning |
|---|---|
| `NOT_STARTED` | No verified implementation exists for the tracked item. Prose, plans, generated snippets, or spike notes do not count. |
| `IN_DEVELOPMENT` | Work is underway but incomplete or experimental. |
| `BRANCH_IMPLEMENTED` | The intended implementation exists in a pushed branch/PR but is not authoritative `main`. |
| `MERGED_MAIN` | The implementation has been merged into canonical `main`. |

`BRANCH_IMPLEMENTED` is **not** `MERGED_MAIN`.

### 3.3 Validation maturity

| Value | Meaning |
|---|---|
| `NOT_VALIDATED` | No reproducible validation evidence has been verified. |
| `SOURCE_ONLY` | Source review, static reasoning, or a source-development harness exists. This is useful defect prevention but **not production evidence**. |
| `PINNED_MODULE` | Relevant tests/static checks were executed against the repository's pinned language/module dependency graph and recorded with exact commands/results. |
| `DOCKER_LOCAL` | Relevant builds/tests were executed in the required Docker/local integration environment with exact source SHA, commands, and results. A Docker spike remains a spike unless the test scope is production-equivalent. |
| `INTEGRATION_VALIDATED` | Relevant real component/database/protocol/API interactions were executed together with reproducible evidence and contract/data-integrity checks. |
| `FAILURE_VALIDATED` | Applicable negative/failure/fault-injection scenarios were executed with required safety and recovery behavior verified. This does not imply performance certification. |
| `PERFORMANCE_CERTIFIED` | A versioned benchmark profile, hardware/environment, workload, thresholds, raw results, and regression comparison satisfy the applicable performance acceptance contract. |
| `GA_CERTIFIED` | All applicable Day-30 release evidence and Mother GA gates are complete, with zero unresolved GA blockers. |

These values are not automatically sequential for every feature; the applicable validation path depends on risk and subsystem.

Critical interpretation rules:

- `SOURCE_ONLY` is not production test evidence.
- `DOCKER_LOCAL` is not `GA_CERTIFIED`.
- A Docker HA spike is not production HA certification.
- A passing unit test is not an integration/failure/performance certification.
- No validation maturity may be inferred from implementation maturity.

---

## 4. Required evidence

No row may advance maturity based only on prose or an agent assertion.

Applicable evidence must be reproducible and identify, where relevant:

- exact commit SHA;
- PR number;
- merge SHA;
- branch/base SHA;
- exact command or test invocation;
- exact test suite/test case;
- executed test count and skipped/failed/error counts where available;
- result and exit status;
- toolchain/module/container version;
- environment/hardware where material;
- benchmark profile and raw artifact;
- failure-injection scenario and observed recovery/safety result;
- security/tenant-isolation review evidence;
- migration/upgrade/rollback evidence;
- package/install/reboot evidence;
- external certification/lab environment where applicable;
- known limitations and unresolved blockers.

Never accept an unsupported statement such as `agent says PASS` as evidence.

A PR description is useful traceability, but high-risk evidence should also be reproducible from committed scripts/results or independently rerunnable commands.

A maturity claim must point to the evidence that justifies it.

---

## 5. Completion rule

Code existing is not completion.

A production feature is complete only when the applicable concerns are addressed and evidenced:

- implementation;
- deterministic validation;
- negative/error handling;
- security;
- tenant isolation;
- data integrity and replay/idempotency where relevant;
- telemetry/observability;
- configuration and secure defaults;
- capacity/resource bounds;
- failure behavior and recovery;
- migration/upgrade/rollback impact;
- packaging/install impact where relevant;
- documentation/traceability;
- operational/runbook behavior;
- performance certification where the requirement is performance-bound.

A feature that compiles but lacks applicable validation, failure behavior, observability, security, upgrade impact, or operating evidence remains incomplete.

---

## 6. Approved 30-day GA programme

This is the approved full-v1 engineering delivery sequence. It does not defer agreed scope and does not guarantee GA by calendar date.

| Day | Scheduled scope |
|---:|---|
| 01 | Mother merge, clean engineering baseline, Test20/P0/test foundation. |
| 02 | Tenancy, MSP model, sites, groups, tags, canonical entity IDs, model, PostgreSQL migrations, API contracts. |
| 03 | Identity: Keycloak/local/OIDC/SAML/LDAP, MFA, Platform RBAC/resource authorization, USE vs REVEAL, audit. |
| 04 | Credential Profiles, PKI, Sensor/Agent enrollment, registry, pools. |
| 05 | Policy hierarchy, CEL, versioning, rollback, preview, maintenance. |
| 06 | Provisioning, bulk operations, diagnostics, placement, MWU, discovery framework. |
| 07 | Auto-discovery. |
| 08 | Universal Sensor scheduler, worker pools, leases/fencing, spool/replay, adaptive polling, Sensor HA. |
| 09 | ICMP/TCP/DNS/TLS/HTTP synthetics. |
| 10 | SNMP. |
| 11 | Interfaces, optics, VLAN, STP, LACP, ARP, FDB, CDP, LLDP. |
| 12 | BGP, OSPF, IS-IS, BFD, HSRP, VRRP, VRF, MPLS. |
| 13 | VXLAN, EVPN, SD-WAN, VPN, QoS, multicast, IP SLA, WLAN, CUCM. |
| 14 | Flow. |
| 15 | Syslog/Event. |
| 16 | Infrastructure Sensor: Linux SSH, Windows WinRM/WMI, Linux Agent. |
| 17 | Windows/Linux Agents. |
| 18 | Hardware, virtualization, storage, SAN, environmental monitoring. |
| 19 | Database monitoring. |
| 20 | Cloud, Kubernetes, containers, OTel. |
| 21 | NCM. |
| 22 | IPAM. |
| 23 | Alert qualification, anomaly detection, correlation, ITSM. |
| 24 | Operational graph and deterministic/causal RCA. |
| 25 | GNN + Gemini-assisted operations. |
| 26 | Studios and all 98 workspaces. |
| 27 | SLA, reporting, advanced formulas, scheduled exports, ITSM, EOL, licensing. |
| 28 | Production two-Core HA, exact PostgreSQL/ClickHouse/Kafka architecture, fencing, DR, upgrade, rollback, storage. |
| 29 | GA packages: DEB/RPM, Windows EXE/MSI, offline distribution, SBOM, security. |
| 30 | Full commercial GA certification: SNWP-2500; Flow/Syslog load; security; soak; chaos; performance; UI; regression; release tag. Zero GA blockers required. |

Sales may begin Day 31 only if the actual GA gates pass.

The calendar is a programme target, not evidence. If Day-30 gates do not pass, GA has not occurred.

---

## 7. Out-of-sequence engineering

The day number is delivery sequencing, not a prohibition on early risk-retirement work.

High-risk or dependency-heavy work may be pulled forward when doing so reduces programme risk. Examples already observed include:

- Sensor scheduler/SNMP/Syslog work being developed before its nominal delivery day;
- a two-Core HA P0 spike being intentionally executed before Day 28.

Rules:

1. Early work must still obey Mother Reference and branch isolation.
2. Early branch/spike work does not change the scheduled day's final acceptance criteria.
3. A spike does not become a fixed production architecture without the required decision/evidence process.
4. Early implementation does not imply integration, failure, performance, release, or GA maturity.
5. Out-of-sequence work must not silently defer the nominal day's remaining scope.

---

## 8. Current GitHub evidence snapshot

**Snapshot basis:** canonical GitHub state observed on 2026-09-07.  
**Latest timestamp among the active snapshot records:** `2026-09-07T09:45:00Z` (`PR #7 updated_at`).  
**Current `main` at snapshot:** `8592863e9a30ae80742e4d2da6b3417ae68a3b89`.

This table records evidence state; it does not replace subsystem-specific acceptance criteria.

| Work | Specification maturity | Implementation maturity | Validation maturity | Verified GitHub evidence | Current interpretation |
|---|---|---|---|---|---|
| Day 01 — engineering baseline/test foundation | `SPECIFIED` | `MERGED_MAIN` | `DOCKER_LOCAL` | PR #2 merged; merge SHA `7065d533c74befdcedb76035d9314dd3d642337a`. PR #2 records `make doctor`, `docker compose config -q`, unit, integration `20/20`, contract, security-smoke, full test and `git diff --check` PASS. Main contains [`../evidence/day-01/baseline.md`](../evidence/day-01/baseline.md), which explicitly says it is not product/performance certification. | Baseline/test foundation exists in main. No broader certification implied. |
| Day 02 — tenancy/canonical entity model | `SPECIFIED` | `MERGED_MAIN` | `INTEGRATION_VALIDATED` | PR #6 merged at `8592863e9a30ae80742e4d2da6b3417ae68a3b89` on `2026-09-07T09:01:04Z`. PR evidence records PostgreSQL migration/tenant-isolation tests, explicit Day-02 database gate, `make test-integration` `20/20`, contract/security/full test, plus migration rollback/concurrency/failure cases. | Day-02 implementation is authoritative main; Day-03 identity/RBAC remains explicitly not implemented. |
| Sensor continuation | `PARTIALLY_SPECIFIED` | `BRANCH_IMPLEMENTED` | `SOURCE_ONLY` | PR #5 OPEN/DRAFT/UNMERGED; base `main`; head `3cae0349dc7a8ac97da603b05a9ffa6be4bb605f`. PR #5 states post-sync real local Docker validation is pending. | Branch work may be reviewed as implementation, but it is not main and cannot be called production validated. Durable Core ACK architecture remains P0. |
| Two-Core HA spike | `TBD_P0` | `IN_DEVELOPMENT` | `DOCKER_LOCAL` | PR #7 OPEN/DRAFT/UNMERGED; head `a8faf3ef37278df6b731e7047999fb23aeb46343`. Committed spike result identifies evidence run SHA `96f93345a3a21701f140e889dbd9655e0184b5b8`, labels Kafka/ClickHouse tests as lab/executed evidence, P0-03 as NOT TESTED, fencing as model/lab analogue, and explicitly says the spike does not certify production Core HA `<60s`. | Experimental evidence only. P0-01/P0-02/P0-04 remain open; P0-03 remains unresolved and untested. |

Rules for this snapshot:

- A draft PR can be `BRANCH_IMPLEMENTED` only for code actually present on that branch.
- An open spike PR does not become production architecture merely because a Docker experiment passed.
- This snapshot must be refreshed from GitHub before any future maturity promotion.

---

## 9. Ten open P0/TBD items

All ten items below remain **OPEN** under [`Platform_12_Decision_Source_Register.md`](../../project_documentation/Platform_12_Decision_Source_Register.md) §5. None is closed by this document.

| ID | Subject | Specification maturity | Implementation maturity | Validation maturity | Current evidence | Required closure evidence |
|---|---|---|---|---|---|---|
| P0-01 | Exact two-Core Kafka implementation/failover/durable ACK architecture. | `TBD_P0` | `IN_DEVELOPMENT` | `DOCKER_LOCAL` | PR #7 contains a two-Core lab candidate and executed STRICT_DUAL/failover tests, but explicitly labels the result a spike and not certified Core HA. PR #5 still treats authoritative durable ACK as unresolved. | Formally approved exact two-Core Kafka/ACK design plus failure/integration evidence proving the certified durable boundary, replay behavior and no silent loss while honoring the two-full-Core constraint. |
| P0-02 | Exact two-Core ClickHouse replication/promotion/fencing architecture. | `TBD_P0` | `IN_DEVELOPMENT` | `DOCKER_LOCAL` | PR #7 contains ClickHouse lab candidate evidence, including DDL partial-failure/reconcile tests, while production resync/fencing remains pending. | Formally approved exact two-Core ClickHouse design with required failover, promotion/fencing, divergence/resync, duplicate/replay and recovery evidence. |
| P0-03 | Exact PostgreSQL two-Core failover/fencing/rewind design. | `TBD_P0` | `NOT_STARTED` | `NOT_VALIDATED` | PR #7 explicitly records PostgreSQL P0-03 as `NOT TESTED`; Day-02 PostgreSQL work is schema/migration/domain work, not two-Core HA closure. | Formally approved primary/standby failover/fencing/rewind mechanics with the required failover, resynchronization, integrity and data-loss-window evidence. |
| P0-04 | Exact Core fencing/VIP failure matrix. | `TBD_P0` | `IN_DEVELOPMENT` | `DOCKER_LOCAL` | PR #7 contains a fencing state-machine/model and Docker kill/stop lab analogue; it explicitly says production fencing remains pending/high risk and the timing is not certified Core HA. | Formally approved fencing/VIP implementation and failure matrix with evidence that ambiguous partitions cannot create dual-active authority and that certified HA scenarios meet the Mother failover requirement. |
| P0-05 | Final contractual DR RPO/RTO/data-loss objectives. | `TBD_P0` | `NOT_STARTED` | `NOT_VALIDATED` | Mother Reference requires DR capability but keeps contractual RPO/RTO/data-loss objectives TBD. No main evidence closes them. | Approved contractual objectives followed by DR/backup/restore/failure evidence demonstrating the agreed objectives. |
| P0-06 | Final SNWP-2500-v1 constants and certification. | `TBD_P0` | `NOT_STARTED` | `NOT_VALIDATED` | Sizing/quality masters define the 8 vCPU/16 GB target and require a versioned workload profile; Day-01 evidence explicitly says it is not SNWP-2500 certification. | Frozen versioned SNWP-2500-v1 workload constants plus benchmark artifacts/results certifying the applicable Core target under the defined environment. |
| P0-07 | Flow records/sec and Syslog EPS certification. | `TBD_P0` | `NOT_STARTED` | `NOT_VALIDATED` | Mother sizing/quality documents state Streaming/Event Sensor rates are benchmark-defined. PR #5 contains branch Syslog work but no certified Flow/Syslog throughput evidence. | Versioned Streaming/Event workload and benchmark artifacts/results certifying Flow records/sec and Syslog EPS with resource/backpressure/drop behavior. |
| P0-08 | Exact GNN model/training/runtime/compute design. | `TBD_P0` | `NOT_STARTED` | `NOT_VALIDATED` | Mother Reference identifies GNN runtime/model/compute/enablement as an open P0; no main evidence closes it. | Approved model architecture/training labels/runtime/compute/enablement threshold plus evaluation, resource and inference evidence required by the Mother AI/RCA quality constraints. |
| P0-09 | Final compliance certification programme/scope. | `TBD_P0` | `NOT_STARTED` | `NOT_VALIDATED` | Mother Reference requires readiness and prohibits false certification claims; no main evidence records an achieved final programme/certification. | Approved certification programme/scope and independently achieved certification evidence for any claimed certification. |
| P0-10 | Exact commercial licensing SKUs/quantities beyond fixed behavior. | `TBD_P0` | `NOT_STARTED` | `NOT_VALIDATED` | Fixed licence-expiry behavior exists in Mother Reference; exact product SKUs/quantities remain explicitly open. | Product-owner/business approval of the commercial SKU/quantity model and corresponding product/contract updates; engineering must not invent this decision. |

A P0 may accumulate promising evidence while remaining `TBD_P0`. One synthetic, unit, model, or Docker-lab run cannot close a P0 unless the Mother-required decision and closure evidence are actually complete and formally recorded.

---

## 10. Merge and quality gates

Default change progression:

`Feature branch → source review → pinned/local validation where applicable → independent review for high-risk work → merge → integration validation → failure/performance gates as applicable → GA certification`

A branch may require additional loops before merge. Merge does not imply GA.

### 10.1 Merge evidence

Before merge, applicable evidence includes:

- correct authoritative base;
- bounded scope and reviewed diff;
- relevant deterministic tests;
- pinned dependency/build verification;
- security/OSS review;
- tenant-isolation review where applicable;
- migration/rollback analysis;
- data-integrity/replay analysis;
- explicit unresolved blockers;
- no hidden/skipped mandatory tests.

### 10.2 Critical blockers

The following prohibit merge or release when applicable to the change/release:

- tenant isolation failure;
- authentication/authorization/security defect;
- secret exposure;
- destructive or unsafe migration;
- data-integrity failure;
- unsafe ACK/replay/deletion semantics;
- split-brain or unfenced dual-active risk in production HA path;
- unsupported telemetry loss;
- unbounded resource behavior;
- failed mandatory tests;
- hidden/skipped mandatory tests represented as passing;
- unresolved critical/high security issue without the Mother-required approved disposition;
- broken package/upgrade/rollback gate;
- GA performance regression or missing certification evidence where required.

Schedule pressure cannot override a safety/quality gate.

---

## 11. Parallel engineering rule

Platform currently uses parallel engineering lanes with distinct roles:

| Lane | Primary use |
|---|---|
| ChatGPT + GitHub | Small isolated implementation/documentation checkpoints with direct canonical-repository traceability. |
| Antigravity | Broad local integration, Docker, multi-module work, empirical labs and full environment validation. |
| Cursor | Adversarial/surgical review, difficult debugging and independent verification. |
| Codex/Desktop | High-leverage independent review, debugging and P0 work when execution quota is available. |
| GitHub | Canonical system of record for branches, PRs, commits, evidence and merged authority. |

Critical collision rule:

> Two agents must not simultaneously edit the same subsystem, branch, or files.

Before writing, an agent verifies current GitHub branch/PR state. Unexpected branch movement is treated as a concurrency signal and must be reconciled before continuing.

Parallel work must use isolated branches unless a task explicitly owns an existing shared branch.

---

## 12. Tool-quota discipline

Limited/high-cost reasoning environments are reserved for work where their additional capability materially reduces engineering risk, especially:

- high-risk architecture;
- independent release review;
- hard debugging;
- security issues;
- tenant/data-integrity issues;
- HA/fencing/DR reasoning;
- difficult performance or P0 analysis.

Routine implementation, deterministic edits, and normal test execution should use cheaper/available lanes when quality is equivalent.

Do not record subscription pricing, account-specific quotas, or private account constraints in repository governance.

---

## 13. Status update rules

Any future agent updating this file must:

1. verify canonical GitHub state first;
2. verify `main` SHA and relevant PR head/base/state;
3. cite exact SHA/PR evidence in this file for every maturity promotion;
4. never promote `SOURCE_ONLY` to tested/integration/performance maturity without reproducible execution evidence;
5. never promote `BRANCH_IMPLEMENTED` to `MERGED_MAIN` before the merge exists in canonical GitHub;
6. never close a P0 from one synthetic/model/lab run;
7. never mark GA without Day-30 evidence satisfying the Mother release gates;
8. never erase a historical blocker without recording the resolution evidence;
9. preserve unresolved P0/TBD visibility until Mother change control resolves it;
10. correct stale snapshots rather than silently reinterpret old states.

Keep history concise. This file is an operational control, not a daily diary.

Recommended evidence annotation for a maturity change:

```text
Work item:
Previous maturity:
New maturity:
Commit/PR/merge SHA:
Exact validation command/artifact:
Result:
Known limitations/blockers:
Recorded by/date:
```

---

## 14. No scope deferral

The approved Platform v1 programme targets the entire agreed v1 scope by Day 30.

This document has no authority to defer or remove agreed requirements, including:

- NCM;
- IPAM;
- cloud monitoring;
- Kubernetes/containers/OTel;
- GNN;
- all 98 planned workspaces;
- HA;
- DR;
- packaging/distribution;
- security/certification work;
- performance/load certification;
- any other Mother-defined v1 requirement.

If schedule pressure occurs, quality and safety gates remain authoritative.

Incomplete or unsafe work means the applicable milestone/GA gate fails. It does not become a silent scope deferral.

A calendar day, merged branch, or completed document never overrides missing production evidence.

---

## Operational summary

This control answers four questions:

1. **What is scheduled to be built each day?** — §6 records the approved 30-day full-v1 programme.
2. **What does implemented mean?** — §3 separates development, branch implementation, and authoritative main.
3. **What evidence is required before tested/complete claims?** — §§3–5 and §10 define validation levels and evidence gates.
4. **Which load-bearing P0/TBD items remain unresolved?** — §9 keeps all ten Mother P0s visibly open until formally resolved.

When in doubt, prefer the more conservative maturity state and point to the missing evidence. False completion is a governance defect.
