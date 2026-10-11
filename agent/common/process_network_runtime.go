package common

import (
	"context"

	"platform/agent/common/collector"
)

// ProcessNetworkRuntime is the narrow optional enrichment lifecycle seam.
// Concrete Linux eBPF construction stays under agent/linux/**. common must not
// import linux/ebpf so Windows compilation remains clean.
//
// The runtime is subordinate to AgentService: base Agent reachability,
// readiness, scheduler, queue and delivery must not depend on Start success.
type ProcessNetworkRuntime interface {
	Start(ctx context.Context) error
	StopContext(ctx context.Context) error
	Capability() collector.ProcessNetworkCapabilitySnapshot
	SupportSnapshot() collector.ProcessNetworkSupportSnapshot
}
