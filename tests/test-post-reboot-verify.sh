#!/usr/bin/env bash
# Behavioral fixture harness for scripts/post-reboot-verify.sh HTTPS probes.
# Uses PATH stubs only — no live Docker stack, real TLS handshake, or host reboot.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERIFY_SCRIPT="${POST_REBOOT_VERIFY_UNDER_TEST:-${ROOT_DIR}/scripts/post-reboot-verify.sh}"

HARNESS_FAILED=0
fail() {
  echo "[FAIL] $*"
  if [ "${POST_REBOOT_VERIFY_COLLECT_ALL:-0}" = "1" ]; then
    HARNESS_FAILED=1
    return 0
  fi
  exit 1
}
pass() { echo "[OK] $*"; }

[[ -f "${VERIFY_SCRIPT}" ]] || fail "missing verifier under test: ${VERIFY_SCRIPT}"
[[ -x "${VERIFY_SCRIPT}" ]] || chmod +x "${VERIFY_SCRIPT}"

EXPECTED_SENSOR="sensor-net-01"
EXPECTED_TENANT="018f0e0a-0000-7000-8000-000000000001"
VALID_RECEIVED_AT="2026-10-17T12:00:00.000Z"
MAX_HEARTBEAT_BYTES=8192

VALID_HEARTBEAT="$(python3 - <<'PY'
import json
print(json.dumps({
    "status": "ACCEPTED",
    "authenticated": True,
    "sensor_identity": "sensor-net-01",
    "tenant_id": "018f0e0a-0000-7000-8000-000000000001",
    "received_at": "2026-10-17T12:00:00.000Z",
}, separators=(",", ":")))
PY
)"

echo "========================================================================"
echo "Platform Post-Reboot Verifier Strict-Auth Fixture Harness"
echo "Under test: ${VERIFY_SCRIPT}"
echo "========================================================================"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/post-reboot-verify-fixture.XXXXXX")"
cleanup() { rm -rf "${WORK}"; }
trap cleanup EXIT

STUB_BIN="${WORK}/stub-bin"
FIXTURE_ROOT="${WORK}/repo"
mkdir -p "${STUB_BIN}" "${FIXTURE_ROOT}/scripts" \
  "${FIXTURE_ROOT}/deploy/docker/certs" \
  "${WORK}/state"

cp "${VERIFY_SCRIPT}" "${FIXTURE_ROOT}/scripts/post-reboot-verify.sh"
chmod +x "${FIXTURE_ROOT}/scripts/post-reboot-verify.sh"

# Synthetic fixture CA/client material (command-construction only; not a TLS handshake).
printf 'FIXTURE-CA\n' > "${FIXTURE_ROOT}/deploy/docker/certs/ca.crt"
printf 'FIXTURE-CERT\n' > "${FIXTURE_ROOT}/deploy/docker/certs/sensor-net.crt"
printf 'FIXTURE-KEY\n' > "${FIXTURE_ROOT}/deploy/docker/certs/sensor-net.key"

# ---- stubs ------------------------------------------------------------------
cat > "${STUB_BIN}/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
STATE_DIR="${POST_REBOOT_FIXTURE_STATE:?}"
cmd="${1:-}"
case "${cmd}" in
  info)
    exit 0
    ;;
  inspect)
    # docker inspect <name> -f '...'
    # Health templates also contain the substring Status; match Health first.
    fmt="${4:-}"
    case "${fmt}" in
      *Health*)
        printf 'healthy\n'
        ;;
      *Status*)
        printf 'running\n'
        ;;
      *)
        printf 'running\n'
        ;;
    esac
    exit 0
    ;;
  ps)
    # foreign-container check: emit nothing (no foreign containers)
    exit 0
    ;;
  exec)
    target="${2:-}"
    shift 2 || true
    case "${target}" in
      platform-postgres)
        printf '1\n'
        ;;
      platform-clickhouse)
        printf '1\n'
        ;;
      platform-valkey)
        printf 'PONG\n'
        ;;
      *)
        echo "unexpected docker exec target: ${target}" >&2
        exit 99
        ;;
    esac
    exit 0
    ;;
  *)
    echo "unexpected docker command: $*" >&2
    exit 99
    ;;
