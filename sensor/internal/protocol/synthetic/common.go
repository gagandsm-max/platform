package synthetic

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type tcpDialFunc func(context.Context, string, string) (net.Conn, error)
type dnsLookupFunc func(context.Context, DNSRequest) ([]net.IP, error)

type Collector struct {
	limits    Limits
	tcpDial   tcpDialFunc
	dnsLookup dnsLookupFunc
	now       func() time.Time
	metrics   metrics
}

func NewCollector(l Limits) (*Collector, error) {
	if err := validateLimits(l); err != nil {
		return nil, err
	}
	c := &Collector{limits: l, now: time.Now}
	c.tcpDial = func(ctx context.Context, network, address string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, address)
	}
	c.dnsLookup = c.lookupDNS
	return c, nil
}

func (c *Collector) Snapshot() MetricsSnapshot {
	if c == nil {
		return MetricsSnapshot{}
	}
	return c.metrics.snapshot()
}

func validateLimits(l Limits) error {
	switch {
	case l.MaxTimeout <= 0, l.MaxSchedulerPayloadBytes <= 0, l.MaxResultBytes <= 0, l.MaxTargetBytes <= 0, l.MaxURLBytes <= 0,
		l.MaxDNSAnswers <= 0, l.MaxRedirects <= 0, l.MaxResponseBytes <= 0, l.MaxExpectedStatuses <= 0, l.MaxExpectedContentBytes <= 0,
		l.MaxHeaders <= 0, l.MaxHeaderNameBytes <= 0, l.MaxHeaderValueBytes <= 0, l.MaxCABundleBytes <= 0,
		l.MaxCertificateChain <= 0, l.MaxSANs <= 0, l.MaxDiagnosticTextBytes <= 0:
		return newError(CodeInvalidRequest, "all synthetic limits must be explicitly positive", nil, false)
	}
	return nil
}

func (c *Collector) record(err error) {
	if c == nil {
		return
	}
	c.metrics.executed.Add(1)
	if err == nil {
		c.metrics.successful.Add(1)
		return
	}
	c.metrics.failed.Add(1)
	switch CodeOf(err) {
	case CodeTimeout, CodeDeadlineExceeded:
		c.metrics.timeout.Add(1)
	case CodeCancelled:
		c.metrics.cancelled.Add(1)
	case CodeTLSUntrusted, CodeTLSHostname, CodeTLSExpired, CodeTLSHandshake:
		c.metrics.tlsFailures.Add(1)
	case CodeHTTPStatus:
		c.metrics.httpStatusMismatch.Add(1)
	case CodeContentMismatch:
		c.metrics.contentMismatch.Add(1)
	case CodeResourceLimit:
		c.metrics.resourceLimit.Add(1)
	}
}

func (c *Collector) operationContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc, error) {
	if parent == nil {
		return nil, nil, newError(CodeInvalidRequest, "context must not be nil", nil, false)
	}
	if timeout <= 0 {
		return nil, nil, newError(CodeInvalidRequest, "timeout must be positive", nil, false)
	}
	if timeout > c.limits.MaxTimeout {
		return nil, nil, newError(CodeResourceLimit, "timeout exceeds configured bound", nil, false)
	}
	if err := parent.Err(); err != nil {
		return nil, nil, contextError(parent, nil)
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	return ctx, cancel, nil
}

func validateHost(host string, max int) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return newError(CodeInvalidRequest, "target hostname/address is required", nil, false)
	}
	if len(host) > max {
		return newError(CodeResourceLimit, "target exceeds configured length bound", nil, false)
	}
	if strings.ContainsAny(host, "/\\?#@\x00\r\n\t ") {
		return newError(CodeInvalidRequest, "target hostname/address is malformed", nil, false)
	}
	if ip := net.ParseIP(host); ip != nil {
		return nil
	}
	if len(host) > 253 || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return newError(CodeInvalidRequest, "hostname is malformed", nil, false)
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return newError(CodeInvalidRequest, "hostname is malformed", nil, false)
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
				return newError(CodeInvalidRequest, "hostname is malformed", nil, false)
			}
		}
	}
	return nil
}

