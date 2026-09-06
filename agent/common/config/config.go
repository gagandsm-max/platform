package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	AgentID       string
	CoreURL       string
	QueueDir      string
	PollInterval  time.Duration
	HealthPort    int
	LogLevel      string
	Environment   string
	MaxQueueItems int
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

func LoadConfig() *Config {
	return &Config{
		AgentID:       getEnv("AGENT_ID", ""),
		CoreURL:       getEnv("CORE_URL", "http://core:8443"),
		QueueDir:      getEnv("AGENT_QUEUE_DIR", "/var/lib/platform/agent/queue"),
		PollInterval:  time.Duration(getEnvInt("AGENT_POLL_INTERVAL_SECONDS", 15)) * time.Second,
		HealthPort:    getEnvInt("AGENT_HEALTH_PORT", 8089),
		LogLevel:      getEnv("PLATFORM_LOG_LEVEL", "INFO"),
		Environment:   getEnv("PLATFORM_ENV", "development"),
		MaxQueueItems: getEnvInt("AGENT_MAX_QUEUE_ITEMS", 1000),
	}
}
