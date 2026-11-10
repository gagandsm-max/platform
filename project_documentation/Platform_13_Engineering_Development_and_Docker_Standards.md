# Platform — Engineering, Development & Docker Standards

**Document ID:** PLATFORM-ENGDEV-001
**Version:** 3.3.0
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.1 document preserves the normative content of the listed v3.1 source documents (via v3.2). Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

## Purpose

Defines the mandatory engineering execution model and the Docker Compose development/test environment.
It governs repository structure, modularity, coding discipline, OSS dependency use, performance rules, logging,
developer-laptop isolation, reboot-safe local state, Compose profiles, secrets, networks, volumes and the Windows
Agent certification exception.

## Source-document provenance

- `Platform_13_Engineering_Execution_Standards.md` (v3.1) — fully incorporated below.
- `Platform_14_Docker_Development_Environment.md` (v3.1) — fully incorporated below.

## Platform — Engineering Execution Standards

> Consolidated from `Platform_13_Engineering_Execution_Standards.md` v3.1. Requirements in this section remain normative.

## 1. Purpose

This document defines how Platform code is structured and changed. It exists to prevent architecture drift, bloated implementations, hidden dependencies and inconsistent behavior across human engineers and coding agents.

## 2. Mandatory preflight for every engineering task

Before modifying code, the engineer/agent must read: `Platform_00_Document_Index.md`, the relevant BRD/PRD requirement, Architecture/HLD/LLD sections, `Platform_07_OSS_Catalog.md`, and any module-specific master. If the requested change contradicts a governing document, implementation stops until change control resolves it.

Engineering-tool preflight also requires canonical `AGENTS.md`, `project_agents/tool_authority.json` and `Platform_12_Decision_Source_Register.md` §18.1. Establish isolated task ownership before writing: concurrent writers use separate branches/worktrees and non-overlapping files, while shared-file work is sequenced or explicitly handed off. Writer output requires independent acceptance and does not grant merge or certification authority.

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

## Platform — Docker Development & Test Environment

> Consolidated from `Platform_14_Docker_Development_Environment.md` v3.1. Requirements in this section remain normative.

## 11. Docker decision

Platform local development and routine automated testing standardize on **Docker Compose**. Podman compatibility is not required for v1. The goal is deterministic isolation from other projects on the developer laptop and reproducibility across engineers/coding agents.

## 12. Host prerequisites

The host should require only Docker Engine/Moby-compatible runtime with Compose plugin, Git, an editor/IDE and normal OS tooling. Project-specific JDK, Go, Python, Node.js, Kafka, PostgreSQL, ClickHouse and Valkey installations on the host are prohibited as development prerequisites.

## 13. Reboot requirement

The laptop is frequently shut down/rebooted. Development must therefore be restart-safe:

- Docker daemon starts at OS boot.
- Durable state uses named volumes.
- `dev` long-lived services default to `restart: unless-stopped` unless explicitly documented otherwise.
- Health checks determine readiness.
- Services retry dependencies with bounded backoff.
- No startup correctness depends on a previous terminal session.
- A documented `make dev-up`, `make dev-down`, `make dev-status`, `make dev-logs`, `make dev-reset` or equivalent contract is required.

`dev-reset` must be explicit/destructive and must not silently delete unrelated Docker assets or intentional named volumes.

## 14. Compose profiles

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

## 15. Network isolation

Use named Docker networks. Expose only ports needed by the developer. Avoid globally common host ports where collisions are likely; allow `.env`/profile port overrides without changing source. Production protocol defaults must not be confused with local host mappings.

## 16. Secrets

No real customer credentials in Compose files or repository. Use local secret files ignored by Git, test-only generated credentials or secret injection. Example `.env` files contain placeholders only.

## 17. Volumes

Separate durable service state from generated caches/build artifacts. Named volumes require documented ownership and backup/reset behavior. Test profiles use disposable unique volumes.

## 18. Build caching

Use multi-stage images and BuildKit cache appropriately, while maintaining reproducible lockfiles and pinned base images/digests for CI certification.

## 19. Laptop resource control

The default `dev` profile must be usable without starting all scale/HA services. Heavy data services start only when required. Resource limits/reservations should prevent runaway local workloads. Synthetic scale tests may require dedicated CI hardware rather than the laptop.

## 20. Docker logs

Container stdout/stderr must be structured and bounded by Docker logging settings. Stateful service logs must not grow without limit. Development logs remain searchable by component/correlation ID.

## 21. Windows Agent exception

Agent business logic and cross-compilation can run in Docker. Final Windows Service/SCM/MSI/reboot testing requires an isolated Windows VM/CI runner. This is required for correctness; Linux containers cannot certify Windows service semantics.

## 22. Host-global shared Docker runtime exclusion

