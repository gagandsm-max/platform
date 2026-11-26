package diagnostics

import (
	"math"
	"testing"
	"time"
)

var start = time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)

func at(seconds int) time.Time {
	return start.Add(time.Duration(seconds) * time.Second)
}

func testPolicy() Policy {
	return Policy{
		Binding: Binding{
			PolicyID: "policy", Version: 1, Generation: 7, TriggerID: "cpu-pressure",
			TenantID: "tenant", EntityID: "host", MetricKey: "cpu",
		},
		Condition: HighSide, TriggerThreshold: 80, RecoveryThreshold: 70,
		Qualification: 10 * time.Second, Cooldown: 15 * time.Second,
		Recovery: 5 * time.Second, MaxObservationAge: 10 * time.Second,
	}
}

func sample(policy Policy, seconds int, value float64) *Observation {
	return &Observation{
		TenantID: policy.TenantID, EntityID: policy.EntityID, MetricKey: policy.MetricKey,
		ObservedAt: at(seconds), Value: value,
	}
}

func check(t *testing.T, got Decision, phase Phase, reason Reason) {
	t.Helper()
	if got.Phase != phase || got.Reason != reason {
		t.Fatalf("got (%s, %s), want (%s, %s)", got.Phase, got.Reason, phase, reason)
	}
}

func TestEligibilityTransitions(t *testing.T) {
	policy := testPolicy()
	authority := &Authority{Binding: policy.Binding, ExpiresAt: at(100)}
	var state State
	var eligible Decision
	for _, step := range []struct {
		name   string
		second int
		value  float64
		record bool
		phase  Phase
		reason Reason
	}{
		{"first crossing", 0, 80, false, Qualifying, ReasonQualifying},
		{"one second short", 9, 90, false, Qualifying, ReasonQualifying},
		{"inside hysteresis interrupts qualification", 10, 79, false, Ineligible, ReasonBelowThreshold},
		{"fresh qualification", 11, 80, false, Qualifying, ReasonQualifying},
		{"exact duration", 21, 80, false, CaptureEligible, ReasonEligible},
		{"record once", 21, 0, true, Cooldown, ReasonCaptureRecorded},
		{"replay during cooldown", 22, 0, true, Cooldown, ReasonStaleCapture},
		{"recovery candidate", 25, 70, false, Cooldown, ReasonCooldown},
		{"hysteresis band interrupts recovery", 30, 71, false, Cooldown, ReasonCooldown},
		{"cooldown elapsed without recovery", 36, 90, false, Cooldown, ReasonAwaitingRecovery},
		{"new recovery candidate", 37, 69, false, Cooldown, ReasonAwaitingRecovery},
		{"before recovery boundary", 41, 70, false, Cooldown, ReasonAwaitingRecovery},
		{"exact recovery boundary", 42, 70, false, Ineligible, ReasonRecovered},
		{"qualify after recovery", 43, 90, false, Qualifying, ReasonQualifying},
		{"second eligibility", 53, 90, false, CaptureEligible, ReasonEligible},
	} {
		t.Run(step.name, func(t *testing.T) {
			var got Decision
			if step.record {
				before := state
				state, got = RecordCapture(state, authority, at(step.second), eligible)
				if step.reason == ReasonStaleCapture && state != before {
					t.Fatal("replay changed state")
				}
				if state.lastCaptureAt != at(21) || state.cooldownUntil != at(36) {
					t.Fatal("capture timestamps did not match the original capture")
				}
			} else {
				state, got = Evaluate(state, policy, authority, at(step.second), sample(policy, step.second, step.value))
				if got.Reason == ReasonEligible {
					eligible = got
				}
			}
			check(t, got, step.phase, step.reason)
		})
	}
}

