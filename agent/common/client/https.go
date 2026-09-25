package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"platform/agent/common/queue"
)

const (
	// SensorReceiverMaxBodyBytes matches sensor agentreceiver.DefaultMaxBodyBytes.
	SensorReceiverMaxBodyBytes  = 512 * 1024
	errCodeRequestTooLarge      = "ERR_REQUEST_TOO_LARGE"
	errCodeRecordTooLarge       = "ERR_RECORD_TOO_LARGE"
	errCodeBatchTooLarge        = "ERR_BATCH_TOO_LARGE"
	errDeliveryEnvelopeTooLarge = "DELIVERY_ENVELOPE_TOO_LARGE"
	errDeliveryRequestTooLarge  = "DELIVERY_REQUEST_TOO_LARGE"
)

// Wire models matching the authoritative Sensor Agent receiver contract
type DeliveryRecord struct {
	Sequence  uint64          `json:"sequence"`
	ID        string          `json:"id"`
	CreatedAt time.Time       `json:"createdAt"`
	Checksum  string          `json:"checksum"`
	Payload   json.RawMessage `json:"payload"`
}

func computeBatchChecksum(records []DeliveryRecord) string {
	h := sha256.New()
	for _, r := range records {
		_, _ = fmt.Fprintf(h, "%d\n%s\n%s\n", r.Sequence, r.ID, r.Checksum)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// BuildLegacyBatchID returns the deterministic v1 batch identity used on the wire.
// Algorithm matches historical HTTPSTransport batch ID construction for wire compatibility.
func BuildLegacyBatchID(agentID string, records []queue.Record) string {
	if len(records) == 0 {
		return ""
	}
	recs := make([]DeliveryRecord, len(records))
	for i, r := range records {
		recs[i] = DeliveryRecord{
			Sequence:  r.Sequence,
			ID:        r.ID,
			CreatedAt: r.CreatedAt,
			Checksum:  r.Checksum,
			Payload:   json.RawMessage(r.Payload),
		}
	}
	firstSeq := records[0].Sequence
	lastSeq := records[len(records)-1].Sequence
	batchChecksum := computeBatchChecksum(recs)
	return fmt.Sprintf("batch-%s-%020d-%020d-%s", agentID, firstSeq, lastSeq, batchChecksum[:16])
}

// BuildBatchID is an alias for BuildLegacyBatchID retained for callers and tests.
func BuildBatchID(agentID string, records []queue.Record) string {
	return BuildLegacyBatchID(agentID, records)
}

// BuildLineageBatchID returns the v2 stream-scoped batch identity.
func BuildLineageBatchID(streamID, agentID string, records []queue.Record) string {
	if len(records) == 0 || strings.TrimSpace(streamID) == "" || strings.TrimSpace(agentID) == "" {
		return ""
	}
	recs := make([]DeliveryRecord, len(records))
	for i, r := range records {
		recs[i] = DeliveryRecord{
			Sequence:  r.Sequence,
			ID:        r.ID,
			CreatedAt: r.CreatedAt,
			Checksum:  r.Checksum,
			Payload:   json.RawMessage(r.Payload),
		}
	}
	firstSeq := records[0].Sequence
	lastSeq := records[len(records)-1].Sequence
	batchChecksum := computeBatchChecksum(recs)
	return fmt.Sprintf("batch-%s-%s-%020d-%020d-%s", streamID, agentID, firstSeq, lastSeq, batchChecksum[:16])
}

type DeliveryEnvelope struct {
	SchemaVersion int              `json:"schemaVersion"`
	AgentID       string           `json:"agentId"`
	TenantID      string           `json:"tenantId"`
	SiteID        string           `json:"siteId,omitempty"`
	CorrelationID string           `json:"correlationId"`
	BatchID       string           `json:"batchId"`
	AgentStreamID string           `json:"agentStreamId,omitempty"`
	FirstSequence uint64           `json:"firstSequence"`
	LastSequence  uint64           `json:"lastSequence"`
	RecordCount   int              `json:"recordCount"`
	Records       []DeliveryRecord `json:"records"`
}

type DeliveryAck struct {
	Status                  string `json:"status"`
	AcceptedThroughSequence uint64 `json:"accepted_through_sequence"`
	AgentID                 string `json:"agent_id"`
	BatchID                 string `json:"batch_id"`
	CorrelationID           string `json:"correlation_id"`
}

type ErrorResponse struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// HTTPSConfig parameterizes the concrete mTLS transport for Agent-to-Sensor telemetry delivery.
// Production TLS is built only from CA/client certificate files; caller-supplied *tls.Config
// is not accepted on the production constructor.
type HTTPSConfig struct {
	SensorURL        string
	AgentID          string
	TenantID         string
	SiteID           string
	CACertFile       string
	ClientCertFile   string
	ClientKeyFile    string
	Timeout          time.Duration
	MaxResponseBytes int64
	Logger           *slog.Logger
}

// HTTPSTransport is the concrete HTTPS+mTLS Transport delivering batches to an Infrastructure Sensor.
type HTTPSTransport struct {
	cfg        HTTPSConfig
	httpClient *http.Client
	endpoint   string
	logger     *slog.Logger
}

// NewHTTPSTransport constructs a validated HTTPS mTLS transport.
// TLS is built exclusively via buildTLSConfig (TLS ≥ 1.2, verify, CA + client cert required).
func NewHTTPSTransport(cfg HTTPSConfig) (*HTTPSTransport, error) {
	if err := validateHTTPSConfigBasics(cfg); err != nil {
		return nil, err
	}
	tlsConfig, err := buildTLSConfig(cfg.CACertFile, cfg.ClientCertFile, cfg.ClientKeyFile)
	if err != nil {
		return nil, err
	}
	return newHTTPSTransport(cfg, tlsConfig)
}

// NewHTTPSTransportForTest is a TEST-ONLY seam that accepts an explicit *tls.Config.
// The config is cloned and validated by enforceTLSConstraints (rejects InsecureSkipVerify,
// MinVersion < TLS1.2 when set, missing RootCAs, missing client Certificates; forces MinVersion ≥ TLS1.2).
func NewHTTPSTransportForTest(cfg HTTPSConfig, tlsConfig *tls.Config) (*HTTPSTransport, error) {
	if err := validateHTTPSConfigBasics(cfg); err != nil {
		return nil, err
	}
	if tlsConfig == nil {
		return nil, errors.New("DELIVERY_TLS_MISSING: test TLS config must not be nil")
	}
	secured, err := enforceTLSConstraints(tlsConfig)
	if err != nil {
		return nil, err
	}
	return newHTTPSTransport(cfg, secured)
}

func validateHTTPSConfigBasics(cfg HTTPSConfig) error {
	if strings.TrimSpace(cfg.SensorURL) == "" {
		return errors.New("DELIVERY_CONFIG_INVALID: SensorURL must be specified")
	}
	u, err := url.Parse(cfg.SensorURL)
	if err != nil {
		return fmt.Errorf("DELIVERY_URL_INVALID: failed to parse sensor URL: %w", err)
	}
	if strings.ToLower(u.Scheme) != "https" {
		return fmt.Errorf("DELIVERY_SCHEME_INSECURE: SensorURL scheme must be https, got %q", u.Scheme)
	}
	if u.User != nil {
		return errors.New("DELIVERY_URL_INSECURE: credentials in SensorURL are strictly prohibited")
	}
	if strings.TrimSpace(cfg.AgentID) == "" {
		return errors.New("DELIVERY_CONFIG_INVALID: AgentID must not be empty")
	}
	if strings.TrimSpace(cfg.TenantID) == "" {
		return errors.New("DELIVERY_CONFIG_INVALID: TenantID must not be empty")
	}
	return nil
}

func newHTTPSTransport(cfg HTTPSConfig, tlsConfig *tls.Config) (*HTTPSTransport, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = 1024 * 1024 // 1 MB
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	transport := &http.Transport{
		Proxy:                 noProxy,
		TLSClientConfig:       tlsConfig,
		ResponseHeaderTimeout: cfg.Timeout,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   5,
	}

	httpClient := &http.Client{
		Transport:     transport,
		Timeout:       cfg.Timeout + (2 * time.Second),
		CheckRedirect: denyHTTPRedirect,
	}

	endpoint := strings.TrimRight(cfg.SensorURL, "/") + "/api/v1/agent/telemetry"

	return &HTTPSTransport{
		cfg:        cfg,
		httpClient: httpClient,
		endpoint:   endpoint,
		logger:     cfg.Logger.With("component", "agent-https-transport", "sensor_url", cfg.SensorURL),
	}, nil
}

var errRedirectForbidden = errors.New("DELIVERY_REDIRECT_FORBIDDEN: agent telemetry POST must not follow HTTP redirects")

func noProxy(*http.Request) (*url.URL, error) {
	return nil, nil
}

func denyHTTPRedirect(*http.Request, []*http.Request) error {
	return errRedirectForbidden
}

// enforceTLSConstraints clones cfg and rejects insecure or incomplete TLS material.
// Always forces MinVersion >= tls.VersionTLS12.
func enforceTLSConstraints(cfg *tls.Config) (*tls.Config, error) {
	if cfg == nil {
		return nil, errors.New("DELIVERY_TLS_INVALID: tls config is nil")
	}
	clone := cfg.Clone()
	if clone.InsecureSkipVerify {
		return nil, errors.New("DELIVERY_TLS_INSECURE: InsecureSkipVerify is prohibited")
	}
	if clone.MinVersion != 0 && clone.MinVersion < tls.VersionTLS12 {
		return nil, fmt.Errorf("DELIVERY_TLS_INSECURE: MinVersion %#x is below TLS 1.2", clone.MinVersion)
	}
	if clone.RootCAs == nil {
		return nil, errors.New("DELIVERY_TLS_MISSING: RootCAs must be configured")
	}
	if len(clone.Certificates) == 0 {
		return nil, errors.New("DELIVERY_TLS_MISSING: client Certificates must be configured")
	}
	clone.MinVersion = tls.VersionTLS12
	clone.InsecureSkipVerify = false
	return clone, nil
}

func buildTLSConfig(caFile, certFile, keyFile string) (*tls.Config, error) {
	if caFile == "" || certFile == "" || keyFile == "" {
		return nil, errors.New("DELIVERY_TLS_MISSING: caFile, certFile, and keyFile must all be provided")
	}

	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("DELIVERY_CA_READ_FAILED: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("DELIVERY_CA_PARSE_FAILED: no certificates could be parsed from CA file")
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("DELIVERY_CERT_LOAD_FAILED: %w", err)
	}

	return &tls.Config{
		RootCAs:      caPool,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// Available returns true for an initialized concrete HTTPS transport.
func (t *HTTPSTransport) Available() bool {
	return true
}

// Deliver sends a batch of queue records to the Sensor receiver and returns an application ACK.
// Any authentication, transport, or server errors retain the local queue records.
func (t *HTTPSTransport) Deliver(ctx context.Context, batch Batch) (Ack, error) {
	if len(batch.Records) == 0 {
		return Ack{}, nil
	}
	if strings.TrimSpace(batch.AgentID) == "" {
		return Ack{}, &DeliveryError{Code: "DELIVERY_BATCH_AGENT_REQUIRED", Class: FailurePermanent, Err: errors.New("batch AgentID must be set before delivery")}
	}
	if strings.TrimSpace(batch.BatchID) == "" {
		return Ack{}, &DeliveryError{Code: "DELIVERY_BATCH_ID_REQUIRED", Class: FailurePermanent, Err: errors.New("batch BatchID must be set before delivery")}
	}
	if batch.AgentID != t.cfg.AgentID {
		return Ack{}, &DeliveryError{
			Code:  "DELIVERY_BATCH_AGENT_MISMATCH",
			Class: FailurePermanent,
			Err:   fmt.Errorf("batch AgentID %q does not match transport configured AgentID %q", batch.AgentID, t.cfg.AgentID),
		}
	}

	var schemaVersion int
	var agentStreamID string
	switch batch.SchemaVersion {
	case 1:
		if strings.TrimSpace(batch.AgentStreamID) != "" {
			return Ack{}, &DeliveryError{
				Code:  "DELIVERY_BATCH_STREAM_FORBIDDEN",
				Class: FailurePermanent,
				Err:   errors.New("schemaVersion 1 must not include agentStreamId"),
			}
		}
		schemaVersion = 1
		agentStreamID = ""
	case 2:
		streamID := strings.TrimSpace(batch.AgentStreamID)
		if streamID == "" {
			return Ack{}, &DeliveryError{
				Code:  "DELIVERY_BATCH_STREAM_REQUIRED",
				Class: FailurePermanent,
				Err:   errors.New("schemaVersion 2 requires agentStreamId"),
			}
		}
		schemaVersion = 2
		agentStreamID = streamID
	default:
		return Ack{}, &DeliveryError{
			Code:  "DELIVERY_BATCH_SCHEMA_UNSUPPORTED",
			Class: FailurePermanent,
			Err:   fmt.Errorf("unsupported batch SchemaVersion %d", batch.SchemaVersion),
		}
	}

	// 1. Build Versioned Delivery Envelope
	recs := make([]DeliveryRecord, len(batch.Records))
	for i, r := range batch.Records {
		recs[i] = DeliveryRecord{
			Sequence:  r.Sequence,
			ID:        r.ID,
			CreatedAt: r.CreatedAt,
			Checksum:  r.Checksum,
			Payload:   json.RawMessage(r.Payload),
		}
	}

	corrID := batch.CorrelationID
	if corrID == "" {
		var genErr error
		corrID, genErr = CorrelationID()
		if genErr != nil {
			return Ack{}, &DeliveryError{Code: "DELIVERY_CORRELATION_ID_FAILED", Class: FailureTemporary, Err: genErr}
		}
	}

	firstSeq := batch.Records[0].Sequence
	lastSeq := batch.Records[len(batch.Records)-1].Sequence
	batchID := batch.BatchID

	env := DeliveryEnvelope{
		SchemaVersion: schemaVersion,
		AgentID:       batch.AgentID,
		TenantID:      t.cfg.TenantID,
		SiteID:        t.cfg.SiteID,
		CorrelationID: corrID,
		BatchID:       batchID,
		AgentStreamID: agentStreamID,
		FirstSequence: firstSeq,
		LastSequence:  lastSeq,
		RecordCount:   len(batch.Records),
		Records:       recs,
	}

	bodyBytes, err := json.Marshal(env)
	if err != nil {
		return Ack{}, &DeliveryError{Code: "DELIVERY_SERIALIZE_FAILED", Class: FailurePermanent, Err: err}
	}
	if int64(len(bodyBytes)) > SensorReceiverMaxBodyBytes {
		return Ack{}, &DeliveryError{
			Code:  errDeliveryEnvelopeTooLarge,
			Class: FailureOversized,
			Err:   fmt.Errorf("serialized Agent envelope is %d bytes; Sensor receiver maximum is %d", len(bodyBytes), SensorReceiverMaxBodyBytes),
		}
	}

	// 2. Formulate HTTP POST Request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return Ack{}, &DeliveryError{Code: "DELIVERY_REQUEST_FAILED", Class: FailurePermanent, Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", corrID)

	// 3. Execute HTTPS Roundtrip
	resp, err := t.httpClient.Do(req)
	if err != nil {
		// Classify context cancellation / deadline vs network error
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Ack{}, &DeliveryError{Code: "DELIVERY_CONTEXT_ABORTED", Class: FailureTemporary, Err: ctx.Err()}
		}
		if errors.Is(err, errRedirectForbidden) {
			return Ack{}, &DeliveryError{Code: "DELIVERY_REDIRECT_FORBIDDEN", Class: FailurePermanent, Err: err}
		}
		return Ack{}, &DeliveryError{Code: "DELIVERY_NETWORK_ERROR", Class: FailureTemporary, Err: err}
	}
	defer resp.Body.Close()

	// 4. Read Response Bounded; detect truncation when LimitReader hits max
	limited := &io.LimitedReader{R: resp.Body, N: t.cfg.MaxResponseBytes}
	respBytes, err := io.ReadAll(limited)
	if err != nil {
		return Ack{}, &DeliveryError{Code: "DELIVERY_RESPONSE_READ_FAILED", Class: FailureTemporary, Err: err}
	}
	if limited.N == 0 {
		var one [1]byte
		n, readErr := resp.Body.Read(one[:])
		if n > 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
			return Ack{}, &DeliveryError{
				Code:  "DELIVERY_RESPONSE_TRUNCATED",
				Class: FailureTemporary,
				Err:   errors.New("response exceeded MaxResponseBytes"),
			}
		}
	}

	// 5. Evaluate HTTP Status Codes
	switch {
	case resp.StatusCode == http.StatusOK:
		var ack DeliveryAck
		dec := json.NewDecoder(bytes.NewReader(respBytes))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&ack); err != nil {
			return Ack{}, &DeliveryError{
				Code:  "DELIVERY_ACK_MALFORMED",
				Class: FailurePermanent,
				Err:   fmt.Errorf("failed to decode sensor ACK JSON: %w", err),
			}
		}
		if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return Ack{}, &DeliveryError{
				Code:  "DELIVERY_ACK_TRAILING_DATA",
				Class: FailurePermanent,
				Err:   errors.New("extraneous data after ACK JSON object"),
			}
		}

		if ack.Status != "ACCEPTED" {
			return Ack{}, &DeliveryError{
				Code:  "DELIVERY_ACK_REJECTED",
				Class: FailurePermanent,
				Err:   fmt.Errorf("sensor returned unexpected ack status: %q", ack.Status),
			}
		}

		if ack.AcceptedThroughSequence == 0 {
			return Ack{}, &DeliveryError{
				Code:  "DELIVERY_ACK_ZERO_WATERMARK",
				Class: FailurePermanent,
				Err:   errors.New("sensor returned invalid zero watermark in ACK"),
			}
		}

		if ack.BatchID != batchID {
			return Ack{}, &DeliveryError{
				Code:  "DELIVERY_ACK_BATCH_MISMATCH",
				Class: FailurePermanent,
				Err:   fmt.Errorf("sensor ACK batch_id %q does not match submitted %q", ack.BatchID, batchID),
			}
		}

		if ack.AgentID != batch.AgentID {
			return Ack{}, &DeliveryError{
				Code:  "DELIVERY_ACK_AGENT_MISMATCH",
				Class: FailurePermanent,
				Err:   fmt.Errorf("sensor ACK agent_id %q does not match submitted %q", ack.AgentID, batch.AgentID),
			}
		}

		if ack.CorrelationID != corrID {
			return Ack{}, &DeliveryError{
				Code:  "DELIVERY_ACK_CORRELATION_MISMATCH",
				Class: FailurePermanent,
				Err:   fmt.Errorf("sensor ACK correlation_id %q does not match submitted %q", ack.CorrelationID, corrID),
			}
		}

		return Ack{
			AcceptedThrough: ack.AcceptedThroughSequence,
			AgentID:         ack.AgentID,
			BatchID:         ack.BatchID,
			CorrelationID:   ack.CorrelationID,
		}, nil

	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		var errResp ErrorResponse
		_ = json.Unmarshal(respBytes, &errResp)
		code := "DELIVERY_AUTH_REJECTED"
		if errResp.Code != "" {
			code = errResp.Code
		}
		return Ack{}, &DeliveryError{
			Code:  code,
			Class: FailureAuthentication,
			Err:   fmt.Errorf("sensor rejected delivery (HTTP %d): %s", resp.StatusCode, errResp.Message),
		}

	case resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusUnprocessableEntity:
		var errResp ErrorResponse
		_ = json.Unmarshal(respBytes, &errResp)
		code := "DELIVERY_CLIENT_ERROR"
		if errResp.Code != "" {
			code = errResp.Code
		}
		return Ack{}, &DeliveryError{
			Code:  code,
			Class: FailurePermanent,
			Err:   fmt.Errorf("sensor rejected delivery as invalid (HTTP %d): %s", resp.StatusCode, errResp.Message),
		}

	case resp.StatusCode == http.StatusRequestEntityTooLarge:
		var errResp ErrorResponse
		_ = json.Unmarshal(respBytes, &errResp)
		code := errDeliveryRequestTooLarge
		if errResp.Code != "" {
			code = errResp.Code
		}
		class := FailureOversized
		switch code {
		case errCodeRequestTooLarge, errCodeRecordTooLarge, errCodeBatchTooLarge, errDeliveryRequestTooLarge:
			class = FailureOversized
		}
		return Ack{}, &DeliveryError{
			Code:  code,
			Class: class,
			Err:   fmt.Errorf("sensor rejected oversized delivery (HTTP 413): %s", errResp.Message),
		}

	case resp.StatusCode >= 500:
		var errResp ErrorResponse
		_ = json.Unmarshal(respBytes, &errResp)
		return Ack{}, &DeliveryError{
			Code:  "DELIVERY_SERVER_ERROR",
			Class: FailureTemporary,
			Err:   fmt.Errorf("sensor internal error (HTTP %d): %s", resp.StatusCode, errResp.Message),
		}

	default:
		return Ack{}, &DeliveryError{
			Code:  "DELIVERY_UNEXPECTED_HTTP_STATUS",
			Class: FailureTemporary,
			Err:   fmt.Errorf("unexpected HTTP status code: %d", resp.StatusCode),
		}
	}
}
