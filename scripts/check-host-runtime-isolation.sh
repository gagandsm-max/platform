#!/usr/bin/env bash
# ==============================================================================
# Platform Host Runtime Isolation Checker
# Governing Standard: AGENTS.md, Platform_13_Engineering_Development_and_Docker_Standards.md
#
# Non-negotiable: Platform project-specific language/DB runtimes must run in
# Docker Compose — not as host development prerequisites on PATH.
#
# Private IDE/agent application bundles may contain embedded runtimes; those
# are exempted only when the resolved path matches known private-bundle roots.
# Broad exemptions such as */.cache/* or */Code*/* are intentionally NOT used.
# ==============================================================================
set -euo pipefail

PROHIBITED_RUNTIMES=(
    java javac mvn gradle
    go
    python python3
    node npm yarn pnpm
    psql postgres
    clickhouse-server clickhouse-client
    valkey-server valkey-cli
    kafka-topics kafka-topics.sh kafka-console-consumer kafka-console-producer
    kafka-server-start kafka-server-start.sh
)

# Distro OS utilities in /usr/bin and /bin (python3, psql) are allowed so ordinary
# OS packages do not false-positive. Project-specific installs under /usr/local,
# pyenv, nvm, conda, etc. remain prohibited. Platform development must still use
# Docker Compose for project runtimes.
is_distro_os_utility() {
    local runtime="$1"
    local real_path="$2"
    case "${runtime}" in
        python|python3|psql|postgres)
            case "${real_path}" in
                /usr/bin/*|/bin/*|/usr/lib/*/bin/*) return 0 ;;
            esac
            ;;
    esac
    return 1
}

is_private_bundle() {
    local bin_path="$1"
    local real_path
    real_path=$(realpath -m "${bin_path}" 2>/dev/null || echo "${bin_path}")

    # Constrained application-private roots only (no generic */.cache/*, */Code*/*, */cursor*/*).
    case "${real_path}" in
        */.cursor/*|*/.cursor-server/*) return 0 ;;
        */.local/share/cursor-agent/*|*/.local/share/cursor/*) return 0 ;;
        */snap/cursor/*) return 0 ;;
        */.vscode-server/*|*/.vscode-oss-server/*) return 0 ;;
        */.vscode/extensions/*) return 0 ;;
        */.antigravity/*|*/.local/share/antigravity/*) return 0 ;;
        */.gemini/antigravity/*) return 0 ;;
        */.local/share/JetBrains/*/plugins/*|*/.cache/JetBrains/*/plugins/*) return 0 ;;
        */AppData/Local/Programs/cursor/*|*/AppData/Local/Programs/Cursor/*) return 0 ;;
        */Applications/Cursor.app/*) return 0 ;;
    esac
    return 1
}

find_prohibited_runtimes() {
    local violations=()
    local runtime matches match_path real_path
    for runtime in "${PROHIBITED_RUNTIMES[@]}"; do
        matches=$(type -ap "${runtime}" 2>/dev/null || true)
        if [ -n "${matches}" ]; then
            while IFS= read -r match_path; do
                [ -z "${match_path}" ] && continue
                real_path=$(realpath -m "${match_path}" 2>/dev/null || echo "${match_path}")
                if is_private_bundle "${match_path}"; then
                    continue
                fi
                if is_distro_os_utility "${runtime}" "${real_path}"; then
                    continue
                fi
                violations+=("${runtime}:${match_path}")
            done <<< "${matches}"
        fi
    done

    if [ ${#violations[@]} -gt 0 ]; then
        printf "%s\n" "${violations[@]}"
        return 1
    fi
    return 0
}

assert_detects() {
    local label="$1"
    local expected_substr="$2"
    local output="$3"
    if ! echo "${output}" | grep -Fq "${expected_substr}"; then
        echo "[ERROR] ${label}: expected to detect '${expected_substr}', got: ${output:-<empty>}"
        return 1
    fi
    echo "[OK] ${label}: detected ${expected_substr}"
}

assert_not_detects() {
    local label="$1"
    local forbidden_substr="$2"
    local output="$3"
    if echo "${output}" | grep -Fq "${forbidden_substr}"; then
        echo "[ERROR] ${label}: falsely detected '${forbidden_substr}': ${output}"
        return 1
    fi
    echo "[OK] ${label}: correctly exempted ${forbidden_substr}"
}

run_synthetic_test() {
    echo "==> Running synthetic host-runtime-isolation verification..."
    local test_dir
    test_dir=$(mktemp -d -t platform-isolation-test-XXXXXX)
    trap 'rm -rf "${test_dir}"' RETURN

    local out

    # 1. System/user Go runtime detected
    mkdir -p "${test_dir}/usr/local/bin"
    touch "${test_dir}/usr/local/bin/go" && chmod +x "${test_dir}/usr/local/bin/go"
    out=$(PATH="${test_dir}/usr/local/bin:${PATH}" find_prohibited_runtimes 2>/dev/null || true)
    assert_detects "system Go" "go:${test_dir}/usr/local/bin/go" "${out}"

    # 2. System/user Node runtime detected
    touch "${test_dir}/usr/local/bin/node" && chmod +x "${test_dir}/usr/local/bin/node"
    out=$(PATH="${test_dir}/usr/local/bin:${PATH}" find_prohibited_runtimes 2>/dev/null || true)
    assert_detects "system Node" "node:${test_dir}/usr/local/bin/node" "${out}"

    # 3. Project/user Python runtime detected
    touch "${test_dir}/usr/local/bin/python3" && chmod +x "${test_dir}/usr/local/bin/python3"
    out=$(PATH="${test_dir}/usr/local/bin:${PATH}" find_prohibited_runtimes 2>/dev/null || true)
    assert_detects "user Python3" "python3:${test_dir}/usr/local/bin/python3" "${out}"

    # 4. Generic ~/.cache/custom-runtime/bin/node NOT exempted
    mkdir -p "${test_dir}/.cache/custom-runtime/bin"
    touch "${test_dir}/.cache/custom-runtime/bin/node" && chmod +x "${test_dir}/.cache/custom-runtime/bin/node"
    out=$(PATH="${test_dir}/.cache/custom-runtime/bin:${PATH}" find_prohibited_runtimes 2>/dev/null || true)
    assert_detects "generic cache node" "node:${test_dir}/.cache/custom-runtime/bin/node" "${out}"

    # 5. Project path containing /Code/ NOT exempted
    mkdir -p "${test_dir}/home/dev/Code/tools/bin"
    touch "${test_dir}/home/dev/Code/tools/bin/go" && chmod +x "${test_dir}/home/dev/Code/tools/bin/go"
    out=$(PATH="${test_dir}/home/dev/Code/tools/bin:${PATH}" find_prohibited_runtimes 2>/dev/null || true)
    assert_detects "Code project path Go" "go:${test_dir}/home/dev/Code/tools/bin/go" "${out}"

    # 6. Mock Cursor private bundle exempted
    mkdir -p "${test_dir}/.local/share/cursor-agent/versions/test/bin"
    touch "${test_dir}/.local/share/cursor-agent/versions/test/bin/node" && chmod +x "${test_dir}/.local/share/cursor-agent/versions/test/bin/node"
    out=$(PATH="${test_dir}/.local/share/cursor-agent/versions/test/bin:${PATH}" find_prohibited_runtimes 2>/dev/null || true)
    assert_not_detects "Cursor agent bundle" "node:${test_dir}/.local/share/cursor-agent/versions/test/bin/node" "${out}"

    # 7. Mock Antigravity private bundle exempted
    mkdir -p "${test_dir}/.antigravity/bin"
    touch "${test_dir}/.antigravity/bin/node" && chmod +x "${test_dir}/.antigravity/bin/node"
    out=$(PATH="${test_dir}/.antigravity/bin:${PATH}" find_prohibited_runtimes 2>/dev/null || true)
    assert_not_detects "Antigravity bundle" "node:${test_dir}/.antigravity/bin/node" "${out}"

    # Also cover psql / kafka CLI detection
    touch "${test_dir}/usr/local/bin/psql" "${test_dir}/usr/local/bin/kafka-topics" && chmod +x "${test_dir}/usr/local/bin/psql" "${test_dir}/usr/local/bin/kafka-topics"
    out=$(PATH="${test_dir}/usr/local/bin:${PATH}" find_prohibited_runtimes 2>/dev/null || true)
    assert_detects "psql" "psql:${test_dir}/usr/local/bin/psql" "${out}"
    assert_detects "kafka-topics" "kafka-topics:${test_dir}/usr/local/bin/kafka-topics" "${out}"

    echo "==> Synthetic host-runtime-isolation tests PASSED."
    return 0
}

if [ "${1:-}" = "--synthetic-test" ]; then
    run_synthetic_test
    exit 0
fi

violations=$(find_prohibited_runtimes || true)
if [ -n "${violations}" ]; then
    echo "PROHIBITED_HOST_RUNTIMES_DETECTED:"
    echo "${violations}"
    exit 1
else
    exit 0
fi
