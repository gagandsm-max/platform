# PR #227 K1.1 INSERT completion correction — Issues #222 / #235

Engineering evidence, 2026-10-15; bounded Codex writer validation, pending independent exact-SHA review.

## Source and ownership

- Parent/task/remote PR head: `26d70b1d7cb5608a83de08e3dc3f40351ce54116`.
- Fetched main: `019252a00133c935a07432462bcf06a4b5ffc9fd`; no rebase or integration performed.
- Worktree: `/home/gagan/nms-issue222-kafka-consumer`, branch `fix/issue222-kafka-consumer-read-committed`; initially clean.
- Terminal Codex PID 599117 was attributed to this session via `CODEX_THREAD_ID=01a13f33-0767-7452-b533-5f39cfde984b` and matching codex-tui session metadata. Other Cursor workers had different worktrees. No competing writer identified.
- Governing references: Platform_00; BRD Docker/durability requirements; PRD pipeline integrity and PRD-SNMP-002; Architecture §§6,26; HLD §§9–10; LLD §§6–7; Dataflow SNMP/Kafka path; OSS-001/002/008; Platform_17 replay contracts. Main's Tranche B envelope reconciliation does not change this correction's implementation scope.
- Applied skills: root_cause_first, bounded_implementation, kafka, replay_idempotency, java_spring, runtime_failure_injection; supporting Docker/transport contract guidance.

## Reproduced defects on parent production source

`ClickHouseClientTest` with added regression tests: 20 tests, 2 expected failures. The late HTTP 200 exception returned normally through the consumer instead of throwing; HTTP 500 without exception metadata was PERMANENT instead of UNKNOWN.

A separate exact plain-IOException injection at `HttpClient.send` failed: expected DEPENDENCY, observed PERMANENT. An abrupt real HTTP connection close already classified as DEPENDENCY through a nested EOFException; it does not alone reproduce the plain IOException gap.

Raw logs: `/tmp/pr227-k11-UZg5FjSJ/baseline.log`, `baseline-reports/`, `plain-io-baseline.log`, `plain-io-baseline-reports/`.

## Correction and ACK boundary

1. Serialization occurs before transport. Only IOException caught from `HttpClient.send` receives typed transport provenance; generic transport IOException is DEPENDENCY. Existing TLS/configuration distinctions are preserved. Jackson/local IOException remains nonretryable. Consumer redaction retains bounded causal class/stack evidence without raw messages.
2. Unrecognized HTTP status/exception metadata is explicit `ERR_CLICKHOUSE_UNKNOWN`, nonretryable. Known allowlisted codes retain their existing classification. UNKNOWN cannot authorize recovery or offset disposition.
3. Fixed `INSERT ... FORMAT JSONEachRow` requests force `async_insert=0` and request `wait_end_of_query=1`. Success requires HTTP 200, absent/zero exception code, and complete empty response body. Any body byte is UNKNOWN and cancels response consumption. No response text is retained or logged. The custom body subscriber retains only completion state, deadline and subscription, independent of response size.
4. The existing 10-second request budget is retained. Body completion also has an absolute monotonic deadline, calculated before send, so an HTTP 200 followed by a stalled body cannot hold the listener indefinitely. Consumer processing/poll/commit budget enforcement is unchanged.
5. State machine: Kafka record → deserialize/serialize existing identity → synchronous INSERT → validate metadata and complete empty body → insert-success metric → existing processing-budget check → MANUAL_IMMEDIATE synchronous ACK. Any failure exits without ACK; registered listener stops abnormally. Crash/lost-ACK after insertion can still replay the same identity under existing at-least-once semantics.

Numeric and Counter64 serialization, timestamps, tenant/entity/sensor/metric/sequence identity, partition key, topics, canonical FINAL views and replacement keys are unchanged. No dependency, migration, production service, K2 recovery, K3 disposition or DLQ was added. Rollback is a normal revert of this commit; no data/schema conversion is needed, though reverting reintroduces the defects.

## Executed validation

All Java execution used isolated Docker Compose project `pr227-k11-uzg5fjsj`, Maven `3.9.9-eclipse-temurin-21-alpine`, and a read-only source mount copied into the test container. The host Maven cache was read-only and copied into each container. No host project runtimes were installed.

- Initial focused run: 77 tests, one test-oracle failure. The new broker fixture incorrectly expected committed offset 2 after the first data record; MANUAL_IMMEDIATE correctly committed 1, before the transaction marker. Corrected that expectation; the unchanged-offset failure assertion was retained.
- Corrected runtime/client/broker run: **33 tests, zero failures/errors/skips**.
- Final `mvn -B -DexcludedGroups=postgres test`: **581 tests, zero failures/errors/skips**, including 30 client cases, 23 consumer cases, 22 Kafka configuration cases, 6 registered-listener/broker cases and 2 real-ClickHouse cases.
- `git diff --check`: passed. Runtime log scan found no `PRIVATE_SENTINEL` or `secret-header` disclosures.