esac
EOF
chmod +x "${STUB_BIN}/docker"

# head stub: prove download-path byte ceiling and retained size for heartbeat.
cat > "${STUB_BIN}/head" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
STATE_DIR="${POST_REBOOT_FIXTURE_STATE:?}"
{
  printf 'ARGV:'
  printf ' %q' "$@"
  printf '\n'
} >> "${STATE_DIR}/head-invocations.log"
out_tmp="${STATE_DIR}/head-retained.bin"
/usr/bin/head "$@" > "${out_tmp}"
wc -c < "${out_tmp}" | tr -d '[:space:]' > "${STATE_DIR}/head-retained.bytes"
cat "${out_tmp}"
EOF
chmod +x "${STUB_BIN}/head"

cat > "${STUB_BIN}/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
STATE_DIR="${POST_REBOOT_FIXTURE_STATE:?}"
log="${STATE_DIR}/curl-invocations.log"

# Record argv for later assertions (no secret material in fixture).
{
  printf 'ARGV:'
  printf ' %q' "$@"
  printf '\n'
} >> "${log}"

saw_insecure=0
for arg in "$@"; do
  case "${arg}" in
    -k|--insecure)
      saw_insecure=1
      ;;
  esac
done

cacert=""
outfile=""
write_fmt=""
url=""
have_cert=0
have_key=0
args=("$@")
i=0
while [ "${i}" -lt "${#args[@]}" ]; do
  a="${args[$i]}"
  case "${a}" in
    --cacert)
      i=$((i + 1))
      cacert="${args[$i]:-}"
      ;;
    --cert)
      i=$((i + 1))
      have_cert=1
      ;;
    --key)
      i=$((i + 1))
      have_key=1
      ;;
    -o|--output)
      i=$((i + 1))
      outfile="${args[$i]:-}"
      ;;
    -w|--write-out)
      i=$((i + 1))
      write_fmt="${args[$i]:-}"
      ;;
    https://*|http://*)
      url="${a}"
      ;;
  esac
  i=$((i + 1))
done

# Default: enforce trusted transport construction. Failing-first against the
# historical -k verifier may set CURL_FIXTURE_ALLOW_INSECURE=1 only.
if [ "${saw_insecure}" -eq 1 ]; then
  echo "INSECURE_FLAG=1" >> "${STATE_DIR}/curl-flags.log"
  if [ "${CURL_FIXTURE_ALLOW_INSECURE:-0}" != "1" ]; then
    echo "curl-fixture: insecure TLS flag forbidden" >&2
    exit 90
  fi
else
  echo "INSECURE_FLAG=0" >> "${STATE_DIR}/curl-flags.log"
fi

if [ "${CURL_FIXTURE_ALLOW_INSECURE:-0}" != "1" ]; then
  if [ -z "${cacert}" ]; then
    echo "curl-fixture: --cacert required" >&2
    exit 91
  fi
  if [ ! -f "${cacert}" ]; then
    echo "curl-fixture: cacert missing: ${cacert}" >&2
    exit 92
  fi
fi

scenario_file="${STATE_DIR}/scenario.env"
# shellcheck disable=SC1090
source "${scenario_file}"

read_body_file() {
  local path="${1:-}"
  if [ -n "${path}" ] && [ -f "${path}" ]; then
    cat "${path}"
  fi
}

write_body_to_outfile() {
  local dest="${1:-}"
  local payload="${2:-}"
  if [ -z "${dest}" ]; then
    printf '%s' "${payload}"
    return 0
  fi
  # Status-only probes discard bodies to /dev/null.
  if [ "${dest}" = "/dev/null" ]; then
    printf 'DISCARD_BODY=1\n' >> "${STATE_DIR}/curl-body-disposition.log"
    return 0
  fi
  # Consumer may close early on the heartbeat byte ceiling (SIGPIPE/EPIPE).
  set +e
  printf '%s' "${payload}" > "${dest}" 2>/dev/null
  set -e
  return 0
}

