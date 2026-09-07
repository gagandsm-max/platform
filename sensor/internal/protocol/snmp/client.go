package snmp

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gosnmp/gosnmp"
)

type Logger interface {
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
}

type session interface {
	Connect() error
	Close() error
	Get(oids []string) (*gosnmp.SnmpPacket, error)
	GetNext(oids []string) (*gosnmp.SnmpPacket, error)
	GetBulk(oids []string, nonRepeaters uint8, maxRepetitions uint32) (*gosnmp.SnmpPacket, error)
	Walk(rootOID string, walkFn gosnmp.WalkFunc) error
	BulkWalk(rootOID string, walkFn gosnmp.WalkFunc) error
}

type sessionFactory func(context.Context, Request) (session, error)

type collectorStats struct {
	executions     atomic.Uint64
	failures       atomic.Uint64
	retries        atomic.Uint64
	observations   atomic.Uint64
	activeWatchers atomic.Int64
}

type Stats struct {
	Executions                 uint64
	Failures                   uint64
	Retries                    uint64
	Observations               uint64
	ActiveCancellationWatchers int64
}

type Collector struct {
	factory sessionFactory
	logger  Logger
	now     func() time.Time
	wait    func(context.Context, time.Duration) error
	jitter  func(time.Duration, float64) time.Duration
	stats   collectorStats
}

func NewCollector(logger Logger) *Collector {
	return &Collector{
		factory: newGoSNMPSession,
		logger:  logger,
		now:     time.Now,
		wait:    waitWithContext,
		jitter:  jitterDuration,
	}
}

func (c *Collector) Stats() Stats {
	if c == nil {
		return Stats{}
	}
	return Stats{
		Executions:                 c.stats.executions.Load(),
		Failures:                   c.stats.failures.Load(),
		Retries:                    c.stats.retries.Load(),
		Observations:               c.stats.observations.Load(),
		ActiveCancellationWatchers: c.stats.activeWatchers.Load(),
	}
}

func (c *Collector) Collect(ctx context.Context, req Request) (CollectionResult, error) {
	if c == nil || c.factory == nil {
		return CollectionResult{}, newError(CodeInvalidRequest, "collector is not initialized", nil, false)
	}
	if err := ValidateRequest(req); err != nil {
		return CollectionResult{}, err
	}
	if ctx == nil {
		return CollectionResult{}, newError(CodeInvalidRequest, "context must not be nil", nil, false)
	}

	c.stats.executions.Add(1)
	execCtx, cancel := context.WithTimeout(ctx, req.ExecutionTimeout)
	defer cancel()
	started := c.now()

	var lastErr error
	for attempt := 1; attempt <= req.Retry.MaxAttempts; attempt++ {
		if err := contextError(execCtx); err != nil {
			lastErr = err
			break
		}
		result, err := c.collectAttempt(execCtx, req)
		if err == nil {
			c.stats.observations.Add(uint64(len(result.Observations)))
			if c.logger != nil {
				c.logger.Debug("sensor snmp collection completed",
					"operation", req.Operation,
					"snmp_version", req.Version,
					"duration_ms", c.now().Sub(started).Milliseconds(),
					"result_count", len(result.Observations),
				)
			}
			return result, nil
		}
		lastErr = err
		if !IsTransient(err) || attempt == req.Retry.MaxAttempts {
			break
		}
		delay := backoffDuration(req.Retry, attempt, c.jitter)
		if err := c.wait(execCtx, delay); err != nil {
			lastErr = contextError(execCtx)
			if lastErr == nil {
				lastErr = classifyTransportError(execCtx, err)
			}
			break
		}
		c.stats.retries.Add(1)
	}

	if lastErr == nil {
		lastErr = newError(CodeProtocolError, "SNMP collection failed without a classified cause", nil, false)
	}
	c.stats.failures.Add(1)
	if c.logger != nil {
		c.logger.Warn("sensor snmp collection failed",
			"operation", req.Operation,
			"snmp_version", req.Version,
			"duration_ms", c.now().Sub(started).Milliseconds(),
			"error_code", ErrorCodeOf(lastErr),
		)
	}
	return CollectionResult{}, lastErr
}

