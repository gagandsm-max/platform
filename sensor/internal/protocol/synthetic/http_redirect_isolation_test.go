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
