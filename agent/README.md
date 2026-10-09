# AmrudAi Agent runtime

This directory contains the Linux-first AmrudAi Agent runtime governed by Platform v3.3.0.

## Architecture boundary

The authoritative flow is:

1. Operators provision Agent identity (`PLATFORM_AGENT_ID`) and mTLS material. Core enrollment/assignment remains a separate control-plane hop; this runtime does not invent a public enrollment client.
2. Runtime telemetry flows **Agent → assigned Infrastructure Sensor** over HTTPS+mTLS to `POST /api/v1/agent/telemetry`.
3. The Sensor owns downstream Agent spool drain into Core agent-ingress (`POST /api/v1/sensor/agent-telemetry`). Agent never talks to Core, Kafka, or databases for telemetry.

Tenant and Sensor authority are **server-derived**. Agent payload `tenantId` is evidence, not authorization. Sensor ACK `status=ACCEPTED` plus matching `agent_id` / `batch_id` / `accepted_through_sequence` is required before local queue delete.

## Implemented Linux telemetry

The Linux collectors use bounded standard-library `/proc`, `/sys`, and `statfs` access only.

Default registered collectors (`cpu`, `memory`, `filesystem`, `pressure`, `diskio`, `net`, `osfacts`):

- uptime and boot time;
- load averages;
- CPU count and aggregate utilization;
- memory total/available/used/utilization plus swap total/available/used/utilization;
- optional `/proc/vmstat` paging counters when present (`page.faults`, `page.major_faults`, `swap.pages_in`, `swap.pages_out`, `oom_kills`) — missing fields are omitted, not invented as zero;
- bounded mounted-filesystem block capacity and inode total/available/used/utilization with `mount` and `fstype` labels;
- Linux PSI pressure from `/proc/pressure/{cpu,memory,io}` (`avg10`/`avg60`/`avg300`/`total` with `resource`+`scope`, plus per-resource `available`);
- bounded block-device I/O from `/proc/diskstats` (hard ceiling 64 devices; kernel 512-byte sector accounting; `device` label);
- bounded NIC counters from `/proc/net/dev` (hard ceiling 64 interfaces; loopback excluded) plus operstate vocabulary label (`up|down|unknown|dormant|lowerlayerdown|testing|notpresent|other`);
- OS scalar facts: boot time (epoch seconds), processes running, processes blocked.

BUILD-011 Slice 3 adds a **local, unwired** Linux process/service source (`LinuxProcessServiceSource`) that reads bounded `/proc` identity/runtime facts and optional read-only systemd unit evidence. It reuses the Slice-1 selector/catalogue contracts, never registers as a `HostCollector`, never encodes the catalogue into scalar metric labels, and does not claim Agent→Sensor structured delivery (BUILD-016/062). Customer alert/policy evaluation remains BUILD-070.

### Bounded Linux eBPF process-network (BUILD-011 Slice 8)

The **base AmrudAi Agent** (host collectors, durable queue, Sensor delivery, health) is independent of eBPF. Optional process-network enrichment is default-off (`PLATFORM_AGENT_EBPF_ENABLED=false`). When unsupported, privilege-denied, kernel-incompatible, or resource-guarded, the base Agent continues; eBPF never takes precedence over protected workloads.

**Slice 8** wires the certified Linux eBPF `Backend` as an **optional subordinate runtime** of `AgentService`:

1. Base lifecycle reaches `RUNNING` first (`health` → `worker` → `scheduler`).
2. Exactly **one** asynchronous backend activation attempt begins after `RUNNING` (no retry daemon).
3. Activation failure never calls `failStart`, never stops scheduler/delivery, and never makes `/ready` `NOT_READY`.
4. `GET /capabilities/process-network-ebpf` reports **current** dynamic capability (`ACTIVE`, `PRIVILEGE_DENIED`, `KERNEL_INCOMPATIBLE`, `UNAVAILABLE`, `FEATURE_DISABLED`, `BACKEND_STOPPED`, …) plus bounded support counters when a runtime is present. Runtime-backed and static `FEATURE_DISABLED` configuration truth are current authority and are not age-staled to `EVIDENCE_STALE` merely because `ValidUntil` elapsed (no Slice-8 recheck daemon). Historical/preflight-only evidence without a runtime may still become `EVIDENCE_STALE`.
5. Shutdown uses the **single** whole-service budget: cancel runtime → settle optional activation under a subordinate sub-deadline (half the whole budget) → when settled, `Backend.StopContext` with scheduler/worker → health last. If activation remains unsettled, skip unsafe backend stop, continue best-effort scheduler/worker/health teardown under the remaining whole-service deadline, and return a truthful joined `AGENT_SHUTDOWN_TIMEOUT` with lifecycle `FAILED`. Optional eBPF construction failure never terminates base Agent startup.

