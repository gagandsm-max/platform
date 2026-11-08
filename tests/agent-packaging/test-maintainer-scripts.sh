#!/usr/bin/env bash
# Maintainer-script lifecycle fixtures for LNX-PKG-01.
# systemctl/getent/useradd/groupadd are MOCKED — not native VM evidence.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

need() { command -v "$1" >/dev/null 2>&1 || { echo "MISSING_TOOL: $1" >&2; exit 78; }; }
need dpkg-deb
need python3

WORKDIR="$(mktemp -d /tmp/platform-agent-pkg-lifecycle.XXXXXX)"
cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT

echo "NOTE: maintainer script evidence below uses MOCKED systemctl/account tools"

BIN="${WORKDIR}/platform-agent-linux"
printf 'SYNTHETIC_PLATFORM_AGENT_PAYLOAD_NOT_EXECUTED\n' > "$BIN"
chmod 0755 "$BIN"
BIN_SHA="$(sha256sum "$BIN" | awk '{print $1}')"
COMMIT="$(git rev-parse HEAD)"
VERSION="0.0.0+internal.lifecycle"
OUT="${WORKDIR}/out"
mkdir -m 0700 "$OUT"

bash agent/packaging/linux/build-deb.sh \
  --binary "$BIN" \
  --version "$VERSION" \
  --source-commit "$COMMIT" \
  --architecture amd64 \
  --source-date-epoch 0 \
  --binary-sha256 "$BIN_SHA" \
  --out "$OUT" >/dev/null

STAGE="${OUT}/platform-agent-${VERSION}-amd64-deb/payload"
test -d "$STAGE/DEBIAN"

install_mocks() {
  local tools="$1"
  mkdir -p "$tools"
  for n in systemctl getent groupadd useradd; do
    cp tests/agent-packaging/fixtures/mock-tools.sh "${tools}/${n}"
    chmod 0755 "${tools}/${n}"
  done
}

new_root() {
  local name="$1"
  local root="${WORKDIR}/${name}"
  mkdir -p "$root"
  printf 'mock-boot\n' > "${WORKDIR}/${name}.boot"
  # lifecycle reads /proc/... only when rooted paths are used; boot_id not required for agent scripts.
  echo "$root"
}

write_state() {
  local root="$1"
  local json="$2"
  mkdir -p "${root}/mock-state"
  printf '%s\n' "$json" > "${root}/mock-state/boundary-state.json"
}

run_hook() {
  local stage_root="$1"
  local phase="$2"
  shift 2
  env PLATFORM_AGENT_ROOT="$stage_root" \
      PLATFORM_AGENT_PACKAGE_TOOLS="${stage_root}/private-tools" \
      PLATFORM_AGENT_MOCK_STATE="${stage_root}/mock-state" \
      /bin/bash "${STAGE}/DEBIAN/${phase}" "$@"
}

expect_fail() {
  local stage_root="$1"
  local phase="$2"
  shift 2
  if env PLATFORM_AGENT_ROOT="$stage_root" \
      PLATFORM_AGENT_PACKAGE_TOOLS="${stage_root}/private-tools" \
      PLATFORM_AGENT_MOCK_STATE="${stage_root}/mock-state" \
      /bin/bash "${STAGE}/DEBIAN/${phase}" "$@" >/dev/null 2>&1; then
    echo "FAIL: expected ${phase} to fail" >&2
    exit 1
  fi
}

expect_fail_diag() {
  # expect_fail_diag <root> <phase> <diag-substring> [args...]
  local stage_root="$1"
  local phase="$2"
  local diag="$3"
  shift 3
  local out rc=0
  set +e
  out="$(env PLATFORM_AGENT_ROOT="$stage_root" \
      PLATFORM_AGENT_PACKAGE_TOOLS="${stage_root}/private-tools" \
      PLATFORM_AGENT_MOCK_STATE="${stage_root}/mock-state" \
      /bin/bash "${STAGE}/DEBIAN/${phase}" "$@" 2>&1)"
  rc=$?
  set -e
  if [[ "$rc" -eq 0 ]]; then
    echo "FAIL: expected ${phase} to fail for ${diag}" >&2
    printf '%s\n' "$out" >&2
    exit 1
  fi
  printf '%s\n' "$out" | grep -q "$diag" || {
    echo "FAIL: expected diagnostic containing ${diag}; rc=${rc}" >&2
    printf '%s\n' "$out" >&2
    exit 1
  }
  if printf '%s\n' "$out" | grep -q 'PLATFORM_AGENT_PACKAGE_POSTINST_OK\|PLATFORM_AGENT_PACKAGE_PRERM_OK'; then
    echo "FAIL: success marker present despite failure for ${diag}" >&2
    printf '%s\n' "$out" >&2
    exit 1
  fi
  printf '%s\n' "$out"
}

