# Pre/post validation and rollback evidence

## Pre-change
- Reachability/collector health separation from target operational state
- Baseline running/startup capture
- Preconditions from Change Plan (site, criticality, dependency notes)
- Dry-run / candidate diff when certified for the family

## Post-change
- Re-acquire configuration
- Validate expected diff scope only—no silent extra changes
- Service/reachability corroboration appropriate to the change class
- Record contradictions and missing evidence explicitly

## Rollback evidence
- Rollback plan reference and authorization path (`remediation_safety`)
- Pre-rollback baseline and post-rollback validation
- Never equate “rollback script exists” with “rollback authorized/safe”
