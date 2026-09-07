package synthetic

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTLSTrustedSuccessAndNegotiatedObservation(t *testing.T) {
	p := makePKI(t, false)
	s, port := startTLSServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	_ = s
	c := newTestCollector(t)
	out, err := c.TLS(context.Background(), TLSRequest{Target: "127.0.0.1", Port: port, ServerName: "127.0.0.1", Timeout: time.Second, CABundlePEM: p.caPEM})
	if err != nil || !out.Success || out.NegotiatedVersion == "" || out.CipherSuite == "" || len(out.PeerCertificates) == 0 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if out.NegotiatedVersion != "TLS1.2" && out.NegotiatedVersion != "TLS1.3" {
		t.Fatalf("version=%s", out.NegotiatedVersion)
	}
}

func TestTLSUntrustedHostnameExpiredAndCancellation(t *testing.T) {
	t.Run("untrusted", func(t *testing.T) {
		p := makePKI(t, false)
		_, port := startTLSServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		c := newTestCollector(t)
		_, err := c.TLS(context.Background(), TLSRequest{Target: "127.0.0.1", Port: port, ServerName: "127.0.0.1", Timeout: time.Second})
		if CodeOf(err) != CodeTLSUntrusted {
			t.Fatalf("code=%s err=%v", CodeOf(err), err)
		}
	})
	t.Run("hostname", func(t *testing.T) {
		p := makePKI(t, false)
		_, port := startTLSServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		c := newTestCollector(t)
		_, err := c.TLS(context.Background(), TLSRequest{Target: "127.0.0.1", Port: port, ServerName: "wrong.example", Timeout: time.Second, CABundlePEM: p.caPEM})
		if CodeOf(err) != CodeTLSHostname {
			t.Fatalf("code=%s err=%v", CodeOf(err), err)
		}
	})
	t.Run("expired", func(t *testing.T) {
		p := makePKI(t, true)
		_, port := startTLSServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		c := newTestCollector(t)
		_, err := c.TLS(context.Background(), TLSRequest{Target: "127.0.0.1", Port: port, ServerName: "127.0.0.1", Timeout: time.Second, CABundlePEM: p.caPEM})
		if CodeOf(err) != CodeTLSExpired {
			t.Fatalf("code=%s err=%v", CodeOf(err), err)
		}
	})
	t.Run("cancel", func(t *testing.T) {
		c := newTestCollector(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := c.TLS(ctx, TLSRequest{Target: "127.0.0.1", Port: 443, ServerName: "127.0.0.1", Timeout: time.Second})
		if CodeOf(err) != CodeCancelled {
			t.Fatalf("code=%s err=%v", CodeOf(err), err)
		}
	})
}

func TestTLSNoInsecureBypassAndBounds(t *testing.T) {
	p := makePKI(t, false)
	_, port := startTLSServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	c := newTestCollector(t)
	// No CA supplied: the private test CA must never be accepted by an insecure bypass.
	if _, err := c.TLS(context.Background(), TLSRequest{Target: "127.0.0.1", Port: port, Timeout: time.Second}); CodeOf(err) != CodeTLSUntrusted {
		t.Fatalf("unexpected acceptance/error=%v", err)
	}
	tooLarge := make([]byte, c.limits.MaxCABundleBytes+1)
	if _, err := c.TLS(context.Background(), TLSRequest{Target: "127.0.0.1", Port: port, Timeout: time.Second, CABundlePEM: tooLarge}); CodeOf(err) != CodeResourceLimit {
		t.Fatalf("err=%v", err)
	}
}

func TestTLSProvidedCAIsExclusiveTrust(t *testing.T) {
	p := makePKI(t, false)
	roots, err := rootPool(p.caPEM)
	if err != nil || roots == nil {
		t.Fatalf("rootPool err=%v roots=%v", err, roots)
	}
	system, sysErr := x509.SystemCertPool()
	if sysErr != nil || system == nil {
		t.Skip("system cert pool unavailable in this environment")
	}
	// A second unrelated CA must not validate the leaf even though system roots exist.
	other := makePKI(t, false)
	otherRoots, err := rootPool(other.caPEM)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(p.certPEM)
	if block == nil {
		t.Fatal("decode leaf")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "localhost"}); err != nil {
		t.Fatalf("exclusive provided CA should trust leaf: %v", err)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: otherRoots, DNSName: "localhost"}); err == nil {
		t.Fatal("foreign exclusive CA unexpectedly trusted leaf")
	}
}

