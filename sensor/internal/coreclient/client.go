package coreclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	heartbeatPath = "/api/v1/sensor/heartbeat"
	statusPath    = "/api/v1/sensor/status"
	telemetryPath = "/api/v1/telemetry"
)

type Config struct {
	BaseURL          string
	CertificateFile  string
	PrivateKeyFile   string
	CAFile           string
	RequestTimeout   time.Duration
	MaxResponseBytes int
	Logger           *slog.Logger
}

type Client struct {
	baseURL          *url.URL
	httpClient       *http.Client
	logger           *slog.Logger
	maxResponseBytes int
	initErr          error
}

type CoreClient = Client

type HeartbeatRequest struct {
	SensorID               string   `json:"sensorId"`
	Version                string   `json:"version"`
	Role                   string   `json:"role"`
	State                  string   `json:"state"`
	Capabilities           []string `json:"capabilities,omitempty"`
	QueueBytes             int64    `json:"queueBytes,omitempty"`
	OldestQueuedAgeSeconds int64    `json:"oldestQueuedAgeSec,omitempty"`
	AssignmentVersion      uint64   `json:"assignmentVersion,omitempty"`
	PolicyVersionWatermark uint64   `json:"policyVersionWatermark,omitempty"`
	CorrelationID          string   `json:"-"`
}

type HeartbeatResponse struct {
	Status               string `json:"status"`
	Authenticated        bool   `json:"authenticated"`
	SensorIdentity       string `json:"sensor_identity"`
	DesiredConfigVersion uint64 `json:"desiredConfigVersion,omitempty"`
	AssignmentVersion    uint64 `json:"assignmentVersion,omitempty"`
	Drain                bool   `json:"drain,omitempty"`
}

type StatusResponse struct {
	Status         string `json:"status"`
	Authenticated  bool   `json:"authenticated"`
	SensorIdentity string `json:"sensor_identity"`
}

type Batch struct {
	SchemaVersion int               `json:"schemaVersion"`
	SensorID      string            `json:"sensorId"`
	BootEpoch     string            `json:"bootEpoch"`
	BatchID       string            `json:"batchId"`
	FirstSequence uint64            `json:"firstSequence"`
	LastSequence  uint64            `json:"lastSequence"`
	Checksum      string            `json:"checksum"`
	Records       []json.RawMessage `json:"records"`
	CorrelationID string            `json:"-"`
}

type UploadAck struct {
	Status                  string  `json:"status"`
	AcceptedThroughSequence *uint64 `json:"accepted_through_sequence,omitempty"`
	SensorIdentity          string  `json:"sensor_identity,omitempty"`
}

func New(cfg Config) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, newError(CodeInvalidConfig, "Core base URL must be an HTTPS origin without credentials/query/fragment", err, false)
	}
	if cfg.RequestTimeout <= 0 {
		return nil, newError(CodeInvalidConfig, "request timeout must be greater than zero", nil, false)
	}
	if cfg.MaxResponseBytes <= 0 {
		return nil, newError(CodeInvalidConfig, "maximum response bytes must be greater than zero", nil, false)
	}
	if cfg.CAFile == "" {
		return nil, newError(CodeTLSConfiguration, "Core CA certificate file is required", nil, false)
	}
	if cfg.CertificateFile == "" || cfg.PrivateKeyFile == "" {
		return nil, newError(CodeTLSConfiguration, "Sensor mTLS certificate and private key are required", nil, false)
	}

	caPEM, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		return nil, newError(CodeTLSConfiguration, "failed to read Core CA certificate", err, false)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		return nil, newError(CodeTLSConfiguration, "Core CA certificate file contains no usable certificate", nil, false)
	}
	cert, err := tls.LoadX509KeyPair(cfg.CertificateFile, cfg.PrivateKeyFile)
	if err != nil {
		return nil, newError(CodeTLSConfiguration, "failed to load Sensor mTLS certificate/key", err, false)
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{cert}}
	transport := &http.Transport{
		TLSClientConfig: tlsConfig, MaxIdleConns: 16, MaxIdleConnsPerHost: 8,
		IdleConnTimeout: 90 * time.Second, ForceAttemptHTTP2: true,
	}
	return &Client{
		baseURL: u, httpClient: &http.Client{Transport: transport, Timeout: cfg.RequestTimeout},
		logger: cfg.Logger, maxResponseBytes: cfg.MaxResponseBytes,
	}, nil
}

// NewCoreClient exists only to keep the pre-runtime bootstrap source buildable
// during migration. It deliberately fails closed instead of recreating the old
// insecure development fallback. Production wiring must use New with explicit
// bounds and checked error handling.
func NewCoreClient(baseURL, certFile, keyFile, caCertFile string, logger *slog.Logger) *CoreClient {
	return &Client{logger: logger, initErr: newError(CodeInvalidConfig,
		"legacy Core client constructor is disabled; use checked New(Config) with explicit request/response bounds", nil, false)}
}

func (c *Client) CheckConnectivity(ctx context.Context) (bool, error) {
	_, err := c.Status(ctx, "")
	return err == nil, err
}

func (c *Client) Status(ctx context.Context, correlationID string) (StatusResponse, error) {
	var out StatusResponse
	if err := c.doJSON(ctx, http.MethodGet, statusPath, nil, correlationID, "", &out); err != nil { return StatusResponse{}, err }
	return out, nil
}

func (c *Client) Heartbeat(ctx context.Context, sensorID, sensorType string) (bool, error) {
	_, err := c.SendHeartbeat(ctx, HeartbeatRequest{SensorID: sensorID, Role: sensorType, State: "UNKNOWN"})
	return err == nil, err
}

