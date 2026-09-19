# Platform — End-to-End Dataflow & Sequence Documentation

**Document ID:** PLATFORM-DF-001  
**Version:** 3.2.2  
**Status:** Authoritative Mother Reference — Engineering Source of Truth
**Governing documents:** BRD, PRD, Architecture, HLD, LLD  

---

## 1. Purpose

This document defines the end-to-end movement of control data, telemetry, events, configuration, identity, alerts, RCA evidence, retention actions and HA/DR state through Platform. It is intended for engineering, QA, security and operations teams validating that no hidden synchronous dependency or unowned state exists.

Every flow must preserve tenant identity, authorization, correlation IDs and audit requirements where applicable.

---

## 2. Flow DF-001 — Standalone Core installation

```mermaid
sequenceDiagram
  actor A as Installer/Admin
  participant I as Core Installer
  participant O as OS/Prereq Checks
  participant C as Core Services
  participant PG as PostgreSQL
  participant CH as ClickHouse
  participant K as Kafka
  participant UI as HTTPS UI

  A->>I: Start Core installation
  I->>A: Select Standalone
  I->>O: Validate Ubuntu/RHEL, CPU/RAM/disk, DNS, NTP, ports
  O-->>I: Readiness result
  I->>C: Install Core modules
  I->>PG: Initialize control schema
  I->>CH: Initialize telemetry tables/rollups
  I->>K: Initialize internal topics/schema
  C->>C: Generate/import TLS identity
  C->>PG: Write deployment metadata
  C->>CH: Health test
  C->>K: Produce/consume health test
  C-->>I: Core ready
  I-->>A: HTTPS endpoint + installation report
  A->>UI: Connect over HTTPS only
```

Failure behavior: installer never reports success if required Core data services are unhealthy.

---

## 3. Flow DF-002 — Two-Core HA installation and pairing

```mermaid
sequenceDiagram
  actor A as Admin
  participant A1 as Core-A Installer/HA Manager
  participant B1 as Core-B Installer/HA Manager
  participant HA as Stable HA Endpoint/VIP
  participant DB as Core State/Data Services

  A->>A1: Install Core-A; select HA; provide HA endpoint/peer plan
  A1->>A1: Create deployment ID + one-time peer token
  A1->>A1: Enter WAITING_FOR_PEER
  A->>B1: Install Core-B; provide HA endpoint/Core-A + peer token
  B1->>A1: Secure peer handshake
  A1->>B1: Verify token/deployment/certificates
  A1->>B1: Establish mutual node trust
  A1->>DB: Initialize Active state
  B1->>DB: Initialize Standby replica/sync
  A1<->>B1: Synchronize persistent state + HA metadata
  A1->>HA: Claim stable endpoint as ACTIVE
  B1->>B1: Enter STANDBY_READY
  A1-->>A: HA Ready: Active/Standby/sync status
```

The one-time pairing token is invalidated after successful pairing.

---

## 4. Flow DF-003 — Sensor installation and enrollment

```mermaid
sequenceDiagram
  actor A as Admin
  participant SI as Sensor Installer
  participant CE as Stable Core Endpoint
  participant EN as Enrollment Service
  participant SC as Sensor Control Service
  participant S as Sensor Runtime

  A->>SI: Install Sensor; Core endpoint; token; role/site/pool hints
  SI->>SI: Validate OS/CPU/RAM/disk/time/DNS
  SI->>CE: HTTPS/mTLS bootstrap request + enrollment token
  CE->>EN: Validate token/tenant/deployment
  EN-->>SI: Sensor ID + unique certificate + trust bundle
  SI->>S: Start runtime
  S->>SC: Authenticated heartbeat/capabilities
  SC->>SC: Resolve Sensor Pool/site/role/policies
  SC-->>S: Signed configuration + Device Packs + assignment version
  S->>S: Self-test protocol workers/spool/uploader
  S->>SC: READY heartbeat
  SC-->>A: Sensor healthy and available for placement
```

Sensor is centrally managed after enrollment; no routine local configuration is required.

---

## 5. Flow DF-004 — Agent installation and Core-directed Sensor assignment

```mermaid
sequenceDiagram
  actor A as Admin/Automation
  participant AG as Agent
  participant CE as Core Endpoint
  participant EN as Agent Enrollment
  participant PS as Placement/Sensor Control
  participant ONB as Onboarding/Policy
  participant S as Assigned Sensor

  A->>AG: Install Agent with Core endpoint + enrollment token
  AG->>CE: Secure bootstrap
  CE->>EN: Validate tenant/token/host metadata
  EN-->>AG: Agent ID + mTLS certificate
  EN->>PS: Request eligible Sensor for provisional enrollment transport
  PS->>PS: Evaluate eligible Sensors + locality + capacity + HA
  PS-->>AG: Primary + fallback Sensor endpoints, provisional assignment version
  AG->>S: Establish authenticated mTLS onboarding/control transport
  S-->>AG: Accept bounded bootstrap/health evidence only
  EN->>ONB: Create/reconcile candidate; PENDING_APPROVAL by default
  Note over AG,S: Provisional transport association is not monitoring/licence activation
  alt Authorized APPROVE or trusted auto-approval
    ONB->>PS: Confirm entitlement/capacity/policy; retain or re-evaluate Sensor
    PS-->>AG: Active monitoring assignment/profile version
    S-->>AG: Activate normal monitoring/test profile
    AG->>S: Normal recurring host metrics / Edge Test results
  else Pending/Unmanaged/Rejected/Excluded
    S-->>AG: No normal monitoring workload/profile
  end
```

The provisional and active phases use the same Sensor Control assignment authority and assignment/versioning contract; approval does not introduce a second Sensor authority mechanism. If Sensor capacity/location changes, Core sends a newer signed assignment; Agent reconnects without reinstall.

---

## 6. Flow DF-005 — Individual network-device provisioning

```mermaid
sequenceDiagram
  actor U as Authorized User
  participant UI as Provisioning UI
  participant P as Provisioning Service
  participant ID as Identity Service
  participant SP as Sensor Placement
  participant S as Candidate Sensor
  participant D as Device
  participant POL as Policy Service

  U->>UI: Enter IP/FQDN, optional name/description/site/profile
  UI->>P: Create staged provisioning job
  P->>P: Normalize safe fields
  P->>ID: Duplicate/identity precheck
  ID-->>P: No duplicate / conflict details
  P->>SP: Select candidate Sensors
  SP-->>P: Recommended eligible Sensor(s)
  P->>S: Run preflight from candidate Sensor
  S->>D: ICMP / SNMP / SSH / API / gNMI tests as configured
  D-->>S: Responses / timeouts / auth results
  S-->>P: Typed evidence + detected identity + RTT
  P->>P: Device Pack/profile match + capacity impact
  P-->>UI: Preview: detected device, Sensor, protocols, warnings, required fixes
  U->>UI: Approve provision
  UI->>P: Activate
  P->>ID: Create/reconcile canonical entity
  P->>POL: Resolve monitoring/alert/validation policies
  P->>SP: Commit assignment/fencing version
  P-->>S: Assigned monitoring job/config
  S->>D: Begin polling
  S-->>P: First successful observations
  P-->>UI: Provisioned/verified
```

---

## 7. Flow DF-006 — Provisioning failure diagnosis

```mermaid
sequenceDiagram
  participant P as Provisioning Service
  participant S as Candidate Sensor
  participant D as Device
  participant UI as UI
  participant AI as AI Gateway optional

  P->>S: Run SNMP preflight
  S->>D: ICMP
  D-->>S: PASS
  S->>D: UDP/161 SNMP request
  D-->>S: No response / auth report / error
  S-->>P: Structured evidence
  P->>P: Classify exact/suspected failure
  P-->>UI: Error code + evidence + uncertainty + remediation
  opt User requests explanation
    UI->>AI: Explain provisioning failure using typed evidence
    AI-->>UI: Evidence-grounded human explanation; no invented certainty
  end
```

Example outputs distinguish authentication failure, authorization failure, no response, suspected firewall/ACL, protocol disabled suspected and Sensor-path issue.

---

## 8. Flow DF-007 — Bulk CSV/XLSX provisioning

```mermaid
sequenceDiagram
  actor U as User
  participant UI as Bulk Import UI
  participant P as Provisioning Service
  participant SP as Sensor Placement
  participant S as Sensor Pool

  U->>UI: Upload CSV/XLSX
  UI->>P: Stage file
  P->>P: Normalize BOM/rows/non-secret whitespace/enums
  P->>P: Validate fields, duplicates, profiles, sites, credential references
  P-->>UI: Ready / corrected / warning / error / duplicate table
  U->>UI: Inline fix invalid rows
  UI->>P: Revalidate changed rows
  P->>SP: Simulate Sensor assignment + MWU capacity for ready rows
  SP-->>P: Distribution + warnings/insufficient capacity
  P-->>UI: Capacity preview + proposed distribution
  U->>UI: Approve ready rows
  loop each batch
    P->>S: Parallel bounded preflight from assigned candidate Sensors
    S-->>P: Evidence/result
  end
  P-->>UI: Completed / failed rows with Fix & Retry
```

