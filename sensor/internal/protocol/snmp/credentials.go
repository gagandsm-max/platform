package snmp

const redacted = "[REDACTED]"

type Secret string

func (Secret) String() string   { return redacted }
func (Secret) GoString() string { return redacted }
func (s Secret) value() string  { return string(s) }

type CommunityCredentials struct {
	Community Secret
}

func (CommunityCredentials) String() string     { return "CommunityCredentials{community=[REDACTED]}" }
func (c CommunityCredentials) GoString() string { return c.String() }

type V3Credentials struct {
	Username        string
	SecurityLevel   SecurityLevel
	AuthProtocol    AuthProtocol
	AuthSecret      Secret
	PrivacyProtocol PrivacyProtocol
	PrivacySecret   Secret
	ContextName     string
}

func (c V3Credentials) String() string {
	return "V3Credentials{username=" + c.Username + ",securityLevel=" + string(c.SecurityLevel) + ",authProtocol=" + string(c.AuthProtocol) + ",authSecret=[REDACTED],privacyProtocol=" + string(c.PrivacyProtocol) + ",privacySecret=[REDACTED],contextName=" + c.ContextName + "}"
}
func (c V3Credentials) GoString() string { return c.String() }

type Credentials struct {
	Community *CommunityCredentials
	V3        *V3Credentials
}

func (c Credentials) String() string {
	switch {
	case c.V3 != nil:
		return c.V3.String()
	case c.Community != nil:
		return c.Community.String()
	default:
		return "Credentials{unset}"
	}
}
func (c Credentials) GoString() string { return c.String() }

func IsLegacyAuthProtocol(p AuthProtocol) bool {
	return p == AuthMD5 || p == AuthSHA1
}

func IsLegacyPrivacyProtocol(p PrivacyProtocol) bool {
	return p == PrivacyDES
}
