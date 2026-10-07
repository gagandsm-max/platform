---
name: install_upgrade
description: deb/rpm/MSI packaging, systemd lifecycle, upgrade/rollback and reboot auto-start evidence.
---

<!-- generated from project_agents/skills/install_upgrade/SKILL.md sha256:9ece5dbcef8a37370c9114e682e5f973148dd535485f7bd3146b899bd81adf12 -->

# install_upgrade

## Purpose

Prove Core/Sensor/Agent install and upgrade behaviour—including skew, migration, rollback constraints and post-upgrade health—without unsupported destructive rollback stories.

## Use when

Packaging (deb/rpm/MSI), installers, systemd units, version skew between Core/Sensor/Agent, upgrade runbooks, rollback/roll-forward decisions, or claims about reboot auto-start / supervised recovery.

## Authoritative inputs

Always read:

1. Packaging/layout docs and existing install scripts for the component
2. Mother constraints: native packages, auto-start after reboot, supervised recovery
3. Compatibility/skew window for the release (Core↔Sensor↔Agent)
4. Migration catalogues affecting the upgrade path (`migration_safety`)
5. Backup prerequisites and secret/config preservation requirements
6. Signature/provenance expectations for packages
7. `references/packaging-reboot-rollback.md` and fixtures/evals for deb/rpm/MSI, spool/identity preservation and crash-loop evidence

## Invariants

- Cover **install**, **upgrade**, and stated **compatibility window**.
- Forward migration is expected; do not invent silent destructive downgrades.
- Sensor/Core/Agent version skew must be explicit—what is supported, what fails closed.
- Rollback vs roll-forward: only support paths that preserve data/config or documented restore-from-backup.
- Config and secrets preservation across upgrade is mandatory unless a breaking change is explicitly versioned.
- Backup prerequisites before upgrade when control-state/telemetry stores are touched.
- Failure halfway must leave a diagnosable state (not silent corruption).
- Restart/reboot auto-start and supervised recovery remain mandatory.
- Downgrade constraints must be documented; unsupported downgrade is refused.
- Package signature/provenance must be verifiable for release claims.
- Operational health after upgrade must be evidenced (health endpoints/units).
- No unsupported destructive rollback (“just delete the DB”).

## Required procedure

1. State component(s), from→to versions, and supported skew matrix.
2. Declare backup/snapshot prerequisites and secret/config paths preserved.
3. Exercise clean install on a representative OS/package format when in scope.
4. Exercise upgrade from supported N-1 (or stated window); record migration outcomes.
5. Inject/observe mid-upgrade failure where feasible; document recovery (roll-forward/restore).
6. Verify systemd/MSI service enablement, reboot auto-start and crash-loop diagnostics visibility.
7. Verify package signatures/hashes when making release claims.
8. Run post-upgrade health checks (Core/Sensor/Agent as applicable) and skew negotiation behaviour.
9. Document unsupported downgrade explicitly; do not test fiction as supported.
10. Capture evidence on exact SHA/artifacts under test.

## Negative / failure cases

- Upgrading without backup when PG migrations run.
- Leaving credentials world-readable after package unpack.
- Claiming rollback while only uninstall+data-loss works.
- Mixing unsupported Agent version against Core and calling it “fine”.
- Skipping reboot auto-start verification.
- Shipping unsigned artifacts as GA.

## Security / tenant implications

Packages must not embed live secrets. Upgrade must preserve mTLS material safely (`pki_mtls`, `secrets_handling`). Multi-tenant Core upgrades must not drop tenant isolation. Windows MSI/BAT paths follow least privilege. Compromise of update channel is a security incident path—do not invent ad-hoc HTTP unsigned upgrade.

## Required evidence

- Version matrix and package artifact IDs/hashes
- Install/upgrade command transcripts
- Migration result or N/A
- Service enablement + reboot/auto-start proof
- Mid-failure recovery notes or `UNVERIFIED`
- Post-upgrade health evidence
- Signature/provenance proof for release claims

## Stop conditions

Stop release/cert claims when:

- Compatibility skew is undefined
- Backup prerequisite cannot be met for stateful upgrade
- Mid-upgrade failure leaves undefined corruption
- Auto-start/supervision regresses
- Rollback story requires silent data destruction

## Anti-patterns

- “Uninstall and reinstall” as the only upgrade path for stateful Core
- Testing only happy-path install on one distro then claiming GA
- Ignoring Agent skew
- Writer self-certifying upgrade safety without runtime evidence

## Handoff / output

Return: matrix; transcripts; health results; residual risks; nextOwner (`platform-sre` / verifier). Does not self-certify MERGE_READY.

## Non-ownership

Does not invent BUILD-120 HA package topology, authorize production change windows, or replace `migration_safety` / `backup_restore`. Owns install/upgrade evidence discipline.
