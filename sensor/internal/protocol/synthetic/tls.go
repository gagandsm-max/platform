package synthetic

import (
	"context"
	"crypto/tls"
	"net"
	"strings"
	"time"
)

func (c *Collector) TLS(parent context.Context, r TLSRequest) (TLSResult, error) {
	var result TLSResult
	if c == nil {
		return result, newError(CodeInvalidRequest, "synthetic collector is nil", nil, false)
	}
	started := c.now().UTC()
	result = TLSResult{SchemaVersion: ResultSchemaVersion, Target: r.Target, Port: r.Port, ServerName: r.ServerName, StartedAt: started}
	err := c.validateTLS(r)
	if err != nil {
		result.ErrorCode = CodeOf(err)
		result.CompletedAt = c.now().UTC()
		c.record(err)
		return result, err
	}
	ctx, cancel, err := c.operationContext(parent, r.Timeout)
	if err != nil {
		result.ErrorCode = CodeOf(err)
		result.CompletedAt = c.now().UTC()
		c.record(err)
		return result, err
	}
	defer cancel()
	serverName := strings.TrimSpace(r.ServerName)
	if serverName == "" {
		serverName = strings.TrimSpace(r.Target)
	}
	cfg, err := tlsConfigFor(serverName, r.CABundlePEM)
	if err != nil {
		result.ErrorCode = CodeOf(err)
		result.CompletedAt = c.now().UTC()
		c.record(err)
		return result, err
	}
	dialer := tls.Dialer{NetDialer: &net.Dialer{}, Config: cfg}
	begin := time.Now()
	conn, dialErr := dialer.DialContext(ctx, "tcp", joinAddress(strings.TrimSpace(r.Target), r.Port))
	elapsed := time.Since(begin)
	result.DurationMicros = elapsed.Microseconds()
	result.CompletedAt = c.now().UTC()
	if dialErr != nil {
		err = classifyTLS(parent, ctx, dialErr)
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		_ = conn.Close()
		err = newError(CodeTLSHandshake, "TLS transport did not return TLS connection state", nil, false)
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	state := tlsConn.ConnectionState()
	closeErr := tlsConn.Close()
	if closeErr != nil {
		err = classifyTransport(parent, ctx, closeErr)
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	result.NegotiatedVersion = tlsVersionName(state.Version)
	result.CipherSuite = tls.CipherSuiteName(state.CipherSuite)
	if len(state.PeerCertificates) == 0 {
		err = newError(CodeTLSHandshake, "TLS peer did not present a certificate", nil, false)
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	if len(state.PeerCertificates) > c.limits.MaxCertificateChain {
		err = newError(CodeResourceLimit, "TLS certificate chain exceeds configured representation bound", nil, false)
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	now := c.now()
	result.PeerCertificates = make([]CertificateSummary, 0, len(state.PeerCertificates))
	for _, cert := range state.PeerCertificates {
		if cert == nil {
			continue
		}
		subject, issuer := cert.Subject.String(), cert.Issuer.String()
		if err = diagnosticBound(subject, c.limits.MaxDiagnosticTextBytes); err != nil {
			result.ErrorCode = CodeOf(err)
			c.record(err)
			return result, err
		}
		if err = diagnosticBound(issuer, c.limits.MaxDiagnosticTextBytes); err != nil {
			result.ErrorCode = CodeOf(err)
			c.record(err)
			return result, err
		}
		sanCount := len(cert.DNSNames) + len(cert.IPAddresses) + len(cert.EmailAddresses) + len(cert.URIs)
		if sanCount > c.limits.MaxSANs {
			err = newError(CodeResourceLimit, "TLS certificate SAN count exceeds configured representation bound", nil, false)
			result.ErrorCode = CodeOf(err)
			c.record(err)
			return result, err
		}
		summary := CertificateSummary{Subject: subject, Issuer: issuer, NotBefore: cert.NotBefore, NotAfter: cert.NotAfter, RemainingLifetimeSeconds: int64(cert.NotAfter.Sub(now).Seconds()), DNSNames: append([]string(nil), cert.DNSNames...), OtherSANCount: len(cert.EmailAddresses) + len(cert.URIs)}
		for _, ip := range cert.IPAddresses {
			summary.IPAddresses = append(summary.IPAddresses, ip.String())
		}
		result.PeerCertificates = append(result.PeerCertificates, summary)
	}
	result.Success = true
	c.record(nil)
	return result, nil
}

func (c *Collector) validateTLS(r TLSRequest) error {
	if err := validateHost(r.Target, c.limits.MaxTargetBytes); err != nil {
		return err
	}
	if err := validatePort(r.Port); err != nil {
		return err
	}
	if r.ServerName != "" {
		if err := validateHost(r.ServerName, c.limits.MaxTargetBytes); err != nil {
			return newError(CodeInvalidRequest, "TLS server name is invalid", nil, false)
		}
	}
	if r.Timeout <= 0 {
		return newError(CodeInvalidRequest, "TLS timeout must be positive", nil, false)
	}
	if r.Timeout > c.limits.MaxTimeout {
		return newError(CodeResourceLimit, "TLS timeout exceeds configured bound", nil, false)
	}
	if err := validateCABundle(c.limits, r.CABundlePEM); err != nil {
		return err
	}
	return nil
}

func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS12:
		return "TLS1.2"
	case tls.VersionTLS13:
		return "TLS1.3"
	default:
		return "UNKNOWN"
	}
}
