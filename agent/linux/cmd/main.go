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
	"platform/agent/common/supportability"
)

const (
	agentVersion = "0.1.0-dev"

	exitConfig   = 1
	exitIdentity = 2
	exitService  = 3
	exitStart    = 4
)

func main() {
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelInfo)
	serviceInstanceID := supportability.NewServiceInstanceID()
	bootstrap := supportability.NewJSONLogger(os.Stdout, levelVar, agentVersion, serviceInstanceID)
	slog.SetDefault(bootstrap)

	cfg, err := config.LoadConfig()
	if err != nil {
		bootstrap.Error("Failed to load agent configuration",
			"event_code", "AGENT_CONFIG_INVALID",
			"safe_context", map[string]any{"reason_class": "config_load_failed"})
		os.Exit(exitConfig)
	}
	levelVar.Set(logLevel(cfg.LogLevel))

	ident, err := identity.LoadOrGenerate(cfg.StateDir, cfg.AgentID, cfg.TenantID)
	if err != nil {
		bootstrap.Error("Failed to initialize agent identity",
			"event_code", "AGENT_IDENTITY_INIT_FAILED",
			"safe_context", map[string]any{"reason_class": "identity_init_failed"})
		os.Exit(exitIdentity)
	}

	logger := bootstrap.With("agent_id", ident.AgentID)
	if strings.TrimSpace(ident.TenantID) != "" {
		logger = logger.With("tenant_id", ident.TenantID)
	}
	slog.SetDefault(logger)

	logger.Info("Platform Linux Agent starting",
		"safe_context", map[string]any{
			"host_id":    ident.HostID,
			"log_level":  cfg.LogLevel,
			"state_dir":  "configured",
			"sensor_url": "configured",
		})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc, err := common.NewAgentService(cfg, ident, nil, logger,
		common.WithSupportabilityIdentity(agentVersion, serviceInstanceID))
	if err != nil {
		logger.Error("Failed to initialize agent service",
			"event_code", "AGENT_SERVICE_INIT_FAILED",
			"safe_context", map[string]any{"reason_class": "initialization_failed"})
		os.Exit(exitService)
	}

	if err := svc.Start(ctx); err != nil {
		logger.Error("Failed to start agent service",
			"event_code", "AGENT_SERVICE_START_FAILED",
			"safe_context", map[string]any{"reason_class": "startup_failed"})
		os.Exit(exitStart)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	logger.Info("Platform Linux Agent received signal, shutting down",
		"safe_context", map[string]any{"signal": sig.String()})
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
