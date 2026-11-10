package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	icmpintegration "platform/sensor/internal/integration/icmp"
	protocolicmp "platform/sensor/internal/protocol/icmp"
	"time"

	"platform/sensor/internal/config"
	snmpintegration "platform/sensor/internal/integration/snmp"
	"platform/sensor/internal/observation"
	protocolsnmp "platform/sensor/internal/protocol/snmp"
	sensorruntime "platform/sensor/internal/runtime"
	"platform/sensor/internal/scheduler"
)

// snmpBootstrap composes the existing runtime, scheduler and durable observation
// path. It admits no recurring work: BUILD-021 owns that boundary. Runtime is the
// sole shutdown owner of Scheduler once construction succeeds.
type snmpBootstrap struct {
	Runtime   *sensorruntime.Runtime
	Scheduler *scheduler.Scheduler
	registry  *sensorruntime.ExecutorRegistry
	SNMP      *snmpintegration.Component
	ICMP      *icmpintegration.Component
}

// startProductionBootstrap activates the integrated runtime only for NETWORK.
// Other roles retain their existing composition and single scheduler until their
// real runtime components are integrated. A nil Runtime is never a ready claim.
func startProductionBootstrap(ctx context.Context, cfg config.RuntimeConfig, sink observation.Sink, logger *slog.Logger) (*snmpBootstrap, error) {
	if err := sensorruntime.ValidateCapabilities(cfg.Role, cfg.EnabledCapabilities); err != nil {
		return nil, err
	}
	if cfg.Role != sensorruntime.RoleNetwork {
		registry := sensorruntime.NewExecutorRegistry()
		sched, err := newProductionScheduler(cfg, registry, logger)
		if err != nil {
			return nil, err
		}
		logger.Warn("Sensor retains legacy operational composition; runtime integration is NETWORK-only",
			"event_code", "SENSOR_RUNTIME_ROLE_NOT_INTEGRATED",
			"safe_context", map[string]any{"sensor_role": string(cfg.Role), "runtime_activated": false, "reason_class": "interim_role_compatibility"})
		return &snmpBootstrap{Scheduler: sched, registry: registry}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b, err := newSNMPBootstrap(cfg, sink, logger)
	if err != nil {
		return nil, err
	}
	if err := b.Runtime.Start(ctx); err != nil {
		// Cancelled/deadline startup must not activate a runtime that continued
		// on its separate root context. Tear down before returning.
		if cerr := ctx.Err(); cerr != nil {
			return nil, errors.Join(cerr, shutdownBootstrap(b, cfg.Scheduler.ShutdownGracePeriod))
		}
		// Collector-local DEGRADED (for example ICMP ping-socket denial) is a
		// truthful readiness outcome, not a bootstrap abort. Only hard FAILED
		// startup tears the NETWORK runtime down.
		if b.Runtime.Snapshot().State == sensorruntime.StateDegraded {
			logger.Warn("NETWORK Sensor runtime activated degraded",
				"event_code", "SENSOR_RUNTIME_STARTED_DEGRADED",
				"error_code", "SENSOR_RUNTIME_DEGRADED",
				"safe_context", map[string]any{"sensor_role": string(cfg.Role), "runtime_activated": true, "reason_class": "collector_capability"})
			return b, nil
		}
		return nil, errors.Join(err, shutdownBootstrap(b, cfg.Scheduler.ShutdownGracePeriod))
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, shutdownBootstrap(b, cfg.Scheduler.ShutdownGracePeriod))
	}
	logger.Info("NETWORK Sensor runtime activated",
		"event_code", "SENSOR_RUNTIME_STARTED",
		"safe_context", map[string]any{"sensor_role": string(cfg.Role), "runtime_activated": true})
	return b, nil
}

func shutdownBootstrap(b *snmpBootstrap, grace time.Duration) error {
	if b == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	return b.shutdown(shutdownCtx)
}

func (b *snmpBootstrap) shutdown(ctx context.Context) error {
	if b.Runtime != nil {
		return b.Runtime.Shutdown(ctx)
	}
	return b.Scheduler.Shutdown(ctx)
}

// newSNMPBootstrap registers real SNMP and ICMP implementations. All requested
// capabilities are retained for runtime fail-closed validation; no recurring
// work is admitted by bootstrap (BUILD-021 owns admission).
func newSNMPBootstrap(cfg config.RuntimeConfig, sink observation.Sink, logger *slog.Logger) (*snmpBootstrap, error) {
	if sink == nil {
		return nil, fmt.Errorf("SNMP_INTEGRATION_INVALID_CONFIG: observation sink is required")
	}
	registry := sensorruntime.NewExecutorRegistry()
	sched, err := newProductionScheduler(cfg, registry, logger)
	if err != nil {
		return nil, err
	}
	rt, err := sensorruntime.New(sensorruntime.Config{
		Role:                 cfg.Role,
		EnabledCapabilities:  append([]sensorruntime.Capability(nil), cfg.EnabledCapabilities...),
		DesiredConfigVersion: cfg.ConfigurationVersion,
	}, sched, registry)
	if err != nil {
		return nil, errors.Join(err, sched.Shutdown(context.Background()))
	}
	b := &snmpBootstrap{Runtime: rt, Scheduler: sched, registry: registry}
	for _, capability := range cfg.EnabledCapabilities {
		if capability == sensorruntime.CapabilityICMP {
			collector, err := protocolicmp.NewCollector(protocolicmp.Limits{MaxSamples: 8, MaxPayloadBytes: 128, MaxExecutionTimeout: 10 * time.Second, MaxReplyPacketsPerSample: 32, MaxSchedulerPayloadBytes: cfg.Scheduler.MaxPayloadBytes})
			if err != nil {
				return nil, errors.Join(err, rt.Shutdown(context.Background()))
			}
			exec, err := icmpintegration.NewExecutor(protocolicmp.NewExecutor(collector), sink, icmpintegration.Config{SensorID: cfg.SensorID, MaxResultBytes: sink.MaxPayloadBytes(), MaxEnvelopeBytes: sink.MaxPayloadBytes(), MaxSamples: 8})
			if err == nil {
				err = icmpintegration.Register(rt, registry, exec)
			}
			if err != nil {
				return nil, errors.Join(err, rt.Shutdown(context.Background()))
			}
			// One-time local ping-socket open/close at Component.Start — no
			// customer probe traffic and no per-/readyz privilege checks.
			exec.Component().SetLocalCapabilityCheck(collector.VerifyLocalCapability)
			b.ICMP = exec.Component()
			continue
		}
		if capability != sensorruntime.CapabilitySNMP {
			continue
		}
		base, err := protocolsnmp.NewExecutor(protocolsnmp.NewCollector(logger), cfg.Scheduler.MaxPayloadBytes)
		if err != nil {
			return nil, errors.Join(err, rt.Shutdown(context.Background()))
		}
		exec, err := snmpintegration.NewExecutor(base, sink, snmpintegration.Config{
			SensorID:           cfg.SensorID,
			MaxSNMPResultBytes: sink.MaxPayloadBytes(),
			MaxEnvelopeBytes:   sink.MaxPayloadBytes(),
		})
		if err == nil {
			err = snmpintegration.Register(rt, registry, exec)
		}
		if err != nil {
			return nil, errors.Join(err, rt.Shutdown(context.Background()))
		}
		b.SNMP = exec.Component()
	}
	return b, nil
}
