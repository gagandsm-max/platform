# Platform — Test, Performance, Benchmark & Release Quality Master

**Document ID:** PLATFORM-QUALITY-001
**Version:** 3.3.0
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform (internal engineering/build identifier) / AmrudAi (commercial/customer-facing product name)
**Consolidation rule:** This document preserves the normative content of the listed v3.1 source documents (via v3.2/v3.2.1). Consolidation reduces file count only; it does not relax, remove or downgrade any requirement. v3.2.2 adds specialized-master quality supplements in §6 without rewriting the base gates.

## 1. Purpose

Defines all test layers and quality gates, mandatory negative/failure tests, false-alert validation,
package/reboot testing, Core/Sensor performance certification, UI/HA/DR benchmarks, regression controls and the
Alpha/Beta/RC/GA acceptance criteria and evidence bundle.

## 2. Source-document provenance

- `Platform_15_Test_Strategy_and_Quality_Gates.md` (v3.1) — fully incorporated below.
- `Platform_22_Performance_and_Benchmark_Plan.md` (v3.1) — fully incorporated below.
- `Platform_24_Release_Acceptance_Criteria.md` (v3.1) — fully incorporated below.

## 3. Platform — Test Strategy & Quality Gates

> Consolidated from `Platform_15_Test_Strategy_and_Quality_Gates.md` v3.1. Requirements in this section remain normative.

## 1. Objective

Platform targets banks, defence and large enterprises; testing therefore validates correctness, failure behavior, security, recoverability, performance and low false-alert behavior—not only functional happy paths.

## 2. Test layers

1. Unit tests for deterministic logic.
2. Component tests for adapters/workers/services.
3. Contract tests for APIs/events/schemas.
4. Protocol tests using containerized simulators/mocks and selected real-device certification labs.
5. Integration tests across Kafka/PostgreSQL/ClickHouse/Valkey/Core/Sensor.
6. UI E2E tests.
7. Provisioning diagnostics tests.
8. HA/failover tests.
9. DR/backup/restore tests.
10. Security/tenant-isolation tests.
11. Performance/resource tests.
12. Upgrade/rollback/package tests.
13. Chaos/fault injection.
14. Windows Agent certification on isolated Windows runner/VM.

## 3. Mandatory negative tests

Wrong SNMP community/v3 user/auth/privacy, SNMP disabled, ACL/firewall drop, unreachable host, wrong API token, TLS failure, expired certificate, DNS failure, Kafka unavailable, ClickHouse unavailable, PostgreSQL failover, sensor disk pressure, Core loss, duplicate/replayed telemetry, malformed bulk import, high-cardinality explosion, clock skew, unauthorized/cross-tenant diagnostic target, arbitrary/ad-hoc target abuse, diagnostic rate/concurrency exhaustion, unbounded continuous test, cancellation/timeout failure, multi-vantage contradictory results, reused/expired diagnostic authorization, CLI session hijack, one-time/reusable credential leakage, command-policy bypass, privilege escalation/config-mode denial and missing diagnostic/CLI/investigation audit evidence.

## 3.1 NOC troubleshooting acceptance tests

UI E2E/integration suites shall verify: device/alert/incident context is preserved while Troubleshoot/embedded CLI is used; on-demand Ping/Trace/SNMP returns source/vantage and bounded results; multi-vantage disagreement remains visible; `Verify Now` returns per-check evidence; Live Evidence distinguishes monitored from on-demand state; Investigation Sessions retain evidence references; stored/personal/one-time CLI credential modes obey policy; low-risk read-only diagnostics avoid unnecessary friction while privileged/configuration commands enforce configured controls; and interactive load cannot starve certified monitoring workload.

## 4. False-alert quality

Qualification tests must prove that SNMP failure alone does not automatically declare device-down where secondary evidence shows device reachability. Flapping/storm/dependency correlation and recovery qualification require regression suites.

### 4.1 Availability Qualification Deadline

Architecture must **not** permit `No Data → wait indefinitely`, and must **not** encode `timeout → DEVICE_DOWN`.

By the applicable bounded **Availability Qualification Deadline**, Platform reaches the most truthful governed semantic state supported by evidence (for example `DOWN`, `MONITORING_DEGRADED`, `UNKNOWN`, `NO_DATA`, `STALE`, Sensor/path/site/platform reachability fault, or other monitor-specific governed state).

Deadline depends on applicable monitor class, configured cadence, criticality, topology, Sensor health, evidence diversity, active validation and policy.

Certification ownership: BUILD-080 / BUILD-082 (with supporting Sensor/path evidence from network/infrastructure builds). Minimum certification scenarios: actual powered-off device; SNMP-only failure; credential failure; Sensor failure; Core connectivity failure; site WAN isolation; upstream router failure; partial multi-vantage failure.

Measure at least: time-to-evidence-loss; time-to-qualified-semantic-state; time-to-incident where applicable; false-DOWN result/rate.

Preserve: `No Data != Device Down`; monitoring-path failure != monitored-object Down.

## 5. Packaging/reboot tests

Every `.deb`/`.rpm` package is installed on clean certified OS images, rebooted, upgraded, rolled back where supported and uninstalled. Certification must prove that every mandatory runtime service for the enabled profile auto-starts, reaches dependency-aware readiness and retains configuration/state after reboot without routine operator start commands. Core certification explicitly covers separately managed PostgreSQL, ClickHouse, Kafka and Valkey services where applicable. Fault-injection tests shall terminate each mandatory process and prove bounded supervised restart, state/config continuity, dependency-safe recovery and visible crash-loop diagnostics when repeated failure is forced. Recovery certification must also prove that automatic restart preserves authority/role/fencing/replication/durability/rejoin contracts: no Standby self-promotion, no dual-active authority, no bypass of fencing or data-service safe-rejoin rules, no treating process existence as success, and explicit FAILED/DEGRADED/RECOVERING plus actionable evidence/runbook when safe automatic recovery cannot be established. Windows Agent MSI/BAT install, reboot auto-start and SCM recovery behavior are tested on Windows runners. Optional/on-demand workers are tested for correct activation when required rather than permanent residency.

## 6. Quality gates

No release promotion with failing P0/P1 tests, unresolved critical/high security defects without approved exception, tenant-isolation failure, data-loss defect in certified normal failure scenarios, failed upgrade/rollback gate, broken SBOM/license gate or performance regression beyond approved threshold.

