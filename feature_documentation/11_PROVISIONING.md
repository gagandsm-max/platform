# Provisioning

**Class:** Derived feature brief — non-authoritative  
**Primary sources:** Platform_02, 09, 10, 20, 23

## Product objective

Onboard devices/endpoints/services through individual entry, bulk import, discovery and Agent enrollment with identity reconciliation, validation, capacity/licence checks and governed activation.

## Entry methods

- Add Entity;
- CSV/XLSX bulk;
- network discovery;
- Agent enrollment;
- Capability Pack/cloud/API onboarding;
- future governed policy-as-code declarations.

## Provisioning workflow

```text
Candidate / input
 -> identity & duplicate check
 -> scope/site/group/tag metadata
 -> credential/profile selection
 -> Sensor placement/capacity
 -> connectivity/preflight
 -> Capability Pack / monitor profile
 -> review/approval
 -> entitlement/licence
 -> activate monitoring
 -> verify first evidence
```

## Identity

Canonical identity uses tenant-aware stable signals such as serial, SNMP engine ID/sysObjectID, MAC/hardware UUID, cloud ID, agent ID, hostname and network namespace. IP is not sole identity.

Display Name, technical hostname and discovered descriptions remain distinct.

## Review lifecycle

Discovered/enrolled candidates enter governed review by default. Authorized trusted auto-approval may be configured only inside explicit scope and still performs identity/preflight/capacity/licence checks.

Approve/reject/unmanaged/exclude actions are audited with reason/comment.

## Preflight

Validate:

- Sensor reachability;
- protocol/port;
- Credential Profile;
- DNS/NTP/time where relevant;
- target identity;
- Pack compatibility;
- capacity;
- licence;
- tenant/site scope.

Failures are actionable and support Fix & Retry rather than forcing bulk-file rework.

## Sensor placement

Locality matters, but nearest Sensor is not selected blindly. Placement accounts for role eligibility, health, capacity, tenant/site policy and failover/redundancy.

## Activation

No candidate becomes fully monitored/licensed merely because it was discovered or Agent-enrolled. Activation is an explicit governed state transition.

## Notifications

Meaningful new/disappeared/conflicting/moved candidates may notify authorized users with dedupe/rate limits. Alert/Incident lifecycle remains separate.