Credentials should normally be references to Credential Profiles, not secrets in the file.

---

## 9. Flow DF-008 — Auto-discovery

```mermaid
sequenceDiagram
  actor U as User
  participant UI as Discovery UI
  participant D as Discovery Service
  participant SP as Sensor Placement
  participant S as Discovery Sensor
  participant N as Network Range/Seeds

  U->>UI: Define CIDRs/seeds/exclusions/credential profiles
  UI->>D: Create discovery job
  D->>SP: Choose discovery Sensor/Pool by locality/capacity
  SP-->>D: Sensor assignment
  D->>S: Discovery job lease
  S->>N: ICMP/SNMP/CDP/LLDP/ARP/FDB/routing/API/SSH probes
  N-->>S: Observations
  S-->>D: Candidate entities + identity/topology evidence
  D->>D: Deduplicate/reconcile
  D-->>UI: Discovery Review Queue
  U->>UI: Provision / unmanaged / ignore / exclude
  UI->>D: Approved actions
  D->>D: Invoke normal provisioning pipeline for approved targets
```

Discovery does not automatically consume monitoring licences unless policy explicitly allows auto-approval.

---

## 10. Flow DF-009 — Sensor placement and capacity simulation

```mermaid
flowchart LR
  E[New/Existing Entity Workload] --> C[Candidate Sensor Discovery]
  C --> R[Reachability/RTT]
  C --> A[Affinity Allowed/Forbidden]
  C --> W[Projected MWU/CPU/RAM/Queue]
  C --> H[HA/Failover Reserve]
  R --> ELIG[Eligibility Filter]
  A --> ELIG
  W --> ELIG
  H --> ELIG
  ELIG --> SCORE[Suitability Score]
  SCORE --> MODE{Placement Mode}
  MODE -->|Auto| ASSIGN[Assign Best]
  MODE -->|Approval| RECOMMEND[Show Why + Approve]
  MODE -->|Pool| POOL[Assign Pool / Scheduler Chooses]
  MODE -->|Manual| PIN[Pin Specific Sensor]
```

---

## 11. Flow DF-010 — Sensor automatic rebalance

```mermaid
sequenceDiagram
  participant M as Capacity Monitor
  participant P as Placement Service
  participant S1 as Sensor-A
  participant S2 as Sensor-B
  participant U as Admin optional

  M->>P: Sustained imbalance / backlog / RTT deterioration
  P->>P: Evaluate hysteresis + minimum improvement + HA reserve
  P->>P: Calculate target moves
  alt Automatic allowed
    P->>S1: Revoke jobs using newer assignment version
    P->>S2: Issue new leases
    S2-->>P: Applied
  else Approval required
    P-->>U: Recommendation + projected before/after load
    U->>P: Approve
    P->>S1: Revoke jobs
    P->>S2: Issue jobs
  end
```

Assignment/fencing version prevents obsolete Sensor from continuing a superseded job after it sees the new epoch.

---

## 12. Flow DF-011 — SNMP metric ingestion

```mermaid
sequenceDiagram
  participant S as Network Sensor
  participant D as Device
  participant SP as Local Spool
  participant I as Ingestion Gateway
  participant K as Kafka
  participant CH as ClickHouse
  participant AE as Alert Engine

  S->>D: SNMP GET/GETBULK
  D-->>S: Varbind response
  S->>S: Device Pack mapping + normalization + quality
  S->>SP: Durable canonical batch
  S->>I: mTLS batch
  I->>K: telemetry.numeric/state
  K->>CH: Storage consumer
  K->>AE: Evaluation consumer
  I-->>S: Durable acceptance watermark
  S->>SP: Release acknowledged segment
```

---

## 13. Flow DF-012 — SNMP trap/event ingestion

```mermaid
sequenceDiagram
  participant D as Device
  participant S as Sensor
  participant M as MIB/Device Pack Runtime
  participant I as Ingestion
  participant K as Kafka
  participant C as Correlation

  D->>S: SNMP trap/inform
  S->>M: Decode security/OIDs
  M-->>S: Structured event + entity/semantic mapping
  S->>I: Canonical event
  I->>K: event.trap.v1
  K->>C: Event correlation/qualification
```

---

## 14. Flow DF-013 — Flow ingestion, quality and temporal enrichment

```mermaid
sequenceDiagram
  participant X as Flow Exporter
  participant S as Dedicated Flow Sensor
  participant T as Exporter/Template/Sampling State
  participant INV as Temporal Identity/Network Context
  participant SP as Durable Flow Spool
  participant I as Ingestion
  participant K as Kafka
  participant CH as ClickHouse
  participant Q as Flow Data Quality

  X->>S: NetFlow/IPFIX/sFlow/J-Flow UDP
  S->>S: Listener/source admission + bounded receive accounting
  S->>T: Decode protocol + exporter/domain/template/sequence/sampling/time
  T-->>S: Raw typed observation + quality/gap state
  S->>INV: Resolve interface/entity/IPAM/DNS/session/site/ASN/cloud/app/geo by effective time
  INV-->>S: Versioned enrichment + provenance/confidence
  S->>SP: Durable canonical directional Flow + replay identity
  S->>I: Batched normalized flows + quality/accounting
  I->>K: network.flow.v1
  K->>CH: Raw Flow + bounded rollups/aggregates
  K->>Q: Exporter/template/sampling/sequence/pipeline quality
  I-->>S: Durable acceptance watermark
  S->>SP: Release acknowledged segment
```

Pre-acceptance UDP remains best-effort. After durable local acceptance, generic Sensor replay/idempotency applies. Receiver/kernel/queue drops, unknown exporters, malformed records, template misses, sequence gaps, sampling unknowns, storage/downstream rejection and replay actions are separately counted.

User attribution is only applied when time-bounded authenticated-session evidence exists. DNS/domain/application/public-GeoIP enrichment is provenance-bearing evidence; Flow does not infer full URL path/query or precise user location.

Equivalent traffic observed at different exporters/interfaces remains valid multi-vantage evidence. Only replay of the same accepted source record is deduplicated.

---

## 15. Flow DF-014 — Syslog ingestion

```mermaid
flowchart LR
  D[Device/App] -->|UDP/TCP/TLS| S[Dedicated Log Sensor Receiver]
  S --> P[Parse PRI/timestamp/structured data]
  P --> M[Device Pack parser + sensitive data policy]
  M --> E[Entity correlation]
  E --> I[Ingestion Gateway]
  I --> K[Kafka event.syslog]
  K --> CH[ClickHouse]
  K --> AE[Alert/Correlation]
```

---

## 16. Flow DF-015 — gNMI streaming telemetry

```mermaid
sequenceDiagram
  participant S as Sensor
  participant D as gNMI Target
  participant DP as Device Pack/YANG Mapping
  participant I as Ingestion

  S->>D: TLS/mTLS gNMI Subscribe
  D-->>S: Update stream
  S->>DP: Path/value normalization
  DP-->>S: Entity/metric/state mapping
  S->>I: Batched canonical telemetry with source timestamps
```

Subscription health is separately monitored so stream failure is not misclassified as device failure.

---

## 17. Flow DF-016 — Infrastructure Agent telemetry

```mermaid
sequenceDiagram
  participant A as Agent
  participant S as Assigned Infra Sensor
  participant SP as Sensor Spool
  participant I as Core Ingestion

  A->>A: Collect host/process/service/edge metrics
  A->>S: mTLS telemetry batch
  S->>S: Normalize / identity / quality
  S->>SP: Durable batch
  S->>I: Upload
```

---

## 18. Flow DF-017 — Agentless SSH/WinRM monitoring

```mermaid
sequenceDiagram
  participant S as Infra Sensor
  participant C as Credential Service/Core
  participant H as Host
  participant I as Ingestion

  S->>C: Resolve credential for permitted job
  C-->>S: Ephemeral authorized credential material/reference
  S->>H: SSH or WinRM HTTPS command/query
  H-->>S: Structured/raw response
  S->>S: Parse/normalize Device/Monitor Pack
  S->>I: Canonical metrics/state/events
```

---

## 19. Flow DF-018 — Synthetic test

```mermaid
flowchart LR
  POL[Synthetic Policy] --> S[Probe/Sensor]
  S --> T[Target DNS/TCP/TLS/HTTP/API]
  T --> R[Test timing/result]
  R --> N[Normalize availability/latency/validation]
  N --> I[Ingestion]
  I --> SLA[SLA/Alert Engine]
```

---

