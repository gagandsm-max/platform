# Capability Packs / MIB Studio / API Studio / Extensibility

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 07, 09, 10, 20, 23

## Product objective

Provide one governed extensibility architecture for technologies/vendors without Core forks, arbitrary plugins or parallel package frameworks.

## Single framework

**Capability Pack** is the single package architecture. Classifications include:

- Device / Network Pack;
- Technology Monitoring Pack;
- SaaS Monitoring Pack;
- Integration Pack;
- optional Customer Extension Pack.

"Diagnostic pack" is a deterministic workflow/declaration, not a second package runtime.

## Pack content

A Pack may declare:

- identity/discovery;
- supported product/version;
- protocol/API mappings;
- metrics/state/inventory/topology;
- events/log mappings;
- SNMP poll/table/index/counter/trap semantics;
- API endpoints/auth/mapping;
- diagnostics;
- NCM actions;
- dashboards/views;
- alert evidence hints;
- compatibility/security/certification metadata;
- provenance/licensing;
- resource/cost hints.

No secrets are embedded.

## Certification

Support is dimensional:

- Discovery;
- L1 basic availability/health;
- L2 deeper performance/operational metrics;
- L3 advanced diagnostics/analytics;
- Topology;
- NCM where relevant;
- Security;
- version compatibility;
- evidence/certification profile.

Specified != implemented != tested != certified != deployed.

## Runtime

Packs load into existing Core/Sensor/Agent/connector surfaces; they do not get dedicated Core services/databases by default. All behavior remains under RBAC, tenant scope, Credential Profiles, audit and resource limits.

## MIB Studio

Supports governed MIB upload/dependencies, OID tree, textual conventions, enums, tables/indexes, notifications, provenance/licensing and mapping generation. Customer MIBs are customer-scoped unless redistribution rights are recorded.

## API Studio

Supports OpenAPI/API definition import, endpoint/auth/mapping configuration, test/preview, schema interpretation and controlled Pack/monitor publication. AI may suggest mappings but cannot silently publish production behavior.

## Test Lab

Capability Pack Test Lab should accept representative fixtures such as SNMP walks, traps/syslogs, API responses, CLI/config outputs and vendor-version metadata, producing reproducible certification evidence.

## Extension safety

No arbitrary customer code in hot paths by default. Declarative transforms must be deterministic, schema validated and cost bounded. New runtime dependencies require OSS/security/packaging review.
