# Linux Agent native packaging (LNX-PKG-01)

Bounded native `.deb` packaging for the Platform/AmrudAi Linux Agent.

## Scope

- First validation profile: **Ubuntu 24.x amd64 `.deb`**.
- RHEL 9.x / `.rpm` is a follow-on in the same Linux-first campaign; this directory may later gain an rpm recipe, but a `.deb`-only result does **not** complete Linux Agent packaging or BUILD-018/019.
- Artifacts from `build-deb.sh` are **UNSIGNED / INTERNAL_ONLY** until separate signing/release gates pass.
- Package scripts never fabricate enrollment, tenant/agent IDs, certificates, or enable eBPF.

## Layout (Platform_15)

| Path | Role |
|---|---|
| `/opt/platform/agent/platform-agent-linux` | Agent binary (`ExecStart`) |
| `/usr/lib/systemd/system/platform-agent.service` | Governed unit (mirrors `agent/linux/systemd` and `deploy/systemd`) |
| `/etc/platform/agent.env` | Operator-supplied EnvironmentFile (fail-closed if missing) |
| `/etc/platform/agent/` | Credential/cert path directory |
| `/var/lib/platform/agent` | Identity/spool state |
| `/usr/share/doc/platform-agent/` | Docs + env example |
| `/usr/lib/platform/agent/packaging/` | Lifecycle helper + provenance |

`/usr/local` is forbidden for package payload (Debian Policy 9.1.2). An existing unmanaged `/usr/local/bin/platform-agent-linux` causes install to **fail closed**; the package does not delete or migrate it.

## Build

Run from the repository root inside a container/tooling environment that already provides `dpkg-deb` (no new FPM/NFPM dependency):

```bash
mkdir -p /tmp/platform-agent-deb-out
bash agent/packaging/linux/build-deb.sh \
  --binary /path/to/platform-agent-linux \
  --version 0.0.0+internal \
  --source-commit "$(git rev-parse HEAD)" \
  --architecture amd64 \
  --source-date-epoch 0 \
  --binary-sha256 "<sha256 of binary>" \
  --out /tmp/platform-agent-deb-out
```

`--version` is an explicit validated input. It does not rewrite any embedded runtime version string inside the binary. Record runtime version separately when known.

## Install semantics (staged vs activated)

Platform_15 §4.3.1 and Platform_16 §§3–4: staged package install and deliberate activation are separate. Maintainer `configure` **never** treats `/etc/platform/agent.env` or a Sensor URL as activation authority and **never** implicitly `enable`/`start`/`unmask` or `disable`/`re-enable`.

- **STAGED_INACTIVE**: `is-enabled=disabled` (consistent non-zero exit) **and** `is-active` directly verifies `inactive`. Fresh staged install or intentional disablement that is actually inactive. Env may be absent or mode-safe present; package scripts still do not enable.
- **DISABLED_PRESERVED**: `is-enabled=disabled` preserved when ActiveState is not verified inactive (e.g. disabled-but-still-active). No enable/start/restart; does **not** claim inactive.
- **ENABLED_PRESERVED**: `systemctl is-enabled` reports boot `enabled` with exit 0 → preserve; on `configure`/`abort-upgrade`/`abort-remove`, controlled `try-restart` only when also **active** and a mode-safe operational env is present. Enabled+inactive stays inactive (no invented start). Disabled/masked/runtime-only/maintenance/unknown are never restarted.
- **MASKED_PRESERVED** / **RUNTIME_ONLY_PRESERVED**: preserve masked or enabled-runtime/linked/linked-runtime; no enable/unmask/restart. `is-enabled` status/exit pairs must match the systemd contract (disabled/masked/linked* ⇒ non-zero; enabled/enabled-runtime ⇒ 0); inconsistent pairs and unknown words fail closed with `PLATFORM_AGENT_PACKAGE_SERVICE_STATE_QUERY_FAILED` (never a misleading `CONFIGURED_ENABLED`).
- **Authorized activation** (explicit operator/systemd): after placing mode-safe `agent.env` (0600/0400) with Sensor URL and cert path references, run `systemctl enable --now platform-agent.service`. Reboot auto-start applies only after that deliberate enablement.
- **remove**: stops/disables unit; preserves env, credentials, and `/var/lib/platform/agent`.
- **purge**: refuses customer-state deletion; state remains until a separately audited operator procedure.

Env contents are never sourced or executed by package scripts; customer state is never rewritten.

## DEB integrity contract (builder interface)

`build-deb.sh` (owned separately) stages the Agent binary as a regular file and records its SHA256. Callers must supply an **ELF64 little-endian** executable matching the package architecture (`amd64` machine type 62 / `EM_X86_64`; `arm64` machine type 183 / `EM_AARCH64` when that arch is built). Maintainer fixtures may use a non-executed host `/bin/true` (or equivalent) only after verifying that ELF class/endian/machine match; digest is computed from the fixture bytes. This contract is **input integrity for packaging**, not a claim of source-to-binary provenance or release signing. Artifacts remain `UNSIGNED_INTERNAL_ONLY` until separate gates pass.

## Tests

- Content/provenance: `tests/agent-packaging/test-package-content.sh` (separate Codex checkout; not modified here)
- Maintainer-script fixtures (mocked `systemctl`/`getent`/`useradd`/`groupadd`; generated/extracted DEB hooks via private rooted mocks; non-executed `/bin/true` fixture with exact digest; no host package install): `tests/agent-packaging/test-maintainer-scripts.sh`
- Native Ubuntu systemd install/reboot/upgrade/rollback VM evidence: **NOT_RUN** unless an isolated lab VM is explicitly available. Mocked fixtures are not reboot certification.

## Non-goals for this slice

- Host install on the Founder laptop or CI runner
- Release signing keys in source
- Enabling held telemetry paths / flipping Kafka/ClickHouse safety HOLDs
- Claiming whole Linux Agent or BUILD-018 complete

## Candidate continuation

The Issue46 package/lifecycle recipes are reused for the current source. The
optional eBPF profile is shipped as `/usr/share/platform-agent/systemd/20-ebpf.conf`;
it is never enabled automatically. On an independently qualified Ubuntu 24.x or
RHEL 9.x kernel/systemd profile, an authorized administrator can install it under
`/etc/systemd/system/platform-agent.service.d/20-ebpf.conf` and reload/restart the
unit after provisioning the approved eBPF configuration. It grants only CAP_BPF
and CAP_PERFMON to the existing non-root service. Legacy base-Agent installs do
not parse or acquire these optional capabilities. Static packaging does not prove
active eBPF or native lifecycle compatibility.

Downgrade is not a supported automatic recovery action. Preserve config, identity
and queue state; use validated same-schema roll-forward or an approved backup
restore procedure. Package removal/purge preserves customer state.
