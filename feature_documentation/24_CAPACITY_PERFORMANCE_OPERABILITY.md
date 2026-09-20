# Capacity / Performance / Operability

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 03, 11, 14, 16

## Product objective

Keep Platform predictable under enterprise workload by sizing from measurable workload dimensions, protecting monitoring during bursts/failures and making internal health visible.

## Sizing model

Do not size only by device count. Use Monitoring Work Units / workload factors such as:

- protocol and poll interval;
- interfaces/OIDs/metrics;
- Flow datagrams/records/bytes;
- Log EPS/bytes and parser cost;
- OTLP rate/cardinality;
- Agent metrics/process/service/log load;
- synthetics frequency/steps;
- NCM jobs;
- diagnostics;
- dashboards/query concurrency;
- reports;
- retention;
- AI/analytics workload;
- route/connector fan-out.

## Dedicated high-volume roles

Production Flow and Log ingestion should run on dedicated FLOW/LOG Sensor roles so their high-rate workloads do not compete with NETWORK Sensor polling/NCM/discovery.

## Headroom

Capacity certification includes burst/replay/failover headroom. "Dedicated Sensor" means workload isolation, not permission to run CPU/RAM/disk at 100%.

The fixed 80% storage-protection watermark remains mandatory.

## Performance dimensions

Measure:

- poll operations/sec;
- ingestion records/bytes/sec;
- queue/backlog age;
- spool growth/drain;
- database insert/query latency;
- dashboard load;
- report/RCA concurrency;
- Flow decode/template/enrichment;
- Log parse/redaction/fingerprint;
- Agent/OTLP ingress;
- failover recovery;
- retention cleanup.

## Query safety

Large interactive queries use bounded rows/scans/time ranges, appropriate rollups, asynchronous exports where required, cancellation and per-tenant/user concurrency limits.

## Operability

Platform monitors itself:

- Core service health;
- Sensor/Agent health;
- Kafka lag;
- PostgreSQL/ClickHouse health;
- ingestion lag;
- drops/rejects;
- spool/backlog;
- storage watermark;
- data freshness;
- certificate expiry;
- connector queues;
- crash loops;
- query latency.

## Capacity Engine

Provisioning and policy changes should preview projected workload. Platform may recommend role separation/rebalancing when safe limits are approached. Rebalancing uses hysteresis to avoid workload thrash.

## Certification

Do not publish universal throughput claims until benchmarked against defined workload profiles, hardware, protocol mix, retention and query load.
