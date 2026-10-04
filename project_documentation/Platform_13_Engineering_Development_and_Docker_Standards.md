# Platform — Engineering, Development & Docker Standards

**Document ID:** PLATFORM-ENGDEV-001
**Version:** 3.3.0
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.1 document preserves the normative content of the listed v3.1 source documents (via v3.2). Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

## 1. Purpose

Defines the mandatory engineering execution model and the Docker Compose development/test environment.
It governs repository structure, modularity, coding discipline, OSS dependency use, performance rules, logging,
developer-laptop isolation, reboot-safe local state, Compose profiles, secrets, networks, volumes and the Windows
Agent certification exception.

## 2. Source-document provenance

- `Platform_13_Engineering_Execution_Standards.md` (v3.1) — fully incorporated below.
- `Platform_14_Docker_Development_Environment.md` (v3.1) — fully incorporated below.

## 3. Platform — Engineering Execution Standards

> Consolidated from `Platform_13_Engineering_Execution_Standards.md` v3.1. Requirements in this section remain normative.

## 1. Purpose

This document defines how Platform code is structured and changed. It exists to prevent architecture drift, bloated implementations, hidden dependencies and inconsistent behavior across human engineers and coding agents.

## 2. Mandatory preflight for every engineering task

Before modifying code, the engineer/agent must read: `Platform_00_Document_Index.md`, the relevant BRD/PRD requirement, Architecture/HLD/LLD sections, `Platform_07_OSS_Catalog.md`, and any module-specific master. If the requested change contradicts a governing document, implementation stops until change control resolves it.

## 3. Repository principles

- Clean-room codebase independent of ThoughtData/NetAI proprietary assets.
- One canonical repository: `https://github.com/gagandsm-max/platform` (USER-FIXED). Local clones are working copies; authority follows GitHub merge. Multi-repo splits require later engineering approval.
- Normal development uses short-lived branches; material changes use pull requests. Do not push normal development changes directly to the protected/default branch. Do not force-push shared/default branches. Commits must be traceable to the change; history preserves decision provenance.
- Selecting GitHub as source of record does **not** freeze a CI provider. GitHub Actions may be evaluated later and is not a fixed architecture decision unless separately approved.
- Clear top-level boundaries for Core, UI, Sensor, Agent, NCM/analytics workers, contracts, packaging, test assets and documentation.
- Shared schemas/contracts are versioned and generated where practical; duplicate hand-written DTOs across languages are discouraged.
- No customer-specific behavior should require source-code changes when it belongs in policies, Device Packs, connectors or configuration.

### 3.1 Mother-document change control

Mother documents are READ-MOSTLY and authoritative. Coding agents must not casually modify them while implementing features. Mother documents **must** be updated under deliberate change control when factual errors, contradictions, approved product/architecture decision changes, formally selected dependencies, resolved P0/TBD items, omitted requirements, or implementation/document conflicts are found. Cross-document reconciliation and Decision Register updates are required where decisions change. Implementation must not permanently diverge from mother-reference truth.

## 4. Modularity

Core is a modular application, not dozens of mandatory microservices. New process boundaries require evidence: independent scaling, fault isolation, security boundary, language/runtime necessity or materially different lifecycle. "Microservice because it is cleaner" is not sufficient.

Sensors use one Universal Sensor Runtime with role profiles rather than divergent products unless benchmark evidence requires specialization.

## 5. Coding rules

- Explicit error types/codes at module boundaries.
- Context/timeouts/cancellation on network and storage operations.
- Retry only on classified transient failures; exponential backoff + jitter; no infinite tight loops.
- Idempotency for provisioning, event ingestion, jobs and configuration application.
- Bounded queues and concurrency. No unbounded goroutines/threads/collections.
- Resource ownership and shutdown paths are explicit.
- All public APIs/events are versioned according to Platform contract masters.
- Secrets never appear in logs, exceptions, traces or UI error payloads.

## 6. Dependency discipline

Only dependencies admitted by `Platform_07_OSS_Catalog.md` may be shipped. New OSS requires version/license/transitive/CVE/provenance review and a catalogue update before merge.

## 7. Performance discipline

Every hot-path change must consider allocations, serialization, DB scans, query cardinality and network round trips. Device count alone is not a reason to multiply Core compute. UI interactions must use pagination/virtualization/aggregation rather than moving large datasets into browsers.

## 8. Logging/diagnostics

All services implement the common structured logging contract and stable event/error codes. Logs are bounded and redacted. Significant state changes emit audit events where required.

## 9. Code review gates

