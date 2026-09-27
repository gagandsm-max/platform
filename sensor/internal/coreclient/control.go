package coreclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	controlDesiredStatePath = "/api/v1/sensor/control/desired-state"
	controlAppliedStatePath = "/api/v1/sensor/control/applied-state"

	controlSchemaVersionV1     = 1
	maxSafeControlGeneration   = uint64(9007199254740991)
	maxControlRejectionCodeLen = 128
	maxControlRejectionMsgLen  = 512
	maxControlAckRequestBytes  = 4096

	CodeControlNoDesired ErrorCode = "CORECLIENT_CONTROL_NO_DESIRED"
	CodeControlProtocol  ErrorCode = "CORECLIENT_CONTROL_PROTOCOL"
)

var (
	lowerHex64RE     = regexp.MustCompile(`^[a-f0-9]{64}$`)
	rejectionCodeRE  = regexp.MustCompile(`^[A-Za-z0-9._:-]+$`)
	exactUintRE      = regexp.MustCompile(`^[0-9]+$`)
)

// ControlDesiredState is the strict GET /desired-state response envelope.
type ControlDesiredState struct {
	SchemaVersion      int                       `json:"schemaVersion"`
	DesiredGeneration  uint64                    `json:"desiredGeneration"`
	DesiredChecksum    string                    `json:"desiredChecksum"`
	Configuration      ControlDesiredConfiguration `json:"configuration"`
	AppliedGeneration  uint64                    `json:"appliedGeneration"`
	AppliedChecksum    *string                   `json:"appliedChecksum"`
	ObservedGeneration uint64                    `json:"observedGeneration"`
	ObservedChecksum   *string                   `json:"observedChecksum"`
	ObservedOutcome    *string                   `json:"observedOutcome"`
	RejectionCode      string                    `json:"rejectionCode,omitempty"`
}

// ControlDesiredConfiguration is the Slice 2 controlled configuration subset.
type ControlDesiredConfiguration struct {
	LogLevel  string                 `json:"logLevel"`
	Scheduler ControlDesiredScheduler `json:"scheduler"`
	Transport ControlDesiredTransport `json:"transport"`
}

// ControlDesiredScheduler mirrors Core scheduler desired config.
type ControlDesiredScheduler struct {
	MaxWorkers                int                              `json:"maxWorkers"`
	MaxQueueDepth             int                              `json:"maxQueueDepth"`
	MaxTrackedAssignments     int                              `json:"maxTrackedAssignments"`
	MaxTrackedJobs            int                              `json:"maxTrackedJobs"`
	MaxPayloadBytes           int                              `json:"maxPayloadBytes"`
	DefaultExecutionTimeoutMs int                              `json:"defaultExecutionTimeoutMs"`
	ShutdownGracePeriodMs     int                              `json:"shutdownGracePeriodMs"`
	RetryMaxAttempts          int                              `json:"retryMaxAttempts"`
	RetryInitialBackoffMs     int                              `json:"retryInitialBackoffMs"`
	RetryMaxBackoffMs         int                              `json:"retryMaxBackoffMs"`
	RetryJitterBasisPoints    int                              `json:"retryJitterBasisPoints"`
	Pools                     map[string]ControlDesiredPool    `json:"pools"`
}

// ControlDesiredPool is a named scheduler pool.
type ControlDesiredPool struct {
	Workers    int `json:"workers"`
	QueueDepth int `json:"queueDepth"`
}

// ControlDesiredTransport mirrors Core transport desired config.
type ControlDesiredTransport struct {
	RequestTimeoutMs   int `json:"requestTimeoutMs"`
	MaxRequestBytes    int `json:"maxRequestBytes"`
	MaxResponseBytes   int `json:"maxResponseBytes"`
	UploadBatchRecords int `json:"uploadBatchRecords"`
	UploadBatchBytes   int `json:"uploadBatchBytes"`
}