Intended data path (when activated):

1. Kernel classic tracepoint `sock/inet_sock_set_state` (primary, required for `ACTIVE`).
2. Bounded in-kernel HASH connection state + ring buffer (+ discrete loss counters).
3. Userspace ring reader → fixed ABI decoder → bounded channel/batch.
4. `ProcessNetworkAggregator.Ingest` → `StructuredEvidenceSink`.

Optional probe `tcp/tcp_retransmit_skb` is independently degradable: primary can stay `ACTIVE` with reason `OPTIONAL_PROBE_UNAVAILABLE` if the retransmit program fails to load/attach.

Observable loss/support classes (counted once, never invent “zero loss”): `MAP_UPDATE_FAIL`, `RING_RESERVE_FAIL`, `DECODER_REJECT`, `USERSPACE_CHANNEL_FULL`, plus `TRANSPORT_NOT_WIRED`, reader/aggregator fault counts when exposed on the capability endpoint.

Public capability states remain exactly: `ACTIVE`, `UNSUPPORTED`, `UNAVAILABLE`, `PRIVILEGE_DENIED`, `KERNEL_INCOMPATIBLE`, `RESOURCE_GUARDED`. Positive reconciled kernel `MAP_UPDATE_FAIL` / `RING_RESERVE_FAIL` while `ACTIVE` latches `RESOURCE_GUARDED` (`RESOURCE_LIMIT`); this is enrichment capability evidence only — never host/service/network DOWN.

**Security / data minimization:** least privilege is `CAP_BPF` + `CAP_PERFMON` (or target-kernel equivalent). Do not broaden to `--privileged`, `CAP_SYS_ADMIN`, `CAP_NET_ADMIN`, or `CAP_NET_RAW` for this feature. No payload, TLS contents, HTTP body, SQL text, env vars, or process cmdline. Comm is the bounded kernel `comm` only. Attach surface is classic tracepoints only — no XDP, tc, LSM, socket-filter, kprobe, or uprobe. Object load fail-closes on embedded-object admission/allowlists before kernel map/program creation; no arbitrary object path, no target-host BPF compilation, no bpffs pins.

**Structured transport (still truthful):** default sink is `UnwiredStructuredEvidenceSink` with delivery state `TRANSPORT_NOT_WIRED` and `ErrStructuredTransportNotWired`. Physical Agent→Sensor structured delivery remains BUILD-016/062; Slice 8 does not claim that path. Production provenance for the eBPF aggregator is `agent.linux.process_network.ebpf` (evidence identity only — not tenant/entity authority).

**Non-goals / deferred certifications:** separate eBPF microservice or datastore; always-on DPI/PCAP; full APM; BUILD-018 packaging delivery; BUILD-019 Sensor spillover claims; BUILD-130 capacity certification; RHEL/arm64/Secure Boot certification; physical structured-delivery certification.

**OSS-124:** temporary `IN_REPO_GOVERNED_REPLACE` of `github.com/cilium/ebpf` under `agent/third_party/github.com/cilium/ebpf/` (see `agent/third_party/OSS-124-PROVENANCE.md`). Not a general third-party dumping mechanism.

Ops: [INSTALLATION.md](INSTALLATION.md), [TROUBLESHOOTING.md](TROUBLESHOOTING.md). Support: support@amrudai.com · https://www.amrudai.com/docs

## Durable local buffer

Agent telemetry is persisted before delivery using:

- stable sequence and record IDs;
- durable `sequence.meta` watermarks for `highestSequence`, optional durable `lineageStart`, `acceptedThrough`, and BUILD-002 `agentStreamId` + `legacyCutoverSequence` (metadata `version` 3; legacy/v2 meta migrates once on open and never regenerates the stream id); ACK-authorized leftovers pending deletion are tracked as bounded `acceptedRecords` `{sequence,id}` so restart cleanup matches exact IDs only;
- SHA-256 integrity over sequence, ID, createdAt, and payload;
- restart recovery with content validation (ACK meta is loaded before active mutate/quarantine so unauthorized or conflicting records at/below `acceptedThrough` fail closed as `QUEUE_ACK_RECOVERY_CONFLICT` rather than quarantine-hide);
- strict ACK-before-delete behavior bound to the exact submitted record IDs (application ACK only; transport/auth/permanent errors retain records); `acceptedThrough` plus `acceptedRecords` are persisted before authorized active-file removal, then `acceptedRecords` is cleared after successful delete; ACK must also bind exact `correlation_id`;
- cross-batch continuity after the first successful ACK (`acceptedThrough+1` must be next) and same-sequence conflict fail-closed holds (`QUEUE_SEQUENCE_GAP` / `QUEUE_SEQUENCE_CONFLICT`); missing unacked lineage from durable `lineageStart` (or legacy lowest covered) through `highestSequence` is `QUEUE_SEQUENCE_GAP`; post-rename meta dirsync ambiguity is `QUEUE_META_UNCERTAIN` (fail-closed, never regress `acceptedThrough`, never adopt a later `lineageStart`);
- sequence space fails closed at `MAX_SAFE_SEQUENCE` (`QUEUE_SEQUENCE_EXHAUSTED`); ReadBatch never spans `legacyCutoverSequence`; modern deliveries use `schemaVersion` 2 with `agentStreamId` and lineage batch IDs while legacy cutover prefixes remain `schemaVersion` 1;
- corruption quarantine with fail-closed rename handling: durable queue-local transition intent (`quarantine.transition`) before active→quarantine rename, then destination directory durability before source deletion durability; intent cleared only after the final filesystem state is durable; restart reconciliation stays blocked on ambiguous intent/copy state (no silent loss on quarantine failure);
- bounded quarantine storage (same record/byte caps as the active buffer) with `QUEUE_QUARANTINE_FULL` fail-closed retain (no oldest-record deletion);
- quarantined sequences block later delivery (no invented sequence skip/tombstone); later records remain intact;
- unknown/noncanonical `rec_*.json` names fail-closed with a global delivery block and are retained in `active/`;
- hard capacity bounds with explicit `QUEUE_FULL` rejection (no automatic head-drop / eviction of oldest records);
- symlink/path protections and `0700`/`0600` state permissions;
- configured maximum record count, total serialized buffer bytes, and payload record size.

The fixed Sensor/Core 80% storage watermark is **not** copied to Agent because Platform v3.2.2 does not define that same mandatory Agent threshold. Agent buffer bounds are engineering safety guards, not commercial sizing claims. Delivery is at-least-once; this runtime does not claim exactly-once.

## Configuration

Configuration is explicit environment input. Unknown `PLATFORM_AGENT_*` settings fail closed.

Required runtime inputs:

- `PLATFORM_AGENT_TENANT_ID`
- `PLATFORM_AGENT_ID` — Core-assigned Agent identity; must match Sensor authorization
- `PLATFORM_AGENT_SENSOR_URL` — HTTPS origin of the assigned Sensor Agent Receiver; no URL credentials/path/query/fragment
- `PLATFORM_AGENT_CA_FILE`
- `PLATFORM_AGENT_CERT_FILE`
- `PLATFORM_AGENT_KEY_FILE`

Optional governed inputs:

- `PLATFORM_AGENT_CORE_URL` — HTTPS origin only; enrollment/control-plane identity, **never** telemetry
- `PLATFORM_AGENT_SITE_ID`
- `PLATFORM_AGENT_STATE_DIR` (default `/var/lib/platform/agent`)
- `PLATFORM_AGENT_COLLECTION_INTERVAL`
- `PLATFORM_AGENT_COLLECTION_TIMEOUT`
- `PLATFORM_AGENT_HEALTH_ADDRESS` (loopback only)
- `PLATFORM_AGENT_MAX_BUFFER_RECORDS`
- `PLATFORM_AGENT_MAX_BUFFER_BYTES`
- `PLATFORM_AGENT_MAX_RECORD_BYTES`
- `PLATFORM_AGENT_MAX_BATCH_RECORDS`
- `PLATFORM_AGENT_MAX_BATCH_BYTES`
- `PLATFORM_AGENT_MAX_FILESYSTEMS`
- `PLATFORM_AGENT_RETRY_BASE`
- `PLATFORM_AGENT_RETRY_MAX`
- `PLATFORM_AGENT_DELIVERY_TIMEOUT`
- `PLATFORM_AGENT_EBPF_ENABLED` (default `false`; when true, Linux composition constructs the optional eBPF Backend and AgentService attempts one async activation after `RUNNING`)
- `PLATFORM_AGENT_EBPF_PREFLIGHT_TIMEOUT` (default `2s`, bounded `100ms..10s`; bounds the one-shot startup preflight so stalled procfs/sysfs cannot hold optional enrichment activation unboundedly)
- `PLATFORM_AGENT_EBPF_RECHECK_MIN` (default `5m`, bounded `1m..24h`)
- `PLATFORM_AGENT_EBPF_RECHECK_MAX` (default `1h`, bounded `1m..24h` and not below the minimum)
- `PLATFORM_LOG_LEVEL`

TLS material is validated before network/spool activation. Private keys must be regular, non-symlink files without group/world permissions. Errors/logs do not print secret material.

Startup/shutdown exit codes: `1` configuration, `2` identity, `3` service init, `4` start, `5` service shutdown failure.

## Linux service lifecycle

`agent/linux/systemd/platform-agent.service` is the Mother LLD §85.3 unit contract (also published under `deploy/systemd/` for later installer packaging; `.deb`/`.rpm` packaging remains BUILD-018):

- dedicated `platform-agent` user/group created by the Linux package (this unit does not create them)
- required `EnvironmentFile=/etc/platform/agent.env` (missing file fails the unit; fail-closed)
- `StateDirectory=platform/agent` with working directory `/var/lib/platform/agent`
- `Restart=on-failure` with bounded `RestartSec=5`
- explicit crash-loop bound: `StartLimitIntervalSec=60` / `StartLimitBurst=5`
- explicit start/stop timeouts (`TimeoutStartSec=30`, `TimeoutStopSec=20`; stop timeout remains above the AgentService 10s shutdown budget)
- `NoNewPrivileges=true`
- SIGTERM graceful stop

Real host `systemd` install/enable/reboot certification is **NOT_RUN — REQUIRES HOST CERTIFICATION** on a real systemd-capable Linux host. Cursor Cloud / Docker-only environments cannot complete that gate.

## Health

Loopback-only health endpoints use certified Slice-3 local-safety semantics:

- `/live` — process liveness; HTTP 200 while the process can answer (including shutdown signaling).
- `/ready` and `/healthz` — readiness:
  - `UP` / HTTP 200 when locally healthy;
  - `DEGRADED` / HTTP 200 / `ready=true` for Sensor/upstream or partial collector degradation while durable local buffering remains safe;
  - `NOT_READY` / HTTP 503 only for local unsafe/unexecutable state (examples: lifecycle failure, scheduler unavailable, zero enabled collectors, all enabled collectors blocked/outstanding, queue hard capacity/integrity block, `BLOCKED` or `OVERSIZED` delivery hold).
- `/health` — same readiness decision as `/ready`, with `SUPPORT_EVIDENCE` detail.
- `/capabilities/process-network-ebpf` — optional enrichment capability + bounded support counters; **never** a readiness hard-reason input.

Constructing an mTLS client is not treated as Sensor reachability.

## Windows

`windows/cmd` is a compile-boundary harness (`GOOS=windows` build). Windows Service/SCM, Event Log, signed EXE, and MSI packaging are **not** implemented. Do not treat an Ubuntu Cloud VM cross-compile as Windows runtime certification.

## Engineering verification

The repository CI executes Agent unit tests in a Go 1.27.1 container. The Agent Docker build additionally runs Agent-owned static/race gates before producing the Linux runtime image:

```text
go vet ./...
go test -race ./...
go build ./linux/cmd
```

The runtime image is non-root and contains no Go compiler/toolchain.
