package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeEnrollmentClient struct {
	response   EnrollmentResponse
	err        error
	seenToken  string
	seenReq    EnrollmentRequest
	calls      int
	signCSR    bool
	sensorID   string
	notBefore  time.Time
	notAfter   time.Time
	noClientAuth bool
	wrongCA      bool
}

func (f *fakeEnrollmentClient) Enroll(_ context.Context, token []byte, req EnrollmentRequest) (EnrollmentResponse, error) {
	f.calls++
	f.seenToken = string(token)
	f.seenReq = req
	if f.err != nil {
		return EnrollmentResponse{}, f.err
	}
	if f.signCSR {
		return issueTestCertificateFromCSR(req.CSRPem, f.sensorID, f.notBefore, f.notAfter, f.noClientAuth, f.wrongCA)
	}
	return f.response, nil
}

type enrollmentMaterial struct {
	Key       *ecdsa.PrivateKey
	KeyPEM    []byte
	CSRPem    []byte
	Response  EnrollmentResponse
	CAKey     *ecdsa.PrivateKey
	CACert    *x509.Certificate
}

func makeEnrollmentMaterial(t *testing.T, sensorID string, notBefore, notAfter time.Time) enrollmentMaterial {
	t.Helper()
	return makeEnrollmentMaterialOpts(t, sensorID, notBefore, notAfter, true, false)
}

func makeEnrollmentMaterialOpts(t *testing.T, sensorID string, notBefore, notAfter time.Time, clientAuth, foreignKey bool) enrollmentMaterial {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             notBefore.Add(-time.Hour),
		NotAfter:              notAfter.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}

	key, keyPEM, err := generateLocalEnrollmentKey()
	if err != nil {
		t.Fatal(err)
	}
	csrPEM, err := createEnrollmentCSR(key)
	if err != nil {
		t.Fatal(err)
	}

	leafKey := key
	if foreignKey {
		leafKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
	}
	eku := []x509.ExtKeyUsage{}
	if clientAuth {
		eku = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	} else {
		eku = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: sensorID},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  eku,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return enrollmentMaterial{
		Key:    key,
		KeyPEM: keyPEM,
		CSRPem: csrPEM,
		Response: EnrollmentResponse{
			SensorID:       sensorID,
			CertificatePEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
			CABundlePEM:    pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}),
		},
		CAKey:  caKey,
		CACert: caCert,
	}
}

func issueTestCertificateFromCSR(csrPEM, sensorID string, notBefore, notAfter time.Time, noClientAuth, wrongCA bool) (EnrollmentResponse, error) {
	csr, err := parseEnrollmentCSR([]byte(csrPEM))
	if err != nil {
		return EnrollmentResponse{}, err
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return EnrollmentResponse{}, err
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             notBefore.Add(-time.Hour),
		NotAfter:              notAfter.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return EnrollmentResponse{}, err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return EnrollmentResponse{}, err
	}
	eku := []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	if noClientAuth {
		eku = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: sensorID},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  eku,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, caCert, csr.PublicKey, caKey)
	if err != nil {
		return EnrollmentResponse{}, err
	}
	caBundle := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	if wrongCA {
		otherKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		otherTemplate := &x509.Certificate{
			SerialNumber:          big.NewInt(99),
			Subject:               pkix.Name{CommonName: "other-ca"},
			NotBefore:             notBefore.Add(-time.Hour),
			NotAfter:              notAfter.Add(time.Hour),
			IsCA:                  true,
			BasicConstraintsValid: true,
			KeyUsage:              x509.KeyUsageCertSign,
		}
		otherDER, err := x509.CreateCertificate(rand.Reader, otherTemplate, otherTemplate, &otherKey.PublicKey, otherKey)
		if err != nil {
			return EnrollmentResponse{}, err
		}
		caBundle = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: otherDER})
	}
	return EnrollmentResponse{
		SensorID:       sensorID,
		CertificatePEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}),
		CABundlePEM:    caBundle,
	}, nil
}

func newTestStore(t *testing.T, now time.Time) *Store {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "identity"))
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	return store
}

func TestBootstrapTokenOneTimeAndRedacted(t *testing.T) {
	token, err := NewBootstrapToken([]byte("test-only-token"))
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{fmt.Sprintf("%v", token), fmt.Sprintf("%+v", token), fmt.Sprintf("%#v", token)} {
		if strings.Contains(rendered, "test-only-token") {
			t.Fatalf("token leaked: %s", rendered)
		}
	}
	value, err := token.Consume()
	if err != nil || string(value) != "test-only-token" {
		t.Fatalf("value=%q err=%v", string(value), err)
	}
	ZeroBytes(value)
	if _, err := token.Consume(); err == nil {
		t.Fatal("token reused")
	}
}

func TestLocalKeyGenerationProducesDistinctKeypairs(t *testing.T) {
	_, pem1, err := generateLocalEnrollmentKey()
	if err != nil {
		t.Fatal(err)
	}
	_, pem2, err := generateLocalEnrollmentKey()
	if err != nil {
		t.Fatal(err)
	}
	if string(pem1) == string(pem2) {
		t.Fatal("two enrollments produced identical private keys")
	}
}

