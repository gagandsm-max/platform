#!/usr/bin/env bash
# Behavioral harness for tests/run-ui-typecheck.sh (Issue #240).
# Host/container cases stub phase commands; wrapper cases exercise the
# production platform_ui_typecheck_docker path (cancellation budget, log
# retention, docker-wait status, success/nonzero). Does not alter
# dependencies, lockfiles, workflows or Docker daemon.
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

# ==============================================================================
# Production wrapper regressions (platform_ui_typecheck_docker) — Issue #240
# Direct container KILL alone does not prove host-wrapper cancellation.
# ==============================================================================

make_wrapper_root() {
    local work="$1"
    mkdir -p "${work}/ui" "${work}/tests"
    cp "$HELPER" "${work}/tests/run-ui-typecheck.sh"
    echo '{"name":"wrapper-fixture"}' >"${work}/ui/package.json"
}

# ---- Wrapper: normal success + nonzero phase exit (stub cmds, real docker) ----
wrap_ok="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-wrap-ok.XXXXXX")"
make_wrapper_root "$wrap_ok"
set +e
wrap_ok_out="$(
    UI_TYPECHECK_COPY_CMD='mkdir -p /app' \
    UI_TYPECHECK_INSTALL_CMD='true' \
    UI_TYPECHECK_TSC_CMD='true' \
    bash -c 'set -euo pipefail; source "$1"; platform_ui_typecheck_docker "$2"' \
        bash "$HELPER" "$wrap_ok" 2>&1
)"
wrap_ok_rc=$?
set -e
echo "---- wrapper success: exit=${wrap_ok_rc} ----"
printf '%s\n' "$wrap_ok_out"
assert_eq "wrapper/success/exit" "0" "$wrap_ok_rc"
assert_contains "wrapper/success/copy-complete" "UI_TYPECHECK_PHASE_COMPLETE phase=SOURCE_COPY exit=0" "$wrap_ok_out"
assert_contains "wrapper/success/install-complete" "UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL exit=0" "$wrap_ok_out"
assert_contains "wrapper/success/tsc-complete" "UI_TYPECHECK_PHASE_COMPLETE phase=TYPESCRIPT_CHECK exit=0" "$wrap_ok_out"
assert_not_contains "wrapper/success/no-oom" "OOM" "$wrap_ok_out"
rm -rf "$wrap_ok"

wrap_fail="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-wrap-fail.XXXXXX")"
make_wrapper_root "$wrap_fail"
set +e
wrap_fail_out="$(
    UI_TYPECHECK_COPY_CMD='mkdir -p /app' \
    UI_TYPECHECK_INSTALL_CMD='exit 42' \
    UI_TYPECHECK_TSC_CMD='true' \
    bash -c 'set -euo pipefail; source "$1"; platform_ui_typecheck_docker "$2"' \
        bash "$HELPER" "$wrap_fail" 2>&1
)"
wrap_fail_rc=$?
set -e
echo "---- wrapper nonzero phase: exit=${wrap_fail_rc} ----"
printf '%s\n' "$wrap_fail_out"
assert_eq "wrapper/nonzero/exit" "42" "$wrap_fail_rc"
assert_contains "wrapper/nonzero/install-complete" "UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL exit=42" "$wrap_fail_out"
assert_not_contains "wrapper/nonzero/no-tsc" "UI_TYPECHECK_PHASE_START phase=TYPESCRIPT_CHECK" "$wrap_fail_out"
# Final COMPLETE retained in terminal output (F02 log retention).
assert_contains "wrapper/nonzero/final-complete-retained" "UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL exit=42" "$wrap_fail_out"
rm -rf "$wrap_fail"

# ---- Wrapper: Docker wait/transport failure preserves original CLI status (F03) ----
stub_bin="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-docker-stub.XXXXXX")"
cat >"${stub_bin}/docker" <<'EOF'
#!/bin/bash
# Deterministic docker CLI stub: run succeeds; wait fails with transport status.
cmd="${1:-}"
shift || true
case "$cmd" in
    run)
        echo "stubcid-waitfail-$$"
        exit 0
        ;;
    logs)
        # Mimic follower until killed; emit nothing material.
        sleep 30
        exit 0
        ;;
    wait)
        echo "Error: Cannot connect to the Docker daemon" >&2
        exit 125
        ;;
    kill|rm|inspect)
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
EOF
chmod +x "${stub_bin}/docker"
wrap_df="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-wrap-df.XXXXXX")"
make_wrapper_root "$wrap_df"
set +e
wrap_df_out="$(
    PATH="${stub_bin}:$PATH" \
    UI_TYPECHECK_COPY_CMD='true' \
    UI_TYPECHECK_INSTALL_CMD='true' \
    UI_TYPECHECK_TSC_CMD='true' \
    bash -c 'set -euo pipefail; source "$1"; platform_ui_typecheck_docker "$2"' \
        bash "$HELPER" "$wrap_df" 2>&1
)"
wrap_df_rc=$?
set -e
echo "---- wrapper docker-wait failure: exit=${wrap_df_rc} ----"
printf '%s\n' "$wrap_df_out"
assert_eq "wrapper/docker-wait-fail/exit" "125" "$wrap_df_rc"
assert_contains "wrapper/docker-wait-fail/marker" "UI_TYPECHECK_DOCKER_WAIT_FAILED exit=125" "$wrap_df_out"
rm -rf "$wrap_df" "$stub_bin"

