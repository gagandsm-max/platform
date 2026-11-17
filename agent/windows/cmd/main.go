package main

import (
	"errors"
	"log/slog"
	"os"
	"strings"

	"platform/agent/common/supportability"
)

const (
	agentVersion = "0.1.0-dev"
	serviceName  = "platform-agent"

	exitConfig   = 1
	exitIdentity = 2
	exitService  = 3
	exitStart    = 4
	exitStop     = 5
	exitSCM      = 6
)

// Platform Windows Agent entrypoint.
//
// Windows service execution enters svc.Run before configuration, identity or
// AgentService initialization. The SCM handler publishes START_PENDING, then
// the shared bootstrap factory constructs AgentService. Initialization failures
// are reported as STOPPED with service-specific codes 1–3 (Start=4, Stop=5).
// Console/debug mode remains a separate path that may still use process exits.
//
// MSI service registration, service-account rights and Event Log source install
// remain BUILD-018 / permission-matrix ownership — not this runtime.
func main() {
	levelVar := new(slog.LevelVar)
	levelVar.Set(slog.LevelInfo)
	serviceInstanceID := supportability.NewServiceInstanceID()
	bootstrap := supportability.NewJSONLogger(os.Stdout, levelVar, agentVersion, serviceInstanceID)
	slog.SetDefault(bootstrap)

	if err := runEntrypoint(levelVar, bootstrap, serviceInstanceID); err != nil {
		os.Exit(classifyRunExit(err))
	}
}

func classifyRunExit(err error) int {
	if err == nil {
		return 0
	}
	var pe *processExitError
	if errors.As(err, &pe) && pe != nil && pe.code != 0 {
		return pe.code
	}
	msg := err.Error()
	switch {
	case errors.Is(err, errConsoleStart), strings.Contains(msg, "AGENT_SERVICE_START"):
		return exitStart
	case errors.Is(err, errConsoleStop), strings.Contains(msg, "AGENT_SHUTDOWN"), strings.Contains(msg, "AGENT_SERVICE_STOP"):
		return exitStop
	case errors.Is(err, errSCMRun):
		return exitSCM
	default:
		return exitSCM
	}
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

func forceConsoleMode() bool {
	if v := strings.TrimSpace(os.Getenv("PLATFORM_AGENT_CONSOLE")); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	for _, arg := range os.Args[1:] {
		switch arg {
		case "-console", "--console":
			return true
		}
	}
	return false
}
