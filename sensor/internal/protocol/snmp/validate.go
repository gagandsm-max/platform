package snmp

import (
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	maxCorrelationIDBytes = 128
	maxContextNameBytes   = 255
)

func ValidateRequest(req Request) error {
	if !validTarget(req.Target) {
		return newError(CodeInvalidRequest, "target must be a valid IP address or hostname", nil, false)
	}
	if req.Port < 1 || req.Port > 65535 {
		return newError(CodeInvalidRequest, "port must be between 1 and 65535", nil, false)
	}
	if req.Timeout <= 0 {
		return newError(CodeInvalidRequest, "timeout must be greater than zero", nil, false)
	}
	if req.ExecutionTimeout <= 0 {
		return newError(CodeInvalidRequest, "execution timeout must be greater than zero", nil, false)
	}
	if req.Retry.MaxAttempts <= 0 {
		return newError(CodeInvalidRequest, "retry max attempts must be greater than zero", nil, false)
	}
	if req.Retry.MaxAttempts > 1 {
		if req.Retry.InitialBackoff <= 0 || req.Retry.MaxBackoff <= 0 {
			return newError(CodeInvalidRequest, "retry backoff values must be greater than zero when retries are enabled", nil, false)
		}
		if req.Retry.MaxBackoff < req.Retry.InitialBackoff {
			return newError(CodeInvalidRequest, "retry max backoff must be greater than or equal to initial backoff", nil, false)
		}
	}
	if req.Retry.JitterFraction < 0 || req.Retry.JitterFraction > 1 {
		return newError(CodeInvalidRequest, "retry jitter fraction must be between zero and one", nil, false)
	}
	if req.Limits.MaxOIDs <= 0 || req.Limits.MaxWalkEntries <= 0 || req.Limits.MaxBulkRepetitions == 0 || req.Limits.MaxResults <= 0 || req.Limits.MaxResultBytes <= 0 {
		return newError(CodeInvalidRequest, "all collection safety limits must be greater than zero", nil, false)
	}
	if len(req.CorrelationID) > maxCorrelationIDBytes || strings.IndexFunc(req.CorrelationID, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
		return newError(CodeInvalidRequest, "correlation ID is invalid or exceeds safety bound", nil, false)
	}

	switch req.Version {
	case Version1, Version2c, Version3:
	default:
		return newError(CodeInvalidRequest, "unsupported SNMP version", nil, false)
	}

	switch req.Operation {
	case OperationGet, OperationGetNext, OperationGetBulk, OperationWalk, OperationBulkWalk:
	default:
		return newError(CodeInvalidRequest, "unsupported SNMP operation", nil, false)
	}
	if req.Version == Version1 && (req.Operation == OperationGetBulk || req.Operation == OperationBulkWalk) {
		return newError(CodeUnsupportedSecurity, "SNMPv1 does not support GETBULK/BULKWALK", nil, false)
	}
	if len(req.OIDs) == 0 {
		return newError(CodeInvalidRequest, "at least one OID is required", nil, false)
	}
	if len(req.OIDs) > req.Limits.MaxOIDs {
		return newError(CodeResourceLimit, "OID count exceeds configured maximum", nil, false)
	}
	if (req.Operation == OperationWalk || req.Operation == OperationBulkWalk) && len(req.OIDs) != 1 {
		return newError(CodeInvalidRequest, "WALK/BULKWALK requires exactly one root OID", nil, false)
	}
	if (req.Operation == OperationGetBulk || req.Operation == OperationBulkWalk) && uint64(req.Limits.MaxBulkRepetitions) > uint64(req.Limits.MaxResults) {
		return newError(CodeResourceLimit, "bulk repetitions exceed maximum result count", nil, false)
	}
	for _, oid := range req.OIDs {
		if _, err := CanonicalOID(oid); err != nil {
			return err
		}
	}
	if err := validateCredentials(req.Version, req.Credentials); err != nil {
		return err
	}
	return nil
}

func validateCredentials(version Version, credentials Credentials) error {
	switch version {
	case Version1, Version2c:
		if credentials.Community == nil || credentials.Community.Community.value() == "" {
			return newError(CodeInvalidRequest, "community credential is required for SNMPv1/v2c", nil, false)
		}
		if credentials.V3 != nil {
			return newError(CodeInvalidRequest, "SNMPv3 credentials are not valid for SNMPv1/v2c", nil, false)
		}
		return nil
	case Version3:
		if credentials.Community != nil {
			return newError(CodeInvalidRequest, "community credentials are not valid for SNMPv3", nil, false)
		}
		if credentials.V3 == nil {
			return newError(CodeInvalidRequest, "SNMPv3 credentials are required", nil, false)
		}
		return validateV3Credentials(*credentials.V3)
	default:
		return newError(CodeInvalidRequest, "unsupported SNMP version", nil, false)
	}
}