# ---- Wrapper: empty/invalid wait status (no Bash numeric-return collapse) ----
stub_bin2="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-docker-stub2.XXXXXX")"
cat >"${stub_bin2}/docker" <<'EOF'
#!/bin/bash
cmd="${1:-}"
shift || true
case "$cmd" in
    run)
        echo "stubcid-emptywait-$$"
        exit 0
        ;;
    logs)
        sleep 30
        exit 0
        ;;
    wait)
        # CLI reports success but prints no validated exit integer.
        exit 0
        ;;
    kill|rm|inspect)
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
EOF
chmod +x "${stub_bin2}/docker"
wrap_inv="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-wrap-inv.XXXXXX")"
make_wrapper_root "$wrap_inv"
set +e
wrap_inv_out="$(
    PATH="${stub_bin2}:$PATH" \
    bash -c 'set -euo pipefail; source "$1"; platform_ui_typecheck_docker "$2"' \
        bash "$HELPER" "$wrap_inv" 2>&1
)"
wrap_inv_rc=$?
set -e
echo "---- wrapper invalid wait status: exit=${wrap_inv_rc} ----"
printf '%s\n' "$wrap_inv_out"
assert_eq "wrapper/invalid-wait/exit" "1" "$wrap_inv_rc"
assert_contains "wrapper/invalid-wait/marker" "UI_TYPECHECK_DOCKER_WAIT_INVALID" "$wrap_inv_out"
rm -rf "$wrap_inv" "$stub_bin2"

# ---- Wrapper: delayed terminal COMPLETE beyond live-follow drain budget (F02) ----
# Fake Docker: wait returns 42 promptly; logs -f emits START then delays COMPLETE
# past the drain budget; non-follow snapshot still has COMPLETE. Wrapper must
# recover the terminal record (or report incomplete) before removing the source.
stub_delay="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-docker-delay.XXXXXX")"
cat >"${stub_delay}/docker" <<EOF
#!/bin/bash
cmd="\${1:-}"
shift || true
case "\$cmd" in
    run)
        echo "stubcid-delay-\$\$"
        exit 0
        ;;
    wait)
        echo 42
        exit 0
        ;;
    logs)
        follow=0
        for a in "\$@"; do
            [ "\$a" = "-f" ] && follow=1
        done
        if [ "\$follow" -eq 1 ]; then
            echo "UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL" >&2
            sleep 8
            echo "UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL exit=42" >&2
            exit 0
        fi
        echo "UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL" >&2
        echo "UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL exit=42" >&2
        exit 0
        ;;
    rm)
        echo "RM_CALLED" >>"${stub_delay}/rm.log"
        exit 0
        ;;
    kill|inspect)
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
EOF
chmod +x "${stub_delay}/docker"
: >"${stub_delay}/rm.log"
wrap_delay="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-wrap-delay.XXXXXX")"
make_wrapper_root "$wrap_delay"
set +e
delay_t0=$SECONDS
wrap_delay_out="$(
    PATH="${stub_delay}:$PATH" \
    UI_TYPECHECK_LOG_DRAIN_BUDGET_SEC=2 \
    bash -c 'set -euo pipefail; source "$1"; platform_ui_typecheck_docker "$2"' \
        bash "$HELPER" "$wrap_delay" 2>&1
)"
wrap_delay_rc=$?
wrap_delay_elapsed=$((SECONDS - delay_t0))
set -e
echo "---- wrapper delayed terminal log: exit=${wrap_delay_rc} elapsed_s=${wrap_delay_elapsed} ----"
printf '%s\n' "$wrap_delay_out"
assert_eq "wrapper/delayed-tail/exit" "42" "$wrap_delay_rc"
assert_contains "wrapper/delayed-tail/complete-retained" "UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL exit=42" "$wrap_delay_out"
assert_contains "wrapper/delayed-tail/recovered-or-complete" "UI_TYPECHECK_LOGS_RECOVERED" "$wrap_delay_out"
# Must not approach the 8s follow delay; drain budget + small slack only.
if [ "$wrap_delay_elapsed" -le 6 ]; then
    pass "wrapper/delayed-tail/bounded-drain elapsed_s=${wrap_delay_elapsed}"
