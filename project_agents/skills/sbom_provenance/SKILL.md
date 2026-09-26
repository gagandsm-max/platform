---
name: sbom_provenance
description: SBOM, provenance, CVE/licence review and supply-chain evidence before new OSS.
---

# sbom_provenance

## Purpose

Produce and review Software Bills of Materials, provenance and CVE/licence evidence so Platform only ships approved, attributable artifacts—and rejects unverifiable supply-chain inputs.

## Use when

Adding/updating OSS, releasing packages, generating SBOM/provenance attestations, triaging CVEs, or reviewing transitive dependencies. Required before new OSS merge per `Platform_07` / AGENTS rules.

## Authoritative inputs

Always read:

1. `Platform_07_OSS_Catalog.md` — approved dependency authority
2. Existing SBOM/provenance CI jobs and artefact paths on the exact head
3. Licence policy and CVE triage process in-repo
4. Release/package signing expectations when present
5. Air-gap/offline bundle procedures if applicable

If signing or provenance attestations are not implemented for an artefact class, mark `UNVERIFIED`—do not claim them.

## Invariants

- Dependency inventory covers package ecosystems in use (Maven/npm/Go/etc. as applicable).
- Direct and transitive dependencies are listed with versions.
- Licences are reviewed for catalogue compatibility before merge.
- CVEs are triaged with severity, exploitability and fix/waiver evidence—not ignored.
- Provenance ties artifacts to source SHA/build identity as implemented.
- Hashes/signatures verified when the release process requires them.
- Build artifacts are attributable; unreproducible mystery binaries are rejected.
- Dependency updates re-run SBOM/CVE gates.
- False-positive CVEs need documented rationale—not silent dismiss.
- Air-gap/offline bundles must still carry inventory evidence.
- Unsupported/unverifiable artifacts are rejected.
- Release evidence includes SBOM pointers for the exact version.

## Required procedure

1. Identify ecosystems and manifests touched by the change.
2. Generate/update SBOM for affected components on the exact SHA.
3. Diff direct/transitive additions against `Platform_07`; stop if unapproved.
4. Run licence + CVE scan; triage with waivers only under policy.
5. Verify hashes/signatures/provenance for release artifacts when required.
6. For air-gap: confirm offline inventory completeness.
7. Reject artifacts without verifiable source/provenance.
8. Attach SBOM/CVE evidence to the change; never “fix later.”

## Negative / failure cases

- Vendoring a jar with no licence and calling it fine.
- Claiming CVE-clean while suppressing scanner failures.
- Shipping unsigned release when policy requires signatures (`UNVERIFIED` gap must block claim).
- Updating transitive deps without SBOM regen.

## Security / tenant implications

Supply-chain compromise is a platform-wide tenant risk. Secrets must not appear in SBOMs. Public SBOM publish must not leak private package paths if policy forbids. Waiver records are auditable.

## Required evidence

- SBOM artefact paths
- Licence/CVE triage results
- Provenance/hash verification or `UNVERIFIED`
- `Platform_07` approval refs for new OSS
- Exact SHA

## Stop conditions

Stop when:

- New OSS not in catalogue / unapproved
- Unverifiable binary would ship
- Critical CVE untriaged
- Provenance required but missing

## Anti-patterns

- `--ignore-scripts` style blind installs without review
- Copying lockfiles from untrusted forks
- Writer self-certifying supply-chain safety

## Handoff / output

Return: SBOM/CVE pack; blockers; nextOwner (AppSec / release verifier). Not MERGE_READY alone.

## Non-ownership

Does not own runtime CVE patching SLAs, invent catalogue entries silently, or replace AppSec authority. Owns SBOM/provenance evidence discipline.
