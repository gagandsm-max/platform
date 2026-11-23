package synthetic

import (
	"context"
	"errors"
	"net"
	"slices"
	"testing"
	"time"
)

func TestDNSResolverFailureObservation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cause     error
		transient bool
	}{
		{"temporary resolver failure", &net.DNSError{Err: "server misbehaving", IsTemporary: true}, true},
		{"generic lookup failure", errors.New("lookup failed"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestCollector(t)
			c.dnsLookup = func(context.Context, DNSRequest) ([]net.IP, error) { return nil, tc.cause }
			out, err := c.DNS(context.Background(), DNSRequest{Hostname: "probe.example.test.", RecordType: DNSRecordA, Timeout: time.Second, MaxAnswers: 1})
			if CodeOf(err) != CodeDNSFailure || !errors.Is(err, tc.cause) || IsTransient(err) != tc.transient {
				t.Fatalf("resolver failure: err=%v transient=%t", err, IsTransient(err))
			}
			if out.Status != DNSStatusFailed || out.ErrorCode != CodeDNSFailure || out.RCode != DNSRCodeUnavailable || len(out.Answers) != 0 {
				t.Fatalf("unexpected DNS observation %+v", out)
			}
		})
	}
}

func TestDNSMultipleAnswersAreSortedAndUnique(t *testing.T) {
	c := newTestCollector(t)
	c.dnsLookup = func(context.Context, DNSRequest) ([]net.IP, error) {
		return []net.IP{net.ParseIP("192.0.2.9"), net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.9")}, nil
	}
	out, err := c.DNS(context.Background(), DNSRequest{Hostname: "probe.example.test", RecordType: DNSRecordA, Timeout: time.Second, MaxAnswers: 3})
	if err != nil || out.Status != DNSStatusOK || !slices.Equal(out.Answers, []string{"192.0.2.1", "192.0.2.9"}) {
		t.Fatalf("unexpected multi-answer DNS observation %+v, err=%v", out, err)
	}
}
