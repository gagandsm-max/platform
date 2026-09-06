package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	sensorruntime "platform/sensor/internal/runtime"
)

const StorageProtectionWatermarkPercent = 80

type Secret string

func (Secret) String() string   { return "[REDACTED]" }
func (Secret) GoString() string { return "[REDACTED]" }
func (s Secret) Value() string  { return string(s) }

type TLSFiles struct {
	Certificate string
	PrivateKey  string
	CA          string
}

type PoolConfig struct {
	Workers    int `json:"workers"`
	QueueDepth int `json:"queueDepth"`
}

type SchedulerConfig struct {
	MaxWorkers              int
	MaxQueueDepth           int
	MaxTrackedAssignments   int
	MaxTrackedJobs          int
	MaxPayloadBytes         int
	DefaultExecutionTimeout time.Duration
	ShutdownGracePeriod     time.Duration
	RetryMaxAttempts        int
	RetryInitialBackoff     time.Duration
	RetryMaxBackoff         time.Duration
	RetryJitterFraction     float64
	Pools                   map[string]PoolConfig
}

type TransportConfig struct {
	RequestTimeout     time.Duration
	MaxRequestBytes    int
	MaxResponseBytes   int
	UploadBatchRecords int
	UploadBatchBytes   int
}

type RuntimeConfig struct {
	SensorID                string
	Role                    sensorruntime.Role
	SiteID                  string
	PoolID                  string
	CoreEndpoint            string
	SpoolPath               string
	HealthAddress           string
	TLS                     TLSFiles
	BootstrapToken          Secret
	ConfigurationVersion    uint64
	LogLevel                string
	EnabledCapabilities     []sensorruntime.Capability
	StorageWatermarkPercent int
	Scheduler               SchedulerConfig
	Transport               TransportConfig
}

type SafeSnapshot struct {
	SensorID                string
	Role                    sensorruntime.Role
	SiteID                  string
	PoolID                  string
	CoreEndpoint            string
	SpoolPath               string
	HealthAddress           string
	TLSCertificate          string
	TLSPrivateKey           string
	TLSCA                   string
	EnrollmentMode          bool
	ConfigurationVersion    uint64
	LogLevel                string
	EnabledCapabilities     []sensorruntime.Capability
	StorageWatermarkPercent int
	Scheduler               SchedulerConfig
	Transport               TransportConfig
}

func (c RuntimeConfig) String() string {
	mode := "OPERATIONAL"
	if c.BootstrapToken.Value() != "" {
		mode = "ENROLLMENT"
	}
	return fmt.Sprintf("SensorConfig{sensorID=%s,role=%s,core=%s,spool=%s,mode=%s,configVersion=%d}", c.SensorID, c.Role, c.CoreEndpoint, c.SpoolPath, mode, c.ConfigurationVersion)
}
func (c RuntimeConfig) GoString() string { return c.String() }

func (c RuntimeConfig) Snapshot() SafeSnapshot {
	return SafeSnapshot{
		SensorID: c.SensorID, Role: c.Role, SiteID: c.SiteID, PoolID: c.PoolID,
		CoreEndpoint: c.CoreEndpoint, SpoolPath: c.SpoolPath, HealthAddress: c.HealthAddress,
		TLSCertificate: c.TLS.Certificate, TLSPrivateKey: c.TLS.PrivateKey, TLSCA: c.TLS.CA,
		EnrollmentMode: c.BootstrapToken.Value() != "", ConfigurationVersion: c.ConfigurationVersion,
		LogLevel: c.LogLevel, EnabledCapabilities: append([]sensorruntime.Capability(nil), c.EnabledCapabilities...),
		StorageWatermarkPercent: c.StorageWatermarkPercent, Scheduler: cloneScheduler(c.Scheduler), Transport: c.Transport,
	}
}

func cloneScheduler(in SchedulerConfig) SchedulerConfig {
	out := in
	out.Pools = make(map[string]PoolConfig, len(in.Pools))
	for k, v := range in.Pools {
		out.Pools[k] = v
	}
	return out
}

type ChangeClass string

const (
	ChangeReloadable      ChangeClass = "RELOADABLE"
	ChangeRestartRequired ChangeClass = "RESTART_REQUIRED"
)

