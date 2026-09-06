#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CERT_DIR="${REPO_ROOT}/deploy/docker/certs"

mkdir -p "${CERT_DIR}"

# Passwords for Java PKCS12 keystore & truststore
KEYSTORE_PASS="platform_dev_keystore_pass"
TRUSTSTORE_PASS="platform_dev_truststore_pass"

echo "==> Generating Platform development PKI for HTTPS and mTLS..."

# 1. Platform Root CA
echo "--> [1/6] Generating Platform Root CA..."
openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "${CERT_DIR}/ca.key" \
    -out "${CERT_DIR}/ca.crt" \
    -days 365 \
    -subj "/C=US/ST=Dev/L=Dev/O=Platform/OU=Engineering/CN=Platform-Dev-Root-CA" \
    2>/dev/null

# 2. Core Server Certificate (with SANs for local development and docker networking)
echo "--> [2/6] Generating Core Server TLS Certificate..."
cat <<EOF > "${CERT_DIR}/core_san.cnf"
[v3_req]
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = localhost
DNS.2 = core
DNS.3 = platform-core
DNS.4 = ui
DNS.5 = platform-ui
IP.1 = 127.0.0.1
EOF

openssl req -new -newkey rsa:2048 -nodes \
    -keyout "${CERT_DIR}/core.key" \
    -out "${CERT_DIR}/core.csr" \
    -subj "/C=US/ST=Dev/L=Dev/O=Platform/OU=Engineering/CN=core" \
    2>/dev/null

openssl x509 -req \
    -in "${CERT_DIR}/core.csr" \
    -CA "${CERT_DIR}/ca.crt" \
    -CAkey "${CERT_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERT_DIR}/core.crt" \
    -days 365 \
    -extfile "${CERT_DIR}/core_san.cnf" \
    -extensions v3_req \
    2>/dev/null

# Maintain platform-dev.crt/key for UI Nginx server certificate
cp "${CERT_DIR}/core.crt" "${CERT_DIR}/platform-dev.crt"
cp "${CERT_DIR}/core.key" "${CERT_DIR}/platform-dev.key"

# 3. Java PKCS12 Keystore & Truststore for Core
echo "--> [3/6] Packaging Java PKCS12 Keystore & Truststore for Core..."
openssl pkcs12 -export \
    -in "${CERT_DIR}/core.crt" \
    -inkey "${CERT_DIR}/core.key" \
    -out "${CERT_DIR}/core-keystore.p12" \
    -name core \
    -passout "pass:${KEYSTORE_PASS}" \
    2>/dev/null

# Use Temurin Docker container to import CA into PKCS12 truststore without requiring host keytool
rm -f "${CERT_DIR}/core-truststore.p12"
docker run --rm \
    -u "$(id -u):$(id -g)" \
    -v "${CERT_DIR}:/certs" \
    -w /certs \
    eclipse-temurin:21-jre-alpine \
    keytool -importcert -noprompt \
    -alias platform-root-ca \
    -file /certs/ca.crt \
    -keystore /certs/core-truststore.p12 \
    -storepass "${TRUSTSTORE_PASS}" \
    -storetype PKCS12 > /dev/null 2>&1

# 4. Network Sensor Client Certificate (mTLS Identity)
echo "--> [4/6] Generating Network Sensor mTLS Client Certificate..."
openssl req -new -newkey rsa:2048 -nodes \
    -keyout "${CERT_DIR}/sensor-net.key" \
    -out "${CERT_DIR}/sensor-net.csr" \
    -subj "/C=US/ST=Dev/L=Dev/O=Platform/OU=Sensors/CN=sensor-net-01.sensor.platform.local" \
    2>/dev/null

openssl x509 -req \
    -in "${CERT_DIR}/sensor-net.csr" \
    -CA "${CERT_DIR}/ca.crt" \
    -CAkey "${CERT_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERT_DIR}/sensor-net.crt" \
    -days 365 \
    2>/dev/null

# 5. Infrastructure Sensor Client Certificate (mTLS Identity)
echo "--> [5/6] Generating Infrastructure Sensor mTLS Client Certificate..."
openssl req -new -newkey rsa:2048 -nodes \
    -keyout "${CERT_DIR}/sensor-infra.key" \
    -out "${CERT_DIR}/sensor-infra.csr" \
    -subj "/C=US/ST=Dev/L=Dev/O=Platform/OU=Sensors/CN=sensor-infra-01.sensor.platform.local" \
    2>/dev/null

openssl x509 -req \
    -in "${CERT_DIR}/sensor-infra.csr" \
    -CA "${CERT_DIR}/ca.crt" \
    -CAkey "${CERT_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERT_DIR}/sensor-infra.crt" \
    -days 365 \
    2>/dev/null

# 6. Untrusted Rogue CA & Certificate (Strictly for Negative Acceptance Testing)
echo "--> [6/6] Generating Untrusted Test Certificate for Negative Verification..."
openssl req -x509 -newkey rsa:2048 -nodes \
    -keyout "${CERT_DIR}/untrusted-ca.key" \
    -out "${CERT_DIR}/untrusted-ca.crt" \
    -days 365 \
    -subj "/C=US/ST=Dev/L=Dev/O=Untrusted/OU=Rogue/CN=Untrusted-Rogue-CA" \
    2>/dev/null

openssl req -new -newkey rsa:2048 -nodes \
    -keyout "${CERT_DIR}/untrusted-client.key" \
    -out "${CERT_DIR}/untrusted-client.csr" \
    -subj "/C=US/ST=Dev/L=Dev/O=Untrusted/OU=Rogue/CN=untrusted-rogue-sensor.local" \
    2>/dev/null

openssl x509 -req \
    -in "${CERT_DIR}/untrusted-client.csr" \
    -CA "${CERT_DIR}/untrusted-ca.crt" \
    -CAkey "${CERT_DIR}/untrusted-ca.key" \
    -CAcreateserial \
    -out "${CERT_DIR}/untrusted-client.crt" \
    -days 365 \
    2>/dev/null

# Cleanup CSRs and temp files
rm -f "${CERT_DIR}"/*.csr "${CERT_DIR}"/*.cnf "${CERT_DIR}"/*.srl

# Secure file permissions (readable by non-root container users)
chmod 644 "${CERT_DIR}"/*.key "${CERT_DIR}"/*.p12 "${CERT_DIR}"/*.crt

echo "==> Development PKI and mTLS credentials successfully generated in ${CERT_DIR}."
