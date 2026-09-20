# Platform — UI/UX, Navigation & Out-of-Box Dashboard Master

**Document ID:** PLATFORM-UIUX-001
**Version:** 3.2.2
**Status:** **Authoritative Mother Reference — UI/UX and Dashboard Specification**
**Product:** Platform
**Last reconciliation:** 2026-09-19 — v3.2.2 NOC troubleshooting integration-audit closure

## 1. Authority and no-assumption rule

This document expands the PRD's UI/UX and dashboard catalogue into an implementation reference. It does not invent customer operational thresholds or polling intervals. Values such as thresholds, refresh periods, SLA targets and retention remain policy-driven. Where this document defines a default UI layout/control, it is an **architecture-selected default** intended to make the product usable out of box and remains configurable where the security model permits.

The non-negotiable UX principles are: fast navigation; readable fonts; role-aware disclosure; UI-only normal administration; no silent failures; contextual troubleshooting; consistent filters/actions; light/dark theme; large-environment server-side data handling; and HTTPS/TLS-only access.

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

- Tenant/customer selector only for users authorized for multiple tenants.
- Global search across device/IP/MAC/serial/site/tag/interface/circuit/incident/alert/ticket/IPAM/NCM/DB/VM/cloud/K8s entities.
- Time range, timezone, comparison period and auto-refresh.
- Quick actions based on permission: Add Device, Create Dashboard, Add Maintenance, Run Investigation, Create Report, Open CLI.
- Platform notifications: Sensor, integration, report, licence, HA, approval and security conditions. The global per-user Notification Center/bell is governed by `Platform_20_Discovery_Onboarding_Identity_and_Notification_Master.md`; this base catalogue is not the detailed owner.
- Context help and user menu: profile, preferences, sessions, API tokens, theme, timezone, language and logout.

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

### 3.3 Accessibility/readability

Support Light/Dark/System theme; Compact/Comfortable/Large density; keyboard navigation; scalable typography; accessible contrast; status icon/text in addition to color; and NOC TV/full-screen layouts. Full admin/studios are desktop-first; mobile/tablet supports dashboards, alerts, incidents, acknowledgement and investigation summaries.

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

## 4. Dashboard Studio

Dashboard library scopes: My, Team, Shared, Tenant/Customer, System and folders/groups. Actions: create, clone, move, rename, share, import/export, permissions, delete, variables and reusable panels. Editor supports drag/resize/duplicate/copy panels, undo/redo and preview. Panel editor tabs: Data, Query, Transformation, Visualization, Thresholds, Links, Overrides, Permissions. Panel types: stat, gauge, time series, bar, pie, table, heatmap, histogram, status grid, Sankey, topology, geo, text, alert list, incident list, RCA and domain-specific visualizations.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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
- Create maintenance
- View processes/services
- View RCA

**Drill-down contract:** Entity/metric/alert/incident clicks retain tenant, site, time range and relevant filters; drill-down can open entity detail, topology, timeline, RCA, NCM/CLI, IPAM, Flow, ITSM or audit context where the relationship exists.

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Data-quality behavior:** Show telemetry freshness/quality and distinguish `No Data`, `Stale`, `Monitoring Degraded`, `Suppressed/Maintenance` and real operational failure. Never convert missing telemetry directly into a device-down assertion.

**Empty/error behavior:** Empty state explains why no entities match and offers filter reset/onboarding action where authorized. Errors show an actionable reason, error/correlation ID and retry/diagnostic path; partial data identifies the affected source/Sensor.

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

---

### UI-044 — Multi-Cloud Overview

**Domain:** Cloud/Containers  
**Primary roles:** Cloud ops  
**Purpose:** AWS/Azure/GCP/OCI organizations/accounts/subscriptions/projects/compartments, regions, services, compute, storage, databases, network, Kubernetes/serverless, SLOs, changes, quotas, observability coverage, connector health and capacity.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Providers
- Organizations / Accounts
- Regions
- Services
- Compute
- Network
- Storage
- Databases
- Kubernetes
- Serverless
- SLOs
- Changes
- Quotas
- Coverage
- Connector Health

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
- Account/Subscription/Project/Compartment
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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

---

### Cloud & Full-Stack Observability UI supplement — Platform_26

The Cloud/Containers workspace shall expose the Platform_26 P0 model without forcing users into separate products:

- **Cloud Integration Administration:** Administration → Integrations → Cloud lists AWS/Azure/GCP/OCI connections with provider scope, auth mode, Sensor pool, selected Cloud Service Packs, state, last verification/reconciliation, API-budget/throttle health and coverage. **Add Cloud Environment** runs the governed provider → scope → authentication → permission preflight → hierarchy discovery → service-pack selection → collection plan → Sensor placement → rate/capacity preview → ownership/policy mapping → ephemeral-resource policy → discovery preview → activate → verify workflow.
- **Cloud Provider Workspace:** every day-one provider root (AWS, Azure, GCP, OCI) exposes hierarchy, regions, service-family health, incidents, SLO impact, quota risk, connector health, changes and coverage gaps. Existing UI-045/046/047 are provider presets; OCI is mandatory through the same provider-workspace contract even though the legacy base catalogue has no dedicated numeric slot.
- **Service Overview:** request rate/error/latency, SLO/error budget, traces, logs, metrics, dependencies, runtime instances, cloud resources, network path, deployments/changes, incidents and RCA.
- **Trace Explorer:** search/filter by service/environment/version/operation/status/duration/cloud/Kubernetes attributes with trace waterfall/span tree, related logs/metrics/events, canonical resource identity, changes and service-graph pivots.
- **Serverless:** functions/services, versions/revisions, invocation/request rate, latency, errors, throttles, concurrency, cold-start indicators where exposed, triggers/dependencies, logs, traces, changes, quotas and SLO.
- **Cloud Network:** provider network graph, routes, load balancer/target relationships, NAT/transit/private connectivity, flow analytics, DNS, synthetics and on-prem path correlation.
- **Observability Coverage:** signal availability/freshness, ownership, SLO, connector/credential state, unsupported signals, blind spots and the evidence gaps limiting RCA confidence.
- **P1 surfaces:** RUM, Mobile Experience, Profiling, Cloud Cost/FinOps, customer AI/LLM Observability and GPU/Accelerator observability use the same global service/resource identity and navigation rather than independent products.

Cloud integration/status UI uses at least `CONNECTED`, `PARTIAL`, `AUTHORIZATION_DEGRADED`, `THROTTLED`, `STALE`, `RECONCILING`, `MISCONFIGURED`, `DISCONNECTED` and `SUSPENDED`. Missing/partial provider telemetry never becomes a generic resource-down state.

---

### UI-045 — AWS Overview

**Domain:** Cloud/Containers  
**Primary roles:** AWS ops  
**Purpose:** AWS organization/account/region and P0 Cloud Service Pack operations across compute, network, storage, database, containers, serverless, messaging/streaming, edge/DNS, CloudWatch/CloudTrail/Health, quotas, changes, coverage and alerts.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Accounts / Regions
- Compute
- Network
- Storage
- Databases
- Containers
- Serverless
- Messaging / Streaming
- Edge / DNS
- Changes / Health
- Quotas
- Coverage
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
- Account/Subscription/Project/Compartment
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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

---

### UI-046 — Azure Overview

**Domain:** Cloud/Containers  
**Primary roles:** Azure ops  
**Purpose:** Azure tenant/management-group/subscription and P0 Cloud Service Pack operations across compute, network, storage, databases, AKS/containers, Functions/App Service, messaging, edge/DNS, Azure Monitor/Activity/Service Health, quotas, changes, coverage and alerts.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Management Groups / Subscriptions
- Compute
- Network
- Storage
- Databases
- AKS / Containers
- Functions / App Services
- Messaging / Events
- Edge / DNS
- Changes / Health
- Quotas
- Coverage
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
- Account/Subscription/Project/Compartment
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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

---

### UI-047 — GCP Overview

**Domain:** Cloud/Containers  
**Primary roles:** GCP ops  
**Purpose:** GCP organization/folder/project and P0 Cloud Service Pack operations across compute, network, storage, databases, GKE, Cloud Run/Functions, Pub/Sub/data services, edge/DNS, Monitoring/Logging/Audit, quotas, changes, coverage and alerts.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Organizations / Projects
- Compute
- Network
- Storage
- Databases
- GKE / Containers
- Cloud Run / Functions
- Messaging / Data Services
- Edge / DNS
- Changes / Health
- Quotas
- Coverage
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
- Account/Subscription/Project/Compartment
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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

---

### UI-048 — Kubernetes Overview

**Domain:** Cloud/Containers  
**Primary roles:** Platform/K8s ops  
**Purpose:** Cluster/node/namespace/controller/workload/pod/container/service/ingress/storage/autoscaling health, scheduling and rollout state, events, SLOs, changes, coverage and capacity.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Nodes
- Namespaces
- Controllers
- Workloads
- Pods
- Containers
- Services
- Ingress
- Storage
- Autoscaling
- Events
- Changes
- SLO
- Coverage
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
- Account/Subscription/Project/Compartment
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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

---

### UI-071 — SLA

**Domain:** Capacity/SLA  
**Primary roles:** Management/NOC  
**Purpose:** SLA by service/region/provider/site, violations, downtime and maintenance exclusions.

**Entry/navigation:** Accessible from the relevant domain dashboard/navigation and from contextual entity/alert/incident drill-downs. Role/tenant/resource scope is enforced server-side.

**Default tabs/views:**
- Overview
- Services
- Sites
- Providers
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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

---

### UI-072 — ISP / Carrier SLA

**Domain:** Capacity/SLA  
**Primary roles:** WAN/vendor mgmt  
**Purpose:** Provider/circuit availability, latency, loss, jitter, target/actual/breach.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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

**Performance:** Default query uses the appropriate ClickHouse rollup resolution for the selected time range, applies bounded row/scan limits and uses server-side pagination/virtualization for large lists.

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