The self-hosted AmrudAi development/CI host shares one Docker daemon across local heavy runners and GitHub Actions jobs labeled `[self-hosted, Linux, X64, platform-ci]`. Concurrent Compose/runtime mutation on that host is prohibited.

### 22.1 Frozen invariant

Any workload that creates, removes, restarts, pauses or mutates shared Docker runtime resources on that host MUST participate in the **same** host-global runtime exclusion mechanism. Project-scoped Compose names, unique CI volume prefixes and foreign-container isolation checks do **not** replace host-global exclusion.

### 22.2 Canonical lock

Canonical lock path (stable):

`/home/gagan/amrudai-evidence/runtime/platform-dev-runner.lock`

Acquisition MUST use an OS advisory lock compatible with existing local execution: `flock` / `fcntl.flock`. Do **not** invent a second unrelated lock file, lock service or soft-mutex. Kernel lock ownership is authoritative for exclusion. Human-readable Issue #211 ACQUIRED/RELEASED records may supplement provenance but do **not** replace the OS exclusion primitive. Never unlink, truncate-replace or recreate the lock inode as a release mechanism.

Ownership metadata written while holding the lock SHOULD identify:

- owner type (for example `ci-integration`, `codex-desktop-e2e`, `codex-cli`, `antigravity-runtime`, `cursor-heavy`, `manual-integration`);
- GitHub run ID / attempt, or local session identity;
- repository SHA under test;
- acquisition time (UTC);
- responsible process PID where applicable.

### 22.3 Mandatory participants

Workloads that MUST acquire the same lock before shared Docker mutation include, where applicable:

- CODEX Desktop heavy E2E;
- Codex CLI heavy runtime;
- Antigravity runtime testing;
- Cursor heavy runtime;
- manually launched Platform integration/E2E tests that mutate Compose/containers/volumes/networks;
- GitHub Actions jobs on `[self-hosted, Linux, X64, platform-ci]` that execute Platform integration/runtime/Compose tests.

Static-contract, source-only and UI/unit jobs do **not** automatically require this lock unless they mutate shared Platform runtime resources. CI job ownership and hold duration are governed by `Platform_15`; acceptance fixtures are owned by `Platform_14`.

### 22.4 Contention behavior

When another owner holds the lock, a contending workload MUST NOT:

- kill the foreign workload;
- delete foreign containers (including `agent-e2e-*`);
- stop another tool's Compose stack;
- clear volumes;
- reset the environment to seize the host;
- bypass workstation/foreign-container isolation checks.

It MAY wait within its bounded job/session timeout. If it cannot obtain the lock safely, it MUST fail visibly as **runtime-busy** rather than mutating another workload.

### 22.5 Crash / cancellation

Lock ownership MUST be process-bound so process or job death releases the kernel lock automatically. Cleanup that runs after cancellation MUST clean only positively owned resources, terminate the caller's lock-holder process when this workload was the owner, and independently verify the lock is free afterwards. Foreign resources remain untouched.

---

## Phase 2 Enterprise RFP Readiness Reconciliation — Issue #269

Phase-2 enterprise breadth MUST NOT be implemented as customer-specific Core forks, duplicate services or dependency sprawl. Prefer Capability Packs, Integration Packs, versioned schemas and policy/configuration. Every new vendor adapter requires an actual supported API/protocol path, bounded failure behavior, tests, observability and exact-version compatibility evidence. Phase-2.5+ APM/DPI work is not an excuse to introduce its runtime/dependency footprint into Phase 2.

---

## Issue #272 engineering guardrails

Future Phase-2.5 implementation SHALL reuse existing identity/RBAC/tenancy/audit/graph/evidence contracts, avoid customer-specific forks, isolate high-volume trace/profile/packet workloads from control-plane correctness, add dependencies only through explicit admission, include deterministic fixtures for trace/RUM/profile/packet parsing, include privacy/redaction/security tests and preserve offline/air-gap build/test paths where the certified deployment profile requires them.

Generated instrumentation/decoder code or third-party agents may not bypass repository quality/security gates.

## P0 documentation and execution acceptance — 2026-11-06

Founder priority for the current handoff is complete Mother Reference correction before product implementation dispatch. The subsequent product order remains Linux → Network+Flow → Infrastructure → Syslog, subject to real shared runtime, data, authority and security dependencies. This instruction supersedes earlier immediate dispatch wording; it does not rewrite historical certifications or release dates. Mother Reference set v3.3.0, existing BUILD IDs and the existing canonical owners remain in force.

The governing acceptance crosswalk is Platform_00 §10.1, Platform_08 §§4.1–4.3 and Platform_12 §29, together with AGENTS. The implementation-plan requirements below operationalize that same gate; they do not create a second acceptance authority. Formal independent audit remains required; collaborative writer/subagent cross-review is supporting review, not its substitute.

