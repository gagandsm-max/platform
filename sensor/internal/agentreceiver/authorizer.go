package agentreceiver

import (
	"context"
)

// AssignmentContext is the authoritative placement result for an authenticated
// Agent delivering to a receiving Sensor. TenantID/SiteID/SensorID/AgentID are
// derived from assignment authority — never from unverified payload claims.
type AssignmentContext struct {
	TenantID string
	SiteID   string
	SensorID string
	AgentID  string
	Assigned bool
	Reason   string
}

// AssignmentAuthorizer defines the authoritative placement validation boundary.
// Lookup MUST use authenticated Agent identity + receiving Sensor identity ONLY.
// Payload tenant/site/sensor values are claims, never authority keys.
type AssignmentAuthorizer interface {
	Authorize(ctx context.Context, authAgent AuthenticatedAgentIdentity, receivingSensorID string) (AssignmentContext, error)
}

// FailClosedAuthorizer is the mandatory production default when no authoritative Core
// assignment channel or registry is yet wired. It rejects all deliveries safely.
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
