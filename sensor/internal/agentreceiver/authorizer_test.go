package agentreceiver

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// InMemoryAssignmentAuthorizer is TEST-ONLY. It stores assignments keyed by
// (agentID, sensorID) → {tenant, site, generation}. It must not be used as
// production authority.
type InMemoryAssignmentAuthorizer struct {
	mu      sync.RWMutex
	allowed map[string]assignmentRecord
}

type assignmentRecord struct {
	tenantID   string
	siteID     string
	generation int64
}

func NewInMemoryAssignmentAuthorizer() *InMemoryAssignmentAuthorizer {
	return &InMemoryAssignmentAuthorizer{
		allowed: make(map[string]assignmentRecord),
	}
}

func (a *InMemoryAssignmentAuthorizer) key(agentID, sensorID string) string {
	return strings.ToLower(strings.TrimSpace(agentID)) + "\x00" + strings.ToLower(strings.TrimSpace(sensorID))
}

// Allow registers a TEST-ONLY assignment with Generation=1.
func (a *InMemoryAssignmentAuthorizer) Allow(agentID, sensorID, tenantID, siteID string) {
	a.AllowGeneration(agentID, sensorID, tenantID, siteID, 1)
}

// AllowGeneration registers a TEST-ONLY assignment with an explicit generation.
func (a *InMemoryAssignmentAuthorizer) AllowGeneration(agentID, sensorID, tenantID, siteID string, generation int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowed[a.key(agentID, sensorID)] = assignmentRecord{
		tenantID:   strings.TrimSpace(tenantID),
		siteID:     strings.TrimSpace(siteID),
		generation: generation,
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

	lookupID := strings.TrimSpace(authAgent.AgentID)
	if lookupID == "" {
		lookupID = strings.TrimSpace(authAgent.FingerprintSHA256)
	}
	rec, ok := a.allowed[a.key(lookupID, receivingSensorID)]
	if !ok && strings.TrimSpace(authAgent.AgentID) != "" {
		// Also allow fingerprint-keyed registrations when AgentID is present.
		rec, ok = a.allowed[a.key(authAgent.FingerprintSHA256, receivingSensorID)]
	}
	if !ok {
		return AssignmentContext{
			AgentID:  authAgent.AgentID,
			SensorID: receivingSensorID,
			Assigned: false,
			Reason:   fmt.Sprintf("agent %q is not assigned to sensor %q", lookupID, receivingSensorID),
		}, nil
	}

	agentID := strings.TrimSpace(authAgent.AgentID)
	if agentID == "" {
		agentID = lookupID
	}

	return AssignmentContext{
		TenantID:   rec.tenantID,
		SiteID:     rec.siteID,
		SensorID:   receivingSensorID,
		AgentID:    agentID,
		Generation: rec.generation,
		Assigned:   true,
		Reason:     "assignment authorized by in-memory test registry",
	}, nil
}

// inconsistentAuthorizer is TEST-ONLY and returns a pre-built AssignmentContext.
type inconsistentAuthorizer struct {
	ctx AssignmentContext
}

func (a *inconsistentAuthorizer) Authorize(ctx context.Context, authAgent AuthenticatedAgentIdentity, receivingSensorID string) (AssignmentContext, error) {
	return a.ctx, nil
}