# Controlled excess producer: stream well past the ceiling; record cancel/bound.
stream_excess_to_outfile() {
  local dest="${1:?}"
  local attempt_bytes="${2:?}"
  local chunk_size=4096
  python3 - "${dest}" "${attempt_bytes}" "${chunk_size}" "${STATE_DIR}/curl-stream.log" <<'PY'
import sys
dest, attempt_s, chunk_s, log_path = sys.argv[1:5]
attempt = int(attempt_s)
chunk_size = int(chunk_s)
chunk = b"x" * chunk_size
written = 0
cancelled = 0
try:
    with open(dest, "wb", buffering=0) as fh:
        while written < attempt:
            n = min(chunk_size, attempt - written)
            try:
                fh.write(chunk[:n])
            except BrokenPipeError:
                cancelled = 1
                break
            written += n
except BrokenPipeError:
    cancelled = 1
except OSError as exc:
    # EPIPE on some platforms surfaces as OSError errno 32.
    if getattr(exc, "errno", None) == 32:
        cancelled = 1
    else:
        raise
with open(log_path, "w", encoding="utf-8") as log:
    log.write(f"ATTEMPT_BYTES={attempt}\n")
    log.write(f"BYTES_WRITTEN={written}\n")
    log.write(f"CANCELLED={cancelled}\n")
# Mimic curl write-error after consumer closes the pipe.
sys.exit(23 if cancelled else 0)
PY
}

body=""
http="200"
ec=0
stream_excess=0
case "${url}" in
  https://127.0.0.1:23000/|https://127.0.0.1:23000)
    body="$(read_body_file "${UI_BODY_FILE:-}")"
    http="${UI_HTTP:-200}"
    ec="${UI_CURL_EC:-0}"
    ;;
  https://127.0.0.1:28443/actuator/health|https://127.0.0.1:28443/actuator/health/)
    body="$(read_body_file "${CORE_BODY_FILE:-}")"
    http="${CORE_HTTP:-200}"
    ec="${CORE_CURL_EC:-0}"
    ;;
  https://127.0.0.1:28443/api/v1/sensor/heartbeat)
    body="$(read_body_file "${HB_BODY_FILE:-}")"
    http="${HB_HTTP:-200}"
    ec="${HB_CURL_EC:-0}"
    stream_excess="${HB_STREAM_EXCESS:-0}"
    if [ "${have_cert}" -ne 1 ] || [ "${have_key}" -ne 1 ]; then
      echo "curl-fixture: heartbeat requires --cert and --key" >&2
      exit 93
    fi
    ;;
  *)
    echo "curl-fixture: unexpected URL: ${url}" >&2
    exit 99
    ;;
esac

if [ "${stream_excess}" = "1" ]; then
  if [ -z "${outfile}" ] || [ "${outfile}" = "/dev/null" ]; then
    echo "curl-fixture: excess stream requires a body outfile" >&2
    exit 94
  fi
  set +e
  stream_excess_to_outfile "${outfile}" "${HB_STREAM_ATTEMPT_BYTES:-2097152}"
  stream_ec=$?
  set -e
  if [ -n "${write_fmt}" ]; then
    printf '%s' "${http}"
  fi
  # Prefer scenario ec if explicitly forcing transport failure; else stream ec.
  if [ "${HB_CURL_EC:-0}" != "0" ]; then
    exit "${HB_CURL_EC}"
  fi
  exit "${stream_ec}"
fi

write_body_to_outfile "${outfile}" "${body}"
if [ -n "${write_fmt}" ]; then
  # Only %{http_code} is used by the verifier.
  printf '%s' "${http}"
fi
exit "${ec}"
EOF
chmod +x "${STUB_BIN}/curl"

