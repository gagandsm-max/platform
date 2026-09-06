#!/usr/bin/env bash
# ==============================================================================
# Platform Control CLI Wrapper
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${ROOT_DIR}"

COMMAND="${1:-help}"
shift || true

case "${COMMAND}" in
    help)
        make help
        ;;
    doctor)
        make doctor
        ;;
    certs)
        make certs
        ;;
    dev-build)
        make dev-build "$@"
        ;;
    dev-up)
        make dev-up "$@"
        ;;
    dev-down)
        make dev-down "$@"
        ;;
    dev-restart)
        make dev-restart "$@"
        ;;
    dev-status)
        make dev-status "$@"
        ;;
    dev-logs)
        make dev-logs "$@"
        ;;
    test-unit)
        make test-unit "$@"
        ;;
    test-integration)
        make test-integration "$@"
        ;;
    test)
        make test "$@"
        ;;
    clean)
        make clean "$@"
        ;;
    dev-reset)
        make dev-reset "$@"
        ;;
    *)
        echo "Unknown command: ${COMMAND}"
        echo "Run './scripts/platform.sh help' for available commands."
        exit 1
        ;;
esac