## 20. Flow DF-019 — Primary monitor failure → Alert Qualification

```mermaid
sequenceDiagram
  participant AE as Alert Engine
  participant QS as Qualification Service
  participant S as Assigned Sensor
  participant S2 as Secondary Sensor optional
  participant D as Device
  participant G as Graph Service

  AE->>QS: Primary SNMP failure candidate
  QS->>S: Retry SNMP + ICMP/TCP/API/traceroute per policy
  S->>D: Validation checks
  D-->>S: Mixed results
  S-->>QS: Evidence
  QS->>G: Neighbor/dependency/Sensor context
  G-->>QS: Topology/context
  opt Multi-vantage enabled
    QS->>S2: Independent validation
    S2-->>QS: Evidence
  end
  QS->>QS: Apply N-of-M/expression/quality
  QS-->>AE: Qualified outcome
```

Possible output is Monitoring Degraded, Credential Failure, Path Failure, Device Down, Sensor Failure or Inconclusive.

---

## 21. Flow DF-020 — Alert storm compression

```mermaid
flowchart TB
  E[Thousands of events] --> D[Deduplicate]
  D --> Q[Qualification]
  Q --> DEP[Dependency/Topology Correlation]
  DEP --> ST[Storm/Rate Grouping]
  ST --> I[One/Few Root Incidents]
  I --> CHILD[Child alerts preserved + inspectable]
  I --> N[Controlled notifications / ITSM]
```

---

## 22. Flow DF-021 — Incident correlation → RCA

```mermaid
sequenceDiagram
  participant C as Correlation Service
  participant R as RCA Service
  participant G as Graph Service
  participant CH as ClickHouse
  participant NCM as Change/Audit Sources
  participant ML as Anomaly/GNN Scorers

  C->>R: Incident candidate + child alerts
  R->>G: Extract temporal subgraph
  G-->>R: Nodes/edges/provenance/history
  R->>CH: Metrics/events/flow/telemetry-quality window
  CH-->>R: Evidence
  R->>NCM: Recent config/audit/maintenance changes
  NCM-->>R: Change evidence
  R->>R: Protocol/causal rules + temporal ordering
  R->>ML: Request anomaly/GNN scores when enabled
  ML-->>R: Learned scores + model/version
  R->>R: Fuse + contradiction check + confidence
  alt Sufficient evidence
    R-->>C: Root cause + causal chain + blast radius + evidence
  else Insufficient evidence
    R-->>C: INCONCLUSIVE + missing evidence + recommended checks
  end
```

---

## 23. Flow DF-022 — GNN shadow-mode evaluation

```mermaid
sequenceDiagram
  participant R as RCA Service
  participant F as Feature Builder
  participant G as GNN Runtime
  participant MR as Model Registry
  participant QA as Evaluation Store

  R->>F: Bounded temporal subgraph + evidence window
  F->>MR: Resolve approved SHADOW model
  MR-->>F: Model/schema/version
  F->>G: Node/edge/time features
  G-->>F: Root/edge/impact scores
  F->>QA: Save score + input snapshot + model version
  Note over R,G: Shadow result does not change production RCA conclusion
```

Promotion to production scoring requires benchmark acceptance.

---

## 24. Flow DF-023 — Conversational AI “Ask the network”

```mermaid
sequenceDiagram
  actor U as User
  participant UI as AI/Investigation UI
  participant AG as AI Gateway
  participant AUTH as Authorization
  participant T as Platform Tools
  participant GM as Gemini

  U->>UI: Natural-language operational question
  UI->>AG: Query + user/tenant context
  AG->>AUTH: Verify scope/tools/data policy
  AUTH-->>AG: Allowed tool set/data boundary
  AG->>T: Search/query graph/metrics/incidents as needed
  T-->>AG: Structured evidence with source IDs
  AG->>AG: Redact/mask sensitive fields
  AG->>GM: Grounded prompt + evidence/tool results
  GM-->>AG: Explanation/structured response
  AG->>AG: Validate referenced evidence/guardrails
  AG-->>UI: Answer + evidence links + uncertainty
```

LLM has no direct credential or unrestricted database access.

---

## 25. Flow DF-024 — RCA feedback

```mermaid
flowchart LR
  RCA[RCA Result] --> U[Operator feedback: Correct / Partial / Incorrect]
  U --> F[Feedback Store]
  F --> E[Evaluation / Model Quality Dashboard]
  F -. controlled offline process .-> TRAIN[Future Model Training]
```

Feedback never auto-retrains production models without governance.

---

## 26. Flow DF-025 — NCM backup

```mermaid
sequenceDiagram
  actor U as User/Schedule
  participant N as Core NCM
  participant P as Policy/RBAC
  participant S as Network Sensor
  participant D as Device
  participant ST as Config Store/Metadata
  participant TL as Timeline

  U->>N: Backup now / scheduled job
  N->>P: Authorization/window/credential check
  P-->>N: Allowed
  N->>S: Signed job lease + adapter + credential ref
  S->>D: SSH/NETCONF/API backup
  D-->>S: Configuration
  S-->>N: Config + hash + execution evidence
  N->>ST: Store version/metadata
  N->>TL: Emit config backup/change fact
```

---

## 27. Flow DF-026 — NCM push with approval and validation

```mermaid
sequenceDiagram
  actor R as Requester
  actor A as Approver
  participant N as NCM Service
  participant S as Sensor
  participant D as Device

  R->>N: Submit change + scope + ticket + dry run
  N->>N: RBAC, impact preview, Device Pack capability, window
  N-->>A: Approval request
  A->>N: Approve
  N->>S: Signed execution job
  S->>D: Pre-check / backup
  D-->>S: Pre-state
  S->>D: Config change
  D-->>S: Result
  S->>D: Post-validation
  alt Validation success
    S-->>N: Success + evidence
  else Validation failure and rollback supported
    S->>D: Rollback
    D-->>S: Rollback result
    S-->>N: Failed/rolled back + evidence
  end
```

---

## 28. Flow DF-027 — Browser CLI session

```mermaid
sequenceDiagram
  actor U as Engineer
  participant UI as xterm UI
  participant B as Core CLI Broker
  participant AUTH as RBAC/JIT/MFA Policy
  participant S as Network Sensor
  participant D as Device
  participant AUD as Audit

  U->>B: Request CLI(entity, reason/ticket)
  B->>AUTH: Evaluate role/scope/conditions
  AUTH-->>B: Allowed privilege level
  B->>S: Open short-lived authorized SSH session
  S->>D: SSH with protected credential
  D-->>S: Terminal
  S-->>B: Stream
  B-->>UI: WebSocket stream
  loop commands
    UI->>B: command
    B->>AUTH: command policy check if required
    B->>S: forward
    S->>D: command
    D-->>S: output
    S-->>B: output
    B->>AUD: command/result/session evidence
    B-->>UI: output
  end
```

---

## 28A. Flow DF-027A — Contextual on-demand diagnostic / multi-vantage verification

```mermaid
sequenceDiagram
  actor U as NOC User
  participant UI as Entity/Alert/Incident UI
  participant D as Diagnostic Orchestrator
  participant AUTH as RBAC/Scope/Policy
  participant SC as Sensor Control
  participant S1 as Assigned Sensor
  participant S2 as Alternate Sensor/Agent optional
  participant T as Target
  participant E as Evidence/Investigation

  U->>UI: Ping / Trace / SNMP / Verify Now
  UI->>D: diagnostic(origin,target,type,params,vantage mode)
  D->>AUTH: Authorize actor + target + action + parameters
  AUTH-->>D: Allowed/denied + limits
  D->>SC: Resolve eligible vantage(s) + capacity
  SC-->>D: Assigned/eligible bounded execution set
  par Assigned vantage
    D->>S1: Execute bounded typed test
    S1->>T: Protocol/network check
    T-->>S1: Result/timeout/error
    S1-->>D: Typed evidence + quality
  and Optional independent vantage
    D->>S2: Execute same bounded validation
    S2->>T: Protocol/network check
    T-->>S2: Result/timeout/error
    S2-->>D: Typed evidence + quality
  end
  D->>E: Persist/audit result refs + correlation
  D-->>UI: Per-vantage results + monitored-vs-live comparison
```

Contradictory vantage evidence is shown, not averaged into a false definitive state. Operator-triggered verification does not automatically rewrite continuously monitored state; qualification/evidence policy decides any state transition.

---

## 28B. Flow DF-027B — Investigation Session evidence capture

