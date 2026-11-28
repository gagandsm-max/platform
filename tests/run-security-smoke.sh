#!/usr/bin/env bash
# ==============================================================================
# Platform Security Smoke Verification Suite
# Governing Spec: Platform_18_Security_Threat_Model.md
# ==============================================================================
mtls_fixture_error() {
    printf '[ERROR] mTLS fixture %s: %s\n' "$1" "$2" >&2
    return 1
}

mtls_validate_fixtures() {
    local dir=$1 name cert key cert_public key_public ca verification rc=0
    for name in ca untrusted-ca sensor-net sensor-infra untrusted-client; do
        cert="$dir/$name.crt"
        [[ -f $cert && -r $cert && -s $cert ]] &&
            openssl x509 -in "$cert" -noout >/dev/null 2>&1 ||
            { mtls_fixture_error "$name.crt" 'missing, unreadable or invalid certificate'; return 1; }
    done
    for name in sensor-net sensor-infra untrusted-client; do
        key="$dir/$name.key"
        [[ -f $key && -r $key && -s $key ]] &&
            key_public=$(openssl pkey -in "$key" -passin pass: -pubout 2>/dev/null) &&
            [[ -n $key_public ]] ||
            { mtls_fixture_error "$name.key" 'missing, unreadable or invalid private key'; return 1; }
        cert_public=$(openssl x509 -in "$dir/$name.crt" -noout -pubkey 2>/dev/null) &&
            [[ $cert_public == "$key_public" ]] ||
            { mtls_fixture_error "$name" 'certificate and key do not match'; return 1; }
        ca="$dir/ca.crt"
        if [[ $name == untrusted-client ]]; then
            ca="$dir/untrusted-ca.crt"
        fi
        # -trusted confines verification to this fixture CA.
        openssl verify -purpose sslclient -trusted "$ca" "$dir/$name.crt" >/dev/null 2>&1 ||
            { mtls_fixture_error "$name" 'verification against intended CA failed'; return 1; }
    done
    verification=$(LC_ALL=C openssl verify -purpose sslclient -trusted "$dir/ca.crt" \
        "$dir/untrusted-client.crt" 2>&1) || rc=$?
    if [[ $rc == 0 ]]; then
        mtls_fixture_error untrusted-client 'trusted by the development CA'
    elif [[ $rc == 2 ]] &&
        grep -qx 'error 20 at 0 depth lookup: unable to get local issuer certificate' <<< "$verification"; then
        return 0
    else
        mtls_fixture_error untrusted-client 'development CA issuer rejection could not be established'
    fi
}

mtls_json_field() {
    python3 -c '
import json
import sys

try:
    body = json.load(sys.stdin)
except (ValueError, UnicodeError):
    sys.exit(1)
sys.exit(not isinstance(body, dict) or body.get(sys.argv[1]) not in sys.argv[2:])
' "$@" < "$probe_dir/body"
}

