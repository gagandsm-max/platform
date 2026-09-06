# Platform Engineering Evidence & GA Programme Workspace

This directory tree houses engineering evidence, spike records, test reports, and benchmarks accumulated across the 30-day commercial GA programme for **Platform v1.0**.

## Authority Notice

* **Authoritative Mother Documents:** [`project_documentation/`](../project_documentation/) is the sole architectural and product source of truth.
* **Non-Authoritative Role:** Nothing in this `engineering/` tree overrides or replaces any mother-reference document. Findings here serve as technical evidence and execution artifacts to resolve open P0/TBD items through formal change control.
* **Governance:** Governed strictly by [`AGENTS.md`](../AGENTS.md).

## Hierarchy

```text
engineering/
├── README.md                  # This file
├── decisions/                 # Engineering implementation notes and architecture decision records (ADRs)
├── spikes/                    # The 10 formal P0/TBD engineering spikes
├── evidence/                  # Day-by-day accumulated test and verification evidence
│   └── day-01/                # Day 1 engineering baseline and regression evidence
├── benchmarks/                # Performance, throughput, and sizing measurements (SNWP-2500, EPS, flow)
├── compatibility/             # OS, hardware, target device, and protocol compatibility matrices
└── release/                   # Packaging, SBOM, CVE, and release gate certification reports
```
