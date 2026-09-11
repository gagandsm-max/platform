# Anonymous PostgreSQL Volume Pool — Disposition (Phase-1 leftover)

Status: **DO NOT DELETE until independent review approval.**

## Confirmed origin

Root cause (proven in source + reproduction):

- Helper: `core/src/test/java/com/platform/core/support/PostgresTestSupport.java`
- Path: `docker run … postgres:16-alpine` without mounting `/var/lib/postgresql/data`
- Image declares `VOLUME /var/lib/postgresql/data` → Docker creates an **anonymous** volume
- Teardown used `docker rm -f` **without `-v`**, which orphans the anonymous volume even when `--rm` was set on `run`
- Callers: Day-02 postgres-tagged tests (~22 containers per full suite run historically)

Compose named volume `platform_pg_data` is **not** this leak.

## Inventory evidence (Phase-1 audit)

| Metric | Value |
|--------|------:|
| Unused zero-link volumes | ~1107 |
| PG-sized (~48–51MB) unused anon | ~832 |
| Empty unused anon (0B) | ~266 |
| Approx reclaimable if approved | **~41GB** |

Filesystem spot-checks showed PostgreSQL data dir markers (`PG_VERSION`, `base`, `pg_wal`) on sampled ~48MB volumes.

## Proof they are not current Platform state

Current Platform stack uses **named** volumes:

- `platform_pg_data`, `platform_kafka_data`, `platform_ch_data`, `platform_valkey_data`
- `platform_sensor_net_spool`, `platform_sensor_infra_spool`

Those remain referenced by running Compose services. The ~832 PG-like volumes are
**anonymous 64-hex names**, `links=0`, unreferenced by any container.

## Timeframe

Created across repeated Day-02 / postgres integration runs on the host Docker engine
(CI/local DinD via mounted `docker.sock`). Exact create timestamps vary per volume
(`docker volume inspect CreatedAt`). Arithmetic: ~832 ÷ ~22 tests/run ≈ ~38 suite runs
at current test counts (historical counts differ).

## Fix status

Code fix lands with tmpfs + `docker rm -fv` + disposable labels. That **stops growth**.
It does **not** reclaim the historical pool.

## Recommended cleanup (separate approved phase)

1. Re-verify: `docker ps -a` shows no container referencing candidate volumes
2. Confirm candidates are anonymous (`^[a-f0-9]{64}$`) and size band / PG markers
3. Exclude any volume still mounted by day02/HA/netai containers
4. Delete only the reviewed candidate set (not `docker volume prune`)
5. Re-run `make dev-disk-check`

Estimated reclaim: **~41GB**.
