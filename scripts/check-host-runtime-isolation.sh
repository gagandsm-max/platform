#!/usr/bin/env bash
# ==============================================================================
# Platform Host Runtime Isolation Checker
# Governing Standard: AGENTS.md, Platform_13_Engineering_Development_and_Docker_Standards.md
#
# Non-negotiable rule: All Platform language runtimes and databases (Java, Go,
# Node, npm, Python, PostgreSQL, ClickHouse, Kafka, Valkey) must run inside
# Docker Compose. No project-specific runtimes may be installed on the host.
#
# Note: Private runtimes bundled internally by desktop applications, editors,
# or agent tooling (e.g. Cursor, Antigravity, VS Code, JetBrains) are NOT
# prohibited host development runtimes unless installed on normal system/user PATH.
# ==============================================================================
set -euo pipefail

PROHIBITED_RUNTIMES=("java" "javac" "mvn" "gradle" "go" "clickhouse-server" "clickhouse-client" "valkey-server" "valkey-cli" "node" "npm" "yarn" "pnpm")

is_private_bundle() {
    local bin_path="$1"
    local real_path
    real_path=$(realpath "${bin_path}" 2>/dev/null || echo "${bin_path}")

    # Inspect path against known private application/IDE/agent bundle hierarchies
    case "${real_path}" in
        */.cursor*/*|*/cursor*/*|*/Cursor*/*) return 0 ;;
        */.vscode*/*|*/vscode*/*|*/Code*/*) return 0 ;;
        */.gemini*/*|*/antigravity*/*|*/.antigravity*/*) return 0 ;;
        */.local/share/JetBrains*/*|*/.idea*/*|*/JetBrains*/*) return 0 ;;
        */.local/share/cursor*/*) return 0 ;;
        */snap/cursor/*|*/snap/code/*) return 0 ;;
        */electron*/*|*/appimage*/*) return 0 ;;
        */.cache/*) return 0 ;;
    esac
    return 1
}

find_prohibited_runtimes() {
    local violations=()
    for runtime in "${PROHIBITED_RUNTIMES[@]}"; do
        # Find all executable instances on current PATH
        local matches
        matches=$(type -ap "${runtime}" 2>/dev/null || true)
        if [ -n "${matches}" ]; then
            while IFS= read -r match_path; do
                [ -z "${match_path}" ] && continue
                if ! is_private_bundle "${match_path}"; then
                    violations+=("${runtime}:${match_path}")
                fi
            done <<< "${matches}"
        fi
    done

    if [ ${#violations[@]} -gt 0 ]; then
        printf "%s\n" "${violations[@]}"
        return 1
    fi
    return 0
}

run_synthetic_test() {
    echo "==> Running synthetic host-runtime-isolation verification..."
    local test_dir
    test_dir=$(mktemp -d -t platform-isolation-test-XXXXXX)
    # Ensure cleanup on return
    trap 'rm -rf "${test_dir}"' RETURN

    # 1. Negative Test: Deliberately inject forbidden runtime in normal path
    mkdir -p "${test_dir}/mock_system_bin"
    touch "${test_dir}/mock_system_bin/go"
    chmod +x "${test_dir}/mock_system_bin/go"

    local detected_violation=""
    detected_violation=$(PATH="${test_dir}/mock_system_bin:${PATH}" find_prohibited_runtimes 2>/dev/null || true)

    if ! echo "${detected_violation}" | grep -q "go:${test_dir}/mock_system_bin/go"; then
        echo "[ERROR] Synthetic test FAILED: Prohibited system runtime was NOT detected!"
        return 1
    fi
    echo "[OK] Synthetic prohibited runtime correctly detected: ${detected_violation}"

    # 2. Positive Test: Inject private application bundle (e.g. mock Cursor / agent node)
    mkdir -p "${test_dir}/.cursor-server/bin"
    touch "${test_dir}/.cursor-server/bin/node"
    chmod +x "${test_dir}/.cursor-server/bin/node"

    local private_violation=""
    # Remove mock_system_bin, only test private bundle on PATH
    private_violation=$(PATH="${test_dir}/.cursor-server/bin:${PATH}" find_prohibited_runtimes 2>/dev/null || true)

    if echo "${private_violation}" | grep -q "node"; then
        echo "[ERROR] Synthetic test FAILED: Private bundle was falsely flagged as host pollution: ${private_violation}"
        return 1
    fi
    echo "[OK] Synthetic private application bundle correctly exempted."

    echo "==> Synthetic host-runtime-isolation tests PASSED."
    return 0
}

# Main entry point
if [ "${1:-}" = "--synthetic-test" ]; then
    run_synthetic_test
    exit 0
fi

# Run normal check
violations=$(find_prohibited_runtimes || true)
if [ -n "${violations}" ]; then
    echo "PROHIBITED_HOST_RUNTIMES_DETECTED:"
    echo "${violations}"
    exit 1
else
    exit 0
fi
