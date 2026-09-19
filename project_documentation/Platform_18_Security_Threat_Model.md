# Platform — Security Threat Model

**Document ID:** PLATFORM-THREAT-001  
**Version:** 3.2.2  
**Status:** Authoritative Mother Reference

## 1. Assets

Credentials, certificates/private keys, tenant data, telemetry, **Flow exporter/listener/template/sampling state, endpoint/user/session/domain/Geo enrichment, user-attributed traffic evidence**, network configurations, privileged CLI/NCM access, audit records, licensing, AI context and software supply chain.

## 2. Trust boundaries

Browser↔Core; Sensor/Agent↔Core ingestion/control; **Flow exporter/network↔dedicated Flow Sensor; Flow Sensor↔identity/DNS/IPAM/session enrichment cache/control context**; Core↔data services; Core↔ITSM/cloud/AI; NCM worker↔managed device; package/update channel; tenant boundary; HA peer boundary.

## 3. Threat classes

Spoofed Sensor/Agent enrollment, credential theft, tenant breakout, command injection, malicious Device Pack/connector, supply-chain dependency compromise, Kafka/event spoofing, replay, insecure legacy protocol misuse, **spoofed/unknown UDP Flow exporter, forged observation-domain/engine/source identity, cross-tenant exporter collision, template/cardinality churn or poisoning, malformed/oversized Flow datagrams, variable-length/enterprise-IE parser abuse, UDP/socket/queue flood, per-exporter starvation, sampling metadata manipulation, sequence/reset confusion, replay/duplicate poisoning, DNS/identity/Geo enrichment poisoning, stale DHCP/IP/user-session attribution, cross-tenant user enrichment, privacy leakage of user-attributed Flow, fabricated URL/precise-location inference, unbounded Flow query/Sankey/cardinality abuse**, log secret leakage, storage exhaustion, privilege escalation, destructive NCM action, poisoned AI context, GNN/AI over-trust, malicious upgrade, arbitrary-target diagnostic abuse/SSRF-like pivoting, cross-tenant diagnostic execution, diagnostic amplification/resource exhaustion, CLI session hijack, one-time credential capture/persistence, command-policy bypass and investigation-evidence tampering.

## 4. Key mitigations

mTLS identity, single-use enrollment tokens, encrypted secret storage, server-side RBAC/resource scope, maker-checker, command policy, signed packages, SBOM/provenance, audit, bounded inputs/queries, schema validation, content redaction, data-quality/confidence handling, **dedicated Flow Sensor role isolation, configured exporter/listener/tenant admission, unknown-exporter quarantine/reject, per-exporter/global resource bounds and fairness, bounded template/observation-domain/field/cardinality state, protocol-aware sequence/generation tracking, observed-vs-estimated sampling provenance, stable replay identity after durable acceptance, temporal enrichment with source/version/confidence, cross-tenant negative tests, distinct permission/audit for user-attributed Flow, approximate Geo labelling and no inferred full URL, bounded top-N/matrix/Sankey queries**, AI tool guardrails, INCONCLUSIVE RCA state, canonical-target authorization, Sensor/Agent-mediated diagnostics rather than browser-direct target sockets, hard diagnostic timeout/rate/concurrency/fan-out limits, cancellation, source/vantage provenance, ephemeral one-time CLI credentials, WebSocket/session binding and expiry, step-up controls for privileged commands, tamper-evident investigation/audit references and separation of monitored versus on-demand evidence.

## 5. Development threat controls

Docker development networks are not trusted production boundaries. Test secrets only. No production credentials in local images/volumes. Build/signing credentials stay outside normal development containers.
