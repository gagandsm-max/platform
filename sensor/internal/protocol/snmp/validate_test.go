package snmp

import (
	"testing"

	"github.com/gosnmp/gosnmp"
)

func TestValidSNMPv1Config(t *testing.T) {
	req := baseV2Request()
	req.Version = Version1
	if err := ValidateRequest(req); err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
}

func TestValidSNMPv2cConfig(t *testing.T) {
	if err := ValidateRequest(baseV2Request()); err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
}

func TestValidSNMPv3SecurityLevels(t *testing.T) {
	for _, level := range []SecurityLevel{SecurityNoAuthNoPriv, SecurityAuthNoPriv, SecurityAuthPriv} {
		t.Run(string(level), func(t *testing.T) {
			if err := ValidateRequest(baseV3Request(level)); err != nil {
				t.Fatalf("ValidateRequest() error = %v", err)
			}
		})
	}
}

func TestInconsistentSNMPv3CredentialsRejected(t *testing.T) {
	cases := []Request{
		func() Request {
			r := baseV3Request(SecurityAuthPriv)
			r.Credentials.V3.AuthSecret = ""
			return r
		}(),
		func() Request {
			r := baseV3Request(SecurityAuthNoPriv)
			r.Credentials.V3.PrivacyProtocol = PrivacyAES
			r.Credentials.V3.PrivacySecret = Secret(testPrivacy)
			return r
		}(),
		func() Request {
			r := baseV3Request(SecurityNoAuthNoPriv)
			r.Credentials.V3.AuthProtocol = AuthSHA256
			return r
		}(),
	}
	for i, req := range cases {
		if err := ValidateRequest(req); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}

func TestMalformedOIDRejected(t *testing.T) {
	for _, oid := range []string{"", ".1", ".1.040.1", ".3.1.2", ".1.40.1", ".1.a.3", ".1..3", ".1.4294967296.0"} {
		t.Run(oid, func(t *testing.T) {
			if _, err := CanonicalOID(oid); err == nil {
				t.Fatalf("CanonicalOID(%q) expected error", oid)
			}
		})
	}
}

func TestEmptyTargetRejected(t *testing.T) {
	req := baseV2Request()
	req.Target = ""
	if err := ValidateRequest(req); !IsCode(err, CodeInvalidRequest) {
		t.Fatalf("error = %v, want %s", err, CodeInvalidRequest)
	}
}

func TestInvalidPortRejected(t *testing.T) {
	for _, port := range []int{0, -1, 65536} {
		req := baseV2Request()
		req.Port = port
		if err := ValidateRequest(req); !IsCode(err, CodeInvalidRequest) {
			t.Fatalf("port %d error = %v, want %s", port, err, CodeInvalidRequest)
		}
	}
}

func TestOIDRequestCountBound(t *testing.T) {
	req := baseV2Request()
	req.Limits.MaxOIDs = 1
	req.OIDs = []string{".1.3.6.1.2.1.1.1.0", ".1.3.6.1.2.1.1.3.0"}
	if err := ValidateRequest(req); !IsCode(err, CodeResourceLimit) {
		t.Fatalf("error = %v, want %s", err, CodeResourceLimit)
	}
}

func TestSNMPv1BulkOperationsRejected(t *testing.T) {
	for _, operation := range []Operation{OperationGetBulk, OperationBulkWalk} {
		req := baseV2Request()
		req.Version = Version1
		req.Operation = operation
		if err := ValidateRequest(req); !IsCode(err, CodeUnsupportedSecurity) {
			t.Fatalf("operation %s error = %v, want %s", operation, err, CodeUnsupportedSecurity)
		}
	}
}

func TestWeakAlgorithmsRequireExplicitSelection(t *testing.T) {
	req := baseV3Request(SecurityAuthNoPriv)
	req.Credentials.V3.AuthProtocol = ""
	if err := ValidateRequest(req); !IsCode(err, CodeInvalidRequest) {
		t.Fatalf("missing auth protocol error = %v", err)
	}

	req = baseV3Request(SecurityAuthNoPriv)
	req.Credentials.V3.AuthProtocol = AuthMD5
	if err := ValidateRequest(req); err != nil {
		t.Fatalf("explicit MD5 compatibility selection rejected: %v", err)
	}
	if !IsLegacyAuthProtocol(AuthMD5) || !IsLegacyAuthProtocol(AuthSHA1) {
		t.Fatal("legacy auth protocol classification incorrect")
	}

	req = baseV3Request(SecurityAuthPriv)
	req.Credentials.V3.PrivacyProtocol = PrivacyDES
	if err := ValidateRequest(req); err != nil {
		t.Fatalf("explicit DES compatibility selection rejected: %v", err)
	}
	if !IsLegacyPrivacyProtocol(PrivacyDES) {
		t.Fatal("legacy privacy protocol classification incorrect")
	}
}

func TestSecurityModesMapWithoutDowngrade(t *testing.T) {
	levels := map[SecurityLevel]gosnmp.SnmpV3MsgFlags{
		SecurityNoAuthNoPriv: gosnmp.NoAuthNoPriv,
		SecurityAuthNoPriv:   gosnmp.AuthNoPriv,
		SecurityAuthPriv:     gosnmp.AuthPriv,
	}
	for input, want := range levels {
		got, err := goSNMPSecurityLevel(input)
		if err != nil || got != want {
			t.Fatalf("level %s mapped to %v, %v; want %v", input, got, err, want)
		}
	}

	auth := map[AuthProtocol]gosnmp.SnmpV3AuthProtocol{
		AuthNone: gosnmp.NoAuth, AuthMD5: gosnmp.MD5, AuthSHA1: gosnmp.SHA,
		AuthSHA224: gosnmp.SHA224, AuthSHA256: gosnmp.SHA256, AuthSHA384: gosnmp.SHA384, AuthSHA512: gosnmp.SHA512,
	}
	for input, want := range auth {
		got, err := goSNMPAuthProtocol(input)
		if err != nil || got != want {
			t.Fatalf("auth %s mapped to %v, %v; want %v", input, got, err, want)
		}
	}

	privacy := map[PrivacyProtocol]gosnmp.SnmpV3PrivProtocol{
		PrivacyNone: gosnmp.NoPriv, PrivacyDES: gosnmp.DES, PrivacyAES: gosnmp.AES,
		PrivacyAES192: gosnmp.AES192, PrivacyAES256: gosnmp.AES256, PrivacyAES192C: gosnmp.AES192C, PrivacyAES256C: gosnmp.AES256C,
	}
	for input, want := range privacy {
		got, err := goSNMPPrivacyProtocol(input)
		if err != nil || got != want {
			t.Fatalf("privacy %s mapped to %v, %v; want %v", input, got, err, want)
		}
	}
}

func FuzzCanonicalOID(f *testing.F) {
	for _, seed := range []string{".1.3.6.1.2.1.1.3.0", "1.3.6.1", "", ".1..3", ".2.4294967295"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		_, _ = CanonicalOID(input)
	})
}
