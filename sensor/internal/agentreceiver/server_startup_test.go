package agentreceiver

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func newStartupTestServer(t *testing.T, listenAddr string) *Server {
	t.Helper()
	caCert, caKey := mustCreateCA(t)
	serverCert, serverKey := mustCreateServerKeyPair(t, caCert, caKey)

	dir := t.TempDir()
	caFile := writePEM(t, dir, "ca.crt", "CERTIFICATE", caCert.Raw)
	certFile := writePEM(t, dir, "server.crt", "CERTIFICATE", serverCert)
	keyFile := writePEM(t, dir, "server.key", "PRIVATE KEY", mustMarshalEC(t, serverKey))

	srv, err := NewServer(ServerConfig{
		ListenAddr:     listenAddr,
		ServerCertFile: certFile,
		ServerKeyFile:  keyFile,
		ClientCAFile:   caFile,
		MaxInFlight:    1,
		ShutdownGrace:  time.Second,
		Handler:        http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		Logger:         slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func TestServerBindFailsSynchronouslyWhenAddressOccupied(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	srv := newStartupTestServer(t, occupied.Addr().String())
	err = srv.Bind()
	if err == nil {
		t.Fatal("expected synchronous bind failure for occupied Agent receiver address")
	}
	if !strings.Contains(err.Error(), "ERR_AGENT_RECEIVER_LISTEN_FAILED") {
		t.Fatalf("expected Agent receiver listen error code, got %v", err)
	}
}

func TestServerListenAndServeRemainsOneShotAfterExplicitBind(t *testing.T) {
	srv := newStartupTestServer(t, "127.0.0.1:0")
	if err := srv.Bind(); err != nil {
		t.Fatal(err)
	}
	// Close the reserved listener so the first Serve attempt terminates
	// deterministically without a goroutine or timing dependency.
	if err := srv.listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := srv.ListenAndServe(); err == nil {
		t.Fatal("expected first Serve attempt on closed listener to fail")
	}
	if err := srv.ListenAndServe(); err == nil || !strings.Contains(err.Error(), "already started") {
		t.Fatalf("expected second Serve attempt to be rejected as already started, got %v", err)
	}
}
