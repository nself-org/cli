// Registry fragment: deploy environment scoping code (E483, P7-DEPL-12).
// Registered from init() through Register; see codes.go for the rules and
// codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E483",
			Category:   "deploy",
			Summary:    "Unknown deploy environment",
			DefaultWhy: "The environment you named is not in the deploy inventory (.nself/control-plane.yaml or NSELF_DEPLOY_HOST_<ENV>), so nothing was deployed. A deploy never falls back to another environment.",
			DefaultFix: "Run nself deploy environments to list the known environments, then re-run with one of them, or add the environment with nself env target add.",
			DocsPath:   "reference/error-codes#e483",
			Exit:       1,
		},
	)
}