type Change struct {
	Field string
	Class ChangeClass
}

func Diff(oldCfg, newCfg RuntimeConfig) []Change {
	var changes []Change
	add := func(field string, class ChangeClass, changed bool) {
		if changed {
			changes = append(changes, Change{Field: field, Class: class})
		}
	}
	add("configuration_version", ChangeReloadable, oldCfg.ConfigurationVersion != newCfg.ConfigurationVersion)
	add("log_level", ChangeReloadable, oldCfg.LogLevel != newCfg.LogLevel)
	add("sensor_id", ChangeRestartRequired, oldCfg.SensorID != newCfg.SensorID)
	add("role", ChangeRestartRequired, oldCfg.Role != newCfg.Role)
	add("site_id", ChangeRestartRequired, oldCfg.SiteID != newCfg.SiteID)
	add("pool_id", ChangeRestartRequired, oldCfg.PoolID != newCfg.PoolID)
	add("core_endpoint", ChangeRestartRequired, oldCfg.CoreEndpoint != newCfg.CoreEndpoint)
	add("spool_path", ChangeRestartRequired, oldCfg.SpoolPath != newCfg.SpoolPath)
	add("health_address", ChangeRestartRequired, oldCfg.HealthAddress != newCfg.HealthAddress)
	add("tls", ChangeRestartRequired, oldCfg.TLS != newCfg.TLS)
	add("bootstrap_token_presence", ChangeRestartRequired, (oldCfg.BootstrapToken.Value() != "") != (newCfg.BootstrapToken.Value() != ""))
	add("enabled_capabilities", ChangeRestartRequired, !reflect.DeepEqual(oldCfg.EnabledCapabilities, newCfg.EnabledCapabilities))
	add("scheduler", ChangeRestartRequired, !reflect.DeepEqual(oldCfg.Scheduler, newCfg.Scheduler))
	add("transport", ChangeRestartRequired, oldCfg.Transport != newCfg.Transport)
	sort.Slice(changes, func(i, j int) bool { return changes[i].Field < changes[j].Field })
	return changes
}

var allowedKeys = map[string]struct{}{
	"PLATFORM_SENSOR_ID": {}, "PLATFORM_SENSOR_ROLE": {}, "PLATFORM_SENSOR_SITE_ID": {}, "PLATFORM_SENSOR_POOL_ID": {},
	"PLATFORM_SENSOR_CORE_ENDPOINT": {}, "PLATFORM_SENSOR_SPOOL_PATH": {}, "PLATFORM_SENSOR_HEALTH_ADDRESS": {},
	"PLATFORM_SENSOR_TLS_CERT_FILE": {}, "PLATFORM_SENSOR_TLS_KEY_FILE": {}, "PLATFORM_SENSOR_CA_FILE": {},
	"PLATFORM_SENSOR_BOOTSTRAP_TOKEN": {}, "PLATFORM_SENSOR_CONFIG_VERSION": {}, "PLATFORM_SENSOR_LOG_LEVEL": {},
	"PLATFORM_SENSOR_ENABLED_CAPABILITIES": {}, "PLATFORM_SENSOR_STORAGE_WATERMARK_PERCENT": {},
	"PLATFORM_SENSOR_SCHEDULER_MAX_WORKERS": {}, "PLATFORM_SENSOR_SCHEDULER_MAX_QUEUE_DEPTH": {},
	"PLATFORM_SENSOR_SCHEDULER_MAX_TRACKED_ASSIGNMENTS": {}, "PLATFORM_SENSOR_SCHEDULER_MAX_TRACKED_JOBS": {},
	"PLATFORM_SENSOR_SCHEDULER_MAX_PAYLOAD_BYTES": {}, "PLATFORM_SENSOR_SCHEDULER_DEFAULT_EXECUTION_TIMEOUT": {},
	"PLATFORM_SENSOR_SCHEDULER_SHUTDOWN_GRACE": {}, "PLATFORM_SENSOR_SCHEDULER_RETRY_MAX_ATTEMPTS": {},
	"PLATFORM_SENSOR_SCHEDULER_RETRY_INITIAL_BACKOFF": {}, "PLATFORM_SENSOR_SCHEDULER_RETRY_MAX_BACKOFF": {},
	"PLATFORM_SENSOR_SCHEDULER_RETRY_JITTER": {}, "PLATFORM_SENSOR_SCHEDULER_POOLS_JSON": {},
	"PLATFORM_SENSOR_TRANSPORT_REQUEST_TIMEOUT": {}, "PLATFORM_SENSOR_TRANSPORT_MAX_REQUEST_BYTES": {},
	"PLATFORM_SENSOR_TRANSPORT_MAX_RESPONSE_BYTES": {}, "PLATFORM_SENSOR_UPLOAD_BATCH_RECORDS": {}, "PLATFORM_SENSOR_UPLOAD_BATCH_BYTES": {},
}

