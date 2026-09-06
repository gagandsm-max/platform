# Platform — Security Threat Model

**Document ID:** PLATFORM-THREAT-001  
**Version:** 3.2.1  
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
