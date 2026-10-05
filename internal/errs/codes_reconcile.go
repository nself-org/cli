// Registry fragment: change-plan codes (E450-E454, P7-LIVE-03). E452-E454 are
// reserved for the reconcile Epic and stay unregistered until a producer
// exists. Registered from init() through Register; see codes.go for the rules
// and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E450",
			Category:   "reconcile",
			Summary:    "Plan id does not match",
			DefaultWhy: "The plan you confirmed no longer matches what this command would change, because an input changed or the plan id is not one this project produces.",
			DefaultFix: "Re-run nself build --plan and pass the new plan_id.",
			DocsPath:   "reference/error-codes#e450",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E451",
			Category:   "reconcile",
			Summary:    "Plan id cannot bind generated secrets",
			DefaultWhy: "This build generates secrets, and random values cannot be reproduced from a plan id.",
			DefaultFix: "Set the secrets in .env.secrets first, or run nself build and confirm at the prompt (or with --yes, without --plan-id).",
			DocsPath:   "reference/error-codes#e451",
			Exit:       1,
		},
	)
}