# Reject any other network/docker-adjacent commands that should not appear.
for unexpected in wget nc nmap openssl docker-compose; do
  cat > "${STUB_BIN}/${unexpected}" <<EOF
#!/usr/bin/env bash
echo "unexpected command: ${unexpected} \$*" >&2
exit 99
EOF
  chmod +x "${STUB_BIN}/${unexpected}"
done

write_scenario() {
  local path="$1"
  shift
  : > "${path}"
  local kv
  for kv in "$@"; do
    printf '%s\n' "${kv}" >> "${path}"
  done
}

# Usage: run_case NAME pass|fail [scenario.env KEY=VAL ...] --body BODY_TEXT
run_case() {
  local name="$1"
  local expect_pass="$2" # pass|fail
  shift 2
  local state="${WORK}/state/${name}"
  rm -rf "${state}"
  mkdir -p "${state}"

  local body_text=""
  local env_kv=()
  while [ "$#" -gt 0 ]; do
    if [ "$1" = "--body" ]; then
      shift
      body_text="${1:-}"
      shift || true
      break
    fi
    env_kv+=("$1")
    shift
  done

  printf '%s' "${body_text}" > "${state}/hb.body"
  write_scenario "${state}/scenario.env" \
    "HB_BODY_FILE=${state}/hb.body" \
    "UI_BODY_FILE=" \
    "CORE_BODY_FILE=" \
    "${env_kv[@]}"

  local out ec
  set +e
  out="$(
    cd "${FIXTURE_ROOT}"
    env -i \
      PATH="${STUB_BIN}:/usr/local/bin:/usr/bin:/bin" \
      HOME="${WORK}/home" \
      POST_REBOOT_FIXTURE_STATE="${state}" \
      CURL_FIXTURE_ALLOW_INSECURE="${CURL_FIXTURE_ALLOW_INSECURE:-0}" \
      VALKEY_PASSWORD="fixture-not-a-secret" \
      bash "./scripts/post-reboot-verify.sh" 2>&1
  )"
  ec=$?
  set -e

  printf '%s\n' "${out}" > "${state}/stdout.txt"
  printf '%s\n' "${ec}" > "${state}/exit.txt"

  # Sanitization applies to the corrected verifier. Historical base prints bodies.
  if [ "${CURL_FIXTURE_ALLOW_INSECURE:-0}" != "1" ]; then
    if grep -F "${VALID_HEARTBEAT}" <<<"${out}" >/dev/null 2>&1; then
      fail "${name}: verifier leaked exact heartbeat body"
    fi
    if grep -E 'FIXTURE-KEY|private key|BEGIN .*PRIVATE' <<<"${out}" >/dev/null 2>&1; then
      fail "${name}: verifier leaked key material"
    fi
    # Oversized / crafted bodies should not be dumped verbatim.
    if grep -E 'status.:.(ACCEPTED|ERROR).*,.?authenticated' <<<"${out}" >/dev/null 2>&1; then
      fail "${name}: verifier appears to print raw JSON body"
    fi
  fi

  if [ "${expect_pass}" = "pass" ]; then
    if [[ "${ec}" -ne 0 ]]; then
      fail "${name}: expected pass exit 0, got ${ec}; output=$(tr '\n' ' ' <<<"${out}" | head -c 400)"
      return 0
    fi
    if ! grep -q 'STRUCTURALLY READY — ACTUAL HOST REBOOT TEST PENDING' <<<"${out}"; then
      fail "${name}: missing STRUCTURALLY READY banner"
      return 0
    fi
    if ! grep -q 'ALL STRUCTURAL CHECKS PASSED' <<<"${out}"; then
      fail "${name}: missing ALL STRUCTURAL CHECKS PASSED"
      return 0
    fi
    pass "${name}"
  else
    if [[ "${ec}" -eq 0 ]]; then
      fail "${name}: expected nonzero failure, got 0 (false-positive evidence path)"
      return 0
    fi
    if ! grep -q 'VERIFICATION RESULT: .*FAILED' <<<"${out}"; then
      fail "${name}: missing FAILED verification result"
      return 0
    fi
    pass "${name} (correctly rejected)"
  fi
}

