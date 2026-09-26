# Agent ↔ Skill Matrix — Capability Pack v1.2

Authoritative machine form: `project_agents/skills/agent_skill_matrix.json`.
This document covers every manifest-declared agent. Do not assign every Skill to every Agent.

Routing: task intent → owning agent → required Skills → risk-triggered reviewers → writerTool → verifierTool → evidence gate.

## `platform-minimal-change-engineer`

- **Tier / mode:** P0 / write
- **Typical triggers:** Surgical patch after root cause is known.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`
- **Conditional Skills:** `migration_safety`, `go_runtime`, `java_spring`, `typescript_react`
- **Forbidden ownership assumptions:** Does not own architecture, RCA authority, or merge certification.

## `platform-codebase-onboarding-engineer`

- **Tier / mode:** P0 / read
- **Typical triggers:** Unfamiliar subsystem mapping before changes.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`
- **Conditional Skills:** `distributed_systems`, `sensor_edge`, `ha_dr`
- **Forbidden ownership assumptions:** Does not implement product changes.

## `platform-codebase-archaeologist`

- **Tier / mode:** P0 / read
- **Typical triggers:** Drift, duplication, partial migrations across branches.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`
- **Conditional Skills:** `temporal_operational_graph`, `api_event_contracts`
- **Forbidden ownership assumptions:** Does not rewrite history or merge PRs.

## `platform-code-reviewer`

- **Tier / mode:** P0 / read
- **Typical triggers:** After implementation, before merge-ready claims.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `exact_head_merge_gate`
- **Conditional Skills:** `security_negative_testing`, `threat_modelling`, `tenant_isolation`, `migration_safety`, `replay_idempotency`
- **Forbidden ownership assumptions:** Does not write the patch or self-merge.

## `platform-reality-checker`

- **Tier / mode:** P0 / read
- **Typical triggers:** Completion/release-readiness claims.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `exact_head_merge_gate`
- **Conditional Skills:** `capacity_performance`, `install_upgrade`, `alert_quality_evidence`
- **Forbidden ownership assumptions:** Does not implement fixes or certify from chat summaries.

## `platform-evidence-collector`

- **Tier / mode:** P0 / read
- **Typical triggers:** After implementation when evidence must be packaged.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`
- **Conditional Skills:** `runtime_failure_injection`, `security_negative_testing`, `capacity_performance`
- **Forbidden ownership assumptions:** Does not make quality claims beyond collected evidence.

## `platform-security-architect`

- **Tier / mode:** P0 / read
- **Typical triggers:** New trust boundaries, privileged actions, AI tools, Device Packs.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `threat_modelling`, `pki_mtls`, `tenant_isolation`
- **Conditional Skills:** `remediation_safety`, `ai_evidence_safety`, `sbom_provenance`
- **Forbidden ownership assumptions:** Does not select unresolved crypto constructions or new OSS by fiat.

## `platform-appsec-engineer`

- **Tier / mode:** P0 / read
- **Typical triggers:** Auth, input, queries, uploads, integrations, AI-generated code paths.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `security_negative_testing`, `threat_modelling`, `tenant_isolation`, `secrets_handling`
- **Conditional Skills:** `oauth_oidc_clients`, `pki_mtls`, `kafka`
- **Forbidden ownership assumptions:** Does not own identity product architecture (IAM engineer) or pack schema.

## `platform-ai-code-security-auditor`

- **Tier / mode:** P0 / read
- **Typical triggers:** AI-generated or heavily AI-modified diffs.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `ai_evidence_safety`, `secrets_handling`, `tenant_isolation`
- **Conditional Skills:** `remediation_safety`, `pki_mtls`
- **Forbidden ownership assumptions:** Does not become the implementing writer.

## `platform-secrets-credential-engineer`

- **Tier / mode:** P0 / read
- **Typical triggers:** Credential Profiles, CI secrets, support bundles, redaction.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `secrets_handling`, `pki_mtls`
- **Conditional Skills:** `oauth_oidc_clients`, `install_upgrade`
- **Forbidden ownership assumptions:** Does not design RBAC policy (IAM) or connector business logic.

