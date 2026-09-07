package snmp

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidRequest       ErrorCode = "SNMP_INVALID_REQUEST"
	CodeAuthenticationFailed ErrorCode = "SNMP_AUTHENTICATION_FAILED"
	CodeAuthorizationFailed  ErrorCode = "SNMP_AUTHORIZATION_FAILED"
	CodeTimeout              ErrorCode = "SNMP_TIMEOUT"
	CodeNetworkUnreachable   ErrorCode = "SNMP_NETWORK_UNREACHABLE"
	CodeTransportFailure     ErrorCode = "SNMP_TRANSPORT_FAILURE"
	CodeUnsupportedSecurity  ErrorCode = "SNMP_UNSUPPORTED_SECURITY"
	CodeMalformedResponse    ErrorCode = "SNMP_MALFORMED_RESPONSE"
	CodeProtocolError        ErrorCode = "SNMP_PROTOCOL_ERROR"
	CodeNoSuchObject         ErrorCode = "SNMP_NO_SUCH_OBJECT"
	CodeNoSuchInstance       ErrorCode = "SNMP_NO_SUCH_INSTANCE"
	CodeEndOfMibView         ErrorCode = "SNMP_END_OF_MIB_VIEW"
	CodeResourceLimit        ErrorCode = "SNMP_RESOURCE_LIMIT"
	CodeCancelled            ErrorCode = "SNMP_CANCELLED"
	CodeDeadlineExceeded     ErrorCode = "SNMP_DEADLINE_EXCEEDED"
)

type Error struct {
	Code      ErrorCode
	Message   string
	cause     error
	transient bool
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) GoString() string { return e.Error() }

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *Error) Transient() bool { return e != nil && e.transient }

func newError(code ErrorCode, message string, cause error, transient bool) *Error {
	return &Error{Code: code, Message: message, cause: cause, transient: transient}
}

func IsCode(err error, code ErrorCode) bool {
	var target *Error
	return errors.As(err, &target) && target.Code == code
}

func IsTransient(err error) bool {
	var target *Error
	return errors.As(err, &target) && target.transient
}

func ErrorCodeOf(err error) ErrorCode {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return CodeProtocolError
}