# Positive control
run_case "valid-exact-authority" pass \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body "${VALID_HEARTBEAT}"

# Transport failure with ACCEPTED body still present on stdout path
run_case "transport-fail-accepted-body" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=35" \
  --body "${VALID_HEARTBEAT}"

# Non-200 with structurally valid body
run_case "non-200-valid-body" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=401" "HB_CURL_EC=0" \
  --body "${VALID_HEARTBEAT}"

# ACCEPTED only in unrelated/error fields
run_case "accepted-in-unrelated-field" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body '{"status":"ERROR","code":"ERR_AUTH","message":"not ACCEPTED","authenticated":false,"sensor_identity":"sensor-net-01","tenant_id":"018f0e0a-0000-7000-8000-000000000001","received_at":"2026-10-17T12:00:00.000Z"}'

run_case "accepted-only-in-message" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body '{"message":"ACCEPTED","authenticated":true,"sensor_identity":"sensor-net-01","tenant_id":"018f0e0a-0000-7000-8000-000000000001","received_at":"2026-10-17T12:00:00.000Z","status":"ERROR"}'

# Authentication type confusion
run_case "authenticated-false" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body '{"status":"ACCEPTED","authenticated":false,"sensor_identity":"sensor-net-01","tenant_id":"018f0e0a-0000-7000-8000-000000000001","received_at":"2026-10-17T12:00:00.000Z"}'

run_case "authenticated-string-true" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body '{"status":"ACCEPTED","authenticated":"true","sensor_identity":"sensor-net-01","tenant_id":"018f0e0a-0000-7000-8000-000000000001","received_at":"2026-10-17T12:00:00.000Z"}'

run_case "authenticated-numeric-one" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body '{"status":"ACCEPTED","authenticated":1,"sensor_identity":"sensor-net-01","tenant_id":"018f0e0a-0000-7000-8000-000000000001","received_at":"2026-10-17T12:00:00.000Z"}'

# Wrong / missing identities
run_case "wrong-sensor-identity" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body '{"status":"ACCEPTED","authenticated":true,"sensor_identity":"sensor-infra-01","tenant_id":"018f0e0a-0000-7000-8000-000000000001","received_at":"2026-10-17T12:00:00.000Z"}'

run_case "wrong-tenant-id" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body '{"status":"ACCEPTED","authenticated":true,"sensor_identity":"sensor-net-01","tenant_id":"018f0e0a-0000-7000-8000-000000000099","received_at":"2026-10-17T12:00:00.000Z"}'

run_case "missing-received-at" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body '{"status":"ACCEPTED","authenticated":true,"sensor_identity":"sensor-net-01","tenant_id":"018f0e0a-0000-7000-8000-000000000001"}'

# Malformed / duplicate JSON
run_case "malformed-json" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body '{"status":"ACCEPTED","authenticated":true'

run_case "duplicate-json-keys" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body '{"status":"ERROR","authenticated":false,"sensor_identity":"sensor-net-01","tenant_id":"018f0e0a-0000-7000-8000-000000000001","received_at":"2026-10-17T12:00:00.000Z","status":"ACCEPTED","authenticated":true}'

# Byte-gate isolation: legal exact-authority shape + trailing whitespace over the
# cap (no forbidden extra JSON properties). Size check must fire before schema.
OVERSIZED_WHITESPACE="$(python3 - <<PY
import json
base = json.dumps({
    "status": "ACCEPTED",
    "authenticated": True,
    "sensor_identity": "sensor-net-01",
    "tenant_id": "${EXPECTED_TENANT}",
    "received_at": "${VALID_RECEIVED_AT}",
}, separators=(",", ":"))
need = ${MAX_HEARTBEAT_BYTES} + 1
if len(base) > need:
    raise SystemExit("fixture base already exceeds cap")
