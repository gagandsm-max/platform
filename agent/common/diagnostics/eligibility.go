// Package diagnostics evaluates local Endpoint Diagnostic Snapshot eligibility.
package diagnostics

import (
	"math"
	"time"
)

type Phase string

const (
	Ineligible      Phase = "INELIGIBLE"
	Qualifying      Phase = "QUALIFYING"
	CaptureEligible Phase = "CAPTURE_ELIGIBLE"
	Cooldown        Phase = "COOLDOWN"
)

type Reason string

const (
	ReasonEligible             Reason = "ELIGIBLE"
	ReasonQualifying           Reason = "QUALIFYING"
	ReasonBelowThreshold       Reason = "BELOW_THRESHOLD"
	ReasonCooldown             Reason = "SUPPRESSED_COOLDOWN"
	ReasonAwaitingRecovery     Reason = "SUPPRESSED_AWAITING_RECOVERY"
	ReasonRecovered            Reason = "RECOVERED"
	ReasonInvalidPolicy        Reason = "INVALID_POLICY"
	ReasonUnsupportedCondition Reason = "UNSUPPORTED_CONDITION"
	ReasonInvalidTime          Reason = "INVALID_TIME"
	ReasonAuthorityUnavailable Reason = "AUTHORITY_UNAVAILABLE"
	ReasonAuthorityMismatch    Reason = "AUTHORITY_MISMATCH"
	ReasonAuthorityExpired     Reason = "AUTHORITY_EXPIRED"
	ReasonNoData               Reason = "OBSERVATION_UNAVAILABLE"
	ReasonInvalidObservation   Reason = "INVALID_OBSERVATION"
	ReasonObservationMismatch  Reason = "OBSERVATION_MISMATCH"
	ReasonStaleObservation     Reason = "STALE_OBSERVATION"
	ReasonObservationOrder     Reason = "OBSERVATION_DUPLICATE_OR_OUT_OF_ORDER"
	ReasonStaleCapture         Reason = "SUPPRESSED_STALE_CAPTURE_DECISION"
	ReasonCaptureRecorded      Reason = "CAPTURE_RECORDED"
)

type Condition string

const HighSide Condition = "HIGH_SIDE"

// Binding identifies one effective trigger and the authority generation that owns it.
type Binding struct {
	PolicyID   string
	Version    uint64
	Generation uint64
	TriggerID  string
	TenantID   string
	EntityID   string
	MetricKey  string
}

// Policy defines inclusive high-side trigger and low-side recovery boundaries.
// MaxObservationAge also bounds gaps between qualifying or recovering samples.
type Policy struct {
	Binding
	Condition         Condition
	TriggerThreshold  float64
	RecoveryThreshold float64
	Qualification     time.Duration
	Cooldown          time.Duration
	Recovery          time.Duration
	MaxObservationAge time.Duration
}

// Authority is current, validated Core evidence supplied by the caller.
// A nil pointer represents absent or revoked authority.
type Authority struct {
	Binding
	ExpiresAt time.Time
}

// Observation carries a normalized scalar for the bound series. The caller
// supplies nil for missing or unknown data and resolves any series dimensions
// before calling Evaluate.
type Observation struct {
	TenantID   string
	EntityID   string
	MetricKey  string
	ObservedAt time.Time
	Value      float64
}

// State holds one binding. The caller threads the returned value through
// serialized transitions. Its zero value represents an ineligible binding.
type State struct {
	policy          Policy
	phase           Phase
	qualifyingSince time.Time
	lastObservedAt  time.Time
	lastEvaluatedAt time.Time
	authorityExpiry time.Time
	lastCaptureAt   time.Time
	cooldownUntil   time.Time
	recoverySince   time.Time
}

type Decision struct {
	Phase  Phase
	Reason Reason

	binding    Binding
	observedAt time.Time
}

