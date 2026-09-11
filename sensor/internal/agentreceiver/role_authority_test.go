package agentreceiver

import (
	"testing"

	"platform/sensor/internal/runtime"
)

func TestAgentIngressRoleEligibilityUsesRuntimeCapabilityAuthority(t *testing.T) {
	roles := []runtime.Role{
		runtime.RoleNetwork,
		runtime.RoleInfrastructure,
		runtime.RoleStreamingEvent,
		runtime.RoleSynthetic,
		runtime.RoleCloudAPI,
		runtime.RoleCombined,
		runtime.Role("UNKNOWN"),
	}

	for _, role := range roles {
		got := IsRoleEligible(role)
		want := runtime.RoleAllows(role, runtime.CapabilityAgentIngress)
		if got != want {
			t.Fatalf("agent ingress eligibility drift for role %q: got %v want %v from runtime capability authority", role, got, want)
		}
	}
}