```mermaid
sequenceDiagram
  actor U as Engineer
  participant UI as Troubleshoot Workspace
  participant INV as Investigation Service
  participant DIAG as Diagnostics
  participant CLI as CLI Broker
  participant Q as Query/Graph/NCM/Flow
  participant INC as Incident Service

  U->>INV: Start Investigation(origin, reason)
  INV-->>UI: investigation_session_id
  U->>DIAG: Run diagnostic in investigation context
  DIAG-->>INV: Attach typed result reference
  U->>CLI: Open CLI in investigation context
  CLI-->>INV: Attach session/command evidence reference
  U->>Q: View/compare topology, config, flow, changes
  Q-->>INV: Attach selected evidence/snapshot references
  opt Incident exists or user links it
    INV->>INC: Link investigation + evidence index
  end
  U->>INV: Close Investigation
  INV-->>UI: Evidence summary + unresolved items
```

Investigation storage references authoritative evidence instead of copying entire telemetry datasets. Authorization is re-evaluated for each privileged action; being inside an investigation does not grant extra privilege.

---

## 29. Flow DF-028 — ITSM incident creation/update

```mermaid
sequenceDiagram
  participant I as Incident Service
  participant Q as Durable Integration Queue
  participant C as ITSM Connector
  participant T as ServiceNow/BMC/etc.

  I->>Q: Enqueue create/update with idempotency key
  Q->>C: Deliver
  C->>T: API request
  alt Success
    T-->>C: Ticket ID/state
    C-->>Q: Ack
    C->>I: Store correlation/ticket state
  else Failure
    T-->>C: Error/timeout
    C-->>Q: Retryable/non-retryable error
    Q->>Q: Backoff/retry/dead-letter policy
  end
```

Connector health exposes stuck/rejected operations.

---

## 30. Flow DF-029 — Incident recovery → ITSM resolution

```mermaid
sequenceDiagram
  participant AE as Alert/Recovery Engine
  participant I as Incident Service
  participant Q as Integration Queue
  participant T as ITSM

  AE->>AE: Primary recovery evidence
  AE->>AE: Stability/recovery qualification window
  alt Stable recovery
    AE->>I: Resolve Platform incident
    I->>Q: Apply configured ITSM recovery policy
    Q->>T: Update/resolve/close or add work note
  else Flapping/unstable
    AE->>I: Keep incident open / monitoring recovery
  end
```

---

## 31. Flow DF-030 — Policy change with impact preview

```mermaid
sequenceDiagram
  actor A as Admin
  participant UI as Policy UI
  participant P as Policy Service
  participant CH as Historical Analytics
  participant CAP as Capacity Model

  A->>UI: Change polling/threshold/retention policy
  UI->>P: Preview draft
  P->>CH: Simulate historical alert/data impact where applicable
  P->>CAP: Calculate Sensor/Core/storage impact
  CH-->>P: Estimated alerts/affected entities
  CAP-->>P: Capacity/storage projection
  P-->>UI: SAFE/WARNING/UNSAFE + details
  A->>UI: Apply/Cancel
  UI->>P: Create immutable new version
  P->>P: Audit + distribute effective config
```

---

## 32. Flow DF-031 — Adaptive polling

```mermaid
flowchart LR
  T[Trigger: alert/trap/syslog/anomaly/config/manual] --> P[Adaptive Policy]
  P --> C{Capacity/Safety Check}
  C -- Safe --> F[Increase polling temporarily]
  C -- Unsafe --> W[Limit/deny + warning]
  F --> OBS[Collect deeper/faster evidence]
  OBS --> E{Return condition/cooldown}
  E --> BASE[Restore baseline polling]
```

---

## 33. Flow DF-032 — Retention rollup

```mermaid
flowchart LR
  RAW[Raw telemetry] --> R1[Configured rollup 1]
  R1 --> R2[Configured rollup 2]
  R2 --> RL[Long-term rollup]
  RAW --> TTL{Raw retention expired?}
  TTL -- yes and required rollup complete --> DEL[Delete raw partition]
  TTL -- no --> KEEP[Keep]
  RL --> ARCH[Optional archive]
```

Rollup interval and retention are customer-configurable; labels here do not imply fixed periods.

---

## 34. Flow DF-033 — Historical dashboard query with automatic resolution

```mermaid
sequenceDiagram
  actor U as User
  participant UI as Dashboard
  participant Q as Query Service
  participant R as Retention/Resolution Catalogue
  participant CH as ClickHouse

  U->>UI: Select 2-year time range
  UI->>Q: Query(resolution=AUTO)
  Q->>R: Find available resolutions/retention
  R-->>Q: e.g. hourly rollup available
  Q->>CH: Query appropriate rollup
  CH-->>Q: Aggregated result
  Q-->>UI: Data + effectiveResolution
  UI-->>U: Chart + "hourly rollup" indicator
```

---

## 35. Flow DF-034 — Core 80% storage protection

```mermaid
sequenceDiagram
  participant M as Storage Monitor
  participant L as Lifecycle Service
  participant P as Purge Planner
  participant CH as ClickHouse/Volumes
  participant A as Audit/Alert

  M->>L: Volume usage >=80%
  L->>P: Build emergency reclaim plan
  P->>P: Respect data class priority, rollups, legal holds, recent window
  P->>CH: Drop/delete oldest eligible partitions/segments
  CH-->>P: Bytes reclaimed
  P->>M: Recheck utilization
  alt Safe headroom reached
    L->>A: Storage protection success + exact deleted ranges/classes
  else Insufficient eligible data
    L->>A: CRITICAL reclamation insufficient + required admin action
  end
```

Monitoring ingestion remains active throughout to the greatest technically safe extent.

---

## 36. Flow DF-035 — Sensor 80% storage protection during WAN outage

```mermaid
sequenceDiagram
  participant S as Sensor
  participant SP as Local Spool
  participant M as Sensor Storage Protector
  participant C as Core unreachable

  S->>SP: Continue polling + append telemetry
  S--xC: Upload fails
  M->>SP: Usage reaches warnings/80%
  M->>SP: Remove acknowledged/temp/old logs first
  alt Still above emergency threshold
    M->>SP: Apply oldest low-priority unacknowledged emergency drop policy
    M->>M: Count exact loss + preserve recent/high-priority data
  end
  M->>S: Continue current monitoring
  Note over S,M: On reconnect, lost-data counters are uploaded; no silent loss
```

---

## 37. Flow DF-036 — Legal/investigation hold

```mermaid
sequenceDiagram
  actor U as Authorized User
  participant H as Retention Hold Service
  participant P as Purge Planner

  U->>H: Hold incident/time range/data classes + reason
  H->>H: Validate privilege/approval and persist hold
  P->>H: Query protected ranges during retention/purge
  H-->>P: Exclusions
  P->>P: Purge other eligible data first
```

---

## 38. Flow DF-037 — Core Active failure and HA failover

```mermaid
sequenceDiagram
  participant A as Core-A Active
  participant B as Core-B Standby
  participant HA as HA Manager/VIP
  participant S as Sensors
  participant DB as Data Services

  A--xB: Heartbeat stops / node failure
  S->>S: Continue polling + local spool
  B->>HA: Evaluate failure/fencing safety
  alt Safe to promote
    B->>DB: Promote required standby data services
    B->>B: Start/promote Active Core modules
    B->>HA: Claim stable endpoint
    HA-->>S: Endpoint available on Core-B
    S->>B: Reconnect and replay buffered data
  else Ambiguous partition
    B->>B: Enter SPLIT_BRAIN_PREVENTED / do not become writable Active
    S->>S: Continue buffering
  end
```

Certified safe failure cases must restore Core service in <60 seconds.

---

## 39. Flow DF-038 — Former Active returns after failover

```mermaid
sequenceDiagram
  participant A as Former Core-A
  participant B as Current Core-B Active
  participant HA as HA Manager

  A->>HA: Rejoin request with old cluster epoch
  HA->>A: Reject active writes; force STANDBY_SYNCING
  A<->>B: Resynchronize PostgreSQL/ClickHouse/Kafka/application state per certified procedures
  B-->>A: Current epoch/config
  A->>HA: Sync complete
  HA->>A: STANDBY_READY
```

Old Core may never independently resume Active because it used to be Active.

---

## 40. Flow DF-039 — Sensor failure and polling reassignment

```mermaid
sequenceDiagram
  participant S1 as Sensor-1
  participant C as Sensor Control
  participant S2 as Sensor-2
  participant D as Devices

  S1--xC: Heartbeats stop
  C->>C: Lease expiry + capacity/locality evaluation
  C->>S2: Issue newer assignment/fencing versions
  S2->>D: Begin polling reassigned targets
  S2-->>C: First observations
  opt S1 returns
    S1->>C: Rejoin with stale assignment versions
    C-->>S1: New state; do not resume superseded jobs
  end
```

---

## 41. Flow DF-040 — Licence expiration lifecycle

