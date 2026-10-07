---
name: redfish_hardware_monitoring
description: BUILD-032 Redfish/BMC/OOB hardware monitoring: identity, inventory, power/thermal/storage health and OEM compatibility without collapsing BMC and OS health.
---

# redfish_hardware_monitoring

## Purpose

Guide Redfish/BMC/OOB hardware monitoring (BUILD-032): chassis/system identity, inventory, CPU/memory where supported, PSU/fans/thermal, physical storage/RAID where exposed, firmware/health/lifecycle, API versions and OEM extensions—with strict separation of BMC evidence from OS health.

## Use when

iDRAC/iLO/CIMC/Redfish BMC collection, hardware inventory/health, OOB firmware/lifecycle evidence, or partial subsystem failure. Primary owner `platform-infrastructure-cloud-engineer`; conditional collaborator `platform-device-pack-engineer`.

## Authoritative inputs

Always read:

1. BUILD-032 scope and Mother References for hardware/OOB
2. `references/redfish-identity-health.md`, `references/oem-version-compatibility.md`
3. Credential Profiles / TLS policy for BMC endpoints
4. Device Pack dimensional claims when pack-backed (listed ≠ certified)
5. Fixture cases under `fixtures/` for success, auth failure, partial subsystem, stale evidence

Do not copy third-party Redfish manuals or MCP materials verbatim.

## Invariants

- **BMC reachable ≠ OS healthy.**
- **BMC unavailable ≠ OS DOWN.**
- Endpoint OS evidence does not become chassis/power/RAID/thermal truth.
- Chassis/system identity is distinct from host Agent identity; reconcile explicitly—never silent merge.
- Inventory/health must carry source provenance, API version and OEM extension markers.
- Partial subsystem failure (e.g., one PSU) must not collapse into generic host DOWN.
- Stale BMC evidence must surface as STALE/UNKNOWN class—not invented freshness.
- Auth failures and TLS trust failures are collector/security evidence, not hardware fault claims.
- Vendor OEM extensions are best-effort and version-gated.

## Required procedure

1. Confirm Redfish/BMC target identity and credential profile.
2. Record Redfish protocol/API version and OEM product family.
3. Collect chassis/system inventory with provenance.
4. Map power/fan/thermal/storage sensors with subsystem scope.
5. Separate BMC collector health from OS monitoring state.
6. Handle partial failures and stale timestamps explicitly.
7. Collaborate with Device Pack engineer when pack mappings/certification are in play.
8. Never assert OS DOWN solely from BMC unreachable.

## Negative / failure cases

- Marking server DOWN because Redfish timed out while OS Agent is healthy.
- Using OS metrics as RAID/PSU truth.
- Claiming universal OEM coverage from one iDRAC version.
- Logging BMC passwords.
- Treating firmware string presence as certified update capability.

## Security / tenant implications

BMC credentials are high privilege. Use Credential Profiles, TLS verification, tenant scope and redaction. Do not expose BMC management from browser-direct paths.

## Required evidence

- Identity/provenance/API version
- Subsystem health with partial-failure cases
- BMC vs OS separation tests
- Exact SHA

## Stop conditions

Stop when:

- BMC and OS health are being collapsed
- Uncertified OEM behavior is claimed universal
- Third-party Redfish text is copied verbatim
- Scope expands into DCIM/BMS productization

## Anti-patterns

- “BMC ping failed ⇒ server down”
- Silent inventory merge on serial collision
- Writer self-certifying hardware pack coverage

## Handoff / output

Return: BMC identity model; subsystem coverage; OEM/version gaps; nextOwner (infra/device-pack verifier). Not MERGE_READY alone.

## Non-ownership

Does not own OS monitoring (`linux_systems`/`windows_monitoring`), Device Pack schema authority, or DCIM/BMS. Does not authorize firmware upgrade automation by implication.
