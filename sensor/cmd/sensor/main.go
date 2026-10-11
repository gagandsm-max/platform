package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"platform/sensor/internal/agentdrain"
	"platform/sensor/internal/agentreceiver"
	"platform/sensor/internal/collector"
	"platform/sensor/internal/config"
	"platform/sensor/internal/control"
	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/discovery"
	"platform/sensor/internal/health"
	"platform/sensor/internal/observation"
	"platform/sensor/internal/packsync"
	sensorruntime "platform/sensor/internal/runtime"
	"platform/sensor/internal/scheduler"
	"platform/sensor/internal/spool"
	"platform/sensor/internal/supportability"
	"platform/sensor/internal/uploader"
)

// newProductionScheduler constructs the single production-owned scheduler.
// The caller supplies the registry shared with the Universal Sensor runtime.
func newProductionScheduler(rt config.RuntimeConfig, registry *sensorruntime.ExecutorRegistry, logger *slog.Logger) (*scheduler.Scheduler, error) {
	if registry == nil {
		return nil, fmt.Errorf("SENSOR_RUNTIME_INVALID_CONFIG: executor registry is required")
	}
	schedCfg, err := config.ToSchedulerConfig(rt.Scheduler)
	if err != nil {
		return nil, err
	}
	return scheduler.New(schedCfg, registry, scheduler.WithLogger(logger))
}

func coreClientConfigFromRuntime(rt config.RuntimeConfig, logger *slog.Logger) coreclient.Config {
	return coreclient.Config{
		BaseURL:          rt.CoreEndpoint,
		CertificateFile:  rt.TLS.Certificate,
		PrivateKeyFile:   rt.TLS.PrivateKey,
		CAFile:           rt.TLS.CA,
		RequestTimeout:   rt.Transport.RequestTimeout,
		MaxResponseBytes: rt.Transport.MaxResponseBytes,
		Logger:           logger,
	}
}

func uploaderConfigFromRuntime(rt config.RuntimeConfig, streamID string) uploader.Config {
	return uploader.Config{
		SensorID:             rt.SensorID,
		StreamID:             streamID,
		MaxBatchRecords:      rt.Transport.UploadBatchRecords,
		MaxBatchPayloadBytes: rt.Transport.UploadBatchBytes,
		MaxRequestBytes:      rt.Transport.MaxRequestBytes,
		MaxAttempts:          5,
		InitialBackoff:       500 * time.Millisecond,
		MaxBackoff:           5 * time.Second,
		JitterFraction:       0.1,
		IdleInterval:         1 * time.Second,
	}
}

const sensorVersion = "0.1.0-dev"