// Evaluate advances eligibility using the supplied evaluation time and sample.
func Evaluate(state State, policy Policy, authority *Authority, now time.Time, observation *Observation) (State, Decision) {
	if now.IsZero() || now.Before(state.lastEvaluatedAt) {
		return interrupt(state, ReasonInvalidTime)
	}
	state.lastEvaluatedAt = now
	if reason := policyProblem(policy); reason != "" {
		return interrupt(state, reason)
	}
	if reason := authorityProblem(policy.Binding, authority, now); reason != "" {
		return interrupt(state, reason)
	}
	if state.policy != policy || !now.Before(state.authorityExpiry) {
		state = clearQualification(state)
		state.policy = policy
	}
	state.authorityExpiry = authority.ExpiresAt

	if observation == nil {
		return interrupt(state, ReasonNoData)
	}
	if observation.TenantID != policy.TenantID || observation.EntityID != policy.EntityID || observation.MetricKey != policy.MetricKey {
		return interrupt(state, ReasonObservationMismatch)
	}
	observedAt := observation.ObservedAt
	if observedAt.IsZero() || observedAt.After(now) {
		return interrupt(state, ReasonInvalidObservation)
	}
	if !observedAt.After(state.lastObservedAt) || !observedAt.After(state.lastCaptureAt) {
		return interrupt(state, ReasonObservationOrder)
	}
	previous := state.lastObservedAt
	state.lastObservedAt = observedAt
	if !finite(observation.Value) {
		return interrupt(state, ReasonInvalidObservation)
	}
	if now.Sub(observedAt) > policy.MaxObservationAge {
		return interrupt(state, ReasonStaleObservation)
	}
	if !previous.IsZero() && observedAt.Sub(previous) > policy.MaxObservationAge {
		state = clearQualification(state)
	}

	if state.phase == Cooldown {
		return recoverBinding(state, now, observedAt, observation.Value)
	}
	if observation.Value < policy.TriggerThreshold {
		return interrupt(state, ReasonBelowThreshold)
	}
	if state.qualifyingSince.IsZero() {
		state.qualifyingSince = observedAt
	}
	if observedAt.Sub(state.qualifyingSince) < policy.Qualification {
		state.phase = Qualifying
		return state, decision(state, ReasonQualifying)
	}
	state.phase = CaptureEligible
	return state, Decision{
		Phase: CaptureEligible, Reason: ReasonEligible,
		binding: policy.Binding, observedAt: observedAt,
	}
}

// RecordCapture consumes a current eligibility decision and starts cooldown.
// Rejections leave state intact. Callers use the returned state for every
// subsequent evaluation or recording.
func RecordCapture(state State, authority *Authority, now time.Time, eligible Decision) (State, Decision) {
	if now.IsZero() || now.Before(state.lastEvaluatedAt) {
		return state, Decision{Phase: Ineligible, Reason: ReasonInvalidTime}
	}
	if reason := authorityProblem(state.policy.Binding, authority, now); reason != "" {
		return state, Decision{Phase: Ineligible, Reason: reason}
	}
	if state.phase != CaptureEligible || eligible.binding != state.policy.Binding ||
		!eligible.observedAt.Equal(state.lastObservedAt) || !eligible.observedAt.After(state.lastCaptureAt) ||
		!now.Before(state.authorityExpiry) || now.Sub(state.lastObservedAt) > state.policy.MaxObservationAge {
		return state, decision(clearQualification(state), ReasonStaleCapture)
	}

	state.phase = Cooldown
	state.lastEvaluatedAt = now
	state.lastCaptureAt = now
	state.cooldownUntil = now.Add(state.policy.Cooldown)
	state.qualifyingSince = time.Time{}
	return state, decision(state, ReasonCaptureRecorded)
}

func recoverBinding(state State, now, observedAt time.Time, value float64) (State, Decision) {
	if value > state.policy.RecoveryThreshold {
		state.recoverySince = time.Time{}
	} else if state.recoverySince.IsZero() {
		state.recoverySince = observedAt
	}
	if now.Before(state.cooldownUntil) {
		return state, decision(state, ReasonCooldown)
	}
	if state.recoverySince.IsZero() || observedAt.Sub(state.recoverySince) < state.policy.Recovery {
		return state, decision(state, ReasonAwaitingRecovery)
	}
	state.phase = Ineligible
	state.recoverySince = time.Time{}
	return state, decision(state, ReasonRecovered)
}

func interrupt(state State, reason Reason) (State, Decision) {
	state = clearQualification(state)
	return state, decision(state, reason)
}

func clearQualification(state State) State {
	state.qualifyingSince = time.Time{}
	state.recoverySince = time.Time{}
	if state.phase != Cooldown {
		state.phase = Ineligible
	}
	return state
}

func decision(state State, reason Reason) Decision {
	phase := state.phase
	if phase == "" {
		phase = Ineligible
	}
	return Decision{Phase: phase, Reason: reason}
}

func policyProblem(policy Policy) Reason {
	if policy.Condition != HighSide {
		return ReasonUnsupportedCondition
	}
	binding := policy.Binding
	if binding.PolicyID == "" || binding.Version == 0 || binding.Generation == 0 ||
		binding.TriggerID == "" || binding.TenantID == "" || binding.EntityID == "" || binding.MetricKey == "" ||
		!finite(policy.TriggerThreshold) || !finite(policy.RecoveryThreshold) ||
		policy.RecoveryThreshold >= policy.TriggerThreshold || policy.Qualification <= 0 ||
		policy.Cooldown <= 0 || policy.Recovery <= 0 || policy.MaxObservationAge <= 0 {
		return ReasonInvalidPolicy
	}
	return ""
}

func authorityProblem(binding Binding, authority *Authority, now time.Time) Reason {
	if authority == nil || authority.ExpiresAt.IsZero() {
		return ReasonAuthorityUnavailable
	}
	if authority.Binding != binding {
		return ReasonAuthorityMismatch
	}
	if !now.Before(authority.ExpiresAt) {
		return ReasonAuthorityExpired
	}
	return ""
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
