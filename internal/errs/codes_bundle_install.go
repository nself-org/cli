// Registry fragment: bundle install app-ready codes (E135-E136), owned by
// P7-PLUG-19. Registered from init() through Register; see codes.go for the
// rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E135",
			Category:   "plugin",
			Summary:    "Bundle not app-ready",
			DefaultWhy: "Every plugin of the bundle was installed, but a later step did not finish: reconcile, start, readiness (boot migrations applied), tracking (every manifest table tracked in Hasura) or health. The message names the bundle, the step and the plugin.",
			DefaultFix: "Read the failing step in the report, fix its cause (docker logs nself_<plugin> for readiness), then run the bundle install again; plugins that are already installed are not reinstalled.",
			DocsPath:   "reference/error-codes#e135",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E136",
			Category:   "plugin",
			Summary:    "Bundle ledger lists a missing plugin",
			DefaultWhy: "The bundle install ledger records a plugin whose directory or plugin.json is gone from the plugin directory, so the ledger and the disk disagree.",
			DefaultFix: "Reinstall the bundle with: nself bundle install <bundle>. Removing the bundle with nself bundle remove <bundle> also clears the record.",
			DocsPath:   "reference/error-codes#e136",
			Exit:       1,
		},
	)
}
