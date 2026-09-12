# Capability Backlog — Auditor Skill Disposition (v1.2)

The auditor proposed overlapping Skill names. v1.2 does **not** implement every proposed name.
Machine form: `project_agents/skills/auditor_skill_disposition.json`.
Counts below are derived from that file; do not freeze them as permanent validator policy.

- IMPLEMENT_NOW: 60
- MERGE_INTO: 108
- DEFER: 56
- Disposition rows: 224

Canonical Skill inventory remains the IMPLEMENT_NOW set in `manifest.json`.
UI amendment maps ten auditor-proposed UI Skill areas into three canonical Skills via MERGE_INTO. Do not create Skill aliases.

## IMPLEMENT_NOW

| Proposed name | Notes |
|---|---|
| `source_truth_rebaseline` | canonical v1.2 skill |
| `mother_reference_reconciliation` | canonical v1.2 skill |
| `root_cause_first` | canonical v1.2 skill |
| `bounded_implementation` | canonical v1.2 skill |
| `migration_safety` | canonical v1.2 skill |
| `security_negative_testing` | canonical v1.2 skill |
| `runtime_failure_injection` | canonical v1.2 skill |
| `alert_quality_evidence` | canonical v1.2 skill |
| `ai_evidence_safety` | canonical v1.2 skill |
| `exact_head_merge_gate` | canonical v1.2 skill |
| `evidence_bundle` | canonical v1.2 skill |
| `post_merge_rebaseline` | canonical v1.2 skill |
| `distributed_systems` | canonical v1.2 skill |
| `sensor_edge` | canonical v1.2 skill |
| `ha_dr` | canonical v1.2 skill |
| `capacity_performance` | canonical v1.2 skill |
| `go_runtime` | canonical v1.2 skill |
| `java_spring` | canonical v1.2 skill |
| `typescript_react` | canonical v1.2 skill |
| `python_runtime` | canonical v1.2 skill |
| `linux_systems` | canonical v1.2 skill |
| `snmp_mib` | canonical v1.2 skill |
| `icmp_path` | canonical v1.2 skill |
| `flow_telemetry` | canonical v1.2 skill |
| `syslog_traps` | canonical v1.2 skill |
| `device_control_protocols` | canonical v1.2 skill |
| `topology_routing` | canonical v1.2 skill |
| `windows_monitoring` | canonical v1.2 skill |
| `agent_architecture` | canonical v1.2 skill |
| `virtualization_storage` | canonical v1.2 skill |
| `kubernetes_monitoring` | canonical v1.2 skill |
| `cloud_monitoring` | canonical v1.2 skill |
| `postgresql` | canonical v1.2 skill |
| `clickhouse` | canonical v1.2 skill |
| `kafka` | canonical v1.2 skill |
| `replay_idempotency` | canonical v1.2 skill |
| `backup_restore` | canonical v1.2 skill |
| `api_event_contracts` | canonical v1.2 skill |
| `connector_framework` | canonical v1.2 skill |
| `oauth_oidc_clients` | canonical v1.2 skill |
| `itsm_change_telemetry` | canonical v1.2 skill |
| `threat_modelling` | canonical v1.2 skill |
| `tenant_isolation` | canonical v1.2 skill |
| `pki_mtls` | canonical v1.2 skill |
| `secrets_handling` | canonical v1.2 skill |
| `remediation_safety` | canonical v1.2 skill |
| `docker_compose` | canonical v1.2 skill |
| `ci_cd` | canonical v1.2 skill |
| `sbom_provenance` | canonical v1.2 skill |
| `install_upgrade` | canonical v1.2 skill |
| `device_pack_authoring` | canonical v1.2 skill |
| `monitoring_work_units` | canonical v1.2 skill |
| `temporal_operational_graph` | canonical v1.2 skill |
| `rca_hypothesis_fusion` | canonical v1.2 skill |
| `incident_flight_recorder` | canonical v1.2 skill |
| `change_intelligence` | canonical v1.2 skill |
| `customer_value_mttr` | canonical v1.2 skill |
| `ux_operational_design` | canonical v1.2 UI skill |
| `web_interface_quality` | canonical v1.2 UI skill |
| `accessibility_inclusive_design` | canonical v1.2 UI skill |