func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return newError(CodeInvalidRequest, "port must be between 1 and 65535", nil, false)
	}
	return nil
}

func validateCABundle(l Limits, pemBytes []byte) error {
	if len(pemBytes) > l.MaxCABundleBytes {
		return newError(CodeResourceLimit, "CA bundle exceeds configured bound", nil, false)
	}
	return nil
}

func rootPool(ca []byte) (*x509.CertPool, error) {
	if len(ca) == 0 {
		return nil, nil
	}
	// Exclusive trust: when a CA bundle is supplied it is the only accepted root
	// set. Merging into the system pool would silently keep public CAs trusted and
	// defeat explicit probe pinning.
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, newError(CodeTLSUntrusted, "CA bundle contains no usable certificate", nil, false)
	}
	return roots, nil
}

func validateHTTPHeaders(l Limits, h map[string]string) error {
	if len(h) > l.MaxHeaders {
		return newError(CodeResourceLimit, "HTTP header count exceeds configured bound", nil, false)
	}
	for k, v := range h {
		if len(k) == 0 || len(k) > l.MaxHeaderNameBytes || len(v) > l.MaxHeaderValueBytes {
			return newError(CodeResourceLimit, "HTTP header exceeds configured bound", nil, false)
		}
		if strings.ContainsAny(k, "\r\n") || strings.ContainsAny(v, "\r\n") {
			return newError(CodeInvalidRequest, "HTTP header contains prohibited control characters", nil, false)
		}
		if textproto.CanonicalMIMEHeaderKey(k) == "" {
			return newError(CodeInvalidRequest, "HTTP header name is invalid", nil, false)
		}
		switch strings.ToLower(k) {
		case "host", "connection", "proxy-connection", "transfer-encoding", "content-length", "trailer", "upgrade":
			return newError(CodeInvalidRequest, "HTTP hop-by-hop or transport-controlled header is not allowed", nil, false)
		}
	}
	return nil
}

// validateHTTPURL applies the URL-only admission rules shared by the initial
// HTTP synthetic request and every resolved followed-redirect destination:
// absolute http/https, size bound, no userinfo, no fragment, valid host/port.
// Method, timeout, header, redirect-policy and body expectations remain in
// validateHTTPRequest so initial-request precedence and codes stay unchanged.
func validateHTTPURL(l Limits, raw string) (*url.URL, error) {
	if len(raw) == 0 {
		return nil, newError(CodeInvalidRequest, "HTTP URL is required", nil, false)
	}
	if len(raw) > l.MaxURLBytes {
		return nil, newError(CodeResourceLimit, "HTTP URL exceeds configured bound", nil, false)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, newError(CodeInvalidRequest, "HTTP URL must be absolute http or https URL", nil, false)
	}
	if u.User != nil {
		return nil, newError(CodeInvalidRequest, "embedded URL credentials are not allowed", nil, false)
	}
	if u.Fragment != "" {
		return nil, newError(CodeInvalidRequest, "HTTP URL fragments are not allowed", nil, false)
	}
	if err := validateHost(u.Hostname(), l.MaxTargetBytes); err != nil {
		return nil, err
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || validatePort(n) != nil {
			return nil, newError(CodeInvalidRequest, "HTTP URL port is invalid", nil, false)
		}
	}
	return u, nil
}

