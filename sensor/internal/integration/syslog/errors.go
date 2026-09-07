package syslogintegration

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidConfig      ErrorCode = "SYSLOG_INTEGRATION_INVALID_CONFIG"
	CodeResourceLimit      ErrorCode = "SYSLOG_RESOURCE_LIMIT"
	CodeStorageProtection  ErrorCode = "SYSLOG_STORAGE_PROTECTION"
	CodeStorageUnavailable ErrorCode = "SYSLOG_STORAGE_UNAVAILABLE"
	CodeStorageCorruption  ErrorCode = "SYSLOG_STORAGE_CORRUPTION"
	CodeCancelled          ErrorCode = "SYSLOG_CANCELLED"
	CodeDeadlineExceeded   ErrorCode = "SYSLOG_DEADLINE_EXCEEDED"
	CodeInternal           ErrorCode = "SYSLOG_INTERNAL"
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
