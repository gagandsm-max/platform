package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	"platform/agent/common/queue"
)

type FailureClass string

const (
	FailureTemporary           FailureClass = "TEMPORARY"
	FailureAuthentication      FailureClass = "AUTHENTICATION"
	FailurePermanent           FailureClass = "PERMANENT"
	FailureReceiverUnavailable FailureClass = "RECEIVER_UNAVAILABLE"
	// FailureOversized is a deterministic payload that the Sensor receiver
	// cannot accept (HTTP 413 / envelope over the 512 KiB contract). The local
	// queue must be retained; the delivery loop must not hot-retry it.
	FailureOversized FailureClass = "OVERSIZED"
	// FailureBlocked is a durable Agent-local delivery hold: a quarantined
	// sequence occupies the stream and later records must not be submitted.
	FailureBlocked FailureClass = "BLOCKED"
)

type DeliveryError struct {
	Code  string
	Class FailureClass
	Err   error
}

func (e *DeliveryError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return string(e.Class)
	}
	return e.Code + ": " + string(e.Class)
}

func (e *DeliveryError) Unwrap() error { return e.Err }

type Batch struct {
	CorrelationID string
	AgentID       string
	BatchID       string
	Records       []queue.Record
}

type Ack struct {
	AcceptedThrough uint64
	AgentID         string
	BatchID         string
}

// Transport is the Agent→Sensor delivery boundary. Production wiring uses
// HTTPSTransport against the Sensor Agent Receiver. Tests may inject
// UnavailableTransport or a fake Transport.
type Transport interface {
	Available() bool
	Deliver(context.Context, Batch) (Ack, error)
}

type UnavailableTransport struct {
	code string
}

func NewUnavailableTransport(code string) *UnavailableTransport {
	if code == "" {
		code = "AGENT_RECEIVER_NOT_IMPLEMENTED"
	}
	return &UnavailableTransport{code: code}
}

func (t *UnavailableTransport) Available() bool { return false }

func (t *UnavailableTransport) Deliver(context.Context, Batch) (Ack, error) {
	return Ack{}, &DeliveryError{Code: t.code, Class: FailureReceiverUnavailable}
}

func Classify(err error) FailureClass {
	if err == nil {
		return ""
	}
	if errors.Is(err, queue.ErrDeliveryBlocked) {
		return FailureBlocked
	}
	if errors.Is(err, queue.ErrQuarantineFull) {
		return FailureBlocked
	}
	if errors.Is(err, queue.ErrQuarantineDestExists) {
		return FailureBlocked
	}
	if errors.Is(err, queue.ErrUnknownSequence) {
		return FailureBlocked
	}
	if errors.Is(err, queue.ErrQuarantineUncertain) {
		return FailureBlocked
	}
	if errors.Is(err, queue.ErrQuarantineSourceSyncFailed) {
		return FailureBlocked
	}
	if errors.Is(err, queue.ErrSequenceGap) {
		return FailureBlocked
	}
	if errors.Is(err, queue.ErrSequenceConflict) {
		return FailureBlocked
	}
	if errors.Is(err, queue.ErrMetaUncertain) {
		return FailureBlocked
	}
	if errors.Is(err, queue.ErrAckRecoveryConflict) {
		return FailureBlocked
	}
	var de *DeliveryError
	if errors.As(err, &de) && de.Class != "" {
		return de.Class
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return FailureTemporary
	}
	return FailureTemporary
}

func CorrelationID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("DELIVERY_CORRELATION_ID_FAILED: %w", err)
	}
	return "corr-" + hex.EncodeToString(buf), nil
}

// Backoff returns bounded exponential backoff with caller-supplied jitter in
// [-1, 1]. Keeping jitter injectable makes retry behavior deterministic in tests.
func Backoff(base, max time.Duration, attempt int, jitter float64) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if max < base {
		max = base
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 30 {
		attempt = 30
	}
	factor := math.Pow(2, float64(attempt))
	d := time.Duration(float64(base) * factor)
	if d > max || d < 0 {
		d = max
	}
	if jitter < -1 {
		jitter = -1
	}
	if jitter > 1 {
		jitter = 1
	}
	// +/-20% prevents synchronized retry storms while retaining a hard cap.
	jd := time.Duration(float64(d) * (1 + 0.20*jitter))
	if jd < base/2 {
		jd = base / 2
	}
	if jd > max {
		jd = max
	}
	return jd
}
