# Platform — Installation, Upgrade, Rollback & Operational Runbooks

**Document ID:** PLATFORM-OPSINSTALL-001
**Version:** 3.3.0
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.2 document preserves the normative content of the listed v3.1 source documents (via v3.2) and adds the v3.2.2 enterprise-SNMP operational-runbook supplement. Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

## 1. Purpose

Defines Core/Sensor/Agent installation and enrollment, auto-start and reboot behavior, HA pairing,
upgrade/rollback sequencing, installer diagnostics, and operational runbooks for component failure, storage pressure,
Kafka/PostgreSQL/ClickHouse faults, Sensor/Agent failures, certificate rotation, backup/restore and DR.

## 2. Source-document provenance

- `Platform_17_Installation_Upgrade_Rollback_Runbook.md` (v3.1) — fully incorporated below.
- `Platform_23_Operational_Runbooks.md` (v3.1) — fully incorporated below.

## 3. Platform — Installation, Upgrade & Rollback Runbook

> Consolidated from `Platform_17_Installation_Upgrade_Rollback_Runbook.md` v3.1. Requirements in this section remain normative.

## 1. Core installation

Required release procedure, subject to implemented/signed installer evidence. Install signed native package. Staged/unconfigured package installation is distinct from deliberate authorized activation (see §4 and `Platform_15` §3.1). Wizard/CLI selects the certified topology profile to activate. The installer must not expose uncertified HA activation or improvise promotion/failover. A planned wizard/CLI flow is not evidence that an accepted current installer exists.

### 1.1 Phase 1 — standalone Core installation, upgrade, rollback, restart/reboot and recovery

Phase 1 certifies **one authoritative standalone Core**. Standalone installation, upgrade, rollback, restart/reboot and recovery obligations are mandatory and separate from Phase 1.5 two-Core HA obligations. Phase 1 preserves:

- standalone durability for the certified standalone failure matrix (process/service crash/restart, governed host reboot and other explicitly certified recoverable failures);
- security/hardening and identity/certificate continuity;
- Sensor assignment generation/fencing, lease ownership and stale privileged-work rejection (multi-Sensor estates remain Phase 1 mandatory even with a standalone Core);
- backup/restore of authoritative state/config;
- site-loss DR for the declared Phase-1 DR profile (redeploy/endpoint redirection where applicable).

Catastrophic single-Core/storage destruction is not Phase-1 zero-RPO HA; recovery uses backup/restore and the declared Phase-1 DR profile. HA ≠ DR. A Phase 1 standalone certificate must not imply Core-HA service continuity. Cross-reference `Platform_03`, `Platform_12` Issue #200/#202 entries and `Platform_14` quality gates.

### 1.2 Phase 1.5 — exactly-two-Core HA installation and pairing

Phase 1.5 certifies exactly two full Core nodes Active/Standby. In HA, first Core receives HA/peer addressing, starts as initial Active and waits for second Core. Second Core is installed with peer/current-master information; mutual identity/certificate handshake completes, state synchronization is verified, and one node remains Active while the other is Standby.

Phase 1.5 HA obligations remain mandatory for that envelope and include positive fencing before automatic promotion, peer-partition fail-closed / `SPLIT_BRAIN_PREVENTED` behavior, former-Active rejoin protection, survivor-durability for the certified permanent single-Core HA failure matrix (`ACKED-FOR-DELETION TELEMETRY RPO = 0` for those HA scenarios; distinct from site/DR RPO), Sensor reconnect/re-anchor and push-protocol continuity. Do not invent a third Core, witness, quorum product or new topology. Physical HA mechanisms remain owned by BUILD-120–124 / `HA_DURABILITY_DESIGN_GATE` and are not selected by this operations document.

## 2. Sensor installation

Install Sensor package in staged/inactive form until authorized enrollment/configuration activates it. Provide current Master Core address/enrollment material and role/site/pool inputs. Sensor performs secure handshake, receives identity/certificate/configuration and joins the appropriate Sensor Pool. Core remains authoritative for assignment/policy. Sensor assignment fencing remains Phase 1 mandatory under standalone Core; Core-HA re-anchor continuity is a Phase 1.5 obligation.

## 3. Agent installation

Linux Agent native package or Windows MSI/BAT wrapper installs in staged/inactive form until authorized enrollment activates the service path. After deliberate activation, the Agent enrolls against current Master Core, receives identity and is directed by Core to the appropriate Sensor. Agent retains enough information to reconnect after reboot while Core may revise assignment.

