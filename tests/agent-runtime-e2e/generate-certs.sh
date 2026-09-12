#!/usr/bin/env bash
# Test-only PKI for Agent runtime E2E. Uses the existing Platform-Dev CA.
# Does not replace Sensor→Core client identity (SENSOR_AGENT_SERVER_* must differ).
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CA_DIR="${ROOT_DIR}/deploy/docker/certs"
OUT_DIR="${ROOT_DIR}/tests/agent-runtime-e2e/certs"

if [[ ! -f "${CA_DIR}/ca.crt" || ! -f "${CA_DIR}/ca.key" ]]; then
    echo "[ERROR] Platform CA missing; run 'make certs' first" >&2
    exit 1
fi

mkdir -p "${OUT_DIR}"
rm -rf "${OUT_DIR}/ca-tmp"

sign_leaf() {
    local name="$1"
    local cn="$2"
    local extfile="$3"
    local days="${4:-365}"

    openssl req -new -newkey rsa:2048 -nodes \
        -keyout "${OUT_DIR}/${name}.key" \
        -out "${OUT_DIR}/${name}.csr" \
        -subj "/C=US/ST=Dev/L=Dev/O=Platform/OU=Agents/CN=${cn}"

    openssl x509 -req \
        -in "${OUT_DIR}/${name}.csr" \
        -CA "${CA_DIR}/ca.crt" \
        -CAkey "${CA_DIR}/ca.key" \
        -CAcreateserial \
        -out "${OUT_DIR}/${name}.crt" \
        -days "${days}" \
        -extfile "${extfile}" \
        -extensions v3_req
}

# OpenSSL 3.0 x509 -req has no -startdate/-not_before. Use openssl ca for dated leaves.
sign_dated_leaf() {
    local name="$1"
    local cn="$2"
    local extfile="$3"
    local startdate="$4"
    local enddate="$5"

    openssl req -new -newkey rsa:2048 -nodes \
        -keyout "${OUT_DIR}/${name}.key" \
        -out "${OUT_DIR}/${name}.csr" \
        -subj "/C=US/ST=Dev/L=Dev/O=Platform/OU=Agents/CN=${cn}"

    local cadir="${OUT_DIR}/ca-tmp/${name}"
    mkdir -p "${cadir}"
    touch "${cadir}/index.txt"
    echo "unique_subject = no" > "${cadir}/index.txt.attr"
    echo "1000" > "${cadir}/serial"

    cat > "${cadir}/ca.cnf" <<EOF
[ca]
default_ca = CA_default

[CA_default]
database = ${cadir}/index.txt
serial = ${cadir}/serial
new_certs_dir = ${cadir}
certificate = ${CA_DIR}/ca.crt
private_key = ${CA_DIR}/ca.key
default_md = sha256
policy = policy_anything
unique_subject = no
x509_extensions = v3_req
copy_extensions = none
default_days = 1

[policy_anything]
countryName = optional
stateOrProvinceName = optional
organizationName = optional
organizationalUnitName = optional
commonName = supplied
EOF

    openssl ca -batch -notext \
        -config "${cadir}/ca.cnf" \
        -startdate "${startdate}" \
        -enddate "${enddate}" \
        -in "${OUT_DIR}/${name}.csr" \
        -out "${OUT_DIR}/${name}.crt" \
        -extfile "${extfile}" \
        -extensions v3_req
}

cat > "${OUT_DIR}/agent-server.cnf" <<'EOF'
[v3_req]
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = sensor-infra
DNS.2 = platform-sensor-infra
DNS.3 = localhost
IP.1 = 127.0.0.1
EOF

cat > "${OUT_DIR}/agent-client.cnf" <<'EOF'
[v3_req]
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = agent-e2e-01
EOF

sign_leaf "sensor-infra-agent-server" "sensor-infra-agent-server.platform.local" "${OUT_DIR}/agent-server.cnf" 365
sign_leaf "agent-e2e" "agent-e2e-01.agent.platform.local" "${OUT_DIR}/agent-client.cnf" 365

# Handshake-negative leaves signed by the real CA.
sign_dated_leaf "agent-e2e-expired" "agent-e2e-expired.agent.platform.local" "${OUT_DIR}/agent-client.cnf" 20200101000000Z 20200102000000Z
sign_dated_leaf "agent-e2e-notyet" "agent-e2e-notyet.agent.platform.local" "${OUT_DIR}/agent-client.cnf" 20400101000000Z 20401231000000Z

python3 - <<PY
import json, subprocess, hashlib
from pathlib import Path
out = Path("${OUT_DIR}")
ca = Path("${CA_DIR}") / "ca.crt"

def fp(path):
    der = subprocess.check_output(["openssl", "x509", "-in", str(path), "-outform", "DER"])
    return hashlib.sha256(der).hexdigest()

def serial(path):
    raw = subprocess.check_output(["openssl", "x509", "-in", str(path), "-noout", "-serial"], text=True)
    hexpart = raw.split("=", 1)[1].strip().lower()
    return format(int(hexpart, 16), "x")

def issuer(path):
    return subprocess.check_output(
        ["openssl", "x509", "-in", str(path), "-noout", "-issuer", "-nameopt", "RFC2253"],
        text=True,
    ).split("=", 1)[1].strip()

def validity(path, which):
    opt = "-startdate" if which == "not_before" else "-enddate"
    raw = subprocess.check_output(["openssl", "x509", "-in", str(path), "-noout", opt], text=True)
    return raw.split("=", 1)[1].strip()

agent = out / "agent-e2e.crt"
expired = out / "agent-e2e-expired.crt"
notyet = out / "agent-e2e-notyet.crt"
meta = {
    "agent_cert": str(agent),
    "fingerprint_sha256": fp(agent),
    "issuer_fingerprint_sha256": fp(ca),
    "serial_number": serial(agent),
    "issuer_dn": issuer(agent),
    "not_before_openssl": validity(agent, "not_before"),
    "not_after_openssl": validity(agent, "not_after"),
    "expired_not_before": validity(expired, "not_before"),
    "expired_not_after": validity(expired, "not_after"),
    "notyet_not_before": validity(notyet, "not_before"),
    "notyet_not_after": validity(notyet, "not_after"),
}
(out / "metadata.json").write_text(json.dumps(meta, indent=2) + "\n")
print("fingerprint", meta["fingerprint_sha256"])
print("expired window", meta["expired_not_before"], "->", meta["expired_not_after"])
print("notyet window", meta["notyet_not_before"], "->", meta["notyet_not_after"])
PY

# Bind-mounts are owned by the host user; container uid 100 needs world-read on test-only keys.
chmod 644 "${OUT_DIR}"/*.crt "${OUT_DIR}"/*.key
rm -f "${OUT_DIR}"/*.csr "${OUT_DIR}"/*.srl
rm -rf "${OUT_DIR}/ca-tmp"
echo "[OK] Agent E2E certs written to ${OUT_DIR}"