func TestInvalidPolicies(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Policy)
		reason Reason
	}{
		{"unsupported", func(p *Policy) { p.Condition = "LOW_SIDE" }, ReasonUnsupportedCondition},
		{"zero version", func(p *Policy) { p.Version = 0 }, ReasonInvalidPolicy},
		{"zero generation", func(p *Policy) { p.Generation = 0 }, ReasonInvalidPolicy},
		{"zero qualification", func(p *Policy) { p.Qualification = 0 }, ReasonInvalidPolicy},
		{"negative qualification", func(p *Policy) { p.Qualification = -1 }, ReasonInvalidPolicy},
		{"zero cooldown", func(p *Policy) { p.Cooldown = 0 }, ReasonInvalidPolicy},
		{"negative cooldown", func(p *Policy) { p.Cooldown = -1 }, ReasonInvalidPolicy},
		{"zero recovery", func(p *Policy) { p.Recovery = 0 }, ReasonInvalidPolicy},
		{"negative recovery", func(p *Policy) { p.Recovery = -1 }, ReasonInvalidPolicy},
		{"zero freshness", func(p *Policy) { p.MaxObservationAge = 0 }, ReasonInvalidPolicy},
		{"negative freshness", func(p *Policy) { p.MaxObservationAge = -1 }, ReasonInvalidPolicy},
		{"NaN trigger", func(p *Policy) { p.TriggerThreshold = math.NaN() }, ReasonInvalidPolicy},
		{"infinite recovery threshold", func(p *Policy) { p.RecoveryThreshold = math.Inf(-1) }, ReasonInvalidPolicy},
		{"equal thresholds", func(p *Policy) { p.RecoveryThreshold = p.TriggerThreshold }, ReasonInvalidPolicy},
		{"reversed thresholds", func(p *Policy) { p.RecoveryThreshold = p.TriggerThreshold + 1 }, ReasonInvalidPolicy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := testPolicy()
			tc.change(&policy)
			authority := &Authority{Binding: policy.Binding, ExpiresAt: at(100)}
			_, got := Evaluate(State{}, policy, authority, at(0), sample(policy, 0, 90))
			check(t, got, Ineligible, tc.reason)
		})
	}
}

func TestObservationInterruptions(t *testing.T) {
	policy := testPolicy()
	authority := &Authority{Binding: policy.Binding, ExpiresAt: at(100)}
	wrongEntity := sample(policy, 4, 90)
	wrongEntity.EntityID = "other"
	for _, tc := range []struct {
		name   string
		now    int
		sample *Observation
		reason Reason
	}{
		{"no data", 4, nil, ReasonNoData},
		{"unknown value", 4, sample(policy, 4, math.NaN()), ReasonInvalidObservation},
		{"infinite value", 4, sample(policy, 4, math.Inf(1)), ReasonInvalidObservation},
		{"wrong entity", 4, wrongEntity, ReasonObservationMismatch},
		{"future", 4, sample(policy, 5, 90), ReasonInvalidObservation},
		{"duplicate", 4, sample(policy, 0, 90), ReasonObservationOrder},
		{"out of order", 4, sample(policy, -1, 90), ReasonObservationOrder},
		{"clock rollback", -1, sample(policy, -1, 90), ReasonInvalidTime},
		{"stale", 21, sample(policy, 10, 90), ReasonStaleObservation},
		{"fresh after excessive gap", 11, sample(policy, 11, 90), ReasonQualifying},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state, _ := Evaluate(State{}, policy, authority, at(0), sample(policy, 0, 90))
			state, got := Evaluate(state, policy, authority, at(tc.now), tc.sample)
			want := Ineligible
			if tc.reason == ReasonQualifying {
				want = Qualifying
			}
			check(t, got, want, tc.reason)
			next := max(10, tc.now+1)
			_, got = Evaluate(state, policy, authority, at(next), sample(policy, next, 90))
			check(t, got, Qualifying, ReasonQualifying)
		})
	}
}

func TestAuthorityAndPolicyReset(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Policy, *Authority) *Authority
		phase  Phase
		reason Reason
	}{
		{"absent", func(_ *Policy, _ *Authority) *Authority { return nil }, Ineligible, ReasonAuthorityUnavailable},
		{"expired", func(_ *Policy, a *Authority) *Authority { a.ExpiresAt = at(10); return a }, Ineligible, ReasonAuthorityExpired},
		{"mismatched generation", func(_ *Policy, a *Authority) *Authority { a.Generation++; return a }, Ineligible, ReasonAuthorityMismatch},
		{"new generation", func(p *Policy, a *Authority) *Authority { p.Generation++; a.Binding = p.Binding; return a }, Qualifying, ReasonQualifying},
		{"new policy version", func(p *Policy, a *Authority) *Authority { p.Version++; a.Binding = p.Binding; return a }, Qualifying, ReasonQualifying},
		{"new metric identity", func(p *Policy, a *Authority) *Authority { p.MetricKey = "memory"; a.Binding = p.Binding; return a }, Qualifying, ReasonQualifying},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := testPolicy()
			authority := Authority{Binding: policy.Binding, ExpiresAt: at(100)}
			state, _ := Evaluate(State{}, policy, &authority, at(0), sample(policy, 0, 90))
			current := tc.change(&policy, &authority)
			state, got := Evaluate(state, policy, current, at(10), sample(policy, 10, 90))
			check(t, got, tc.phase, tc.reason)
			fresh := &Authority{Binding: policy.Binding, ExpiresAt: at(100)}
			_, got = Evaluate(state, policy, fresh, at(11), sample(policy, 11, 90))
			check(t, got, Qualifying, ReasonQualifying)
		})
	}
}

