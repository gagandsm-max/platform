# Platform — Security Threat Model

**Document ID:** PLATFORM-THREAT-001  
**Version:** 3.2.2  
**Status:** Authoritative Mother Reference

## 1. Assets

Credentials, certificates/private keys, tenant data, telemetry, network configurations, privileged CLI/NCM access, audit records, licensing, AI context and software supply chain.

## 2. Trust boundaries

Browser↔Core; Sensor/Agent↔Core ingestion/control; Core↔data services; Core↔ITSM/cloud/AI; NCM worker↔managed device; package/update channel; tenant boundary; HA peer boundary.

## 3. Threat classes

Spoofed Sensor/Agent enrollment, credential theft, tenant breakout, command injection, malicious Device Pack/connector, supply-chain dependency compromise, Kafka/event spoofing, replay, insecure legacy protocol misuse, log secret leakage, storage exhaustion, privilege escalation, destructive NCM action, poisoned AI context, GNN/AI over-trust, malicious upgrade, arbitrary-target diagnostic abuse/SSRF-like pivoting, cross-tenant diagnostic execution, diagnostic amplification/resource exhaustion, CLI session hijack, one-time credential capture/persistence, command-policy bypass and investigation-evidence tampering.

## 4. Key mitigations

mTLS identity, single-use enrollment tokens, encrypted secret storage, server-side RBAC/resource scope, maker-checker, command policy, signed packages, SBOM/provenance, audit, bounded inputs/queries, schema validation, content redaction, data-quality/confidence handling, AI tool guardrails, INCONCLUSIVE RCA state, canonical-target authorization, Sensor/Agent-mediated diagnostics rather than browser-direct target sockets, hard diagnostic timeout/rate/concurrency/fan-out limits, cancellation, source/vantage provenance, ephemeral one-time CLI credentials, WebSocket/session binding and expiry, step-up controls for privileged commands, tamper-evident investigation/audit references and separation of monitored versus on-demand evidence.

## 4.1 Cloud and full-stack observability threat supplement

Additional threat classes include over-privileged or stolen cloud delegated identity, cross-account/subscription/project/compartment enumeration, permission drift, forged/replayed provider events/webhooks, provider API abuse or SSRF-like target expansion, trace/log attribute injection, sensitive request/response data in telemetry, trace/span identity collision across tenants, high-cardinality/storage-exhaustion attacks, malicious or malformed OpenTelemetry/Prometheus payloads, cloud connector throttling used to create monitoring blindness, and P1 privacy/privilege risks from RUM, profiling, eBPF, billing/cost and customer AI/LLM telemetry.

Required mitigations include provider-native least privilege and short-lived identity where practical; tenant/provider-scope authorization; explicit hierarchy include/exclude rules; credential-vault references; permission preflight and drift detection; provider source/signature validation where available; bounded API call budgets; schema/payload/attribute size limits; tenant/cardinality/retention quotas; sensitive-field allow/drop/hash/mask/store policy; trace/log/RUM secret scanning/redaction; canonical tenant identity independent of trace ID or provider ID; replay/idempotency protection; and connector-health states that surface monitoring degradation instead of falsely asserting target failure.

P1 eBPF requires explicit kernel/privilege/overhead/security certification. P1 RUM/mobile telemetry must not capture prohibited employee-surveillance content by default. P1 cloud-cost and customer AI/LLM data are tenant-confidential telemetry and inherit normal encryption, RBAC, export and data-egress controls.

## 5. Development threat controls

Docker development networks are not trusted production boundaries. Test secrets only. No production credentials in local images/volumes. Build/signing credentials stay outside normal development containers.
