package synthetic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// Fixture-only marker names/values. Never log header values.
const (
	fixturePrivateHeader = "X-Private"
	fixturePrivateValue  = "fixture-private-marker"
)

type headerPresence struct {
	mu    sync.Mutex
	seen  map[string]bool // path -> whether fixturePrivateHeader was present (value never stored)
	count map[string]int
}

func newHeaderPresence() *headerPresence {
	return &headerPresence{seen: make(map[string]bool), count: make(map[string]int)}
}

func (h *headerPresence) record(path string, hdr http.Header) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.count[path]++
	h.seen[path] = hdr.Get(fixturePrivateHeader) != ""
}

func (h *headerPresence) present(path string) (bool, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.seen[path], h.count[path]
}

// TestHTTPRedirectIsolationStickyOriginBoundary exercises the collector entry
// point against loopback fixtures. The multi-hop foreign case must fail on
// original production bytes at the second foreign-origin hop (same foreign
// origin as the first hop): net/http re-copies initial headers, and the prior
// callback only stripped on an immediate origin transition.
func TestHTTPRedirectIsolationStickyOriginBoundary(t *testing.T) {
	t.Run("multi_hop_foreign_second_hop_strips", func(t *testing.T) {
		presence := newHeaderPresence()
		var foreign *httptest.Server
		foreign = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presence.record(r.URL.Path, r.Header)
			switch r.URL.Path {
			case "/hop1":
				http.Redirect(w, r, foreign.URL+"/hop2", http.StatusFound)
			case "/hop2":
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer foreign.Close()

		initial := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presence.record(r.URL.Path, r.Header)
			http.Redirect(w, r, foreign.URL+"/hop1", http.StatusFound)
		}))
		defer initial.Close()

		c := newTestCollector(t)
		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: initial.URL + "/start", Method: HTTPMethodGET,
			Headers: map[string]string{fixturePrivateHeader: fixturePrivateValue},
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 4,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
		})
		if err != nil || !out.Success || out.Redirects != 2 {
			t.Fatalf("collector entry failed: success=%v redirects=%d err=%v", out.Success, out.Redirects, err)
		}

		if present, n := presence.present("/start"); n != 1 || !present {
			t.Fatalf("initial hop: want fixture header present once, got present=%v count=%d", present, n)
		}
		if present, n := presence.present("/hop1"); n != 1 || present {
			t.Fatalf("first foreign hop: want fixture header absent, got present=%v count=%d", present, n)
		}
		// Regression point: second foreign-origin hop (same foreign origin as hop1).
		if present, n := presence.present("/hop2"); n != 1 || present {
			t.Fatalf("second foreign-origin hop: want fixture header absent, got present=%v count=%d", present, n)
		}
	})

	t.Run("same_origin_only_preserves_headers", func(t *testing.T) {
		presence := newHeaderPresence()
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presence.record(r.URL.Path, r.Header)
			switch r.URL.Path {
			case "/a":
				http.Redirect(w, r, srv.URL+"/b", http.StatusFound)
			case "/b":
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		c := newTestCollector(t)
		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: srv.URL + "/a", Method: HTTPMethodGET,
			Headers: map[string]string{fixturePrivateHeader: fixturePrivateValue},
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 2,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
		})
		if err != nil || !out.Success || out.Redirects != 1 {
			t.Fatalf("same-origin collector entry failed: success=%v redirects=%d err=%v", out.Success, out.Redirects, err)
		}
		if present, n := presence.present("/a"); n != 1 || !present {
			t.Fatalf("same-origin start: want header present, got present=%v count=%d", present, n)
		}
		if present, n := presence.present("/b"); n != 1 || !present {
			t.Fatalf("same-origin redirect: want header preserved, got present=%v count=%d", present, n)
		}
	})

	t.Run("return_to_initial_origin_keeps_strip", func(t *testing.T) {
		presence := newHeaderPresence()
		var initial, foreign *httptest.Server
		initial = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presence.record(r.URL.Path, r.Header)
			switch r.URL.Path {
			case "/start":
				http.Redirect(w, r, foreign.URL+"/away", http.StatusFound)
			case "/back":
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer initial.Close()
		foreign = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presence.record(r.URL.Path, r.Header)
			http.Redirect(w, r, initial.URL+"/back", http.StatusFound)
		}))
		defer foreign.Close()

		c := newTestCollector(t)
		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: initial.URL + "/start", Method: HTTPMethodGET,
			Headers: map[string]string{fixturePrivateHeader: fixturePrivateValue},
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 4,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{200},
		})
		if err != nil || !out.Success || out.Redirects != 2 {
			t.Fatalf("return-to-initial collector entry failed: success=%v redirects=%d err=%v", out.Success, out.Redirects, err)
		}
		if present, _ := presence.present("/start"); !present {
			t.Fatalf("initial: want fixture header present")
		}
		if present, _ := presence.present("/away"); present {
			t.Fatalf("foreign hop: want fixture header absent")
		}
		if present, n := presence.present("/back"); n != 1 || present {
			t.Fatalf("return to initial origin: want fixture header still absent, got present=%v count=%d", present, n)
		}
	})

	t.Run("no_follow_stops_at_redirect", func(t *testing.T) {
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("no-follow must not contact redirect target")
		}))
		defer target.Close()
		source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, target.URL, http.StatusFound)
		}))
		defer source.Close()

		c := newTestCollector(t)
		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: source.URL, Method: HTTPMethodGET,
			Headers: map[string]string{fixturePrivateHeader: fixturePrivateValue},
			Timeout: time.Second, FollowRedirects: false, MaxRedirects: 0,
			MaxResponseBytes: 64, ExpectedStatusCodes: []int{302},
		})
		if err != nil || !out.Success {
			t.Fatalf("no-follow: success=%v err=%v", out.Success, err)
		}
		if out.StatusCode != http.StatusFound {
			t.Fatalf("no-follow: want status 302, got %d", out.StatusCode)
		}
		// CheckRedirect still observes the redirect (len(via)=1) before ErrUseLastResponse.
		if out.Redirects != 1 {
			t.Fatalf("no-follow: want redirects=1 (observed, not followed), got %d", out.Redirects)
		}
	})

	t.Run("max_redirects_enforced", func(t *testing.T) {
		var loop *httptest.Server
		loop = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, loop.URL, http.StatusFound)
		}))
		defer loop.Close()

		c := newTestCollector(t)
		_, err := c.HTTP(context.Background(), HTTPRequest{
			URL: loop.URL, Method: HTTPMethodGET,
			Headers: map[string]string{fixturePrivateHeader: fixturePrivateValue},
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 1,
			MaxResponseBytes: 64,
		})
		if CodeOf(err) != CodeResourceLimit {
			t.Fatalf("max redirects: want CodeResourceLimit, got code=%s err=%v", CodeOf(err), err)
		}
	})
}

