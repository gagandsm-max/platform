#!/bin/bash
# Platform Linux Agent package lifecycle helpers (Issue #46 LNX-PKG-01).
# Used by generated maintainer scripts and private fixture tests.
# systemctl interactions under PLATFORM_AGENT_PACKAGE_TOOLS are MOCKED when that
# directory supplies tool shims; label such evidence as mocked.
set -euo pipefail

PLATFORM_AGENT_USER="${PLATFORM_AGENT_USER:-platform-agent}"
PLATFORM_AGENT_GROUP="${PLATFORM_AGENT_GROUP:-platform-agent}"
PLATFORM_AGENT_HOME="${PLATFORM_AGENT_HOME:-/var/lib/platform/agent}"
PLATFORM_AGENT_ENV="${PLATFORM_AGENT_ENV:-/etc/platform/agent.env}"
PLATFORM_AGENT_CRED_DIR="${PLATFORM_AGENT_CRED_DIR:-/etc/platform/agent}"
PLATFORM_AGENT_OPT_DIR="${PLATFORM_AGENT_OPT_DIR:-/opt/platform/agent}"
PLATFORM_AGENT_BINARY="${PLATFORM_AGENT_BINARY:-/opt/platform/agent/platform-agent-linux}"
PLATFORM_AGENT_UNIT="${PLATFORM_AGENT_UNIT:-platform-agent.service}"
PLATFORM_AGENT_UNMANAGED_LEGACY="${PLATFORM_AGENT_UNMANAGED_LEGACY:-/usr/local/bin/platform-agent-linux}"
PLATFORM_AGENT_ROOT="${PLATFORM_AGENT_ROOT:-}"

die() {
  local code="$1"
  shift
  echo "PLATFORM_AGENT_PACKAGE_${code}: $*" >&2
  exit 78
}

rooted() {
  local p="$1"
  if [[ -n "${PLATFORM_AGENT_ROOT}" ]]; then
    printf '%s%s' "${PLATFORM_AGENT_ROOT}" "$p"
  else
    printf '%s' "$p"
  fi
}

tool_path() {
  local name="$1"
  if [[ -n "${PLATFORM_AGENT_PACKAGE_TOOLS:-}" ]]; then
    printf '%s/%s' "${PLATFORM_AGENT_PACKAGE_TOOLS}" "$name"
  else
    case "$name" in
      systemctl) printf '%s' /usr/bin/systemctl ;;
      getent) printf '%s' /usr/bin/getent ;;
      groupadd) printf '%s' /usr/sbin/groupadd ;;
      useradd) printf '%s' /usr/sbin/useradd ;;
      *) die TOOL_DENIED "tool not allowlisted: ${name}" ;;
    esac
  fi
}

tool() {
  local name="$1"
  shift
  local exe
  exe="$(tool_path "$name")"
  if [[ -n "${PLATFORM_AGENT_PACKAGE_TOOLS:-}" ]]; then
    [[ -x "$exe" ]] || die TOOL_MISSING "mocked tool missing: ${name}"
  fi
  "$exe" "$@"
}

is_symlink() {
  [[ -L "$(rooted "$1")" ]]
}

is_regular_file() {
  local p
  p="$(rooted "$1")"
  [[ -f "$p" && ! -L "$p" ]]
}

is_dir() {
  local p
  p="$(rooted "$1")"
  [[ -d "$p" && ! -L "$p" ]]
}

refuse_symlink_path() {
  local p="$1"
  if is_symlink "$p"; then
    die SYMLINK_REFUSED "refusing symlink at $p"
  fi
}

refuse_unmanaged_legacy() {
  local legacy
  legacy="$(rooted "${PLATFORM_AGENT_UNMANAGED_LEGACY}")"
  if [[ -e "$legacy" || -L "$legacy" ]]; then
    die UNMANAGED_USR_LOCAL \
      "unmanaged ${PLATFORM_AGENT_UNMANAGED_LEGACY} present; native package uses ${PLATFORM_AGENT_BINARY} and will not migrate/delete /usr/local installs"
  fi
}

