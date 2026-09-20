# Logs

**Class:** Derived feature brief — non-authoritative  
**Canonical sources:** Platform_02, 03, 04, 05, 06, 09, 10, 11, 17, 18, 22, 23  
**Draft enhancement:** PR #67  
**Pending architecture refinement:** production `sensor_role = LOG`, central SIEM forwarding default.

## Product objective

Provide one operational log/event telemetry architecture covering network, host, application, middleware, database, container, OTLP, cloud/API and custom logs, while keeping Log Event, Alert and Incident semantics separate.

## Production Sensor role

High-volume production log collection runs on dedicated `sensor_role = LOG`. The LOG Sensor runs log receivers/collectors/parsers/bounded spool/upload/health only plus common Sensor control services; it does not run Flow, SNMP polling, NCM, discovery or synthetics.

## Sources

- Syslog RFC3164/RFC5424 over UDP/TCP/TLS;
- Linux/application files;
- journald;
- Windows Event Log and configured files;
- Kubernetes/container logs;
- OTLP Logs;
- middleware/database logs via Packs;
- cloud/API/webhook/custom sources;
- customer-owned Fluent Bit/OTel Collector interoperability where governed.

## Canonical pipeline

```text
Collect
 -> identify/authenticate source
 -> bounded durable buffer
 -> parse/multiline
 -> normalize
 -> sensitive-data policy
 -> entity/service resolution
 -> fingerprint/data quality
 -> store/search
 -> correlate / Alert Evidence
 -> route/forward
```

## Canonical Log Event

Important fields include trusted tenant/source, entity/service, event/observed/ingest times, timestamp quality, original/normalized severity, message/body, app/component/environment, host/container/K8s context, event class/code, fingerprint, trace/span/correlation IDs, parser/version, provenance/quality and bounded raw reference.

Arbitrary customer fields do not automatically become ClickHouse columns.

## Parser framework

Support JSON, key/value, Syslog, bounded regex/Grok-like mappings and multiline for common Java/.NET/Python/database/middleware records. Parser execution is resource bounded and versioned. Preview should show raw -> parsed -> normalized -> redacted -> entity mapping.

## Data quality

Expose parser errors, missing/bad timestamp, clock skew, severity/entity mapping, late/out-of-order, duplicates, rate limits, cardinality interventions, drops and source health.

## Sensitive data

Ingress/storage and egress policy may differ. Mask/hash/drop configured secrets/PII fields before required boundaries. Plain hashing is not a universal privacy boundary; governed keyed pseudonymization is used where required.

## Search/UX

`Observe -> Logs` provides tenant-scoped search/filter, raw/parsed/quality/provenance, surrounding context, fingerprints/patterns and bounded live tail.

## AI

Do not stream unrestricted raw logs to an LLM. Use query/fingerprint/statistical reduction -> bounded evidence -> context builder. Log text remains untrusted prompt content.

## Implementation note

PR #67 is draft and stale relative to newer main. Treat its detailed architecture as review input until renumbered/reconciled. Current Syslog foundations are more advanced than generic file/journald/WEL/Log Explorer/runtime completeness.
