package recurring

import (
	"sync"
	"time"
)

// DueWork is one coalesced current-due submission candidate.
type DueWork struct {
	Definition    RecurringDefinition
	CurrentDue    time.Time
	MissedSkipped int64
	FreshnessDebt bool
}

// AssignmentRuntime is per-assignment due-engine state (not durable).
type AssignmentRuntime struct {
	LastSubmittedDue time.Time
	LastSubmittedAt  time.Time
	LastSuccessAt    time.Time
	InFlightUntil    time.Time
	SkippedCoalesced int64
	OverdueCount     int64
}

// DueEngine evaluates recurring due times with injectable clock via Evaluate(now).
type DueEngine struct {
	mu      sync.Mutex
	runtime map[string]*AssignmentRuntime
}

// NewDueEngine constructs an empty due engine.
func NewDueEngine() *DueEngine {
	return &DueEngine{runtime: make(map[string]*AssignmentRuntime)}
}

// Reset clears runtime state (e.g. after generation apply).
func (e *DueEngine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.runtime = make(map[string]*AssignmentRuntime)
}

// SuppressCurrentCycle marks each definition's current due slot as already
// handled without submitting work. Used after Sensor restart (and optionally
// after desired-state apply) so recovery does not become a fleet-wide poll burst.
// The next real submission waits for the subsequent cadence slot.
func (e *DueEngine) SuppressCurrentCycle(now time.Time, defs []RecurringDefinition) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range defs {
		def := defs[i]
		phase, err := ComputePhase(def)
		if err != nil {
			continue
		}
		cadence := time.Duration(def.CadenceMs) * time.Millisecond
		due := currentDueTime(now, phase, cadence)
		rt := e.runtime[def.AssignmentID]
		if rt == nil {
			rt = &AssignmentRuntime{}
			e.runtime[def.AssignmentID] = rt
		}
		rt.LastSubmittedDue = due
		rt.LastSubmittedAt = now
		rt.InFlightUntil = time.Time{}
	}
}

// RuntimeSnapshot returns a copy of assignment runtime for tests/metrics.
func (e *DueEngine) RuntimeSnapshot(assignmentID string) (AssignmentRuntime, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	rt, ok := e.runtime[assignmentID]
	if !ok || rt == nil {
		return AssignmentRuntime{}, false
	}
	return *rt, true
}

// Evaluate returns at most one DueWork per definition for the CURRENT due slot.
// Missed cycles coalesce: historical dues are never enqueued; SkippedCoalesced
// increases by missed-1. Overlap is refused while InFlightUntil is in the future.
// Freshness debt is counted when last success/submit is older than freshnessMs.
func (e *DueEngine) Evaluate(now time.Time, defs []RecurringDefinition) []DueWork {
	e.mu.Lock()
	defer e.mu.Unlock()

	out := make([]DueWork, 0, len(defs))
	active := make(map[string]struct{}, len(defs))
	for i := range defs {
		def := defs[i]
		active[def.AssignmentID] = struct{}{}
		rt := e.runtime[def.AssignmentID]
		if rt == nil {
			rt = &AssignmentRuntime{}
			e.runtime[def.AssignmentID] = rt
		}

		phase, err := ComputePhase(def)
		if err != nil {
			continue
		}
		cadence := time.Duration(def.CadenceMs) * time.Millisecond
		currentDue := currentDueTime(now, phase, cadence)

		freshnessDebt := false
		anchor := rt.LastSuccessAt
		if anchor.IsZero() {
			anchor = rt.LastSubmittedAt
		}
		if !anchor.IsZero() {
			freshness := time.Duration(def.FreshnessMs) * time.Millisecond
			if now.Sub(anchor) > freshness {
				freshnessDebt = true
				rt.OverdueCount++
			}
		}

		if rt.InFlightUntil.After(now) {
			continue
		}
		if !rt.LastSubmittedDue.IsZero() && !rt.LastSubmittedDue.Before(currentDue) {
			// Already submitted for this (or a future) due slot.
			continue
		}

		var missedSkipped int64
		if !rt.LastSubmittedDue.IsZero() {
			elapsed := currentDue.Sub(rt.LastSubmittedDue)
			if elapsed > cadence {
				missed := int64(elapsed / cadence)
				if missed > 1 {
					missedSkipped = missed - 1
					rt.SkippedCoalesced += missedSkipped
				}
			}
		}

		out = append(out, DueWork{
			Definition:    def,
			CurrentDue:    currentDue,
			MissedSkipped: missedSkipped,
			FreshnessDebt: freshnessDebt,
		})
	}

	// Drop runtime for assignments no longer present.
	for id := range e.runtime {
		if _, ok := active[id]; !ok {
			delete(e.runtime, id)
		}
	}
	return out
}

// MarkSubmitted records that due work was admitted to the scheduler.
func (e *DueEngine) MarkSubmitted(assignmentID string, due, submittedAt, inFlightUntil time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	rt := e.runtime[assignmentID]
	if rt == nil {
		rt = &AssignmentRuntime{}
		e.runtime[assignmentID] = rt
	}
	rt.LastSubmittedDue = due
	rt.LastSubmittedAt = submittedAt
	rt.InFlightUntil = inFlightUntil
}

// MarkFinished clears in-flight fencing after executor completion.
func (e *DueEngine) MarkFinished(assignmentID string, successAt time.Time, ok bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	rt := e.runtime[assignmentID]
	if rt == nil {
		return
	}
	rt.InFlightUntil = time.Time{}
	if ok && !successAt.IsZero() {
		rt.LastSuccessAt = successAt
	}
}

// TotalSkippedCoalesced sums skipped counters across assignments.
func (e *DueEngine) TotalSkippedCoalesced() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	var n int64
	for _, rt := range e.runtime {
		n += rt.SkippedCoalesced
	}
	return n
}

// TotalOverdueCount sums overdue freshness counters.
func (e *DueEngine) TotalOverdueCount() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	var n int64
	for _, rt := range e.runtime {
		n += rt.OverdueCount
	}
	return n
}

// currentDueTime returns the phase-aligned due instant for the slot containing now.
// Uses floor division so negative (now < phase) is handled correctly.
func currentDueTime(now time.Time, phase, cadence time.Duration) time.Time {
	nowNs := now.UnixNano()
	phaseNs := phase.Nanoseconds()
	cadenceNs := cadence.Nanoseconds()
	if cadenceNs <= 0 {
		return time.Unix(0, phaseNs)
	}
	delta := nowNs - phaseNs
	var cycles int64
	if delta >= 0 {
		cycles = delta / cadenceNs
	} else {
		// Floor division for negative deltas.
		cycles = delta / cadenceNs
		if delta%cadenceNs != 0 {
			cycles--
		}
	}
	dueNs := phaseNs + cycles*cadenceNs
	return time.Unix(0, dueNs).UTC()
}