func validateV3Credentials(c V3Credentials) error {
	if c.Username == "" {
		return newError(CodeInvalidRequest, "SNMPv3 username is required", nil, false)
	}
	if len(c.ContextName) > maxContextNameBytes {
		return newError(CodeInvalidRequest, "SNMPv3 context name exceeds safety bound", nil, false)
	}

	switch c.SecurityLevel {
	case SecurityNoAuthNoPriv:
		if c.AuthProtocol != AuthNone || c.AuthSecret.value() != "" || c.PrivacyProtocol != PrivacyNone || c.PrivacySecret.value() != "" {
			return newError(CodeInvalidRequest, "NO_AUTH_NO_PRIV must not include authentication or privacy configuration", nil, false)
		}
	case SecurityAuthNoPriv:
		if !validAuthProtocol(c.AuthProtocol) || c.AuthProtocol == AuthNone || c.AuthSecret.value() == "" {
			return newError(CodeInvalidRequest, "AUTH_NO_PRIV requires an explicit authentication protocol and secret", nil, false)
		}
		if c.PrivacyProtocol != PrivacyNone || c.PrivacySecret.value() != "" {
			return newError(CodeInvalidRequest, "AUTH_NO_PRIV must not include privacy configuration", nil, false)
		}
	case SecurityAuthPriv:
		if !validAuthProtocol(c.AuthProtocol) || c.AuthProtocol == AuthNone || c.AuthSecret.value() == "" {
			return newError(CodeInvalidRequest, "AUTH_PRIV requires an explicit authentication protocol and secret", nil, false)
		}
		if !validPrivacyProtocol(c.PrivacyProtocol) || c.PrivacyProtocol == PrivacyNone || c.PrivacySecret.value() == "" {
			return newError(CodeInvalidRequest, "AUTH_PRIV requires an explicit privacy protocol and secret", nil, false)
		}
	default:
		return newError(CodeUnsupportedSecurity, "unsupported SNMPv3 security level", nil, false)
	}
	return nil
}

func validAuthProtocol(p AuthProtocol) bool {
	switch p {
	case AuthNone, AuthMD5, AuthSHA1, AuthSHA224, AuthSHA256, AuthSHA384, AuthSHA512:
		return true
	default:
		return false
	}
}

func validPrivacyProtocol(p PrivacyProtocol) bool {
	switch p {
	case PrivacyNone, PrivacyDES, PrivacyAES, PrivacyAES192, PrivacyAES256, PrivacyAES192C, PrivacyAES256C:
		return true
	default:
		return false
	}
}

func validTarget(target string) bool {
	if target == "" || strings.TrimSpace(target) != target || strings.ContainsAny(target, "/\\") {
		return false
	}
	if net.ParseIP(target) != nil {
		return true
	}
	if len(target) > 253 || strings.Contains(target, ":") {
		return false
	}
	for _, label := range strings.Split(target, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
				return false
			}
		}
	}
	return true
}

func CanonicalOID(input string) (string, error) {
	if input == "" || strings.TrimSpace(input) != input {
		return "", newError(CodeInvalidRequest, "OID is empty or contains surrounding whitespace", nil, false)
	}
	trimmed := strings.TrimPrefix(input, ".")
	parts := strings.Split(trimmed, ".")
	if len(parts) < 2 {
		return "", newError(CodeInvalidRequest, "OID must contain at least two numeric arcs", nil, false)
	}
	arcs := make([]uint64, len(parts))
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return "", newError(CodeInvalidRequest, "OID contains an invalid numeric arc", nil, false)
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return "", newError(CodeInvalidRequest, "OID contains a non-numeric arc", nil, false)
			}
		}
		value, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return "", newError(CodeInvalidRequest, "OID arc exceeds the SNMP sub-identifier range", err, false)
		}
		arcs[i] = value
	}
	if arcs[0] > 2 {
		return "", newError(CodeInvalidRequest, "OID first arc must be 0, 1, or 2", nil, false)
	}
	if arcs[0] < 2 && arcs[1] > 39 {
		return "", newError(CodeInvalidRequest, "OID second arc is invalid for the first arc", nil, false)
	}
	return "." + strings.Join(parts, "."), nil
}

func durationFromMilliseconds(value int64, field string) (time.Duration, error) {
	if value <= 0 {
		return 0, newError(CodeInvalidRequest, field+" must be greater than zero", nil, false)
	}
	const maxMillis = int64((1<<63 - 1) / int64(time.Millisecond))
	if value > maxMillis {
		return 0, newError(CodeInvalidRequest, field+" exceeds duration range", nil, false)
	}
	return time.Duration(value) * time.Millisecond, nil
}
