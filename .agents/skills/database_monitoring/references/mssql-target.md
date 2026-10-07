# Microsoft SQL Server monitored target

- Connectivity via approved driver path only (OSS catalogue constrained).
- Sessions/waits/blocking/filegroup capacity with least-privilege role.
- Version compatibility gate; unsupported builds ⇒ UNSUPPORTED.
- No arbitrary T-SQL remediation; no sa-by-default.
- Collector failure ≠ database DOWN without corroboration.