print(base + (" " * (need - len(base))), end="")
PY
)"
run_case "oversized-whitespace-legal-shape" fail \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body "${OVERSIZED_WHITESPACE}"
# Pin byte-gate isolation (code=2), not schema/key failure or transport.
OW_OUT="${WORK}/state/oversized-whitespace-legal-shape/stdout.txt"
[[ -f "${OW_OUT}" ]] || fail "oversized-whitespace-legal-shape: missing stdout"
grep -q 'authority/schema validation (code=2)' "${OW_OUT}" \
  || fail "oversized-whitespace-legal-shape: expected validation code=2"
if grep -q 'Sensor heartbeat transport failed' "${OW_OUT}"; then
  fail "oversized-whitespace-legal-shape: byte gate misclassified as transport failure"
fi

# Valid response exactly at the byte ceiling (compact JSON + spaces).
EXACT_CAP="$(python3 - <<PY
import json
base = json.dumps({
    "status": "ACCEPTED",
    "authenticated": True,
    "sensor_identity": "sensor-net-01",
    "tenant_id": "${EXPECTED_TENANT}",
    "received_at": "${VALID_RECEIVED_AT}",
}, separators=(",", ":"))
need = ${MAX_HEARTBEAT_BYTES}
if len(base) > need:
    raise SystemExit("fixture base already exceeds cap")
body = base + (" " * (need - len(base)))
assert len(body) == need
print(body, end="")
PY
)"
run_case "valid-exact-byte-cap" pass \
  "UI_HTTP=200" "UI_CURL_EC=0" \
  "CORE_HTTP=200" "CORE_CURL_EC=0" \
  "HB_HTTP=200" "HB_CURL_EC=0" \
  --body "${EXACT_CAP}"

