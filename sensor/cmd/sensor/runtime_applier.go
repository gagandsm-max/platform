package main

import (
	"context"
	"log/slog"
	"sync"

	"platform/sensor/internal/config"
	"platform/sensor/internal/control"
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
		current:  initial,
		levelVar: levelVar,
	}
}

// Snapshot returns a copy of the currently accepted RuntimeConfig.
func (s *runtimeConfigState) Snapshot() config.RuntimeConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
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
	s.current = candidate
	return nil
}
