package agentdrain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"platform/sensor/internal/coreclient"
	"platform/sensor/internal/spool"
)

const (
	agentBatchRecordType = "agent.delivery.batch"
	poisonDirName        = "agent-poison"
)

type ErrorCode string

const (
	CodeInvalidConfig ErrorCode = "AGENTDRAIN_INVALID_CONFIG"
	CodeSpool         ErrorCode = "AGENTDRAIN_SPOOL"
	CodeTransport     ErrorCode = "AGENTDRAIN_TRANSPORT"
	CodeAckInvalid    ErrorCode = "AGENTDRAIN_ACK_INVALID"
	CodePoison        ErrorCode = "AGENTDRAIN_POISON"
	CodeCancelled     ErrorCode = "AGENTDRAIN_CANCELLED"
	CodeDeadline      ErrorCode = "AGENTDRAIN_DEADLINE_EXCEEDED"
)

type Error struct {
	Code      ErrorCode
	msg       string
	cause     error
	transient bool
}

func (e *Error) Error() string   { return string(e.Code) + ": " + e.msg }
func (e *Error) Unwrap() error   { return e.cause }
func (e *Error) Transient() bool { return e != nil && e.transient }

func codeErr(code ErrorCode, msg string, cause error, transient bool) error {
	return &Error{Code: code, msg: msg, cause: cause, transient: transient}
}

// durableBatchPayload mirrors agentreceiver.durableBatchPayload for decode-only use.
type durableBatchPayload struct {
	TenantID      string          `json:"tenantId"`
	AgentID       string          `json:"agentId"`
	SensorID      string          `json:"sensorId"`
	Generation    int64           `json:"assignmentGeneration"`
	FirstSequence uint64          `json:"firstSequence"`
	LastSequence  uint64          `json:"lastSequence"`
	RecordIDs     []string        `json:"recordIds"`
	BatchChecksum string          `json:"batchChecksum"`
	BatchID       string          `json:"batchId"`
	CorrelationID string          `json:"correlationId"`
	Timestamp     time.Time       `json:"timestamp"`
	Envelope      json.RawMessage `json:"envelope"`
}

type envelopeLite struct {
	Records json.RawMessage `json:"records"`
}

type Config struct {
	SensorID             string
	AgentSpoolDir        string
	MaxBatchRecords      int
	MaxBatchPayloadBytes int
	MaxRequestBytes      int
	RequestTimeout       time.Duration
	InitialBackoff       time.Duration
	MaxBackoff           time.Duration
	JitterFraction       float64
	IdleInterval         time.Duration
	ShutdownGrace        time.Duration
	Logger               *slog.Logger
}

type Transport interface {
	UploadAgentTelemetry(context.Context, coreclient.AgentTelemetryBatch) (coreclient.AgentTelemetryAck, error)
}

type Spool interface {
	ReadBatch(int, int) (spool.Batch, error)
	AcknowledgeThrough(uint64) error
	Stats() (spool.Stats, error)
}

type Metrics struct {
	PendingRecords   atomic.Int64
	PendingBytes     atomic.Int64
	OldestAgeSeconds atomic.Int64
	Attempts         atomic.Uint64
	Successes        atomic.Uint64
	FailuresTransient atomic.Uint64
	FailuresPermanent atomic.Uint64
	DurableAcks      atomic.Uint64
	PoisonCount      atomic.Uint64
	Backpressure     atomic.Uint64
	LastSuccessUnix  atomic.Int64
}

func (m *Metrics) LastSuccessAge() time.Duration {
	sec := m.LastSuccessUnix.Load()
	if sec == 0 {
		return 0
	}
	return time.Since(time.Unix(sec, 0))
}

type Drain struct {
	cfg      Config
	spool    Spool
	transport Transport
	metrics  *Metrics
	logger   *slog.Logger
	poisonDir string

	mu          sync.Mutex
	cancel      context.CancelFunc
	started     bool
	done        chan struct{}
	startOnce   sync.Once
	stopOnce    sync.Once
	wait        func(context.Context, time.Duration) error
	jitter      func(time.Duration, float64) time.Duration
}

