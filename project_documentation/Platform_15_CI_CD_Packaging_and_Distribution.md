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

Package installation/profile activation shall register and enable every mandatory runtime service required by the selected deployment profile. Linux services use the approved `systemd` supervision contract with boot enablement, dependency-aware readiness and bounded restart on recoverable failure. When PostgreSQL, ClickHouse, Kafka or Valkey are packaged/deployed as separate mandatory Core services, their service definitions and enablement are part of the Core distribution contract rather than a manual post-install step. Packaging/enablement must preserve component authority/role/fencing/replication/durability/rejoin contracts and must not invent HA failover supervisors or select Patroni/repmgr/Keeper/etcd/Consul/Raft/witness/third-Core mechanisms; BUILD-120–124 own exactly-two-Core HA fencing/failover/rejoin.

Package scripts must be idempotent and must not require a customer to run ad-hoc start commands after a supported reboot or recoverable process crash. Automatic restart must not force-start known-unsafe/corrupt/incompatible local state or treat process existence as recovery success; if safe automatic recovery cannot be established, the component remains FAILED/DEGRADED/RECOVERING with actionable evidence and a governed operator/runbook path. Optional, scheduled or on-demand workers may remain disabled/inactive until their capability or workload requires them. Repeated crash loops must remain observable and diagnosable.

## 4. Windows Agent

Runtime: `PlatformAgent.exe` (name subject to branding freeze), compiled and code-signed. GA installer: code-signed MSI. Convenience wrappers: `install-agent.bat`, `uninstall-agent.bat`, optionally parameterized for silent enterprise deployment. BAT is not the runtime binary.

## 5. Offline bundles

Air-gapped release bundle contains packages, dependencies/repository metadata as required, SBOMs, notices, checksums, signatures/certificates, upgrade/rollback metadata and documentation. No runtime internet download is required.

## 6. Uninstall semantics

Normal uninstall removes binaries/service but preserves customer state/config unless explicit purge is requested. Purge is separately confirmed/audited.

## 7. Linux Agent eBPF packaging and compatibility (conditional)

If and when `github.com/cilium/ebpf` (or another formally approved eBPF loader dependency) is selected and implemented for the Linux Agent:

- **Packaging format:** Pre-compiled eBPF object assets (CO-RE / BTF compliant) are packaged directly inside the Linux Agent `.deb` and `.rpm` packages (e.g. `/usr/lib/platform/agent/ebpf/`).
- **Separation of Base Agent Support vs Certified eBPF Matrix:** Base Linux Agent package installation and base infrastructure telemetry continue across the broader governed distribution matrix (including Ubuntu 22.04 LTS and RHEL 8.x). However, Phase-1 eBPF activation is certified only on **Ubuntu 24.x** and **RHEL 9.x** kernels. Being installable does NOT imply eBPF certification, and BTF availability alone does not imply certification.
- **Graceful degradation on uncertified / incompatible platforms:** On a base-Agent-supported OS outside the certified eBPF matrix (or where the host kernel lacks required eBPF/BTF features or privileges), eBPF instrumentation remains strictly disabled and reports capability state as `UNSUPPORTED` or `KERNEL_INCOMPATIBLE`. Base host metric, systemd, and disk monitoring continue unaffected without crash, panic, or install failure. Expansion of the certified eBPF OS matrix is owned by **BUILD-019** benchmark and test evidence.
- **Signing and provenance:** eBPF probe objects and Go loader binaries are built reproducibly in CI, signed with the release signing key, and catalogued in the release SBOM (SPDX/CycloneDX). No compilation from C sources is required or performed on the target customer host during installation.
- **Air-gapped offline distribution:** The offline release bundle includes all necessary kernel compatibility assets; no external internet repository access is attempted or required at runtime.


---

## 7. v3.3.0 governed Pack and privileged artifact distribution

Capability Packs shipped by Platform are versioned release artifacts with declared provenance, integrity/signing metadata, compatibility range, licence/redistribution evidence, SBOM/third-party notices where applicable and promotion/rollback metadata. A pack cannot become Official Certified merely because it is packaged or signed; certification evidence remains Platform_23/14 authority.

Phase-1.5 firmware images are **customer/vendor-entitled privileged artifacts**, not ordinary Platform dependencies. If Platform provides repository/caching/distribution functions, metadata must preserve source/vendor, exact image/version/model compatibility, entitlement/reference, checksum and vendor signature status where available, acquisition/import actor/time, approval state and immutable content identity. Firmware artifacts are access-controlled, auditable and excluded from general telemetry/support-bundle export. Platform must not redistribute vendor firmware without appropriate rights/entitlement.

Selecting a browser-runtime package for Phase-1.5 synthetics requires separate Platform_07 admission, pinned browser/runtime provenance, SBOM/CVE/licence scans, signed/offline packaging strategy and rollback/upgrade evidence before release.
