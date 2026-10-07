# Skill Router — Capability Pack v1.3

Canonical skills live only under `project_agents/skills/`. Generated copies under
`.agents/skills/` (Cursor + Antigravity) and `skills/` (legacy mirror) are not a
second source of truth.

Routing is:

`task intent → owning agent → required Skills → risk-triggered reviewers → writerTool → verifierTool → evidence gate`

Do not load every Skill body into context. Resolve names from this router and
`AGENT_SKILL_MATRIX.md`, then open only the Skill files that apply.

## Intent → Skill

| Intent / risk | Canonical Skill |
|---|---|
| Stale main/head, unknown PR base | `source_truth_rebaseline` |
| Mother Reference / requirement conflict | `mother_reference_reconciliation` |
| Symptom-only patch | `root_cause_first` |
| Scope creep / drive-by refactor | `bounded_implementation` |
| Schema/data change | `migration_safety` |
| Auth/tenant/secret negative cases | `security_negative_testing` |
| Crash/retry/timeout/durability | `runtime_failure_injection` |
| Alert quality / telemetry trust | `alert_quality_evidence` |
| LLM/GNN claims or tool use | `ai_evidence_safety` |
| Merge-ready / exact-head CI | `exact_head_merge_gate` |
| Typed claims package | `evidence_bundle` |
| After authorized merge | `post_merge_rebaseline` |
| Queues, failover, backpressure, split-brain | `distributed_systems` |
| Sensor workers/spool/uploader/leases | `sensor_edge` |
| Active/Standby Core failover | `ha_dr` |
| Hot-path capacity / cardinality | `capacity_performance` |
| `sensor/` or `agent/` Go | `go_runtime` |
| `core/` Java 21 / Spring | `java_spring` |
| `ui/` React/TypeScript implementation | `typescript_react` |
| Operator job / IA / NOC / dense-data UX | `ux_operational_design` |
| Design-system use, component consistency, responsive/state/performance | `web_interface_quality` |
| Keyboard/focus, semantics, contrast, reduced motion | `accessibility_inclusive_design` |
| Approved Python workers | `python_runtime` |
| SNMP/MIB | `snmp_mib` |
| ICMP / path | `icmp_path` |
| Flow | `flow_telemetry` |
| Syslog / traps | `syslog_traps` |
| SSH/CLI, NETCONF/YANG, gNMI/gRPC | `device_control_protocols` |
| Topology / routing | `topology_routing` |
| Linux host / systemd | `linux_systems` |
| Windows Agent / Windows targets | `windows_monitoring` |
| Endpoint Agent architecture | `agent_architecture` |
| Virt / storage targets | `virtualization_storage` |
| Kubernetes / containers as **monitored** targets | `kubernetes_monitoring` |
| Cloud-account / OTel import | `cloud_monitoring` |
| PostgreSQL | `postgresql` |
| ClickHouse | `clickhouse` |
| Kafka | `kafka` |
| Replay / idempotency / ack | `replay_idempotency` |
| Backup / restore / integrity | `backup_restore` |
| API / event contracts, REST/gRPC | `api_event_contracts` |
| Connector framework | `connector_framework` |
| OAuth/OIDC clients | `oauth_oidc_clients` |
| ITSM / notifications / change feeds | `itsm_change_telemetry` |
| Threat model | `threat_modelling` |
| Tenant isolation / RBAC | `tenant_isolation` |
| PKI / mTLS | `pki_mtls` |
| Secrets / credentials | `secrets_handling` |
| Privileged NCM / remediation | `remediation_safety` |
| NCM config acquisition/diff/drift/Change Plan evidence | `ncm_configuration_assurance` |
| Redfish/BMC/OOB hardware | `redfish_hardware_monitoring` |
| Customer PostgreSQL/MSSQL monitoring targets | `database_monitoring` |
| Docker Compose | `docker_compose` |
| CI/CD | `ci_cd` |
| SBOM / provenance | `sbom_provenance` |
| Install / upgrade / packaging / systemd | `install_upgrade` |
| Device Pack schema | `device_pack_authoring` |
| Work-unit / Sensor placement | `monitoring_work_units` |
| Temporal graph / topology confidence | `temporal_operational_graph` |
| RCA fusion / INCONCLUSIVE | `rca_hypothesis_fusion` |
| Flight recorder / similar-incident memory | `incident_flight_recorder` |
| External change / provider evidence | `change_intelligence` |
| MTTR / customer-value analytics | `customer_value_mttr` |

Overlapping auditor names are classified in `CAPABILITY_BACKLOG.md`. Do not
create a second Skill merely because an auditor listed a synonym.

## Reviewer Skills (risk-triggered, not default)

Add only when the change actually carries that risk:

- Security/auth/tenant: `threat_modelling`, `tenant_isolation`, `security_negative_testing`, `pki_mtls`, `secrets_handling`
- Privileged device action: `remediation_safety`
- AI/LLM: `ai_evidence_safety`
- Alert semantics: `alert_quality_evidence`
- Material UI: `ux_operational_design`, `web_interface_quality`, `accessibility_inclusive_design`
- Migration: `migration_safety`
- Release/install: `install_upgrade`, `sbom_provenance`, `ci_cd`

## Writer / verifier tools

| Role | Tools |
|---|---|
| writerTool | `cursor-cli` is the default bounded implementation writer; `codex` is an additional authorized bounded implementation, test and debugging writer. Neither requires `writerElevation`. Parallel writing requires separate assigned branches/worktrees and non-overlapping file ownership. `antigravity` is not a writer and has no writer-elevation path; `cursor-cloud` and `github-copilot` are prohibited writers |
| verifierTool | `chatgpt-github`, `antigravity` only. Antigravity is the independent read-only verifier. Cursor CLI and Codex are not certification verifiers or gatekeepers. GitHub Copilot remains auxiliary PR review only and never sole certification evidence |
| evidence gate | `exact_head_merge_gate` + `evidence_bundle`; writer cannot close it |

This table summarizes current authority; it does not replace `project_agents/tool_authority.json`, canonical `AGENTS.md`, or `project_documentation/Platform_12_Decision_Source_Register.md` §18.1.

ChatGPT+GitHub does not load Skill bodies unless the selected names require it.
Use `AGENT_SKILL_MATRIX.md` for per-agent required vs conditional Skills.
