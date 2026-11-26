package synthetic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"testing"
	"time"
)

func TestHTTPCompletionIncludesResponseBody(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantContent string
		maxBytes                int
		truncated               bool
		wantCode                ErrorCode
	}{
		{name: "success", body: "ok", wantContent: "ok", maxBytes: 32},
		{name: "content mismatch", body: "ok", wantContent: "absent", maxBytes: 32, wantCode: CodeContentMismatch},
		{name: "stream exceeds bound", body: "too big", maxBytes: 3, wantCode: CodeResourceLimit},
		{name: "truncated response", body: "short", maxBytes: 32, truncated: true, wantCode: CodeTransportFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			firstByte := make(chan struct{})
			type interval struct{ start, end time.Time }
			bodyWait := make(chan interval, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.truncated {
					w.Header().Set("Content-Length", "32")
				}
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				select {
				case <-firstByte:
				case <-r.Context().Done():
					return
				}
				start := time.Now()
				time.Sleep(30 * time.Millisecond)
				bodyWait <- interval{start, time.Now()}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
				GotFirstResponseByte: func() { close(firstByte) },
			})
			out, err := newTestCollector(t).HTTP(ctx, HTTPRequest{
				URL: server.URL, Method: HTTPMethodGET, Timeout: 2 * time.Second,
				ExpectedContent: tc.wantContent, MaxResponseBytes: tc.maxBytes,
			})
			if CodeOf(err) != tc.wantCode || out.ErrorCode != tc.wantCode || out.Success != (tc.wantCode == "") {
				t.Fatalf("success=%v code=%q err=%v", out.Success, out.ErrorCode, err)
			}
			var wait interval
			select {
			case wait = <-bodyWait:
			default:
				t.Fatal("probe returned before response body release")
			}
			if out.Timings.TTFBMicros == nil {
				t.Fatal("first byte timing missing")
			}
			bodyMicros := wait.end.Sub(wait.start).Microseconds()
			if out.CompletedAt.Before(wait.end) || out.Timings.TotalMicros-*out.Timings.TTFBMicros < bodyMicros {
				t.Errorf("completed %v, body released %v; total=%d firstByte=%d bodyWait=%d microseconds",
					out.CompletedAt, wait.end, out.Timings.TotalMicros, *out.Timings.TTFBMicros, bodyMicros)
			}
		})
	}
}
