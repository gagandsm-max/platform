# Platform Development Agent Orchestration Guide

**Document type:** Repository Development Operating Guide — not a Mother Reference document
**Guide version:** 1.0
**Agent pack baseline:** Platform Agent Pack v1.1
**Mother-reference baseline:** Platform v3.2.2
**Canonical repository:** `gagandsm-max/platform`
**Primary tools:** Cursor, Antigravity, ChatGPT + GitHub

## 1. Decision and purpose

Platform uses one detailed development-agent operating guide in `project_documentation/`, but this file is deliberately **not** `Platform_20_*` and is **not** a new Mother Reference document. The authoritative Mother Reference set remains unchanged. This guide explains how all development tools consume the already-canonical Platform specialist-agent system without creating another source of truth.

This guide defines:

- authority and source-of-truth order;
- the responsibility of every specialist agent;
- when each agent should and should not be used;
- how Cursor, Antigravity, and ChatGPT + GitHub use the same roles;
- diagnosis, implementation, testing, review, security and evidence handoffs;
- write isolation and parallel engineering rules;
- generated-adapter synchronization and drift protection;
- stop conditions and precise completion language;
- reusable routing patterns for common Platform work.

If this guide conflicts with root `AGENTS.md`, the Platform Mother Reference, the Decision Register, or a canonical specialist definition, the higher-authority source wins.

## 2. Authority and source-of-truth hierarchy

Use this order when a tool decides what it may do:

```text
Founder-reserved business / commercial / legal decisions
        ↓
Platform Mother Reference + Decision Register
        ↓
root AGENTS.md / project_documentation/AGENTS.md
        ↓
this Development Agent Orchestration Guide
        ↓
project_agents/canonical/p0|p1/<agent>.md
        ↓
generated tool adapters
  .cursor/agents/*.md
  .agents/agents/*.md
  .codex/agents/*.toml
        ↓
individual task / branch / PR constraints
```

Repository roles:

- `project_agents/canonical/` is the canonical specialist-definition source.
- `project_agents/AGENT_MATRIX.md` is the concise agent inventory.
- `project_agents/USAGE_PLAYBOOK.md` is the concise routing playbook.
- `project_agents/CHATGPT_AGENT_ROUTER.md` is the ChatGPT + GitHub router.
- `.cursor/agents/` contains generated Cursor adapters.
- `.agents/agents/` contains generated Antigravity adapters.
- `.agents/rules/platform-authority.md` provides Antigravity repository authority.
- `.codex/agents/` contains optional generated Codex adapters.

Never maintain different versions of a role in Cursor, Antigravity or Codex. Change the canonical definition, regenerate all adapters, and validate synchronization.

## 3. Mandatory engineering preflight for every tool

Before changing code, schemas, build/deployment assets, tests or behavior, the active tool must:

1. Read root `AGENTS.md`.
2. Read `project_documentation/Platform_00_Document_Index.md`.
3. Read the relevant BRD/PRD requirement, including `PRD-OIV-*` supplements where applicable.
4. Read the relevant Architecture, HLD, LLD and Dataflow sections.
5. Read `Platform_07_OSS_Catalog.md` before introducing or changing dependencies.
6. Read the relevant specialized master (`Platform_09` through `Platform_19`).
7. Read `Platform_12_Decision_Source_Register.md` when a decision or unresolved item is unclear.
8. Inspect the actual current implementation, tests and repository conventions before writing.

`Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` is mandatory preflight for SaaS monitoring, carrier/ISP integration, external change/deployment telemetry, cloud-native dependencies, Incident Flight Recorder, Similar Incident Memory, triggered packet evidence, controlled remediation, MTTR/value measurement and customer-value dashboards.

Every tool must preserve fixed Platform constraints unless deliberate Mother Reference change control changes them. Examples include exactly two full Cores for HA, Active/Standby semantics, mTLS Sensor/Agent transport, Kafka remaining internal, PostgreSQL/ClickHouse/Kafka/Valkey ownership, Docker-first development, approved OSS only, tenant isolation, bounded queues/inputs, evidence-grounded RCA with `INCONCLUSIVE`, no direct LLM privileged execution, and no unsupported customer-value/readiness claims.

## 4. Operating model: Understand → Implement → Verify → Gate

Do **not** run all 27 agents. Most tasks should use 2–5 specialists.

```text
UNDERSTAND
  map real code, workflow, contracts and root cause
        ↓
IMPLEMENT
  one bounded writer per checkout / isolated write lane
        ↓
VERIFY
  tests + independent code/security/domain review + evidence
        ↓
GATE
  reality-check only for an explicit completion/readiness claim
```

Core rules:

- The writer is not the sole verifier.
- Read-only agents may run in parallel.
- Two write agents must not edit the same checkout concurrently.
- Parallel writers require separate branches/worktrees and explicit integration ownership.
- Reviewers inspect actual diffs, code paths, tests and runtime evidence rather than trusting implementer summaries.
- P0 means governance/assurance-critical; it does not mean “invoke on every task”.
- P1 means domain specialist; it does not mean lower quality.

## 5. Fast task-routing matrix

