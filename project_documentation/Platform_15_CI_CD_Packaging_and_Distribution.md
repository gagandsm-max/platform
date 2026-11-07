# Platform — CI/CD, Packaging & Distribution Master

**Document ID:** PLATFORM-RELEASEENG-001
**Version:** 3.3.0
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2.1 document preserves the normative content of the listed v3.1 source documents (via v3.2). Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

## 1. Purpose

Defines reproducible CI/CD, SBOM/licence/security gates, signing, artifact promotion, `.deb` and `.rpm`
packaging for Core/Sensor/Linux Agent, Windows Agent EXE/MSI/BAT packaging, filesystem/service identities,
offline/air-gapped bundles and uninstall semantics.

## 2. Source-document provenance

- `Platform_16_CI_CD_and_Release_Engineering.md` (v3.1) — fully incorporated below.
- `Platform_18_Packaging_and_Distribution.md` (v3.1) — fully incorporated below.

## 3. Platform — CI/CD & Release Engineering

> Consolidated from `Platform_16_CI_CD_and_Release_Engineering.md` v3.1. Requirements in this section remain normative.

## 1. Pipeline stages

Preflight → compile/build → unit → static analysis → OSS/license scan → SBOM → component tests → integration/contract → security → package → package install tests → performance smoke → sign → publish candidate → environment promotion → release evidence bundle.

CI provider selection is **not** frozen by the GitHub source-of-record decision. Pipelines may run on GitHub Actions or another approved CI system after explicit evaluation; this document does not claim GitHub Actions is a fixed architecture decision.

## 1.1 Source-of-record and change flow

Canonical repository: `https://github.com/gagandsm-max/platform`.
Material code, packaging and mother-document changes enter through short-lived branches and pull requests. Do not push normal development changes directly to the protected/default branch. Do not force-push shared/default branches. Release artifacts and documentation authority follow merged GitHub history.

## 2. Reproducibility

Build containers and base images are pinned. Language dependency lockfiles are mandatory. CI must record source commit, dependency graph, build image digest, compiler/runtime versions, package checksums and signing metadata.

## 3. Artifacts

Separate versioned artifacts: Core `.deb/.rpm`, Sensor `.deb/.rpm`, Linux Agent `.deb/.rpm`, Windows Agent EXE/MSI/BAT wrappers, optional container images for engineering/test, SBOMs, notices, checksums, provenance and release notes.

## 4. Signing

Release packages and Windows binaries/installers are signed using controlled release credentials. Signing keys do not live in source repositories or general developer containers.

## 5. Promotion

Development → internal alpha → beta/pilot → release candidate → GA. Promotion is evidence-driven and governed by `Platform_14_Test_Performance_and_Release_Quality.md`.

## 6. OSS/security gates

ORT/Syft/Trivy/ScanCode or approved equivalents run according to the OSS catalogue. RED dependencies fail the build; YELLOW requires explicit approval. Critical/high vulnerabilities are dispositioned before release.

## 4. Platform — Packaging & Distribution Master

> Consolidated from `Platform_18_Packaging_and_Distribution.md` v3.1. Requirements in this section remain normative.

## 1. Linux artifacts

Core, Sensor and Linux Agent each produce `.deb` and `.rpm`. They are separate packages and may version independently within compatibility rules.

## 2. Linux filesystem contract

Recommended layout:

- binaries/runtime: `/opt/platform/<component>/`
- config: `/etc/platform/<component>/`
- mutable state: `/var/lib/platform/<component>/`
- logs where file logs are needed: `/var/log/platform/<component>/`
- certificates/secret references: protected component-specific state/config path with least privilege

Exact final paths must remain consistent across `.deb` and `.rpm` unless OS conventions require a documented difference.

## 3. Service identities

Separate least-privilege service users for Core/Sensor/Agent as appropriate. No interactive shell unless required for support and explicitly controlled.

### 3.1 Runtime service continuity

