# Platform Linux Agent — installation notes (BUILD-011 Slice 7 eBPF)

Concise ops guidance for the bounded Linux eBPF process-network backend. Packaging installers remain **BUILD-018**; this document does not certify `.deb`/`.rpm` delivery.

## Prerequisites (optional eBPF enrichment)

Base Agent host metrics and Sensor delivery do **not** require eBPF.

For optional process-network eBPF:

- Linux host with BTF available and readable at `/sys/kernel/btf/vmlinux` (CO-RE path).
- Effective (and permitted/bounding as required by preflight) `CAP_BPF` and `CAP_PERFMON`. UID 0 alone is **not** capability evidence.
- Architecture/OS/kernel within the implementation preflight floor (linux; amd64/arm64 preflight vocabulary). Preflight floor ≠ product certification.
- Feature flag `PLATFORM_AGENT_EBPF_ENABLED=true` when operators intend enrichment evidence.

## Base Agent when eBPF is unsupported

If eBPF is disabled, missing caps, missing/incompatible BTF, unsupported OS/arch/kernel, or `RESOURCE_GUARDED`, the Agent continues scalar collection, durable queue, and Sensor telemetry. eBPF states are enrichment capability evidence only — never authoritative host/process/network DOWN.

## Object and provenance validation

Governed bytecode is embedded (`agent/linux/ebpf/bpf/object/process_net.o`). Load fail-closes on:

- empty/corrupt object;
- ELF/machine/admission gates (`AdmitGovernedObject`);
- program/map allowlists (primary `handle_inet_sock_set_state`; optional `handle_tcp_retransmit_skb`; maps `conn_state`, `events`, `loss_counters`);
- map policy (HASH size / ring size contracts).

There is **no** configurable arbitrary BPF object path. Do not replace the governed object on the target host.

OSS-124 (`cilium/ebpf`) builds via temporary local replace under `agent/third_party/` (`IN_REPO_GOVERNED_REPLACE`). See `agent/third_party/OSS-124-PROVENANCE.md`.

## No target-host BPF compilation

Deployed Agents load the pre-built embedded object. Operators must not install Clang/LLVM on monitored hosts to “rebuild BPF,” and must not compile BPF at Agent start.

## Capability endpoint

Loopback health server (default `127.0.0.1:8089`):

```text
GET /capabilities/process-network-ebpf
```

Returns the capability snapshot (`state`, `reason_codes`, BTF/cap presence, resource guard). Does not alter `/live`, `/ready`, or `/health` schemas.

Until Slice 8 wires `Backend.Start` into AgentService after `RUNNING`, a compatible preflight outcome remains `UNAVAILABLE` / `BACKEND_NOT_WIRED`.

## Verification (engineering)

From the Agent tree in the repository’s Go container / CI conventions (do not install project Go on the developer laptop outside Docker policy):

```text
go vet ./...
go test -race ./...
go build ./linux/cmd
```

Or the Agent Docker image build, which runs Agent-owned static/race gates before producing the non-root runtime image (no compiler in the final image).

## Privilege boundary (install advice)

Grant only what preflight and the loader require: **`CAP_BPF` and `CAP_PERFMON`**.

**Prohibited as eBPF install advice:**

- `--privileged`
- `CAP_SYS_ADMIN`
- `CAP_NET_ADMIN`
- `CAP_NET_RAW`

Do not disable SELinux/AppArmor/Secure Boot or other security controls to force load. Do not broaden the systemd unit beyond the governed contract for this feature.

## BUILD-018 packaging boundary

`agent/linux/systemd/platform-agent.service` is the unit contract. Creating `platform-agent` user/group, shipping `.deb`/`.rpm`, and host enable/reboot certification are **BUILD-018 / host certification**. Cursor Cloud / Docker-only environments cannot complete real systemd install certification.

## Current certification limitations (do not claim)

This slice does **not** claim:

- RHEL, arm64, or Secure Boot product certification;
- BUILD-018 package install certification;
- BUILD-019 Sensor-path claims;
- physical Agent→Sensor structured evidence delivery (still `TRANSPORT_NOT_WIRED` / `UnwiredStructuredEvidenceSink`);
- live production `ACTIVE` attach via AgentService (Slice 8 wiring).

## Support

- Email: support@amrudai.com
- Docs: https://www.amrudai.com/docs

See also [README.md](README.md) and [TROUBLESHOOTING.md](TROUBLESHOOTING.md).