// ControlAppliedAckRequest is the PUT /applied-state request body.
// Identity fields (tenantId/sensorId/actor/desired_by) are intentionally absent.
type ControlAppliedAckRequest struct {
	SchemaVersion    int    `json:"schemaVersion"`
	Generation       uint64 `json:"generation"`
	Checksum         string `json:"checksum"`
	Outcome          string `json:"outcome"`
	RejectionCode    string `json:"rejectionCode,omitempty"`
	RejectionMessage string `json:"rejectionMessage,omitempty"`
}

// ControlAppliedAckResponse is Core's ACCEPTED ACK response.
type ControlAppliedAckResponse struct {
	Status        string `json:"status"`
	SchemaVersion int    `json:"schemaVersion"`
	Generation    uint64 `json:"generation"`
	Checksum      string `json:"checksum"`
	Outcome       string `json:"outcome"`
}

// FetchControlDesiredState performs GET /api/v1/sensor/control/desired-state.
// HTTP 204 returns (nil, nil) meaning no desired state.
func (c *Client) FetchControlDesiredState(ctx context.Context) (*ControlDesiredState, error) {
	status, body, err := c.exchangeJSON(ctx, http.MethodGet, controlDesiredStatePath, nil, "", "")
	if err != nil {
		return nil, err
	}
	if status == http.StatusNoContent {
		if len(bytes.TrimSpace(body)) != 0 {
			return nil, newError(CodeControlProtocol, "Core control 204 response must have empty body", nil, false)
		}
		return nil, nil
	}
	if status < 200 || status >= 300 {
		return nil, classifyStatus(status)
	}
	state, err := decodeControlDesiredState(body)
	if err != nil {
		return nil, err
	}
	return state, nil
}

// AckControlAppliedState performs PUT /api/v1/sensor/control/applied-state and validates
// that Core accepted the exact generation/checksum/outcome.
func (c *Client) AckControlAppliedState(ctx context.Context, req ControlAppliedAckRequest) (ControlAppliedAckResponse, error) {
	if err := validateControlAckRequest(req); err != nil {
		return ControlAppliedAckResponse{}, err
	}
	wire, err := controlAckWireBody(req)
	if err != nil {
		return ControlAppliedAckResponse{}, err
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		return ControlAppliedAckResponse{}, newError(CodeProtocol, "failed to encode control ACK", err, false)
	}
	if len(payload) > maxControlAckRequestBytes {
		return ControlAppliedAckResponse{}, newError(CodeInvalidConfig, "control ACK request exceeds byte ceiling", nil, false)
	}
	if err := assertNoIdentityFields(payload); err != nil {
		return ControlAppliedAckResponse{}, err
	}
	status, body, err := c.exchangeRawJSON(ctx, http.MethodPut, controlAppliedStatePath, payload, "", "")
	if err != nil {
		return ControlAppliedAckResponse{}, err
	}
	if status < 200 || status >= 300 {
		return ControlAppliedAckResponse{}, classifyStatus(status)
	}
	resp, err := decodeControlAckResponse(body)
	if err != nil {
		return ControlAppliedAckResponse{}, err
	}
	if err := validateControlAckBinding(req, resp); err != nil {
		return ControlAppliedAckResponse{}, err
	}
	return resp, nil
}