```mermaid
sequenceDiagram
  participant L as Licence Service
  participant UI as Dashboards
  participant S as Scheduler/Sensor Control
  participant H as Historical Query

  L->>UI: 30-day warning notification
  L->>UI: 7-day global expiry banner
  L->>L: Licence expires; enter 7-day grace
  L->>S: Keep polling enabled during grace
  L->>L: Grace expires
  L->>S: Disable new polling assignments / signed polling-stop state
  L->>H: Historical queries remain enabled
  Note over L,H: Licence admin remains accessible for extension/renewal
```

---

## 42. Flow DF-041 — Licence renewed after polling stop

```mermaid
sequenceDiagram
  actor A as Licence Admin
  participant L as Licence Service
  participant P as Policy/Scheduler
  participant S as Sensors

  A->>L: Import/activate renewed licence
  L->>L: Validate signature/entitlements
  L->>P: Re-enable polling eligibility
  P->>S: New policy/assignment version
  S->>S: Resume polling
```

---

## 43. Flow DF-042 — Dashboard query and fast UI response

```mermaid
sequenceDiagram
  actor U as User
  participant UI as React UI
  participant API as Query API
  participant AUTH as RBAC
  participant V as Valkey
  participant CH as ClickHouse

  U->>UI: Open dashboard/filter/time range
  UI->>API: Typed query
  API->>AUTH: Scope authorization
  AUTH-->>API: Allowed scope
  API->>V: Cache lookup
  alt Cache hit
    V-->>API: Result
  else Miss
    API->>CH: Bounded optimized query / rollup
    CH-->>API: Result
    API->>V: Cache safe derived result
  end
  API-->>UI: Data + resolution + freshness
```

Large tables use server-side pagination; oversized exports become background jobs.

---

## 44. Flow DF-043 — Report execution

```mermaid
flowchart LR
  DEF[Report Definition + Schedule] --> JOB[Async Report Job]
  JOB --> AUTH[Scope/RBAC snapshot]
  AUTH --> Q[Query Service]
  Q --> DATA[ClickHouse/PostgreSQL Aggregates]
  DATA --> CALC[Safe formula/CEL calculations]
  CALC --> RENDER[PDFBox/POI/CSV/JSON renderer]
  RENDER --> DELIV[Download / Email / Integration]
  DELIV --> HIST[Execution history / audit]
```

---

## 45. Flow DF-044 — Device Pack creation from MIB

```mermaid
sequenceDiagram
  actor U as Admin/Engineer
  participant MS as MIB Studio
  participant AI as AI Gateway optional
  participant DP as Device Pack Studio
  participant T as Test Device/Sample

  U->>MS: Upload MIB(s)
  MS->>MS: Provenance/hash/dependency/compile/OID tree
  MS->>T: Optional walk/test
  T-->>MS: Sample values
  opt AI semantic help
    MS->>AI: Ask for metric/enum/visualization suggestions using parsed MIB context
    AI-->>MS: AI Suggested mappings
  end
  U->>MS: Confirm/edit mappings
  MS->>DP: Create Monitor Definitions/traps/topology semantics
  DP->>DP: Version/sign/test/publish
```

---

## 46. Flow DF-045 — API Studio connector creation

```mermaid
sequenceDiagram
  actor U as Admin
  participant AS as API Studio
  participant T as Target API
  participant AI as AI Gateway optional
  participant DP as Device Pack/Connector Store

  U->>AS: OpenAPI URL/file or manual endpoint
  AS->>T: Authenticated test request
  T-->>AS: JSON/XML response
  AS->>AS: Schema/entity/collection inference
  opt AI help
    AS->>AI: Suggest semantic mappings
    AI-->>AS: AI Suggested mappings
  end
  AS->>AS: Cardinality/MWU/rate preview
  U->>AS: Confirm mappings
  AS->>DP: Version/sign/publish connector
```

---

## 47. Flow DF-046 — Data masking before AI

```mermaid
flowchart LR
  SRC[Logs / Config / Inventory / RCA Evidence] --> CLASS[Data Classification]
  CLASS --> POL[AI Data Boundary Policy]
  POL --> DROP[Drop fields]
  POL --> MASK[Mask/hash fields]
  POL --> ALLOW[Allowed evidence]
  MASK --> CTX[AI Context Builder]
  ALLOW --> CTX
  CTX --> GEM[Gemini/Provider]
```

Credentials/private keys never enter the allowed path.

---

## 48. Flow DF-047 — Backup and restore verification

```mermaid
sequenceDiagram
  participant B as Backup Service
  participant PG as PostgreSQL
  participant CH as ClickHouse
  participant FS as Artifact/Object Store
  participant V as Verification

  B->>PG: Snapshot/backup
  PG-->>B: Backup reference/hash
  B->>CH: Backup required partitions/metadata
  CH-->>B: Backup reference/hash
  B->>FS: Save Device Packs/dashboards/policies/certs per secure policy
  B->>V: Build signed manifest + checksums
  V->>V: Verify readability/integrity/compatibility metadata
  V-->>B: VERIFIED or FAILED
```

A backup failure is a Platform alert.

---

## 49. Flow DF-048 — DR activation (architecture-level, exact RPO/RTO TBD)

```mermaid
sequenceDiagram
  actor A as Authorized DR Operator
  participant P as Primary Core/Site
  participant D as DR Core/Site
  participant S as Sensors/Agents
  participant DNS as DR Endpoint/DNS/VIP mechanism

  A->>D: Run DR readiness/restore/replication validation
  D-->>A: Ready / blocking issues
  A->>P: Confirm primary unavailable or planned DR switch
  A->>D: Authorize DR activation
  D->>D: Promote restored/replicated state per certified runbook
  A->>DNS: Activate DR stable Core endpoint
  DNS-->>S: Resolve/reconnect to DR Core
  S->>D: Re-enroll/reconnect using preserved trust/DR policy
  S->>D: Replay buffered telemetry as supported
```

Exact automation, node count and RPO/RTO remain TBD.

---

## 50. Flow DF-049 — Core upgrade in HA

```mermaid
sequenceDiagram
  actor A as Admin
  participant U as Upgrade Orchestrator
  participant CA as Core-A Active
  participant CB as Core-B Standby
  participant S as Sensors

  A->>U: Start approved upgrade
  U->>U: Preflight + backup + compatibility
  U->>CB: Upgrade Standby first where release supports
  CB-->>U: Health/schema compatibility ready
  U->>CA: Controlled switchover/drain if required
  S->>S: Continue polling/buffer through switch
  U->>CB: Promote upgraded node Active
  U->>CA: Upgrade former Active
  CA-->>U: Rejoin Standby and sync
  U-->>A: HA healthy / upgrade complete
```

Exact sequence may differ for schema-breaking releases but must remain documented/tested.

---

## 51. Flow DF-050 — Sensor canary upgrade

```mermaid
flowchart LR
  REL[New Sensor Release] --> C1[Canary Sensor/Pool Cohort]
  C1 --> DRN[Drain jobs to peers]
  DRN --> UP[Upgrade]
  UP --> VAL[Self-test + poll/replay validation]
  VAL -->|Pass| NEXT[Expand cohort]
  VAL -->|Fail| RB[Rollback/stop rollout]
  NEXT --> FULL[Full rollout]
```

---

## 52. Flow DF-051 — Cardinality explosion protection

```mermaid
sequenceDiagram
  participant I as Ingestion/Connector Runtime
  participant G as Telemetry Guardrail
  participant A as Admin UI

  I->>G: New labels/entities/metric series rate
  G->>G: Compare with baseline/limits
  alt Explosion detected
    G-->>A: Critical warning + offending connector/field + projected storage
    G->>I: Enforce configured rate/cardinality guardrail if safety policy requires
  end
```

The system reports dropped/rejected records explicitly.

---

## 53. Flow DF-052 — Telemetry quality → RCA confidence

```mermaid
flowchart LR
  EXP[Expected polls/streams] --> Q[Quality Engine]
  GOT[Received/late/duplicate/failed/clock state] --> Q
  Q --> SCORE[Quality score + missing evidence]
  SCORE --> RCA[RCA Confidence Calibrator]
  RCA --> OUT[Result / lower confidence / INCONCLUSIVE]
```

---

## 54. Flow DF-053 — Historical topology during incident investigation

```mermaid
sequenceDiagram
  actor U as Investigator
  participant UI as Incident/Topology UI
  participant G as Graph Service
  participant H as Topology History

  U->>UI: View topology at incident time
  UI->>G: getGraph(asOf=incident timestamp, layers=...)
  G->>H: Reconstruct relationships valid at time
  H-->>G: Historical edges/provenance
  G-->>UI: Time-consistent graph
```

---

## 55. Flow DF-054 — Unified Operations Timeline

```mermaid
flowchart TB
  MET[Metric anomalies/state changes] --> T[Timeline Aggregator]
  EVT[Traps/Syslog/Events] --> T
  AL[Alerts/Incidents] --> T
  CFG[NCM/Config changes] --> T
  TOPO[Topology changes] --> T
  AUD[User/Audit actions] --> T
  SENS[Sensor assignment/health] --> T
  ITSM[ITSM lifecycle] --> T
  RCA[RCA runs] --> T
  T --> UI[Typed chronological timeline with source links]
```