Native base-Linux Agent legacy qualification targets from Issue #189 (**Ubuntu 16.04 LTS amd64** and individually pinned **RHEL/CentOS 7.x amd64**) remain `TARGET` / `CERTIFICATION_PENDING` with profile dimensions and unresolved exact minor/image blockers recorded in `Platform_15` §7.1. Modern Linux/Windows Agent profiles remain required. Operations must not claim blanket old-Linux support, require eBPF on legacy systems, relabel agentless results as native-Agent completion, or weaken TLS/security for compatibility.

## 4. Auto-start and supervised recovery

**Staged versus activated:** a staged/unconfigured package remains disabled/inactive across reboot. Installers must not auto-enable mandatory services solely because the package is present. Once valid authorized configuration/enrollment deliberately activates a certified deployment profile, installers enable every mandatory runtime service required by that profile automatically, while respecting intentional authorized maintenance/disablement.

After activation, Core, Sensor and Linux Agent services use `systemd` boot enablement and bounded supervised recovery; Windows Agent uses automatic SCM start and configured recovery actions. Where PostgreSQL, ClickHouse, Kafka or Valkey run as separate mandatory Core services, they are included in the same automatic enable/start/recovery contract for the activated profile.

After a supported host reboot or recoverable process crash **of an activated profile**, the platform shall restore the required monitoring/control profile without routine customer process-start commands.

**Phase 1 standalone recovery:** bounded supervised restart, dependency-aware readiness, configuration/state continuity, Sensor assignment fencing preservation, standalone deletion-ACK durability for the certified standalone failure matrix, and backup/DR recovery paths remain mandatory. Standalone/non-HA bounded supervised restart must not be deferred to Phase 1.5.

**Phase 1.5 HA recovery:** automatic boot/start and supervised recovery additionally preserve Active/Standby authority/role/fencing/replication/durability/rejoin contracts and must not self-promote Standby, bypass Core Active/Standby fencing, bypass PostgreSQL/Kafka/ClickHouse replication/durability/safe-rejoin rules, create dual-active authority, repeatedly force-start known-unsafe local state, or treat process existence as recovery success. BUILD-120–124 own exactly-two-Core HA authority/fencing and stateful failover/rejoin.

If safe automatic recovery cannot be established in either envelope, the component remains FAILED/DEGRADED/RECOVERING with actionable health/diagnostic evidence and a governed operator/runbook path. Dependency-aware health/readiness governs startup order and recovery. Optional, scheduled or on-demand workers need not remain resident when no enabled workload requires them. Repeated crash loops must surface in Platform Health/diagnostics and must not be hidden by infinite restart.

Installation certification requires both cold-reboot auto-start and process-kill/fault-injection recovery tests for the mandatory runtime set of the activated profile, including proof that recovery does not bypass authority/fencing/durability safety applicable to that phase envelope. Staged-only evidence is insufficient for activated-lifecycle certification; see `Platform_15` §7.2 native package acceptance binding.

## 5. Upgrade

Preflight → capacity/free-space check → package signature → state/config backup → drain/standby orchestration (HA only) → package update → schema migration → service start → health verification → compatibility verification → completion.

**Phase 1 standalone upgrades** exercise backup/restore prerequisites, signature/provenance checks, schema migration safety, identity/certificate continuity, unacknowledged spool continuity and post-upgrade health without Core-HA switchover claims.

**Phase 1.5 HA Core upgrades** are rolling where architecture permits: upgrade Standby, verify, controlled switchover, upgrade former Active, with fencing/rejoin safety preserved. Exact data-service upgrade sequencing follows component version compatibility.

## 6. Rollback

Rollback restores compatible package/config/schema state where supported. One-way data migrations require explicit release note and pre-upgrade backup/restore plan. Phase 1 rollback/restore evidence remains mandatory independently of Phase 1.5 HA rollback/rejoin evidence.

## 7. Failure diagnostics

Every installer returns stable error code, writes sanitized install log and points to remediation. Failed install must not leave ambiguous partially registered identities without cleanup/retry guidance. Native package acceptance evidence must bind exact source commit/tree, builder identity, binary/package hashes, commands, true exits and logs, activated lifecycle, upgrade/rollback, identity/certificate continuity, unacknowledged spool continuity, permissions/hardening, resource behavior and relevant failure/recovery evidence per `Platform_15` §7.2. Container fixtures and staged-only evidence remain bounded; unsigned internal development evidence is not release signing certification.

