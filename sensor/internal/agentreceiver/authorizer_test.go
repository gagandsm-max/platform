package agentreceiver

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// InMemoryAssignmentAuthorizer is TEST-ONLY. It stores assignments keyed by
// (agentID, sensorID) → {tenant, site}. It must not be used as production authority.
type InMemoryAssignmentAuthorizer struct {
	mu      sync.RWMutex
	allowed map[string]assignmentRecord
}

type assignmentRecord struct {
	tenantID string
	siteID   string
}

func NewInMemoryAssignmentAuthorizer() *InMemoryAssignmentAuthorizer {
	return &InMemoryAssignmentAuthorizer{
		allowed: make(map[string]assignmentRecord),
	}
}

func (a *InMemoryAssignmentAuthorizer) key(agentID, sensorID string) string {
	return strings.ToLower(strings.TrimSpace(agentID)) + "\x00" + strings.ToLower(strings.TrimSpace(sensorID))
}

// Allow registers a TEST-ONLY assignment of agentID → sensorID with authoritative tenant/site.
func (a *InMemoryAssignmentAuthorizer) Allow(agentID, sensorID, tenantID, siteID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowed[a.key(agentID, sensorID)] = assignmentRecord{
		tenantID: strings.TrimSpace(tenantID),
		siteID:   strings.TrimSpace(siteID),
	}
}

// Revoke removes a TEST-ONLY assignment for (agentID, sensorID).
func (a *InMemoryAssignmentAuthorizer) Revoke(agentID, sensorID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.allowed, a.key(agentID, sensorID))
}

func (a *InMemoryAssignmentAuthorizer) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowed = make(map[string]assignmentRecord)
}

func (a *InMemoryAssignmentAuthorizer) Authorize(ctx context.Context, authAgent AuthenticatedAgentIdentity, receivingSensorID string) (AssignmentContext, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	rec, ok := a.allowed[a.key(authAgent.AgentID, receivingSensorID)]
	if !ok {
		return AssignmentContext{
			AgentID:  authAgent.AgentID,
			SensorID: receivingSensorID,
			Assigned: false,
			Reason:   fmt.Sprintf("agent %q is not assigned to sensor %q", authAgent.AgentID, receivingSensorID),
		}, nil
	}

	return AssignmentContext{
		TenantID: rec.tenantID,
		SiteID:   rec.siteID,
		SensorID: receivingSensorID,
		AgentID:  authAgent.AgentID,
		Assigned: true,
		Reason:   "assignment authorized by in-memory test registry",
	}, nil
}

// inconsistentAuthorizer is TEST-ONLY and returns a pre-built AssignmentContext.
type inconsistentAuthorizer struct {
	ctx AssignmentContext
}

func (a *inconsistentAuthorizer) Authorize(ctx context.Context, authAgent AuthenticatedAgentIdentity, receivingSensorID string) (AssignmentContext, error) {
	return a.ctx, nil
}
