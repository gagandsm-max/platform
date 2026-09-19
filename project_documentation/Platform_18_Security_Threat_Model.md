# Platform — Security Threat Model

**Document ID:** PLATFORM-THREAT-001  
**Version:** 3.2.2  
**Status:** Authoritative Mother Reference

## 1. Assets

Credentials, certificates/private keys, tenant data, telemetry, **Flow exporter/listener mappings, templates/options state, user/session attribution, DNS/domain/application enrichment, public-IP GeoIP classification**, network configurations, privileged CLI/NCM access, audit records, licensing, AI context and software supply chain.

## 2. Trust boundaries

Browser↔Core; Sensor/Agent↔Core ingestion/control; **Flow exporter/network↔dedicated Flow Sensor listener; identity/DNS/IPAM/AD-Entra/VPN/RADIUS/NAC/firewall enrichment sources↔Core/Flow enrichment; public GeoIP/cloud/ASN catalogue↔approved enrichment boundary**; Core↔data services; Core↔ITSM/cloud/AI; NCM worker↔managed device; package/update channel; tenant boundary; HA peer boundary.

## 3. Threat classes

Spoofed Sensor/Agent enrollment, credential theft, tenant breakout, command injection, malicious Device Pack/connector, supply-chain dependency compromise, Kafka/event spoofing, replay, insecure legacy protocol misuse, **spoofed/unknown UDP Flow exporter, forged observation-domain/source metadata, cross-tenant exporter collision, template poisoning/churn/cardinality exhaustion, oversized/malformed/enterprise-IE Flow payloads, packet/record flood and one-exporter starvation, sampling/sequence/time manipulation, replay-based traffic inflation, identity/user-attribution poisoning, stale DHCP/DNS/session enrichment, misleading GeoIP/application classification, unauthorized user-attributed traffic access/export, sensitive URL/query/token leakage, unauthorized Sensor-role expansion or mixed-role activation**, log secret leakage, storage exhaustion, privilege escalation, destructive NCM action, poisoned AI context, GNN/AI over-trust, malicious upgrade, arbitrary-target diagnostic abuse/SSRF-like pivoting, cross-tenant diagnostic execution, diagnostic amplification/resource exhaustion, CLI session hijack, one-time credential capture/persistence, command-policy bypass and investigation-evidence tampering.

## 4. Key mitigations

mTLS identity, single-use enrollment tokens, encrypted secret storage, server-side RBAC/resource scope, maker-checker, command policy, signed packages, SBOM/provenance, audit, bounded inputs/queries, schema validation, content redaction, data-quality/confidence handling, **configured tenant/listener/exporter admission, unknown-exporter quarantine, bounded datagram/record/template/domain/IE cardinality, per-exporter fairness, protocol-aware template/sampling/sequence/time quality, stable replay identity after durable acceptance, temporal identity/session enrichment, conflict-preserving attribution, separate user-Flow authorization/audit, domain-vs-URL minimization, GeoIP precision labelling, centrally enforced exclusive Sensor roles**, AI tool guardrails, INCONCLUSIVE RCA state, canonical-target authorization, Sensor/Agent-mediated diagnostics rather than browser-direct target sockets, hard diagnostic timeout/rate/concurrency/fan-out limits, cancellation, source/vantage provenance, ephemeral one-time CLI credentials, WebSocket/session binding and expiry, step-up controls for privileged commands, tamper-evident investigation/audit references and separation of monitored versus on-demand evidence.

## 5. Development threat controls

Docker development networks are not trusted production boundaries. Test secrets only. No production credentials in local images/volumes. Build/signing credentials stay outside normal development containers.