seed_configured_env() {
  local root="$1"
  mkdir -p "${root}/etc/platform"
  printf 'PLATFORM_AGENT_SENSOR_URL=https://sensor.example.invalid:8443\n' > "${root}/etc/platform/agent.env"
  chmod 0600 "${root}/etc/platform/agent.env"
}

# --- Case 1: clean staged install (no agent.env) ---
R1="$(new_root root-staged)"
install_mocks "${R1}/private-tools"
mkdir -p "${R1}/mock-state"
run_hook "$R1" preinst install | tee "${WORKDIR}/c1-preinst.txt"
run_hook "$R1" postinst configure | tee "${WORKDIR}/c1-postinst.txt"
grep -q 'PLATFORM_AGENT_PACKAGE_STATE=STAGED_UNCONFIGURED' "${WORKDIR}/c1-postinst.txt"
python3 - <<PY
import json
st=json.load(open("${R1}/mock-state/boundary-state.json"))
assert "platform-agent" in st["users"]
assert "platform-agent" in st["groups"]
uid=st["users"]["platform-agent"].split(":")[2]
gid=st["groups"]["platform-agent"].split(":")[2]
assert uid != "0" and gid != "0", st
unit=st["units"].get("platform-agent.service", {})
assert unit.get("enabled") != "enabled", unit
print("CASE1_STAGED_OK mocked")
PY

# --- Case 2: configured enable when env present with mode 0600 ---
R2="$(new_root root-configured)"
install_mocks "${R2}/private-tools"
mkdir -p "${R2}/mock-state" "${R2}/etc/platform"
printf 'PLATFORM_AGENT_SENSOR_URL=https://sensor.example.invalid:8443\n' > "${R2}/etc/platform/agent.env"
chmod 0600 "${R2}/etc/platform/agent.env"
run_hook "$R2" preinst install >/dev/null
run_hook "$R2" postinst configure | tee "${WORKDIR}/c2-postinst.txt"
grep -q 'PLATFORM_AGENT_PACKAGE_STATE=CONFIGURED_ENABLED' "${WORKDIR}/c2-postinst.txt"
python3 - <<PY
import json
st=json.load(open("${R2}/mock-state/boundary-state.json"))
assert st["units"]["platform-agent.service"]["enabled"]=="enabled"
# Fresh configured install enables without starting.
assert st["units"]["platform-agent.service"].get("active","inactive")!="active"
calls=st["calls"]
assert ["systemctl","enable","platform-agent.service"] in calls
assert not any(c[:2]==["systemctl","start"] for c in calls), calls
assert not any(c[:2]==["systemctl","try-restart"] for c in calls), calls
print("CASE2_CONFIGURED_OK mocked")
PY

# --- Case 3: refuse unmanaged /usr/local binary ---
R3="$(new_root root-usr-local)"
install_mocks "${R3}/private-tools"
mkdir -p "${R3}/usr/local/bin" "${R3}/mock-state"
printf 'legacy\n' > "${R3}/usr/local/bin/platform-agent-linux"
expect_fail "$R3" preinst install
echo "CASE3_USR_LOCAL_REFUSED_OK"

# --- Case 4: refuse symlink at opt binary path ---
R4="$(new_root root-symlink)"
install_mocks "${R4}/private-tools"
mkdir -p "${R4}/opt/platform/agent" "${R4}/mock-state" "${R4}/tmp"
printf 'x\n' > "${R4}/tmp/target"
ln -s /tmp/target "${R4}/opt/platform/agent/platform-agent-linux"
expect_fail "$R4" preinst install
echo "CASE4_SYMLINK_REFUSED_OK"

