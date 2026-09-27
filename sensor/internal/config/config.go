package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is the Sensor process bootstrap container.
// RuntimeConfig is the sole authority for BUILD-004-controlled shared runtime fields.
// Remaining fields are local-only process settings not represented by RuntimeConfig.
type Config struct {
	Runtime RuntimeConfig

	Environment  string
	PollInterval time.Duration

	// Agent Receiver local listener/server-TLS/spool settings (SENSOR_AGENT_*).
	AgentReceiverEnabled       bool
	AgentReceiverListenAddr    string
	AgentServerCertFile        string
	AgentServerKeyFile         string
	AgentClientCAFile          string
	AgentSpoolDir              string
	AgentReceiverMaxInFlight   int
	AgentReceiverShutdownGrace time.Duration

	// Agent drain local retry/idle/shutdown settings not represented in RuntimeConfig.
	AgentDrainMaxBatchRecords      int
	AgentDrainMaxBatchPayloadBytes int
	AgentDrainMaxRequestBytes      int
	AgentDrainRequestTimeout       time.Duration
	AgentDrainInitialBackoff       time.Duration
	AgentDrainMaxBackoff           time.Duration
	AgentDrainIdleInterval         time.Duration
	AgentDrainShutdownGrace        time.Duration

	// Capability Pack durable staging directory (local process setting).
	CapabilityPackDir string
}

// legacySharedRuntimeEnvKeys are former shared-runtime authorities that must not
// compete with PLATFORM_SENSOR_* RuntimeConfig. Presence fails closed.
var legacySharedRuntimeEnvKeys = []string{
	"SENSOR_ID",
	"SENSOR_TYPE",
	"CORE_URL",
	"SENSOR_SPOOL_DIR",
	"SENSOR_HEALTH_PORT",
	"PLATFORM_LOG_LEVEL",
	"PLATFORM_TLS_CERT_FILE",
	"SENSOR_TLS_CERT_FILE",
	"PLATFORM_TLS_KEY_FILE",
	"SENSOR_TLS_KEY_FILE",
	"PLATFORM_CA_CERT_FILE",
	"SENSOR_CA_CERT_FILE",
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return defaultVal
	}
	switch strings.ToLower(val) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultVal
	}
}

func getEnvDurationSeconds(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil && i >= 0 {
			return time.Duration(i) * time.Second
		}
	}
	return defaultVal
}

func rejectLegacySharedRuntimeEnv() error {
	var present []string
	for _, key := range legacySharedRuntimeEnvKeys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			present = append(present, key)
		}
	}
	if len(present) == 0 {
		return nil
	}
	return fmt.Errorf("SENSOR_CONFIG_LEGACY_RUNTIME_ENV: legacy shared runtime keys are rejected (%s); use PLATFORM_SENSOR_*", strings.Join(present, ", "))
}

// LoadConfig loads strict RuntimeConfig plus local-only process settings.
// It fails closed before any runtime resources should be created.
func LoadConfig() (*Config, error) {
	if err := rejectLegacySharedRuntimeEnv(); err != nil {
		return nil, err
	}
	rt, err := LoadFromEnv()
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Runtime:     rt,
		Environment: getEnv("PLATFORM_ENV", "development"),
		PollInterval: time.Duration(getEnvInt("SENSOR_POLL_INTERVAL_SECONDS", 10)) * time.Second,

		AgentReceiverEnabled:       getEnvBool("SENSOR_AGENT_RECEIVER_ENABLED", false),
		AgentReceiverListenAddr:    getEnv("SENSOR_AGENT_RECEIVER_LISTEN_ADDR", ""),
		AgentServerCertFile:        getEnv("SENSOR_AGENT_SERVER_CERT_FILE", ""),
		AgentServerKeyFile:         getEnv("SENSOR_AGENT_SERVER_KEY_FILE", ""),
		AgentClientCAFile:          getEnv("SENSOR_AGENT_CLIENT_CA_FILE", ""),
		AgentSpoolDir:              getEnv("SENSOR_AGENT_SPOOL_DIR", ""),
		AgentReceiverMaxInFlight:   getEnvInt("SENSOR_AGENT_RECEIVER_MAX_IN_FLIGHT", 64),
		AgentReceiverShutdownGrace: getEnvDurationSeconds("SENSOR_AGENT_RECEIVER_SHUTDOWN_GRACE_SECONDS", 10*time.Second),

		AgentDrainMaxBatchRecords:      getEnvInt("SENSOR_AGENT_DRAIN_MAX_BATCH_RECORDS", 32),
		AgentDrainMaxBatchPayloadBytes: getEnvInt("SENSOR_AGENT_DRAIN_MAX_BATCH_PAYLOAD_BYTES", 1024*1024),
		AgentDrainMaxRequestBytes:      getEnvInt("SENSOR_AGENT_DRAIN_MAX_REQUEST_BYTES", 2*1024*1024),
		AgentDrainRequestTimeout:       getEnvDurationSeconds("SENSOR_AGENT_DRAIN_REQUEST_TIMEOUT_SECONDS", 15*time.Second),
		AgentDrainInitialBackoff:       getEnvDurationSeconds("SENSOR_AGENT_DRAIN_INITIAL_BACKOFF_SECONDS", 1*time.Second),
		AgentDrainMaxBackoff:           getEnvDurationSeconds("SENSOR_AGENT_DRAIN_MAX_BACKOFF_SECONDS", 30*time.Second),
		AgentDrainIdleInterval:         getEnvDurationSeconds("SENSOR_AGENT_DRAIN_IDLE_INTERVAL_SECONDS", 1*time.Second),
		AgentDrainShutdownGrace:        getEnvDurationSeconds("SENSOR_AGENT_DRAIN_SHUTDOWN_GRACE_SECONDS", 10*time.Second),

		CapabilityPackDir: getEnv("SENSOR_CAPABILITY_PACK_DIR", ""),
	}
	if cfg.PollInterval <= 0 {
		return nil, fmt.Errorf("SENSOR_CONFIG_INVALID: SENSOR_POLL_INTERVAL_SECONDS must be greater than zero")
	}
	return cfg, nil
}