For Windows Agent MSI, `3010`/other actual Windows Installer reboot-required outcomes and blocked in-use binaries must be surfaced to the deployment operator with bounded retry/reboot procedure; an unattended install must not schedule an unapproved forced reboot. After any approved reboot, verify SCM start, identity/cert continuity, spool replay/ACK safety and actual telemetry freshness; MSI success alone is not recovery proof. An irreducible blocked native call/EDR lock requires explicit degraded state and operator path, not an infinite stop wait or silent deletion of queue segments.


## 4. Platform — Operational Runbooks Master

> Consolidated from `Platform_23_Operational_Runbooks.md` v3.1. Requirements in this section remain normative.

## 1. Required runbooks

Core process failure; repeated crash loop; **Phase 1 standalone Core recovery/backup/restore/site-loss DR**; **Phase 1.5 HA failover/fencing/peer-partition/former-Active rejoin** (distinct from Phase 1 standalone recovery); Sensor failure; Agent offline; Kafka unavailable/lag; PostgreSQL fault; ClickHouse fault; Valkey fault; ingestion backlog; Core storage ≥80%; Sensor disk ≥80%; certificate expiry/rotation; ITSM connector failure; **SNMP poll backlog/degradation; SNMPv3 authentication/engine/time-window failure; SNMP credential rotation; GETBULK/tooBig incompatibility; counter reset/discontinuity anomaly; trap/inform storm or receiver failure; MIB/Capability Pack mapping failure; interface-identity reconciliation conflict**; package upgrade failure; backup failure; restore; DR activation; corrupted/invalid Device Pack; licensing expiry; telemetry-quality degradation; support-bundle generation; staged-versus-activated lifecycle misconfiguration.

## 2. Runbook structure

Every runbook states trigger, impact, immediate safety action, diagnostic evidence, commands/UI path, remediation, verification, escalation and audit/support evidence. It must distinguish automated self-heal from operator-required intervention.

## 3. Logging

Support teams should first use Platform Health/Audit/Support Bundle rather than ad-hoc shell access. Shell commands are documented only where necessary and must not expose secrets.


## 4. SNMP operational runbook supplement — v3.2.2

SNMP operational diagnostics use Platform Health, device/profile Monitoring Quality and the in-product SNMP Test before ad-hoc shell tools where possible.

Runbooks shall distinguish at minimum:

- target/path unreachable versus SNMP protocol unavailable;
- timeout versus wrong community/v3 username/auth/privacy/context versus authorization/no-access;
- `unknownEngineID` / `notInTimeWindow` / reboot-time resynchronization;
- `tooBig` / bulk-planning incompatibility versus general timeout;
- unsupported optional OID versus mandatory Pack mapping failure;
- raw counter wrap/reset/discontinuity versus genuine traffic-rate change;
- interface ifIndex renumbering versus genuine interface replacement;
- routine trap burst versus sustained trap storm/receiver overload;
- trap/Inform receiver loss versus monitored-device operational failure;
- Inform durable-acceptance failure versus enrichment/downstream pipeline lag;
- Credential Profile rotation/application drift;
- MIB compile/dependency/provenance failure;
- Sensor worker/backlog/capacity pressure.

Remediation must not insert hard-coded communities or downgrade SNMP security silently. Any temporary legacy mode change is authorized/audited and verified. Routine runbooks do not use SNMP SET; configuration-changing SET belongs to approved NCM/remediation procedure.

Support bundles include sanitized SNMP component health, poll-plan/profile/version identifiers, stable error counts, queue/backlog, last success/failure, trap receiver/storm counters, counter-quality statistics and MIB/Pack version/provenance references, but never community/auth/privacy secret values.


## Timezone / scheduler / HA operations notes — v3.2.2 handoff

- IANA TZDB must be maintained as part of supported platform/runtime updates.
- Host timezone does not define product semantics for telemetry, alerts, SLAs or schedules.
- Schedule and timezone preference changes are auditable.
- Scheduler recovery does not mass-catch-up missed history or fabricate samples.
- Two-Core HA remains a Phase 1.5 certification envelope and remains uncertified until Platform_14 gates pass; operators must not treat documentation as production HA certification and must not treat Phase 1 standalone recovery evidence as HA continuity.
- No new infrastructure service is introduced by this baseline. No third Core, witness, quorum product or alternate HA topology is selected here.

