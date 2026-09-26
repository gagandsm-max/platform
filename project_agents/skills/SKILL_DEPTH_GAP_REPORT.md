# Skill procedural-depth gap report (Phase B2/B3)

Generated during Agent/Skill control-plane hardening (`admin/agent-skill-control-plane-hardening`).

This report is **informational**. It does **not** fail `validate_agents.py` in this tranche.
Do **not** mass-template generic filler to clear this list. Author real procedures in Phase B2/B3.

- Depth OK (all 10 signals detected): **10**
- P0 (thin + missing critical signals): **6**
- P1 (missing critical signals): **0**
- P2 (missing non-critical signals only): **44**

- Canonical skills scanned: **60**

| Priority | Skill | Lines | Present | Missing |
|---|---|---:|---|---|
| P0 | `ai_evidence_safety` | 23 | trigger, authoritative_inputs, invariants, security_tenancy, evidence, anti_patterns | procedure, negative_failure, stop_conditions, handoff |
| P0 | `alert_quality_evidence` | 22 | trigger, invariants, evidence, anti_patterns | authoritative_inputs, procedure, negative_failure, security_tenancy, stop_conditions, handoff |
| P0 | `bounded_implementation` | 23 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, stop_conditions, anti_patterns, handoff | negative_failure, evidence |
| P0 | `evidence_bundle` | 27 | trigger, invariants, security_tenancy, evidence, anti_patterns | authoritative_inputs, procedure, negative_failure, stop_conditions, handoff |
| P0 | `migration_safety` | 23 | trigger, authoritative_inputs, invariants, security_tenancy, evidence, anti_patterns | procedure, negative_failure, stop_conditions, handoff |
| P0 | `security_negative_testing` | 23 | trigger, invariants, negative_failure, security_tenancy, evidence, anti_patterns | authoritative_inputs, procedure, stop_conditions, handoff |
| P2 | `accessibility_inclusive_design` | 67 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `agent_architecture` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `api_event_contracts` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `backup_restore` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `capacity_performance` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `change_intelligence` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `ci_cd` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `clickhouse` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `cloud_monitoring` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `connector_framework` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `customer_value_mttr` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `device_control_protocols` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `device_pack_authoring` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `docker_compose` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `exact_head_merge_gate` | 22 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, anti_patterns, handoff | negative_failure, stop_conditions |
| P2 | `flow_telemetry` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `ha_dr` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `icmp_path` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `incident_flight_recorder` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `install_upgrade` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `itsm_change_telemetry` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `kubernetes_monitoring` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `linux_systems` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `monitoring_work_units` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `mother_reference_reconciliation` | 22 | trigger, authoritative_inputs, invariants, procedure, negative_failure, evidence, anti_patterns | security_tenancy, stop_conditions, handoff |
| P2 | `post_merge_rebaseline` | 22 | trigger, authoritative_inputs, invariants, procedure, evidence, anti_patterns, handoff | negative_failure, security_tenancy, stop_conditions |
| P2 | `postgresql` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `rca_hypothesis_fusion` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `remediation_safety` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `replay_idempotency` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `root_cause_first` | 23 | trigger, authoritative_inputs, invariants, procedure, negative_failure, evidence, anti_patterns | security_tenancy, stop_conditions, handoff |
| P2 | `runtime_failure_injection` | 21 | trigger, invariants, procedure, evidence, anti_patterns | authoritative_inputs, negative_failure, security_tenancy, stop_conditions, handoff |
| P2 | `sbom_provenance` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `secrets_handling` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `snmp_mib` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `source_truth_rebaseline` | 21 | trigger, authoritative_inputs, invariants, procedure, evidence, anti_patterns | negative_failure, security_tenancy, stop_conditions, handoff |
| P2 | `syslog_traps` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `temporal_operational_graph` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `topology_routing` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `typescript_react` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `ux_operational_design` | 85 | trigger, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | authoritative_inputs, negative_failure |
| P2 | `virtualization_storage` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| P2 | `web_interface_quality` | 57 | trigger, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | authoritative_inputs, negative_failure |
| P2 | `windows_monitoring` | 32 | trigger, authoritative_inputs, invariants, procedure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | negative_failure |
| OK | `distributed_systems` | 32 | trigger, authoritative_inputs, invariants, procedure, negative_failure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | _none_ |
| OK | `go_runtime` | 32 | trigger, authoritative_inputs, invariants, procedure, negative_failure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | _none_ |
| OK | `java_spring` | 32 | trigger, authoritative_inputs, invariants, procedure, negative_failure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | _none_ |
| OK | `kafka` | 32 | trigger, authoritative_inputs, invariants, procedure, negative_failure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | _none_ |
| OK | `oauth_oidc_clients` | 32 | trigger, authoritative_inputs, invariants, procedure, negative_failure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | _none_ |
| OK | `pki_mtls` | 32 | trigger, authoritative_inputs, invariants, procedure, negative_failure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | _none_ |
| OK | `python_runtime` | 32 | trigger, authoritative_inputs, invariants, procedure, negative_failure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | _none_ |
| OK | `sensor_edge` | 32 | trigger, authoritative_inputs, invariants, procedure, negative_failure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | _none_ |
| OK | `tenant_isolation` | 32 | trigger, authoritative_inputs, invariants, procedure, negative_failure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | _none_ |
| OK | `threat_modelling` | 32 | trigger, authoritative_inputs, invariants, procedure, negative_failure, security_tenancy, evidence, stop_conditions, anti_patterns, handoff | _none_ |

## Critical signals
`trigger`/`purpose`, `invariants`/`guardrails`, `procedure`, `evidence`

## Phase B2 recommendation
Prioritize the P0 thin Skills: add explicit `## Use when`, keep `## Required procedure`, and ensure `## Required evidence` + `## Non-ownership` are present without boilerplate padding.

## Phase B3 recommendation
For P2 Skills, add missing `authoritative_inputs`, `negative_failure`, and explicit `handoff` sections where the Skill currently relies only on guardrail bullets.

