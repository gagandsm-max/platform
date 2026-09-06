package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"platform/agent/common"
	"platform/agent/common/config"
	"platform/agent/common/identity"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With(
		"service", "platform-agent-linux",
	)
	slog.SetDefault(logger)

	cfg := config.LoadConfig()
	ident, err := identity.GenerateOrLoadIdentity(cfg.AgentID)
	if err != nil {
		logger.Error("Failed to initialize agent identity", "error", err)
		os.Exit(1)
	}

	logger.Info("Platform Linux Agent starting",
		"version", "0.1.0-dev",
		"agent_id", ident.AgentID,
		"core_url", cfg.CoreURL,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := common.NewAgentService(cfg, ident, logger)
	svc.Start(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	logger.Info("Platform Linux Agent received signal, shutting down", "signal", sig.String())
	cancel()
	svc.Stop()
	logger.Info("Platform Linux Agent exited cleanly")
}