## 4. Platform — Performance & Benchmark Plan

> Consolidated from `Platform_22_Performance_and_Benchmark_Plan.md` v3.1. Requirements in this section remain normative.

## 1. Core target

≤2,500 reference network devices on 8 vCPU / 32 GB Core is a mandatory engineering target under a versioned Standard Network Workload Profile, not a blanket guarantee for arbitrary Flow/log/high-cardinality workloads.

## 2. Sensor targets

Planning baseline 4 vCPU/16 GB/150 GB ≈800 normal network devices. Benchmark objective: 8 vCPU/16 GB class approaching 2,000–2,500 comparable normal devices where workload permits. Streaming/Event Sensor benchmarked separately by records/events per second.

## 3. Benchmark dimensions

Devices, interfaces, metrics, poll interval, SNMP version/security, RTT, timeouts, traps/events, flow rate, syslog EPS, gNMI subscriptions, concurrent users, dashboard queries, reports, RCA jobs and storage retention.

## 4. UI targets

Measure navigation shell, dashboard first meaningful render, common query latency, search suggestions, alert action acknowledgment, large table interaction and concurrent NOC users. Exact GA SLOs are frozen from benchmark evidence.

## 5. HA/DR

Measure Core failover (<60s requirement), sensor uninterrupted collection/buffering, backlog replay time and correctness. DR measures RPO/RTO once contractual values are frozen.

## 6. Regression gate

Each release compares CPU/RAM/disk/query/poll/ingestion metrics with baseline. Material regression requires documented approval or remediation before GA.

## 5. Platform — Release Acceptance Criteria

> Consolidated from `Platform_24_Release_Acceptance_Criteria.md` v3.1. Requirements in this section remain normative.

## 1. Alpha

Core workflows function in Docker test environment; schemas/contracts versioned; no known architecture violation; basic package builds available; critical diagnostics/logging implemented.

## 2. Beta/Pilot

Representative protocols/vendors certified; provisioning diagnostics actionable; two-Core failover tested; package install/reboot/upgrade tested; security/tenant isolation passed; SBOM/license gates clean; backup/restore exercised.

## 3. Release Candidate

Full regression, performance certification, HA/DR evidence, migration/upgrade matrix, `.deb/.rpm` install tests, Windows MSI/service certification, signed artifacts, vulnerability disposition, documentation and support runbooks complete.

## 4. GA blockers

- tenant isolation defect;
- known critical security vulnerability without exceptional approved mitigation;
- false device-down behavior caused by single primary-protocol failure contrary to qualification rules;
- uncontrolled storage exhaustion behavior;
- broken reboot auto-start or supervised crash recovery for any mandatory runtime service;
- package upgrade that loses configuration/data;
- missing SBOM/notices/provenance;
- HA failover violating agreed <60s target under certified test profile;
- unsupported silent data loss in certified normal failure scenarios;
- unresolved RED OSS dependency.

## 5. Evidence

Every GA build has a release evidence bundle containing test results, benchmark summary, package hashes/signatures, SBOMs, OSS notices, vulnerability scan, upgrade/rollback results, HA/DR results and known limitations.

## 6. v3.2.2 specialized-master quality supplements

Generic test layers and Alpha/Beta/RC/GA gates above remain the base quality contract. Family-specific gates live in the owning specialized masters and are not duplicated here.

Capability Pack certification, when a pack is implemented, requires evidence appropriate to:

- functional behavior;
- version compatibility;
- malformed/partial responses;
- timeouts;
- authentication failures;
- rate limiting;
- stale telemetry;
- tenant isolation;
- least privilege;
- resource consumption;
- upgrades;
- rollback/migration;
- deprecation/EOL.

Customer Acceptance Profile results must be evidence-backed. A technology listed in `Platform_23` is not certified by specification.

Specified ≠ implemented. Implemented ≠ tested. Tested ≠ certified. Certified ≠ deployed.

OIV, onboarding, enterprise-gap, monitoring-intelligence and enterprise-monitoring-coverage class-specific gates remain in `Platform_19`, `Platform_20`, `Platform_21`, `Platform_22` and `Platform_23` respectively.

UI/OOB-manifest and recurring-Discovery quality evidence additionally covers, where implemented:

- schema validation of System dashboard/workspace manifests and failure on unknown query/metric/action references;
- proof that every shipped OOB panel resolves to authorized query/data contracts and renders Permission Restricted / Not Licensed / Query Limited / Partial / Stale / Monitoring Degraded without leaking hidden resource metadata;
- visualization-threshold changes cannot mutate Alert/Validation Policy and alert-policy edits cannot silently mutate customer styling;
- System-definition upgrade/restore/clone behavior preserves customer-owned definitions and revision rollback;
- query cancellation, stale-response suppression, panel failure isolation, time/timezone correctness, accessibility, responsive/wallboard behavior and representative high-cardinality protection;
- recurring Discovery exact edge semantics: monthly LAST_VALID_DAY, DST-gap forward resolution, ambiguous-time earlier-instant single execution, one-run missed-occurrence coalescing, pause/resume without catch-up, stale-generation pre-execution rejection, obsolete in-flight result fencing, immutable Run Once UTC instant, deterministic spreading, rate/concurrency bounds, Sensor placement, credential-scope failure, tenant isolation and large-estate scan-storm tests.

A registry entry or JSON schema alone is not implementation/certification evidence.

Before presentation architecture is considered stable, R1 certification exercises the representative cross-class set defined in Platform_09 §4.10. Evidence must prove the common envelope and applicable class-specific definitions without coercing console/entity/investigation/map/governance workspaces into dashboard grids. The same presentation-foundation gate includes one representative `REPORT_DEFINITION` (SLA Scheduled Report) and one representative `SAVED_VIEW` so QMC/VIS/context contracts are not certified against dashboards alone; tests cover scheduled execution independent of browser runtime, execution-vs-display timezone, PDF/CSV-only report artifact enforcement, target/filter/sort/group/time-context persistence, URL/context compatibility, authorization re-evaluation and stale/deleted/unauthorized references. TEAM-scoped artifacts remain excluded from production certification until the Identity/Teams/RBAC tranche provides authoritative Team binding/revocation semantics.

---

