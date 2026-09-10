package agentreceiver

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

// StaticTestIdentityVerifier is TEST-ONLY. It maps a leaf SHA-256 fingerprint
// (hex, lowercase) or an exact CommonName string to an AgentID. It does not
// invent or trust .agent.platform.local naming authority.
type StaticTestIdentityVerifier struct {
	mu    sync.RWMutex
	byKey map[string]string // fingerprint or exact CN → AgentID
}

func NewStaticTestIdentityVerifier() *StaticTestIdentityVerifier {
	return &StaticTestIdentityVerifier{byKey: make(map[string]string)}
}

// AllowFingerprint registers a leaf certificate SHA-256 fingerprint → AgentID mapping.
func (v *StaticTestIdentityVerifier) AllowFingerprint(fingerprintHex, agentID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.byKey[strings.ToLower(strings.TrimSpace(fingerprintHex))] = strings.TrimSpace(agentID)
}

// AllowCommonName registers an exact leaf CN → AgentID mapping (case-insensitive CN lookup).
func (v *StaticTestIdentityVerifier) AllowCommonName(cn, agentID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.byKey["cn:"+strings.ToLower(strings.TrimSpace(cn))] = strings.TrimSpace(agentID)
}

func (v *StaticTestIdentityVerifier) VerifyPeerIdentity(verifiedChain []*x509.Certificate) (AuthenticatedAgentIdentity, error) {
	if len(verifiedChain) == 0 || verifiedChain[0] == nil {
		return AuthenticatedAgentIdentity{}, fmt.Errorf("%s: verified peer certificate chain is empty", ErrCodeAuthRequired)
	}
	leaf := verifiedChain[0]
	fp := leafFingerprintSHA256(leaf)
	cn := strings.TrimSpace(leaf.Subject.CommonName)

	v.mu.RLock()
	defer v.mu.RUnlock()

	if agentID, ok := v.byKey[fp]; ok && agentID != "" {
		return AuthenticatedAgentIdentity{AgentID: agentID, FingerprintSHA256: fp}, nil
	}
	if cn != "" {
		if agentID, ok := v.byKey["cn:"+strings.ToLower(cn)]; ok && agentID != "" {
			return AuthenticatedAgentIdentity{AgentID: agentID, FingerprintSHA256: fp}, nil
		}
	}
	return AuthenticatedAgentIdentity{}, fmt.Errorf("%s: leaf certificate is not mapped to an agent identity", ErrCodeAuthInvalid)
}

func leafFingerprintSHA256(cert *x509.Certificate) string {
	if fp := LeafFingerprintSHA256(cert); fp != "" {
		return fp
	}
	// httptest mock certificates often omit Raw DER; synthesize a stable test fingerprint.
	cn := ""
	if cert != nil {
		cn = strings.TrimSpace(cert.Subject.CommonName)
	}
	sum := sha256.Sum256([]byte("agentreceiver-test-leaf:" + cn))
	return hex.EncodeToString(sum[:])
}
