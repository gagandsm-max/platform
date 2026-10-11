package main

import (
	"log/slog"
	"time"

	"platform/agent/common"
	"platform/agent/common/collector"
	"platform/agent/common/config"
	"platform/agent/linux/ebpf"
)

// processNetworkRuntimeFactory constructs the optional Linux eBPF runtime.
// Tests inject failing factories; production uses defaultProcessNetworkRuntime.
type processNetworkRuntimeFactory func(cfg *config.Config) (common.ProcessNetworkRuntime, error)

func defaultProcessNetworkRuntime(cfg *config.Config) (common.ProcessNetworkRuntime, error) {
	agg, err := collector.NewProcessNetworkAggregator(collector.ProcessNetworkAggregatorConfig{
		SourceProvenance: collector.ProcessNetworkEBPFSourceProvenance,
	}, collector.UnwiredStructuredEvidenceSink{})
	if err != nil {
		return nil, err
	}
	return ebpf.NewBackend(ebpf.BackendConfig{
		Config: ebpf.Config{
			Enabled:            true,
			PreflightTimeout:   cfg.EBPFPreflightTimeout,
			RecheckMin:         cfg.EBPFRecheckMin,
			RecheckMax:         cfg.EBPFRecheckMax,
			RequestedMapBytes:  ebpf.RequestedMapBytesEstimate,
			RequestedRingBytes: ebpf.RequestedRingBytesExact,
			LowWaterSatisfied:  true,
		},
		Aggregator: agg,
	})
}

func featureDisabledCapability(now time.Time, recheckMin time.Duration) collector.ProcessNetworkCapabilitySnapshot {
	if recheckMin <= 0 {
		recheckMin = time.Minute
	}
	return collector.ProcessNetworkCapabilitySnapshot{
		State:            collector.ProcessNetworkCapabilityUnavailable,
		ReasonCodes:      []collector.ProcessNetworkCapabilityReason{collector.ReasonFeatureDisabled},
		ObservedAt:       now,
		TransitionAt:     now,
		ValidUntil:       now.Add(recheckMin),
		NextRecheckAt:    now.Add(recheckMin),
		SourceQuality:    "DISABLED",
		CurrentAuthority: true,
	}
}

func constructionFailureCapability(now time.Time, recheckMin time.Duration) collector.ProcessNetworkCapabilitySnapshot {
	if recheckMin <= 0 {
		recheckMin = time.Minute
	}
	return collector.ProcessNetworkCapabilitySnapshot{
		State:            collector.ProcessNetworkCapabilityUnavailable,
		ReasonCodes:      []collector.ProcessNetworkCapabilityReason{collector.ReasonSourceUnavailable},
		ObservedAt:       now,
		TransitionAt:     now,
		ValidUntil:       now.Add(recheckMin),
		NextRecheckAt:    now.Add(recheckMin),
		SourceQuality:    "UNAVAILABLE",
		CurrentAuthority: true,
	}
}

// optionalProcessNetworkOption returns the subordinate enrichment ServiceOption.
// Optional construction failure emits bounded non-ACTIVE evidence and never
// prevents base Agent construction/startup.
func optionalProcessNetworkOption(cfg *config.Config, logger *slog.Logger, factory processNetworkRuntimeFactory) common.ServiceOption {
	if factory == nil {
		factory = defaultProcessNetworkRuntime
	}
	now := time.Now().UTC()
	if !cfg.EBPFEnabled {
		return common.WithProcessNetworkCapability(featureDisabledCapability(now, cfg.EBPFRecheckMin))
	}
	rt, err := factory(cfg)
	if err != nil {
		logger.Error("AmrudAi Agent optional eBPF construction failed; continuing base Agent",
			"event_code", "AGENT_EBPF_CONSTRUCTION_FAILED",
			"safe_context", map[string]any{
				"reason_class": "optional_enrichment_construction_failed",
				"capability":   string(collector.ProcessNetworkCapabilityUnavailable),
				"detail":       string(collector.ReasonSourceUnavailable),
			})
		return common.WithProcessNetworkCapability(constructionFailureCapability(now, cfg.EBPFRecheckMin))
	}
	logger.Info("AmrudAi Agent optional eBPF runtime constructed",
		"event_code", "AGENT_EBPF_RUNTIME_WIRED",
		"safe_context", map[string]any{
			"reason_class": "optional_enrichment",
			"activation":   "after_running_async_once",
		})
	return common.WithProcessNetworkRuntime(rt)
}