ensure_safe_ancestors() {
  local path="$1"
  local cur="/"
  local IFS='/'
  # shellcheck disable=SC2086
  set -- ${path#/}
  for part in "$@"; do
    cur="${cur%/}/${part}"
    refuse_symlink_path "$cur"
  done
}

ensure_dir() {
  local path="$1"
  local mode="$2"
  local owner="$3"
  ensure_safe_ancestors "$path"
  local target
  target="$(rooted "$path")"
  if [[ -e "$target" || -L "$target" ]]; then
    refuse_symlink_path "$path"
    [[ -d "$target" ]] || die PATH_COLLISION "expected directory at $path"
  else
    mkdir -p "$target"
  fi
  chmod "$mode" "$target"
  if [[ -n "$owner" && -z "${PLATFORM_AGENT_ROOT}" ]]; then
    chown "$owner" "$target"
  fi
}

is_decimal_uint() {
  # Exact unsigned decimal integer; reject empty, signs, and leading-zero ambiguity (except "0").
  [[ "$1" =~ ^[0-9]+$ ]] || return 1
  [[ "$1" == "0" || "$1" =~ ^[1-9][0-9]*$ ]] || return 1
  return 0
}

passwd_field_count() {
  local ent="$1"
  # Literal colon-delimited field count: no word-splitting and no pathname
  # expansion. Empty fields are preserved (N delimiters => N+1 fields).
  local delim="${ent//[^:]/}"
  printf '%s' "$((${#delim} + 1))"
}

lookup_group_ent() {
  # Prints the group entry on success. Returns 2 when absent. Dies on tool/malformed errors.
  local gname="$1"
  local rc=0
  local out
  out="$(tool getent group "$gname" 2>/dev/null)" && rc=0 || rc=$?
  if [[ "$rc" -eq 2 ]]; then
    return 2
  fi
  if [[ "$rc" -ne 0 ]]; then
    die IDENTITY_LOOKUP_FAILED "getent group ${gname} failed rc=${rc}"
  fi
  if [[ -z "$out" ]]; then
    die IDENTITY_LOOKUP_FAILED "getent group ${gname} returned empty success"
  fi
  # Ambiguous multi-line responses are invalid for a single key lookup.
  if [[ "$(printf '%s\n' "$out" | wc -l)" -ne 1 ]]; then
    die IDENTITY_MALFORMED "ambiguous getent group ${gname} response"
  fi
  printf '%s\n' "$out"
  return 0
}

lookup_passwd_ent() {
  local uname="$1"
  local rc=0
  local out
  out="$(tool getent passwd "$uname" 2>/dev/null)" && rc=0 || rc=$?
  if [[ "$rc" -eq 2 ]]; then
    return 2
  fi
  if [[ "$rc" -ne 0 ]]; then
    die IDENTITY_LOOKUP_FAILED "getent passwd ${uname} failed rc=${rc}"
  fi
  if [[ -z "$out" ]]; then
    die IDENTITY_LOOKUP_FAILED "getent passwd ${uname} returned empty success"
  fi
  if [[ "$(printf '%s\n' "$out" | wc -l)" -ne 1 ]]; then
    die IDENTITY_MALFORMED "ambiguous getent passwd ${uname} response"
  fi
  printf '%s\n' "$out"
  return 0
}

validate_group_ent() {
  local ent="$1"
  local name _pw gid _members
  IFS=':' read -r name _pw gid _members <<<"$ent" || die IDENTITY_MALFORMED "unable to parse group record"
  [[ "$(passwd_field_count "$ent")" -eq 4 ]] || die IDENTITY_MALFORMED "group record field count invalid: ${ent}"
  [[ "$name" == "${PLATFORM_AGENT_GROUP}" ]] || die IDENTITY_MALFORMED "group name mismatch in record: ${name}"
  is_decimal_uint "$gid" || die IDENTITY_MALFORMED "group GID not a valid decimal integer: ${gid}"
  if [[ "$gid" == "0" ]]; then
    die IDENTITY_UNSAFE "service-group ${PLATFORM_AGENT_GROUP} has GID 0"
  fi
}

validate_passwd_ent() {
  local ent="$1"
  local name _pw uid gid _gecos home shell
  IFS=':' read -r name _pw uid gid _gecos home shell <<<"$ent" || die IDENTITY_MALFORMED "unable to parse passwd record"
  [[ "$(passwd_field_count "$ent")" -eq 7 ]] || die IDENTITY_MALFORMED "passwd record field count invalid: ${ent}"
  [[ "$name" == "${PLATFORM_AGENT_USER}" ]] || die IDENTITY_MALFORMED "passwd name mismatch in record: ${name}"
  is_decimal_uint "$uid" || die IDENTITY_MALFORMED "UID not a valid decimal integer: ${uid}"
  is_decimal_uint "$gid" || die IDENTITY_MALFORMED "primary GID not a valid decimal integer: ${gid}"
  if [[ "$uid" == "0" ]]; then
    die IDENTITY_UNSAFE "account ${PLATFORM_AGENT_USER} has UID 0"
  fi
  if [[ "$gid" == "0" ]]; then
    die IDENTITY_UNSAFE "account ${PLATFORM_AGENT_USER} has primary GID 0"
  fi
  [[ "$home" == "${PLATFORM_AGENT_HOME}" ]] || die IDENTITY_UNSAFE "existing ${PLATFORM_AGENT_USER} home=$home want ${PLATFORM_AGENT_HOME}"
  case "$shell" in
    /usr/sbin/nologin|/sbin/nologin|/bin/false) ;;
    *) die IDENTITY_UNSAFE "existing ${PLATFORM_AGENT_USER} shell=$shell is interactive/login-capable" ;;
  esac
}