func main() {
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelInfo)
	serviceInstanceID := supportability.NewServiceInstanceID()
	logger := supportability.NewJSONLogger(os.Stdout, levelVar, sensorVersion, serviceInstanceID)
	slog.SetDefault(logger)

	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("Invalid Sensor configuration",
			"error_code", "SENSOR_CONFIG_INVALID",
			"safe_context", map[string]any{"reason_class": "config_load_failed"})
		os.Exit(1)
	}
	if err := config.ApplyLogLevel(levelVar, cfg.Runtime.LogLevel); err != nil {
		logger.Error("Invalid Sensor log level",
			"error_code", "SENSOR_CONFIG_INVALID",
			"safe_context", map[string]any{"reason_class": "log_level_invalid"})
		os.Exit(1)
	}

	rt := cfg.Runtime
	logger = logger.With("sensor_id", rt.SensorID)
	slog.SetDefault(logger)
	logger.Info("Platform Sensor starting up",
		"safe_context", map[string]any{
			"environment":               cfg.Environment,
			"sensor_role":               string(rt.Role),
			"log_level":                 rt.LogLevel,
			"storage_watermark_percent": rt.StorageWatermarkPercent,
			"config_version":            rt.ConfigurationVersion,
		})

	diskSpool, err := spool.Open(spool.Config{
		Dir:            rt.SpoolPath,
		MaxRecordBytes: 1024 * 1024,
		Logger:         logger,
	})
	if err != nil {
		logger.Error("Failed to initialize disk spool",
			"error_code", "SENSOR_SPOOL_INIT_FAILED",
			"safe_context", map[string]any{"reason_class": "initialization_failed"})
		os.Exit(1)
	}

	controlStorePath, err := prepareControlStorePath(rt.SpoolPath)
	if err != nil {
		logger.Error("Failed to prepare control store path",
			"error_code", "SENSOR_CONTROL_STORE_INIT_FAILED",
			"safe_context", map[string]any{"reason_class": "path_preparation_failed"})
		os.Exit(1)
	}
	controlStore, err := control.NewStore(controlStorePath)
	if err != nil {
		logger.Error("Failed to initialize control store",
			"error_code", "SENSOR_CONTROL_STORE_INIT_FAILED",
			"safe_context", map[string]any{"reason_class": "initialization_failed"})
		os.Exit(1)
	}
	controlMetrics := &control.Metrics{}
	controlStatus := control.NewStatusTracker(controlMetrics)
	if _, loadErr := controlStore.Load(); loadErr != nil {
		logger.Error("Control applied-state snapshot is untrusted",
			"error_code", "SENSOR_CONTROL_STATE_CORRUPT",
			"safe_context", map[string]any{"reason_class": "state_corrupt"})
		controlStatus.MarkUntrusted(control.CodeStateCorrupt)
	}

	runtimeState := newRuntimeConfigState(rt, levelVar)

	sink, err := observation.NewDiskSink(diskSpool, 1024*1024)
	if err != nil {
		logger.Error("Failed to initialize observation sink",
			"error_code", "SENSOR_OBSERVATION_SINK_INIT_FAILED",
			"safe_context", map[string]any{"reason_class": "initialization_failed"})
		os.Exit(1)
	}

	client, err := coreclient.New(coreClientConfigFromRuntime(rt, logger))
	if err != nil {
		logger.Error("Failed to initialize Core client",
			"error_code", "SENSOR_CORECLIENT_INIT_FAILED",
			"safe_context", map[string]any{"reason_class": "initialization_failed"})
		os.Exit(1)
	}

	up, err := uploader.New(uploaderConfigFromRuntime(rt, diskSpool.StreamID()), diskSpool, client)
	if err != nil {
		logger.Error("Failed to initialize uploader",
			"error_code", "SENSOR_UPLOADER_INIT_FAILED",
			"safe_context", map[string]any{"reason_class": "initialization_failed"})
		os.Exit(1)
	}

	// Activation of newSNMPBootstrap is blocked until every default enabled
	// capability (including ICMP) has a real runtime integration. Keep the
	// existing lifecycle here; do not filter capabilities to disguise that gap.
	prodScheduler, err := newProductionScheduler(rt, sensorruntime.NewExecutorRegistry(), logger)
	if err != nil {
		logger.Error("Failed to initialize production scheduler",
			"error_code", "SENSOR_SCHEDULER_INIT_FAILED",
			"safe_context", map[string]any{"reason_class": "initialization_failed"})
		os.Exit(1)
	}

	controlReconciler, err := control.NewReconciler(client, controlStore, rt, runtimeState, logger, controlMetrics)
	if err != nil {
		logger.Error("Failed to initialize control reconciler",
			"error_code", "SENSOR_CONTROL_INIT_FAILED",
			"safe_context", map[string]any{"reason_class": "initialization_failed"})
		os.Exit(1)
	}

	healthServer := health.NewHealthServer(rt.HealthAddress, diskSpool, logger,
		health.WithIdentity(sensorVersion, serviceInstanceID))
	healthServer.SetScheduler(prodScheduler)
	healthServer.SetControl(controlStatus)
	go func() {
		if err := healthServer.Start(); err != nil {
			logger.Error("Health server encountered error",
				"error_code", "SENSOR_HEALTH_SERVER_FAILED",
				"safe_context", map[string]any{"reason_class": "runtime_failed"})
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	if err := up.Start(ctx); err != nil {
		logger.Error("Failed to start uploader",
			"error_code", "SENSOR_UPLOADER_START_FAILED",
			"safe_context", map[string]any{"reason_class": "startup_failed"})
		os.Exit(1)
	}

	engine := collector.NewCollectorEngine(rt.SensorID, string(rt.Role), sink, cfg.PollInterval, logger)
	engine.Start(ctx)

	discoveryExecutor := discovery.NewExecutor(rt.SensorID, discovery.DefaultMaxConcurrent, logger)
	discoveryService := discovery.NewService(rt.SensorID, client, discoveryExecutor, cfg.PollInterval, logger)
	discoveryService.Start(ctx)

	var packSyncer *packsync.Syncer
	packDir, packDirErr := packsync.ResolveCapabilityPackDir(cfg.CapabilityPackDir, rt.SpoolPath)
	if packDirErr != nil {
		logger.Error("Failed to resolve Capability Pack staging directory",
			"error_code", "SENSOR_PACK_INVALID_CONFIG",
			"safe_context", map[string]any{"reason_class": "config_path_invalid"})
		os.Exit(1)
	}
	if filepath.Clean(packDir) == filepath.Clean(rt.SpoolPath) ||
		(cfg.AgentSpoolDir != "" && filepath.Clean(packDir) == filepath.Clean(cfg.AgentSpoolDir)) {
		logger.Error("Capability Pack directory must be distinct from telemetry/agent spool",
			"error_code", "SENSOR_PACK_INVALID_CONFIG",
			"safe_context", map[string]any{"reason_class": "config_path_conflict"})
		os.Exit(1)
	}
	packSyncer, err = packsync.New(packsync.Config{
		Dir:          packDir,
		PollInterval: cfg.PollInterval,
		Logger:       logger,
	}, client)
	if err != nil {
		logger.Error("Failed to initialize Capability Pack sync",
			"error_code", "SENSOR_PACK_INVALID_CONFIG",
			"safe_context", map[string]any{"reason_class": "initialization_failed"})
		os.Exit(1)
	}
	healthServer.SetPackSync(packSyncer)
	packSyncer.Start(ctx)
	logger.Info("Capability Pack sync started", "capability_pack_dir", packDir)

	var (
		agentServer *agentreceiver.Server
		agentSpool  *spool.DiskSpool
		agentDrain  *agentdrain.Drain
	)
	if cfg.AgentReceiverEnabled {
		agentServer, agentSpool, err = startAgentReceiver(cfg, client, logger)
		if err != nil {
			logger.Error("Failed to start Agent Receiver",
				"error_code", "SENSOR_AGENT_RECEIVER_INIT_FAILED",
				"safe_context", map[string]any{"reason_class": "startup_failed"})
			os.Exit(1)
		}
		healthServer.SetAgentSpool(agentSpool)

		agentDrain, err = agentdrain.New(agentdrain.Config{
			SensorID:             rt.SensorID,
			StreamID:             agentSpool.StreamID(),
			AgentSpoolDir:        cfg.AgentSpoolDir,
			MaxBatchRecords:      cfg.AgentDrainMaxBatchRecords,
			MaxBatchPayloadBytes: cfg.AgentDrainMaxBatchPayloadBytes,
			MaxRequestBytes:      cfg.AgentDrainMaxRequestBytes,
			RequestTimeout:       cfg.AgentDrainRequestTimeout,
			InitialBackoff:       cfg.AgentDrainInitialBackoff,
			MaxBackoff:           cfg.AgentDrainMaxBackoff,
			JitterFraction:       0.1,
			IdleInterval:         cfg.AgentDrainIdleInterval,
			ShutdownGrace:        cfg.AgentDrainShutdownGrace,
			Logger:               logger,
		}, agentSpool, client)
		if err != nil {
			logger.Error("Failed to initialize Agent drain",
				"error_code", "SENSOR_AGENT_DRAIN_INIT_FAILED",
				"safe_context", map[string]any{"reason_class": "initialization_failed"})
			os.Exit(1)
		}
		if err := agentDrain.Start(ctx); err != nil {
			logger.Error("Failed to start Agent drain",
				"error_code", "SENSOR_AGENT_DRAIN_START_FAILED",
				"safe_context", map[string]any{"reason_class": "startup_failed"})
			os.Exit(1)
		}

		go func() {
			if err := agentServer.ListenAndServe(); err != nil {
				logger.Error("Agent Receiver listener stopped",
					"error_code", "SENSOR_AGENT_RECEIVER_LISTEN_FAILED",
					"safe_context", map[string]any{"reason_class": "listener_failed"})
			}
		}()
	}

	controlLoopDone := make(chan struct{})
	go runControlReconcileLoop(ctx, controlReconciler, controlStatus, cfg.ControlReconcileInterval, logger, controlLoopDone)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	healthServer.MarkShuttingDown()
	logger.Info("Received termination signal, starting graceful shutdown",
		"safe_context", map[string]any{"signal": sig.String()})

	cancel()
	select {
	case <-controlLoopDone:
	case <-time.After(controlShutdownWait(rt.Transport.RequestTimeout)):
		logger.Warn("Control reconcile loop shutdown timed out",
			"error_code", "SENSOR_CONTROL_SHUTDOWN_TIMEOUT",
			"safe_context", map[string]any{"reason_class": "shutdown_timeout"})
	}
	if agentServer != nil {
		shutdownAgentCtx, shutdownAgentCancel := context.WithTimeout(context.Background(), cfg.AgentReceiverShutdownGrace)
		defer shutdownAgentCancel()
		if err := agentServer.Shutdown(shutdownAgentCtx); err != nil {
			logger.Warn("Agent Receiver shutdown error",
				"error_code", "SENSOR_AGENT_RECEIVER_SHUTDOWN_FAILED",
				"safe_context", map[string]any{"reason_class": "shutdown_failed"})
		}
	}
	if agentDrain != nil {
		drainCtx, drainCancel := context.WithTimeout(context.Background(), cfg.AgentDrainShutdownGrace)
		defer drainCancel()
		if err := agentDrain.Shutdown(drainCtx); err != nil {
			logger.Warn("Agent drain shutdown error",
				"error_code", "SENSOR_AGENT_DRAIN_SHUTDOWN_FAILED",
				"safe_context", map[string]any{"reason_class": "shutdown_failed"})
		}
	}
	if agentSpool != nil {
		if err := agentSpool.Close(); err != nil {
			logger.Warn("Agent spool close error",
				"error_code", "SENSOR_AGENT_SPOOL_CLOSE_FAILED",
				"safe_context", map[string]any{"reason_class": "close_failed"})
		}
	}
	discoveryService.Stop()
	engine.Stop()
	if packSyncer != nil {
		packCtx, packCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer packCancel()
		if err := packSyncer.Shutdown(packCtx); err != nil {
			logger.Warn("Capability Pack sync shutdown error",
				"error_code", "SENSOR_PACK_SHUTDOWN_FAILED",
				"safe_context", map[string]any{"reason_class": "shutdown_failed"})
		}
	}

	if err := prodScheduler.Shutdown(context.Background()); err != nil {
		logger.Warn("Production scheduler shutdown error",
			"error_code", "SENSOR_SCHEDULER_SHUTDOWN_FAILED",
			"safe_context", map[string]any{"reason_class": "shutdown_failed"})
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := up.Shutdown(shutdownCtx); err != nil {
		logger.Warn("Uploader shutdown error",
			"error_code", "SENSOR_UPLOADER_SHUTDOWN_FAILED",
			"safe_context", map[string]any{"reason_class": "shutdown_failed"})
	}
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("Health server shutdown error",
			"error_code", "SENSOR_HEALTH_SHUTDOWN_FAILED",
			"safe_context", map[string]any{"reason_class": "shutdown_failed"})
	}
	logger.Info("Platform Sensor shutdown complete")
}

func prepareControlStorePath(spoolPath string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(spoolPath))
	if clean == "" || !filepath.IsAbs(clean) {
		return "", fmt.Errorf("spool path must be absolute")
	}
	dir := filepath.Join(clean, "control")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	return filepath.Join(dir, "applied-state.json"), nil
}

