#!/usr/bin/env bash
# Platform development disk diagnostic (read-only).
# Does not delete volumes, containers, images, or source.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

pct="$(df -P / | awk 'NR==2 {gsub(/%/,"",$5); print $5}')"
# Test override only — never set in production hygiene automation.
if [[ -n "${PLATFORM_DISK_PCT_OVERRIDE:-}" ]]; then
  pct="${PLATFORM_DISK_PCT_OVERRIDE}"
fi
used="$(df -h / | awk 'NR==2 {print $3}')"
avail="$(df -h / | awk 'NR==2 {print $4}')"
size="$(df -h / | awk 'NR==2 {print $2}')"

level="NORMAL"
if (( pct >= 90 )); then
  level="CRITICAL"
elif (( pct >= 80 )); then
  level="CLEANUP_REQUIRED"
elif (( pct >= 70 )); then
  level="WARNING"
fi

echo "========================================================================"
echo "Platform Development Disk Check"
echo "========================================================================"
echo "Filesystem / : size=${size} used=${used} avail=${avail} use=${pct}% level=${level}"
echo
echo "--- docker system df ---"
docker system df
echo
echo "--- build cache (docker builder du) ---"
docker builder du 2>/dev/null || docker buildx du 2>/dev/null || echo "(builder du unavailable)"
echo
echo "--- counts ---"
images="$(docker images -q | wc -l | tr -d ' ')"
containers="$(docker ps -aq | wc -l | tr -d ' ')"
running="$(docker ps -q | wc -l | tr -d ' ')"
volumes="$(docker volume ls -q | wc -l | tr -d ' ')"
anon="$(docker volume ls -q | grep -Ec '^[a-f0-9]{64}$' || true)"
echo "images=${images} containers_total=${containers} containers_running=${running}"
echo "volumes_total=${volumes} volumes_anonymous=${anon}"
echo
echo "--- named Platform volumes (keep) ---"
for v in platform_pg_data platform_kafka_data platform_ch_data platform_valkey_data \
         platform_sensor_net_spool platform_sensor_infra_spool; do
  if docker volume inspect "$v" >/dev/null 2>&1; then
    echo "KEEP  ${v}"
  else
    echo "MISSING  ${v}"
  fi
done
echo
echo "--- threshold policy ---"
echo "<70% NORMAL | >=70% WARNING | >=80% CLEANUP REQUIRED | >=90% CRITICAL"
echo "Safe reclaim: make dev-hygiene  (build cache + dangling images only)"
echo "Forbidden: docker volume prune | docker system prune --volumes | docker image prune -a"
echo "Destructive named-volume reset: make dev-reset CONFIRM=yes"
echo

case "${level}" in
  CRITICAL)
    echo "CRITICAL: do not start scale/HA workloads until space is reclaimed."
    echo "Run: make dev-hygiene   then re-check. Volume deletion requires separate approval."
    exit 2
    ;;
  CLEANUP_REQUIRED)
    echo "CLEANUP REQUIRED: run make dev-hygiene, then make dev-disk-check."
    exit 1
    ;;
  WARNING)
    echo "WARNING: investigate growth; prefer make dev-hygiene before large builds/tests."
    exit 0
    ;;
  *)
    echo "NORMAL: disk pressure within development guardrails."
    exit 0
    ;;
esac
