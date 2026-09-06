# Platform — CI/CD, Packaging & Distribution Master

**Document ID:** PLATFORM-RELEASEENG-001
**Version:** 3.2
**Status:** **Authoritative Mother Reference — Consolidated Engineering Contract**
**Product:** Platform
**Consolidation rule:** This v3.2 document preserves the normative content of the listed v3.1 source documents. Consolidation reduces file count only; it does not relax, remove or downgrade any requirement.

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

## 4. Windows Agent

Runtime: `PlatformAgent.exe` (name subject to branding freeze), compiled and code-signed. GA installer: code-signed MSI. Convenience wrappers: `install-agent.bat`, `uninstall-agent.bat`, optionally parameterized for silent enterprise deployment. BAT is not the runtime binary.

## 5. Offline bundles

Air-gapped release bundle contains packages, dependencies/repository metadata as required, SBOMs, notices, checksums, signatures/certificates, upgrade/rollback metadata and documentation. No runtime internet download is required.

## 6. Uninstall semantics

Normal uninstall removes binaries/service but preserves customer state/config unless explicit purge is requested. Purge is separately confirmed/audited.