func (c *Collector) collectAttempt(ctx context.Context, req Request) (CollectionResult, error) {
	s, err := c.factory(ctx, req)
	if err != nil {
		return CollectionResult{}, err
	}
	// GoSNMP Connect uses the supplied Context/DialContext. Start the explicit
	// socket-closing watcher only after Connect succeeds so cancellation cannot
	// consume the idempotent close guard before a socket exists.
	if err := s.Connect(); err != nil {
		_ = s.Close()
		return CollectionResult{}, classifyTransportError(ctx, err)
	}

	var closeOnce sync.Once
	closeSession := func() {
		closeOnce.Do(func() { _ = s.Close() })
	}
	stopWatcher := make(chan struct{})
	watcherDone := make(chan struct{})
	c.stats.activeWatchers.Add(1)
	go func() {
		defer c.stats.activeWatchers.Add(-1)
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			closeSession()
		case <-stopWatcher:
		}
	}()
	defer func() {
		close(stopWatcher)
		<-watcherDone
		closeSession()
	}()

	if err := contextError(ctx); err != nil {
		return CollectionResult{}, err
	}

	collectedAt := c.now().UTC()
	result := CollectionResult{
		SchemaVersion: ResultSchemaVersion,
		Version:       req.Version,
		Operation:     req.Operation,
		CollectedAt:   collectedAt,
		Observations:  make([]Observation, 0, min(req.Limits.MaxResults, len(req.OIDs))),
	}

	resultBytes := 0
	appendBounded := func(sourceOID string, pdu gosnmp.SnmpPDU, walkEntry bool) error {
		if walkEntry && len(result.Observations) >= req.Limits.MaxWalkEntries {
			return newError(CodeResourceLimit, "walk entry count exceeds configured maximum", nil, false)
		}
		if len(result.Observations) >= req.Limits.MaxResults {
			return newError(CodeResourceLimit, "result count exceeds configured maximum", nil, false)
		}
		observation, err := normalizePDU(sourceOID, pdu, collectedAt)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(observation)
		if err != nil {
			return newError(CodeMalformedResponse, "normalized observation cannot be encoded", err, false)
		}
		if len(encoded) > req.Limits.MaxResultBytes-resultBytes {
			return newError(CodeResourceLimit, "normalized result bytes exceed configured maximum", nil, false)
		}
		resultBytes += len(encoded)
		result.Observations = append(result.Observations, observation)
		return nil
	}

	switch req.Operation {
	case OperationGet:
		packet, err := s.Get(req.OIDs)
		if err != nil {
			return CollectionResult{}, classifyTransportError(ctx, err)
		}
		if err := appendPacket(req, packet, appendBounded, false); err != nil {
			return CollectionResult{}, err
		}
	case OperationGetNext:
		packet, err := s.GetNext(req.OIDs)
		if err != nil {
			return CollectionResult{}, classifyTransportError(ctx, err)
		}
		if err := appendPacket(req, packet, appendBounded, false); err != nil {
			return CollectionResult{}, err
		}
	case OperationGetBulk:
		packet, err := s.GetBulk(req.OIDs, 0, req.Limits.MaxBulkRepetitions)
		if err != nil {
			return CollectionResult{}, classifyTransportError(ctx, err)
		}
		if err := appendPacket(req, packet, appendBounded, true); err != nil {
			return CollectionResult{}, err
		}
	case OperationWalk:
		root := req.OIDs[0]
		if err := s.Walk(root, func(pdu gosnmp.SnmpPDU) error { return appendBounded(root, pdu, true) }); err != nil {
			return CollectionResult{}, classifyOperationError(ctx, err)
		}
	case OperationBulkWalk:
		root := req.OIDs[0]
		if err := s.BulkWalk(root, func(pdu gosnmp.SnmpPDU) error { return appendBounded(root, pdu, true) }); err != nil {
			return CollectionResult{}, classifyOperationError(ctx, err)
		}
	default:
		return CollectionResult{}, newError(CodeInvalidRequest, "unsupported SNMP operation", nil, false)
	}

	encodedResult, err := json.Marshal(result)
	if err != nil {
		return CollectionResult{}, newError(CodeMalformedResponse, "normalized collection result cannot be encoded", err, false)
	}
	if len(encodedResult) > req.Limits.MaxResultBytes {
		return CollectionResult{}, newError(CodeResourceLimit, "normalized result bytes exceed configured maximum", nil, false)
	}
	return result, nil
}

func appendPacket(req Request, packet *gosnmp.SnmpPacket, appendFn func(string, gosnmp.SnmpPDU, bool) error, bulk bool) error {
	if packet == nil {
		return newError(CodeMalformedResponse, "SNMP response packet is nil", nil, false)
	}
	if err := packetError(packet.Error); err != nil {
		return err
	}
	if !bulk && len(packet.Variables) > len(req.OIDs) {
		return newError(CodeMalformedResponse, "SNMP response contains more variables than requested", nil, false)
	}
	if len(packet.Variables) == 0 {
		return newError(CodeMalformedResponse, "SNMP response contains no variables", nil, false)
	}
	for i, pdu := range packet.Variables {
		var source string
		if bulk {
			source = req.OIDs[i%len(req.OIDs)]
		} else {
			source = req.OIDs[i]
		}
		if err := appendFn(source, pdu, false); err != nil {
			return err
		}
	}
	return nil
}

