package recurring

import (
	"fmt"
	"strings"
)

const (
	ErrCodeStateRegression = "SENSOR_RECURRING_STATE_REGRESSION"
	ErrCodeStateConflict   = "SENSOR_RECURRING_STATE_CONFLICT"
	ErrCodeTenantMismatch  = "SENSOR_RECURRING_TENANT_MISMATCH"
)

// ApplyResult describes the outcome of ApplyDesired.
type ApplyResult struct {
	Accepted   bool
	Noop       bool
	Rejected   bool
	ReasonCode string
}

// ApplyDesired reconciles desired into applied under generation fencing.
//
//	desired.Generation < current.Generation → reject stale
//	same generation, different checksum → reject conflict
//	same generation + checksum → idempotent no-op
//	higher generation → accept
//	wrong tenant on any definition vs expectedTenant → reject
func ApplyDesired(current AppliedState, desired DesiredState, expectedTenant string) (AppliedState, ApplyResult, error) {
	if err := ValidateDesiredState(desired); err != nil {
		return current, ApplyResult{Rejected: true, ReasonCode: ErrCodeInvalidDesired}, err
	}
	expectedTenant = strings.TrimSpace(expectedTenant)
	for _, def := range desired.Definitions {
		if expectedTenant != "" && def.TenantID != expectedTenant {
			err := fmt.Errorf("%s: definition tenant %q does not match expected %q", ErrCodeTenantMismatch, def.TenantID, expectedTenant)
			return current, ApplyResult{Rejected: true, ReasonCode: ErrCodeTenantMismatch}, err
		}
	}

	if desired.Generation < current.Generation {
		err := fmt.Errorf("%s: desired generation %d is stale versus applied %d", ErrCodeStateRegression, desired.Generation, current.Generation)
		return current, ApplyResult{Rejected: true, ReasonCode: ErrCodeStateRegression}, err
	}
	if desired.Generation == current.Generation {
		if desired.Checksum != current.Checksum {
			err := fmt.Errorf("%s: same generation %d with conflicting checksum", ErrCodeStateConflict, desired.Generation)
			return current, ApplyResult{Rejected: true, ReasonCode: ErrCodeStateConflict}, err
		}
		return current, ApplyResult{Accepted: true, Noop: true}, nil
	}

	next := AppliedState{
		Generation:               desired.Generation,
		Checksum:                 desired.Checksum,
		Definitions:              append([]RecurringDefinition(nil), desired.Definitions...),
		CoreAuthorityEstablished: true,
	}
	if next.Definitions == nil {
		next.Definitions = []RecurringDefinition{}
	}
	return next, ApplyResult{Accepted: true}, nil
}
