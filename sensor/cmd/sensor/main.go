package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"platform/sensor/internal/agentdrain"
	"platform/sensor/internal/agentreceiver"
	"platform/sensor/internal/collector"
	"platform/sensor/internal/config"
	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/discovery"
	"platform/sensor/internal/health"
	"platform/sensor/internal/observation"
	"platform/sensor/internal/spool"
	"platform/sensor/internal/uploader"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})).With(
		"service", "platform-sensor",
	)
	slog.SetDefault(logger)

	cfg := config.LoadConfig()
	logger = logger.With("sensor_id", cfg.SensorID, "sensor_type", cfg.SensorType)
	logger.Info("Platform Sensor starting up",
		"version", "0.1.0-dev", "environment", cfg.Environment, "core_url", cfg.CoreURL,
		"spool_dir", cfg.SpoolDir, "watermark_percent", cfg.StorageWatermarkPercent)

	// Preserve current main telemetry pipeline: checked spool → DiskSink → Core client → uploader.
	diskSpool, err := spool.Open(spool.Config{
		Dir:            cfg.SpoolDir,
		MaxRecordBytes: 1024 * 1024,
		Logger:         logger,
	})
	if err != nil {
		logger.Error("Failed to initialize disk spool", "error_code", "SENSOR_SPOOL_INIT_FAILED")
		os.Exit(1)
	}

	sink, err := observation.NewDiskSink(diskSpool, 1024*1024)
	if err != nil {
		logger.Error("Failed to initialize observation sink", "error", err)
		os.Exit(1)
	}

	client, err := coreclient.New(coreclient.Config{
		BaseURL:          cfg.CoreURL,
		CertificateFile:  cfg.TLSCertFile,
		PrivateKeyFile:   cfg.TLSKeyFile,
		CAFile:           cfg.CACertFile,
		RequestTimeout:   10 * time.Second,
		MaxResponseBytes: 1024 * 1024,
		Logger:           logger,
	})
	if err != nil {
		logger.Error("Failed to initialize Core client", "error_code", "SENSOR_CORECLIENT_INIT_FAILED")
		os.Exit(1)
	}

	up, err := uploader.New(uploader.Config{
		SensorID:             cfg.SensorID,
		BootEpoch:            fmt.Sprintf("epoch-%d", time.Now().Unix()),
		MaxBatchRecords:      100,
		MaxBatchPayloadBytes: 1024 * 1024,
		MaxRequestBytes:      2 * 1024 * 1024,
		MaxAttempts:          5,
		InitialBackoff:       500 * time.Millisecond,
		MaxBackoff:           5 * time.Second,
		JitterFraction:       0.1,
		IdleInterval:         1 * time.Second,
	}, diskSpool, client)
	if err != nil {
		logger.Error("Failed to initialize uploader", "error", err)
		os.Exit(1)
	}

	healthServer := health.NewHealthServer(cfg.HealthPort, diskSpool, logger)
	go func() {
		if err := healthServer.Start(); err != nil {
			logger.Error("Health server encountered error", "error_code", "SENSOR_HEALTH_SERVER_FAILED")
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	if err := up.Start(ctx); err != nil {
		logger.Error("Failed to start uploader", "error", err)
		os.Exit(1)
	}

	engine := collector.NewCollectorEngine(cfg.SensorID, cfg.SensorType, sink, cfg.PollInterval, logger)
	engine.Start(ctx)

	discoveryExecutor := discovery.NewExecutor(cfg.SensorID, discovery.DefaultMaxConcurrent, logger)
	discoveryService := discovery.NewService(cfg.SensorID, client, discoveryExecutor, cfg.PollInterval, logger)
	discoveryService.Start(ctx)

	var (
		agentServer *agentreceiver.Server
		agentSpool  *spool.DiskSpool
		agentDrain  *agentdrain.Drain
	)
	if cfg.AgentReceiverEnabled {
		agentServer, agentSpool, err = startAgentReceiver(cfg, client, logger)
		if err != nil {
			logger.Error("Failed to start Agent Receiver", "error", err, "error_code", "SENSOR_AGENT_RECEIVER_INIT_FAILED")
			os.Exit(1)
		}
		healthServer.SetAgentSpool(agentSpool)

		agentDrain, err = agentdrain.New(agentdrain.Config{
			SensorID:             cfg.SensorID,
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
			logger.Error("Failed to initialize Agent drain", "error", err, "error_code", "SENSOR_AGENT_DRAIN_INIT_FAILED")
			os.Exit(1)
		}
		if err := agentDrain.Start(ctx); err != nil {
			logger.Error("Failed to start Agent drain", "error", err)
			os.Exit(1)
		}

		go func() {
			if err := agentServer.ListenAndServe(); err != nil {
				logger.Error("Agent Receiver listener stopped", "error", err, "error_code", "SENSOR_AGENT_RECEIVER_LISTEN_FAILED")
			}
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("Received termination signal, starting graceful shutdown", "signal", sig.String())

	cancel()
	// Shutdown order: stop Agent ingress → stop Agent drain → close Agent spool → continue Sensor shutdown.
	if agentServer != nil {
		shutdownAgentCtx, shutdownAgentCancel := context.WithTimeout(context.Background(), cfg.AgentReceiverShutdownGrace)
		defer shutdownAgentCancel()
		if err := agentServer.Shutdown(shutdownAgentCtx); err != nil {
			logger.Warn("Agent Receiver shutdown error", "error", err)
		}
	}
	if agentDrain != nil {
		drainCtx, drainCancel := context.WithTimeout(context.Background(), cfg.AgentDrainShutdownGrace)
		defer drainCancel()
		if err := agentDrain.Shutdown(drainCtx); err != nil {
			logger.Warn("Agent drain shutdown error", "error", err)
		}
	}
	if agentSpool != nil {
		if err := agentSpool.Close(); err != nil {
			logger.Warn("Agent spool close error", "error", err)
		}
	}
	discoveryService.Stop()
	engine.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := up.Shutdown(shutdownCtx); err != nil {
		logger.Warn("Uploader shutdown error", "error", err)
	}
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("Health server shutdown error", "error_code", "SENSOR_HEALTH_SHUTDOWN_FAILED")
	}
	logger.Info("Platform Sensor shutdown complete")
}

func startAgentReceiver(cfg *config.Config, client *coreclient.Client, logger *slog.Logger) (*agentreceiver.Server, *spool.DiskSpool, error) {
	if err := config.ValidateAgentReceiverConfig(cfg); err != nil {
		return nil, nil, err
	}

	role := agentreceiver.RoleFromSensorType(cfg.SensorType)
	if !agentreceiver.IsRoleEligible(role) {
		return nil, nil, fmt.Errorf("%s: sensor type %q maps to role %q which is not eligible for Agent ingress",
			agentreceiver.ErrCodeRoleIneligible, cfg.SensorType, role)
	}

	agentDir := filepath.Clean(cfg.AgentSpoolDir)
	agentSpool, err := spool.Open(spool.Config{
		Dir:            agentDir,
		MaxRecordBytes: 2 * 1024 * 1024,
		Logger:         logger.With("spool", "agent-ingress"),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open Agent spool: %w", err)
	}

	ident := agentreceiver.NewFingerprintIdentityVerifier()
	authorizer, err := agentreceiver.NewCoreBackedAuthorizer(client, cfg.SensorID)
	if err != nil {
		_ = agentSpool.Close()
		return nil, nil, err
	}
	acceptance, err := agentreceiver.NewSpoolAcceptanceSink(agentSpool, agentDir)
	if err != nil {
		_ = agentSpool.Close()
		return nil, nil, err
	}

	// Fail closed if production adapters were somehow not installed.
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
		SensorID:   cfg.SensorID,
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