# --- Case 5: pre-existing safe account is accepted ---
R5="$(new_root root-preexisting)"
install_mocks "${R5}/private-tools"
write_state "$R5" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32100:"},"users":{"platform-agent":"platform-agent:x:32100:32100::/var/lib/platform/agent:/usr/sbin/nologin"},"units":{},"fail":null}'
run_hook "$R5" preinst install >/dev/null
run_hook "$R5" postinst configure >/dev/null
python3 - <<PY
import json
st=json.load(open("${R5}/mock-state/boundary-state.json"))
assert not any(c[0]=="useradd" for c in st["calls"]), st["calls"]
assert not any(c[0]=="groupadd" for c in st["calls"]), st["calls"]
print("CASE5_PREEXISTING_ACCOUNT_OK")
PY

# --- Case 6: unsafe interactive shell account refused ---
R6="$(new_root root-bad-shell)"
install_mocks "${R6}/private-tools"
write_state "$R6" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32101:"},"users":{"platform-agent":"platform-agent:x:32101:32101::/var/lib/platform/agent:/bin/bash"},"units":{},"fail":null}'
expect_fail_diag "$R6" postinst 'PLATFORM_AGENT_PACKAGE_IDENTITY_UNSAFE' configure >/dev/null
echo "CASE6_BAD_SHELL_REFUSED_OK"

# --- Case 7: remove preserves state paths and disables unit (mocked) ---
R7="$(new_root root-remove)"
install_mocks "${R7}/private-tools"
mkdir -p "${R7}/mock-state" "${R7}/etc/platform/agent" "${R7}/var/lib/platform/agent"
printf 'PLATFORM_AGENT_ID=keep-me\n' > "${R7}/etc/platform/agent.env"
chmod 0600 "${R7}/etc/platform/agent.env"
printf 'spool\n' > "${R7}/var/lib/platform/agent/spool.marker"
printf 'cert\n' > "${R7}/etc/platform/agent/agent.crt"
run_hook "$R7" preinst install >/dev/null
run_hook "$R7" postinst configure >/dev/null
run_hook "$R7" prerm remove | tee "${WORKDIR}/c7-prerm.txt"
grep -q 'PLATFORM_AGENT_PACKAGE_PRERM_OK' "${WORKDIR}/c7-prerm.txt"
run_hook "$R7" postrm remove | tee "${WORKDIR}/c7-postrm.txt"
grep -q 'PLATFORM_AGENT_PACKAGE_STATE_PRESERVED' "${WORKDIR}/c7-postrm.txt"
test -f "${R7}/etc/platform/agent.env"
test -f "${R7}/var/lib/platform/agent/spool.marker"
test -f "${R7}/etc/platform/agent/agent.crt"
python3 - <<PY
import json
st=json.load(open("${R7}/mock-state/boundary-state.json"))
assert st["units"]["platform-agent.service"]["enabled"]=="disabled"
assert st["units"]["platform-agent.service"]["active"]=="inactive"
print("CASE7_REMOVE_PRESERVE_OK mocked")
PY

# --- Case 8: purge refuses customer-state deletion ---
run_hook "$R7" postrm purge | tee "${WORKDIR}/c8-purge.txt"
grep -q 'PLATFORM_AGENT_PACKAGE_PURGE_REFUSED' "${WORKDIR}/c8-purge.txt"
test -f "${R7}/etc/platform/agent.env"
test -f "${R7}/var/lib/platform/agent/spool.marker"
echo "CASE8_PURGE_REFUSE_OK"

# --- Case 9: open env mode refused ---
R9="$(new_root root-open-env)"
install_mocks "${R9}/private-tools"
mkdir -p "${R9}/mock-state" "${R9}/etc/platform"
printf 'PLATFORM_AGENT_SENSOR_URL=https://sensor.example.invalid:8443\n' > "${R9}/etc/platform/agent.env"
chmod 0644 "${R9}/etc/platform/agent.env"
run_hook "$R9" preinst install >/dev/null
expect_fail "$R9" postinst configure
echo "CASE9_OPEN_ENV_REFUSED_OK"

