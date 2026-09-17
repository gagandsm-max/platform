package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"platform/agent/common"
	"platform/agent/common/config"
	"platform/agent/common/identity"
)

const (
	exitConfig   = 1
	exitIdentity = 2
	exitService  = 3
	exitStart    = 4
)

func main() {
	bootstrap := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With("service", "platform-agent-linux")
	slog.SetDefault(bootstrap)

	cfg, err := config.LoadConfig()
	if err != nil {
		bootstrap.Error("Failed to load agent configuration", "error", err)
		os.Exit(exitConfig)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel(cfg.LogLevel),
	})).With("service", "platform-agent-linux")
	slog.SetDefault(logger)

	ident, err := identity.LoadOrGenerate(cfg.StateDir, cfg.AgentID, cfg.TenantID)
	if err != nil {
		logger.Error("Failed to initialize agent identity", "error", err)
		os.Exit(exitIdentity)
	}

	logger.Info("Platform Linux Agent starting",
		"version", "0.1.0-dev",
		"agent_id", ident.AgentID,
		"tenant_id", ident.TenantID,
		"host_id", ident.HostID,
		"sensor_url", cfg.SensorURL,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := common.NewAgentService(cfg, ident, nil, logger)
	if err != nil {
		logger.Error("Failed to initialize agent service", "error", err)
		os.Exit(exitService)
	}

	if err := svc.Start(ctx); err != nil {
		logger.Error("Failed to start agent service", "error", err)
		os.Exit(exitStart)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	logger.Info("Platform Linux Agent received signal, shutting down", "signal", sig.String())
	cancel()
	_ = svc.Stop()
	logger.Info("Platform Linux Agent exited cleanly")
}

func logLevel(value string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
