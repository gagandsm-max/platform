# Platform — Security Threat Model

**Document ID:** PLATFORM-THREAT-001  
**Version:** 3.2.2  
**Status:** Authoritative Mother Reference

## 1. Assets

Credentials, certificates/private keys, tenant data, telemetry, **Flow exporter/source mappings, templates/sampling state, endpoint/user identity enrichment, DNS/domain/Geo/cloud provenance and role-local Sensor spools**, network configurations, privileged CLI/NCM access, audit records, licensing, AI context and software supply chain.

## 2. Trust boundaries

Browser↔Core; Sensor/Agent↔Core ingestion/control; **Flow exporter↔dedicated FLOW Sensor listener; identity/DNS/DHCP/AD/Entra/VPN/RADIUS/NAC/WLAN/firewall/proxy enrichment source↔Core; FLOW/LOG Sensor role boundary**; Core↔data services; Core↔ITSM/cloud/AI; NCM worker↔managed device; package/update channel; tenant boundary; HA peer boundary.

## 3. Threat classes

Spoofed Sensor/Agent enrollment, credential theft, tenant breakout, command injection, malicious Device Pack/connector, supply-chain dependency compromise, Kafka/event spoofing, replay, insecure legacy protocol misuse, **spoofed/forged UDP Flow exporter, cross-tenant exporter/source confusion, observation-domain/template collisions, template/enterprise-IE cardinality or parser resource exhaustion, malformed/oversized Flow records, sequence/replay poisoning, sampling manipulation, exporter/clock identity confusion, Flow flood/socket exhaustion, enrichment poisoning, stale DNS/DHCP/IP ownership, false AD/user attribution, public-Geo overclaim, sensitive URL/user data exposure, employee-surveillance misuse, Sensor role-bypass/co-hosting resource starvation**, log secret leakage, storage exhaustion, privilege escalation, destructive NCM action, poisoned AI context, GNN/AI over-trust, malicious upgrade, arbitrary-target diagnostic abuse/SSRF-like pivoting, cross-tenant diagnostic execution, diagnostic amplification/resource exhaustion, CLI session hijack, one-time credential capture/persistence, command-policy bypass and investigation-evidence tampering.

## 4. Key mitigations

mTLS identity, single-use enrollment tokens, encrypted secret storage, server-side RBAC/resource scope, maker-checker, command policy, signed packages, SBOM/provenance, audit, bounded inputs/queries, schema validation, content redaction, data-quality/confidence handling, **FLOW/LOG production-role allowlists, configured listener/exporter tenant authority, unknown-source quarantine/reject, per-exporter/global rate/fairness bounds, template/domain/field/cardinality limits, protocol-specific sequence/reset quality, durable replay identity, temporal identity/DNS/DHCP/session joins with provenance/confidence/conflict, separate user-attribution permission/audit, URL minimization, approximate-Geo labelling and no person-location claim**, AI tool guardrails, INCONCLUSIVE RCA state, canonical-target authorization, Sensor/Agent-mediated diagnostics rather than browser-direct target sockets, hard diagnostic timeout/rate/concurrency/fan-out limits, cancellation, source/vantage provenance, ephemeral one-time CLI credentials, WebSocket/session binding and expiry, step-up controls for privileged commands, tamper-evident investigation/audit references and separation of monitored versus on-demand evidence.

## 5. Development threat controls

Docker development networks are not trusted production boundaries. Test secrets only. No production credentials in local images/volumes. Build/signing credentials stay outside normal development containers.
