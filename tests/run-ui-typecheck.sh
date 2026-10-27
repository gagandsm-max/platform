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

# Finite soft-signal cancellation budget (seconds) under a responsive Docker
# daemon. After the soft signal, cleanup force-removes the owned container if
# it has not exited. Daemon-unreachable cleanup remains uncertain.
# UI_TYPECHECK_CANCEL_BUDGET_SEC may override for harness injection.
platform_ui_typecheck_cancel_budget_sec() {
    local v="${UI_TYPECHECK_CANCEL_BUDGET_SEC:-8}"
    case "$v" in
        ''|*[!0-9]*) echo 8 ;;
        *) echo "$v" ;;
    esac
}

# Bound how long we wait for docker logs -f to drain after container exit
# before force-stopping the owned logger (does not inflate UI workload timeout).
platform_ui_typecheck_log_drain_budget_sec() {
    local v="${UI_TYPECHECK_LOG_DRAIN_BUDGET_SEC:-5}"
    case "$v" in
        ''|*[!0-9]*) echo 5 ;;
        *) echo "$v" ;;
    esac
}

# Containerized runner. Preserves docker/child exit. Forwards TERM/INT/HUP to
# the owned container with a finite cancellation budget, drains terminal logs
# before deleting the container, and distinguishes Docker CLI/transport failure
# from a validated container exit status. Does not log env/credentials.
platform_ui_typecheck_docker() {
    local root_dir="$1"
    local image="${UI_TYPECHECK_IMAGE:-node:20-alpine}"
    local name="platform-ui-typecheck-$$"
    local cid=""
    local rc=0
    local logs_pid=""
    local wait_pid=""
    local wait_file=""
    local wait_err=""
    local cancel_budget
    local drain_budget
    local remote_cmd
    local docker_wait_rc=0
    local status_text=""
    local retain_container=0
    local log_file=""
    local log_offset=0

    cancel_budget="$(platform_ui_typecheck_cancel_budget_sec)"
    drain_budget="$(platform_ui_typecheck_log_drain_budget_sec)"
    # Production default unchanged; harness may inject a PID1 that ignores soft signals.
    remote_cmd="${UI_TYPECHECK_REMOTE_CMD:-set -eu; . /runner/run-ui-typecheck.sh; platform_ui_typecheck_run_phases}"

    # Emit newly appended follower bytes to stderr (near-live; avoids holding
    # command-substitution pipes open via a background writer FD).
    _platform_ui_typecheck_emit_new_logs() {
        local size
        if [ -z "$log_file" ] || [ ! -f "$log_file" ]; then
            return 0
        fi
        size="$(wc -c <"$log_file" 2>/dev/null | tr -d ' ')"
        case "$size" in
            ''|*[!0-9]*) return 0 ;;
        esac
        if [ "$size" -gt "$log_offset" ]; then
            tail -c +"$((log_offset + 1))" "$log_file" >&2 || true
            log_offset="$size"
        fi
    }

    # Finalize terminal logs before deleting the only log source (F02).
    # Live-follow drain is bounded; on timeout or logger failure, attempt a
    # non-follow snapshot while the container still exists. If that also fails,
    # retain the container and report incomplete collection explicitly.
    # Does not change workload/transport exit classification.
    _platform_ui_typecheck_finalize_logs() {
        local budget="$1"
        local i=0
        local logs_rc=0
        local force_stopped=0
        local snap_rc=0
        local reason="logger_failed"

        if [ -n "$logs_pid" ]; then
            # docker logs -f should exit after the container stops; poll for drain
            # before killing so COMPLETE/error records are not truncated.
            while kill -0 "$logs_pid" >/dev/null 2>&1; do
                _platform_ui_typecheck_emit_new_logs
                if [ "$i" -ge "$budget" ]; then
                    # Soft stop first; escalate so wait cannot hang on an
                    # uninterruptible follower (keeps drain budget meaningful).
                    kill "$logs_pid" >/dev/null 2>&1 || true
                    sleep 1
                    if kill -0 "$logs_pid" >/dev/null 2>&1; then
                        kill -9 "$logs_pid" >/dev/null 2>&1 || true
                    fi
                    force_stopped=1
                    break
                fi
                sleep 1
                i=$((i + 1))
            done
            set +e
            wait "$logs_pid"
            logs_rc=$?
            set -e
            logs_pid=""
            _platform_ui_typecheck_emit_new_logs
        fi

        if [ "$force_stopped" -eq 0 ] && [ "$logs_rc" -eq 0 ]; then
            return 0
        fi

        if [ "$force_stopped" -eq 1 ]; then
            reason="drain_budget_exceeded"
        else
            reason="logger_exit_${logs_rc}"
        fi

        if [ -z "$cid" ]; then
            printf 'UI_TYPECHECK_LOGS_INCOMPLETE container=none retained=0 reason=%s\n' "$reason" >&2
            return 0
        fi

        # Recover delayed/truncated tail while the log source still exists.
        # Append into the evidence file then emit so capture sees COMPLETE.
        set +e
        docker logs "$cid" >>"$log_file" 2>&1
        snap_rc=$?
        set -e
        _platform_ui_typecheck_emit_new_logs
        if [ "$snap_rc" -eq 0 ]; then
            printf 'UI_TYPECHECK_LOGS_RECOVERED container=%s via=docker_logs_snapshot reason=%s\n' "$cid" "$reason" >&2
            return 0
        fi

        printf 'UI_TYPECHECK_LOGS_INCOMPLETE container=%s retained=1 reason=final_retrieval_failed prior=%s\n' "$cid" "$reason" >&2
        retain_container=1
        return 1
    }

    _platform_ui_typecheck_force_rm() {
        # Best-effort owned-container removal. If the daemon is unreachable,
        # cleanup success is uncertain — report but do not hang forever.
        if [ -n "$cid" ]; then
            if ! docker rm -f "$cid" >/dev/null 2>&1; then
                printf 'UI_TYPECHECK_CLEANUP_UNCERTAIN container=%s reason=docker_rm_failed\n' "$cid" >&2
            fi
        fi
        cid=""
    }

    _platform_ui_typecheck_cleanup_owned() {
        # Never delete the log source until finalization completes or explicit
        # retain-for-evidence is recorded.
        _platform_ui_typecheck_finalize_logs "$drain_budget" || true
        if [ "$retain_container" -eq 0 ]; then
            _platform_ui_typecheck_force_rm
        elif [ -n "$cid" ]; then
            printf 'UI_TYPECHECK_CLEANUP_RETAINED container=%s reason=incomplete_logs\n' "$cid" >&2
        fi
        if [ -n "$log_file" ]; then
            rm -f "$log_file" >/dev/null 2>&1 || true
            log_file=""
        fi
    }

    _platform_ui_typecheck_bounded_cancel() {
        local sig="$1"
        local i=0
        if [ -n "$cid" ]; then
            docker kill -s "$sig" "$cid" >/dev/null 2>&1 || true
            # Finite soft-signal wait; do not unbounded docker wait (F01).
            while [ "$i" -lt "$cancel_budget" ]; do
                _platform_ui_typecheck_emit_new_logs
                case "$(docker inspect -f '{{.State.Running}}' "$cid" 2>/dev/null || echo false)" in
                    true|True) ;;
                    *) break ;;
                esac
                sleep 1
                i=$((i + 1))
            done
        fi
        if [ -n "$wait_pid" ]; then
            kill "$wait_pid" >/dev/null 2>&1 || true
            wait "$wait_pid" >/dev/null 2>&1 || true
            wait_pid=""
        fi
        # Same evidence rule as the normal path: finalize before delete (F02).
        _platform_ui_typecheck_cleanup_owned
    }

    _platform_ui_typecheck_on_signal() {
        local sig="$1"
        trap - TERM INT HUP
        _platform_ui_typecheck_bounded_cancel "$sig"
        if [ -n "$wait_file" ]; then
            rm -f "$wait_file" "$wait_err" >/dev/null 2>&1 || true
        fi
        if [ -n "$log_file" ]; then
            rm -f "$log_file" >/dev/null 2>&1 || true
            log_file=""
        fi
        # Propagate as 128+signal without claiming OOM/146 meaning.
        exit $((128 + sig))
    }

    trap '_platform_ui_typecheck_on_signal 15' TERM
    trap '_platform_ui_typecheck_on_signal 2' INT
    trap '_platform_ui_typecheck_on_signal 1' HUP

    # Pass through optional phase-command overrides when the harness sets them.
    # Production leaves them unset so container defaults apply.
    cid="$(docker run -d --name "$name" \
        -v "${root_dir}/ui:/src:ro" \
        -v "${root_dir}/tests/run-ui-typecheck.sh:/runner/run-ui-typecheck.sh:ro" \
        -e UI_TYPECHECK_COPY_CMD \
        -e UI_TYPECHECK_INSTALL_CMD \
        -e UI_TYPECHECK_TSC_CMD \
        -e UI_TYPECHECK_SRC \
        -e UI_TYPECHECK_APP \
        "$image" \
        sh -c "$remote_cmd")" || {
        rc=$?
        trap - TERM INT HUP
        return "$rc"
    }

    wait_file="$(mktemp "${TMPDIR:-/tmp}/ui-tc-wait.XXXXXX")"
    wait_err="$(mktemp "${TMPDIR:-/tmp}/ui-tc-wait-err.XXXXXX")"
    log_file="$(mktemp "${TMPDIR:-/tmp}/ui-tc-logs.XXXXXX")"
    log_offset=0

    # Follow into a private file so a delayed follower cannot hold command-
    # substitution pipes open after kill (F02 bounded drain). Emit near-live.
    docker logs -f "$cid" >"$log_file" 2>&1 &
    logs_pid=$!

    # Background docker wait so Bash can deliver TERM/INT/HUP to this shell
    # promptly (foreground command-substitution deferred traps — F01).
    # Capture CLI status separately from stdout exit text (F03).
    # Poll with interruptible sleep rather than blocking `wait` so traps run
    # under bash builds that defer traps until an explicit wait completes.
    (
        docker wait "$cid" >"$wait_file" 2>"$wait_err"
        exit $?
    ) &
    wait_pid=$!

    set +e
    while kill -0 "$wait_pid" >/dev/null 2>&1; do
        _platform_ui_typecheck_emit_new_logs
        sleep 1
    done
    wait "$wait_pid"
    docker_wait_rc=$?
    set -e
    wait_pid=""
    _platform_ui_typecheck_emit_new_logs

    # If wait was interrupted by our trap, the trap already exited.
    # Distinguish Docker CLI/transport failure from container exit value.
    if [ "$docker_wait_rc" -ne 0 ]; then
        printf 'UI_TYPECHECK_DOCKER_WAIT_FAILED exit=%s\n' "$docker_wait_rc" >&2
        if [ -s "$wait_err" ]; then
            cat "$wait_err" >&2 || true
        fi
        rc="$docker_wait_rc"
    else
        status_text="$(tr -d ' \t\r\n' <"$wait_file" 2>/dev/null || true)"
        case "$status_text" in
            ''|*[!0-9]*)
                printf 'UI_TYPECHECK_DOCKER_WAIT_INVALID status=%s\n' "${status_text:-empty}" >&2
                if [ -s "$wait_err" ]; then
                    cat "$wait_err" >&2 || true
                fi
                # No validated container exit — surface as failure, not a Bash
                # numeric-return error from an empty value.
                rc=1
                ;;
            *)
                rc="$status_text"
                ;;
        esac
    fi

    rm -f "$wait_file" "$wait_err" >/dev/null 2>&1 || true
    wait_file=""
    wait_err=""

    # Retain terminal diagnostics through the final COMPLETE/error record before
    # deleting the log source container (F02). Workload/transport rc unchanged.
    _platform_ui_typecheck_cleanup_owned
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
