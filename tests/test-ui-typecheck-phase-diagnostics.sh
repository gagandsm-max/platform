#!/usr/bin/env bash
# Behavioral harness for tests/run-ui-typecheck.sh phase helper (Issue #240).
# Host cases stub phase commands; container cases use node:20-alpine with the
# same helper. Does not alter dependencies, lockfiles, workflows or Docker daemon.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HELPER="${ROOT_DIR}/tests/run-ui-typecheck.sh"
PASSED=0
FAILED=0

fail() {
    echo "FAIL: $*" >&2
    FAILED=$((FAILED + 1))
}

pass() {
    echo "PASS: $*"
    PASSED=$((PASSED + 1))
}

assert_eq() {
    local label="$1" expected="$2" actual="$3"
    if [ "$expected" = "$actual" ]; then
        pass "$label (expected=$expected)"
    else
        fail "$label expected=$expected actual=$actual"
    fi
}

assert_contains() {
    local label="$1" needle="$2" haystack="$3"
    if grep -Fq -- "$needle" <<<"$haystack"; then
        pass "$label"
    else
        fail "$label missing '$needle'"
    fi
}

assert_not_contains() {
    local label="$1" needle="$2" haystack="$3"
    if grep -Fq -- "$needle" <<<"$haystack"; then
        fail "$label unexpectedly contains '$needle'"
    else
        pass "$label"
    fi
}

# ---- Host: source helper with stub phase commands ----
run_host_case() {
    local name="$1" copy_rc="$2" install_rc="$3" tsc_rc="$4" expect_rc="$5"
    local work out rc
    work="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-host-${name}.XXXXXX")"
    mkdir -p "${work}/src" "${work}/bin" "${work}/app_parent"
    # Destination parent for default-style copy target.
    cat >"${work}/bin/copy.sh" <<EOF
#!/bin/sh
echo "stub-copy" >>"${work}/phases.log"
mkdir -p "${work}/app"
exit ${copy_rc}
EOF
    cat >"${work}/bin/install.sh" <<EOF
#!/bin/sh
echo "stub-install" >>"${work}/phases.log"
exit ${install_rc}
EOF
    cat >"${work}/bin/tsc.sh" <<EOF
