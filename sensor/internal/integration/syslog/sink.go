package syslogintegration

import (
	"context"
	"errors"
	"strings"
	"sync"

	"platform/sensor/internal/observation"
	protocolsyslog "platform/sensor/internal/protocol/syslog"
	"platform/sensor/internal/spool"
)

const RecordTypeSyslogEvent = "SYSLOG_EVENT"

type SpoolAppender interface {
	Append(spool.AppendRequest) (spool.Record, error)
}

type SinkConfig struct {
	SensorID         string
	TenantID         string
	MaxEventBytes    int
	MaxEnvelopeBytes int
}

func (c SinkConfig) Validate() error {
	if strings.TrimSpace(c.SensorID) == "" || len(c.SensorID) > 256 {
		return &Error{Code: CodeInvalidConfig, Message: "Syslog Sensor ID is required and must be within envelope bounds"}
	}
	if strings.TrimSpace(c.TenantID) == "" || len(c.TenantID) > 256 {
		return &Error{Code: CodeInvalidConfig, Message: "Syslog tenant ID is required and must be within envelope bounds"}
	}
	if c.MaxEventBytes <= 0 || c.MaxEnvelopeBytes <= 0 || c.MaxEnvelopeBytes <= c.MaxEventBytes {
		return &Error{Code: CodeInvalidConfig, Message: "Syslog event and envelope bounds are invalid"}
	}
	return nil
}

type SinkSnapshot struct {
	Degraded      bool
	LastErrorCode ErrorCode
}

type DurableSink struct {
	cfg      SinkConfig
	parser   protocolsyslog.ParserConfig
	spool    SpoolAppender
	mu       sync.RWMutex
	degraded bool
	lastErr  ErrorCode
}

func NewDurableSink(cfg SinkConfig, parser protocolsyslog.ParserConfig, durable SpoolAppender) (*DurableSink, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := parser.Validate(); err != nil {
		return nil, err
	}
	if durable == nil {
		return nil, &Error{Code: CodeInvalidConfig, Message: "durable Syslog spool is required"}
	}
	return &DurableSink{cfg: cfg, parser: parser, spool: durable}, nil
}

func (s *DurableSink) Snapshot() SinkSnapshot {
	if s == nil {
		return SinkSnapshot{Degraded: true, LastErrorCode: CodeStorageUnavailable}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return SinkSnapshot{Degraded: s.degraded, LastErrorCode: s.lastErr}
}

func (s *DurableSink) Accept(ctx context.Context, event protocolsyslog.Event) error {
	if s == nil {
		return &Error{Code: CodeStorageUnavailable, Message: "durable Syslog sink is not initialized"}
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	eventPayload, err := protocolsyslog.EncodeEvent(event, s.cfg.MaxEventBytes, s.parser)
	if err != nil {
		if protocolsyslog.CodeOf(err) == protocolsyslog.CodeResourceLimit {
			return &Error{Code: CodeResourceLimit, Message: "normalized Syslog event exceeds configured durable bound", Cause: err}
		}
		return &Error{Code: CodeInternal, Message: "normalized Syslog event could not be encoded", Cause: err}
	}
	env := observation.EventEnvelope{
		SchemaVersion: observation.EventEnvelopeSchemaVersion,
		ReceivedAt:    event.ReceivedAt,
		EventTime:     event.EventTime,
		Source:        observation.EventSource{SensorID: s.cfg.SensorID, TenantID: s.cfg.TenantID, Transport: string(event.Source.Transport), SourceAddress: event.Source.Address},
		Protocol:      "SYSLOG", Format: string(event.Format), Quality: string(event.Quality), StatusCode: string(event.StatusCode),
		PayloadSchemaVersion: event.SchemaVersion, Payload: eventPayload,
	}
	encoded, err := observation.EncodeEventEnvelope(env, s.cfg.MaxEnvelopeBytes, s.cfg.MaxEventBytes)
	if err != nil {
		var envelopeErr *observation.EventEnvelopeError
		if errors.As(err, &envelopeErr) && envelopeErr.Code == observation.EventEnvelopeResourceLimit {
			return &Error{Code: CodeResourceLimit, Message: "encoded Syslog event envelope exceeds configured bound", Cause: err}
		}
		return &Error{Code: CodeInternal, Message: "Syslog event envelope could not be encoded", Cause: err}
	}
	if err := contextError(ctx); err != nil {
		return err
	}
	_, err = s.spool.Append(spool.AppendRequest{Class: spool.ClassSyslog, Type: RecordTypeSyslogEvent, Payload: encoded, CreatedAt: event.ReceivedAt})
	if err != nil {
		mapped := mapSpoolError(err)
		s.setFailure(CodeOf(mapped))
		return mapped
	}
	s.clearFailure()
	return nil
}

func mapSpoolError(err error) error {
	switch spool.CodeOf(err) {
	case spool.CodeProtection:
		return &Error{Code: CodeStorageProtection, Message: "Syslog durable spool rejected the event at the storage-protection boundary", Cause: err}
	case spool.CodeCorruption, spool.CodeSequence, spool.CodeUnsafePath:
		return &Error{Code: CodeStorageCorruption, Message: "Syslog durable spool integrity is degraded", Cause: err}
	case spool.CodeInvalidRecord:
		return &Error{Code: CodeResourceLimit, Message: "Syslog durable record exceeds spool payload bounds", Cause: err}
	case spool.CodeClosed, spool.CodeIO, spool.CodeInvalidConfig, spool.CodeUnsafeLegacyAPI:
		return &Error{Code: CodeStorageUnavailable, Message: "Syslog durable spool is unavailable", Cause: err}
	default:
		return &Error{Code: CodeStorageUnavailable, Message: "Syslog durable spool append failed", Cause: err}
	}
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &Error{Code: CodeDeadlineExceeded, Message: "Syslog durable sink deadline exceeded", Cause: ctx.Err()}
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return &Error{Code: CodeCancelled, Message: "Syslog durable sink cancelled", Cause: ctx.Err()}
	}
	return nil
}

func (s *DurableSink) setFailure(code ErrorCode) {
	s.mu.Lock()
	s.degraded = true
	s.lastErr = code
	s.mu.Unlock()
}
func (s *DurableSink) clearFailure() { s.mu.Lock(); s.degraded = false; s.mu.Unlock() }
