package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var allowedEnvVars = map[string]bool{
	"PLATFORM_AGENT_TENANT_ID":           true,
	"PLATFORM_AGENT_SENSOR_URL":          true,
	"PLATFORM_AGENT_CORE_URL":            true,
	"PLATFORM_AGENT_CA_FILE":             true,
	"PLATFORM_AGENT_CERT_FILE":           true,
	"PLATFORM_AGENT_KEY_FILE":            true,
	"PLATFORM_AGENT_ID":                  true,
	"PLATFORM_AGENT_SITE_ID":             true,
	"PLATFORM_AGENT_STATE_DIR":           true,
	"PLATFORM_AGENT_COLLECTION_INTERVAL": true,
	"PLATFORM_AGENT_COLLECTION_TIMEOUT":  true,
	"PLATFORM_AGENT_HEALTH_ADDRESS":      true,
	"PLATFORM_AGENT_MAX_BUFFER_RECORDS":  true,
	"PLATFORM_AGENT_MAX_BUFFER_BYTES":    true,
	"PLATFORM_AGENT_MAX_RECORD_BYTES":    true,
	"PLATFORM_AGENT_MAX_BATCH_RECORDS":   true,
	"PLATFORM_AGENT_MAX_BATCH_BYTES":     true,
	"PLATFORM_AGENT_MAX_FILESYSTEMS":     true,
	"PLATFORM_AGENT_RETRY_BASE":          true,
	"PLATFORM_AGENT_RETRY_MAX":           true,
	"PLATFORM_AGENT_DELIVERY_TIMEOUT":    true,
	"PLATFORM_LOG_LEVEL":                 true,
	"PLATFORM_ENV":                       true,
}

type Config struct {
	TenantID           string
	SensorURL          string
	CoreURL            string
	CAFile             string
	CertFile           string
	KeyFile            string
	AgentID            string
	SiteID             string
	StateDir           string
	QueueDir           string
	CollectionInterval time.Duration
	CollectionTimeout  time.Duration
	HealthAddress      string
	MaxBufferRecords   int
	MaxBufferBytes     int64
	MaxRecordBytes     int
	MaxBatchRecords    int
	MaxBatchBytes      int64
	MaxFilesystems     int
	RetryBase          time.Duration
	RetryMax           time.Duration
	DeliveryTimeout    time.Duration
	LogLevel           string
	Environment        string
}

func LoadConfig() (*Config, error) {
	return LoadConfigFromEnv(os.Environ())
}