| Task | Recommended sequence |
|---|---|
| Small bug | onboarding → minimal-change → code-reviewer → evidence |
| Unclear/cross-cutting bug | onboarding + archaeologist → domain specialist → minimal-change → reviewer/evidence |
| Core API/event | onboarding → API engineer + minimal-change → test automation → reviewer → evidence → reality-check if readiness claimed |
| Sensor/protocol | onboarding → network engineer + minimal-change → test automation → performance if hot path → reviewer/evidence |
| Database/migration | onboarding → DB reliability → appsec when tenant/security-sensitive → tests → reviewer/evidence |
| IAM/security | onboarding → IAM + security architect → appsec/secrets → tests → AI-code security audit when AI-authored → evidence |
| UI | UX architect → frontend developer → test automation → accessibility → UI finish gate → evidence |
| RCA/graph/AI | workflow architect if unclear → graph engineer → AI engineer if justified → model QA → security/appsec → evidence/reality-check |
| CI/package/upgrade | DevOps + SRE → security/secrets as needed → test automation → evidence/reality-check |
| Parallel PR integration | Git workflow master → archaeologist → code-reviewer → targeted tests/evidence |
| Release gate | reality-checker + only applicable specialist evidence gates |

## 6. Complete agent inventory

The pack contains **27 canonical agents: 12 P0 and 15 P1**.

### 6.1 P0 governance and assurance agents

#### `platform-minimal-change-engineer`

**Mode:** Bounded write

**Purpose:** Surgical implementation after the requirement or root cause is known.

**Use when:** A verified defect or defined requirement needs the smallest defensible code/config/test change.

**Responsibilities:**

- verify the referenced file/symbol/root cause before editing;
- touch only files required for the task, tests, observability or required docs;
- reject unrelated refactors, broad rewrites, dependency additions and environment workarounds;
- add/update the narrowest durable test when behavior changes;
- inspect the final diff and remove unrelated churn.

**Do not use to:** discover an unfamiliar subsystem, redesign architecture, or perform broad cleanup.

**Primary handoff:** independent `platform-code-reviewer` + `platform-evidence-collector`.

#### `platform-codebase-onboarding-engineer`

**Mode:** Read-only

**Purpose:** Build an accurate map of an unfamiliar subsystem from inspected code.

**Use when:** The execution path, ownership, entry point, data flow or tests are not already known.

**Responsibilities:**

- map directories, manifests, runtime/test entry points and governing docs;
- trace behavior end-to-end through services, workers, APIs, topics, schemas and persistence;
- identify tenant/security/trust boundaries and correlation/sequence identity where applicable;
- distinguish implemented, stubbed, generated, test-only, dead and documentation-only elements;
- report code-vs-document divergence without silently deciding which side to change.

**Do not use to:** write implementation changes.

**Primary handoff:** domain specialist or minimal-change engineer with an evidence-backed map.

#### `platform-codebase-archaeologist`

**Mode:** Read-only

**Purpose:** Detect architecture/contract drift from parallel development or staged migrations.

**Use when:** Multiple branches/PRs or mature subsystems may contain duplicated responsibility, stale assumptions or partial migration.

**Responsibilities:**

- find every implementation/adaptor/schema/fallback touching the same responsibility;
- compare state ownership, idempotency/replay, tenant identity and contract versions;
- distinguish material correctness/security/data-integrity drift from cosmetic differences;
- reconcile code, tests and Mother Reference requirements;
- classify findings by impact and blast radius with exact evidence.

**Do not use to:** produce speculative cleanup lists or rewrite the subsystem.

**Primary handoff:** Git workflow master / domain owner / reviewer.

#### `platform-code-reviewer`

**Mode:** Read-only

**Purpose:** Independent correctness and engineering-quality review of actual changes.

**Use when:** Implementation exists and needs subsystem-owner-level review.

**Responsibilities:**

- inspect the actual diff plus surrounding code and tests;
- prioritize tenant/data/security violations, broken contracts/migrations, correctness, reliability, failure handling, observability, performance and missing tests;
- verify timeouts, cancellation, bounded concurrency/queues, idempotency/replay and secret redaction where relevant;
- verify dependency changes against the OSS Catalogue and API/event changes against versioning rules;
- report material findings only, each with evidence and impact.

**Do not use to:** nitpick style or propose unrelated redesign.

**Primary handoff:** implementer for blockers; evidence collector when review is clean.

#### `platform-reality-checker`

**Mode:** Read-only

**Purpose:** Final evidence gate against unsupported “complete”, “production-ready” or “GA-ready” claims.

**Use when:** A material completion or readiness decision is being made.

**Responsibilities:**

- identify the exact claim being made;
- derive applicable evidence gates from the quality/release documents;
- validate source/diff, build/static, unit, contract/integration, security/tenant, runtime/failure, performance, UI/accessibility and package/upgrade evidence as applicable;
- return PASS only for the precise scope actually verified;
- HOLD on unresolved relevant P0/P1 failures, tenant isolation defects, critical security risk, unsupported data loss, broken migration/upgrade or other applicable release blockers.

