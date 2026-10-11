// Registry fragment: plugin install table tracking code (E140), owned by
// P7-PLUG-78. Registered from init() through Register; see codes.go for the
// rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E140",
			Category:   "plugin",
			Summary:    "Plugin tables not tracked in Hasura",
			DefaultWhy: "The plugin installed and reported its boot migrations applied, but the CLI could not track its manifest tables in Hasura (Hasura unreachable, the admin call failed, or a table the manifest lists does not exist in Postgres), so the plugin's tables are not queryable through GraphQL. The message names the plugin, the untracked tables and the underlying error.",
			DefaultFix: "Run the install again: tracking is idempotent and only tracks what is still missing. Check that Hasura is up with `nself db hasura` (status) and that the plugin applied its migrations (docker logs nself_<plugin>).",
			DocsPath:   "reference/error-codes#e140",
			Exit:       2,
		},
	)
}