readback_group() {
  local ent rc=0
  ent="$(lookup_group_ent "${PLATFORM_AGENT_GROUP}")" && rc=0 || rc=$?
  if [[ "$rc" -eq 2 ]]; then
    die IDENTITY_READBACK_FAILED "group ${PLATFORM_AGENT_GROUP} missing after create"
  fi
  validate_group_ent "$ent"
}

readback_passwd() {
  local ent rc=0
  ent="$(lookup_passwd_ent "${PLATFORM_AGENT_USER}")" && rc=0 || rc=$?
  if [[ "$rc" -eq 2 ]]; then
    die IDENTITY_READBACK_FAILED "account ${PLATFORM_AGENT_USER} missing after create"
  fi
  validate_passwd_ent "$ent"
}

ensure_identity() {
  local gent pent rc=0

  gent="$(lookup_group_ent "${PLATFORM_AGENT_GROUP}")" && rc=0 || rc=$?
  if [[ "$rc" -eq 2 ]]; then
    tool groupadd --system "${PLATFORM_AGENT_GROUP}"
    readback_group
  else
    validate_group_ent "$gent"
  fi

  rc=0
  pent="$(lookup_passwd_ent "${PLATFORM_AGENT_USER}")" && rc=0 || rc=$?
  if [[ "$rc" -eq 2 ]]; then
    tool useradd --system \
      --gid "${PLATFORM_AGENT_GROUP}" \
      --home-dir "${PLATFORM_AGENT_HOME}" \
      --no-create-home \
      --shell /usr/sbin/nologin \
      "${PLATFORM_AGENT_USER}"
    readback_passwd
  else
    validate_passwd_ent "$pent"
  fi
}

operational_env_present() {
  local envf
  envf="$(rooted "${PLATFORM_AGENT_ENV}")"
  [[ -f "$envf" && ! -L "$envf" ]]
}

ensure_service_manager() {
  local exe
  exe="$(tool_path systemctl)"
  if [[ ! -x "$exe" ]]; then
    die SERVICE_MANAGER_UNSUPPORTED \
      "native-systemd packaging requires an executable systemctl (${exe}); service operations unapplied"
  fi
}

daemon_reload() {
  ensure_service_manager
  tool systemctl daemon-reload || die SERVICE_DAEMON_RELOAD_FAILED "systemctl daemon-reload failed"
}

unit_enable() {
  ensure_service_manager
  tool systemctl enable "${PLATFORM_AGENT_UNIT}" \
    || die SERVICE_ENABLE_FAILED "systemctl enable ${PLATFORM_AGENT_UNIT} failed"
}

