# Platform — Test, Performance, Benchmark & Release Quality Master

**Document ID:** PLATFORM-QUALITY-001
**Version:** 3.2
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2 document preserves the normative content of the listed v3.1 source documents. Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

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

Wrong SNMP community/v3 user/auth/privacy, SNMP disabled, ACL/firewall drop, unreachable host, wrong API token, TLS failure, expired certificate, DNS failure, Kafka unavailable, ClickHouse unavailable, PostgreSQL failover, sensor disk pressure, Core loss, duplicate/replayed telemetry, malformed bulk import, high-cardinality explosion and clock skew.

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
