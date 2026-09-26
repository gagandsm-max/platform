---
name: device_pack_authoring
description: Device Pack schema, identification, mappings, versioning, certification/provenance and test-lab semantics.
---

<!-- generated from project_agents/skills/device_pack_authoring/SKILL.md sha256:5ce308979531d9bc2bccb1cafc9fffba22afd34e1ce5de99588b2b0e8c120912 -->

# device_pack_authoring

## Purpose

Author Device Packs with correct identity/version, capability maps and fixture-proven parsers—so packs do not false-classify devices or ship unvalidated OID/API/CLI mappings.

## Use when

Creating/updating Device Packs, identification rules, SNMP/API/CLI maps, derived metrics, pack versioning/provenance or certification lab evidence. Required for `platform-device-pack-engineer`.

## Authoritative inputs

Always read:

1. Device Pack schema and SDK docs in-repo on the exact head
2. `Platform_23` Capability Pack / certification dimensional notes when present
3. `snmp_mib` / `device_control_protocols` Skills for protocol constraints
4. Existing golden fixtures and pack provenance fields
5. OSS catalogue constraints for pack tooling dependencies

If a certification harness dimension is not built, mark `UNVERIFIED`—specification ≠ certification.

## Invariants

- Pack identity/version is explicit; upgrades are compatibility-reviewed.
- Vendor/model/OS matching rules are deterministic and tested; ambiguous matches stay unresolved—not guessed.
- Capabilities declare what is supported (L1/L2/… dimensions when applicable)—no scalar “full support” lie.
- OID/API/CLI definitions live in the pack—not hard-coded ad-hoc in Core.
- Parser validation uses fixtures/golden data; negative tests for malformed payloads.
- Derived metrics state formulas and counter semantics (wrap, gauge vs counter).
- Firmware variance: pack documents tested trains; unknown firmware ⇒ safe fallback/`UNSUPPORTED`.
- Safe fallbacks never invent values; unsupported fields are explicit.
- Pack provenance (author, source, checksum/signature as implemented) is retained.
- **No false device classification** to force a pack match.

## Required procedure

1. Define pack id/version and compatibility window with prior versions.
2. Write identification rules; add positive and near-miss negative fixtures.
3. Map capabilities and protocol definitions; mark gaps `UNSUPPORTED`.
4. Validate parsers against golden + malformed fixtures.
5. Document derived metric math and counter types.
6. Record firmware/OS coverage; define fallback behaviour.
7. Run pack tests in the available lab/harness; do not claim dimensional certification without evidence.
8. Publish provenance metadata required by schema.

## Negative / failure cases

- Matching “Cisco-like” sysObjectID to the wrong pack.
- Shipping OID tables without fixtures.
- Treating missing OID as zero traffic.
- Breaking older Agents/Sensors by incompatible pack major bump without migration notes.

## Security / tenant implications

Packs must not embed credentials. Pack install/update is signed/provenance-checked as implemented. Malicious pack content is a supply-chain risk—follow `sbom_provenance` / AppSec review for new tooling.

## Required evidence

- Fixtures and test results
- Identification positive/negative cases
- Version/compatibility notes
- Provenance fields
- Certification status honest (`UNVERIFIED` vs evidenced)
- Exact SHA

## Stop conditions

Stop when:

- Classification ambiguity would silently pick a pack
- Fixtures/negative tests absent
- Counter semantics undefined for rates
- Provenance requirements unmet

## Anti-patterns

- Copying vendor MIB dumps without validation
- “Works on my lab device” as estate certification
- Writer self-certifying pack GA

## Handoff / output

Return: pack version; test evidence; coverage caveats; nextOwner (pack verifier / network). Not MERGE_READY alone.

## Non-ownership

Does not own Sensor scheduler, invent Core forks for vendors, or declare dimensional certification without harness evidence. Owns pack authoring discipline.
