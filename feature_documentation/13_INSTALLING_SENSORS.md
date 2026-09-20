# Installing Sensors

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 03, 10, 11, 16  
**Pending enhancement:** production Sensor role exclusivity defined in `35_SENSOR_DEPLOYMENT_ARCHITECTURE.md`.

## Product objective

Install a headless Sensor near monitored sources, securely enroll it with Core, assign a production role, validate capacity/connectivity and begin only the workloads authorized for that role.

## Packaging

One `platform-sensor` product/package for supported Ubuntu/RHEL environments. Do not create separate Flow/Log/Network source trees or installers.

Native service:

- package install;
- auto-start after reboot;
- supervised recovery;
- structured local health/logs;
- secure upgrade/rollback.

## Enrollment

```text
Install Sensor
 -> Core endpoint
 -> deployment/enrollment identity/token
 -> outbound authenticated registration
 -> certificate/mTLS establishment
 -> role selection
 -> capacity/site/pool metadata
 -> connectivity validation
 -> Core approval/activation
 -> receive effective config
```

Sensor-to-Core transport uses HTTPS/gRPC over mTLS. Kafka remains internal to Core.

## Role selection

Production installation should choose exactly one major role:

- `NETWORK`;
- `FLOW`;
- `LOG`.

Role determines which collection modules can start. Common control-plane, health, spool, configuration, certificate and upgrade functions always remain.

## Network requirements

Generate/export required connectivity from effective role and assigned target/source scope. Flow/Log listeners require inbound connectivity from configured sources; Core communication remains outbound from Sensor where architecture permits.

## Storage

Sensor has bounded local durable storage with the fixed 80% protection policy. Required disk capacity depends on role-specific ingress rate, outage buffer horizon and replay workload.

## Validation

Before activation verify:

- mTLS/Core connectivity;
- time/DNS;
- role;
- storage;
- CPU/RAM;
- listener/target connectivity;
- source/target policy;
- certificate state;
- initial health heartbeat;
- no conflicting role workload.

## Role change

Production role conversion should require drain, flush/settle spool, remove/reassign sources/jobs, reconfigure/restart and post-change health validation. Do not silently mix major roles during conversion.