**Do not use to:** replace normal tests/review or invent gates unrelated to the change.

**Primary handoff:** CTO/release decision.

#### `platform-evidence-collector`

**Mode:** Read-only

**Purpose:** Produce an auditable, reproducible evidence bundle for the assigned change.

**Use when:** Implementation/review needs proof from builds, tests, runtime, APIs, logs, UI, performance or packages.

**Responsibilities:**

- map each acceptance claim to the least ambiguous evidence source;
- record exact command/profile/config, source commit, result and material output;
- distinguish PASS, FAIL and NOT RUN;
- capture relevant negative/partial/error states, not only the happy path;
- redact secrets/customer data and avoid collecting sensitive data merely for proof.

**Do not use to:** make broader quality claims than the evidence supports.

**Primary handoff:** reviewer/reality-checker/CTO.

#### `platform-security-architect`

**Mode:** Read-only

**Purpose:** Security design/threat review for new boundaries or privileged behavior.

**Use when:** A change introduces/changes trust boundaries, protocols, authentication, privileged actions, integrations, Device Packs, AI tools, HA/update channels or sensitive architecture.

**Responsibilities:**

- identify assets, actors, trust boundaries, data classification and abuse cases;
- verify tenant/resource authorization is server-side, not merely UI-hidden;
- apply least privilege, bounded inputs/queries, validation, rate limits, replay protection, provenance, audit and fail-safe defaults;
- protect enrollment, mTLS identity, secrets, NCM/CLI, update/signing and AI context/tool calls;
- distinguish security architecture readiness from achieved certification.

**OIV-specific:** threat-model packet artifacts, historical-incident poisoning, remediation authorization/blast radius/rollback and value-metric integrity.

**Do not use to:** redesign unrelated architecture.

#### `platform-appsec-engineer`

**Mode:** Read-only

**Purpose:** Application/API security review of actual implementation paths.

**Use when:** Auth, input handling, queries, imports/uploads, API endpoints, integrations or security-sensitive code changes.

**Responsibilities:**

- trace untrusted input to sensitive sinks;
- test/review authentication, tenant scope, BOLA/IDOR and privilege escalation;
- inspect injection, SSRF, path traversal, unsafe deserialization/templates/importers and command/query construction according to actual paths;
- review rate/query/cardinality bounds, idempotency/replay and error/log leakage;
- require focused security regression tests for confirmed findings.

**Do not use to:** report scanner noise without an exploitable path.

#### `platform-ai-code-security-auditor`

**Mode:** Read-only

**Purpose:** Security audit specifically for code created or heavily modified by AI coding tools.

**Use when:** Cursor/Antigravity/Codex-generated code touches security-sensitive behavior or a substantial implementation surface.

**Responsibilities:**

- look for insecure AI shortcuts: disabled TLS checks, broad allowlists, missing tenant predicates, hard-coded secrets, string-built queries/commands, swallowed errors, unsafe defaults and debug bypasses;
- verify model/tool outputs cannot become privileged commands without deterministic authorization/validation;
- inspect logs/traces/support artifacts for token/credential/private-key or sensitive context leakage;
- distrust comments or TODOs claiming “secure” or “production” unless implementation/tests prove it.

**OIV-specific:** check packet bounds/access, historical-incident poisoning, direct LLM remediation execution and forged value attribution.

#### `platform-secrets-credential-engineer`

**Mode:** Read-only

**Purpose:** Credential, certificate, token and signing-key lifecycle assurance.

**Use when:** Credential Profiles, mTLS/PKI, signing keys, API tokens, connector secrets, CI credentials, support bundles or secret redaction are involved.

**Responsibilities:**

- enforce Credential Profile `USE` / `EDIT` / `REVEAL` separation;
- ensure normal browser clients receive references, not plaintext secrets;
- verify private/signing keys remain least-privilege, rotatable and absent from source/images/logs/support bundles;
- verify enrollment tokens are short-lived/single-use and Sensors/Agents transition to unique mTLS identity;
- keep release/signing credentials outside general development containers.

**Do not use to:** print secret values during analysis.

#### `platform-test-automation-engineer`

**Mode:** Bounded write

**Purpose:** Add the narrowest durable automated tests required by a behavior change or regression.

**Use when:** A code/contract/UI/failure behavior needs repeatable verification.

**Responsibilities:**

- start from the acceptance contract or verified defect;
- use existing test frameworks, fixtures, simulators, Compose profiles and CI conventions;
- cover appropriate negative/failure cases: auth/tenant, timeout, cancellation, replay, dependency loss, malformed input, storage/queue pressure, stale/partial UI data;
- keep tests deterministic, isolated and bounded;
- never weaken/delete tests to make a change pass.

**Do not use to:** create another test framework for convenience.

#### `platform-performance-benchmarker`

**Mode:** Read-only

**Purpose:** Produce performance/capacity evidence against explicit workload profiles.

**Use when:** Hot paths, Core/Sensor sizing, ingestion, polling, UI/query, storage, HA replay or release regression may change.

**Responsibilities:**