func packetError(status gosnmp.SNMPError) error {
	switch status {
	case gosnmp.NoError:
		return nil
	case gosnmp.NoAccess, gosnmp.AuthorizationError:
		return newError(CodeAuthorizationFailed, "SNMP agent denied access", nil, false)
	case gosnmp.TooBig:
		return newError(CodeResourceLimit, "SNMP agent reported response too large", nil, false)
	case gosnmp.NoSuchName:
		return newError(CodeNoSuchObject, "SNMP agent reported no such name", nil, false)
	default:
		return newError(CodeProtocolError, "SNMP agent returned protocol error", nil, false)
	}
}

func classifyOperationError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	return classifyTransportError(ctx, err)
}

func classifyTransportError(ctx context.Context, err error) error {
	if ctxErr := contextError(ctx); ctxErr != nil {
		return ctxErr
	}
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, gosnmp.ErrUnknownUsername), errors.Is(err, gosnmp.ErrWrongDigest), errors.Is(err, gosnmp.ErrDecryption):
		return newError(CodeAuthenticationFailed, "SNMPv3 authentication or privacy verification failed", err, false)
	case errors.Is(err, gosnmp.ErrUnknownSecurityLevel), errors.Is(err, gosnmp.ErrUnknownSecurityModels):
		return newError(CodeUnsupportedSecurity, "SNMPv3 security mode is unsupported", err, false)
	case errors.Is(err, gosnmp.ErrInvalidMsgs):
		return newError(CodeMalformedResponse, "SNMP response is malformed", err, false)
	case errors.Is(err, gosnmp.ErrUnknownPDUHandlers), errors.Is(err, gosnmp.ErrUnknownReportPDU), errors.Is(err, gosnmp.ErrNotInTimeWindow), errors.Is(err, gosnmp.ErrUnknownEngineID):
		return newError(CodeProtocolError, "SNMP protocol negotiation failed", err, false)
	case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EHOSTUNREACH):
		return newError(CodeNetworkUnreachable, "network path to SNMP target is unreachable", err, true)
	case errors.Is(err, syscall.ECONNREFUSED):
		return newError(CodeTransportFailure, "SNMP transport connection was refused", err, true)
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return newError(CodeTimeout, "SNMP request timed out", err, true)
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out") {
		return newError(CodeTimeout, "SNMP request timed out", err, true)
	}
	if strings.Contains(lower, "not authentic") {
		return newError(CodeAuthenticationFailed, "SNMPv3 authentication verification failed", err, false)
	}
	return newError(CodeTransportFailure, "SNMP transport operation failed", err, true)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return newError(CodeInvalidRequest, "context must not be nil", nil, false)
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return newError(CodeDeadlineExceeded, "SNMP execution deadline exceeded", context.DeadlineExceeded, false)
		}
		return newError(CodeCancelled, "SNMP execution cancelled", context.Canceled, false)
	}
	return nil
}

type goSNMPSession struct {
	client *gosnmp.GoSNMP
}

func newGoSNMPSession(ctx context.Context, req Request) (session, error) {
	version, err := goSNMPVersion(req.Version)
	if err != nil {
		return nil, err
	}
	client := &gosnmp.GoSNMP{
		Target:             req.Target,
		Port:               uint16(req.Port),
		Transport:          "udp",
		Version:            version,
		Context:            ctx,
		Timeout:            req.Timeout,
		Retries:            0,
		ExponentialTimeout: false,
		MaxOids:            req.Limits.MaxOIDs,
		MaxRepetitions:     req.Limits.MaxBulkRepetitions,
	}

	switch req.Version {
	case Version1, Version2c:
		client.Community = req.Credentials.Community.Community.value()
	case Version3:
		// GoSNMP enables a process-global password-derived-key cache by default.
		// Disable it for every Platform v3 session so credentials are never retained
		// or shared across jobs/tenants through that cache. The upstream function is
		// concurrency-safe and clears the cache when disabling it.
		gosnmp.PasswordCaching(false)
		v3 := req.Credentials.V3
		level, err := goSNMPSecurityLevel(v3.SecurityLevel)
		if err != nil {
			return nil, err
		}
		auth, err := goSNMPAuthProtocol(v3.AuthProtocol)
		if err != nil {
			return nil, err
		}
		privacy, err := goSNMPPrivacyProtocol(v3.PrivacyProtocol)
		if err != nil {
			return nil, err
		}
		client.MsgFlags = level
		client.SecurityModel = gosnmp.UserSecurityModel
		client.ContextName = v3.ContextName
		client.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 v3.Username,
			AuthenticationProtocol:   auth,
			AuthenticationPassphrase: v3.AuthSecret.value(),
			PrivacyProtocol:          privacy,
			PrivacyPassphrase:        v3.PrivacySecret.value(),
		}
	}
	return &goSNMPSession{client: client}, nil
}

