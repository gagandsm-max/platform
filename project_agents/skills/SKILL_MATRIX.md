# Skill Matrix — Capability Pack v1.2

Canonical Skill source: `project_agents/skills/`.
Inventory is declared by `project_agents/skills/manifest.json`; do not freeze a Skill count as permanent validator policy.
v1.2 currently ships the following bounded library (governance Skills retained; overlapping auditor names merged or deferred).

Current canonical Skill count (manifest-derived): **57**.

## Governance / workflow

| Skill | Description |
|---|---|
| `source_truth_rebaseline` | Re-fetch authoritative main, exact task head, PR base/head and governing repository truth before relying on prior conclusions. |
| `mother_reference_reconciliation` | Discover the Mother Reference set dynamically from Platform_00 and reconcile requirements, decisions and implementation claims without stale document ceilings. |
| `root_cause_first` | Establish the evidenced failure mechanism before implementation; reject symptom-only patches and unsupported assumptions. |
| `bounded_implementation` | Apply the smallest defensible change inside the approved scope with explicit non-goals and no unrelated refactor. |
| `migration_safety` | Protect schema/data migrations with compatibility, rollback, idempotency, tenant isolation and failure-path evidence. |
| `security_negative_testing` | Prove security boundaries with adversarial and negative cases rather than success-path evidence alone. |
| `runtime_failure_injection` | Exercise crash, retry, timeout, durability and partial-failure paths and distinguish runtime evidence from source inference. |
| `alert_quality_evidence` | Evaluate alert evidence quality, telemetry trust, false-alert controls, severity/confidence separation and INCONCLUSIVE behavior. |
| `ai_evidence_safety` | Constrain AI claims and actions to authoritative evidence, deterministic tool boundaries, tenant/data-egress controls and graceful degradation. |
| `exact_head_merge_gate` | Gate review and merge claims on the exact current head, complete required CI/evidence and independent verification; drift invalidates stale evidence. |
| `evidence_bundle` | Package typed SOURCE_FACT, EXECUTION_EVIDENCE, CI_EVIDENCE, INFERENCE and UNVERIFIED claims with reproducible references. |
| `post_merge_rebaseline` | After an authorized merge, re-fetch main and dependent heads, invalidate stale stack assumptions and reconcile downstream work before continuing. |

## Architecture

| Skill | Description |
|---|---|
| `distributed_systems` | Design and change Core/Sensor distributed behavior: queues, backpressure, timeouts, consistency, failover and split-brain avoidance without reopening two-Core P0 designs. |
| `sensor_edge` | Sensor process/runtime mechanics: scheduling, worker pools, leases/fencing, durable spool, uploader/replay, resource lifecycle and Sensor self-observability. |
| `ha_dr` | Exactly-two-Core Active/Standby HA/DR operability, failover evidence and Sensor continue-to-collect behavior without inventing unresolved replication designs. |
| `capacity_performance` | Hot-path capacity, latency, lag, cardinality and resource-budget evidence for Core, Sensor, UI and storage protection. |

## Languages / runtime

| Skill | Description |
|---|---|
| `go_runtime` | Go Sensor/Agent implementation: context/cancellation, bounded concurrency, durable files, and fail-closed ack/offset behavior. |
| `java_spring` | Java 21 / Spring Core implementation: transactions, validation, actuator boundaries, and fail-closed consumers. |
| `typescript_react` | TypeScript/React UI: role-aware operational screens, server-driven state, accessibility and no generic dashboard filler. |
| `python_runtime` | Approved Python use for AI/analytics/NCM workers: isolation, no silent host installs, and catalogue-constrained dependencies. |

## Network / protocol

| Skill | Description |
|---|---|
| `snmp_mib` | SNMP/MIB collection, SNMPv3 security, counter semantics and Device Pack mapping without declaring device-down from SNMP alone. |
| `icmp_path` | ICMP/path reachability, RTT/loss/timeout evidence and path diagnostics as corroboration, not sole identity. |
| `flow_telemetry` | Flow export collection, template/replay/drop counters, cardinality control and Sensor isolation from flow floods. |
| `syslog_traps` | Syslog and SNMP trap ingestion, source attribution, storm control and evidence-quality parsing. |
| `device_control_protocols` | SSH/CLI, NETCONF/YANG and gNMI/gRPC device control/telemetry with credential, timeout and blast-radius constraints. |
| `topology_routing` | Routing/topology discovery, network-service dependencies (DNS/DHCP/NTP/AAA) and provenance/confidence on graph edges. |