## MERGE_INTO

| Proposed name | Canonical Skill | Notes |
|---|---|---|
| `core_platform` | `distributed_systems` | overlapping auditor alias |
| `linux_monitoring` | `linux_systems` | overlapping auditor alias |
| `network_services` | `topology_routing` | overlapping auditor alias |
| `ssh_cli` | `device_control_protocols` | overlapping auditor alias |
| `netconf_yang` | `device_control_protocols` | overlapping auditor alias |
| `gnmi_grpc` | `device_control_protocols` | overlapping auditor alias |
| `opentelemetry` | `cloud_monitoring` | overlapping auditor alias |
| `schema_migration` | `migration_safety` | overlapping auditor alias |
| `retention_cardinality` | `backup_restore` | overlapping auditor alias |
| `rest_grpc` | `api_event_contracts` | overlapping auditor alias |
| `rbac` | `tenant_isolation` | overlapping auditor alias |
| `supply_chain` | `sbom_provenance` | overlapping auditor alias |
| `packaging_lifecycle` | `install_upgrade` | overlapping auditor alias |
| `systemd_service_lifecycle` | `install_upgrade` | overlapping auditor alias |
| `device_identification` | `device_pack_authoring` | overlapping auditor alias |
| `sensor_placement` | `monitoring_work_units` | overlapping auditor alias |
| `alert_qualification` | `alert_quality_evidence` | overlapping auditor alias |
| `topology_provenance_confidence` | `temporal_operational_graph` | overlapping auditor alias |
| `similar_incident_memory` | `incident_flight_recorder` | overlapping auditor alias |
| `controlled_remediation` | `remediation_safety` | overlapping auditor alias |
| `unit_testing` | `root_cause_first` | overlapping auditor alias |
| `component_testing` | `root_cause_first` | overlapping auditor alias |
| `contract_testing` | `api_event_contracts` | overlapping auditor alias |
| `integration_testing` | `runtime_failure_injection` | overlapping auditor alias |
| `performance_load` | `capacity_performance` | overlapping auditor alias |
| `chaos_testing` | `runtime_failure_injection` | overlapping auditor alias |
| `package_reboot_testing` | `install_upgrade` | overlapping auditor alias |
| `replay_testing` | `replay_idempotency` | overlapping auditor alias |
| `data_integrity_testing` | `backup_restore` | overlapping auditor alias |
| `security_architecture` | `threat_modelling` | overlapping auditor alias |
| `log_redaction` | `secrets_handling` | overlapping auditor alias |
| `support_bundle_safety` | `secrets_handling` | overlapping auditor alias |
| `mtls_enrollment` | `pki_mtls` | overlapping auditor alias |
| `certificate_expiry` | `pki_mtls` | overlapping auditor alias |
| `keycloak_oidc` | `oauth_oidc_clients` | overlapping auditor alias |
| `notification_center_fanout` | `itsm_change_telemetry` | overlapping auditor alias |
| `webhook_retry` | `connector_framework` | overlapping auditor alias |
| `carrier_status_ingest` | `itsm_change_telemetry` | overlapping auditor alias |
| `saas_connector_auth` | `connector_framework` | overlapping auditor alias |
| `kafka_poison_parking` | `kafka` | overlapping auditor alias |
| `clickhouse_insert_retry` | `clickhouse` | overlapping auditor alias |
| `postgres_tenant_predicates` | `postgresql` | overlapping auditor alias |
| `valkey_ephemeral_only` | `distributed_systems` | overlapping auditor alias |
| `spool_uploader` | `sensor_edge` | overlapping auditor alias |
| `worker_pool_scheduling` | `sensor_edge` | overlapping auditor alias |
| `lease_fencing` | `sensor_edge` | overlapping auditor alias |
| `sensor_self_observability` | `sensor_edge` | overlapping auditor alias |
| `snmpv3_security` | `snmp_mib` | overlapping auditor alias |
| `mib_compilation` | `device_pack_authoring` | overlapping auditor alias |
| `trap_storm_control` | `syslog_traps` | overlapping auditor alias |
| `flow_cardinality` | `flow_telemetry` | overlapping auditor alias |
| `icmp_rate_limit` | `icmp_path` | overlapping auditor alias |
| `windows_msi` | `windows_monitoring` | overlapping auditor alias |
| `linux_agent_identity` | `agent_architecture` | overlapping auditor alias |
| `endpoint_dex_privacy` | `agent_architecture` | overlapping auditor alias |
| `k8s_collector_health` | `kubernetes_monitoring` | overlapping auditor alias |
| `otel_receiver_bounds` | `cloud_monitoring` | overlapping auditor alias |
| `hypervisor_metrics` | `virtualization_storage` | overlapping auditor alias |
| `storage_protection_80` | `backup_restore` | overlapping auditor alias |
| `flyway_retry_assessment` | `migration_safety` | overlapping auditor alias |
| `exactly_once_effects` | `replay_idempotency` | overlapping auditor alias |
| `error_code_stability` | `api_event_contracts` | overlapping auditor alias |
| `correlation_ids` | `api_event_contracts` | overlapping auditor alias |
| `actuator_health_boundary` | `api_event_contracts` | overlapping auditor alias |
| `maker_checker` | `remediation_safety` | overlapping auditor alias |
| `blast_radius` | `remediation_safety` | overlapping auditor alias |
| `ncm_rollback` | `remediation_safety` | overlapping auditor alias |
| `gnn_not_causal` | `rca_hypothesis_fusion` | overlapping auditor alias |
| `inconclusive_rca` | `rca_hypothesis_fusion` | overlapping auditor alias |
| `flight_recorder_bounds` | `incident_flight_recorder` | overlapping auditor alias |
| `packet_evidence_limits` | `incident_flight_recorder` | overlapping auditor alias |
| `mttr_cohort_methodology` | `customer_value_mttr` | overlapping auditor alias |
| `value_dashboard_assumptions` | `customer_value_mttr` | overlapping auditor alias |
| `graph_no_new_database` | `temporal_operational_graph` | overlapping auditor alias |
| `device_pack_certification` | `device_pack_authoring` | overlapping auditor alias |
| `work_unit_licence_preview` | `monitoring_work_units` | overlapping auditor alias |
| `compose_named_volumes` | `docker_compose` | overlapping auditor alias |
| `github_actions_exact_head` | `ci_cd` | overlapping auditor alias |
| `deb_rpm_msi` | `install_upgrade` | overlapping auditor alias |
| `crash_loop_diagnostics` | `install_upgrade` | overlapping auditor alias |
| `java21_only` | `java_spring` | overlapping auditor alias |
| `react_server_authority` | `typescript_react` | overlapping auditor alias |
| `go_context_timeouts` | `go_runtime` | overlapping auditor alias |
| `python_approved_paths_only` | `python_runtime` | overlapping auditor alias |
| `ha_two_core_only` | `ha_dr` | overlapping auditor alias |
| `sensor_continue_on_core_failover` | `ha_dr` | overlapping auditor alias |
| `backpressure` | `distributed_systems` | overlapping auditor alias |
| `split_brain_avoidance` | `ha_dr` | overlapping auditor alias |
| `ui_no_generic_dashboard` | `ux_operational_design` | retargeted from typescript_react; operational specificity lives in UX/design Skills |
| `wcag_operational_screens` | `accessibility_inclusive_design` | retargeted from typescript_react; independent accessibility Skill |
| `nested_exception_leakage` | `security_negative_testing` | overlapping auditor alias |
| `fail_closed_offsets` | `kafka` | overlapping auditor alias |
| `no_llm_privileged_exec` | `remediation_safety` | overlapping auditor alias |
| `no_writer_self_certify` | `exact_head_merge_gate` | overlapping auditor alias |
| `no_force_push` | `bounded_implementation` | overlapping auditor alias |
| `no_direct_main` | `bounded_implementation` | overlapping auditor alias |
| `no_secret_in_git` | `secrets_handling` | overlapping auditor alias |
| `no_docker_prune` | `docker_compose` | overlapping auditor alias |
| `usability_information_architecture` | `ux_operational_design` | UI amendment MERGE_INTO; usability/IA |
| `product_ui_visual_interaction` | `ux_operational_design` | UI amendment MERGE_INTO; product UI visual/interaction design |
| `ux_writing_content_intent` | `ux_operational_design` | UI amendment MERGE_INTO; UX writing/content intent |
| `dashboard_decision_design` | `ux_operational_design` | UI amendment MERGE_INTO; dashboard decision design |
| `noc_operational_ux` | `ux_operational_design` | UI amendment MERGE_INTO; NOC operational UX |
| `large_estate_dense_ui` | `ux_operational_design` | UI amendment MERGE_INTO; large-estate/data-dense UI |
| `design_system_architecture` | `web_interface_quality` | UI amendment MERGE_INTO; design-system architecture |
| `web_visual_runtime_validation` | `web_interface_quality` | UI amendment MERGE_INTO; runtime/visual validation and responsive quality |
| `frontend_state_completeness` | `web_interface_quality` | UI amendment MERGE_INTO; state completeness and frontend performance |
| `inclusive_keyboard_semantics` | `accessibility_inclusive_design` | UI amendment MERGE_INTO; keyboard/focus/semantic/inclusive design |

