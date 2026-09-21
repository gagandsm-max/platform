#!/usr/bin/env bash
# Run ClickHouse forward-only migrations in order.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "${SCRIPT_DIR}/V002__metric_numeric_replay_identity.sh"
bash "${SCRIPT_DIR}/V003__metric_counter_exact_uint64.sh"