#!/bin/sh
echo "stub-tsc" >>"${work}/phases.log"
exit ${tsc_rc}
EOF
    chmod +x "${work}/bin/"*.sh

    set +e
    out="$(
        UI_TYPECHECK_SRC="${work}/src" \
        UI_TYPECHECK_APP="${work}/app" \
        UI_TYPECHECK_COPY_CMD="${work}/bin/copy.sh" \
        UI_TYPECHECK_INSTALL_CMD="${work}/bin/install.sh" \
        UI_TYPECHECK_TSC_CMD="${work}/bin/tsc.sh" \
        bash -c 'set -euo pipefail; source "$1"; platform_ui_typecheck_run_phases' bash "$HELPER" 2>&1
    )"
    rc=$?
    set -e

    echo "---- host case ${name}: exit=${rc} ----"
    printf '%s\n' "$out"

    assert_eq "host/${name}/exit" "$expect_rc" "$rc"
    assert_contains "host/${name}/copy-start" "UI_TYPECHECK_PHASE_START phase=SOURCE_COPY" "$out"

    if [ "$copy_rc" -ne 0 ]; then
        assert_contains "host/${name}/copy-complete-fail" "UI_TYPECHECK_PHASE_COMPLETE phase=SOURCE_COPY exit=${copy_rc}" "$out"
        assert_not_contains "host/${name}/no-install-start" "UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL" "$out"
        assert_not_contains "host/${name}/no-tsc-start" "UI_TYPECHECK_PHASE_START phase=TYPESCRIPT_CHECK" "$out"
        assert_not_contains "host/${name}/no-install-stub" "stub-install" "$(cat "${work}/phases.log" 2>/dev/null || true)"
        assert_not_contains "host/${name}/no-tsc-stub" "stub-tsc" "$(cat "${work}/phases.log" 2>/dev/null || true)"
    else
        assert_contains "host/${name}/copy-complete" "UI_TYPECHECK_PHASE_COMPLETE phase=SOURCE_COPY exit=0" "$out"
        assert_contains "host/${name}/install-start" "UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL" "$out"
        if [ "$install_rc" -ne 0 ]; then
            assert_contains "host/${name}/install-complete-fail" "UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL exit=${install_rc}" "$out"
            assert_not_contains "host/${name}/no-tsc-start" "UI_TYPECHECK_PHASE_START phase=TYPESCRIPT_CHECK" "$out"
            assert_not_contains "host/${name}/no-tsc-stub" "stub-tsc" "$(cat "${work}/phases.log" 2>/dev/null || true)"
        else
            assert_contains "host/${name}/install-complete" "UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL exit=0" "$out"
            assert_contains "host/${name}/tsc-start" "UI_TYPECHECK_PHASE_START phase=TYPESCRIPT_CHECK" "$out"
            assert_contains "host/${name}/tsc-complete" "UI_TYPECHECK_PHASE_COMPLETE phase=TYPESCRIPT_CHECK exit=${tsc_rc}" "$out"
        fi
    fi

    # Elapsed field present on every COMPLETE line; no env/credential dumps.
    assert_not_contains "host/${name}/no-env-dump" "npm_config" "$out"
    assert_not_contains "host/${name}/no-trace" "+ platform_ui_typecheck" "$out"
    while IFS= read -r line; do
        case "$line" in
            UI_TYPECHECK_PHASE_COMPLETE*)
                if ! grep -Eq 'elapsed_ms=[0-9]+|elapsed_ms=unknown' <<<"$line"; then
                    fail "host/${name}/elapsed missing on: $line"
                else
                    pass "host/${name}/elapsed on complete"
                fi
                # Must not invent OOM/signal provenance from opaque exits.
                if grep -Eqi 'oom|sig(kill|term)|signal 18|exit 146 means' <<<"$line"; then
                    fail "host/${name}/forbidden provenance inference: $line"
                fi
                ;;
        esac
    done <<<"$out"

    rm -rf "$work"
}

run_host_case success 0 0 0 0
run_host_case fail-copy 17 0 0 17
run_host_case fail-install 0 42 0 42
run_host_case fail-typecheck 0 0 7 7

# ---- Host: owned-process signal leaves missing COMPLETE without invented cause ----
signal_work="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-signal.XXXXXX")"
mkdir -p "${signal_work}/src" "${signal_work}/bin"
cat >"${signal_work}/bin/copy.sh" <<EOF
#!/bin/sh
echo "stub-copy" >>"${signal_work}/phases.log"
mkdir -p "${signal_work}/app"
exit 0
EOF
cat >"${signal_work}/bin/install.sh" <<'EOF'
#!/bin/sh
# Long hang so the harness can signal mid-phase.
sleep 30
exit 0
EOF
cat >"${signal_work}/bin/tsc.sh" <<EOF
#!/bin/sh
echo "stub-tsc-should-not-run" >>"${signal_work}/phases.log"
exit 0
EOF
chmod +x "${signal_work}/bin/"*.sh

set +e
bash -c '
set -euo pipefail
source "$1"
export UI_TYPECHECK_SRC="$2/src"
export UI_TYPECHECK_APP="$2/app"
export UI_TYPECHECK_COPY_CMD="$2/bin/copy.sh"
export UI_TYPECHECK_INSTALL_CMD="$2/bin/install.sh"
export UI_TYPECHECK_TSC_CMD="$2/bin/tsc.sh"
platform_ui_typecheck_run_phases
' bash "$HELPER" "$signal_work" >"${signal_work}/out.txt" 2>&1 &
sig_pid=$!
# Wait until install START appears, then TERM the owned helper process.
for _ in $(seq 1 50); do
    if grep -Fq 'UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL' "${signal_work}/out.txt" 2>/dev/null; then
        break
    fi
    sleep 0.1
done
kill -TERM "$sig_pid" 2>/dev/null || true
wait "$sig_pid"
sig_rc=$?
set -e
sig_out="$(cat "${signal_work}/out.txt")"
echo "---- host signal case: exit=${sig_rc} ----"
printf '%s\n' "$sig_out"