---

## 56. Flow DF-055 — Query-too-large protection

```mermaid
sequenceDiagram
  actor U as User
  participant UI as UI
  participant Q as Query Service

  U->>UI: Request 5 years raw flow export
  UI->>Q: Query
  Q->>Q: Estimate scan/rows/cost exceeds interactive limits
  Q-->>UI: Refuse interactive execution; offer background export / aggregate resolution
  U->>UI: Choose background export
  UI->>Q: Submit async job
```

---

## 57. Flow DF-056 — Integration test sandbox

```mermaid
sequenceDiagram
  actor A as Integration Admin
  participant UI as Integration Studio
  participant C as Connector
  participant T as ITSM/Endpoint

  A->>UI: Test connector
  UI->>C: Connection/auth test
  C->>T: Health/auth
  T-->>C: Success
  C->>T: Create test item
  T-->>C: ID
  C->>T: Update test item
  C->>T: Resolve/delete according to sandbox semantics
  C-->>UI: PASS/FAIL per stage + field mapping evidence
```

---

## 58. Flow DF-057 — Support bundle

```mermaid
flowchart LR
  REQ[Authorized support-bundle request] --> COL[Collect versions/health/logs/config metadata]
  COL --> RED[Redact credentials/tokens/private keys/sensitive fields]
  RED --> MAN[Manifest + correlation IDs + timestamps]
  MAN --> B[Encrypted/signed support bundle]
  B --> AUD[Audit event]
```

---

## 59. Flow DF-058 — Change freeze calendar

```mermaid
sequenceDiagram
  actor U as User
  participant OP as Operational Calendar
  participant N as NCM/Upgrade/Device Pack Job

  U->>N: Request risky change during freeze
  N->>OP: Evaluate freeze/maintenance/business calendar
  OP-->>N: BLOCKED unless emergency override role/approval
  N-->>U: Explain freeze window and allowed override process
```

---

## 60. Flow DF-059 — Post-incident review generation

```mermaid
flowchart LR
  INC[Closed Incident] --> PIR[PIR Generator]
  TL[Unified Timeline] --> PIR
  RCA[RCA/Evidence] --> PIR
  ITSM[Ticket/Notes] --> PIR
  CHG[Config/Changes] --> PIR
  PIR --> DRAFT[Draft PIR: impact, cause, evidence, actions, recovery]
  DRAFT --> HUMAN[Human corrective/preventive actions + approval]
  HUMAN --> PDF[Report/export]
```

---

## 61. Flow DF-060 — Platform self-monitoring

```mermaid
flowchart TB
  CORE[Core health] --> PH[Platform Health Engine]
  PG[PostgreSQL] --> PH
  CH[ClickHouse] --> PH
  K[Kafka] --> PH
  S[Sensor fleet] --> PH
  AG[Agents] --> PH
  INT[Integrations] --> PH
  CERT[Certificates/Licence] --> PH
  ST[Storage] --> PH
  PH --> DASH[Platform Operations Dashboard]
  PH --> ALERT[Internal Platform Alerts]
```

Internal platform failure alerts are classified separately from customer-device alerts.

---

## 62. Flow DF-061 — User access request and authorization

```mermaid
sequenceDiagram
  actor U as User
  participant UI as UI
  participant IDP as IdP/Local Auth
  participant API as Core API
  participant AUTH as Authorization Engine

  U->>UI: Login
  UI->>IDP: OIDC/SAML/local/MFA
  IDP-->>UI: Authenticated subject/session
  UI->>API: Request resource/action
  API->>AUTH: Who + tenant + resource + action + conditions
  AUTH-->>API: Allow/Deny + explanation
  alt Allow
    API-->>UI: Scoped result
  else Deny
    API-->>UI: 403 / policy explanation according to role
  end
```

---

## 63. Flow DF-062 — Credential rotation

```mermaid
sequenceDiagram
  actor A as Credential Admin
  participant C as Credential Service
  participant T as Test Sensor
  participant D as Sample Target
  participant P as Policy/Assignments

  A->>C: Update/rotate credential profile
  C->>C: Store new encrypted version; preserve audit metadata
  C->>T: Test new version from eligible Sensor
  T->>D: Protocol authentication test
  D-->>T: Result
  T-->>C: Verified/failed
  alt Verified
    C->>P: Activate credential version for scoped targets
  else Failed
    C-->>A: Do not activate / provide diagnostic evidence
  end
```

---

## 64. Flow DF-063 — Data deletion due to normal retention vs emergency

```mermaid
flowchart TB
  DATA[Data partition/segment] --> MODE{Deletion reason}
  MODE -->|Normal TTL| ROLL{Required rollup/archive complete?}
  ROLL -- No --> KEEP[Retain / retry rollup]
  ROLL -- Yes --> HOLD{Protected/legal hold?}
  MODE -->|Emergency >=80%| HOLD
  HOLD -- Yes --> ALT[Choose another eligible target / raise conflict if insufficient]
  HOLD -- No --> DEL[Delete oldest eligible]
  DEL --> AUD[Audit exact range/class/bytes/reason]
```

---

## 65. Flow DF-064 — Device comparison

```mermaid
sequenceDiagram
  actor U as Engineer
  participant UI as Compare UI
  participant Q as Query Service
  participant G as Graph/Inventory
  participant N as NCM

  U->>UI: Compare Device A vs Device B / time A vs time B
  UI->>G: Identity/model/profile/topology attributes
  UI->>Q: Metrics/alerts/events same normalized keys/time
  UI->>N: Config versions/diff where authorized
  G-->>UI: Inventory/topology differences
  Q-->>UI: Performance/state differences
  N-->>UI: Config differences
  UI-->>U: Aligned comparison with missing/incompatible fields marked
```

---

## 66. Flow DF-065 — Runbook recommendation

```mermaid
flowchart LR
  RCA[RCA cause + entity/device pack + symptoms] --> KB[Approved Runbook Library]
  KB --> MATCH[Rank relevant approved runbooks]
  MATCH --> UI[Operator recommendation]
  UI --> AI[Optional AI summary of approved runbook]
```

Customer-approved runbook remains authoritative; AI summary cannot modify executable steps.

---

## 67. Flow DF-066 — Future controlled remediation

Future/Phase 2 only:

```mermaid
flowchart LR
  RCA[Evidence-backed RCA] --> REC[Recommended action]
  REC --> PRE[Preconditions / blast radius]
  PRE --> APP[Approval / policy gate]
  APP --> EX[Execute via Sensor/automation adapter]
  EX --> VAL[Validate service/alert state]
  VAL -->|Pass| CLOSE[Record success]
  VAL -->|Fail| RB[Rollback if supported]
  RB --> AUD[Audit / escalation]
```

No direct LLM-to-device execution.

---

## 68. Data ownership matrix

| Data | System of record | Secondary/history |
|---|---|---|
| Tenant/users/roles | PostgreSQL | Audit history |
| Current inventory | PostgreSQL | ClickHouse observation/history |
| Relationships current | Graph metadata/PostgreSQL projection | ClickHouse topology history |
| Monitoring policies | PostgreSQL immutable versions | Audit/export |
| Credentials | Encrypted secret subsystem + PostgreSQL metadata | No plaintext history |
| Metrics | ClickHouse | Optional archive/rollups |
| Flow | ClickHouse | Rollups/archive |
| Raw events/syslog/traps | ClickHouse | Archive/rollups |
| Current alerts/incidents | PostgreSQL | ClickHouse lifecycle history |
| NCM metadata/current versions | PostgreSQL + protected config artifact store | Version history/audit |
| IPAM current | PostgreSQL | ClickHouse utilization/history |
| Dashboards/reports | PostgreSQL | Export/backup |
| Audit | ClickHouse/tamper-evident history + indexes | SIEM/archive |
| Sensor spool | Sensor local disk | Deleted after durable acceptance |
| GNN/model metadata | PostgreSQL/model artifact store | Evaluation history |
| Kafka | Internal transient/durable stream | Not long-term system of record |

---

## 69. Dataflow invariants

1. Tenant context is present from ingestion through storage/query.
2. Sensor/Agent identity is cryptographically verified.
3. No browser gets direct DB credentials or reusable device secrets.
4. No Sensor needs synchronous WAN database access to complete a poll.
5. No local spool segment is silently dropped; emergency loss is counted/audited.
6. Event time and ingest time remain distinct.
7. Duplicate/replay is expected and must be idempotently handled.
8. Correlation never destroys underlying evidence.
9. AI output is not a replacement for source evidence.
10. GNN score is not proof of causality.
11. Historical data may be purged to protect live monitoring at the mandatory storage watermark according to policy.
12. HA ambiguity must not create two writable authoritative Core nodes.