func LoadConfigFromEnv(environ []string) (*Config, error) {
	envMap := make(map[string]string)
	for _, entry := range environ {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			k := parts[0]
			v := parts[1]
			// Strict fail-closed: reject any unknown PLATFORM_AGENT_* settings
			if strings.HasPrefix(k, "PLATFORM_AGENT_") && !allowedEnvVars[k] {
				return nil, fmt.Errorf("CONFIG_UNKNOWN_VARIABLE: unknown agent setting %q is prohibited", k)
			}
			envMap[k] = v
		}
	}

	c := &Config{}

	// 1. Required Tenant ID
	c.TenantID = strings.TrimSpace(envMap["PLATFORM_AGENT_TENANT_ID"])
	if c.TenantID == "" {
		return nil, errors.New("CONFIG_TENANT_ID_REQUIRED: PLATFORM_AGENT_TENANT_ID must be set")
	}

	// 2. Required Sensor telemetry URL (HTTPS origin only). Telemetry never
	// uses Core. Optional Core URL is enrollment/control-plane origin only.
	sensorOrigin, err := parseHTTPSOrigin(envMap["PLATFORM_AGENT_SENSOR_URL"], "PLATFORM_AGENT_SENSOR_URL")
	if err != nil {
		return nil, err
	}
	if sensorOrigin == "" {
		return nil, errors.New("CONFIG_SENSOR_URL_REQUIRED: PLATFORM_AGENT_SENSOR_URL must be set")
	}
	c.SensorURL = sensorOrigin

	coreRaw := strings.TrimSpace(envMap["PLATFORM_AGENT_CORE_URL"])
	if coreRaw != "" {
		coreOrigin, err := parseHTTPSOrigin(coreRaw, "PLATFORM_AGENT_CORE_URL")
		if err != nil {
			return nil, err
		}
		c.CoreURL = coreOrigin
	}

	// 3. Required TLS file paths
	c.CAFile = strings.TrimSpace(envMap["PLATFORM_AGENT_CA_FILE"])
	if c.CAFile == "" {
		return nil, errors.New("CONFIG_CA_FILE_REQUIRED: PLATFORM_AGENT_CA_FILE must be set")
	}
	if err := validateRegularFile(c.CAFile, false); err != nil {
		return nil, fmt.Errorf("CONFIG_CA_FILE_INVALID: %w", err)
	}

	c.CertFile = strings.TrimSpace(envMap["PLATFORM_AGENT_CERT_FILE"])
	if c.CertFile == "" {
		return nil, errors.New("CONFIG_CERT_FILE_REQUIRED: PLATFORM_AGENT_CERT_FILE must be set")
	}
	if err := validateRegularFile(c.CertFile, false); err != nil {
		return nil, fmt.Errorf("CONFIG_CERT_FILE_INVALID: %w", err)
	}

	c.KeyFile = strings.TrimSpace(envMap["PLATFORM_AGENT_KEY_FILE"])
	if c.KeyFile == "" {
		return nil, errors.New("CONFIG_KEY_FILE_REQUIRED: PLATFORM_AGENT_KEY_FILE must be set")
	}
	if err := validateRegularFile(c.KeyFile, true); err != nil {
		return nil, fmt.Errorf("CONFIG_KEY_FILE_INVALID: %w", err)
	}

	// Verify keypair loads cleanly
	if _, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile); err != nil {
		return nil, fmt.Errorf("CONFIG_KEYPAIR_INVALID: certificate and key do not match or cannot be parsed: %w", err)
	}

	// 4. Optional IDs and directories
	c.AgentID = strings.TrimSpace(envMap["PLATFORM_AGENT_ID"])
	if c.AgentID == "" {
		return nil, errors.New("CONFIG_AGENT_ID_REQUIRED: PLATFORM_AGENT_ID must be set; Sensor authorizes Core-assigned agent identity")
	}
	c.SiteID = strings.TrimSpace(envMap["PLATFORM_AGENT_SITE_ID"])

	c.StateDir = strings.TrimSpace(envMap["PLATFORM_AGENT_STATE_DIR"])
	if c.StateDir == "" {
		c.StateDir = "/var/lib/platform/agent"
	}
	c.QueueDir = filepath.Join(c.StateDir, "queue")

	// 5. Intervals & timeouts
	c.CollectionInterval = 15 * time.Second
	if val := envMap["PLATFORM_AGENT_COLLECTION_INTERVAL"]; val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			return nil, fmt.Errorf("CONFIG_COLLECTION_INTERVAL_INVALID: %w", err)
		}
		if d < time.Second || d > time.Hour {
			return nil, errors.New("CONFIG_COLLECTION_INTERVAL_OUT_OF_BOUNDS: must be between 1s and 1h")
		}
		c.CollectionInterval = d
	}

	c.CollectionTimeout = 5 * time.Second
	if val := envMap["PLATFORM_AGENT_COLLECTION_TIMEOUT"]; val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			return nil, fmt.Errorf("CONFIG_COLLECTION_TIMEOUT_INVALID: %w", err)
		}
		if d < 100*time.Millisecond || d > 30*time.Second {
			return nil, errors.New("CONFIG_COLLECTION_TIMEOUT_OUT_OF_BOUNDS: must be between 100ms and 30s")
		}
		c.CollectionTimeout = d
	}

	c.DeliveryTimeout = 10 * time.Second
	if val := envMap["PLATFORM_AGENT_DELIVERY_TIMEOUT"]; val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			return nil, fmt.Errorf("CONFIG_DELIVERY_TIMEOUT_INVALID: %w", err)
		}
		if d < time.Second || d > 60*time.Second {
			return nil, errors.New("CONFIG_DELIVERY_TIMEOUT_OUT_OF_BOUNDS: must be between 1s and 60s")
		}
		c.DeliveryTimeout = d
	}

	c.RetryBase = time.Second
	if val := envMap["PLATFORM_AGENT_RETRY_BASE"]; val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			return nil, fmt.Errorf("CONFIG_RETRY_BASE_INVALID: %w", err)
		}
		if d < 100*time.Millisecond || d > 30*time.Second {
			return nil, errors.New("CONFIG_RETRY_BASE_OUT_OF_BOUNDS: must be between 100ms and 30s")
		}
		c.RetryBase = d
	}

	c.RetryMax = 60 * time.Second
	if val := envMap["PLATFORM_AGENT_RETRY_MAX"]; val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			return nil, fmt.Errorf("CONFIG_RETRY_MAX_INVALID: %w", err)
		}
		if d < time.Second || d > 10*time.Minute {
			return nil, errors.New("CONFIG_RETRY_MAX_OUT_OF_BOUNDS: must be between 1s and 10m")
		}
		c.RetryMax = d
	}
	if c.RetryMax < c.RetryBase {
		return nil, errors.New("CONFIG_RETRY_BOUNDS_INVALID: RETRY_MAX cannot be less than RETRY_BASE")
	}

	// 6. Health address (loopback only)
	c.HealthAddress = strings.TrimSpace(envMap["PLATFORM_AGENT_HEALTH_ADDRESS"])
	if c.HealthAddress == "" {
		c.HealthAddress = "127.0.0.1:8089"
	}
	host, _, err := net.SplitHostPort(c.HealthAddress)
	if err != nil {
		return nil, fmt.Errorf("CONFIG_HEALTH_ADDRESS_INVALID: %w", err)
	}
	if host != "127.0.0.1" && host != "::1" && host != "localhost" {
		return nil, errors.New("CONFIG_HEALTH_ADDRESS_INSECURE: health endpoint must bind to loopback address only")
	}

	// 7. Numeric limits
	c.MaxBufferRecords = 1000
	if val := envMap["PLATFORM_AGENT_MAX_BUFFER_RECORDS"]; val != "" {
		n, err := strconv.Atoi(val)
		if err != nil || n < 10 || n > 100000 {
			return nil, errors.New("CONFIG_MAX_BUFFER_RECORDS_OUT_OF_BOUNDS: must be between 10 and 100000")
		}
		c.MaxBufferRecords = n
	}

	c.MaxBufferBytes = 50 * 1024 * 1024 // 50MB
	if val := envMap["PLATFORM_AGENT_MAX_BUFFER_BYTES"]; val != "" {
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil || n < 1024*1024 || n > 1024*1024*1024 {
			return nil, errors.New("CONFIG_MAX_BUFFER_BYTES_OUT_OF_BOUNDS: must be between 1MB and 1GB")
		}
		c.MaxBufferBytes = n
	}

	c.MaxRecordBytes = 64 * 1024 // 64KB
	if val := envMap["PLATFORM_AGENT_MAX_RECORD_BYTES"]; val != "" {
		n, err := strconv.Atoi(val)
		if err != nil || n < 1024 || n > 1024*1024 {
			return nil, errors.New("CONFIG_MAX_RECORD_BYTES_OUT_OF_BOUNDS: must be between 1KB and 1MB")
		}
		c.MaxRecordBytes = n
	}

	c.MaxBatchRecords = 100
	if val := envMap["PLATFORM_AGENT_MAX_BATCH_RECORDS"]; val != "" {
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 || n > 1000 {
			return nil, errors.New("CONFIG_MAX_BATCH_RECORDS_OUT_OF_BOUNDS: must be between 1 and 1000")
		}
		c.MaxBatchRecords = n
	}

	// One Agent delivery envelope becomes one Core agent-ingress record.
	// Core AgentIngestionValidator.MAX_RECORD_BYTES is 512 KiB; leave wrap room.
	c.MaxBatchBytes = 384 * 1024
	if val := envMap["PLATFORM_AGENT_MAX_BATCH_BYTES"]; val != "" {
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil || n < 1024 || n > 10*1024*1024 {
			return nil, errors.New("CONFIG_MAX_BATCH_BYTES_OUT_OF_BOUNDS: must be between 1KB and 10MB")
		}
		c.MaxBatchBytes = n
	}

	c.MaxFilesystems = 32
	if val := envMap["PLATFORM_AGENT_MAX_FILESYSTEMS"]; val != "" {
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 || n > 256 {
			return nil, errors.New("CONFIG_MAX_FILESYSTEMS_OUT_OF_BOUNDS: must be between 1 and 256")
		}
		c.MaxFilesystems = n
	}

	// 8. Log level
	c.LogLevel = "INFO"
	if val := envMap["PLATFORM_LOG_LEVEL"]; val != "" {
		val = strings.ToUpper(strings.TrimSpace(val))
		switch val {
		case "DEBUG", "INFO", "WARN", "ERROR":
			c.LogLevel = val
		default:
			return nil, fmt.Errorf("CONFIG_LOG_LEVEL_INVALID: unsupported log level %q", val)
		}
	}

	c.Environment = "development"
	if val := envMap["PLATFORM_ENV"]; val != "" {
		c.Environment = strings.TrimSpace(val)
	}

	return c, nil
}