assert_contains "host/signal/copy-complete" "UI_TYPECHECK_PHASE_COMPLETE phase=SOURCE_COPY exit=0" "$sig_out"
assert_contains "host/signal/install-start" "UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL" "$sig_out"
assert_not_contains "host/signal/install-complete-absent" "UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL" "$sig_out"
assert_not_contains "host/signal/no-tsc" "UI_TYPECHECK_PHASE_START phase=TYPESCRIPT_CHECK" "$sig_out"
assert_not_contains "host/signal/no-oom-claim" "OOM" "$sig_out"
assert_not_contains "host/signal/no-146-claim" "146 means" "$sig_out"
# TERM to bash typically yields 143 (128+15) or 15 depending on wait path.
case "$sig_rc" in
    143|15|130) pass "host/signal/nonzero-termination exit=${sig_rc}" ;;
    *) fail "host/signal/nonzero-termination unexpected exit=${sig_rc}" ;;
esac
rm -rf "$signal_work"

# ---- Container cases (node:20-alpine, read-only helper mount) ----
# Tiny fixture tree; phase commands are stubs — no npm registry / lockfile use.
run_container_case() {
    local name="$1" copy_rc="$2" install_rc="$3" tsc_rc="$4" expect_rc="$5"
    local work out rc name_c
    work="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-ctr-${name}.XXXXXX")"
    mkdir -p "${work}/ui" "${work}/stubs"
    # Minimal files so a real cp -r would succeed if invoked; stubs override.
    echo '{"name":"fixture"}' >"${work}/ui/package.json"
    cat >"${work}/stubs/copy.sh" <<EOF
#!/bin/sh
mkdir -p /app
echo ok > /app/.copied
exit ${copy_rc}
EOF
    cat >"${work}/stubs/install.sh" <<EOF
#!/bin/sh
exit ${install_rc}
EOF
    cat >"${work}/stubs/tsc.sh" <<EOF
#!/bin/sh
exit ${tsc_rc}
EOF
    chmod +x "${work}/stubs/"*.sh
    name_c="platform-ui-tc-${name}-$$"

    set +e
    out="$(docker run --rm --name "$name_c" \
        -v "${ROOT_DIR}/tests/run-ui-typecheck.sh:/runner/run-ui-typecheck.sh:ro" \
        -v "${work}/ui:/src:ro" \
        -v "${work}/stubs:/stubs:ro" \
        -e "UI_TYPECHECK_COPY_CMD=/stubs/copy.sh" \
        -e "UI_TYPECHECK_INSTALL_CMD=/stubs/install.sh" \
        -e "UI_TYPECHECK_TSC_CMD=/stubs/tsc.sh" \
        node:20-alpine \
        sh -c 'set -eu; . /runner/run-ui-typecheck.sh; platform_ui_typecheck_run_phases' 2>&1)"
    rc=$?
    set -e

    echo "---- container case ${name}: exit=${rc} ----"
    printf '%s\n' "$out"

    assert_eq "container/${name}/exit" "$expect_rc" "$rc"
    assert_contains "container/${name}/copy-start" "UI_TYPECHECK_PHASE_START phase=SOURCE_COPY" "$out"
    if [ "$copy_rc" -ne 0 ]; then
        assert_contains "container/${name}/copy-fail" "UI_TYPECHECK_PHASE_COMPLETE phase=SOURCE_COPY exit=${copy_rc}" "$out"
        assert_not_contains "container/${name}/no-install" "UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL" "$out"
        assert_not_contains "container/${name}/no-tsc" "UI_TYPECHECK_PHASE_START phase=TYPESCRIPT_CHECK" "$out"
    elif [ "$install_rc" -ne 0 ]; then
        assert_contains "container/${name}/install-fail" "UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL exit=${install_rc}" "$out"
        assert_not_contains "container/${name}/no-tsc" "UI_TYPECHECK_PHASE_START phase=TYPESCRIPT_CHECK" "$out"
    else
        assert_contains "container/${name}/tsc-complete" "UI_TYPECHECK_PHASE_COMPLETE phase=TYPESCRIPT_CHECK exit=${tsc_rc}" "$out"
    fi
    assert_not_contains "container/${name}/no-oom-inference" "OOM" "$out"
    rm -rf "$work"
}