## 5. Linux Agent eBPF and active-path operational runbook supplement — v3.2.2

> **Reconciliation reference:** Issue #142 (Master Chat comments `5909076167` and `5909787672`). Target Phase-1 capabilities are subject to applicable implementation, benchmark, and certification gates.

### 5.1 Linux Agent eBPF operational states and runbooks

The Linux Agent exposes discrete operational capability states for eBPF. Ubuntu 24.x/RHEL 9.x are the governed certification targets, not a claim that every kernel/architecture is already certified; diagnose against the exact admitted source and claimed package profile:

- `UNSUPPORTED`: Host kernel version or configuration falls outside the certified Ubuntu 24.x/RHEL 9.x compatibility matrix or lacks required eBPF/BTF features (e.g. `CONFIG_DEBUG_INFO_BTF` disabled). The Agent continues normal metric/inventory collection; eBPF is disabled without operator intervention.
- `UNAVAILABLE`: Kernel supports eBPF but runtime probe attachment or ELF loading failed. Runbook: inspect the `platform-agent` systemd journal for the configured source profile (`agent/linux/systemd/platform-agent.service` sets `StandardOutput=journal` and `StandardError=journal`); do not assume `/var/log/platform/agent/agent.log` exists. Correlate verifier errors and kernel lockdown/security-module restrictions with the exact admitted package/probe provenance.
- `PRIVILEGE_DENIED`: Agent service lacks required Linux capabilities (`CAP_BPF`, `CAP_PERFMON`, or `CAP_SYS_ADMIN`). Runbook: compare the installed unit and capability state with the exact certified least-privilege package profile. The current source unit does not set AmbientCapabilities; do not paste a capability override, enable eBPF, add CAP_SYS_ADMIN, or grant root merely to bypass a denial. Preserve safe base monitoring and escalate the missing certified privilege profile with evidence.
- `KERNEL_INCOMPATIBLE`: Kernel BTF layout differs from packaged CO-RE expectations or verifier rejects program on host kernel patchlevel. Runbook: update agent package or submit kernel profile; base monitoring continues unaffected.
- `RESOURCE_GUARDED`: Diagnose the actual capability reason and resource budget. At source `36e1513f`, `agent/linux/ebpf/backend.go` latches ACTIVE → RESOURCE_GUARDED with `RESOURCE_LIMIT` on positive reconciled kernel map/ring loss; zero additional loss does not automatically restore ACTIVE. This state alone does not prove host CPU/RAM exhaustion or implementation of a complete host-pressure controller. Inspect the journal and the source-owned `ProcessNetworkSupportSnapshot` fields in `agent/common/collector/process_network_support.go`: `MapUpdateFail`, `RingReserveFail`, `DecoderReject`, `UserspaceChannelFull`, `TransportNotWired`, `AggregatorFaults`, `ReaderFaults` and `OptionalProbeAttached`. These are diagnostic-source field names, not invented exporter counters. Preserve safe base monitoring and transport, collect workload/cardinality and loss evidence, and use only the governed profile's disable/recovery/restart path. Do not infer successful physical delivery or recommend uncertified threshold/window tuning from this state.

The support-bundle contract MUST include available eBPF capability/reason/resource and loss evidence plus kernel/probe provenance, with explicit missing fields and no packet payloads or environment secrets. This requirement does not claim that the complete Support Bundle orchestration or every exporter field is implemented.

### 5.2 Active path degradation runbook

- **Incomplete traceroutes:** Intermediate hops returning `*` (ICMP time-exceeded suppressed) must not be treated as path failure if destination is reachable. Operators should verify destination probe results and edge status.
- **Hidden hops and ECMP variance:** Alternating fingerprints may reflect ECMP, an actual route change or unqualified/inconsistent probes; they prove neither ECMP nor flapping. Inspect `path_identity_quality`, `comparison_eligible`, flow identity, source/vantage, protocol and observation time. Both baseline and current evidence must be qualified and comparable before asserting a path change. Preserve candidate variants and uncertainty; group them as ECMP only when independent evidence supports that interpretation.
- **Multi-vantage contradiction:** Conflicting reachability triggers scoped investigation. Check source/collector health, freshness/clock quality, target identity, credentials, probe protocol/policy and independent destination/path/service evidence. Preserve source and endpoint alternatives as well as routing/partition hypotheses; a silent intermediate hop is not a forwarding-failure proof. The incident console may propose a fault domain only with qualified corroboration and retained contradictions. Unresolved evidence remains `INCONCLUSIVE`, not an inferred fault domain or target DOWN.

