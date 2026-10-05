// Registry fragment: plugin migration readiness codes (E118-E120), owned by
// P7-PLUG-59. Registered from init() through Register; see codes.go for the
// rules and codes_blocks.go for the allocation.
package errs

func init() {
	Register(
		CodeEntry{
			Code:       "E118",
			Category:   "plugin",
			Summary:    "Plugin migrations not applied in time",
			DefaultWhy: "The bounded wait ended before the plugin's /health reported every migration applied: it reported fewer applied than expected, or it never answered HTTP 200 (hang, error status, redirect). The plugin applies its own SQL at boot; the CLI never applies it.",
			DefaultFix: "Read the plugin logs (docker logs nself_<plugin>) for the failing migration. A slow first boot needs a longer wait: set NSELF_PLUGIN_READY_TIMEOUT (seconds).",
			DocsPath:   "reference/error-codes#e118",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E119",
			Category:   "plugin",
			Summary:    "Plugin health lacks migrations status",
			DefaultWhy: "The plugin declares migrations but its /health response has no valid \"migrations\" {applied, expected} field (absent, wrong type, negative, applied above expected, or a body over 1 MiB), so readiness cannot be proven.",
			DefaultFix: "Update the plugin to a release built on sdk/go/migrate, which serves the migrations field in /health.",
			DocsPath:   "reference/error-codes#e119",
			Exit:       2,
		},
		CodeEntry{
			Code:       "E120",
			Category:   "plugin",
			Summary:    "Plugin ships migrations without boot apply",
			DefaultWhy: "The plugin has a migrations/ directory but its manifest does not declare migrations.apply: boot, so nothing applies the SQL when the plugin starts.",
			DefaultFix: "Add \"migrations\": {\"dir\": \"migrations\", \"apply\": \"boot\"} to plugin.json and apply the files at boot with sdk/go/migrate.",
			DocsPath:   "reference/error-codes#e120",
			Exit:       1,
		},
	)
}