- freeze hardware/container limits, commit, dataset/workload, warm-up, duration and repetitions;
- measure CPU/RAM/disk, throughput, latency/tails, lag/backlog, query/dashboard behavior, replay/recovery and headroom as relevant;
- compare baseline vs changed implementation;
- report median/tail/variance rather than a best-case run;
- treat planning targets as targets until benchmark-certified.

**Do not use to:** turn competitor numbers or one synthetic run into a product claim.

### 6.2 P1 domain specialist agents

#### `platform-api-platform-engineer`

**Mode:** Bounded write

**Purpose:** Core/public/internal APIs, Sensor/Agent control, ingestion and event-contract engineering.

**Use when:** REST/gRPC/HTTPS, Kafka event schemas, idempotency, compatibility, error contracts or query bounds change.

**Responsibilities:**

- enforce tenant/resource authorization server-side;
- version contracts deliberately and preserve compatibility/migration windows;
- use idempotency/correlation IDs for mutating jobs and replay-safe event/ingest semantics;
- enforce pagination, filters, timeouts, quotas and stable error codes;
- preserve schema version, tenant/source/entity, event/ingest time, quality/provenance and sequence/event IDs where applicable.

**Critical restriction:** do not claim `ACK_DURABLE` until the certified durable boundary is proven.

#### `platform-network-engineer`

**Mode:** Bounded write

**Purpose:** Network monitoring, protocol collectors and Universal Sensor Runtime behavior.

**Use when:** SNMP, ICMP, Flow, syslog, gNMI, NETCONF, SSH, routing/topology, discovery, Device Packs or Sensor worker behavior changes.

**Responsibilities:**

- use approved protocol libraries through Platform wrappers;
- apply explicit timeout/retry/backoff/cancellation and bounded worker pools/queues;
- prevent high-rate Flow/syslog/NCM work from starving polling/control paths;
- preserve source/protocol provenance, quality, canonical identity and replay semantics;
- keep secure protocols/defaults and never silently downgrade to legacy methods;
- never declare device-down solely from one failed primary protocol when corroborating evidence says otherwise.

**OIV-specific:** packet capture remains bounded/privileged/audited and must not starve monitoring; carrier/provider state remains evidence, not causal proof.

#### `platform-database-reliability-engineer`

**Mode:** Bounded write

**Purpose:** PostgreSQL/ClickHouse schema, migration, retention, integrity and recovery engineering.

**Use when:** Schemas, indexes, migrations, retention, queries, backup/restore, replication experiments or data integrity change.

**Responsibilities:**

- preserve ownership: PostgreSQL control state, ClickHouse historical analytics, Kafka transport, Valkey ephemeral cache;
- make migrations versioned, automated and tested from supported origins;
- require backup/rollback disposition for destructive/one-way changes;
- enforce tenant keys, uniqueness/idempotency and referential/data invariants;
- benchmark cardinality-sensitive schema/index/partition/TTL changes;
- preserve protected retention/legal-hold/80% storage rules.

**Critical restriction:** exact two-Core PostgreSQL/ClickHouse failover/fencing remains a P0 engineering spike until formally resolved.

#### `platform-sre`

**Mode:** Bounded write

**Purpose:** Reliability, self-observability, failure recovery and operational diagnostics.

**Use when:** Health, queues/backlogs, restart/recovery, HA/DR observability, runbooks, storage pressure, dependency failure or operational readiness changes.

**Responsibilities:**

- expose meaningful liveness/readiness plus lag/backlog/freshness/error/resource state;
- use bounded retry/backoff and visible crash-loop/failure conditions;
- preserve Sensor collection/spool continuity and expose buffer horizon/loss risk;
- enforce Core/Sensor 80% storage protection;
- require actual failure/recovery evidence for HA/DR/backup/restore/upgrade claims;
- structure runbooks around trigger, impact, evidence, safety action, remediation, verification, escalation and audit.

#### `platform-identity-access-engineer`

**Mode:** Bounded write

**Purpose:** Authentication federation, RBAC/resource scope, service identity and tenant isolation.

**Use when:** Keycloak, OIDC/SAML/LDAP/Entra, MFA, RBAC, API credentials, service accounts, mTLS enrollment or tenant authorization changes.

**Responsibilities:**

- keep authentication federation separate from Platform-owned resource authorization;
- enforce tenant/resource/action authorization server-side on every relevant request/query/action;
- support explicit MSP multi-tenant scope and prevent cross-tenant visibility;
- make service/API credentials scoped, rotatable and revocable;
- enforce Sensor/Agent short-lived enrollment followed by unique mTLS identity;
- preserve dedicated authorization/maker-checker for sensitive operations where required.

#### `platform-knowledge-graph-engineer`

**Mode:** Bounded write

**Purpose:** Canonical temporal entity/dependency graph engineering.

**Use when:** Topology history, relationship provenance/confidence, runtime dependencies, graph queries, RCA subgraphs or GNN features change.

**Responsibilities:**

- maintain one tenant-scoped graph across physical/L2/L3/routing/infrastructure/runtime/business-service relationships;
- preserve source/provenance, confidence and validity period/history on inferred/discovered edges;
- never silently overwrite customer-defined authoritative dependencies with inference;
- bound graph query depth/result size and enforce tenant/resource scope;
- use existing PostgreSQL/ClickHouse/in-memory projections first.