## 7. Endpoint diagnostics and local-intelligence quality supplement — v3.2.2

Applicable implementation may not be represented as production-ready until evidence covers:

- Linux Agent continuous telemetry: CPU/scheduler/PSI, memory/swap/page pressure, filesystem/inodes, block I/O, NIC, bounded process/service and selected OS/event evidence;
- Windows Agent native runtime on supported Windows hosts, including real Service/SCM lifecycle and applicable performance/service/Event Log evidence; cross-compilation alone is not certification;
- Linux SSH and Windows WinRM/WMI/CIM agentless monitoring with secure credential handling and explicit coverage gaps;
- EDS capture-eligibility state machine (`INELIGIBLE|QUALIFYING|CAPTURE_ELIGIBLE|COOLDOWN`): duration, hysteresis, recovery, cooldown, duplicate coalescing and policy-version changes; prove these states are capture eligibility only and do not assert authoritative host DOWN/severity;
- Core-issued authority/generation fencing for automatic EDS: fresh, stale, expired, revoked, replayed and reassigned cases fail closed;
- manual/Core-requested EDS Diagnostic Job authorization, expiry, idempotency and replay protection;
- `COMPLETE/PARTIAL/FAILED/SUPPRESSED` capture_status under privilege denial, unsupported evidence, timeout, collector fault and resource pressure, with explicit/observable incomplete evidence;
- endpoint idle-overhead benchmark plus bounded transient capture CPU/RAM/I/O/network overhead;
- Sensor/Core storm tests with many simultaneous capture-eligible endpoints, backpressure, queue/storage pressure and **preservation of routine monitoring/ingestion durability ahead of EDS**;
- negative security tests proving no arbitrary shell/`sh -c`/`bash -c`/PowerShell/script-upload execution path and no secret/user-content leakage;
- Agent/agentless semantic normalization and explicit parity differences;
- deterministic Core RCA remaining correct when EDS is missing, partial, late or contradictory;
- Tier-0 endpoint logic with all AI/model runtimes disabled;
- Tier-1 analytics cancellation/yield behavior under Core pressure;
- Tier-2 hosted/on-Core/dedicated-local/no-LLM degradation behavior as applicable;
- dedicated local/private inference provider outage (when that optional mode is enabled) proving no impact on collection, alerting, incidents, deterministic RCA, Core HA or data durability; and proving that mode is not required for EDS/base monitoring.

**Mandatory real-Windows Agent test matrix for BUILD-014/015/016/018/019** (not a cross-compiled simulator-only pass):
- Fresh signed MSI install, SCM automatic/delayed boot, update/uninstall/rollback, service recovery and stop under a blocked collector; Windows SCM wait-hint/checkpoint correctness; silent deployment without an unapproved forced reboot, and truthful reboot-required/locked-binary outcomes.
- Service-token capability/denial matrix under local vs. domain account, virtual-account outbound network identity, domain Group Policy and restrictive EDR: PDH/PerfLib, readable vs. protected Event Log channels, SCM read, process executable path including PPL/EDR-denied paths; no blanket group/ACL escalation or debug privilege.
- Non-English Windows Server counter paths, valid localized/English PDH paths, nonexistent/corrupt/inaccessible/stalled PerfLib providers; exact status-code handling, bounded hung-call concurrency and rapid SCM stop without losing the Agent control loop.
- Processor-group CPU accounting on representative >64-CPU hosts or documented group-aware fixture; commit/limit/paged/nonpaged-pool and physical-memory pressure contrasted; unavailable source never becomes zero.
- Local fixed/mounted/SAN-backed vs. mapped UNC/removable drives, blocked volume query, disk/NTFS/StorPort event provider/channel/version distinctions, quota/cancellation/suppression and no indefinite worker growth.
- NTFS Agent spool rename/compact/delete under open handles and Defender/EDR-style sharing/lock errors; crash/restart/disk-full/replay/ACK fault injection proves no deletion of unacknowledged data and no regression of existing deletion-authorizing ACK durability.
- On-demand Windows TCP aggregate/owner-PID IPv4/IPv6 tables with bounded high-connection-count workload and PID reuse, privacy filters, default port-range configuration changes, Event Log inaccessible/late/reordered outcomes and no per-connection EStats mutation.
- Windows service expected-state tests for delayed/trigger/manual starts, recovery transitions, startup grace, deliberate stops, error and zero exit codes; time-service source/offset/last-sync freshness; explicit `PARTIAL`/`UNAVAILABLE` and no false host/service DOWN.


No fixed endpoint-overhead number is asserted by documentation alone. Release certification records measured idle and triggered overhead per supported OS/profile and establishes the admissible envelope. Specified ≠ implemented ≠ tested ≠ certified ≠ deployed.


## 7. Enterprise SNMP quality and certification supplement

A release or Capability Pack shall not claim production SNMP support merely because GoSNMP unit tests, a single v2c GET, or an isolated Sensor spool test passes. `SPECIFIED != IMPLEMENTED != TESTED != BENCHMARKED != CERTIFIED != DEPLOYED`.

Current known implementation foundations include GoSNMP protocol support, BER typed values, bounded Sensor spool primitives, and—after PR #84—the exact raw Counter64 Sensor→Core→Kafka→ClickHouse path with `UInt64` persistence, replay-safe canonical collapse and full unsigned-boundary tests. That raw path is **IMPLEMENTED+TESTED**, not yet benchmarked/certified. Production gaps that must not be overstated as complete include, among others: SNMP production scheduler wiring; governed production Credential Profile resolution in discovery/polling; complete typed Core ingestion for all SNMP semantic classes; derived Counter32/Counter64 delta/rate/reset/wrap/discontinuity logic; trap/inform production runtime and durable Inform acceptance; MIB Studio/gosmi runtime; interface reconciliation runtime; adaptive polling runtime; full SNMP Test UI; and full Monitoring Quality UI.

Required evidence, as applicable to the declared certification dimensions, includes:

- **Protocol/security:** v1, v2c and v3; noAuthNoPriv/authNoPriv/authPriv; certified SHA-2/AES combinations; explicit legacy MD5/SHA-1/DES cases; context; engineID/boots/time synchronization; unknownEngineID/notInTimeWindow recovery; no silent security downgrade.
- **Operations:** GET, GETNEXT, GETBULK, WALK/BULKWALK; malformed/partial responses; tooBig; controlled bulk-size reduction/fallback; cancellation; deadline; bounded retry/backoff; deterministic poll spreading across the configured interval (normal operational default 5 minutes unless overridden).
- **Credentials:** Credential Profile USE without REVEAL; no secret in logs/Kafka/ClickHouse/audit/support bundles; no default community; concurrent tenants/jobs cannot share credential state.
- **Typed values:** INTEGER, unsigned/gauge/timeticks, Counter32, Counter64, octets/text, OID, IP address, bit string, NULL and protocol exceptions are normalized to their valid semantic classes rather than incorrectly rejected or coerced.
- **Counter correctness:** exact Counter64 above 2^53; Counter32 wrap; device reboot/reset; interface discontinuity; missed poll/irregular elapsed interval; derived rate/utilization; invalid interval produces explicit degraded/unknown quality.
- **Identity:** ifIndex change without false interface duplication; ambiguous interface evidence enters reconciliation/quality state; canonical history/SLA continuity is preserved when identity is proven.
- **Discovery/preflight:** selected-Sensor execution, governed credential resolution, sysObjectID/sysName/sysDescr identity, actionable timeout/auth/authz/context/MIB errors and no hard-coded community.
- **Runtime/durability:** Core assignment/fence → Sensor scheduler → SNMP executor → durable spool → ingestion → Kafka → typed persistence; Core/WAN interruption, Sensor restart and replay/duplicate cases are proven.
- **Trap/Inform:** v1/v2c/v3 notification decoding, source/security attribution, malformed packets, trap storm/fingerprint/rate control; Inform durable local acceptance/accounting before ACK; receiver restart; disk/storage pressure; retransmission; duplicate Inform; failed durable acceptance; enrichment unavailable after durable acceptance; and no direct trap→alert shortcut.
- **Capacity:** representative device/interface/OID distributions, GETBULK response sizes, RTT/loss/timeouts, retries, SNMPv3 CPU cost, scheduler queue age, adaptive-poll bursts and trap EPS under simultaneous certified monitoring workload.
- **UX:** SNMP Test shows Sensor/vantage, separated reachability/auth/authz/object-support outcomes, raw+resolved OID/value, stable error/remediation and no secrets; poll-health UI exposes freshness, failures and monitoring degradation.
- **Security/NCM:** routine monitoring remains read-only; SET is unavailable from normal polling/test paths and any certified SET action proves NCM/remediation authorization, pre/post verification and audit.

False-alert regression shall include: SNMP failure while ICMP/API/neighbor/path evidence remains healthy → `MONITORING_DEGRADED`/appropriate semantic state, not authoritative `DEVICE_DOWN`.


## 8. Final handoff certification supplements — v3.2.2

### 8.1 Display timezone

Required evidence includes: Europe/London spring-forward and fall-back; America/New_York DST; Asia/Kolkata non-DST; half-hour and quarter-hour offsets; same event rendered in multiple zones; server-host timezone different from user and tenant; changing display timezone without changing policy evaluation; ambiguous/nonexistent wall-clock operational schedule input; report/export timezone disclosure; preference precedence and URL/saved-view override behavior.

### 8.2 Generic scheduler / freshness

Required evidence includes: 2,500-device scenario; deterministic distribution; restart; failover; 5,000 due-job recovery; broad site outage; API/controller outage; resource pressure; spool 80%; Core outage; rate limit; huge WLC; slow healthy devices; credential failure; event/adaptive storm; mass interval change; Sensor reassignment; clock skew; fairness; no duplicate scheduled work; no fleet herd; no synthetic historical samples.

### 8.3 Counter64

Required evidence includes: source values > 2^53; near UInt64 maximum; reset; reboot; discontinuity; legitimate wrap only where provable; irregular sample interval; Kafka serialization without JSON Number loss; ClickHouse raw exactness.

### 8.3.1 Database audit hardening evidence gate — 2026-10-30

In addition to the existing SNMP/Counter64 and release gates, certify these *only when relevant implementation exists*; absence of a test is not evidence that the claimed runtime defect exists:

1. **PostgreSQL access paths:** compare `EXPLAIN (ANALYZE, BUFFERS)`, query p95/p99, write/lock cost and storage with/without candidate FK/composite/JSONB indexes on representative tenant cardinality. Detect redundant PK/UNIQUE indexes and migration lock/rollback risk; do not require every FK to be separately indexed.
2. **Tenant isolation:** test missing/wrong tenant context, cross-tenant references, policies, joins, exports, role/owner privilege, transaction rollback, connection-pool reuse, backup/restore and background jobs. If PostgreSQL RLS is selected, test both permissive/restrictive access and `USING`/`WITH CHECK`; if ClickHouse row policies are used, test exact read-only roles/engine version. Tenant-first `ORDER BY` is never accepted as isolation proof. MSP certification remains Phase 1.5.
3. **Selective erasure and holds:** place two tenants and a held entity in the same ClickHouse time partition; purge one permitted entity/tenant without touching others, verifying visible query state, physical-removal or documented pending state, rollup/replica/archive/backup treatment, legal-hold denial, progress and audit. Measure mutation pressure against ingestion/query SLOs and the 80% safeguard.
4. **Valkey faults:** inject timeout, crash/restart and network partition separately for cache, session/revocation, security admission/rate limit, dedup/lock and HA-related paths. Prove safe cache bypass with bounded backend load, fail-closed sensitive operations, no dual-active fencing and observable degraded/recovery state. No arbitrary cache-rebuild SLA is assumed.
5. **Rate-state continuity:** exercise exact `UInt64` values over `2^53`, Counter32 wrap, ambiguous Counter64 decrease, reboot/discontinuity, polling gaps, stale/duplicate/out-of-order input, Sensor restart/reassignment/cache eviction, nonpositive elapsed time and first-sample baseline. The result must be exact raw history plus valid rate or explicit `UNKNOWN`/`DEGRADED`; never a false utilization spike.

Record SPECIFIED/IMPLEMENTED/TESTED/BENCHMARKED/CERTIFIED separately with commit, fixture, workload, tool output and failure evidence. Preserve Phase-1 standalone versus Phase-1.5 MSP/HA scope. These are refinements of existing `REQ-DATA-*`, `PRD-EGC-006` and Counter64 contracts; no new BUILD ID or GA claim.


