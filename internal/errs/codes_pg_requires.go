// Registry fragment: plugin Postgres-extension requirement codes (E507-E508,
// P7-ADOPT-06). Registered from init() through Register; see codes.go for the
// rules. The only producer is internal/plugin/requires.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E507",
			Category:   "adopt",
			Summary:    "Required Postgres extension unavailable",
			DefaultWhy: "The plugin declares requires.postgres_extensions and the project's Postgres (the running cluster, or the configured image when it is stopped) does not provide one of them.",
			DefaultFix: "Use a Postgres image that ships the extension; for vector run `nself db image switch --to pgvector`.",
			DocsPath:   "reference/error-codes#e507",
			Exit:       1,
		},
		CodeEntry{
			Code:       "E508",
			Category:   "adopt",
			Summary:    "Postgres extension availability unknown for a custom image (warning)",
			DefaultWhy: "The postgres image is neither postgres:* nor pgvector/pgvector:* and the database is not running, so nself cannot tell which extensions it ships.",
			DefaultFix: "Start the stack so the cluster can be checked, or use an image that ships the extension. The install continues.",
			DocsPath:   "reference/error-codes#e508",
			Exit:       1,
		},
	)
}
