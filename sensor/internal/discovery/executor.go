package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sort"
	"sync"
	"time"

	"platform/sensor/internal/protocol/icmp"
	"platform/sensor/internal/scheduler"
)

const (
	JobKindDiscovery     = scheduler.JobKind("DISCOVERY")
	DefaultMaxConcurrent = 10
	DefaultSNMPTimeout   = 2 * time.Second
)

const (
	OIDSysDescr    = "1.3.6.1.2.1.1.1.0"
	OIDSysObjectID = "1.3.6.1.2.1.1.2.0"
	OIDSysName     = "1.3.6.1.2.1.1.5.0"
)

type ICMPProber interface {
	Probe(ctx context.Context, target string, timeout time.Duration) (bool, int64, string, error)
}

type SNMPProber interface {
	ProbeIdentity(ctx context.Context, target string, port int, timeout time.Duration, credentialProfileRef string) (map[string]any, string, error)
}

type Executor struct {
	sensorID      string
	maxConcurrent int
	icmpProber    ICMPProber
	snmpProber    SNMPProber
	logger        *slog.Logger
}

var _ scheduler.Executor = (*Executor)(nil)

func NewExecutor(sensorID string, maxConcurrent int, logger *slog.Logger) *Executor {
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrent
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Executor{
		sensorID: sensorID, maxConcurrent: maxConcurrent, logger: logger,
		icmpProber: &defaultICMPProber{}, snmpProber: &unavailableSNMPProber{},
	}
}

// SetProbers is a deterministic test/integration seam. Production SNMP wiring must use an
// authorized Credential Profile resolver and the existing protocol/snmp collector; this package
// deliberately does not accept raw credentials or embed a fallback community.
func (e *Executor) SetProbers(icmpP ICMPProber, snmpP SNMPProber) {
	if icmpP != nil { e.icmpProber = icmpP }
	if snmpP != nil { e.snmpProber = snmpP }
}

func (e *Executor) Execute(ctx context.Context, job scheduler.Job) (scheduler.ExecutionResult, error) {
	if job.Class != scheduler.ClassDiscovery {
		return scheduler.ExecutionResult{}, scheduler.Permanent(fmt.Errorf("INVALID_JOB_CLASS: expected DISCOVERY, got %s", job.Class))
	}
	var task Task
	if err := json.Unmarshal(job.Payload, &task); err != nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(fmt.Errorf("MALFORMED_TASK_PAYLOAD: %w", err))
	}
	result := e.Run(ctx, task)
	payload, err := json.Marshal(result)
	if err != nil {
		return scheduler.ExecutionResult{}, scheduler.Permanent(fmt.Errorf("FAILED_ENCODING_RESULT: %w", err))
	}
	return scheduler.ExecutionResult{Payload: payload}, nil
}

func (e *Executor) Run(ctx context.Context, task Task) Result {
	startedAt := time.Now().UTC()
	hasICMP, hasSNMP := requestedMethods(task.RequestedMethods)
	timeout := task.Timeout
	if timeout <= 0 { timeout = DefaultSNMPTimeout }

	e.logger.Info("Starting discovery task execution",
		"task_id", task.TaskID, "request_id", task.RequestID, "tenant_id", task.TenantID,
		"target_count", len(task.Targets), "methods", task.RequestedMethods)

	type job struct{ index int }
	jobs := make(chan job)
	results := make([][]Evidence, len(task.Targets))
	workers := e.maxConcurrent
	if workers > len(task.Targets) { workers = len(task.Targets) }
	if workers < 1 { workers = 1 }

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for work := range jobs {
				if ctx.Err() != nil { continue }
				target := task.Targets[work.index]
				perTarget := make([]Evidence, 0, 2)
				if hasICMP && ctx.Err() == nil {
					perTarget = append(perTarget, e.probeICMP(ctx, target, timeout))
				}
				if hasSNMP && ctx.Err() == nil {
					port := task.SNMPPort
					if port <= 0 { port = 161 }
					perTarget = append(perTarget, e.probeSNMP(ctx, target, port, timeout, task.CredentialProfileRef))
				}
				results[work.index] = perTarget
			}
		}()
	}