func LoadFromEnv() (RuntimeConfig, error) {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(key, "PLATFORM_SENSOR_") {
			continue
		}
		values[key] = value
	}
	return LoadMap(values)
}

func LoadMap(values map[string]string) (RuntimeConfig, error) {
	for key := range values {
		if strings.HasPrefix(key, "PLATFORM_SENSOR_") {
			if _, ok := allowedKeys[key]; !ok {
				return RuntimeConfig{}, fmt.Errorf("SENSOR_CONFIG_UNKNOWN_FIELD: %s", key)
			}
		}
	}
	get := func(key string) string { return strings.TrimSpace(values[key]) }
	cfg := RuntimeConfig{
		SensorID: get("PLATFORM_SENSOR_ID"), Role: sensorruntime.Role(strings.ToUpper(get("PLATFORM_SENSOR_ROLE"))),
		SiteID: get("PLATFORM_SENSOR_SITE_ID"), PoolID: get("PLATFORM_SENSOR_POOL_ID"), CoreEndpoint: get("PLATFORM_SENSOR_CORE_ENDPOINT"),
		SpoolPath: get("PLATFORM_SENSOR_SPOOL_PATH"), HealthAddress: get("PLATFORM_SENSOR_HEALTH_ADDRESS"),
		TLS: TLSFiles{Certificate: get("PLATFORM_SENSOR_TLS_CERT_FILE"), PrivateKey: get("PLATFORM_SENSOR_TLS_KEY_FILE"), CA: get("PLATFORM_SENSOR_CA_FILE")},
		BootstrapToken: Secret(values["PLATFORM_SENSOR_BOOTSTRAP_TOKEN"]), LogLevel: strings.ToUpper(get("PLATFORM_SENSOR_LOG_LEVEL")),
		StorageWatermarkPercent: StorageProtectionWatermarkPercent,
	}
	var err error
	if cfg.ConfigurationVersion, err = parseUint64(get("PLATFORM_SENSOR_CONFIG_VERSION"), "configuration version"); err != nil { return RuntimeConfig{}, err }
	if cfg.EnabledCapabilities, err = parseCapabilities(get("PLATFORM_SENSOR_ENABLED_CAPABILITIES")); err != nil { return RuntimeConfig{}, err }
	if raw := get("PLATFORM_SENSOR_STORAGE_WATERMARK_PERCENT"); raw != "" && raw != strconv.Itoa(StorageProtectionWatermarkPercent) {
		return RuntimeConfig{}, fmt.Errorf("SENSOR_CONFIG_STORAGE_PROTECTION_FIXED: Sensor watermark must remain %d%%", StorageProtectionWatermarkPercent)
	}
	if cfg.Scheduler, err = parseScheduler(values); err != nil { return RuntimeConfig{}, err }
	if cfg.Transport, err = parseTransport(values); err != nil { return RuntimeConfig{}, err }
	if err := cfg.Validate(); err != nil { return RuntimeConfig{}, err }
	return cfg, nil
}