### 8.4 Two-Core HA

HA certification gates remain explicit and unmet until evidence exists. Do not invent passing evidence. Status remains SPECIFIED / P0 / NOT PRODUCTION CERTIFIED.

## 9. Unified RCA and regional Sensor/provisioning certification supplement — v3.2.2

### 9.1 RCA/correlation

Before RCA/correlation is certified, deterministic fixtures must prove:

- one canonical incident/RCA authority across network, infrastructure, storage, virtualization, database, cloud and service evidence;
- explicit competing hypotheses, causal roles, support/contradiction/missing evidence and rejection reasons;
- immutable/reproducible RCA revisions after late evidence, topology/change updates and recovery;
- first causal event versus first alert ordering;
- counterfactual expected-effect checks;
- recovery evidence strengthens/weakens hypotheses without creating post-hoc certainty;
- redundancy-success cases do not become false service outages;
- shared-risk relationships require provenance/confidence/temporal validity;
- stale topology, clock skew, out-of-order, duplicate/replayed and missing evidence;
- deterministic cross-domain chains including ToR, iSCSI/NFS/SAN, hypervisor contention, DNS/AAA, PDU/power and Sensor-failure scenarios;
- bounded graph neighborhood/time/evidence fan-out/concurrency/timeout under incident storms;
- machine confidence is measured separately from validated correctness;
- tenant-scoped human feedback cannot directly mutate production rules/models.

Quality reporting includes root-cause precision, false-root rate, appropriate-inconclusive rate, time-to-root-candidate, correlation latency, evidence completeness, blast-radius accuracy and recovery-validation result where applicable. No universal acceptance percentage is invented here.

### 9.2 Region / Sensor Pool / enrollment / secure transport

Certification must include:

- Region/Site/Pool tenant-isolation and lifecycle/migration tests;
- placement against role/capability/capacity/failure-domain policy;
- generation/fencing under concurrent reassignment, restart and failover;
- Sensor and Agent short-lived/single-use token success, expiry, replay, wrong-scope and revoked cases;
- local key generation / proof-of-possession with no private key in Core DB, logs, audit, support bundle or events;
- certificate rotation/revocation and DNS/IP/VIP SAN/trust failures;
- no certificate-verification bypass;
- no production plaintext Sensor↔Core or Agent↔Sensor transport path;
- legacy monitored-device insecure protocols cannot downgrade Platform-owned transport;
- Agent normal telemetry remains assigned-Sensor mediated after bootstrap;
- pool capacity/rebalance under Sensor loss and recovery without duplicate ownership;
- resource-efficiency benchmarks measure CPU/RAM/network/storage before increasing certified hardware profiles.

Specified ≠ implemented ≠ tested ≠ benchmarked ≠ certified ≠ deployed.

---

## 10. Service Assurance / SLA-OLA reporting certification supplement — 2026-09-27

Phase-1 service assurance is not release-ready until deterministic fixtures prove:

- **Formula correctness:** mathematical evaluation across arithmetic, ratios, percentages, counts, sums, min/max, averages, weighted averages, percentiles, durations, and compliance calculations;
- **Formula injection and security:** safe expression validation, type/unit checking, divide-by-zero handling, and strict prevention of arbitrary code or SQL execution (no Python, JavaScript, shell, or arbitrary SQL eval);
- **Calendars, timezones, and DST:** reusable service-time profiles with IANA timezone calculations, holiday calendars, working days, and daylight saving time transitions (spring-forward / fall-back);
- **Country and site calendar overrides:** localized calendar variations and site-specific overrides with audited exception handling;
- **Cross-midnight shifts and team/shift handoffs:** shifts spanning midnight boundaries, multi-shift coverage, and historical segmented ownership preservation without rewriting past intervals;
- **Threshold model:** independent target/expected, minimum, at-risk, and breach threshold evaluation;
- **Service dependency and redundancy rules:** topology-aware and dependency-grounded service availability without blind averaging of component uptime;
- **Degraded-service treatment and persistence:** informational-only, partial penalty, outage only after configured persistence, or immediate outage;
- **Measurement bases and impacted-unit calculations:** service minutes, entity minutes, impacted-unit minutes, transaction counts, ticket counts, event counts, and weighted scores;
- **Maintenance and exclusion handling:** approved maintenance windows, customer-responsibility exclusions, contractual deviations, and retroactive adjustment lineage preserving raw results;
- **Monitoring quality and UNKNOWN behavior:** separate evidentiary confidence from service state; explicit `VALID`, `PARTIAL`, `INSUFFICIENT_EVIDENCE`, `UNKNOWN`, and `DISPUTED` states; monitoring-path failures or gaps cannot silently become service downtime or perfect compliance;
- **Multi-vantage and quorum rules:** multi-point measurement, N-of-M quorum rules, and end-to-end synthetic corroboration preserving contradictory evidence;
- **Policy simulation:** read-only backtesting of draft/updated policies against retained historical evidence showing outcome deltas without mutating production truth;
- **FINAL period immutability:** immutable period close (`OPEN -> PRELIMINARY -> REVIEW -> FINAL`) binding policy, formula, calendar, and scope snapshots;
- **RESTATED lineage:** versioned restatement and audited revision history for post-final corrections;
- **Audit, approval, and attestation:** report obligation lifecycle, delivery tracking, review/attestation states (`UNDER_REVIEW`, `ACCEPTED`, `REJECTED`, `DISPUTED`), and audit lineage;
- **Tenant isolation:** strict tenant boundaries for policies, calendars, evidence ledgers, obligations, reports, and formulas with cross-tenant negative testing;
- **PDF rendering with effective commercial logo:** PDF reports embed the effective commercial logo appropriate to background contrast (defaulting to the governed AmrudAi logo: `amrud_blk.png` for light background, `amrud_whi.png` for dark background, preserving aspect ratio without stretching, or tenant/MSP logo where a governed white-label configuration is active);
- **CSV report generation:** detailed machine-readable tabular evidence generation without embedded images;
- **Report resource isolation under monitoring load:** bounded query execution, lower worker priority than live monitoring/ingestion/alerting, query cancellation, and resource caps preventing monitoring degradation;
- **Strict report artifact format restriction:** generated, downloaded, and scheduled report artifacts are restricted to PDF and CSV only; no XLSX, PPTX, or JSON report artifact generation.

