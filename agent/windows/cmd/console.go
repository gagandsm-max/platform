package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"platform/agent/common"
)

var (
	errConsoleStart = errors.New("AGENT_CONSOLE_START_FAILED")
	errConsoleStop  = errors.New("AGENT_CONSOLE_STOP_FAILED")
)

// runConsole starts AgentService under console/debug control (SIGINT/SIGTERM).
// Used on non-Windows hosts for the compile boundary and on Windows when not
// executing as an SCM service (or when PLATFORM_AGENT_CONSOLE / -console is set).
func runConsole(agent *common.AgentService, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("AmrudAi Windows Agent console mode",
		"event_code", "AGENT_CONSOLE_MODE",
		"safe_context", map[string]any{"reason_class": "console_debug"})

	if err := agent.Start(context.Background()); err != nil {
		logger.Error("Failed to start Windows agent service",
			"event_code", "AGENT_SERVICE_START_FAILED",
			"safe_context", map[string]any{"reason_class": "startup_failed"})
		return fmt.Errorf("%w: %w", errConsoleStart, err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	sig := <-sigCh
	logger.Info("AmrudAi Windows Agent received signal, shutting down",
		"safe_context", map[string]any{"signal": sig.String()})

	// AgentService owns runtime cancellation; Stop cancels and bounds shutdown.
	if err := agent.Stop(); err != nil {
		logger.Error("AmrudAi Windows Agent shutdown failed",
			"event_code", "AGENT_SERVICE_STOP_FAILED",
			"safe_context", map[string]any{"reason_class": "shutdown_failed"})
		return fmt.Errorf("%w: %w", errConsoleStop, err)
	}
	logger.Info("AmrudAi Windows Agent exited cleanly")
	return nil
}