func (c *Client) SendHeartbeat(ctx context.Context, heartbeat HeartbeatRequest) (HeartbeatResponse, error) {
	if strings.TrimSpace(heartbeat.SensorID) == "" { return HeartbeatResponse{}, newError(CodeInvalidConfig, "Sensor ID is required for heartbeat", nil, false) }
	if strings.TrimSpace(heartbeat.Role) == "" { return HeartbeatResponse{}, newError(CodeInvalidConfig, "Sensor role is required for heartbeat", nil, false) }
	var out HeartbeatResponse
	if err := c.doJSON(ctx, http.MethodPost, heartbeatPath, heartbeat, heartbeat.CorrelationID, "", &out); err != nil { return HeartbeatResponse{}, err }
	return out, nil
}

func (c *Client) Upload(ctx context.Context, batch Batch) (UploadAck, error) {
	if batch.SchemaVersion <= 0 || batch.SensorID == "" || batch.BootEpoch == "" || batch.BatchID == "" ||
		batch.FirstSequence == 0 || batch.LastSequence < batch.FirstSequence || batch.Checksum == "" || len(batch.Records) == 0 {
		return UploadAck{}, newError(CodeInvalidConfig, "upload batch metadata is incomplete or invalid", nil, false)
	}
	var ack UploadAck
	if err := c.doJSON(ctx, http.MethodPost, telemetryPath, batch, batch.CorrelationID, batch.BatchID, &ack); err != nil { return UploadAck{}, err }
	if ack.AcceptedThroughSequence == nil {
		return ack, newError(CodeAckContractMissing,
			"Core accepted telemetry transport but did not return the durable accepted-through sequence contract", nil, false)
	}
	if *ack.AcceptedThroughSequence < batch.FirstSequence || *ack.AcceptedThroughSequence > batch.LastSequence {
		return ack, newError(CodeProtocol, "Core acknowledgement sequence is outside the submitted batch range", nil, false)
	}
	return ack, nil
}

// SendBatch is retained only for the pre-runtime demo collector to compile.
// Raw unsequenced payloads cannot satisfy the durable replay contract and are
// therefore rejected rather than falsely treated as delivered.
func (c *Client) SendBatch(context.Context, []byte) error {
	if c != nil && c.initErr != nil { return c.initErr }
	return newError(CodeAckContractMissing, "raw unsequenced telemetry upload is disabled; use Upload with replay metadata", nil, false)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, correlationID, idempotencyKey string, out any) error {
	if c == nil { return newError(CodeInvalidConfig, "Core client is nil", nil, false) }
	if c.initErr != nil { return c.initErr }
	if ctx == nil { return newError(CodeInvalidConfig, "context must not be nil", nil, false) }
	if err := contextClassified(ctx); err != nil { return err }
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(c.baseURL.Path, "/") + path

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil { return newError(CodeProtocol, "failed to encode Core request", err, false) }
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil { return newError(CodeProtocol, "failed to create Core request", err, false) }
	req.Header.Set("Accept", "application/json")
	if body != nil { req.Header.Set("Content-Type", "application/json") }
	if correlationID != "" { req.Header.Set("X-Correlation-ID", correlationID) }
	if idempotencyKey != "" { req.Header.Set("Idempotency-Key", idempotencyKey) }

	resp, err := c.httpClient.Do(req)
	if err != nil { return classifyTransport(ctx, err) }
	defer resp.Body.Close()
	responseBody, err := readBounded(resp.Body, c.maxResponseBytes)
	if err != nil { return err }
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return classifyStatus(resp.StatusCode) }
	if out == nil || len(bytes.TrimSpace(responseBody)) == 0 { return nil }
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		// Core responses may gain additive fields across compatible versions. A
		// narrow projection may ignore them, but malformed JSON is still rejected.
		if err := json.Unmarshal(responseBody, out); err != nil { return newError(CodeProtocol, "Core response is not valid JSON", err, false) }
	}
	return nil
}

func readBounded(reader io.Reader, maxBytes int) ([]byte, error) {
	limited := io.LimitReader(reader, int64(maxBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil { return nil, newError(CodeTransport, "failed reading Core response", err, true) }
	if len(data) > maxBytes { return nil, newError(CodeResponseTooLarge, "Core response exceeds configured maximum", nil, false) }
	return data, nil
}

func classifyStatus(status int) error {
	switch {
	case status == http.StatusUnauthorized:
		return newError(CodeAuthentication, "Core rejected Sensor authentication", nil, false)
	case status == http.StatusForbidden:
		return newError(CodeAuthorization, "Core denied Sensor operation", nil, false)
	case status == http.StatusNotFound:
		return newError(CodeNotFound, "Core endpoint is not available", nil, false)
	case status == http.StatusTooManyRequests:
		return newError(CodeRateLimited, "Core rate limited Sensor request", nil, true)
	case status >= 500:
		return newError(CodeServerUnavailable, "Core service is temporarily unavailable", nil, true)
	default:
		return newError(CodeProtocol, fmt.Sprintf("Core returned HTTP status %d", status), nil, false)
	}
}

func classifyTransport(ctx context.Context, err error) error {
	if classified := contextClassified(ctx); classified != nil { return classified }
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() { return newError(CodeTimeout, "Core request timed out", err, true) }
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if errors.As(urlErr.Err, &netErr) && netErr.Timeout() { return newError(CodeTimeout, "Core request timed out", err, true) }
	}
	return newError(CodeTransport, "Core transport request failed", err, true)
}

func contextClassified(ctx context.Context) error {
	if ctx == nil { return newError(CodeInvalidConfig, "context must not be nil", nil, false) }
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) { return newError(CodeDeadlineExceeded, "Core request deadline exceeded", context.DeadlineExceeded, false) }
		return newError(CodeCancelled, "Core request cancelled", context.Canceled, false)
	}
	return nil
}