---

## Phase-1 endpoint operations runbook extension — Issues #169/#170

Operational runbooks shall include the following new failure classes without creating new resident services:

- **Software inventory source degraded/unsupported/permission denied:** preserve prior authoritative current state, mark freshness/quality degraded, do not emit mass uninstall events, and retry/reconcile on the governed schedule.
- **Software inventory reconciliation backlog/cardinality pressure:** throttle/spread lower-priority inventory work before routine monitoring or telemetry durability is affected; expose backlog and last successful authoritative reconciliation.
- **Process/service selector ambiguity or PID reuse:** do not silently retarget; mark review/unsupported selector state and preserve the last valid monitor definition until an authorized correction.
- **Criticality/Profile capacity denial:** keep the previous effective profile, surface projected capacity/MWU impact and reason, and do not partially apply a bulk profile expansion.
- **EDS suppressed/partial/resource guarded:** show the diagnostic definition, trigger, missing evidence and suppression/resource reason; routine monitoring remains authoritative and continues where safe.
- **Cross-vantage evidence unavailable:** retain endpoint EDS and other available evidence independently; do not convert missing OOB/VMware/storage/path evidence into confirmation of a cause.

Support evidence shall expose the last inventory reconciliation, effective process/service monitor selector, effective Monitoring Profile source/mode, EDS job/result status and related evidence-window references without exposing credentials/secrets.

Windows-specific support runbooks (BUILD-014/015/016/018/019) shall distinguish: missing PDH counter vs. localized path vs. corrupted/hung PerfLib; inaccessible Event Log channel vs. event absence; PPL/process-path access denial; machine-registry view failure vs. genuine uninstall; trigger-start/DelayedAutoStart service behavior; stale Windows Time quality; EDR/NTFS sharing violation; blocked SAN/volume collection; Agent spool pending/un-ACKed state; and Windows MSI/SCM stop/reboot-required recovery. Each gives permission-safe diagnostic steps, bounded retry/backoff, escalation and a negative-proof rule against declaring endpoint DOWN or data loss from missing evidence alone.


No installation/upgrade change is required merely by these logical features beyond their owning future BUILDs; BUILD-018/126 remain packaging/install authorities.


---

## 13. v3.3.0 operational runbook additions

### 13.1 OTLP source degradation/backpressure

Operations must expose per source/tenant/ingress profile authentication failures, accepted/rejected/dropped records, cardinality interventions, queue depth/oldest age, retries/replay, Kafka/ClickHouse lag and storage pressure. Runbooks distinguish telemetry-source failure from monitored application/service failure. Under pressure, bounded OTLP enrichment is throttled/shed according to policy before protected monitoring/control/durability is impaired; silent loss is prohibited.

### 13.2 Integration Service failure and recovery

Runbooks cover vendor outage, credential expiry/revocation, webhook authentication failure/replay flood, 429/rate limiting, mapping/required-field failure, unsupported transition, inbox/outbox backlog, poison payload/quarantine/DLQ, loop suppression, wrong-CI/conflict queue and restart/checkpoint recovery. Replay/drain must preserve idempotency and vendor rate budgets; recovering from outage must not generate a thundering herd. Integration degradation never blocks core monitoring or changes observed target state.

### 13.3 CMDB reconciliation

Operations expose connector/binding counts, match-confidence/conflict states, stale external versions, incremental-sync checkpoint/lag, relationship reconciliation health and pending operator conflicts. External CI deletion/missing state does not automatically delete a Platform entity; decommission remains governed.

### 13.4 NCM Change Assurance

Runbooks cover stale/failed backup, precheck failure, external approval unavailable/rejected/stale, plan mutation, canary/batch stop, postcheck/stabilization failure, unsupported rollback, partial rollback and escalation. A failed rollback must preserve evidence and stop further destructive rollout. Maintenance/suppression is always explicitly ended or left in a visible failed/attention state; no silent indefinite maintenance.

### 13.5 Firmware — Phase 1.5

