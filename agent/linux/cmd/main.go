package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"platform/agent/common"
	"platform/agent/common/collector"
	"platform/agent/common/config"
	"platform/agent/common/identity"
	"platform/agent/common/supportability"
	"platform/agent/linux/ebpf"
)

const (
	agentVersion = "0.1.0-dev"

	exitConfig   = 1
	exitIdentity = 2
	exitService  = 3
	exitStart    = 4
	exitStop     = 5
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

	logger.Info("AmrudAi Linux Agent starting",
		"safe_context", map[string]any{
			"host_id":    ident.HostID,
			"log_level":  cfg.LogLevel,
			"state_dir":  "configured",
			"sensor_url": "configured",
		})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := []common.ServiceOption{
		common.WithSupportabilityIdentity(agentVersion, serviceInstanceID),
	}

	// Optional eBPF Backend is constructed only on the Linux production path.
	// Concrete ebpf types never enter agent/common. Disabled remains a static
	// FEATURE_DISABLED capability with no kernel activation attempt.
	if cfg.EBPFEnabled {
		agg, err := collector.NewProcessNetworkAggregator(collector.ProcessNetworkAggregatorConfig{
			SourceProvenance: collector.ProcessNetworkEBPFSourceProvenance,
		}, collector.UnwiredStructuredEvidenceSink{})
		if err != nil {
			logger.Error("Failed to initialize process-network aggregator",
				"event_code", "AGENT_EBPF_AGGREGATOR_INIT_FAILED",
				"safe_context", map[string]any{"reason_class": "initialization_failed"})
			os.Exit(exitService)
		}
		backend, err := ebpf.NewBackend(ebpf.BackendConfig{
			Config: ebpf.Config{
				Enabled:            true,
				PreflightTimeout:   cfg.EBPFPreflightTimeout,
				RecheckMin:         cfg.EBPFRecheckMin,
				RecheckMax:         cfg.EBPFRecheckMax,
				RequestedMapBytes:  ebpf.RequestedMapBytesEstimate,
				RequestedRingBytes: ebpf.RequestedRingBytesExact,
				LowWaterSatisfied:  true,
			},
			Aggregator: agg,
		})
		if err != nil {
			logger.Error("Failed to construct optional eBPF backend",
				"event_code", "AGENT_EBPF_BACKEND_INIT_FAILED",
				"safe_context", map[string]any{"reason_class": "initialization_failed"})
			os.Exit(exitService)
		}
		opts = append(opts, common.WithProcessNetworkRuntime(backend))
		logger.Info("AmrudAi Agent optional eBPF runtime constructed",
			"event_code", "AGENT_EBPF_RUNTIME_WIRED",
			"safe_context", map[string]any{
				"reason_class": "optional_enrichment",
				"activation":   "after_running_async_once",
			})
	} else {
		now := time.Now().UTC()
		opts = append(opts, common.WithProcessNetworkCapability(collector.ProcessNetworkCapabilitySnapshot{
			State:         collector.ProcessNetworkCapabilityUnavailable,
			ReasonCodes:   []collector.ProcessNetworkCapabilityReason{collector.ReasonFeatureDisabled},
			ObservedAt:    now,
			TransitionAt:  now,
			ValidUntil:    now.Add(cfg.EBPFRecheckMin),
			NextRecheckAt: now.Add(cfg.EBPFRecheckMin),
			SourceQuality: "DISABLED",
		}))
	}

	svc, err := common.NewAgentService(cfg, ident, nil, logger, opts...)
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
	logger.Info("AmrudAi Linux Agent received signal, shutting down",
		"safe_context", map[string]any{"signal": sig.String()})
	// AgentService owns runtime cancellation; Stop cancels and bounds shutdown.
	// Parent cancel remains a belt-and-braces signal for any Start-scoped work.
	cancel()
	if err := svc.Stop(); err != nil {
		logger.Error("AmrudAi Linux Agent shutdown failed",
			"event_code", "AGENT_SERVICE_STOP_FAILED",
			"safe_context", map[string]any{"reason_class": "shutdown_failed"})
		os.Exit(exitStop)
	}
	logger.Info("AmrudAi Linux Agent exited cleanly")
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
