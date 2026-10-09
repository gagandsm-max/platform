# AmrudAi Linux Agent — eBPF troubleshooting (BUILD-011 Slice 8)

Use loopback `GET /capabilities/process-network-ebpf` and structured logs (`AGENT_EBPF_ACTIVATION_*`, reason codes only — never raw verifier dumps, secrets, or customer payloads). Base AmrudAi Agent host telemetry should keep running when eBPF cannot activate.

## Symptom → reason map

| Observation | Typical `state` / `reason_codes` | Safe action |
|---|---|---|
| Feature off | `UNAVAILABLE` / `FEATURE_DISABLED` | Set `PLATFORM_AGENT_EBPF_ENABLED=true` only if enrichment is intended; leave false otherwise. No kernel backend is constructed or started. |
| Missing BPF/perfmon caps | `PRIVILEGE_DENIED` / `CAP_BPF_MISSING`, `CAP_PERFMON_MISSING` | Grant **only** `CAP_BPF` + `CAP_PERFMON` to the Agent process. Do not use root as a substitute for cap evidence. Base `/ready` must stay healthy. |
| Capability status unreadable | `UNAVAILABLE` or `PRIVILEGE_DENIED` / `CAPABILITY_READ_FAILED` | Fix `/proc/self/status` readability for the Agent; do not disable security frameworks. |
| No BTF | `KERNEL_INCOMPATIBLE` / `BTF_MISSING`, `BTF_UNREADABLE`, `BTF_INCOMPATIBLE` | Use a BTF-enabled kernel with readable `/sys/kernel/btf/vmlinux`. Do not compile BPF on the target. |
| Object admission failure | `UNAVAILABLE` / `OBJECT_INTEGRITY_FAILED` | Use the shipping Agent binary/object. Do not replace the governed BPF object or point at a custom `.o`. |
| Verifier rejection | `UNAVAILABLE` / `BACKEND_VERIFIER_REJECTED` | Collect safe capability snapshot + Agent version/kernel release; escalate to support. Do not patch bytecode on-host. |
| Tracepoint missing | `UNAVAILABLE` / `BACKEND_TRACEPOINT_UNAVAILABLE` | Primary needs `sock/inet_sock_set_state`. Kernel without the governed classic tracepoint cannot run this backend. |
| Attach failure | `UNAVAILABLE` / `BACKEND_ATTACH_FAILED` | Check privilege/tracepoint availability; primary attach failure never becomes `ACTIVE`. |
| Optional retransmit only failed | `ACTIVE` + `OPTIONAL_PROBE_UNAVAILABLE` | Expected independent degradation; primary enrichment may still operate. Check `support_counters.optional_probe_attached`. |
| Map/ring pressure | `RESOURCE_GUARDED` / `RESOURCE_LIMIT` plus loss: `MAP_UPDATE_FAIL`, `RING_RESERVE_FAIL` | Positive reconciled kernel map/ring loss latches enrichment to `RESOURCE_GUARDED` with `PARTIAL` quality evidence; base Agent `/ready` and host monitoring stay up. Guard remains latched until governed healthy restart (no automatic ACTIVE recovery from “no new drops”). Do not raise privileges to “fix” loss. |
| Decoder rejects | Loss: `DECODER_REJECT` | ABI/size/type/family/address/comm/protocol rejections; count and quality degrade — not host DOWN. |
| Userspace backpressure | Loss: `USERSPACE_CHANNEL_FULL` | Bounded channel drop; do not enlarge by inventing unbounded queues on the host. |
| Transport not wired | Counter: `TRANSPORT_NOT_WIRED` | Expected until BUILD-016/062; physical ProcessNetworkEvidence delivery is absent by design. |
| Resource ceiling / preflight guard | `RESOURCE_GUARDED` / `RESOURCE_LIMIT` | Implementation resource guard at preflight or runtime; reduce contention. Do not disable the guard. Runtime low-water re-entry remains evidence-gated under BUILD-019 / BUILD-130. |
| Runtime backend failure after ACTIVE | `UNAVAILABLE` + typed reason (e.g. `READER_FAILED`) | Capability endpoint updates; Agent lifecycle stays `RUNNING` and base readiness unchanged. |
| Backend stopped | `UNAVAILABLE` / `BACKEND_STOPPED` | After clean stop/cleanup under the Agent whole-service shutdown budget. |
| Shutdown timeout during activation | Agent `FAILED` / `AGENT_SHUTDOWN_TIMEOUT` | Non-cooperative activation did not settle before the subordinate activation sub-budget; backend stop is skipped while scheduler/worker/health teardown still run under the remaining whole-service budget. Do not force-close kernel resources. Restart via systemd. |
| Optional construction failed | `UNAVAILABLE` / `SOURCE_UNAVAILABLE` | Aggregator/backend construction failed; base Agent continues. Inspect construction logs (`AGENT_EBPF_CONSTRUCTION_FAILED`); fix config/object integrity before expecting enrichment. |
| False `EVIDENCE_STALE` while runtime attached | should not occur | Runtime-backed current authority and static `FEATURE_DISABLED` are not age-staled. If seen without a runtime, historical/preflight evidence expired as designed. |

## Cleanup

- Prefer Agent stop/restart via the governed systemd unit (`SIGTERM` graceful stop).
- Backend stop releases ring reader, links, and maps; no bpffs pins are used.
- Do **not** delete durable Agent state under `PLATFORM_AGENT_STATE_DIR` (default `/var/lib/platform/agent`) to “fix” eBPF — that damages telemetry continuity/ACK lineage.

## Safe evidence for support

Collect:

- Capability JSON from `/capabilities/process-network-ebpf` (reason codes, kernel fingerprint fields, cap presence, resource guard, support counters — no raw BTF blob).
- Agent version / service instance id from structured logs.
- Kernel release string already present in the snapshot (not full `dmesg` dumps unless support requests a redacted subset).
- Confirmation that base `/live` and `/ready` remain healthy.

Do **not** attach: private keys, mTLS material, full process listings with cmdline/env, packet captures, or customer application payloads.

## Explicitly prohibited “fixes”

Do **not**:

- run the Agent with `--privileged`;
- add `CAP_SYS_ADMIN`, `CAP_NET_ADMIN`, or `CAP_NET_RAW` for eBPF;
- disable SELinux, AppArmor, Secure Boot, or other security controls to force load;
- replace or hot-patch the governed BPF object on the host;
- compile BPF on the monitored target;
- delete durable Agent queue/state to clear eBPF faults;
- treat eBPF loss or `UNAVAILABLE` as device/host DOWN.

## Support

- Email: support@amrudai.com
- Docs: https://www.amrudai.com/docs

See also [README.md](README.md) and [INSTALLATION.md](INSTALLATION.md).