**Staged package installation versus deliberate activation are separate states.** A staged/unconfigured native package install places binaries, unit/service definitions, filesystem layout and least-privilege identities on disk in a disabled/inactive state. Staged packages must not silently enable mandatory services, enroll identity, join Sensor pools, or claim an active deployment profile. After a supported host reboot, a staged/unconfigured install remains staged/inactive until valid authorized configuration/enrollment deliberately activates a certified profile. Intentional maintenance/disablement by an authorized operator must also be respected and must not be overridden by package scripts or unsupervised boot enablement.

Once valid authorized configuration/enrollment deliberately activates a certified deployment profile, packaging shall register and enable every mandatory runtime service required by that profile. Linux services use the approved `systemd` supervision contract with boot enablement, dependency-aware readiness and bounded restart on recoverable failure. When PostgreSQL, ClickHouse, Kafka or Valkey are packaged/deployed as separate mandatory Core services, their service definitions and enablement are part of the Core distribution contract rather than a manual post-install step.

**Phase envelope packaging (Issue #200 / #204):** Phase 1 packaging/enablement obligations apply to one authoritative standalone Core plus Sensors/Agents under that standalone topology—including standalone durability, security, Sensor assignment fencing, backup/restore and site-loss DR continuity owners cross-referenced in `Platform_03`, `Platform_16` and `Platform_14`. Phase 1.5 packaging/enablement obligations additionally cover exactly-two-Core Active/Standby HA promotion, fencing, peer-partition fail-closed behavior, former-Active rejoin and survivor-durability contracts. Packaging/enablement must preserve component authority/role/fencing/replication/durability/rejoin contracts and must not invent HA failover supervisors or select Patroni/repmgr/Keeper/etcd/Consul/Raft/witness/third-Core mechanisms; BUILD-120–124 own exactly-two-Core HA fencing/failover/rejoin. A Phase 1 standalone package certificate must not imply Phase 1.5 HA continuity.

Package scripts must be idempotent and must not require a customer to run ad-hoc start commands after a supported reboot or recoverable process crash **once a certified profile is deliberately activated**. Automatic restart must not force-start known-unsafe/corrupt/incompatible local state or treat process existence as recovery success; if safe automatic recovery cannot be established, the component remains FAILED/DEGRADED/RECOVERING with actionable evidence and a governed operator/runbook path. Optional, scheduled or on-demand workers may remain disabled/inactive until their capability or workload requires them. Repeated crash loops must remain observable and diagnosable.

## 4. Windows Agent

Runtime: `PlatformAgent.exe` (name subject to branding freeze), compiled and code-signed. GA installer: code-signed MSI. Convenience wrappers: `install-agent.bat`, `uninstall-agent.bat`, optionally parameterized for silent enterprise deployment. BAT is not the runtime binary.

Windows MSI/SCM packaging (BUILD-018) shall version and attest: exact service identity/rights chosen from the BUILD-014/019 permission matrix, protected spool/config ACLs, install/upgrade/rollback identity and certificate preservation, automatic/delayed start, recovery and failure diagnostics. The MSI must **not** silently add the service identity to Performance Monitor Users or Event Log Readers, rewrite broad SCM/Event Log ACLs, grant `SeDebugPrivilege` or default to `LocalSystem`. Any additional group/channel read grant must be minimal, justified by real token tests, opted into by authorized customer administration, reversible and compatible with domain GPO.

Service-control stop/drain and file replacement shall tolerate Windows sharing/lock contention using bounded retries and Windows Installer/Restart Manager semantics. No package step may delete unacknowledged Agent spool segments or purge enrollment/customer state as a workaround. Suppressed unattended reboot, explicit `3010` reboot-required (where returned), safe post-reboot resume and negative cases for an irreducible file-in-use condition must be tested; neither MSI success nor silently returned success may falsely guarantee that every deployment is reboot-free. Package logs are secret-safe and actionable.


## 5. Offline bundles

Air-gapped release bundle contains packages, dependencies/repository metadata as required, SBOMs, notices, checksums, signatures/certificates, upgrade/rollback metadata and documentation. No runtime internet download is required.

## 6. Uninstall semantics

Normal uninstall removes binaries/service but preserves customer state/config unless explicit purge is requested. Purge is separately confirmed/audited.

## 7. Linux Agent eBPF packaging and compatibility (conditional)

`github.com/cilium/ebpf` (`OSS-124`) is engineering-admitted under `Platform_07` Decision=`USE/WRAP` / Truth state=`CURRENT_RUNTIME` for the merged optional bounded Linux source path (`agent/go.mod`, `agent/linux/ebpf/`, `agent/linux/cmd/optional_ebpf.go`). Physical structured Agent evidence delivery remains a separate BUILD-016/062 gate. **Engineering dependency admission is not release/runtime certification** and does not authorize customer support claims, signed-package completeness claims, or certified-matrix expansion by itself.

For release packaging of the implemented optional OSS-124 path (or a separately admitted replacement), the following remain required acceptance gates rather than a statement that package/runtime certification has passed:

- **Packaging format:** Pre-compiled eBPF object assets (CO-RE / BTF compliant) are packaged directly inside the Linux Agent `.deb` and `.rpm` packages (e.g. `/usr/lib/platform/agent/ebpf/`).
- **Separation of Base Agent Support vs Certified eBPF Matrix:** Base Linux Agent package installation and base infrastructure telemetry continue across the broader governed distribution matrix (including Ubuntu 22.04 LTS and RHEL 8.x). The Phase-1 eBPF certification target is **Ubuntu 24.x** and **RHEL 9.x** kernels; exact supported kernels/architectures require BUILD-018/019/130/134 evidence before any certified-matrix claim. Being installable does NOT imply eBPF certification, and BTF availability alone does not imply certification.
- **Graceful degradation on uncertified / incompatible platforms:** On a base-Agent-supported OS outside the certified eBPF matrix (or where the host kernel lacks required eBPF/BTF features or privileges), eBPF instrumentation remains strictly disabled and reports capability state as `UNSUPPORTED` or `KERNEL_INCOMPATIBLE`. Base host metric, systemd, and disk monitoring continue unaffected without crash, panic, or install failure. Expansion of the certified eBPF OS matrix is owned by **BUILD-019** benchmark and test evidence.
- **Signing and provenance:** eBPF probe objects and Go loader binaries are built reproducibly in CI, signed with the release signing key, and catalogued in the release SBOM (SPDX/CycloneDX). No compilation from C sources is required or performed on the target customer host during installation. Engineering admission does not claim these release signing/SBOM gates are already complete.
- **Air-gapped offline distribution:** The offline release bundle includes all necessary kernel compatibility assets; no external internet repository access is attempted or required at runtime.

### 7.1 Native base-Linux Agent legacy qualification targets (Issue #189)

Issue #189 (2026-11-04 Linux-only sequencing decision) promotes the following **native base-Agent** legacy qualification targets into the current Linux completion campaign while retaining modern certification profiles. Status for each exact profile remains **`TARGET` / `CERTIFICATION_PENDING`** until dimensional evidence closes the profile. These statements do **not** claim blanket old-Linux support, do **not** require eBPF on legacy systems, do **not** substitute agentless monitoring for native-Agent completion, do **not** weaken TLS/certificate validation or modern service hardening, and do **not** authorize installing obsolete compilers/toolchains on the developer host.

| Profile | Arch | Status | Notes |
|---|---|---|---|
| Ubuntu 16.04 LTS | amd64 | `TARGET` / `CERTIFICATION_PENDING` | Native base-Agent qualification target; eBPF not required and remains `UNSUPPORTED`/`KERNEL_INCOMPATIBLE` where applicable. |
| RHEL/CentOS 7.x | amd64 | `TARGET` / `CERTIFICATION_PENDING` | Individually pinned and tested distro minors and vendor revisions only; family-level naming is not a certificate. |

**Modern profiles retained (not replaced):** existing modern Linux Agent certification targets remain required, including Ubuntu 22.04 LTS / 24.x and RHEL 8.x / 9.x base/eBPF separation above. Broader legacy distribution coverage from the Issue #189 Phase 1.5 Legacy Agent family remains explicit and unclaimed until each profile passes its own dimensional gate. Cross-reference dimensional certification ownership in `Platform_23` / BUILD-018 packaging / BUILD-019 compatibility; do not duplicate those matrices here.

**Profile dimensions that must appear in each result:** OS family/release; exact pinned minor/image identity and provenance; kernel; libc/runtime; architecture/ISA (conservative amd64 baseline); init/service manager and package-manager versions; filesystem/proc interfaces exercised; TLS/crypto posture; privilege/service-hardening controls actually supported and tested on that profile; vendor maintenance/extended-support condition (recorded as fact, not an external support promise).

**Unresolved exact minor/image blockers (must remain explicit until closed):**
- exact RHEL 7.x and CentOS 7.x minor/image pins and provenance for each candidate result;
- Ubuntu 16.04 LTS exact image/kernel/libc/service-manager pin for each candidate result;
- legacy `systemd` directive compatibility assessment (modern unit directives vs controls actually enforced on the profile; an old manager ignoring an option is not proof of safe sandboxing);
- verified binary dependency set on the conservative amd64 baseline (static compilation alone is not a compatibility certificate);
- signed-package, upgrade/rollback, identity/certificate/spool continuity and mTLS/revocation evidence on each pinned image.

Container fixtures may assist packaging smoke tests but do not replace isolated native VM gates required by Issue #189 for old-kernel service parsing, clean install/configured activation, reboot/crash, upgrade/rollback/remove/reinstall, identity/certificate/spool preservation and end-to-end delivery/recovery.


---

## 7.2 Native package acceptance evidence binding

Native `.deb` / `.rpm` / Windows MSI acceptance evidence for Core, Sensor and Agent packages must bind, for the exact artifact under test:

- source commit SHA and tree SHA of the package inputs;
- builder identity (CI job/runner/image digest or governed equivalent);
- binary and package hashes (and signatures where the release-signing gate applies);
- exact commands executed, true exit codes and sanitized logs;
- staged-install versus deliberately activated lifecycle evidence (see §3.1);
- upgrade and supported rollback paths with identity/certificate continuity and unacknowledged spool continuity;
- permissions/hardening posture for service identity, config, spool and credential paths;
- resource behavior under the declared profile (including non-interference with protected monitoring workloads where applicable);
- relevant failure/recovery evidence (crash-loop visibility, safe-recovery refusal, reboot auto-start after activation).

Container fixtures and staged-only evidence remain bounded helpers and do not by themselves constitute native lifecycle certification. Unsigned internal development package evidence is not release signing certification. Release promotion remains governed by `Platform_14`; operational install/upgrade/rollback runbooks remain owned by `Platform_16`. BUILD-018 remains packaging authority; BUILD-019 remains compatibility certification authority.

---

## 8. v3.3.0 governed Pack and privileged artifact distribution

Capability Packs shipped by Platform are versioned release artifacts with declared provenance, integrity/signing metadata, compatibility range, licence/redistribution evidence, SBOM/third-party notices where applicable and promotion/rollback metadata. A pack cannot become Official Certified merely because it is packaged or signed; certification evidence remains Platform_23/14 authority.

Phase-1.5 firmware images are **customer/vendor-entitled privileged artifacts**, not ordinary Platform dependencies. If Platform provides repository/caching/distribution functions, metadata must preserve source/vendor, exact image/version/model compatibility, entitlement/reference, checksum and vendor signature status where available, acquisition/import actor/time, approval state and immutable content identity. Firmware artifacts are access-controlled, auditable and excluded from general telemetry/support-bundle export. Platform must not redistribute vendor firmware without appropriate rights/entitlement.

Selecting a browser-runtime package for Phase-1.5 synthetics requires separate Platform_07 admission, pinned browser/runtime provenance, SBOM/CVE/licence scans, signed/offline packaging strategy and rollback/upgrade evidence before release.

---

## Phase 2 Enterprise RFP Readiness Reconciliation — Issue #269

Phase-2 Enterprise/Regulated profiles shall be distributable through the existing signed reproducible native package and offline/air-gap model. New Capability/Integration Packs must carry version/provenance/compatibility metadata and be admitted into release evidence. No Phase-2 customer deployment may require a public SaaS control plane for core monitoring. Full native APM/DPI agent/packet-engine packaging is not part of Phase 2.

---

## Issue #272 packaging/release roadmap

Phase-2.5 native instrumentation, RUM/mobile components and packet-capture/decoder components SHALL be independently versioned, signed, SBOM-covered and compatibility-tested where they become distributable artifacts.

Offline/air-gap distribution SHALL remain supported for certified regulated profiles.

A Phase-2.5 component SHALL NOT silently auto-update outside the governed package/update/rollback model.

## Current artifact and pipeline truth — 2026-11-06

The pipeline stages above are normative release requirements, not a claim that every stage is automated or passed. The actual four-job workflow and its evidence limits are recorded in Platform_14 under “Current CI evidence boundary”. This correction changes no workflow, runner, dependency, installer or signing configuration.

| Source/configuration paths first | Existing implementation / required release operation | Current boundary |
|---|---|---|
| `agent/linux/systemd/platform-agent.service`; `agent/linux/systemd/platform-agent.env.example` | Current source requires `/etc/platform/agent.env`, starts `/usr/local/bin/platform-agent-linux`, uses `/var/lib/platform/agent` state and writes stdout/stderr to journald as `platform-agent`. | Verified native source profile; it is a documented exception to the generic recommended `/opt/platform/<component>/` layout. Preserve it until a separately reviewed package migration. Source does not prove an installed host or package artifact. |
| `.github/workflows/platform-ci.yml`; `Makefile`; `tests/` | Current static/unit/UI/integration/security operations. | Successful exact-head runs cover only their executed scope. Required release gates absent from those commands remain pending, not silently satisfied. |
| `agent/linux/ebpf/`; `agent/go.mod`; approved OSS-124 provenance | Bounded source loader/probe integration; future reproducible signed object/binary/native-package evidence. | Preserve exact admitted source/backports. Optional source implementation does not certify `.deb/.rpm`, kernel/architecture/Secure Boot/LSM matrices, offline installation, distribution rights or physical evidence delivery. The example object asset path above is a release layout proposal, not proof of a current installed file. |
| BUILD-018/019/126/127/129/134 release artifacts and manifest | Signed Linux packages, Windows MSI/BAT wrappers for the approved mandatory Windows Phase-1/first-GA scope, SBOM/notices/provenance, offline bundle, install/upgrade/rollback and native recovery evidence. | Main source/runtime tests and operator-reported clean package worktree status cannot stand in for these artifacts or acceptance. A bounded Linux package certificate does not waive the Windows/full-product gate. Do not invent an installer invocation when no current accepted builder is present in the reviewed main. |

Each package/release record must name source SHA/tree, artifact hash, target OS/architecture/profile, embedded dependency/probe provenance, signature verification, migration direction, mixed-version compatibility, rollback/restore limits, service identity/paths and executed acceptance evidence. Phase-1 standalone recovery/DR is mandatory; Phase-1.5 HA adds its fencing/replication/rejoin matrix. Whole cross-platform release acceptance remains separate from a bounded Linux package slice.
