package synthetic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Synthetic fixture credentials only. Never log raw values or Location bodies.
const (
	fixtureRedirectUser = "fixture-redirect-user"
	fixtureRedirectPass = "fixture-redirect-pass"
)

func credentialedURL(base *url.URL, path string) string {
	u := *base
	u.User = url.UserPassword(fixtureRedirectUser, fixtureRedirectPass)
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func assertNoCredentialLeak(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	if strings.Contains(msg, fixtureRedirectUser) || strings.Contains(msg, fixtureRedirectPass) {
		t.Fatalf("diagnostic leaked fixture credentials")
	}
	if strings.Contains(strings.ToLower(msg), "fixture-redirect") {
		t.Fatalf("diagnostic leaked fixture credential material")
	}
}

func TestHTTPRedirectDestinationAdmission(t *testing.T) {
	c := newTestCollector(t)

	t.Run("same_origin_userinfo_get_not_sent", func(t *testing.T) {
		var hits atomic.Int64
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/start":
				http.Redirect(w, r, credentialedURL(mustParseURL(t, srv.URL), "/prohibited"), http.StatusFound)
			case "/prohibited":
				hits.Add(1)
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: srv.URL + "/start", Method: HTTPMethodGET,
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 2,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
		})
		assertNoCredentialLeak(t, err)
		if CodeOf(err) != CodeInvalidRequest || out.Success {
			t.Fatalf("want CodeInvalidRequest, success=%v code=%s err=%v", out.Success, CodeOf(err), err)
		}
		if hits.Load() != 0 {
			t.Fatalf("prohibited same-origin redirect was sent: hits=%d", hits.Load())
		}
	})

	t.Run("same_origin_userinfo_head_not_sent", func(t *testing.T) {
		var hits atomic.Int64
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/start":
				http.Redirect(w, r, credentialedURL(mustParseURL(t, srv.URL), "/prohibited"), http.StatusFound)
			case "/prohibited":
				hits.Add(1)
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: srv.URL + "/start", Method: HTTPMethodHEAD,
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 2,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
		})
		assertNoCredentialLeak(t, err)
		if CodeOf(err) != CodeInvalidRequest || out.Success {
			t.Fatalf("want CodeInvalidRequest, success=%v code=%s err=%v", out.Success, CodeOf(err), err)
		}
		if hits.Load() != 0 {
			t.Fatalf("prohibited same-origin HEAD redirect was sent: hits=%d", hits.Load())
		}
	})

	t.Run("foreign_userinfo_get_not_sent", func(t *testing.T) {
		var hits atomic.Int64
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer target.Close()
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, credentialedURL(mustParseURL(t, target.URL), "/secret"), http.StatusFound)
		}))
		defer source.Close()

		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: source.URL, Method: HTTPMethodGET,
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 2,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
		})
		assertNoCredentialLeak(t, err)
		if CodeOf(err) != CodeInvalidRequest || out.Success {
			t.Fatalf("want CodeInvalidRequest, success=%v code=%s err=%v", out.Success, CodeOf(err), err)
		}
		if hits.Load() != 0 {
			t.Fatalf("prohibited foreign redirect was sent: hits=%d", hits.Load())
		}
	})

	t.Run("foreign_userinfo_head_not_sent", func(t *testing.T) {
		var hits atomic.Int64
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer target.Close()
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, credentialedURL(mustParseURL(t, target.URL), "/secret"), http.StatusFound)
		}))
		defer source.Close()

		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: source.URL, Method: HTTPMethodHEAD,
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 2,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
		})
		assertNoCredentialLeak(t, err)
		if CodeOf(err) != CodeInvalidRequest || out.Success {
			t.Fatalf("want CodeInvalidRequest, success=%v code=%s err=%v", out.Success, CodeOf(err), err)
		}
		if hits.Load() != 0 {
			t.Fatalf("prohibited foreign HEAD redirect was sent: hits=%d", hits.Load())
		}
	})

	t.Run("fragment_not_sent", func(t *testing.T) {
		var hits atomic.Int64
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/start":
				w.Header().Set("Location", srv.URL+"/ok#fixture-fragment")
				w.WriteHeader(http.StatusFound)
			case "/ok":
				hits.Add(1)
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: srv.URL + "/start", Method: HTTPMethodGET,
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 2,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
		})
		if CodeOf(err) != CodeInvalidRequest || out.Success {
			t.Fatalf("want CodeInvalidRequest for fragment, success=%v code=%s err=%v", out.Success, CodeOf(err), err)
		}
		if hits.Load() != 0 {
			t.Fatalf("fragment redirect was sent: hits=%d", hits.Load())
		}
	})

	t.Run("overlong_target_not_sent", func(t *testing.T) {
		var hits atomic.Int64
		var srv *httptest.Server
		overlongPath := "/" + strings.Repeat("a", c.limits.MaxURLBytes+64)
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/start":
				w.Header().Set("Location", overlongPath)
				w.WriteHeader(http.StatusFound)
			case strings.HasPrefix(r.URL.Path, "/aaa"):
				hits.Add(1)
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: srv.URL + "/start", Method: HTTPMethodGET,
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 2,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
		})
		if CodeOf(err) != CodeResourceLimit || out.Success {
			t.Fatalf("want CodeResourceLimit, success=%v code=%s err=%v", out.Success, CodeOf(err), err)
		}
		if hits.Load() != 0 {
			t.Fatalf("overlong redirect was sent: hits=%d", hits.Load())
		}
	})

	t.Run("invalid_scheme_host_port_not_sent", func(t *testing.T) {
		cases := []struct {
			name     string
			location string
		}{
			{name: "scheme", location: "ftp://127.0.0.1/prohibited"},
			// Space-bearing hosts fail net/http Location parse before CheckRedirect;
			// use a parseable host that still fails validateHost (underscore).
			{name: "host", location: "http://bad_host.example/prohibited"},
			{name: "port", location: "http://127.0.0.1:70000/prohibited"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Location", tc.location)
					w.WriteHeader(http.StatusFound)
				}))
				defer source.Close()

				out, err := c.HTTP(context.Background(), HTTPRequest{
					URL: source.URL, Method: HTTPMethodGET,
					Timeout: time.Second, FollowRedirects: true, MaxRedirects: 2,
					MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
				})
				if CodeOf(err) != CodeInvalidRequest || out.Success {
					t.Fatalf("want CodeInvalidRequest, success=%v code=%s err=%v", out.Success, CodeOf(err), err)
				}
			})
		}
	})

	t.Run("normal_relative_followed", func(t *testing.T) {
		var hits atomic.Int64
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/start":
				http.Redirect(w, r, "/ok", http.StatusFound)
			case "/ok":
				hits.Add(1)
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: srv.URL + "/start", Method: HTTPMethodGET,
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 2,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
		})
		if err != nil || !out.Success || out.Redirects != 1 {
			t.Fatalf("relative follow failed: success=%v redirects=%d err=%v", out.Success, out.Redirects, err)
		}
		if hits.Load() != 1 {
			t.Fatalf("relative destination hits=%d want 1", hits.Load())
		}
	})

	t.Run("normal_cross_origin_followed", func(t *testing.T) {
		var hits atomic.Int64
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer target.Close()
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL+"/ok", http.StatusFound)
		}))
		defer source.Close()

		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: source.URL, Method: HTTPMethodGET,
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 2,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
		})
		if err != nil || !out.Success || out.Redirects != 1 {
			t.Fatalf("cross-origin follow failed: success=%v redirects=%d err=%v", out.Success, out.Redirects, err)
		}
		if hits.Load() != 1 {
			t.Fatalf("cross-origin destination hits=%d want 1", hits.Load())
		}
	})

	t.Run("no_follow_skips_admission_and_does_not_contact", func(t *testing.T) {
		var hits atomic.Int64
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer target.Close()
		// Manual Location without http.Redirect HTML body: a credentialed
		// Redirect body can exceed MaxResponseBytes and obscure the no-follow path.
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", credentialedURL(mustParseURL(t, target.URL), "/secret"))
			w.WriteHeader(http.StatusFound)
		}))
		defer source.Close()

		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: source.URL, Method: HTTPMethodGET,
			Timeout: time.Second, FollowRedirects: false, MaxRedirects: 0,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{302},
		})
		assertNoCredentialLeak(t, err)
		if err != nil || !out.Success || out.StatusCode != http.StatusFound {
			t.Fatalf("no-follow: success=%v status=%d err=%v", out.Success, out.StatusCode, err)
		}
		if hits.Load() != 0 {
			t.Fatalf("no-follow contacted credentialed target: hits=%d", hits.Load())
		}
	})

	t.Run("count_limit_precedes_admission_and_does_not_contact", func(t *testing.T) {
		var prohibitedHits atomic.Int64
		var hop1 *httptest.Server
		prohibited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			prohibitedHits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		defer prohibited.Close()
		hop1 = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, credentialedURL(mustParseURL(t, prohibited.URL), "/secret"), http.StatusFound)
		}))
		defer hop1.Close()
		initial := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, hop1.URL+"/next", http.StatusFound)
		}))
		defer initial.Close()

		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: initial.URL, Method: HTTPMethodGET,
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 1,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
		})
		assertNoCredentialLeak(t, err)
		// Second hop exceeds MaxRedirects=1; count guard must win over userinfo admission.
		if CodeOf(err) != CodeResourceLimit || out.Success {
			t.Fatalf("want CodeResourceLimit from count guard, success=%v code=%s err=%v", out.Success, CodeOf(err), err)
		}
		if prohibitedHits.Load() != 0 {
			t.Fatalf("count-limited credentialed hop was sent: hits=%d", prohibitedHits.Load())
		}
	})
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse fixture url: %v", err)
	}
	return u
}
