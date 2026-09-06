package identity

import (
	"testing"
)

func TestIdentityGeneration(t *testing.T) {
	// Predefined ID
	id1, err := GenerateOrLoadIdentity("agent-custom-99")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1.AgentID != "agent-custom-99" {
		t.Fatalf("expected agent-custom-99, got %s", id1.AgentID)
	}

	// Generated ID
	id2, err := GenerateOrLoadIdentity("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id2.AgentID) == 0 {
		t.Fatalf("expected generated agent id, got empty")
	}
	if id2.Hostname == "" {
		t.Fatalf("expected hostname, got empty")
	}
}