mtls_heartbeat_probe() {
    local fixture=$1 identity=$2 rc=0 exit_code=0 status reported_status alert
    local -a client=()
    if [[ -n $fixture ]]; then
        client=(--cert "$cert_dir/$fixture.crt" --key "$cert_dir/$fixture.key")
    fi
    : > "$probe_dir/body" || return 1
    # Preserve trailing whitespace so malformed status output cannot become valid.
    status=$(
        LC_ALL=C curl --disable --proxy '' --noproxy '*' --proto '=https' \
            --connect-timeout 3 --max-time 10 --silent --show-error \
            --cacert "$cert_dir/ca.crt" "${client[@]}" \
            --output "$probe_dir/body" --write-out '%{http_code}' \
            --header 'Content-Type: application/json' --data "{\"sensor_id\":\"$identity\"}" \
            "https://127.0.0.1:$core_port/api/v1/sensor/heartbeat" 2>"$probe_dir/diagnostic" || exit_code=$?
        printf '.'
        exit "$exit_code"
    ) || rc=$?
    status=${status%.}

    if [[ $fixture == untrusted-client ]]; then
        # Direct verified HTTPS plus a Core authorization error attributes the 403
        # to the heartbeat request with the rogue fixture.
        if [[ $rc == 0 && $status == 403 ]] &&
            mtls_json_field code ERR_SENSOR_CREDENTIAL_UNKNOWN; then
            echo '[OK] Rogue heartbeat rejected with HTTP 403.'
            return 0
        fi
        # Match only received OpenSSL alerts with certificate-specific reason codes.
        alert='((0A000418|14094418|1408F418):SSL routines:[^:]*:tlsv1 alert unknown ca'
        alert+='|(0A000416|14094416|1408F416):SSL routines:[^:]*:sslv3 alert certificate unknown'
        alert+='|(0A000412|14094412|1408F412):SSL routines:[^:]*:sslv3 alert bad certificate)'
        if [[ $status == 000 && ( $rc == 35 || $rc == 56 ) ]] &&
            grep -Eq "^curl: \($rc\) (OpenSSL SSL_(connect|read): )?(OpenSSL/[0-9]+[.][0-9]+[.][0-9]+: )?error:$alert(, errno [0-9]+| in connection to 127[.]0[.]0[.]1:[0-9]+[[:blank:]]*)?$" "$probe_dir/diagnostic"; then
            echo '[OK] Rogue heartbeat rejected by peer TLS client-certificate alert.'
            return 0
        fi
    elif [[ -z $fixture ]]; then
        if [[ $rc == 0 && $status == 401 ]]; then
            echo '[OK] Heartbeat without a client certificate returned HTTP 401.'
            return 0
        fi
    elif [[ $rc == 0 && $status == 200 ]] && mtls_json_field sensor_identity "$identity"; then
        printf '[OK] Trusted heartbeat returned HTTP 200 for %s.\n' "$identity"
        return 0
    fi

    # Raw response and diagnostic can contain arbitrary data. Log only fixed
    # labels, curl's exit code and a validated status.
    reported_status=malformed
    if [[ -z $status ]]; then
        reported_status=empty
    elif [[ $status =~ ^[0-9]{3}$ ]]; then
        reported_status=$status
    fi
    printf '[ERROR] mTLS heartbeat %s: expected evidence absent (curl_exit=%s http=%s).\n' \
        "${fixture:-no-client-cert}" "$rc" "$reported_status" >&2
    return 1
}

mtls_heartbeat_guard() (
    set +x
    local cert_dir=$1 core_port=$2 probe_dir
    mtls_validate_fixtures "$cert_dir" || return 1
    probe_dir=$(mktemp -d) || return 1
    trap 'rm -rf -- "$probe_dir"' EXIT
    mtls_heartbeat_probe sensor-net sensor-net-01 &&
        mtls_heartbeat_probe sensor-infra sensor-infra-01 &&
        mtls_heartbeat_probe '' sensor-anonymous &&
        mtls_heartbeat_probe untrusted-client sensor-rogue &&
        mtls_heartbeat_probe sensor-net sensor-net-01 &&
        mtls_heartbeat_probe sensor-infra sensor-infra-01
)