const maxControlShutdownWait = 15 * time.Second

// controlShutdownWait bounds how long process termination waits for the control loop.
// It does not change HTTP request-timeout authority.
func controlShutdownWait(requestTimeout time.Duration) time.Duration {
	wait := requestTimeout + 2*time.Second
	if wait > maxControlShutdownWait {
		return maxControlShutdownWait
	}
	if wait < 0 {
		return maxControlShutdownWait
	}
	return wait
}

func controlReconcileEventCode(res control.Result, err error) string {
	if code := strings.TrimSpace(res.Code); code != "" {
		return code
	}
	if code := strings.TrimSpace(control.CodeOf(err)); code != "" {
		return code
	}
	return "SENSOR_CONTROL_RECONCILE_FAILED"
}

func controlReconcileLogContext(res control.Result, err error) map[string]any {
	ctx := map[string]any{
		"action":       res.Action,
		"reason_class": "reconcile_failed",
	}
	if code := strings.TrimSpace(control.CodeOf(err)); code != "" && code != controlReconcileEventCode(res, err) {
		ctx["ack_result_code"] = code
	}
	return ctx
}

func runControlReconcileLoop(
	ctx context.Context,
	rec *control.Reconciler,
	status *control.StatusTracker,
	interval time.Duration,
	logger *slog.Logger,
	done chan struct{},
) {
	defer close(done)
	if interval <= 0 {
		interval = 30 * time.Second
	}
	runOnce := func() {
		if ctx.Err() != nil {
			return
		}
		res, err := rec.RunOnce(ctx)
		status.Observe(res, err)
		if err != nil {
			eventCode := controlReconcileEventCode(res, err)
			logger.Warn("control reconcile iteration finished with error",
				"event_code", eventCode,
				"safe_context", controlReconcileLogContext(res, err))
		}
	}
	runOnce()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce()
		}
	}
}

