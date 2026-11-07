package scheduler

import (
	"encoding/json"
	"errors"

	"platform/agent/common/queue"
	"platform/agent/common/telemetry"
)

const (
	// In addition to the existing per-collector item bound, cap retained encoded
	// contribution bytes and the serialized retry backlog. These are memory
	// bounds, not new durable stores or promises of crash-safe pre-admission data.
	maxPendingContributionBytes  = 1 << 20
	maxAdmissionBacklogBytes     = 1 << 20
	maxAdmissionBacklogFragments = 256
	defaultRecordBytes           = 64 * 1024
)

var ErrPendingAdmission = errors.New("SCHEDULER_ADMISSION_INCOMPLETE: unaccepted memory backlog or queue recovery required")

// AdmissionStatus separates local queue custody, explicit loss, and uncertain
// custody. Memory backlog does not survive a process crash. Durable counts are
// not Sensor ACKs; only the existing delivery worker may commit an ACK.
type AdmissionStatus struct {
	DurableFragments      uint64 `json:"durable_fragments"`
	DurableObservations   uint64 `json:"durable_observations"`
	DurableErrors         uint64 `json:"durable_errors"`
	FailedAttempts        uint64 `json:"failed_attempts"`
	RejectedAttempts      uint64 `json:"rejected_attempts"`
	DroppedObservations   uint64 `json:"dropped_observations"`
	DroppedErrors         uint64 `json:"dropped_errors"`
	UncertainObservations uint64 `json:"uncertain_observations"`
	UncertainErrors       uint64 `json:"uncertain_errors"`
	BacklogFragments      int    `json:"backlog_fragments"`
	BacklogBytes          int    `json:"backlog_bytes"`
	BacklogObservations   int    `json:"backlog_observations"`
	BacklogErrors         int    `json:"backlog_errors"`
	RecoveryRequired      bool   `json:"recovery_required"`
}

type collectionFragment struct {
	payload              []byte
	observations, errors int
	collectors           map[string]struct{}
}

func contributionBytes(c pendingContribution) int {
	// Encoding errors are handled per item by fragmentation, so one invalid item
	// cannot discard valid siblings. Retained Go values already have the existing
	// normalization limits; this additionally bounds their encoded representation.
	n := 0
	for _, o := range c.observations {
		b, err := json.Marshal(o)
		if err == nil {
			n += len(b)
		}
		if n > maxPendingContributionBytes {
			return n
		}
	}
	for _, e := range c.errors {
		b, _ := json.Marshal(e)
		n += len(b)
		if n > maxPendingContributionBytes {
			return n
		}
	}
	return n
}

// boundContribution skips an item that cannot fit without starving smaller
// siblings. Do not retain an invalid or arbitrarily large item in pending memory.
func (s *Scheduler) boundContribution(c pendingContribution) pendingContribution {
	out := pendingContribution{name: c.name, collectedAt: c.collectedAt, durationMs: c.durationMs}
	droppedObs, droppedErrs := 0, 0
	for _, o := range c.observations {
		b, err := json.Marshal(o)
		if err != nil || len(b) > maxPendingContributionBytes-out.encodedBytes {
			droppedObs++
			continue
		}
		out.observations = append(out.observations, o)
		out.encodedBytes += len(b)
	}
	for _, e := range c.errors {
		b, err := json.Marshal(e)
		if err != nil || len(b) > maxPendingContributionBytes-out.encodedBytes {
			droppedErrs++
			continue
		}
		out.errors = append(out.errors, e)
		out.encodedBytes += len(b)
	}
	if droppedObs+droppedErrs > 0 {
		s.recordAdmissionDrop(droppedObs, droppedErrs, "pending_encoding_or_byte_bound")
	}
	return out
}

func (s *Scheduler) recordAdmissionDrop(obs, errs int, reason string) {
	s.mu.Lock()
	s.admissionStatus.DroppedObservations += uint64(obs)
	s.admissionStatus.DroppedErrors += uint64(errs)
	s.runErrors++
	s.mu.Unlock()
	s.logger.Warn("Collection items dropped before durable admission", "event_code", "AGENT_COLLECTION_ADMISSION_DROPPED", "result", "DROPPED", "safe_context", map[string]any{"reason_class": reason, "observation_count": obs, "error_count": errs})
}

func (s *Scheduler) recordLimit() int {
	if q, ok := s.enqueuer.(interface{ MaxAdmissionPayloadBytes() int }); ok {
		return q.MaxAdmissionPayloadBytes()
	}
	if q, ok := s.enqueuer.(interface{ MaxRecordBytes() int }); ok && q.MaxRecordBytes() > 0 {
		return q.MaxRecordBytes()
	}
	return defaultRecordBytes
}

