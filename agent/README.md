# Platform Agent runtime foundation

This directory contains the Linux-first Platform Agent runtime checkpoint governed by Platform v3.2.2.

## Architecture boundary

The authoritative flow is:

1. Agent initially enrolls through the stable Core endpoint.
2. Core owns tenant/site identity and assigns primary/fallback Infrastructure Sensor endpoints.
3. Runtime telemetry flows Agent -> assigned Sensor over the Platform mTLS channel.
4. The Sensor owns downstream normalization/spooling/upload to Core ingestion.

The current repository does **not** yet contain the receiving-side Agent ingress implementation on Sensor. This Agent therefore does not invent an HTTP/gRPC endpoint. It collects and durably buffers real Linux host telemetry and exposes a typed delivery/ACK boundary. Production delivery is intentionally wired to `AGENT_RECEIVER_NOT_IMPLEMENTED` until the Sensor-owned receiver contract lands.

`/live` reports process liveness. `/health`, `/healthz`, and `/ready` remain degraded/not-ready while the authoritative receiver is unavailable, so a live process cannot falsely report a healthy telemetry pipeline.

## Implemented Linux telemetry

The Linux collectors use bounded standard-library `/proc` and `statfs` access only:

- uptime and boot time;
- load averages;
- CPU count and aggregate utilization;
- memory total/available/used/utilization;
- bounded mounted-filesystem total/used/free/utilization with mount and filesystem type.

This checkpoint does not collect process command lines, environment variables, packet contents, arbitrary command output, application traces, or unrestricted process data. Windows is a compile-boundary harness only (`GOOS=windows` build of `windows/cmd`); it is not a production Windows Agent. Windows Service/SCM, Event Log, signed EXE, and MSI packaging are not implemented in this checkpoint.

## Durable local buffer

Agent telemetry is persisted before delivery using:

- stable sequence and record IDs;
- SHA-256 integrity over sequence, ID, createdAt, and payload;
- restart recovery with content validation;
- strict ACK-before-delete behavior (application ACK only; transport/auth/permanent errors retain records);
- corruption quarantine with fail-closed rename handling (no silent loss on quarantine failure);
- hard capacity bounds with explicit `QUEUE_FULL` rejection (no automatic head-drop / eviction of oldest records);
- symlink/path protections and `0700`/`0600` state permissions;
- configured maximum record count, total serialized buffer bytes, and payload record size.

The fixed Sensor/Core 80% storage watermark is **not** copied to Agent because Platform v3.2.2 does not define that same mandatory Agent threshold. Agent buffer bounds are engineering safety guards, not commercial sizing claims.

## Configuration

Configuration is explicit environment input. Unknown `PLATFORM_AGENT_*` settings fail closed. Required runtime inputs are:

- `PLATFORM_AGENT_TENANT_ID`
- `PLATFORM_AGENT_CORE_URL` — HTTPS origin only; no URL credentials/path/query/fragment
- `PLATFORM_AGENT_CA_FILE`
- `PLATFORM_AGENT_CERT_FILE`
- `PLATFORM_AGENT_KEY_FILE`

Optional governed inputs:

- `PLATFORM_AGENT_ID`
- `PLATFORM_AGENT_SITE_ID`
- `PLATFORM_AGENT_STATE_DIR`
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
- `PLATFORM_LOG_LEVEL`

TLS material is validated before runtime startup. Private keys must be regular, non-symlink files without group/world permissions. Errors/logs do not print secret material.

## Engineering verification

The repository CI already executes Agent unit tests in a Go 1.27.1 container. The Agent Docker build additionally runs Agent-owned static/race gates before producing the Linux runtime image:

```text
go vet ./...
go test -race ./...
go build ./linux/cmd
```

The runtime image is non-root and contains no Go compiler/toolchain. Final Windows Service/SCM/MSI certification remains a separate Windows-runner requirement.
