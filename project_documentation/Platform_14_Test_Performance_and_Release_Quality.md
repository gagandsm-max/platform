# Platform — Test, Performance, Benchmark & Release Quality Master

**Document ID:** PLATFORM-QUALITY-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
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

## 5. Packaging/reboot tests

Every `.deb`/`.rpm` package is installed on clean certified OS images, rebooted, upgraded, rolled back where supported and uninstalled. Services must auto-start and retain configuration. Windows Agent MSI/BAT install and reboot behavior is tested on Windows runners.

## 6. Quality gates

No release promotion with failing P0/P1 tests, unresolved critical/high security defects without approved exception, tenant-isolation failure, data-loss defect in certified normal failure scenarios, failed upgrade/rollback gate, broken SBOM/license gate or performance regression beyond approved threshold.

## 4. Platform — Performance & Benchmark Plan

> Consolidated from `Platform_22_Performance_and_Benchmark_Plan.md` v3.1. Requirements in this section remain normative.

## 1. Core target

≤2,500 reference network devices on 8 vCPU / 16 GB Core is a mandatory engineering target under a versioned Standard Network Workload Profile, not a blanket guarantee for arbitrary Flow/log/high-cardinality workloads.

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
- broken reboot auto-start/recovery;
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
- recurring Discovery timezone/DST behavior, pause/resume, schedule edit generation, missed-run skip/coalesce, deterministic spreading, rate/concurrency bounds, Sensor placement, credential-scope failure, tenant isolation and large-estate scan-storm tests.

A registry entry or JSON schema alone is not implementation/certification evidence.

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