# Controlled excess producer: stream >> cap; prove cancel + bounded retain.
run_case_stream_excess() {
  local name="excess-stream-cancelled-bounded"
  local state="${WORK}/state/${name}"
  rm -rf "${state}"
  mkdir -p "${state}"
  # Empty HB body file — producer ignores it and streams excess instead.
  : > "${state}/hb.body"
  write_scenario "${state}/scenario.env" \
    "UI_HTTP=200" "UI_CURL_EC=0" \
    "CORE_HTTP=200" "CORE_CURL_EC=0" \
    "HB_HTTP=200" "HB_CURL_EC=0" \
    "HB_STREAM_EXCESS=1" \
    "HB_STREAM_ATTEMPT_BYTES=2097152" \
    "HB_BODY_FILE=${state}/hb.body" \
    "UI_BODY_FILE=" \
    "CORE_BODY_FILE="

  local out ec
  set +e
  out="$(
    cd "${FIXTURE_ROOT}"
    env -i \
      PATH="${STUB_BIN}:/usr/local/bin:/usr/bin:/bin" \
      HOME="${WORK}/home" \
      POST_REBOOT_FIXTURE_STATE="${state}" \
      CURL_FIXTURE_ALLOW_INSECURE="${CURL_FIXTURE_ALLOW_INSECURE:-0}" \
      VALKEY_PASSWORD="fixture-not-a-secret" \
      bash "./scripts/post-reboot-verify.sh" 2>&1
  )"
  ec=$?
  set -e
  printf '%s\n' "${out}" > "${state}/stdout.txt"
  printf '%s\n' "${ec}" > "${state}/exit.txt"

  if [[ "${ec}" -eq 0 ]]; then
    fail "${name}: expected nonzero failure on excess stream"
    return 0
  fi
  if ! grep -q 'VERIFICATION RESULT: .*FAILED' <<<"${out}"; then
    fail "${name}: missing FAILED verification result"
    return 0
  fi
  if grep -q 'Sensor heartbeat transport failed' <<<"${out}"; then
    fail "${name}: excess ceiling cancel misclassified as transport failure"
    return 0
  fi
  if ! grep -q 'authority/schema validation (code=2)' <<<"${out}"; then
    fail "${name}: expected byte-ceiling validation code=2"
    return 0
  fi

  # head stub must retain at most MAX+1 bytes (download-path ceiling).
  if [ ! -f "${state}/head-retained.bytes" ]; then
    fail "${name}: missing head retained-byte evidence (download ceiling not applied)"
    return 0
  fi
  local retained
  retained="$(tr -d '[:space:]' < "${state}/head-retained.bytes")"
  if [ "${retained}" -gt $((MAX_HEARTBEAT_BYTES + 1)) ]; then
    fail "${name}: retained tempfile bytes ${retained} exceed MAX+1"
    return 0
  fi
  if [ "${retained}" -le "${MAX_HEARTBEAT_BYTES}" ]; then
    fail "${name}: expected retained MAX+1 overflow sentinel, got ${retained}"
    return 0
  fi
  grep -Eq -- '-c[[:space:]]+'"$((MAX_HEARTBEAT_BYTES + 1))" "${state}/head-invocations.log" \
    || fail "${name}: head not invoked with -c MAX+1"

  # Producer must cancel well before the full 2MiB attempt (pipe buffer slack OK).
  if [ ! -f "${state}/curl-stream.log" ]; then
    fail "${name}: missing curl stream log"
    return 0
  fi
  # shellcheck disable=SC1090
  source "${state}/curl-stream.log"
  if [ "${CANCELLED:-0}" != "1" ]; then
    fail "${name}: excess producer was not cancelled (CANCELLED=${CANCELLED:-unset})"
    return 0
  fi
  if [ "${BYTES_WRITTEN:-0}" -ge "${ATTEMPT_BYTES:-2097152}" ]; then
    fail "${name}: producer wrote full attempt (${BYTES_WRITTEN}) — unbounded"
    return 0
  fi
  # Bound producer progress: retained ceiling + one typical Linux pipe buffer.
  if [ "${BYTES_WRITTEN:-0}" -gt $((MAX_HEARTBEAT_BYTES + 1 + 65536)) ]; then
    fail "${name}: producer wrote ${BYTES_WRITTEN} before cancel — not adequately bounded"
    return 0
  fi
  pass "${name} (cancelled; retained=${retained}; written=${BYTES_WRITTEN})"
}
run_case_stream_excess

# Missing CA: remove trust file after fixture setup
run_case_missing_ca() {
  local name="missing-ca"
  local state="${WORK}/state/${name}"
  rm -rf "${state}"
  mkdir -p "${state}"
  printf '%s' "${VALID_HEARTBEAT}" > "${state}/hb.body"
  write_scenario "${state}/scenario.env" \
    "UI_HTTP=200" "UI_CURL_EC=0" \
    "CORE_HTTP=200" "CORE_CURL_EC=0" \
    "HB_HTTP=200" "HB_CURL_EC=0" \
    "HB_BODY_FILE=${state}/hb.body" \
    "UI_BODY_FILE=" \
    "CORE_BODY_FILE="

  rm -f "${FIXTURE_ROOT}/deploy/docker/certs/ca.crt"

  local out ec
  set +e
  out="$(
    cd "${FIXTURE_ROOT}"
    env -i \
      PATH="${STUB_BIN}:/usr/local/bin:/usr/bin:/bin" \
      HOME="${WORK}/home" \
      POST_REBOOT_FIXTURE_STATE="${state}" \
      CURL_FIXTURE_ALLOW_INSECURE="${CURL_FIXTURE_ALLOW_INSECURE:-0}" \
      VALKEY_PASSWORD="fixture-not-a-secret" \
      bash "./scripts/post-reboot-verify.sh" 2>&1
  )"
  ec=$?
  set -e
  printf '%s\n' "${out}" > "${state}/stdout.txt"
  printf '%s\n' "${ec}" > "${state}/exit.txt"

  # Restore CA for any later local debugging of the workdir (trap cleans anyway).
  printf 'FIXTURE-CA\n' > "${FIXTURE_ROOT}/deploy/docker/certs/ca.crt"

  if [[ "${ec}" -eq 0 ]]; then
    fail "${name}: expected nonzero failure when CA missing"
    return 0
  fi
  if ! grep -q 'VERIFICATION RESULT: .*FAILED' <<<"${out}"; then
    fail "${name}: missing FAILED verification result"
    return 0
  fi
  if [ "${CURL_FIXTURE_ALLOW_INSECURE:-0}" != "1" ]; then
    if grep -F "${VALID_HEARTBEAT}" <<<"${out}" >/dev/null 2>&1; then
      fail "${name}: verifier leaked exact heartbeat body"
      return 0
    fi
  fi
  pass "${name} (correctly rejected)"
}
run_case_missing_ca

