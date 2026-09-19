package coreclient

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidConfig      ErrorCode = "CORECLIENT_INVALID_CONFIG"
	CodeTLSConfiguration   ErrorCode = "CORECLIENT_TLS_CONFIGURATION"
	CodeTransport          ErrorCode = "CORECLIENT_TRANSPORT"
	CodeTimeout            ErrorCode = "CORECLIENT_TIMEOUT"
	CodeAuthentication     ErrorCode = "CORECLIENT_AUTHENTICATION"
	CodeAuthorization      ErrorCode = "CORECLIENT_AUTHORIZATION"
	CodeNotFound           ErrorCode = "CORECLIENT_NOT_FOUND"
	CodeRateLimited        ErrorCode = "CORECLIENT_RATE_LIMITED"
	CodeServerUnavailable  ErrorCode = "CORECLIENT_SERVER_UNAVAILABLE"
	CodeProtocol           ErrorCode = "CORECLIENT_PROTOCOL"
	CodeResponseTooLarge   ErrorCode = "CORECLIENT_RESPONSE_TOO_LARGE"
	CodeAckContractMissing ErrorCode = "CORECLIENT_ACK_CONTRACT_UNAVAILABLE"
	CodeCancelled          ErrorCode = "CORECLIENT_CANCELLED"
	CodeDeadlineExceeded   ErrorCode = "CORECLIENT_DEADLINE_EXCEEDED"
)

type Error struct {
	Code      ErrorCode
	Message   string
	cause     error
	transient bool
}

func (e *Error) Error() string {
	if e == nil { return "" }
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}
func (e *Error) GoString() string { return e.Error() }
func (e *Error) Unwrap() error { if e == nil { return nil }; return e.cause }
func (e *Error) Transient() bool { return e != nil && e.transient }
func newError(code ErrorCode, message string, cause error, transient bool) *Error { return &Error{Code:code,Message:message,cause:cause,transient:transient} }
func IsCode(err error, code ErrorCode) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}
func IsTransient(err error) bool {
	var target *Error
	if errors.As(err, &target) && target.transient {
		return true
	}
	var api *CoreAPIError
	return errors.As(err, &api) && api.transient
}