Named tests include `lateExceptionAfter200NeverCountsSuccessOrAcknowledges`, `plainIOExceptionFromSendIsRetryableButJacksonIOExceptionIsNot`, `stalledBodyAfter200HeadersStillHasWholeRequestDeadline`, `noSuccessOrAckBeforeEmptyResponseCompletes`, `sendTlsFailureRetainsConfigurationClassification`, `anyUnexpected200BodyFailsClosed`, and `lateHttp200ExceptionRetainsBrokerOffsetUntilExplicitReplaySucceeds`. Existing HTTP-code tests cover nonzero exception headers, unknown 500 and ordinary known failures.

Registered production @KafkaListener / KafkaListenerEndpointRegistry evidence against embedded KRaft:

```text
K11_OFFSET before=1 late_error=1 explicit_replay=3 inserts=2 failures=1
```

The initial genuine success accounts for one insert. The failed record produced no extra insert-success metric, no insertion retry and no offset commit. Explicit operator-style listener restart replayed the identical INSERT body, then committed offset 3. No automatic restart was exercised or added.

## Real ClickHouse 24.8

Actual version `24.8.14.39`, image `clickhouse/clickhouse-server:24.8-alpine`; repository bootstrap SQL mounted read-only. `/var/lib/clickhouse` and logs used fresh tmpfs. No persistent volume was mounted, migrated, formatted or removed; no host database port was published. After validation, project-scoped `docker compose down` removed only this fixture container/network; tmpfs state was disposable. No prune or volume deletion was used.

```text
K11_REAL_CH late_select status=200 body_bytes=292 exception=true
K11_REAL_CH failed_insert status=200 body_bytes=281 exception=true
K11_REAL_CH wait_end_failed_insert status=200 body_bytes=281 exception=true
K11_REAL_CH synchronous_insert status=200 body_bytes=0 numeric=42 counter=18446744073709551615 canonical_replay_count=1
```

The real late INSERT uses bounded `INSERT SELECT` with sleep/throwIf, small blocks and progress headers. It demonstrates late server failure even with wait_end_of_query enabled. The ACK/metric regression uses an HTTP wire fixture emitting the same late-error behavior through the actual production client and registered listener; it does not claim that the INSERT SELECT drill itself traveled through that listener.

Successful JSONEachRow insertion uses the production client directly against real ClickHouse. Repeated numeric/UInt64 INSERTs preserve exact values and one canonical logical row per identity. Raw physical duplicate counts are not an exactly-once claim.

Official reference: [ClickHouse HTTP interface, response buffering and response-code caveats](https://clickhouse.com/docs/interfaces/http). wait_end_of_query is mitigation; metadata and body must still be verified.

## Reproduction and evidence location

Compose configuration, raw transcripts and Surefire XML/text reports remain at `/tmp/pr227-k11-UZg5FjSJ/` (`compose.yaml`, `runtime.log`, `runtime-reports/`, `final-core.log`, `final-core-reports/`). Final source patch is `source.diff` in that directory. To repeat with a fresh isolated server, use a unique project name and tmpfs-only ClickHouse initialized from `deploy/docker/clickhouse/init.sql` with the test-only credentials in the fixture. Set `K11_CLICKHOUSE_HOST` to its Compose service DNS name; without that variable the two real-server tests are deliberately skipped.

Executed command shape:

```sh
docker compose -p pr227-k11-uzg5fjsj -f /tmp/pr227-k11-UZg5FjSJ/compose.yaml up -d --wait clickhouse
docker compose -p pr227-k11-uzg5fjsj -f /tmp/pr227-k11-UZg5FjSJ/compose.yaml run --rm tests 'cp -a /src/. /build/; mkdir -p /root/.m2; cp -a /cache/. /root/.m2/; mvn -B -DexcludedGroups=postgres test'
```

## Unverified limits and remaining risks

- P1: process/host crash-reboot, filesystem/fsync/power-loss durability, full Sensor deletion-ACK matrix, permanent Core loss, HA and backup/restore are **NOT_RUN / NOT_CERTIFIED**. Query-completion checks do not establish those larger durability boundaries.
- P1: K2 automatic listener recovery and K3 poison/unknown offset disposition remain deferred. UNKNOWN intentionally blocks the partition/listener until governed correction and explicit restart; no success or transient cause is inferred from it.
- P1: lost response/ACK or partially applied failing INSERT can replay physical rows. Existing stable identity and canonical FINAL reads are preserved and locally exercised; arbitrary conflicting replay and full production recovery are not certified here.
- P2: throughput/connection reuse/cancellation overhead and other ClickHouse releases/profiles/proxies are not benchmarked. The accepted success contract is the fixed synchronous result-free INSERT against the trusted ClickHouse endpoint; unexpected success output fails closed.
- PostgreSQL-tagged tests, exact-new-head CI and independent Antigravity review: **NOT_RUN**. Writer evidence does not close REALITY_GATE, MERGE_READY or certification. Next owner: independent exact-SHA verifier / Master control plane.
