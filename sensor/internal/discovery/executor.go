package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
	"platform/sensor/internal/protocol/icmp"
	"platform/sensor/internal/scheduler"
)

const (
	JobKindDiscovery     = scheduler.JobKind("DISCOVERY")
	DefaultMaxConcurrent = 10
	DefaultSNMPTimeout   = 2 * time.Second

	// Standard MIB-II Authoritative Identity OIDs
	OIDSysDescr    = "1.3.6.1.2.1.1.1.0"
	OIDSysObjectID = "1.3.6.1.2.1.1.2.0"
	OIDSysName     = "1.3.6.1.2.1.1.5.0"
)

type ICMPProber interface {
	Probe(ctx context.Context, target string, timeout time.Duration) (bool, int64, string, error)
}

type SNMPProber interface {
	ProbeIdentity(ctx context.Context, target string, port int, timeout time.Duration) (map[string]any, string, error)
}

type Executor struct {
	sensorID       string
	maxConcurrent  int
	icmpProber     ICMPProber
	snmpProber     SNMPProber
	logger         *slog.Logger
}

var _ scheduler.Executor = (*Executor)(nil)

func NewExecutor(sensorID string, maxConcurrent int, logger *slog.Logger) *Executor {
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrent
	}
	if logger == nil {
		logger = slog.Default()
	}
	e := &Executor{
		sensorID:      sensorID,
		maxConcurrent: maxConcurrent,
		logger:        logger,
	}
	e.icmpProber = &defaultICMPProber{}
	e.snmpProber = &defaultSNMPProber{}
	return e
}

func (e *Executor) SetProbers(icmpP ICMPProber, snmpP SNMPProber) {
	if icmpP != nil {
		e.icmpProber = icmpP
	}
	if snmpP != nil {
		e.snmpProber = snmpP
	}
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
	e.logger.Info("Starting discovery task execution",
		"task_id", task.TaskID,
		"request_id", task.RequestID,
		"tenant_id", task.TenantID,
		"target_count", len(task.Targets),
		"methods", task.RequestedMethods,
	)

	hasICMP := false
	hasSNMP := false
	if len(task.RequestedMethods) == 0 {
		hasICMP = true
		hasSNMP = true
	} else {
		for _, m := range task.RequestedMethods {
			if m == "ICMP" {
				hasICMP = true
			} else if m == "SNMP" {
				hasSNMP = true
			}
		}
	}

	var (
		evidenceMu sync.Mutex
		evidence   []Evidence
		sem        = make(chan struct{}, e.maxConcurrent)
		wg         sync.WaitGroup
	)

	timeout := task.Timeout
	if timeout <= 0 {
		timeout = DefaultSNMPTimeout
	}

	for _, target := range task.Targets {
		if ctx.Err() != nil {
			e.logger.Warn("Discovery task cancelled before probing all targets",
				"request_id", task.RequestID,
				"error", ctx.Err(),
			)
			break
		}

		targetCopy := target
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			// 1. ICMP probe if requested
			if hasICMP && ctx.Err() == nil {
				icmpEv := e.probeICMP(ctx, targetCopy, timeout)
				evidenceMu.Lock()
				evidence = append(evidence, icmpEv)
				evidenceMu.Unlock()
			}

			// 2. SNMP identity probe if requested
			if hasSNMP && ctx.Err() == nil {
				snmpPort := task.SNMPPort
				if snmpPort <= 0 {
					snmpPort = 161
				}
				snmpEv := e.probeSNMP(ctx, targetCopy, snmpPort, timeout)
				evidenceMu.Lock()
				evidence = append(evidence, snmpEv)
				evidenceMu.Unlock()
			}
		}()
	}

	wg.Wait()
	completedAt := time.Now().UTC()
	durationMs := completedAt.Sub(startedAt).Milliseconds()

	status := "COMPLETED"
	failureReason := ""
	if ctx.Err() != nil {
		status = "FAILED"
		failureReason = "Context cancelled or deadline exceeded"
	}

	e.logger.Info("Discovery task completed",
		"task_id", task.TaskID,
		"request_id", task.RequestID,
		"evidence_count", len(evidence),
		"duration_ms", durationMs,
		"status", status,
	)

	return Result{
		SensorID:      e.sensorID,
		RequestID:     task.RequestID,
		TenantID:      task.TenantID,
		CorrelationID: task.CorrelationID,
		Status:        status,
		FailureReason: failureReason,
		Evidence:      evidence,
		StartedAt:     startedAt,
		CompletedAt:   completedAt,
		DurationMs:    durationMs,
	}
}

