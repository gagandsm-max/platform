# Platform Dashboard & Report Definition Contracts

**Contract family:** Presentation definitions
**Schema version:** `1.0.0`
**Authority:** Platform v3.2.2 Mother Reference
**Checkpoint:** static contracts, metadata, deterministic validation and fictional fixtures only

## Purpose

This directory defines the Platform-owned static contract foundation for future dashboard, panel, variable/filter, saved-view and report definitions. It is execution-neutral: definitions describe presentation intent and governed references; they do not execute queries, render React, calculate SLA, schedule jobs, persist state, send email or generate report files.

The Mother Reference remains authoritative. These schemas are an implementation contract derived from it, not a substitute for it.

## Files

- `dashboard-definition.schema.json` — versioned dashboard definition envelope.
- `report-definition.schema.json` — versioned report definition envelope.
- `saved-view.schema.json` — saved-view/filter-state metadata.
- `tools/validate.mjs` — dependency-free semantic validator; zero network calls.
- `examples/` — fictional examples only; none is a mandatory shipped screen.
- `fixtures/cases.json` — deterministic valid/invalid cases.

## Dashboard definition

A dashboard definition can represent:

- stable definition identity/version, name and description;
- Dashboard Studio origin/scope and recoverable System identity;
- clone/derivation provenance;
- folder/category/tags;
- presentation-only role/resource/permission/entitlement references;
- time/timezone/comparison/auto-refresh and operational scope context capabilities;
- governed query references with opaque parameter/grouping/aggregation/time-binding references;
- variables, reusable-filter references and dashboard filters;
- tabs/views and resizable grid layout;
- panels, table/list column references, actions and drill-downs;
- required UI/data states plus freshness/provenance/affected-source/correlation-ID presentation intent;
- maintenance/suppression presentation references where runtime supplies those contexts;
- server-side pagination/filter/sort, virtualization and bounded query-policy references;
- optional Platform Value disclosure guardrails.

It does not implement Dashboard Studio or any of the 98 mandatory out-of-box screens.

## Origin and mutability

Origins are deliberately small:

- `SYSTEM` — stable, authoritative and recoverable product default;
- `CUSTOMER_CREATED` — customer-owned definition that cannot masquerade as System;
- `CLONE` — derived definition requiring explicit source definition ID/version.

`SYSTEM` requires `scope=SYSTEM`, `systemDefinition=true` and `recoverableSystemDefinition=true`. Customer-created/clone definitions cannot set those authoritative markers or overwrite source identity. No publishing/approval lifecycle is invented because the Mother Reference does not freeze one.

## Authorization and entitlement boundary

`roleRefs`, `resourceScopeRef`, `permissionRefs` and `entitlementRefs` are **presentation references only**.

They do not grant access. `serverAuthorizationRequired` is fixed to `true`. Runtime Core/Query/Reporting services must re-evaluate tenant/resource authorization and entitlement independently. Entitlement never implies authorization and authorization never implies entitlement.

Saved views likewise never grant access merely because a prior authorized user stored state.

## Panel contract

Only Mother-Reference panel families are admitted:

`STAT`, `GAUGE`, `TIME_SERIES`, `BAR`, `PIE`, `TABLE`, `HEATMAP`, `HISTOGRAM`, `STATUS_GRID`, `SANKEY`, `TOPOLOGY`, `GEO`, `TEXT`, `ALERT_LIST`, `INCIDENT_LIST`, `RCA`, `DOMAIN_SPECIFIC`.

Panel metadata supports ID/title/description/type, bounded grid layout, governed query uses, transformation/visualization/threshold/field-override references, filter/variable/action references, drill-downs, state-policy references and optional presentation visibility references. `columnRefs` can describe future table/list columns without embedding data.

`TEXT` panels carry bounded text/Markdown-source metadata only; there is no raw-HTML escape hatch. Non-text panels require at least one governed query use so an undefined blank-data panel does not silently become a valid definition.

## Governed query/data boundary

This contract does not define SQL, a query DSL, database access or executable formulas.

Definitions register local query bindings:

```text
queryId
  -> governed queryRef
  -> dataSourceClassRef
  -> parameterRefs
  -> groupingRef? / aggregationRef? / timeContextBindingRef?
```

Panels/report sections reference the local `queryId`. Parameter bindings may source values only from declared variables, filters or runtime context keys. Runtime values never become executable query text through this contract.

The future Query Service remains responsible for typed query contracts, server-side authorization, pagination, query limits, rollup selection and safe execution.

Reporting formulas are represented only by opaque `calculationRefs`. The LLD permits a future safe expression engine such as CEL, but executable expression syntax is intentionally outside this checkpoint.

Forbidden content includes raw SQL, shell, JavaScript, Python, formula/expression text, database credentials, datasource secrets and unrestricted URLs.

## Variables, filters and runtime context

Variables support stable ID/label, governed allowed-value source reference, opaque default-selection references, multi-select, required/optional, URL-state persistence intent and optional dependency on another variable. Variable dependency cycles are invalid.

Filters support stable ID/label, governed source/operator references, variable or runtime-context binding and reusable-filter references. A filter cannot ambiguously bind both a variable and a runtime context.

Runtime context keys are:

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

Definitions state which contexts they accept. They do not hard-code customer-specific values, refresh periods, SLA targets, operational thresholds, polling intervals or retention values.

## Drill-downs

Supported declarative destination classes are:

`ENTITY_DETAIL`, `DASHBOARD`, `TOPOLOGY`, `TIMELINE`, `RCA`, `INCIDENT`, `ALERT`, `NCM_CLI`, `IPAM`, `FLOW`, `ITSM`, `AUDIT`, `REPORT`.