func (c RuntimeConfig) Validate() error {
	if err := sensorruntime.ValidateCapabilities(c.Role, c.EnabledCapabilities); err != nil { return fmt.Errorf("SENSOR_CONFIG_INVALID_ROLE_CAPABILITIES: %w", err) }
	if c.CoreEndpoint == "" { return fmt.Errorf("SENSOR_CONFIG_CORE_ENDPOINT_REQUIRED: Core endpoint is required") }
	u, err := url.Parse(c.CoreEndpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("SENSOR_CONFIG_CORE_ENDPOINT_INVALID: Core endpoint must be an HTTPS origin without credentials/query/fragment")
	}
	if c.SpoolPath == "" || !filepath.IsAbs(c.SpoolPath) { return fmt.Errorf("SENSOR_CONFIG_SPOOL_PATH_INVALID: spool path must be absolute") }
	if c.HealthAddress == "" { return fmt.Errorf("SENSOR_CONFIG_HEALTH_ADDRESS_REQUIRED: health address is required") }
	host, port, err := net.SplitHostPort(c.HealthAddress)
	if err != nil || host == "" && !strings.HasPrefix(c.HealthAddress, ":") || port == "" { return fmt.Errorf("SENSOR_CONFIG_HEALTH_ADDRESS_INVALID: health address must be host:port or :port") }
	if _, err := strconv.ParseUint(port, 10, 16); err != nil || port == "0" { return fmt.Errorf("SENSOR_CONFIG_HEALTH_ADDRESS_INVALID: health port must be 1-65535") }
	if c.TLS.CA == "" || !filepath.IsAbs(c.TLS.CA) { return fmt.Errorf("SENSOR_CONFIG_CA_REQUIRED: absolute CA path is required") }
	if (c.TLS.Certificate == "") != (c.TLS.PrivateKey == "") { return fmt.Errorf("SENSOR_CONFIG_TLS_KEYPAIR_INCOMPLETE: certificate and private key must be configured together") }
	enrollment := c.BootstrapToken.Value() != ""
	operationalIdentity := c.SensorID != "" && c.TLS.Certificate != "" && c.TLS.PrivateKey != ""
	if enrollment && operationalIdentity { return fmt.Errorf("SENSOR_CONFIG_IDENTITY_CONFLICT: bootstrap token and operational identity must not be configured together") }
	if !enrollment && !operationalIdentity { return fmt.Errorf("SENSOR_CONFIG_IDENTITY_REQUIRED: configure bootstrap token or sensor ID plus client certificate/key") }
	if c.TLS.Certificate != "" && (!filepath.IsAbs(c.TLS.Certificate) || !filepath.IsAbs(c.TLS.PrivateKey)) { return fmt.Errorf("SENSOR_CONFIG_TLS_PATH_INVALID: certificate and private key paths must be absolute") }
	if c.ConfigurationVersion == 0 { return fmt.Errorf("SENSOR_CONFIG_VERSION_REQUIRED: configuration version must be greater than zero") }
	switch c.LogLevel { case "DEBUG", "INFO", "WARN", "ERROR": default: return fmt.Errorf("SENSOR_CONFIG_LOG_LEVEL_INVALID: supported values are DEBUG, INFO, WARN, ERROR") }
	if c.StorageWatermarkPercent != StorageProtectionWatermarkPercent { return fmt.Errorf("SENSOR_CONFIG_STORAGE_PROTECTION_FIXED: Sensor watermark must remain %d%%", StorageProtectionWatermarkPercent) }
	if err := validateScheduler(c.Scheduler); err != nil { return err }
	if err := validateTransport(c.Transport); err != nil { return err }
	return nil
}

func parseCapabilities(raw string) ([]sensorruntime.Capability, error) {
	if raw == "" { return nil, fmt.Errorf("SENSOR_CONFIG_CAPABILITIES_REQUIRED: at least one capability is required") }
	parts := strings.Split(raw, ",")
	out := make([]sensorruntime.Capability, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" { return nil, fmt.Errorf("SENSOR_CONFIG_CAPABILITY_INVALID: empty capability") }
		out = append(out, sensorruntime.Capability(strings.ToUpper(value)))
	}
	return out, nil
}

