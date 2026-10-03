#!/usr/bin/env bash
# BUILD-005 whole-build supportability stress/soak gate.
# CERTIFICATION-FIRST. Exit non-zero on hard failure.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"

log() { echo "==> $*"; }
fail_hard() { echo "[HARD_FAIL] $*"; STRESS_PASS=0; }

# Do not source .env (may contain local secrets). Prefer non-secret port keys only.
resolve_core_host_port() {
  local published
  # Prefer the live published mapping when Core is already running (dev or CI ports).
  published="$(docker port platform-core 8443 2>/dev/null | head -n1 | awk -F: '{print $NF}' | tr -d '\r' || true)"
  if [[ "${published}" =~ ^[0-9]+$ ]]; then
    echo "${published}"
    return 0
  fi
  if [ -n "${PLATFORM_CORE_HOST_PORT:-}" ]; then
    echo "${PLATFORM_CORE_HOST_PORT}"
    return 0
  fi
  # Read only the port assignment line from .env / .env.example — never source the file.
  local from_file=""
  if [ -f .env ]; then
    from_file="$(grep -E '^PLATFORM_CORE_HOST_PORT=' .env | head -n1 | cut -d= -f2- | tr -d '"' || true)"
  fi
  if [ -z "${from_file}" ] && [ -f .env.example ]; then
    from_file="$(grep -E '^PLATFORM_CORE_HOST_PORT=' .env.example | head -n1 | cut -d= -f2- | tr -d '"' || true)"
  fi
  echo "${from_file:-28443}"
}

GO_IMG="${PLATFORM_STRESS_GO_IMAGE:-golang:1.27.1-alpine3.24}"
GO_TMPFS="/tmp:rw,exec,size=2147483648,mode=1777"
CORE_PORT="$(resolve_core_host_port)"
log "Resolved CORE_PORT=${CORE_PORT} (no .env source)"

AGENT_EVENTS=0
SENSOR_EVENTS=0
ANALYTICS_CASES=0
PROBE_REQUESTS=0
CORR_OPS=0
MALFORMED=0
DUP_KEYS=0
ID_MUT=0
LEAKS=0
NONFINITE=0
PROBE_FAIL=0
PROBE_SOAK_OK=0
CORR_FAIL=0
PANIC_DEADLOCK=0
RACE_STATUS="RACE_TOOL_UNAVAILABLE"
MEM_AGENT=""
MEM_SENSOR=""
MEM_COMPOSE=""
MEM_ASSESSMENT="UNMEASURED"
MEM_CORE_LINE=""
MEM_SENSOR_LINE=""
MEM_ANALYTICS_LINE=""
ANALYTICS_RESTART_LINE=""
STRESS_PASS=1
DURATION_START=$(date +%s)

run_go_stress() {
  local module_path="$1"
  local label="$2"
  local pkg="$3"
  log "Running ${label} stress tests (-tags=stress -run TestStress)..."
  local out
  out=$(docker run --rm --tmpfs "${GO_TMPFS}" \
    -v "${ROOT_DIR}/${module_path}:/app/mod:ro" \
    -w /app/mod \
    "${GO_IMG}" \
    go test -v -tags=stress -count=1 -timeout=20m -run 'TestStress' "${pkg}" 2>&1) || {
      echo "${out}"
      fail_hard "${label} stress tests failed"
      echo "${out}" | tee "/tmp/build005-${label}-stress.txt" >/dev/null
      return 0
    }
  echo "${out}" | tee "/tmp/build005-${label}-stress.txt"
  if echo "${out}" | grep -q 'AGENT_STRESS events='; then
    AGENT_EVENTS=$(echo "${out}" | sed -n 's/.*AGENT_STRESS events=\([0-9]*\).*/\1/p' | tail -n1)
    local line
    line=$(echo "${out}" | grep 'AGENT_STRESS events=' | tail -n1)
    MALFORMED=$((MALFORMED + $(echo "${line}" | sed -n 's/.*malformed=\([0-9]*\).*/\1/p')))
    DUP_KEYS=$((DUP_KEYS + $(echo "${line}" | sed -n 's/.*dup=\([0-9]*\).*/\1/p')))
    ID_MUT=$((ID_MUT + $(echo "${line}" | sed -n 's/.*id_mut=\([0-9]*\).*/\1/p')))
    LEAKS=$((LEAKS + $(echo "${line}" | sed -n 's/.*leaks=\([0-9]*\).*/\1/p')))
    NONFINITE=$((NONFINITE + $(echo "${line}" | sed -n 's/.*nonfinite=\([0-9]*\).*/\1/p')))
    PANIC_DEADLOCK=$((PANIC_DEADLOCK + $(echo "${line}" | sed -n 's/.*panics=\([0-9]*\).*/\1/p')))
  fi
  if echo "${out}" | grep -q 'SENSOR_STRESS events='; then
    SENSOR_EVENTS=$(echo "${out}" | sed -n 's/.*SENSOR_STRESS events=\([0-9]*\).*/\1/p' | tail -n1)
    line=$(echo "${out}" | grep 'SENSOR_STRESS events=' | tail -n1)
    MALFORMED=$((MALFORMED + $(echo "${line}" | sed -n 's/.*malformed=\([0-9]*\).*/\1/p')))
    DUP_KEYS=$((DUP_KEYS + $(echo "${line}" | sed -n 's/.*dup=\([0-9]*\).*/\1/p')))
    ID_MUT=$((ID_MUT + $(echo "${line}" | sed -n 's/.*id_mut=\([0-9]*\).*/\1/p')))
    LEAKS=$((LEAKS + $(echo "${line}" | sed -n 's/.*leaks=\([0-9]*\).*/\1/p')))
    NONFINITE=$((NONFINITE + $(echo "${line}" | sed -n 's/.*nonfinite=\([0-9]*\).*/\1/p')))
    PANIC_DEADLOCK=$((PANIC_DEADLOCK + $(echo "${line}" | sed -n 's/.*panics=\([0-9]*\).*/\1/p')))
  fi
  if echo "${out}" | grep -Eq 'malformed=[1-9]|dup=[1-9]|id_mut=[1-9]|leaks=[1-9]|nonfinite=[1-9]|panics=[1-9]'; then
    fail_hard "${label} stress counters indicate failure"
  fi
  local memline
  memline=$(echo "${out}" | grep -E 'AGENT_MEM|SENSOR_MEM' | tail -n1 || true)
  if [ -n "${memline}" ]; then
    if [ "${label}" = "agent" ]; then MEM_AGENT="${memline}"; else MEM_SENSOR="${memline}"; fi
  fi
}

