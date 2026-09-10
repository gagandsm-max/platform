package coreclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const agentAuthorityResolvePath = "/api/v1/sensor/agent-authority/resolve"

const (
	coreCodeAgentAuthorityUnavailable = "ERR_AGENT_AUTHORITY_UNAVAILABLE"
)

// AgentAuthorityView is the ACCEPTED body from Core agent-authority resolve.
type AgentAuthorityView struct {
	Status               string `json:"status"`
	AgentID              string `json:"agent_id"`
	TenantID             string `json:"tenant_id"`
	SiteID               string `json:"site_id,omitempty"`
	SensorID             string `json:"sensor_id"`
	AssignmentGeneration int64  `json:"assignment_generation"`
}

// AgentAuthorityError preserves Core `code` for definitive vs transient mapping.
type AgentAuthorityError struct {
	Code       string
	Message    string
	HTTPStatus int
	Transient  bool
	cause      error
}

func (e *AgentAuthorityError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

func (e *AgentAuthorityError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *AgentAuthorityError) IsTransient() bool {
	return e != nil && e.Transient
}

// ResolveAgentAuthority calls POST /api/v1/sensor/agent-authority/resolve once.
// It does not cache authority results.
func (c *Client) ResolveAgentAuthority(ctx context.Context, fingerprintSHA256 string) (AgentAuthorityView, error) {
	if c == nil {
		return AgentAuthorityView{}, newError(CodeInvalidConfig, "Core client is nil", nil, false)
	}
	if c.initErr != nil {
		return AgentAuthorityView{}, c.initErr
	}
	if ctx == nil {
		return AgentAuthorityView{}, newError(CodeInvalidConfig, "context must not be nil", nil, false)
	}
	if err := contextClassified(ctx); err != nil {
		return AgentAuthorityView{}, wrapAuthorityTransport(err)
	}
	fp := strings.TrimSpace(strings.ToLower(fingerprintSHA256))
	if fp == "" {
		return AgentAuthorityView{}, &AgentAuthorityError{
			Code:      "ERR_AGENT_CREDENTIAL_UNKNOWN",
			Message:   "fingerprint_sha256 is required",
			Transient: false,
		}
	}

	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(c.baseURL.Path, "/") + agentAuthorityResolvePath
	payload, err := json.Marshal(map[string]string{"fingerprint_sha256": fp})
	if err != nil {
		return AgentAuthorityView{}, newError(CodeProtocol, "failed to encode agent authority request", err, false)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return AgentAuthorityView{}, newError(CodeProtocol, "failed to create agent authority request", err, false)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return AgentAuthorityView{}, wrapAuthorityTransport(classifyTransport(ctx, err))
	}
	defer resp.Body.Close()

	body, err := readBounded(resp.Body, c.maxResponseBytes)
	if err != nil {
		return AgentAuthorityView{}, wrapAuthorityTransport(err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return AgentAuthorityView{}, classifyAgentAuthorityStatus(resp.StatusCode, body)
	}

	var view AgentAuthorityView
	if err := json.Unmarshal(body, &view); err != nil {
		return AgentAuthorityView{}, &AgentAuthorityError{
			Code:       string(CodeProtocol),
			Message:    "Core agent authority response is not valid JSON",
			HTTPStatus: resp.StatusCode,
			Transient:  false,
			cause:      err,
		}
	}
	if !strings.EqualFold(strings.TrimSpace(view.Status), "ACCEPTED") {
		return AgentAuthorityView{}, &AgentAuthorityError{
			Code:       string(CodeProtocol),
			Message:    "Core agent authority response status is not ACCEPTED",
			HTTPStatus: resp.StatusCode,
			Transient:  false,
		}
	}
	if strings.TrimSpace(view.AgentID) == "" || strings.TrimSpace(view.TenantID) == "" ||
		strings.TrimSpace(view.SensorID) == "" || view.AssignmentGeneration < 1 {
		return AgentAuthorityView{}, &AgentAuthorityError{
			Code:       string(CodeProtocol),
			Message:    "Core agent authority ACCEPTED body is incomplete",
			HTTPStatus: resp.StatusCode,
			Transient:  false,
		}
	}
	return view, nil
}

func classifyAgentAuthorityStatus(status int, body []byte) error {
	code, message := parseCoreErrorBody(body)
	transient := status == http.StatusTooManyRequests || status >= 500
	if code == coreCodeAgentAuthorityUnavailable {
		transient = true
	}
	if code == "" {
		switch {
		case status == http.StatusUnauthorized:
			code = string(CodeAuthentication)
			message = "Core rejected Sensor authentication"
		case status == http.StatusForbidden:
			code = string(CodeAuthorization)
			message = "Core denied Sensor agent authority resolve"
		case status == http.StatusTooManyRequests:
			code = string(CodeRateLimited)
			message = "Core rate limited agent authority resolve"
			transient = true
		case status >= 500:
			code = coreCodeAgentAuthorityUnavailable
			message = "Core service is temporarily unavailable"
			transient = true
		default:
			code = string(CodeProtocol)
			message = fmt.Sprintf("Core returned HTTP status %d", status)
		}
	}
	if message == "" {
		message = "Core agent authority resolve failed"
	}
	if transient && code != coreCodeAgentAuthorityUnavailable && !strings.HasPrefix(code, "ERR_AGENT_") {
		code = coreCodeAgentAuthorityUnavailable
	}
	return &AgentAuthorityError{
		Code:       code,
		Message:    message,
		HTTPStatus: status,
		Transient:  transient,
	}
}

func parseCoreErrorBody(body []byte) (code, message string) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", ""
	}
	var payload struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return "", ""
	}
	return strings.TrimSpace(payload.Code), strings.TrimSpace(payload.Message)
}

func wrapAuthorityTransport(err error) error {
	if err == nil {
		return nil
	}
	var authErr *AgentAuthorityError
	if errors.As(err, &authErr) {
		return authErr
	}
	msg := "Core agent authority temporarily unavailable"
	var coreErr *Error
	if errors.As(err, &coreErr) {
		msg = coreErr.Message
		switch coreErr.Code {
		case CodeAuthentication, CodeAuthorization:
			return &AgentAuthorityError{
				Code:      string(coreErr.Code),
				Message:   coreErr.Message,
				Transient: false,
				cause:     err,
			}
		}
	}
	return &AgentAuthorityError{
		Code:      coreCodeAgentAuthorityUnavailable,
		Message:   msg,
		Transient: true,
		cause:     err,
	}
}