Runbooks cover image repository/provenance/integrity failure, entitlement/compatibility/intermediate-version uncertainty, insufficient flash/storage, transfer interruption, reboot timeout, canary failure, post-upgrade health failure and rollback unavailable/failed. Unsafe or uncertified rollback escalates rather than guessing.

### 13.6 Browser journeys — Phase 1.5

Before browser runtime release, operations must define browser worker crash cleanup, stale process cleanup, credential/token expiry, screenshot/artifact TTL/access/redaction, site/Sensor capacity exhaustion and degraded-mode behavior that cannot starve base monitoring.

---

## Phase 2 Enterprise RFP Readiness Reconciliation — Issue #269

Phase-2 operational readiness adds runbooks for: Capability Pack incompatibility/degradation, Integration Pack auth/rate-limit/backlog/schema failure, CMDB reconciliation conflict, external APM/NPM/DPI evidence staleness, cloud-provider throttling, storage/SAN/database/middleware/backup connector failure, regulated data-egress violation, archive/WORM failure and enterprise retention pressure.

Install/upgrade must preserve pack/connector compatibility and provide preflight before activating a new pack version. Unsupported versions fail visibly rather than silently degrading coverage.

---

## Issue #272 operational planning

Phase-2.5 operations SHALL include explicit install/upgrade/rollback/runbook coverage for application instrumentation agents/SDKs, browser/mobile instrumentation where used, profile collectors, packet capture sources/sensors, decoder/content compatibility, trace/profile/packet storage, privacy/redaction policies and TLS-decryption/key-custody configuration where explicitly enabled.

Failure of deep-observability components MUST degrade visibility, not silently change authoritative device/service state or bypass safe RCA evidence rules.

---

## Infrastructure/Log Sensor safe operations — 2026-10-30 architecture reconciliation

For certified Syslog/TLS, OTLP, SSH/WinRM, vSphere/cloud, DB/storage and SIEM profiles, deployment/runbooks MUST document trusted ingress profile-to-tenant/source assignment, listener/interface/NAT and TLS certificate mapping, source Syslog framing, credential rotation/expiry, connector query privilege, vendor quota discovery, target-side impact baselines and alert/monitoring-quality interpretations. An unresolved/ambiguous source must remain denied or sanitized metadata-only; an MSP cannot enable a shared Syslog listener on overlapping private networks without proving deterministic attribution.

**Producer continuity:** Prefer local durable file/journald or managed asynchronous shipping to the Log Sensor. Synchronous network appenders can block if TCP receiver/relay slows; operators must test source-side backpressure, finite buffer/loss policies, relay restarts and upstream durability before production activation. Do not promise that a Sensor can always avoid TCP ZeroWindow while retaining every event.

**Spool/export runbooks:** Expose per-Sensor primary replay horizon and 80% storage health, per-source redaction/parser failures and multiline truncation, OTLP rejected/partial-success counts, per-tenant ingress drops, per-SIEM destination backlog/oldest age/expiry/loss and recovery state. Declare actual end-to-end delivery/ACK, retry horizon, privacy/retention/legal-hold/export policy, destination outage drill and operator escalation. Protected audit streams require an explicitly provisioned alternate/authoritative durable path or visible fail-safe; finite disk cannot guarantee infinite no-loss quarantine.

**Agentless maintenance:** Detect and report quota/GPO/permission/TTY/identity/host-key failure separately from guest/VM/database/volume health; rate-govern vCenter inventory resync and cloud 429 recovery with measured budgets. This is a documentation/runbook requirement until its connector/certification tests pass.

## Current operational path and procedure truth — 2026-11-06

The installation/upgrade sections define required product behavior. They are not evidence that a current accepted installer, every domain API or every runbook is executable. Each operation must be classified as CURRENT_SOURCE_PROFILE, SPECIFIED_AWAITING_IMPLEMENTATION, or PROFILE_BLOCKED, with exact source/artifact and acceptance evidence. Current source is not a claim that the operator host is running that profile.