# --- Case 10: repeated configure idempotent (mocked) ---
R10="$(new_root root-idempotent)"
install_mocks "${R10}/private-tools"
mkdir -p "${R10}/mock-state" "${R10}/etc/platform"
printf 'PLATFORM_AGENT_SENSOR_URL=https://sensor.example.invalid:8443\n' > "${R10}/etc/platform/agent.env"
chmod 0600 "${R10}/etc/platform/agent.env"
run_hook "$R10" preinst install >/dev/null
run_hook "$R10" postinst configure >/dev/null
run_hook "$R10" postinst configure >/dev/null
run_hook "$R10" postinst configure >/dev/null
echo "CASE10_IDEMPOTENT_OK mocked"

# --- Case 11: upgrade prerm does not disable ---
R11="$(new_root root-upgrade)"
install_mocks "${R11}/private-tools"
mkdir -p "${R11}/mock-state" "${R11}/etc/platform"
printf 'PLATFORM_AGENT_SENSOR_URL=https://sensor.example.invalid:8443\n' > "${R11}/etc/platform/agent.env"
chmod 0600 "${R11}/etc/platform/agent.env"
run_hook "$R11" preinst install >/dev/null
run_hook "$R11" postinst configure >/dev/null
run_hook "$R11" prerm upgrade 0.0.0+internal.lifecycle >/dev/null
python3 - <<PY
import json
st=json.load(open("${R11}/mock-state/boundary-state.json"))
assert st["units"]["platform-agent.service"]["enabled"]=="enabled"
print("CASE11_UPGRADE_PRERM_KEEPS_ENABLE_OK mocked")
PY

# --- Case 12: failed-upgrade / abort-upgrade keeps identity and does not purge state ---
R12="$(new_root root-failed-upgrade)"
install_mocks "${R12}/private-tools"
mkdir -p "${R12}/mock-state" "${R12}/etc/platform" "${R12}/var/lib/platform/agent"
printf 'PLATFORM_AGENT_SENSOR_URL=https://sensor.example.invalid:8443\n' > "${R12}/etc/platform/agent.env"
chmod 0600 "${R12}/etc/platform/agent.env"
printf 'pending-spool\n' > "${R12}/var/lib/platform/agent/spool.marker"
run_hook "$R12" preinst install >/dev/null
run_hook "$R12" postinst configure >/dev/null
run_hook "$R12" prerm upgrade "${VERSION}" >/dev/null
run_hook "$R12" preinst abort-upgrade "${VERSION}" >/dev/null
run_hook "$R12" postinst abort-upgrade "${VERSION}" >/dev/null
test -f "${R12}/etc/platform/agent.env"
test -f "${R12}/var/lib/platform/agent/spool.marker"
python3 - <<PY
import json
st=json.load(open("${R12}/mock-state/boundary-state.json"))
assert st["units"]["platform-agent.service"]["enabled"]=="enabled"
print("CASE12_FAILED_UPGRADE_PRESERVE_OK mocked")
PY

# --- Case 13: rollback-shaped remove after upgrade attempt preserves spool/env ---
run_hook "$R12" prerm remove >/dev/null
run_hook "$R12" postrm remove >/dev/null
test -f "${R12}/etc/platform/agent.env"
test -f "${R12}/var/lib/platform/agent/spool.marker"
echo "CASE13_ROLLBACK_SHAPED_REMOVE_PRESERVE_OK mocked"

# --- Case 14: refuse UID 0 ---
R14="$(new_root root-uid0)"
install_mocks "${R14}/private-tools"
write_state "$R14" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32114:"},"users":{"platform-agent":"platform-agent:x:0:32114::/var/lib/platform/agent:/usr/sbin/nologin"},"units":{},"fail":null}'
expect_fail_diag "$R14" postinst 'PLATFORM_AGENT_PACKAGE_IDENTITY_UNSAFE' configure >/dev/null
python3 - <<PY
import json
st=json.load(open("${R14}/mock-state/boundary-state.json"))
assert not any(c[0] in ("useradd","groupadd") for c in st["calls"]), st["calls"]
print("CASE14_UID0_REFUSED_OK mocked")
PY

