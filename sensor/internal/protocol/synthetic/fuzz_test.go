package synthetic

import (
	"testing"
	"time"
)

func FuzzSyntheticPayloadDecode(f *testing.F) {
	f.Add([]byte(`{"schemaVersion":1,"target":"127.0.0.1","port":443,"timeoutMillis":100}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`not-json`))
	f.Fuzz(func(t *testing.T, data []byte) { var p tcpJobRequest; _ = decodeStrict(data, &p) })
}

func FuzzHTTPRequestValidation(f *testing.F) {
	f.Add("https://example.test/", "GET")
	f.Add("https://user:pass@example.test/", "GET")
	f.Add("not a url", "HEAD")
	l := testLimits()
	f.Fuzz(func(t *testing.T, raw, method string) {
		if len(raw) > l.MaxURLBytes+32 {
			return
		}
		r := HTTPRequest{URL: raw, Method: HTTPMethod(method), Timeout: time.Second, MaxResponseBytes: 64}
		_, _ = validateHTTPRequest(l, r)
	})
}
