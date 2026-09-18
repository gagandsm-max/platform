package coreclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFetchCapabilityPackArtifactRequiresSHAAndPreservesBytes(t *testing.T) {
	payload := []byte(`{"schemaVersion":"1.0.0","packId":"p1","packVersion":"1.0.0"}`)
	sum := sha256.Sum256(payload)
	sha := hex.EncodeToString(sum[:])

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sensor/capability-packs/artifacts/p1/versions/1.0.0" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set(HeaderPackSHA256, sha)
		w.Header().Set(HeaderPackID, "p1")
		w.Header().Set(HeaderPackVersion, "1.0.0")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	client := testClient(t, srv)
	art, err := client.FetchCapabilityPackArtifact(context.Background(), "p1", "1.0.0", len(payload))
	if err != nil {
		t.Fatal(err)
	}
	if string(art.Bytes) != string(payload) {
		t.Fatalf("bytes not preserved")
	}
	if art.ManifestSHA256 != sha {
		t.Fatalf("sha mismatch")
	}
}

func TestFetchCapabilityPackArtifactRejectsMissingSHA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(HeaderPackID, "p1")
		w.Header().Set(HeaderPackVersion, "1.0.0")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	client := testClient(t, srv)
	_, err := client.FetchCapabilityPackArtifact(context.Background(), "p1", "1.0.0", 64)
	if err == nil || !IsCode(err, CodeProtocol) {
		t.Fatalf("expected protocol error, got %v", err)
	}
}

func TestFetchCapabilityPackArtifactBoundsResponse(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(HeaderPackSHA256, strings.Repeat("a", 64))
		w.Header().Set(HeaderPackID, "p1")
		w.Header().Set(HeaderPackVersion, "1.0.0")
		_, _ = io.Copy(w, strings.NewReader(strings.Repeat("x", 100)))
	}))
	defer srv.Close()
	client := testClient(t, srv)
	_, err := client.FetchCapabilityPackArtifact(context.Background(), "p1", "1.0.0", 16)
	if err == nil || !IsCode(err, CodeResponseTooLarge) {
		t.Fatalf("expected too large, got %v", err)
	}
}

func TestCoreClientRefusesRedirects(t *testing.T) {
	final := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"generation":0,"packs":[]}`))
	}))
	defer final.Close()
	redir := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/api/v1/sensor/capability-packs/desired-state", http.StatusFound)
	}))
	defer redir.Close()
	client := testClient(t, redir)
	_, err := client.FetchCapabilityPackDesiredState(context.Background())
	if err == nil {
		t.Fatal("expected redirect refusal")
	}
}

func testClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &Client{
		baseURL: u,
		httpClient: &http.Client{
			Transport: srv.Client().Transport,
			Timeout:   5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return newError(CodeProtocol, "Core redirects are not followed", nil, false)
			},
		},
		maxResponseBytes: 1024 * 1024,
	}
}
