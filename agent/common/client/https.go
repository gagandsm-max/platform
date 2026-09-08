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

type DeliveryEnvelope struct {
	SchemaVersion int              `json:"schemaVersion"`
	AgentID       string           `json:"agentId"`
	TenantID      string           `json:"tenantId"`
	SiteID        string           `json:"siteId,omitempty"`
	CorrelationID string           `json:"correlationId"`
	BatchID       string           `json:"batchId"`
	FirstSequence uint64           `json:"firstSequence"`
	LastSequence  uint64           `json:"lastSequence"`
	RecordCount   int              `json:"recordCount"`
	Records       []DeliveryRecord `json:"records"`
}

type DeliveryAck struct {
	Status                  string `json:"status"`
	AcceptedThroughSequence uint64 `json:"accepted_through_sequence"`
	AgentID                 string `json:"agent_id"`
	CorrelationID           string `json:"correlation_id"`
}

type ErrorResponse struct {
	Status  string `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// HTTPSConfig parameterizes the concrete mTLS transport for Agent-to-Sensor telemetry delivery.
type HTTPSConfig struct {
	SensorURL        string
	AgentID          string
	TenantID         string
	SiteID           string
	CACertFile       string
	ClientCertFile   string
	ClientKeyFile    string
	TLSConfig        *tls.Config // Optional pre-configured TLS config (e.g. for test harnesses)
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
func NewHTTPSTransport(cfg HTTPSConfig) (*HTTPSTransport, error) {
	if strings.TrimSpace(cfg.SensorURL) == "" {
		return nil, errors.New("DELIVERY_CONFIG_INVALID: SensorURL must be specified")
	}
	u, err := url.Parse(cfg.SensorURL)
	if err != nil {
		return nil, fmt.Errorf("DELIVERY_URL_INVALID: failed to parse sensor URL: %w", err)
	}
	if strings.ToLower(u.Scheme) != "https" {
		return nil, fmt.Errorf("DELIVERY_SCHEME_INSECURE: SensorURL scheme must be https, got %q", u.Scheme)
	}
	if u.User != nil {
		return nil, errors.New("DELIVERY_URL_INSECURE: credentials in SensorURL are strictly prohibited")
	}

	if strings.TrimSpace(cfg.AgentID) == "" {
		return nil, errors.New("DELIVERY_CONFIG_INVALID: AgentID must not be empty")
	}
	if strings.TrimSpace(cfg.TenantID) == "" {
		return nil, errors.New("DELIVERY_CONFIG_INVALID: TenantID must not be empty")
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = 1024 * 1024 // 1 MB
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	tlsConfig := cfg.TLSConfig
	if tlsConfig == nil {
		var loadErr error
		tlsConfig, loadErr = buildTLSConfig(cfg.CACertFile, cfg.ClientCertFile, cfg.ClientKeyFile)
		if loadErr != nil {
			return nil, loadErr
		}
	}

	transport := &http.Transport{
		TLSClientConfig:       tlsConfig,
		ResponseHeaderTimeout: cfg.Timeout,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   5,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout + (2 * time.Second),
	}

	endpoint := strings.TrimRight(cfg.SensorURL, "/") + "/api/v1/agent/telemetry"

	return &HTTPSTransport{
		cfg:        cfg,
		httpClient: client,
		endpoint:   endpoint,
		logger:     cfg.Logger.With("component", "agent-https-transport", "sensor_url", cfg.SensorURL),
	}, nil
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
	batchChecksum := computeBatchChecksum(recs)
	batchID := fmt.Sprintf("batch-%s-%020d-%020d-%s", t.cfg.AgentID, firstSeq, lastSeq, batchChecksum[:16])

	env := DeliveryEnvelope{
		SchemaVersion: 1,
		AgentID:       t.cfg.AgentID,
		TenantID:      t.cfg.TenantID,
		SiteID:        t.cfg.SiteID,
		CorrelationID: corrID,
		BatchID:       batchID,
		FirstSequence: firstSeq,
		LastSequence:  lastSeq,
		RecordCount:   len(batch.Records),
		Records:       recs,
	}

	bodyBytes, err := json.Marshal(env)
	if err != nil {
		return Ack{}, &DeliveryError{Code: "DELIVERY_SERIALIZE_FAILED", Class: FailurePermanent, Err: err}
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
		return Ack{}, &DeliveryError{Code: "DELIVERY_NETWORK_ERROR", Class: FailureTemporary, Err: err}
	}
	defer resp.Body.Close()

	// 4. Read Response Bounded
	limitedReader := io.LimitReader(resp.Body, t.cfg.MaxResponseBytes)
	respBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return Ack{}, &DeliveryError{Code: "DELIVERY_RESPONSE_READ_FAILED", Class: FailureTemporary, Err: err}
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

		if ack.AgentID != "" && !strings.EqualFold(ack.AgentID, t.cfg.AgentID) {
			return Ack{}, &DeliveryError{
				Code:  "DELIVERY_ACK_AGENT_MISMATCH",
				Class: FailurePermanent,
				Err:   fmt.Errorf("sensor ACK agent_id %q does not match configured %q", ack.AgentID, t.cfg.AgentID),
			}
		}

		return Ack{AcceptedThrough: ack.AcceptedThroughSequence}, nil

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
