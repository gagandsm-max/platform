#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CERT_DIR="${REPO_ROOT}/deploy/docker/certs"

mkdir -p "${CERT_DIR}"

if [[ -f "${CERT_DIR}/platform-dev.crt" && -f "${CERT_DIR}/platform-dev.key" ]]; then
    echo "Development TLS certificates already exist in ${CERT_DIR}."
    exit 0
fi

echo "Generating local development TLS certificates..."

# Generate Root CA
openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "${CERT_DIR}/ca.key" \
    -out "${CERT_DIR}/ca.crt" \
    -days 365 \
    -subj "/C=US/ST=Dev/L=Dev/O=Platform/OU=Engineering/CN=Platform-Dev-Root-CA" \
    2>/dev/null

# Generate Server CSR & Key
openssl req -new -newkey rsa:2048 -nodes \
    -keyout "${CERT_DIR}/platform-dev.key" \
    -out "${CERT_DIR}/platform-dev.csr" \
    -subj "/C=US/ST=Dev/L=Dev/O=Platform/OU=Engineering/CN=localhost" \
    2>/dev/null

# Create OpenSSL extension config for SAN
cat <<EOF > "${CERT_DIR}/san.cnf"
[v3_req]
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = core
DNS.3 = ui
DNS.4 = sensor-net
DNS.5 = sensor-infra
IP.1 = 127.0.0.1
EOF

# Sign Server Certificate with Root CA
openssl x509 -req \
    -in "${CERT_DIR}/platform-dev.csr" \
    -CA "${CERT_DIR}/ca.crt" \
    -CAkey "${CERT_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERT_DIR}/platform-dev.crt" \
    -days 365 \
    -extfile "${CERT_DIR}/san.cnf" \
    -extensions v3_req \
    2>/dev/null

# Cleanup CSR and temporary files
rm -f "${CERT_DIR}/platform-dev.csr" "${CERT_DIR}/san.cnf" "${CERT_DIR}/ca.srl"

# Set permissions
chmod 600 "${CERT_DIR}"/*.key
chmod 644 "${CERT_DIR}"/*.crt

echo "Development TLS certificates generated successfully in ${CERT_DIR}."
