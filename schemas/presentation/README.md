# Platform Dashboard & Report Definition Contracts

**Contract family:** Presentation definitions  
**Schema version:** `1.0.0`  
**Authority:** Platform v3.2.2 Mother Reference  
**Checkpoint:** static contracts, metadata, deterministic validation and fictional fixtures only

## Purpose

This directory defines the Platform-owned static contract foundation for future dashboard, panel, variable/filter, saved-view and report definitions. It is intentionally execution-neutral: it describes what a presentation definition references and how it preserves context; it does not execute queries, render React, calculate SLA, schedule jobs, persist state, send email or generate PDF/CSV/XLSX/JSON output.

The authoritative product requirements remain the Mother Reference. These files are an implementation contract derived from those requirements, not a replacement for them.

## Files

- `dashboard-definition.schema.json` — versioned dashboard definition envelope.
- `report-definition.schema.json` — versioned report definition envelope.
- `saved-view.schema.json` — saved view/filter-state metadata.
- `tools/validate.mjs` — dependency-free semantic validator; zero network calls.
- `examples/` — small fictional definitions only; these are not the mandatory 98 screens.
- `fixtures/cases.json` — deterministic valid/invalid cases consumed by the validator.

## Dashboard definition boundary

A dashboard definition can describe:

- stable definition identity and version;
- display metadata, folder/category/tags;
- `SYSTEM`, `CUSTOMER_CREATED` or `CLONE` origin;
- Dashboard Studio scope (`MY`, `TEAM`, `SHARED`, `TENANT_CUSTOMER`, `SYSTEM`);
- recoverable System-definition semantics and clone provenance;
- presentation-only role/resource/permission/entitlement references;
- runtime context capabilities for time, timezone, comparison, auto-refresh and scope dimensions;
- local governed-query bindings through opaque `queryRef` identifiers;
- variables and dashboard filters without executable query text;
- tabs/views, grid layout and panels;
- contextual action references and drill-down metadata;
- the ten Mother-Reference UI/data states;
- server-side pagination/filter/sort and virtualization intent;
- optional Platform Value presentation safeguards.

A definition **does not grant access**. Server-side tenant/resource authorization remains authoritative. Entitlement is separate from authorization. `permissionRefs`, `roleRefs`, `resourceScopeRef` and `entitlementRefs` are presentation references only and may never be interpreted as proof that a request is authorized.

## System / clone semantics

System definitions are stable and recoverable product defaults. The validator enforces:

- `origin = SYSTEM` requires `scope = SYSTEM`, `systemDefinition = true`, `recoverableSystemDefinition = true`, and no derivation source;
- customer-created definitions cannot mark themselves as System or recoverable System definitions;
- `origin = CLONE` requires explicit `derivedFrom` identity/version and a different definition ID;
- clones cannot overwrite or masquerade as the authoritative source definition.

No publishing/approval lifecycle is introduced here because the Mother Reference does not freeze one.

## Panel families

Only Mother-Reference panel families are admitted:

`STAT`, `GAUGE`, `TIME_SERIES`, `BAR`, `PIE`, `TABLE`, `HEATMAP`, `HISTOGRAM`, `STATUS_GRID`, `SANKEY`, `TOPOLOGY`, `GEO`, `TEXT`, `ALERT_LIST`, `INCIDENT_LIST`, `RCA`, `DOMAIN_SPECIFIC`.

Panels carry declarative metadata and references only. Rendering is outside this checkpoint.

## Governed query/data boundary

This contract does not define SQL, a query DSL, database access or arbitrary expressions.

A definition may register local query bindings:

```text
queryId -> governed opaque queryRef + declarative parameter names
```

Panels/report sections reference the local `queryId`. Parameter bindings may draw only from declared variables, filters or runtime context keys. Values do not become executable query text. The future Query Service remains responsible for typed query contracts, server-side authorization, pagination, resource limits, rollup selection and safe execution.

Forbidden definition content includes raw SQL, shell, JavaScript, Python, executable expressions, database credentials, datasource secrets and unrestricted executable URLs.

The reporting LLD allows a safe expression engine such as CEL for future formulas, but this checkpoint intentionally **does not embed formula/expression text**. Formula behavior is referenced through opaque governed identifiers until its separate executable contract is defined.

## Variables, filters and runtime context

Variables are declarative selectors with stable IDs, labels, governed source references, optional opaque default selections, multi-select/required flags, URL-state persistence intent and optional dependency on another variable.

Filters similarly use stable IDs and governed source/operator references. They may bind to a variable or a Mother-Reference runtime context key. Saved views may persist opaque selection references; persisted state never grants access to the referenced data.

Supported runtime context keys are:

- `TIME_RANGE`
- `TIMEZONE`
- `COMPARISON_PERIOD`
- `AUTO_REFRESH`
- `TENANT_CUSTOMER`
- `REGION`
- `SITE`
- `GROUP_SERVICE`
- `ENTITY`
- `TAGS`
- `TECHNOLOGY_DOMAIN`
- `BUSINESS_SERVICE`
- `INCIDENT`
- `SLA`

Definitions describe which contexts they accept. They do not hard-code customer-specific runtime values, refresh periods, SLA targets, polling intervals, thresholds or retention values.

## Drill-downs

A drill-down is a declarative destination plus bounded target reference and explicit context-preservation intent. Supported destination classes are:

- `ENTITY_DETAIL`
- `DASHBOARD`
- `TOPOLOGY`
- `TIMELINE`
- `RCA`
- `INCIDENT`
- `ALERT`
- `NCM_CLI`
- `IPAM`
- `FLOW`
- `ITSM`
- `AUDIT`
- `REPORT`

No JavaScript URLs, arbitrary external URLs, credentials or routing implementation are present in this contract.

## Required UI/data states

Every dashboard definition must declare presentation handling for exactly these ten Mother-Reference states:

1. `LOADING`
2. `EMPTY`
3. `PARTIAL_DATA`
4. `STALE_DATA`
5. `PERMISSION_RESTRICTED`
6. `NOT_LICENSED`
7. `QUERY_LIMITED`
8. `ERROR`
9. `MONITORING_DEGRADED`
10. `NORMAL`

Each state requires a bounded `presentationRef`; a blank panel is not an error strategy. Definitions may request freshness/provenance/affected-source/correlation-ID display when runtime supplies those values.

Missing, stale or partial telemetry is presentation evidence only. A dashboard definition cannot conclude `DEVICE_DOWN` or another causal operational failure from missing data.

## Saved views

Saved views contain presentation state only: target dashboard/workspace reference, owner/scope references, filter selections, time-context policy reference, sort/group/view-mode references and provenance.

Saved views do not carry authorization grants, entitlement decisions, credentials, tokens, cookies or executable query text. Runtime authorization is re-evaluated whenever the saved view is used.

## Report definitions

A report definition can describe:

- stable identity/version/origin/scope/provenance;
- presentation-only role/resource/permission/entitlement references;
- governed query bindings, variables and filters;
- report sections for `TEXT`, `TABLE`, `VISUALIZATION` and governed `IMAGE_REFERENCE` content;
- SLA/reporting policy references;
- allowed output formats required by the PRD: `PDF`, `CSV`, `XLSX`, `JSON`, `API`;
- schedule intent through an opaque `scheduleRef` plus explicit timezone;
- delivery intent through governed target references for `DOWNLOAD`, `EMAIL` or `INTEGRATION`.

The contract does not execute schedules, contain recipients, store notification credentials, render documents or send outputs.

## Scale/performance intent

Definitions can require:

- server-side pagination;
- server-side filtering;
- server-side sorting;
- browser virtualization;
- bounded-result/query-policy references.

No row limit, timeout, refresh interval or query threshold is invented here. Large-environment limits remain governed by future Query Service policy and benchmark evidence.

## Platform Value / OIV readiness

The metadata model can represent future Platform Value, MTTR, noise-reduction, diagnosis-effectiveness, incident-improvement and operational/economic-benefit views without embedding customer claims.

Optional `valuePresentationPolicy` metadata can require methodology, sample-size and assumption disclosure and prohibit unsupported causal claims. Financial presentation requires customer-approved assumption references. Example definitions contain no savings, MTTR-reduction percentage or financial amount.

The contract preserves Platform_19's rule that capability participation is not proof of causal benefit.

## Security and privacy

The schemas are strict and the semantic validator fails closed for secret-bearing or executable content, including:

- passwords/password hashes;
- tokens/bearer tokens/API keys;
- cookies/session material;
- private keys/client secrets/database credentials;
- credentials embedded in URLs;
- raw SQL or SQL-like query text;
- JavaScript/Python/shell/command/executable fields;
- active/raw HTML and dangerous URL schemes;
- path traversal and absolute filesystem paths;
- unknown fields;
- unbounded strings/collections/documents;
- undefined/cyclic references;
- clone/System provenance corruption;
- authorization-grant semantics embedded in presentation metadata.

Text fields are metadata/plain text only. There is no raw-HTML escape hatch. Any future Markdown renderer must sanitize independently.

## Defensive bounds

The schemas and validator use conservative size/count limits solely for parser/input-safety and denial-of-service resistance. They are **not** product operational thresholds, dashboard row limits, refresh defaults, SLA targets, licensing limits or customer policy defaults.

The validator also bounds directory traversal, document count and input file size and follows no symlinks.

## Versioning

`schemaVersion` is currently `1.0.0`. The validator rejects unsupported schema versions rather than silently reinterpreting them.

`definitionVersion` remains a bounded opaque version identifier owned by definition persistence/versioning; this contract does not impose SemVer on customer definitions.

Breaking contract changes require a new incompatible schema version and an explicit compatibility/migration path before runtime adoption. No migration service is implemented here.

## What this checkpoint does not implement

- Dashboard Studio or Report Studio UI;
- the 98 mandatory screen definitions;
- React components/panel rendering;
- query execution or database access;
- SQL/query DSL/formula execution;
- backend APIs/persistence;
- report scheduler/workers/history;
- SLA calculation;
- PDF/CSV/XLSX/JSON generation;
- email/integration delivery;
- role/RBAC/resource-scope authorization;
- entitlement decisions;
- customer-specific dashboards, values, thresholds or targets.

The later integration sequence is: independently validate/merge this contract, independently validate/merge the UI foundation, then add governed Core/Query/Reporting contracts and instantiate the mandatory screen definitions against those merged authorities.

## Local Docker validation (later)

From repository root:

```bash
docker run --rm \
  -v "$PWD:/repo:ro" \
  -w /repo \
  node:20-alpine \
  node schemas/presentation/tools/validate.mjs
```

This README does not claim that command has executed in this GitHub-only checkpoint.