Do not mark these tests as implemented/passing unless physical test evidence exists. No customer-specific target percentage, credit amount, contractual liability or financial consequence is certified by these tests unless separately configured and authorized.

---

## 11. Network & Infrastructure Observability Certification Profile (NIOCP-P1-v1)

> **Status:** NOT YET CERTIFIED. NIOCP-P1-v1 is a future certification profile. Documenting this certification profile does **not** claim that NIOCP-P1-v1 has passed or that Phase-1 capabilities are certified. Capability claims require reproducible execution evidence for all applicable scenarios.

Phase-1 Unified Network & Infrastructure Observability capability claims are governed by the **Network & Infrastructure Observability Certification Profile (NIOCP-P1-v1)**. NIOCP is a named release-quality profile and gate inside existing Platform_14 governance; it is **not** a new runtime service or certification daemon.

1. **Real device/service outage:** Ground-truth hard and soft failure scenarios prove that authoritative outages are detected, qualified, and correlated to correct root-cause candidates within SLA budgets.
2. **Monitoring/credential/Sensor failure without false DOWN:** Injected SNMP timeout, invalid v3 credentials, Sensor reachability loss, or probe failure must produce `MONITORING_DEGRADED` or `UNKNOWN` with actionable diagnostic evidence; it must **never** assert monitored object `DOWN` when corroborating path/service evidence indicates the device is reachable.
3. **WAN congestion and packet loss:** Injected WAN queueing delay and synthetic loss demonstrate telemetry spool buffering, adaptive backoff, and loss visibility without dropping monitoring continuity.
4. **Routing and configuration change:** Live BGP/OSPF route flap or device configuration change correlates into Change Intelligence and the Unified Operations Timeline without masking independent concurrent operational failures.
5. **Interface identity continuity:** Monitored interface `ifIndex` renumbering across device reboot or line-card reseat preserves historical continuity without creating duplicate or phantom interfaces.
6. **Temporal replay and incident-time reconstruction:** Temporal operational graph faithfully reproduces estate topology and relationship state at exact incident timestamps during post-incident review and Investigation Sessions.
7. **Cross-domain incident correlation:** Multi-signal incident spanning infrastructure host CPU, network interface drop, database session exhaustion, and synthetic endpoint degradation fuses into a unified incident context with explicit causal roles.
8. **Insufficient evidence handling:** When required corroborating signals are missing or telemetry quality is low, RCA engines must truthfully emit `INCONCLUSIVE` or `UNKNOWN` rather than hallucinating certainty.
9. **Active path observability profile:** Bounded multi-vantage path probes (ICMP, TCP, UDP where certified) demonstrate:
   - path fingerprinting and change detection;
   - hop-by-hop and destination latency, loss, and jitter measurement;
   - graceful handling of incomplete traceroutes without declaring path failure;
   - preservation of hidden-hop uncertainty and ECMP path variance;
   - multi-vantage contradiction preservation;
   - IPv4/IPv6 dual-stack and NAT traversal correctness;
   - strict enforcement of non-O(N²) probe budgets and rate bounds.
10. **Linux Agent eBPF functional and failure profile:**
    - verified mapping of process, service, executable, and container/cgroup to remote network endpoints;
    - local bounded aggregation into defined time buckets without uploading raw kernel events;
    - resource limits and bounded memory budgets derived from BUILD-019 / BUILD-130 benchmark certification across certified Ubuntu 24.x and RHEL 9.x kernels;
    - explicit capability states: `UNSUPPORTED`, `UNAVAILABLE`, `PRIVILEGE_DENIED`, `KERNEL_INCOMPATIBLE`, `RESOURCE_GUARDED`;
    - graceful degradation under verifier rejection, BTF/CO-RE incompatibility, or unsupported older kernels;
    - loss-accounting verification: explicit increments of `dropped`, `shed`, `sampled`, and `resource_guarded` counters under event-rate or map pressure without crashing the Agent;
    - security and least privilege: verification that unprivileged execution fails closed, no packet payloads/TLS secrets/SQL texts/environment variables are captured, and process metadata adheres to data minimization.
11. **Host infrastructure, virtualization, and storage:** Validated telemetry and health representation across physical servers, VMware/hypervisors, and storage arrays without converting Platform into a DCIM/BMS suite.
12. **Cloud and Kubernetes depth:** Validated normalization of cloud-network entities (VPCs, subnets, route tables, security policies, load balancers, NAT gateways) and Kubernetes workloads into canonical Platform graph models with connector self-health separation. Cloud/provider flow logs are SUPPORTED WHEN ENABLED/CERTIFIED subject to provider capability, cost, and rate limits; they are not a mandatory Phase-1 dependency.
13. **SLO and error budget evidence:** Where BUILD-095 applies, error budget burn rates and compliance scorecards calculate deterministically from qualified evidence ledgers.
14. **Telemetry overload and load-shedding:** Extreme metric, flow, and syslog surges exercise Sensor spool watermarks, Kafka consumer backpressure, and ClickHouse micro-batching without dropping high-priority control or liveness signals.
15. **Core and Sensor interruption and replay:** Active Core crash or Sensor disconnection verifies zero duplicate authority, stateful failover within the <60s target, and idempotent replay of buffered Sensor spools upon reconnection.
16. **Low-compute and non-interference guarantee:** Bounded Linux eBPF telemetry and active path probing must measurably operate within declared CPU, RAM, and network envelopes; if resource contention occurs, optional enrichment must yield, throttle, or suspend before primary monitoring, collection, durable transport, ingestion, alert qualification, incident processing, or deterministic RCA are degraded.

---

## Phase-1 endpoint operations certification supplement — Issues #169/#170

### Installed-software inventory

Certification requires applicable Linux and Windows tests for:

- authoritative package/product enumeration from supported sources into a versioned `software_inventory_snapshot` envelope;
- snapshot completeness semantics (`FULL|TARGETED`, `COMPLETE|PARTIAL|FAILED`, truncation/loss reasons, item_count);
- native source/package identity preservation;
- version/vendor/architecture/scope/provenance mapping;
- stable/versioned `inventory_scope` comparison identity and proof that source/scope-definition changes do not appear as mass uninstall/install;
- install, removal and version-change reconciliation under removal-safety rules;
- proof that only `COMPLETE` + non-truncated + authoritative `FULL` same endpoint+native-source+`inventory_scope` snapshots may infer general absence/removal;
- proof that `PARTIAL`/`FAILED`/truncated/`TARGETED` captures and permission/access failures never generate mass or unrelated removals;
- same-capture retry with stable `snapshot_id` and Core idempotency;
- duplicate replay of identical `snapshot_id` does not create duplicate authoritative state, duplicate `SOFTWARE_*` events or false first/last-seen mutation;
- late older FULL snapshot after newer FULL authority does not roll current state backwards or generate reverse change events;
- out-of-order arrival retains original capture provenance and does not gain authority merely by later Core arrival;
- transport/job ordering preferred over wall-clock-only ordering when stronger Agent/Sensor/Diagnostic Job sequencing exists;
- daily deterministic spreading and reliable change-trigger coalescing;
- missed local event repaired by later authoritative reconciliation;
- restart/disconnect/replay/idempotency;
- large installed-set CPU/RAM/I/O/network bounds;
- tenant isolation and API/RBAC;
- normalization ambiguity/false-merge negative fixtures and proof that resolver-only changes do not masquerade as package events or fabricate first/last-seen lifecycle changes;
- proof that typed inventory is not faked through scalar metric labels on the current Agent CollectionBatch path;
- proof that no patch/deployment/remediation action is implied.

Windows inventory certification must additionally exercise 64-bit/32-bit registry views (including dual-written and distinct bundle/MSI keys), access denied to one declared view, both views complete, unsupported MSIX/AppX sources, scoped `FULL` vs. `PARTIAL`, user profiles not loaded, `HKEY_CURRENT_USER` service-token isolation, view-qualified `native_package_id` and source-provenance preservation. A view failure must not emit any false uninstall event, and resolver-based deduplication must not change native package lifecycle truth. Read-only enumeration must never instantiate `Win32_Product`/installer repair or load offline `NTUSER.DAT`.


Deterministic RCA certification must prove RCA remains available with contradictions/missing evidence/`INCONCLUSIVE` when software inventory/change evidence is absent; BUILD-098 evidence is consumed when available and must not be a hard runtime dependency.

### Process/service monitoring

Test:
- Monitor and Stop Monitoring workflow;
- service/unit and executable selectors;
- PID reuse/restart does not retarget monitoring incorrectly;
- minimum/maximum instances, absent/stopped, restart-rate, CPU/memory and suspected-unresponsive conditions where supported;
- short-lived process-cardinality stress;
- RBAC/audit;
- alert creation does not grant restart/remediation authority.

### Criticality/Profile mapping

Test:
- deterministic mapping and inheritance;
- `AUTO_FROM_CRITICALITY` vs `EXPLICIT_PROFILE`;
- explicit profile survives Criticality change;
- capacity/MWU preview and admission;
- large-scope profile change is spread/bounded and does not create a poll/collection storm;
- rollback/version/audit behavior.

### Triggered Fast RCA

Extend the existing EDS/RCA corpus with trigger-specific ground truth:

- CPU-bound process vs high iowait vs VMware CPU contention vs directly evidenced thermal throttling;
- memory growth/pressure, swap/OOM and VMware ballooning/contention;
- filesystem capacity, inode exhaustion, block latency/error and datastore/storage contention;
- OOM-killed process, dependency unavailable, port/listener conflict, restart loop, deliberate stop and false-hung case;
- NIC errors/drops, local route/gateway issue, remote/path issue and contradictory local-vs-path evidence.

For each scenario prove:
- the correct bounded EDS definition ran;
- expected evidence/provenance exists;
- unavailable/denied evidence is explicit;
- unsupported causes are not asserted;
- contradictions lower confidence;
- `INCONCLUSIVE` is emitted when appropriate;
- late evidence produces a new immutable RCA revision rather than rewriting history;
- cross-vantage fan-out is bounded and authorization/capacity aware;
- diagnostics do not materially worsen simulated CPU, memory, disk or network pressure.

VMware/API terminology and metric availability must be certified against supported versions; documentation names do not prove API availability.


---

## 12. v3.3.0 ACP certification and negative-test gates

### 12.1 Official Certified Capability Packs

A pack/profile is not certified by parser success or vendor name. Applicable gates cover supported product/version identification, authentication/least privilege, discovery/reconciliation/no-duplicates, inventory/entities/relationships, metric units/counter/state semantics, topology, safe default monitoring profile, dashboards/alerts/qualification, diagnostics, malformed/partial/timeout/auth failure/unsupported version, data quality, resource budget, upgrade/migration/rollback compatibility, provenance and dimensional Discovery/L1/L2/L3/Topology/Diagnostics/NCM/Security evidence.

### 12.2 OTLP Phase 1

Test certified OTLP/gRPC and HTTP profiles for metrics and logs, resource/scope identity, trace/span correlation IDs, exemplars where present, wrong tenant/source, invalid/revoked cert/token, malformed protobuf/JSON where supported, partial success, duplicate/replay, missing parent/reference, out-of-order timestamps, clock skew, huge attributes, cardinality attack, sensitive-field redaction, compressed/decompression bombs, rate limits, queue/backpressure, failover/replay, storage pressure, rejected/dropped accounting, entity-resolution conflicts and proof that OTLP source failure/no-data does not assert application DOWN.

### 12.3 OTLP Phase 1.5 spans

Before certification prove span schema correctness, head/tail sampling behavior, trace routing, sampling provenance, memory/state limits, restart/HA behavior, short-retention enforcement, trace query bounds, service-edge provenance, Flight Recorder pinning, cross-signal correlation and measured Core/Sensor/ClickHouse cost. Numerical acceptance values are benchmark outputs, not Mother Reference inventions.

### 12.4 NCM Change Assurance

Negative/edge tests include backup failure/staleness, config search redaction, raw/semantic diff parser mismatch, privilege failure, missing/forged/replayed/stale approval, modified-after-approval targets/config/window, concurrent/out-of-band change, precheck deterioration, canary failure, partial rollout, stop-condition enforcement, HA-pair/failure-domain sequencing, post-validation failure, unsupported/failed rollback, device unreachable, vendor rate/session limits, maintenance-window expiry and complete audit/evidence manifest.

### 12.5 Firmware — Phase 1.5