| Paths first | Existing operation/source | Evidence and safe boundary |
|---|---|---|
| `agent/linux/systemd/platform-agent.service`; installed `/usr/local/bin/platform-agent-linux`; `/etc/platform/agent.env`; `/var/lib/platform/agent` | Linux service identity `platform-agent`; required environment file; bounded restart; journal stdout/stderr. | CURRENT_SOURCE_PROFILE. Diagnose the installed unit/state/journal before remedy. Never assume the generic `/opt` recommendation or a file log overrides this source. No host service was inspected or changed by the documentation correction. |
| `scripts/generate-dev-certs.sh`; `deploy/docker/certs/` | Development PKI generation overwrites CA/private-key paths. | Existing operation with destructive identity effects. It is not a safe reboot/PKI recovery procedure and not a production enrollment mechanism. Preserve identities and use the applicable accepted recovery contract. |
| `Makefile`; `.env.example`; `.env`; `compose.yaml` | Makefile environment bootstrap occurs while parsing; Compose has dev/test/ha/scale/security/dr profiles. | A help/profile invocation is not necessarily read-only, and a profile name does not prove DR, HA or capacity recovery. Diagnose from source for documentation audits. |
| Native Core/Sensor/Windows package, backup/restore/upgrade and HA procedures | Required behavior in Platform_15/16 and existing BUILD-018/019/120–129/134. | SPECIFIED_AWAITING_IMPLEMENTATION or PROFILE_BLOCKED until the exact supported artifact/OS/profile and executable procedure are accepted. Do not substitute invented commands or manual database/volume deletion. |

Every completed runbook must include trigger and affected profile, preconditions/authority, paths and existing command or UI action, expected diagnostic result, safe remedy, rollback/abort boundary, post-recovery data/authority verification, logs/metrics/audit and escalation. No routine “healthy” result may rely only on process existence. For Phase 1 verify standalone recovery and DR; apply Active/Standby promotion/rejoin steps only to the later certified HA profile.

## Storage assurance A+B+C runbook delta — Issue #292

This is a specified procedure and acceptance delta under [Issue #292](https://github.com/gagandsm-max/platform/issues/292) and its [source mapping adjudication](https://github.com/gagandsm-max/platform/issues/292#issuecomment-6576078417); it is not a currently implemented capacity/notification workflow. Reuse the existing Storage Lifecycle, Platform Operations, Notification/ITSM, retention and legal-hold owners. Quality scenarios are in Platform_14 “Storage assurance A+B+C acceptance — Issue #292”; sizing arithmetic and measurement ownership remain in Platform_11 and the existing contract owners.

1. **Establish physical scope and evidence.** Identify tenant/resource, node, physical mount/volume, data classes, actual allocation/used/free, observed time and quality, protected versus eligible bytes, replica/backup/archive copies and authoritative measurement owner. If unavailable or stale, state UNKNOWN; do not infer a reliable exhaustion date or contractual retention coverage.
2. **Explain capacity and thresholds.** Keep the >=20% estimation uncertainty margin separate from the initial adjustable 70% planning goal and fixed non-disableable 80% emergency watermark. Higher measured uncertainty requires a higher multiplier. Existing 70/75/80 operational transitions and approved hysteresis remain visible; 1500 GB for 1200 GB conservative used is at the emergency boundary, not safely below it. Disclose units and allocation rounding.
3. **Diagnose retention/protection conflict before deletion.** Preview requested retention/profile/topology change and physical capacity; customer 180/365-day requirements are scoped contracts, not global defaults. Reclaim only eligible data under the existing audited lifecycle. Preserve held/contractually protected records across partitions/archive/backup. If no eligible reclamation exists, enter CRITICAL with owner, incident/escalation and explicit degraded/ingest fail-safe outcome; do not silently shorten retention, override legal hold or promise unlimited durability.
4. **Notify through existing authorized channels.** After BUILD-086 and tenant/RBAC/audit/connector prerequisites, create one deduplicated operational case per resource/policy, apply configured procurement lead time and escalation/maintenance/ack/reopen/closure policy, and expose actual delivery and retry/poison status. Do not treat a queued notification as delivered. 60/30/14/7-day examples are not globally frozen defaults. Missing connector or authorization is actionable failure, not delivery success.
5. **Verify remedy and custody.** After approved expansion, retention change, restore or rollback, remeasure physical capacity, protected/reclaimable accounting, data-age/retrieval obligations, Kafka/ClickHouse replay, Sensor spool horizon, queue/lag/drop metrics and case state. Preserve deletion/audit evidence. Never reset volumes/spools or regenerate identities to hide a recovery defect.

The future executable runbook must bind these steps to accepted source paths/UI/API actions and test evidence; those bindings remain blocked where the storage estimator, measurement authority or notification workflow is absent. This task supplies no unverified host command or customer compliance promise.