// fragmentCollection marshals each item once for budgeting, then each complete
// fragment once. It avoids repeatedly encoding an ever-growing full collection.
// All fragments inherit the original cycle metadata, IDs and observation times.
func (s *Scheduler) fragmentCollection(batch telemetry.CollectionBatch) {
	limit := s.recordLimit()
	observations, errs := batch.Observations, batch.Errors
	batch.Observations = []telemetry.Observation{}
	batch.Errors = nil
	base, err := json.Marshal(batch)
	if err != nil || len(base) > limit {
		s.recordAdmissionDrop(len(observations), len(errs), "envelope_too_large")
		return
	}
	size := len(base)
	emit := func() {
		if len(batch.Observations)+len(batch.Errors) == 0 {
			return
		}
		payload, err := json.Marshal(batch)
		if err != nil || len(payload) > limit {
			s.recordAdmissionDrop(len(batch.Observations), len(batch.Errors), "encoding_failed")
		} else if len(s.backlog) >= maxAdmissionBacklogFragments || s.backlogBytes+len(payload) > maxAdmissionBacklogBytes {
			s.recordAdmissionDrop(len(batch.Observations), len(batch.Errors), "backlog_bound")
		} else {
			names := make(map[string]struct{})
			for _, o := range batch.Observations {
				names[o.Collector] = struct{}{}
			}
			for _, e := range batch.Errors {
				names[e.Collector] = struct{}{}
			}
			s.backlog = append(s.backlog, collectionFragment{payload: payload, observations: len(batch.Observations), errors: len(batch.Errors), collectors: names})
			s.backlogBytes += len(payload)
			s.drainBacklog()
		}
		batch.Observations = []telemetry.Observation{}
		batch.Errors = nil
		size = len(base)
	}
	for _, o := range observations {
		b, err := json.Marshal(o)
		if err != nil {
			s.recordAdmissionDrop(1, 0, "invalid_observation")
			continue
		}
		cost := len(b)
		if len(batch.Observations) > 0 {
			cost++
		}
		if size+cost > limit {
			emit()
			cost = len(b)
		}
		if size+cost > limit {
			s.recordAdmissionDrop(1, 0, "observation_too_large")
			continue
		}
		batch.Observations = append(batch.Observations, o)
		size += cost
	}
	for _, e := range errs {
		b, err := json.Marshal(e)
		if err != nil {
			s.recordAdmissionDrop(0, 1, "invalid_collector_error")
			continue
		}
		cost := len(b) + 1
		if len(batch.Errors) == 0 {
			cost = len(b) + len(`,"errors":[]`)
		}
		if size+cost > limit {
			emit()
			cost = len(b) + len(`,"errors":[]`)
		}
		if size+cost > limit {
			s.recordAdmissionDrop(0, 1, "collector_error_too_large")
			continue
		}
		batch.Errors = append(batch.Errors, e)
		size += cost
	}
	emit()
}

func (s *Scheduler) drainBacklog() {
	s.mu.Lock()
	blocked := s.admissionStatus.RecoveryRequired
	s.mu.Unlock()
	if blocked || s.admissionPaused {
		return
	}
	for len(s.backlog) > 0 {
		f := s.backlog[0]
		_, err := s.enqueuer.Enqueue(f.payload)
		disposition, recovery := queue.AdmissionOutcome(err)
		s.mu.Lock()
		if disposition == queue.DurablyAdmitted {
			s.admissionStatus.DurableFragments++
			s.admissionStatus.DurableObservations += uint64(f.observations)
			s.admissionStatus.DurableErrors += uint64(f.errors)
		}
		if err != nil {
			s.runErrors++
			s.admissionStatus.FailedAttempts++
		}
		if disposition == queue.NotAdmitted {
			s.admissionStatus.RejectedAttempts++
		}
		if disposition == queue.AdmissionUncertain {
			s.admissionStatus.UncertainObservations += uint64(f.observations)
			s.admissionStatus.UncertainErrors += uint64(f.errors)
			recovery = true
		}
		if recovery {
			s.admissionStatus.RecoveryRequired = true
		}
		s.mu.Unlock()
		if err != nil {
			for name := range f.collectors {
				if rt := s.runtimeByName(name); rt != nil {
					rt.mu.Lock()
					rt.errorCount++
					rt.lastError = "enqueue_failed"
					rt.mu.Unlock()
				}
			}
			s.logger.Error("Collection fragment admission failed", "event_code", "AGENT_COLLECTION_ENQUEUE_FAILED", "result", string(disposition), "safe_context", map[string]any{"encoded_bytes": len(f.payload), "observation_count": f.observations, "error_count": f.errors, "recovery_required": recovery, "reason_class": admissionReason(err)})
		}
		if disposition == queue.DurablyAdmitted {
			s.backlogBytes -= len(f.payload)
			s.backlog[0] = collectionFragment{}
			s.backlog = s.backlog[1:]
		} else if errors.Is(err, queue.ErrRecordTooLarge) && !recovery {
			// A legacy/non-limit-aware enqueuer rejected the complete record. Do not
			// retry it forever or infer a larger receiver budget. Report exact loss.
			s.recordAdmissionDrop(f.observations, f.errors, "queue_record_limit")
			s.backlogBytes -= len(f.payload)
			s.backlog[0] = collectionFragment{}
			s.backlog = s.backlog[1:]
			continue
		} else {
			s.admissionPaused = true
			return
		}
		if recovery {
			return
		}
	}
	s.backlog = nil
}

func admissionReason(err error) string {
	switch {
	case errors.Is(err, queue.ErrQueueFull):
		return "queue_full"
	case errors.Is(err, queue.ErrRecordTooLarge):
		return "record_too_large"
	case errors.Is(err, queue.ErrMetaUncertain):
		return "meta_uncertain"
	case errors.Is(err, queue.ErrQuarantineUncertain):
		return "location_uncertain"
	case errors.Is(err, queue.ErrSequenceExhausted):
		return "sequence_exhausted"
	default:
		return "queue_io_or_unknown"
	}
}

func (s *Scheduler) publishBacklog() {
	obs, errs := 0, 0
	for _, f := range s.backlog {
		obs += f.observations
		errs += f.errors
	}
	s.mu.Lock()
	s.admissionStatus.BacklogFragments = len(s.backlog)
	s.admissionStatus.BacklogBytes = s.backlogBytes
	s.admissionStatus.BacklogObservations = obs
	s.admissionStatus.BacklogErrors = errs
	s.mu.Unlock()
}
