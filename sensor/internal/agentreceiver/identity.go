package agentreceiver

import (
	"crypto/x509"
	"errors"
)

// AuthenticatedAgentIdentity is the authoritative Agent identity derived from
// verified peer certificate material. Enrollment registry binding is owned by
// future Core; this component only consumes a verified AgentID string.
type AuthenticatedAgentIdentity struct {
	AgentID string
}

// AgentIdentityVerifier maps a verified mTLS peer certificate chain to an
// authoritative Agent identity. Production wiring must fail closed until Core
// enrollment exists.
//
// Callers MUST pass only cryptographically verified chain material
// (e.g. tls.ConnectionState.VerifiedChains[0]). Unverified PeerCertificates
// alone must never be supplied.
type AgentIdentityVerifier interface {
	VerifyPeerIdentity(verifiedChain []*x509.Certificate) (AuthenticatedAgentIdentity, error)
}

// FailClosedIdentityVerifier is the mandatory production default when no
// authoritative Core enrollment registry is wired. It rejects all peers.
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