ui_port80_absent() {
    local before after id listeners
    local format='{{.Id}}|{{.Name}}|{{.State.Running}}'
    local -a bounded=(timeout --signal=TERM --kill-after=2 8 docker)
    before=$("${bounded[@]}" inspect --type container --format "$format" platform-ui) || {
        echo '[ERROR] UI container inspection failed.' >&2
        return 1
    }
    if [[ ! $before =~ ^([0-9a-f]{64})[|]/platform-ui[|]true$ ]]; then
        echo '[ERROR] Expected the exact running platform-ui container and its full ID.' >&2
        return 1
    fi
    id=${BASH_REMATCH[1]}
    listeners=$("${bounded[@]}" exec "$id" busybox netstat -lnt 2>&1) || {
        echo '[ERROR] UI listener inventory command failed.' >&2
        return 1
    }
    after=$("${bounded[@]}" inspect --type container --format "$format" platform-ui) || {
        echo '[ERROR] UI container reinspection failed.' >&2
        return 1
    }
    if [[ $after != "$before" ]]; then
        echo '[ERROR] UI container identity or running state changed during listener inspection.' >&2
        return 1
    fi

    # The nginx:1.27-alpine runtime uses BusyBox netstat. Its numeric IPv6
    # endpoints use the final colon as the port separator, e.g. :::443.
    local status=0
    LC_ALL=C awk '
        function ipv4(address, octets, n, i) {
            n = split(address, octets, /[.]/)
            if (n != 4) return 0
            for (i = 1; i <= n; i++)
                if (octets[i] !~ /^[0-9]+$/ || length(octets[i]) > 3 || octets[i] + 0 > 255) return 0
            return 1
        }
        function endpoint(value, remote, address, port, groups, n, i, compressed, tail) {
            if (value !~ /:[0-9]+$/ && !(remote && value ~ /:[*]$/)) return 0
            port = value
            sub(/^.*:/, "", port)
            if (port != "*" && (length(port) > 5 || port + 0 > 65535)) return 0
            address = value
            sub(/:[^:]*$/, "", address)
            if (ipv4(address)) return 1
            if (address ~ /^\[[^]]+\]$/) address = substr(address, 2, length(address) - 2)
            if (address !~ /^[0-9a-fA-F:.]+$/ || address !~ /:/ || address ~ /:::/) return 0
            if (address ~ /[.]/) {
                tail = address
                sub(/^.*:/, "", tail)
                if (!ipv4(tail)) return 0
                sub(/[^:]+$/, "0:0", address)
            }
            compressed = sub(/::/, ":%:", address)
            if (address ~ /::|^:[^%]|[^%]:$/) return 0
            sub(/^:/, "", address)
            sub(/:$/, "", address)
            n = split(address, groups, ":")
            for (i = 1; i <= n; i++)
                if (groups[i] != "%" && (groups[i] !~ /^[0-9a-fA-F]+$/ || length(groups[i]) > 4)) return 0
            return compressed ? n <= 8 : n == 8
        }
        NR == 1 { if ($0 != "Active Internet connections (only servers)") bad = 1; next }
        NR == 2 {
            if ($0 !~ /^Proto +Recv-Q +Send-Q +Local Address +Foreign Address +State *$/) bad = 1
            next
        }
        NF != 6 || $1 !~ /^tcp6?$/ || $2 !~ /^[0-9]+$/ || $3 !~ /^[0-9]+$/ ||
            $6 != "LISTEN" || !endpoint($4, 0) || !endpoint($5, 1) { bad = 1; next }
        { count++; if ($4 ~ /:0*80$/) port80 = 1 }
        END { if (bad || !count) exit 1; if (port80) exit 2 }
    ' <<< "$listeners" || status=$?
    case $status in
        0) return 0 ;;
        2) echo '[ERROR] UI container is listening on TCP port 80.' >&2 ;;
        *) echo '[ERROR] UI listener inventory is empty or malformed.' >&2 ;;
    esac
    return 1
}

if [[ ${BASH_SOURCE[0]} != "$0" ]]; then
    return 0
fi

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

source .env 2>/dev/null || true
CORE_PORT="${PLATFORM_CORE_HOST_PORT:-28443}"
UI_PORT="${PLATFORM_UI_HOST_PORT:-23000}"
CERT_DIR="${ROOT_DIR}/deploy/docker/certs"

echo "========================================================================"
echo "Running Platform Security Smoke Suite"
echo "========================================================================"

# 1. Cleartext HTTP rejection on Core port
echo "--> [1/4] Verifying Core cleartext rejection..."
CLEARTEXT_RESP=$(curl -s --connect-timeout 2 "http://127.0.0.1:${CORE_PORT}/api/v1/version" 2>&1 || true)
if echo "${CLEARTEXT_RESP}" | grep -q "requires TLS\|400 Bad Request"; then
    echo "[OK] Core rejects cleartext HTTP with 400 Bad Request."
else
    echo "[ERROR] Core unexpectedly accepted cleartext HTTP: ${CLEARTEXT_RESP}"
    exit 1
fi

# 2. Port 80 absence in UI container
echo "--> [2/4] Verifying UI port 80 listener absence..."
ui_port80_absent
echo "[OK] UI has no TCP port 80 listener."

# 3. mTLS enforcement on Core ingestion
echo "--> [3/4] Verifying mTLS enforcement and untrusted cert rejection..."
mtls_heartbeat_guard "$CERT_DIR" "$CORE_PORT"

# 4. Developer-host isolation policy plus synthetic detector verification.
# GitHub-hosted CI runners intentionally contain preinstalled language runtimes.
# Platform code/tests still run through Docker; therefore CI validates the detector
# itself without misclassifying the runner image as a developer-policy violation.
echo "--> [4/4] Verifying host runtime isolation policy and synthetic negative detector..."
if [ "${PLATFORM_CI:-0}" = "1" ]; then
    echo "[INFO] PLATFORM_CI=1: skipping ephemeral runner PATH policy enforcement; running detector regression only."
else
    bash "${ROOT_DIR}/scripts/check-host-runtime-isolation.sh"
fi
bash "${ROOT_DIR}/scripts/check-host-runtime-isolation.sh" --synthetic-test

echo "========================================================================"
echo "SECURITY SMOKE SUITE PASSED SUCCESSFULLY"
echo "========================================================================"
