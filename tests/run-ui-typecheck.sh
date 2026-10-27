#!/usr/bin/env bash
# ==============================================================================
# Platform UI Typecheck with bounded phase diagnostics (Issue #240)
#
# Retains node:20-alpine, read-only /src mount, and copy -> npm ci -> tsc
# semantics. Emits distinct START/COMPLETE records per phase with elapsed_ms
# and actual exit values. A failed phase prevents later phases. External
# termination without COMPLETE leaves provenance UNKNOWN (do not infer OOM
# or a signal from exit 146).
#
# Sourceable helper: platform_ui_typecheck_run_phases (POSIX-compatible body
# so node:20-alpine /bin/sh can . this file). Host execution uses bash.
# Behavioral harness: tests/test-ui-typecheck-phase-diagnostics.sh
# ==============================================================================

# Monotonic millisecond clock from /proc/uptime (host and alpine).
platform_ui_typecheck_now_ms() {
    # shellcheck disable=SC2034
    local up unused
    read -r up unused < /proc/uptime || return 1
    awk -v u="$up" 'BEGIN { printf "%d", (u * 1000) + 0 }'
}

# Run one named phase. Records START before the command and COMPLETE after
# with the actual exit. Does not reinterpret exit values.
platform_ui_typecheck_phase() {
    local name="$1"
    shift
    local start_ms end_ms elapsed_ms rc

    # Phase records on stderr: unbuffered on pipes so START is visible before
    # a long-running child (and under docker logs) without shell tracing.
    printf 'UI_TYPECHECK_PHASE_START phase=%s\n' "$name" >&2

    if ! start_ms="$(platform_ui_typecheck_now_ms)"; then
        printf 'UI_TYPECHECK_PHASE_COMPLETE phase=%s exit=1 elapsed_ms=unknown clock=unavailable\n' "$name" >&2
        return 1
    fi

    # Capture the real child exit under `set -e` without skipping COMPLETE.
    rc=0
    "$@" || rc=$?

    if end_ms="$(platform_ui_typecheck_now_ms)"; then
        elapsed_ms=$((end_ms - start_ms))
        if [ "$elapsed_ms" -lt 0 ]; then
            elapsed_ms=0
        fi
    else
        elapsed_ms=unknown
    fi

    printf 'UI_TYPECHECK_PHASE_COMPLETE phase=%s exit=%s elapsed_ms=%s\n' "$name" "$rc" "$elapsed_ms" >&2
    return "$rc"
}

# Sequential copy -> install -> typecheck. Override commands via env for
# harness injection; production defaults match the historical unit-runner seam.
platform_ui_typecheck_run_phases() {
    local src="${UI_TYPECHECK_SRC:-/src}"
    local app="${UI_TYPECHECK_APP:-/app}"
    local copy_cmd="${UI_TYPECHECK_COPY_CMD:-}"
    local install_cmd="${UI_TYPECHECK_INSTALL_CMD:-}"
    local tsc_cmd="${UI_TYPECHECK_TSC_CMD:-}"
    local rc

    if [ -z "$copy_cmd" ]; then
        copy_cmd="cp -r ${src} ${app}"
    fi
    if [ -z "$install_cmd" ]; then
        install_cmd="npm ci --silent"
    fi
    if [ -z "$tsc_cmd" ]; then
        tsc_cmd="npx tsc --noEmit"
    fi

    # SOURCE_COPY — stop before install/typecheck on failure.
    # Intentionally use sh -c so override strings remain single commands.
    # `|| rc=$?` preserves the actual exit under caller `set -e`.
    # shellcheck disable=SC2086
    rc=0
    platform_ui_typecheck_phase SOURCE_COPY sh -c "$copy_cmd" || rc=$?
    if [ "$rc" -ne 0 ]; then
        return "$rc"
    fi

    if ! cd "$app"; then
        # cd is part of the historical "cd /app && npm ci" contract after copy.
        # Surface as install-phase failure without starting typecheck.
        printf 'UI_TYPECHECK_PHASE_START phase=DEPENDENCY_INSTALL\n' >&2
        printf 'UI_TYPECHECK_PHASE_COMPLETE phase=DEPENDENCY_INSTALL exit=1 elapsed_ms=0 note=cd_app_failed\n' >&2
        return 1
    fi

    # DEPENDENCY_INSTALL
    # shellcheck disable=SC2086
    rc=0
    platform_ui_typecheck_phase DEPENDENCY_INSTALL sh -c "$install_cmd" || rc=$?
    if [ "$rc" -ne 0 ]; then
        return "$rc"
    fi

    # TYPESCRIPT_CHECK
    # shellcheck disable=SC2086
    rc=0
    platform_ui_typecheck_phase TYPESCRIPT_CHECK sh -c "$tsc_cmd" || rc=$?
    return "$rc"
}

# Containerized runner. Preserves docker/child exit. Forwards TERM/INT/HUP to
# the owned container. Does not log environment variables or credentials.
platform_ui_typecheck_docker() {
    local root_dir="$1"
    local image="${UI_TYPECHECK_IMAGE:-node:20-alpine}"
    local name="platform-ui-typecheck-$$"
    local cid=""
    local rc=0
    local logs_pid=""

    _platform_ui_typecheck_forward() {
        local sig="$1"
        if [ -n "$cid" ]; then
            docker kill -s "$sig" "$cid" >/dev/null 2>&1 || true
        fi
    }

    _platform_ui_typecheck_on_signal() {
        local sig="$1"
        _platform_ui_typecheck_forward "$sig"
        if [ -n "$cid" ]; then
            docker wait "$cid" >/dev/null 2>&1 || true
            docker rm -f "$cid" >/dev/null 2>&1 || true
        fi
        if [ -n "$logs_pid" ]; then
            kill "$logs_pid" >/dev/null 2>&1 || true
        fi
        trap - TERM INT HUP
        # Propagate as 128+signal without claiming OOM/146 meaning.
        exit $((128 + sig))
    }

    trap '_platform_ui_typecheck_on_signal 15' TERM
    trap '_platform_ui_typecheck_on_signal 2' INT
    trap '_platform_ui_typecheck_on_signal 1' HUP

    cid="$(docker run -d --name "$name" \
        -v "${root_dir}/ui:/src:ro" \
        -v "${root_dir}/tests/run-ui-typecheck.sh:/runner/run-ui-typecheck.sh:ro" \
        "$image" \
        sh -c 'set -eu; . /runner/run-ui-typecheck.sh; platform_ui_typecheck_run_phases')" || {
        rc=$?
        trap - TERM INT HUP
        return "$rc"
    }

    docker logs -f "$cid" &
    logs_pid=$!

    set +e
    rc="$(docker wait "$cid")"
    set -e

    kill "$logs_pid" >/dev/null 2>&1 || true
    wait "$logs_pid" 2>/dev/null || true
    docker rm -f "$cid" >/dev/null 2>&1 || true
    cid=""
    trap - TERM INT HUP
    return "$rc"
}

# Direct execution only under bash (alpine /bin/sh sources helpers only).
if [ -n "${BASH_VERSION:-}" ] && [ "${BASH_SOURCE[0]}" = "$0" ]; then
    set -eu
    ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
    echo "--> Running UI Typecheck (phase-diagnosed)..."
    platform_ui_typecheck_docker "${ROOT_DIR}"
    exit $?
fi