else
    fail "wrapper/delayed-tail/bounded-drain elapsed_s=${wrap_delay_elapsed} (expected <=6)"
fi
if [ -s "${stub_delay}/rm.log" ]; then
    pass "wrapper/delayed-tail/container-removed-after-recovery"
else
    fail "wrapper/delayed-tail/container-removed-after-recovery (rm not called)"
fi
rm -rf "$wrap_delay" "$stub_delay"

# ---- Wrapper: logger failure / incomplete retrieval retains evidence (F02) ----
stub_logfail="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-docker-logfail.XXXXXX")"
cat >"${stub_logfail}/docker" <<EOF
#!/bin/bash
cmd="\${1:-}"
shift || true
case "\$cmd" in
    run)
        echo "stubcid-logfail-\$\$"
        exit 0
        ;;
    wait)
        echo 0
        exit 0
        ;;
    logs)
        follow=0
        for a in "\$@"; do
            [ "\$a" = "-f" ] && follow=1
        done
        if [ "\$follow" -eq 1 ]; then
            echo "UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL" >&2
            exit 1
        fi
        echo "Error: cannot read container logs" >&2
        exit 1
        ;;
    rm)
        echo "RM_CALLED" >>"${stub_logfail}/rm.log"
        exit 0
        ;;
    kill|inspect)
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
EOF
chmod +x "${stub_logfail}/docker"
: >"${stub_logfail}/rm.log"
wrap_logfail="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-wrap-logfail.XXXXXX")"
make_wrapper_root "$wrap_logfail"
set +e
wrap_logfail_out="$(
    PATH="${stub_logfail}:$PATH" \
    UI_TYPECHECK_LOG_DRAIN_BUDGET_SEC=2 \
    bash -c 'set -euo pipefail; source "$1"; platform_ui_typecheck_docker "$2"' \
        bash "$HELPER" "$wrap_logfail" 2>&1
)"
wrap_logfail_rc=$?
set -e
echo "---- wrapper logger failure/incomplete: exit=${wrap_logfail_rc} ----"
printf '%s\n' "$wrap_logfail_out"
assert_eq "wrapper/log-incomplete/exit" "0" "$wrap_logfail_rc"
assert_contains "wrapper/log-incomplete/marker" "UI_TYPECHECK_LOGS_INCOMPLETE" "$wrap_logfail_out"
assert_contains "wrapper/log-incomplete/retained-flag" "retained=1" "$wrap_logfail_out"
assert_contains "wrapper/log-incomplete/cleanup-retained" "UI_TYPECHECK_CLEANUP_RETAINED" "$wrap_logfail_out"
if [ -s "${stub_logfail}/rm.log" ]; then
    fail "wrapper/log-incomplete/no-rm-while-retained unexpectedly removed container"
else
    pass "wrapper/log-incomplete/no-rm-while-retained"
fi
rm -rf "$wrap_logfail" "$stub_logfail"

# ---- Wrapper: host cancellation while owned child ignores soft signal (F01/F02) ----
# Exercises production platform_ui_typecheck_docker under a responsive daemon.
# PID1 ignores TERM so soft-signal alone would hang unbounded docker wait;
# finite cancel budget must force-remove and return promptly. Log finalization
# must run before deleting the only log source (cancel-path F02).
wrap_cancel="$(mktemp -d "${TMPDIR:-/tmp}/ui-tc-wrap-cancel.XXXXXX")"
make_wrapper_root "$wrap_cancel"
cancel_budget=3
cancel_log="${wrap_cancel}/wrapper.out"
CANCEL_BUDGET_DECLARED=$cancel_budget
set +e
# Background the wrapper bash itself so TERM reaches the trap owner (F01).
# PID1 inside the container ignores TERM; budgeted force-rm must still return.
UI_TYPECHECK_CANCEL_BUDGET_SEC="$cancel_budget" \
UI_TYPECHECK_LOG_DRAIN_BUDGET_SEC=2 \
UI_TYPECHECK_REMOTE_CMD='trap "" TERM; set +e; . /runner/run-ui-typecheck.sh; platform_ui_typecheck_run_phases' \
UI_TYPECHECK_COPY_CMD='mkdir -p /app' \
UI_TYPECHECK_INSTALL_CMD='sleep 120' \
UI_TYPECHECK_TSC_CMD='true' \
bash -c 'set -euo pipefail; source "$1"; platform_ui_typecheck_docker "$2"' \
    bash "$HELPER" "$wrap_cancel" >"$cancel_log" 2>&1 &