try_race() {
  log "Attempting Go race detector for Agent/Sensor stress (no package install)..."
  # No apk/apt/dnf/yum/new image. Reuse the same approved golang alpine image used by unit tests.
  # That image has no C compiler; -race requires cgo. Record exact evidence if unavailable.
  local any_pass=0
  local out rc
  set +e
  out=$(docker run --rm --tmpfs "${GO_TMPFS}" \
    -v "${ROOT_DIR}/agent:/app/agent:ro" \
    -w /app/agent \
    "${GO_IMG}" \
    sh -c 'CGO_ENABLED=1 go test -tags=stress -race -count=1 -timeout=25m -run TestStress ./common/supportability/' 2>&1)
  rc=$?
  set -e
  echo "${out}" | tee /tmp/build005-agent-race.txt | tail -n 40
  if [ "${rc}" -eq 0 ]; then
    any_pass=1
  elif echo "${out}" | grep -qi 'DATA RACE\|race detected'; then
    fail_hard "data race detected in agent"
    RACE_STATUS="RACE_DETECTED — agent"
    return 0
  elif echo "${out}" | grep -qi 'C compiler\|gcc.*not found\|requires cgo\|cgo: C compiler\|-race requires cgo\|loadinternal: cannot find runtime/cgo'; then
    RACE_STATUS="RACE_TOOL_UNAVAILABLE — ${GO_IMG} lacks C toolchain for -race (CGO_ENABLED=1; no apk/apt allowed); evidence=$(echo "${out}" | tr '\n' ' ' | sed 's/  */ /g' | cut -c1-240)"
  else
    # Distinguish tool absence from unexpected failure without installs.
    if echo "${out}" | grep -qi 'build failed\|build constraints\|cannot find package'; then
      RACE_STATUS="RACE_TOOL_UNAVAILABLE — agent -race build failed without installable toolchain; evidence=$(echo "${out}" | tr '\n' ' ' | sed 's/  */ /g' | cut -c1-240)"
    else
      fail_hard "agent -race stress failed rc=${rc}"
      RACE_STATUS="RACE_TOOL_AVAILABLE — agent failed"
    fi
  fi

  set +e
  out=$(docker run --rm --tmpfs "${GO_TMPFS}" \
    -v "${ROOT_DIR}/sensor:/app/sensor:ro" \
    -w /app/sensor \
    "${GO_IMG}" \
    sh -c 'CGO_ENABLED=1 go test -tags=stress -race -count=1 -timeout=25m -run TestStress ./internal/supportability/' 2>&1)
  rc=$?
  set -e
  echo "${out}" | tee /tmp/build005-sensor-race.txt | tail -n 40
  if [ "${rc}" -eq 0 ]; then
    if [ "${any_pass}" -eq 1 ]; then
      RACE_STATUS="RACE_TOOL_AVAILABLE — PASS (agent+sensor stress -race)"
    else
      RACE_STATUS="RACE_TOOL_AVAILABLE — PASS (sensor); agent inconclusive"
    fi
  elif echo "${out}" | grep -qi 'DATA RACE\|race detected'; then
    fail_hard "data race detected in sensor"
    RACE_STATUS="RACE_DETECTED — sensor"
  elif echo "${out}" | grep -qi 'C compiler\|gcc.*not found\|requires cgo\|cgo: C compiler\|-race requires cgo\|loadinternal: cannot find runtime/cgo'; then
    if [[ "${RACE_STATUS}" != RACE_TOOL_AVAILABLE* ]] && [[ "${RACE_STATUS}" != RACE_DETECTED* ]]; then
      RACE_STATUS="RACE_TOOL_UNAVAILABLE — ${GO_IMG} lacks C toolchain for -race (CGO_ENABLED=1; no apk/apt allowed); evidence=$(echo "${out}" | tr '\n' ' ' | sed 's/  */ /g' | cut -c1-240)"
    fi
  else
    if echo "${out}" | grep -qi 'build failed\|build constraints\|cannot find package'; then
      if [[ "${RACE_STATUS}" != RACE_TOOL_AVAILABLE* ]] && [[ "${RACE_STATUS}" != RACE_DETECTED* ]]; then
        RACE_STATUS="RACE_TOOL_UNAVAILABLE — sensor -race build failed without installable toolchain; evidence=$(echo "${out}" | tr '\n' ' ' | sed 's/  */ /g' | cut -c1-240)"
      fi
    else
      fail_hard "sensor -race stress failed rc=${rc}"
      RACE_STATUS="RACE_TOOL_AVAILABLE — sensor failed"
    fi
  fi
}