func TestHTTPRedirectGeneratedRefererIsolation(t *testing.T) {
	const fixturePathQuery = "/start-fixture?marker=fixture-query-marker"
	const fixtureConfiguredReferer = "https://fixture-configured-referer.example/marker"

	newRecorder := func() (func(string, http.Header), func(string) (bool, int)) {
		var mu sync.Mutex
		seen := make(map[string]bool)
		count := make(map[string]int)
		record := func(key string, hdr http.Header) {
			mu.Lock()
			defer mu.Unlock()
			count[key]++
			// Presence only — never store or log Referer values.
			if len(hdr.Values("Referer")) > 0 {
				seen[key] = true
			}
		}
		present := func(key string) (bool, int) {
			mu.Lock()
			defer mu.Unlock()
			return seen[key], count[key]
		}
		return record, present
	}

	methods := []struct {
		name   string
		isHead bool
	}{
		{name: "GET", isHead: false},
		{name: "HEAD", isHead: true},
	}

	for _, m := range methods {
		newReq := func(url string, follow bool, maxRedirects int, expectedStatus int) HTTPRequest {
			req := HTTPRequest{
				URL:                 url,
				Method:              HTTPMethodGET,
				Timeout:             time.Second,
				FollowRedirects:     follow,
				MaxRedirects:        maxRedirects,
				MaxResponseBytes:    256,
				ExpectedStatusCodes: []int{expectedStatus},
			}
			if m.isHead {
				req.Method = HTTPMethodHEAD
			}
			return req
		}

		t.Run(m.name+"/multi_hop_foreign_strips_generated_referer", func(t *testing.T) {
			record, present := newRecorder()
			var foreign *httptest.Server
			foreign = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record("foreign:"+r.URL.Path, r.Header)
				switch r.URL.Path {
				case "/hop1":
					http.Redirect(w, r, foreign.URL+"/hop2", http.StatusFound)
				case "/hop2":
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer foreign.Close()

			initial := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record("initial:"+r.URL.Path, r.Header)
				http.Redirect(w, r, foreign.URL+"/hop1", http.StatusFound)
			}))
			defer initial.Close()

			c := newTestCollector(t)
			out, err := c.HTTP(context.Background(), newReq(initial.URL+fixturePathQuery, true, 4, http.StatusOK))
			if err != nil || !out.Success || out.Redirects != 2 {
				t.Fatalf("multi-hop foreign failed: success=%v redirects=%d err=%v", out.Success, out.Redirects, err)
			}
			if seen, n := present("initial:/start-fixture"); n != 1 || seen {
				t.Fatalf("initial hop: want no Referer once, got present=%v count=%d", seen, n)
			}
			if seen, n := present("foreign:/hop1"); n != 1 || seen {
				t.Fatalf("first foreign hop: want generated Referer stripped, got present=%v count=%d", seen, n)
			}
			if seen, n := present("foreign:/hop2"); n != 1 || seen {
				t.Fatalf("second foreign hop: want generated Referer stripped, got present=%v count=%d", seen, n)
			}
		})

		t.Run(m.name+"/return_to_initial_origin_keeps_generated_referer_stripped", func(t *testing.T) {
			record, present := newRecorder()
			var initial, foreign *httptest.Server
			initial = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record("initial:"+r.URL.Path, r.Header)
				switch r.URL.Path {
				case "/start-fixture":
					http.Redirect(w, r, foreign.URL+"/away", http.StatusFound)
				case "/back":
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer initial.Close()

			foreign = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record("foreign:"+r.URL.Path, r.Header)
				http.Redirect(w, r, initial.URL+"/back", http.StatusFound)
			}))
			defer foreign.Close()

			c := newTestCollector(t)
			out, err := c.HTTP(context.Background(), newReq(initial.URL+fixturePathQuery, true, 4, http.StatusOK))
			if err != nil || !out.Success || out.Redirects != 2 {
				t.Fatalf("return-to-initial failed: success=%v redirects=%d err=%v", out.Success, out.Redirects, err)
			}
			if seen, n := present("initial:/start-fixture"); n != 1 || seen {
				t.Fatalf("initial hop: want no Referer once, got present=%v count=%d", seen, n)
			}
			if seen, n := present("foreign:/away"); n != 1 || seen {
				t.Fatalf("foreign hop: want generated Referer stripped, got present=%v count=%d", seen, n)
			}
			if seen, n := present("initial:/back"); n != 1 || seen {
				t.Fatalf("return-to-initial hop: want generated Referer still stripped, got present=%v count=%d", seen, n)
			}
		})

		t.Run(m.name+"/same_origin_only_preserves_generated_referer", func(t *testing.T) {
			record, present := newRecorder()
			var srv *httptest.Server
			srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record("same:"+r.URL.Path, r.Header)
				switch r.URL.Path {
				case "/start-fixture":
					http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
				case "/next":
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()

			c := newTestCollector(t)
			out, err := c.HTTP(context.Background(), newReq(srv.URL+fixturePathQuery, true, 2, http.StatusOK))
			if err != nil || !out.Success || out.Redirects != 1 {
				t.Fatalf("same-origin failed: success=%v redirects=%d err=%v", out.Success, out.Redirects, err)
			}
			if seen, n := present("same:/start-fixture"); n != 1 || seen {
				t.Fatalf("same-origin start: want no Referer once, got present=%v count=%d", seen, n)
			}
			if seen, n := present("same:/next"); n != 1 || !seen {
				t.Fatalf("same-origin redirect: want generated Referer preserved, got present=%v count=%d", seen, n)
			}
		})

		t.Run(m.name+"/no_follow_stops_without_contacting_foreign_target", func(t *testing.T) {
			record, present := newRecorder()
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record("target", r.Header)
				w.WriteHeader(http.StatusOK)
			}))
			defer target.Close()

			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record("source", r.Header)
				http.Redirect(w, r, target.URL+"/never", http.StatusFound)
			}))
			defer source.Close()

			c := newTestCollector(t)
			out, err := c.HTTP(context.Background(), newReq(source.URL+fixturePathQuery, false, 0, http.StatusFound))
			if err != nil || !out.Success || out.StatusCode != http.StatusFound || out.Redirects != 1 {
				t.Fatalf("no-follow failed: success=%v status=%d redirects=%d err=%v", out.Success, out.StatusCode, out.Redirects, err)
			}
			if seen, n := present("source"); n != 1 || seen {
				t.Fatalf("no-follow source: want no Referer once, got present=%v count=%d", seen, n)
			}
			if seen, n := present("target"); n != 0 || seen {
				t.Fatalf("no-follow target: want zero contact, got present=%v count=%d", seen, n)
			}
		})

		t.Run(m.name+"/configured_referer_stripped_after_origin_boundary", func(t *testing.T) {
			record, present := newRecorder()
			var foreign *httptest.Server
			foreign = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record("foreign:"+r.URL.Path, r.Header)
				switch r.URL.Path {
				case "/hop1":
					http.Redirect(w, r, foreign.URL+"/hop2", http.StatusFound)
				case "/hop2":
					w.WriteHeader(http.StatusOK)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer foreign.Close()

			initial := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				record("initial:"+r.URL.Path, r.Header)
				http.Redirect(w, r, foreign.URL+"/hop1", http.StatusFound)
			}))
			defer initial.Close()

			req := newReq(initial.URL+fixturePathQuery, true, 4, http.StatusOK)
			req.Headers = map[string]string{"Referer": fixtureConfiguredReferer}

			c := newTestCollector(t)
			out, err := c.HTTP(context.Background(), req)
			if err != nil || !out.Success || out.Redirects != 2 {
				t.Fatalf("configured Referer multi-hop failed: success=%v redirects=%d err=%v", out.Success, out.Redirects, err)
			}
			if seen, n := present("initial:/start-fixture"); n != 1 || !seen {
				t.Fatalf("initial hop: want configured Referer once, got present=%v count=%d", seen, n)
			}
			if seen, n := present("foreign:/hop1"); n != 1 || seen {
				t.Fatalf("first foreign hop: want configured/generated Referer stripped, got present=%v count=%d", seen, n)
			}
			if seen, n := present("foreign:/hop2"); n != 1 || seen {
				t.Fatalf("second foreign hop: want Referer still stripped, got present=%v count=%d", seen, n)
			}
		})
	}

	t.Run("https_to_https_trusted_ca_strips_generated_referer", func(t *testing.T) {
		record, present := newRecorder()
		p := makePKI(t, false)

		var foreign *httptest.Server
		foreign, _ = startTLSServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record("foreign:"+r.URL.Path, r.Header)
			switch r.URL.Path {
			case "/hop1":
				http.Redirect(w, r, foreign.URL+"/hop2", http.StatusFound)
			case "/hop2":
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

		initial, _ := startTLSServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record("initial:"+r.URL.Path, r.Header)
			http.Redirect(w, r, foreign.URL+"/hop1", http.StatusFound)
		}))

		c := newTestCollector(t)
		out, err := c.HTTP(context.Background(), HTTPRequest{
			URL: initial.URL + fixturePathQuery, Method: HTTPMethodGET,
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 4,
			MaxResponseBytes: 256, ExpectedStatusCodes: []int{http.StatusOK},
			CABundlePEM: p.caPEM,
		})
		if err != nil || !out.Success || out.Redirects != 2 {
			t.Fatalf("https-to-https failed: success=%v redirects=%d err=%v", out.Success, out.Redirects, err)
		}
		if seen, n := present("initial:/start-fixture"); n != 1 || seen {
			t.Fatalf("https initial hop: want no Referer once, got present=%v count=%d", seen, n)
		}
		if seen, n := present("foreign:/hop1"); n != 1 || seen {
			t.Fatalf("https first foreign hop: want generated Referer stripped, got present=%v count=%d", seen, n)
		}
		if seen, n := present("foreign:/hop2"); n != 1 || seen {
			t.Fatalf("https second foreign hop: want generated Referer stripped, got present=%v count=%d", seen, n)
		}
	})

	t.Run("fresh_probe_resets_origin_boundary_latch", func(t *testing.T) {
		record, present := newRecorder()
		var foreign *httptest.Server
		foreign = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record("foreign:"+r.URL.Path, r.Header)
			w.WriteHeader(http.StatusOK)
		}))
		defer foreign.Close()

		cross := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record("cross:"+r.URL.Path, r.Header)
			http.Redirect(w, r, foreign.URL+"/land", http.StatusFound)
		}))
		defer cross.Close()

		var same *httptest.Server
		same = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			record("same:"+r.URL.Path, r.Header)
			switch r.URL.Path {
			case "/start-fixture":
				http.Redirect(w, r, same.URL+"/next", http.StatusFound)
			case "/next":
				w.WriteHeader(http.StatusOK)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer same.Close()

		c := newTestCollector(t)

		// Probe 1: cross-origin — engages sticky latch and strips generated Referer.
		out1, err := c.HTTP(context.Background(), HTTPRequest{
			URL: cross.URL + fixturePathQuery, Method: HTTPMethodGET,
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 2,
			MaxResponseBytes: 256, ExpectedStatusCodes: []int{http.StatusOK},
		})
		if err != nil || !out1.Success || out1.Redirects != 1 {
			t.Fatalf("fresh-probe cross failed: success=%v redirects=%d err=%v", out1.Success, out1.Redirects, err)
		}
		if seen, n := present("foreign:/land"); n != 1 || seen {
			t.Fatalf("fresh-probe cross foreign: want Referer stripped, got present=%v count=%d", seen, n)
		}

		// Probe 2: independent same-origin-only call must not inherit the prior latch.
		out2, err := c.HTTP(context.Background(), HTTPRequest{
			URL: same.URL + fixturePathQuery, Method: HTTPMethodGET,
			Timeout: time.Second, FollowRedirects: true, MaxRedirects: 2,
			MaxResponseBytes: 256, ExpectedStatusCodes: []int{http.StatusOK},
		})
		if err != nil || !out2.Success || out2.Redirects != 1 {
			t.Fatalf("fresh-probe same-origin failed: success=%v redirects=%d err=%v", out2.Success, out2.Redirects, err)
		}
		if seen, n := present("same:/start-fixture"); n != 1 || seen {
			t.Fatalf("fresh-probe same start: want no Referer once, got present=%v count=%d", seen, n)
		}
		if seen, n := present("same:/next"); n != 1 || !seen {
			t.Fatalf("fresh-probe same redirect: want generated Referer preserved after latch reset, got present=%v count=%d", seen, n)
		}
	})
}