Test wrong/unauthorized image, checksum/signature failure, entitlement failure, unsupported model/version, missing intermediate release, insufficient storage, stale/failed backup, transfer interruption, canary/reboot/health failure, rollback unavailable/fails and supply-chain/provenance evidence. Firmware is not first-GA complete based on a successful happy-path upgrade.

### 12.6 ITSM/CMDB

Per certified connector/edition/API test authentication/permissions, standard/custom mappings, required fields, create/read/update/comments/work notes, allowed/unsupported transitions, resolve/reopen, change/approval, webhook authentication/replay/duplicate/out-of-order, polling checkpoint, vendor outage, retry/rate-limit recovery, loop prevention, wrong tenant, stale external version, conflict handling, wrong-CI prevention, CMDB read/write/relationships/business services where claimed, ServiceNow IRE behavior, no external state overriding monitoring truth and no free-text path to privileged execution. Integration Test Sandbox capability result must be precise rather than generic connection success.

### 12.7 Phase-1.5 browser/DB/virtualization/endpoint

Browser journeys require credential isolation, sandbox/runtime admission, deterministic steps/assertions/timing, screenshot/artifact access/TTL/redaction, crash/timeout/cancellation and capacity isolation. DB intelligence requires safe fingerprint/wait/blocking/deadlock/regression/plan evidence without dangerous query execution/autotuning. Virtualization recommendations require evidence/false-positive testing and no autonomous apply. Endpoint/switch-port history requires identity/provenance/movement/tenant/privacy tests and no duplicate endpoint store.

## Phase 1.5 Serverless certification gates — Issue #268

Governing requirement: `REQ-SLS-009` / `PRD-SLS-009`. Status: SPECIFIED / NOT IMPLEMENTED / NOT CERTIFIED. Listing a provider or metric is NOT certification.

Phase 1.5 serverless certification for AWS Lambda, Azure Functions, Google Cloud Functions and Google Cloud Run MUST include at minimum:

1. wrong/expired credentials;
2. permission denied;
3. provider API outage;
4. API throttling / 429;
5. quota exhaustion;
6. pagination;
7. large function estates;
8. high-cardinality tags;
9. function creation/deletion/recreation;
10. alias/version/revision changes;
11. trigger changes;
12. zero-invocation function;
13. scale-to-zero behavior;
14. stale/no-data distinction;
15. provider monitoring lag;
16. missing metrics;
17. log/event burst;
18. dead-letter/destination failure where provider supports it;
19. concurrency saturation;
20. retry storms;
21. cold-start evidence where available;
22. connector restart/recovery;
23. tenant isolation;
24. overlapping/provider IDs across tenants;
25. no false Function DOWN caused by connector failure (`REQ-SLS-005`);
26. performance/resource/API-cost regression (`REQ-SLS-007`);
27. audit/redaction (`REQ-SLS-006`).

Owning evidence lane: BUILD-036 Phase 1.5 with BUILD-026 pack dimensional evidence and downstream alert/RCA/UI gates as applicable.

---

## Phase 2 Enterprise RFP Readiness Reconciliation — Issue #269

### ECRP-P2-v1 — Enterprise Customer RFP Readiness Profile

Phase 2 cannot be certified complete until a representative large-enterprise profile proves the applicable chain:
`discovery -> onboarding -> monitoring -> topology/service mapping -> qualification -> incident -> RCA -> ITSM/integration -> recovery -> reporting/assurance`.

The profile shall cover representative multi-DC/DR/branch, large-estate inventory, network, Windows/Linux plus declared Unix coverage, virtualisation/HCI, hardware/OOB, storage/SAN, Oracle-class and additional databases, middleware, backup/data protection, Kubernetes, AWS/Azure/GCP/OCI, Flow/log/OTel, CMDB/ITSM, external APM, external NPM/DPI and SIEM evidence, regulated data boundaries, HA/DR, retention/archive, concurrent NOC users and connector failure/degradation.

Certification requires exact product/version/collection-mode matrices, tenant/security negatives, stale/missing/contradictory evidence, API throttling, replay/idempotency, upgrade compatibility, scale/cardinality, retention, failure/recovery and operator workflows. Full native APM and full native DPI are explicitly **not** ECRP-P2-v1 requirements.

---

## Issue #272 certification extension

### Strengthened ECRP-P2-v1

At Phase-2 exit, **no accepted enterprise requirement may remain an unresolved architecture-level Partial**.

Each requirement class must resolve to:
- **Native — Certified**;
- **Integrated — Certified**;
- **Customer Profile — Certifiable**;
- **Explicit Phase-2.5 deferment**.

Customer-specific absolute SLA claims remain separately negotiated and evidenced.

### DORP-P2.5-v1 — Deep Observability Readiness Profile

`DORP-P2.5-v1` is a certification profile, **not a BUILD ID**.

A Phase-2.5 native deep-observability completion claim is prohibited until representative evidence covers:

**APM pillar**
1. supported instrumentation runtimes and version matrix;
2. service/resource identity and tenant isolation;
3. distributed trace/transaction correctness;
4. adaptive sampling/cardinality/backpressure;
5. errors, DB/external calls and service dependencies;
6. continuous CPU/memory/allocation/thread/lock profiling;
7. browser/mobile RUM and Core Web Vitals where enabled;
8. consent/privacy-governed session replay where enabled;
9. secret/PII redaction;
10. trace/profile/log/metric/change/incident correlation;
11. agent/collector outage, upgrade/rollback and compatibility.

**DPI / Network Performance Intelligence pillar**
1. SPAN/TAP/vTAP/packet-broker ingest;
2. supported line-rate/capture-loss envelope;
3. packet metadata/flow reconstruction;
4. L2-L7 decoder correctness;
5. TCP/DNS/TLS/HTTP network-performance diagnostics;
6. network-versus-application response decomposition;
7. packet-to-service/trace correlation;
8. bounded PCAP retention/retrieval;
9. payload masking and tenant/RBAC isolation;
10. controlled TLS-decryption authorization/key-custody tests where enabled;
11. capture/storage exhaustion and degraded-mode behavior;
12. upgrade/rollback and decoder compatibility.

The gate SHALL include scale/load/soak, failure/recovery, security, privacy, tenant-negative, retention, migration/upgrade and operator-workflow evidence.