## DEFER

| Proposed name | Disposition | Notes |
|---|---|---|
| `browser_synthetics_runtime` | DEFER: no runtime/vendor selected; Platform_21 reserved | out of v1.2 bounded library |
| `outside_in_probe_runtime` | DEFER: customer-operated probes not a v1.2 runtime pick | out of v1.2 bounded library |
| `regional_federation` | DEFER: must not reopen two-Core HA | out of v1.2 bounded library |
| `vector_database` | DEFER: OSS/security/performance review required | out of v1.2 bounded library |
| `gpu_gnn_runtime` | DEFER: GNN production runtime is TBD | out of v1.2 bounded library |
| `scim_certification` | DEFER: SCIM is compatibility target until independently validated | out of v1.2 bounded library |
| `contractual_slo_commitments` | DEFER: Founder-reserved | out of v1.2 bounded library |
| `audit_crypto_construction` | DEFER: needs architecture/security review before construction choice | out of v1.2 bounded library |
| `dcim_bms_integration` | DEFER: facilities evidence must not expand product into BMS/DCIM | out of v1.2 bounded library |
| `always_on_dpi` | DEFER: triggered packet evidence does not authorize DPI | out of v1.2 bounded library |
| `employee_surveillance` | DEFER: prohibited; endpoint DEX is operational only | out of v1.2 bounded library |
| `cross_tenant_learning` | DEFER: forbidden unless explicitly authorized | out of v1.2 bounded library |
| `new_graph_database` | DEFER: do not select by default | out of v1.2 bounded library |
| `multi_cloud_control_plane` | DEFER: Platform is not a public-cloud control plane | out of v1.2 bounded library |
| `price_sku_invention` | DEFER: Founder-reserved licensing | out of v1.2 bounded library |
| `prometheus_remote_write_product` | DEFER: not selected as product dependency | out of v1.2 bounded library |
| `opensearch_product` | DEFER: not in current datastore authority | out of v1.2 bounded library |
| `timeseries_db_replacement` | DEFER: ClickHouse remains historical analytics | out of v1.2 bounded library |
| `service_mesh_product` | DEFER: not a Platform Core dependency | out of v1.2 bounded library |
| `ebpf_agent` | DEFER: not selected | out of v1.2 bounded library |
| `wasm_plugins` | DEFER: not selected | out of v1.2 bounded library |
| `mobile_app` | DEFER: out of v1.2 programme | out of v1.2 bounded library |
| `public_rest_sdk` | DEFER: no customer SDK commitment | out of v1.2 bounded library |
| `terraform_provider` | DEFER: policy-as-code inputs cannot bypass RBAC | out of v1.2 bounded library |
| `ansible_collection` | DEFER: NCM owns privileged network change | out of v1.2 bounded library |
| `helm_chart_product` | DEFER: packaging remains deb/rpm/MSI | out of v1.2 bounded library |
| `operator_pattern` | DEFER: k8s is a monitored target, not Core runtime | out of v1.2 bounded library |
| `istio_telemetry` | DEFER: not selected | out of v1.2 bounded library |
| `cilium_hubble` | DEFER: not selected | out of v1.2 bounded library |
| `vault_product_lockin` | DEFER: secret service is Platform-owned abstraction | out of v1.2 bounded library |
| `okta_only_identity` | DEFER: identity remains protocol-based | out of v1.2 bounded library |
| `slack_first_notifications` | DEFER: Notification Center is the product surface | out of v1.2 bounded library |
| `pagerduty_exclusive` | DEFER: connector family, not exclusive vendor | out of v1.2 bounded library |
| `datadog_export` | DEFER: Platform is the monitoring product | out of v1.2 bounded library |
| `grafana_embedding` | DEFER: UI is Platform React, not Grafana | out of v1.2 bounded library |
| `notebook_rca` | DEFER: RCA is in-product and evidence-bound | out of v1.2 bounded library |
| `auto_remediate_by_llm` | DEFER: forbidden privileged LLM execution | out of v1.2 bounded library |
| `model_vendor_lock` | DEFER: AI Gateway is provider-independent | out of v1.2 bounded library |
| `internet_required_monitoring` | DEFER: core monitoring cannot require Internet/LLM | out of v1.2 bounded library |
| `clickhouse_as_control_state` | DEFER: forbidden | out of v1.2 bounded library |
| `valkey_as_authority` | DEFER: forbidden | out of v1.2 bounded library |
| `kafka_external_api` | DEFER: Kafka stays internal | out of v1.2 bounded library |
| `port_80_ui` | DEFER: forbidden | out of v1.2 bounded library |
| `snmp_alone_device_down` | DEFER: forbidden | out of v1.2 bounded library |
| `universal_alert_threshold` | DEFER: forbidden | out of v1.2 bounded library |
| `down_migrations_required` | DEFER: not implied by Flyway numbering | out of v1.2 bounded library |
| `v1_three_core_ha` | DEFER: forbidden; exactly two Cores | out of v1.2 bounded library |
| `thought_data_reuse` | DEFER: clean-room; forbidden | out of v1.2 bounded library |
| `enterprise360_reuse` | DEFER: forbidden | out of v1.2 bounded library |
| `netai_reuse` | DEFER: forbidden | out of v1.2 bounded library |
| `host_java_install` | DEFER: Docker Compose only for routine tests | out of v1.2 bounded library |
| `cursor_agent_cli_flag` | DEFER: Cursor `--agent` unsupported | out of v1.2 bounded library |
| `antigravity_write_by_default` | DEFER: default posture is read-only/adversarial | out of v1.2 bounded library |
| `codex_skill_path` | DEFER: no invented Codex skill path | out of v1.2 bounded library |
| `federation_runtime` | DEFER: future regional federation is not a v1.2 pick | out of v1.2 bounded library |
| `probe_browser_engine` | DEFER: browser synthetics runtime not selected | out of v1.2 bounded library |
