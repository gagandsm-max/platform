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

// Platform Windows Agent entrypoint / compile-boundary harness only.
// Not a production Windows Agent: Service/SCM, Event Log, signed EXE, and MSI are not implemented here.
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})).With(
		"service", "platform-agent-windows",
	)
	slog.SetDefault(logger)

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("Failed to load Windows agent configuration", "error", err)
		os.Exit(1)
	}

	ident, err := identity.LoadOrGenerate(cfg.StateDir, cfg.AgentID, cfg.TenantID)
	if err != nil {
		logger.Error("Failed to initialize Windows agent identity", "error", err)
		os.Exit(1)
	}

	logger.Info("Platform Windows Agent compile-boundary harness starting",
		"version", "0.1.0-dev",
		"agent_id", ident.AgentID,
		"core_url", cfg.CoreURL,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := common.NewAgentService(cfg, ident, nil, logger)
	if err != nil {
		logger.Error("Failed to initialize Windows agent service", "error", err)
		os.Exit(1)
	}

	if err := svc.Start(ctx); err != nil {
		logger.Error("Failed to start Windows agent service", "error", err)
		os.Exit(1)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	sig := <-sigCh
	logger.Info("Platform Windows Agent stopping on signal", "signal", sig.String())
	cancel()
	_ = svc.Stop()
	logger.Info("Platform Windows Agent exited cleanly")
}
