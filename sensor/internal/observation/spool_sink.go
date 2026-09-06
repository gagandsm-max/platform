package observation

import (
	"context"
	"fmt"

	"platform/sensor/internal/spool"
)

type Receipt struct {
	Sequence uint64 `json:"sequence"`
	RecordID string `json:"recordId"`
}

type Sink interface {
	Write(context.Context, Envelope) (Receipt, error)
	MaxPayloadBytes() int
}

type DiskSink struct {
	spool           *spool.DiskSpool
	maxPayloadBytes int
}

func NewDiskSink(durable *spool.DiskSpool, maxPayloadBytes int) (*DiskSink, error) {
	if durable == nil {
		return nil, &Error{Code: CodeInvalidEnvelope, Message: "durable spool is required"}
	}
	if maxPayloadBytes <= 0 {
		return nil, &Error{Code: CodeResourceLimit, Message: "observation sink payload bound must be positive"}
	}
	return &DiskSink{spool: durable, maxPayloadBytes: maxPayloadBytes}, nil
}

func (s *DiskSink) MaxPayloadBytes() int {
	if s == nil {
		return 0
	}
	return s.maxPayloadBytes
}

func (s *DiskSink) Write(ctx context.Context, envelope Envelope) (Receipt, error) {
	if s == nil || s.spool == nil {
		return Receipt{}, &Error{Code: CodeInvalidEnvelope, Message: "observation sink is not initialized"}
	}
	payload, err := EncodeBounded(ctx, envelope, s.maxPayloadBytes)
	if err != nil {
		return Receipt{}, err
	}
	if err := contextError(ctx); err != nil {
		return Receipt{}, err
	}
	record, err := s.spool.Append(spool.AppendRequest{
		Class:     spool.ClassMetric,
		Type:      envelope.RecordType,
		Payload:   payload,
		CreatedAt: envelope.CollectedAt,
	})
	if err != nil {
		return Receipt{}, fmt.Errorf("observation spool append failed: %w", err)
	}
	// Append is a synchronous durable boundary. If ctx becomes cancelled during
	// the bounded fsync/rename operation, successful return from Append means the
	// record is already durable and must not be reported as absent.
	return Receipt{Sequence: record.Sequence, RecordID: record.ID}, nil
}
