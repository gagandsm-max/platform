package control

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
)

// Domain is the Sensor control desired-config checksum domain prefix (Java SensorControlDesiredChecksum.DOMAIN).
const Domain = "platform.sensor.control.desired.v1"

// GoldenChecksumSHA256 is the cross-language golden digest for the Slice 2/3 fixture.
const GoldenChecksumSHA256 = "ec1e58221b8f3bb05ce0ddc4d8104b2e6bfce709e0de4cc07062e80919346f1d"

// DesiredConfiguration is the Slice 2 controlled configuration subset (no identity/secrets).
type DesiredConfiguration struct {
	LogLevel  string            `json:"logLevel"`
	Scheduler DesiredScheduler  `json:"scheduler"`
	Transport DesiredTransport  `json:"transport"`
}

// DesiredScheduler mirrors Core SensorSchedulerDesiredConfig.
type DesiredScheduler struct {
	MaxWorkers               int                        `json:"maxWorkers"`
	MaxQueueDepth            int                        `json:"maxQueueDepth"`
	MaxTrackedAssignments    int                        `json:"maxTrackedAssignments"`
	MaxTrackedJobs           int                        `json:"maxTrackedJobs"`
	MaxPayloadBytes          int                        `json:"maxPayloadBytes"`
	DefaultExecutionTimeoutMs int                       `json:"defaultExecutionTimeoutMs"`
	ShutdownGracePeriodMs    int                        `json:"shutdownGracePeriodMs"`
	RetryMaxAttempts         int                        `json:"retryMaxAttempts"`
	RetryInitialBackoffMs    int                        `json:"retryInitialBackoffMs"`
	RetryMaxBackoffMs        int                        `json:"retryMaxBackoffMs"`
	RetryJitterBasisPoints   int                        `json:"retryJitterBasisPoints"`
	Pools                    map[string]DesiredPool     `json:"pools"`
}

// DesiredPool is a named scheduler pool.
type DesiredPool struct {
	Workers    int `json:"workers"`
	QueueDepth int `json:"queueDepth"`
}

// DesiredTransport mirrors Core SensorTransportDesiredConfig.
type DesiredTransport struct {
	RequestTimeoutMs   int `json:"requestTimeoutMs"`
	MaxRequestBytes    int `json:"maxRequestBytes"`
	MaxResponseBytes   int `json:"maxResponseBytes"`
	UploadBatchRecords int `json:"uploadBatchRecords"`
	UploadBatchBytes   int `json:"uploadBatchBytes"`
}

// ComputeDesiredChecksum returns lowercase SHA-256 hex over semantic configuration fields.
// Generation is intentionally excluded. Pool map insertion order must not affect the digest.
func ComputeDesiredChecksum(cfg DesiredConfiguration) (string, error) {
	if cfg.Scheduler.Pools == nil {
		return "", fmt.Errorf("%s: pools map is required", CodeInvalidDesired)
	}
	h := sha256.New()
	h.Write([]byte(Domain))
	h.Write([]byte{0x00})

	write := func(name, value string) {
		writeLengthPrefixed(h, name)
		writeLengthPrefixed(h, value)
	}

	write("logLevel", cfg.LogLevel)
	write("maxWorkers", strconv.Itoa(cfg.Scheduler.MaxWorkers))
	write("maxQueueDepth", strconv.Itoa(cfg.Scheduler.MaxQueueDepth))
	write("maxTrackedAssignments", strconv.Itoa(cfg.Scheduler.MaxTrackedAssignments))
	write("maxTrackedJobs", strconv.Itoa(cfg.Scheduler.MaxTrackedJobs))
	write("maxPayloadBytes", strconv.Itoa(cfg.Scheduler.MaxPayloadBytes))
	write("defaultExecutionTimeoutMs", strconv.Itoa(cfg.Scheduler.DefaultExecutionTimeoutMs))
	write("shutdownGracePeriodMs", strconv.Itoa(cfg.Scheduler.ShutdownGracePeriodMs))
	write("retryMaxAttempts", strconv.Itoa(cfg.Scheduler.RetryMaxAttempts))
	write("retryInitialBackoffMs", strconv.Itoa(cfg.Scheduler.RetryInitialBackoffMs))
	write("retryMaxBackoffMs", strconv.Itoa(cfg.Scheduler.RetryMaxBackoffMs))
	write("retryJitterBasisPoints", strconv.Itoa(cfg.Scheduler.RetryJitterBasisPoints))

	names := make([]string, 0, len(cfg.Scheduler.Pools))
	for name := range cfg.Scheduler.Pools {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pool := cfg.Scheduler.Pools[name]
		write("pool", name)
		write("workers", strconv.Itoa(pool.Workers))
		write("queueDepth", strconv.Itoa(pool.QueueDepth))
	}

	write("requestTimeoutMs", strconv.Itoa(cfg.Transport.RequestTimeoutMs))
	write("maxRequestBytes", strconv.Itoa(cfg.Transport.MaxRequestBytes))
	write("maxResponseBytes", strconv.Itoa(cfg.Transport.MaxResponseBytes))
	write("uploadBatchRecords", strconv.Itoa(cfg.Transport.UploadBatchRecords))
	write("uploadBatchBytes", strconv.Itoa(cfg.Transport.UploadBatchBytes))

	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeLengthPrefixed(h interface{ Write([]byte) (int, error) }, value string) {
	b := []byte(value)
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(b)))
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write(b)
}

// GoldenConfiguration returns the Java SensorControlDesiredChecksumTest golden fixture.
func GoldenConfiguration() DesiredConfiguration {
	return DesiredConfiguration{
		LogLevel: "INFO",
		Scheduler: DesiredScheduler{
			MaxWorkers:                4,
			MaxQueueDepth:             16,
			MaxTrackedAssignments:     32,
			MaxTrackedJobs:            64,
			MaxPayloadBytes:           1_048_576,
			DefaultExecutionTimeoutMs: 5_000,
			ShutdownGracePeriodMs:     10_000,
			RetryMaxAttempts:          3,
			RetryInitialBackoffMs:     100,
			RetryMaxBackoffMs:         1_000,
			RetryJitterBasisPoints:    500,
			Pools: map[string]DesiredPool{
				"SYNTHETIC":    {Workers: 1, QueueDepth: 4},
				"NETWORK_POLL": {Workers: 2, QueueDepth: 8},
			},
		},
		Transport: DesiredTransport{
			RequestTimeoutMs:   15_000,
			MaxRequestBytes:    1_048_576,
			MaxResponseBytes:   1_048_576,
			UploadBatchRecords: 100,
			UploadBatchBytes:   524_288,
		},
	}
}