### One all-six-capability gate

`DOC_P0_BLOCKED` is the aggregate documentation state until **all six** capabilities below pass independent semantic review of the exact candidate source. Partial coverage, a template, a requirement count, a structural validator PASS, or a successful CI run cannot clear it. Each review result records reviewer, candidate SHA/tree, exact owner sections, unresolved defects and disposition. Documentation acceptance is separate from implementation, runtime testing, production certification and deployment.

| Gate | Required reviewable result | Existing owning references |
|---|---|---|
| TRD | Concrete technical requirements with inputs/outputs, authority, states/errors, budgets or explicitly blocked budget decisions, compatibility and finite acceptance. | Platform_02/03/04/05/10/11/17/18 and specialized owners; global index Platform_08. |
| JOURNEYS | Complete normal, permission-denied, invalid/stale input, failure/retry/cancel, recovery and audit flows from user/system entry through persistence and visible outcome. | Platform_06, applicable Platform_02/09/17/20/21/22 sections. |
| UI_UX | Implementation-level screen/workspace/component behavior, source/query/mutation, role/resource scope, form validation and loading/empty/no-data/error/denied/recovery states, plus accessibility/acceptance. | Platform_09/17 and Platform_14 UI quality gates. |
| DATA_CONTRACTS | Exact applicable DB/API/event/schema ownership, tenant identity, validation, error/idempotency/replay semantics, migration/compatibility/rollback and security/failure tests. | Platform_05/17/18 plus current source and migration manifest. |
| BUILD_PLAN | Existing BUILD IDs have phase/profile applicability, current evidence, design/implementation/certification predecessors, source allocation, sequencing and integration gates. | Root `Platform_Master_Build_Sequence_Register_v1.0.md`; Platform_12 decision provenance. |
| MODULE_PLANS | Filled executable plan records for every in-scope module/slice, including source boundaries, contract, dependencies, acceptance, rollout/rollback and evidence; unclosed records remain explicit blockers. | Build Register execution-plan ledger and the owning design/contract sections. |

### Executable module plan record

Before a writer is dispatched, its existing BUILD/slice/profile record SHALL identify:

1. Stable REQ/PRD and exact owning design, normal/failure flow, UI and DB/API/event sections; phase and OS/domain profile; accepted decision provenance.
2. Actual source paths, bounded target-file/module allocation and exclusions; current source presence, default activation, runtime evidence and certified scope as separate fields.
3. Concrete inputs, outputs, schemas/endpoints/events and authoritative state transitions; tenant/entity/credential/fencing authority; validation/error behavior; idempotency, replay/dedupe, retry/cancellation, time and resource bounds as applicable.
4. Separate **design**, **implementation** and **certification** predecessors. A Linux slice may depend on an accepted shared Windows-compatible contract without requiring the Windows runtime to be finished; its certificate must then remain Linux-scoped. Whole cross-platform BUILD acceptance retains all required profiles.
5. Finite positive, negative, failure, security/tenant, migration/upgrade, capacity/non-interference and UI acceptance definitions, reproducible fixture identities and existing test entry points. No numeric performance target or vendor behavior may be invented to fill a table.
6. Configuration, monitoring/logs/support evidence, installation, rollout, rollback and runbook impact; destructive actions and recovery verification explicitly identified. Commands must exist and be applicable to the declared profile.
7. Current exact source/tree/CI/artifact evidence and unresolved gates; independent reviewer scope; safe integration order and downstream impact. Source or test presence is not a passing test result.

A family-level allocation or the thirty logical intelligence responsibilities in Platform_22 does not by itself meet this record. A design-blocked module must name the missing decision and its owner; it is not eligible for speculative implementation. No new BUILD ID or duplicate master is required to record the gap.

### Structural checks and semantic proof

`project_documentation/tools/validate_mother_references.py` explicitly provides structural protection. Its owner checks establish canonical filename, nonempty locator and formal-ID occurrence in the owner, not semantic completeness of that section. Keep the existing validator and existing documentation tests; this correction adds no validator, runner or CI code. Review must also trace requirement → design/journey/UI/contract → BUILD/source → acceptance definition → evidence status and test dependency counterexamples (standalone before HA; Linux before Windows; AI disabled; missing/partial telemetry; failed downstream dependency). The six-gate review in Platform_14 records the result.

### Evidence and workspace discipline

The 6 November operator terminal transcript establishes reported executable paths and Git states only. A clean status is not source review, test evidence, package signing or runtime health. Preserve untracked files, worktrees and stashes. Do not use environment resets, dependency installs or service operations to complete a documentation task. Current `Makefile` can create `.env` while parsing, and `scripts/generate-dev-certs.sh` rewrites development CA/key files; neither `make help` nor `make certs` is a preservation-safe read-only diagnostic. Use source inspection for this correction.