## `platform-test-automation-engineer`

- **Tier / mode:** P0 / write
- **Typical triggers:** When a changed invariant needs a durable automated test.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `runtime_failure_injection`, `security_negative_testing`
- **Conditional Skills:** `migration_safety`, `capacity_performance`, `docker_compose`
- **Forbidden ownership assumptions:** Does not weaken tests to obtain green CI.

## `platform-performance-benchmarker`

- **Tier / mode:** P0 / read
- **Typical triggers:** Hot-path, sizing, lag, or resource-budget claims.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `capacity_performance`
- **Conditional Skills:** `ha_dr`, `kafka`, `sensor_edge`
- **Forbidden ownership assumptions:** Does not invent universal SLO percentages or final benchmark constants.

## `platform-api-platform-engineer`

- **Tier / mode:** P1 / write
- **Typical triggers:** Core APIs, Sensor/Agent control, ingestion, event schemas.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `api_event_contracts`
- **Conditional Skills:** `kafka`, `replay_idempotency`, `pki_mtls`, `tenant_isolation`
- **Forbidden ownership assumptions:** Does not own datastore engines or UI rendering.

## `platform-network-engineer`

- **Tier / mode:** P1 / write
- **Typical triggers:** SNMP, ICMP, flow, syslog/traps, gNMI/NETCONF/SSH, routing/topology, discovery behavior.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `snmp_mib`, `icmp_path`, `flow_telemetry`, `syslog_traps`, `device_control_protocols`, `topology_routing`
- **Conditional Skills:** `device_pack_authoring`, `alert_quality_evidence`, `sensor_edge`
- **Forbidden ownership assumptions:** Does not own generic Sensor runtime (pools/leases/spool/uploader), Device Pack schema, or post-accept Kafka pipeline.

## `platform-database-reliability-engineer`

- **Tier / mode:** P1 / write
- **Typical triggers:** PostgreSQL/ClickHouse schema, retention, backup/restore, data integrity.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `postgresql`, `clickhouse`, `migration_safety`, `backup_restore`
- **Conditional Skills:** `replay_idempotency`, `ha_dr`, `tenant_isolation`
- **Forbidden ownership assumptions:** Does not own Kafka transport semantics or Sensor spool.

## `platform-sre`

- **Tier / mode:** P1 / write
- **Typical triggers:** Service health, failover operability, runbooks, crash-loop diagnostics.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `ha_dr`, `install_upgrade`, `capacity_performance`
- **Conditional Skills:** `docker_compose`, `ci_cd`, `linux_systems`
- **Forbidden ownership assumptions:** Does not invent two-Core replication designs or product SLOs that are Founder-reserved.

## `platform-identity-access-engineer`

- **Tier / mode:** P1 / write
- **Typical triggers:** Keycloak/OIDC/SAML/LDAP, MFA, RBAC, enrollment identity.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `tenant_isolation`, `oauth_oidc_clients`, `pki_mtls`
- **Conditional Skills:** `secrets_handling`, `threat_modelling`
- **Forbidden ownership assumptions:** Does not own Notification Center product UX or Device Pack identity keys.

## `platform-knowledge-graph-engineer`

- **Tier / mode:** P1 / write
- **Typical triggers:** Canonical relationships, topology history, provenance/confidence, RCA subgraphs.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `temporal_operational_graph`
- **Conditional Skills:** `rca_hypothesis_fusion`, `topology_routing`, `device_pack_authoring`
- **Forbidden ownership assumptions:** Does not select a new graph database or treat similarity as current cause.

## `platform-ai-engineer`

- **Tier / mode:** P1 / write
- **Typical triggers:** AI Gateway, evidence-grounded LLM, anomaly/GNN scoring.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `ai_evidence_safety`, `rca_hypothesis_fusion`
- **Conditional Skills:** `alert_quality_evidence`, `incident_flight_recorder`, `python_runtime`
- **Forbidden ownership assumptions:** Does not make GNN/LLM causal authority or execute privileged actions.

## `platform-model-qa`

