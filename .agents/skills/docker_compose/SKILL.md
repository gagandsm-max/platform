---
name: docker_compose
description: Deterministic Docker Compose development, isolation, health checks and project-scoped teardown without host runtime installs.
---

<!-- generated from project_agents/skills/docker_compose/SKILL.md sha256:0173f946c85f24a8e0cf44124a6a0369267da94f6b9f94e8cef607389ab040ca -->

# docker_compose

## Purpose

Run Platform development and automated tests through Docker Compose with deterministic project naming, isolation and safe teardown—never by installing project runtimes on the host or pruning unrelated developer state.

## Use when

Editing compose files, dev Makefiles, test harness containers, local integration stacks, or diagnosing “port already allocated” / volume clashes. Required for routine `make doctor|certs|dev-up|test-unit` work.

## Authoritative inputs

Always read:

1. Root `AGENTS.md` / Cursor Cloud contract: Docker-only project dependencies
2. Compose files and `.env.example` (never commit `.env`)
3. Project name / `-p` conventions and documented ports (UI `23000`, Core `28443` defaults)
4. Volume names and healthcheck definitions
5. Existing persistent developer containers/volumes inventory before destructive actions

## Invariants

- Deterministic **project naming** so multiple checkouts do not collide blindly.
- Network and port isolation; no assuming host port 80 for the UI.
- Named volumes for reboot-safe data where compose defines them.
- Secrets via env files/secrets—never baked into images or logged.
- Health checks and service dependency order are required for readiness claims.
- Never run global Docker prune (`docker system prune`, volume prune); that can destroy unrelated persistent developer containers/volumes.
- Clean teardown is limited to the compose project.
- Production-slice fidelity: test compose must not silently disable TLS/auth “for convenience” without test-only labelling.
- Test isolation: parallel jobs need unique project names/networks.
- Failure diagnostics: capture `compose ps`, health, and service logs bound to the project.
- Compose rendering/validation before claiming stack-up.

## Required procedure

1. Confirm host will not receive project Java/Go/Node/PG/Kafka installs; use compose/Make targets.
2. Copy `.env.example` → `.env` when missing; ensure `.env` stays untracked.
3. Select explicit project name; document ports; check for collisions with persistent containers.
4. Validate compose config (`docker compose … config`) before up.
5. Bring up with health dependency awareness; wait on healthchecks, not fixed sleeps alone.
6. Run tests through documented containerized runners; keep network host mode only when the harness requires it and document why.
7. On failure: collect project-scoped logs/ps; do not prune globally.
8. Tear down with project-scoped `down` (volumes only when the task explicitly authorizes data reset).
9. Never remove unrelated containers named for other developers/products on the machine.

## Negative / failure cases

- `docker volume prune` deleting another engineer’s PG data.
- Binding UI to host port 80.
- Committing `.env` with secrets.
- Disabling healthchecks to “speed up” CI.
- Assuming a single global compose project name across parallel agents.
- Installing JDK on the host because “compose was slow”.

## Security / tenant implications

Compose must not expose plaintext management ports beyond documented loopback bindings. Certs/keys in `deploy/docker/certs` stay local and uncommitted when generated. Tenant fixtures in containers are synthetic. Do not mount host credential stores casually into services.

## Required evidence

- Compose file paths + project name
- `compose config` / up transcripts
- Healthcheck readiness evidence
- Port/network collision check notes
- Tear-down scope used
- Test container commands and results
- Confirmation no global prune executed

## Stop conditions

Stop when:

- Required ports are owned by unrelated persistent containers and cannot be remapped safely
- Secrets would be committed or logged
- Task asks for global prune/destructive cleanup of shared Docker state
- Health can only be claimed by ignoring failed healthchecks

## Anti-patterns

- Host runtime sprawl
- `docker rm -f $(docker ps -aq)` culture
- Sharing one `.env` with production credentials on a laptop
- Silent `network_mode: host` without justification
- Writer self-certifying stack health without logs

## Handoff / output

Return: project name; ports; health status; logs pointers; teardown performed; nextOwner. Compose-up is not MERGE_READY.

## Non-ownership

Does not replace production packaging (`install_upgrade`), invent cloud orchestration platforms, or authorize destructive shared-host cleanup. Owns compose isolation discipline.