run_analytics_stress() {
  log "Running Analytics supportability stress..."
  local out rc=0
  set +e
  out=$(python3 "${ROOT_DIR}/analytics/tests/stress_supportability.py" 2>&1)
  rc=$?
  set -e
  echo "${out}"
  ANALYTICS_CASES=$(echo "${out}" | sed -n 's/.*cases=\([0-9]*\).*/\1/p' | tail -n1)
  local leaks malformed_a nonfinite_a
  leaks=$(echo "${out}" | sed -n 's/.*leaks=\([0-9]*\).*/\1/p' | tail -n1)
  malformed_a=$(echo "${out}" | sed -n 's/.*malformed=\([0-9]*\).*/\1/p' | tail -n1)
  nonfinite_a=$(echo "${out}" | sed -n 's/.*nonfinite=\([0-9]*\).*/\1/p' | tail -n1)
  MALFORMED=$((MALFORMED + ${malformed_a:-0}))
  NONFINITE=$((NONFINITE + ${nonfinite_a:-0}))
  if [ "${rc}" -ne 0 ]; then
    fail_hard "Analytics stress failed"
    return 0
  fi
  if [ "${leaks:-1}" != "0" ]; then
    LEAKS=$((LEAKS + leaks))
    fail_hard "Analytics credential leaks=${leaks}"
  fi
}

ensure_compose() {
  log "Ensuring Compose runtime for probe soak..."
  local live_port file_port env_name core_up=0
  live_port="$(docker port platform-core 8443 2>/dev/null | head -n1 | awk -F: '{print $NF}' | tr -d '\r' || true)"
  file_port="$(grep -E '^PLATFORM_CORE_HOST_PORT=' .env 2>/dev/null | head -n1 | cut -d= -f2- | tr -d '"' || true)"
  env_name="$(grep -E '^PLATFORM_ENV=' .env 2>/dev/null | head -n1 | cut -d= -f2- | tr -d '"' || true)"
  if docker compose ps --status running --format '{{.Name}}' 2>/dev/null | grep -q 'platform-core'; then
    core_up=1
  fi

  # Self-hosted Platform CI uses platform-* names on port 21443 with PLATFORM_ENV=ci.
  if docker ps --format '{{.Names}}' 2>/dev/null | grep -Eq '^ci_|^platform-day0'; then
    fail_hard "CI/isolation containers detected on host — refuse probe soak to avoid collision"
    return 1
  fi
  # Fail closed before any mutate/bring-up when a global platform-core is on the CI host port,
  # even if this checkout's compose project does not list that container (core_up=0).
  if [ "${live_port}" = "21443" ]; then
    fail_hard "platform-core published on CI port 21443 — refuse probe soak"
    return 1
  fi
  if [ "${core_up}" -eq 1 ]; then
    if [ "${env_name}" = "ci" ] || [ "${file_port}" = "21443" ]; then
      fail_hard "CI runtime markers present (PLATFORM_ENV=${env_name:-?} live_port=${live_port:-?} file_port=${file_port:-?}) — refuse probe soak"
      return 1
    fi
  else
    # If a prior CI run left .env in CI shape, restore only non-secret port/env keys before local bring-up.
    if [ "${env_name}" = "ci" ] || [ "${file_port}" = "21443" ]; then
      log "Resetting non-secret compose port/env keys from .env.example for local stress"
      local k v
      for k in PLATFORM_CORE_HOST_PORT PLATFORM_UI_HOST_PORT PLATFORM_POSTGRES_HOST_PORT \
               PLATFORM_CLICKHOUSE_HTTP_HOST_PORT PLATFORM_CLICKHOUSE_NATIVE_HOST_PORT \
               PLATFORM_VALKEY_HOST_PORT PLATFORM_ENV; do
        v="$(grep -E "^${k}=" .env.example 2>/dev/null | head -n1 | cut -d= -f2- || true)"
        [ -n "${v}" ] || continue
        if grep -qE "^${k}=" .env 2>/dev/null; then
          sed -i "s|^${k}=.*|${k}=${v}|" .env
        else
          echo "${k}=${v}" >> .env
        fi
      done
    fi
    log "Bringing up Compose (make dev-up)..."
    make dev-up
  fi
  CORE_PORT="$(resolve_core_host_port)"
  export CORE_PORT
  log "Probe soak using CORE_PORT=${CORE_PORT}"
  if [ "${CORE_PORT}" = "21443" ]; then
    fail_hard "CORE_PORT resolved to CI port 21443 — refuse probe soak"
    return 1
  fi
  local i
  for i in $(seq 1 60); do
    CORE_PORT="$(resolve_core_host_port)"
    export CORE_PORT
    if curl -k -sf --max-time 5 "https://127.0.0.1:${CORE_PORT}/actuator/health/liveness" >/dev/null 2>&1 \
      && docker exec platform-sensor-net curl -sf --max-time 5 http://127.0.0.1:8088/livez >/dev/null 2>&1 \
      && docker exec platform-analytics curl -sf --max-time 5 http://127.0.0.1:8090/health >/dev/null 2>&1; then
      # Require two consecutive healthy polls so mid-start CI collision is less likely.
      sleep 2
      if curl -k -sf --max-time 5 "https://127.0.0.1:${CORE_PORT}/actuator/health/liveness" >/dev/null 2>&1 \
        && docker exec platform-sensor-net curl -sf --max-time 5 http://127.0.0.1:8088/livez >/dev/null 2>&1 \
        && docker exec platform-analytics curl -sf --max-time 5 http://127.0.0.1:8090/health >/dev/null 2>&1; then
        return 0
      fi
    fi
    sleep 5
  done
  fail_hard "Compose services not healthy for probe soak (CORE_PORT=${CORE_PORT})"
  return 1
}

