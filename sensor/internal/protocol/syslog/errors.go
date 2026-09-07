package syslog

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidConfig    ErrorCode = "SYSLOG_INVALID_CONFIG"
	CodeMessageTooLarge  ErrorCode = "SYSLOG_MESSAGE_TOO_LARGE"
	CodeMalformedPRI     ErrorCode = "SYSLOG_MALFORMED_PRI"
	CodeMalformedRFC5424 ErrorCode = "SYSLOG_MALFORMED_RFC5424"
	CodeMalformedRFC3164 ErrorCode = "SYSLOG_MALFORMED_RFC3164"
	CodeStructuredData   ErrorCode = "SYSLOG_STRUCTURED_DATA_INVALID"
	CodeInvalidUTF8      ErrorCode = "SYSLOG_INVALID_UTF8"
	CodeResourceLimit    ErrorCode = "SYSLOG_RESOURCE_LIMIT"
	CodeInternal         ErrorCode = "SYSLOG_INTERNAL"
	CodePartialUnknown   ErrorCode = "SYSLOG_PARTIAL_UNKNOWN"
	CodeQueueFull        ErrorCode = "SYSLOG_QUEUE_FULL"
	CodeTimeout          ErrorCode = "SYSLOG_TIMEOUT"
	CodeCancelled        ErrorCode = "SYSLOG_CANCELLED"
)

type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func CodeOf(err error) ErrorCode {
	var target *Error
	if errors.As(err, &target) {
		return target.Code
	}
	return ""
}

func fail(code ErrorCode, message string, cause error) error {
	return &Error{Code: code, Message: message, Cause: cause}
}
