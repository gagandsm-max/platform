# Agent vs WinRM/WMI/CIM

## Planes
- Windows Agent service: preferred for continuous evidence; MSI/SCM lifecycle.
- WinRM/WMI/CIM agentless: explicit capability/auth matrix; UAC/firewall impact.

## Evidence
- Services (SCM), Event Log, Perf counters—bounded collection.
- Auth failure and firewall blocks are collector evidence, not host DOWN alone.
- Version differences (Server SKUs) must be tested or UNVERIFIED.
- Privacy: no desktop surveillance; operational telemetry only.

## Critical invariant
Collector failure ≠ host DOWN.
