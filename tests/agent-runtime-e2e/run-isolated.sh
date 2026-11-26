#!/usr/bin/env bash
# Invoke only after Issue #211 runtime admission.
set -euo pipefail
exec python3 "$(dirname "${BASH_SOURCE[0]}")/isolation.py" launch