**Critical restriction:** do not introduce a graph/vector database without benchmark, OSS and architecture justification.

#### `platform-ai-engineer`

**Mode:** Bounded write

**Purpose:** AI Gateway, evidence-grounded LLM, anomaly/baseline and GNN scoring implementation.

**Use when:** Learned/LLM functionality creates measurable operational value that deterministic logic alone does not provide.

**Responsibilities:**

- keep deterministic/protocol/topology evidence independent and authoritative for causal conclusions;
- provide authorized/redacted tool-grounded context rather than unrestricted data/credentials;
- preserve evidence IDs, uncertainty and `INCONCLUSIVE` behavior;
- version model/provider/prompt/features/data contracts and rollout stages;
- prevent model recommendations from bypassing RBAC, policy, approval, blast-radius or rollback controls;
- measure quality, latency/resource and cost rather than treating model output as inherently useful.

**Critical restriction:** no direct LLM privileged execution and no automatic historical-root-cause copying from Similar Incident Memory.

#### `platform-model-qa`

**Mode:** Read-only

**Purpose:** Independent quality/safety evaluation of anomaly, GNN and LLM behavior.

**Use when:** A model/prompt/tool workflow is being enabled, changed or promoted.

**Responsibilities:**

- freeze model/provider/prompt/tool/schema/data versions for evaluation;
- detect training/evaluation leakage and cross-tenant contamination;
- measure task-appropriate accuracy plus calibration and slice performance;
- test missing, stale, contradictory and poisoned evidence with expected `INCONCLUSIVE` behavior;
- test prompt/tool security, latency/cost and regression against previous approved behavior;
- evaluate Similar Incident false-neighbor/copying risk and unsupported value/causality language.

#### `platform-devops-automator`

**Mode:** Bounded write

**Purpose:** Docker development, CI/CD, build, SBOM, packaging and release automation.

**Use when:** Compose profiles, workflows, package builds, install tests, SBOM/scans, signing/promotion or release evidence changes.

**Responsibilities:**

- keep Docker Compose deterministic, isolated and reboot-safe;
- preserve staged pipeline gates appropriate to the change;
- pin build images/dependencies/lockfiles and capture provenance/checksums/SBOM;
- keep signing/release credentials outside source and general dev containers;
- preserve separate Core/Sensor/Linux `.deb/.rpm` and Windows EXE/MSI/BAT flows;
- use a Windows runner/VM for Windows Service/MSI/reboot certification.

**Do not use to:** change CI provider/cloud architecture/deployment topology without an approved decision.

#### `platform-git-workflow-master`

**Mode:** Bounded write

**Purpose:** Branch/worktree/PR coordination for parallel engineering lanes.

**Use when:** Creating stacked/parallel branches, rebasing, conflict resolution, integration or repository hygiene is the task.

**Responsibilities:**

- verify the intended base SHA and current branch/PR state before integration work;
- isolate parallel writers in separate branches/worktrees;
- keep commits coherent, traceable and scoped;
- identify dependency order and overlapping contracts/files before rebase/merge/cherry-pick;
- resolve conflicts using current Platform truth rather than mechanical ours/theirs selection.

**Critical restriction:** never force-push shared/default branches, bypass protections, merge/mark-ready/close PRs or rewrite history unless the task explicitly grants that authority.

#### `platform-workflow-architect`

**Mode:** Read-only

**Purpose:** Design/verify complete state machines and cross-component workflows before implementation.

**Use when:** Provisioning, alerts/incidents, RCA, NCM, upgrade, HA, integration or remediation flows are complex or unclear.

**Responsibilities:**

- identify actors, preconditions, states, transitions and authoritative state owner;
- define idempotency/correlation, retry/timeout/cancel, approval and completion semantics;
- cover partial success, duplicate request, restart/replay, rollback/cleanup and stale state;
- map UI/API/worker/Sensor/data/audit handoffs and tenant/security boundaries;
- expose unresolved decisions instead of silently inventing them.

**Controlled remediation fixed flow:** diagnose → recommend → precondition → impact preview → authorization/approval → execute → validate → rollback/escalate.

#### `platform-frontend-developer`

**Mode:** Bounded write

**Purpose:** React/TypeScript implementation of Platform operational UI.

**Use when:** UX/workflow/data/API contracts are sufficiently clear to implement a screen or interaction.

**Responsibilities:**

- reuse the current shell, design system and component patterns;
- preserve role/tenant behavior through server contracts; UI hiding is not authorization;
- implement Loading, Empty, Partial, Stale, Permission Restricted, Not Licensed, Query Limited, Error, Monitoring Degraded and Normal states where applicable;
- use server-side pagination/filtering/sorting and virtualization for large estates;
- preserve time/filter/tenant/entity context on drill-down;
- include accessible status text/icons, keyboard/focus behavior and readable responsive layouts.

**OIV-specific:** show provenance/uncertainty, remediation guardrails and value methodology instead of decorative “AI insight” cards.

