package synthetic

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"syscall"
)

type ErrorCode string

const (
	CodeInvalidRequest     ErrorCode = "SYNTHETIC_INVALID_REQUEST"
	CodeDNSFailure         ErrorCode = "SYNTHETIC_DNS_FAILURE"
	CodeTimeout            ErrorCode = "SYNTHETIC_TIMEOUT"
	CodeConnectionRefused  ErrorCode = "SYNTHETIC_CONNECTION_REFUSED"
	CodeNetworkUnreachable ErrorCode = "SYNTHETIC_NETWORK_UNREACHABLE"
	CodeTLSUntrusted       ErrorCode = "SYNTHETIC_TLS_UNTRUSTED"
	CodeTLSHostname        ErrorCode = "SYNTHETIC_TLS_HOSTNAME"
	CodeTLSExpired         ErrorCode = "SYNTHETIC_TLS_EXPIRED"
	CodeTLSHandshake       ErrorCode = "SYNTHETIC_TLS_HANDSHAKE"
	CodeHTTPStatus         ErrorCode = "SYNTHETIC_HTTP_STATUS"
	CodeContentMismatch    ErrorCode = "SYNTHETIC_CONTENT_MISMATCH"
	CodeResourceLimit      ErrorCode = "SYNTHETIC_RESOURCE_LIMIT"
	CodeCancelled          ErrorCode = "SYNTHETIC_CANCELLED"
	CodeDeadlineExceeded   ErrorCode = "SYNTHETIC_DEADLINE_EXCEEDED"
	CodeTransportFailure   ErrorCode = "SYNTHETIC_TRANSPORT_FAILURE"
)

type Error struct {
	Code      ErrorCode
	message   string
	cause     error
	transient bool
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.message)
}
func (e *Error) GoString() string { return e.Error() }
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}
func (e *Error) Transient() bool { return e != nil && e.transient }
func newError(code ErrorCode, message string, cause error, transient bool) error {
	return &Error{Code: code, message: message, cause: cause, transient: transient}
}
func CodeOf(err error) ErrorCode {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}
func IsTransient(err error) bool { var e *Error; return errors.As(err, &e) && e.transient }

func contextError(parent, op context.Context) error {
	if parent == nil {
		return newError(CodeInvalidRequest, "context must not be nil", nil, false)
	}
	if err := parent.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return newError(CodeDeadlineExceeded, "synthetic execution deadline exceeded", context.DeadlineExceeded, false)
		}
		return newError(CodeCancelled, "synthetic execution cancelled", context.Canceled, false)
	}
	if op != nil && errors.Is(op.Err(), context.DeadlineExceeded) {
		return newError(CodeTimeout, "synthetic operation timed out", context.DeadlineExceeded, true)
	}
	return nil
}

func classifyTransport(parent, op context.Context, err error) error {
	if ce := contextError(parent, op); ce != nil {
		return ce
	}
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		if dnsErr.Timeout() || dnsErr.IsTemporary {
			return newError(CodeDNSFailure, "DNS resolution failed temporarily", err, true)
		}
		return newError(CodeDNSFailure, "DNS resolution failed", err, false)
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return newError(CodeConnectionRefused, "connection was refused", err, false)
	}
	if errors.Is(err, syscall.ENETUNREACH) || errors.Is(err, syscall.EHOSTUNREACH) {
		return newError(CodeNetworkUnreachable, "network or host is unreachable", err, true)
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
			return newError(CodeConnectionRefused, "connection was refused", err, false)
		}
		if errors.Is(opErr.Err, syscall.ENETUNREACH) || errors.Is(opErr.Err, syscall.EHOSTUNREACH) {
			return newError(CodeNetworkUnreachable, "network or host is unreachable", err, true)
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return newError(CodeTimeout, "synthetic operation timed out", err, true)
	}
	return newError(CodeTransportFailure, "synthetic transport failed", err, false)
}

func classifyTLS(parent, op context.Context, err error) error {
	if ce := contextError(parent, op); ce != nil {
		return ce
	}
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return newError(CodeTLSHostname, "TLS certificate hostname verification failed", err, false)
	}
	var authorityErr x509.UnknownAuthorityError
	if errors.As(err, &authorityErr) {
		return newError(CodeTLSUntrusted, "TLS certificate chain is not trusted", err, false)
	}
	var certErr x509.CertificateInvalidError
	if errors.As(err, &certErr) {
		if certErr.Reason == x509.Expired {
			return newError(CodeTLSExpired, "TLS certificate is expired or not currently valid", err, false)
		}
		return newError(CodeTLSHandshake, "TLS certificate validation failed", err, false)
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return classifyTLS(parent, op, urlErr.Err)
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return newError(CodeTLSUntrusted, "TLS trust material could not be loaded", err, false)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return newError(CodeTimeout, "TLS handshake timed out", err, true)
	}
	return newError(CodeTLSHandshake, "TLS handshake failed", err, false)
}