run_probe_soak() {
  log "Running 5-minute mixed health/probe soak (>=5000 requests, 16 workers)..."
  PROBE_SOAK_OK=0
  ensure_compose || return 0
  local end=$((SECONDS + 300))
  local workers=16
  export CORE_PORT
  # Wipe prior-run artifact so hung/failed soaks cannot inherit stale success counters.
  rm -f /tmp/build005-probe-soak.txt
  # sample memory
  local mem_start mem_mid mem_end
  mem_start=$(docker stats --no-stream --format '{{.Name}}={{.MemUsage}}' platform-core platform-sensor-net platform-analytics 2>/dev/null | tr '\n' ';')
  python3 - <<'PY' &
import os, subprocess, time, json, urllib.request, ssl, uuid, threading
CORE=os.environ.get("CORE_PORT","28443")
ctx=ssl._create_unverified_context()
# Bound concurrent docker execs so soak does not starve container healthchecks.
DOCKER_EXEC_SEM=threading.Semaphore(6)
routes=[
 ("LIVENESS", f"https://127.0.0.1:{CORE}/actuator/health/liveness"),
 ("READINESS", f"https://127.0.0.1:{CORE}/actuator/health/readiness"),
 ("PUBLIC_OVERALL", f"https://127.0.0.1:{CORE}/actuator/health"),
]
sensor=[
 ("LIVENESS","/livez"),("READINESS","/readyz"),("DETAILED","/health"),("DETAILED","/healthz"),
]
analytics_ok=[("/health","PUBLIC"),("/healthz","PUBLIC")]
analytics_404=["/live","/livez","/ready","/readyz"]
deadline=time.time()+300

def curl_http(url):
  try:
    with urllib.request.urlopen(url, context=ctx, timeout=5) as r:
      return r.status, r.read()
  except Exception as e:
    code=getattr(e,'code',None)
    if code: return code, getattr(e,'read',lambda:b'')()
    return 0, b''

def docker_curl(ctr, port, path):
  # Single docker exec; unique temp path avoids shared-/tmp races without 3x exec storms.
  token = uuid.uuid4().hex
  script = (
      "f=/tmp/pb-" + token + "; "
      "c=$(curl -s --max-time 5 -o \"$f\" -w '%{http_code}' 'http://127.0.0.1:"
      + str(int(port))
      + path
      + "'); "
      "printf '%s\\n' \"$c\"; cat \"$f\" 2>/dev/null; rm -f \"$f\""
  )
  with DOCKER_EXEC_SEM:
    try:
      p = subprocess.run(
          ["docker", "exec", ctr, "sh", "-c", script],
          capture_output=True,
          timeout=10,
      )
    except subprocess.TimeoutExpired:
      return "000", b""
  raw = p.stdout or b""
  nl = raw.find(b"\n")
  if nl < 0:
    return "000", b""
  code = raw[:nl].decode("ascii", "replace").strip()[-3:] or "000"
  return code, raw[nl + 1 :]

def check_core(role, code, body):
  if code not in (200,503):
    return 1
  try:
    obj=json.loads(body.decode("utf-8","replace"))
  except Exception:
    return 1
  if "status" not in obj:
    return 1
  # Frozen roles: liveness/readiness/public_overall must not expose dependency graphs.
  if role in ("LIVENESS","READINESS","PUBLIC_OVERALL"):
    for leak in ("components","db","clickHouse","kafka","redis"):
      if leak in obj:
        return 1
  return 0

def check_sensor(role, code, body):
  if code not in ("200","503"):
    return 1
  try:
    obj=json.loads(body.decode("utf-8","replace"))
  except Exception:
    return 1
  detailed_keys = ("document_type", "scheduler_status", "queue_depth")
  if role == "LIVENESS":
    if "status" not in obj:
      return 1
    if any(k in obj for k in detailed_keys):
      return 1
    return 0
  if role == "READINESS":
    if "status" not in obj:
      return 1
    if any(k in obj for k in detailed_keys):
      return 1
    return 0
  if role == "DETAILED":
    # /health and /healthz must remain DETAILED support-evidence envelopes.
    if obj.get("document_type") != "SUPPORT_EVIDENCE":
      return 1
    if "component" not in obj:
      return 1
    return 0
  return 1

def check_analytics_ok(code, body):
  if code not in ("200","503"):
    return 1
  try:
    obj=json.loads(body.decode("utf-8","replace"))
  except Exception:
    return 1
  if obj.get("component")!="platform-analytics":
    return 1
  if "status" not in obj:
    return 1
  return 0

import concurrent.futures

def one(worker_id):
  local_fail=0
  local_reqs=0
  # Run the full soak window (no low per-worker cap) so >=5000 requests remain reachable
  # under bounded docker-exec concurrency.
  while time.time()<deadline:
    for role,url in routes:
      code,body=curl_http(url); local_reqs+=1
      local_fail+=check_core(role, code, body)
    for role,path in sensor:
      code,body=docker_curl("platform-sensor-net",8088,path); local_reqs+=1
      local_fail+=check_sensor(role, code, body)
    for path,_ in analytics_ok:
      code,body=docker_curl("platform-analytics",8090,path); local_reqs+=1
      local_fail+=check_analytics_ok(code, body)
    for path in analytics_404:
      code,_=docker_curl("platform-analytics",8090,path); local_reqs+=1
      if code!="404": local_fail+=1
  return local_reqs, local_fail

reqs=0; fail=0
with concurrent.futures.ThreadPoolExecutor(max_workers=16) as ex:
  futs=[ex.submit(one,i) for i in range(16)]
  for f in concurrent.futures.as_completed(futs):
    r,fcount=f.result(); reqs+=r; fail+=fcount
print(f"PROBE_SOAK requests={reqs} semantic_failures={fail}")
open("/tmp/build005-probe-soak.txt","w").write(f"requests={reqs}\nfailures={fail}\n")
raise SystemExit(1 if fail else 0)
PY
  local probe_pid=$!
  sleep 150
  mem_mid=$(docker stats --no-stream --format '{{.Name}}={{.MemUsage}}' platform-core platform-sensor-net platform-analytics 2>/dev/null | tr '\n' ';')
  set +e
  local waited=0 probe_rc=124
  while [ "${waited}" -lt 360 ]; do
    if ! kill -0 "${probe_pid}" 2>/dev/null; then
      wait "${probe_pid}"
      probe_rc=$?
      break
    fi
    sleep 5
    waited=$((waited + 5))
  done
  if kill -0 "${probe_pid}" 2>/dev/null; then
    kill "${probe_pid}" 2>/dev/null || true
    wait "${probe_pid}" 2>/dev/null || true
    probe_rc=124
    fail_hard "Probe soak exceeded 360s wall clock (hung workers)"
  fi
  set -e
  mem_end=$(docker stats --no-stream --format '{{.Name}}={{.MemUsage}}' platform-core platform-sensor-net platform-analytics 2>/dev/null | tr '\n' ';')
  MEM_COMPOSE="start=${mem_start} mid=${mem_mid} end=${mem_end}"
  PROBE_REQUESTS=0
  PROBE_FAIL=0
  if [ -f /tmp/build005-probe-soak.txt ]; then
    PROBE_REQUESTS=$(sed -n 's/requests=\([0-9]*\)/\1/p' /tmp/build005-probe-soak.txt | head -n1)
    PROBE_FAIL=$(sed -n 's/failures=\([0-9]*\)/\1/p' /tmp/build005-probe-soak.txt | head -n1)
  elif [ "${probe_rc}" -eq 0 ]; then
    fail_hard "Probe soak missing result artifact after claimed success"
    probe_rc=1
  fi
  if [ "${probe_rc}" -ne 0 ] || [ "${PROBE_FAIL:-0}" -ne 0 ]; then
    fail_hard "Probe soak semantic failures=${PROBE_FAIL:-?} rc=${probe_rc}"
  fi
  if [ "${PROBE_REQUESTS:-0}" -lt 5000 ]; then
    fail_hard "Probe soak requests ${PROBE_REQUESTS} < 5000"
  fi
  if [ "${probe_rc}" -eq 0 ] && [ "${PROBE_FAIL:-0}" -eq 0 ] && [ "${PROBE_REQUESTS:-0}" -ge 5000 ]; then
    PROBE_SOAK_OK=1
  fi
  evaluate_compose_memory "${mem_start}" "${mem_mid}" "${mem_end}"
}