func startAgentReceiver(cfg *config.Config, client *coreclient.Client, logger *slog.Logger) (*agentreceiver.Server, *spool.DiskSpool, error) {
	if err := config.ValidateAgentReceiverConfig(cfg); err != nil {
		return nil, nil, err
	}

	role := cfg.Runtime.Role
	if !agentreceiver.IsRoleEligible(role) {
		return nil, nil, fmt.Errorf("%s: sensor role %q is not eligible for Agent ingress",
			agentreceiver.ErrCodeRoleIneligible, role)
	}

	agentDir := filepath.Clean(cfg.AgentSpoolDir)
	agentSpool, err := spool.Open(spool.Config{
		Dir:            agentDir,
		MaxRecordBytes: 2 * 1024 * 1024,
		Kind:           spool.KindAgentIngress,
		Logger:         logger,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open Agent spool: %w", err)
	}

	ident := agentreceiver.NewFingerprintIdentityVerifier()
	authorizer, err := agentreceiver.NewCoreBackedAuthorizer(client, cfg.Runtime.SensorID)
	if err != nil {
		_ = agentSpool.Close()
		return nil, nil, err
	}
	acceptance, err := agentreceiver.NewSpoolAcceptanceSink(agentSpool, agentDir)
	if err != nil {
		_ = agentSpool.Close()
		return nil, nil, err
	}

	if _, ok := any(ident).(*agentreceiver.FailClosedIdentityVerifier); ok {
		_ = agentSpool.Close()
		return nil, nil, fmt.Errorf("ERR_INVALID_CONFIG: Agent Receiver enabled but FailClosed identity verifier is still installed")
	}
	if _, ok := any(authorizer).(*agentreceiver.FailClosedAuthorizer); ok {
		_ = agentSpool.Close()
		return nil, nil, fmt.Errorf("ERR_INVALID_CONFIG: Agent Receiver enabled but FailClosed authorizer is still installed")
	}
	if _, ok := any(acceptance).(*agentreceiver.FailClosedAcceptanceSink); ok {
		_ = agentSpool.Close()
		return nil, nil, fmt.Errorf("ERR_INVALID_CONFIG: Agent Receiver enabled but FailClosed acceptance sink is still installed")
	}

	rcv, err := agentreceiver.NewReceiver(agentreceiver.Config{
		SensorID:   cfg.Runtime.SensorID,
		SensorRole: role,
	}, ident, authorizer, acceptance, logger)
	if err != nil {
		_ = agentSpool.Close()
		return nil, nil, err
	}

	srv, err := agentreceiver.NewServer(agentreceiver.ServerConfig{
		ListenAddr:     cfg.AgentReceiverListenAddr,
		ServerCertFile: cfg.AgentServerCertFile,
		ServerKeyFile:  cfg.AgentServerKeyFile,
		ClientCAFile:   cfg.AgentClientCAFile,
		MaxInFlight:    cfg.AgentReceiverMaxInFlight,
		ShutdownGrace:  cfg.AgentReceiverShutdownGrace,
		Handler:        rcv,
		Logger:         logger,
	})
	if err != nil {
		_ = agentSpool.Close()
		return nil, nil, err
	}
	if err := srv.Bind(); err != nil {
		_ = agentSpool.Close()
		return nil, nil, err
	}
	return srv, agentSpool, nil
}
