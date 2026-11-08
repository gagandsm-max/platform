# Linux Agent RPM adapter

This reuses the Issue46 adapter and shared lifecycle helper. The current candidate
accepts an explicit immutable payload commit, recipe commit and supplied binary
SHA-256 instead of the historical source16/binary pins. It verifies every payload
and recipe byte against those commits and refuses uncommitted drift. Binary
compilation provenance must be supplied separately; a caller-provided hash alone
is not source-to-binary proof. Artifacts remain `UNSIGNED_INTERNAL_ONLY`.

Target: RHEL 9.x x86_64 under the declared native qualification profile. Package
assembly or source fixtures do not establish native RPM/systemd compatibility.
No compiler, dependency download, package installation, signing or environment
change runs in this builder. Use an admitted existing immutable RPM-capable image.

## Build

Run `build-rpm.sh --help` for required arguments. Full mode requires the current
`--source-commit`, `--recipe-commit`, `--binary`, `--binary-sha256`,
`--builder-image sha256:...`, explicit `--version`, `--release`, and an existing
private `--out` outside the source/binary directories. Both commit identities are
verified through Git. Use `--source-only --fixture-inputs` only for synthetic
fixtures; that combination cannot build an RPM. Missing RPM tools fail closed.

## Lifecycle

The existing helper is embedded unchanged in each RPM scriptlet. Installation
counts map `%pre` to preinst install/upgrade, `%post` to postinst configure,
`%preun` to prerm remove/upgrade, and `%postun` to postrm remove/upgrade. Malformed
counts fail with `RPM_COUNT_INVALID`. Old-payload removal during upgrade cannot
stop the new service. Record scriptlet exits, package-manager transaction exits,
rpmdb, payload and service state separately; a failed scriptlet may leave payload
installed. No automatic downgrade/rollback is claimed.

No `/etc` or `/var` customer-state payload is owned by RPM. Missing configuration
leaves `STAGED_UNCONFIGURED`. Configured installs enable the service and restart
only an already-active service. Removal retains configuration, credentials,
identity, queue and service account. eBPF defaults off; the optional narrow-cap
drop-in ships inactive under `/usr/share/platform-agent/systemd/20-ebpf.conf`.

## Evidence

`tests/agent-packaging/rpm/compose.yaml` requires fresh Issue211 admission and an
explicit immutable existing image. `--source-only` checks generated source and
mocked lifecycle behavior. `--full` requires a real RPM receipt and verifies
extracted scriptlets before those mock tests. Neither proves native installation.
Native install/start/reinstall/upgrade/erase/reboot, SELinux and non-root active
eBPF remain separate exact-binary gates. No certification follows from this file.