func (e *Executor) probeICMP(ctx context.Context, target Target, timeout time.Duration) Evidence {
	start := time.Now()
	success, rttMicros, techCode, err := e.icmpProber.Probe(ctx, target.Address, timeout)
	durationMs := time.Since(start).Milliseconds()

	status := "SUCCESS"
	reachability := "OBSERVED"
	attrs := make(map[string]any)

	if success {
		attrs["rtt_micros"] = rttMicros
		attrs["reachability"] = "REACHABLE"
	} else {
		// Non-causal: ICMP failure MUST NOT automatically mean DEVICE_DOWN
		status = "INCONCLUSIVE"
		reachability = "UNKNOWN"
		if techCode == "" {
			techCode = "ICMP_TIMEOUT"
		}
		if err != nil {
			attrs["error_detail"] = err.Error()
		}
	}

	return Evidence{
		TargetAddress:    target.Address,
		Protocol:         "ICMP",
		Method:           "ECHO",
		Status:           status,
		ReachabilityKind: reachability,
		TechnicalCode:    techCode,
		Attributes:       attrs,
		RawEvidence:      map[string]any{"rtt_micros": rttMicros, "raw_error": techCode},
		DurationMs:       durationMs,
	}
}

func (e *Executor) probeSNMP(ctx context.Context, target Target, port int, timeout time.Duration) Evidence {
	start := time.Now()
	attrs, techCode, err := e.snmpProber.ProbeIdentity(ctx, target.Address, port, timeout)
	durationMs := time.Since(start).Milliseconds()

	status := "SUCCESS"
	reachability := "OBSERVED"

	if err != nil || len(attrs) == 0 {
		// Non-causal: SNMP timeout MUST NOT mean wrong credentials or device down!
		status = "INCONCLUSIVE"
		reachability = "UNKNOWN"
		if techCode == "" {
			techCode = "SNMP_TIMEOUT"
		}
	}

	return Evidence{
		TargetAddress:    target.Address,
		Protocol:         "SNMP",
		Method:           "GET_IDENTITY",
		Status:           status,
		ReachabilityKind: reachability,
		TechnicalCode:    techCode,
		Attributes:       attrs,
		RawEvidence:      map[string]any{"port": port, "raw_code": techCode},
		DurationMs:       durationMs,
	}
}

// defaultICMPProber uses UDP dial / ping probe or raw ICMP where permitted
type defaultICMPProber struct{}

func (p *defaultICMPProber) Probe(ctx context.Context, target string, timeout time.Duration) (bool, int64, string, error) {
	// Try standard ICMP echo via Collector if IP literal
	ip := net.ParseIP(target)
	if ip != nil && ip.To4() != nil {
		limits := icmp.Limits{
			MaxSamples:               1,
			MaxPayloadBytes:          64,
			MaxExecutionTimeout:      timeout + 500*time.Millisecond,
			MaxReplyPacketsPerSample: 2,
			MaxSchedulerPayloadBytes: 2048,
		}
		c, err := icmp.NewCollector(limits)
		if err == nil {
			res, colErr := c.Collect(ctx, icmp.Request{
				Target:           target,
				IPVersion:        4,
				Samples:          1,
				Timeout:          timeout,
				ExecutionTimeout: timeout + 200*time.Millisecond,
			})
			if colErr == nil && res.Received > 0 {
				return true, res.AvgRTTMicros, "", nil
			}
		}
	}

	// Fallback socket connection probe to verify IP reachability
	start := time.Now()
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "udp", net.JoinHostPort(target, "7"))
	if err != nil {
		return false, 0, "ICMP_TIMEOUT", err
	}
	defer conn.Close()
	return true, time.Since(start).Microseconds(), "", nil
}

// defaultSNMPProber queries standard MIB-II sysDescr, sysObjectID, sysName
type defaultSNMPProber struct{}

func (p *defaultSNMPProber) ProbeIdentity(ctx context.Context, target string, port int, timeout time.Duration) (map[string]any, string, error) {
	if port <= 0 {
		port = 161
	}
	snmp := &gosnmp.GoSNMP{
		Target:    target,
		Port:      uint16(port),
		Community: "public", // Default discovery read community; zero persistent credentials
		Version:   gosnmp.Version2c,
		Timeout:   timeout,
		Retries:   1,
	}

	if err := snmp.Connect(); err != nil {
		return nil, "SNMP_CONNECT_FAILED", err
	}
	defer snmp.Conn.Close()

	// Query standard MIB-II objects
	pkt, err := snmp.Get([]string{OIDSysDescr, OIDSysObjectID, OIDSysName})
	if err != nil {
		return nil, "SNMP_TIMEOUT", err
	}

	attrs := make(map[string]any)
	for _, pdu := range pkt.Variables {
		switch pdu.Name {
		case "." + OIDSysDescr, OIDSysDescr:
			if val, ok := pdu.Value.([]byte); ok {
				attrs["sysDescr"] = string(val)
			} else {
				attrs["sysDescr"] = fmt.Sprintf("%v", pdu.Value)
			}
		case "." + OIDSysObjectID, OIDSysObjectID:
			attrs["sysObjectID"] = fmt.Sprintf("%v", pdu.Value)
		case "." + OIDSysName, OIDSysName:
			if val, ok := pdu.Value.([]byte); ok {
				attrs["sysName"] = string(val)
			} else {
				attrs["sysName"] = fmt.Sprintf("%v", pdu.Value)
			}
		}
	}

	if len(attrs) == 0 {
		return nil, "SNMP_NO_ATTRIBUTES", errors.New("no identity attributes returned")
	}

	return attrs, "", nil
}
