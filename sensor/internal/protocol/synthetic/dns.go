package synthetic

import (
	"context"
	"errors"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (c *Collector) DNS(parent context.Context, r DNSRequest) (DNSResult, error) {
	var result DNSResult
	if c == nil {
		return result, newError(CodeInvalidRequest, "synthetic collector is nil", nil, false)
	}
	started := c.now().UTC()
	result = DNSResult{SchemaVersion: ResultSchemaVersion, Hostname: r.Hostname, RecordType: r.RecordType, Resolver: r.ResolverAddress, StartedAt: started, RCode: DNSRCodeUnavailable, Answers: []string{}}
	err := c.validateDNS(r)
	if err != nil {
		result.Status = DNSStatusFailed
		result.ErrorCode = CodeOf(err)
		result.CompletedAt = c.now().UTC()
		c.record(err)
		return result, err
	}
	ctx, cancel, err := c.operationContext(parent, r.Timeout)
	if err != nil {
		result.Status = DNSStatusFailed
		result.ErrorCode = CodeOf(err)
		result.CompletedAt = c.now().UTC()
		c.record(err)
		return result, err
	}
	defer cancel()
	begin := time.Now()
	ips, lookupErr := c.dnsLookup(ctx, r)
	elapsed := time.Since(begin)
	result.DurationMicros = elapsed.Microseconds()
	result.CompletedAt = c.now().UTC()
	if lookupErr != nil {
		result.Status = DNSStatusFailed
		if ce := contextError(parent, ctx); ce != nil {
			err = ce
		} else {
			var dnsErr *net.DNSError
			if errors.As(lookupErr, &dnsErr) && dnsErr.IsNotFound {
				err = newError(CodeDNSFailure, "DNS name was not found", lookupErr, false)
				result.Status = DNSStatusNotFound
			} else {
				err = classifyTransport(parent, ctx, lookupErr)
				if CodeOf(err) != CodeDNSFailure {
					err = newError(CodeDNSFailure, "DNS lookup failed", lookupErr, IsTransient(err))
				}
			}
		}
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	if len(ips) > r.MaxAnswers {
		err = newError(CodeResourceLimit, "DNS answer count exceeds request bound", nil, false)
		result.Status = DNSStatusFailed
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	seen := make(map[string]struct{}, len(ips))
	answers := make([]string, 0, len(ips))
	for _, ip := range ips {
		if ip == nil {
			continue
		}
		s := ip.String()
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		answers = append(answers, s)
	}
	if len(answers) > r.MaxAnswers {
		err = newError(CodeResourceLimit, "DNS answer count exceeds request bound", nil, false)
		result.Status = DNSStatusFailed
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	sort.Strings(answers)
	result.Answers = answers
	result.Status = DNSStatusOK
	c.record(nil)
	return result, nil
}

func (c *Collector) validateDNS(r DNSRequest) error {
	hostname := strings.TrimSpace(r.Hostname)
	if len(hostname) > c.limits.MaxTargetBytes {
		return newError(CodeResourceLimit, "target exceeds configured length bound", nil, false)
	}
	// Validate only the labels. Keep the optional root dot on the lookup so
	// the resolver cannot append a local search suffix to an absolute name.
	labels := strings.TrimSuffix(hostname, ".")
	if strings.TrimSpace(labels) != labels {
		return newError(CodeInvalidRequest, "hostname is malformed", nil, false)
	}
	if err := validateHost(labels, c.limits.MaxTargetBytes); err != nil {
		return err
	}
	if net.ParseIP(labels) != nil {
		return newError(CodeInvalidRequest, "DNS hostname must not be an IP literal", nil, false)
	}
	if r.RecordType != DNSRecordA && r.RecordType != DNSRecordAAAA {
		return newError(CodeInvalidRequest, "DNS record type must be A or AAAA", nil, false)
	}
	if r.Timeout <= 0 {
		return newError(CodeInvalidRequest, "DNS timeout must be positive", nil, false)
	}
	if r.Timeout > c.limits.MaxTimeout {
		return newError(CodeResourceLimit, "DNS timeout exceeds configured bound", nil, false)
	}
	if r.MaxAnswers <= 0 || r.MaxAnswers > c.limits.MaxDNSAnswers {
		return newError(CodeResourceLimit, "DNS answer limit is outside configured bound", nil, false)
	}
	if r.ResolverAddress != "" {
		host, port, err := net.SplitHostPort(r.ResolverAddress)
		if err != nil || host == "" || port == "" {
			return newError(CodeInvalidRequest, "resolver address must be host:port", nil, false)
		}
		if err := validateHost(strings.Trim(host, "[]"), c.limits.MaxTargetBytes); err != nil {
			return newError(CodeInvalidRequest, "resolver host is invalid", nil, false)
		}
		n, err := strconv.Atoi(port)
		if err != nil || validatePort(n) != nil {
			return newError(CodeInvalidRequest, "resolver port is invalid", nil, false)
		}
	}
	return nil
}

func (c *Collector) lookupDNS(ctx context.Context, r DNSRequest) ([]net.IP, error) {
	resolver := net.DefaultResolver
	if r.ResolverAddress != "" {
		d := net.Dialer{}
		resolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return d.DialContext(ctx, network, r.ResolverAddress)
		}}
	}
	network := "ip4"
	if r.RecordType == DNSRecordAAAA {
		network = "ip6"
	}
	return resolver.LookupIP(ctx, network, strings.TrimSpace(r.Hostname))
}