---

## 70. Dataflow acceptance tests

QA must trace correlation IDs end-to-end for at least:

- one SNMP sample;
- one SNMP auth failure provisioning case;
- one trap;
- one NetFlow record;
- one Agent metric;
- one device-down qualification;
- one correlated incident/RCA;
- one ITSM ticket lifecycle;
- one NCM change;
- one storage emergency purge;
- one Core failover/replay;
- one Sensor failure/reassignment;
- one licence stop/resume;
- one AI query with evidence/tool audit;
- one GNN shadow run;
- one historical topology reconstruction.

Each test must prove source → normalized message → persistence → downstream outcome → audit/visibility with no unexplained data loss.

---

## 71. Explicit Dataflow TBDs

- Exact durable ingest acknowledgment boundary under the final two-Core Kafka/ClickHouse design.
- Exact HA fencing mechanism.
- Exact DR activation/endpoint switch automation and RPO/RTO.
- GNN runtime/framework and feature serving transport.
- Exact default Sensor spool size/retention by profile after benchmark.


---

## 72. Flow DF-067 — Desired state reconciliation

```mermaid
sequenceDiagram
  participant P as Policy/Sensor Control
  participant S as Sensor/Agent
  participant R as Reconciliation Service
  participant O as Platform Operations

  P->>S: Desired assignment/policy/version hash
  S-->>P: Applied/observed version hash
  P->>R: Desired state
  S->>R: Observed state
  R->>R: Compare
  alt Mismatch
    R->>O: Drift case + explanation/remediation
  else Match
    R->>O: In sync
  end
```

## 73. Flow DF-068 — Migration Studio import

```mermaid
flowchart LR
  SRC[Existing tool export/API/CSV] --> ADP[Source Adapter]
  ADP --> STG[Staging Canonical Model]
  STG --> MAP[Map entities/sites/groups/settings]
  MAP --> VAL[Identity/capacity/semantic validation]
  VAL --> REV[User Review: mapped / warning / unsupported]
  REV --> COM[Commit approved objects]
  COM --> PROV[Normal Platform provisioning/reconciliation]
```

## 74. Flow DF-069 — Alert policy effectiveness recommendation

```mermaid
flowchart LR
  HIST[Alert/Incident/Suppression History] --> E[Effectiveness Analyzer]
  E --> SCORE[Noise/repeat/incident-conversion metrics]
  SCORE --> REC[Review recommendation]
  REC --> USER[Admin review]
  USER -->|approved change| POL[Normal policy preview/version workflow]
```

## 75. Flow DF-070 — Pipeline integrity reconciliation

```mermaid
flowchart LR
  S[Sensor produced/acked watermarks] --> R[Integrity Reconciler]
  I[Ingestion accepted/rejected] --> R
  K[Kafka produced/consumer counts] --> R
  C[ClickHouse inserted/deduped] --> R
  R --> Q{Explained?}
  Q -- Yes --> DASH[Integrity dashboard]
  Q -- No --> ALERT[Internal data-loss/gap alert]
```


---

# Version 3.0 Reconciliation — Requirements That Must Not Be Lost

This section is part of the authoritative document. It was added after a complete reconciliation of the Platform design discussions because the earlier split-document version compressed or omitted details. The canonical requirement IDs are in `Platform_08_Requirements_Traceability.md`; OSS decisions are in `Platform_07_OSS_Catalog.md`.

The following cross-cutting capabilities are mandatory where applicable and must be represented in design, UI, APIs, security, audit and QA:

- asset lifecycle/reconciliation, stale/disappeared/replaced device handling and decommission workflow;
- technology lifecycle/EOL/EOS/firmware risk visibility without pretending to be a vulnerability scanner;
- backup, restore verification, v1.0 DR, low-downtime upgrade, rollback, canary and feature-flag controls;
- desired-vs-actual monitoring state, drift detection, change impact preview and dry-run;
- path analysis, device/time comparison, circuit inventory, business/service ownership and shift handover;
- learned baselines/seasonality, alert storm protection, flapping detection, event compression and alert-policy effectiveness;
- unified Operations Timeline combining telemetry, events, topology, NCM, audit, user action, sensor activity, ITSM and RCA;
- data provenance/quality/confidence, topology relationship provenance/confidence and historical topology;
- global search, command palette, contextual troubleshooting actions, support bundle and customer-visible Platform Health;
- licensing/usage dashboard, data-growth forecast and entitlement-aware UI;
- sensitive-field classification, masking/drop/hash/store policy, AI data boundaries and AI audit;
- Device Pack certification/test simulator, import/export, template library, onboarding wizard and contextual help;
- API-first implementation, webhooks/outbound events, API governance, rate limits and query protection;
- integration test sandbox, timezone/site timezone handling, internationalization foundations and accessibility;
- telemetry/cardinality explosion protection, tenant/noisy-neighbor quotas, ingestion dedupe, late/out-of-order/replay handling and legal hold;
- Migration Studio and Environment Readiness Scanner;
- change-freeze/business calendars, runbook/knowledge library and post-incident review/problem management;
- RCA accuracy/feedback metrics, monitoring optimization recommendations and Sensor placement learning;
- certificate/PKI lifecycle, proxy/no-proxy and data-sovereignty/regional-placement readiness;
- IPv6/dual-stack first-class behavior;
- scheduled housekeeping and end-to-end pipeline-integrity visibility;
- stable customer-facing error codes/correlation IDs and **No Silent Failure**.


## DF-071 — OSS dependency admission and release

```mermaid
sequenceDiagram
  actor Dev as Engineer
  participant Cat as OSS Catalogue
  participant CI as CI Pipeline
  participant ORT as ORT
  participant Syft as Syft
  participant Trivy as Trivy
  participant SC as ScanCode
  participant Rel as Release Store
  Dev->>Cat: Propose exact component/version
  Cat-->>Dev: Approved / Yellow review / Rejected
  Dev->>CI: Commit dependency lock
  CI->>ORT: License/policy/transitive evaluation
  CI->>Syft: Generate SPDX + CycloneDX SBOM
  CI->>Trivy: Vulnerability/license/secret scan
  alt Ambiguous/source-level review required
    CI->>SC: Deep license/provenance scan
  end
  alt RED or unapproved YELLOW/UNKNOWN
    CI-->>Dev: Build fails with evidence
  else Approved
    CI->>Rel: Package binary + SBOM + notices + licenses + checksums + provenance
  end
```

## DF-072 — SNMP polling through GoSNMP adapter

```mermaid
sequenceDiagram
  participant C as Core Scheduler
  participant S as Network Sensor
  participant A as SnmpAdapter/GoSNMP
  participant D as Device
  participant I as Core Ingestion
  C-->>S: Desired monitoring job/profile
  S->>S: Capacity + credential + Device Pack validation
  S->>A: Execute planned GetBulk/Walk
  A->>D: SNMP request using configured v1/v2c/v3 security
  D-->>A: Response / protocol error / timeout
  A-->>S: Canonical observations + normalized error
  S->>S: Qualification/retry policy
  S->>I: mTLS batch with source sequence IDs
```

## DF-073 — MIB Studio to Device Pack

```mermaid
sequenceDiagram
  actor Admin
  participant UI as MIB Studio
  participant MC as MibCompiler/gosmi
  participant Prov as Provenance Store
  participant Test as Test Device
  participant Pack as Device Pack Registry
  Admin->>UI: Upload MIB(s)
  UI->>Prov: Record source/owner/license/redistribution status
  UI->>MC: Parse + resolve imports + validate
  MC-->>UI: OID tree/tables/enums/traps/errors
  Admin->>UI: Select/confirm metrics and semantics
  UI->>Test: Walk/test selected definitions via eligible Sensor
  Test-->>UI: Sample values/types/errors
  Admin->>UI: Confirm mapping/dashboard/alerts
  UI->>Pack: Build versioned signed Device Pack
```

## DF-074 — Flow ingestion through GoFlow2

```mermaid
sequenceDiagram
  participant E as Exporter
  participant SS as Streaming Sensor
  participant F as FlowDecoder/GoFlow2
  participant I as Core Ingestion
  participant K as Kafka
  participant EN as Enrichment
  participant CH as ClickHouse
  E->>SS: NetFlow/IPFIX/sFlow datagrams
  SS->>F: Decode template/record
  F-->>SS: Canonical raw flow
  SS->>SS: local sequence + buffer/cardinality protection
  SS->>I: mTLS batched Flow records
  I->>K: telemetry.flow.v1
  K->>EN: consume
  EN->>EN: enrich entity/interface/site/IPAM/ASN/VRF/QoS
  EN->>CH: Insert network_flow
```

