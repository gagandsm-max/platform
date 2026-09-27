package agentreceiver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"platform/sensor/internal/coreclient"
)

// AssignmentContext is the authoritative placement result for an authenticated
// Agent delivering to a receiving Sensor. TenantID/SiteID/SensorID/AgentID and
// Generation are derived from assignment authority — never from unverified
// payload claims.
type AssignmentContext struct {
	TenantID   string
	SiteID     string
	SensorID   string
	AgentID    string
	Generation int64
	Assigned   bool
	Reason     string
}

// AssignmentAuthorizer defines the authoritative placement validation boundary.
// Lookup MUST use authenticated Agent identity + receiving Sensor identity ONLY.
// Payload tenant/site/sensor values are claims, never authority keys.
type AssignmentAuthorizer interface {
	Authorize(ctx context.Context, authAgent AuthenticatedAgentIdentity, receivingSensorID string) (AssignmentContext, error)
}

// AuthorizerError is a typed assignment failure used to classify definitive
// (403) versus transient (503) responses.
type AuthorizerError struct {
	Code      string
	Message   string
	Transient bool
}

func (e *AuthorizerError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}

func (e *AuthorizerError) IsTransient() bool {
	return e != nil && e.Transient
}

func AsAuthorizerError(err error) (*AuthorizerError, bool) {
	var target *AuthorizerError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

// FailClosedAuthorizer is the mandatory production default when no authoritative
// Core assignment channel is wired. It rejects all deliveries safely.
type FailClosedAuthorizer struct{}

func NewFailClosedAuthorizer() *FailClosedAuthorizer {
	return &FailClosedAuthorizer{}
}

func (a *FailClosedAuthorizer) Authorize(ctx context.Context, authAgent AuthenticatedAgentIdentity, receivingSensorID string) (AssignmentContext, error) {
	return AssignmentContext{
		AgentID:  authAgent.AgentID,
		SensorID: receivingSensorID,
		Assigned: false,
		Reason:   "AGENT_SENSOR_ASSIGNMENT_INTEGRATION_BLOCKED: authoritative Core assignment source unavailable",
	}, nil
}

// AgentAuthorityClient is the Core resolve dependency for CoreBackedAuthorizer.
type AgentAuthorityClient interface {
	ResolveAgentAuthority(ctx context.Context, fingerprintSHA256 string) (coreclient.AgentAuthorityView, error)
}

// CoreBackedAuthorizer resolves placement via Core once per request.
// It does not cache authority across requests.
type CoreBackedAuthorizer struct {
	client            AgentAuthorityClient
	receivingSensorID string
}

func NewCoreBackedAuthorizer(client AgentAuthorityClient, receivingSensorID string) (*CoreBackedAuthorizer, error) {
	if client == nil {
		return nil, errors.New("ERR_INVALID_CONFIG: Core agent authority client is required")
	}
	sensorID := strings.TrimSpace(receivingSensorID)
	if sensorID == "" {
		return nil, errors.New("ERR_INVALID_CONFIG: receiving SensorID is required")
	}
	return &CoreBackedAuthorizer{client: client, receivingSensorID: sensorID}, nil
}

func (a *CoreBackedAuthorizer) Authorize(ctx context.Context, authAgent AuthenticatedAgentIdentity, receivingSensorID string) (AssignmentContext, error) {
	fp := strings.TrimSpace(authAgent.FingerprintSHA256)
	if fp == "" {
		return AssignmentContext{}, &AuthorizerError{
			Code:      ErrCodeAuthInvalid,
			Message:   "authenticated agent fingerprint is required for Core authority resolve",
			Transient: false,
		}
	}
	sensorID := strings.TrimSpace(receivingSensorID)
	if sensorID == "" {
		sensorID = a.receivingSensorID
	}
	if sensorID != a.receivingSensorID {
		return AssignmentContext{}, &AuthorizerError{
			Code:      ErrCodeAgentNotAssigned,
			Message:   "receiving sensor identity does not match authorizer binding",
			Transient: false,
		}
	}

	view, err := a.client.ResolveAgentAuthority(ctx, fp)
	if err != nil {
		return AssignmentContext{}, mapCoreAuthorityError(err)
	}

	agentID := strings.TrimSpace(view.AgentID)
	tenantID := strings.TrimSpace(view.TenantID)
	assignedSensor := strings.TrimSpace(view.SensorID)
	if agentID == "" || tenantID == "" || assignedSensor == "" ||
		view.AssignmentGeneration < 1 || view.AssignmentGeneration > MAX_SAFE_SEQUENCE {
		return AssignmentContext{}, &AuthorizerError{
			Code:      ErrCodeAgentNotAssigned,
			Message:   "Core authority response is incomplete or assignment generation out of range",
			Transient: false,
		}
	}
	// Exact SensorID match (case-sensitive), matching Core fencing semantics.
	if assignedSensor != sensorID {
		return AssignmentContext{}, &AuthorizerError{
			Code:      ErrCodeAgentAssignmentSensorMismatch,
			Message:   fmt.Sprintf("assigned sensor %q does not match receiving sensor %q", assignedSensor, sensorID),
			Transient: false,
		}
	}

	return AssignmentContext{
		TenantID:   tenantID,
		SiteID:     strings.TrimSpace(view.SiteID),
		SensorID:   assignedSensor,
		AgentID:    agentID,
		Generation: view.AssignmentGeneration,
		Assigned:   true,
		Reason:     "assignment authorized by Core agent-authority resolve",
	}, nil
}

func mapCoreAuthorityError(err error) error {
	var authErr *coreclient.AgentAuthorityError
	if errors.As(err, &authErr) {
		code := strings.TrimSpace(authErr.Code)
		if code == "" {
			code = ErrCodeAgentNotAssigned
		}
		transient := authErr.Transient
		if code == ErrCodeAgentAuthorityUnavailable {
			transient = true
		}
		if transient {
			return &AuthorizerError{
				Code:      ErrCodeAgentAuthorityUnavailable,
				Message:   authErr.Message,
				Transient: true,
			}
		}
		return &AuthorizerError{
			Code:      code,
			Message:   authErr.Message,
			Transient: false,
		}
	}
	if coreclient.IsTransient(err) {
		return &AuthorizerError{
			Code:      ErrCodeAgentAuthorityUnavailable,
			Message:   "Core agent authority temporarily unavailable",
			Transient: true,
		}
	}
	return &AuthorizerError{
		Code:      ErrCodeAgentNotAssigned,
		Message:   "Core agent authority denied assignment",
		Transient: false,
	}
}