# Parse docker-stats used memory ("243MiB / 1GiB") into integer bytes.
# Enforce Master threshold: FAIL only when end-start > 64MiB AND >25%.
evaluate_compose_memory() {
  local start_blob="$1" mid_blob="$2" end_blob="$3"
  MEM_COMPOSE="start=${start_blob} mid=${mid_blob} end=${end_blob}"
  local eval_out mem_rc=0
  set +e
  eval_out=$(START_BLOB="${start_blob}" MID_BLOB="${mid_blob}" END_BLOB="${end_blob}" python3 - <<'PY'
import os, re, sys

THRESH_ABS = 64 * 1024 * 1024
THRESH_PCT = 25.0
UNITS = {
    "b": 1,
    "kib": 1024,
    "kb": 1000,
    "mib": 1024**2,
    "mb": 1000**2,
    "gib": 1024**3,
    "gb": 1000**3,
}
NAME_MAP = {
    "platform-core": "CORE_MEMORY",
    "platform-sensor-net": "SENSOR_MEMORY",
    "platform-analytics": "ANALYTICS_MEMORY",
}

def parse_used(blob_item: str) -> int:
    # "platform-core=243MiB / 1GiB"
    if "=" not in blob_item:
        raise ValueError(f"bad stats item: {blob_item!r}")
    _, usage = blob_item.split("=", 1)
    used = usage.split("/", 1)[0].strip()
    m = re.fullmatch(r"([0-9]*\.?[0-9]+)\s*([A-Za-z]+)", used)
    if not m:
        raise ValueError(f"unparseable used memory: {used!r}")
    num = float(m.group(1))
    unit = m.group(2).lower()
    if unit not in UNITS:
        raise ValueError(f"unsupported memory unit: {unit!r}")
    return int(num * UNITS[unit])

def parse_blob(blob: str) -> dict:
    out = {}
    for part in (blob or "").split(";"):
        part = part.strip()
        if not part or "=" not in part:
            continue
        name = part.split("=", 1)[0].strip()
        if name in NAME_MAP:
            out[name] = parse_used(part)
    return out

try:
    start = parse_blob(os.environ.get("START_BLOB", ""))
    mid = parse_blob(os.environ.get("MID_BLOB", ""))
    end = parse_blob(os.environ.get("END_BLOB", ""))
except Exception as e:
    print(f"MEMORY_PARSE_ERROR {e}")
    sys.exit(2)

overall = "PASS"
for cname, label in NAME_MAP.items():
    if cname not in start or cname not in end:
        print(f"{label} start_bytes=? mid_bytes=? end_bytes=? delta_bytes=? percent=? result=FAIL reason=missing_sample")
        overall = "FAIL"
        continue
    s = start[cname]
    m = mid.get(cname, -1)
    e = end[cname]
    delta = e - s
    pct = (delta / s * 100.0) if s > 0 else (100.0 if delta > 0 else 0.0)
    fail = (delta > THRESH_ABS) and (pct > THRESH_PCT)
    result = "FAIL" if fail else "PASS"
    if fail:
        overall = "FAIL"
    print(
        f"{label} start_bytes={s} mid_bytes={m} end_bytes={e} "
        f"delta_bytes={delta} percent={pct:.2f} result={result}"
    )
print(f"COMPOSE_MEMORY_GATE result={overall}")
sys.exit(1 if overall == "FAIL" else 0)
PY
)
  mem_rc=$?
  set -e
  echo "${eval_out}"
  MEM_CORE_LINE=$(echo "${eval_out}" | grep '^CORE_MEMORY ' | tail -n1 || true)
  MEM_SENSOR_LINE=$(echo "${eval_out}" | grep '^SENSOR_MEMORY ' | tail -n1 || true)
  MEM_ANALYTICS_LINE=$(echo "${eval_out}" | grep '^ANALYTICS_MEMORY ' | tail -n1 || true)
  if [ "${mem_rc}" -eq 2 ]; then
    fail_hard "Compose memory sample parse failed"
    MEM_ASSESSMENT="COMPOSE_MEMORY_GATE=FAIL (parse error); Go logger post-GC 32MiB gate remains hard"
    return 0
  fi
  if [ "${mem_rc}" -ne 0 ]; then
    fail_hard "Compose RSS threshold exceeded (absolute>64MiB AND percent>25%)"
    MEM_ASSESSMENT="COMPOSE_MEMORY_GATE=FAIL; Go logger post-GC 32MiB gate remains hard"
  else
    MEM_ASSESSMENT="COMPOSE_MEMORY_GATE=PASS (fail only if end-start >64MiB AND >25%); Go logger post-GC 32MiB gate remains hard"
  fi
}