## Infrastructure / cloud

| Skill | Description |
|---|---|
| `linux_systems` | Linux OS, systemd, process, disk, and host-path semantics for Core/Sensor/Agent packaging and monitoring targets. |
| `windows_monitoring` | Windows Agent service, MSI/BAT install, event/perf evidence and privacy-bounded endpoint collection. |
| `agent_architecture` | Endpoint Agent identity, enrollment/mTLS, durable local collection and privacy-bounded operational telemetry. |
| `virtualization_storage` | Virtualization, datastore and storage-target monitoring without turning Platform into DCIM/BMS. |
| `kubernetes_monitoring` | Kubernetes/container target monitoring, workload identity and collector-health separation. |
| `cloud_monitoring` | Cloud-account/target monitoring, OpenTelemetry import boundaries and vendor-ID normalization. |

## Data

| Skill | Description |
|---|---|
| `postgresql` | PostgreSQL control-state schema, transactions, tenant isolation and backup/restore of authoritative data. |
| `clickhouse` | ClickHouse historical analytics, insert idempotency, retention and lag without treating CH as control state. |
| `kafka` | Internal Kafka transport, consumer fail-closed ACK/offset, poison parking and lag evidence. |
| `replay_idempotency` | Replay, dedup, late-data and exactly-once-effect evidence across Sensor spool, Kafka and datastores. |
| `backup_restore` | Backup/restore, retention, cardinality and 80% storage-protection behavior including legal-hold limits. |

## API / integration

| Skill | Description |
|---|---|
| `api_event_contracts` | HTTPS/gRPC/REST and Kafka event contracts, compatibility, error codes and correlation IDs. |
| `connector_framework` | Outbound connector framework: auth, rate limit, retry, idempotency, durable outbound and certification. |
| `oauth_oidc_clients` | OAuth/OIDC client integrations, token handling, redirect safety and secret storage. |
| `itsm_change_telemetry` | ITSM/notification/change-telemetry connectors as supporting evidence, not causal proof. |

## Security

| Skill | Description |
|---|---|
| `threat_modelling` | Threat-model trust boundaries, privileged actions, Device Packs, AI tools and update channels before implementation. |
| `tenant_isolation` | Server-side tenant isolation, RBAC/resource scope and cross-tenant negative cases. |
| `pki_mtls` | PKI, mTLS enrollment, certificate identity and untrusted-cert rejection. |
| `secrets_handling` | Credential Profiles, connector secrets, CI credentials, redaction and support-bundle safety. |
| `remediation_safety` | Controlled remediation: deterministic actions, maker-checker, blast-radius, rollback and no LLM privileged execution. |

## Release / operations

| Skill | Description |
|---|---|
| `docker_compose` | Docker Compose development/test hygiene: named volumes, healthchecks, no destructive prune, reboot-safe local stack. |
| `ci_cd` | GitHub Actions/CI wiring, exact-head evidence, and no test weakening to obtain green. |
| `sbom_provenance` | SBOM, provenance, CVE/licence review and supply-chain evidence before new OSS. |
| `install_upgrade` | deb/rpm/MSI packaging, systemd lifecycle, upgrade/rollback and reboot auto-start evidence. |

## Platform differentiators

| Skill | Description |
|---|---|
| `device_pack_authoring` | Device Pack schema, identification, mappings, versioning, certification/provenance and test-lab semantics. |
| `monitoring_work_units` | Monitoring work-unit placement, Sensor assignment, capacity impact and locality constraints. |
| `temporal_operational_graph` | Temporal operational graph, topology provenance/confidence and relationship history. |
| `rca_hypothesis_fusion` | RCA hypothesis fusion: observed facts, likely cause, confidence, contradictions, missing evidence and INCONCLUSIVE. |
| `incident_flight_recorder` | Incident Flight Recorder and Similar Incident Memory: bounded, tenant-scoped, advisory historical evidence. |
| `change_intelligence` | Change Intelligence: external/provider/change evidence as supporting context with explicit non-causality. |
| `customer_value_mttr` | MTTR/value analytics with cohort methodology, data quality and no fabricated savings percentages. |
