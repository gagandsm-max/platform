package main

import (
	"fmt"
	"log/slog"
	"strings"

	"platform/agent/common"
	"platform/agent/common/config"
	"platform/agent/common/identity"
)

// agentRuntime is the initialized AgentService ownership for one process run.
// Scheduler, delivery worker, queue and transport remain owned solely by
// AgentService — this type only holds the constructed instance and logger.
type agentRuntime struct {
	cfg    *config.Config
	ident  *identity.AgentIdentity
	logger *slog.Logger
	svc    *common.AgentService
}

// processExitError carries a stable process exit code for console/debug paths.
// SCM paths must not os.Exit from the handler; they publish STOPPED with the
// matching service-specific code instead.
type processExitError struct {
	code int
	err  error
}

func (e *processExitError) Error() string {
	if e == nil {
		return ""
	}
	if e.err != nil {
		return e.err.Error()
	}
	return fmt.Sprintf("process exit %d", e.code)
}

func (e *processExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// agentBootstrapDeps is the injectable initialization seam shared by console
// and SCM paths. Production uses defaultBootstrapDeps; tests inject failures.
type agentBootstrapDeps struct {
	loadConfig   func() (*config.Config, error)
	loadIdentity func(stateDir, agentID, tenantID string) (*identity.AgentIdentity, error)
	newService   func(cfg *config.Config, ident *identity.AgentIdentity, logger *slog.Logger) (*common.AgentService, error)

	levelVar          *slog.LevelVar
	bootstrapLogger   *slog.Logger
	serviceInstanceID string
}

func defaultBootstrapDeps(levelVar *slog.LevelVar, bootstrap *slog.Logger, serviceInstanceID string) agentBootstrapDeps {
	return agentBootstrapDeps{
		loadConfig:   config.LoadConfig,
		loadIdentity: identity.LoadOrGenerate,
		newService: func(cfg *config.Config, ident *identity.AgentIdentity, logger *slog.Logger) (*common.AgentService, error) {
			return common.NewAgentService(
				cfg,
				ident,
				nil,
				logger,
				common.WithSupportabilityIdentity(agentVersion, serviceInstanceID),
			)
		},
		levelVar:          levelVar,
		bootstrapLogger:   bootstrap,
		serviceInstanceID: serviceInstanceID,
	}
}

// initialize loads configuration, identity and constructs AgentService once.
// Failure codes match SCM service-specific exits: 1=config, 2=identity, 3=service.
func (d agentBootstrapDeps) initialize() (*agentRuntime, uint32, error) {
	log := d.bootstrapLogger
	if log == nil {
		log = slog.Default()
	}

	loadConfig := d.loadConfig
	if loadConfig == nil {
		loadConfig = config.LoadConfig
	}
	cfg, err := loadConfig()
	if err != nil {
		log.Error("Failed to load Windows agent configuration",
			"event_code", "AGENT_CONFIG_INVALID",
			"safe_context", map[string]any{"reason_class": "config_load_failed"})
		return nil, exitConfig, err
	}
	if d.levelVar != nil {
		d.levelVar.Set(logLevel(cfg.LogLevel))
	}

	loadIdentity := d.loadIdentity
	if loadIdentity == nil {
		loadIdentity = identity.LoadOrGenerate
	}
	ident, err := loadIdentity(cfg.StateDir, cfg.AgentID, cfg.TenantID)
	if err != nil {
		log.Error("Failed to initialize Windows agent identity",
			"event_code", "AGENT_IDENTITY_INIT_FAILED",
			"safe_context", map[string]any{"reason_class": "identity_init_failed"})
		return nil, exitIdentity, err
	}

	logger := log.With("agent_id", ident.AgentID)
	if strings.TrimSpace(ident.TenantID) != "" {
		logger = logger.With("tenant_id", ident.TenantID)
	}
	slog.SetDefault(logger)

	logger.Info("AmrudAi Windows Agent starting",
		"safe_context", map[string]any{
			"host_id":   ident.HostID,
			"log_level": cfg.LogLevel,
			"state_dir": "configured",
			"os":        ident.OS,
			"arch":      ident.Arch,
		})

	newService := d.newService
	if newService == nil {
		newService = func(cfg *config.Config, ident *identity.AgentIdentity, logger *slog.Logger) (*common.AgentService, error) {
			return common.NewAgentService(cfg, ident, nil, logger)
		}
	}
	svc, err := newService(cfg, ident, logger)
	if err != nil {
		logger.Error("Failed to initialize Windows agent service",
			"event_code", "AGENT_SERVICE_INIT_FAILED",
			"safe_context", map[string]any{"reason_class": "initialization_failed"})
		return nil, exitService, err
	}

	return &agentRuntime{
		cfg:    cfg,
		ident:  ident,
		logger: logger,
		svc:    svc,
	}, 0, nil
}

// runConsoleEntrypoint initializes then runs console/debug mode.
func runConsoleEntrypoint(deps agentBootstrapDeps) error {
	rt, code, err := deps.initialize()
	if err != nil {
		return &processExitError{code: int(code), err: err}
	}
	return runConsole(rt.svc, rt.logger)
}