func TestHTTPRequestStringRedactsHeadersContentAndCA(t *testing.T) {
	r := HTTPRequest{URL: "https://example.test/path?token=query-secret", Method: HTTPMethodGET, Headers: map[string]string{"Authorization": "Bearer header-secret"}, Timeout: time.Second, ExpectedContent: "content-secret", CABundlePEM: []byte("ca-secret")}
	for _, formatted := range []string{fmt.Sprintf("%v", r), fmt.Sprintf("%+v", r), fmt.Sprintf("%#v", r)} {
		for _, secret := range []string{"header-secret", "query-secret", "content-secret", "ca-secret"} {
			if strings.Contains(formatted, secret) {
				t.Fatalf("secret %q leaked in %q", secret, formatted)
			}
		}
	}
}

func TestHTTPGetHeadStatusContentAndBound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/large" {
			_, _ = io.WriteString(w, strings.Repeat("x", 128))
			return
		}
		if r.URL.Path == "/status" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		if r.Method == http.MethodHead {
			w.Header().Set("X-Test", "head")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(w, "hello synthetic world")
	}))
	defer server.Close()
	c := newTestCollector(t)
	get, err := c.HTTP(context.Background(), HTTPRequest{URL: server.URL, Method: HTTPMethodGET, Timeout: time.Second, MaxResponseBytes: 64, ExpectedStatusCodes: []int{200}, ExpectedContent: "synthetic"})
	if err != nil || !get.Success || get.ContentMatched == nil || !*get.ContentMatched {
		t.Fatalf("get=%+v err=%v", get, err)
	}
	head, err := c.HTTP(context.Background(), HTTPRequest{URL: server.URL, Method: HTTPMethodHEAD, Timeout: time.Second, MaxResponseBytes: 64, ExpectedStatusCodes: []int{204}})
	if err != nil || !head.Success || head.StatusCode != 204 {
		t.Fatalf("head=%+v err=%v", head, err)
	}
	_, err = c.HTTP(context.Background(), HTTPRequest{URL: server.URL + "/status", Method: HTTPMethodGET, Timeout: time.Second, MaxResponseBytes: 64, ExpectedStatusCodes: []int{200}})
	if CodeOf(err) != CodeHTTPStatus {
		t.Fatalf("status err=%v", err)
	}
	_, err = c.HTTP(context.Background(), HTTPRequest{URL: server.URL, Method: HTTPMethodGET, Timeout: time.Second, MaxResponseBytes: 64, ExpectedContent: "missing"})
	if CodeOf(err) != CodeContentMismatch {
		t.Fatalf("content err=%v", err)
	}
	_, err = c.HTTP(context.Background(), HTTPRequest{URL: server.URL + "/large", Method: HTTPMethodGET, Timeout: time.Second, MaxResponseBytes: 32})
	if CodeOf(err) != CodeResourceLimit {
		t.Fatalf("bound err=%v", err)
	}
}

func TestHTTPRedirectAllowedLimitAndSensitiveHeaderIsolation(t *testing.T) {
	received := make(chan http.Header, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	c := newTestCollector(t)
	out, err := c.HTTP(context.Background(), HTTPRequest{URL: source.URL, Method: HTTPMethodGET, Headers: map[string]string{"Authorization": "Bearer secret", "X-Private": "secret-two"}, Timeout: time.Second, FollowRedirects: true, MaxRedirects: 1, MaxResponseBytes: 64, ExpectedStatusCodes: []int{200}})
	if err != nil || !out.Success {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	h := <-received
	if h.Get("Authorization") != "" || h.Get("X-Private") != "" {
		t.Fatalf("sensitive/custom headers forwarded cross-origin: %v", h)
	}

	loop := httptest.NewServer(nil)
	defer loop.Close()
	loop.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, loop.URL, http.StatusFound) })
	_, err = c.HTTP(context.Background(), HTTPRequest{URL: loop.URL, Method: HTTPMethodGET, Timeout: time.Second, FollowRedirects: true, MaxRedirects: 1, MaxResponseBytes: 64})
	if CodeOf(err) != CodeResourceLimit {
		t.Fatalf("redirect limit err=%v code=%s", err, CodeOf(err))
	}
}