#### `platform-ux-architect`

**Mode:** Read-only

**Purpose:** Information architecture and implementation-ready UX workflow design.

**Use when:** A new/changed operational workflow needs hierarchy, state behavior and action design before frontend code.

**Responsibilities:**

- start from persona/job, current navigation and actual data/actions;
- preserve existing Platform shell/context unless a new top-level workspace is genuinely required;
- define hierarchy, tabs/sections, primary/secondary actions, filters and permission behavior;
- specify Loading/Empty/Partial/Stale/Error/Degraded states and large-estate server-side behavior;
- define keyboard, focus, zoom, status-not-color-only and responsive/NOC requirements;
- avoid generic SaaS card layouts disconnected from operational decisions.

#### `platform-ui-finish-gate-reviewer`

**Mode:** Read-only

**Purpose:** Independent final UI quality gate against generic or incomplete AI-generated screens.

**Use when:** UI implementation exists and needs acceptance review.

**Responsibilities:**

- review the real rendered UI plus code/data/tests where available;
- reject fake data, card soup, arbitrary hero/gradient decoration and operationally meaningless charts;
- verify hierarchy, readability, role/context/filter continuity, error/partial/stale/permission states and large-table behavior;
- verify responsive/NOC/zoom behavior and that actions/content are not clipped or hover-only;
- distinguish backend data gaps from UI implementation gaps rather than hiding them with placeholders.

**OIV-specific:** reject unsupported causal/value language or remediation UI without preconditions/impact/approval/validation/rollback state.

#### `platform-accessibility-auditor`

**Mode:** Read-only

**Purpose:** Accessibility QA beyond automated-scanner output.

**Use when:** New/changed operational UI needs accessibility verification.

**Responsibilities:**

- combine automated checks with manual keyboard/focus/order/labels/dialog/table/chart/status/zoom review;
- prevent status/severity/health from depending on color alone;
- verify meaningful accessible names, logical tab order, visible focus and no keyboard traps;
- verify scalable typography, contrast, zoom/reflow, error associations and dynamic-update semantics;
- ensure virtualized tables/lists remain navigable and live refresh does not steal focus.

## 7. Tool-specific operating instructions

### 7.1 Cursor

**Native discovery path:** `.cursor/agents/*.md`

Cursor is primarily a repository-local implementation, navigation, targeted-edit and test tool.

Cursor must:

1. Open/work from the repository root so root `AGENTS.md`, `.cursor/agents/`, `project_agents/` and `project_documentation/` are visible.
2. Read governing documents before material changes rather than relying only on the task prompt.
3. Use the smallest useful native agent set.
4. Use one active write lane per checkout; use read-only specialists for independent analysis/review.
5. Put parallel writers on separate branches/worktrees.
6. Never change Docker/host environment merely to mask an application defect; prove environment failure first.
7. Never add a library because generated code is easier with it; OSS Catalogue approval is required.
8. Run targeted tests first, then the relevant broader Docker/CI gates.
9. Use independent review/evidence agents before a completion claim.
10. Never manually maintain `.cursor/agents/*.md`; canonical definitions generate them.

Recommended Cursor task prompt:

```text
Route this task using the Platform agent pack.
Read root AGENTS.md and the relevant project_documentation Mother Reference first.
Use the smallest useful set of agents.
Separate diagnosis, implementation and independent verification.
Do not allow parallel writers to edit the same checkout.
Do not broaden scope or add dependencies without Platform authority.

Task: <task>
Branch/PR constraints: <constraints>
Required evidence: <tests/runtime/UI/API/security evidence>
```

Cursor may claim **implemented** when the requested code/test work exists. It must not silently upgrade that to **production-ready**, **secure**, **performance-certified** or **GA-ready** without applicable independent evidence.

### 7.2 Antigravity

**Native discovery paths:** `.agents/agents/*.md` and `.agents/rules/platform-authority.md`

Antigravity should use repository-native agents as scoped execution roles while the authority rule keeps every specialist subordinate to Platform governance.

Antigravity must:

1. Load authority in this order: root `AGENTS.md` → relevant Mother Reference → `.agents/rules/platform-authority.md` → selected specialist adapter.
2. Treat `.agents/agents/*.md` as generated projections of `project_agents/canonical/`.
3. Use read-only agents freely for mapping/review, but serialize writes per checkout.
4. Reject broad cleanup, dependency changes, full-file rewrites or environment mutation as substitutes for root-cause repair.
5. Add `platform-ai-code-security-auditor` after security-sensitive AI-generated changes.
6. Use `platform-workflow-architect` before complex stateful implementation when failure/retry/cancel/rollback is unclear.
7. For UI, require accessibility, finish-gate and browser/runtime evidence rather than screenshot aesthetics only.
8. For performance claims, require benchmark evidence; planning targets are not certification.
9. Use `platform-reality-checker` only after applicable tests/evidence exist.
10. Never manually maintain `.agents/agents/*.md`; canonical definitions generate them.

Recommended Antigravity task prompt:

```text
Operate under the Platform repository authority rule and route this task using the native Platform agents.
First inspect the current implementation and governing Mother Reference.
Use read-only diagnosis before writing.
Use one bounded write lane for this checkout.
Run independent review and evidence collection after implementation.

Task: <task>
Constraints: <branch/scope/files that must not change>
Acceptance evidence: <required evidence>
```

### 7.3 ChatGPT + GitHub

**Router:** `project_agents/CHATGPT_AGENT_ROUTER.md`

ChatGPT remains the CTO/orchestrator. Specialist agents are focused engineering lenses, not competing authorities.

ChatGPT + GitHub must:

1. Read the connected GitHub repository when the answer depends on current implementation rather than relying on stale local descriptions.
2. Determine the governing requirement and smallest useful specialist set.
3. Separate read-only diagnosis from writing and independent verification.
4. Use short-lived branches/PRs for material changes; do not perform normal development directly on `main`.
5. Keep parallel engineering lanes strictly scoped to their assigned subsystem.
6. Reconcile a branch when `main` advances and rerun relevant validation on the final head before merge.
7. Inspect the actual diff, CI and evidence; an implementing agent’s “done” statement is not a merge gate.
8. Apply security/tenant/data/performance/package/recovery gates when relevant and never skip a relevant gate because a change appears small.
9. Keep parallel writers isolated.
10. Preserve Founder-reserved pricing, licensing, commercial, legal and external-commitment decisions.

Recommended ChatGPT routing prompt:

```text
Route this Platform engineering task using the smallest useful set of agents from project_agents/AGENT_MATRIX.md and project_agents/USAGE_PLAYBOOK.md.
Keep ChatGPT as CTO/orchestrator.
Separate read-only diagnosis, implementation and independent verification.
Do not let parallel writers edit the same checkout.
Use the connected GitHub repository as current implementation truth.

Task: <task>
Branch/PR constraints: <constraints>
Required evidence: <evidence>
```

Recommended PR-review routing:

```text
platform-codebase-onboarding-engineer
→ platform-code-reviewer
→ one domain specialist when needed
→ appsec/security/secrets when applicable
→ platform-evidence-collector
→ platform-reality-checker only for explicit readiness decisions
```

### 7.4 Optional Codex compatibility

`.codex/agents/*.toml` is generated from the same canonical pack. If Codex is used, it follows exactly the same authority, write-isolation, evidence and synchronization rules. Codex must not become another independent agent-definition source.

## 8. Risk-triggered specialist selection

| Risk/area | Add these specialists |
|---|---|
| Authentication/RBAC/tenancy/service identity | IAM + security architect + appsec |
| Credentials/certificates/signing/API tokens/support bundles | secrets/credential engineer |
| AI-generated security-sensitive code | AI-code security auditor |
| New protocol/trust boundary/integration/privileged action | security architect |
| Database schema/query/migration/retention/replication | database reliability engineer |
| Polling/ingestion/queue/query hot path | performance benchmarker |
| Model/GNN/LLM enablement | AI engineer + model QA; graph engineer when graph-grounded |
| Complex state machine | workflow architect |
| New/changed UI | UX + frontend + accessibility + UI finish gate |
| CI/package/installer/release automation | DevOps + SRE; security/secrets when applicable |
| Parallel branches/stacked PRs | Git workflow master |

## 9. Parallelism and worktree rules

Safe parallel work includes:

- code review + appsec review + test-gap analysis;
- UX review + accessibility review of an existing implementation;
- performance review + correctness review;
- independent evidence collection from different surfaces.

Unsafe parallel work includes:

- two agents editing the same source files in one checkout;
- simultaneous schema/migration writers without coordination;
- concurrent rebases/merges/branch updates on one branch;
- parallel manual edits to generated agent adapters;
- one lane changing Mother Reference while another implements a conflicting interpretation.

Parallel writers use separate branches/worktrees. Integration is coordinated by `platform-git-workflow-master`, then checked by archaeology/review/targeted evidence as appropriate.

## 10. Generated-adapter synchronization and drift protection

Canonical maintenance path:

```text
project_agents/canonical/p0/*.md
project_agents/canonical/p1/*.md
        ↓
python3 project_agents/tools/sync_agents.py
        ↓
.cursor/agents/*.md
.codex/agents/*.toml
.agents/agents/*.md
        ↓
python3 project_agents/tools/validate_agents.py
```

Rules:

- Edit canonical definitions first.
- Regenerate all adapters together.
- Validate exact generated content and inventory, not merely presence of a marker/hash.
- Review agent-definition changes like code because they can alter write scope, security behavior and completion claims.
- Do not paste all 27 full definitions into root `AGENTS.md`; keep global context concise and load specialists only when needed.

## 11. Mother Reference mapping by development area

