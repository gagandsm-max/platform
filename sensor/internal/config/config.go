package config

import (
	"os"
	"strconv"
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
	}
}