func New(cfg Config, sp Spool, tr Transport) (*Drain, error) {
	if strings.TrimSpace(cfg.SensorID) == "" || sp == nil || tr == nil {
		return nil, codeErr(CodeInvalidConfig, "sensor id, spool and transport are required", nil, false)
	}
	if cfg.MaxBatchRecords <= 0 {
		cfg.MaxBatchRecords = 32
	}
	if cfg.MaxBatchPayloadBytes <= 0 {
		cfg.MaxBatchPayloadBytes = 1024 * 1024
	}
	if cfg.MaxRequestBytes <= 0 {
		cfg.MaxRequestBytes = 2 * 1024 * 1024
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 15 * time.Second
	}
	if cfg.InitialBackoff <= 0 {
		cfg.InitialBackoff = 500 * time.Millisecond
	}
	if cfg.MaxBackoff < cfg.InitialBackoff {
		cfg.MaxBackoff = 30 * time.Second
	}
	if cfg.JitterFraction < 0 || cfg.JitterFraction > 1 {
		cfg.JitterFraction = 0.1
	}
	if cfg.IdleInterval <= 0 {
		cfg.IdleInterval = time.Second
	}
	if cfg.ShutdownGrace <= 0 {
		cfg.ShutdownGrace = 10 * time.Second
	}
	dir := filepath.Clean(strings.TrimSpace(cfg.AgentSpoolDir))
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, codeErr(CodeInvalidConfig, "agent spool directory must be absolute", nil, false)
	}
	poisonDir := filepath.Join(dir, poisonDirName)
	if err := os.MkdirAll(poisonDir, 0750); err != nil {
		return nil, codeErr(CodeInvalidConfig, "failed to create agent-poison directory", err, false)
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	d := &Drain{
		cfg:       cfg,
		spool:     sp,
		transport: tr,
		metrics:   &Metrics{},
		logger:    logger.With("component", "agentdrain"),
		poisonDir: poisonDir,
		done:      make(chan struct{}),
		wait:      waitContext,
		jitter:    jitterDuration,
	}
	return d, nil
}

func (d *Drain) Metrics() *Metrics { return d.metrics }