feed:
	for i := range task.Targets {
		select {
		case jobs <- job{index: i}:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

	// Flatten in target order and method order. Worker scheduling never changes evidence ordering.
	evidence := make([]Evidence, 0, len(task.Targets)*2)
	for i := range results { evidence = append(evidence, results[i]...) }

	completedAt := time.Now().UTC()
	status, failureReason := "COMPLETED", ""
	if ctx.Err() != nil {
		status = "FAILED"
		failureReason = "Discovery execution cancelled or deadline exceeded"
	}
	e.logger.Info("Discovery task completed",
		"task_id", task.TaskID, "request_id", task.RequestID, "evidence_count", len(evidence),
		"duration_ms", completedAt.Sub(startedAt).Milliseconds(), "status", status)

	return Result{
		RequestID: task.RequestID, TenantID: task.TenantID, CorrelationID: task.CorrelationID,
		Status: status, FailureReason: failureReason, Evidence: evidence,
		StartedAt: startedAt, CompletedAt: completedAt, DurationMs: completedAt.Sub(startedAt).Milliseconds(),
	}
}

func requestedMethods(methods []string) (bool, bool) {
	if len(methods) == 0 { return true, true }
	hasICMP, hasSNMP := false, false
	for _, method := range methods {
		switch method {
		case "ICMP": hasICMP = true
		case "SNMP": hasSNMP = true
		}
	}
	return hasICMP, hasSNMP
}

func (e *Executor) probeICMP(ctx context.Context, target Target, timeout time.Duration) Evidence {
	start := time.Now()
	success, rttMicros, techCode, _ := e.icmpProber.Probe(ctx, target.Address, timeout)
	attrs := make(map[string]any)
	status, reachability := "SUCCESS", "OBSERVED"
	if success {
		attrs["rtt_micros"] = rttMicros
		attrs["reachability"] = "REACHABLE"
		if techCode == "" { techCode = "ICMP_ECHO_REPLY" }
	} else {
		status, reachability = "INCONCLUSIVE", "UNKNOWN"
		if techCode == "" { techCode = "ICMP_TIMEOUT" }
	}
	return Evidence{
		TargetAddress: target.Address, Protocol: "ICMP", Method: "ECHO", Status: status,
		ReachabilityKind: reachability, TechnicalCode: techCode, Attributes: attrs,
		RawEvidence: map[string]any{"rtt_micros": rttMicros, "raw_code": techCode},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

func (e *Executor) probeSNMP(ctx context.Context, target Target, port int, timeout time.Duration, credentialProfileRef string) Evidence {
	start := time.Now()
	attrs, techCode, err := e.snmpProber.ProbeIdentity(ctx, target.Address, port, timeout, credentialProfileRef)
	status, reachability := "SUCCESS", "OBSERVED"
	if err != nil || len(attrs) == 0 {
		status, reachability = "INCONCLUSIVE", "UNKNOWN"
		if techCode == "" { techCode = "SNMP_NO_RESPONSE" }
	}
	return Evidence{
		TargetAddress: target.Address, Protocol: "SNMP", Method: "GET_IDENTITY", Status: status,
		ReachabilityKind: reachability, TechnicalCode: techCode, Attributes: attrs,
		RawEvidence: map[string]any{"port": port, "raw_code": techCode},
		DurationMs: time.Since(start).Milliseconds(),
	}
}

type defaultICMPProber struct{}

func (p *defaultICMPProber) Probe(ctx context.Context, target string, timeout time.Duration) (bool, int64, string, error) {
	ip, code, err := resolveSafeTarget(ctx, target)
	if err != nil { return false, 0, code, err }
	version := 6
	if ip.To4() != nil { version = 4 }
	limits := icmp.Limits{
		MaxSamples: 1, MaxPayloadBytes: 64, MaxExecutionTimeout: timeout + 500*time.Millisecond,
		MaxReplyPacketsPerSample: 2, MaxSchedulerPayloadBytes: 2048,
	}
	collector, err := icmp.NewCollector(limits)
	if err != nil { return false, 0, "ICMP_COLLECTOR_UNAVAILABLE", err }
	result, err := collector.Collect(ctx, icmp.Request{
		Target: ip.String(), IPVersion: version, Samples: 1, Timeout: timeout,
		ExecutionTimeout: timeout + 200*time.Millisecond,
	})
	if err != nil {
		code := string(icmp.CodeOf(err))
		if code == "" { code = "ICMP_INCONCLUSIVE" }
		return false, 0, code, err
	}
	if result.Received == 0 { return false, 0, "ICMP_TIMEOUT", errors.New("no ICMP echo reply received") }
	return true, result.AvgRTTMicros, "ICMP_ECHO_REPLY", nil
}

// resolveSafeTarget resolves hostnames at the assigned Sensor, never at Core, and re-applies
// SSRF-sensitive address exclusions to the actual resolved destination. RFC1918/ULA addresses remain allowed.
func resolveSafeTarget(ctx context.Context, target string) (net.IP, string, error) {
	if ip := net.ParseIP(target); ip != nil {
		if prohibitedIP(ip) { return nil, "TARGET_RESOLUTION_PROHIBITED", errors.New("resolved target is prohibited") }
		return ip, "", nil
	}
	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, target)
	if err != nil { return nil, "DNS_FAILURE", err }
	candidates := make([]string, 0, len(resolved))
	byString := make(map[string]net.IP, len(resolved))
	for _, result := range resolved {
		ip := result.IP
		if ip == nil || prohibitedIP(ip) { continue }
		key := ip.String()
		candidates = append(candidates, key)
		byString[key] = ip
	}
	if len(candidates) == 0 { return nil, "TARGET_RESOLUTION_PROHIBITED", errors.New("hostname has no permitted address") }
	sort.Strings(candidates)
	return byString[candidates[0]], "", nil
}

func prohibitedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()
}

type unavailableSNMPProber struct{}

func (p *unavailableSNMPProber) ProbeIdentity(context.Context, string, int, time.Duration, string) (map[string]any, string, error) {
	return nil, "SNMP_CREDENTIAL_RESOLVER_UNAVAILABLE",
		errors.New("governed Sensor Credential Profile resolver is not implemented")
}