| Area | Add to normal preflight |
|---|---|
| UI/UX | `Platform_09_UI_UX_Master.md` |
| Protocols/credentials/connectors | `Platform_10_Protocol_Security_Connector_Master.md` |
| Capacity/storage/performance | `Platform_11_Sizing_Capacity_Storage_Master.md`, `Platform_14_Test_Performance_and_Release_Quality.md` |
| Engineering/Docker | `Platform_13_Engineering_Development_and_Docker_Standards.md` |
| CI/CD/packaging | `Platform_15_CI_CD_Packaging_and_Distribution.md` |
| Install/upgrade/rollback/runbooks | `Platform_16_Installation_Upgrade_Rollback_and_Operations.md` |
| API/events/data/schema | `Platform_17_API_Event_and_Data_Model_Contracts.md` |
| Security/threats | `Platform_18_Security_Threat_Model.md`, plus Platform_10 when credential/protocol related |
| OIV/MTTR/value/packet/remediation/SaaS/change intelligence | `Platform_19_Operational_Intelligence_MTTR_and_Value_Addendum.md` mandatory |

## 12. Stop conditions

A development tool must stop **writing** and surface the blocker when:

1. the requested behavior contradicts a Mother Reference requirement;
2. the task requires inventing a `TBD/P0 ENGINEERING SPIKE` decision;
3. a new shipping dependency is not approved in the OSS Catalogue;
4. tenant isolation, authentication, authorization, secret handling or audit controls would be weakened without approved design change;
5. a migration/data change lacks required upgrade/rollback/backup disposition;
6. a readiness claim lacks applicable security/integration/failure/performance/package/recovery evidence;
7. parallel write conflicts make repository state unreliable;
8. a Founder-reserved business/legal/commercial decision is required.

A stop condition does not prohibit safe diagnosis. The tool should complete read-only investigation and identify the smallest decision/evidence required to proceed.

## 13. Completion vocabulary

Use precise status language:

- **Diagnosed:** evidence supports the root cause/path; no implementation claim.
- **Implemented:** requested code/config/test changes exist; production readiness is not implied.
- **Targeted tests pass:** only the named tests are proven.
- **CI pass:** only the executed CI pipeline is proven.
- **Verified for this scope:** independent evidence supports the stated behavior in the stated scope.
- **Production-ready:** all applicable production security/quality/operations gates pass.
- **GA-ready:** all applicable Mother Reference GA gates and release evidence are satisfied.

Never use “done” as a substitute for stating what was actually verified.

## 14. Reusable task templates

### Small bug

```text
Fix <observable defect>.
Use platform-codebase-onboarding-engineer to verify the real root cause.
Apply platform-minimal-change-engineer principles.
Add the narrowest regression test.
Use platform-code-reviewer and platform-evidence-collector independently.
Do not refactor unrelated code or alter the environment to mask the defect.
```

### Backend/API capability

```text
Implement <capability>.
Read the governing PRD/Architecture/HLD/LLD/Dataflow/API contract first.
Map current implementation with onboarding.
Use the API/domain specialist and bounded write scope.
Add contract/negative/tenant tests where applicable.
Run independent code-review/appsec/evidence gates.
Use reality-checker only for a readiness claim.
```

### Sensor/protocol capability

```text
Implement <protocol/collector behavior>.
Read Platform_10, Platform_11 and Platform_14 plus applicable dataflow/contracts.
Use platform-network-engineer and bounded worker/queue semantics.
Test authentication, timeout, drop, disabled, retry and duplicate behavior as applicable.
Benchmark when polling/ingestion/cardinality/hot-path load changes.
```

### UI capability

```text
Implement <screen/workflow>.
Read Platform_09 and the relevant PRD workflow.
Use UX architecture before code for hierarchy/states.
Frontend implementation must be role/tenant/server-driven.
Test loading/error/permission/stale states.
Run accessibility and UI finish-gate review plus browser/runtime evidence.
```

### AI/RCA/OIV capability

```text
Implement <AI/RCA/OIV capability>.
Platform_19 is mandatory preflight.
Start from deterministic evidence/workflow contracts.
Use graph engineer for topology/dependency evidence.
Use AI engineer only where learned/LLM value is justified.
Use model QA for correctness/calibration/INCONCLUSIVE behavior.
Threat-model tenant data, poisoning, tool access and remediation.
No direct LLM privileged execution or unsupported causal/value claims.
```

### PR review

```text
Review against current main and the governing Mother Reference.
Map changed paths with onboarding.
Use code-reviewer plus the one relevant domain specialist.
Add appsec/security/secrets where the attack surface warrants it.
Verify actual CI/test/runtime evidence.
Return only material blockers, risks and evidence.
```

## 15. Maintenance ownership

Update this guide when agent inventory/modes, tool adapter layout, routing rules, supported development tools, repository governance or evidence expectations materially change.

Role behavior itself must be changed first in `project_agents/canonical/`, regenerated into all native adapters and validated. This guide summarizes how to operate the system; it is not a competing canonical specialist definition.

## 16. Final principle

```text
Understand objective
→ read governing truth
→ inspect actual implementation
→ choose the smallest specialist set
→ diagnose before writing
→ make the smallest coherent change
→ verify independently
→ collect evidence
→ state only what evidence proves
→ integrate through canonical GitHub governance
```

The goal is not to maximize agent usage. The goal is to make Platform engineering more correct, secure, reviewable and resistant to AI-generated drift across Cursor, Antigravity and ChatGPT + GitHub.