Each drill-down carries a bounded target reference and explicit intent to preserve tenant, scope, time and filters. Routing is not implemented. Arbitrary URL fields, JavaScript URLs and embedded credentials are prohibited.

## Required UI/data states

Dashboard state handling must contain exactly the ten Mother-Reference states:

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

Each state has a bounded `presentationRef`; a blank panel is not failure handling. Optional maintenance/suppression references describe runtime presentation context without inventing additional core states.

Definitions may request freshness, provenance, affected source/Sensor and correlation-ID display when runtime supplies them. Missing, stale or partial telemetry never permits the definition to infer `DEVICE_DOWN` or another physical/causal conclusion; `causalInferenceFromMissingData` is fixed to `PROHIBITED`.

## Saved views

Saved views contain presentation state only:

- saved-view identity/version;
- owner/scope references;
- dashboard/workspace target reference;
- opaque filter-selection references;
- time-context policy reference;
- sort/group/view-mode references;
- provenance;
- mandatory runtime server authorization.

They do not contain passwords, tokens, cookies, arbitrary payloads, authorization grants or executable query text.

## Report definition

Report definitions can represent:

- stable identity/version/origin/scope/provenance;
- presentation-only role/resource/permission/entitlement references;
- folder/category/tags;
- branding/layout references;
- bounded query-policy reference;
- governed query, grouping, aggregation and time-binding references;
- variables/filters;
- opaque calculation references rather than formula text;
- `TEXT`, `TABLE`, `VISUALIZATION` and governed `IMAGE_REFERENCE` sections;
- column/table/visualization/image references;
- SLA/reporting policy references;
- report formats required by the PRD: `PDF`, `CSV`, `XLSX`, `JSON`, `API`;
- schedule intent through `scheduleRef`, timezone and enabled flag;
- delivery intent for `DOWNLOAD`, `EMAIL` or `INTEGRATION` through governed target references.

Definitions contain no recipient addresses, notification credentials or delivery secrets. This checkpoint does not render or send anything.

## Schedule/export boundary

The schema does not define scheduler syntax or cron. `scheduleRef` points at a future governed schedule contract and timezone is explicit. Script hooks/commands are invalid.

Export declarations only express supported/requested format metadata. They do not prove PDF/CSV/XLSX/JSON generation exists.

## Scale/performance metadata

Dashboard definitions can require server-side pagination, filtering, sorting, browser virtualization and a bounded `queryPolicyRef`. Reports can also reference a governed query policy.

No row limit, timeout, refresh interval, query concurrency limit or export threshold is invented here. Large-environment values remain future Query Service policy/benchmark decisions.

## Platform Value / OIV readiness

The metadata model can represent future Platform Value, MTTR, noise-reduction, diagnosis-effectiveness, incident-improvement and operational/economic-benefit views without embedding customer claims.

`valuePresentationPolicy` can require methodology, sample-size and assumption disclosure, prohibit unsupported causal claims and require customer-approved assumptions for financial estimates. Static definitions containing unsupported MTTR/savings/benefit percentages are rejected by the validator.

Capability participation is not proof of causal benefit.

## Security

Schemas are strict and the semantic validator fails closed for:

- passwords, hashes, tokens, bearer/API keys, cookies, private keys, client secrets and database/datasource credentials;
- credentials embedded in URLs;
- raw SQL or SQL-like query text;
- JavaScript, Python, shell, commands, scripts and executable/formula/expression fields;
- raw URL fields/dangerous schemes;
- active/unsafe HTML;
- path traversal and absolute filesystem paths;
- unknown fields;
- embedded authorization/entitlement decision semantics;
- unbounded strings/arrays/documents;
- duplicate IDs and broken references;
- variable cycles;
- System/clone identity corruption;
- missing required states;
- invalid report sections/schedules/delivery metadata;
- unsupported static OIV/value percentages.

Text fields are plain text/Markdown-source metadata only. Any future renderer must sanitize independently.

## Defensive bounds

The schemas/validator contain conservative parser/input-safety limits for denial-of-service resistance. These are engineering security bounds only, **not** customer operational thresholds, dashboard row limits, refresh defaults, SLA targets, retention values or licensing limits.

Directory validation is bounded, ignores symlinks and does not treat schema/fixture-definition JSON as runtime documents.

## Versioning

`schemaVersion` is `1.0.0`. Unsupported versions fail closed rather than being silently reinterpreted.

`definitionVersion` is a bounded opaque definition revision identifier; this contract does not impose SemVer on customer definitions. Breaking schema changes require a new incompatible contract version plus an explicit compatibility/migration path before runtime adoption. No migration service is implemented here.

## What this checkpoint does not implement

- Dashboard Studio / Report Studio UI;
- the 98 mandatory screen definitions or a 98/98 completeness claim;
- React/panel rendering;
- query/database execution;
- SQL/query DSL/formula execution;
- APIs or backend persistence;
- report scheduling workers/history;
- SLA calculation;
- PDF/CSV/XLSX/JSON rendering;
- email/integration delivery;
- RBAC/resource authorization or entitlement decisions;
- customer-specific thresholds, SLA values, refresh periods, retention or value claims.

Future integration should consume this contract only after independent validation/merge and preserve server-side authorization/query boundaries.

## Local Docker validation (later)

From repository root:

```bash
docker run --rm \
  -v "$PWD:/repo:ro" \
  -w /repo \
  node:20-alpine \
  node schemas/presentation/tools/validate.mjs
```

This GitHub-only checkpoint does not claim that command has executed.