A change cannot merge until relevant tests pass, architecture/OSS/security impacts are reviewed, migrations are reversible or explicitly one-way with approval, and documentation/traceability are updated when behavior changes.

## 10. Definition of done

"Done" means code + tests + observability + upgrade/migration behavior + documentation + acceptance evidence. A compile-success or passing happy-path test is not completion.

## 4. Platform — Docker Development & Test Environment

> Consolidated from `Platform_14_Docker_Development_Environment.md` v3.1. Requirements in this section remain normative.

## 1. Decision

Platform local development and routine automated testing standardize on **Docker Compose**. Podman compatibility is not required for v1. The goal is deterministic isolation from other projects on the developer laptop and reproducibility across engineers/coding agents.

## 2. Host prerequisites

The host should require only Docker Engine/Moby-compatible runtime with Compose plugin, Git, an editor/IDE and normal OS tooling. Project-specific JDK, Go, Python, Node.js, Kafka, PostgreSQL, ClickHouse and Valkey installations on the host are prohibited as development prerequisites.

## 3. Reboot requirement

The laptop is frequently shut down/rebooted. Development must therefore be restart-safe:

- Docker daemon starts at OS boot.
- Durable state uses named volumes.
- `dev` long-lived services default to `restart: unless-stopped` unless explicitly documented otherwise.
- Health checks determine readiness.
- Services retry dependencies with bounded backoff.
- No startup correctness depends on a previous terminal session.
- A documented `make dev-up`, `make dev-down`, `make dev-status`, `make dev-logs`, `make dev-reset` or equivalent contract is required.

`dev-reset` must be explicit/destructive and must not silently delete unrelated Docker assets or intentional named volumes.

## 4. Compose profiles

Required profiles: `dev`, `test`, `ha`, `scale`, `security`, `dr`. Optional module-specific profiles may exist.

### `dev`
Minimal interactive stack; persistent named volumes; fast rebuild; source mounts where appropriate.

### `test`
Disposable isolated volumes/network; seeded fixtures; deterministic exit code; no dependency on `dev` state.

### `ha`
Two Core nodes with Active/Standby behavior, sensor buffering/reconnect and fault-injection harness.

### `scale`
Synthetic SNMP/flow/syslog/gNMI/API/infrastructure load generators with resource telemetry.

### `security`
Negative auth/TLS/protocol cases, dependency scans and security regression suites.

### `dr`
Backup/export, site-loss simulation, restore and data-integrity verification.

## 5. Network isolation

Use named Docker networks. Expose only ports needed by the developer. Avoid globally common host ports where collisions are likely; allow `.env`/profile port overrides without changing source. Production protocol defaults must not be confused with local host mappings.

## 6. Secrets

No real customer credentials in Compose files or repository. Use local secret files ignored by Git, test-only generated credentials or secret injection. Example `.env` files contain placeholders only.

## 7. Volumes

Separate durable service state from generated caches/build artifacts. Named volumes require documented ownership and backup/reset behavior. Test profiles use disposable unique volumes.

## 8. Build caching

Use multi-stage images and BuildKit cache appropriately, while maintaining reproducible lockfiles and pinned base images/digests for CI certification.

## 9. Laptop resource control

The default `dev` profile must be usable without starting all scale/HA services. Heavy data services start only when required. Resource limits/reservations should prevent runaway local workloads. Synthetic scale tests may require dedicated CI hardware rather than the laptop.

## 10. Logs

Container stdout/stderr must be structured and bounded by Docker logging settings. Stateful service logs must not grow without limit. Development logs remain searchable by component/correlation ID.

## 11. Windows Agent exception

Agent business logic and cross-compilation can run in Docker. Final Windows Service/SCM/MSI/reboot testing requires an isolated Windows VM/CI runner. This is required for correctness; Linux containers cannot certify Windows service semantics.


## v3.3.0 engineering rules for OTel and pack depth

- OpenTelemetry Collector/contrib, SDK and semantic-convention versions used for certification must be pinned and recorded through OSS/release governance; upstream "latest" is never a production dependency contract.
- Development-status OTel semantic conventions must be isolated behind versioned normalization adapters and contract fixtures.
- Trace ingestion code must use bounded decode/validation, explicit cancellation/timeouts and measured allocation profiles; no unbounded attribute/map copying.
- Pack development remains declarative/data-driven where possible. Official/Private packs may not introduce arbitrary executable code merely for convenience.
- Every Official Certified Pack requires deterministic known-good and malformed/version-drift fixtures suitable for offline CI.
- Test-only browser tooling is not a shipped browser-synthetic runtime. Runtime admission is separate.