# --- Case 15: refuse service-group GID 0 ---
R15="$(new_root root-sgid0)"
install_mocks "${R15}/private-tools"
write_state "$R15" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:0:"},"users":{},"units":{},"fail":null}'
expect_fail_diag "$R15" postinst 'PLATFORM_AGENT_PACKAGE_IDENTITY_UNSAFE' configure >/dev/null
echo "CASE15_SERVICE_GROUP_GID0_REFUSED_OK mocked"

# --- Case 16: refuse account primary GID 0 ---
R16="$(new_root root-pgid0)"
install_mocks "${R16}/private-tools"
write_state "$R16" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32116:"},"users":{"platform-agent":"platform-agent:x:32116:0::/var/lib/platform/agent:/usr/sbin/nologin"},"units":{},"fail":null}'
expect_fail_diag "$R16" postinst 'PLATFORM_AGENT_PACKAGE_IDENTITY_UNSAFE' configure >/dev/null
echo "CASE16_PRIMARY_GID0_REFUSED_OK mocked"

# --- Case 17: malformed identity record ---
R17="$(new_root root-malformed)"
install_mocks "${R17}/private-tools"
write_state "$R17" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32117:"},"users":{"platform-agent":"platform-agent:x:not-a-uid:32117::/var/lib/platform/agent:/usr/sbin/nologin"},"units":{},"fail":null}'
expect_fail_diag "$R17" postinst 'PLATFORM_AGENT_PACKAGE_IDENTITY_MALFORMED' configure >/dev/null
echo "CASE17_MALFORMED_IDENTITY_REFUSED_OK mocked"

# --- Case 18: erroring identity lookup must not be treated as absent ---
R18="$(new_root root-lookup-err)"
install_mocks "${R18}/private-tools"
write_state "$R18" '{"calls":[],"groups":{},"users":{},"units":{},"fail":null,"getent_error":{"passwd":{"platform-agent":1}}}'
out18="$(expect_fail_diag "$R18" postinst 'PLATFORM_AGENT_PACKAGE_IDENTITY_LOOKUP_FAILED' configure)"
python3 - <<PY
import json
st=json.load(open("${R18}/mock-state/boundary-state.json"))
assert not any(c[0]=="useradd" for c in st["calls"]), st["calls"]
print("CASE18_LOOKUP_ERROR_NOT_ABSENT_OK mocked")
PY

# --- Case 19: failed identity read-back after create ---
R19="$(new_root root-readback)"
install_mocks "${R19}/private-tools"
write_state "$R19" '{"calls":[],"groups":{},"users":{},"units":{},"fail":null,"suppress_user_readback":true}'
expect_fail_diag "$R19" postinst 'PLATFORM_AGENT_PACKAGE_IDENTITY_READBACK_FAILED' configure >/dev/null
python3 - <<PY
import json
st=json.load(open("${R19}/mock-state/boundary-state.json"))
assert any(c[0]=="useradd" for c in st["calls"]), st["calls"]
assert "platform-agent" in st["users"]
print("CASE19_READBACK_FAILED_OK mocked")
PY

# --- Case 20: newly created identities are validated (non-zero ids, nologin home) ---
R20="$(new_root root-new-identity)"
install_mocks "${R20}/private-tools"
mkdir -p "${R20}/mock-state"
run_hook "$R20" postinst configure >/dev/null
python3 - <<PY
import json
st=json.load(open("${R20}/mock-state/boundary-state.json"))
u=st["users"]["platform-agent"].split(":")
g=st["groups"]["platform-agent"].split(":")
assert u[2] != "0" and u[3] != "0" and g[2] != "0"
assert u[5] == "/var/lib/platform/agent"
assert u[6] == "/usr/sbin/nologin"
assert any(c[0]=="useradd" for c in st["calls"])
assert any(c[0]=="groupadd" for c in st["calls"])
print("CASE20_NEW_IDENTITY_VALIDATED_OK mocked")
PY