# Enforce curl construction on the positive path: --cacert present, no insecure flags.
# Skipped only during intentional failing-first reproduction against the historical -k verifier.
POS_LOG="${WORK}/state/valid-exact-authority/curl-invocations.log"
[[ -f "${POS_LOG}" ]] || fail "missing curl invocation log for positive case"
if [ "${CURL_FIXTURE_ALLOW_INSECURE:-0}" = "1" ]; then
  grep -Eq -- '(^|[[:space:]])(-k|--insecure)([[:space:]]|$)' "${POS_LOG}" \
    || fail "failing-first expected historical -k usage on base verifier"
  pass "failing-first recorded historical insecure curl construction"
else
  grep -q -- '--cacert' "${POS_LOG}" || fail "positive case curl missing --cacert"
  if grep -Eq -- '(^|[[:space:]])(-k|--insecure)([[:space:]]|$)' "${POS_LOG}"; then
    fail "positive case curl used insecure TLS flag"
  fi
  cacert_count="$(grep -c -- '--cacert' "${POS_LOG}" || true)"
  [[ "${cacert_count}" -ge 3 ]] || fail "expected >=3 --cacert uses, got ${cacert_count}"
  pass "curl fixture enforced --cacert and rejected insecure flags"

  # UI/Core status probes must discard bodies (/dev/null); heartbeat retains.
  POS_DISPOSITION="${WORK}/state/valid-exact-authority/curl-body-disposition.log"
  if [ -f "${POS_DISPOSITION}" ]; then
    discard_count="$(grep -c 'DISCARD_BODY=1' "${POS_DISPOSITION}" || true)"
    [[ "${discard_count}" -ge 2 ]] || fail "expected >=2 status-only body discards, got ${discard_count}"
    pass "UI/Core status probes discarded response bodies"
  else
    fail "missing curl body-disposition log for status-only discards"
  fi

  POS_HEAD_LOG="${WORK}/state/valid-exact-authority/head-invocations.log"
  [[ -f "${POS_HEAD_LOG}" ]] || fail "positive case missing head download-ceiling invocation"
  grep -Eq -- '-c[[:space:]]+'"$((MAX_HEARTBEAT_BYTES + 1))" "${POS_HEAD_LOG}" \
    || fail "positive case head not invoked with -c MAX+1"
  POS_RETAINED="${WORK}/state/valid-exact-authority/head-retained.bytes"
  [[ -f "${POS_RETAINED}" ]] || fail "positive case missing head retained-byte evidence"
  pos_retained="$(tr -d '[:space:]' < "${POS_RETAINED}")"
  [[ "${pos_retained}" -le "${MAX_HEARTBEAT_BYTES}" ]] \
    || fail "positive case retained ${pos_retained} exceeds MAX"
  pass "heartbeat download path applied head -c ceiling (retained=${pos_retained})"
fi

echo "========================================================================"
if [ "${HARNESS_FAILED}" -ne 0 ]; then
  echo "POST-REBOOT VERIFY FIXTURE HARNESS FAILED"
  echo "========================================================================"
  exit 1
fi
echo "POST-REBOOT VERIFY FIXTURE HARNESS PASSED"
echo "========================================================================"
