package snmp

import (
	"context"
	"testing"

	"github.com/gosnmp/gosnmp"
)

func TestGoSNMPSessionConfigurationPreservesRequestedV3Modes(t *testing.T) {
	cases := []struct {
		name    string
		request Request
		flags   gosnmp.SnmpV3MsgFlags
		auth    gosnmp.SnmpV3AuthProtocol
		privacy gosnmp.SnmpV3PrivProtocol
	}{
		{"noauth", baseV3Request(SecurityNoAuthNoPriv), gosnmp.NoAuthNoPriv, gosnmp.NoAuth, gosnmp.NoPriv},
		{"auth", baseV3Request(SecurityAuthNoPriv), gosnmp.AuthNoPriv, gosnmp.SHA256, gosnmp.NoPriv},
		{"authpriv", baseV3Request(SecurityAuthPriv), gosnmp.AuthPriv, gosnmp.SHA256, gosnmp.AES},
		{"legacy-explicit", func() Request {
			r := baseV3Request(SecurityAuthPriv)
			r.Credentials.V3.AuthProtocol = AuthMD5
			r.Credentials.V3.PrivacyProtocol = PrivacyDES
			return r
		}(), gosnmp.AuthPriv, gosnmp.MD5, gosnmp.DES},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sess, err := newGoSNMPSession(context.Background(), tc.request)
			if err != nil {
				t.Fatalf("newGoSNMPSession() error = %v", err)
			}
			configured := sess.(*goSNMPSession).client
			if configured.MsgFlags != tc.flags {
				t.Fatalf("MsgFlags = %v, want %v", configured.MsgFlags, tc.flags)
			}
			usm, ok := configured.SecurityParameters.(*gosnmp.UsmSecurityParameters)
			if !ok {
				t.Fatalf("SecurityParameters type = %T", configured.SecurityParameters)
			}
			if usm.AuthenticationProtocol != tc.auth || usm.PrivacyProtocol != tc.privacy {
				t.Fatalf("protocols = %v/%v, want %v/%v", usm.AuthenticationProtocol, usm.PrivacyProtocol, tc.auth, tc.privacy)
			}
			if configured.Logger != (gosnmp.Logger{}) {
				t.Fatal("GoSNMP debug logger must remain disabled in production session configuration")
			}
			if configured.Retries != 0 || configured.ExponentialTimeout {
				t.Fatalf("GoSNMP internal retries must be disabled; got Retries=%d ExponentialTimeout=%v", configured.Retries, configured.ExponentialTimeout)
			}
		})
	}
}

func TestGoSNMPVersionMapping(t *testing.T) {
	cases := map[Version]gosnmp.SnmpVersion{
		Version1: gosnmp.Version1, Version2c: gosnmp.Version2c, Version3: gosnmp.Version3,
	}
	for input, want := range cases {
		got, err := goSNMPVersion(input)
		if err != nil || got != want {
			t.Fatalf("version %s mapped to %v, %v; want %v", input, got, err, want)
		}
	}
}
