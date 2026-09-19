# Platform — Security Threat Model

**Document ID:** PLATFORM-THREAT-001  
**Version:** 3.2.2  
**Status:** Authoritative Mother Reference

## 1. Assets

Credentials, certificates/private keys, tenant data, telemetry/log contents, log source/parser/routing policy, external-destination credentials, network configurations, privileged CLI/NCM access, audit records, licensing, AI context and software supply chain.

## 2. Trust boundaries

Browser↔Core; log/device/application source↔Sensor/Agent; Sensor/Agent↔Core ingestion/control; Core↔data services; Core/eligible Streaming Sensor↔external SIEM/log destination; Core↔ITSM/cloud/AI; NCM worker↔managed device; package/update channel; tenant boundary; HA peer boundary.

## 3. Threat classes

Spoofed Sensor/Agent enrollment, credential theft, tenant breakout, command injection, malicious Device Pack/connector, supply-chain dependency compromise, Kafka/event spoofing, replay, insecure legacy protocol misuse, spoofed Syslog/log source, forged tenant/source fields, cross-tenant log query/forwarding, malicious parser/regex, malformed/oversized log input, log injection, log secret/PII leakage, high-cardinality/log-flood DoS, queue/disk exhaustion, destination credential theft, outbound data exfiltration, forwarding loops, unauthorized replay/dead-letter abuse, storage exhaustion, privilege escalation, destructive NCM action, poisoned AI context/prompt injection through log text, GNN/AI over-trust, malicious upgrade, arbitrary-target diagnostic abuse/SSRF-like pivoting, cross-tenant diagnostic execution, diagnostic amplification/resource exhaustion, CLI session hijack, one-time credential capture/persistence, command-policy bypass and investigation-evidence tampering.

## 4. Key mitigations

mTLS identity, single-use enrollment tokens, encrypted secret storage, server-side RBAC/resource scope, maker-checker, command policy, signed packages, SBOM/provenance, audit, bounded inputs/queries/parsers, schema validation, content redaction, data-quality/confidence handling, tenant-derived trusted ingestion envelopes, source/destination quotas, rate/cardinality limits, bounded spool/queues, forwarding-loop protection, destination failure isolation, sensitive-data storage/egress policy, audited replay/dead-letter actions, AI untrusted-log handling/tool guardrails, INCONCLUSIVE RCA state, canonical-target authorization, Sensor/Agent-mediated diagnostics rather than browser-direct target sockets, hard diagnostic timeout/rate/concurrency/fan-out limits, cancellation, source/vantage provenance, ephemeral one-time CLI credentials, WebSocket/session binding and expiry, step-up controls for privileged commands, tamper-evident investigation/audit references and separation of monitored versus on-demand evidence.

## 5. Development threat controls

Docker development networks are not trusted production boundaries. Test secrets only. No production credentials in local images/volumes. Build/signing credentials stay outside normal development containers.
