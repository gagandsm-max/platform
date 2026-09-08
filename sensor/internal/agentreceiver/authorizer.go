package agentreceiver

import (
	"context"
	"fmt"
	"sync"
)

// AssignmentDecision encapsulates the authorization verdict for an Agent-to-Sensor delivery.
type AssignmentDecision struct {
	Assigned bool
	Reason   string
}

// AssignmentAuthorizer defines the authoritative placement validation boundary.
// Core owns Agent-to-Sensor assignment; an Infrastructure Sensor must verify that
// the delivering Agent is explicitly assigned to it before accepting telemetry.
type AssignmentAuthorizer interface {
	AuthorizeAssignment(ctx context.Context, agentID, tenantID, sensorID string) (AssignmentDecision, error)
}

// FailClosedAuthorizer is the mandatory production default when no authoritative Core
// assignment channel or registry is yet wired. It rejects all deliveries safely.
type FailClosedAuthorizer struct{}

func NewFailClosedAuthorizer() *FailClosedAuthorizer {
	return &FailClosedAuthorizer{}
}

func (a *FailClosedAuthorizer) AuthorizeAssignment(ctx context.Context, agentID, tenantID, sensorID string) (AssignmentDecision, error) {
	return AssignmentDecision{
		Assigned: false,
		Reason:   "AGENT_SENSOR_ASSIGNMENT_INTEGRATION_BLOCKED: authoritative Core assignment source unavailable",
	}, nil
}

// InMemoryAssignmentAuthorizer provides deterministic assignment resolution for test harnesses.
type InMemoryAssignmentAuthorizer struct {
	mu      sync.RWMutex
	allowed map[string]bool
}

func NewInMemoryAssignmentAuthorizer() *InMemoryAssignmentAuthorizer {
	return &InMemoryAssignmentAuthorizer{
		allowed: make(map[string]bool),
	}
}

func (a *InMemoryAssignmentAuthorizer) key(agentID, tenantID, sensorID string) string {
	return fmt.Sprintf("%s:%s:%s", tenantID, agentID, sensorID)
}

func (a *InMemoryAssignmentAuthorizer) Allow(agentID, tenantID, sensorID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowed[a.key(agentID, tenantID, sensorID)] = true
}

func (a *InMemoryAssignmentAuthorizer) Revoke(agentID, tenantID, sensorID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.allowed, a.key(agentID, tenantID, sensorID))
}

func (a *InMemoryAssignmentAuthorizer) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowed = make(map[string]bool)
}

func (a *InMemoryAssignmentAuthorizer) AuthorizeAssignment(ctx context.Context, agentID, tenantID, sensorID string) (AssignmentDecision, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.allowed[a.key(agentID, tenantID, sensorID)] {
		return AssignmentDecision{
			Assigned: true,
			Reason:   "assignment authorized by in-memory registry",
		}, nil
	}

	return AssignmentDecision{
		Assigned: false,
		Reason:   fmt.Sprintf("agent %q is not assigned to sensor %q for tenant %q", agentID, sensorID, tenantID),
	}, nil
}
