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

Planning baseline 4 vCPU/16 GB/150 GB ≈800 normal network devices. Benchmark objective: 8 vCPU/16 GB class approaching 2,000–2,500 comparable normal devices where workload permits. Dedicated Flow Sensors and dedicated Log Sensors are benchmarked separately using their protocol-specific workload dimensions; they are not production-mixed with Network Sensor workloads.

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


## 7. Enterprise Flow/NTA and dedicated Sensor-role certification supplement

A release shall not claim production Flow/NTA readiness merely because GoFlow2 decodes a fixture or a dashboard renders top talkers.

Required evidence, as applicable to the declared protocol/profile, includes:

- **Role isolation:** production FLOW_SENSOR and LOG_SENSOR activate only their allowed high-rate modules plus common control/health/spool/uploader; negative tests prove Flow cannot start SNMP/discovery/NCM/log workers and Log cannot start Flow/SNMP/discovery/NCM workers. Combined high-rate role is not production-certified.
- **Role change:** drain/reassignment, spool/accounting completion, incompatible-assignment rejection, restart/config apply and post-change health verification.
- **Protocols:** NetFlow v5/v9, IPFIX, sFlow and certified vendor-compatible variants with malformed/oversized/truncated datagrams and unsupported versions.
- **Exporter/source authority:** tenant/listener/exporter mapping, same source IP in different tenants, NAT/relay scenarios, unknown/quarantined exporters, spoofed observation domain/engine fields and no packet-derived tenant override.
- **Templates:** arrival/update/expiry/withdrawal/reuse, exporter restart/generation, options templates, enterprise IEs, variable-length fields, missing/invalid templates, Sensor restart with cache validity and template churn/cardinality attack.
- **Sampling:** known/unknown sampling, 1:N changes during a time window, exporter-provided algorithms/options, observed-vs-estimated bytes/packets and no exact-unsampled misrepresentation.
- **Time/sequence:** flow/export/observed/ingest time quality, uptime-derived time/restart, sequence gaps, duplicates, replay, out-of-order and protocol-specific semantics.
- **Schema:** IPv4/IPv6/dual stack, NAT, VRF/VLAN, interfaces, next-hop/BGP, QoS/DSCP/ECN, TCP/ICMP fields, application-provenance and bounded enterprise extensions.
- **Durability:** UDP pre-acceptance loss is observable/best-effort; certified local durable acceptance creates stable replay identity; Sensor restart/Core/WAN/Kafka/ClickHouse interruption proves idempotent replay and explicit terminal-gap accounting.
- **Data quality:** socket/kernel drops where observable, receiver queue drops, malformed/unsupported, template misses, sequence gaps, rate limits, sampling unknown, clock skew, spool/storage pressure and downstream rejection/lag are distinguishable.
- **Fairness/resource safety:** one exporter or template/cardinality storm cannot monopolize Flow Sensor workers/memory/queue/disk; Flow/Log role isolation prevents starvation of Network monitoring.
- **Temporal enrichment:** DHCP/IPAM/entity/hostname/DNS/user/session/site/ASN/Geo/cloud/application changes do not rewrite historical identity incorrectly; conflicts preserve uncertainty; directory membership without session evidence cannot assert user attribution.
- **Privacy:** domain/application is not presented as URL; no fabricated path/query; public-IP Geo is approximate; user-attributed Flow access/export obeys permission/audit and surveillance restrictions.
- **Observation points:** replay of the same source record can dedupe; legitimate multi-exporter/interface observations are retained and are not globally five-tuple-deduplicated.
- **Coverage:** compatible SNMP/interface counter comparison accounts for sampling/direction/observation point and yields qualified Flow Coverage/consistency, including UNKNOWN when comparison is invalid.
- **Analytics/UI:** bounded top-N, conversation, site/interface/application/QoS, matrix and Sankey behavior; exporter/source health; Template Explorer; Data Quality; interface→Investigate Traffic context; query/cardinality/time bounds.
- **Performance:** datagrams/sec, records/sec, bytes/sec, exporter count, records/datagram, fields/record, template rate/churn, sampling mix, IPv4/IPv6 mix, enrichment cost, socket/queue loss, spool growth/replay drain, Core/Kafka/ClickHouse insert/rollup load and concurrent Flow query latency.
- **False-alert semantics:** Flow disappearance/degradation/anomaly alone cannot assert no traffic, link down, device down or root cause when other credible evidence contradicts it.
- **Redundancy:** each certified exporter failover/multi-collector pattern proves duplicate/loss/template/session behavior; a generic load-balancer assumption is not certification evidence.

Capacity results are profile-specific. No universal Flow records/sec, exporter-count or retention guarantee is created by this section.