run_correlation_stress() {
  log "Running correlation repetition stress (Core filter stress + Agent delivery loops)..."
  local out rc
  set +e
  out=$(docker run --rm \
    -v "${ROOT_DIR}/core:/src:ro" \
    -v "${HOME}/.m2:/root/.m2" \
    maven:3.9.9-eclipse-temurin-21-alpine \
    sh -c 'cp -r /src /build && cd /build && mvn -q -Dtest=CorrelationIdFilterStressTest,CorrelationIdFilterTest test && echo CORR_CORE_OK' 2>&1)
  rc=$?
  set -e
  echo "${out}" | tee /tmp/build005-corr-core.txt | tail -n 30
  if [ "${rc}" -ne 0 ]; then
    CORR_FAIL=$((CORR_FAIL + 1))
    fail_hard "Core correlation stress failed"
  else
    CORR_OPS=$((CORR_OPS + 2000))
  fi

  set +e
  out=$(docker run --rm --tmpfs "${GO_TMPFS}" \
    -v "${ROOT_DIR}/agent:/app/agent:ro" \
    -w /app/agent \
    "${GO_IMG}" \
    sh -c 'for i in $(seq 1 5); do go test -count=1 ./common/delivery/ -run "Correlation|Ack|Worker" || exit 1; done; echo CORR_AGENT_LOOPS=5' 2>&1)
  rc=$?
  set -e
  echo "${out}" | tee /tmp/build005-corr-agent.txt | tail -n 20
  if [ "${rc}" -ne 0 ]; then
    CORR_FAIL=$((CORR_FAIL + 1))
    fail_hard "Agent delivery correlation repetition failed"
  else
    CORR_OPS=$((CORR_OPS + 5))
  fi
}

