# VMware first-GA depth (vCenter/ESXi)

## Scope
- Deep VMware first-GA: vCenter, ESXi, VM identity, datastore, cluster, vMotion, storage vMotion.
- Stale inventory and eventual consistency are expected; do not invent freshness.
- **vCenter unavailable ≠ all VMs DOWN.**
- Test strategy may use vcsim/simulator for contract tests; live certification remains separate.
- Do **not** claim equivalent Hyper-V deep certification in this tranche.

## Identity
- VM identity prefers stable BIOS UUID / instance UUID + vCenter authority binding.
- Datastore/cluster relationships carry provenance/confidence.
