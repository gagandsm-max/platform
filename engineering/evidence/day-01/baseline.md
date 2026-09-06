# Platform v1.0 — Day 01 Engineering Baseline & Evidence Report

**Document ID:** PLATFORM-EVID-DAY01-001  
**Date:** 2026-09-06T18:08:45+05:30  
**Status:** Certified Engineering Baseline  
**Governing Standard:** AGENTS.md, Platform_13_Engineering_Development_and_Docker_Standards.md  
**Authoritative Mother Reference:** v3.2.1  

---

## 1. Environment & Source Metadata

| Parameter | Value |
| :--- | :--- |
| **Canonical Repository** | `https://github.com/gagandsm-max/platform` |
| **Baseline Main SHA** | `82089b0a7f78f7b57fc2f504ae1fe01c7de3539a` |
| **Feature Branch** | `feature/day-01-engineering-baseline` |
| **Mother-Reference Version** | `v3.2.1` |
| **Docker Engine Version** | `Docker version 29.7.2, build a7dcaa6` |
| **Docker Compose Version** | `Docker Compose version v5.5.0` |
| **Operating System** | `Linux 6.8.0-52-generic x86_64` |
| **Host Disk Space** | `113 GiB available` |
| **Host RAM** | `19 GiB available` |

---

## 2. Component & Dependency Architecture Baseline

| Component | Technology / Runtime | Base Image / Version | Direct Pinned Dependencies | Catalog Reference |
| :--- | :--- | :--- | :--- | :--- |
| **PostgreSQL** | Relational Transaction Store | `postgres:16-alpine` | PostgreSQL 16.x | OSS-001 |
| **ClickHouse** | Columnar Analytical Store | `clickhouse/clickhouse-server:24.8-alpine` | ClickHouse 24.8.x LTS | OSS-002 |
| **Kafka** | KRaft Streaming Backbone | `apache/kafka:3.8.0` | Kafka 3.8.0 | OSS-003 |
| **Valkey** | Fast In-Memory Cache | `valkey/valkey:7.2-alpine` | Valkey 7.2.x | OSS-004 |
| **Platform Core** | Spring Boot Control Plane | `eclipse-temurin:21-jre-alpine` (Build: `maven:3.9.9-eclipse-temurin-21-alpine`) | Spring Boot 3.3.4, Spring Kafka, HikariCP, PostgreSQL JDBC, logstash-logback-encoder 7.4 | OSS-010, OSS-116 |
| **Platform Sensor** | Universal Collector Runtime | `alpine:3.20` (Build: `golang:1.23-alpine`) | Go 1.23 standard library | OSS-011 |
| **Platform Agent** | Host Endpoint Agent | `alpine:3.20` (Build: `golang:1.23-alpine`) | Go 1.23 standard library (Linux & Windows cross-build) | OSS-012 |
| **Platform UI** | React SPA + Nginx Proxy | `nginx:1.27-alpine` (Build: `node:20-alpine`) | React 18.3.1, TypeScript 5.5.4 (lock: 5.9.3), Vite 5.4.6 (lock: 5.4.21), @vitejs/plugin-react (lock: 4.7.0) | OSS-013, OSS-110 to OSS-115 |
| **Platform Analytics** | Scientific Analytics Engine | `python:3.12-slim` | NumPy 2.1.1, Polars 1.8.2 | OSS-103, OSS-104 |

---

## 3. Pre-Change & Post-Change Verification Baseline

### 3.1 Doctor Verification (`make doctor`)
```text
Checking Docker Engine... OK (Docker version 29.7.2, build a7dcaa6)
Checking Docker Compose... OK (Docker Compose version v5.5.0)
Checking Dev TLS Certificates... OK
Checking Port Conflicts in range 20000-29999... OK (No conflicts detected)
Checking Host Disk Space... 113G available
Checking Host RAM... 19Gi available
==> Platform Doctor Check Passed.
```

### 3.2 Unit Test Matrix (`make test-unit`)
* **Go Tests (Sensor & Agent):**
  * `TestSpoolWriteAndDrain`: PASS (0.004s)
  * `TestWatermarkThreshold`: PASS (0.004s)
  * `TestIdentityGeneration`: PASS (0.004s)
  * `TestMemoryQueueOperations`: PASS (0.009s)
* **Java Core Tests (Maven Surefire):**
  * `testApplicationContextLoads`: PASS
  * `testSensorHeartbeatRejectsWithoutClientCert`: PASS
  * `testSensorHeartbeatAcceptsWithClientCert`: PASS
  * *Result:* Tests run: 3, Failures: 0, Errors: 0, Skipped: 0. BUILD SUCCESS
* **UI Typecheck:**
  * `tsc`: PASS (0 errors)