# --- Case 21: inactive unit skips restart on configure ---
R21="$(new_root root-inactive-skip)"
install_mocks "${R21}/private-tools"
seed_configured_env "$R21"
write_state "$R21" '{"calls":[],"groups":{},"users":{},"units":{"platform-agent.service":{"active":"inactive","enabled":"disabled"}},"fail":null}'
run_hook "$R21" postinst configure | tee "${WORKDIR}/c21-postinst.txt"
grep -q 'PLATFORM_AGENT_PACKAGE_POSTINST_OK' "${WORKDIR}/c21-postinst.txt"
python3 - <<PY
import json
st=json.load(open("${R21}/mock-state/boundary-state.json"))
assert st["units"]["platform-agent.service"]["enabled"]=="enabled"
assert st["units"]["platform-agent.service"]["active"]=="inactive"
assert ["systemctl","is-active","platform-agent.service"] in st["calls"]
assert not any(c[:2]==["systemctl","try-restart"] for c in st["calls"]), st["calls"]
assert not any(c[:2]==["systemctl","start"] for c in st["calls"]), st["calls"]
print("CASE21_INACTIVE_NO_RESTART_OK mocked")
PY

# --- Case 22: active unit restarts successfully ---
R22="$(new_root root-active-restart)"
install_mocks "${R22}/private-tools"
seed_configured_env "$R22"
write_state "$R22" '{"calls":[],"groups":{},"users":{},"units":{"platform-agent.service":{"active":"active","enabled":"enabled"}},"fail":null}'
run_hook "$R22" postinst configure | tee "${WORKDIR}/c22-postinst.txt"
grep -q 'PLATFORM_AGENT_PACKAGE_POSTINST_OK' "${WORKDIR}/c22-postinst.txt"
python3 - <<PY
import json
st=json.load(open("${R22}/mock-state/boundary-state.json"))
assert ["systemctl","try-restart","platform-agent.service"] in st["calls"]
print("CASE22_ACTIVE_RESTART_OK mocked")
PY

# --- Case 23: active unit restart failure suppresses POSTINST_OK ---
R23="$(new_root root-restart-fail)"
install_mocks "${R23}/private-tools"
seed_configured_env "$R23"
write_state "$R23" '{"calls":[],"groups":{},"users":{},"units":{"platform-agent.service":{"active":"active","enabled":"enabled"}},"fail":["systemctl","try-restart"]}'
expect_fail_diag "$R23" postinst 'PLATFORM_AGENT_PACKAGE_SERVICE_RESTART_FAILED' configure >/dev/null
python3 - <<PY
import json
st=json.load(open("${R23}/mock-state/boundary-state.json"))
# enable may have succeeded before restart failure; customer state must remain.
print("CASE23_RESTART_FAIL_NO_POSTINST_OK mocked")
PY
test -f "${R23}/etc/platform/agent.env"

# --- Case 24: stop failure on remove suppresses PRERM_OK ---
R24="$(new_root root-stop-fail)"
install_mocks "${R24}/private-tools"
seed_configured_env "$R24"
write_state "$R24" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32124:"},"users":{"platform-agent":"platform-agent:x:32124:32124::/var/lib/platform/agent:/usr/sbin/nologin"},"units":{"platform-agent.service":{"active":"active","enabled":"enabled"}},"fail":["systemctl","stop"]}'
mkdir -p "${R24}/var/lib/platform/agent"
printf 'spool\n' > "${R24}/var/lib/platform/agent/spool.marker"
expect_fail_diag "$R24" prerm 'PLATFORM_AGENT_PACKAGE_SERVICE_STOP_FAILED' remove >/dev/null
test -f "${R24}/etc/platform/agent.env"
test -f "${R24}/var/lib/platform/agent/spool.marker"
echo "CASE24_STOP_FAIL_NO_PRERM_OK mocked"

