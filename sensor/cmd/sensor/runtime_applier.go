package main

import (
	"context"
	"log/slog"
	"sync"

	"platform/sensor/internal/config"
	"platform/sensor/internal/control"
	sensorruntime "platform/sensor/internal/runtime"
)

// runtimeConfigState owns the Sensor's currently accepted RuntimeConfig.
// It is the sole production Applier used by control.Reconciler.
type runtimeConfigState struct {
	mu       sync.RWMutex
	current  config.RuntimeConfig
	levelVar *slog.LevelVar
}

func newRuntimeConfigState(initial config.RuntimeConfig, levelVar *slog.LevelVar) *runtimeConfigState {
	return &runtimeConfigState{
		current:  cloneRuntimeConfig(initial),
		levelVar: levelVar,
	}
}

// Snapshot returns a deep copy of the currently accepted RuntimeConfig.
func (s *runtimeConfigState) Snapshot() config.RuntimeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneRuntimeConfig(s.current)
}

// Apply implements control.Applier with atomic Diff-before-mutate semantics.
func (s *runtimeConfigState) Apply(_ context.Context, _ uint64, _ string, candidate config.RuntimeConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	changes := config.Diff(s.current, candidate)
	if len(changes) == 0 {
		return nil
	}

	hasRestart := false
	hasLogLevel := false
	unexpectedReloadable := false
	for _, change := range changes {
		switch change.Class {
		case config.ChangeRestartRequired:
			hasRestart = true
		case config.ChangeReloadable:
			if change.Field == "log_level" {
				hasLogLevel = true
			} else {
				// Desired-state v1 cannot legitimately modify other reloadable fields.
				unexpectedReloadable = true
			}
		}
	}

	if hasRestart {
		return control.NewRestartRequiredError("desired configuration requires process restart")
	}
	if unexpectedReloadable {
		return control.NewApplyFailedError("unsupported reloadable configuration change", nil)
	}
	if !hasLogLevel {
		return nil
	}

	if err := config.ApplyLogLevel(s.levelVar, candidate.LogLevel); err != nil {
		return control.NewApplyFailedError("failed to apply log level", err)
	}
	s.current = cloneRuntimeConfig(candidate)
	return nil
}

func cloneRuntimeConfig(in config.RuntimeConfig) config.RuntimeConfig {
	out := in
	if in.EnabledCapabilities != nil {
		out.EnabledCapabilities = append([]sensorruntime.Capability(nil), in.EnabledCapabilities...)
	}
	out.Scheduler = cloneSchedulerConfig(in.Scheduler)
	return out
}

func cloneSchedulerConfig(in config.SchedulerConfig) config.SchedulerConfig {
	out := in
	if in.Pools != nil {
		out.Pools = make(map[string]config.PoolConfig, len(in.Pools))
		for k, v := range in.Pools {
			out.Pools[k] = v
		}
	}
	return out
}
