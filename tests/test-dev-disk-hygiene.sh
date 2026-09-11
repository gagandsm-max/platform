#!/usr/bin/env bash
# Guardrail tests for disk hygiene scripts (no destructive Docker cleanup).
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

fail() { echo "[FAIL] $*"; exit 1; }
pass() { echo "[OK] $*"; }

echo "========================================================================"
echo "Platform Dev Disk Hygiene Guardrails"
echo "========================================================================"

[[ -x scripts/dev-disk-check.sh ]] || chmod +x scripts/dev-disk-check.sh
[[ -x scripts/dev-hygiene.sh ]] || chmod +x scripts/dev-hygiene.sh

# Fail only on executable lines (not comments / echo / documentation strings).
if awk '
  /^[[:space:]]*#/ { next }
  /echo[[:space:]]/ { next }
  /Forbidden/ { next }
  /never[[:space:]]/ { next }
  /docker volume prune/ { bad=1 }
  /docker system prune --volumes/ { bad=1 }
  /docker system prune -a/ { bad=1 }
  /docker image prune -a/ { bad=1 }
  /rm -rf \/var\/lib\/docker/ { bad=1 }
  /docker volume rm platform_pg_data/ { bad=1 }
  END { exit bad ? 1 : 0 }
' scripts/dev-hygiene.sh scripts/dev-disk-check.sh; then
  pass "hygiene scripts do not execute forbidden prune/delete commands"
else
  fail "hygiene script must not execute forbidden volume/system prune commands"
fi

grep -q 'docker builder prune' scripts/dev-hygiene.sh || fail "missing builder prune"
grep -q 'until=72h' scripts/dev-hygiene.sh || fail "missing 72h age filter"
grep -q 'max-used-space 10GB' scripts/dev-hygiene.sh || fail "missing 10GB cache cap"
grep -q 'docker image prune -f' scripts/dev-hygiene.sh || fail "missing dangling image prune"
grep -q 'com.platform.lifecycle=ephemeral' scripts/dev-hygiene.sh || fail "missing ephemeral label filter"
pass "hygiene allows only safe reclaim classes"

grep -q 'CRITICAL' scripts/dev-disk-check.sh || fail "disk-check missing CRITICAL"
grep -q 'CLEANUP_REQUIRED' scripts/dev-disk-check.sh || fail "disk-check missing CLEANUP_REQUIRED"
grep -q 'WARNING' scripts/dev-disk-check.sh || fail "disk-check missing WARNING"
grep -q 'NORMAL' scripts/dev-disk-check.sh || fail "disk-check missing NORMAL"
pass "disk-check encodes 70/80/90 thresholds"

grep -q 'dev-disk-check' Makefile || fail "Makefile missing dev-disk-check"
grep -q 'dev-hygiene' Makefile || fail "Makefile missing dev-hygiene"
pass "Makefile exposes targets"

# Compose logging must remain bounded via shared anchor (no secret interpolation to disk).
CFG="$(mktemp)"
trap 'rm -f "${CFG}"' EXIT
umask 077
# Validate with empty env-file so local .env secrets are not rendered to disk.
docker compose --env-file /dev/null -f compose.yaml \
  --profile dev --profile test --profile ha --profile scale --profile security --profile dr \
  config >"${CFG}"
python3 - "$CFG" <<'PY'
import re, sys, subprocess
path = sys.argv[1]
text = open(path, encoding="utf-8").read()
svc = subprocess.check_output(
    [
        "docker", "compose", "--env-file", "/dev/null", "-f", "compose.yaml",
        "--profile", "dev", "--profile", "test", "--profile", "ha",
        "--profile", "scale", "--profile", "security", "--profile", "dr",
        "config", "--services",
    ],
    text=True,
).split()
if not re.search(r'max-size:\s*"?10m"?', text):
    print("compose config missing max-size 10m")
    sys.exit(1)
if not re.search(r'max-file:\s*"?3"?', text):
    print("compose config missing max-file 3")
    sys.exit(1)
count = len(re.findall(r'max-size:\s*"?10m"?', text))
if count < len(svc):
    print(f"bounded logging under-applied: max-size entries={count} services={len(svc)} {svc}")
    sys.exit(1)