func TestCSRSignatureAndPublicKeyMatchLocalKey(t *testing.T) {
	key, _, err := generateLocalEnrollmentKey()
	if err != nil {
		t.Fatal(err)
	}
	csrPEM, err := createEnrollmentCSR(key)
	if err != nil {
		t.Fatal(err)
	}
	csr, err := parseEnrollmentCSR(csrPEM)
	if err != nil {
		t.Fatal(err)
	}
	pub, ok := csr.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("unexpected public key type %T", csr.PublicKey)
	}
	if !pub.Equal(&key.PublicKey) {
		t.Fatal("CSR public key does not match local private key")
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatal(err)
	}
}

func TestEnrollGeneratesLocalKeySendsCSRAndPersists(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	client := &fakeEnrollmentClient{
		signCSR:   true,
		sensorID:  "sensor-001",
		notBefore: now.Add(-time.Minute),
		notAfter:  now.Add(time.Hour),
	}
	token, _ := NewBootstrapToken([]byte("test-only-bootstrap"))
	identity, err := store.Enroll(context.Background(), client, token, EnrollmentRequest{Role: "NETWORK"})
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 || client.seenToken != "test-only-bootstrap" {
		t.Fatalf("client=%+v", client)
	}
	if strings.TrimSpace(client.seenReq.CSRPem) == "" {
		t.Fatal("enrollment request missing csrPem")
	}
	if strings.Contains(strings.ToLower(client.seenReq.CSRPem), "private") {
		t.Fatal("CSR contained private-key material")
	}
	if _, err := parseEnrollmentCSR([]byte(client.seenReq.CSRPem)); err != nil {
		t.Fatal(err)
	}
	if identity.SensorID != "sensor-001" {
		t.Fatalf("identity=%+v", identity)
	}
	if _, err := token.Consume(); err == nil {
		t.Fatal("token remained reusable after enrollment")
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CertificateSHA256 != identity.CertificateSHA256 {
		t.Fatalf("loaded=%+v identity=%+v", loaded, identity)
	}
}

func TestTwoEnrollmentsGenerateDifferentLocalKeys(t *testing.T) {
	now := time.Now().UTC()
	store1 := newTestStore(t, now)
	store2 := newTestStore(t, now)
	client1 := &fakeEnrollmentClient{signCSR: true, sensorID: "sensor-a", notBefore: now.Add(-time.Minute), notAfter: now.Add(time.Hour)}
	client2 := &fakeEnrollmentClient{signCSR: true, sensorID: "sensor-b", notBefore: now.Add(-time.Minute), notAfter: now.Add(time.Hour)}
	token1, _ := NewBootstrapToken([]byte("token-1"))
	token2, _ := NewBootstrapToken([]byte("token-2"))
	id1, err := store1.Enroll(context.Background(), client1, token1, EnrollmentRequest{Role: "NETWORK"})
	if err != nil {
		t.Fatal(err)
	}
	id2, err := store2.Enroll(context.Background(), client2, token2, EnrollmentRequest{Role: "NETWORK"})
	if err != nil {
		t.Fatal(err)
	}
	_, key1, _, _ := store1.Paths(id1)
	_, key2, _, _ := store2.Paths(id2)
	b1, _ := os.ReadFile(key1)
	b2, _ := os.ReadFile(key2)
	if string(b1) == string(b2) {
		t.Fatal("enrollments reused the same private key")
	}
}

func TestPersistRejectsCertificateForDifferentKey(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	material := makeEnrollmentMaterialOpts(t, "sensor-001", now.Add(-time.Minute), now.Add(time.Hour), true, true)
	if _, err := store.Persist(material.Response, material.KeyPEM); err == nil || !strings.Contains(err.Error(), "KEYPAIR_INVALID") {
		t.Fatalf("error=%v", err)
	}
}

func TestPersistRejectsUntrustedCA(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	material := makeEnrollmentMaterial(t, "sensor-001", now.Add(-time.Minute), now.Add(time.Hour))
	other := makeEnrollmentMaterial(t, "sensor-other", now.Add(-time.Minute), now.Add(time.Hour))
	material.Response.CABundlePEM = other.Response.CABundlePEM
	if _, err := store.Persist(material.Response, material.KeyPEM); err == nil || !strings.Contains(err.Error(), "UNTRUSTED") {
		t.Fatalf("error=%v", err)
	}
}

func TestPersistRejectsExpiredAndNotYetValidCertificate(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	expired := makeEnrollmentMaterial(t, "sensor-001", now.Add(-2*time.Hour), now.Add(-time.Hour))
	if _, err := store.Persist(expired.Response, expired.KeyPEM); err == nil || !strings.Contains(err.Error(), "CERTIFICATE_INVALID") {
		t.Fatalf("expired error=%v", err)
	}
	future := makeEnrollmentMaterial(t, "sensor-001", now.Add(time.Hour), now.Add(2*time.Hour))
	if _, err := store.Persist(future.Response, future.KeyPEM); err == nil || !strings.Contains(err.Error(), "CERTIFICATE_INVALID") {
		t.Fatalf("not-yet-valid error=%v", err)
	}
}

func TestPersistRejectsNonClientAuthCertificate(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	material := makeEnrollmentMaterialOpts(t, "sensor-001", now.Add(-time.Minute), now.Add(time.Hour), false, false)
	if _, err := store.Persist(material.Response, material.KeyPEM); err == nil || !strings.Contains(err.Error(), "client authentication") {
		t.Fatalf("error=%v", err)
	}
}

func TestPersistRejectsMalformedCertificateAndInvalidCA(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	material := makeEnrollmentMaterial(t, "sensor-001", now.Add(-time.Minute), now.Add(time.Hour))
	material.Response.CertificatePEM = []byte("not-a-cert")
	if _, err := store.Persist(material.Response, material.KeyPEM); err == nil {
		t.Fatal("malformed certificate accepted")
	}
	valid := makeEnrollmentMaterial(t, "sensor-001", now.Add(-time.Minute), now.Add(time.Hour))
	valid.Response.CABundlePEM = []byte("invalid")
	if _, err := store.Persist(valid.Response, valid.KeyPEM); err == nil || !strings.Contains(err.Error(), "CA_INVALID") {
		t.Fatalf("ca error=%v", err)
	}
}

func TestEnrollmentResponseTypeHasNoPrivateKeyField(t *testing.T) {
	var response EnrollmentResponse
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(encoded)), "private") {
		t.Fatalf("EnrollmentResponse leaked private-key field: %s", encoded)
	}
}

