# Platform — UI/UX, Navigation & Out-of-Box Dashboard Master

**Document ID:** PLATFORM-UIUX-001
**Version:** 3.3.0
**Status:** **Authoritative Mother Reference — UI/UX and Dashboard Specification**
**Product:** Platform (internal engineering/build identifier) / AmrudAi (commercial/customer-facing product name)
**Last reconciliation:** 2026-09-30 — Unified Network & Infrastructure Observability Phase-1 closure and eBPF integration (Issue #142)

## 1. Authority and no-assumption rule

This document expands the PRD's UI/UX and dashboard catalogue into an implementation reference. It does not invent customer operational thresholds or polling intervals. Values such as thresholds, refresh periods, SLA targets and retention remain policy-driven. Where this document defines a default UI layout/control, it is an **architecture-selected default** intended to make the product usable out of box and remains configurable where the security model permits.

The non-negotiable UX principles are: fast navigation; readable fonts; role-aware disclosure; UI-only normal administration; no silent failures; contextual troubleshooting; consistent filters/actions; light/dark theme; large-environment server-side data handling; and HTTPS/TLS-only access.


### 1.0 Normative requirements — `REQ-UI` family


This subsection is the **primary normative owner contract** for the `REQ-UI` family in this document. Each row is an implementation-binding SHALL statement for this owner; it is not an RTM index, ownership footer or summary-only catalogue.

**Shared normative envelope:** every Requirement ID in this subsection inherits the parent section's concrete feature/behavior contract (purpose, persona, dependencies, preconditions, primary workflow, failure/degraded behavior, acceptance outcome, maturity boundary). Implementation writers MUST use this envelope plus the distinctive SHALL below; the RTM one-liner alone is not a sufficient contract.

**Parent-section obligations incorporated by reference:** Parent section obligations for purpose, workflow, failure/degraded behavior, authorization/tenant boundary, acceptance outcome and maturity boundary.

| Requirement ID | Normative SHALL contract | Parent-section envelope | Boundary / non-goal |
|---|---|---|---|
| `REQ-UI-001` | Platform SHALL enforce: Native Platform Dashboard Studio with flexible composable dashboards; Grafana may inspire interaction patterns but is not copied/embedded. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-002` | Platform SHALL enforce: Create/group/folder/clone/import/export/share dashboards. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-003` | Platform SHALL enforce: Drag/drop/resizable panels and broad chart/table/topology/map/Sankey types. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-004` | Platform SHALL enforce: Light/dark theme, readable fonts, density options. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-005` | Platform SHALL enforce: Role-specific landing pages and permission-aware navigation. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-006` | Platform SHALL enforce: Global search, command palette, breadcrumbs, context side panel, favorites/recent/saved views. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-007` | Platform SHALL enforce: Inventory with server-side pagination, bulk actions and saved filters. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-008` | Platform SHALL enforce: Map Studio, Report Studio, MIB Studio, API Studio, Device Pack Studio. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-009` | Platform SHALL enforce: Report Studio PDF/CSV-only artifacts, effective commercial logo embedding in PDF, branding, schedules, obligations and attestation. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-010` | Platform SHALL enforce: SLA/OLA/Provider-SLA service-assurance engine with service-time, shift/team, maintenance, evidence-quality and audit rules. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-011` | Platform SHALL enforce: Accessibility, keyboard navigation, contrast and no color-only status. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-012` | Platform SHALL enforce: Mobile incident/dashboard experience; desktop for full admin/studios. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-013` | Platform SHALL enforce: Query protection: timeout/row/scan/concurrency/async export. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-014` | Platform SHALL enforce: Fast UI with caching/preaggregation/virtualization/lazy loading/query cancellation. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-015` | Platform SHALL enforce: No-context-switch NOC troubleshooting from entity/alert/incident context. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-016` | Platform SHALL enforce: Persistent contextual action bar + Troubleshoot drawer/workspace. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-017` | Platform SHALL enforce: On-demand bounded ICMP/traceroute/SNMP/TCP/DNS/SSH/API tests through eligible Sensor/Agent. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-018` | Platform SHALL enforce: Multi-vantage verification with contradictory evidence preserved. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-019` | Platform SHALL enforce: Verify Now deterministic diagnostic bundles/packs. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-020` | Platform SHALL enforce: Live Evidence distinguishes monitored state from fresh operator-triggered verification. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-021` | Platform SHALL enforce: Why/What Changed/Compare contextual investigation actions. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-022` | Platform SHALL enforce: Investigation Session preserves diagnostic/CLI/config/topology/flow evidence references and can link to incident. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-023` | Platform SHALL enforce: Risk-proportionate action friction; CLI supports stored/personal/one-time credentials without browser exposure of reusable secrets. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-024` | Platform SHALL enforce: Diagnostic jobs bounded by tenant/scope/target/rate/concurrency/timeout/cancellation and isolated from routine monitoring capacity. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |
| `REQ-UI-025` | Platform SHALL enforce: AI may explain/suggest permitted next tests but cannot fabricate results or bypass authorization/execution controls. Distinctive obligations are binding together with the shared normative envelope above and must be implemented without inventing unspecified HA mechanisms, endpoints, schemas or silent scope expansion. | Inherits shared envelope + parent section | Do not treat the RTM summary as complete; do not invent load-bearing behavior absent from this owner section or an explicitly cross-referenced specialized master. |



## 2. Global application shell

### 2.1 Primary navigation

```text
Home
Dashboards
Inventory
Topology & Maps
Alerts & Incidents
RCA & Investigations
Network Configuration (NCM)
IPAM
Reports & SLA
Integrations
Studios
  Dashboard Studio
  Report Studio
  Map Studio
  MIB Studio
  API Studio
  Device Pack Studio
Administration
Audit & Governance
Platform Operations
```

Navigation entries are shown/hidden/read-only/action-limited according to RBAC, resource scope, tenant and entitlement. Commercial entitlement and authorization are distinct decisions.

### 2.2 Global top bar

- Brand block: Displays the effective commercial product name (**AmrudAi** by default) and product logo. In the default dark shell header, uses the white logo (`amrud_whi.png`); in light header themes, uses the black logo (`amrud_blk.png`) to ensure visual contrast. Logo rendering must preserve aspect ratio without stretching. In white-label or MSP tenant configurations, displays the tenant-configured brand name and logo while maintaining platform structural integrity.
- Tenant/customer selector only for users authorized for multiple tenants.
- Global search across device/IP/MAC/serial/site/tag/interface/circuit/incident/alert/ticket/IPAM/NCM/DB/VM/cloud/K8s entities.
- Time range, timezone, comparison period and auto-refresh.
- Quick actions based on permission: Add Device, Create Dashboard, Add Maintenance, Run Investigation, Create Report, Open CLI.
- Platform notifications: Sensor, integration, report, licence, HA, approval and security conditions. The global per-user Notification Center/bell is governed by `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md`; this base catalogue is not the detailed owner.
- Context help and user menu: profile, preferences, sessions, API tokens, theme, timezone, language and logout.


## Display timezone UX — v3.2.2 handoff baseline

### Formal interactive timezone requirements

| Requirement ID | Normative UI/UX requirement |
|---|---|
| `REQ-TIME-002` | Interactive display shall use named IANA TZDB identifiers. Where no saved preference exists, first use defaults to the browser/device local IANA timezone. Users may select zones such as Asia/Kolkata, Europe/London, America/New_York or UTC; fixed `UTC+X` offsets must not model DST-sensitive regions. |
| `REQ-TIME-003` | Effective display timezone precedence is: explicit dashboard/view/session override → saved user `display_timezone` preference → browser/device local IANA zone → `tenant.timezone` → UTC. Explicit overrides may be preserved in URL/saved views; absence resolves per viewer preference. |

The global shell and applicable operational screens shall expose:

- a current effective timezone indicator;
- a searchable IANA zone picker;
- shortcuts: **Local** (browser/device), **Tenant Default** (`tenant.timezone`), **UTC**;
- saved user preference (`display_timezone`);
- temporary view/session override;
- URL/saved-view preservation of an explicit override;
- resolution without override per viewer preference precedence;
- report/export timezone disclosure whenever local timestamps are rendered.

Changing timezone updates labels and calendar grouping only. It must not rewrite underlying data identity, alert evaluation or operational policy schedules. Friendly current offsets may be shown as informational labels; canonical preference stores the IANA identifier.

### 2.3 Global navigation behavior

- Breadcrumbs preserve tenant → region → site → group/service → entity context.
- Clicking an entity may open a fast side panel; full page preserves original time/filter context.
- Favorites, recent entities/incidents/dashboards and saved views are first-class.
- `Ctrl/Cmd + K` command palette is supported for expert navigation/actions.
- Large tables use server-side pagination/filtering/sorting and browser virtualization; the browser must not load the entire 200K-device inventory.
- Error states always expose what failed, available evidence, correlation/error ID and next action.

## 3. Dashboard-wide controls and states

### 3.1 Universal controls

All applicable dashboards inherit time range, timezone, compare period, auto-refresh, scope filters, Group By, visualization switching, saved views, export/report, annotations and contextual actions. Filters are composable and reflected in the URL/state so links preserve context.

### 3.2 Required states

Every screen/panel must explicitly design: Loading, Empty, Partial Data, Stale Data, Permission Restricted, Not Licensed, Query Limited, Error, Monitoring Degraded and Normal. A blank panel is never an acceptable error state.

These are presentation outcomes, not one mutually exclusive resource-health enum. Permission/entitlement, query lifecycle, evidence quality, capability readiness and monitored-object lifecycle remain separate dimensions. On an authoritative denial/revocation/expiry response or a session/scope change, the UI hides protected payloads, clears no-longer-authorized cached rows and disables affected actions. The backend reauthorizes every protected request regardless of cached UI state; stale client permission data never authorizes an action. Client detection timing depends on the approved session/expiry/revocation mechanism, which must be bound before acceptance; this rule does not imply instant detection of a server-side revocation. **Bounded revalidation:** while a session remains apparently valid, the UI MAY perform bounded background revalidation of session/entitlement/scope on a governed interval and on privileged-action boundaries; detected expiry, revocation or scope loss MUST force reauthentication/reauthorization before continuing protected work, without inventing a second auth authority or bypassing server checks. A query error may coexist with a clearly dated last-known value only while the approved authorization/cache contract permits its display and no authority-loss trigger has occurred. Maintenance/suppression is an explicit overlay with scope/window/reason; it neither repairs telemetry nor proves the resource healthy.

An empty authorized inventory/filter result means no matching objects. `No Data` means an existing object has no qualifying observation for the selected scope/window. An unbound/unavailable source means the data provider is unavailable; it is not an empty estate. A measured zero is shown as zero only when the source contract supplies valid evidence. `Normal` means query/data presentation completed normally, not `DEVICE_UP`/`HOST_UP`. Unknown or insufficient evidence remains explicit, including `INCONCLUSIVE` RCA and OIV `INSUFFICIENT EVIDENCE`. No missing, stale, partial, suppressed or collector-failed evidence is converted directly to DOWN.

The implementation's `screen-state.ts` and health-overview presentation types already separate several of these cases; the rules above define required composition, not a claim that every screen implements it. Each frozen manifest must give examples for every applicable state and specify which actions remain enabled, why others are disabled, affected source/time and safe retry/cancel behavior.

### 3.3 Accessibility/readability

Support Light/Dark/System theme; Compact/Comfortable/Large density; keyboard navigation; scalable typography; accessible contrast; status icon/text in addition to color; and NOC TV/full-screen layouts. Full admin/studios are desktop-first; mobile/tablet supports dashboards, alerts, incidents, acknowledgement and investigation summaries.

Acceptance is observable: keyboard-only users can reach and operate every enabled control; tabs expose selected state and support arrow/Home/End navigation; drawers/dialogs have a named heading, preserve a logical focus order and return focus to their launching control on close. Modal confirmation retains focus inside its scope while open. Loading and result/error messages are announced without repeated interruption; a static unbound preview is not an assertive alarm. Labels identify units, timezone, required input and validation error without relying on placeholder text or color. Text zoom, supported theme/density and each declared desktop/tablet/mobile/wallboard layout must preserve critical values, errors and authorized actions without overlap or clipped controls; tables may use a labelled bounded scroll region. Each screen's frozen manifest declares supported viewport/layout cases and verification evidence. A responsive foundation alone does not certify all 98 screens.

### 3.4 NOC Operational Troubleshooting Experience

Platform shall optimize NOC workflow around **observe → verify → diagnose → act → preserve evidence**. Routine troubleshooting should not require an operator to leave Platform for external ping, traceroute, SSH terminal, SNMP tester, flow explorer or configuration-comparison tools. This does not remove safety controls; friction increases with action risk.

**Contextual Action Bar.** Applicable entity, interface, alert and incident pages expose a persistent permission-aware action bar. Network-device defaults are **CLI, Ping, Trace/Path, SNMP Test, Flow, Config, Compare, Investigate, Maintenance**; alert/incident contexts additionally expose **Verify Now** where a deterministic pack exists. Entity types adapt actions to their domain.

**Troubleshoot Drawer / Workspace.** Actions open a resizable drawer/workspace while the originating dashboard remains visible. Entity, tenant, time range, filters and incident/investigation context are preserved. The drawer contains applicable tests, recent results and investigation history and supports cancellation of bounded live operations.

**On-demand ICMP.** Ping is one-click for authorized users with safe defaults and exposes source/vantage, target, packet count/size, timeout, transmitted/received, loss, min/avg/max RTT and jitter. `Run Again`, bounded `Continuous Test`, `Compare` and `Add to Investigation/Incident` are supported where authorized. Default source is the assigned eligible Sensor; alternate Sensor/Agent and Multi-Vantage choices appear only when eligible/authorized.

**Multi-Vantage Verification.** Independent results are displayed by source/vantage so operators can distinguish target failure from path/locality/Sensor failure. Contradictory evidence is explicit and may yield Path/Monitoring Degraded or Inconclusive rather than a manufactured definitive result.

**Verify Now.** For supported alerts/incidents, a deterministic bounded diagnostic pack runs technology/failure-class checks and shows each check, result, duration, source/vantage, quality and any skipped/denied check.

**Live Evidence Bar.** Important entity pages show continuously monitored state separately from latest applicable on-demand verification, with source/vantage and age. Fresh on-demand evidence does not silently rewrite historical monitoring observations.

**Why? / What Changed? / Compare.** `Why?` explains evidence/policy/quality inputs behind displayed state. `What Changed?` opens relevant NCM/configuration/routing/topology/maintenance/Platform/external changes without asserting causality solely from time proximity. `Compare` supports previous period, baseline, peer entity/interface/site, pre-incident and pre-change views where data permits.

**Investigation Session.** `Start Investigation` creates an auditable context. Diagnostics, CLI sessions/commands, selected graph/config/flow/log snapshots, notes, RCA/change evidence and comparisons may be attached automatically or explicitly and linked to an Incident. Starting an investigation grants no additional privilege.

**Suggested Next Test / Diagnostic Packs.** The workspace may show deterministic next checks from approved packs such as Device Unreachable, WAN Degradation, BGP/Routing, Wireless Client and Service Reachability. AI may explain/prioritize permitted next checks but must not fabricate results or execute privileged actions directly.

**CLI interaction.** CLI opens inside Platform without losing dashboard context. Launch options may include an authorized stored Credential Profile, a policy-permitted personal credential, or one-time session credentials. One-time credentials are ephemeral. Privileged/configuration modes may require step-up MFA, reason/ticket, approval and stronger command policy.

**Safety and capacity.** Every on-demand action is tenant/resource scoped, audited and bounded by timeout/rate/concurrency/cancellation. Browser clients do not directly open device sockets. Interactive worker budgets cannot starve routine polling.

### 3.4.1 Observability Phase-1 cross-signal UI projection — v3.2.2 reconciliation

> **Reconciliation reference:** Issue #142 (Master Chat comment `5909076167`). Governed by `REQ-INF-013` (bounded Linux Agent eBPF process-to-network telemetry), `REQ-NET-016` (active path hop/loss/latency/fingerprint observability), and decisions `DEC-ARCH-039`, `DEC-ARCH-040`, `DEC-ARCH-041`.

Linux Agent eBPF process-to-network evidence and Sensor active-path telemetry project directly into existing UI/UX workflows without creating an independent or competing troubleshooting surface:

1. **Investigation Session:** Process-network records (process identity, socket counts, bytes, TCP RTT, retransmission bursts, and shed/drop counters) and active-path hop-by-hop records attach as typed evidence items to the ordered investigation manifest. Operators can compare socket behavior and transit path hops alongside logs, metrics, alerts, and NCM configuration changes without leaving the session.
2. **Live Evidence Bar:** On monitored Linux hosts, the Live Evidence Bar displays continuous eBPF process socket health alongside latest verified on-demand snapshots. On network entities and circuit endpoints, it renders current active-path status (`COMPLETE | PARTIAL | TARGET_UNREACHABLE | FAILED`), hop count, and end-to-end RTT/loss. Status `FAILED` indicates that the probe execution or collector itself failed or could not produce valid path evidence (a monitoring/evidence-health condition); Live Evidence makes this distinction visually explicit and must NEVER render or interpret `FAILED` as `TARGET_UNREACHABLE` or as target/device DOWN.
3. **Incident Command Workspace:** Qualifying incidents pin correlated socket surges (e.g. backend connection timeout spikes) and path divergence events (`path.change.detected`) directly to the incident evidence pane. Operators can inspect whether path latency shifted prior to service degradation.
4. **Unified Timeline:** Interleaves process-level connection resets, TCP retransmission spikes, and transit path hop anomalies into the temporal timeline alongside infrastructure alerts, deployments, CI/CD changes, and configuration revisions.
5. **Entity Detail:** Linux host entity views provide a **Process Network & Sockets** tab displaying top communicating processes/services, remote endpoints, active sockets, throughput, and RTT, with explicit indicators for `RESOURCE_GUARDED` or shed states. Network entity and service views provide an **Active Path & Hops** tab showing hop-by-hop latency sparklines, packet loss rates, and historical path fingerprints.
6. **Topology / Path Analysis:** Overlays active-path hop metrics onto logical and physical topology maps. Intermediate unresponsive hops or hidden MPLS hops are rendered with distinct visual status (grey dashed node/edge) and do NOT mark the overall path or target entity as down.
7. **No Competing Workspaces:** AmrudAi Phase 1 does NOT create a separate "eBPF Dashboard" or "Path Explorer" silo. All observability signals converge into the unified contextual troubleshooting drawer, entity details, and incident workspaces.

### 3.5 SNMP operational experience

**SNMP Test.** From an authorized network entity/interface, `SNMP Test` opens in the Troubleshoot workspace and defaults to the assigned eligible Sensor. It shows target, port, Sensor/vantage, SNMP version and Credential Profile **name/reference only**. It must never display community/auth/privacy secrets.

The test separates:

- network/protocol reachability;
- SNMP response;
- authentication;
- authorization;
- version/security compatibility;
- MIB/object support;
- collection quality.

Where permitted it supports bounded GET, WALK and BULKWALK with explicit OID/root selection and safe result limits. Results show raw numeric OID, MIB-resolved name when available, raw typed value, decoded/normalized value, response time, retries, error/status code, quality/provenance and remediation. Identity quick-test includes applicable `sysObjectID`, `sysName`, `sysDescr`, `sysUpTime` and engine identity evidence.

Stable customer outcomes distinguish timeout/path, unknown v3 user, auth mismatch, privacy/decryption mismatch, unsupported security, context mismatch, authorization/no-access, noSuchObject/noSuchInstance, tooBig and malformed response rather than flattening all failures to "SNMP failed."

**SNMP Monitoring Quality.** Device/profile views expose poll success rate, duration, attempted/successful OIDs, retry/timeout/auth/authz/tooBig/noSuch counts, freshness/staleness, expected-vs-received samples, last success and assigned Sensor. `DEVICE HEALTHY + SNMP MONITORING DEGRADED` is a valid display state.

**Counter explanation.** Interface metric detail can show raw counter samples, actual sample interval, reset/wrap/discontinuity quality and derived rate/utilization explanation for authorized troubleshooting. Invalid/ambiguous counter intervals show `UNKNOWN/DEGRADED` rather than a fabricated spike.

**Interface identity.** When protocol ifIndex changes but Platform reconciles the same canonical interface, UI history remains continuous and may expose the old/new protocol index in provenance/audit rather than presenting a duplicate interface.

**MIB Studio.** The Studio provides library/upload, dependency/import status, compile errors with module/location where possible, OID tree/search, textual conventions, enums, tables/indexes, notifications, unresolved numeric OIDs, authorized sample-device GET/WALK/BULKWALK, raw-versus-decoded preview, semantic mapping, provenance/licensing/redistribution metadata and controlled creation/update of the applicable Capability/Device Pack. Uploading a MIB does not automatically activate polling. Specifying MIB Studio behavior does not claim the Studio runtime is already implemented/certified.

**Polling policy.** Monitoring Profile/Adaptive Polling UI exposes normal/adaptive interval, cooldown, maximum duration, target scope, retry/bulk policy where configurable, projected Sensor impact and effective inherited policy. The normal SNMP operational-metric default is **5 minutes**; the UI must make clear that this does not require every available OID every five minutes and that availability/liveness remains a separate plane. Unsafe settings are warned or blocked by capacity/policy.

**SNMP SET.** Routine SNMP Test/monitoring UI is read-only. Any supported SET action appears only in an explicitly privileged NCM/remediation workflow with authorization, reason/ticket/approval where required, pre/post value evidence and audit.

### 3.6 Shared read, action and context contract

Every frozen view declares its authoritative query/read-model owner. Current configuration, authorization, workflow and other operational state use their owning contracts; eligible historical telemetry uses the selected ClickHouse raw/rollup resolution. A screen may combine these through governed APIs without turning ClickHouse, Valkey or browser state into a new authority. Query contracts specify supported filters/sorts, deterministic ordering, time-window semantics, units/precision/nullability, default and maximum page/row/scan/time bounds, freshness/provenance and explicit truncation. Server pagination and rendered-row virtualization solve different limits; neither authorizes downloading the entire estate into the browser. No data-source assumption follows from a screen's title.

Every enabled action declares target identity/revision, current server permission and entitlement, allowed parameters, validation, risk/reason/confirmation or approval where required, idempotency/conflict behavior, asynchronous status and cancellation/recovery evidence. A request accepted into a job is not completed work. Cancel requested, cancelled, partially applied, validation failed and rollback unsupported must not be rendered as success. Unsupported actions are absent or explicitly unavailable with an actionable reason; a generic “Apply” label never grants an unspecified mutation.

Drill-downs preserve authorized tenant/site, selected object, time range, display timezone, filters and incident/investigation context when applicable. A detail destination lacking a required object shows a bounded selector or a clear context-required state; it must not guess an entity. Back/close returns to the prior list/tab/selection/focus. Context is revalidated on navigation and shared-link open. Credentials, tokens and sensitive evidence are never placed in URLs. The source route path remains stable; each manifest freezes the allowed context serialization and query/action references before implementation.

These rules are inherited by §5. Domain-specific columns and actions are additional minimum behavior, not an exhaustive substitute for the frozen per-view contract. Missing essential field/query/action semantics remain a documentation/design HOLD. A fully specified contract awaiting source integration is an implementation HOLD, a separate condition.


## 4. Dashboard Studio

Dashboard library scopes: My, Team, Shared, Tenant/Customer, System and folders/groups. Actions: create, clone, move, rename, share, import/export, permissions, delete, variables and reusable panels. Editor supports drag/resize/duplicate/copy panels, undo/redo and preview. Panel editor tabs: Data, Query, Transformation, Visualization, Thresholds, Links, Overrides, Permissions. Panel types: stat, gauge, time series, bar, pie, table, heatmap, histogram, status grid, Sankey, topology, geo, text, alert list, incident list, RCA and domain-specific visualizations.

## 4.1 OOB workspace classification and manifest contract — v3.2.2 reconciliation

The 98 mandatory UI entries are **not all grid dashboards**. Before implementation or GA classification, every UI-001..UI-098 definition shall carry an explicit workspace class selected from the smallest useful vocabulary, such as:

```text
EXECUTIVE_DASHBOARD | NOC_DASHBOARD | WALLBOARD | ANALYTICAL_DASHBOARD |
OPERATIONAL_CONSOLE | ENTITY_DETAIL | INVENTORY_FLEET | TOPOLOGY_MAP |
INVESTIGATION | CONFIGURATION | GOVERNANCE | PLATFORM_OPERATIONS |
REPORT_ANALYSIS
```

The class controls the default interaction model. A console may be queue/table-first; an entity workspace may be detail/tab-first; an investigation may be evidence/timeline/graph-first. Platform shall not force decorative chart grids onto workspaces where they reduce operational usability.

Each shipped System workspace requires a versioned **OOB manifest**. For every applicable tab/view the manifest records at least: stable workspace/view/panel identifiers, purpose, default order/layout, visualization/widget type, governed query/API reference, authoritative data-source/domain reference, entity/scope binding, aggregation/dimensions/unit, time/comparison behavior, visualization settings reference, presentation-threshold/value-mapping reference, null/no-data behavior, drill-down/action references, freshness/provenance expectations, permission/entitlement references, query bounds, responsive/wallboard behavior and customer editability.

A candidate default panel that cannot be mapped to an authoritative requirement + governed query/API + data source + permission/entitlement + test evidence is **not production-ready**. It is removed from the shipped default or explicitly retained as deferred design metadata; it must not be implemented with fake metrics, placeholder operational truth or an invented backend contract.

The 98 broad workspace specifications in §5 remain authoritative product scope. This reconciliation does **not** require all 98 panel manifests to be fabricated immediately. Manifests are progressively frozen only when their underlying metric/query/data contracts are authoritative.

### 4.2 Declarative System definitions and recoverability

System OOB definitions shall be schema-backed, versioned and CI-validated rather than being authoritative only as hard-coded React composition. The current presentation-schema family is the starting contract; future manifest/schema evolution remains versioned and backward-compatible according to normal Platform contract rules.

System definitions are immutable product versions. Upgrade may introduce a newer System version, but shall not silently overwrite a customer clone or customer-owned dashboard. Authorized users can restore/reopen the current System definition and clone a newer System version. Team/Tenant/Shared mutable dashboards require revision history sufficient for safe rollback of accidental edits. A mandatory maker-checker workflow is **not** implied merely by this requirement.

### 4.3 Visualization styling and threshold separation

Dashboard Studio shall expose only visualization controls that are meaningful for the selected panel type. The governed styling vocabulary may include series/palette, theme-aware color, line/fill/opacity/marker, axes and linear/log scale, unit/precision/min/max, legend/tooltip/stacking, display name, null display, value/status mapping, threshold colors, conditional/table formatting, text/title/subtitle/alignment, field overrides and links/drill-downs.

Accessibility remains mandatory: status must not depend on color alone, contrast must remain usable in supported themes, and chart/table alternatives or summaries are provided where practical.

**Visualization thresholds are presentation metadata only.** They do not create, modify or override Alert/Validation Policy behavior. Alert-policy thresholds are operational control state and may influence a visualization only through an explicit governed link/reference. Changing chart colors never silently changes alerting, and changing an alert rule never silently rewrites customer styling.

### 4.4 Team-scope guardrail pending Identity/Access reconciliation

Existing `TEAM` dashboard scope, notification subscriptions and owner/team references do **not** by themselves define a first-class human Team authorization model. Until the Identity/Access reconciliation freezes that model:

- a directory/AD/LDAP/Entra/SCIM group is not automatically a Platform Team;
- a Team label/reference never grants resource access;
- server-side tenant/resource authorization remains authoritative;
- Team dashboards/work queues must not leak resources merely because a definition references them.

The proposed first-class Team object, effective-access inspector, directory mapping, JIT grants and work-routing model are valid enterprise concerns but are **deferred to the Identity/Access specialized reconciliation** so Platform does not create a competing IAM model from the UI layer.

### 4.5 Formal UI requirement supplement

| REQ ID | Requirement |
|---|---|
| `REQ-UI-026` | Every mandatory OOB UI workspace has an explicit workspace class and progressively frozen versioned System manifest appropriate to its interaction model; not every UI entry is forced into a dashboard grid. |
| `REQ-UI-027` | Every shipped OOB panel/widget maps to authoritative requirement, governed query/API, data source, permission/entitlement, state/no-data semantics, query bounds, drill-down/action and test evidence; unsupported/fake operational panels are prohibited. |
| `REQ-UI-028` | Dashboard visualization styling is governed, accessible and panel-type appropriate; presentation thresholds/value mappings remain semantically separate from Alert/Validation Policy thresholds. |
| `REQ-UI-029` | System dashboards/workspaces are versioned and recoverable across upgrades; customer clones are not silently overwritten; mutable Team/Tenant/Shared definitions retain rollback-capable revision history without implying mandatory maker-checker. |
| `REQ-UI-030` | Platform uses a common workspace-manifest envelope for UI-001..UI-098 with a mandatory workspace class and typed class-specific definition reference; `DASHBOARD_DEFINITION` remains authoritative only for dashboard-like classes and is not stretched into a giant generic schema. |
| `REQ-UI-031` | Dashboard/Report Studio references resolve through governed Presentation Query & Metric Catalogue and Presentation Capability Registry contracts; React must not invent query semantics, visualization settings, field overrides, value/status mappings or presentation thresholds. |
| `REQ-UI-032` | `TEAM` presentation scope is reserved but not implementation-ready until an authoritative tenant-scoped Team identity/binding exists; a Team reference never grants resource authorization. |

### 4.6 Workspace-manifest architecture

The canonical presentation architecture is a **common workspace-manifest envelope + class-specific definition contracts**.

The common envelope owns:

- UI/workspace identity and version;
- workspace class;
- display name/domain/persona purpose;
- navigation/context entry metadata;
- tenant/resource/entitlement visibility references;
- state/freshness/provenance behavior;
- responsive/wallboard capability metadata;
- System/customer ownership and recoverability metadata;
- a typed `contentDefinitionRef`.

Class-specific definition contracts own the interaction-specific body. Dashboard-like classes continue to use the existing `dashboard-definition.schema.json`. Operational Console, Entity Detail, Investigation, Configuration, Governance, Topology/Map, Platform Operations and other non-dashboard classes will receive the smallest typed contract required by their actual interaction model.

Do **not** create one giant schema with hundreds of optional properties and do not coerce non-dashboard workspaces into `DASHBOARD_DEFINITION` merely to avoid defining the correct class contract.

The existing React `workspace-registry.ts` remains implementation metadata, not the canonical persisted manifest authority. The schema-backed manifest family becomes authoritative as each class is implemented.

### 4.7 TEAM scope readiness boundary

Current dashboard/report schemas admit `scope=TEAM` but do not identify an authoritative tenant-scoped Team object. Therefore:

- `TEAM` is currently a **reserved contract value**, not proof that Team ownership is production-ready;
- no Team-scoped dashboard/report may become production-authoritative until the Identity/Access tranche defines stable `team_id`, tenant ownership, lifecycle, membership semantics, server-side authorization and revocation behavior;
- future Team-owned definitions require an explicit Team binding, not inference from role name, directory group name, folder name or browser state;
- current server authorization/resource scope checks remain authoritative regardless of presentation scope.

Named follow-up architecture tranche:

**Identity, Teams, RBAC, Resource Scope & NOC Work Ownership Reconciliation**

It owns Team identity/lifecycle, directory-group mapping, effective access explanation, role/resource-scope binding, revocation/session-cache behavior, Team-owned dashboards/reports, My Work/Team Work assignment and NOC work ownership/routing. It must reconcile `TEAM` presentation semantics before Team-owned artifacts or queues are implementation-ready.

### 4.8 Presentation Query & Metric Catalogue and Presentation Capability Registry

Current `queryRef`, `dataSourceClassRef`, `visualizationSettingsRef`, `thresholdPolicyRef` and `fieldOverrideRefs` are correct indirections, but current source does not contain authoritative registries behind all of them.

Architecture requires two governed logical catalogues, preferably schema-backed/static plus existing Core/Query-service APIs before any new service/database is considered:

**Presentation Query & Metric Catalogue (QMC)**

Minimum discoverable metadata:

- stable query/metric identity and version;
- data domain and supported entity types;
- typed parameters and dimensions;
- allowed aggregations/grouping/top-N/sort;
- unit/type and semantic counter/gauge/state behavior;
- available rollup/resolution and time semantics;
- freshness/quality/provenance fields;
- permission/resource-scope and entitlement references;
- row/series/cardinality/time-range/scan bounds;
- supported transformations/formulas where governed;
- compatibility/deprecation metadata.

Dashboard/Report Studio uses this catalogue to build bounded no-code/low-code queries. Ordinary users are not required to know SQL/ClickHouse syntax, and arbitrary browser-defined SQL/JavaScript/eval is not part of the base architecture.

**Presentation Capability Registry (VIS)**

Minimum metadata:

- panel/visualization type;
- allowed settings by panel type;
- axis/unit/precision/legend/tooltip/stacking/palette capabilities;
- field override capabilities;
- value/status mapping contract;
- presentation-threshold contract;
- accessibility requirements;
- responsive/wallboard capability;
- supported drill-down/action classes.

These are logical registries, not authorization systems and not justification for a new microservice.

#### Presentation Foundation — Workspace Manifest, Query/Metric & Visualization Contract Reconciliation

This is the next **presentation-specific** implementation/reconciliation tranche. It does not displace higher-priority P0 security/enrollment work in the overall Platform sequence, but Dashboard Runtime/Studio/Report work must not invent presentation contracts ahead of it.

The tranche freezes only the smallest contract family that current interaction semantics require:

- existing `DASHBOARD_DEFINITION` remains the composition contract for `EXECUTIVE_DASHBOARD`, `NOC_DASHBOARD`, `ANALYTICAL_DASHBOARD` and `WALLBOARD`; wallboard differences are capability/presentation constraints rather than a second dashboard schema;
- `REPORT_ANALYSIS` may reuse dashboard composition when it is an interactive analytical workspace, while scheduled/static report artifacts continue to use the separate `REPORT_DEFINITION`;
- one bounded **collection/operations workspace** contract may serve `INVENTORY_FLEET`, `OPERATIONAL_CONSOLE`, `GOVERNANCE` and `PLATFORM_OPERATIONS` only where they share table/list/queue/summary/action semantics; the contract must use typed view/body kinds and must not become an all-optional property bag;
- `ENTITY_DETAIL`, `INVESTIGATION`, `TOPOLOGY_MAP` and `CONFIGURATION` retain distinct contracts because their identity/context, evidence, graph/map and mutation semantics materially differ.

The tranche owns WME schema/versioning/identity/context/visibility/freshness/content-definition typing and System/customer ownership; QMC metadata and compatibility; VIS capability/settings/overrides/mappings/presentation-threshold/accessibility/drill-down rules; and reference-integrity validation.

| Consumer | Presentation Foundation blocker |
|---|---|
| Dashboard Runtime | WME + dashboard content mapping + QMC resolution + VIS capability/reference validation + fail-safe authorization/entitlement resolution |
| Dashboard Studio | Dashboard Runtime foundation plus typed authoring metadata, variables/transformations and reference-integrity validation |
| Report Studio | QMC/VIS reusable primitives + `REPORT_DEFINITION` reconciliation + execution/display-timezone semantics + server authorization/reference validation; WME is not required merely to persist a report artifact |
| R1 OOB manifests | WME + applicable class contract + QMC/VIS + authoritative DOMAIN references |
| Remaining OOB manifests | R1 architecture proof first, then domain-by-domain manifests only when their DOMAIN/query contracts are authoritative |

### 4.9 Dashboard enhancement proposal dispositions

| Proposal | Decision | Architecture disposition |
|---|---|---|
| No-code query builder | ACCEPT WITH MODIFICATION | Uses QMC typed metadata and bounded server query contracts; no arbitrary SQL/JS by default. |
| Cascading variables | ACCEPT | Typed, permission-filtered, bounded variable sources; URL/saved-view compatible. |
| Synchronized crosshair/shared temporal interaction | ACCEPT WITH MODIFICATION | Shared runtime interaction bus for compatible panels only; no new service. |
| Click-to-filter | ACCEPT | Context-preserving and permission-safe. |
| Shared zoom | ACCEPT WITH MODIFICATION | Applies only to compatible temporal panels and updates governed time context. |
| Pause/freeze | ACCEPT | Freezes presentation/query refresh only; never changes monitoring cadence. |
| Annotation/event/change overlays | ACCEPT WITH MODIFICATION | RBAC/tenant scoped, bounded, provenance-bearing and time-window limited. |
| Inspect Panel / provenance | ACCEPT | Shows governed query/data source/scope/freshness/resolution/limitations without leaking secrets or sensitive raw SQL. |
| Wallboard playlist | DEFER | Useful after base wallboard runtime is stable; not a prerequisite for R1 architecture proof. |
| First-class table behavior | ACCEPT | Server pagination/filter/sort, virtualization, columns, grouping, row actions, saved layout and freshness. |
| Topology/map panel behavior | ACCEPT | Provenance-aware node/edge/map semantics; inferred relationships remain distinguishable. |
| Import/export security | ACCEPT | Version/reference/tenant/permission/query-safety/content/size validation; no executable arbitrary code. |
| Dashboard → Report | ACCEPT WITH MODIFICATION | Reuse governed query/presentation primitives; Report runtime remains separate from browser rendering. |
| Dashboard → Alert Policy | ACCEPT WITH MODIFICATION | Panel context may seed governed policy creation; visualization threshold never becomes a hidden alert rule. |
| Dashboard → Investigation | ACCEPT | Context/time/evidence references seed/open Investigation Session without duplicating authoritative data unnecessarily. |
| Panel query-cost guardrails | ACCEPT | Time/rows/series/cardinality/scan/concurrency/timeout/cancellation/downsampling/Query Limited state. |
| Representative dashboard certification set | ACCEPT | R1 proof set below must pass before freezing the rest of the 98 manifests. |

### 4.10 Representative cross-class architecture proof set

Before all 98 panel manifests are frozen, the common envelope, class-specific definitions, QMC/VIS contracts and runtime architecture must be proven against this deliberately diverse R1 set:

```text
UI-001 Executive / CIO Overview
UI-003 NOC Command Center
UI-009 Network Health Overview
UI-010 Network Device Detail
UI-017 Network Flow / Traffic Analytics
UI-029 Infrastructure Health Overview
UI-042 Database Estate Overview
UI-048 Kubernetes Overview
UI-055 Alert Console
UI-057 Incident Detail
UI-058 RCA / Investigation Workspace
UI-066 IPAM Overview
UI-073 Sensor Estate Overview
UI-076 Telemetry Quality
UI-083 Audit Explorer
UI-088 Shift Handover
UI-092 Data Pipeline Integrity
```

R1 proves schema/runtime fit across dashboard, wallboard, entity detail, analytics, console, investigation, governance, fleet and Platform-operations classes. Only after R1 is coherent are the remaining manifests frozen domain-by-domain.

The presentation-foundation proof is intentionally broader than dashboards. It also includes two schema-level representative artifacts; these are architecture/test fixtures, not new UI IDs or runtime implementation in this reconciliation:

- **REPORT_DEFINITION — SLA Scheduled Report**: proves governed QMC query bindings, filters/variables/calculations, table/visualization reuse, static rendering, explicit execution timezone distinct from display/render timezone, allowed PDF/CSV report artifacts only; API access remains a separate interface, server authorization and scheduled execution independent of browser runtime.
- **SAVED_VIEW — representative dashboard/workspace view**: proves target workspace/dashboard reference, filters, grouping/sort, time-context policy, URL/context compatibility, server authorization re-evaluation and fail-safe behavior for stale, deleted or unauthorized target/reference state.

### 4.11 UI-001..UI-098 authoritative classification and coverage matrix

Status legend:

- **Manifest Status = `SPEC_CONTRACT=FROZEN`** — System Workspace Manifest for the UI ID is frozen in §16 (panels, domain/query bindings, actions, RBAC, states, pagination, drill-down, a11y, phase). Specification completeness is independent of runtime code.
- **Query/API Authority = `IMPLEMENTATION=PENDING`** — domain/query/operation contracts are named in §16 and `Platform_17` §15; runtime wiring remains pending. Do not invent endpoints. Do not treat `IMPLEMENTATION=PENDING` as an incomplete specification.
- **WME** — common Workspace Manifest Envelope / applicable class-specific definition contract.
- **QMC** — Presentation Query & Metric Catalogue.
- **VIS** — Presentation Capability Registry.
- **DOMAIN** — underlying authoritative domain APIs/data/authorization needed by that workspace.

Semantic review of each §5 Purpose/Tabs/Actions found eight classifications where the existing class would drive the wrong interaction contract. The authoritative corrections are: UI-035/UI-036/UI-037/UI-038 → `INVENTORY_FLEET`; UI-062 → `OPERATIONAL_CONSOLE`; UI-065 → `INVESTIGATION`; UI-090/UI-091 → `ANALYTICAL_DASHBOARD`. The other 90 classifications PASS this semantic review and are not changed merely for naming/aesthetic preference.

| UI ID | Workspace | Class | Manifest Status | Query/API Authority Status | Blocking Contract | Planned Phase |
|---|---|---|---|---|---|---|
| UI-001 | Executive / CIO Overview | `EXECUTIVE_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-002 | Global Operations Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-003 | NOC Command Center | `WALLBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-004 | Geographic Operations Map | `TOPOLOGY_MAP` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-005 | Business Service Health | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-006 | Site Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-007 | Branch Experience | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-008 | Branch Comparison | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-009 | Network Health Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-010 | Network Device Detail | `ENTITY_DETAIL` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-011 | Interface Detail | `ENTITY_DETAIL` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-012 | Critical Link | `ENTITY_DETAIL` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-013 | Link Monitor | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-014 | Availability | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-015 | Network Performance | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-016 | Network Capacity | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-017 | Network Flow / Traffic Analytics | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-018 | Site-to-Site Traffic | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-019 | Routing Health | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-020 | BGP | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-021 | MPLS / VRF | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-022 | VXLAN / EVPN | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-023 | SD-WAN Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-024 | VPN | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-025 | WLAN Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-026 | VoIP / CUCM | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-027 | IP SLA / Network Assurance | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-028 | QoS / CBQoS | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-029 | Infrastructure Health Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-030 | Server Fleet | `INVENTORY_FLEET` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-031 | Server Detail | `ENTITY_DETAIL` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-032 | Process & Service | `ENTITY_DETAIL` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-033 | Hardware / Out-of-Band | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-034 | Virtualization Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-035 | VMware | `INVENTORY_FLEET` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-036 | Nutanix | `INVENTORY_FLEET` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-037 | Hyper-V | `INVENTORY_FLEET` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-038 | Proxmox | `INVENTORY_FLEET` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-039 | Storage Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-040 | SAN Fabric | `TOPOLOGY_MAP` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-041 | Data Center Environmental | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-042 | Database Estate Overview | `INVENTORY_FLEET` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-043 | Database Detail | `ENTITY_DETAIL` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-044 | Multi-Cloud Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-045 | AWS Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-046 | Azure Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-047 | GCP Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-048 | Kubernetes Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-049 | Container Detail | `ENTITY_DETAIL` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-050 | Synthetic Monitoring Overview | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-051 | Synthetic Test Detail | `ENTITY_DETAIL` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-052 | Certificate | `INVENTORY_FLEET` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-053 | DNS / DHCP / NTP | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-054 | AD / Identity Infrastructure | `NOC_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-055 | Alert Console | `OPERATIONAL_CONSOLE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-056 | Incident Console | `OPERATIONAL_CONSOLE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-057 | Incident Detail | `INVESTIGATION` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-058 | RCA / Investigation Workspace | `INVESTIGATION` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-059 | False Alert Qualification | `GOVERNANCE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-060 | Maintenance & Suppression | `CONFIGURATION` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-061 | Change Timeline | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-062 | NCM Overview | `OPERATIONAL_CONSOLE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-063 | Device Configuration | `CONFIGURATION` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-064 | NCM Compliance | `GOVERNANCE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-065 | Configuration Change | `INVESTIGATION` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-066 | IPAM Overview | `INVENTORY_FLEET` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-067 | Subnet Detail | `ENTITY_DETAIL` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-068 | IP Conflict | `OPERATIONAL_CONSOLE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-069 | IPAM Capacity Forecast | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-070 | Capacity Planning Overview | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-071 | SLA | `REPORT_ANALYSIS` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-072 | ISP / Carrier SLA | `REPORT_ANALYSIS` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-073 | Sensor Estate Overview | `INVENTORY_FLEET` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-074 | Sensor Detail | `ENTITY_DETAIL` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-075 | Sensor Pool / HA | `PLATFORM_OPERATIONS` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-076 | Telemetry Quality | `PLATFORM_OPERATIONS` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-077 | Agent Estate | `INVENTORY_FLEET` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-078 | Kafka / Data Pipeline Health | `PLATFORM_OPERATIONS` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-079 | Core Database / Storage Health | `PLATFORM_OPERATIONS` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-080 | Integration Health | `PLATFORM_OPERATIONS` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-081 | ITSM Lifecycle | `OPERATIONAL_CONSOLE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-082 | Audit Overview | `GOVERNANCE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-083 | Audit Explorer | `GOVERNANCE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-084 | Access Governance | `GOVERNANCE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-085 | CLI Session History | `GOVERNANCE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-086 | Technology Lifecycle Risk | `GOVERNANCE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-087 | Desired vs Actual / Configuration Drift | `GOVERNANCE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-088 | Shift Handover | `OPERATIONAL_CONSOLE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-089 | Migration Studio | `CONFIGURATION` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-090 | Intelligence Quality | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-091 | Alert Policy Effectiveness | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-092 | Data Pipeline Integrity | `PLATFORM_OPERATIONS` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R1 REPRESENTATIVE PROOF |
| UI-093 | Storage & Retention | `PLATFORM_OPERATIONS` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-094 | Usage & Licensing | `GOVERNANCE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-095 | Housekeeping & Lifecycle Jobs | `PLATFORM_OPERATIONS` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-096 | Post-Incident Review | `INVESTIGATION` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-097 | Runbook Library | `GOVERNANCE` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |
| UI-098 | Device / Time Comparison | `ANALYTICAL_DASHBOARD` | SPEC_CONTRACT=FROZEN | IMPLEMENTATION=PENDING | WME + QMC + VIS + DOMAIN | R2 DOMAIN MANIFEST AFTER R1 |

### 4.12 Source binding checkpoint and manifest closure

**Checkpoint:** repository `gagandsm-max/platform`, main `36e1513f25265d0bcd45bdfe5156ee02fdbf5238`, reviewed 2026-11-06. This records the source baseline, not a live deployment or certification claim. `ui/src/platform/workspace-registry.ts` contains exactly 98 `UI-001..UI-098` entries, all `IMPLEMENTATION=PENDING`; preserve each existing `uiCode` → `path` association. `ui/src/platform/primary-navigation.ts` separately contains fourteen sidebar categories, six Studio children and `/provisioning` under Inventory. Category paths are not aliases for similarly named numbered screens; `/provisioning` is not UI-089 Migration Studio.

`ui/src/App.tsx` uses identity-free `FOUNDATION_ACCESS` for presentation only. `WorkspacePage.tsx` does not bind a live data-state provider. UI-009/UI-029 have dedicated unbound health view models; the other routes use the generic host. Their accessible shell/state behavior does not prove authenticated interaction, live telemetry, aggregate health, action APIs or completed manifests. Do not replace pending capability with fabricated metrics or green health.

The R1 representative source routes are:

| UI ID | Existing route | Binding status at checkpoint |
|---|---|---|
| UI-001 | `/executive-cio-overview` | Presentation only; value/aggregate query contract pending |
| UI-003 | `/noc-command-center` | Presentation only; wallboard queries pending |
| UI-009 | `/network-health-overview` | Dedicated unbound presentation; aggregate health via shared Domain Query Catalogue (`Platform_17` §15); `IMPLEMENTATION=PENDING` |
| UI-010 | `/network-device-detail` | Presentation only; entity context and governed detail bindings pending |
| UI-017 | `/network-flow-traffic-analytics` | Presentation only; bounded Flow query contract pending |
| UI-029 | `/infrastructure-health-overview` | Dedicated unbound presentation; aggregate health via shared Domain Query Catalogue (`Platform_17` §15); `IMPLEMENTATION=PENDING` |
| UI-042 | `/database-estate-overview` | Presentation only; estate query contract pending |
| UI-048 | `/kubernetes-overview` | Presentation only; estate/query binding pending |
| UI-055 | `/alert-console` | Presentation only; qualified-alert read/action bindings pending |
| UI-057 | `/incident-detail` | Presentation only; incident context and read/action bindings pending |
| UI-058 | `/rca-investigation-workspace` | Presentation only; versioned evidence/read/action bindings pending |
| UI-066 | `/ipam-overview` | Presentation only; IPAM read/action bindings pending |
| UI-073 | `/sensor-estate-overview` | Presentation only; estate/control read/action bindings pending |
| UI-076 | `/telemetry-quality` | Presentation only; quality query binding pending |
| UI-083 | `/audit-explorer` | Presentation only; governed audit search/export binding pending |
| UI-088 | `/shift-handover` | Presentation only; handover state/read/action bindings pending |
| UI-092 | `/data-pipeline-integrity` | Presentation only; interval/stream reconciliation query binding pending |

The complete existing route inventory remains the pinned source registry; the table does not allocate new IDs or waive the remaining 81 manifests. R1/R2 are design-proof sequencing labels, not release phases. The two R1 schema fixtures remain subject to their existing contracts and are not additional numbered screens.

Verified backend foundations are distinct from UI integration:

| Existing source contract | Implemented boundary | What is not established by it |
|---|---|---|
| `core/.../api/EntityController.java` at `/api/v1/tenants/{tenantId}/entities` | Entity CRUD/identifier/group/tag operations and bounded list reads | Aggregate estate-health, all declared filters/sorts, monitor creation or a connected screen |
| `core/.../api/DiscoveryController.java` at `/api/v1/tenants/{tenantId}/discovery` | Create/read/list/cancel a Discovery request; read candidates/evidence | Recurring schedule CRUD/dispatch and candidate Approve/Reject/Exclude mutation contracts |
| `core/.../api/EntityMetricObservationController.java` at `/api/v1/tenants/{tenantId}/entities/{entityId}/metric-observations` | Bounded per-entity typed observations with quality, freshness, provenance and truncation | Estate aggregate health, derived rate/availability or integrated UI |
| `core/.../domain/discovery/scheduling/DiscoveryOccurrenceCalculator.java` | Pure calendar occurrence calculation | Persisted schedules, execution ownership, dispatch or recurrence UI |

The abbreviated Java prefix above is `core/src/main/java/com/platform/core/`. Supported endpoint claims must be checked against the exact controller/DTO and current security route mapping; this checkpoint is not a new API specification. Existing paged domain APIs use `PageResult` (default 50, maximum 200); do not extend those numbers to unrelated query contracts without their owner.

**Manifest closure record:** for each existing UI ID/view, bind the governing requirement(s), class-specific definition, registered route/context, domain-specific fields/units/states, QMC/query reference, action/permission/entitlement contract, layout/responsive behavior and test/evidence references. Record two independent statuses: **`SPEC_CONTRACT=FROZEN` or documentation HOLD**, and **`IMPLEMENTATION`/`CERTIFICATION` state**. An essential query/action/field/state choice still unselected cannot be passed merely by labelling runtime pending. Conversely, an adequate normative contract is not defective solely because implementation has not started. The owner is the existing domain BUILD plus BUILD-110..118 UI work, as sequenced in the Master Build Register; no duplicate registry service or new master is created here. §4.11 matrix and §16 System Workspace Manifests freeze UI-001..UI-098 specification bindings (`SPEC_CONTRACT=FROZEN`; `IMPLEMENTATION=PENDING`). Visual/interaction certification evidence remains a separate gate.


## 5. Dashboard/workspace screen specifications

The 98 out-of-box screens below are mandatory. Customers may clone/customize them; System versions remain recoverable as product defaults. Entity-detail pages may be contextual and need not be separate left-navigation items.

### UI-001 — Executive / CIO Overview

**Domain:** Executive/Operations
**Primary roles:** CIO/CTO/management
**Purpose:** Overall technology and business-service health, SLA, major incidents, capacity risk, MTTA/MTTR, alert reduction and platform health.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Business Services
- Availability
- Incidents
- Capacity
- SLA
- Trends

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Technology domain
- Business Service
- Incident severity
- SLA state

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-002 — Global Operations Overview

**Domain:** Executive/Operations
**Primary roles:** NOC/IT operations
**Purpose:** Cross-domain operational health, active incidents/alerts, sites, monitoring degradation, recent changes and ITSM state.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Network
- Infrastructure
- Cloud
- Services
- Incidents
- Collectors

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Technology domain
- Business Service
- Incident severity
- SLA state

**Primary table/list columns where applicable:**
- Scope
- Health
- Availability/SLA
- Critical incidents
- Capacity risk
- Trend
- Owner

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-003 — NOC Command Center

**Domain:** Executive/Operations
**Primary roles:** NOC wallboard
**Purpose:** Large-screen critical incidents, affected sites/services, WAN/DC health, changes, Sensor health and ticket queue; TV mode.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Wallboard/Full Screen

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Technology domain
- Business Service
- Incident severity
- SLA state

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-004 — Geographic Operations Map

**Domain:** Executive/Operations
**Primary roles:** Global/regional ops
**Purpose:** Country/city/site/DC/cloud-region health with drill-down.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Map
- Sites
- Incidents

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Technology domain
- Business Service
- Incident severity
- SLA state

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-005 — Business Service Health

**Domain:** Executive/Operations
**Primary roles:** Service owners/NOC
**Purpose:** Service health, dependencies, incidents, SLA, changes and synthetics.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Health
- Dependencies
- Topology
- Incidents
- SLA
- Changes

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Technology domain
- Business Service
- Incident severity
- SLA state

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-006 — Site Overview

**Domain:** Site/Branch
**Primary roles:** Regional/site ops
**Purpose:** Network, infrastructure, WAN, wireless, traffic, services, alerts and SLA for one site.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Network
- Infrastructure
- WAN
- Wireless
- Traffic
- Services
- Alerts
- Topology
- SLA

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Branch/Site
- ISP/Carrier
- Sensor/Probe
- WAN/Internet/SD-WAN
- Status

**Primary table/list columns where applicable:**
- Site/Branch
- Region
- Network health
- Infra health
- WAN/Internet
- Latency/Loss
- Sensor/Agent
- Incidents
- SLA

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-007 — Branch Experience

**Domain:** Site/Branch
**Primary roles:** Branch/NOC
**Purpose:** Branch→DC and DC→branch latency/loss/jitter/path, DNS/HTTP/VPN/SD-WAN and Agent health.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Experience
- Paths
- Services
- Network
- Agent
- History

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Branch/Site
- ISP/Carrier
- Sensor/Probe
- WAN/Internet/SD-WAN
- Status

**Primary table/list columns where applicable:**
- Site/Branch
- Region
- Network health
- Infra health
- WAN/Internet
- Latency/Loss
- Sensor/Agent
- Incidents
- SLA

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-008 — Branch Comparison

**Domain:** Site/Branch
**Primary roles:** NOC
**Purpose:** Fleet heatmap/table comparing branch availability, WAN, Internet, latency/loss/jitter, SD-WAN, Agent, alerts and SLA.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Comparison
- Heatmap
- SLA

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Branch/Site
- ISP/Carrier
- Sensor/Probe
- WAN/Internet/SD-WAN
- Status

**Primary table/list columns where applicable:**
- Site/Branch
- Region
- Network health
- Infra health
- WAN/Internet
- Latency/Loss
- Sensor/Agent
- Incidents
- SLA

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-009 — Network Health Overview

**Domain:** Network
**Primary roles:** Network NOC
**Purpose:** Network device, interface, link, routing, traffic, availability, error and config-change health.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Devices
- Interfaces
- Links
- Routing
- Traffic
- Availability
- Errors
- Changes

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Persistent Action Bar: CLI; Ping; Trace/Path; SNMP Test; Flow; Config; Compare; Investigate; Maintenance
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Start/continue Investigation Session
- On-demand Ping with assigned/alternate/multi-vantage source where eligible
- Traceroute/Path Analysis and healthy-path comparison
- SNMP/monitoring protocol verification
- Verify Now diagnostic pack where applicable
- Why? / What Changed? / Compare
- View Live Evidence
- Create maintenance
- View topology
- View alerts/incidents
- Open embedded CLI (RBAC; stored/personal/one-time credential modes per policy)
- View/backup configuration (RBAC)
- Run monitoring test/poll now (RBAC)

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-010 — Network Device Detail

**Domain:** Network
**Primary roles:** Network engineer
**Purpose:** Universal device page with identity, CPU/memory/uptime/hardware, interfaces, routing, traffic, topology, NCM, events, RCA and audit.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Interfaces
- Routing
- Traffic
- Hardware
- Topology
- Configuration
- Events
- RCA
- Monitoring
- Audit

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Persistent Action Bar: CLI; Ping; Trace/Path; SNMP Test; Flow; Config; Compare; Investigate; Maintenance
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Start/continue Investigation Session
- On-demand Ping with assigned/alternate/multi-vantage source where eligible
- Traceroute/Path Analysis and healthy-path comparison
- SNMP/monitoring protocol verification
- Verify Now diagnostic pack where applicable
- Why? / What Changed? / Compare
- View Live Evidence
- Create maintenance
- View topology
- View alerts/incidents
- Open embedded CLI (RBAC; stored/personal/one-time credential modes per policy)
- View/backup configuration (RBAC)
- Run monitoring test/poll now (RBAC)

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-011 — Interface Detail

**Domain:** Network
**Primary roles:** Network engineer
**Purpose:** Interface state, utilization, errors/discards, traffic, QoS, connectivity, optical/neighbor context and history.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Performance
- Errors
- Traffic
- QoS
- Connectivity
- Events
- History

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Persistent Interface Action Bar: Ping Peer; Trace Peer/Path; Flow; Errors; QoS; Config; Neighbor; Compare; Investigate; Open Device CLI
- Open contextual entity/device/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Start/continue Investigation Session
- On-demand Ping of applicable peer/endpoint with assigned/alternate/multi-vantage source where eligible
- Traceroute/Path Analysis and healthy-path comparison
- Open Flow scoped to the interface/circuit
- View interface errors/discards, QoS and neighbor context
- Why? / What Changed? / Compare
- View Live Evidence
- Verify Now where an interface/link diagnostic workflow applies
- Open embedded parent-device CLI (RBAC)
- Open relevant configuration/NCM context (RBAC)
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-012 — Critical Link

**Domain:** Network
**Primary roles:** NOC/network
**Purpose:** Availability, utilization, latency, jitter, loss, SLA, provider, backup/failover and traffic for critical circuits.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- SLA
- Traffic
- History

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents
- Open CLI (RBAC)
- View/backup configuration (RBAC)
- Run monitoring test/poll now (RBAC)

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-013 — Link Monitor

**Domain:** Network
**Primary roles:** NOC
**Purpose:** Mass link table/heatmap with provider, bandwidth, utilization, latency, loss, errors, availability and SLA.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Table
- Utilization Heatmap
- Latency Heatmap
- SLA Breach

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents
- Open CLI (RBAC)
- View/backup configuration (RBAC)
- Run monitoring test/poll now (RBAC)

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-014 — Availability

**Domain:** Network
**Primary roles:** NOC/management
**Purpose:** Device/link/site/service availability, maintenance-adjusted availability and rollups.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Devices
- Links
- Sites
- Services

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-015 — Network Performance

**Domain:** Network
**Primary roles:** Network engineer
**Purpose:** CPU/memory/interface/latency/loss/errors/drops/queueing/congestion and top offenders.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Top Offenders
- Trends

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-016 — Network Capacity

**Domain:** Network
**Primary roles:** Capacity/network
**Purpose:** Top utilized links, sustained usage, growth, percentiles, forecast and upgrade candidates.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Current
- Trends
- Forecast
- Recommendations

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-017 — Network Flow / Traffic Analytics

**Domain:** Network
**Primary roles:** Network engineer
**Purpose:** Top talkers/conversations/protocols/ports/sites/interfaces/ASN, traffic matrix, Sankey, time series and anomalies.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Conversations
- Sources
- Destinations
- Protocols
- Sites
- Interfaces
- ASN
- Traffic Matrix

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity
- Source IP/Subnet
- Destination IP/Subnet
- Protocol
- Port
- ASN
- VRF
- Interface

**Primary table/list columns where applicable:**
- Observation window and source freshness/quality
- Source and destination address/subnet with authorized enrichment
- Protocol and source/destination port where applicable
- Exporter / observation domain / Sensor and source interface
- Direction, VRF and site context where supplied
- Bytes, packets and interval-derived bit/s or packet/s with explicit units
- Sampling/template quality and estimated-versus-observed label
- Application/ASN classification with source/provenance where supported

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Domain contract:** Flow filters and grouping must not silently mix incompatible sampling, direction, observation windows or exporter domains. Unknown application/ASN/interface identity remains unknown. Derived rates use the actual qualified interval; raw counts and estimated sampled totals are labelled separately. This contract does not imply packet/DPI payload access.

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-018 — Site-to-Site Traffic

**Domain:** Network
**Primary roles:** Network/capacity
**Purpose:** Inter-site traffic volumes, protocol mix, trends and capacity context.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Matrix
- Trends
- Conversations

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity
- Source IP/Subnet
- Destination IP/Subnet
- Protocol
- Port
- ASN
- VRF
- Interface

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-019 — Routing Health

**Domain:** Network
**Primary roles:** Network engineer
**Purpose:** BGP/OSPF/IS-IS/BFD/HA protocol status, flaps and route-change anomalies.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- BGP
- OSPF
- IS-IS
- BFD
- HA Protocols
- Routes
- Changes

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents
- Open CLI (RBAC)
- View/backup configuration (RBAC)
- Run monitoring test/poll now (RBAC)

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-020 — BGP

**Domain:** Network
**Primary roles:** Network engineer
**Purpose:** Peers, state, ASN, prefixes, flaps, uptime, BFD/interface relationships and route changes.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Peers
- Prefixes
- Flaps
- Changes

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity
- Local ASN
- Peer ASN
- Peer state
- VRF

**Primary table/list columns where applicable:**
- Canonical device / routing instance / VRF
- Peer identity and observed address
- Local ASN and peer ASN
- Peer session state and last transition
- Session uptime / age with units
- Received/advertised/accepted prefix counts where exposed
- Flap count and explicit observation window
- Related BFD/interface evidence and freshness
- Collection source, quality and last successful observation

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents
- Open CLI (RBAC)
- View/backup configuration (RBAC)
- Run monitoring test/poll now (RBAC)

**Domain contract:** A stale or unavailable BGP collection does not prove the peer is down. Prefix counters with different provider meanings are not merged into an unlabeled total; unsupported counters remain unavailable.

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-021 — MPLS / VRF

**Domain:** Network
**Primary roles:** Network engineer
**Purpose:** VRF, PE/CE, LSP/path, latency, utilization, route health and site reachability.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- VRFs
- LSPs
- PE-CE
- Health
- Traffic

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-022 — VXLAN / EVPN

**Domain:** Network
**Primary roles:** DC network
**Purpose:** VTEP, VNI, EVPN peers, underlay/overlay, reachability and MAC/IP relationships.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- VTEPs
- VNIs
- Peers
- Underlay
- Overlay

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-023 — SD-WAN Overview

**Domain:** Network
**Primary roles:** WAN ops
**Purpose:** Sites/edges, underlay links, overlay tunnels, SLA classes, BFD, latency/loss/jitter and vendor-normalized KPIs.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Sites
- Links
- Tunnels
- Underlay
- Overlay
- SLA
- Applications
- Incidents

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-024 — VPN

**Domain:** Network
**Primary roles:** Network/security ops
**Purpose:** VPN tunnels, peer, duration, throughput, latency, auth/failure and alert history.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Tunnels
- Peers
- History

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-025 — WLAN Overview

**Domain:** Network
**Primary roles:** Wireless ops
**Purpose:** Controllers/APs/clients/radios, channel/interference, RSSI/SNR, auth/association/roaming/retransmits.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- APs
- Clients
- Radios
- Controllers
- Sites
- Experience

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity
- Controller
- AP
- Radio band
- SSID
- Client

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-026 — VoIP / CUCM

**Domain:** Network
**Primary roles:** Voice ops
**Purpose:** CUCM/gateway/trunk/phone health, registration and call-quality latency/jitter/loss.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Gateways
- Trunks
- Phones
- Quality

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-027 — IP SLA / Network Assurance

**Domain:** Network
**Primary roles:** Network ops
**Purpose:** RTT/jitter/loss/availability plus DNS/HTTP/UDP/ICMP probes against SLA.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Tests
- SLA
- History

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-028 — QoS / CBQoS

**Domain:** Network
**Primary roles:** Network engineer
**Purpose:** Policy/class offered traffic, drops, queues, utilization and congestion history.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Policies
- Classes
- Drops
- Trends

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Device type
- Vendor
- Model
- Technology
- Sensor/Sensor Pool
- Interface
- Circuit/Provider where relevant
- Status/Severity

**Primary table/list columns where applicable:**
- Entity/Link
- IP/Identifier
- Site
- Vendor/Model
- Status
- Availability
- Primary KPI
- Sensor
- Active alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping
- Traceroute/Path Analysis
- Create maintenance
- View topology
- View alerts/incidents

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-029 — Infrastructure Health Overview

**Domain:** Infrastructure
**Primary roles:** Infra NOC
**Purpose:** Physical server, OS, virtualization, storage, DB, hardware, cloud and K8s health.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Servers
- Virtualization
- Storage
- Databases
- Cloud
- Kubernetes

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Host/Entity
- OS/Platform
- Site
- CPU
- Memory
- Disk/Storage
- Network
- Monitoring method
- Status
- Alerts

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping/Traceroute where applicable
- Agent/WinRM/SSH test
- Request bounded Endpoint Diagnostic Snapshot where supported
- View automatic/manual EDS history, status, trigger and missing evidence
- Create maintenance
- View processes/services
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-030 — Server Fleet

**Domain:** Infrastructure
**Primary roles:** Infra engineer
**Purpose:** Server table with OS/site/CPU/memory/disk/network/process/service/Agent/alerts.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Fleet
- Health
- Capacity

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Host/Entity
- OS/Platform
- Site
- CPU
- Memory
- Disk/Storage
- Network
- Monitoring method
- Status
- Alerts

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping/Traceroute where applicable
- Agent/WinRM/SSH test
- Create maintenance
- View processes/services
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-031 — Server Detail

**Domain:** Infrastructure
**Primary roles:** Infra engineer
**Purpose:** Host overview, disk, NIC, process, services, hardware, logs, events, RCA and audit.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Disk
- Network
- Processes
- Services
- Hardware
- Logs
- Events
- Diagnostics / Evidence
- RCA
- Audit

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping/Traceroute where applicable
- Agent/WinRM/SSH test
- Create maintenance
- View processes/services
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion. The Diagnostics/Evidence view shows EDS as Troubleshoot / Live Evidence / Investigation results for Diagnostic Job profile `ENDPOINT_DIAGNOSTIC_SNAPSHOT`, including job state, capture status (`COMPLETE/PARTIAL/FAILED/SUPPRESSED`), trigger/policy/authority provenance, source mode/vantage, capture age, evidence categories and explicit missing/denied/suppressed evidence. Endpoint capture-eligibility indicators and endpoint evidence are visually separated from Core RCA conclusions and must not be presented as authoritative host DOWN or independent root cause.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-032 — Process & Service

**Domain:** Infrastructure
**Primary roles:** Infra engineer
**Purpose:** Process name/command line/PID/CPU/memory/restarts/state and Windows/Linux services.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Processes
- Services
- History

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Canonical host and process/service technical identity
- Stable monitor selector / service unit or service name
- Display name and authorized redacted command context
- Current PID(s) as runtime evidence only
- Observed process/service state and expected instance range
- CPU utilization (%) and memory (bytes or explicitly labelled %), where supported
- Restart count/rate and declared observation window
- Effective Monitoring Profile/version and assignment source
- Collector method, observation time and quality

**Actions (subject to RBAC, scope and entitlement):**
- Open host/process/service detail
- Monitor with stable-selector, condition and policy/capacity preview
- Edit supported monitor conditions/profile with current authorization
- Stop Monitoring with reason and audit
- Open bounded Diagnostics/Evidence where authorized
- Open related alerts, history and audit; preserve shared permitted view/export actions

**Domain contract:** Apply the Phase-1 endpoint operations amendment below. Monitoring does not authorize restart or remediation. PID is never the durable selector; absent/stale/denied evidence is distinct from a confirmed missing expected process.

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-033 — Hardware / Out-of-Band

**Domain:** Infrastructure
**Primary roles:** Infra/DC
**Purpose:** iDRAC/iLO/CIMC/Redfish chassis, BIOS, CPU/DIMM/RAID/disk/PSU/temp/fans/battery/NIC/lifecycle.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Components
- Sensors
- Firmware
- Events

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Canonical chassis / managed entity and component identity
- Management interface/vendor/model and certified source
- Component type: CPU, DIMM, RAID, disk, PSU, fan, battery or other supported sensor
- Observed component state and vendor-normalized severity
- Sensor value with explicit unit and applicable threshold source
- Firmware/lifecycle version where exposed
- Observation time, source freshness and quality

**Actions (subject to RBAC, scope and entitlement):**
- Open chassis/component/sensor detail
- Inspect source/provenance and recent hardware events
- Open related host, dependency, alert or investigation
- Run an approved bounded read-only OOB diagnostic where supported
- Preserve shared permitted view/report/export/history actions

**Domain contract:** Power cycle, firmware change or device configuration is not implied by a health-screen action. Missing/denied OOB telemetry is monitoring degradation, not failed hardware.

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-034 — Virtualization Overview

**Domain:** Infrastructure
**Primary roles:** Infra/virtualization
**Purpose:** VMware, Hyper-V, Proxmox, Nutanix clusters/hosts/VMs/datastores, contention and capacity.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Clusters
- Hosts
- VMs
- Storage
- Capacity

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Host/Entity
- OS/Platform
- Site
- CPU
- Memory
- Disk/Storage
- Network
- Monitoring method
- Status
- Alerts

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping/Traceroute where applicable
- Agent/WinRM/SSH test
- Create maintenance
- View processes/services
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-035 — VMware

**Domain:** Infrastructure
**Primary roles:** VMware admin
**Purpose:** vCenter→DC→cluster→ESXi→VM→datastore hierarchy and health.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Clusters
- Hosts
- VMs
- Datastores
- Networks
- Capacity
- Events

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Host/Entity
- OS/Platform
- Site
- CPU
- Memory
- Disk/Storage
- Network
- Monitoring method
- Status
- Alerts

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping/Traceroute where applicable
- Agent/WinRM/SSH test
- Create maintenance
- View processes/services
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-036 — Nutanix

**Domain:** Infrastructure
**Primary roles:** Nutanix admin
**Purpose:** Cluster/nodes/CVM/VM/storage/alerts/capacity/latency.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Nodes
- CVMs
- VMs
- Storage
- Capacity

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Host/Entity
- OS/Platform
- Site
- CPU
- Memory
- Disk/Storage
- Network
- Monitoring method
- Status
- Alerts

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping/Traceroute where applicable
- Agent/WinRM/SSH test
- Create maintenance
- View processes/services
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-037 — Hyper-V

**Domain:** Infrastructure
**Primary roles:** Windows virtualization
**Purpose:** Host/cluster/VM/storage/network and capacity.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Hosts
- VMs
- Storage
- Network

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Host/Entity
- OS/Platform
- Site
- CPU
- Memory
- Disk/Storage
- Network
- Monitoring method
- Status
- Alerts

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping/Traceroute where applicable
- Agent/WinRM/SSH test
- Create maintenance
- View processes/services
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-038 — Proxmox

**Domain:** Infrastructure
**Primary roles:** Proxmox admin
**Purpose:** Cluster/node/VM/container/storage/network.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Nodes
- VMs
- Containers
- Storage
- Network

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Host/Entity
- OS/Platform
- Site
- CPU
- Memory
- Disk/Storage
- Network
- Monitoring method
- Status
- Alerts

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Ping/Traceroute where applicable
- Agent/WinRM/SSH test
- Create maintenance
- View processes/services
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-039 — Storage Overview

**Domain:** Infrastructure
**Primary roles:** Storage admin
**Purpose:** Capacity/utilization/IOPS/throughput/latency/controller/disk/pool/volume/replication/hardware health.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Pools
- Volumes
- Hardware
- Replication
- Capacity

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Canonical array / controller / pool / volume identity
- Certified vendor/model/source and tenant/site context
- Health, redundancy and replication state where exposed
- Usable/used/free capacity in explicit bytes or labelled units
- Read/write IOPS, throughput (bytes/s) and latency (ms), where supported
- Capacity/latency observation window and aggregation
- Dependencies, relevant alerts, source freshness and quality

**Actions (subject to RBAC, scope and entitlement):**
- Open array/controller/pool/volume detail and dependencies
- Compare qualified capacity/performance windows
- Open replication/hardware evidence and related investigation
- Preserve shared permitted view/report/export/history actions

**Domain contract:** This monitors customer storage. Platform’s own retention/protection policy is UI-093; neither view gains array provisioning or deletion authority from a generic Apply action.

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-040 — SAN Fabric

**Domain:** Infrastructure
**Primary roles:** Storage/network
**Purpose:** FC switches/ports/fabrics/zones/WWNs/link/errors/congestion/utilization.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Fabric
- Switches
- Ports
- Zones
- Errors

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Fabric, switch and port identity
- WWN / port WWN and zone membership where exposed
- Link/admin/operational state with provenance
- Negotiated link speed and throughput with explicit bit/s units
- Utilization (%) with declared denominator
- Error/discard/congestion counters and qualified interval
- Related host/storage paths and source confidence
- Observation time, freshness and collection quality

**Actions (subject to RBAC, scope and entitlement):**
- Open fabric/switch/port/zone detail
- Inspect related host/storage path and historical evidence
- Compare supported link/error/congestion intervals
- Open related alert/investigation and audit
- Preserve shared permitted view/report/export actions

**Domain contract:** Zone or port configuration changes require their separately approved privileged workflow. SAN membership, unavailable collection and actual link failure are different states; this view does not use host CPU/WinRM semantics as its primary contract.

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-041 — Data Center Environmental

**Domain:** Infrastructure
**Primary roles:** DC ops
**Purpose:** UPS/PDU/temperature/humidity/power/cooling/rack sensor health.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Power
- Environment
- Cooling
- Events

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- OS
- Host type
- Agent/Agentless
- Virtualization platform
- Storage type
- Sensor/Sensor Pool
- Status/Severity

**Primary table/list columns where applicable:**
- Site / room / rack / device / sensor identity
- UPS/PDU/power/cooling/environment sensor type
- Current value and native/normalized unit (for example °C, % humidity or W)
- Source-normalized state and applicable threshold/policy
- Power/cooling redundancy or battery state where supported
- Observation window/time, source freshness and quality
- Related affected equipment and active alerts

**Actions (subject to RBAC, scope and entitlement):**
- Open environmental device/sensor and related equipment
- Inspect qualified history, threshold source and provenance
- Open related alert/investigation
- Preserve shared permitted view/report/export/history actions

**Domain contract:** No-data/unsupported sensor values are not zero temperature, zero load or healthy power. Outlet switching, power cycling and cooling control are not authorized by this monitoring screen.

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-042 — Database Estate Overview

**Domain:** Database
**Primary roles:** DBA/NOC
**Purpose:** Cross-database availability, sessions, host/storage, replication, locks/errors and health.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Engines
- Health
- Capacity
- Alerts

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Database engine
- Instance/Cluster
- Host
- Environment
- Sensor
- Status/Severity

**Primary table/list columns where applicable:**
- Database/Instance
- Engine
- Host/Cluster
- Availability
- Connections/Sessions
- Storage
- Replication
- Locks/Errors
- Status
- Alerts

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Test connection (RBAC)
- View host dependency
- Create maintenance
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-043 — Database Detail

**Domain:** Database
**Primary roles:** DBA
**Purpose:** Connections, query health, locks/waits, storage, replication, errors, host dependency, events and RCA.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Connections
- Queries
- Locks
- Storage
- Replication
- Errors
- Host
- Events
- RCA

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Database engine
- Instance/Cluster
- Host
- Environment
- Sensor
- Status/Severity

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Test connection (RBAC)
- View host dependency
- Create maintenance
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-044 — Multi-Cloud Overview

**Domain:** Cloud/Containers
**Primary roles:** Cloud ops
**Purpose:** AWS/Azure/GCP accounts, regions, compute, storage, DB, network, K8s/LB, Serverless/FaaS (Phase 1.5), health and capacity.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Accounts
- Regions
- Compute
- Network
- Storage
- Databases
- Kubernetes
- Serverless

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Cloud provider
- Account/Subscription/Project
- Region/Zone
- Cluster
- Namespace
- Workload
- Status/Severity

**Primary table/list columns where applicable:**
- Resource
- Provider/Cluster
- Region/Namespace
- Type
- CPU/Memory where relevant
- Health
- Dependencies
- Alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Open dependency/topology
- View events
- Create maintenance
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion. For Serverless (`REQ-SLS-005` / `REQ-SLS-008`), connector/API/auth/quota/stale/no-data MUST NOT become Function DOWN.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-045 — AWS Overview

**Domain:** Cloud/Containers
**Primary roles:** AWS ops
**Purpose:** EC2/RDS/network/LB/storage/CloudWatch/regions/Lambda/Serverless (Phase 1.5)/alerts.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- EC2
- RDS
- Network
- Load Balancers
- Storage
- Regions
- Lambda/Serverless
- Alerts

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Cloud provider
- Account/Subscription/Project
- Region/Zone
- Cluster
- Namespace
- Workload
- Status/Severity

**Primary table/list columns where applicable:**
- Resource
- Provider/Cluster
- Region/Namespace
- Type
- CPU/Memory where relevant
- Health
- Dependencies
- Alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Open dependency/topology
- View events
- Create maintenance
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-046 — Azure Overview

**Domain:** Cloud/Containers
**Primary roles:** Azure ops
**Purpose:** VMs, Azure Monitor, VNet, SQL, storage, LB, AKS and Functions/Serverless (Phase 1.5).

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- VMs
- Network
- SQL
- Storage
- Load Balancers
- AKS
- Functions/Serverless
- Alerts

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Cloud provider
- Account/Subscription/Project
- Region/Zone
- Cluster
- Namespace
- Workload
- Status/Severity

**Primary table/list columns where applicable:**
- Resource
- Provider/Cluster
- Region/Namespace
- Type
- CPU/Memory where relevant
- Health
- Dependencies
- Alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Open dependency/topology
- View events
- Create maintenance
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-047 — GCP Overview

**Domain:** Cloud/Containers
**Primary roles:** GCP ops
**Purpose:** Compute, networking, storage, GKE, DB, Cloud Functions/Cloud Run/Serverless (Phase 1.5) and monitoring.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Compute
- Network
- Storage
- GKE
- Databases
- Functions/Cloud Run/Serverless
- Alerts

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Cloud provider
- Account/Subscription/Project
- Region/Zone
- Cluster
- Namespace
- Workload
- Status/Severity

**Primary table/list columns where applicable:**
- Resource
- Provider/Cluster
- Region/Namespace
- Type
- CPU/Memory where relevant
- Health
- Dependencies
- Alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Open dependency/topology
- View events
- Create maintenance
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-048 — Kubernetes Overview

**Domain:** Cloud/Containers
**Primary roles:** Platform/K8s ops
**Purpose:** Cluster/node/namespace/workload/pod/container/service/ingress health, events, restarts and capacity.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Nodes
- Namespaces
- Workloads
- Pods
- Containers
- Services
- Ingress
- Events
- Capacity

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Cloud provider
- Account/Subscription/Project
- Region/Zone
- Cluster
- Namespace
- Workload
- Status/Severity

**Primary table/list columns where applicable:**
- Resource
- Provider/Cluster
- Region/Namespace
- Type
- CPU/Memory where relevant
- Health
- Dependencies
- Alerts
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Open dependency/topology
- View events
- Create maintenance
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-049 — Container Detail

**Domain:** Cloud/Containers
**Primary roles:** K8s/container ops
**Purpose:** CPU/memory/network/disk/restarts/state/logs and node/host context.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Resources
- Network
- Logs
- History

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Cloud provider
- Account/Subscription/Project
- Region/Zone
- Cluster
- Namespace
- Workload
- Status/Severity

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Investigate
- Open dependency/topology
- View events
- Create maintenance
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-050 — Synthetic Monitoring Overview

**Domain:** Service Assurance
**Primary roles:** NOC/service owners
**Purpose:** ICMP/TCP/DNS/HTTP/HTTPS/API/TLS tests, availability, latency, failures, locations and SLA.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Tests
- Locations
- SLA

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Test type
- Probe location
- Target
- Service
- Certificate state
- Status/Severity

**Primary table/list columns where applicable:**
- Test/Target
- Location
- Type
- Availability
- Latency
- Validation result
- SLA
- Last run
- Status

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Run test now
- Investigate failure
- Create maintenance
- Open target entity
- View SLA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-051 — Synthetic Test Detail

**Domain:** Service Assurance
**Primary roles:** NOC/service owner
**Purpose:** Availability/timing/DNS/TCP/TLS/HTTP/location/history with detailed timing.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Availability
- Timing
- DNS
- TCP
- TLS
- HTTP
- Locations
- History

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Test type
- Probe location
- Target
- Service
- Certificate state
- Status/Severity

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Run test now
- Investigate failure
- Create maintenance
- Open target entity
- View SLA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-052 — Certificate

**Domain:** Service Assurance
**Primary roles:** Security/platform ops
**Purpose:** Expiring/invalid certificates, hostname mismatch, chain, weak TLS policy and days remaining.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Expiring
- Invalid
- Chain
- Policy

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Test type
- Probe location
- Target
- Service
- Certificate state
- Status/Severity

**Primary table/list columns where applicable:**
- Endpoint/service and port with observed TLS identity
- Certificate subject / SAN and issuer where available
- Validity start/end as timestamps and computed days remaining
- Chain validation, hostname match and applicable TLS policy result
- Observed fingerprint/serial and chain depth where permitted
- Probe/source vantage, last observed and quality

**Actions (subject to RBAC, scope and entitlement):**
- Open endpoint/certificate chain and validation evidence
- Run an approved bounded certificate/TLS check
- Open expiry/policy alert and related service context
- Preserve shared permitted view/report/export/history actions

**Domain contract:** A failed probe or unavailable chain is distinct from an observed invalid certificate. Monitoring exposes public certificate evidence only; private key export or certificate rotation is not implied.

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-053 — DNS / DHCP / NTP

**Domain:** Service Assurance
**Primary roles:** Network services
**Purpose:** DNS response/latency/record validation; DHCP scope/lease; NTP offset/stratum/sync.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- DNS
- DHCP
- NTP

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Test type
- Probe location
- Target
- Service
- Certificate state
- Status/Severity

**Primary table/list columns where applicable:**
- Test/Target
- Location
- Type
- Availability
- Latency
- Validation result
- SLA
- Last run
- Status

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Run test now
- Investigate failure
- Create maintenance
- Open target entity
- View SLA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-054 — AD / Identity Infrastructure

**Domain:** Service Assurance
**Primary roles:** Identity ops
**Purpose:** DCs, replication, LDAP, DNS dependency, Kerberos/auth health and selected RADIUS/TACACS tests.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Domain Controllers
- Replication
- Authentication
- DNS

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Test type
- Probe location
- Target
- Service
- Certificate state
- Status/Severity

**Primary table/list columns where applicable:**
- Test/Target
- Location
- Type
- Availability
- Latency
- Validation result
- SLA
- Last run
- Status

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Run test now
- Investigate failure
- Create maintenance
- Open target entity
- View SLA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-055 — Alert Console

**Domain:** Alerts/RCA/Change
**Primary roles:** NOC
**Purpose:** Filterable qualified alerts with severity/state/entity/site/duration/confidence/incident/ITSM and bulk operations.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- All
- Unacknowledged
- Assigned
- Suppressed
- Maintenance

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Severity
- Lifecycle state
- Assigned team/user
- Technology
- Source
- Acknowledgement
- Suppression/Maintenance
- ITSM state
- RCA status/confidence

**Primary table/list columns where applicable:**
- Time
- Severity/Type
- State
- Entity
- Site
- Summary
- Confidence/RCA state
- Owner
- Incident/ITSM
- Duration

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Acknowledge (RBAC)
- Assign (RBAC)
- Investigate / Start Investigation Session
- Verify Now where a deterministic diagnostic pack applies
- Why? / What Changed? / Compare
- Open Live Evidence and context-relevant Troubleshoot drawer
- Create/attach incident
- Create maintenance/suppression (RBAC)
- Open ITSM ticket/context

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-056 — Incident Console

**Domain:** Alerts/RCA/Change
**Primary roles:** NOC
**Purpose:** Open/acknowledged/investigating/resolved incidents with root entity, impact, RCA and ticket state.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Open
- Acknowledged
- Investigating
- Resolved
- Closed
- Suppressed

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Severity
- Lifecycle state
- Assigned team/user
- Technology
- Source
- Acknowledgement
- Suppression/Maintenance
- ITSM state
- RCA status/confidence

**Primary table/list columns where applicable:**
- Time
- Severity/Type
- State
- Entity
- Site
- Summary
- Confidence/RCA state
- Owner
- Incident/ITSM
- Duration

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Acknowledge (RBAC)
- Assign (RBAC)
- Investigate / Start Investigation Session
- Verify Now where a deterministic diagnostic pack applies
- Why? / What Changed? / Compare
- Open Live Evidence and context-relevant Troubleshoot drawer
- Create/attach incident
- Create maintenance/suppression (RBAC)
- Open ITSM ticket/context

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-057 — Incident Detail

**Domain:** Alerts/RCA/Change
**Primary roles:** NOC/engineers
**Purpose:** Incident summary, RCA, evidence, correlated alerts, impact, topology, timeline, changes, ITSM, notes and audit.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Summary
- Root Cause
- Evidence
- Correlated Alerts
- Impact
- Topology
- Timeline
- Changes
- ITSM
- Notes
- Audit

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Severity
- Lifecycle state
- Assigned team/user
- Technology
- Source
- Acknowledgement
- Suppression/Maintenance
- ITSM state
- RCA status/confidence

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Acknowledge (RBAC)
- Assign (RBAC)
- Investigate / Start Investigation Session
- Verify Now where a deterministic diagnostic pack applies
- Why? / What Changed? / Compare
- Open Live Evidence and context-relevant Troubleshoot drawer
- Create/attach incident
- Create maintenance/suppression (RBAC)
- Open ITSM ticket/context

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-058 — RCA / Investigation Workspace

**Domain:** Alerts/RCA/Change
**Primary roles:** NOC/engineers
**Purpose:** Root cause/confidence, causal graph, evidence, contradictions, missing evidence, metrics, timeline, changes and recommendations.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Conclusion
- Evidence
- Causal Graph
- Timeline
- Metrics
- Changes
- Topology
- Recommendations

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Severity
- Lifecycle state
- Assigned team/user
- Technology
- Source
- Acknowledgement
- Suppression/Maintenance
- ITSM state
- RCA status/confidence

**Primary table/list columns where applicable:**
- Time
- Severity/Type
- State
- Entity
- Site
- Summary
- Confidence/RCA state
- Owner
- Incident/ITSM
- Duration

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Acknowledge (RBAC)
- Assign (RBAC)
- Start/continue Investigation Session
- Open context-relevant Troubleshoot drawer
- Verify Now where a deterministic diagnostic workflow applies
- Run/select authorized next validation test
- Why? / What Changed? / Compare
- Open Live Evidence and Evidence Coverage
- Attach/select diagnostic, CLI, topology, config, Flow, log/event and change evidence
- Create/attach incident
- Create maintenance/suppression (RBAC)
- Open ITSM ticket/context

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-059 — False Alert Qualification

**Domain:** Alerts/RCA/Change
**Primary roles:** NOC/admin
**Purpose:** Primary failures, validation outcomes, monitoring degradation, actual outages and false/noise prevention metrics.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Validation
- Outcomes
- Policies

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Severity
- Lifecycle state
- Assigned team/user
- Technology
- Source
- Acknowledgement
- Suppression/Maintenance
- ITSM state
- RCA status/confidence

**Primary table/list columns where applicable:**
- Time
- Severity/Type
- State
- Entity
- Site
- Summary
- Confidence/RCA state
- Owner
- Incident/ITSM
- Duration

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Acknowledge (RBAC)
- Assign (RBAC)
- Investigate
- Create/attach incident
- Create maintenance/suppression (RBAC)
- Open ITSM ticket/context

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-060 — Maintenance & Suppression

**Domain:** Alerts/RCA/Change
**Primary roles:** NOC/admin
**Purpose:** Active/scheduled/recurring/expired windows, scope, reason, ticket, creator and approver.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Active
- Scheduled
- Recurring
- History

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Severity
- Lifecycle state
- Assigned team/user
- Technology
- Source
- Acknowledgement
- Suppression/Maintenance
- ITSM state
- RCA status/confidence

**Primary table/list columns where applicable:**
- Maintenance/suppression record ID and current revision
- Authorized tenant/resource scope and affected-object preview
- Lifecycle state: scheduled, active, ended, cancelled or expired as applicable
- Start/end, execution timezone and recurrence where supported
- Reason/ticket, creator and required approval state
- Suppressed alert/notification effects separately from monitoring evidence
- Next occurrence / actual activation / end time and audit reference

**Actions (subject to RBAC, scope and entitlement):**
- Create or edit a permitted window with scope/effect preview and expected revision
- Confirm reason/ticket and approval where policy requires
- Cancel a scheduled window or request early end of an active window
- Inspect affected alerts and preserved underlying evidence
- Open recurrence/history/audit; preserve shared permitted view/report/export actions

**Domain contract:** Show the effective interval and timezone before save. A stale edit returns a visible revision conflict; cancellation/early-end status remains pending until the authoritative transition is confirmed. Expiry restores ordinary qualification/notification behavior according to policy without deleting suppressed evidence or asserting recovery. Maintenance is not alert acknowledgement.

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-061 — Change Timeline

**Domain:** Alerts/RCA/Change
**Primary roles:** NOC/RCA
**Purpose:** NCM, user, monitoring-policy, maintenance, software, ITSM and device changes over time.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Timeline
- Filters
- Correlation

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Severity
- Lifecycle state
- Assigned team/user
- Technology
- Source
- Acknowledgement
- Suppression/Maintenance
- ITSM state
- RCA status/confidence

**Primary table/list columns where applicable:**
- Time
- Severity/Type
- State
- Entity
- Site
- Summary
- Confidence/RCA state
- Owner
- Incident/ITSM
- Duration

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Acknowledge (RBAC)
- Assign (RBAC)
- Investigate
- Create/attach incident
- Create maintenance/suppression (RBAC)
- Open ITSM ticket/context

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-062 — NCM Overview

**Domain:** NCM
**Primary roles:** Network config team
**Purpose:** Backup success/failure, changes, compliance, deviations, approvals and bulk jobs.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Devices
- Backups
- Changes
- Compliance
- Jobs
- Templates
- Approvals
- Reports

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Vendor
- Device type
- Job type
- Compliance policy
- Approval state
- Change origin
- Result

**Primary table/list columns where applicable:**
- Device
- Site
- Vendor
- Backup state
- Last change
- Compliance
- Job/Approval state
- Owner
- Result

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Backup now (RBAC)
- Compare/Diff
- Open CLI (RBAC)
- Push/Approve/Rollback according to role
- View audit/RCA evidence

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-063 — Device Configuration

**Domain:** NCM
**Primary roles:** Network config
**Purpose:** Current config, versions, diff, compliance, history and jobs.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Current Configuration
- Versions
- Diff
- Compliance
- History
- Jobs

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Vendor
- Device type
- Job type
- Compliance policy
- Approval state
- Change origin
- Result

**Primary table/list columns where applicable:**
- Device
- Site
- Vendor
- Backup state
- Last change
- Compliance
- Job/Approval state
- Owner
- Result

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Backup now (RBAC)
- Compare/Diff
- Open CLI (RBAC)
- Push/Approve/Rollback according to role
- View audit/RCA evidence

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-064 — NCM Compliance

**Domain:** NCM
**Primary roles:** Network/security
**Purpose:** Compliant/noncompliant/unknown by policy/site/vendor with violation detail.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Policies
- Devices
- Violations

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Vendor
- Device type
- Job type
- Compliance policy
- Approval state
- Change origin
- Result

**Primary table/list columns where applicable:**
- Device
- Site
- Vendor
- Backup state
- Last change
- Compliance
- Job/Approval state
- Owner
- Result

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Backup now (RBAC)
- Compare/Diff
- Open CLI (RBAC)
- Push/Approve/Rollback according to role
- View audit/RCA evidence

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-065 — Configuration Change

**Domain:** NCM
**Primary roles:** Network/RCA
**Purpose:** Changes correlated with users, CLI sessions, NCM jobs, alerts and incidents.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Timeline
- Users
- Devices
- Incidents

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Vendor
- Device type
- Job type
- Compliance policy
- Approval state
- Change origin
- Result

**Primary table/list columns where applicable:**
- Device
- Site
- Vendor
- Backup state
- Last change
- Compliance
- Job/Approval state
- Owner
- Result

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Backup now (RBAC)
- Compare/Diff
- Open CLI (RBAC)
- Push/Approve/Rollback according to role
- View audit/RCA evidence

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-066 — IPAM Overview

**Domain:** IPAM
**Primary roles:** IPAM admin
**Purpose:** IPv4/IPv6 utilization, subnets, free/used/reserved, conflicts, DHCP/DNS and forecast.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- IPv4
- IPv6
- Subnets
- Conflicts
- Forecast

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Address family
- VRF
- VLAN
- Subnet/Supernet
- Address state
- DHCP/DNS source
- Conflict state

**Primary table/list columns where applicable:**
- Prefix/IP
- VRF/VLAN
- Site
- State
- Owner/Device
- DHCP/DNS
- Utilization/Conflict
- Last seen

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Reserve/Release/Edit according to RBAC
- Investigate conflict
- Open related device
- Export addresses

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-067 — Subnet Detail

**Domain:** IPAM
**Primary roles:** IPAM admin
**Purpose:** Addresses, utilization, devices, DHCP, DNS, history and forecast.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Addresses
- Utilization
- Devices
- DHCP
- DNS
- History
- Forecast

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Address family
- VRF
- VLAN
- Subnet/Supernet
- Address state
- DHCP/DNS source
- Conflict state

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Reserve/Release/Edit according to RBAC
- Investigate conflict
- Open related device
- Export addresses

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-068 — IP Conflict

**Domain:** IPAM
**Primary roles:** IPAM/NOC
**Purpose:** Duplicate IP/MAC conflict, first/last seen, site and remediation state.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Open
- History
- Resolved

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Address family
- VRF
- VLAN
- Subnet/Supernet
- Address state
- DHCP/DNS source
- Conflict state

**Primary table/list columns where applicable:**
- Prefix/IP
- VRF/VLAN
- Site
- State
- Owner/Device
- DHCP/DNS
- Utilization/Conflict
- Last seen

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Reserve/Release/Edit according to RBAC
- Investigate conflict
- Open related device
- Export addresses

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-069 — IPAM Capacity Forecast

**Domain:** IPAM
**Primary roles:** IPAM/capacity
**Purpose:** Subnet growth and predicted exhaustion date.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Forecast
- Risk

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Address family
- VRF
- VLAN
- Subnet/Supernet
- Address state
- DHCP/DNS source
- Conflict state

**Primary table/list columns where applicable:**
- Prefix/IP
- VRF/VLAN
- Site
- State
- Owner/Device
- DHCP/DNS
- Utilization/Conflict
- Last seen

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Reserve/Release/Edit according to RBAC
- Investigate conflict
- Open related device
- Export addresses

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-070 — Capacity Planning Overview

**Domain:** Capacity/SLA
**Primary roles:** Capacity/management
**Purpose:** Network/server/storage/cloud/DB/site utilization, growth and risk horizon.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Current
- Growth
- Forecast
- Recommendations

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- SLA policy
- Provider
- Service
- Resource type
- Risk horizon
- Breach state

**Primary table/list columns where applicable:**
- Scope
- Target
- Actual
- Trend
- Risk/Breach
- Downtime/Utilization
- Forecast
- Owner

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Create recommendation/work item
- Drill into contributing outages/consumers
- Export SLA evidence

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-071 — SLA

**Domain:** Capacity/SLA
**Primary roles:** Management/NOC
**Purpose:** SLA/OLA/Provider-SLA/SLO service assurance by service/region/provider/site/team/shift, with target/minimum, qualified downtime, evidence quality, violations, exclusions, remaining budget and auditable period close.

**Service-assurance contract:** Provide editable OOB industry templates plus custom policy authoring; reusable service-time/holiday/shift calendars; expected and minimum thresholds; policy simulation/backtest; measurement-quality/completeness; raw versus adjusted result; scope snapshot; degraded-condition rules; dependency-aware service calculation; OLA/provider attribution; period lifecycle (`OPEN`, `PRELIMINARY`, `REVIEW`, `FINAL`, audited `RESTATED`); evidence drill-down; report obligations and attestation. Team-attributed OLA becomes authoritative only when canonical tenant-scoped Team identity/binding is available.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- SLAs
- OLAs
- Services
- Sites
- Providers
- Policies
- Violations
- Evidence
- Reports

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- SLA policy
- Provider
- Service
- Resource type
- Risk horizon
- Breach state

**Primary table/list columns where applicable:**
- Scope
- Target
- Actual
- Trend
- Risk/Breach
- Downtime/Utilization
- Forecast
- Owner

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Create recommendation/work item
- Drill into contributing outages/consumers
- Export SLA evidence

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-072 — ISP / Carrier SLA

**Domain:** Capacity/SLA
**Primary roles:** WAN/vendor mgmt
**Purpose:** Provider/circuit SLA assurance for availability, latency, loss, jitter, target/minimum, actual, breach/at-risk state, exclusions, provider attribution and evidence.

**Provider-SLA contract:** Support circuit/provider/service scope, measurement vantage/path, service-time calendar, degraded-condition treatment, approved exclusions, provider ticket/reference evidence, dispute/attestation and PDF/CSV evidence export. Provider data is evidence and never overrides qualified Platform monitoring truth without explicit policy.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Providers
- Circuits
- Breaches

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- SLA policy
- Provider
- Service
- Resource type
- Risk horizon
- Breach state

**Primary table/list columns where applicable:**
- Scope
- Target
- Actual
- Trend
- Risk/Breach
- Downtime/Utilization
- Forecast
- Owner

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Create recommendation/work item
- Drill into contributing outages/consumers
- Export SLA evidence

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-073 — Sensor Estate Overview

**Domain:** Platform/Sensor/Data
**Primary roles:** Platform admin
**Purpose:** Sensor roles, health, offline/drain/maintenance/standby and capacity.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Pools
- Capacity
- Versions

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Core node
- Sensor role
- Sensor Pool
- Agent version
- Data class
- Topic/consumer
- Storage volume
- Health state

**Primary table/list columns where applicable:**
- Component
- Role/Type
- Site/Node
- Health
- CPU/RAM
- Disk/Queue
- Throughput/Backlog
- Version
- Last heartbeat

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Run diagnostics
- Drain/rebalance where permitted
- View capacity
- View logs/errors
- Generate support bundle where applicable

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-074 — Sensor Detail

**Domain:** Platform/Sensor/Data
**Primary roles:** Platform admin
**Purpose:** CPU/RAM/disk/queue/polls/sec/metrics/sec/failures/RTT/buffer/targets/capacity.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Workload
- Targets
- Queues
- HA
- Logs
- Updates
- Audit

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Core node
- Sensor role
- Sensor Pool
- Agent version
- Data class
- Topic/consumer
- Storage volume
- Health state

**Primary table/list columns where applicable:**
- Contextual summary cards/panels rather than a mandatory primary table; any table must expose entity, state, time, site/owner and drill-down.

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Run diagnostics
- Drain/rebalance where permitted
- View capacity
- View logs/errors
- Generate support bundle where applicable

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-075 — Sensor Pool / HA

**Domain:** Platform/Sensor/Data
**Primary roles:** Platform admin
**Purpose:** Active/standby capacity, N+1/N+K/1:1 state, utilization, failover and assignments.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Assignments
- Failover
- Capacity

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Core node
- Sensor role
- Sensor Pool
- Agent version
- Data class
- Topic/consumer
- Storage volume
- Health state

**Primary table/list columns where applicable:**
- Component
- Role/Type
- Site/Node
- Health
- CPU/RAM
- Disk/Queue
- Throughput/Backlog
- Version
- Last heartbeat

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Run diagnostics
- Drain/rebalance where permitted
- View capacity
- View logs/errors
- Generate support bundle where applicable

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-076 — Telemetry Quality

**Domain:** Platform/Sensor/Data
**Primary roles:** Platform/NOC
**Purpose:** Freshness/completeness/missing metrics/failed polls/delays/clock/schema errors and data-quality score.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Freshness
- Completeness
- Failures
- Clock
- Schema

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Core node
- Sensor role
- Sensor Pool
- Agent version
- Data class
- Topic/consumer
- Storage volume
- Health state

**Primary table/list columns where applicable:**
- Component
- Role/Type
- Site/Node
- Health
- CPU/RAM
- Disk/Queue
- Throughput/Backlog
- Version
- Last heartbeat

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Run diagnostics
- Drain/rebalance where permitted
- View capacity
- View logs/errors
- Generate support bundle where applicable

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-077 — Agent Estate

**Domain:** Platform/Sensor/Data
**Primary roles:** Platform/infra
**Purpose:** Agent OS/version/online state/update/heartbeat/site/buffer/test profile.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Versions
- Health
- Updates
- Edge Tests

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Core node
- Sensor role
- Sensor Pool
- Agent version
- Data class
- Topic/consumer
- Storage volume
- Health state

**Primary table/list columns where applicable:**
- Component
- Role/Type
- Site/Node
- Health
- CPU/RAM
- Disk/Queue
- Throughput/Backlog
- Version
- Last heartbeat

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Run diagnostics
- Drain/rebalance where permitted
- View capacity
- View logs/errors
- Generate support bundle where applicable

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-078 — Kafka / Data Pipeline Health

**Domain:** Platform/Sensor/Data
**Primary roles:** Platform admin
**Purpose:** Broker/topic/partition/consumer lag/throughput/schema/ingestion health.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Topics
- Consumers
- Errors
- Throughput

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Core node
- Sensor role
- Sensor Pool
- Agent version
- Data class
- Topic/consumer
- Storage volume
- Health state

**Primary table/list columns where applicable:**
- Component
- Role/Type
- Site/Node
- Health
- CPU/RAM
- Disk/Queue
- Throughput/Backlog
- Version
- Last heartbeat

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Run diagnostics
- Drain/rebalance where permitted
- View capacity
- View logs/errors
- Generate support bundle where applicable

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-079 — Core Database / Storage Health

**Domain:** Platform/Sensor/Data
**Primary roles:** Platform admin
**Purpose:** PostgreSQL and ClickHouse health, connections/inserts/queries/merges/disk/sync.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- PostgreSQL
- ClickHouse
- Storage
- Health

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Core node
- Sensor role
- Sensor Pool
- Agent version
- Data class
- Topic/consumer
- Storage volume
- Health state

**Primary table/list columns where applicable:**
- Component
- Role/Type
- Site/Node
- Health
- CPU/RAM
- Disk/Queue
- Throughput/Backlog
- Version
- Last heartbeat

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Run diagnostics
- Drain/rebalance where permitted
- View capacity
- View logs/errors
- Generate support bundle where applicable

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-080 — Integration Health

**Domain:** Integrations
**Primary roles:** Integration admin
**Purpose:** ServiceNow/BMC/ServiceDesk/SummitAI/Freshservice/Teams/Slack/email/webhook connection, queue, errors and latency.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Connectors
- Queue
- Errors

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Connector type
- Connector instance
- Direction
- Queue state
- Result
- ITSM status

**Primary table/list columns where applicable:**
- Connector
- Type
- State
- Queue
- Success rate
- Latency
- Last success
- Last error

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Test connector
- Retry failed item
- Pause/enable connector (RBAC)
- Open mapping
- View audit

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-081 — ITSM Lifecycle

**Domain:** Integrations
**Primary roles:** NOC/integration
**Purpose:** Tickets created/updated/resolved/closed/reopened/failed sync and Platform↔ITSM correlation.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Created
- Updated
- Recovery
- Failures

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Connector type
- Connector instance
- Direction
- Queue state
- Result
- ITSM status

**Primary table/list columns where applicable:**
- Platform incident/change identity and external binding ID
- Connector instance, external system and external object key/link
- Platform monitoring/incident state
- External ITSM lifecycle state, displayed separately
- Synchronization direction, mapping/version and last attempted operation
- Delivery result, retry/dead-letter/conflict state and correlation ID
- Last successful synchronization and source freshness

**Actions (subject to RBAC, scope and entitlement):**
- Open authorized Platform object and external-object link
- Inspect sanitized delivery history, mapping and conflict evidence
- Request an authorized idempotent retry/reconcile through the Integration owner
- Open connector health/test sandbox and audit
- Preserve shared permitted view/report/export actions

**Domain contract:** An external closed/resolved ticket cannot mark the monitored resource UP or replace Platform recovery qualification. Retry is unavailable until the existing outbox/inbox contract and current permission authorize it; duplicate delivery must not create a duplicate external incident.

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-082 — Audit Overview

**Domain:** Governance/Security
**Primary roles:** Auditor/security
**Purpose:** User actions, config changes, login failures, CLI/NCM/RBAC/credential/API actions.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Privileged
- Authentication
- Configuration

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- User
- Role
- Action
- Resource
- Source IP
- Session
- Result
- Privilege
- MFA state

**Primary table/list columns where applicable:**
- Time
- Actor/User
- Action
- Resource
- Tenant/Site
- Source IP/Session
- Result
- Reason/Ticket

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Open audit detail
- Export according to policy
- Review access
- Revoke/disable where permitted

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-083 — Audit Explorer

**Domain:** Governance/Security
**Primary roles:** Auditor/security
**Purpose:** Searchable audit by user/action/resource/module/tenant/site/IP/session/result with before/after.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Search
- Detail
- Export

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- User
- Role
- Action
- Resource
- Source IP
- Session
- Result
- Privilege
- MFA state

**Primary table/list columns where applicable:**
- Time
- Actor/User
- Action
- Resource
- Tenant/Site
- Source IP/Session
- Result
- Reason/Ticket

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Open audit detail
- Export according to policy
- Review access
- Revoke/disable where permitted

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-084 — Access Governance

**Domain:** Governance/Security
**Primary roles:** Security/admin
**Purpose:** Users, privileged users, dormant/service accounts, MFA, temporary access and denied attempts.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Users
- Privileged
- Temporary Access
- Failures

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- User
- Role
- Action
- Resource
- Source IP
- Session
- Result
- Privilege
- MFA state

**Primary table/list columns where applicable:**
- Time
- Actor/User
- Action
- Resource
- Tenant/Site
- Source IP/Session
- Result
- Reason/Ticket

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Open audit detail
- Export according to policy
- Review access
- Revoke/disable where permitted

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-085 — CLI Session History

**Domain:** Governance/Security
**Primary roles:** Security/network
**Purpose:** User/device/start/end/commands/privilege/result/config-change/ticket and optional playback.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Sessions
- Commands
- Changes
- Playback

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- User
- Role
- Action
- Resource
- Source IP
- Session
- Result
- Privilege
- MFA state

**Primary table/list columns where applicable:**
- Time
- Actor/User
- Action
- Resource
- Tenant/Site
- Source IP/Session
- Result
- Reason/Ticket

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Open audit detail
- Export according to policy
- Review access
- Revoke/disable where permitted

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-086 — Technology Lifecycle Risk

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** EOS/EOL/unsupported firmware/model risk by vendor/site/criticality

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Lifecycle risk
- Firmware
- Models
- Sites
- Export
- Create remediation plan

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Scope/Entity
- State
- Finding
- Risk/Impact
- Owner
- Recommended action
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Review recommendation
- Apply only with RBAC/approval
- Open audit/history

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-087 — Desired vs Actual / Configuration Drift

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** Monitoring/assignment/profile/version drift

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Drift by Sensor/entity/policy
- Explain source
- Reconcile
- Audit

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Scope/Entity
- State
- Finding
- Risk/Impact
- Owner
- Recommended action
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Review recommendation
- Apply only with RBAC/approval
- Open audit/history

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-088 — Shift Handover

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** NOC handover of unresolved operational state

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Critical incidents
- maintenance
- changes
- pending approvals
- platform issues
- notes
- handover report

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Scope/Entity
- State
- Finding
- Risk/Impact
- Owner
- Recommended action
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Review recommendation
- Apply only with RBAC/approval
- Open audit/history

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-089 — Migration Studio

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** Import/reconcile from existing monitoring/CMDB tools

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Sources
- Imports
- Mapping
- Review
- Unsupported
- Job history

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Scope/Entity
- State
- Finding
- Risk/Impact
- Owner
- Recommended action
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Review recommendation
- Apply only with RBAC/approval
- Open audit/history

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-090 — Intelligence Quality

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** RCA/anomaly/qualification quality and feedback metrics

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- RCA accuracy
- Inconclusive
- false-root
- time-to-RCA
- feedback

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Scope/Entity
- State
- Finding
- Risk/Impact
- Owner
- Recommended action
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Review recommendation
- Apply only with RBAC/approval
- Open audit/history

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-091 — Alert Policy Effectiveness

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** Identify noisy/ineffective rules

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Alert volume
- incident conversion
- repeats
- suppressions
- recommendations

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Scope/Entity
- State
- Finding
- Risk/Impact
- Owner
- Recommended action
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Review recommendation
- Apply only with RBAC/approval
- Open audit/history

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-092 — Data Pipeline Integrity

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** Prove telemetry continuity across ingestion pipeline

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Produced
- accepted
- Kafka
- processed
- inserted
- rejected
- duplicates
- late
- dropped

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Tenant/source/Sensor and stream lineage or other canonical ingestion scope
- Explicit reconciliation interval and evidence watermark
- Produced and durably accepted counts at the named boundaries
- Kafka committed / processed / inserted counts with their exact contract meaning
- Rejected, duplicate, late and dropped counts with reason/source
- Unresolved backlog, missing accounting evidence and collection quality
- Last reconciliation and supporting evidence/audit references

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Review recommendation
- Apply only with RBAC/approval
- Open audit/history

**Domain contract:** Counters from different windows, streams or ACK boundaries cannot be subtracted as if they measure the same population. Missing accounting evidence is UNKNOWN, not a zero-loss assertion. This view never changes frozen ACK semantics or claims end-to-end exactly-once delivery from visual equality.

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-093 — Storage & Retention

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** Capacity, growth, retention, rollups, archive and 80% protection

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Breakdown by data class/volume
- forecast
- purge history
- policy simulation

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Volume/deployment role and affected tenant/data-class scope
- Usable capacity, current usage and measurement time/quality
- Measured daily growth and requested/effective retention by data class
- Calculator/model version, assumptions and uncertainty uplift
- Forecast required space, headroom/shortfall and confidence/UNKNOWN state
- Planning target versus fixed emergency-protection state
- Protected metadata, legal/investigation holds and eligible reclamation
- Last policy revision/change, purge outcome and audit reference
- Customer notification state, delivery attempt/result and next retry/escalation

**Actions (subject to RBAC, scope and entitlement):**
- Open the canonical per-volume capacity calculator preview
- Compare requested retention against the qualified model and available headroom
- Preview a retention-policy change and protected/affected data before confirmation
- Apply only a currently authorized, revision-checked policy change with reason and required approval
- Inspect purge/hold history, risk notifications and actual delivery outcomes
- Preserve shared permitted view/report/export/history actions

**Domain contract:** The calculator is owned by Platform_11 §9–11 and Platform_05 §37, including the approved #292 storage assurance: at least 20% uncertainty uplift, a separate 70% planned-utilization goal and fixed non-disableable 80% emergency protection. Display measured inputs, assumptions and the resulting per-volume recommendation with explicit `OBSERVED` / `ESTIMATED` / `UNKNOWN` / `PROJECTED` labels; missing measurements remain UNKNOWN. Backend quantities are integer bytes; UI labels decimal GB/TB versus binary GiB/TiB explicitly (`Platform_11` §9.3.1). Do not promise universal 180-day retention or a customer SLA. A version-bound policy preview shows requested/effective retention, growth/shortfall, protected data and legal-hold conflict. Apply requires current authorization and the same reviewed policy revision; a concurrent policy change or stale preview is rejected and forces re-simulation; a changed estimate/scope is shown before confirmation. Planned steady-state above the configured early-warning threshold requires an explicit authorized disposition. Capacity/retention risk notifications expose generated, queued, delivered or failed/retrying outcome according to the Notification/Integration contract: generated is not delivered. Delivery failure remains visible and auditable. No action disables the 80% safety rule or silently deletes protected/legal-held data.

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-094 — Usage & Licensing

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** Entitlement/consumption/expiry forecasting

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Licensed vs used
- module usage
- growth
- expiry/grace state

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Scope/Entity
- State
- Finding
- Risk/Impact
- Owner
- Recommended action
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Review recommendation
- Apply only with RBAC/approval
- Open audit/history

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-095 — Housekeeping & Lifecycle Jobs

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** Background maintenance/rollup/backup/archive/job status

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Running
- completed
- failed
- duration
- next run
- details

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Scope/Entity
- State
- Finding
- Risk/Impact
- Owner
- Recommended action
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Review recommendation
- Apply only with RBAC/approval
- Open audit/history

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-096 — Post-Incident Review

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** Generate/review PIR and problem-management actions

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Timeline
- root cause
- impact
- CAPA
- lessons
- approvals
- export

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Scope/Entity
- State
- Finding
- Risk/Impact
- Owner
- Recommended action
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Review recommendation
- Apply only with RBAC/approval
- Open audit/history

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-097 — Runbook Library

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** Approved troubleshooting/operations procedures

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Search
- by incident/device/service
- version
- owner
- recommendation usage

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Scope/Entity
- State
- Finding
- Risk/Impact
- Owner
- Recommended action
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Review recommendation
- Apply only with RBAC/approval
- Open audit/history

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

### UI-098 — Device / Time Comparison

**Domain:** Cross-cutting Operations
**Primary roles:** Role determined by RBAC and domain ownership
**Purpose:** Side-by-side operational comparison

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Metrics
- config
- topology
- firmware
- alerts
- profile
- time comparison

**Default filters:**
- Time range
- Timezone
- Auto-refresh
- Tenant/Customer (only where user is authorized for multiple tenants)
- Region
- Country
- Site
- Business Unit
- Group/Dynamic Group
- Criticality
- Environment
- Tags
- Status

**Primary table/list columns where applicable:**
- Scope/Entity
- State
- Finding
- Risk/Impact
- Owner
- Recommended action
- Last update

**Actions (subject to RBAC, scope and entitlement):**
- Open contextual entity/detail
- Save view
- Share permitted view
- Export
- Create/Schedule report
- Add annotation where time-series context exists
- Review recommendation
- Apply only with RBAC/approval
- Open audit/history

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Apply §3.6 with this view’s authoritative read/query contract, bounded server pagination and rendered-row virtualization where needed. Use time-series rollups only for eligible historical telemetry; disclose truncation and stale/partial results.

---

## 6. Administration screen architecture

Administration is permission-aware and includes: Customers/Tenants; Sites/Groups/Hierarchies/Tags; Users/Roles/Permissions/Service Accounts/API Clients/Temporary Access/Approvals/Sessions; Authentication (Local/AD/LDAP/SAML/OIDC/Entra/MFA/session/password); Credentials & Secrets; Discovery; Monitoring Profiles; Adaptive Polling; Alert/Validation Policies; Maintenance/Suppression; Notifications; ITSM; Sensors/Pools/HA/Capacity/Deployment/Versions; Agents/Policies/Updates/Edge Tests; Data/Retention/Rollups/Archive; HA/DR; AI/Data Boundaries/Model Registry; Licensing; Security/PKI/API governance; and System/Backup/Restore/Upgrade/Support Bundle.

## 7. UI acceptance gates

- No ordinary provisioning or policy workflow requires file/database/code edits.
- All Add/Create/Test/Save/Apply actions are functional and covered by end-to-end tests.
- Role simulator verifies hidden/read-only/action-limited states.
- Dashboard font/readability and dark/light modes are tested.
- 200K inventory does not result in 200K browser rows.
- Query-too-large path offers asynchronous export instead of destabilizing Core.
- All failure states satisfy No Silent Failure.
- HTTPS-only UI; no application listener on TCP/80.
- Dashboard context is preserved across drill-down and back navigation.
- System dashboards can be restored even after customer customization.

## 8. Platform service, boot and diagnostic UX — v3.1

Administration → Platform Operations shall expose Core, Sensor and Agent service health without requiring shell access. Authorized users shall be able to see service version, host/instance identity, uptime, last start time, restart count, current health, last failure code/reason, last successful recovery, disk/log pressure and links to sanitized diagnostics/support-bundle generation.

The UI must distinguish:

- service intentionally stopped;
- host unreachable;
- service process failed;
- dependency unavailable;
- repeated crash loop;
- upgrade/restart in progress;
- healthy after automatic recovery.

Normal users do not receive raw secret-bearing logs. Diagnostic download/generation is RBAC controlled and redacted.

## 9. v3.2.2 specialized-master UI amendment — Notification Center and Discovery Review

The 98 out-of-box screens in §5 remain the base mandatory catalogue and are not silently renumbered. Specialized-master views supplement that catalogue. They are not additional `UI-099+` rows unless separately added to §5 through change control.

### 9.1 Global Notification Center

Normative owner: `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md`. This amendment is a cross-reference only.

Required UI behavior:

- one global per-user bell/center;
- tenant/RBAC/resource scoped server-side;
- platform/workflow notifications remain distinct from operational Alerts/Incidents;
- unread/read state;
- deep links;
- acknowledgement/action only when authorized.

### 9.2 Discovery / Onboarding Review

Normative owner: `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md`. This amendment is a cross-reference only.

Required UI behavior:

- candidate review queue;
- identity/reconciliation evidence;
- duplicate/conflict state;
- approve/reject/unmanaged/exclude controls where authorized;
- reason/comment;
- group/sub-group assignment;
- policy preview;
- capacity preview;
- audit trail.

Capability Pack / dimensional-certification operator views, when later specified, are supplements governed by `Platform_23` and must not be invented as a parallel screen catalogue here.

## 10. v3.2.2 RCA and regional Sensor administration amendment — 2026-09-21

### 10.1 RCA / Investigation progressive disclosure

Normative owner: `Platform_22_Monitoring_Intelligence_Alert_Quality_and_AI_Assurance_Master.md`.

The existing RCA/Incident/Investigation surfaces shall present, in progressive disclosure order:

1. current conclusion or `INCONCLUSIVE`;
2. severity separately from causal trust/confidence;
3. first causal evidence separately from first alert;
4. blast radius / affected services;
5. supporting evidence;
6. contradictory and missing evidence;
7. rejected/superseded hypotheses with reason;
8. timeline, relevant changes and topology/causal graph;
9. correlated child alerts/evidence;
10. recovery validation;
11. Similar Incident / Flight Recorder references where available;
12. authorized next diagnostic/runbook/remediation proposal.

Prior RCA revisions remain accessible from history; a later machine conclusion must not silently replace the reasoning visible for an earlier revision.

### 10.2 Region / Sensor Pool administration

Normative owner: `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md`.

Administration → Sensors/Pools shall expose optional customer-defined Region, Site, Sensor Pool and Sensor placement context. Region names are customer-defined and are not fixed geography enums.

Pool views show at minimum scope, role/capability eligibility, health, capacity/headroom, failure-domain/placement policy, members, assignment counts and degraded/maintenance state. Reassignment/drain actions display generation/state and remain server-authorized/audited.

Sensor/Agent enrollment UX shows token/authority expiry and resulting certificate/assignment state but never private keys or reusable secrets. DNS or IP/VIP bootstrap errors surface trust/SAN mismatch explicitly; the UI must not offer “disable certificate verification” or plaintext Platform transport as a production workaround.

---

## Phase-1 endpoint operations UI reconciliation — Issues #169/#170

### Server/VM/Endpoint detail

The Server Detail experience shall expose an **Installed Software** view/tab for supported Windows/Linux entities. It shows at minimum product/package name, version, vendor/publisher where available, architecture, source/package type, installation scope, last observed, first/last seen from Core reconciliation, provenance/quality and recent install/remove/version-change evidence. Unsupported or permission-limited fields are visibly unavailable rather than fabricated.

### Process & Service monitoring workflow

UI-032 Process & Service gains governed actions:

1. select discovered process/service;
2. **Monitor**;
3. display the stable selector that will be used;
4. configure supported conditions;
5. select/inherit Monitoring Profile;
6. preview capacity/MWU and inherited policy;
7. save with RBAC/audit;
8. show effective policy and current monitored state;
9. **Stop Monitoring** where authorized.

PID is displayed as current runtime evidence only and is never presented as durable monitor identity. Short-lived process populations are summarized/bounded; the UI does not encourage monitoring every transient PID.

Applicable conditions include expected process absent, service stopped/failed, unexpected termination, restart/restart-rate, expected instance range, sustained CPU/memory and suspected unresponsiveness when an applicable evidence contract exists. Restart/remediation remains a separate privileged action and is not granted by Monitor.

### Criticality and effective Monitoring Profile

Entity detail/onboarding/profile surfaces show:

- business Criticality;
- suggested Monitoring Profile;
- effective Monitoring Profile/version;
- assignment mode `AUTO_FROM_CRITICALITY` or `EXPLICIT_PROFILE`;
- inheritance/override source;
- capacity impact;
- audit history.

Changing Criticality shall preview the resulting automatic-profile change. If an explicit profile is active, the UI clearly states that Criticality will not replace it unless the user deliberately changes assignment mode/profile.

### Triggered Fast RCA presentation

Do not create a second “Fast RCA dashboard.” Triggered Fast RCA is presented through existing Diagnostics/Evidence, Investigation and RCA experiences.

The UI may show which EDS definition ran, capture status, evidence categories, missing/denied/suppressed evidence, related physical/virtualization/storage/path vantages, evidence age/provenance, current RCA revision, prior revisions, confidence, contradictions and missing evidence.

Endpoint indicators are visually labelled as evidence, not final cause. VMware/hardware/storage evidence is shown with its own vantage. `INCONCLUSIVE` is a supported outcome.


---

## 15. v3.3.0 productized operations UX

### 15.1 Investigation Workbench — Phase 1

The existing Incident/Investigation experience is productized as one Investigation Workbench. It retains tenant/site/time scope while synchronizing available metrics, qualified alerts/incidents, Flow, logs/Syslog/traps/events, topology, Path Intelligence, routing/config/NCM changes, application/service telemetry, database/cloud/deployment events, synthetics, Sensor state, Flight Recorder and ITSM activity. Selecting evidence in one pane updates the common time/evidence context without creating another evidence store. Users can always distinguish observed fact, external/declarative context, inferred relationship, stale/missing evidence and RCA hypothesis; `INCONCLUSIVE` remains valid.

### 15.2 Path Intelligence — Phase 1

Path views show current/historical/multi-vantage path, path fingerprints/change, hop latency/loss/jitter where reliable, hidden/incomplete-hop quality, related interfaces/routing peers/config changes/Flow/synthetics/provider/business-service context and prior incident evidence. Path and topology time travel use the canonical temporal graph.

### 15.3 OTel onboarding and service operations

Administration exposes **Add OpenTelemetry Source** with ingress profile, endpoint/authentication, signal selection, tenant/site/source binding, connection test, telemetry preview, resource/service identity-resolution preview, cardinality/sensitive-field/quality findings, estimated capacity impact and controlled activation. Phase-1 service views use metrics/logs and trace-context/exemplars when present; Phase-1 UI does not promise distributed trace waterfall/search. Phase-1.5 may add bounded trace search/waterfall after certification.

### 15.4 NCM Change Assurance

NCM UI adds Change Plans, Assurance Records, Pre-checks, Risk/Blast Radius, External Change/Approval, Canary/Batches, Post-check/Stabilization and Outcome/Audit views. The approval panel displays exact plan version, target/config hashes and approved window and clearly marks `STALE_REAPPROVAL_REQUIRED`. Risk/blast-radius views expose evidence freshness/confidence and never present graph inference as certain impact.

### 15.5 ITSM & CMDB Integration onboarding

Administration -> Integrations -> ITSM & CMDB -> Add Integration provides vendor selection, endpoint/authentication, project/tenant context where applicable, capability discovery, permission validation, standard mapping proposal, unsupported/edition-limited capability disclosure, Integration Test Sandbox, enablement toggles and Integration Health. Mapping Studio provides declarative versioned mappings with preview/test/promotion/rollback; no executable customer scripting.

Synchronization/conflict UI keeps **Monitoring State** separate from **External ITSM State**. CMDB reconciliation provides match evidence/confidence, external binding, authority profile, field/relationship conflicts and operator resolution/audit. External CI disappearance never presents the monitored entity as automatically deleted.

### 15.6 Phase-1.5 specialist UX

Endpoint & Switch-Port Intelligence adds MAC/IP/user-when-available/AP/switch/port/VLAN attachment history and provenance. Browser Journey Synthetics adds step/assertion/timing/failure-evidence views only after browser runtime admission. Database Performance Intelligence adds bounded fingerprint/wait/blocking/deadlock/regression/plan evidence. Virtualization Optimization shows evidence-backed recommendations requiring governed human action. None of these Phase-1.5 screens are first-GA implementation claims.

## Phase 1.5 Serverless UI extension — Issue #268

**Status:** SPECIFIED / NOT IMPLEMENTED / NOT CERTIFIED. Governing requirements: `REQ-SLS-008`, `PRD-SLS-008`. Does **not** add a new UI-099 or top-level navigation product; extends UI-044..UI-047 and contextual entity-detail patterns within the frozen UI-001..UI-098 catalogue.

### Contextual Function / Serverless Service Detail

Accessible from Multi-Cloud/AWS/Azure/GCP Serverless lists and from alert/incident/graph drill-downs. Role/tenant/resource scope enforced server-side. Where applicable, default tabs/views include:

- Overview
- Invocations
- Errors
- Latency
- Concurrency
- Cold Starts
- Triggers
- Dependencies
- Versions/Revisions
- Logs/Events
- Changes
- Quotas
- Cost/Usage
- Alerts/Incidents
- RCA
- Telemetry Quality

**Data-quality behavior (`REQ-SLS-005`):** Distinguish `No Data`, `Stale`, `Monitoring Degraded`, connector-health, deleted/retired, disabled, zero-invocation and scale-to-zero from operational Function/service failure. Never convert missing or connector-failed telemetry into Function DOWN.

**RCA/Investigation:** Serverless entities participate in Investigation Workbench / canonical graph; provider status is supporting evidence; `INCONCLUSIVE` remains valid.

---

## Phase 2 Enterprise RFP Readiness Reconciliation — Issue #269

Phase-2 UI shall expose enterprise coverage truth instead of generic vendor badges:
- Compatibility & Certification Matrix: product/version, collection mode, dimensional support, limitations and certification state.
- Integration Health: connector freshness, auth/rate/throttle/backlog/degraded state.
- Regulated Deployment / Data Boundary view: data-egress mode, archive/retention/legal-hold and DR evidence.
- Enterprise Technology views for database, storage/SAN, middleware, backup/data protection and multi-cloud.
- External evidence is visibly source-labelled; external APM/NPM/DPI conclusions are not presented as native Platform truth.
- APM/DPI native-only controls are absent in Phase 2; UI must not imply bytecode instrumentation, RUM/session replay or continuous DPI availability.

---

## Issue #272 UI/UX roadmap

Phase 2 UI SHALL expose honest capability/certification state and customer-profile scope.

Phase 2.5 planning SHALL include role-aware deep-observability experiences for application/service transaction traces, service maps and trace-waterfall correlation, errors/exceptions, continuous profiles/flame graphs, browser/mobile experience and user journeys, packet-derived network/application response analysis, packet conversations/transactions and historical investigation, and evidence linkage from trace/profile/packet findings into Investigation/RCA.

Raw payload/session-replay visibility SHALL be privacy-, tenant- and RBAC-governed and must not be exposed by default.

## 16. System Workspace Manifests UI-001..UI-098

### 16.0 Scope, status and legend

`SPEC_CONTRACT=FROZEN` (effective on independent exact-head acceptance) · `IMPLEMENTATION=PENDING` for all 98 workspaces. This appendix freezes the **workspace-level manifest** for every `UI-001..UI-098` (§4.11 class and route are preserved). It binds each workspace to shared domain API families (DAF) and shared operation families (OP, defined in `Platform_17` §15). It creates no per-screen endpoint. Metric-level QMC entries remain in §4.8 (Presentation Query & Metric Catalogue); a panel with no authoritative QMC/DAF mapping is removed from the shipped default (§4.1). Source routes stay as pinned in `ui/src/platform/workspace-registry.ts` (all 98 `IMPLEMENTATION=PENDING` at the §4.12 checkpoint). Specification ≠ implementation ≠ certification.

**Phase:** `1` = first-GA Phase-1 envelope; `1.5` = Phase-1.5 (**NOT ACTIVE**); `†` = allocation inferred from the owning BUILD/pack target (`Platform_02` §58.1, Register) and subject to Master acceptance. **Class** uses only the §4.1 vocabulary. R1/R2 labels in §4.11 are design-proof sequencing, unchanged.

### 16.1 Shared manifest envelope (inherited by all 98)

| Field | Frozen value |
|---|---|
| Identity/version | `workspace_id=UI-nnn`, `manifest_version=1.0.0`, class per §4.11, `contentDefinitionRef` per §4.6 (dashboard classes → `dashboard-definition.schema.json`; others → class contract in §16.2). Customer clones are never overwritten by a newer System version (§4.2). |
| Authorization | Tenant from session. Every read/action is re-authorized server-side by permission + resource scope (+ entitlement). Stale client permission never authorizes. Denial/revocation hides protected payloads and disables actions (§3.2). `TEAM` scope is a reserved value until the Identity/Teams tranche (§4.7). |
| Filters / time / scope | Universal controls (§3.1): time range, display timezone, compare period, auto-refresh, scope filters (tenant/region/site/group/tag/entity), saved views. Serialized in the URL; **no credentials/tokens/sensitive evidence in URLs**. Context is revalidated on navigation and shared-link open. |
| States (all required) | Loading · Empty (authorized, no matching objects) · Partial · Stale (dated last-known) · Permission-Restricted · Not-Licensed · Query-Limited (explicit truncation) · Error (actionable, with retry/cancel) · Monitoring-Degraded · Normal. `Normal` ≠ `UP`. `No Data` ≠ `DOWN`. Unbound provider ≠ empty estate. A blank panel is never an error state. |
| Bounds | Server-side pagination (`PageResult` default 50, max 200 for existing paged domain APIs; other maxima owned by `Platform_11`/BUILD-130, never inferred); cursor/keyset for large estates (BUILD-067); row virtualization is separate from pagination; every query has timeout/row/scan/concurrency limits, cancellation and an explicit truncation flag; large exports are async. |
| Drill-down / context | Preserves tenant/site, selected object, time range, timezone, filters and incident/investigation context; a destination lacking its object shows a bounded selector (never guesses); Back restores list/tab/selection/focus. |
| Accessibility / keyboard | WCAG-aligned: all controls keyboard-operable; tabs expose selected state with arrow/Home/End; dialogs/drawers have named headings, trap focus while modal and return focus to the launcher; status uses icon + text (never colour alone); announcements are polite (no repeated interruption); units/timezone/required fields labelled; supports Light/Dark/System, Compact/Comfortable/Large density, text zoom, tablet/mobile summaries and wallboard where the class declares it. |
| Actions | Every enabled action declares target + revision, permission/entitlement, validation, reason/confirm/approval by risk, idempotency/conflict behavior, async status and cancel/recovery. Accepted ≠ completed; cancel-requested, partial, validation-failed and rollback-unsupported are never shown as success. Unsupported actions are absent or disabled with a reason. |
| Provenance | Each panel shows freshness, source/vantage and quality where the data contract provides them. Maintenance/suppression is an overlay and does not prove health. Operational AI output (when enabled) is labelled advisory. |

### 16.2 Class contracts (shared; per-ID deltas in §16.4)

| Class (§4.1) | Default interaction | Required panel archetypes | Class-specific rules |
|---|---|---|---|
| `EXECUTIVE_DASHBOARD` | summary tiles → trends | KPI/status tiles, trend, top-N risk, data-quality banner | Aggregates only; every tile shows scope, window and evidence quality; no unqualified "100%". Export via `OP-RPT-RUN`. |
| `NOC_DASHBOARD` | status grid + live list | health grid, active alerts/incidents, trend, top-N, freshness | Semantic states (UP/DOWN/DEGRADED/UNKNOWN/NO_DATA/STALE/MONITORING_DEGRADED/…) shown separately from severity and confidence. |
| `WALLBOARD` | full-screen read-only | large status tiles, incident ticker, map/summary | Read-only, no mutations; auto-refresh with staleness indicator; wallboard-safe contrast/size; auth session expiry shows a re-auth screen, not stale data. |
| `ANALYTICAL_DASHBOARD` | time-series/compare | time series, distribution, table, compare period, heatmap | Governed QMC queries only; downsampling resolution disclosed; threshold/value mappings are presentation-only (§4.3). |
| `OPERATIONAL_CONSOLE` | queue/table first | filterable queue, detail drawer, bulk action bar, saved views | Server-side sort/filter/paging; selection never implies all-pages; bulk returns per-item results. |
| `ENTITY_DETAIL` | tab/detail first | header (identity/state), tabs (Overview/Metrics/Topology/Alerts/Timeline/Config/Evidence), contextual action bar | Contextual action bar + Troubleshoot drawer (§3.4); entity context persists across tabs. |
| `INVENTORY_FLEET` | table + facets | virtualized table, facet filters, saved views, bulk actions | Server-side pagination/keyset; no client-side full-estate load; per-item bulk results. |
| `TOPOLOGY_MAP` | graph/map | graph/map canvas, legend, selection panel, as-of control | Node/edge cap with progressive disclosure; edge provenance/confidence/age; as-of history; declared ≠ observed. |
| `INVESTIGATION` | evidence/timeline/graph | timeline, evidence list, hypothesis/contradiction panels, notes | Pinned revision with "newer revision" notice; `INCONCLUSIVE` allowed; evidence references, not copies. |
| `CONFIGURATION` | form/editor + revisions | editor, validation, preview/impact, revision history | Preview before commit; `If-Match` revision; unsaved-change guard; field-level validation. |
| `GOVERNANCE` | review/audit tables | list, detail, decision/approval panel, evidence | Reason mandatory for decisions; immutable history; export bounded and audited. |
| `PLATFORM_OPERATIONS` | health + procedures | component health, queue/lag, capacity, operation list | Separates platform health from monitored-object health; destructive actions need reason/confirm/approval and show recovery. |
| `REPORT_ANALYSIS` | report builder/viewer | period selector, result/quality, evidence, run/schedule | Finalized periods immutable; PDF/CSV only; raw vs adjusted shown; restatement as linked revision. |

### 16.3 Shared Domain API Families (DAF) and permission/scope

All reads are `GET /api/v1/tenants/{tenantId}/…` through the Query Service (BUILD-067) or the owning domain API; `Platform_17` §15.3 gives DTO/limit detail. Read permission = `<perm>.read`; resource scope = the listed scope type.

| DAF | Domain read family | Perm / scope | Owner BUILD |
|---|---|---|---|
| DAF-INV | entities, sites, groups, tags, inventory query | `inventory` / site, group, entity | 066/067 |
| DAF-OBS | per-entity metric observations; QMC time series | `telemetry` / entity | 042/064/067 |
| DAF-HLT | aggregate health and semantic-state read model | `health` / site, group, service | 080 |
| DAF-ALR | alerts (current, history, evidence) | `alerts` / entity, site | 081/082 |
| DAF-INC | incidents, links, timeline | `incidents` / entity, site, service | 084/085 |
| DAF-RCA | RCA revisions, evidence, Unified Timeline | `rca` / incident | 085/100 |
| DAF-TOP | topology graph, as-of history, path overlay | `topology` / site, entity | 072 |
| DAF-NET | network protocol domains (routing, BGP, VRF/MPLS, VXLAN, WLAN, VoIP, QoS, VPN, IP SLA, SD-WAN) via Pack-normalized observations | `network` / site, entity | 041–045/049/071 |
| DAF-PTH | active path observations/changes | `network` / entity, path | 040 |
| DAF-FLW | flow/NTA queries | `flow` / site, exporter | 051–053/093 |
| DAF-LOG | logs/syslog/trap/event search | `logs` / source, site | 050/094 |
| DAF-INF | host, process/service, software, hardware/OOB | `infrastructure` / entity | 011/014/030–032/098 |
| DAF-VIRT | virtualization & HCI | `infrastructure` / cluster, host | 033 |
| DAF-STO | storage/SAN arrays, pools, fabrics | `infrastructure` / array, fabric | 034 |
| DAF-DB | database estate/detail | `databases` / instance | 035 |
| DAF-CLD | cloud accounts/resources, K8s/containers | `cloud` / account, cluster | 036 |
| DAF-SYN | synthetic tests/results | `synthetics` / test | 047 |
| DAF-CRT | certificates and shared-service dependency evidence (DNS/DHCP/NTP/AD/PKI) | `shared_services` / entity | 036/047/Platform_22 |
| DAF-NCM | config versions, compliance, change jobs | `ncm` / entity | 090 |
| DAF-IPAM | VRF/subnet/address/reservation/conflict | `ipam` / vrf, subnet | 092 |
| DAF-SLA | service assurance, SLA/OLA/provider SLA, error budget | `service_assurance` / service | 095 |
| DAF-CAP | capacity/forecast | `capacity` / site, entity | 064/067/093 |
| DAF-SEN | Sensor/Agent/pool estate, assignment, capacity | `sensors` / region, pool | 020/025/068 |
| DAF-QLT | telemetry quality, evidence coverage | `telemetry_quality` / source | 074 |
| DAF-PIPE | pipeline integrity, Kafka/DB/storage health | `platform_health` / component | 074/132 |
| DAF-INT | integration health, ITSM lifecycle, bindings | `integration` / connector | 086 |
| DAF-AUD | audit events and export | `audit` / tenant | 073 |
| DAF-IAM | access governance (roles, grants, sessions) | `access` / tenant | 060 |
| DAF-CHG | change/deployment/config events | `changes` / entity | 085/090 |
| DAF-DIAG | diagnostic jobs, live evidence, investigation, CLI history | `diagnostics` / entity | 012/015/091 |
| DAF-POL | alert policy, maintenance, suppression, effectiveness, false-alert evaluation | `policy` / scope | 070/083/101 |
| DAF-KNW | runbooks, PIR/problem, lifecycle/EOS, migration | `knowledge` / tenant | 097 |
| DAF-LIC | licence usage/entitlement | `licence` / tenant | 096 |
| DAF-STG | storage lifecycle/retention/housekeeping | `storage` / volume | 097/132 |
| DAF-NTF | notifications | `notifications` / user | 086 |
| DAF-HND | shift handover, My Work/Team Work | `operations` / team | 084/117 |

### 16.4 Per-workspace manifests (UI-001..UI-098)

Each row inherits §16.1 and its class from §16.2. **Panels** are the required panel set (delta over the class archetypes). **Reads** are DAF bindings. **Actions** are OP bindings (`—` = read-only/drill-only; `OP-RPT-RUN` = export). **Scope/filters** are additional filters beyond universal controls. All rows: `SPEC_CONTRACT=FROZEN · IMPLEMENTATION=PENDING`.

| UI | Workspace | Class | Ph | Panels | Reads | Actions | Scope / filters (delta) | Drill-down |
|---|---|---|---|---|---|---|---|---|
| UI-001 | Executive / CIO Overview | EXECUTIVE_DASHBOARD | 1 | service-health tiles; availability/SLA posture; incident trend; top risks; data-quality banner | DAF-HLT, DAF-SLA, DAF-INC, DAF-QLT | OP-RPT-RUN | business service, region; 7/30/90d | UI-005, UI-056, UI-071 |
| UI-002 | Global Operations Overview | NOC_DASHBOARD | 1 | domain health grid; active alerts/incidents; top-N sites; Sensor/pipeline status strip | DAF-HLT, DAF-ALR, DAF-INC, DAF-SEN | — | region, domain | UI-006, UI-055, UI-056 |
| UI-003 | NOC Command Center | WALLBOARD | 1 | critical incident ticker; health tiles; map summary; pipeline health | DAF-HLT, DAF-INC, DAF-TOP, DAF-PIPE | — (read-only) | saved wallboard profile | UI-056, UI-004 |
| UI-004 | Geographic Operations Map | TOPOLOGY_MAP | 1 | geo map; site status; link overlays; selection panel | DAF-TOP, DAF-HLT, DAF-INV | — | region/site; as-of | UI-006, UI-012 |
| UI-005 | Business Service Health | NOC_DASHBOARD | 1 | service tiles; dependency impact; SLA state; related incidents | DAF-HLT, DAF-TOP, DAF-SLA, DAF-INC | — | service, owner team | UI-057, UI-071 |
| UI-006 | Site Overview | NOC_DASHBOARD | 1 | site health; devices/links; alerts; Sensor/Agent presence; capacity | DAF-HLT, DAF-INV, DAF-ALR, DAF-SEN | — | site | UI-009, UI-010, UI-029 |
| UI-007 | Branch Experience | ANALYTICAL_DASHBOARD | 1 | branch experience score; WAN/app/synthetic trends; ranked branches | DAF-OBS, DAF-SYN, DAF-NET | OP-RPT-RUN | branch group; compare | UI-008, UI-006 |
| UI-008 | Branch Comparison | ANALYTICAL_DASHBOARD | 1 | side-by-side metrics; normalized comparison; delta table | DAF-OBS, DAF-NET, DAF-FLW | OP-RPT-RUN | 2..N branches; window | UI-007 |
| UI-009 | Network Health Overview | NOC_DASHBOARD | 1 | network semantic-state grid; top-N interfaces/devices; alerts; collector health | DAF-HLT, DAF-ALR, DAF-QLT, DAF-OBS | — | site, role, vendor | UI-010, UI-011, UI-055 |
| UI-010 | Network Device Detail | ENTITY_DETAIL | 1 | identity/state header; interfaces; metrics; neighbors/topology; alerts; config; Live Evidence; diagnostics | DAF-INV, DAF-OBS, DAF-TOP, DAF-ALR, DAF-NCM, DAF-DIAG | OP-DIAG-JOB-CREATE, OP-ALERT-ACK, OP-CLI-SESSION-CREATE | entity | UI-011, UI-063, UI-058 |
| UI-011 | Interface Detail | ENTITY_DETAIL | 1 | counters/utilization/errors; status history; neighbors; flow top talkers | DAF-OBS, DAF-TOP, DAF-FLW, DAF-ALR | OP-DIAG-JOB-CREATE | interface (canonical id) | UI-010, UI-012 |
| UI-012 | Critical Link | ENTITY_DETAIL | 1 | link health; utilization/loss/latency; path; incidents | DAF-OBS, DAF-PTH, DAF-TOP, DAF-INC | OP-DIAG-JOB-CREATE | link | UI-013, UI-011 |
| UI-013 | Link Monitor | ANALYTICAL_DASHBOARD | 1 | link table; utilization/error trend; top-N; compare | DAF-OBS, DAF-NET | OP-RPT-RUN | site, link group | UI-012 |
| UI-014 | Availability | ANALYTICAL_DASHBOARD | 1 | availability by scope; qualified downtime; UNKNOWN/STALE share; evidence quality | DAF-HLT, DAF-SLA, DAF-QLT | OP-RPT-RUN | scope, service time profile | UI-071, UI-076 |
| UI-015 | Network Performance | ANALYTICAL_DASHBOARD | 1 | latency/loss/jitter; CPU/mem; interface trend; heatmap | DAF-OBS, DAF-PTH, DAF-NET | OP-RPT-RUN | site, role | UI-010, UI-016 |
| UI-016 | Network Capacity | ANALYTICAL_DASHBOARD | 1 | utilization forecast; saturation list; growth trend | DAF-CAP, DAF-OBS | OP-RPT-RUN | site, link, horizon | UI-070, UI-012 |
| UI-017 | Network Flow / Traffic Analytics | ANALYTICAL_DASHBOARD | 1 | top talkers/apps/conversations; interface traffic; baseline; exporter health | DAF-FLW, DAF-QLT | OP-RPT-RUN | exporter, interface, app, direction; cardinality-bounded top-N | UI-018, UI-011 |
| UI-018 | Site-to-Site Traffic | ANALYTICAL_DASHBOARD | 1 | site-pair matrix; volume/trend; top apps | DAF-FLW, DAF-INV | OP-RPT-RUN | site pair, app | UI-017 |
| UI-019 | Routing Health | NOC_DASHBOARD | 1 | adjacency states; route churn; protocol health; alerts | DAF-NET, DAF-ALR, DAF-TOP | — | protocol, site | UI-020, UI-010 |
| UI-020 | BGP | ANALYTICAL_DASHBOARD | 1 | peer states; prefix counts; flap trend; session table | DAF-NET, DAF-ALR | — | peer, ASN, VRF | UI-019, UI-010 |
| UI-021 | MPLS / VRF | ANALYTICAL_DASHBOARD | 1.5† | VRF health; LSP/LDP state; per-VRF utilization | DAF-NET, DAF-FLW | — | VRF, PE | UI-010 |
| UI-022 | VXLAN / EVPN | ANALYTICAL_DASHBOARD | 1.5† | VTEP/VNI state; EVPN route health; fabric view | DAF-NET, DAF-TOP | — | fabric, VNI | UI-010 |
| UI-023 | SD-WAN Overview | NOC_DASHBOARD | 1.5 | normalized fabric/site/edge health; tunnel/path SLA class; policy bindings; connector health | DAF-NET, DAF-HLT, DAF-INT | — | controller, site, SLA class | UI-024, UI-007 |
| UI-024 | VPN | ANALYTICAL_DASHBOARD | 1 | tunnel state; throughput; flaps; peer table | DAF-NET, DAF-ALR | — | tunnel, peer, site | UI-010 |
| UI-025 | WLAN Overview | NOC_DASHBOARD | 1.5 | AP/controller health; client count; RF/airtime; SSID status | DAF-NET, DAF-HLT | — | controller, site, SSID | UI-010 |
| UI-026 | VoIP / CUCM | NOC_DASHBOARD | 1.5 | call-manager health; call quality; registration; trunk status | DAF-NET, DAF-SYN | — | cluster, site | UI-010 |
| UI-027 | IP SLA / Network Assurance | ANALYTICAL_DASHBOARD | 1 | operation results; latency/loss/jitter; threshold state; path context | DAF-NET, DAF-PTH, DAF-SYN | — | operation, source/target | UI-012 |
| UI-028 | QoS / CBQoS | ANALYTICAL_DASHBOARD | 1† | class drops/queues; policy maps; interface QoS trend | DAF-NET, DAF-OBS | — | policy, class, interface | UI-011 |
| UI-029 | Infrastructure Health Overview | NOC_DASHBOARD | 1 | host/VM/DB/cloud semantic-state grid; top pressure; alerts; Agent/collector health | DAF-HLT, DAF-INF, DAF-ALR, DAF-QLT | — | domain, site | UI-030, UI-031, UI-055 |
| UI-030 | Server Fleet | INVENTORY_FLEET | 1 | server table; OS/role facets; health; Agent state; criticality/profile | DAF-INV, DAF-INF, DAF-HLT | OP-ONB-CAND-DECIDE (link), OP-PROV-CREATE | OS, role, criticality, profile mode, Agent state | UI-031 |
| UI-031 | Server Detail | ENTITY_DETAIL | 1 | identity; CPU/mem/disk/NIC; processes/services; installed software; events; EDS/Live Evidence; criticality/profile | DAF-INF, DAF-OBS, DAF-DIAG, DAF-ALR | OP-DIAG-JOB-CREATE, OP-CLI-SESSION-CREATE, OP-ALERT-ACK | entity | UI-032, UI-058 |
| UI-032 | Process & Service | ENTITY_DETAIL | 1 | process/service list; selected monitors; state history; Stop Monitoring; software-change evidence | DAF-INF, DAF-ALR | OP-PROFILE-ASSIGN (monitor selection) | entity, selector type | UI-031 |
| UI-033 | Hardware / Out-of-Band | NOC_DASHBOARD | 1 | chassis/PSU/fan/temp/disk health; lifecycle events; OOB vantage freshness | DAF-INF, DAF-ALR | — | server, vendor | UI-031 |
| UI-034 | Virtualization Overview | NOC_DASHBOARD | 1 | cluster/host/VM health; contention; datastore pressure | DAF-VIRT, DAF-HLT | — | hypervisor, cluster | UI-035 |
| UI-035 | VMware | INVENTORY_FLEET | 1 | vCenter/ESXi/VM/datastore tables; contention; events | DAF-VIRT, DAF-INV | — | vCenter, cluster, host | UI-031, UI-034 |
| UI-036 | Nutanix | INVENTORY_FLEET | 1.5 | cluster/node/CVM/VM/storage tables | DAF-VIRT | — | cluster, node | UI-034 |
| UI-037 | Hyper-V | INVENTORY_FLEET | 1.5 | host/cluster/VM/storage/network tables | DAF-VIRT | — | host, cluster | UI-034 |
| UI-038 | Proxmox | INVENTORY_FLEET | 1.5 | cluster/node/VM/container/storage tables | DAF-VIRT | — | cluster, node | UI-034 |
| UI-039 | Storage Overview | NOC_DASHBOARD | 1.5 | array/pool/LUN health; capacity; latency; alerts | DAF-STO, DAF-HLT | — | array, pool | UI-040 |
| UI-040 | SAN Fabric | TOPOLOGY_MAP | 1.5 | fabric graph; port/ISL state; zoning evidence | DAF-STO, DAF-TOP | — | fabric, switch | UI-039 |
| UI-041 | Data Center Environmental | NOC_DASHBOARD | 1.5 | temperature/humidity/power evidence; thresholds; related incidents (facilities evidence only; not BMS/DCIM) | DAF-INF, DAF-ALR | — | room, rack | UI-033 |
| UI-042 | Database Estate Overview | INVENTORY_FLEET | 1 | instance table; engine facets; waits/capacity/health | DAF-DB, DAF-INV | — | engine, instance group | UI-043 |
| UI-043 | Database Detail | ENTITY_DETAIL | 1 | sessions/waits/capacity/replication; queries (bounded); alerts | DAF-DB, DAF-ALR | OP-DIAG-JOB-CREATE | instance | UI-042 |
| UI-044 | Multi-Cloud Overview | NOC_DASHBOARD | 1 | account/region health; resource counts; cost/cardinality guard; connector health | DAF-CLD, DAF-INT, DAF-HLT | — | provider, account | UI-045, UI-046, UI-047 |
| UI-045 | AWS Overview | NOC_DASHBOARD | 1 | account/region; compute/network/DB; connector health | DAF-CLD, DAF-INT | — | account, region | UI-044 |
| UI-046 | Azure Overview | NOC_DASHBOARD | 1 | subscription/region; compute/VNet/DB; connector health | DAF-CLD, DAF-INT | — | subscription, region | UI-044 |
| UI-047 | GCP Overview | NOC_DASHBOARD | 1.5 | project/region health; connector health | DAF-CLD, DAF-INT | — | project, region | UI-044 |
| UI-048 | Kubernetes Overview | NOC_DASHBOARD | 1 | cluster/node/workload health; restarts; pressure; connector health | DAF-CLD, DAF-HLT | — | cluster, namespace | UI-049 |
| UI-049 | Container Detail | ENTITY_DETAIL | 1 | pod/container state; resource use; events; owner workload | DAF-CLD, DAF-ALR | — | pod/container | UI-048 |
| UI-050 | Synthetic Monitoring Overview | NOC_DASHBOARD | 1 | test health; failing tests; latency trend; vantage status | DAF-SYN, DAF-HLT | — | test group, vantage | UI-051 |
| UI-051 | Synthetic Test Detail | ENTITY_DETAIL | 1 | result history; step timings; assertions; vantage comparison | DAF-SYN | OP-DIAG-JOB-CREATE | test | UI-050 |
| UI-052 | Certificate | INVENTORY_FLEET | 1 | certificate table; expiry buckets; chain/health; owner | DAF-CRT, DAF-INV | — | expiry window, issuer | UI-053 |
| UI-053 | DNS / DHCP / NTP | NOC_DASHBOARD | 1 | shared-service health; dependency impact; response/latency | DAF-CRT, DAF-SYN, DAF-TOP | — | service type, site | UI-057 |
| UI-054 | AD / Identity Infrastructure | NOC_DASHBOARD | 1 | DC/replication health; auth latency; dependency evidence | DAF-CRT, DAF-INF | — | domain, site | UI-053 |
| UI-055 | Alert Console | OPERATIONAL_CONSOLE | 1 | alert queue; state/ack/assignee; severity vs confidence; evidence quality; suppression overlay; detail drawer | DAF-ALR, DAF-POL | OP-ALERT-ACK, OP-ALERT-ASSIGN, OP-ALERT-NOTE, OP-ALERT-CLOSE, OP-ALERT-MUTE | state, severity, confidence, entity, policy, assignee, overlay | UI-057, UI-058, UI-010 |
| UI-056 | Incident Console | OPERATIONAL_CONSOLE | 1 | incident queue; state/owner; impact; RCA status; ITSM sync | DAF-INC, DAF-INT | OP-INC-ACK, OP-INC-ASSIGN, OP-INC-RESOLVE, OP-INC-REOPEN | state, priority, owner, service | UI-057 |
| UI-057 | Incident Detail | INVESTIGATION | 1 | summary; linked alerts; RCA revision; timeline; diagnostics; ITSM state | DAF-INC, DAF-RCA, DAF-DIAG, DAF-INT | OP-INC-*, OP-DIAG-JOB-CREATE, OP-INV-SESSION-CREATE | incident | UI-058 |
| UI-058 | RCA / Investigation Workspace | INVESTIGATION | 1 | pinned RCA revision; evidence (supporting/contradicting/missing); topology as-of; timeline; session evidence | DAF-RCA, DAF-TOP, DAF-DIAG, DAF-CHG | OP-RCA-FEEDBACK, OP-RCA-RERUN, OP-INV-SESSION-ATTACH, OP-DIAG-JOB-CREATE | incident, revision, as-of | UI-057, UI-061 |
| UI-059 | False Alert Qualification | GOVERNANCE | 1 | false-alert measurement; policy cohorts; backtest/shadow results; feedback queue | DAF-POL, DAF-ALR | OP-POLICY-LIFECYCLE (backtest/shadow only) | policy, cohort, window | UI-091 |
| UI-060 | Maintenance & Suppression | CONFIGURATION | 1 | window list/calendar; editor; impact preview; active overlay list | DAF-POL | OP-MNT-CREATE, OP-MNT-UPDATE, OP-MNT-CANCEL, OP-MNT-END-EARLY | scope, status, timezone | UI-055 |
| UI-061 | Change Timeline | ANALYTICAL_DASHBOARD | 1 | change/deployment/config events overlaid on health | DAF-CHG, DAF-HLT | — | entity, change type | UI-058, UI-065 |
| UI-062 | NCM Overview | OPERATIONAL_CONSOLE | 1 (+1.5 plan panels not active) | backup job queue; last-backup coverage; compliance summary; change jobs | DAF-NCM | OP-NCM-JOB-CREATE (backup), OP-NCM-JOB-CANCEL | job kind, state, vendor | UI-063, UI-064 |
| UI-063 | Device Configuration | CONFIGURATION | 1 | version list; diff; golden-state compare; backup now; CLI launch | DAF-NCM | OP-NCM-JOB-CREATE, OP-CLI-SESSION-CREATE | entity, version | UI-065 |
| UI-064 | NCM Compliance | GOVERNANCE | 1 | rule results; violations; remediation evidence (no automated push in Phase 1) | DAF-NCM | OP-NCM-COMPLIANCE-ACK | rule set, severity | UI-063 |
| UI-065 | Configuration Change | INVESTIGATION | 1 (+1.5 plan/rollback not active) | pre/post snapshots; diff; session evidence; approvals (1.5) | DAF-NCM, DAF-CHG | OP-NCM-CHANGE-CREATE, OP-NCM-ROLLBACK-CREATE | change job | UI-063 |
| UI-066 | IPAM Overview | INVENTORY_FLEET | 1 | VRF/subnet table; utilization; conflicts; reservations | DAF-IPAM | OP-IPAM-RESERVE | VRF, utilization band | UI-067, UI-068 |
| UI-067 | Subnet Detail | ENTITY_DETAIL | 1 | address map; observed vs declared; reservations; history | DAF-IPAM | OP-IPAM-RESERVE, OP-IPAM-RELEASE | subnet | UI-068 |
| UI-068 | IP Conflict | OPERATIONAL_CONSOLE | 1 | conflict queue; competing evidence; owners | DAF-IPAM | OP-IPAM-CONFLICT-ACTION | state, VRF | UI-067 |
| UI-069 | IPAM Capacity Forecast | ANALYTICAL_DASHBOARD | 1 | subnet exhaustion forecast; assumptions; confidence | DAF-IPAM, DAF-CAP | OP-RPT-RUN | subnet, horizon | UI-067 |
| UI-070 | Capacity Planning Overview | ANALYTICAL_DASHBOARD | 1 | forecast by domain; thresholds; assumptions and data quality | DAF-CAP | OP-RPT-RUN | domain, horizon | UI-016 |
| UI-071 | SLA | REPORT_ANALYSIS | 1 | policy/period; raw vs adjusted; evidence quality; error budget; restatement history | DAF-SLA | OP-RPT-RUN, OP-RPT-SCHEDULE, OP-SLA-DISPUTE | policy, period, service time | UI-014 |
| UI-072 | ISP / Carrier SLA | REPORT_ANALYSIS | 1† | provider SLA vs measured; underpinning evidence; circuit view | DAF-SLA, DAF-NET | OP-RPT-RUN, OP-RPT-SCHEDULE | provider, circuit | UI-071 |
| UI-073 | Sensor Estate Overview | INVENTORY_FLEET | 1 | Sensor table; role/region/pool; health; capacity; version | DAF-SEN | OP-SENSOR-REBALANCE-PREVIEW | region, pool, role, state | UI-074, UI-075 |
| UI-074 | Sensor Detail | ENTITY_DETAIL | 1 | identity/lease; work units; spool/ACK lag; versions | DAF-SEN, DAF-QLT | OP-SENSOR-DRAIN | sensor | UI-073 |
| UI-075 | Sensor Pool / HA | PLATFORM_OPERATIONS | 1 (HA panels 1.5, not active) | pool capacity; placement/preview; failure-domain; reassignment progress | DAF-SEN | OP-AGENT-REASSIGN, OP-SENSOR-REBALANCE-PREVIEW | region, pool | UI-073 |
| UI-076 | Telemetry Quality | PLATFORM_OPERATIONS | 1 | freshness/completeness; collector health; evidence coverage; drop counters | DAF-QLT | — | source, protocol | UI-092 |
| UI-077 | Agent Estate | INVENTORY_FLEET | 1 | Agent table; enrollment state; binding; assignment generation; version | DAF-SEN, DAF-INV | OP-ONB-CAND-DECIDE, OP-AGENT-REASSIGN, OP-AGENT-REVOKE | OS, state, Sensor | UI-031 |
| UI-078 | Kafka / Data Pipeline Health | PLATFORM_OPERATIONS | 1 | topic/consumer lag; ACK/finalize state; backlog | DAF-PIPE | — | stream, consumer | UI-092 |
| UI-079 | Core Database / Storage Health | PLATFORM_OPERATIONS | 1 | PostgreSQL/ClickHouse health; storage vs 70%/80%; replication state | DAF-PIPE, DAF-STG | — | component, volume | UI-093 |
| UI-080 | Integration Health | PLATFORM_OPERATIONS | 1 | connector health; queue depth/age; DEAD items; rate-limit | DAF-INT | OP-ITSM-RECONCILE, OP-ITSM-REPLAY | connector | UI-081 |
| UI-081 | ITSM Lifecycle | OPERATIONAL_CONSOLE | 1 | per-incident sync state; bindings; conflicts | DAF-INT, DAF-INC | OP-ITSM-RECONCILE, OP-ITSM-REPLAY | connector, state | UI-056 |
| UI-082 | Audit Overview | GOVERNANCE | 1 | audit volume/trend; privileged actions; export status | DAF-AUD | — | actor, action class | UI-083 |
| UI-083 | Audit Explorer | GOVERNANCE | 1 | audit search; event detail; export | DAF-AUD | OP-AUDIT-EXPORT | actor, action, resource, window | UI-082 |
| UI-084 | Access Governance | GOVERNANCE | 1 | roles/grants; effective access; sessions; recertification | DAF-IAM | OP-ACCESS-REQUEST-DECIDE | principal, role, scope | UI-083 |
| UI-085 | CLI Session History | GOVERNANCE | 1 | session list; reason/ticket; recording reference; commands per policy | DAF-DIAG, DAF-AUD | OP-CLI-SESSION-TERMINATE (active only) | user, target, state | UI-083 |
| UI-086 | Technology Lifecycle Risk | GOVERNANCE | 1.5† | EOS/EOL inventory risk; remediation plan | DAF-KNW, DAF-INV | — | vendor, EOL window | UI-089 |
| UI-087 | Desired vs Actual / Configuration Drift | GOVERNANCE | 1 | desired vs observed state; drift events; resolution | DAF-SEN, DAF-NCM | — | scope, drift type | UI-063 |
| UI-088 | Shift Handover | OPERATIONAL_CONSOLE | 1 | open items; handover notes; ownership; unresolved evidence | DAF-HND, DAF-INC, DAF-ALR | OP-HANDOVER-CREATE | team, shift | UI-056 |
| UI-089 | Migration Studio | CONFIGURATION | 1.5† | import mapping; validation; staged apply | DAF-KNW, DAF-INV | OP-MIGRATION-IMPORT | source, mapping | UI-066 |
| UI-090 | Intelligence Quality | ANALYTICAL_DASHBOARD | 1 | RCA accuracy/INCONCLUSIVE rate; confidence calibration; model/data-drift | DAF-POL, DAF-RCA | — | model, window | UI-091 |
| UI-091 | Alert Policy Effectiveness | ANALYTICAL_DASHBOARD | 1 | false-alert/noise by policy; backtest/shadow; recommendation queue | DAF-POL | OP-POLICY-LIFECYCLE | policy, window | UI-059 |
| UI-092 | Data Pipeline Integrity | PLATFORM_OPERATIONS | 1 | produced/accepted/normalized/inserted reconciliation per interval/stream | DAF-PIPE, DAF-QLT | — | stream, interval | UI-078 |
| UI-093 | Storage & Retention | PLATFORM_OPERATIONS | 1 | volume/data-class measurements; headroom/shortfall; retention preview; protected data | DAF-STG | OP-STO-RETENTION-PREVIEW, OP-STO-RETENTION-ACTIVATE, OP-STO-ACTION | volume, data class | UI-079 |
| UI-094 | Usage & Licensing | GOVERNANCE | 1 | licensed vs used; state (T-7/EXPIRED_GRACE_15D); forecast | DAF-LIC | OP-LIC-LOAD, OP-LIC-EXTEND | module | UI-093 |
| UI-095 | Housekeeping & Lifecycle Jobs | PLATFORM_OPERATIONS | 1 | job list; backup/restore/upgrade operations; verification | DAF-STG, DAF-PIPE | OP-BKP-RUN, OP-BKP-RESTORE, OP-UPG-RUN, OP-UPG-ROLLBACK | operation kind, state | UI-093 |
| UI-096 | Post-Incident Review | INVESTIGATION | 1.5† | incident evidence; timeline; findings; actions | DAF-KNW, DAF-INC, DAF-RCA | OP-PIR-CREATE | incident | UI-057 |
| UI-097 | Runbook Library | GOVERNANCE | 1.5† | runbook list/versions; applicability; usage | DAF-KNW | OP-RUNBOOK-UPSERT | domain, version | UI-058 |
| UI-098 | Device / Time Comparison | ANALYTICAL_DASHBOARD | 1 | side-by-side devices/time windows; delta; evidence | DAF-OBS, DAF-NCM, DAF-CHG | — | 2..N entities; two windows | UI-010 |

### 16.5 Coverage assertion and closure

All 98 identifiers `UI-001..UI-098` are listed exactly once in §16.4 with the §4.11 class unchanged. No workspace introduces a unique endpoint: every read binds to a shared DAF and every mutation binds to a `Platform_17` §15 operation family. Not-Licensed, Permission-Restricted, Query-Limited and Monitoring-Degraded behavior comes from §16.1 and applies to every row. A `1.5` row renders `Not-Licensed`/unavailable (not a fake panel) until its owning BUILD is certified. UI implementation maturity remains `IMPLEMENTATION=PENDING` per §4.12. Domain UI presence never certifies a backend (BUILD-116).