# --- Case 25: disable failure on remove suppresses PRERM_OK ---
R25="$(new_root root-disable-fail)"
install_mocks "${R25}/private-tools"
seed_configured_env "$R25"
mkdir -p "${R25}/var/lib/platform/agent"
printf 'spool\n' > "${R25}/var/lib/platform/agent/spool.marker"
write_state "$R25" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32125:"},"users":{"platform-agent":"platform-agent:x:32125:32125::/var/lib/platform/agent:/usr/sbin/nologin"},"units":{"platform-agent.service":{"active":"active","enabled":"enabled"}},"fail":["systemctl","disable"]}'
expect_fail_diag "$R25" prerm 'PLATFORM_AGENT_PACKAGE_SERVICE_DISABLE_FAILED' remove >/dev/null
test -f "${R25}/etc/platform/agent.env"
test -f "${R25}/var/lib/platform/agent/spool.marker"
python3 - <<PY
import json
st=json.load(open("${R25}/mock-state/boundary-state.json"))
# stop should have been attempted before disable failure
assert ["systemctl","stop","platform-agent.service"] in st["calls"]
assert ["systemctl","disable","platform-agent.service"] in st["calls"]
print("CASE25_DISABLE_FAIL_NO_PRERM_OK mocked")
PY

# --- Case 26: service-state query failure is not treated as inactive ---
R26="$(new_root root-state-query-fail)"
install_mocks "${R26}/private-tools"
seed_configured_env "$R26"
write_state "$R26" '{"calls":[],"groups":{},"users":{},"units":{"platform-agent.service":{"active":"inactive","enabled":"disabled","query_error":true,"query_error_exit":1}},"fail":null}'
expect_fail_diag "$R26" postinst 'PLATFORM_AGENT_PACKAGE_SERVICE_STATE_QUERY_FAILED' configure >/dev/null
python3 - <<PY
import json
st=json.load(open("${R26}/mock-state/boundary-state.json"))
assert not any(c[:2]==["systemctl","try-restart"] for c in st["calls"]), st["calls"]
print("CASE26_STATE_QUERY_FAIL_OK mocked")
PY

# --- Case 27: unavailable service manager is unsupported/unapplied (not a success) ---
R27="$(new_root root-no-systemctl)"
install_mocks "${R27}/private-tools"
seed_configured_env "$R27"
mkdir -p "${R27}/mock-state"
rm -f "${R27}/private-tools/systemctl"
expect_fail_diag "$R27" postinst 'PLATFORM_AGENT_PACKAGE_SERVICE_MANAGER_UNSUPPORTED' configure >/dev/null
test -f "${R27}/etc/platform/agent.env"
echo "CASE27_SERVICE_MANAGER_UNSUPPORTED_OK mocked"

# --- Case 28: pre-existing safe "*" password account from fixture cwd with >=2 entries ---
R28="$(new_root root-preexisting-star)"
install_mocks "${R28}/private-tools"
write_state "$R28" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32128:"},"users":{"platform-agent":"platform-agent:*:32128:32128::/var/lib/platform/agent:/usr/sbin/nologin"},"units":{},"fail":null}'
FIXTURE_CWD="${R28}/fixture-cwd"
mkdir -p "$FIXTURE_CWD"
printf 'one\n' > "${FIXTURE_CWD}/visible-a"
printf 'two\n' > "${FIXTURE_CWD}/visible-b"
(
  cd "$FIXTURE_CWD"
  # shellcheck disable=SC2035
  visible=(*)
  [[ "${#visible[@]}" -ge 2 ]] || { echo "FAIL: fixture cwd needs >=2 visible entries" >&2; exit 1; }
  run_hook "$R28" preinst install >/dev/null
  run_hook "$R28" postinst configure >/dev/null
)
python3 - <<PY
import json
st=json.load(open("${R28}/mock-state/boundary-state.json"))
assert not any(c[0]=="useradd" for c in st["calls"]), st["calls"]
assert not any(c[0]=="groupadd" for c in st["calls"]), st["calls"]
assert not any(c[:2]==["systemctl","enable"] for c in st["calls"]), st["calls"]
print("CASE28_STAR_PASSWORD_PREEXISTING_OK")
PY