func TestHTTPEmbeddedCredentialsAndHTTPSValidation(t *testing.T) {
	c := newTestCollector(t)
	if _, err := c.HTTP(context.Background(), HTTPRequest{URL: "https://user:pass@example.test/", Method: HTTPMethodGET, Timeout: time.Second, MaxResponseBytes: 64}); CodeOf(err) != CodeInvalidRequest {
		t.Fatalf("userinfo err=%v", err)
	}
	p := makePKI(t, false)
	server, _ := startTLSServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") }))
	trusted, err := c.HTTP(context.Background(), HTTPRequest{URL: server.URL, Method: HTTPMethodGET, Timeout: time.Second, MaxResponseBytes: 64, CABundlePEM: p.caPEM, ExpectedStatusCodes: []int{200}})
	if err != nil || !trusted.Success {
		t.Fatalf("trusted=%+v err=%v", trusted, err)
	}
	_, err = c.HTTP(context.Background(), HTTPRequest{URL: server.URL, Method: HTTPMethodGET, Timeout: time.Second, MaxResponseBytes: 64})
	if CodeOf(err) != CodeTLSUntrusted {
		t.Fatalf("untrusted code=%s err=%v", CodeOf(err), err)
	}
}

func TestHTTPCancellationAndDeadline(t *testing.T) {
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { started <- struct{}{}; <-r.Context().Done() }))
	defer server.Close()
	c := newTestCollector(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.HTTP(ctx, HTTPRequest{URL: server.URL, Method: HTTPMethodGET, Timeout: time.Second, MaxResponseBytes: 64})
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; CodeOf(err) != CodeCancelled {
		t.Fatalf("cancel err=%v code=%s", err, CodeOf(err))
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel2()
	_, err := c.HTTP(ctx2, HTTPRequest{URL: server.URL, Method: HTTPMethodGET, Timeout: time.Second, MaxResponseBytes: 64})
	if CodeOf(err) != CodeDeadlineExceeded {
		t.Fatalf("deadline err=%v code=%s", err, CodeOf(err))
	}
}

func TestConcurrentHTTPExecutionsDoNotShareMutableState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, r.Header.Get("X-Request-ID")) }))
	defer server.Close()
	c := newTestCollector(t)
	const n = 12
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("req-%d", i)
			out, err := c.HTTP(context.Background(), HTTPRequest{URL: server.URL, Method: HTTPMethodGET, Headers: map[string]string{"X-Request-ID": id}, Timeout: time.Second, MaxResponseBytes: 64, ExpectedContent: id})
			if err != nil || !out.Success {
				errs <- fmt.Errorf("id=%s out=%+v err=%w", id, out, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestMetricsLowCardinalityCounters(t *testing.T) {
	c := newTestCollector(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer server.Close()
	_, _ = c.HTTP(context.Background(), HTTPRequest{URL: server.URL, Method: HTTPMethodGET, Timeout: time.Second, MaxResponseBytes: 64, ExpectedStatusCodes: []int{200}})
	snap := c.Snapshot()
	if snap.Executed != 1 || snap.Failed != 1 || snap.HTTPStatusMismatch != 1 {
		t.Fatalf("snapshot=%+v", snap)
	}
}

var _ = tls.VersionTLS13

func TestHTTPErrorDoesNotExposeHeaderSecret(t *testing.T) {
	c := newTestCollector(t)
	secret := "do-not-log-this-token"
	_, err := c.HTTP(context.Background(), HTTPRequest{URL: "https://example.test/", Method: HTTPMethodGET, Headers: map[string]string{"Authorization": "Bearer " + secret, "Bad\nHeader": "x"}, Timeout: time.Second, MaxResponseBytes: 64})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("secret leaked in error: %v", err)
	}
}