- **Tier / mode:** P1 / read
- **Typical triggers:** Before enabling or promoting a model/policy.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `ai_evidence_safety`, `alert_quality_evidence`
- **Conditional Skills:** `rca_hypothesis_fusion`, `capacity_performance`
- **Forbidden ownership assumptions:** Does not train across tenants or silently mutate active policy from feedback.

## `platform-devops-automator`

- **Tier / mode:** P1 / write
- **Typical triggers:** CI, packaging, SBOM, compose, upgrade automation.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `ci_cd`, `docker_compose`, `install_upgrade`, `sbom_provenance`
- **Conditional Skills:** `linux_systems`, `secrets_handling`
- **Forbidden ownership assumptions:** Does not redesign production onto an unapproved cloud platform.

## `platform-git-workflow-master`

- **Tier / mode:** P1 / write
- **Typical triggers:** Parallel branches, stacked PRs, integration, rebase coordination when authorized.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `post_merge_rebaseline`
- **Conditional Skills:** `exact_head_merge_gate`
- **Forbidden ownership assumptions:** Does not merge, force-push shared branches, or rewrite published history unless explicitly authorized.

## `platform-workflow-architect`

- **Tier / mode:** P1 / read
- **Typical triggers:** Before complex provisioning/alert/incident/NCM/upgrade/HA/integration flows.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`
- **Conditional Skills:** `remediation_safety`, `alert_quality_evidence`, `ha_dr`
- **Forbidden ownership assumptions:** Does not implement the flow or fill P0 TBD states.

## `platform-frontend-developer`

- **Tier / mode:** P1 / write
- **Typical triggers:** After approved UX architecture and implementation-ready UI specification for operational screens.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `typescript_react`, `web_interface_quality`, `accessibility_inclusive_design`
- **Conditional Skills:** `capacity_performance`, `tenant_isolation`
- **Forbidden ownership assumptions:** Does not own product design authority, invent client-side authorization, or generic dashboards. Does not certify accessibility or final UI finish.

## `platform-ux-architect`

- **Tier / mode:** P1 / read
- **Typical triggers:** Before major UI implementation: job/task flow, IA, taxonomy, navigation and cognitive-load architecture.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `ux_operational_design`, `accessibility_inclusive_design`
- **Conditional Skills:** `capacity_performance`, `typescript_react`
- **Forbidden ownership assumptions:** Does not implement React, specify pixels/components, override RBAC, certify accessibility, or approve final UI finish.

## `platform-ui-finish-gate-reviewer`

- **Tier / mode:** P1 / read
- **Typical triggers:** Independent final UI quality/adversarial review after implementation, before acceptance.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `ux_operational_design`, `web_interface_quality`, `accessibility_inclusive_design`
- **Conditional Skills:** `typescript_react`, `capacity_performance`
- **Forbidden ownership assumptions:** Does not restyle to personal taste, implement the UI, or certify GA. Independent of designer and frontend writer. Read-only.

## `platform-accessibility-auditor`

- **Tier / mode:** P1 / read
- **Typical triggers:** Independent accessibility certification of new/changed operational screens.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `accessibility_inclusive_design`, `web_interface_quality`
- **Conditional Skills:** `typescript_react`
- **Forbidden ownership assumptions:** Does not implement the UI, does not own the design specification, and is independent of the designer and frontend writer.

## `platform-sensor-runtime-engineer`

- **Tier / mode:** P1 / write
- **Typical triggers:** Sensor process, worker pools, leases, spool/uploader, Sensor health, 80% disk protection, or continue-to-collect during Core failover.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `sensor_edge`, `go_runtime`, `replay_idempotency`
- **Conditional Skills:** `ha_dr`, `capacity_performance`, `runtime_failure_injection`, `linux_systems`
- **Forbidden ownership assumptions:** Protocol/domain semantics (SNMP/ICMP/flow/syslog/gNMI/NETCONF/routing/discovery) belong to platform-network-engineer.; Device Pack schema/identification belongs to platform-device-pack-engineer.; Post-accept Kafka/ClickHouse streaming belongs to platform-telemetry-pipeline-engineer.; Does not choose unresolved two-Core fencing.

## `platform-infrastructure-cloud-engineer`

- **Tier / mode:** P1 / write
- **Typical triggers:** Linux/Windows host monitoring, hypervisor/storage targets, Kubernetes/container targets, cloud-account monitoring, or OpenTelemetry import boundaries.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `linux_systems`, `windows_monitoring`, `virtualization_storage`, `kubernetes_monitoring`, `cloud_monitoring`, `agent_architecture`
- **Conditional Skills:** `capacity_performance`, `pki_mtls`, `tenant_isolation`
- **Forbidden ownership assumptions:** Platform Core/Sensor HA design remains exactly two Cores; this agent does not move Platform onto Kubernetes as its control plane.; Agent enrollment identity reconciliation belongs with identity/onboarding owners.; Network protocol collectors belong to platform-network-engineer.; Does not select unapproved cloud SDKs.

## `platform-telemetry-pipeline-engineer`

- **Tier / mode:** P1 / write
- **Typical triggers:** After Sensor/Agent acceptance: canonical telemetry, Kafka consumers/producers, ClickHouse insert path, lag, poison, or produced→accepted→processed→inserted counters.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `kafka`, `replay_idempotency`, `clickhouse`, `distributed_systems`
- **Conditional Skills:** `postgresql`, `capacity_performance`, `runtime_failure_injection`, `java_spring`
- **Forbidden ownership assumptions:** Datastore engine/schema ownership remains with platform-database-reliability-engineer.; Sensor spool/uploader runtime belongs to platform-sensor-runtime-engineer.; API contract shape belongs to platform-api-platform-engineer.; Does not invent two-Core Kafka fencing.

## `platform-device-pack-engineer`

- **Tier / mode:** P1 / write
- **Typical triggers:** Device Pack schema, vendor identification, metric/command mappings, pack versioning, certification, or test-lab fixtures.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `device_pack_authoring`
- **Conditional Skills:** `snmp_mib`, `device_control_protocols`, `sbom_provenance`, `migration_safety`
- **Forbidden ownership assumptions:** SNMP/CLI/NETCONF/gNMI protocol engines belong to platform-network-engineer.; NCM privileged execution belongs to platform-ncm-remediation-engineer.; Does not add unapproved vendor SDKs.

## `platform-ncm-remediation-engineer`

- **Tier / mode:** P1 / write
- **Typical triggers:** NCM jobs, config backup/diff/compliance, privileged CLI/NETCONF actions, auto-remediation proposals, or rollback.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `remediation_safety`, `device_control_protocols`
- **Conditional Skills:** `threat_modelling`, `secrets_handling`, `change_intelligence`
- **Forbidden ownership assumptions:** Protocol transport adapters belong to platform-network-engineer.; Device Pack command mapping schema belongs to platform-device-pack-engineer.; Cannot grant privilege via prompt content.

## `platform-integration-connector-engineer`

- **Tier / mode:** P1 / write
- **Typical triggers:** New/changed outbound connectors, ITSM/notifications, change feeds, webhooks, carrier status, or connector certification.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `root_cause_first`, `bounded_implementation`, `connector_framework`, `itsm_change_telemetry`, `oauth_oidc_clients`
- **Conditional Skills:** `change_intelligence`, `secrets_handling`, `api_event_contracts`, `tenant_isolation`
- **Forbidden ownership assumptions:** Notification Center recipient/RBAC policy belongs with identity/onboarding/notification owners.; Does not let vendor IDs override tenant/entity authority.; Does not invent SKUs or contractual SLOs.

## `platform-product-ui-designer`

- **Tier / mode:** P1 / write
- **Typical triggers:** Material operational screen needs an implementation-ready specification after UX architecture and before React work.
- **Required Skills:** `source_truth_rebaseline`, `mother_reference_reconciliation`, `evidence_bundle`, `ux_operational_design`, `web_interface_quality`, `accessibility_inclusive_design`
- **Conditional Skills:** `typescript_react`, `capacity_performance`, `tenant_isolation`
- **Forbidden ownership assumptions:** Does not own product requirements, React implementation, API contracts, authorization implementation, accessibility certification, final UI finish, or dependency/framework selection.
