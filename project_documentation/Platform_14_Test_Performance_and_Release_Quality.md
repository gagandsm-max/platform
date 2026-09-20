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

Wrong SNMP community/v3 user/auth/privacy, SNMP disabled, ACL/firewall drop, unreachable host, wrong API token, TLS failure, expired certificate, DNS failure, Kafka unavailable, ClickHouse unavailable, PostgreSQL failover, sensor disk pressure, Core loss, duplicate/replayed telemetry, malformed bulk import, high-cardinality explosion, clock skew, unauthorized/cross-tenant diagnostic target, arbitrary/ad-hoc target abuse, diagnostic rate/concurrency exhaustion, unbounded continuous test, cancellation/timeout failure, multi-vantage contradictory results, reused/expired diagnostic authorization, CLI session hijack, one-time/reusable credential leakage, command-policy bypass, privilege escalation/config-mode denial and missing diagnostic/CLI/investigation audit evidence. Cloud/full-stack negative cases additionally include revoked/expired cloud identity, insufficient provider permissions, cross-account/subscription/project/compartment scope escape, provider API throttling, malformed/truncated pagination or cursor replay, missed provider events followed by reconciliation, high-churn ephemeral resources, duplicate cloud events, trace/log cross-tenant correlation attempts, trace attribute/cardinality bombs, sensitive trace/log field leakage, sampling/backpressure failure and cloud connector stale/partial state being misclassified as resource-down.

## 3.1 NOC troubleshooting acceptance tests

UI E2E/integration suites shall verify: device/alert/incident context is preserved while Troubleshoot/embedded CLI is used; on-demand Ping/Trace/SNMP returns source/vantage and bounded results; multi-vantage disagreement remains visible; `Verify Now` returns per-check evidence; Live Evidence distinguishes monitored from on-demand state; Investigation Sessions retain evidence references; stored/personal/one-time CLI credential modes obey policy; low-risk read-only diagnostics avoid unnecessary friction while privileged/configuration commands enforce configured controls; and interactive load cannot starve certified monitoring workload.

## 4. False-alert quality

Qualification tests must prove that SNMP failure alone does not automatically declare device-down where secondary evidence shows device reachability. Flapping/storm/dependency correlation and recovery qualification require regression suites.

## 5. Packaging/reboot tests

Every `.deb`/`.rpm` package is installed on clean certified OS images, rebooted, upgraded, rolled back where supported and uninstalled. Services must auto-start and retain configuration. Windows Agent MSI/BAT install and reboot behavior is tested on Windows runners.

## 6. Quality gates

No release promotion with failing P0/P1 tests, unresolved critical/high security defects without approved exception, tenant-isolation failure, data-loss defect in certified normal failure scenarios, failed upgrade/rollback gate, broken SBOM/license gate or performance regression beyond approved threshold.

For a **Cloud Observability GA** claim, all `PRD-CLOBS-001..021` / `REQ-CLOBS-001..021` P0 gates in `Platform_26` are mandatory. AWS, Azure, GCP and OCI provider roots must each have certified onboarding, hierarchy/inventory, connector health, required P0 evidence classes and mandatory high-value Cloud Service Pack coverage. Specification or a generic connector is not certification.

## 4. Platform — Performance & Benchmark Plan

> Consolidated from `Platform_22_Performance_and_Benchmark_Plan.md` v3.1. Requirements in this section remain normative.

## 1. Core target

≤2,500 reference network devices on 8 vCPU / 16 GB Core is a mandatory engineering target under a versioned Standard Network Workload Profile, not a blanket guarantee for arbitrary Flow/log/high-cardinality workloads.

## 2. Sensor targets

Planning baseline 4 vCPU/16 GB/150 GB ≈800 normal network devices. Benchmark objective: 8 vCPU/16 GB class approaching 2,000–2,500 comparable normal devices where workload permits. Streaming/Event Sensor benchmarked separately by records/events per second.

## 3. Benchmark dimensions

Devices, interfaces, metrics, poll interval, SNMP version/security, RTT, timeouts, traps/events, flow rate, syslog EPS, gNMI subscriptions, concurrent users, dashboard queries, reports, RCA jobs and storage retention. Cloud/full-stack dimensions additionally include provider organizations/accounts/subscriptions/projects/compartments, resources by region/type, provider API calls/minute and throttling, metric samples/sec, log bytes/events/sec, spans/sec and attributes/span, Kubernetes object/event churn, serverless invocation volume, cloud flow records/sec, cloud audit/change events/sec, SLO evaluations and concurrent Trace/Log/Service-graph queries. P1 benchmarks add RUM events, profiling volume and cloud-cost facts.

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
- unresolved RED OSS dependency;
- Cloud Observability GA with any AWS/Azure/GCP/OCI P0 provider root or mandatory Platform_26 service-family certification missing;
- cloud credential/trace/log/resource tenant-isolation defect or secret/sensitive-field leakage;
- connector/API degradation that can silently convert missing/stale cloud telemetry into false resource-down state.

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
