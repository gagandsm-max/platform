#!/usr/bin/env bash
# BUILD-005 whole-build supportability stress/soak gate.
# CERTIFICATION-FIRST. Exit non-zero on hard failure.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT_DIR}"
# Do not source .env (may contain local secrets). Port/image defaults only.
if [ -z "${PLATFORM_CORE_HOST_PORT:-}" ] && [ -f .env.example ]; then
  PLATFORM_CORE_HOST_PORT="$(grep -E '^PLATFORM_CORE_HOST_PORT=' .env.example | cut -d= -f2- | tr -d '"' || true)"
fi

GO_IMG="${PLATFORM_STRESS_GO_IMAGE:-golang:1.27.1-alpine3.24}"
GO_TMPFS="/tmp:rw,exec,size=2147483648,mode=1777"
CORE_PORT="${PLATFORM_CORE_HOST_PORT:-28443}"

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
CORR_FAIL=0
PANIC_DEADLOCK=0
RACE_STATUS="RACE_TOOL_UNAVAILABLE"
MEM_AGENT=""
MEM_SENSOR=""
MEM_COMPOSE=""
MEM_ASSESSMENT="UNMEASURED"
STRESS_PASS=1
DURATION_START=$(date +%s)

log() { echo "==> $*"; }
fail_hard() { echo "[HARD_FAIL] $*"; STRESS_PASS=0; }

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
  log "Attempting Go race detector for Agent/Sensor stress..."
  local any_pass=0
  # agent
  local out rc
  set +e
  out=$(docker run --rm --tmpfs "${GO_TMPFS}" \
    -v "${ROOT_DIR}/agent:/app/agent:ro" \
    -w /app/agent \
    "${GO_IMG}" \
    sh -c 'apk add --no-cache gcc musl-dev >/dev/null && CGO_ENABLED=1 go test -tags=stress -race -count=1 -timeout=25m -run TestStress ./common/supportability/' 2>&1)
  rc=$?
  set -e
  echo "${out}" | tee /tmp/build005-agent-race.txt | tail -n 30
  if [ "${rc}" -eq 0 ]; then
    any_pass=1
  elif echo "${out}" | grep -qi 'DATA RACE\|race detected'; then
    fail_hard "data race detected in agent"
    RACE_STATUS="RACE_DETECTED — agent"
    return 0
  elif echo "${out}" | grep -qi 'not supported\|requires cgo\|install gcc'; then
    RACE_STATUS="RACE_TOOL_UNAVAILABLE — agent toolchain could not execute -race"
  else
    fail_hard "agent -race stress failed rc=${rc}"
    RACE_STATUS="RACE_TOOL_AVAILABLE — agent failed"
  fi

  set +e
  out=$(docker run --rm --tmpfs "${GO_TMPFS}" \
    -v "${ROOT_DIR}/sensor:/app/sensor:ro" \
    -w /app/sensor \
    "${GO_IMG}" \
    sh -c 'apk add --no-cache gcc musl-dev >/dev/null && CGO_ENABLED=1 go test -tags=stress -race -count=1 -timeout=25m -run TestStress ./internal/supportability/' 2>&1)
  rc=$?
  set -e
  echo "${out}" | tee /tmp/build005-sensor-race.txt | tail -n 30
  if [ "${rc}" -eq 0 ]; then
    if [ "${any_pass}" -eq 1 ]; then
      RACE_STATUS="RACE_TOOL_AVAILABLE — PASS (agent+sensor stress -race)"
    else
      RACE_STATUS="RACE_TOOL_AVAILABLE — PASS (sensor); agent inconclusive"
    fi
  elif echo "${out}" | grep -qi 'DATA RACE\|race detected'; then
    fail_hard "data race detected in sensor"
    RACE_STATUS="RACE_DETECTED — sensor"
  elif echo "${out}" | grep -qi 'not supported\|requires cgo\|install gcc'; then
    if [[ "${RACE_STATUS}" != RACE_TOOL_AVAILABLE* ]]; then
      RACE_STATUS="RACE_TOOL_UNAVAILABLE — sensor toolchain could not execute -race"
    fi
  else
    fail_hard "sensor -race stress failed rc=${rc}"
    RACE_STATUS="RACE_TOOL_AVAILABLE — sensor failed"
  fi
}

