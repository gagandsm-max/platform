---
name: linux_systems
description: Linux OS, systemd, process, disk, and host-path semantics for Core/Sensor/Agent packaging and monitoring targets.
---

# linux_systems

## Purpose

Monitor and operate Linux hosts with stable identity, least-privilege collection and honest separation of collector failure from host DOWN—across Agent and remote methods.

## Use when

Linux host monitoring, Agent host metrics, systemd/service checks, filesystem/mount evidence, or packaging/runtime behaviour that depends on Linux semantics. Required for Linux Agent and host-target paths.

## Authoritative inputs

Always read:

1. Existing Linux Agent/collector code on the exact head
2. `agent_architecture` for identity/spool when Agent-backed
3. Packaging/install Skills for Core/Sensor/Agent on Linux
4. Least-privilege / permissions expectations in security docs
5. Distro matrix actually tested (do not assume universal)
6. `references/agent-vs-ssh-proc-sysfs.md` and fixtures/evals for Agent vs SSH, permission errors and partial evidence

If remote-only SSH host monitoring or specific distros are unimplemented, mark `UNVERIFIED`.

## Invariants

- Host identity prefers stable Agent/`entity_id` anchors—not DHCP IP.
- Agent vs remote monitoring paths are declared; capabilities differ.
- CPU/memory/disk/network/process metrics have documented sources (proc/sysfs, etc.).
- Filesystem/mount monitoring handles bind mounts and container host distinction.
- systemd/services: unit state is evidence; missing systemd ≠ generic DOWN without classification.
- Permissions: collectors run least privilege; privilege escalation only where packaging requires and is reviewed.
- proc/sysfs parsing is version-tolerant; kernel differences documented.
- Container host vs guest/container identity must not be confused.
- Reboot detection and time sync (NTP/chrony) are operational evidence classes.
- Log access is scoped; not unbounded sensitive file read.
- Command safety: no arbitrary root shell from monitoring paths.
- **Collector failure ≠ host DOWN** when corroboration policy requires more.
- Distro/version differences are explicit in support claims.

## Required procedure

1. State collection method (Agent/remote) and verify code on this SHA.
2. Confirm host identity rules and reboot/time-sync signals.
3. Define metric sources and units; test on at least the claimed distros or mark `UNVERIFIED`.
4. Handle mounts/disk: capacity, inodes, read-only; respect 80% themes where applicable to Platform nodes.
5. systemd: map active/failed/inactive without collapsing to DOWN incorrectly.
6. Enforce least privilege; review file ACLs for Agent.
7. Separate Agent disconnect / permission errors from host operational failure.
8. Forbid arbitrary command execution unless device_control/remediation allowlisted.

## Negative / failure cases

- Host DOWN solely because Agent stopped (without policy).
- Parsing container cgroup CPU as bare-metal host capacity wrongly.
- Reading `/etc/shadow` for “inventory.”
- Assuming Debian paths on RHEL without tests.

## Security / tenant implications

Host Agents are privileged endpoints—tenant-bound. Minimize collected process command lines if secrets appear. Support bundles exclude key material. SELinux/AppArmor contexts respected.

## Required evidence

- Method/distro matrix + `UNVERIFIED`
- Identity and DOWN-separation tests
- Permission model notes
- Exact SHA

## Stop conditions

Stop when:

- IP used as sole host identity
- Collector errors map straight to DOWN improperly
- Arbitrary root execution introduced
- Distro support over-claimed

## Anti-patterns

- “It works on Ubuntu 22 so all Linux is fine”
- Disabling Agent sandboxing for metrics
- Writer self-certifying Linux coverage estate-wide

## Handoff / output

Return: collection model; identity notes; gaps; nextOwner (Agent/linux verifier). Not MERGE_READY alone.

## Non-ownership

Does not own Windows (`windows_monitoring`), K8s, or invent full EDR. Owns Linux host monitoring discipline.
