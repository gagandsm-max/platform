# Redfish identity and health separation

## Identity
- Prefer Redfish ServiceRoot / Chassis / System UUIDs + manufacturer serial where available.
- BMC IP is a mutable locator, not durable entity identity alone.
- Do not silently merge BMC identity with OS Agent entity_id; require explicit reconciliation.

## Health classes
- BMC reachable / auth OK / schema readable
- Subsystem health: power, fans, thermal, storage/RAID (when exposed)
- OS health from Agent/SSH/WinRM—independent plane

## Critical boundaries
- BMC reachable ≠ OS healthy
- BMC unavailable ≠ OS DOWN
- Endpoint OS evidence ≠ chassis/power/RAID/thermal truth
