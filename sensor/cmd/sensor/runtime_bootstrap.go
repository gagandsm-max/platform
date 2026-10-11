package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

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
}

// newSNMPBootstrap is source-complete but intentionally not activated in main:
// the default SNMP+ICMP configuration fails Runtime.Start without real ICMP
// integration. All requested capabilities are retained for fail-closed validation.
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
