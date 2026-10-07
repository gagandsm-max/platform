# PostgreSQL monitored target

- Connectivity: bounded connect + simple liveness probe.
- Sessions/waits/blocking: use least-privilege views; bound row counts.
- Capacity: database size, tablespace/filesystem evidence where authorized.
- Permission denied / timeout / malformed: typed collector failures.
- Collector failure ≠ database DOWN without corroboration.
- Distinct from Platform control-plane `postgresql` Skill.
