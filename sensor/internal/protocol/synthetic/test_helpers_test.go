package synthetic

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func testLimits() Limits {
	return Limits{
		MaxTimeout: 2 * time.Second, MaxSchedulerPayloadBytes: 16 << 10, MaxResultBytes: 16 << 10,
		MaxTargetBytes: 512, MaxURLBytes: 2048, MaxDNSAnswers: 8, MaxRedirects: 4,
		MaxResponseBytes: 4096, MaxExpectedStatuses: 8, MaxExpectedContentBytes: 1024,
		MaxHeaders: 16, MaxHeaderNameBytes: 128, MaxHeaderValueBytes: 1024,
		MaxCABundleBytes: 32 << 10, MaxCertificateChain: 4, MaxSANs: 16, MaxDiagnosticTextBytes: 1024,
	}
}

func newTestCollector(t *testing.T) *Collector {
	t.Helper()
	c, err := NewCollector(testLimits())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type pki struct{ caPEM, certPEM, keyPEM []byte }

func makePKI(t *testing.T, expired bool) pki {
	t.Helper()
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caT := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "synthetic-test-ca"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, caT, caT, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	notBefore, notAfter := now.Add(-time.Hour), now.Add(time.Hour)
	if expired {
		notBefore = now.Add(-2 * time.Hour)
		notAfter = now.Add(-time.Hour)
	}
	leafT := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: notBefore, NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafT, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatal(err)
	}
	return pki{caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), keyPEM: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})}
}

func startTLSServer(t *testing.T, p pki, handler http.Handler) (*httptest.Server, int) {
	t.Helper()
	cert, err := tls.X509KeyPair(p.certPEM, p.keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewUnstartedServer(handler)
	s.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	s.StartTLS()
	t.Cleanup(s.Close)
	_, portStr, err := net.SplitHostPort(s.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return s, port
}

func waitForContext(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