func validateControlAckRequest(req ControlAppliedAckRequest) error {
	if req.SchemaVersion != controlSchemaVersionV1 {
		return newError(CodeInvalidConfig, "control ACK schemaVersion unsupported", nil, false)
	}
	if req.Generation == 0 || req.Generation > maxSafeControlGeneration {
		return newError(CodeInvalidConfig, "control ACK generation out of range", nil, false)
	}
	if !lowerHex64RE.MatchString(req.Checksum) {
		return newError(CodeInvalidConfig, "control ACK checksum must be lowercase 64-hex", nil, false)
	}
	switch req.Outcome {
	case "APPLIED":
		if req.RejectionCode != "" || req.RejectionMessage != "" {
			return newError(CodeInvalidConfig, "APPLIED ACK must not include rejection evidence", nil, false)
		}
	case "REJECTED":
		if req.RejectionCode == "" {
			return newError(CodeInvalidConfig, "REJECTED ACK requires rejectionCode", nil, false)
		}
		if len(req.RejectionCode) > maxControlRejectionCodeLen || !rejectionCodeRE.MatchString(req.RejectionCode) {
			return newError(CodeInvalidConfig, "rejectionCode is invalid", nil, false)
		}
		if req.RejectionMessage != "" && len(req.RejectionMessage) > maxControlRejectionMsgLen {
			return newError(CodeInvalidConfig, "rejectionMessage exceeds limit", nil, false)
		}
	default:
		return newError(CodeInvalidConfig, "control ACK outcome must be APPLIED or REJECTED", nil, false)
	}
	return nil
}

type controlAckWire struct {
	SchemaVersion    int    `json:"schemaVersion"`
	Generation       uint64 `json:"generation"`
	Checksum         string `json:"checksum"`
	Outcome          string `json:"outcome"`
	RejectionCode    string `json:"rejectionCode,omitempty"`
	RejectionMessage string `json:"rejectionMessage,omitempty"`
}

func controlAckWireBody(req ControlAppliedAckRequest) (controlAckWire, error) {
	body := controlAckWire{
		SchemaVersion: req.SchemaVersion,
		Generation:    req.Generation,
		Checksum:      req.Checksum,
		Outcome:       req.Outcome,
	}
	if req.Outcome == "REJECTED" {
		body.RejectionCode = req.RejectionCode
		if req.RejectionMessage != "" {
			body.RejectionMessage = req.RejectionMessage
		}
	}
	return body, nil
}

func assertNoIdentityFields(payload []byte) error {
	lower := strings.ToLower(string(payload))
	for _, forbidden := range []string{`"tenantid"`, `"tenant_id"`, `"sensorid"`, `"sensor_id"`, `"actor"`, `"desired_by"`, `"desiredby"`} {
		if strings.Contains(lower, forbidden) {
			return newError(CodeProtocol, "control ACK must not carry identity fields", nil, false)
		}
	}
	return nil
}