func (s *goSNMPSession) Connect() error { return s.client.Connect() }
func (s *goSNMPSession) Close() error   { return s.client.Close() }
func (s *goSNMPSession) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	return s.client.Get(oids)
}
func (s *goSNMPSession) GetNext(oids []string) (*gosnmp.SnmpPacket, error) {
	return s.client.GetNext(oids)
}
func (s *goSNMPSession) GetBulk(oids []string, nonRepeaters uint8, maxRepetitions uint32) (*gosnmp.SnmpPacket, error) {
	return s.client.GetBulk(oids, nonRepeaters, maxRepetitions)
}
func (s *goSNMPSession) Walk(rootOID string, walkFn gosnmp.WalkFunc) error {
	return s.client.Walk(rootOID, walkFn)
}
func (s *goSNMPSession) BulkWalk(rootOID string, walkFn gosnmp.WalkFunc) error {
	return s.client.BulkWalk(rootOID, walkFn)
}

func goSNMPVersion(version Version) (gosnmp.SnmpVersion, error) {
	switch version {
	case Version1:
		return gosnmp.Version1, nil
	case Version2c:
		return gosnmp.Version2c, nil
	case Version3:
		return gosnmp.Version3, nil
	default:
		return 0, newError(CodeInvalidRequest, "unsupported SNMP version", nil, false)
	}
}

func goSNMPSecurityLevel(level SecurityLevel) (gosnmp.SnmpV3MsgFlags, error) {
	switch level {
	case SecurityNoAuthNoPriv:
		return gosnmp.NoAuthNoPriv, nil
	case SecurityAuthNoPriv:
		return gosnmp.AuthNoPriv, nil
	case SecurityAuthPriv:
		return gosnmp.AuthPriv, nil
	default:
		return 0, newError(CodeUnsupportedSecurity, "unsupported SNMPv3 security level", nil, false)
	}
}

func goSNMPAuthProtocol(protocol AuthProtocol) (gosnmp.SnmpV3AuthProtocol, error) {
	switch protocol {
	case AuthNone:
		return gosnmp.NoAuth, nil
	case AuthMD5:
		return gosnmp.MD5, nil
	case AuthSHA1:
		return gosnmp.SHA, nil
	case AuthSHA224:
		return gosnmp.SHA224, nil
	case AuthSHA256:
		return gosnmp.SHA256, nil
	case AuthSHA384:
		return gosnmp.SHA384, nil
	case AuthSHA512:
		return gosnmp.SHA512, nil
	default:
		return 0, newError(CodeUnsupportedSecurity, "unsupported SNMPv3 authentication protocol", nil, false)
	}
}

func goSNMPPrivacyProtocol(protocol PrivacyProtocol) (gosnmp.SnmpV3PrivProtocol, error) {
	switch protocol {
	case PrivacyNone:
		return gosnmp.NoPriv, nil
	case PrivacyDES:
		return gosnmp.DES, nil
	case PrivacyAES:
		return gosnmp.AES, nil
	case PrivacyAES192:
		return gosnmp.AES192, nil
	case PrivacyAES256:
		return gosnmp.AES256, nil
	case PrivacyAES192C:
		return gosnmp.AES192C, nil
	case PrivacyAES256C:
		return gosnmp.AES256C, nil
	default:
		return 0, newError(CodeUnsupportedSecurity, "unsupported SNMPv3 privacy protocol", nil, false)
	}
}

func waitWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func jitterDuration(base time.Duration, fraction float64) time.Duration {
	if base <= 0 || fraction <= 0 {
		return base
	}
	factor := 1 + ((rand.Float64()*2)-1)*fraction
	value := float64(base) * factor
	if value < 0 {
		return 0
	}
	if value > float64(math.MaxInt64) {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(value)
}

func backoffDuration(policy RetryPolicy, retryNumber int, jitter func(time.Duration, float64) time.Duration) time.Duration {
	if retryNumber <= 0 || policy.MaxAttempts <= 1 {
		return 0
	}
	delay := policy.InitialBackoff
	for i := 1; i < retryNumber; i++ {
		if delay >= policy.MaxBackoff/2 {
			delay = policy.MaxBackoff
			break
		}
		delay *= 2
	}
	if delay > policy.MaxBackoff {
		delay = policy.MaxBackoff
	}
	delay = jitter(delay, policy.JitterFraction)
	if delay < 0 {
		return 0
	}
	if delay > policy.MaxBackoff {
		return policy.MaxBackoff
	}
	return delay
}
