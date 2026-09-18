# Platform — Security Threat Model

**Document ID:** PLATFORM-THREAT-001  
**Version:** 3.2.3
**Status:** Authoritative Mother Reference

## 1. Assets

Credentials, certificates/private keys, tenant data, telemetry, network configurations, privileged CLI/NCM access, audit records, licensing, AI context and software supply chain.

## 2. Trust boundaries

Browser↔Core; Sensor/Agent↔Core ingestion/control; Core↔data services; Core↔ITSM/cloud/AI; NCM worker↔managed device; package/update channel; tenant boundary; HA peer boundary.

## 3. Threat classes

Spoofed Sensor/Agent enrollment, credential theft, tenant breakout, command injection, malicious Device Pack/connector, supply-chain dependency compromise, Kafka/event spoofing, replay, insecure legacy protocol misuse, log secret leakage, storage exhaustion, privilege escalation, destructive NCM action, poisoned AI context, GNN/AI over-trust and malicious upgrade.

## 4. Key mitigations

mTLS identity, single-use enrollment tokens, encrypted secret storage, server-side RBAC/resource scope, maker-checker, command policy, signed packages, SBOM/provenance, audit, bounded inputs/queries, schema validation, content redaction, data-quality/confidence handling, AI tool guardrails and INCONCLUSIVE RCA state.

## 5. Development threat controls

Docker development networks are not trusted production boundaries. Test secrets only. No production credentials in local images/volumes. Build/signing credentials stay outside normal development containers.

---

## 6. v3.2.3 UI/dashboard/provisioning threat supplement

### Added threats

- cross-tenant dashboard/saved-view/data-source reference injection;
- raw-query injection or mutation attempts;
- Query-AST tampering to bypass tenant/resource filters;
- query amplification/cardinality/resource-exhaustion through huge ranges, joins, variables or panels;
- malicious dashboard blueprint/import content including script/HTML/XSS and permission/data-source substitution;
- unsafe rendering of logs, SQL, JSON or connector responses;
- credential/token exfiltration through provisioning forms, URLs, browser logs, exports or saved dashboard state;
- stale-response races that replace newer operator context with older data;
- malicious/compromised visualization/map/search dependency;
- browser terminal/ZTP misuse leading to privileged command execution;
- misleading RCA confidence or UI-local thresholds causing unsafe operator action.

### Required mitigations

- server-derived tenant/resource scope and RBAC for every query/action;
- typed schema validation and allowlisted panel/query AST fields;
- parameterization; no direct concatenation of variables into privileged queries;
- read-only grammar/policy for expert raw mode plus query budgets/timeouts/concurrency;
- import validation, executable-content rejection, escaped raw-text rendering and CSP;
- independent panel cancellation/stale-response fencing;
- secret references and USE-vs-REVEAL separation;
- dependency SBOM/CVE/licence/offline admission;
- semantic status plus evidence provenance;
- calibrated confidence only and explicit `INCONCLUSIVE`;
- privileged browser terminal/ZTP withheld until dedicated bastion/NCM authorization, audit, approval and rollback controls exist.

---

## 7. v3.2.3 Operational-view threat additions

Additional abuse cases include:
- fabricated/forged faceplate metadata misleading operators about physical interfaces;
- synthetic response-inspector leakage of authorization headers, cookies, tokens or sensitive bodies;
- database query/plan evidence leaking literals, credentials, PII or cross-tenant SQL text;
- visualization-threshold edits being confused with or incorrectly wired to alert-policy mutation;
- pointer-only operational actions creating inaccessible or unsafe alternate workflows.

Mitigations:
- faceplate rendering only from trusted tenant-scoped inventory/Device Pack evidence with logical fallback;
- response-inspector size bounds, escaping, redaction and separate authorization;
- query/plan normalization, tenant scope, bounded samples and redaction;
- distinct API/data contracts for presentation thresholds versus monitoring policy;
- keyboard/equivalent action paths using the same server authorization as pointer actions.