unit_disable() {
  ensure_service_manager
  tool systemctl disable "${PLATFORM_AGENT_UNIT}" \
    || die SERVICE_DISABLE_FAILED "systemctl disable ${PLATFORM_AGENT_UNIT} failed"
}

unit_stop() {
  ensure_service_manager
  tool systemctl stop "${PLATFORM_AGENT_UNIT}" \
    || die SERVICE_STOP_FAILED "systemctl stop ${PLATFORM_AGENT_UNIT} failed"
}

unit_active_state() {
  # Establish unit ActiveState explicitly. Lookup/tool failures are not "inactive".
  ensure_service_manager
  local rc=0
  local out
  out="$(tool systemctl is-active "${PLATFORM_AGENT_UNIT}" 2>/dev/null)" && rc=0 || rc=$?
  out="$(printf '%s' "$out" | tr -d '\r' | head -n1)"
  case "$rc" in
    0)
      [[ "$out" == "active" ]] || die SERVICE_STATE_QUERY_FAILED \
        "systemctl is-active ${PLATFORM_AGENT_UNIT} rc=0 but state='${out}'"
      printf '%s\n' "active"
      ;;
    3)
      case "$out" in
        inactive|failed|activating|deactivating|maintenance|reloading)
          printf '%s\n' "$out"
          ;;
        "")
          die SERVICE_STATE_QUERY_FAILED \
            "systemctl is-active ${PLATFORM_AGENT_UNIT} rc=3 with empty state"
          ;;
        *)
          die SERVICE_STATE_QUERY_FAILED \
            "systemctl is-active ${PLATFORM_AGENT_UNIT} rc=3 with unexpected state='${out}'"
          ;;
      esac
      ;;
    *)
      die SERVICE_STATE_QUERY_FAILED \
        "systemctl is-active ${PLATFORM_AGENT_UNIT} failed rc=${rc} state='${out}'"
      ;;
  esac
}

unit_enable_state() {
  # systemd is-enabled is the sole package activation authority. Status word AND
  # exit code must both match the supported systemd contract; inconsistent pairs
  # fail closed. Exit code alone is insufficient (enabled-runtime also returns 0).
  # Contract (systemctl is-enabled): enabled/enabled-runtime => 0;
  # disabled/masked/masked-runtime/linked/linked-runtime => non-zero.
  ensure_service_manager
  local rc=0
  local out
  out="$(tool systemctl is-enabled "${PLATFORM_AGENT_UNIT}" 2>/dev/null)" && rc=0 || rc=$?
  out="$(printf '%s' "$out" | tr -d '\r' | head -n1)"
  case "$out" in
    enabled)
      [[ "$rc" -eq 0 ]] || die SERVICE_STATE_QUERY_FAILED \
        "systemctl is-enabled ${PLATFORM_AGENT_UNIT} state=enabled but rc=${rc}"
      printf '%s\n' "enabled"
      ;;
    disabled)
      [[ "$rc" -ne 0 ]] || die SERVICE_STATE_QUERY_FAILED \
        "systemctl is-enabled ${PLATFORM_AGENT_UNIT} state=disabled but rc=0 (inconsistent)"
      printf '%s\n' "disabled"
      ;;
    masked|masked-runtime)
      [[ "$rc" -ne 0 ]] || die SERVICE_STATE_QUERY_FAILED \
        "systemctl is-enabled ${PLATFORM_AGENT_UNIT} state=${out} but rc=0 (inconsistent)"
      printf '%s\n' "masked"
      ;;
    enabled-runtime)
      [[ "$rc" -eq 0 ]] || die SERVICE_STATE_QUERY_FAILED \
        "systemctl is-enabled ${PLATFORM_AGENT_UNIT} state=enabled-runtime but rc=${rc}"
      # Runtime-only enablement is not boot activation authority.
      printf '%s\n' "runtime-only"
      ;;
    linked|linked-runtime)
      [[ "$rc" -ne 0 ]] || die SERVICE_STATE_QUERY_FAILED \
        "systemctl is-enabled ${PLATFORM_AGENT_UNIT} state=${out} but rc=0 (inconsistent)"
      # Linked availability is not boot activation authority.
      printf '%s\n' "runtime-only"
      ;;
    "")
      die SERVICE_STATE_QUERY_FAILED \
        "systemctl is-enabled ${PLATFORM_AGENT_UNIT} rc=${rc} with empty state"
      ;;
    *)
      die SERVICE_STATE_QUERY_FAILED \
        "systemctl is-enabled ${PLATFORM_AGENT_UNIT} unexpected state='${out}' rc=${rc}"
      ;;
  esac
}