# Render uses --env-file /dev/null so host .env secrets are not interpolated.
# Compose YAML placeholder defaults may still appear; that is repository text, not .env leakage.
print(f"compose services with bounded logging: {len(svc)}")
PY
pass "docker compose config renders with bounded logging"

# PostgresTestSupport must use tmpfs and rm -fv
grep -q 'tmpfs' core/src/test/java/com/platform/core/support/PostgresTestSupport.java \
  || fail "PostgresTestSupport missing tmpfs"
grep -q 'rm", "-fv"' core/src/test/java/com/platform/core/support/PostgresTestSupport.java \
  || fail "PostgresTestSupport missing docker rm -fv"
grep -q 'com.platform.lifecycle=ephemeral' core/src/test/java/com/platform/core/support/PostgresTestSupport.java \
  || fail "PostgresTestSupport missing ephemeral label"
pass "PostgresTestSupport ephemeral storage policy present"

[[ -f core/src/test/java/com/platform/core/support/PostgresTestSupportVolumeLifecycleTest.java ]] \
  || fail "missing volume lifecycle regression test"
pass "volume lifecycle regression test present"

# Threshold classification matrix (override only; no disk mutation).
classify_level() {
  local p="$1"
  if (( p >= 90 )); then echo CRITICAL
  elif (( p >= 80 )); then echo CLEANUP_REQUIRED
  elif (( p >= 70 )); then echo WARNING
  else echo NORMAL
  fi
}
[[ "$(classify_level 69)" == "NORMAL" ]] || fail "69 must be NORMAL"
[[ "$(classify_level 70)" == "WARNING" ]] || fail "70 must be WARNING"
[[ "$(classify_level 79)" == "WARNING" ]] || fail "79 must be WARNING"
[[ "$(classify_level 80)" == "CLEANUP_REQUIRED" ]] || fail "80 must be CLEANUP_REQUIRED"
[[ "$(classify_level 89)" == "CLEANUP_REQUIRED" ]] || fail "89 must be CLEANUP_REQUIRED"
[[ "$(classify_level 90)" == "CRITICAL" ]] || fail "90 must be CRITICAL"
out69="$(PLATFORM_DISK_PCT_OVERRIDE=69 bash scripts/dev-disk-check.sh)"
echo "${out69}" | grep -q 'level=NORMAL' || fail "disk-check 69 not NORMAL"
out70="$(PLATFORM_DISK_PCT_OVERRIDE=70 bash scripts/dev-disk-check.sh)"
echo "${out70}" | grep -q 'level=WARNING' || fail "disk-check 70 not WARNING"
set +e
out80="$(PLATFORM_DISK_PCT_OVERRIDE=80 bash scripts/dev-disk-check.sh)"; ec80=$?
set -e
echo "${out80}" | grep -q 'level=CLEANUP_REQUIRED' || fail "disk-check 80 not CLEANUP_REQUIRED"
[[ "${ec80}" -eq 1 ]] || fail "disk-check 80 exit must be 1 (got ${ec80})"
set +e
out90="$(PLATFORM_DISK_PCT_OVERRIDE=90 bash scripts/dev-disk-check.sh)"; ec90=$?
set -e
echo "${out90}" | grep -q 'level=CRITICAL' || fail "disk-check 90 not CRITICAL"
[[ "${ec90}" -eq 2 ]] || fail "disk-check 90 exit must be 2 (got ${ec90})"
pass "threshold classification <70 NORMAL, >=70 WARNING, >=80 CLEANUP_REQUIRED, >=90 CRITICAL"