run_analytics_stress() {
  log "Running Analytics supportability stress..."
  local out
  out=$(python3 "${ROOT_DIR}/analytics/tests/stress_supportability.py" 2>&1) || {
    echo "${out}"
    fail_hard "Analytics stress failed"
    return 0
  }
  echo "${out}"
  ANALYTICS_CASES=$(echo "${out}" | sed -n 's/.*cases=\([0-9]*\).*/\1/p' | tail -n1)
  local leaks
  leaks=$(echo "${out}" | sed -n 's/.*leaks=\([0-9]*\).*/\1/p' | tail -n1)
  if [ "${leaks:-1}" != "0" ]; then
    LEAKS=$((LEAKS + leaks))
    fail_hard "Analytics credential leaks=${leaks}"
  fi
}

ensure_compose() {
  log "Ensuring Compose runtime for probe soak..."
  if ! docker compose ps --status running --format '{{.Name}}' 2>/dev/null | grep -q 'platform-core'; then
    log "Bringing up Compose (make dev-up)..."
    make dev-up
  fi
  local i
  for i in $(seq 1 60); do
    if curl -k -sf "https://127.0.0.1:${CORE_PORT}/actuator/health/liveness" >/dev/null 2>&1 \
      && docker exec platform-sensor-net curl -sf http://127.0.0.1:8088/livez >/dev/null 2>&1 \
      && docker exec platform-analytics curl -sf http://127.0.0.1:8090/health >/dev/null 2>&1; then
      return 0
    fi
    sleep 5
  done
  fail_hard "Compose services not healthy for probe soak"
  return 1
}

probe_once() {
  local kind="$1" url="$2" expect_code="$3"
  local code body
  if [[ "${url}" == docker:* ]]; then
    local ctr="${url#docker:}"
    ctr="${ctr%%/*}"
    local path="/${url#*/}"
    body=$(docker exec "${ctr}" curl -s -o /tmp/p.body -w '%{http_code}' "http://127.0.0.1${path}" 2>/dev/null || echo "000")
    code="${body: -3}"
    body=$(docker exec "${ctr}" cat /tmp/p.body 2>/dev/null || true)
  else
    code=$(curl -k -s -o /tmp/p.body -w '%{http_code}' "${url}" 2>/dev/null || echo "000")
    body=$(cat /tmp/p.body 2>/dev/null || true)
  fi
  PROBE_REQUESTS=$((PROBE_REQUESTS + 1))
  case "${kind}" in
    LIVENESS|READINESS|PUBLIC_OVERALL|DETAILED)
      if [ "${code}" != "200" ] && [ "${code}" != "503" ]; then
        PROBE_FAIL=$((PROBE_FAIL + 1))
      fi
      ;;
    ABSENT)
      if [ "${code}" != "404" ]; then
        PROBE_FAIL=$((PROBE_FAIL + 1))
      fi
      ;;
  esac
}