# --- Case 29: empty group-members field is accepted (exactly four group fields) ---
R29="$(new_root root-empty-group-members)"
install_mocks "${R29}/private-tools"
write_state "$R29" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32129:"},"users":{"platform-agent":"platform-agent:x:32129:32129::/var/lib/platform/agent:/usr/sbin/nologin"},"units":{},"fail":null}'
run_hook "$R29" preinst install >/dev/null
run_hook "$R29" postinst configure >/dev/null
python3 - <<PY
import json
st=json.load(open("${R29}/mock-state/boundary-state.json"))
assert st["groups"]["platform-agent"].endswith(":"), st["groups"]["platform-agent"]
assert not any(c[0]=="useradd" for c in st["calls"]), st["calls"]
assert not any(c[0]=="groupadd" for c in st["calls"]), st["calls"]
print("CASE29_EMPTY_GROUP_MEMBERS_OK")
PY

# --- Case 30: group record missing fields refused (IDENTITY_MALFORMED; no enable) ---
R30="$(new_root root-group-missing-fields)"
install_mocks "${R30}/private-tools"
write_state "$R30" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32130"},"users":{},"units":{},"fail":null}'
expect_fail_diag "$R30" postinst 'PLATFORM_AGENT_PACKAGE_IDENTITY_MALFORMED' configure >/dev/null
python3 - <<PY
import json
st=json.load(open("${R30}/mock-state/boundary-state.json"))
assert not any(c[0] in ("useradd","groupadd") for c in st["calls"]), st["calls"]
assert not any(c[:2]==["systemctl","enable"] for c in st["calls"]), st["calls"]
print("CASE30_GROUP_MISSING_FIELDS_REFUSED_OK")
PY

# --- Case 31: group record extra fields refused (IDENTITY_MALFORMED; no enable) ---
R31="$(new_root root-group-extra-fields)"
install_mocks "${R31}/private-tools"
write_state "$R31" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32131:members:extra"},"users":{},"units":{},"fail":null}'
expect_fail_diag "$R31" postinst 'PLATFORM_AGENT_PACKAGE_IDENTITY_MALFORMED' configure >/dev/null
python3 - <<PY
import json
st=json.load(open("${R31}/mock-state/boundary-state.json"))
assert not any(c[0] in ("useradd","groupadd") for c in st["calls"]), st["calls"]
assert not any(c[:2]==["systemctl","enable"] for c in st["calls"]), st["calls"]
print("CASE31_GROUP_EXTRA_FIELDS_REFUSED_OK")
PY

# --- Case 32: passwd record too few fields refused (IDENTITY_MALFORMED; no enable) ---
R32="$(new_root root-passwd-too-few)"
install_mocks "${R32}/private-tools"
write_state "$R32" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32132:"},"users":{"platform-agent":"platform-agent:x:32132:32132::/var/lib/platform/agent"},"units":{},"fail":null}'
expect_fail_diag "$R32" postinst 'PLATFORM_AGENT_PACKAGE_IDENTITY_MALFORMED' configure >/dev/null
python3 - <<PY
import json
st=json.load(open("${R32}/mock-state/boundary-state.json"))
assert not any(c[0] in ("useradd","groupadd") for c in st["calls"]), st["calls"]
assert not any(c[:2]==["systemctl","enable"] for c in st["calls"]), st["calls"]
print("CASE32_PASSWD_TOO_FEW_FIELDS_REFUSED_OK")
PY

# --- Case 33: passwd record too many fields refused (IDENTITY_MALFORMED; no enable) ---
R33="$(new_root root-passwd-too-many)"
install_mocks "${R33}/private-tools"
write_state "$R33" '{"calls":[],"groups":{"platform-agent":"platform-agent:x:32133:"},"users":{"platform-agent":"platform-agent:x:32133:32133::/var/lib/platform/agent:/usr/sbin/nologin:extra"},"units":{},"fail":null}'
expect_fail_diag "$R33" postinst 'PLATFORM_AGENT_PACKAGE_IDENTITY_MALFORMED' configure >/dev/null
python3 - <<PY
import json
st=json.load(open("${R33}/mock-state/boundary-state.json"))
assert not any(c[0] in ("useradd","groupadd") for c in st["calls"]), st["calls"]
assert not any(c[:2]==["systemctl","enable"] for c in st["calls"]), st["calls"]
print("CASE33_PASSWD_TOO_MANY_FIELDS_REFUSED_OK")
PY

echo "PASS test-maintainer-scripts.sh (MOCKED systemctl/account tools; native VM NOT_RUN)"
