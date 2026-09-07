package runtime

import (
	"fmt"
	"sort"
)

type Role string

const (
	RoleNetwork        Role = "NETWORK"
	RoleInfrastructure Role = "INFRASTRUCTURE"
	RoleStreamingEvent Role = "STREAMING_EVENT"
	RoleSynthetic      Role = "SYNTHETIC"
	RoleCloudAPI       Role = "CLOUD_API"
	RoleCombined       Role = "COMBINED"
)

type Capability string

const (
	CapabilityICMP         Capability = "ICMP"
	CapabilitySNMP         Capability = "SNMP"
	CapabilitySSH          Capability = "SSH"
	CapabilityNETCONF      Capability = "NETCONF"
	CapabilityGNMI         Capability = "GNMI"
	CapabilityREST         Capability = "REST"
	CapabilityDiscovery    Capability = "DISCOVERY"
	CapabilitySNMPTrap     Capability = "SNMP_TRAP"
	CapabilitySyslog       Capability = "SYSLOG"
	CapabilityFlow         Capability = "FLOW"
	CapabilityInfra        Capability = "INFRA"
	CapabilityWinRM        Capability = "WINRM"
	CapabilityRedfish      Capability = "REDFISH"
	CapabilityVMware       Capability = "VMWARE"
	CapabilityKubernetes   Capability = "KUBERNETES"
	CapabilityDatabase     Capability = "DATABASE"
	CapabilityAgentIngress Capability = "AGENT_INGRESS"
	CapabilitySynthetic    Capability = "SYNTHETIC"
	CapabilityCloudAPI     Capability = "CLOUD_API"
	CapabilityOTel         Capability = "OTEL"
)

var roleCapabilities = map[Role]map[Capability]struct{}{
	RoleNetwork: setOf(
		CapabilityICMP, CapabilitySNMP, CapabilitySSH, CapabilityNETCONF,
		CapabilityGNMI, CapabilityREST, CapabilityDiscovery,
		CapabilitySNMPTrap, CapabilitySyslog, CapabilityFlow,
	),
	RoleInfrastructure: setOf(
		CapabilitySSH, CapabilityREST, CapabilityInfra, CapabilityWinRM,
		CapabilityRedfish, CapabilityVMware, CapabilityKubernetes,
		CapabilityDatabase, CapabilityAgentIngress, CapabilityOTel,
	),
	RoleStreamingEvent: setOf(CapabilitySNMPTrap, CapabilitySyslog, CapabilityFlow, CapabilityOTel),
	RoleSynthetic:      setOf(CapabilitySynthetic),
	RoleCloudAPI:       setOf(CapabilityREST, CapabilityCloudAPI, CapabilityKubernetes),
}

func init() {
	combined := make(map[Capability]struct{})
	for _, role := range []Role{RoleNetwork, RoleInfrastructure, RoleStreamingEvent, RoleSynthetic, RoleCloudAPI} {
		for capability := range roleCapabilities[role] {
			combined[capability] = struct{}{}
		}
	}
	roleCapabilities[RoleCombined] = combined
}

func setOf(values ...Capability) map[Capability]struct{} {
	out := make(map[Capability]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func ValidateRole(role Role) error {
	if _, ok := roleCapabilities[role]; !ok {
		return fmt.Errorf("SENSOR_RUNTIME_INVALID_ROLE: unsupported sensor role %q", role)
	}
	return nil
}

func RoleAllows(role Role, capability Capability) bool {
	allowed, ok := roleCapabilities[role]
	if !ok {
		return false
	}
	_, ok = allowed[capability]
	return ok
}

func ValidateCapabilities(role Role, capabilities []Capability) error {
	if err := ValidateRole(role); err != nil {
		return err
	}
	seen := make(map[Capability]struct{}, len(capabilities))
	for _, capability := range capabilities {
		if capability == "" {
			return fmt.Errorf("SENSOR_RUNTIME_INVALID_CAPABILITY: empty capability")
		}
		if _, duplicate := seen[capability]; duplicate {
			return fmt.Errorf("SENSOR_RUNTIME_INVALID_CAPABILITY: duplicate capability %q", capability)
		}
		seen[capability] = struct{}{}
		if !RoleAllows(role, capability) {
			return fmt.Errorf("SENSOR_RUNTIME_CAPABILITY_NOT_ALLOWED: role %s does not allow capability %s", role, capability)
		}
	}
	return nil
}

func CapabilitiesForRole(role Role) []Capability {
	allowed := roleCapabilities[role]
	out := make([]Capability, 0, len(allowed))
	for capability := range allowed {
		out = append(out, capability)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
