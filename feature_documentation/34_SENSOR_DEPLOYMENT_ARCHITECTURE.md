# Sensor Deployment Architecture

**Class:** Derived feature brief — non-authoritative  
**Canonical base:** Platform Sensor/Core architecture  
**Architecture enhancement:** pending Mother Reference reconciliation; explicitly requested product direction.

## Decision objective

Use **one Sensor software product** with an explicit production role selected at installation/enrollment. High-volume Flow and Log workloads receive dedicated Sensor instances so they cannot starve network polling/control workloads.

## Production roles

```text
sensor_role = NETWORK
sensor_role = FLOW
sensor_role = LOG
```

A production Sensor has exactly one major role.

### NETWORK Sensor

Runs network/control-oriented collection such as:

- SNMP polling and network state;
- ICMP;
- gNMI/REST where assigned;
- network discovery;
- topology acquisition;
- NCM/SSH execution where governed;
- bounded diagnostics;
- ordinary SNMP trap handling if assigned and certified.

Does **not** run production Flow or high-volume Log ingestion.

### FLOW Sensor

Runs:

- NetFlow v5/v9;
- IPFIX;
- sFlow;
- J-Flow/vendor Flow;
- exporter/listener state;
- template/sequence/sampling state;
- Flow normalization;
- bounded Flow spool/upload;
- Flow health.

Does **not** run SNMP polling, discovery, NCM, synthetics or Log collection.

### LOG Sensor

Runs:

- Syslog UDP/TCP/TLS;
- high-rate log/event receiving;
- applicable remote/customer log ingestion;
- parser/multiline/redaction/fingerprint pipeline;
- bounded Log spool/upload;
- Log source/pipeline health;
- OTLP Logs where specifically assigned by the governed architecture.

Does **not** run Flow, SNMP polling, NCM, discovery or synthetics.

## Common runtime

All roles share only the common Sensor platform needed for safe operation:

- Core registration/control;
- mTLS identity/certificate rotation;
- effective configuration;
- health/self-monitoring;
- bounded local durable storage;
- uploader/replay;
- package/upgrade/rollback;
- audit/support diagnostics.

Do not create three independent codebases, installers or release trains.

## Installation workflow

```text
Install platform-sensor
 -> register to Core
 -> choose/assign role
 -> site/pool metadata
 -> role-specific connectivity
 -> capacity/disk validation
 -> source/target assignment
 -> activate
 -> health/certification checks
```

The UI/installer must show enabled and disabled capabilities for the selected role.

## Production exclusivity

Production certification rejects mixed FLOW+LOG or FLOW/LOG+NETWORK workloads on the same Sensor.

A `COMBINED_DEV`/lab profile may exist only for development/CI/demo and is explicitly **not production certified**.

## Why

Dedicated roles improve:

- CPU/RAM predictability;
- disk/spool isolation;
- socket/queue isolation;
- performance certification;
- blast-radius containment;
- troubleshooting;
- capacity planning;
- independent scaling.

A Flow/log storm cannot take down SNMP polling/NCM.

## Headroom

"Dedicated" does not mean 100% resource utilization. Capacity benchmarks preserve burst, replay, failover, maintenance and storage-protection headroom.

## Placement and pools

Core assigns workload only to Sensors with the required role, tenant/site eligibility, health and capacity. Nearest geography is not enough.

## Role conversion

Role change requires:

1. stop new assignments;
2. drain/flush or safely account for spool;
3. remove/reassign sources/jobs;
4. validate no incompatible workload remains;
5. change role;
6. restart/reconfigure modules;
7. run role-specific health checks;
8. reactivate.

No live hidden mixed-role conversion.

## HA/redundancy

Dedicated Flow and Log roles require source/protocol-specific redundancy design. Exporters/log sources differ in multi-collector, UDP/TCP/TLS and failover behavior; do not assume a generic load balancer solves every case.