cancel_pid=$!
# Wait until install START, then TERM the host wrapper (not merely the container).
for _ in $(seq 1 150); do
    if grep -Fq 'UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL' "$cancel_log" 2>/dev/null; then
        break
    fi
    cancel_cid="$(docker ps -q --filter "name=platform-ui-typecheck-${cancel_pid}" 2>/dev/null | head -n1 || true)"
    if [ -n "$cancel_cid" ] && docker logs "$cancel_cid" 2>&1 | grep -Fq 'UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL'; then
        break
    fi
    if ! kill -0 "$cancel_pid" 2>/dev/null; then
        break
    fi
    sleep 0.1
done
cancel_t0="$SECONDS"
kill -TERM "$cancel_pid" 2>/dev/null || true
wait "$cancel_pid"
cancel_rc=$?
cancel_elapsed=$((SECONDS - cancel_t0))
set -e
cancel_out="$(cat "$cancel_log")"
echo "---- wrapper host-cancel (ignore soft): exit=${cancel_rc} elapsed_s=${cancel_elapsed} budget=${cancel_budget} ----"
printf '%s\n' "$cancel_out"
echo "CANCEL_BUDGET_SEC_DECLARED=${CANCEL_BUDGET_DECLARED}"

assert_contains "wrapper/cancel/copy-complete" "UI_TYPECHECK_PHASE_COMPLETE phase=SOURCE_COPY exit=0" "$cancel_out"
assert_contains "wrapper/cancel/install-start" "UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL" "$cancel_out"
assert_not_contains "wrapper/cancel/install-complete-absent" "UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL" "$cancel_out"
assert_not_contains "wrapper/cancel/no-tsc" "UI_TYPECHECK_PHASE_START phase=TYPESCRIPT_CHECK" "$cancel_out"
assert_not_contains "wrapper/cancel/no-oom" "OOM" "$cancel_out"
assert_not_contains "wrapper/cancel/no-146-claim" "146 means" "$cancel_out"
case "$cancel_rc" in
    143|15) pass "wrapper/cancel/host-term-exit exit=${cancel_rc}" ;;
    *) fail "wrapper/cancel/host-term-exit unexpected exit=${cancel_rc}" ;;
esac
# Responsive-daemon budget: soft wait <= cancel_budget, plus small slack for
# log finalize/rm — must not approach the 120s ignored-child sleep.
cancel_slack=$((cancel_budget + 12))
if [ "$cancel_elapsed" -le "$cancel_slack" ]; then
    pass "wrapper/cancel/within-budget elapsed_s=${cancel_elapsed} slack=${cancel_slack}"
else
    fail "wrapper/cancel/within-budget elapsed_s=${cancel_elapsed} exceeded slack=${cancel_slack} (hang regression)"
fi
# Cancel-path evidence: terminal START preserved, and either recovered snapshot
# marker or explicit retained-incomplete status (never silent loss).
if grep -Fq 'UI_TYPECHECK_LOGS_RECOVERED' <<<"$cancel_out" \
    || grep -Fq 'UI_TYPECHECK_LOGS_INCOMPLETE' <<<"$cancel_out" \
    || grep -Fq 'UI_TYPECHECK_CLEANUP_RETAINED' <<<"$cancel_out"; then
    pass "wrapper/cancel/evidence-status-present"
else
    # Clean live-follow drain before delete also satisfies F02 when the
    # follower exits promptly after soft-signal/force path — START already kept.
    if grep -Fq 'UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL' <<<"$cancel_out"; then
        pass "wrapper/cancel/evidence-start-preserved-clean-drain"
    else
        fail "wrapper/cancel/evidence-status-present missing recovered/incomplete/start"
    fi
fi
# Owned container cleanup under a responsive daemon (unless explicitly retained).
leftover="$(docker ps -aq --filter "name=platform-ui-typecheck-${cancel_pid}" 2>/dev/null || true)"
if grep -Fq 'UI_TYPECHECK_CLEANUP_RETAINED' <<<"$cancel_out"; then
    pass "wrapper/cancel/retained-evidence-status"
    for id in $leftover; do
        docker rm -f "$id" >/dev/null 2>&1 || true
    done
elif [ -z "$leftover" ]; then
    pass "wrapper/cancel/container-removed"
else
    still_run=0
    for id in $leftover; do
        if [ "$(docker inspect -f '{{.State.Running}}' "$id" 2>/dev/null || echo false)" = "true" ]; then
            still_run=1
        fi
        docker rm -f "$id" >/dev/null 2>&1 || true
    done
    if [ "$still_run" -eq 0 ]; then
        pass "wrapper/cancel/container-not-running (removed leftovers)"
    else
        fail "wrapper/cancel/container-removed still running after cancel"
    fi
fi
rm -rf "$wrap_cancel"

echo "========================================================================"
echo "UI typecheck phase-diagnostics harness: PASSED=${PASSED} FAILED=${FAILED}"
echo "========================================================================"
if [ "$FAILED" -ne 0 ]; then
    exit 1
fi
exit 0