func parseHTTPSOrigin(raw, name string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("CONFIG_%s_INVALID: %w", strings.TrimPrefix(name, "PLATFORM_AGENT_"), err)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("CONFIG_%s_SCHEME_INSECURE: %s must use https", strings.TrimPrefix(name, "PLATFORM_AGENT_"), name)
	}
	if u.User != nil {
		return "", fmt.Errorf("CONFIG_%s_CREDENTIALS_PROHIBITED: %s must not contain credentials", strings.TrimPrefix(name, "PLATFORM_AGENT_"), name)
	}
	if u.Host == "" {
		return "", fmt.Errorf("CONFIG_%s_HOST_REQUIRED: %s must include valid host", strings.TrimPrefix(name, "PLATFORM_AGENT_"), name)
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("CONFIG_%s_PATH_PROHIBITED: %s must be origin-only", strings.TrimPrefix(name, "PLATFORM_AGENT_"), name)
	}
	return "https://" + u.Host, nil
}

func validateRegularFile(path string, checkKeyPerms bool) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(absPath)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return errors.New("symlinks are prohibited for TLS security material")
	}
	if !fi.Mode().IsRegular() {
		return errors.New("file is not a regular file")
	}

	if checkKeyPerms {
		// Verify no group or world read/write/exec permissions
		if fi.Mode().Perm()&0077 != 0 {
			return errors.New("private key permissions are insecure; must not be group- or world-accessible (mode 0600 expected)")
		}
	}
	return nil
}