## DF-075 — gNMI/YANG subscription

Sensor loads Device Pack/YANG mapping, validates subscription paths using goyang/ygot metadata, establishes OpenConfig gNMI subscription, applies backpressure/rate policy, converts updates/deletes to canonical telemetry, and sends batches through Core Ingestion. Subscription failure produces a structured collection-health event rather than a generic `device down` condition.

## DF-076 — API Studio OpenAPI connector

User imports OpenAPI/Swagger or configures endpoint -> kin-openapi validates schema -> authentication/test executes through selected Sensor -> response preview/inference -> user confirms entity/metric mapping -> Device Pack/Connector version is published -> scheduler issues jobs -> Sensor executes paging/token refresh/chained calls -> canonical observations enter ingestion.

## DF-077 — NCM controlled change using OSS worker

Core creates approved NCM job -> NCM Worker uses Nornir/scrapli/scrapli_cfg/approved driver -> retrieves pre-change config -> computes diff -> maker/checker approval where policy requires -> executes -> validates -> captures post-change config -> rollback on policy/validation failure -> emits NCM change evidence to audit and RCA timeline.

## DF-078 — Dashboard query path

Browser React/TanStack Query -> Core Query API -> authorization/resource-scope check -> query planner selects raw or rollup resolution -> ClickHouse/PostgreSQL as appropriate -> result limits/timeout -> response -> ECharts/TanStack/Cytoscape/MapLibre render. Browser never directly queries databases.

## DF-079 — 80% Core storage protection

Per-volume Storage Lifecycle monitor detects managed volume >=80% -> marks emergency protection active -> calculates protected/legal-hold/nonpurgeable data -> selects oldest eligible raw partitions by configured purge priority -> preserves rollups/recent minimum where possible -> drops/TTLs efficient partitions -> continues ingestion -> records reclaimed bytes, effective retention loss and actions -> raises visible Platform event/audit until safe headroom restored.

## DF-080 — Sensor 80% disk protection

Sensor local volume reaches protection watermark -> clean disposable logs/cache/temp first -> evaluate spool age/priority -> preserve recent/critical unsent records first -> discard only oldest eligible lower-priority buffered data when unavoidable -> continue polling -> increment `sensor_data_dropped_due_to_storage` counters -> report exact loss to Core when connectivity resumes.

## DF-081 — License expiry

T-30 warning event -> T-7 persistent dashboard banner -> expiry enters 7-day grace while Scheduler continues -> grace expiry transitions deployment to `POLLING_SUSPENDED` -> Scheduler stops new/renewed poll jobs -> Sensors receive suspension desired state -> historical query/UI/admin/license import remain available -> renewal returns state to VALID and polling resumes.

## DF-082 — GNN evidence scoring and RCA fusion

Incident candidate graph/time window -> Graph Feature Builder -> versioned graph snapshot -> GraphScoringService (candidate PyTorch/PyG) -> root/propagation/impact scores -> RCA Fusion combines protocol/topology/config/anomaly/telemetry-quality evidence -> contradiction/missing-evidence checks -> root hypothesis or INCONCLUSIVE -> evidence/causal chain/blast radius stored -> Gemini may explain/summarize through AI Gateway but cannot alter RCA truth.

## DF-083 — Legal hold

Authorized user selects incident/time/entities -> Legal Hold Service resolves raw/rollup/event/flow/config/audit evidence references -> marks eligible partitions/records as protected -> storage-purge planner excludes held evidence where feasible/required -> hold release requires authorized audited action.

## DF-084 — Integration test sandbox

Admin configures ITSM/notification connector -> Test Sandbox verifies DNS/TLS/auth -> executes safe test create/update/resolve where supported -> validates field mapping and permissions -> connector cannot be marked Production Ready until required checks pass or an authorized exception is recorded.

## DF-085 — Migration Studio

Import source/CSV/API -> parse source inventory/groups/tags/threshold metadata -> normalize -> map to Platform entities/sites/groups/credential/profile placeholders -> classify automatically mapped / review / unsupported -> user approves staging -> run normal Provisioning Preflight/Sensor Placement/Capacity Simulation -> activate only approved entities.

## DF-086 — Support bundle

Authorized admin requests support bundle -> Support Bundle Service gathers versions/health/config metadata/relevant logs/Kafka/ClickHouse/PostgreSQL/Sensor diagnostics -> Secret Redaction Engine removes credentials/tokens/keys -> manifest/checksums generated -> audit records bundle creation/download.

## DF-087 — Certificate rotation

PKI admin imports/generates new certificate/CA -> validation -> staged trust overlap -> Core/Sensor/Agent receives new trust material through secure policy -> endpoints rotate -> health validation -> old certificate removed after overlap -> audit event. Failed rotation retains old trusted path until policy-expiry/safety rule.

## DF-088 — Change impact/dry-run

Admin proposes broad polling/threshold/NCM/retention/Sensor policy change -> Impact Service evaluates affected entities, Sensor workload, storage, expected historical alert rate or NCM targets -> returns SAFE/WARNING/UNSAFE with evidence -> dry-run produces no production mutation -> authorized user applies only after review/approval policy.


## Mother-reference cross-document contract (v3.1)

The following supporting mother references are normative and must be read together with this document:

- `Platform_07_OSS_Catalog.md` — approved/candidate OSS by module, license class, usage, rationale and restrictions.
- `Platform_08_Requirements_Traceability.md` — completeness control; a missing mapped requirement is a documentation defect.
- `Platform_09_UI_UX_Master.md` — detailed global shell, Studios, 98 out-of-box dashboard/workspace screen contracts, filters/actions/states/RBAC behavior.
- `Platform_10_Protocol_Security_Connector_Master.md` — protocol/security/authentication/credential/connectivity matrix and provisioning diagnostics.
- `Platform_11_Sizing_Capacity_Storage_Master.md` — Core/Sensor compute targets, MWU, placement, retention/storage and 80% protection.
- `Platform_12_Decision_Source_Register.md` — USER-FIXED vs ARCH-DELEGATED vs REFERENCE-LEARNING vs TBD/P0 decisions.

**OSS rule:** implementation teams may not add a runtime/build dependency merely because it is open source. It must be admitted to the Platform OSS Catalogue by exact version, transitive license/CVE scan and release policy.


## DF-089 — Future regional data-sovereignty routing

**Status:** architecture-ready/future federation, not initial multi-Core scope.

1. Tenant/site policy resolves permitted data region.
2. Regional Core/Sensor placement validates policy before telemetry assignment.
3. Raw telemetry remains in the permitted regional store.
4. Only policy-permitted summaries/incidents/inventory are federated to Master Core.
5. Cross-region drill-down queries are authorized and routed to the owning region rather than copying unrestricted raw data centrally.
6. Audit records the requesting user, region and data class.

No production claim is made until this architecture is implemented and validated.

## DF-090 — Developer laptop reboot / Docker Compose recovery — v3.1

1. Engineer shuts down/reboots laptop normally.
2. Docker daemon stops containers; named volumes retain durable state.
3. Host boots and Docker daemon starts automatically.
4. `dev` services configured with restart policy restart.
5. Data services perform their own crash/recovery checks.
6. Health checks prevent dependent Platform services from treating process start as readiness.
7. Core/Sensors/UI reconnect using retry/backoff.
8. Developer can verify `docker compose ps`/Platform health and continue without host-runtime repair.
9. If recovery fails, documented reset/rebuild path exists and failure logs are retained.

## DF-091 — Linux Core/Sensor/Agent boot and recovery

1. Linux host boots.
2. systemd starts enabled Platform service after required OS/network dependencies.
3. Service loads protected configuration and identity.
4. Service emits STARTING lifecycle event.
5. Dependency connections use bounded retry/backoff.
6. Service reaches HEALTHY or DEGRADED with reason code.
7. If process crashes, systemd performs bounded restart according to policy.
8. Repeated failure creates a visible health condition and preserves diagnostic logs.

## DF-092 — Windows Agent boot and recovery

1. Windows boots.
2. SCM starts PlatformAgent service automatically.
3. Agent loads enrollment identity/certificate/configuration.
4. Agent contacts Core, receives/refreshes Sensor assignment as required.
5. Agent resumes collection and store-and-forward sequence.
6. Failure triggers configured SCM recovery action.
7. Repeated failures are logged to Windows Event Log/Agent diagnostics and surfaced to Core when communication resumes.

## DF-093 — Native package upgrade

1. Preflight validates OS, free disk, compatibility and package signature.
2. Current configuration/state is backed up or checkpointed.
3. Service drains or pauses safely according to component role.
4. Package manager installs signed `.deb`/`.rpm` or MSI update.
5. Migration runs idempotently.
6. Service restarts automatically.
7. Health/compatibility checks execute.
8. On failure, rollback path is invoked where supported and incident/support evidence is retained.