func (d *Drain) Start(parent context.Context) error {
	if parent == nil {
		return codeErr(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	startedNow := false
	d.startOnce.Do(func() {
		startedNow = true
		ctx, cancel := context.WithCancel(parent)
		d.mu.Lock()
		d.cancel = cancel
		d.started = true
		d.mu.Unlock()
		go d.loop(ctx)
	})
	if !startedNow {
		return codeErr(CodeInvalidConfig, "agent drain already started", nil, false)
	}
	return nil
}

func (d *Drain) loop(ctx context.Context) {
	defer close(d.done)
	for {
		_, err := d.RunOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		delay := d.cfg.IdleInterval
		if err != nil {
			var te *Error
			if errors.As(err, &te) && te.transient {
				delay = d.cfg.MaxBackoff
			}
		}
		if err := d.wait(ctx, delay); err != nil {
			return
		}
	}
}

func (d *Drain) RunOnce(ctx context.Context) (bool, error) {
	if err := ctxErr(ctx); err != nil {
		return false, err
	}
	d.refreshStats()
	batch, err := d.spool.ReadBatch(d.cfg.MaxBatchRecords, d.cfg.MaxBatchPayloadBytes)
	if err != nil {
		return false, codeErr(CodeSpool, "failed reading agent spool batch", err, false)
	}
	if len(batch.Records) == 0 {
		return false, nil
	}

	// Under spool protection (80%) or high FS use, keep draining so ACK→delete can free space.
	// Pausing here would wedge the Agent spool (uploader does not pause either). Observe only.
	if st, serr := d.spool.Stats(); serr == nil {
		if st.FilesystemUsedPct >= 95 || st.ProtectionState == spool.ProtectionTriggered {
			d.metrics.Backpressure.Add(1)
		}
	}

	req, err := d.makeRequest(batch)
	if err != nil {
		// Malformed durable payload is terminal for the head record — park it.
		if parkErr := d.parkHead(batch, "ERR_MALFORMED_DURABLE_PAYLOAD", err); parkErr != nil {
			return false, parkErr
		}
		return false, codeErr(CodePoison, "parked malformed durable agent batch", err, false)
	}

	uploadCtx, cancel := context.WithTimeout(ctx, d.cfg.RequestTimeout)
	defer cancel()
	d.metrics.Attempts.Add(1)
	ack, err := d.transport.UploadAgentTelemetry(uploadCtx, req)
	if err == nil {
		if aerr := d.spool.AcknowledgeThrough(ack.AcceptedThroughIngressSequence); aerr != nil {
			return false, codeErr(CodeSpool, "failed applying durable agent ACK to spool", aerr, false)
		}
		d.metrics.Successes.Add(1)
		d.metrics.DurableAcks.Add(1)
		d.metrics.LastSuccessUnix.Store(time.Now().Unix())
		d.refreshStats()
		return true, nil
	}

	if isTerminalCoreError(err) {
		code := terminalCode(err)
		if parkErr := d.parkHead(batch, code, err); parkErr != nil {
			return false, parkErr
		}
		d.metrics.FailuresPermanent.Add(1)
		return false, codeErr(CodePoison, "parked terminal agent-telemetry failure", err, false)
	}

	if isTransient(err) {
		d.metrics.FailuresTransient.Add(1)
		backoff := jitterDuration(d.cfg.InitialBackoff, d.cfg.JitterFraction)
		if backoff > d.cfg.MaxBackoff {
			backoff = d.cfg.MaxBackoff
		}
		_ = d.wait(ctx, backoff)
		return false, codeErr(CodeTransport, "transient Core agent-telemetry failure; spool retained", err, true)
	}

	d.metrics.FailuresPermanent.Add(1)
	return false, codeErr(CodeTransport, "permanent Core agent-telemetry failure; spool retained", err, false)
}

func (d *Drain) makeRequest(batch spool.Batch) (coreclient.AgentTelemetryBatch, error) {
	records := make([]coreclient.AgentIngressRecord, 0, len(batch.Records))
	h := sha256.New()
	for _, rec := range batch.Records {
		if rec.Type != agentBatchRecordType {
			return coreclient.AgentTelemetryBatch{}, fmt.Errorf("unexpected spool record type %q", rec.Type)
		}
		var payload durableBatchPayload
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			return coreclient.AgentTelemetryBatch{}, fmt.Errorf("corrupt durable agent payload: %w", err)
		}
		if strings.TrimSpace(payload.TenantID) == "" || strings.TrimSpace(payload.AgentID) == "" ||
			strings.TrimSpace(payload.SensorID) == "" || payload.Generation < 1 ||
			payload.FirstSequence == 0 || payload.LastSequence < payload.FirstSequence {
			return coreclient.AgentTelemetryBatch{}, fmt.Errorf("incomplete durable agent payload")
		}
		var env envelopeLite
		deliveryRecords := json.RawMessage("[]")
		if len(payload.Envelope) > 0 {
			if err := json.Unmarshal(payload.Envelope, &env); err == nil && len(env.Records) > 0 {
				deliveryRecords = env.Records
			}
		}
		acceptedAt := payload.Timestamp.UTC().Format(time.RFC3339Nano)
		ingress := coreclient.AgentIngressRecord{
			SensorIngressSequence: rec.Sequence,
			SensorIngressRecordID:  fmt.Sprintf("agent-spool-%d", rec.Sequence),
			TenantID:              payload.TenantID,
			AgentID:               payload.AgentID,
			AcceptingSensorID:     payload.SensorID,
			AssignmentGeneration:  payload.Generation,
			AgentFirstSequence:    payload.FirstSequence,
			AgentLastSequence:     payload.LastSequence,
			AgentBatchID:          payload.BatchID,
			CorrelationID:         payload.CorrelationID,
			AcceptedAt:            acceptedAt,
			Records:               deliveryRecords,
		}
		// Stable checksum shared with Core AgentIngestionValidator (scalar identity fields).
		var line = fmt.Sprintf("%d|%s|%d|%d|%d|%s|%s|%s",
			ingress.SensorIngressSequence, ingress.AgentBatchID,
			ingress.AgentFirstSequence, ingress.AgentLastSequence, ingress.AssignmentGeneration,
			ingress.TenantID, ingress.AgentID, ingress.AcceptingSensorID)
		var deliveries []struct {
			Sequence uint64 `json:"sequence"`
			ID       string `json:"id"`
			Checksum string `json:"checksum"`
		}
		_ = json.Unmarshal(deliveryRecords, &deliveries)
		for _, dr := range deliveries {
			line += fmt.Sprintf("|%d|%s|%s", dr.Sequence, dr.ID, dr.Checksum)
		}
		_, _ = h.Write([]byte(line))
		records = append(records, ingress)
	}
	checksum := hex.EncodeToString(h.Sum(nil))
	batchID := fmt.Sprintf("agent-ingress-%s-%020d-%020d-%s",
		d.cfg.SensorID, batch.FirstSequence, batch.LastSequence, checksum[:16])
	corr := fmt.Sprintf("agentdrain-%s", batchID)
	out := coreclient.AgentTelemetryBatch{
		SchemaVersion:        1,
		SensorID:             d.cfg.SensorID,
		BatchID:              batchID,
		FirstIngressSequence: batch.FirstSequence,
		LastIngressSequence:  batch.LastSequence,
		RecordCount:          len(records),
		Checksum:             checksum,
		CorrelationID:        corr,
		Records:              records,
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return coreclient.AgentTelemetryBatch{}, err
	}
	if len(encoded) > d.cfg.MaxRequestBytes {
		return coreclient.AgentTelemetryBatch{}, fmt.Errorf("serialized agent-telemetry batch exceeds MaxRequestBytes")
	}
	return out, nil
}

func (d *Drain) parkHead(batch spool.Batch, reason string, cause error) error {
	if len(batch.Records) == 0 {
		return nil
	}
	head := batch.Records[0]
	name := fmt.Sprintf("%020d-%s.json", head.Sequence, sanitizeReason(reason))
	path := filepath.Join(d.poisonDir, name)
	meta := map[string]any{
		"reason":        reason,
		"sequence":      head.Sequence,
		"type":          head.Type,
		"parkedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"cause":         fmt.Sprintf("%v", cause),
		"payloadBytes":  len(head.Payload),
	}
	metaBytes, _ := json.Marshal(meta)
	tmp, err := os.CreateTemp(d.poisonDir, ".poison-*.tmp")
	if err != nil {
		return codeErr(CodePoison, "failed creating poison temp file", err, false)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(metaBytes); err != nil {
		_ = tmp.Close()
		return codeErr(CodePoison, "failed writing poison metadata", err, false)
	}
	if _, err := tmp.Write([]byte("\n")); err != nil {
		_ = tmp.Close()
		return codeErr(CodePoison, "failed writing poison separator", err, false)
	}
	if _, err := tmp.Write(head.Payload); err != nil {
		_ = tmp.Close()
		return codeErr(CodePoison, "failed writing poison payload", err, false)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return codeErr(CodePoison, "failed syncing poison file", err, false)
	}
	if err := tmp.Close(); err != nil {
		return codeErr(CodePoison, "failed closing poison file", err, false)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return codeErr(CodePoison, "failed publishing poison file", err, false)
	}
	cleanup = false
	if err := d.spool.AcknowledgeThrough(head.Sequence); err != nil {
		return codeErr(CodeSpool, "failed removing parked record from spool", err, false)
	}
	d.metrics.PoisonCount.Add(1)
	d.logger.Warn("Parked terminal agent-ingress record",
		"reason", reason, "sequence", head.Sequence, "poison_path", path)
	return nil
}

func (d *Drain) refreshStats() {
	st, err := d.spool.Stats()
	if err != nil {
		return
	}
	d.metrics.PendingRecords.Store(int64(st.RecordCount))
	d.metrics.PendingBytes.Store(st.SpoolBytes)
	d.metrics.OldestAgeSeconds.Store(int64(st.OldestRecordAge.Seconds()))
}

func (d *Drain) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return codeErr(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	d.mu.Lock()
	started := d.started
	cancel := d.cancel
	d.mu.Unlock()
	if !started {
		return nil
	}
	d.stopOnce.Do(func() {
		if cancel != nil {
			cancel()
		}
	})
	select {
	case <-d.done:
		return nil
	case <-ctx.Done():
		return ctxErr(ctx)
	}
}

func isTerminalCoreError(err error) bool {
	if coreclient.IsCoreAPICode(err, "ERR_AGENT_ENTITY_UNRESOLVED") ||
		coreclient.IsCoreAPICode(err, "ERR_AGENT_ASSIGNMENT_UNKNOWN") ||
		coreclient.IsCoreAPICode(err, "ERR_AUTH_IDENTITY_MISMATCH") ||
		coreclient.IsCoreAPICode(err, "ERR_AGENT_ASSIGNMENT_SENSOR_MISMATCH") ||
		coreclient.IsCoreAPICode(err, "ERR_AGENT_ACCEPTING_SENSOR_MISMATCH") ||
		coreclient.IsCoreAPICode(err, "ERR_TENANT_SCOPE_MISMATCH") ||
		coreclient.IsCoreAPICode(err, "ERR_IDEMPOTENCY_CONFLICT") ||
		coreclient.IsCoreAPICode(err, "ERR_SEQUENCE_REGRESSION") ||
		coreclient.IsCoreAPICode(err, "ERR_MALFORMED_JSON") ||
		coreclient.IsCoreAPICode(err, "ERR_MALFORMED_RECORD") ||
		coreclient.IsCoreAPICode(err, "ERR_MALFORMED_RECORD_PAYLOAD") ||
		coreclient.IsCoreAPICode(err, "ERR_UNSUPPORTED_SCHEMA_VERSION") ||
		coreclient.IsCoreAPICode(err, "ERR_CHECKSUM_MISMATCH") ||
		coreclient.IsCoreAPICode(err, "ERR_AGENT_HOST_ID_REQUIRED") {
		return true
	}
	var api *coreclient.CoreAPIError
	if errors.As(err, &api) && !api.Transient() {
		switch api.HTTPStatus {
		case 400, 403, 409, 422:
			return true
		}
	}
	return false
}

func terminalCode(err error) string {
	var api *coreclient.CoreAPIError
	if errors.As(err, &api) && api.Code != "" {
		return api.Code
	}
	return "ERR_TERMINAL"
}

func isTransient(err error) bool {
	if coreclient.IsTransient(err) {
		return true
	}
	var t interface{ Transient() bool }
	return errors.As(err, &t) && t.Transient()
}

func sanitizeReason(reason string) string {
	r := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, reason)
	if len(r) > 64 {
		return r[:64]
	}
	return r
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return codeErr(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return codeErr(CodeDeadline, "agent drain deadline exceeded", err, false)
		}
		return codeErr(CodeCancelled, "agent drain cancelled", err, false)
	}
	return nil
}

func waitContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func jitterDuration(d time.Duration, f float64) time.Duration {
	if f <= 0 {
		return d
	}
	delta := float64(d) * f
	v := float64(d) - delta + rand.Float64()*2*delta
	if v < 0 {
		return 0
	}
	return time.Duration(v)
}
