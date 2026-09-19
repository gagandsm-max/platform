package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	SensorID                string
	SensorType              string
	Environment             string
	CoreURL                 string
	SpoolDir                string
	StorageWatermarkPercent int
	HealthPort              int
	PollInterval            time.Duration
	LogLevel                string
	TLSCertFile             string
	TLSKeyFile              string
	CACertFile              string

	// Agent Receiver (legacy LoadConfig / main wiring). Disabled by default.
	// Env prefix SENSOR_AGENT_* is consistent with other LoadConfig SENSOR_* keys.
	AgentReceiverEnabled       bool
	AgentReceiverListenAddr    string
	AgentServerCertFile        string
	AgentServerKeyFile         string
	AgentClientCAFile          string
	AgentSpoolDir              string
	AgentReceiverMaxInFlight   int
	AgentReceiverShutdownGrace time.Duration

	// Agent drain (Sensor agent-ingress → Core agent-telemetry).
	AgentDrainMaxBatchRecords      int
	AgentDrainMaxBatchPayloadBytes int
	AgentDrainMaxRequestBytes      int
	AgentDrainRequestTimeout       time.Duration
	AgentDrainInitialBackoff       time.Duration
	AgentDrainMaxBackoff           time.Duration
	AgentDrainIdleInterval         time.Duration
	AgentDrainShutdownGrace        time.Duration

	// Capability Pack durable staging (EMC1.3). Absolute path; distinct from telemetry/agent spool.
	CapabilityPackDir string
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

func LoadConfig() *Config {
	return &Config{
		SensorID:                getEnv("SENSOR_ID", "sensor-dev-01"),
		SensorType:              getEnv("SENSOR_TYPE", "generic"),
		Environment:             getEnv("PLATFORM_ENV", "development"),
		CoreURL:                 getEnv("CORE_URL", "https://core:8443"),
		SpoolDir:                getEnv("SENSOR_SPOOL_DIR", "/var/lib/platform/sensor/spool"),
		StorageWatermarkPercent: getEnvInt("PLATFORM_STORAGE_WATERMARK_PERCENT", 80),
		HealthPort:              getEnvInt("SENSOR_HEALTH_PORT", 8088),
		PollInterval:            time.Duration(getEnvInt("SENSOR_POLL_INTERVAL_SECONDS", 10)) * time.Second,
		LogLevel:                getEnv("PLATFORM_LOG_LEVEL", "INFO"),
		TLSCertFile:             getEnv("PLATFORM_TLS_CERT_FILE", getEnv("SENSOR_TLS_CERT_FILE", "")),
		TLSKeyFile:              getEnv("PLATFORM_TLS_KEY_FILE", getEnv("SENSOR_TLS_KEY_FILE", "")),
		CACertFile:              getEnv("PLATFORM_CA_CERT_FILE", getEnv("SENSOR_CA_CERT_FILE", "")),

		// Agent Receiver — SENSOR_AGENT_* (documented; default disabled).
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
	obsSpool := filepath.Clean(strings.TrimSpace(cfg.SpoolDir))
	if filepath.Clean(agentSpool) == obsSpool {
		return errAgentCfg("SENSOR_AGENT_SPOOL_DIR must be distinct from SENSOR_SPOOL_DIR")
	}
	// Keep Agent ingress server identity distinct from Sensor→Core client identity.
	// Agent Client CA may share the Core CA file when the same root signs both.
	coreCert := filepath.Clean(strings.TrimSpace(cfg.TLSCertFile))
	coreKey := filepath.Clean(strings.TrimSpace(cfg.TLSKeyFile))
	agentCert := filepath.Clean(strings.TrimSpace(cfg.AgentServerCertFile))
	agentKey := filepath.Clean(strings.TrimSpace(cfg.AgentServerKeyFile))
	if coreCert != "" && agentCert == coreCert {
		return errAgentCfg("SENSOR_AGENT_SERVER_CERT_FILE must be distinct from SENSOR_TLS_CERT_FILE")
	}
	if coreKey != "" && agentKey == coreKey {
		return errAgentCfg("SENSOR_AGENT_SERVER_KEY_FILE must be distinct from SENSOR_TLS_KEY_FILE")
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
