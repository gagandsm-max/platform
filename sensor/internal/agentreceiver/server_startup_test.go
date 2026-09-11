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

func TestServerBindFailsSynchronouslyWhenAddressOccupied(t *testing.T) {
	caCert, caKey := mustCreateCA(t)
	serverCert, serverKey := mustCreateServerKeyPair(t, caCert, caKey)

	dir := t.TempDir()
	caFile := writePEM(t, dir, "ca.crt", "CERTIFICATE", caCert.Raw)
	certFile := writePEM(t, dir, "server.crt", "CERTIFICATE", serverCert)
	keyFile := writePEM(t, dir, "server.key", "PRIVATE KEY", mustMarshalEC(t, serverKey))

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	srv, err := NewServer(ServerConfig{
		ListenAddr:     occupied.Addr().String(),
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

	err = srv.Bind()
	if err == nil {
		t.Fatal("expected synchronous bind failure for occupied Agent receiver address")
	}
	if !strings.Contains(err.Error(), "ERR_AGENT_RECEIVER_LISTEN_FAILED") {
		t.Fatalf("expected Agent receiver listen error code, got %v", err)
	}
}
