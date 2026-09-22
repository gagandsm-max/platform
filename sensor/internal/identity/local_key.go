package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
)

// generateLocalEnrollmentKey creates an ECDSA P-256 keypair on the Sensor.
// Returns the key and PKCS#8 PEM. Caller must ZeroBytes the PEM after persistence.
func generateLocalEnrollmentKey() (*ecdsa.PrivateKey, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("SENSOR_IDENTITY_KEYGEN_FAILED: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("SENSOR_IDENTITY_KEYGEN_FAILED: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if len(pemBytes) == 0 {
		return nil, nil, fmt.Errorf("SENSOR_IDENTITY_KEYGEN_FAILED: empty PKCS#8 PEM")
	}
	return key, pemBytes, nil
}

// createEnrollmentCSR builds a PKCS#10 CSR PEM signed by the local private key.
// Subject is intentionally minimal: Core assigns authoritative Sensor identity.
func createEnrollmentCSR(key *ecdsa.PrivateKey) ([]byte, error) {
	if key == nil {
		return nil, fmt.Errorf("SENSOR_IDENTITY_CSR_INVALID: private key is required")
	}
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			// No authoritative Sensor ID / tenant / site claim — Core assigns identity.
			CommonName: "platform-sensor-enrollment",
		},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		return nil, fmt.Errorf("SENSOR_IDENTITY_CSR_FAILED: %w", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
	if len(pemBytes) == 0 {
		return nil, fmt.Errorf("SENSOR_IDENTITY_CSR_FAILED: empty CSR PEM")
	}
	return pemBytes, nil
}

func parseEnrollmentCSR(csrPEM []byte) (*x509.CertificateRequest, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, fmt.Errorf("SENSOR_IDENTITY_CSR_INVALID: PEM certificate request not found")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("SENSOR_IDENTITY_CSR_INVALID: %w", err)
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("SENSOR_IDENTITY_CSR_INVALID: CSR signature is invalid")
	}
	return csr, nil
}