// exchangeRawJSON is like exchangeJSON but sends a pre-encoded JSON body without remarsaling.
func (c *Client) exchangeRawJSON(ctx context.Context, method, path string, payload []byte, correlationID, idempotencyKey string) (int, []byte, error) {
	if c == nil {
		return 0, nil, newError(CodeInvalidConfig, "Core client is nil", nil, false)
	}
	if c.initErr != nil {
		return 0, nil, c.initErr
	}
	if ctx == nil {
		return 0, nil, newError(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	if err := contextClassified(ctx); err != nil {
		return 0, nil, err
	}
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(c.baseURL.Path, "/") + path

	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return 0, nil, newError(CodeProtocol, "failed to create Core request", err, false)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if correlationID != "" {
		req.Header.Set("X-Correlation-ID", correlationID)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, classifyTransport(ctx, err)
	}
	defer resp.Body.Close()
	responseBody, err := readBounded(resp.Body, c.maxResponseBytes)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, responseBody, nil
}

func validateControlAckBinding(req ControlAppliedAckRequest, resp ControlAppliedAckResponse) error {
	if resp.Status != "ACCEPTED" {
		return newError(CodeControlProtocol, "control ACK status is not ACCEPTED", nil, false)
	}
	if resp.SchemaVersion != controlSchemaVersionV1 {
		return newError(CodeControlProtocol, "control ACK response schemaVersion mismatch", nil, false)
	}
	if resp.Generation != req.Generation {
		return newError(CodeControlProtocol, "control ACK response generation mismatch", nil, false)
	}
	if resp.Checksum != req.Checksum {
		return newError(CodeControlProtocol, "control ACK response checksum mismatch", nil, false)
	}
	if resp.Outcome != req.Outcome {
		return newError(CodeControlProtocol, "control ACK response outcome mismatch", nil, false)
	}
	return nil
}

// NewControlClientForTest creates an in-memory client against a test server for internal test suites.
func NewControlClientForTest(baseURL *url.URL, httpClient *http.Client, maxResponseBytes int) *Client {
	if maxResponseBytes <= 0 {
		maxResponseBytes = 1024 * 1024
	}
	return &Client{
		baseURL:          baseURL,
		httpClient:       httpClient,
		maxResponseBytes: maxResponseBytes,
	}
}

func decodeControlDesiredState(data []byte) (*ControlDesiredState, error) {
	raw, err := decodeStrictObject(data)
	if err != nil {
		return nil, newError(CodeControlProtocol, "control desired-state JSON is invalid", err, false)
	}

	// Safely parse the minimum immutable binding needed for rejection / dispatch.
	schemaRaw, hasSchema := raw["schemaVersion"]
	if !hasSchema {
		return nil, newError(CodeControlProtocol, "control desired-state missing schemaVersion", nil, false)
	}
	genRaw, hasGen := raw["desiredGeneration"]
	if !hasGen {
		return nil, newError(CodeControlProtocol, "control desired-state missing desiredGeneration", nil, false)
	}
	ckRaw, hasCk := raw["desiredChecksum"]
	if !hasCk {
		return nil, newError(CodeControlProtocol, "control desired-state missing desiredChecksum", nil, false)
	}

	schemaVersion, err := parseExactUint(schemaRaw, 1, maxSafeControlGeneration)
	if err != nil {
		return nil, newError(CodeControlProtocol, "schemaVersion invalid", err, false)
	}
	desiredGen, err := parseExactUint(genRaw, 1, maxSafeControlGeneration)
	if err != nil {
		return nil, newError(CodeControlProtocol, "desiredGeneration invalid", err, false)
	}
	desiredChecksum, err := parseLowerHex64(ckRaw)
	if err != nil {
		return nil, newError(CodeControlProtocol, "desiredChecksum invalid", err, false)
	}

	// For unsupported schemaVersion (!= 1), return the safe binding without validating
	// v1 fields or applying configuration, so reconciler can send SENSOR_CONTROL_UNSUPPORTED_SCHEMA.
	// Additional/unknown top-level fields in schemaVersion != 1 must not be rejected here.
	if schemaVersion != controlSchemaVersionV1 {
		return &ControlDesiredState{
			SchemaVersion:     int(schemaVersion),
			DesiredGeneration: desiredGen,
			DesiredChecksum:   desiredChecksum,
		}, nil
	}

	// For schemaVersion == 1, enforce all strict v1 rules: top-level allowlist, required fields, configuration.
	allowed := map[string]struct{}{
		"schemaVersion": {}, "desiredGeneration": {}, "desiredChecksum": {}, "configuration": {},
		"appliedGeneration": {}, "appliedChecksum": {}, "observedGeneration": {}, "observedChecksum": {},
		"observedOutcome": {}, "rejectionCode": {},
	}
	for k := range raw {
		if _, ok := allowed[k]; !ok {
			return nil, newError(CodeControlProtocol, "control desired-state contains unknown field", nil, false)
		}
	}
	for _, req := range []string{
		"schemaVersion", "desiredGeneration", "desiredChecksum", "configuration",
		"appliedGeneration", "appliedChecksum", "observedGeneration", "observedChecksum", "observedOutcome",
	} {
		if _, ok := raw[req]; !ok {
			return nil, newError(CodeControlProtocol, "control desired-state missing required field", nil, false)
		}
	}
	cfg, err := parseDesiredConfiguration(raw["configuration"])
	if err != nil {
		return nil, newError(CodeControlProtocol, "configuration invalid", err, false)
	}
	appliedGen, err := parseExactUint(raw["appliedGeneration"], 0, maxSafeControlGeneration)
	if err != nil {
		return nil, newError(CodeControlProtocol, "appliedGeneration invalid", err, false)
	}
	appliedChecksum, err := parseNullableLowerHex64(raw["appliedChecksum"])
	if err != nil {
		return nil, newError(CodeControlProtocol, "appliedChecksum invalid", err, false)
	}
	observedGen, err := parseExactUint(raw["observedGeneration"], 0, maxSafeControlGeneration)
	if err != nil {
		return nil, newError(CodeControlProtocol, "observedGeneration invalid", err, false)
	}
	observedChecksum, err := parseNullableLowerHex64(raw["observedChecksum"])
	if err != nil {
		return nil, newError(CodeControlProtocol, "observedChecksum invalid", err, false)
	}
	observedOutcome, err := parseNullableOutcome(raw["observedOutcome"])
	if err != nil {
		return nil, newError(CodeControlProtocol, "observedOutcome invalid", err, false)
	}

	out := &ControlDesiredState{
		SchemaVersion:      int(schemaVersion),
		DesiredGeneration:  desiredGen,
		DesiredChecksum:    desiredChecksum,
		Configuration:      cfg,
		AppliedGeneration:  appliedGen,
		AppliedChecksum:    appliedChecksum,
		ObservedGeneration: observedGen,
		ObservedChecksum:   observedChecksum,
		ObservedOutcome:    observedOutcome,
	}

	if appliedGen == 0 {
		if appliedChecksum != nil {
			return nil, newError(CodeControlProtocol, "appliedChecksum must be null when appliedGeneration is 0", nil, false)
		}
	} else if appliedChecksum == nil {
		return nil, newError(CodeControlProtocol, "appliedChecksum required when appliedGeneration > 0", nil, false)
	}

	if observedGen == 0 {
		if observedChecksum != nil || observedOutcome != nil {
			return nil, newError(CodeControlProtocol, "observed fields must be null when observedGeneration is 0", nil, false)
		}
		if _, has := raw["rejectionCode"]; has {
			return nil, newError(CodeControlProtocol, "rejectionCode forbidden when observedGeneration is 0", nil, false)
		}
	} else {
		if observedChecksum == nil || observedOutcome == nil {
			return nil, newError(CodeControlProtocol, "observedChecksum/outcome required when observedGeneration > 0", nil, false)
		}
	}

	if _, has := raw["rejectionCode"]; has {
		code, err := parseRejectionCode(raw["rejectionCode"])
		if err != nil {
			return nil, newError(CodeControlProtocol, "rejectionCode invalid", err, false)
		}
		out.RejectionCode = code
	}
	if observedOutcome != nil && *observedOutcome == "REJECTED" {
		if out.RejectionCode == "" {
			return nil, newError(CodeControlProtocol, "rejectionCode required when observedOutcome is REJECTED", nil, false)
		}
	}
	if observedOutcome != nil && *observedOutcome == "APPLIED" && out.RejectionCode != "" {
		return nil, newError(CodeControlProtocol, "rejectionCode forbidden when observedOutcome is APPLIED", nil, false)
	}
	return out, nil
}

func decodeControlAckResponse(data []byte) (ControlAppliedAckResponse, error) {
	raw, err := decodeStrictObject(data)
	if err != nil {
		return ControlAppliedAckResponse{}, newError(CodeControlProtocol, "control ACK response JSON is invalid", err, false)
	}
	allowed := map[string]struct{}{
		"status": {}, "schemaVersion": {}, "generation": {}, "checksum": {}, "outcome": {},
	}
	for k := range raw {
		if _, ok := allowed[k]; !ok {
			return ControlAppliedAckResponse{}, newError(CodeControlProtocol, "control ACK response contains unknown field", nil, false)
		}
	}
	for _, req := range []string{"status", "schemaVersion", "generation", "checksum", "outcome"} {
		if _, ok := raw[req]; !ok {
			return ControlAppliedAckResponse{}, newError(CodeControlProtocol, "control ACK response missing required field", nil, false)
		}
	}
	status, err := parseExactString(raw["status"])
	if err != nil {
		return ControlAppliedAckResponse{}, newError(CodeControlProtocol, "ACK status invalid", err, false)
	}
	schemaVersion, err := parseExactUint(raw["schemaVersion"], 1, 1)
	if err != nil {
		return ControlAppliedAckResponse{}, newError(CodeControlProtocol, "ACK schemaVersion invalid", err, false)
	}
	generation, err := parseExactUint(raw["generation"], 1, maxSafeControlGeneration)
	if err != nil {
		return ControlAppliedAckResponse{}, newError(CodeControlProtocol, "ACK generation invalid", err, false)
	}
	checksum, err := parseLowerHex64(raw["checksum"])
	if err != nil {
		return ControlAppliedAckResponse{}, newError(CodeControlProtocol, "ACK checksum invalid", err, false)
	}
	outcome, err := parseExactString(raw["outcome"])
	if err != nil || (outcome != "APPLIED" && outcome != "REJECTED") {
		return ControlAppliedAckResponse{}, newError(CodeControlProtocol, "ACK outcome invalid", err, false)
	}
	return ControlAppliedAckResponse{
		Status:        status,
		SchemaVersion: int(schemaVersion),
		Generation:    generation,
		Checksum:      checksum,
		Outcome:       outcome,
	}, nil
}

func decodeStrictObject(data []byte) (map[string]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New("empty JSON document")
	}
	if trimmed[0] != '{' {
		return nil, errors.New("JSON document is not an object")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing JSON document")
	}
	return raw, nil
}

func parseDesiredConfiguration(raw json.RawMessage) (ControlDesiredConfiguration, error) {
	obj, err := decodeStrictObject(raw)
	if err != nil {
		return ControlDesiredConfiguration{}, err
	}
	allowed := map[string]struct{}{"logLevel": {}, "scheduler": {}, "transport": {}}
	for k := range obj {
		if _, ok := allowed[k]; !ok {
			return ControlDesiredConfiguration{}, fmt.Errorf("unknown configuration field")
		}
	}
	for _, req := range []string{"logLevel", "scheduler", "transport"} {
		if _, ok := obj[req]; !ok {
			return ControlDesiredConfiguration{}, fmt.Errorf("missing configuration field")
		}
	}
	logLevel, err := parseExactString(obj["logLevel"])
	if err != nil {
		return ControlDesiredConfiguration{}, err
	}
	switch logLevel {
	case "DEBUG", "INFO", "WARN", "ERROR":
	default:
		return ControlDesiredConfiguration{}, fmt.Errorf("invalid logLevel")
	}
	scheduler, err := parseDesiredScheduler(obj["scheduler"])
	if err != nil {
		return ControlDesiredConfiguration{}, err
	}
	transport, err := parseDesiredTransport(obj["transport"])
	if err != nil {
		return ControlDesiredConfiguration{}, err
	}
	return ControlDesiredConfiguration{LogLevel: logLevel, Scheduler: scheduler, Transport: transport}, nil
}

func parseDesiredScheduler(raw json.RawMessage) (ControlDesiredScheduler, error) {
	obj, err := decodeStrictObject(raw)
	if err != nil {
		return ControlDesiredScheduler{}, err
	}
	required := []string{
		"maxWorkers", "maxQueueDepth", "maxTrackedAssignments", "maxTrackedJobs", "maxPayloadBytes",
		"defaultExecutionTimeoutMs", "shutdownGracePeriodMs", "retryMaxAttempts", "retryInitialBackoffMs",
		"retryMaxBackoffMs", "retryJitterBasisPoints", "pools",
	}
	allowed := map[string]struct{}{}
	for _, k := range required {
		allowed[k] = struct{}{}
	}
	for k := range obj {
		if _, ok := allowed[k]; !ok {
			return ControlDesiredScheduler{}, fmt.Errorf("unknown scheduler field")
		}
	}
	for _, k := range required {
		if _, ok := obj[k]; !ok {
			return ControlDesiredScheduler{}, fmt.Errorf("missing scheduler field")
		}
	}
	getInt := func(key string, min, max int64) (int, error) {
		v, err := parseExactUint(obj[key], uint64(min), uint64(max))
		if err != nil {
			return 0, err
		}
		return int(v), nil
	}
	out := ControlDesiredScheduler{}
	if out.MaxWorkers, err = getInt("maxWorkers", 1, 256); err != nil {
		return ControlDesiredScheduler{}, err
	}
	if out.MaxQueueDepth, err = getInt("maxQueueDepth", 1, 10000); err != nil {
		return ControlDesiredScheduler{}, err
	}
	if out.MaxTrackedAssignments, err = getInt("maxTrackedAssignments", 1, 50000); err != nil {
		return ControlDesiredScheduler{}, err
	}
	if out.MaxTrackedJobs, err = getInt("maxTrackedJobs", 1, 50000); err != nil {
		return ControlDesiredScheduler{}, err
	}
	if out.MaxPayloadBytes, err = getInt("maxPayloadBytes", 1, 4*1024*1024); err != nil {
		return ControlDesiredScheduler{}, err
	}
	if out.DefaultExecutionTimeoutMs, err = getInt("defaultExecutionTimeoutMs", 1, 3600000); err != nil {
		return ControlDesiredScheduler{}, err
	}
	if out.ShutdownGracePeriodMs, err = getInt("shutdownGracePeriodMs", 1, 600000); err != nil {
		return ControlDesiredScheduler{}, err
	}
	if out.RetryMaxAttempts, err = getInt("retryMaxAttempts", 1, 100); err != nil {
		return ControlDesiredScheduler{}, err
	}
	if out.RetryInitialBackoffMs, err = getInt("retryInitialBackoffMs", 1, 3600000); err != nil {
		return ControlDesiredScheduler{}, err
	}
	if out.RetryMaxBackoffMs, err = getInt("retryMaxBackoffMs", 1, 3600000); err != nil {
		return ControlDesiredScheduler{}, err
	}
	if out.RetryJitterBasisPoints, err = getInt("retryJitterBasisPoints", 0, 10000); err != nil {
		return ControlDesiredScheduler{}, err
	}
	pools, err := parseDesiredPools(obj["pools"])
	if err != nil {
		return ControlDesiredScheduler{}, err
	}
	out.Pools = pools
	return out, nil
}

func parseDesiredPools(raw json.RawMessage) (map[string]ControlDesiredPool, error) {
	obj, err := decodeStrictObject(raw)
	if err != nil {
		return nil, err
	}
	if len(obj) < 1 || len(obj) > 6 {
		return nil, fmt.Errorf("pools count out of range")
	}
	out := make(map[string]ControlDesiredPool, len(obj))
	for name, value := range obj {
		switch name {
		case "NETWORK_POLL", "STREAMING", "INFRA", "DISCOVERY", "NCM", "SYNTHETIC":
		default:
			return nil, fmt.Errorf("unknown pool class")
		}
		poolObj, err := decodeStrictObject(value)
		if err != nil {
			return nil, err
		}
		for k := range poolObj {
			if k != "workers" && k != "queueDepth" {
				return nil, fmt.Errorf("unknown pool field")
			}
		}
		if _, ok := poolObj["workers"]; !ok {
			return nil, fmt.Errorf("pool workers required")
		}
		if _, ok := poolObj["queueDepth"]; !ok {
			return nil, fmt.Errorf("pool queueDepth required")
		}
		workersU, err := parseExactUint(poolObj["workers"], 1, 256)
		if err != nil {
			return nil, err
		}
		queueU, err := parseExactUint(poolObj["queueDepth"], 1, 10000)
		if err != nil {
			return nil, err
		}
		out[name] = ControlDesiredPool{Workers: int(workersU), QueueDepth: int(queueU)}
	}
	return out, nil
}

func parseDesiredTransport(raw json.RawMessage) (ControlDesiredTransport, error) {
	obj, err := decodeStrictObject(raw)
	if err != nil {
		return ControlDesiredTransport{}, err
	}
	required := []string{"requestTimeoutMs", "maxRequestBytes", "maxResponseBytes", "uploadBatchRecords", "uploadBatchBytes"}
	allowed := map[string]struct{}{}
	for _, k := range required {
		allowed[k] = struct{}{}
	}
	for k := range obj {
		if _, ok := allowed[k]; !ok {
			return ControlDesiredTransport{}, fmt.Errorf("unknown transport field")
		}
	}
	for _, k := range required {
		if _, ok := obj[k]; !ok {
			return ControlDesiredTransport{}, fmt.Errorf("missing transport field")
		}
	}
	getInt := func(key string, min, max int64) (int, error) {
		v, err := parseExactUint(obj[key], uint64(min), uint64(max))
		if err != nil {
			return 0, err
		}
		return int(v), nil
	}
	out := ControlDesiredTransport{}
	if out.RequestTimeoutMs, err = getInt("requestTimeoutMs", 1, 3600000); err != nil {
		return ControlDesiredTransport{}, err
	}
	if out.MaxRequestBytes, err = getInt("maxRequestBytes", 1, 16*1024*1024); err != nil {
		return ControlDesiredTransport{}, err
	}
	if out.MaxResponseBytes, err = getInt("maxResponseBytes", 1, 16*1024*1024); err != nil {
		return ControlDesiredTransport{}, err
	}
	if out.UploadBatchRecords, err = getInt("uploadBatchRecords", 1, 10000); err != nil {
		return ControlDesiredTransport{}, err
	}
	if out.UploadBatchBytes, err = getInt("uploadBatchBytes", 1, 16*1024*1024); err != nil {
		return ControlDesiredTransport{}, err
	}
	return out, nil
}

func parseExactUint(raw json.RawMessage, min, max uint64) (uint64, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, errors.New("null or empty number")
	}
	if trimmed[0] == '"' {
		return 0, errors.New("number encoded as string")
	}
	var num json.Number
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&num); err != nil {
		return 0, err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return 0, errors.New("trailing tokens in number")
	}
	s := num.String()
	if !exactUintRE.MatchString(s) {
		return 0, errors.New("non-integer numeric encoding")
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, err
	}
	if v < min || v > max {
		return 0, errors.New("number out of range")
	}
	return v, nil
}

func parseExactString(raw json.RawMessage) (string, error) {
	var s string
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return "", err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", errors.New("trailing tokens in string")
	}
	return s, nil
}

func parseLowerHex64(raw json.RawMessage) (string, error) {
	s, err := parseExactString(raw)
	if err != nil {
		return "", err
	}
	if !lowerHex64RE.MatchString(s) {
		return "", errors.New("not lowercase 64-hex")
	}
	return s, nil
}

func parseNullableLowerHex64(raw json.RawMessage) (*string, error) {
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	s, err := parseLowerHex64(raw)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func parseNullableOutcome(raw json.RawMessage) (*string, error) {
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	s, err := parseExactString(raw)
	if err != nil {
		return nil, err
	}
	if s != "APPLIED" && s != "REJECTED" {
		return nil, errors.New("invalid outcome")
	}
	return &s, nil
}

func parseRejectionCode(raw json.RawMessage) (string, error) {
	s, err := parseExactString(raw)
	if err != nil {
		return "", err
	}
	if s == "" || len(s) > maxControlRejectionCodeLen || !rejectionCodeRE.MatchString(s) {
		return "", errors.New("invalid rejectionCode")
	}
	return s, nil
}
