package synthetic

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

func TestDNSAbsoluteNameResolves(t *testing.T) {
	const name = "probe.example.test."
	for _, tc := range []struct {
		record DNSRecordType
		qtype  dnsmessage.Type
		answer dnsmessage.ResourceBody
		want   string
	}{
		{DNSRecordA, dnsmessage.TypeA, &dnsmessage.AResource{A: [4]byte{192, 0, 2, 73}}, "192.0.2.73"},
		{DNSRecordAAAA, dnsmessage.TypeAAAA, &dnsmessage.AAAAResource{AAAA: [16]byte{0x20, 0x01, 0x0d, 0xb8, 15: 0x49}}, "2001:db8::49"},
	} {
		t.Run(string(tc.record), func(t *testing.T) {
			conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			done := make(chan error, 1)
			go func() {
				var buf [2048]byte
				n, peer, err := conn.ReadFromUDP(buf[:])
				if err != nil {
					done <- err
					return
				}
				var query dnsmessage.Message
				if err := query.Unpack(buf[:n]); err != nil {
					done <- err
					return
				}
				if len(query.Questions) != 1 || query.Questions[0].Name.String() != name ||
					query.Questions[0].Type != tc.qtype || query.Questions[0].Class != dnsmessage.ClassINET {
					done <- fmt.Errorf("unexpected DNS questions %v", query.Questions)
					return
				}
				q := query.Questions[0]
				reply := dnsmessage.Message{
					Header:    dnsmessage.Header{ID: query.ID, Response: true, RecursionAvailable: true},
					Questions: query.Questions,
					Answers:   []dnsmessage.Resource{{Header: dnsmessage.ResourceHeader{Name: q.Name, Class: q.Class, TTL: 60}, Body: tc.answer}},
				}
				raw, err := reply.Pack()
				if err == nil {
					_, err = conn.WriteToUDP(raw, peer)
				}
				done <- err
			}()
			c := newTestCollector(t)
			req := DNSRequest{Hostname: name, RecordType: tc.record, ResolverAddress: conn.LocalAddr().String(), Timeout: time.Second, MaxAnswers: 1}
			result, err := c.DNS(context.Background(), req)
			if err != nil {
				t.Fatalf("absolute DNS lookup returned %v", err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if result.Status != DNSStatusOK || result.Hostname != name || result.RecordType != tc.record ||
				result.Resolver != req.ResolverAddress || !slices.Equal(result.Answers, []string{tc.want}) {
				t.Fatalf("unexpected DNS observation %+v", result)
			}
		})
	}
}

func TestDNSAbsoluteNameValidation(t *testing.T) {
	longest := strings.Repeat(strings.Repeat("a", 63)+".", 3) + strings.Repeat("b", 61) + "."
	for _, tc := range []struct {
		name  string
		bound int
		want  ErrorCode
	}{
		{"relative.test", 13, ""},
		{"absolute.test.", 14, ""},
		{"single.", 7, ""},
		{longest, 254, ""},
		{longest, 253, CodeResourceLimit},
		{"absolute.test.", 13, CodeResourceLimit},
		{"a" + longest, 255, CodeInvalidRequest},
		{strings.Repeat("a.", 127) + "b.", 256, CodeInvalidRequest},
		{".", 8, CodeInvalidRequest},
		{"a..", 8, CodeInvalidRequest},
		{"a..b.", 8, CodeInvalidRequest},
		{"a .", 8, CodeInvalidRequest},
		{" label . ", 32, CodeInvalidRequest},
		{"a-.", 8, CodeInvalidRequest},
		{"192.0.2.1.", 32, CodeInvalidRequest},
		{"2001:db8::1.", 32, CodeInvalidRequest},
	} {
		c := newTestCollector(t)
		c.limits.MaxTargetBytes = tc.bound
		var calls int
		c.dnsLookup = func(ctx context.Context, r DNSRequest) ([]net.IP, error) {
			calls++
			if r.Hostname != tc.name {
				t.Fatalf("resolver received %q instead of %q", r.Hostname, tc.name)
			}
			return []net.IP{net.IPv4(192, 0, 2, 1)}, nil
		}
		_, err := c.DNS(context.Background(), DNSRequest{Hostname: tc.name, RecordType: DNSRecordA, Timeout: time.Second, MaxAnswers: 1})
		wantCalls := 0
		if tc.want == "" {
			wantCalls = 1
		}
		if CodeOf(err) != tc.want || (err == nil) != (tc.want == "") || calls != wantCalls {
			t.Errorf("name %q, bound %d: code=%s calls=%d, want %s", tc.name, tc.bound, CodeOf(err), calls, tc.want)
		}
	}
}