func parseScheduler(values map[string]string) (SchedulerConfig, error) {
	var c SchedulerConfig
	var err error
	if c.MaxWorkers, err = parsePositiveInt(values["PLATFORM_SENSOR_SCHEDULER_MAX_WORKERS"], "scheduler max workers"); err != nil { return c, err }
	if c.MaxQueueDepth, err = parsePositiveInt(values["PLATFORM_SENSOR_SCHEDULER_MAX_QUEUE_DEPTH"], "scheduler max queue depth"); err != nil { return c, err }
	if c.MaxTrackedAssignments, err = parsePositiveInt(values["PLATFORM_SENSOR_SCHEDULER_MAX_TRACKED_ASSIGNMENTS"], "scheduler max tracked assignments"); err != nil { return c, err }
	if c.MaxTrackedJobs, err = parsePositiveInt(values["PLATFORM_SENSOR_SCHEDULER_MAX_TRACKED_JOBS"], "scheduler max tracked jobs"); err != nil { return c, err }
	if c.MaxPayloadBytes, err = parsePositiveInt(values["PLATFORM_SENSOR_SCHEDULER_MAX_PAYLOAD_BYTES"], "scheduler max payload bytes"); err != nil { return c, err }
	if c.DefaultExecutionTimeout, err = parsePositiveDuration(values["PLATFORM_SENSOR_SCHEDULER_DEFAULT_EXECUTION_TIMEOUT"], "scheduler default execution timeout"); err != nil { return c, err }
	if c.ShutdownGracePeriod, err = parsePositiveDuration(values["PLATFORM_SENSOR_SCHEDULER_SHUTDOWN_GRACE"], "scheduler shutdown grace"); err != nil { return c, err }
	if c.RetryMaxAttempts, err = parsePositiveInt(values["PLATFORM_SENSOR_SCHEDULER_RETRY_MAX_ATTEMPTS"], "scheduler retry max attempts"); err != nil { return c, err }
	if c.RetryInitialBackoff, err = parsePositiveDuration(values["PLATFORM_SENSOR_SCHEDULER_RETRY_INITIAL_BACKOFF"], "scheduler retry initial backoff"); err != nil { return c, err }
	if c.RetryMaxBackoff, err = parsePositiveDuration(values["PLATFORM_SENSOR_SCHEDULER_RETRY_MAX_BACKOFF"], "scheduler retry max backoff"); err != nil { return c, err }
	if c.RetryJitterFraction, err = parseFraction(values["PLATFORM_SENSOR_SCHEDULER_RETRY_JITTER"], "scheduler retry jitter"); err != nil { return c, err }
	raw := strings.TrimSpace(values["PLATFORM_SENSOR_SCHEDULER_POOLS_JSON"])
	if raw == "" { return c, fmt.Errorf("SENSOR_CONFIG_SCHEDULER_POOLS_REQUIRED: scheduler pools JSON is required") }
	dec := json.NewDecoder(strings.NewReader(raw)); dec.DisallowUnknownFields()
	if err := dec.Decode(&c.Pools); err != nil { return c, fmt.Errorf("SENSOR_CONFIG_SCHEDULER_POOLS_INVALID: %w", err) }
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF { return c, fmt.Errorf("SENSOR_CONFIG_SCHEDULER_POOLS_INVALID: trailing JSON document") }
	return c, nil
}

