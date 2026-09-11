package agentreceiver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// ServerConfig parameterizes the Agent ingress TLS listener.
type ServerConfig struct {
	ListenAddr      string
	ServerCertFile  string
	ServerKeyFile   string
	ClientCAFile    string
	MaxInFlight     int
	ShutdownGrace   time.Duration
	Handler         http.Handler
	Logger          *slog.Logger
}

// Server is the Agent-facing HTTPS listener (RequireAndVerifyClientCert).
type Server struct {
	cfg          ServerConfig
	httpSrv      *http.Server
	listener     net.Listener
	logger       *slog.Logger
	sem          chan struct{}
	mu           sync.Mutex
	bound        bool
	serveStarted bool
}

func NewServer(cfg ServerConfig) (*Server, error) {
	addr := strings.TrimSpace(cfg.ListenAddr)
	if addr == "" {
		return nil, errors.New("ERR_INVALID_CONFIG: AgentReceiverListenAddr is required when Agent receiver is enabled")
	}
	if strings.TrimSpace(cfg.ServerCertFile) == "" || strings.TrimSpace(cfg.ServerKeyFile) == "" {
		return nil, errors.New("ERR_INVALID_CONFIG: Agent server certificate and key are required")
	}
	if strings.TrimSpace(cfg.ClientCAFile) == "" {
		return nil, errors.New("ERR_INVALID_CONFIG: Agent client CA file is required")
	}
	if cfg.Handler == nil {
		return nil, errors.New("ERR_INVALID_CONFIG: Agent receiver handler is required")
	}
	if cfg.MaxInFlight <= 0 {
		cfg.MaxInFlight = 64
	}
	if cfg.ShutdownGrace <= 0 {
		cfg.ShutdownGrace = 10 * time.Second
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	cert, err := tls.LoadX509KeyPair(cfg.ServerCertFile, cfg.ServerKeyFile)
	if err != nil {
		return nil, fmt.Errorf("ERR_INVALID_CONFIG: failed to load Agent server TLS keypair: %w", err)
	}
	caPEM, err := os.ReadFile(cfg.ClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("ERR_INVALID_CONFIG: failed to read Agent client CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("ERR_INVALID_CONFIG: Agent client CA contains no usable certificate")
	}

	sem := make(chan struct{}, cfg.MaxInFlight)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			cfg.Handler.ServeHTTP(w, r)
		default:
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"ERROR","code":"ERR_AGENT_RECEIVER_BUSY","message":"Agent receiver concurrency limit reached"}`))
		}
	})

	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
	}
	httpSrv := &http.Server{
		Addr:      addr,
		Handler:   handler,
		TLSConfig: tlsCfg,
		// Explicit timeouts keep a hung Agent from holding slots forever.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	return &Server{
		cfg:     cfg,
		httpSrv: httpSrv,
		logger:  logger.With("component", "agentreceiver.server"),
		sem:     sem,
	}, nil
}

// Bind reserves the configured TCP listener synchronously so startup cannot
// report success while the Agent ingress address is unavailable.
func (s *Server) Bind() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bound {
		return errors.New("ERR_INVALID_CONFIG: Agent receiver server already bound")
	}
	ln, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("ERR_AGENT_RECEIVER_LISTEN_FAILED: %w", err)
	}
	s.listener = ln
	s.bound = true
	return nil
}

// ListenAndServe starts the TLS server on the listener reserved by Bind. For
// compatibility, it binds itself when the caller has not already done so.
func (s *Server) ListenAndServe() error {
	s.mu.Lock()
	bound := s.bound && s.listener != nil
	s.mu.Unlock()
	if !bound {
		if err := s.Bind(); err != nil {
			return err
		}
	}

	s.mu.Lock()
	if s.serveStarted {
		s.mu.Unlock()
		return errors.New("ERR_INVALID_CONFIG: Agent receiver server already started")
	}
	ln := s.listener
	if ln == nil {
		s.mu.Unlock()
		return errors.New("ERR_AGENT_RECEIVER_LISTEN_FAILED: Agent receiver listener is unavailable")
	}
	s.serveStarted = true
	s.mu.Unlock()

	s.logger.Info("Agent receiver TLS listener started",
		"listen_addr", s.cfg.ListenAddr,
		"client_auth", "RequireAndVerifyClientCert",
	)
	err := s.httpSrv.Serve(tls.NewListener(ln, s.httpSrv.TLSConfig))
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Shutdown stops accepts and drains in-flight requests up to ShutdownGrace.
func (s *Server) Shutdown(ctx context.Context) error {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), s.cfg.ShutdownGrace)
		defer cancel()
	}
	s.logger.Info("Agent receiver shutting down", "grace", s.cfg.ShutdownGrace.String())
	return s.httpSrv.Shutdown(ctx)
}
