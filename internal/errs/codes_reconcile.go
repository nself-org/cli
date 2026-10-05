// Registry fragment: change-plan codes (E450-E454, P7-LIVE-03). E451-E454 are
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
	)
}