func TestCaptureDecisionMustStillBeCurrent(t *testing.T) {
	policy := testPolicy()
	authority := Authority{Binding: policy.Binding, ExpiresAt: at(18)}
	state, _ := Evaluate(State{}, policy, &authority, at(0), sample(policy, 0, 90))
	state, old := Evaluate(state, policy, &authority, at(10), sample(policy, 10, 90))
	state, current := Evaluate(state, policy, &authority, at(11), sample(policy, 11, 90))
	replacement := Authority{Binding: policy.Binding, ExpiresAt: at(100)}
	mismatch := replacement
	mismatch.Generation++
	for _, tc := range []struct {
		name      string
		authority *Authority
		now       int
		decision  Decision
		reason    Reason
	}{
		{"superseded sample", &authority, 11, old, ReasonStaleCapture},
		{"wrong generation", &mismatch, 11, current, ReasonAuthorityMismatch},
		{"absent", nil, 11, current, ReasonAuthorityUnavailable},
		{"expired", &authority, 18, current, ReasonAuthorityExpired},
		{"authority lapse", &replacement, 18, current, ReasonStaleCapture},
		{"backdated", &authority, 10, current, ReasonInvalidTime},
		{"empty decision", &authority, 11, Decision{}, ReasonStaleCapture},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next, got := RecordCapture(state, tc.authority, at(tc.now), tc.decision)
			check(t, got, Ineligible, tc.reason)
			if next != state {
				t.Fatal("rejected recording changed state")
			}
		})
	}
	_, got := Evaluate(state, policy, &replacement, at(18), sample(policy, 18, 90))
	check(t, got, Qualifying, ReasonQualifying)
}

func TestStateRetainsOnlyCurrentBinding(t *testing.T) {
	policy := testPolicy()
	var state State
	for second := range 1000 {
		policy.Version++
		authority := &Authority{Binding: policy.Binding, ExpiresAt: at(second + 1)}
		var got Decision
		state, got = Evaluate(state, policy, authority, at(second), sample(policy, second, 90))
		check(t, got, Qualifying, ReasonQualifying)
		want := State{
			policy: policy, phase: Qualifying, qualifyingSince: at(second),
			lastObservedAt: at(second), lastEvaluatedAt: at(second), authorityExpiry: authority.ExpiresAt,
		}
		if state != want {
			t.Fatal("state retained history across policy versions")
		}
	}
}

func TestCaptureFreshnessAndCooldownAcrossPolicyRefresh(t *testing.T) {
	policy := testPolicy()
	authority := Authority{Binding: policy.Binding, ExpiresAt: at(100)}
	state, _ := Evaluate(State{}, policy, &authority, at(0), sample(policy, 0, 90))
	state, eligible := Evaluate(state, policy, &authority, at(10), sample(policy, 10, 90))
	for _, tc := range []struct {
		name   string
		second int
		phase  Phase
		reason Reason
	}{
		{"one second past freshness", 21, Ineligible, ReasonStaleCapture},
		{"exact freshness boundary", 20, Cooldown, ReasonCaptureRecorded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, got := RecordCapture(state, &authority, at(tc.second), eligible)
			check(t, got, tc.phase, tc.reason)
		})
	}
	state, _ = RecordCapture(state, &authority, at(20), eligible)
	policy.Version++
	authority.Binding = policy.Binding

	state, got := Evaluate(state, policy, &authority, at(21), sample(policy, 19, 70))
	check(t, got, Cooldown, ReasonObservationOrder)
	state, got = Evaluate(state, policy, &authority, at(25), sample(policy, 25, 70))
	check(t, got, Cooldown, ReasonCooldown)
	state, got = Evaluate(state, policy, &authority, at(34), sample(policy, 34, 70))
	check(t, got, Cooldown, ReasonCooldown)
	_, got = Evaluate(state, policy, &authority, at(35), sample(policy, 35, 70))
	check(t, got, Ineligible, ReasonRecovered)
}
