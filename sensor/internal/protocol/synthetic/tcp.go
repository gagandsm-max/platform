package synthetic

import (
	"context"
	"strings"
	"time"
)

func (c *Collector) TCP(parent context.Context, r TCPRequest) (TCPResult, error) {
	var result TCPResult
	if c == nil {
		return result, newError(CodeInvalidRequest, "synthetic collector is nil", nil, false)
	}
	started := c.now().UTC()
	result = TCPResult{SchemaVersion: ResultSchemaVersion, Target: r.Target, Port: r.Port, StartedAt: started}
	err := c.validateTCP(r)
	if err != nil {
		result.ErrorCode = CodeOf(err)
		result.CompletedAt = c.now().UTC()
		c.record(err)
		return result, err
	}
	ctx, cancel, err := c.operationContext(parent, r.Timeout)
	if err != nil {
		result.ErrorCode = CodeOf(err)
		result.CompletedAt = c.now().UTC()
		c.record(err)
		return result, err
	}
	defer cancel()
	begin := time.Now()
	conn, dialErr := c.tcpDial(ctx, "tcp", joinAddress(strings.TrimSpace(r.Target), r.Port))
	elapsed := time.Since(begin)
	if dialErr != nil {
		err = classifyTransport(parent, ctx, dialErr)
		result.DurationMicros = elapsed.Microseconds()
		result.CompletedAt = c.now().UTC()
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	closeErr := conn.Close()
	result.DurationMicros = elapsed.Microseconds()
	result.CompletedAt = c.now().UTC()
	if closeErr != nil {
		err = classifyTransport(parent, ctx, closeErr)
		result.ErrorCode = CodeOf(err)
		c.record(err)
		return result, err
	}
	result.Success = true
	c.record(nil)
	return result, nil
}

func (c *Collector) validateTCP(r TCPRequest) error {
	if err := validateHost(r.Target, c.limits.MaxTargetBytes); err != nil {
		return err
	}
	if err := validatePort(r.Port); err != nil {
		return err
	}
	if r.Timeout <= 0 {
		return newError(CodeInvalidRequest, "TCP timeout must be positive", nil, false)
	}
	if r.Timeout > c.limits.MaxTimeout {
		return newError(CodeResourceLimit, "TCP timeout exceeds configured bound", nil, false)
	}
	return nil
}