run_recovery_smoke() {
  log "Bounded recovery smoke: restart analytics, verify new service_instance_id..."
  local live_port env_name
  live_port="$(docker port platform-core 8443 2>/dev/null | head -n1 | awk -F: '{print $NF}' | tr -d '\r' || true)"
  env_name="$(grep -E '^PLATFORM_ENV=' .env 2>/dev/null | head -n1 | cut -d= -f2- | tr -d '"' || true)"
  if [ "${live_port}" = "21443" ] || [ "${env_name}" = "ci" ]; then
    log "Skip recovery smoke — CI runtime markers present (live_port=${live_port:-?} PLATFORM_ENV=${env_name:-?})"
    return 0
  fi
  if [ "${PROBE_SOAK_OK:-0}" -ne 1 ]; then
    log "Skip recovery smoke — probe soak did not complete successfully"
    return 0
  fi
  if [ "${PROBE_REQUESTS:-0}" -lt 5000 ] || [ "${PROBE_FAIL:-0}" -ne 0 ]; then
    log "Skip recovery smoke — probe soak counters not successful"
    return 0
  fi
  if ! docker compose ps --status running --format '{{.Name}}' 2>/dev/null | grep -q platform-analytics; then
    log "Skip recovery smoke — analytics not running"
    return 0
  fi

  local pre_body post_body pre_id post_id
  pre_body=$(docker exec platform-analytics curl -s --max-time 5 http://127.0.0.1:8090/health 2>/dev/null || true)
  printf '%s' "${pre_body}" > /tmp/build005-analytics-pre-health.json
  if ! pre_id=$(python3 - <<'PY'
import json, re, sys
obj = json.load(open("/tmp/build005-analytics-pre-health.json", encoding="utf-8"))
sid = obj.get("service_instance_id") or ""
if not isinstance(sid, str) or not sid.strip():
    print("pre_health_missing_service_instance_id", file=sys.stderr); sys.exit(1)
if not re.fullmatch(r"[A-Za-z0-9._:-]{1,128}", sid):
    print(f"pre_health_invalid_service_instance_id={sid!r}", file=sys.stderr); sys.exit(1)
if obj.get("component") != "platform-analytics":
    print(f"pre_health_bad_component={obj.get('component')!r}", file=sys.stderr); sys.exit(1)
for k in ("status", "version", "ts"):
    if k not in obj:
        print(f"pre_health_missing_{k}", file=sys.stderr); sys.exit(1)
print(sid)
PY
  ); then
    fail_hard "Analytics pre-restart /health failed PUBLIC_OVERALL / service_instance_id validation"
    return 0
  fi

  docker restart platform-analytics >/dev/null

  local i code recovered=0
  for i in $(seq 1 30); do
    code=$(docker exec platform-analytics curl -s --max-time 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:8090/health 2>/dev/null || echo 000)
    if [ "${code}" = "200" ]; then
      recovered=1
      break
    fi
    sleep 2
  done
  if [ "${recovered}" -ne 1 ]; then
    fail_hard "Analytics did not recover /health after restart"
    return 0
  fi

  post_body=$(docker exec platform-analytics curl -s --max-time 5 http://127.0.0.1:8090/health 2>/dev/null || true)
  printf '%s' "${post_body}" > /tmp/build005-analytics-post-health.json
  if ! post_id=$(PRE_ID="${pre_id}" python3 - <<'PY'
import json, os, re, sys
pre = os.environ["PRE_ID"]
obj = json.load(open("/tmp/build005-analytics-post-health.json", encoding="utf-8"))
sid = obj.get("service_instance_id") or ""
if not isinstance(sid, str) or not sid.strip():
    print("post_health_missing_service_instance_id", file=sys.stderr); sys.exit(1)
if not re.fullmatch(r"[A-Za-z0-9._:-]{1,128}", sid):
    print(f"post_health_invalid_service_instance_id={sid!r}", file=sys.stderr); sys.exit(1)
if sid == pre:
    print(f"post_health_service_instance_id_unchanged={sid}", file=sys.stderr); sys.exit(1)
if obj.get("component") != "platform-analytics":
    print(f"post_health_bad_component={obj.get('component')!r}", file=sys.stderr); sys.exit(1)
for k in ("status", "version", "ts"):
    if k not in obj:
        print(f"post_health_missing_{k}", file=sys.stderr); sys.exit(1)
ver = obj.get("version") or ""
if not isinstance(ver, str) or not ver.strip() or len(ver) > 64:
    print(f"post_health_bad_version={ver!r}", file=sys.stderr); sys.exit(1)
print(sid)
PY
  ); then
    fail_hard "Analytics post-restart /health failed identity/PUBLIC_OVERALL validation"
    return 0
  fi

  docker logs --since 5m platform-analytics > /tmp/build005-analytics-post-logs.txt 2>&1 || true
  if ! POST_ID="${post_id}" python3 - <<'PY'
import json, os, re, sys
post_id = os.environ["POST_ID"]
logs = open("/tmp/build005-analytics-post-logs.txt", encoding="utf-8", errors="replace").read()
sentinels = (
    "STRESS-BEARER-SENTINEL",
    "STRESS-PEM-SENTINEL",
    "stress-password-sentinel",
    "STRESS-BASIC",
    "U1RSRVNTLUJBU0lD",
)
allowed = {
    "ts", "level", "component", "version", "service_instance_id",
    "message", "event_code", "correlation_id", "request_id", "job_id",
    "retry_count", "safe_context", "agent_id", "sensor_id",
}
found = False
for line in logs.splitlines():
    line = line.strip()
    if not line.startswith("{"):
        continue
    def _dup(pairs):
        seen = set()
        out = {}
        for k, v in pairs:
            if k in seen:
                raise ValueError(f"duplicate:{k}")
            seen.add(k)
            out[k] = v
        return out
    try:
        obj = json.loads(line, object_pairs_hook=_dup)
    except ValueError as e:
        if str(e).startswith("duplicate:"):
            print(f"log_duplicate_key {e}", file=sys.stderr); sys.exit(1)
        continue
    except Exception:
        continue
    if not isinstance(obj, dict):
        continue
    if obj.get("component") != "platform-analytics":
        continue
    if obj.get("service_instance_id") != post_id:
        continue
    for k in obj.keys():
        if k not in allowed:
            print(f"log_forbidden_or_unknown_field={k}", file=sys.stderr); sys.exit(1)
    lowered = line.lower()
    for s in sentinels:
        if s.lower() in lowered:
            print(f"log_sentinel_leak={s}", file=sys.stderr); sys.exit(1)
    found = True
    break
if not found:
    print("no_post_restart_structured_log_with_new_service_instance_id", file=sys.stderr)
    sys.exit(1)
print("ok")
PY
  then
    fail_hard "Analytics post-restart structured log validation failed"
    ANALYTICS_RESTART_LINE="ANALYTICS_RESTART old_service_instance_id=${pre_id} new_service_instance_id=${post_id} health_recovered=true log_valid=false result=FAIL"
    return 0
  fi

  ANALYTICS_RESTART_LINE="ANALYTICS_RESTART old_service_instance_id=${pre_id} new_service_instance_id=${post_id} health_recovered=true log_valid=true result=PASS"
  echo "${ANALYTICS_RESTART_LINE}"
  log "Analytics recovered with rotated service_instance_id"
}

print_summary() {
  local duration=$(( $(date +%s) - DURATION_START ))
  echo "========================================================================"
  echo "BUILD-005 SUPPORTABILITY STRESS SUMMARY"
  echo "========================================================================"
  echo "duration_s=${duration}"
  echo "agent_log_events=${AGENT_EVENTS}"
  echo "sensor_log_events=${SENSOR_EVENTS}"
  echo "analytics_hostile_cases=${ANALYTICS_CASES}"
  echo "concurrent_workers_logger=32"
  echo "concurrent_workers_probe=16"
  echo "probe_soak_duration_s=300"
  echo "total_probe_requests=${PROBE_REQUESTS}"
  echo "correlation_ops=${CORR_OPS}"
  echo "malformed_json=${MALFORMED}"
  echo "duplicate_keys=${DUP_KEYS}"
  echo "identity_mutations=${ID_MUT}"
  echo "credential_sentinel_leaks=${LEAKS}"
  echo "nonfinite_leaks=${NONFINITE}"
  echo "probe_semantic_failures=${PROBE_FAIL}"
  echo "probe_soak_ok=${PROBE_SOAK_OK}"
  echo "correlation_failures=${CORR_FAIL}"
  echo "panic_deadlock=${PANIC_DEADLOCK}"
  echo "race_status=${RACE_STATUS}"
  echo "memory_agent=${MEM_AGENT}"
  echo "memory_sensor=${MEM_SENSOR}"
  echo "memory_compose=${MEM_COMPOSE}"
  echo "memory_core=${MEM_CORE_LINE}"
  echo "memory_sensor_compose=${MEM_SENSOR_LINE}"
  echo "memory_analytics_compose=${MEM_ANALYTICS_LINE}"
  echo "memory_assessment=${MEM_ASSESSMENT}"
  echo "analytics_restart=${ANALYTICS_RESTART_LINE}"
  if [ "${STRESS_PASS}" -eq 1 ]; then
    echo "RESULT=PASS"
  else
    echo "RESULT=FAIL"
  fi
  echo "========================================================================"
}

# ---- main ----
log "BUILD-005 supportability stress gate starting at $(date -u +%Y-%m-%dT%H:%M:%SZ)"
run_go_stress "agent" "agent" "./common/supportability/"
run_go_stress "sensor" "sensor" "./internal/supportability/"
try_race || true
run_analytics_stress
run_correlation_stress
run_probe_soak || true
run_recovery_smoke || true
print_summary
if [ "${STRESS_PASS}" -ne 1 ]; then
  exit 1
fi
exit 0