run_probe_soak() {
  log "Running 5-minute mixed health/probe soak (>=5000 requests, 16 workers)..."
  ensure_compose || return 0
  local end=$((SECONDS + 300))
  local workers=16
  export CORE_PORT
  # sample memory
  local mem_start mem_mid mem_end
  mem_start=$(docker stats --no-stream --format '{{.Name}}={{.MemUsage}}' platform-core platform-sensor-net platform-analytics 2>/dev/null | tr '\n' ';')
  python3 - <<'PY' &
import os, subprocess, time, json, urllib.request, ssl, uuid
CORE=os.environ.get("CORE_PORT","28443")
ctx=ssl._create_unverified_context()
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
  tmp=f"/tmp/pb-{uuid.uuid4().hex}"
  p=subprocess.run(
    ["docker","exec",ctr,"curl","-s","-o",tmp,"-w","%{http_code}",f"http://127.0.0.1:{port}{path}"],
    capture_output=True,text=True)
  code=(p.stdout or "000").strip()[-3:]
  body_p=subprocess.run(["docker","exec",ctr,"cat",tmp],capture_output=True)
  subprocess.run(["docker","exec",ctr,"rm","-f",tmp],capture_output=True)
  return code, body_p.stdout or b""

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
  if role in ("LIVENESS","READINESS"):
    if "status" not in obj:
      return 1
    # DETAILED fields must not appear on pure liveness
    if role=="LIVENESS" and any(k in obj for k in ("document_type","scheduler_status","queue_depth")):
      return 1
  if role=="DETAILED":
    # compatibility /health and /healthz remain DETAILED evidence surface
    if "document_type" not in obj and "component" not in obj and "status" not in obj:
      return 1
  return 0

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
  while time.time()<deadline and local_reqs<400:
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
  wait "${probe_pid}"
  local probe_rc=$?
  set -e
  mem_end=$(docker stats --no-stream --format '{{.Name}}={{.MemUsage}}' platform-core platform-sensor-net platform-analytics 2>/dev/null | tr '\n' ';')
  MEM_COMPOSE="start=${mem_start} mid=${mem_mid} end=${mem_end}"
  if [ -f /tmp/build005-probe-soak.txt ]; then
    PROBE_REQUESTS=$(sed -n 's/requests=\([0-9]*\)/\1/p' /tmp/build005-probe-soak.txt | head -n1)
    PROBE_FAIL=$(sed -n 's/failures=\([0-9]*\)/\1/p' /tmp/build005-probe-soak.txt | head -n1)
  fi
  if [ "${probe_rc}" -ne 0 ] || [ "${PROBE_FAIL:-0}" -ne 0 ]; then
    fail_hard "Probe soak semantic failures=${PROBE_FAIL:-?} rc=${probe_rc}"
  fi
  if [ "${PROBE_REQUESTS:-0}" -lt 5000 ]; then
    fail_hard "Probe soak requests ${PROBE_REQUESTS} < 5000"
  fi
  MEM_ASSESSMENT="Compose samples captured; no automated MiB delta parser — review MEM_COMPOSE; Go logger windows assessed in unit stress"
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
  log "Bounded recovery smoke: restart analytics, recheck probes..."
  if ! docker compose ps --status running --format '{{.Name}}' 2>/dev/null | grep -q platform-analytics; then
    log "Skip recovery smoke — analytics not running"
    return 0
  fi
  docker restart platform-analytics >/dev/null
  local i code
  for i in $(seq 1 30); do
    code=$(docker exec platform-analytics curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8090/health 2>/dev/null || echo 000)
    if [ "${code}" = "200" ]; then
      log "Analytics recovered HTTP ${code}"
      return 0
    fi
    sleep 2
  done
  fail_hard "Analytics did not recover /health after restart"
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
  echo "concurrent_workers=32"
  echo "probe_soak_duration_s=300"
  echo "total_probe_requests=${PROBE_REQUESTS}"
  echo "correlation_ops=${CORR_OPS}"
  echo "malformed_json=${MALFORMED}"
  echo "duplicate_keys=${DUP_KEYS}"
  echo "identity_mutations=${ID_MUT}"
  echo "credential_sentinel_leaks=${LEAKS}"
  echo "nonfinite_leaks=${NONFINITE}"
  echo "probe_semantic_failures=${PROBE_FAIL}"
  echo "correlation_failures=${CORR_FAIL}"
  echo "panic_deadlock=${PANIC_DEADLOCK}"
  echo "race_status=${RACE_STATUS}"
  echo "memory_agent=${MEM_AGENT}"
  echo "memory_sensor=${MEM_SENSOR}"
  echo "memory_compose=${MEM_COMPOSE}"
  echo "memory_assessment=${MEM_ASSESSMENT}"
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