run_container_case success 0 0 0 0
run_container_case fail-copy 19 0 0 19
run_container_case fail-install 0 23 0 23
run_container_case fail-typecheck 0 0 11 11

# ---- Container: signal/cleanup of owned long-running phase ----
sigc_work="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-ctr-sig.XXXXXX")"
mkdir -p "${sigc_work}/ui" "${sigc_work}/stubs"
echo '{}' >"${sigc_work}/ui/package.json"
cat >"${sigc_work}/stubs/copy.sh" <<'EOF'
#!/bin/sh
mkdir -p /app
exit 0
EOF
cat >"${sigc_work}/stubs/install.sh" <<'EOF'
#!/bin/sh
# Long child so the harness can externally stop the owned container mid-phase.
# PID 1 in a bare alpine container may ignore SIGTERM; the harness uses KILL.
sleep 120
exit 0
EOF
cat >"${sigc_work}/stubs/tsc.sh" <<'EOF'
#!/bin/sh
exit 0
EOF
chmod +x "${sigc_work}/stubs/"*.sh
sigc_name="platform-ui-tc-sig-$$"

set +e
cid="$(docker run -d --name "$sigc_name" \
    -v "${ROOT_DIR}/tests/run-ui-typecheck.sh:/runner/run-ui-typecheck.sh:ro" \
    -v "${sigc_work}/ui:/src:ro" \
    -v "${sigc_work}/stubs:/stubs:ro" \
    -e "UI_TYPECHECK_COPY_CMD=/stubs/copy.sh" \
    -e "UI_TYPECHECK_INSTALL_CMD=/stubs/install.sh" \
    -e "UI_TYPECHECK_TSC_CMD=/stubs/tsc.sh" \
    node:20-alpine \
    sh -c 'set -eu; . /runner/run-ui-typecheck.sh; platform_ui_typecheck_run_phases')"
# Wait for install START, then SIGKILL the owned container.
# SIGKILL is required for reliable stop when container PID1 does not forward TERM;
# do not infer OOM or historical exit-146 meaning from the resulting status.
for _ in $(seq 1 100); do
    if docker logs "$cid" 2>&1 | grep -Fq 'UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL'; then
        break
    fi
    sleep 0.1
done
docker kill -s KILL "$cid" >/dev/null
sigc_rc="$(docker wait "$cid")"
sigc_out="$(docker logs "$cid" 2>&1)"
docker rm -f "$cid" >/dev/null 2>&1 || true
set -e

echo "---- container signal case: exit=${sigc_rc} ----"
printf '%s\n' "$sigc_out"
assert_contains "container/signal/copy-complete" "UI_TYPECHECK_PHASE_COMPLETE phase=SOURCE_COPY exit=0" "$sigc_out"
assert_contains "container/signal/install-start" "UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL" "$sigc_out"
assert_not_contains "container/signal/install-complete-absent" "UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL" "$sigc_out"
assert_not_contains "container/signal/no-tsc" "UI_TYPECHECK_PHASE_START phase=TYPESCRIPT_CHECK" "$sigc_out"
assert_not_contains "container/signal/no-oom" "OOM" "$sigc_out"
assert_not_contains "container/signal/no-146-claim" "146 means" "$sigc_out"
# Docker maps SIGKILL to 137; accept related nonzero without inventing meaning.
case "$sigc_rc" in
    137|143|130|1) pass "container/signal/cleanup-exit exit=${sigc_rc}" ;;
    *) fail "container/signal/cleanup-exit unexpected exit=${sigc_rc}" ;;
esac
# Prove container is gone (cleanup).
if docker inspect "$cid" >/dev/null 2>&1; then
    fail "container/signal/removed still present"
else
    pass "container/signal/removed"
fi
rm -rf "$sigc_work"

echo "========================================================================"
echo "UI typecheck phase-diagnostics harness: PASSED=${PASSED} FAILED=${FAILED}"
echo "========================================================================"
if [ "$FAILED" -ne 0 ]; then
    exit 1
fi
exit 0