func validateScheduler(c SchedulerConfig) error {
	if c.MaxWorkers <= 0 || c.MaxQueueDepth <= 0 || c.MaxTrackedAssignments <= 0 || c.MaxTrackedJobs <= 0 || c.MaxPayloadBytes <= 0 { return fmt.Errorf("SENSOR_CONFIG_SCHEDULER_BOUNDS_INVALID: scheduler bounds must be positive") }
	if c.DefaultExecutionTimeout <= 0 || c.ShutdownGracePeriod <= 0 { return fmt.Errorf("SENSOR_CONFIG_SCHEDULER_TIMEOUT_INVALID: scheduler timeouts must be positive") }
	if c.RetryMaxAttempts <= 0 || c.RetryInitialBackoff <= 0 || c.RetryMaxBackoff < c.RetryInitialBackoff || c.RetryJitterFraction < 0 || c.RetryJitterFraction > 1 { return fmt.Errorf("SENSOR_CONFIG_SCHEDULER_RETRY_INVALID: scheduler retry policy is invalid") }
	if len(c.Pools) == 0 { return fmt.Errorf("SENSOR_CONFIG_SCHEDULER_POOLS_REQUIRED: at least one scheduler pool is required") }
	workers, queues := 0, 0
	for class, pool := range c.Pools {
		switch class { case "NETWORK_POLL", "STREAMING", "INFRA", "DISCOVERY", "NCM", "SYNTHETIC": default: return fmt.Errorf("SENSOR_CONFIG_SCHEDULER_POOL_CLASS_INVALID: %s", class) }
		if pool.Workers <= 0 || pool.QueueDepth <= 0 { return fmt.Errorf("SENSOR_CONFIG_SCHEDULER_POOL_INVALID: pool %s bounds must be positive", class) }
		workers += pool.Workers; queues += pool.QueueDepth
		if workers > c.MaxWorkers || queues > c.MaxQueueDepth { return fmt.Errorf("SENSOR_CONFIG_SCHEDULER_POOL_BOUNDS_EXCEEDED: configured pools exceed scheduler maxima") }
	}
	if c.MaxTrackedJobs < c.MaxQueueDepth || c.MaxTrackedJobs-c.MaxQueueDepth < c.MaxWorkers { return fmt.Errorf("SENSOR_CONFIG_SCHEDULER_TRACKING_INVALID: tracked jobs must cover queued and active work") }
	return nil
}

func parseTransport(values map[string]string) (TransportConfig, error) {
	var c TransportConfig
	var err error
	if c.RequestTimeout, err = parsePositiveDuration(values["PLATFORM_SENSOR_TRANSPORT_REQUEST_TIMEOUT"], "transport request timeout"); err != nil { return c, err }
	if c.MaxRequestBytes, err = parsePositiveInt(values["PLATFORM_SENSOR_TRANSPORT_MAX_REQUEST_BYTES"], "transport max request bytes"); err != nil { return c, err }
	if c.MaxResponseBytes, err = parsePositiveInt(values["PLATFORM_SENSOR_TRANSPORT_MAX_RESPONSE_BYTES"], "transport max response bytes"); err != nil { return c, err }
	if c.UploadBatchRecords, err = parsePositiveInt(values["PLATFORM_SENSOR_UPLOAD_BATCH_RECORDS"], "upload batch records"); err != nil { return c, err }
	if c.UploadBatchBytes, err = parsePositiveInt(values["PLATFORM_SENSOR_UPLOAD_BATCH_BYTES"], "upload batch bytes"); err != nil { return c, err }
	return c, nil
}
func validateTransport(c TransportConfig) error {
	if c.RequestTimeout <= 0 || c.MaxRequestBytes <= 0 || c.MaxResponseBytes <= 0 || c.UploadBatchRecords <= 0 || c.UploadBatchBytes <= 0 { return fmt.Errorf("SENSOR_CONFIG_TRANSPORT_BOUNDS_INVALID: transport bounds must be positive") }
	if c.UploadBatchBytes > c.MaxRequestBytes { return fmt.Errorf("SENSOR_CONFIG_UPLOAD_BOUND_INVALID: upload batch bytes cannot exceed transport max request bytes") }
	return nil
}

func parsePositiveInt(raw, name string) (int, error) { v, err := strconv.Atoi(strings.TrimSpace(raw)); if err != nil || v <= 0 { return 0, fmt.Errorf("SENSOR_CONFIG_VALUE_INVALID: %s must be a positive integer", name) }; return v, nil }
func parseUint64(raw, name string) (uint64, error) { v, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64); if err != nil || v == 0 { return 0, fmt.Errorf("SENSOR_CONFIG_VALUE_INVALID: %s must be a positive integer", name) }; return v, nil }
func parsePositiveDuration(raw, name string) (time.Duration, error) { v, err := time.ParseDuration(strings.TrimSpace(raw)); if err != nil || v <= 0 { return 0, fmt.Errorf("SENSOR_CONFIG_VALUE_INVALID: %s must be a positive duration", name) }; return v, nil }
func parseFraction(raw, name string) (float64, error) { v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); if err != nil || v < 0 || v > 1 { return 0, fmt.Errorf("SENSOR_CONFIG_VALUE_INVALID: %s must be between 0 and 1", name) }; return v, nil }
