package synthetic

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

type timingRecorder struct {
	mu                               sync.Mutex
	dnsStart, connectStart, tlsStart time.Time
	dns, connect, tlsHandshake, ttfb *int64
	overallStart                     time.Time
}

func (r *timingRecorder) setDNSStart(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dnsStart.IsZero() {
		r.dnsStart = t
	}
}
func (r *timingRecorder) setDNSDone(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dns == nil && !r.dnsStart.IsZero() {
		v := t.Sub(r.dnsStart).Microseconds()
		r.dns = &v
	}
}
func (r *timingRecorder) setConnectStart(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.connectStart.IsZero() {
		r.connectStart = t
	}
}
func (r *timingRecorder) setConnectDone(t time.Time, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err == nil && r.connect == nil && !r.connectStart.IsZero() {
		v := t.Sub(r.connectStart).Microseconds()
		r.connect = &v
	}
}
func (r *timingRecorder) setTLSStart(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tlsStart.IsZero() {
		r.tlsStart = t
	}
}
func (r *timingRecorder) setTLSDone(t time.Time, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err == nil && r.tlsHandshake == nil && !r.tlsStart.IsZero() {
		v := t.Sub(r.tlsStart).Microseconds()
		r.tlsHandshake = &v
	}
}
func (r *timingRecorder) setTTFB(t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ttfb == nil && !r.overallStart.IsZero() {
		v := t.Sub(r.overallStart).Microseconds()
		r.ttfb = &v
	}
}
func (r *timingRecorder) snapshot(total time.Duration) PhaseTimings {
	r.mu.Lock()
	defer r.mu.Unlock()
	return PhaseTimings{DNSMicros: cloneInt64Ptr(r.dns), ConnectMicros: cloneInt64Ptr(r.connect), TLSHandshakeMicros: cloneInt64Ptr(r.tlsHandshake), TTFBMicros: cloneInt64Ptr(r.ttfb), TotalMicros: total.Microseconds()}
}
func cloneInt64Ptr(in *int64) *int64 {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func (c *Collector) HTTP(parent context.Context, r HTTPRequest) (result HTTPResult, err error) {
	if c == nil {
		return result, newError(CodeInvalidRequest, "synthetic collector is nil", nil, false)
	}
	u, err := validateHTTPRequest(c.limits, r)
	started := c.now().UTC()
	result = HTTPResult{SchemaVersion: ResultSchemaVersion, Method: r.Method, URL: safeURL(r.URL), StartedAt: started}
	if err != nil {
		result.CompletedAt = c.now().UTC()
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	ctx, cancel, err := c.operationContext(parent, r.Timeout)
	if err != nil {
		result.CompletedAt = c.now().UTC()
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	defer cancel()
	roots, err := rootPool(r.CABundlePEM)
	if err != nil {
		result.CompletedAt = c.now().UTC()
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}, DialContext: (&net.Dialer{}).DialContext, ForceAttemptHTTP2: true, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	rec := &timingRecorder{}
	trace := &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { rec.setDNSStart(time.Now()) }, DNSDone: func(httptrace.DNSDoneInfo) { rec.setDNSDone(time.Now()) },
		ConnectStart: func(_, _ string) { rec.setConnectStart(time.Now()) }, ConnectDone: func(_, _ string, e error) { rec.setConnectDone(time.Now(), e) },
		TLSHandshakeStart: func() { rec.setTLSStart(time.Now()) }, TLSHandshakeDone: func(_ tls.ConnectionState, e error) { rec.setTLSDone(time.Now(), e) },
		GotFirstResponseByte: func() { rec.setTTFB(time.Now()) },
	}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), string(r.Method), u.String(), nil)
	if err != nil {
		err = newError(CodeInvalidRequest, "failed to create HTTP request", nil, false)
		result.CompletedAt = c.now().UTC()
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	req.Header = canonicalHeaders(r.Headers)
	userHeaders := make(map[string]struct{}, len(req.Header))
	for k := range req.Header {
		userHeaders[http.CanonicalHeaderKey(k)] = struct{}{}
	}
	redirects := 0
	// Sticky latch: once any redirect crosses scheme/host/effective-port, strip
	// originally configured header names and any Referer (including net/http-
	// generated) on that hop and every subsequent hop (including return to the
	// initial origin). net/http re-copies initial headers on each redirect;
	// immediate-only stripping is insufficient.
	originBoundaryCrossed := false
	client := &http.Client{Transport: transport, CheckRedirect: func(next *http.Request, via []*http.Request) error {
		redirects = len(via)
		if !r.FollowRedirects {
			return http.ErrUseLastResponse
		}
		if len(via) > r.MaxRedirects {
			return newError(CodeResourceLimit, "HTTP redirect count exceeds configured bound", nil, false)
		}
		// URL-only admission on the resolved destination before send. net/http
		// may synthesize Basic Authorization from URL.User after CheckRedirect;
		// reject rather than strip. Initial-request validation remains in
		// validateHTTPRequest with unchanged precedence/codes.
		if _, err := validateHTTPURL(c.limits, next.URL.String()); err != nil {
			return err
		}
		if len(via) > 0 {
			if !sameOrigin(via[len(via)-1].URL, next.URL) {
				originBoundaryCrossed = true
			}
			if originBoundaryCrossed {
				next.Header.Del("Referer")
				for k := range userHeaders {
					next.Header.Del(k)
				}
			}
		}
		return nil
	}}
	rec.overallStart = time.Now()
	// Reading and validating the response body are part of the observed probe.
	defer func() {
		result.Timings = rec.snapshot(time.Since(rec.overallStart))
		result.CompletedAt = c.now().UTC()
	}()
	resp, doErr := client.Do(req)
	result.Redirects = redirects
	if doErr != nil {
		err = classifyHTTP(parent, ctx, doErr)
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	defer closeResponse(resp)
	result.StatusCode = resp.StatusCode
	result.FinalURL = safeURL(resp.Request.URL.String())
	if resp.ContentLength > int64(r.MaxResponseBytes) && resp.ContentLength >= 0 {
		err = newError(CodeResourceLimit, "HTTP response exceeds configured byte limit", nil, false)
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	body, readErr := boundedRead(resp.Body, r.MaxResponseBytes)
	if readErr != nil {
		err = classifyHTTP(parent, ctx, readErr)
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	result.ResponseBytes = len(body)
	if !statusExpected(resp.StatusCode, r.ExpectedStatusCodes) {
		err = newError(CodeHTTPStatus, "HTTP response status did not match expected policy", nil, false)
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	if r.ExpectedContent != "" {
		matched := bytes.Contains(body, []byte(r.ExpectedContent))
		result.ContentMatched = &matched
		if !matched {
			err = newError(CodeContentMismatch, "HTTP response content did not match expected policy", nil, false)
			result.ErrorCode = CodeOf(err)
			c.record(err)
			return result, err
		}
	}
	result.Success = true
	c.record(nil)
	return result, nil
}

func classifyHTTP(parent, op context.Context, err error) error {
	if ce := contextError(parent, op); ce != nil {
		return ce
	}
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	var certVerify *tls.CertificateVerificationError
	var hostname x509.HostnameError
	var authority x509.UnknownAuthorityError
	var certInvalid x509.CertificateInvalidError
	var recordHeader tls.RecordHeaderError
	var alert tls.AlertError
	if errors.As(err, &certVerify) || errors.As(err, &hostname) || errors.As(err, &authority) || errors.As(err, &certInvalid) || errors.As(err, &recordHeader) || errors.As(err, &alert) {
		return classifyTLS(parent, op, err)
	}
	return classifyTransport(parent, op, err)
}
