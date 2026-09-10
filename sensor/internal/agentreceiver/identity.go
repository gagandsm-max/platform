package agentreceiver

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"strings"
)

// AuthenticatedAgentIdentity is the peer identity derived from verified mTLS
// certificate material. FingerprintSHA256 is the authority key for Core resolve.
// AgentID may be empty until Core assignment authority returns it.
type AuthenticatedAgentIdentity struct {
	AgentID           string
	FingerprintSHA256 string
}

// AgentIdentityVerifier maps a verified mTLS peer certificate chain to an
// authenticated Agent identity. Production wiring must fail closed until a
// production verifier is installed.
//
// Callers MUST pass only cryptographically verified chain material
// (e.g. tls.ConnectionState.VerifiedChains[0]). Unverified PeerCertificates
// alone must never be supplied.
type AgentIdentityVerifier interface {
	VerifyPeerIdentity(verifiedChain []*x509.Certificate) (AuthenticatedAgentIdentity, error)
}

// FailClosedIdentityVerifier is the mandatory production default when no
// production fingerprint verifier is wired. It rejects all peers.
type FailClosedIdentityVerifier struct{}

func NewFailClosedIdentityVerifier() *FailClosedIdentityVerifier {
	return &FailClosedIdentityVerifier{}
}

func (v *FailClosedIdentityVerifier) VerifyPeerIdentity(verifiedChain []*x509.Certificate) (AuthenticatedAgentIdentity, error) {
	if len(verifiedChain) == 0 || verifiedChain[0] == nil {
		return AuthenticatedAgentIdentity{}, errors.New(ErrCodeAuthRequired + ": verified peer certificate chain is empty")
	}
	return AuthenticatedAgentIdentity{}, errors.New(ErrCodeAuthInvalid + ": AGENT_IDENTITY_ENROLLMENT_BLOCKED: authoritative Core enrollment unavailable")
}

// FingerprintIdentityVerifier derives identity solely from the leaf DER
// fingerprint of the verified chain. CN/SAN are never authority.
type FingerprintIdentityVerifier struct{}

func NewFingerprintIdentityVerifier() *FingerprintIdentityVerifier {
	return &FingerprintIdentityVerifier{}
}

func (v *FingerprintIdentityVerifier) VerifyPeerIdentity(verifiedChain []*x509.Certificate) (AuthenticatedAgentIdentity, error) {
	if len(verifiedChain) == 0 || verifiedChain[0] == nil {
		return AuthenticatedAgentIdentity{}, errors.New(ErrCodeAuthRequired + ": verified peer certificate chain is empty")
	}
	fp := LeafFingerprintSHA256(verifiedChain[0])
	if strings.TrimSpace(fp) == "" {
		return AuthenticatedAgentIdentity{}, errors.New(ErrCodeAuthInvalid + ": leaf certificate fingerprint is empty")
	}
	return AuthenticatedAgentIdentity{
		FingerprintSHA256: fp,
		// AgentID remains empty until Core assignment authority resolves it.
	}, nil
}

// LeafFingerprintSHA256 returns lowercase hex SHA-256 of cert.Raw (DER).
func LeafFingerprintSHA256(cert *x509.Certificate) string {
	if cert == nil || len(cert.Raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}
