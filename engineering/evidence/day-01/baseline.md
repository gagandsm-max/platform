# Platform v1.0 — Day 01 Engineering Baseline & Evidence Report

**Document ID:** PLATFORM-EVID-DAY01-001
**Date:** 2026-09-06
**Status:** **VERIFIED DAY-1 ENGINEERING BASELINE**
**Governing Standard:** `AGENTS.md`, `Platform_13_Engineering_Development_and_Docker_Standards.md`
**Authoritative Mother Reference:** v3.2.1

> This document records Day-1 bootstrap engineering evidence. It is **not** a product certification, compliance certificate, or performance certification.

---

## 1. Environment & Source Metadata

| Parameter | Value |
| :--- | :--- |
| **Canonical Repository** | `https://github.com/gagandsm-max/platform` |
| **Baseline Main SHA** | `82089b0a7f78f7b57fc2f504ae1fe01c7de3539a` |
| **Feature Branch** | `feature/day-01-engineering-baseline` |
| **Mother-Reference Version** | `v3.2.1` |
| **Docker Engine Version** | Record from `make doctor` at verification time |
| **Docker Compose Version** | Record from `make doctor` at verification time |

---

## 2. Component & Dependency Architecture Baseline

Separate **technology/runtime** from **Platform_07 OSS catalogue dependency ID**.
Catalogue IDs below match mother reference **v3.2.1**. Languages/runtimes (Go, Java SE, Agent binary) have **no fictitious OSS IDs**.

| Component | Technology / Runtime | Base Image / Version | Direct Pinned Dependencies | Catalog Reference |
| :--- | :--- | :--- | :--- | :--- |
| **PostgreSQL** | Relational transaction store | `postgres:16-alpine` | PostgreSQL 16.x | **OSS-007** |
| **ClickHouse** | Columnar analytical store | `clickhouse/clickhouse-server:24.8-alpine` | ClickHouse 24.8.x | **OSS-008** |
| **Kafka** | Internal streaming backbone | `apache/kafka:3.8.0` | Kafka 3.8.0 | **OSS-002** |
| **Valkey** | Cache / ephemeral state | `valkey/valkey:7.2-alpine` | Valkey 7.2.x | **OSS-009** |
| **Platform Core** | Java 21 + Spring Boot control plane | `eclipse-temurin:21-jre-alpine` (build: `maven:3.9.9-eclipse-temurin-21-alpine`) | Spring Boot 3.3.4; Spring Kafka; JDBC/PostgreSQL driver; logstash-logback-encoder 7.4 | **OSS-001** (Spring Boot), **OSS-116** (logstash-logback-encoder); Kafka via **OSS-002**; PG via **OSS-007** |
| **Platform Sensor** | Go collector runtime | `alpine:3.20` (build: `golang:1.23-alpine`) | Go 1.23 standard library | Runtime only — **no OSS ID for Go language** |
| **Platform Agent** | Host endpoint agent | `alpine:3.20` (build: `golang:1.23-alpine`) | Go 1.23 stdlib (Linux + Windows cross-build) | Runtime only — **no OSS ID for Agent/Go** |
| **Platform UI** | React SPA + HTTPS reverse proxy | `nginx:1.27-alpine` (build: `node:20-alpine`) | React 18.3.1; TypeScript (lock 5.9.3); Vite (lock 5.4.21); `@vitejs/plugin-react` (lock 4.7.0) | **OSS-080** (React); **OSS-110** (NGINX USE-DEV/TOOLING); **OSS-111** Node; **OSS-112** npm (YELLOW/USE-TOOLING); **OSS-113** Vite; **OSS-114** plugin-react; **OSS-115** TypeScript |
| **Platform Analytics** | Python analytics worker | `python:3.12-slim` | NumPy 2.1.1; Polars 1.8.2 | **OSS-104** (NumPy), **OSS-103** (Polars) |
| **Keycloak / OPA / CEL / GoSNMP** | Identity / policy / SNMP (not all wired Day-1) | — | Catalogue-selected / candidate per Platform_07 | **OSS-010** Keycloak; **OSS-011** OPA (CANDIDATE); **OSS-012** CEL; **OSS-020** GoSNMP |

---

## 3. Verification baseline

Re-run and record actual results after each evidence update:

* `make doctor`
* `docker compose config -q`
* `make test-unit`
* `make test-integration`
* `make test-contract`
* `make test-security-smoke`
* `make test`

Update counts/SHA from the run that accompanies the evidence commit — do not claim “certified” product status.

---

## 4. Resource footprint note

`docker stats` snapshots are operational observations for the Day-1 stack only. They are **not** SNWP-2500 certification results (see spike **P0-06**).

---

## 5. Dependency reproducibility audit

* Docker images: pinned tags (no floating `latest` for Platform services).
* Go modules: stdlib Sensor/Agent; `go 1.23`.
* Maven: Spring Boot parent `3.3.4`; logstash-logback-encoder `7.4` (**OSS-116**).
* npm: `package-lock.json` + `npm ci` for reproducible UI builds.
* Python: `numpy==2.1.1`, `polars==1.8.2`.

---

## 6. Known Day-1 limitations

1. Development PKI only.
2. Protocol collectors are foundational (spool/mTLS/health); full protocol packs follow later days.
3. Compose default is single Active Core; dual-Core VIP/fencing remains **P0-04**.

---

## 7. Authoritative P0/TBD spikes

Per `Platform_12_Decision_Source_Register.md` §5 (engineering spike records under `engineering/spikes/`):

1. P0-01 Two-Core Kafka implementation/failover / durable ACK
2. P0-02 Two-Core ClickHouse replication/promotion/fencing
3. P0-03 PostgreSQL failover/fencing/rewind
4. P0-04 Core fencing/VIP failure matrix
5. P0-05 DR RPO/RTO/data-loss objectives
6. P0-06 SNWP-2500-v1 workload constants/results
7. P0-07 Streaming Sensor Flow/Syslog certified rates
8. P0-08 GNN model/runtime/compute/enablement
9. P0-09 Compliance certification programme/scope/status
10. P0-10 SKU/licensing quantities beyond fixed behavioural rules

Spike files must not convert unknowns into authoritative acceptance criteria.