# Live probe: tmpfs postgres run must not increase anonymous volume count.
if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  before="$(docker volume ls -q | grep -Ec '^[a-f0-9]{64}$' || true)"
  probe="platform-hygiene-probe-$$"
  docker run -d --rm --name "${probe}" \
    --tmpfs /var/lib/postgresql/data:rw,exec,size=64m \
    --label com.platform.lifecycle=ephemeral \
    --label com.platform.owner=test \
    -e POSTGRES_PASSWORD=probe \
    postgres:16-alpine >/dev/null
  # Brief wait so inspect sees a fully created container config.
  sleep 1
  tmpfs_json="$(docker inspect --format '{{json .HostConfig.Tmpfs}}' "${probe}")"
  docker rm -fv "${probe}" >/dev/null
  after="$(docker volume ls -q | grep -Ec '^[a-f0-9]{64}$' || true)"
  echo "${tmpfs_json}" | grep -q '/var/lib/postgresql/data' \
    || fail "probe missing HostConfig.Tmpfs for PG data: ${tmpfs_json}"
  [[ "${before}" == "${after}" ]] || fail "tmpfs probe changed anon volume count ${before} -> ${after}"
  pass "live tmpfs probe leaves anonymous volume count unchanged (${after})"

  # Hygiene volume safety matrix (dry-run; never touches historical unlabeled pool).
  suffix="$$"
  vol_keep="platform_pg_data"
  vol_unlabeled="platform-hygiene-unlabeled-${suffix}"
  vol_one_label="platform-hygiene-one-label-${suffix}"
  vol_dual_ref="platform-hygiene-dual-ref-${suffix}"
  vol_dual_ok="platform-hygiene-dual-ok-${suffix}"

  docker volume create "${vol_unlabeled}" >/dev/null
  docker volume create --label com.platform.lifecycle=ephemeral "${vol_one_label}" >/dev/null
  docker volume create \
    --label com.platform.lifecycle=ephemeral \
    --label com.platform.owner=test \
    "${vol_dual_ref}" >/dev/null
  docker volume create \
    --label com.platform.lifecycle=ephemeral \
    --label com.platform.owner=test \
    "${vol_dual_ok}" >/dev/null

  # Reference dual-labeled volume with a short-lived alpine container (stopped, still references).
  refc="platform-hygiene-refc-${suffix}"
  docker run -d --name "${refc}" -v "${vol_dual_ref}:/data" alpine:3.20 sleep 30 >/dev/null
  docker stop "${refc}" >/dev/null

  dry="$(PLATFORM_HYGIENE_DRY_RUN=1 bash scripts/dev-hygiene.sh 2>&1 || true)"
  echo "${dry}" | grep -q "DRY-RUN would remove labeled ephemeral volume: ${vol_dual_ok}" \
    || fail "dual-labeled unreferenced volume not eligible: ${vol_dual_ok}"
  echo "${dry}" | grep -q "SKIP referenced volume: ${vol_dual_ref}" \
    || fail "referenced ephemeral volume must be skipped"
  if echo "${dry}" | grep -Fq "${vol_unlabeled}"; then
    fail "unlabeled volume must never be hygiene candidate"
  fi
  if echo "${dry}" | grep -Fq "${vol_one_label}"; then
    fail "one-label-only volume must never be hygiene candidate"
  fi
  if echo "${dry}" | grep -Fq "REMOVE labeled ephemeral volume: ${vol_keep}"; then
    fail "named Platform volume must never be removed"
  fi
  if echo "${dry}" | grep -Fq "DRY-RUN would remove labeled ephemeral volume: ${vol_keep}"; then
    fail "named Platform volume must never be dry-removed"
  fi
  # Persistent named volumes must remain if present (dev laptop with stack).
  if docker volume inspect "${vol_keep}" >/dev/null 2>&1; then
    docker volume inspect "${vol_keep}" >/dev/null || fail "platform_pg_data missing after dry-run hygiene"
  else
    echo "[INFO] ${vol_keep} not present in this environment (acceptable for CI without Compose stack)"
  fi
  docker volume inspect "${vol_unlabeled}" >/dev/null || fail "unlabeled test volume was deleted"
  docker volume inspect "${vol_one_label}" >/dev/null || fail "one-label volume was deleted"
  docker volume inspect "${vol_dual_ref}" >/dev/null || fail "referenced dual-label volume was deleted"
  pass "hygiene refuses named/unlabeled/one-label/referenced volumes; dual-labeled unreferenced is eligible"

  docker rm -fv "${refc}" >/dev/null 2>&1 || true
  docker volume rm -f "${vol_unlabeled}" "${vol_one_label}" "${vol_dual_ref}" "${vol_dual_ok}" >/dev/null 2>&1 || true
else
  echo "[SKIP] docker unavailable for live tmpfs/hygiene volume probes"
fi

echo "========================================================================"
echo "Dev disk hygiene guardrails PASSED"
echo "========================================================================"