### 3.3 Integration Verification Matrix (`make test-integration`)
20 out of 20 automated tests passed against the running stack:
* Test 1: Repository Integrity & Specification Traceability — PASS
* Test 2: `make doctor` Host Environment & Port Range Check — PASS
* Test 3: Dev TLS Certificate Generation & SAN Validation — PASS
* Test 4: Multi-stage Container Builds — PASS
* Test 5: Compose Profiles & Enforceable Resource Limits — PASS
* Test 6: Platform Dev Stack Startup & Health Convergence (6s) — PASS
* Test 7: PostgreSQL Bootstrap & Schema Migration — PASS
* Test 8: ClickHouse Analytical Store Health & Telemetry Tables — PASS
* Test 9: Kafka KRaft Streaming Backbone Internal Isolation — PASS
* Test 10: Valkey Fast Cache Health, Auth & Read/Write — PASS
* Test 11: Core Control Plane Health over HTTPS (`:28443/actuator/health`) — PASS
* Test 12: Core API Version & Cleartext Host Port Absence — PASS
* Test 13: Core Quad-Datastore Connectivity — PASS
* Test 14: Sensor Instances Startup & Spool Volumes — PASS
* Test 15: Sensor 80% Storage Watermark Protection Logic — PASS
* Test 16: UI HTTPS-Only Port 23000 & Port 80 Listener Absence — PASS
* Test 17: UI Reverse-Proxy Routing to Core over HTTPS — PASS
* Test 18: Sensor -> Core mTLS Authentication & Access Control — PASS
* Test 19: Kafka Failure Observability & Auto-Recovery (503 on pause, 200 on unpause) — PASS
* Test 20: Persistence, Structured Logs & Workstation Isolation (Host isolation & synthetic test verified) — PASS

---

## 4. Resource Utilization & Process Footprint

Captured via `docker stats --no-stream`:

| Container Name | CPU % | Memory Usage / Limit | Memory % | Block I/O | PIDs | Health Status |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `platform-ui` | 0.00% | 2.70 MiB / 256 MiB | 1.05% | 0B / 4.1kB | 2 | healthy |
| `platform-analytics` | 0.01% | 14.84 MiB / 512 MiB | 2.90% | 0B / 0B | 2 | healthy |
| `platform-sensor-net` | 0.86% | 2.76 MiB / 256 MiB | 1.08% | 0B / 0B | 8 | healthy |
| `platform-sensor-infra`| 0.09% | 2.40 MiB / 256 MiB | 0.94% | 0B / 0B | 8 | healthy |
| `platform-core` | 5.68% | 219.60 MiB / 1 GiB | 21.45% | 0B / 49.2kB | 45 | healthy |
| `platform-postgres` | 0.06% | 22.17 MiB / 512 MiB | 4.33% | 0B / 225kB | 8 | healthy |
| `platform-clickhouse` | 37.60%| 290.80 MiB / 1.5 GiB | 18.93% | 0B / 2.38MB | 700 | healthy |
| `platform-kafka` | 5.30% | 274.60 MiB / 1 GiB | 26.82% | 0B / 2.71MB | 90 | healthy |
| `platform-valkey` | 0.21% | 3.71 MiB / 256 MiB | 1.45% | 4.1kB / 0B | 7 | healthy |
| **Total Stack** | **~49.8% (burst)**| **~833.58 MiB / 5.44 GiB** | **~15.3%** | — | **870** | **9/9 Healthy** |

*Workstation Co-existence:* Foreign container `netai-linux-subscriber-agent` is running undisturbed, consuming 24.04 MiB (0.08% of host limit).

---

## 5. Dependency Reproducibility Audit

* **Docker Images:** Zero floating `latest` tags. Pinned versions used across all images.
* **Go Modules:** Standard library only for Sensor and Agent; `go.mod` files pinned to Go 1.23.
* **Maven Core:** Dependency versions managed via `spring-boot-starter-parent:3.3.4` and explicit `7.4` for `logstash-logback-encoder`.
* **npm UI:** `package.json` paired with complete `package-lock.json` (115 packages locked). Dockerfile updated to execute `npm ci` ensuring bit-for-bit reproducible builds.
* **Python Analytics:** Strictly pinned to `numpy==2.1.1` and `polars==1.8.2`. Unapproved packages (`scipy`, `requests`, `pandas`) purged.

---

## 6. Current Known Limitations

1. **Local PKI:** Certificates are development-only self-signed certificates with SANs for local docker network resolution.
2. **Collector Engines:** Protocol collection routines (SNMP, Flow, WMI) are structured skeletons with disk spool buffers; full protocol decoding logic is scheduled across Days 2–15.
3. **Core Redundancy:** Local Compose runs single active Core by default (`core-standby` profile exists for HA testing); automated dual-Core VIP failover requires multi-node test infrastructure.

---

## 7. Authoritative P0/TBD Spikes

Formally registered per `Platform_12_Decision_Source_Register.md` §5:
1. P0-01: Two-Core Kafka implementation/failover honoring no third full Core.
2. P0-02: Two-Core ClickHouse replication/promotion/fencing design.
3. P0-03: PostgreSQL two-Core failover/fencing/rewind mechanics.
4. P0-04: Core service fencing/VIP implementation and failure matrix.
5. P0-05: Contractual DR RPO/RTO and data-loss objectives.
6. P0-06: SNWP-2500-v1 metric/interface/event mix and certified performance results.
7. P0-07: Streaming Sensor certified Flow records/sec and Syslog EPS.
8. P0-08: GNN model architecture, training labels, inference latency, compute footprint.
9. P0-09: Compliance certification programme/scope and achieved status.
10. P0-10: Product SKUs/licensing quantities beyond fixed behavioral rules.