// ParseLogLevel maps RuntimeConfig log level strings to slog levels.
func ParseLogLevel(level string) (slog.Level, error) {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("SENSOR_CONFIG_LOG_LEVEL_INVALID: supported values are DEBUG, INFO, WARN, ERROR")
	}
}

// ApplyLogLevel sets levelVar from a validated RuntimeConfig log level string.
func ApplyLogLevel(levelVar *slog.LevelVar, level string) error {
	if levelVar == nil {
		return fmt.Errorf("SENSOR_CONFIG_INVALID: log level variable is required")
	}
	parsed, err := ParseLogLevel(level)
	if err != nil {
		return err
	}
	levelVar.Set(parsed)
	return nil
}

// ValidateAgentReceiverConfig returns a non-nil error when Agent Receiver is
// enabled but required fields are missing/invalid. When disabled, returns nil.
func ValidateAgentReceiverConfig(cfg *Config) error {
	if cfg == nil || !cfg.AgentReceiverEnabled {
		return nil
	}
	if strings.TrimSpace(cfg.AgentReceiverListenAddr) == "" {
		return errAgentCfg("SENSOR_AGENT_RECEIVER_LISTEN_ADDR is required when Agent Receiver is enabled")
	}
	if strings.TrimSpace(cfg.AgentServerCertFile) == "" || strings.TrimSpace(cfg.AgentServerKeyFile) == "" {
		return errAgentCfg("SENSOR_AGENT_SERVER_CERT_FILE and SENSOR_AGENT_SERVER_KEY_FILE are required when Agent Receiver is enabled")
	}
	if !filepath.IsAbs(strings.TrimSpace(cfg.AgentServerCertFile)) || !filepath.IsAbs(strings.TrimSpace(cfg.AgentServerKeyFile)) {
		return errAgentCfg("SENSOR_AGENT_SERVER_CERT_FILE and SENSOR_AGENT_SERVER_KEY_FILE must be absolute paths")
	}
	if strings.TrimSpace(cfg.AgentClientCAFile) == "" {
		return errAgentCfg("SENSOR_AGENT_CLIENT_CA_FILE is required when Agent Receiver is enabled")
	}
	if !filepath.IsAbs(strings.TrimSpace(cfg.AgentClientCAFile)) {
		return errAgentCfg("SENSOR_AGENT_CLIENT_CA_FILE must be an absolute path")
	}
	agentSpool := strings.TrimSpace(cfg.AgentSpoolDir)
	if agentSpool == "" || !filepath.IsAbs(agentSpool) {
		return errAgentCfg("SENSOR_AGENT_SPOOL_DIR must be an absolute path when Agent Receiver is enabled")
	}
	obsSpool := filepath.Clean(strings.TrimSpace(cfg.Runtime.SpoolPath))
	if filepath.Clean(agentSpool) == obsSpool {
		return errAgentCfg("SENSOR_AGENT_SPOOL_DIR must be distinct from PLATFORM_SENSOR_SPOOL_PATH")
	}
	// Keep Agent ingress server identity distinct from Sensor→Core client identity.
	coreCert := filepath.Clean(strings.TrimSpace(cfg.Runtime.TLS.Certificate))
	coreKey := filepath.Clean(strings.TrimSpace(cfg.Runtime.TLS.PrivateKey))
	agentCert := filepath.Clean(strings.TrimSpace(cfg.AgentServerCertFile))
	agentKey := filepath.Clean(strings.TrimSpace(cfg.AgentServerKeyFile))
	if coreCert != "" && agentCert == coreCert {
		return errAgentCfg("SENSOR_AGENT_SERVER_CERT_FILE must be distinct from PLATFORM_SENSOR_TLS_CERT_FILE")
	}
	if coreKey != "" && agentKey == coreKey {
		return errAgentCfg("SENSOR_AGENT_SERVER_KEY_FILE must be distinct from PLATFORM_SENSOR_TLS_KEY_FILE")
	}
	if cfg.AgentReceiverMaxInFlight <= 0 {
		return errAgentCfg("SENSOR_AGENT_RECEIVER_MAX_IN_FLIGHT must be greater than zero")
	}
	if cfg.AgentReceiverShutdownGrace <= 0 {
		return errAgentCfg("SENSOR_AGENT_RECEIVER_SHUTDOWN_GRACE_SECONDS must be greater than zero")
	}
	return nil
}

func errAgentCfg(msg string) error {
	return &agentConfigError{msg: msg}
}

type agentConfigError struct{ msg string }

func (e *agentConfigError) Error() string {
	return "ERR_INVALID_CONFIG: " + e.msg
}
