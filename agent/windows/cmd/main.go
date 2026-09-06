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

// Platform Windows Agent entrypoint / service runner harness
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With(
		"service", "platform-agent-windows",
	)
	slog.SetDefault(logger)

	cfg := config.LoadConfig()
	ident, err := identity.GenerateOrLoadIdentity(cfg.AgentID)
	if err != nil {
		logger.Error("Failed to initialize Windows agent identity", "error", err)
		os.Exit(1)
	}

	logger.Info("Platform Windows Agent starting",
		"version", "0.1.0-dev",
		"agent_id", ident.AgentID,
		"core_url", cfg.CoreURL,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := common.NewAgentService(cfg, ident, logger)
	svc.Start(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	sig := <-sigCh
	logger.Info("Platform Windows Agent stopping on signal", "signal", sig.String())
	cancel()
	svc.Stop()
	logger.Info("Platform Windows Agent exited cleanly")
}
