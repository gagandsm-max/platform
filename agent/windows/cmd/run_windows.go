//go:build windows

package main

import (
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/sys/windows/svc"
)

var errSCMRun = errors.New("AGENT_SCM_RUN_FAILED")

// runEntrypoint selects Windows SCM service mode or console/debug mode.
// Service mode enters svc.Run before configuration/identity/AgentService init.
func runEntrypoint(levelVar *slog.LevelVar, bootstrap *slog.Logger, serviceInstanceID string) error {
	deps := defaultBootstrapDeps(levelVar, bootstrap, serviceInstanceID)

	inService, err := svc.IsWindowsService()
	if err != nil {
		bootstrap.Error("Failed to determine Windows service execution mode",
			"event_code", "AGENT_SCM_MODE_DETECT_FAILED",
			"safe_context", map[string]any{"reason_class": "scm_detect_failed"})
		return fmt.Errorf("%w: %w", errSCMRun, err)
	}
	if inService {
		if forceConsoleMode() {
			bootstrap.Warn("Ignoring console override in Windows service mode",
				"event_code", "AGENT_SCM_CONSOLE_OVERRIDE_IGNORED",
				"safe_context", map[string]any{"reason_class": "scm_service_mode_enforced"})
		}
		return runSCMEntrypoint(deps, bootstrap)
	}
	return runConsoleEntrypoint(deps)
}

func runSCMEntrypoint(deps agentBootstrapDeps, bootstrap *slog.Logger) error {
	if bootstrap == nil {
		bootstrap = slog.Default()
	}

	bootstrap.Info("AmrudAi Windows Agent SCM mode",
		"event_code", "AGENT_SCM_MODE",
		"safe_context", map[string]any{
			"service_name": serviceName,
			"reason_class": "scm_lifecycle",
		})

	factory := func() (agentLifecycle, uint32, error) {
		rt, code, err := deps.initialize()
		if err != nil {
			return nil, code, err
		}
		return rt.svc, 0, nil
	}

	adapter := newSCMAdapterWithFactory(factory, bootstrap)
	handler := &windowsSCMHandler{adapter: adapter}
	if err := svc.Run(serviceName, handler); err != nil {
		bootstrap.Error("AmrudAi Windows Agent SCM run failed",
			"event_code", "AGENT_SCM_RUN_FAILED",
			"safe_context", map[string]any{"reason_class": "scm_run_failed"})
		return fmt.Errorf("%w: %w", errSCMRun, err)
	}
	return nil
}
