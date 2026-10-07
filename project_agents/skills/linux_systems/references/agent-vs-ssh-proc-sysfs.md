# Agent vs SSH-agentless; proc/sysfs/systemd

## Collection planes
- Agent-backed: identity via entity_id/agent_id; local proc/sysfs/systemd reads.
- SSH-agentless: explicit capability matrix; permission errors are first-class.

## Evidence sources
- proc/sysfs for CPU/mem/net/disk where authorized; bounded reads.
- systemd unit state is evidence; missing systemd ≠ generic host DOWN.
- Process/service/package evidence uses stable selectors; PID is runtime-only.

## Distros
- Ubuntu vs RHEL path/package differences must be tested or marked UNVERIFIED.
- Permission errors and partial evidence must not invent metrics.

## Critical invariant
Collector failure ≠ host DOWN.