unit_try_restart_if_safe() {
  # Controlled try-restart only for verified boot-enabled + active units with a
  # mode-safe operational env already validated by the caller. Never start,
  # never restart disabled-active / masked / maintenance / unknown.
  operational_env_present || return 0
  local state
  state="$(unit_active_state)"
  case "$state" in
    active)
      tool systemctl try-restart "${PLATFORM_AGENT_UNIT}" \
        || die SERVICE_RESTART_FAILED "systemctl try-restart ${PLATFORM_AGENT_UNIT} failed"
      ;;
    inactive|failed|activating|deactivating|maintenance|reloading)
      # Preserve non-steady or inactive runtime; do not invent a start.
      ;;
    *)
      die SERVICE_STATE_QUERY_FAILED \
        "refusing try-restart for unexpected ActiveState='${state}'"
      ;;
  esac
}

emit_activation_hint() {
  echo "PLATFORM_AGENT_PACKAGE_HINT: staged/package configure never enables; authorized activation is explicit: systemctl enable --now ${PLATFORM_AGENT_UNIT} (after ${PLATFORM_AGENT_ENV} mode 0600/0400 with Sensor URL and cert path references). Env/URL alone is not activation authority."
}

preinst() {
  local action="${1:-}"
  refuse_unmanaged_legacy
  ensure_safe_ancestors "${PLATFORM_AGENT_OPT_DIR}"
  ensure_safe_ancestors "${PLATFORM_AGENT_HOME}"
  ensure_safe_ancestors "${PLATFORM_AGENT_CRED_DIR}"
  if is_symlink "${PLATFORM_AGENT_OPT_DIR}" || is_symlink "${PLATFORM_AGENT_BINARY}"; then
    die SYMLINK_REFUSED "refusing packaged path that is a symlink"
  fi
  if [[ -e "$(rooted "${PLATFORM_AGENT_BINARY}")" && ! -f "$(rooted "${PLATFORM_AGENT_BINARY}")" ]]; then
    die PATH_COLLISION "binary path exists and is not a regular file"
  fi
  # Upgrade path: existing ownership of state must remain safe.
  if [[ "$action" == "upgrade" || "$action" == "abort-upgrade" ]]; then
    if is_dir "${PLATFORM_AGENT_HOME}"; then
      refuse_symlink_path "${PLATFORM_AGENT_HOME}"
    fi
  fi
  echo "PLATFORM_AGENT_PACKAGE_PREINST_OK action=${action:-install}"
}