func TestPrivateKeyPermissionsAreRestricted(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	material := makeEnrollmentMaterial(t, "sensor-001", now.Add(-time.Minute), now.Add(time.Hour))
	identity, err := store.Persist(material.Response, material.KeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	_, key, _, err := store.Paths(identity)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(key)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("private key mode=%o", info.Mode().Perm())
	}
	if err := os.Chmod(key, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "KEY_PERMISSIONS") {
		t.Fatalf("error=%v", err)
	}
}

func TestCertificateExpiryStates(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	material := makeEnrollmentMaterial(t, "sensor-001", now.Add(-time.Minute), now.Add(2*time.Hour))
	identity, err := store.Persist(material.Response, material.KeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Status(identity, 30*time.Minute).State; got != CertificateValid {
		t.Fatalf("state=%s", got)
	}
	if got := store.Status(identity, 3*time.Hour).State; got != CertificateExpiring {
		t.Fatalf("state=%s", got)
	}
	store.now = func() time.Time { return now.Add(3 * time.Hour) }
	if got := store.Status(identity, 30*time.Minute).State; got != CertificateExpired {
		t.Fatalf("state=%s", got)
	}
}

func TestEnrollmentRejectsMismatchedCertificateIdentity(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	material := makeEnrollmentMaterial(t, "sensor-cert-name", now.Add(-time.Minute), now.Add(time.Hour))
	material.Response.SensorID = "sensor-other"
	if _, err := store.Persist(material.Response, material.KeyPEM); err == nil || !strings.Contains(err.Error(), "CERTIFICATE_MISMATCH") {
		t.Fatalf("error=%v", err)
	}
}

func TestLoadRejectsPathTraversalMetadata(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	metadata := Identity{SensorID: "sensor-001", CertificateFile: "../cert", PrivateKeyFile: "key", CAFile: "ca", CertificateSHA256: "x"}
	data, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(store.dir, metadataFile), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "PATH_UNSAFE") {
		t.Fatalf("error=%v", err)
	}
}

func TestLoadRejectsSymlinkIdentityFiles(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	material := makeEnrollmentMaterial(t, "sensor-001", now.Add(-time.Minute), now.Add(time.Hour))
	identity, err := store.Persist(material.Response, material.KeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	cert, _, _, _ := store.Paths(identity)
	target := filepath.Join(t.TempDir(), "other.crt")
	_ = os.WriteFile(target, []byte("x"), 0600)
	_ = os.Remove(cert)
	if err := os.Symlink(target, cert); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "PATH_UNSAFE") {
		t.Fatalf("error=%v", err)
	}
}

func TestCrashStylePartialGenerationDoesNotReplaceCommittedMetadata(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	material := makeEnrollmentMaterial(t, "sensor-001", now.Add(-time.Minute), now.Add(time.Hour))
	first, err := store.Persist(material.Response, material.KeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.dir, "sensor-partial.crt"), []byte("partial"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.dir, "sensor-partial.key"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CertificateSHA256 != first.CertificateSHA256 {
		t.Fatal("partial generation displaced committed identity")
	}
}

func TestMetadataUnknownFieldRejected(t *testing.T) {
	now := time.Now().UTC()
	store := newTestStore(t, now)
	_ = os.WriteFile(filepath.Join(store.dir, metadataFile), []byte(`{"sensorId":"x","unexpected":true}`), 0600)
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "METADATA_INVALID") {
		t.Fatalf("error=%v", err)
	}
}
