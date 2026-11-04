package controlplane

import (
	"fmt"

	"github.com/nself-org/cli/internal/errs"
)

// ValidDataPolicyAccept values accepted in DataPolicy.Accept.
const (
	DataPolicyAcceptSeed   = "seed"
	DataPolicyAcceptMasked = "masked"
)

// validDataPolicyRules are the permitted values for MaskRule.Rule.
var validDataPolicyRules = map[string]bool{
	"null":       true,
	"hash":       true,
	"fake_email": true,
	"fixed":      true,
}

// forbiddenMaskColumns are columns that may never be masked.
var forbiddenMaskColumns = map[string]bool{
	"tenant_id":         true,
	"source_account_id": true,
	"id":                true,
}

// EffectiveAccept returns the seed-acceptance policy for an environment.
// If env has no DataPolicy, every non-prod environment accepts "seed" and
// prod environments return an empty slice (no accept field permitted).
// When a policy is present, its Accept list is returned as-is.
func EffectiveAccept(env Environment) []string {
	if env.DataPolicy != nil && len(env.DataPolicy.Accept) > 0 {
		out := make([]string, len(env.DataPolicy.Accept))
		copy(out, env.DataPolicy.Accept)
		return out
	}
	if env.Tier == TierProd {
		return nil
	}
	return []string{DataPolicyAcceptSeed}
}

// ValidateDataPolicy checks that a DataPolicy attached to env obeys the
// contract:
// - unknown rules are rejected
// - accept must be empty on prod environments
// - no mask rule may target tenant_id, source_account_id, or a primary
// key column named id
//
// The error is E485 and names the offending field.
func ValidateDataPolicy(env Environment, envName string) error {
	if env.DataPolicy == nil {
		return nil
	}
	dp := env.DataPolicy

	// prod environments must not carry an accept field.
	if env.Tier == TierProd && len(dp.Accept) > 0 {
		return errs.New("E485", fmt.Sprintf("environments.%s.data_policy.accept: prod tier must not set accept", envName))
	}

	// validate accept values
	for i, a := range dp.Accept {
		if a != DataPolicyAcceptSeed && a != DataPolicyAcceptMasked {
			return errs.New("E485", fmt.Sprintf("environments.%s.data_policy.accept[%d]: unknown value %q", envName, i, a))
		}
	}

	// validate mask rules
	for i, m := range dp.Mask {
		if !validDataPolicyRules[m.Rule] {
			return errs.New("E485", fmt.Sprintf("environments.%s.data_policy.mask[%d].rule: unknown rule %q", envName, i, m.Rule))
		}
		if forbiddenMaskColumns[m.Column] {
			return errs.New("E485", fmt.Sprintf("environments.%s.data_policy.mask[%d].column: %q may not be masked", envName, i, m.Column))
		}
	}

	return nil
}
