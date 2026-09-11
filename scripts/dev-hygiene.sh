#!/usr/bin/env bash
# Platform safe development hygiene.
# Allowed: aged/capped BuildKit cache, dangling images, labeled ephemeral unused volumes.
# Forbidden: volume prune, system prune --volumes, image prune -a, named Platform volumes.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

KEEP_VOLUMES=(
  platform_pg_data
  platform_kafka_data
  platform_ch_data
  platform_valkey_data
  platform_sensor_net_spool
  platform_sensor_infra_spool
  platform_postgres_ha_spike_platform_ha_pg_a_data
  platform_postgres_ha_spike_platform_ha_pg_b_data
  platform_go_pkg_cache
  nms-gopkg
)

echo "========================================================================"
echo "Platform Development Safe Hygiene"
echo "========================================================================"
echo "Policy: never docker volume prune / system prune --volumes / image prune -a"
DRY_RUN="${PLATFORM_HYGIENE_DRY_RUN:-0}"
if [[ "${DRY_RUN}" == "1" ]]; then
  echo "MODE: DRY-RUN (no cache/image/volume mutations)"
fi
echo

if [[ "${DRY_RUN}" != "1" ]]; then
  echo "==> BuildKit cache: drop entries older than 72h (if any)"
  docker builder prune -f --filter until=72h

  echo "==> BuildKit cache: cap footprint (~10GB max-used, keep reserved headroom)"
  # Docker 29.7.2 supports --max-used-space / --reserved-space on buildx prune.
  docker builder prune -f --max-used-space 10GB --reserved-space 2GB

  echo "==> Dangling images only"
  docker image prune -f
else
  echo "==> Skipping builder/image prune in DRY-RUN"
fi

echo "==> Labeled ephemeral test volumes that are unreferenced"
# Only volumes explicitly marked disposable by Platform tests.
# ALL of the following are required before any volume rm:
#   1) label com.platform.lifecycle=ephemeral
#   2) label com.platform.owner=test
#   3) zero container references (running or stopped)
#   4) not on persistent Platform keep list
#   5) exact label re-validation via docker volume inspect
# Historical unlabeled anonymous volumes are never candidates.
mapfile -t candidates < <(docker volume ls -q --filter "label=com.platform.lifecycle=ephemeral" --filter "label=com.platform.owner=test" || true)
removed=0
skipped=0
for vol in "${candidates[@]:-}"; do
  [[ -z "${vol}" ]] && continue
  keep=0
  for k in "${KEEP_VOLUMES[@]}"; do
    if [[ "${vol}" == "${k}" ]]; then
      keep=1
      break
    fi
  done
  if (( keep == 1 )); then
    echo "SKIP keep-list volume: ${vol}"
    skipped=$((skipped + 1))
    continue
  fi
  # Refuse if any container (running or stopped) still references the volume.
  if docker ps -a --filter "volume=${vol}" --format '{{.ID}}' | grep -q .; then
    echo "SKIP referenced volume: ${vol}"
    skipped=$((skipped + 1))
    continue
  fi
  # Exact label re-validation (defense in depth beyond filter listing).
  lifecycle="$(docker volume inspect -f '{{index .Labels "com.platform.lifecycle"}}' "${vol}" 2>/dev/null || true)"
  owner="$(docker volume inspect -f '{{index .Labels "com.platform.owner"}}' "${vol}" 2>/dev/null || true)"
  if [[ "${lifecycle}" != "ephemeral" || "${owner}" != "test" ]]; then
    echo "SKIP labels incomplete: ${vol} lifecycle=${lifecycle:-} owner=${owner:-}"
    skipped=$((skipped + 1))
    continue
  fi
  if [[ "${DRY_RUN}" == "1" ]]; then
    echo "DRY-RUN would remove labeled ephemeral volume: ${vol}"
    removed=$((removed + 1))
    continue
  fi
  echo "REMOVE labeled ephemeral volume: ${vol}"
  docker volume rm "${vol}"
  removed=$((removed + 1))
done
echo "Labeled ephemeral volumes removed=${removed} skipped=${skipped}"

if [[ "${DRY_RUN}" != "1" ]]; then
  echo "==> Platform-generated temp under /tmp/platform-* (empty dirs only, best-effort)"
  # Never recurse-delete contents; only remove empty stale marker dirs if present.
  shopt -s nullglob
  for d in /tmp/platform-*; do
    if [[ -d "${d}" ]] && [[ -z "$(ls -A "${d}" 2>/dev/null || true)" ]]; then
      rmdir "${d}" 2>/dev/null || true
    fi
  done
fi

echo
echo "Hygiene complete. Re-check with: make dev-disk-check"
if [[ "${DRY_RUN}" != "1" ]]; then
  docker system df
fi