postinst() {
  local action="${1:-}"
  ensure_identity
  ensure_dir /opt/platform 0755 ""
  ensure_dir "${PLATFORM_AGENT_OPT_DIR}" 0755 "root:root"
  ensure_dir /etc/platform 0755 "root:root"
  ensure_dir "${PLATFORM_AGENT_CRED_DIR}" 0750 "${PLATFORM_AGENT_USER}:${PLATFORM_AGENT_GROUP}"
  ensure_dir "${PLATFORM_AGENT_HOME}" 0750 "${PLATFORM_AGENT_USER}:${PLATFORM_AGENT_GROUP}"
  # Never fabricate agent.env, certificates, tenant/agent IDs, or enable eBPF.
  # Never source/execute env contents; mode-safe presence is configuration safety only,
  # never activation authority (Platform_15 §4.3.1 / Platform_16 §§3–4).
  if [[ -f "$(rooted "${PLATFORM_AGENT_ENV}")" ]]; then
    refuse_symlink_path "${PLATFORM_AGENT_ENV}"
    local mode
    mode="$(stat -c '%a' "$(rooted "${PLATFORM_AGENT_ENV}")" 2>/dev/null || stat -f '%OLp' "$(rooted "${PLATFORM_AGENT_ENV}")")"
    case "$mode" in
      600|400) ;;
      *) die ENV_PERMS "refusing ${PLATFORM_AGENT_ENV} mode=${mode}; require 0600 or 0400" ;;
    esac
  fi
  daemon_reload
  # configure/upgrade/abort must not enable/start/unmask or disable/re-enable.
  # Preserve intentional maintenance/disablement, masked, runtime-only, and
  # existing enabled inactive/active units. systemd is-enabled is authoritative.
  local enable_state
  enable_state="$(unit_enable_state)"
  case "$enable_state" in
    enabled)
      case "$action" in
        configure|abort-upgrade|abort-remove)
          unit_try_restart_if_safe
          ;;
      esac
      echo "PLATFORM_AGENT_PACKAGE_STATE=ENABLED_PRESERVED"
      ;;
    disabled)
      # Preserve intentional disablement: never enable/start/restart.
      # Claim STAGED_INACTIVE only when ActiveState is directly verified inactive;
      # otherwise report DISABLED_PRESERVED (e.g. disabled-but-still-active).
      local active_state
      active_state="$(unit_active_state)"
      case "$active_state" in
        inactive)
          echo "PLATFORM_AGENT_PACKAGE_STATE=STAGED_INACTIVE"
          ;;
        *)
          echo "PLATFORM_AGENT_PACKAGE_STATE=DISABLED_PRESERVED"
          ;;
      esac
      emit_activation_hint
      ;;
    masked)
      echo "PLATFORM_AGENT_PACKAGE_STATE=MASKED_PRESERVED"
      ;;
    runtime-only)
      echo "PLATFORM_AGENT_PACKAGE_STATE=RUNTIME_ONLY_PRESERVED"
      emit_activation_hint
      ;;
    *)
      die SERVICE_STATE_QUERY_FAILED "unhandled enable state '${enable_state}'"
      ;;
  esac
  echo "PLATFORM_AGENT_PACKAGE_POSTINST_OK action=${action:-configure} eBPF=not_enabled_by_package signing=UNSIGNED_INTERNAL_ONLY"
}

prerm() {
  local action="${1:-}"
  case "$action" in
    remove)
      unit_stop
      unit_disable
      ;;
    upgrade|deconfigure|failed-upgrade)
      # Keep unit enabled across upgrade; stop briefly only when needed by dpkg replace.
      ;;
  esac
  echo "PLATFORM_AGENT_PACKAGE_PRERM_OK action=${action:-}"
}

postrm() {
  local action="${1:-}"
  case "$action" in
    remove|abort-install|abort-upgrade)
      # Best-effort reload after unit file removal; never claim service readiness here.
      if [[ -x "$(tool_path systemctl)" ]]; then
        tool systemctl daemon-reload || true
      fi
      echo "PLATFORM_AGENT_PACKAGE_STATE_PRESERVED paths=${PLATFORM_AGENT_ENV},${PLATFORM_AGENT_CRED_DIR},${PLATFORM_AGENT_HOME}"
      ;;
    purge)
      # Keep on stdout so operators/fixtures observe the refuse marker in logs.
      echo "PLATFORM_AGENT_PACKAGE_PURGE_REFUSED: customer state/config/identity/spool are not deleted by package purge; use a separately audited operator procedure"
      if [[ -x "$(tool_path systemctl)" ]]; then
        tool systemctl daemon-reload || true
      fi
      # Exit 0 so dpkg can finish removing package metadata; state remains.
      ;;
  esac
  echo "PLATFORM_AGENT_PACKAGE_POSTRM_OK action=${action:-}"
}

usage() {
  cat <<'EOF'
Usage: platform-agent-lifecycle.sh <preinst|postinst|prerm|postrm> [dpkg-args...]
Private fixture root: set PLATFORM_AGENT_ROOT and PLATFORM_AGENT_PACKAGE_TOOLS.
EOF
}

main() {
  local phase="${1:-}"
  shift || true
  case "$phase" in
    preinst) preinst "${1:-}" ;;
    postinst) postinst "${1:-}" ;;
    prerm) prerm "${1:-}" ;;
    postrm) postrm "${1:-}" ;;
    -h|--help|help) usage ;;
    *) die USAGE "unknown phase: ${phase:-}" ;;
  esac
}

main "$@"
