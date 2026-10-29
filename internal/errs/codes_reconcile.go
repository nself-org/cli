// Registry fragment: change-plan codes (E450-E454, P7-LIVE-03).
// Registered from init() through Register; see codes.go for the rules
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
			DefaultWhy: "This build generates secrets, and the random values change the plan itself, so they cannot be reproduced from a plan id.",
			DefaultFix: "Set the secrets in .env.secrets first, or run nself build and confirm at the prompt (or with --yes, without --plan-id).",
			DocsPath:   "reference/error-codes#e451",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E452",
			Category:   "reconcile",
			Summary:    "Planned files were not written",
			DefaultWhy: "After the build, a file the confirmed plan listed is missing, has other content, or has another mode.",
			DefaultFix: "Re-run nself build --plan to see what differs, then run nself build again; check that the project is writable.",
			DocsPath:   "reference/error-codes#e452",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E453",
			Category:   "reconcile",
			Summary:    "Plan id cannot bind plugin changes",
			DefaultWhy: "The plan installs or removes plugins, and what those change is only known after they run, so a plan id cannot describe the final render.",
			DefaultFix: "Run nself build --yes without --plan-id (the render after the plugin changes is printed and held), or install or remove the plugins first.",
			DocsPath:   "reference/error-codes#e453",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E454",
			Category:   "reconcile",
			Summary:    "Plan id cannot bind certificate or trust effects",
			DefaultWhy: "The plan changes certificates or host trust outside local development (a prod-class env, a non-local domain or an unnamed trust change); a plan id binds only local development certificates and the mkcert CA named by its fingerprint.",
			DefaultFix: "Run nself build and confirm at the prompt (or with --yes, without --plan-id), or provision certificates and host trust first.",
			DocsPath:   "reference/error-codes#e454",
			Exit:       1,
		},
	)
}