func validateHTTPRequest(l Limits, r HTTPRequest) (*url.URL, error) {
	u, err := validateHTTPURL(l, r.URL)
	if err != nil {
		return nil, err
	}
	if r.Method != HTTPMethodGET && r.Method != HTTPMethodHEAD {
		return nil, newError(CodeInvalidRequest, "HTTP method must be GET or HEAD", nil, false)
	}
	if r.Timeout <= 0 {
		return nil, newError(CodeInvalidRequest, "HTTP timeout must be positive", nil, false)
	}
	if r.Timeout > l.MaxTimeout {
		return nil, newError(CodeResourceLimit, "HTTP timeout exceeds configured bound", nil, false)
	}
	if r.MaxResponseBytes <= 0 || r.MaxResponseBytes > l.MaxResponseBytes {
		return nil, newError(CodeResourceLimit, "HTTP response byte limit is outside configured bound", nil, false)
	}
	if r.FollowRedirects {
		if r.MaxRedirects <= 0 || r.MaxRedirects > l.MaxRedirects {
			return nil, newError(CodeResourceLimit, "HTTP redirect limit is outside configured bound", nil, false)
		}
	} else if r.MaxRedirects != 0 {
		return nil, newError(CodeInvalidRequest, "redirect limit must be zero when redirects are disabled", nil, false)
	}
	if len(r.ExpectedStatusCodes) > l.MaxExpectedStatuses {
		return nil, newError(CodeResourceLimit, "expected status list exceeds configured bound", nil, false)
	}
	seen := map[int]struct{}{}
	for _, status := range r.ExpectedStatusCodes {
		if status < 100 || status > 599 {
			return nil, newError(CodeInvalidRequest, "expected HTTP status is invalid", nil, false)
		}
		if _, ok := seen[status]; ok {
			return nil, newError(CodeInvalidRequest, "expected HTTP status list contains duplicates", nil, false)
		}
		seen[status] = struct{}{}
	}
	if len(r.ExpectedContent) > l.MaxExpectedContentBytes {
		return nil, newError(CodeResourceLimit, "expected content exceeds configured bound", nil, false)
	}
	if r.Method == HTTPMethodHEAD && r.ExpectedContent != "" {
		return nil, newError(CodeInvalidRequest, "HEAD synthetic cannot require response body content", nil, false)
	}
	if err := validateHTTPHeaders(l, r.Headers); err != nil {
		return nil, err
	}
	if err := validateCABundle(l, r.CABundlePEM); err != nil {
		return nil, err
	}
	return u, nil
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	norm := func(u *url.URL) string {
		port := u.Port()
		if port == "" {
			if u.Scheme == "https" {
				port = "443"
			} else if u.Scheme == "http" {
				port = "80"
			}
		}
		return strings.ToLower(u.Scheme) + "|" + strings.ToLower(u.Hostname()) + "|" + port
	}
	return norm(a) == norm(b)
}

func boundedRead(r io.Reader, max int) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(max)+1))
	if err != nil {
		return nil, newError(CodeTransportFailure, "failed reading HTTP response", err, true)
	}
	if len(data) > max {
		return nil, newError(CodeResourceLimit, "HTTP response exceeds configured byte limit", nil, false)
	}
	return data, nil
}

func tlsConfigFor(serverName string, ca []byte) (*tls.Config, error) {
	roots, err := rootPool(ca)
	if err != nil {
		return nil, err
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, ServerName: serverName, RootCAs: roots}, nil
}

func statusExpected(code int, expected []int) bool {
	if len(expected) == 0 {
		return true
	}
	for _, v := range expected {
		if code == v {
			return true
		}
	}
	return false
}

func closeResponse(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func finiteMillis(ms int64) (time.Duration, error) {
	if ms <= 0 {
		return 0, newError(CodeInvalidRequest, "timeout milliseconds must be positive", nil, false)
	}
	const maxMillis = int64(^uint64(0)>>1) / int64(time.Millisecond)
	if ms > maxMillis {
		return 0, newError(CodeResourceLimit, "duration exceeds representable bound", nil, false)
	}
	return time.Duration(ms) * time.Millisecond, nil
}

func joinAddress(host string, port int) string { return net.JoinHostPort(host, strconv.Itoa(port)) }

func canonicalHeaders(h map[string]string) http.Header {
	out := make(http.Header, len(h))
	for _, k := range sortedHeaderKeys(h) {
		out.Set(k, h[k])
	}
	return out
}

func diagnosticBound(s string, max int) error {
	if len(s) > max {
		return newError(CodeResourceLimit, "certificate diagnostic text exceeds configured bound", nil, false)
	}
	return nil
}
